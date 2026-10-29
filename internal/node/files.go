package node

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// FS ist der Dateizugriff auf den Ordner eines Servers. Er benutzt os.Root: Pfade mit "..", absolute Pfade und
// Symlinks, die aus dem Ordner hinauszeigen, kommen nie an Dateien außerhalb heran.
type FS struct {
	root *os.Root
}

// OpenFS öffnet (und legt bei Bedarf an) den Ordner.
func OpenFS(dir string) (*FS, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &FS{root: root}, nil
}

// Close gibt den Ordner frei.
func (f *FS) Close() error { return f.root.Close() }

// Entry ist eine Datei oder ein Ordner in einer Auflistung.
type Entry struct {
	Name     string `json:"name"`
	Dir      bool   `json:"dir"`
	Symlink  bool   `json:"symlink,omitempty"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Mode     string `json:"mode"`
}

// ErrOutside meldet einen Pfad, der den Serverordner verlassen würde.
var ErrOutside = errors.New("der Pfad liegt außerhalb des Serverordners")

// clean macht aus einem Pfad des Benutzers einen relativen, bereinigten Pfad ("." für den Ordner selbst).
func clean(name string) (string, error) {
	if strings.ContainsRune(name, 0) {
		return "", errors.New("ungültiger Pfad")
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return ".", nil
	}
	return name, nil
}

func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return fs.ErrNotExist
	}
	// os.Root meldet Ausbruchsversuche (Symlinks, "..") mit diesem Text
	if strings.Contains(err.Error(), "path escapes from parent") {
		return ErrOutside
	}
	return err
}

// Exists sagt, ob es den Pfad gibt (Symlinks zählen als vorhanden, ohne ihnen zu folgen).
func (f *FS) Exists(name string) bool {
	rel, err := clean(name)
	if err != nil {
		return false
	}
	_, err = f.root.Lstat(rel)
	return err == nil
}

// List listet einen Ordner, Ordner zuerst.
func (f *FS) List(name string) ([]Entry, error) {
	rel, err := clean(name)
	if err != nil {
		return nil, err
	}
	dir, err := f.root.Open(rel)
	if err != nil {
		return nil, translate(err)
	}
	defer dir.Close()
	items, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			continue
		}
		entry := Entry{Name: item.Name(), Dir: item.IsDir(), Symlink: info.Mode()&fs.ModeSymlink != 0, Size: info.Size(), Modified: info.ModTime().UnixMilli(), Mode: info.Mode().Perm().String()}
		if entry.Symlink {
			// Ob das Ziel ein Ordner ist, sagt Stat (und scheitert bei Ausbruch, dann bleibt es eine Datei)
			if target, err := f.root.Stat(path.Join(rel, item.Name())); err == nil {
				entry.Dir = target.IsDir()
			}
		}
		if entry.Dir {
			entry.Size = 0
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

// Read liest eine Datei bis max Bytes; größere werden abgelehnt.
func (f *FS) Read(name string, max int64) ([]byte, error) {
	rel, err := clean(name)
	if err != nil {
		return nil, err
	}
	file, err := f.root.Open(rel)
	if err != nil {
		return nil, translate(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("das ist ein Ordner")
	}
	if info.Size() > max {
		return nil, fmt.Errorf("die Datei ist zu groß (%d KB, erlaubt sind %d KB)", info.Size()/1024, max/1024)
	}
	return io.ReadAll(io.LimitReader(file, max+1))
}

// Open öffnet eine Datei zum Herunterladen.
func (f *FS) Open(name string) (*os.File, os.FileInfo, error) {
	rel, err := clean(name)
	if err != nil {
		return nil, nil, err
	}
	file, err := f.root.Open(rel)
	if err != nil {
		return nil, nil, translate(err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		file.Close()
		return nil, nil, errors.New("das ist ein Ordner")
	}
	return file, info, nil
}

// Write schreibt den Inhalt in eine Datei, höchstens max Bytes. Mit create darf es die Datei noch nicht geben,
// sonst muss sie schon da sein (so bleiben Rechte und Besitzer erhalten).
func (f *FS) Write(name string, content io.Reader, max int64, create bool) error {
	rel, err := clean(name)
	if err != nil {
		return err
	}
	if rel == "." {
		return errors.New("das ist der Serverordner selbst")
	}
	flags := os.O_WRONLY
	perm := os.FileMode(0o644)
	if create {
		flags |= os.O_CREATE | os.O_EXCL
	} else {
		flags |= os.O_TRUNC
	}
	file, err := f.root.OpenFile(rel, flags, perm)
	if err != nil {
		if create && errors.Is(err, fs.ErrExist) {
			return fs.ErrExist
		}
		return translate(err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(content, max+1))
	closeErr := file.Close()
	if copyErr == nil && written > max {
		copyErr = fmt.Errorf("die Datei ist zu groß (erlaubt sind %d MB)", max>>20)
	}
	if copyErr != nil {
		if create || written > max {
			_ = f.root.Remove(rel)
		}
		return copyErr
	}
	return closeErr
}

// Mkdir legt einen Ordner samt fehlender übergeordneter Ordner an.
func (f *FS) Mkdir(name string) error {
	rel, err := clean(name)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	return translate(f.root.MkdirAll(rel, 0o755))
}

// Remove löscht eine Datei oder einen Ordner; Ordner nur leer, außer mit recursive.
func (f *FS) Remove(name string, recursive bool) error {
	rel, err := clean(name)
	if err != nil {
		return err
	}
	if rel == "." {
		return errors.New("der Serverordner selbst kann nicht gelöscht werden")
	}
	if recursive {
		return translate(f.root.RemoveAll(rel))
	}
	return translate(f.root.Remove(rel))
}

// Rename benennt um oder verschiebt innerhalb des Serverordners. Ein vorhandenes Ziel wird nie überschrieben.
func (f *FS) Rename(from, to string) error {
	source, err := clean(from)
	if err != nil {
		return err
	}
	target, err := clean(to)
	if err != nil {
		return err
	}
	if source == "." || target == "." {
		return errors.New("der Serverordner selbst kann nicht verschoben werden")
	}
	if _, err := f.root.Lstat(target); err == nil {
		return fs.ErrExist
	}
	if _, err := f.root.Lstat(source); err != nil {
		return translate(err)
	}
	return translate(f.root.Rename(source, target))
}
