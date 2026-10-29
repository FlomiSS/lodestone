package node

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

// Backup ist ein Archiv des Serverordners.
type Backup struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Size     int64  `json:"size"`
	Created  int64  `json:"created"`
	Creating bool   `json:"creating,omitempty"`
}

var (
	backupName  = regexp.MustCompile(`^(\d{8}-\d{6})(?:-([a-z0-9-]{1,40}))?\.tar\.gz$`)
	labelClean  = regexp.MustCompile(`[^a-z0-9]+`)
	errBusy     = fmt.Errorf("%w: es läuft gerade ein Backup oder eine Wiederherstellung", ErrBadState)
	restoreTemp = ".lodestone-restore"
)

func (s *Server) backupDir() string { return filepath.Join(s.m.cfg.DataDir, "backups", s.spec.ID) }

// ListBackups listet die Backups, das neueste zuerst. Noch laufende stehen mit creating.
func (s *Server) ListBackups() ([]Backup, error) {
	entries, err := os.ReadDir(s.backupDir())
	if errors.Is(err, fs.ErrNotExist) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	list := []Backup{}
	for _, entry := range entries {
		name, creating := entry.Name(), false
		if strings.HasSuffix(name, ".part") {
			name, creating = strings.TrimSuffix(name, ".part"), true
		}
		match := backupName.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		created, _ := time.ParseInLocation("20060102-150405", match[1], time.UTC)
		list = append(list, Backup{Name: name, Label: match[2], Size: info.Size(), Created: created.UnixMilli(), Creating: creating})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Created != list[j].Created {
			return list[i].Created > list[j].Created
		}
		return list[i].Name > list[j].Name
	})
	return list, nil
}

// CreateBackup startet ein Backup im Hintergrund und gibt den Eintrag zurück.
func (s *Server) CreateBackup(label string) (Backup, error) {
	if !s.backupBusy.CompareAndSwap(false, true) {
		return Backup{}, errBusy
	}
	label = strings.Trim(labelClean.ReplaceAllString(strings.ToLower(label), "-"), "-")
	if len(label) > 40 {
		label = strings.Trim(label[:40], "-")
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	name := stamp + ".tar.gz"
	if label != "" {
		name = stamp + "-" + label + ".tar.gz"
	}
	if err := os.MkdirAll(s.backupDir(), 0o755); err != nil {
		s.backupBusy.Store(false)
		return Backup{}, err
	}
	go s.runBackup(name)
	return Backup{Name: name, Label: label, Created: time.Now().UnixMilli(), Creating: true}, nil
}

func (s *Server) runBackup(name string) {
	defer s.backupBusy.Store(false)
	s.logf("Backup %s wird erstellt", name)
	quiet := s.quiesce()
	part := filepath.Join(s.backupDir(), name+".part")
	err := s.writeArchive(part)
	if quiet {
		_ = s.Command("save-on")
	}
	if err == nil {
		err = os.Rename(part, filepath.Join(s.backupDir(), name))
	}
	if err != nil {
		_ = os.Remove(part)
		s.logf("Backup fehlgeschlagen: %v", err)
		return
	}
	s.logf("Backup %s fertig", name)
	s.pruneBackups()
}

// quiesce hält einen laufenden Server an, Änderungen auf die Platte zu schreiben, damit das Archiv stimmig ist.
func (s *Server) quiesce() bool {
	s.mu.Lock()
	running := s.state == StateRunning
	s.mu.Unlock()
	if info, ok := software.Lookup(s.spec.Software); !running || !ok || info.Kind != software.KindServer {
		return false
	}
	events, cancel, _, _ := s.Subscribe(0)
	defer cancel()
	if s.Command("save-off") != nil || s.Command("save-all flush") != nil {
		return false
	}
	timeout := time.After(20 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return true
			}
			if event.Type == "line" && strings.Contains(event.Line.Text, "Saved the game") {
				return true
			}
		case <-timeout:
			return true
		}
	}
}

