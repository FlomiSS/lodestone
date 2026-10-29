package node

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

// fakeRuntime ersetzt Docker: Ein "Container" ist ein kleines Programm im Speicher, das wie ein Minecraft-Server
// "Done (1s)!" meldet, Befehle zurückgibt und bei "stop" endet.
type fakeRuntime struct {
	mu         sync.Mutex
	counter    int
	containers map[string]*fakeContainer
	ignoreStop bool
	pulled     []string
}

type fakeContainer struct {
	id      string
	spec    ContainerSpec
	running bool
	exit    chan struct{} // wird beim Ende geschlossen, so wie Docker alle Wartenden benachrichtigt
	code    int
	history []string
	stdinR  *io.PipeReader
	stdinW  *io.PipeWriter
	exited  bool
	ignore  bool

	cm   sync.Mutex
	subs []chan string
	done bool
}

func newFakeRuntime() *fakeRuntime { return &fakeRuntime{containers: map[string]*fakeContainer{}} }

func (f *fakeRuntime) Info(context.Context) (SystemInfo, error) {
	return SystemInfo{DockerVersion: "fake"}, nil
}

func (f *fakeRuntime) EnsureNetwork(context.Context, string) error { return nil }

func (f *fakeRuntime) EnsureImage(_ context.Context, image string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulled = append(f.pulled, image)
	return nil
}

func (f *fakeRuntime) find(idOrName string) *fakeContainer {
	for _, c := range f.containers {
		if c.id == idOrName || c.spec.Name == idOrName {
			return c
		}
	}
	return nil
}

func (f *fakeRuntime) Create(_ context.Context, spec ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.find(spec.Name) != nil {
		return "", errors.New("Name schon vergeben")
	}
	f.counter++
	c := &fakeContainer{id: fmt.Sprintf("c%d", f.counter), spec: spec, exit: make(chan struct{}), ignore: f.ignoreStop}
	c.stdinR, c.stdinW = io.Pipe()
	f.containers[c.id] = c
	return c.id, nil
}

// say schreibt eine Zeile an alle Verbindungen, wie es jedes "docker attach" für sich bekommt.
func (c *fakeContainer) say(text string) {
	c.cm.Lock()
	defer c.cm.Unlock()
	c.history = append(c.history, text)
	for _, sub := range c.subs {
		select {
		case sub <- text:
		default:
		}
	}
}

func (c *fakeContainer) closeOutput() {
	c.cm.Lock()
	defer c.cm.Unlock()
	if !c.done {
		c.done = true
		for _, sub := range c.subs {
			close(sub)
		}
	}
}

// chanReader macht aus den Zeilen eines Kanals einen Strom mit Zeilenenden wie von einem TTY.
type chanReader struct {
	lines <-chan string
	rest  []byte
}

func (r *chanReader) Read(p []byte) (int, error) {
	for len(r.rest) == 0 {
		line, ok := <-r.lines
		if !ok {
			return 0, io.EOF
		}
		r.rest = []byte(line + "\r\n")
	}
	n := copy(p, r.rest)
	r.rest = r.rest[n:]
	return n, nil
}

func (f *fakeRuntime) Start(_ context.Context, id string) error {
	f.mu.Lock()
	c := f.find(id)
	if c == nil {
		f.mu.Unlock()
		return errors.New("unbekannter Container")
	}
	c.running = true
	f.mu.Unlock()
	go func() {
		c.say("Starting minecraft server")
		c.say(`[12:00:00 INFO]: Done (1.234s)! For help, type "help"`)
		scanner := bufio.NewScanner(c.stdinR)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "stop" || line == "end" {
				if c.ignore {
					c.say("(ignoriere stop)")
					continue
				}
				c.say("Stopping server")
				f.finish(c, 0)
				return
			}
			c.say("[Server] Befehl: " + line)
			if strings.HasPrefix(line, "save-all") {
				c.say("[Server] Saved the game")
			}
		}
	}()
	return nil
}

func (f *fakeRuntime) finish(c *fakeContainer, code int) {
	f.mu.Lock()
	if c.exited {
		f.mu.Unlock()
		return
	}
	c.exited = true
	c.running = false
	f.mu.Unlock()
	c.code = code
	close(c.exit)
	c.closeOutput()
	c.stdinR.Close()
}

func (f *fakeRuntime) Stop(ctx context.Context, id string, _ time.Duration) error {
	return f.Kill(ctx, id)
}

