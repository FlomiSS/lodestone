package node

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/lodestone-mc/lodestone/internal/software"
)

// PortMapping verbindet einen Port im Container mit einem Port der Maschine.
type PortMapping struct {
	Container int `json:"container"`
	// Host ist der Port auf der Maschine; 0 lässt den Daemon einen freien aus dem Bereich wählen.
	Host     int    `json:"host"`
	Protocol string `json:"protocol"` // tcp oder udp
}

// Spec beschreibt einen Server, wie ihn das Panel anlegt.
type Spec struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Software software.ID       `json:"software"`
	Version  string            `json:"version"`
	MemoryMB int               `json:"memoryMB"`
	CPU      float64           `json:"cpu"` // Kerne; 0 = unbegrenzt
	Ports    []PortMapping     `json:"ports"`
	Env      map[string]string `json:"env,omitempty"`
	// Startup überschreibt die Standard-Startzeile der Software.
	Startup string `json:"startup,omitempty"`
	// Image überschreibt das Docker-Image (sonst eclipse-temurin passend zur Java-Version).
	Image string `json:"image,omitempty"`
	// EULA bestätigt die Minecraft-EULA (https://aka.ms/MinecraftEULA).
	EULA bool `json:"eula"`
	// NoPublish verzichtet auf den Standard-Port auf der Maschine; der Server ist dann nur im Docker-Netzwerk
	// erreichbar (z. B. hinter einem Proxy). Ausdrücklich angegebene Ports werden trotzdem veröffentlicht.
	NoPublish bool `json:"noPublish,omitempty"`
	// BackupKeep ist die Zahl der Backups, die aufbewahrt werden (die ältesten werden gelöscht).
	BackupKeep int `json:"backupKeep"`

	// Autoconfig sind Änderungen an Dateien des Servers, die das Panel vorgibt (z. B. Proxy-Einstellungen).
	Autoconfig *Autoconfig `json:"autoconfig,omitempty"`

	// Vom Daemon gepflegt:
	Installed         bool `json:"installed"`
	JavaMajor         int  `json:"javaMajor,omitempty"`
	AutoconfigApplied int  `json:"autoconfigApplied,omitempty"`
}

var (
	idPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	envPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,200}$`)
)

const (
	minMemoryMB = 128
	maxMemoryMB = 1 << 20
	maxPorts    = 16
	maxEnv      = 64
)

func newID() string {
	buffer := make([]byte, 6)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

// Normalize ergänzt Standardwerte und prüft alles. Das Ergebnis ist sicher zu speichern und auszuführen.
func (s *Spec) Normalize() error {
	if s.ID == "" {
		s.ID = newID()
	}
	if !idPattern.MatchString(s.ID) {
		return errors.New("ungültige Server-ID (erlaubt: Kleinbuchstaben, Ziffern, Bindestrich)")
	}
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		s.Name = s.ID
	}
	if len(s.Name) > 64 {
		return errors.New("der Name darf höchstens 64 Zeichen haben")
	}
	info, ok := software.Lookup(s.Software)
	if !ok {
		return fmt.Errorf("unbekannte Software: %q", s.Software)
	}
	s.Version = strings.TrimSpace(s.Version)
	if s.Version == "" || len(s.Version) > 64 || strings.ContainsAny(s.Version, "/\\ ?#") {
		return fmt.Errorf("ungültige Version: %q", s.Version)
	}
	if s.MemoryMB == 0 {
		s.MemoryMB = 2048
	}
	if s.MemoryMB < minMemoryMB || s.MemoryMB > maxMemoryMB {
		return fmt.Errorf("der Arbeitsspeicher muss zwischen %d und %d MB liegen", minMemoryMB, maxMemoryMB)
	}
	if s.CPU < 0 || s.CPU > 256 {
		return errors.New("CPU muss zwischen 0 und 256 Kernen liegen")
	}
	if len(s.Ports) == 0 && !s.NoPublish {
		s.Ports = []PortMapping{{Container: info.DefaultPort}}
	}
	if s.BackupKeep == 0 {
		s.BackupKeep = 5
	}
	if s.BackupKeep < 0 || s.BackupKeep > 200 {
		return errors.New("die Zahl der Backups muss zwischen 1 und 200 liegen")
	}
	if s.Autoconfig != nil {
		if err := s.Autoconfig.Validate(); err != nil {
			return err
		}
	}
	if len(s.Ports) > maxPorts {
		return fmt.Errorf("höchstens %d Ports", maxPorts)
	}
	seen := map[string]bool{}
	for i := range s.Ports {
		port := &s.Ports[i]
		if port.Protocol == "" {
			port.Protocol = "tcp"
		}
		if port.Protocol != "tcp" && port.Protocol != "udp" {
			return fmt.Errorf("Protokoll muss tcp oder udp sein, nicht %q", port.Protocol)
		}
		if port.Container < 1 || port.Container > 65535 || port.Host < 0 || port.Host > 65535 {
			return errors.New("Ports müssen zwischen 1 und 65535 liegen")
		}
		key := fmt.Sprintf("%d/%s", port.Container, port.Protocol)
		if seen[key] {
			return fmt.Errorf("der Container-Port %s ist doppelt belegt", key)
		}
		seen[key] = true
	}
	if len(s.Env) > maxEnv {
		return fmt.Errorf("höchstens %d Umgebungsvariablen", maxEnv)
	}
	for key, value := range s.Env {
		if !envPattern.MatchString(key) {
			return fmt.Errorf("ungültiger Name einer Umgebungsvariable: %q", key)
		}
		if len(value) > 4096 || strings.ContainsRune(value, 0) {
			return fmt.Errorf("ungültiger Wert für %s", key)
		}
	}
	if len(s.Startup) > 4096 || strings.ContainsRune(s.Startup, 0) {
		return errors.New("ungültige Startzeile")
	}
	if s.Image != "" && !imagePattern.MatchString(s.Image) {
		return fmt.Errorf("ungültiges Docker-Image: %q", s.Image)
	}
	return nil
}