func (s *Server) writeArchive(dst string) error {
	file, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(file)
	archive := tar.NewWriter(gz)
	walkErr := fs.WalkDir(s.fs.root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return nil // unlesbares überspringen
		}
		if strings.HasPrefix(name, restoreTemp) || strings.HasSuffix(name, ".lodestone-part") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return nil // Symlinks werden nie gefolgt
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		header.Name = name
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
		if entry.IsDir() {
			header.Name += "/"
			return archive.WriteHeader(header)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		source, err := s.fs.root.Open(name)
		if err != nil {
			return nil
		}
		defer source.Close()
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		// Die Datei kann sich während des Packens ändern: genau die angekündigte Größe schreiben
		written, err := io.Copy(archive, io.LimitReader(source, header.Size))
		if err != nil {
			return err
		}
		if written < header.Size {
			_, err = io.CopyN(archive, zeros{}, header.Size-written)
		}
		return err
	})
	closeErr := errors.Join(archive.Close(), gz.Close(), file.Close())
	return errors.Join(walkErr, closeErr)
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func (s *Server) pruneBackups() {
	list, err := s.ListBackups()
	if err != nil {
		return
	}
	keep := s.snapshot().BackupKeep
	finished := list[:0:0]
	for _, backup := range list {
		if !backup.Creating {
			finished = append(finished, backup)
		}
	}
	for i := keep; i < len(finished); i++ {
		_ = os.Remove(filepath.Join(s.backupDir(), finished[i].Name))
		s.logf("Altes Backup %s gelöscht (es werden %d aufbewahrt)", finished[i].Name, keep)
	}
}

// BackupFile liefert den Pfad eines fertigen Backups (geprüft, damit nur Backups dieses Servers erreichbar sind).
func (s *Server) BackupFile(name string) (string, error) {
	if !backupName.MatchString(name) {
		return "", badRequest("ungültiger Backup-Name")
	}
	file := filepath.Join(s.backupDir(), name)
	if _, err := os.Stat(file); err != nil {
		return "", fs.ErrNotExist
	}
	return file, nil
}

// DeleteBackup löscht ein Backup.
func (s *Server) DeleteBackup(name string) error {
	file, err := s.BackupFile(name)
	if err != nil {
		return err
	}
	return os.Remove(file)
}

// RestoreBackup spielt ein Backup ein. Der Server muss aus sein; alles im Serverordner wird durch den Inhalt des
// Backups ersetzt. Es wird erst in einen Hilfsordner entpackt, damit ein defektes Archiv nichts zerstört.
func (s *Server) RestoreBackup(name string) error {
	file, err := s.BackupFile(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.state != StateOffline {
		s.mu.Unlock()
		return ErrBadState
	}
	if !s.backupBusy.CompareAndSwap(false, true) {
		s.mu.Unlock()
		return errBusy
	}
	s.setStateLocked(StateRestoring, "")
	s.mu.Unlock()
	go func() {
		defer s.backupBusy.Store(false)
		s.logf("Backup %s wird eingespielt", name)
		if err := s.extract(file); err != nil {
			s.logf("Wiederherstellung fehlgeschlagen, die bisherigen Dateien sind unverändert: %v", err)
			s.setState(StateOffline, "Wiederherstellung fehlgeschlagen: "+err.Error())
			return
		}
		s.logf("Backup %s eingespielt", name)
		s.setState(StateOffline, "")
	}()
	return nil
}

func (s *Server) extract(file string) error {
	source, err := os.Open(file)
	if err != nil {
		return err
	}
	defer source.Close()
	gz, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gz.Close()
	root := s.fs.root
	_ = root.RemoveAll(restoreTemp)
	if err := root.Mkdir(restoreTemp, 0o755); err != nil {
		return err
	}
	fail := func(err error) error {
		_ = root.RemoveAll(restoreTemp)
		return err
	}
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(err)
		}
		name, ok := archiveName(header.Name)
		if !ok {
			return fail(fmt.Errorf("das Archiv enthält einen ungültigen Pfad: %q", header.Name))
		}
		if name == "" {
			continue
		}
		target := path.Join(restoreTemp, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(target, 0o755); err != nil {
				return fail(err)
			}
		case tar.TypeReg:
			if dir := path.Dir(target); dir != "." {
				if err := root.MkdirAll(dir, 0o755); err != nil {
					return fail(err)
				}
			}
			perm := header.FileInfo().Mode().Perm() | 0o600
			out, err := root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
			if err != nil {
				return fail(err)
			}
			_, copyErr := io.Copy(out, archive)
			if err := errors.Join(copyErr, out.Close()); err != nil {
				return fail(err)
			}
		}
	}
	// Alles ist entpackt: Altes entfernen, Neues an seinen Platz
	old, err := s.fs.List(".")
	if err != nil {
		return fail(err)
	}
	for _, entry := range old {
		if entry.Name != restoreTemp {
			if err := root.RemoveAll(entry.Name); err != nil {
				return fail(err)
			}
		}
	}
	fresh, err := s.fs.List(restoreTemp)
	if err != nil {
		return err
	}
	for _, entry := range fresh {
		if err := root.Rename(path.Join(restoreTemp, entry.Name), entry.Name); err != nil {
			return err
		}
	}
	return root.RemoveAll(restoreTemp)
}

// archiveName macht aus einem Pfad im Archiv einen sicheren relativen Pfad; ok ist false bei Ausbruchsversuchen.
func archiveName(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." {
		return "", true
	}
	return cleaned, true
}
