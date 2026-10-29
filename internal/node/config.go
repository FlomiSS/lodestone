// Package node ist der Daemon, der auf jeder Maschine läuft: Er legt Minecraft-Server als Docker-Container an,
// startet und stoppt sie, streamt die Konsole und verwaltet ihre Dateien.
package node

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config ist die Einstellung des Daemons (config.json).
type Config struct {
	// Listen ist die Adresse der HTTP-Schnittstelle.
	Listen string `json:"listen"`
	// Token ist der Schlüssel, mit dem sich das Panel anmeldet.
	Token string `json:"token"`
	// DataDir enthält Server-Beschreibungen und -Dateien.
	DataDir string `json:"dataDir"`
	// ContainerUser ist "uid:gid", unter dem Server im Container laufen. Leer heißt: unter Linux der Benutzer des
	// Daemons, sonst (Docker Desktop) der Standard des Images.
	ContainerUser string `json:"containerUser"`
	// PortRangeStart und PortRangeEnd begrenzen die automatisch vergebenen Host-Ports.
	PortRangeStart int `json:"portRangeStart"`
	PortRangeEnd   int `json:"portRangeEnd"`
	// MaxUploadMB begrenzt einen Datei-Upload.
	MaxUploadMB int `json:"maxUploadMB"`
	// DockerHost überschreibt DOCKER_HOST, z. B. unix:///var/run/docker.sock.
	DockerHost string `json:"dockerHost,omitempty"`
	// PublicHost ist die Adresse, unter der Spieler diese Maschine erreichen (für Anzeigen und Voicechat).
	PublicHost string `json:"publicHost,omitempty"`
	// Network ist das Docker-Netzwerk, in dem alle Server laufen. Dort erreichen sie sich über ihren Container-Namen.
	Network string `json:"network"`
	// PullHosts sind die Hosts, von denen Plugins und Mods geladen werden dürfen ("*" erlaubt jeden).
	PullHosts []string `json:"pullHosts"`
	// MemoryReserveMB ist der Arbeitsspeicher, der nach dem Start eines Servers auf der Maschine noch frei sein muss.
	// Ein Start, der das unterschreiten würde, wird abgelehnt – lieber ein Fehler als eine Maschine, die in den
	// Swap rutscht und nicht mehr antwortet. 0 schaltet die Prüfung ab.
	MemoryReserveMB int `json:"memoryReserveMB"`
	// PlayitImage ist das Docker-Image des playit-Agenten (leer = das offizielle).
	PlayitImage string `json:"playitImage,omitempty"`
}

// DefaultConfig liefert die Standardwerte (ohne Token).
func DefaultConfig() Config {
	return Config{
		Listen:          "127.0.0.1:8480",
		DataDir:         "data",
		PortRangeStart:  25565,
		PortRangeEnd:    25999,
		MaxUploadMB:     512,
		Network:         "lodestone",
		MemoryReserveMB: 1024,
		PullHosts: []string{"cdn.modrinth.com", "github.com", "githubusercontent.com", "hangarcdn.papermc.io",
			"fill-data.papermc.io", "api.papermc.io", "maven.fabricmc.net", "meta.fabricmc.net", "maven.neoforged.net", "cdn.discordapp.com"},
	}
}

// LoadConfig liest die Config. Gibt es sie nicht, wird sie mit einem zufälligen Token angelegt (created = true).
func LoadConfig(path string) (config Config, created bool, err error) {
	config = DefaultConfig()
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist) && os.Getenv("LODESTONE_NODE_TOKEN") != "":
		// Läuft der Daemon im Container, kommt alles aus der Umgebung und es wird keine Datei angelegt
	case errors.Is(err, os.ErrNotExist):
		token, tokenErr := randomToken()
		if tokenErr != nil {
			return config, false, tokenErr
		}
		config.Token = token
		if err := writeConfig(path, config); err != nil {
			return config, false, err
		}
		created = true
	case err != nil:
		return config, false, err
	default:
		if err := json.Unmarshal(data, &config); err != nil {
			return config, false, fmt.Errorf("%s ist keine gültige Config: %w", path, err)
		}
	}
	config.applyEnvironment()
	if err := config.normalize(filepath.Dir(path)); err != nil {
		return config, created, err
	}
	return config, created, nil
}

// applyEnvironment lässt Umgebungsvariablen (für Docker) die Datei überschreiben.
func (c *Config) applyEnvironment() {
	text := func(name string, into *string) {
		if value := os.Getenv(name); value != "" {
			*into = value
		}
	}
	text("LODESTONE_NODE_TOKEN", &c.Token)
	text("LODESTONE_NODE_LISTEN", &c.Listen)
	text("LODESTONE_NODE_DATA_DIR", &c.DataDir)
	text("LODESTONE_NODE_CONTAINER_USER", &c.ContainerUser)
	text("LODESTONE_NODE_PUBLIC_HOST", &c.PublicHost)
	text("LODESTONE_NODE_NETWORK", &c.Network)
	text("DOCKER_HOST_OVERRIDE", &c.DockerHost)
	text("LODESTONE_NODE_PLAYIT_IMAGE", &c.PlayitImage)
	if value := os.Getenv("LODESTONE_NODE_MEMORY_RESERVE_MB"); value != "" {
		var megabytes int
		if _, err := fmt.Sscanf(value, "%d", &megabytes); err == nil && megabytes >= 0 {
			c.MemoryReserveMB = megabytes
		}
	}
	if value := os.Getenv("LODESTONE_NODE_PORT_RANGE"); value != "" {
		var from, to int
		if _, err := fmt.Sscanf(value, "%d-%d", &from, &to); err == nil {
			c.PortRangeStart, c.PortRangeEnd = from, to
		}
	}
}

func writeConfig(path string, config Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func (c *Config) normalize(base string) error {
	if len(c.Token) < 24 {
		return errors.New("token in der Config ist zu kurz (mindestens 24 Zeichen)")
	}
	if c.Listen == "" {
		c.Listen = DefaultConfig().Listen
	}
	if c.DataDir == "" {
		c.DataDir = DefaultConfig().DataDir
	}
	if !filepath.IsAbs(c.DataDir) {
		c.DataDir = filepath.Join(base, c.DataDir)
	}
	if abs, err := filepath.Abs(c.DataDir); err == nil {
		c.DataDir = abs
	}
	if c.PortRangeStart <= 0 || c.PortRangeEnd < c.PortRangeStart || c.PortRangeEnd > 65535 {
		c.PortRangeStart, c.PortRangeEnd = DefaultConfig().PortRangeStart, DefaultConfig().PortRangeEnd
	}
	if c.MaxUploadMB <= 0 {
		c.MaxUploadMB = DefaultConfig().MaxUploadMB
	}
	if c.Network == "" {
		c.Network = DefaultConfig().Network
	}
	if len(c.PullHosts) == 0 {
		c.PullHosts = DefaultConfig().PullHosts
	}
	if c.ContainerUser == "" && runtime.GOOS == "linux" {
		c.ContainerUser = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}
	return nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
