package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

// Software ist, was der Daemon von der Software-Auflösung braucht (in Tests ersetzbar).
type Software interface {
	Resolve(ctx context.Context, id software.ID, version string) (software.Artifact, error)
	JavaMajor(ctx context.Context, id software.ID, version string) int
	Download(ctx context.Context, artifact software.Artifact, dst string) error
	Versions(ctx context.Context, id software.ID, prerelease bool) ([]string, error)
}

// Manager verwaltet alle Server dieser Maschine.
type Manager struct {
	cfg Config
	rt  Runtime
	sw  Software
	log *slog.Logger

	// Für Downloads per Link; in Tests ersetzbar.
	pullClient   *http.Client
	insecurePull bool

	// availableMemory liefert den freien Speicher des Hosts in Bytes (0 = unbekannt); in Tests ersetzbar.
	availableMemory func() int64

	playit playitState

	// stopGrace ist die Zeit, die ein Server zum sauberen Herunterfahren bekommt, bevor er beendet wird.
	stopGrace time.Duration

	mu      sync.RWMutex
	servers map[string]*Server

	// Die vergebenen Host-Ports stehen getrennt, damit die Vergabe nie Server-Sperren braucht.
	portMu sync.Mutex
	ports  map[string][]PortMapping
}

// NewManager legt einen Manager an. Load liest die vorhandenen Server.
func NewManager(cfg Config, rt Runtime, sw Software, logger *slog.Logger) *Manager {
	return &Manager{cfg: cfg, rt: rt, sw: sw, log: logger, stopGrace: 60 * time.Second, servers: map[string]*Server{}, ports: map[string][]PortMapping{}, availableMemory: AvailableMemory}
}

// checkMemory lehnt den Start eines Servers ab, wenn auf der Maschine danach weniger als die Reserve frei wäre.
func (m *Manager) checkMemory(memoryMB int) error {
	if m.cfg.MemoryReserveMB <= 0 || m.availableMemory == nil {
		return nil
	}
	available := m.availableMemory() >> 20
	if available <= 0 {
		return nil // unbekannt
	}
	needed := int64(memoryMB + m.cfg.MemoryReserveMB)
	if available < needed {
		return fmt.Errorf("%w: Zu wenig freier Arbeitsspeicher auf der Maschine (frei: %d MB, nötig: %d MB inklusive %d MB Reserve). Stoppe einen anderen Server oder verkleinere diesen", ErrConflict, available, needed, m.cfg.MemoryReserveMB)
	}
	return nil
}

func (m *Manager) serversDir() string { return filepath.Join(m.cfg.DataDir, "servers") }

// Load liest alle gespeicherten Server von der Platte.
func (m *Manager) Load() error {
	if err := os.MkdirAll(m.serversDir(), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(m.serversDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(m.serversDir(), entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "spec.json"))
		if err != nil {
			continue
		}
		var spec Spec
		if err := json.Unmarshal(data, &spec); err != nil {
			m.log.Warn("spec.json ist ungültig, der Server wird übersprungen", "dir", dir, "error", err)
			continue
		}
		if spec.ID != entry.Name() {
			m.log.Warn("die ID in spec.json passt nicht zum Ordner, der Server wird übersprungen", "dir", dir)
			continue
		}
		installed, java := spec.Installed, spec.JavaMajor
		if err := spec.Normalize(); err != nil {
			m.log.Warn("spec.json ist ungültig, der Server wird übersprungen", "dir", dir, "error", err)
			continue
		}
		spec.Installed, spec.JavaMajor = installed, java
		server, err := newServer(m, dir, spec)
		if err != nil {
			return err
		}
		m.servers[spec.ID] = server
		m.setPorts(spec.ID, spec.Ports)
	}
	return nil
}

// Reconcile gleicht mit Docker ab: Läuft der Container eines Servers schon (der Daemon wurde neu gestartet), wird er
// wieder übernommen.
func (m *Manager) Reconcile(ctx context.Context) error {
	refs, err := m.rt.List(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		server, ok := m.Get(ref.ServerID)
		if !ok {
			continue
		}
		server.mu.Lock()
		server.container = ref.ID
		active := server.state != StateOffline
		server.mu.Unlock()
		if !ref.Running || active {
			continue
		}
		conn, err := m.rt.Attach(ctx, ref.ID, true)
		if err != nil {
			m.log.Warn("Laufender Server konnte nicht übernommen werden", "server", ref.ServerID, "error", err)
			continue
		}
		server.mu.Lock()
		server.setStateLocked(StateRunning, "")
		server.mu.Unlock()
		server.logf("Der Daemon wurde neu gestartet und hat den laufenden Server wieder übernommen")
		server.adoptRun(ref.ID, conn)
		m.log.Info("Laufender Server übernommen", "server", ref.ServerID)
	}
	return nil
}

