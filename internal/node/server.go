package node

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

// Fehler, die die Schnittstelle in Statuscodes übersetzt.
var (
	ErrBadState = errors.New("das ist im aktuellen Zustand des Servers nicht möglich")
	ErrNotFound = errors.New("diesen Server gibt es nicht")
	ErrConflict = errors.New("das kollidiert mit einem vorhandenen Server")
)

const (
	jarName        = "server.jar"
	consoleLines   = 2000
	maxLineLength  = 8192
	maxCommandSize = 4096
)

// View ist, was die Schnittstelle von einem Server zeigt.
type View struct {
	Spec
	State     State  `json:"state"`
	LastError string `json:"lastError,omitempty"`
	Stats     *Stats `json:"stats,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
	// AutoconfigPending heißt: Ein Satz Dateiänderungen wartet noch darauf, angewendet zu werden.
	AutoconfigPending bool `json:"autoconfigPending,omitempty"`
	// LastExit beschreibt, wie der letzte Lauf geendet hat (das Panel erkennt daran Abstürze).
	LastExit *ExitInfo `json:"lastExit,omitempty"`
}

// ExitInfo sagt, wie ein Lauf geendet hat. Ein Ende mit Code ungleich 0, das niemand verlangt hat, ist ein Absturz.
type ExitInfo struct {
	Code int `json:"code"`
	// Requested: Der Daemon hat das Ende verlangt (Stopp, Neustart, Kill).
	Requested bool  `json:"requested"`
	At        int64 `json:"at"`
}

// Server ist ein verwalteter Minecraft-Server.
type Server struct {
	m   *Manager
	dir string
	fs  *FS

	mu        sync.Mutex
	spec      Spec
	state     State
	lastError string
	container string
	stdin     io.WriteCloser
	done      chan struct{}
	startedAt time.Time
	stats     *Stats
	ready     *regexp.Regexp

	console ring
	events  hub

	applyMu    sync.Mutex // serialisiert das Anwenden von Autoconfig
	addons     addonCache
	backupBusy atomic.Bool

	// lastExit ist das Ende des letzten Laufs; killRequested merkt sich einen Kill über die Schnittstelle.
	lastExit      *ExitInfo
	killRequested bool

	// Modpack: läuft gerade eine Installation, und wie weit ist sie
	modpackBusy atomic.Bool
	modpackMu   sync.Mutex
	modpack     ModpackStatus
}

func newServer(m *Manager, dir string, spec Spec) (*Server, error) {
	files, err := OpenFS(filepath.Join(dir, "data"))
	if err != nil {
		return nil, err
	}
	s := &Server{m: m, dir: dir, fs: files, spec: spec, state: StateOffline, console: ring{limit: consoleLines}}
	s.refreshPattern()
	return s, nil
}

func (s *Server) refreshPattern() {
	if info, ok := software.Lookup(s.spec.Software); ok && info.ReadyPattern != "" {
		s.ready = regexp.MustCompile(info.ReadyPattern)
	}
}

// ---------------------------------------------------------------- Ansicht

// View liefert den aktuellen Stand.
func (s *Server) View() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	view := View{Spec: s.spec, State: s.state, LastError: s.lastError, Stats: s.stats, AutoconfigPending: s.autoconfigPending()}
	if s.lastExit != nil {
		exit := *s.lastExit
		view.LastExit = &exit
	}
	if !s.startedAt.IsZero() && (s.state == StateRunning || s.state == StateStarting || s.state == StateStopping) {
		view.StartedAt = s.startedAt.UnixMilli()
	}
	return view
}

// ID ist die Kennung des Servers.
func (s *Server) ID() string { return s.spec.ID }

// FS ist der Dateizugriff auf den Serverordner.
func (s *Server) FS() *FS { return s.fs }

// Subscribe liefert künftige Meldungen und den bisherigen Verlauf (in dieser Reihenfolge, damit nichts fehlt;
// Doppelte erkennt man an Line.Seq).
func (s *Server) Subscribe(history int) (<-chan Event, func(), []Line, State) {
	channel, cancel := s.events.subscribe()
	lines := s.console.tail(history)
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	return channel, cancel, lines, state
}

// Logs liefert die letzten Konsolenzeilen.
func (s *Server) Logs(n int) []Line { return s.console.tail(n) }

func (s *Server) logf(format string, args ...any) {
	s.appendLine("[lodestone] " + fmt.Sprintf(format, args...))
}

func (s *Server) appendLine(text string) {
	if len(text) > maxLineLength {
		text = text[:maxLineLength] + " …"
	}
	line := s.console.add(time.Now().UnixMilli(), text)
	s.events.publish(Event{Type: "line", Line: &line})
}

func (s *Server) setState(state State, problem string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStateLocked(state, problem)
}

func (s *Server) setStateLocked(state State, problem string) {
	s.state = state
	s.lastError = problem
	if state == StateOffline {
		s.stats = nil
	}
	s.events.publish(Event{Type: "state", State: state, Error: problem})
}

// ---------------------------------------------------------------- Speichern

func (s *Server) saveLocked() error {
	data, err := json.MarshalIndent(s.spec, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, "spec.json")
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// ---------------------------------------------------------------- Installation

// install lädt die Server-Datei und bereitet den Ordner vor. Der Zustand muss vorher auf "installing" stehen.
func (s *Server) install(ctx context.Context) error {
	spec := s.snapshot()
	info, ok := software.Lookup(spec.Software)
	if !ok {
		return fmt.Errorf("unbekannte Software: %s", spec.Software)
	}
	s.logf("Installation: %s %s", info.Name, spec.Version)
	artifact, err := s.m.sw.Resolve(ctx, spec.Software, spec.Version)
	if err != nil {
		return fmt.Errorf("Version nicht gefunden: %w", err)
	}
	java := s.m.sw.JavaMajor(ctx, spec.Software, spec.Version)
	s.logf("Download: %s (Java %d)", artifact.FileName, java)
	if err := s.m.sw.Download(ctx, artifact, filepath.Join(s.dir, "data", jarName)); err != nil {
		return fmt.Errorf("Download fehlgeschlagen: %w", err)
	}
	_ = s.fs.Remove("run.sh", false)
	if info.Kind == software.KindServer && spec.EULA {
		if err := s.writeText("eula.txt", "# Bestätigt im Lodestone-Panel (https://aka.ms/MinecraftEULA)\neula=true\n"); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spec.Installed = true
	s.spec.JavaMajor = java
	return s.saveLocked()
}

func (s *Server) writeText(name, content string) error {
	err := s.fs.Write(name, strings.NewReader(content), 1<<20, true)
	if errors.Is(err, fs.ErrExist) {
		err = s.fs.Write(name, strings.NewReader(content), 1<<20, false)
	}
	return err
}

// installAsync installiert im Hintergrund; Fehler landen in Konsole und lastError, der Server bleibt offline.
func (s *Server) installAsync() {
	s.setState(StateInstalling, "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := s.install(ctx); err != nil {
		s.logf("Installation fehlgeschlagen: %v", err)
		s.setState(StateOffline, err.Error())
		return
	}
	s.logf("Installation abgeschlossen")
	s.setState(StateOffline, "")
}

func (s *Server) snapshot() Spec {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spec
}

// ---------------------------------------------------------------- Starten

// Start startet den Server im Hintergrund.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.state != StateOffline {
		s.mu.Unlock()
		return ErrBadState
	}
	if s.modpackBusy.Load() {
		s.mu.Unlock()
		return fmt.Errorf("%w: Das Modpack wird noch installiert", ErrBadState)
	}
	if err := s.m.checkMemory(s.spec.MemoryMB); err != nil {
		s.mu.Unlock()
		return err
	}
	s.setStateLocked(StateStarting, "")
	s.mu.Unlock()
	go s.launch()
	return nil
}

func (s *Server) launch() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := s.doLaunch(ctx); err != nil {
		s.logf("Start fehlgeschlagen: %v", err)
		s.setState(StateOffline, err.Error())
	}
}

func (s *Server) doLaunch(ctx context.Context) error {
	spec := s.snapshot()
	if !spec.Installed {
		if err := s.install(ctx); err != nil {
			return err
		}
		spec = s.snapshot()
	}
	containerSpec := s.containerSpec(spec)
	if err := s.m.rt.EnsureNetwork(ctx, containerSpec.Network); err != nil {
		return fmt.Errorf("Docker-Netzwerk %q: %w", containerSpec.Network, err)
	}
	s.logf("Image wird geprüft: %s", containerSpec.Image)
	if err := s.m.rt.EnsureImage(ctx, containerSpec.Image); err != nil {
		return err
	}
	// Reste eines früheren Laufs entfernen (der Name ist fest)
	if err := s.m.rt.Remove(ctx, containerSpec.Name); err != nil {
		return err
	}
	id, err := s.m.rt.Create(ctx, containerSpec)
	if err != nil {
		return err
	}
	conn, err := s.m.rt.Attach(ctx, id, false)
	if err != nil {
		_ = s.m.rt.Remove(ctx, id)
		return err
	}
	if err := s.m.rt.Start(ctx, id); err != nil {
		conn.Close()
		_ = s.m.rt.Remove(ctx, id)
		return err
	}
	s.logf("Server gestartet")
	s.adoptRun(id, conn)
	return nil
}

// adoptRun übernimmt einen laufenden Container: Ausgabe lesen, Ende beobachten, Messwerte holen.
func (s *Server) adoptRun(id string, conn io.ReadWriteCloser) {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	s.container = id
	s.stdin = conn
	s.done = done
	s.startedAt = time.Now()
	s.mu.Unlock()

	go s.pump(conn)
	go func() {
		code, err := s.m.rt.Wait(context.Background(), id)
		cancel()
		conn.Close()
		s.mu.Lock()
		s.stdin = nil
		if err == nil {
			s.lastExit = &ExitInfo{Code: code, Requested: s.state == StateStopping || s.killRequested, At: time.Now().UnixMilli()}
		}
		s.killRequested = false
		s.setStateLocked(StateOffline, "")
		close(done)
		s.mu.Unlock()
		if err != nil {
			s.logf("Docker meldet beim Warten einen Fehler: %v", err)
			return
		}
		s.logf("Server beendet (Exit-Code %d)", code)
	}()
	go func() {
		err := s.m.rt.StatsStream(runCtx, id, func(stats Stats) {
			s.mu.Lock()
			s.stats = &stats
			s.mu.Unlock()
			s.events.publish(Event{Type: "stats", Stats: &stats})
		})
		if err != nil && runCtx.Err() == nil {
			s.m.log.Warn("Messwerte nicht verfügbar", "server", s.spec.ID, "error", err)
		}
	}()
}

// pump liest die Konsole zeilenweise und erkennt, wann der Server bereit ist.
func (s *Server) pump(conn io.Reader) {
	reader := bufio.NewReaderSize(conn, 64*1024)
	for {
		raw, err := reader.ReadString('\n')
		if raw != "" {
			text := strings.TrimRight(raw, "\r\n")
			s.appendLine(text)
			s.mu.Lock()
			if s.state == StateStarting && s.ready != nil && s.ready.MatchString(text) {
				s.setStateLocked(StateRunning, "")
				go s.tryApply(true)
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *Server) containerSpec(spec Spec) ContainerSpec {
	info, _ := software.Lookup(spec.Software)
	startup := spec.Startup
	if startup == "" {
		startup = info.Startup
	}
	// Der Container braucht neben dem Heap Platz für Metaspace, Threads und native Speicher, sonst beendet ihn der
	// Kernel mitten im Start (Exit-Code 137). Deshalb bekommt der Heap höchstens drei Viertel, mindestens 256 MB bleiben frei.
	heap := min(spec.MemoryMB*75/100, spec.MemoryMB-256)
	if heap < 128 {
		heap = max(spec.MemoryMB/2, 64)
	}
	startup = strings.NewReplacer("{{MEMORY}}", fmt.Sprint(heap), "{{JAR}}", jarName).Replace(startup)
	image := spec.Image
	if image == "" {
		java := spec.JavaMajor
		if java == 0 {
			java = software.FallbackJava(spec.Version)
		}
		image = fmt.Sprintf("eclipse-temurin:%d-jre", java)
	}
	env := []string{fmt.Sprintf("SERVER_MEMORY=%d", spec.MemoryMB)}
	for key, value := range spec.Env {
		env = append(env, key+"="+value)
	}
	return ContainerSpec{
		Name:        "lodestone-" + spec.ID,
		Image:       image,
		Cmd:         []string{"/bin/sh", "-c", startup},
		Env:         env,
		WorkDir:     "/data",
		Binds:       []string{filepath.Join(s.dir, "data") + ":/data"},
		Ports:       spec.Ports,
		MemoryBytes: int64(spec.MemoryMB) << 20,
		NanoCPUs:    int64(spec.CPU * 1e9),
		User:        s.m.cfg.ContainerUser,
		Labels:      map[string]string{LabelServer: spec.ID},
		Network:     s.m.cfg.Network,
	}
}

// ---------------------------------------------------------------- Stoppen

// Stop fährt den Server sauber herunter (Stop-Befehl in der Konsole) und beendet ihn hart, wenn er nicht reagiert.
func (s *Server) Stop() error {
	_, err := s.stop()
	return err
}

func (s *Server) stop() (<-chan struct{}, error) {
	s.mu.Lock()
	if s.state != StateStarting && s.state != StateRunning {
		s.mu.Unlock()
		return nil, ErrBadState
	}
	stdin, done, id := s.stdin, s.done, s.container
	if stdin == nil && s.state == StateStarting {
		// der Container wird gerade erst angelegt, es gibt noch nichts zu stoppen
		s.mu.Unlock()
		return nil, ErrBadState
	}
	s.setStateLocked(StateStopping, "")
	s.mu.Unlock()

	command := "stop"
	if info, ok := software.Lookup(s.spec.Software); ok {
		command = info.StopCommand
	}
	s.logf("Server wird gestoppt")
	if stdin == nil {
		go s.hardStop(id, done, 0)
		return done, nil
	}
	if _, err := io.WriteString(stdin, command+"\n"); err != nil {
		go s.hardStop(id, done, 0)
		return done, nil
	}
	go s.hardStop(id, done, s.m.stopGrace)
	return done, nil
}

// hardStop beendet den Container, wenn er nach der Schonfrist noch läuft.
func (s *Server) hardStop(id string, done <-chan struct{}, grace time.Duration) {
	if grace > 0 {
		select {
		case <-done:
			return
		case <-time.After(grace):
			s.logf("Der Server reagiert nicht, er wird beendet")
		}
	}
	if err := s.m.rt.Kill(context.Background(), id); err != nil {
		s.logf("Beenden fehlgeschlagen: %v", err)
	}
}

// Kill beendet den Server sofort.
func (s *Server) Kill() error {
	s.mu.Lock()
	if s.state != StateStarting && s.state != StateRunning && s.state != StateStopping {
		s.mu.Unlock()
		return ErrBadState
	}
	id := s.container
	s.killRequested = true
	s.mu.Unlock()
	s.logf("Server wird beendet (Kill)")
	return s.m.rt.Kill(context.Background(), id)
}

// Restart stoppt den Server und startet ihn danach neu.
func (s *Server) Restart() error {
	s.mu.Lock()
	offline := s.state == StateOffline
	s.mu.Unlock()
	if offline {
		return s.Start()
	}
	done, err := s.stop()
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-done:
		case <-time.After(s.m.stopGrace + 30*time.Second):
			return
		}
		if err := s.Start(); err != nil {
			s.logf("Neustart fehlgeschlagen: %v", err)
		}
	}()
	return nil
}

// Command schickt einen Befehl in die Konsole.
func (s *Server) Command(command string) error {
	command = strings.TrimSpace(command)
	if command == "" || len(command) > maxCommandSize || strings.ContainsAny(command, "\r\n\x00") {
		return errors.New("ungültiger Befehl")
	}
	s.mu.Lock()
	stdin := s.stdin
	state := s.state
	s.mu.Unlock()
	if stdin == nil || (state != StateRunning && state != StateStarting && state != StateStopping) {
		return ErrBadState
	}
	_, err := io.WriteString(stdin, command+"\n")
	return err
}

// ---------------------------------------------------------------- Ändern

// Patch enthält die Felder, die sich nachträglich ändern lassen. Nicht gesetzte Felder bleiben.
type Patch struct {
	Name     *string            `json:"name"`
	MemoryMB *int               `json:"memoryMB"`
	CPU      *float64           `json:"cpu"`
	Ports    *[]PortMapping     `json:"ports"`
	Env      *map[string]string `json:"env"`
	Startup  *string            `json:"startup"`
	Image    *string            `json:"image"`
	EULA     *bool              `json:"eula"`
	// BackupKeep und NoPublish wie in Spec.
	BackupKeep *int  `json:"backupKeep"`
	NoPublish  *bool `json:"noPublish"`
}

// Update ändert Einstellungen. Das geht nur, solange der Server aus ist.
func (s *Server) Update(patch Patch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateOffline {
		return ErrBadState
	}
	next := s.spec
	if patch.Name != nil {
		next.Name = *patch.Name
	}
	if patch.MemoryMB != nil {
		next.MemoryMB = *patch.MemoryMB
	}
	if patch.CPU != nil {
		next.CPU = *patch.CPU
	}
	if patch.Ports != nil {
		next.Ports = *patch.Ports
	}
	if patch.Env != nil {
		next.Env = *patch.Env
	}
	if patch.Startup != nil {
		next.Startup = *patch.Startup
	}
	if patch.Image != nil {
		next.Image = *patch.Image
	}
	if patch.EULA != nil {
		next.EULA = *patch.EULA
	}
	if patch.BackupKeep != nil {
		next.BackupKeep = *patch.BackupKeep
	}
	if patch.NoPublish != nil {
		next.NoPublish = *patch.NoPublish
	}
	if err := next.Normalize(); err != nil {
		return err
	}
	if err := s.m.allocatePorts(&next, s.spec.ID); err != nil {
		return err
	}
	s.spec = next
	if next.EULA {
		// beim nächsten Start gilt die Bestätigung
		go func() { _ = s.writeText("eula.txt", "eula=true\n") }()
	}
	return s.saveLocked()
}

// Reinstall wechselt Software oder Version und lädt die Server-Datei neu. Welten, Plugins und Configs bleiben.
func (s *Server) Reinstall(id software.ID, version string) error {
	s.mu.Lock()
	if s.state != StateOffline {
		s.mu.Unlock()
		return ErrBadState
	}
	next := s.spec
	next.Software, next.Version = id, version
	if err := next.Normalize(); err != nil {
		s.mu.Unlock()
		return err
	}
	next.Installed = false
	next.JavaMajor = 0
	s.spec = next
	s.refreshPattern()
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	go s.installAsync()
	return nil
}

// close gibt Ressourcen frei (nach dem Löschen).
func (s *Server) close() { _ = s.fs.Close() }
