package node

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxModpackBytes  = 1 << 30   // das .mrpack-Archiv selbst
	maxModpackFile   = 512 << 20 // eine einzelne Datei
	maxModpackTotal  = 6 << 30   // alles zusammen (gegen Zip-Bomben)
	maxModpackFiles  = 4000
	modpackWorkers   = 4
	modpackTempName  = ".lodestone-modpack.mrpack"
	modpackIndexName = "modrinth.index.json"
)

// ModpackRequest bittet den Daemon, ein Modrinth-Modpack (.mrpack) auf einen Server zu installieren.
type ModpackRequest struct {
	URL    string `json:"url"`
	SHA1   string `json:"sha1,omitempty"`
	SHA512 string `json:"sha512,omitempty"`
}

// ModpackStatus sagt, wie weit die Installation ist.
type ModpackStatus struct {
	// State ist idle, running, done oder failed.
	State   string `json:"state"`
	Phase   string `json:"phase,omitempty"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	// Minecraft und Loader stammen aus dem Modpack.
	Minecraft string `json:"minecraft,omitempty"`
	Loader    string `json:"loader,omitempty"`
	Error     string `json:"error,omitempty"`
	Started   int64  `json:"started,omitempty"`
	Finished  int64  `json:"finished,omitempty"`
}

type mrpackFile struct {
	Path   string `json:"path"`
	Hashes struct {
		SHA1   string `json:"sha1"`
		SHA512 string `json:"sha512"`
	} `json:"hashes"`
	Env *struct {
		Client string `json:"client"`
		Server string `json:"server"`
	} `json:"env"`
	Downloads []string `json:"downloads"`
	FileSize  int64    `json:"fileSize"`
}

type mrpackIndex struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Files         []mrpackFile      `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

// Loader nennt den Lader des Modpacks (fabric-loader, quilt-loader, forge, neoforge) oder "" ohne.
func (i mrpackIndex) loader() string {
	for _, name := range []string{"fabric-loader", "quilt-loader", "neoforge", "forge"} {
		if _, ok := i.Dependencies[name]; ok {
			return name
		}
	}
	return ""
}

// ModpackStatus liefert den Stand der letzten Modpack-Installation.
func (s *Server) ModpackStatus() ModpackStatus {
	s.modpackMu.Lock()
	defer s.modpackMu.Unlock()
	if s.modpack.State == "" {
		return ModpackStatus{State: "idle"}
	}
	return s.modpack
}

func (s *Server) setModpack(update func(*ModpackStatus)) {
	s.modpackMu.Lock()
	defer s.modpackMu.Unlock()
	update(&s.modpack)
}

// InstallModpack lädt das Modpack und legt Mods, Configs und Dateien auf dem Server ab. Das läuft im Hintergrund;
// der Stand steht in ModpackStatus. Der Server muss aus sein.
func (m *Manager) InstallModpack(s *Server, request ModpackRequest) error {
	address, err := url.Parse(request.URL)
	if err != nil || address.Host == "" {
		return badRequest("ungültige Adresse")
	}
	if !m.checkPullURL(address) {
		return badRequest("von %q darf nichts geladen werden (erlaubt: https und die Hosts in der Config)", address.Hostname())
	}
	s.mu.Lock()
	if s.state != StateOffline {
		s.mu.Unlock()
		return ErrBadState
	}
	if !s.modpackBusy.CompareAndSwap(false, true) {
		s.mu.Unlock()
		return fmt.Errorf("%w: Es läuft schon eine Modpack-Installation", ErrBadState)
	}
	s.mu.Unlock()
	s.setModpack(func(status *ModpackStatus) {
		*status = ModpackStatus{State: "running", Phase: "Modpack wird heruntergeladen", Started: time.Now().UnixMilli()}
	})
	go func() {
		defer s.modpackBusy.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if err := m.runModpack(ctx, s, request); err != nil {
			s.logf("Modpack-Installation fehlgeschlagen: %v", err)
			s.setModpack(func(status *ModpackStatus) {
				status.State, status.Error, status.Phase, status.Finished = "failed", err.Error(), "", time.Now().UnixMilli()
			})
			return
		}
		s.setModpack(func(status *ModpackStatus) {
			status.State, status.Phase, status.Finished = "done", "", time.Now().UnixMilli()
		})
	}()
	return nil
}

func (m *Manager) httpClient() *http.Client {
	client := &http.Client{}
	if m.pullClient != nil {
		copied := *m.pullClient
		client = &copied
	}
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("zu viele Weiterleitungen")
		}
		if !m.checkPullURL(next.URL) {
			return fmt.Errorf("die Weiterleitung auf %q ist nicht erlaubt", next.URL.Hostname())
		}
		return nil
	}
	return client
}

