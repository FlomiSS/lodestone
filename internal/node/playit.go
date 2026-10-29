package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Der playit-Agent verbindet die Maschine mit playit.gg, damit Server ohne Portfreigabe im Router erreichbar sind.
// Die Tunnel (welche öffentliche Adresse auf welchen lokalen Port zeigt) legt man im playit-Dashboard an; der Daemon
// startet und überwacht nur den Agenten mit dem Schlüssel des Benutzers.

const (
	playitContainer = "lodestone-playit"
	// DefaultPlayitImage ist das offizielle Image des Agenten.
	DefaultPlayitImage = "ghcr.io/playit-cloud/playit-agent:0.15"
	playitLogLines     = 30
)

var playitSecret = regexp.MustCompile(`^[A-Za-z0-9._-]{16,256}$`)

type playitFile struct {
	Secret string `json:"secret"`
}

// PlayitStatus beschreibt den Agenten; der Schlüssel selbst wird nie ausgegeben.
type PlayitStatus struct {
	Configured bool     `json:"configured"`
	Running    bool     `json:"running"`
	Image      string   `json:"image"`
	Logs       []string `json:"logs"`
	Error      string   `json:"error,omitempty"`
	// Since ist der Zeitpunkt der letzten Einrichtung (Millisekunden).
	Since int64 `json:"since,omitempty"`
}

type playitState struct {
	mu sync.Mutex
}

func (m *Manager) playitPath() string { return filepath.Join(m.cfg.DataDir, "playit.json") }

func (m *Manager) playitImage() string {
	if m.cfg.PlayitImage != "" {
		return m.cfg.PlayitImage
	}
	return DefaultPlayitImage
}

func (m *Manager) readPlayit() (string, time.Time, error) {
	info, err := os.Stat(m.playitPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	data, err := os.ReadFile(m.playitPath())
	if err != nil {
		return "", time.Time{}, err
	}
	var file playitFile
	if err := json.Unmarshal(data, &file); err != nil {
		return "", time.Time{}, fmt.Errorf("playit.json ist ungültig: %w", err)
	}
	return file.Secret, info.ModTime(), nil
}

// ConfigurePlayit speichert den Schlüssel und (neu) startet den Agenten.
func (m *Manager) ConfigurePlayit(ctx context.Context, secret string) error {
	secret = strings.TrimSpace(secret)
	if !playitSecret.MatchString(secret) {
		return badRequest("das ist kein gültiger playit-Schlüssel (der „Secret Key“ eines Agenten aus dem playit-Dashboard)")
	}
	m.playit.mu.Lock()
	defer m.playit.mu.Unlock()
	data, _ := json.Marshal(playitFile{Secret: secret})
	if err := os.WriteFile(m.playitPath(), append(data, '\n'), 0o600); err != nil {
		return err
	}
	return m.startPlayitLocked(ctx, true)
}

// RemovePlayit stoppt den Agenten und vergisst den Schlüssel.
func (m *Manager) RemovePlayit(ctx context.Context) error {
	m.playit.mu.Lock()
	defer m.playit.mu.Unlock()
	if err := m.rt.Remove(ctx, playitContainer); err != nil {
		return err
	}
	if err := os.Remove(m.playitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// EnsurePlayit sorgt dafür, dass der Agent läuft, wenn ein Schlüssel eingerichtet ist (nach einem Neustart des Daemons).
func (m *Manager) EnsurePlayit(ctx context.Context) error {
	m.playit.mu.Lock()
	defer m.playit.mu.Unlock()
	return m.startPlayitLocked(ctx, false)
}

func (m *Manager) startPlayitLocked(ctx context.Context, recreate bool) error {
	secret, _, err := m.readPlayit()
	if err != nil || secret == "" {
		return err
	}
	state, err := m.rt.Inspect(ctx, playitContainer)
	if err != nil {
		return err
	}
	if state.Exists && !recreate {
		if state.Running {
			return nil
		}
		return m.rt.Start(ctx, playitContainer)
	}
	if state.Exists {
		if err := m.rt.Remove(ctx, playitContainer); err != nil {
			return err
		}
	}
	image := m.playitImage()
	if !imagePattern.MatchString(image) {
		return fmt.Errorf("ungültiges Image für playit: %q", image)
	}
	if err := m.rt.EnsureImage(ctx, image); err != nil {
		return err
	}
	if _, err := m.rt.Create(ctx, ContainerSpec{
		Name:        playitContainer,
		Image:       image,
		Env:         []string{"SECRET_KEY=" + secret},
		Network:     "host", // der Agent muss die Server über 127.0.0.1 und ihre veröffentlichten Ports erreichen
		Restart:     "unless-stopped",
		MemoryBytes: 192 << 20,
		Labels:      map[string]string{LabelService: "playit"},
	}); err != nil {
		return err
	}
	return m.rt.Start(ctx, playitContainer)
}

// PlayitStatus liest Zustand und letzte Zeilen der Ausgabe (der Schlüssel wird entfernt).
func (m *Manager) PlayitStatus(ctx context.Context) PlayitStatus {
	status := PlayitStatus{Image: m.playitImage(), Logs: []string{}}
	secret, since, err := m.readPlayit()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	if secret == "" {
		return status
	}
	status.Configured = true
	status.Since = since.UnixMilli()
	state, err := m.rt.Inspect(ctx, playitContainer)
	if err != nil {
		status.Error = "Docker ist nicht erreichbar: " + err.Error()
		return status
	}
	status.Running = state.Running
	if !state.Exists {
		status.Error = "Der Agent-Container fehlt und wird neu angelegt."
		return status
	}
	logs, err := m.rt.Logs(ctx, playitContainer, playitLogLines)
	if err == nil {
		for _, line := range strings.Split(strings.ReplaceAll(logs, "\r", ""), "\n") {
			line = strings.TrimSpace(strings.ReplaceAll(line, secret, "••••"))
			if line != "" {
				status.Logs = append(status.Logs, line)
			}
		}
	}
	return status
}