// KeepReconciling versucht den Abgleich, bis Docker erreichbar ist (der Daemon startet auch ohne Docker).
func (m *Manager) KeepReconciling(ctx context.Context) {
	for {
		if err := m.Reconcile(ctx); err == nil {
			if err := m.EnsurePlayit(ctx); err != nil {
				m.log.Warn("Der playit-Agent ließ sich nicht starten", "error", err)
			}
			return
		} else {
			m.log.Warn("Docker ist nicht erreichbar, neuer Versuch in 10 Sekunden", "error", err, "host", os.Getenv("DOCKER_HOST"))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// Close gibt die Dateizugriffe aller Server frei. Laufende Container bleiben unberührt.
func (m *Manager) Close() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, server := range m.servers {
		server.close()
	}
}

// Get findet einen Server.
func (m *Manager) Get(id string) (*Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[id]
	return server, ok
}

// List liefert alle Server nach Name sortiert.
func (m *Manager) List() []*Server {
	m.mu.RLock()
	list := make([]*Server, 0, len(m.servers))
	for _, server := range m.servers {
		list = append(list, server)
	}
	m.mu.RUnlock()
	names := make(map[*Server]string, len(list))
	for _, server := range list {
		names[server] = strings.ToLower(server.snapshot().Name)
	}
	sort.Slice(list, func(i, j int) bool { return names[list[i]] < names[list[j]] })
	return list
}

// Create legt einen Server an und installiert ihn im Hintergrund.
func (m *Manager) Create(spec Spec) (*Server, error) {
	spec.Installed, spec.JavaMajor = false, 0
	if err := spec.Normalize(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if _, exists := m.servers[spec.ID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: die ID %q gibt es schon", ErrConflict, spec.ID)
	}
	m.mu.Unlock()
	if err := m.allocatePorts(&spec, spec.ID); err != nil {
		return nil, err
	}
	dir := filepath.Join(m.serversDir(), spec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.setPorts(spec.ID, nil)
		return nil, err
	}
	server, err := newServer(m, dir, spec)
	if err == nil {
		server.mu.Lock()
		err = server.saveLocked()
		server.mu.Unlock()
	}
	if err != nil {
		m.setPorts(spec.ID, nil)
		_ = os.RemoveAll(dir)
		return nil, err
	}
	m.mu.Lock()
	m.servers[spec.ID] = server
	m.mu.Unlock()
	go server.installAsync()
	return server, nil
}

// Delete löscht einen Server samt Dateien. Ein laufender Server wird vorher beendet.
func (m *Manager) Delete(id string) error {
	server, ok := m.Get(id)
	if !ok {
		return ErrNotFound
	}
	server.mu.Lock()
	state, container, done := server.state, server.container, server.done
	if state == StateInstalling {
		server.mu.Unlock()
		return ErrBadState
	}
	server.mu.Unlock()
	if state != StateOffline {
		_ = server.Kill()
		if done != nil {
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				return errors.New("der Server lässt sich nicht beenden")
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.rt.Remove(ctx, "lodestone-"+id); err != nil {
		return err
	}
	if container != "" {
		_ = m.rt.Remove(ctx, container)
	}
	m.mu.Lock()
	delete(m.servers, id)
	m.mu.Unlock()
	m.setPorts(id, nil)
	server.close()
	// Backups gehören zum Server und gehen mit ihm
	return errors.Join(os.RemoveAll(server.dir), os.RemoveAll(filepath.Join(m.cfg.DataDir, "backups", id)))
}

// ---------------------------------------------------------------- Ports

func (m *Manager) setPorts(id string, ports []PortMapping) {
	m.portMu.Lock()
	defer m.portMu.Unlock()
	if ports == nil {
		delete(m.ports, id)
		return
	}
	m.ports[id] = append([]PortMapping(nil), ports...)
}

// allocatePorts vergibt freie Host-Ports (Host 0) und prüft ausdrücklich gewünschte auf Kollisionen mit anderen
// Servern. Das Ergebnis wird für den Server vorgemerkt.
func (m *Manager) allocatePorts(spec *Spec, self string) error {
	m.portMu.Lock()
	defer m.portMu.Unlock()
	taken := map[string]bool{}
	for id, ports := range m.ports {
		if id == self {
			continue
		}
		for _, port := range ports {
			taken[fmt.Sprintf("%d/%s", port.Host, port.Protocol)] = true
		}
	}
	chosen := make([]PortMapping, len(spec.Ports))
	copy(chosen, spec.Ports)
	for i := range chosen {
		port := &chosen[i]
		if port.Host != 0 {
			if taken[fmt.Sprintf("%d/%s", port.Host, port.Protocol)] {
				return fmt.Errorf("%w: der Port %d/%s ist schon vergeben", ErrConflict, port.Host, port.Protocol)
			}
			taken[fmt.Sprintf("%d/%s", port.Host, port.Protocol)] = true
			continue
		}
		found := false
		for candidate := m.cfg.PortRangeStart; candidate <= m.cfg.PortRangeEnd; candidate++ {
			key := fmt.Sprintf("%d/%s", candidate, port.Protocol)
			if taken[key] || !portFree(candidate, port.Protocol) {
				continue
			}
			port.Host = candidate
			taken[key] = true
			found = true
			break
		}
		if !found {
			return fmt.Errorf("kein freier Port zwischen %d und %d", m.cfg.PortRangeStart, m.cfg.PortRangeEnd)
		}
	}
	spec.Ports = chosen
	m.ports[self] = append([]PortMapping(nil), chosen...)
	return nil
}

// portFree prüft, ob die Maschine den Port gerade frei hat.
func portFree(port int, protocol string) bool {
	address := fmt.Sprintf(":%d", port)
	if protocol == "udp" {
		conn, err := net.ListenPacket("udp", address)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