func (m *Manager) runModpack(ctx context.Context, s *Server, request ModpackRequest) error {
	client := m.httpClient()
	s.logf("Modpack wird heruntergeladen")
	if err := m.downloadTo(ctx, client, s, request.URL, modpackTempName, maxModpackBytes, request.SHA1, request.SHA512); err != nil {
		return err
	}
	defer s.fs.Remove(modpackTempName, false)

	archive, err := openZip(s, modpackTempName)
	if err != nil {
		return fmt.Errorf("das Modpack ist kein gültiges .mrpack: %w", err)
	}
	defer archive.close()

	index, err := archive.index()
	if err != nil {
		return err
	}
	if index.FormatVersion != 1 || (index.Game != "" && index.Game != "minecraft") {
		return fmt.Errorf("nicht unterstütztes Modpack-Format (formatVersion %d, Spiel %q)", index.FormatVersion, index.Game)
	}
	if len(index.Files) > maxModpackFiles {
		return fmt.Errorf("das Modpack hat zu viele Dateien (%d)", len(index.Files))
	}
	loader := index.loader()
	files := make([]mrpackFile, 0, len(index.Files))
	for _, file := range index.Files {
		if file.Env != nil && file.Env.Server == "unsupported" {
			continue // nur für den Client
		}
		files = append(files, file)
	}
	overrides := archive.overrides()
	s.setModpack(func(status *ModpackStatus) {
		status.Name, status.Version, status.Minecraft, status.Loader = index.Name, index.VersionID, index.Dependencies["minecraft"], loader
		status.Total = len(files) + len(overrides)
		status.Phase = "Mods und Dateien werden geladen"
	})
	s.logf("Modpack „%s“ %s: %d Dateien, %d aus dem Archiv (Minecraft %s, %s)", index.Name, index.VersionID, len(files), len(overrides), index.Dependencies["minecraft"], loader)

	// Dateien aus dem Netz, parallel (aber nicht zu viele auf einmal)
	var (
		wg       sync.WaitGroup
		failed   atomic.Value
		finished atomic.Int64
		total    atomic.Int64
	)
	jobs := make(chan mrpackFile)
	for worker := 0; worker < modpackWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				if failed.Load() != nil || ctx.Err() != nil {
					continue
				}
				if err := m.downloadModpackFile(ctx, client, s, file, &total); err != nil {
					failed.CompareAndSwap(nil, fmt.Errorf("%s: %w", file.Path, err))
					continue
				}
				done := int(finished.Add(1))
				s.setModpack(func(status *ModpackStatus) { status.Done = done })
				if done%25 == 0 {
					s.logf("Modpack: %d von %d Dateien geladen", done, len(files))
				}
			}
		}()
	}
	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	wg.Wait()
	if err, _ := failed.Load().(error); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Dateien aus dem Archiv: erst overrides, dann server-overrides (die gewinnen)
	s.setModpack(func(status *ModpackStatus) { status.Phase = "Konfiguration wird entpackt" })
	written := total.Load()
	for _, entry := range overrides {
		if err := archive.extract(s, entry, &written); err != nil {
			return err
		}
		done := int(finished.Add(1))
		s.setModpack(func(status *ModpackStatus) { status.Done = done })
	}
	s.logf("Modpack „%s“ ist installiert", index.Name)
	return nil
}

// downloadModpackFile lädt eine Datei des Modpacks, prüft sie gegen die Prüfsummen und legt sie sicher ab.
func (m *Manager) downloadModpackFile(ctx context.Context, client *http.Client, s *Server, file mrpackFile, total *atomic.Int64) error {
	target, err := safeModpackPath(file.Path)
	if err != nil {
		return err
	}
	if file.FileSize > maxModpackFile {
		return fmt.Errorf("die Datei ist größer als %d MB", maxModpackFile>>20)
	}
	var lastErr error
	for _, address := range file.Downloads {
		parsed, err := url.Parse(address)
		if err != nil || !m.checkPullURL(parsed) {
			lastErr = fmt.Errorf("von %q darf nichts geladen werden", hostOf(address))
			continue
		}
		if err := m.downloadTo(ctx, client, s, address, target, maxModpackFile, file.Hashes.SHA1, file.Hashes.SHA512); err != nil {
			lastErr = err
			continue
		}
		total.Add(file.FileSize)
		if total.Load() > maxModpackTotal {
			return errors.New("das Modpack ist größer als erlaubt")
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("keine Download-Adresse")
	}
	return lastErr
}

func hostOf(address string) string {
	if parsed, err := url.Parse(address); err == nil {
		return parsed.Hostname()
	}
	return address
}

// downloadTo lädt address nach rel (über eine Teildatei) und vergleicht die Prüfsummen, soweit angegeben.
func (m *Manager) downloadTo(ctx context.Context, client *http.Client, s *Server, address, rel string, limit int64, wantSHA1, wantSHA512 string) error {
	if dir := path.Dir(rel); dir != "." {
		if err := s.fs.Mkdir(dir); err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return badRequest("ungültige Adresse")
	}
	request.Header.Set("User-Agent", "Lodestone")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Download fehlgeschlagen: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Download fehlgeschlagen: Status %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return fmt.Errorf("die Datei ist größer als %d MB", limit>>20)
	}
	part := rel + ".lodestone-part"
	_ = s.fs.Remove(part, false)
	sum1, sum512 := sha1.New(), sha512.New()
	if err := s.fs.Write(part, io.TeeReader(response.Body, io.MultiWriter(sum1, sum512)), limit, true); err != nil {
		_ = s.fs.Remove(part, false)
		return err
	}
	if !matches(sum512, wantSHA512) || !matches(sum1, wantSHA1) {
		_ = s.fs.Remove(part, false)
		return errors.New("die Prüfsumme der Datei stimmt nicht")
	}
	_ = s.fs.Remove(rel, false)
	if err := s.fs.Rename(part, rel); err != nil {
		_ = s.fs.Remove(part, false)
		return err
	}
	return nil
}