func (f *fakeRuntime) Kill(_ context.Context, id string) error {
	f.mu.Lock()
	c := f.find(id)
	f.mu.Unlock()
	if c != nil && c.running {
		f.finish(c, 137)
	}
	return nil
}

func (f *fakeRuntime) Remove(ctx context.Context, id string) error {
	_ = f.Kill(ctx, id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.find(id); c != nil {
		delete(f.containers, c.id)
	}
	return nil
}

func (f *fakeRuntime) Inspect(_ context.Context, id string) (ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.find(id); c != nil {
		return ContainerState{Exists: true, Running: c.running}, nil
	}
	return ContainerState{}, nil
}

type fakeConn struct {
	io.Reader
	io.Writer
	closer func()
}

func (c fakeConn) Close() error { c.closer(); return nil }

func (f *fakeRuntime) Attach(_ context.Context, id string, replay bool) (io.ReadWriteCloser, error) {
	f.mu.Lock()
	c := f.find(id)
	f.mu.Unlock()
	if c == nil {
		return nil, errors.New("unbekannter Container")
	}
	c.cm.Lock()
	defer c.cm.Unlock()
	lines := make(chan string, 4096)
	if replay {
		for _, line := range c.history {
			lines <- line
		}
	}
	if c.done {
		close(lines)
	} else {
		c.subs = append(c.subs, lines)
	}
	return fakeConn{Reader: &chanReader{lines: lines}, Writer: c.stdinW, closer: func() {}}, nil
}

func (f *fakeRuntime) Wait(ctx context.Context, id string) (int, error) {
	f.mu.Lock()
	c := f.find(id)
	f.mu.Unlock()
	if c == nil {
		return 0, nil
	}
	select {
	case <-c.exit:
		return c.code, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (f *fakeRuntime) StatsStream(ctx context.Context, _ string, report func(Stats)) error {
	report(Stats{CPUPercent: 12.5, MemoryBytes: 1 << 29, MemoryLimit: 1 << 31})
	<-ctx.Done()
	return nil
}

func (f *fakeRuntime) List(context.Context) ([]ContainerRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var refs []ContainerRef
	for _, c := range f.containers {
		refs = append(refs, ContainerRef{ID: c.id, ServerID: c.spec.Labels[LabelServer], Running: c.running})
	}
	return refs, nil
}

func (f *fakeRuntime) Logs(_ context.Context, id string, lines int) (string, error) {
	f.mu.Lock()
	c := f.find(id)
	f.mu.Unlock()
	if c == nil {
		return "", nil
	}
	c.cm.Lock()
	defer c.cm.Unlock()
	history := c.history
	if len(history) > lines {
		history = history[len(history)-lines:]
	}
	return strings.Join(history, "\n"), nil
}

func (f *fakeRuntime) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

// fakeSoftware liefert eine winzige "Jar", ohne Netz.
type fakeSoftware struct{ fail bool }

func (s fakeSoftware) Resolve(_ context.Context, _ software.ID, version string) (software.Artifact, error) {
	if s.fail || version == "0.0" {
		return software.Artifact{}, software.ErrNotFound
	}
	return software.Artifact{FileName: "fake-" + version + ".jar", URL: "http://example.invalid"}, nil
}
func (fakeSoftware) JavaMajor(context.Context, software.ID, string) int { return 21 }
func (fakeSoftware) Download(_ context.Context, a software.Artifact, dst string) error {
	return os.WriteFile(dst, []byte("jar:"+a.FileName), 0o644)
}
func (fakeSoftware) Versions(context.Context, software.ID, bool) ([]string, error) {
	return []string{"1.21.8", "1.21.7"}, nil
}

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{Listen: "127.0.0.1:0", Token: strings.Repeat("t", 32), DataDir: filepath.Join(t.TempDir(), "data"), PortRangeStart: 41000, PortRangeEnd: 41200, MaxUploadMB: 5}
}

func newTestManager(t *testing.T, cfg Config, rt Runtime) *Manager {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewManager(cfg, rt, fakeSoftware{}, logger)
	manager.stopGrace = 300 * time.Millisecond
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return manager
}

// waitFor wartet, bis die Bedingung stimmt.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Zeitüberschreitung beim Warten auf: %s", what)
}

func waitState(t *testing.T, server *Server, state State) {
	t.Helper()
	waitFor(t, "Zustand "+string(state)+" (ist "+string(server.View().State)+")", func() bool { return server.View().State == state })
}

func hasLine(server *Server, substring string) bool {
	for _, line := range server.Logs(0) {
		if strings.Contains(line.Text, substring) {
			return true
		}
	}
	return false
}