func matches(sum hash.Hash, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	return want == "" || hex.EncodeToString(sum.Sum(nil)) == want
}

// safeModpackPath macht aus einem Pfad im Modpack einen sicheren relativen Pfad im Serverordner.
func safeModpackPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") || (len(name) > 1 && name[1] == ':') {
		return "", fmt.Errorf("ungültiger Pfad %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("ungültiger Pfad %q", name)
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." || strings.HasPrefix(cleaned, ".lodestone") || cleaned == "spec.json" {
		return "", fmt.Errorf("ungültiger Pfad %q", name)
	}
	return cleaned, nil
}

// ---------------------------------------------------------------- Archiv

type mrpack struct {
	file *zip.ReadCloser
}

func openZip(s *Server, rel string) (*mrpack, error) {
	// Das Archiv hat der Daemon selbst im Serverordner angelegt; zip braucht den echten Pfad der Datei
	reader, err := zip.OpenReader(filepath.Join(s.dir, "data", filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	return &mrpack{file: reader}, nil
}

func (p *mrpack) close() { _ = p.file.Close() }

func (p *mrpack) index() (mrpackIndex, error) {
	for _, entry := range p.file.File {
		if entry.Name != modpackIndexName {
			continue
		}
		if entry.UncompressedSize64 > 8<<20 {
			return mrpackIndex{}, errors.New("modrinth.index.json ist zu groß")
		}
		reader, err := entry.Open()
		if err != nil {
			return mrpackIndex{}, err
		}
		defer reader.Close()
		var index mrpackIndex
		if err := json.NewDecoder(io.LimitReader(reader, 8<<20)).Decode(&index); err != nil {
			return mrpackIndex{}, fmt.Errorf("modrinth.index.json ist ungültig: %w", err)
		}
		return index, nil
	}
	return mrpackIndex{}, errors.New("im Archiv fehlt modrinth.index.json – das ist kein Modrinth-Modpack")
}

type override struct {
	entry *zip.File
	rel   string
	order int
}

// overrides sind die Dateien aus overrides/ und server-overrides/; Letztere stehen hinten und überschreiben.
func (p *mrpack) overrides() []override {
	var list []override
	for _, entry := range p.file.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		switch {
		case strings.HasPrefix(entry.Name, "overrides/"):
			list = append(list, override{entry, strings.TrimPrefix(entry.Name, "overrides/"), 0})
		case strings.HasPrefix(entry.Name, "server-overrides/"):
			list = append(list, override{entry, strings.TrimPrefix(entry.Name, "server-overrides/"), 1})
		}
	}
	// stabil nach order, damit server-overrides zuletzt geschrieben werden
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].order < list[j-1].order; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	return list
}

func (p *mrpack) extract(s *Server, item override, written *int64) error {
	rel, err := safeModpackPath(item.rel)
	if err != nil {
		return err
	}
	if item.entry.Mode()&0o170000 == 0o120000 { // Symlink im Archiv: nie anlegen
		return nil
	}
	if item.entry.UncompressedSize64 > maxModpackFile {
		return fmt.Errorf("%s ist zu groß", item.rel)
	}
	*written += int64(item.entry.UncompressedSize64)
	if *written > maxModpackTotal {
		return errors.New("das Modpack ist größer als erlaubt")
	}
	if dir := path.Dir(rel); dir != "." {
		if err := s.fs.Mkdir(dir); err != nil {
			return err
		}
	}
	reader, err := item.entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	_ = s.fs.Remove(rel, false) // ein vorhandenes (auch ein Symlink) wird ersetzt, nie hindurch geschrieben
	return s.fs.Write(rel, reader, maxModpackFile, true)
}
