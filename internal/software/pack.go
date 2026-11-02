package software

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// PackMeta beschreibt die Metadaten eines Position-Packs in meta.json.
type PackMeta struct {
	ID           ID           `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Version      string       `json:"version,omitempty"`
	Author       string       `json:"author,omitempty"`
	Kind         Kind         `json:"kind"`               // server oder proxy
	Extensions   Extensions   `json:"extensions"`         // plugins, mods oder none
	DefaultPort  int          `json:"defaultPort"`        // Standard: 25565
	Startup      string       `json:"startup"`            // Startbefehl mit {{MEMORY}} und {{JAR}}
	StopCommand  string       `json:"stopCommand"`        // Standard: stop (oder end bei Proxy)
	ReadyPattern string       `json:"readyPattern"`       // Konsolen-Muster bei Bereitschaft
	Java         PackJava     `json:"java,omitempty"`     // Java-Versionsregeln
	Provider     PackProvider `json:"provider,omitempty"` // Bezugsquelle für Versionen/Downloads
	Backend      PackBackend  `json:"backend,omitempty"`  // Velocity-Kompatibilität
}

// PackJava steuert, welche Java-Laufzeitumgebung gewählt wird.
type PackJava struct {
	Strategy string `json:"strategy,omitempty"` // mojang, velocity oder fixed
	Major    int    `json:"major,omitempty"`    // z. B. 21
}

// PackProvider steuert, wie Versionen gefunden und Artefakte heruntergeladen werden.
type PackProvider struct {
	Type        string   `json:"type"`                  // paper, fabric, purpur, leaf, neoforge, forge, vanilla, velocity, quilt, static, direct
	Versions    []string `json:"versions,omitempty"`    // feste Liste für static
	DownloadURL string   `json:"downloadUrl,omitempty"` // Direkter Download-Link mit {{VERSION}}
	FileName    string   `json:"fileName,omitempty"`    // Dateiname mit {{VERSION}}
}

// PackBackend beschreibt, ob und wie der Server hinter Velocity läuft.
type PackBackend struct {
	Supported       bool   `json:"supported"`                 // kann als Backend laufen
	ProxyMod        string `json:"proxyMod,omitempty"`        // Modrinth-Slug, z. B. fabricproxy-lite
	VoiceConfigFile string `json:"voiceConfigFile,omitempty"` // Datei für Simple Voice Chat
}

// PackAutoconfig beschreibt Dateiänderungen und Einstellungen in autoconfig.json.
type PackAutoconfig struct {
	Backend    *PackAutoconfigTarget `json:"backend,omitempty"`
	Proxy      *PackAutoconfigTarget `json:"proxy,omitempty"`
	Standalone *PackAutoconfigTarget `json:"standalone,omitempty"`
	Voicechat  *PackAutoconfigVoice  `json:"voicechat,omitempty"`
}

// PackAutoconfigTarget bündelt Konfigurationsschritte für eine Rolle.
type PackAutoconfigTarget struct {
	Edits   []PackEdit  `json:"edits,omitempty"`
	Writes  []PackWrite `json:"writes,omitempty"`
	Restart bool        `json:"restart,omitempty"`
	Reload  string      `json:"reload,omitempty"`
}

// PackEdit ändert eine Einstellungsdatei.
type PackEdit struct {
	File   string      `json:"file"`
	Format string      `json:"format"` // properties, yaml, toml
	Create bool        `json:"create,omitempty"`
	Set    []PackSetOp `json:"set,omitempty"`
}

// PackSetOp setzt einen Wert an einem Pfad.
type PackSetOp struct {
	Path  []string `json:"path"`
	Value any      `json:"value"`
}

// PackWrite legt eine Datei an.
type PackWrite struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PackAutoconfigVoice konfiguriert Simple Voice Chat.
type PackAutoconfigVoice struct {
	File   string      `json:"file"`
	Format string      `json:"format"`
	Set    []PackSetOp `json:"set,omitempty"`
}

// PositionPack ist ein im Speicher geladenes Position-Pack.
type PositionPack struct {
	Meta       PackMeta       `json:"meta"`
	Autoconfig PackAutoconfig `json:"autoconfig"`
	Script     string         `json:"script,omitempty"`     // Inhalt von autoconfig.sh
	Custom     bool           `json:"custom"`               // vom Benutzer hochgeladen
	Overridden bool           `json:"overridden,omitempty"` // überschreibt ein Standard-Pack
	ZipSize    int64          `json:"zipSize"`
}

// PackSummary fasst ein Position-Pack für Übersichten zusammen.
type PackSummary struct {
	ID            ID         `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Version       string     `json:"version"`
	Author        string     `json:"author"`
	Kind          Kind       `json:"kind"`
	Extensions    Extensions `json:"extensions"`
	DefaultPort   int        `json:"defaultPort"`
	Custom        bool       `json:"custom"`
	Overridden    bool       `json:"overridden,omitempty"`
	ZipSize       int64      `json:"zipSize"`
	HasAutoconfig bool       `json:"hasAutoconfig"`
	HasScript     bool       `json:"hasScript"`
	CanBeBackend  bool       `json:"canBeBackend"`
	CanVoice      bool       `json:"canVoice"`
}

// Summary liefert die kompakte Übersicht eines Position-Packs.
func (p *PositionPack) Summary() PackSummary {
	hasAutoconfig := p.Autoconfig.Backend != nil || p.Autoconfig.Proxy != nil || p.Autoconfig.Standalone != nil || p.Autoconfig.Voicechat != nil
	canBackend := p.Meta.Backend.Supported || (p.Autoconfig.Backend != nil && len(p.Autoconfig.Backend.Edits) > 0)
	canVoice := p.Meta.Backend.VoiceConfigFile != "" || p.Autoconfig.Voicechat != nil
	return PackSummary{
		ID:            p.Meta.ID,
		Name:          p.Meta.Name,
		Description:   p.Meta.Description,
		Version:       p.Meta.Version,
		Author:        p.Meta.Author,
		Kind:          p.Meta.Kind,
		Extensions:    p.Meta.Extensions,
		DefaultPort:   p.Meta.DefaultPort,
		Custom:        p.Custom,
		Overridden:    p.Overridden,
		ZipSize:       p.ZipSize,
		HasAutoconfig: hasAutoconfig,
		HasScript:     strings.TrimSpace(p.Script) != "",
		CanBeBackend:  canBackend,
		CanVoice:      canVoice,
	}
}

// Info liefert die Software-Info für den bestehenden Katalog.
func (p *PositionPack) Info() Info {
	return Info{
		ID:           p.Meta.ID,
		Name:         p.Meta.Name,
		Kind:         p.Meta.Kind,
		Extensions:   p.Meta.Extensions,
		DefaultPort:  p.Meta.DefaultPort,
		Startup:      p.Meta.Startup,
		StopCommand:  p.Meta.StopCommand,
		ReadyPattern: p.Meta.ReadyPattern,
		Description:  p.Meta.Description,
		CanBeBackend: p.Meta.Backend.Supported || (p.Autoconfig.Backend != nil && len(p.Autoconfig.Backend.Edits) > 0),
		CanVoice:     p.Meta.Backend.VoiceConfigFile != "" || p.Autoconfig.Voicechat != nil,
		Custom:       p.Custom,
	}
}

var idSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidateMeta prüft die Metadaten eines Position-Packs.
func (m *PackMeta) Validate() error {
	m.ID = ID(strings.ToLower(strings.TrimSpace(string(m.ID))))
	if !idSlugPattern.MatchString(string(m.ID)) {
		return errors.New("die ID darf nur aus Kleinbuchstaben, Ziffern, Unter- und Bindestrichen bestehen (1-32 Zeichen)")
	}
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" || len(m.Name) > 64 {
		return errors.New("der Name braucht 1 bis 64 Zeichen")
	}
	switch m.Kind {
	case KindServer, KindProxy:
	default:
		return fmt.Errorf("ungültiger Software-Typ %q (server oder proxy erwartet)", m.Kind)
	}
	switch m.Extensions {
	case ExtPlugins, ExtMods, ExtNone:
	default:
		return fmt.Errorf("ungültige Erweiterung %q (plugins, mods oder none erwartet)", m.Extensions)
	}
	if m.DefaultPort <= 0 || m.DefaultPort > 65535 {
		m.DefaultPort = 25565
	}
	if strings.TrimSpace(m.Startup) == "" {
		if m.Kind == KindProxy {
			m.Startup = javaStartup
		} else {
			m.Startup = javaStartup + " nogui"
		}
	}
	if strings.TrimSpace(m.StopCommand) == "" {
		if m.Kind == KindProxy {
			m.StopCommand = "end"
		} else {
			m.StopCommand = "stop"
		}
	}
	if strings.TrimSpace(m.ReadyPattern) == "" {
		if m.Kind == KindProxy {
			m.ReadyPattern = `Done \(|Listening on`
		} else {
			m.ReadyPattern = `Done \(`
		}
	}
	return nil
}

// Limits für ZIP-Einträge und Position-Pack-Daten
const (
	MaxPackMetaSize       = 2 << 20 // 2 MB
	MaxPackAutoconfigSize = 2 << 20 // 2 MB
	MaxPackScriptSize     = 1 << 20 // 1 MB
	MaxPackJSONSize       = 8 << 20 // 8 MB
)

// Validate prüft die Metadaten und Grenzwerte eines Position-Packs.
func (p *PositionPack) Validate() error {
	if err := p.Meta.Validate(); err != nil {
		return err
	}
	if len(p.Script) > MaxPackScriptSize {
		return fmt.Errorf("das Skript überschreitet die Höchstgröße von %d Bytes", MaxPackScriptSize)
	}
	hasAutoconfig := p.Autoconfig.Backend != nil || p.Autoconfig.Proxy != nil || p.Autoconfig.Standalone != nil || p.Autoconfig.Voicechat != nil
	if hasAutoconfig {
		acBytes, err := json.Marshal(p.Autoconfig)
		if err != nil {
			return fmt.Errorf("autoconfig ungültig: %w", err)
		}
		if len(acBytes) > MaxPackAutoconfigSize {
			return fmt.Errorf("die Autokonfiguration überschreitet die Höchstgröße von %d Bytes", MaxPackAutoconfigSize)
		}
	}
	return nil
}

// ReadZipPack liest ein Position-Pack aus einem ZIP-Archiv.
func ReadZipPack(r io.ReaderAt, size int64) (*PositionPack, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("ungültiges ZIP-Archiv: %w", err)
	}

	pack := &PositionPack{ZipSize: size}
	var metaData, autoconfigData []byte

	for _, file := range zr.File {
		clean := filepath.Clean(file.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return nil, errors.New("ungültiger Dateipfad im Archiv")
		}
		base := strings.ToLower(filepath.Base(clean))
		if file.FileInfo().IsDir() {
			continue
		}

		if base == "meta.json" || base == "position-pack.json" || base == "pack.json" {
			data, err := readZipEntry(file, MaxPackMetaSize)
			if err != nil {
				return nil, err
			}
			metaData = data
		} else if base == "autoconfig.json" {
			data, err := readZipEntry(file, MaxPackAutoconfigSize)
			if err != nil {
				return nil, err
			}
			autoconfigData = data
		} else if base == "autoconfig.sh" || base == "script.sh" {
			data, err := readZipEntry(file, MaxPackScriptSize)
			if err != nil {
				return nil, err
			}
			pack.Script = string(data)
		}
	}

	if len(metaData) == 0 {
		return nil, errors.New("die Datei meta.json fehlt im Position-Pack")
	}

	if err := json.Unmarshal(metaData, &pack.Meta); err != nil {
		return nil, fmt.Errorf("meta.json ist kein gültiges JSON: %w", err)
	}

	if len(autoconfigData) > 0 {
		if err := json.Unmarshal(autoconfigData, &pack.Autoconfig); err != nil {
			return nil, fmt.Errorf("autoconfig.json ist kein gültiges JSON: %w", err)
		}
	}

	if err := pack.Validate(); err != nil {
		return nil, err
	}

	return pack, nil
}

func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("die Datei %s im Position-Pack überschreitet die Höchstgröße von %d Bytes", f.Name, limit)
	}
	return data, nil
}

// CreateZipPack packt ein Position-Pack in ein ZIP-Archiv.
func CreateZipPack(pack *PositionPack) ([]byte, error) {
	if err := pack.Validate(); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	metaBytes, err := json.MarshalIndent(pack.Meta, "", "  ")
	if err != nil {
		return nil, err
	}
	mf, err := zw.Create("meta.json")
	if err != nil {
		return nil, err
	}
	if _, err := mf.Write(metaBytes); err != nil {
		return nil, err
	}

	hasAutoconfig := pack.Autoconfig.Backend != nil || pack.Autoconfig.Proxy != nil || pack.Autoconfig.Standalone != nil || pack.Autoconfig.Voicechat != nil
	if hasAutoconfig {
		acBytes, err := json.MarshalIndent(pack.Autoconfig, "", "  ")
		if err != nil {
			return nil, err
		}
		af, err := zw.Create("autoconfig.json")
		if err != nil {
			return nil, err
		}
		if _, err := af.Write(acBytes); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(pack.Script) != "" {
		sf, err := zw.Create("autoconfig.sh")
		if err != nil {
			return nil, err
		}
		if _, err := sf.Write([]byte(pack.Script)); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------- PackManager

// PackManager verwaltet eingebaute und eigene Position-Packs.
type PackManager struct {
	mu    sync.RWMutex
	dir   string
	packs map[ID]*PositionPack
}

var (
	globalManagerMu sync.RWMutex
	globalManager   *PackManager
)

// InitPackManager initialisiert den globalen PackManager mit Verzeichnis.
func InitPackManager(dir string) *PackManager {
	globalManagerMu.Lock()
	defer globalManagerMu.Unlock()
	globalManager = NewPackManager(dir)
	return globalManager
}

// GetPackManager liefert den aktiven PackManager.
func GetPackManager() *PackManager {
	globalManagerMu.Lock()
	defer globalManagerMu.Unlock()
	if globalManager == nil {
		globalManager = NewPackManager("")
	}
	return globalManager
}

// GetPack ist eine Abkürzung zum Nachschlagen eines Position-Packs.
func GetPack(id ID) (*PositionPack, bool) {
	return GetPackManager().Get(id)
}

// NewPackManager baut einen neuen PackManager mit den Standard-Packs.
func NewPackManager(dir string) *PackManager {
	pm := &PackManager{
		dir:   dir,
		packs: make(map[ID]*PositionPack),
	}
	pm.loadDefaults()
	if dir != "" {
		_ = os.MkdirAll(dir, 0o700)
		pm.loadFromDir(dir)
	}
	return pm
}

func (pm *PackManager) loadDefaults() {
	defaults := DefaultPacks()
	for _, p := range defaults {
		pm.packs[p.Meta.ID] = p
	}
}

func (pm *PackManager) loadFromDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".positionpack") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		pack, err := ReadZipPack(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			continue
		}
		markPackOrigin(pack)
		pack.ZipSize = int64(len(data))
		pm.packs[pack.Meta.ID] = pack
	}
}

// All liefert alle registrierten Position-Packs.
func (pm *PackManager) All() []*PositionPack {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]*PositionPack, 0, len(pm.packs))
	for _, p := range pm.packs {
		out = append(out, p)
	}
	return out
}

// Summaries liefert Zusammenfassungen aller Position-Packs.
func (pm *PackManager) Summaries() []PackSummary {
	packs := pm.All()
	out := make([]PackSummary, len(packs))
	for i, p := range packs {
		out[i] = p.Summary()
	}
	return out
}

// Get findet ein Position-Pack anhand seiner ID.
func (pm *PackManager) Get(id ID) (*PositionPack, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.packs[id]
	return p, ok
}

// Add fügt ein neues Position-Pack aus ZIP-Daten hinzu und speichert es.
func (pm *PackManager) Add(zipData []byte) (*PositionPack, error) {
	pack, err := ReadZipPack(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}
	markPackOrigin(pack)
	pack.ZipSize = int64(len(zipData))

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.dir != "" {
		dst := filepath.Join(pm.dir, string(pack.Meta.ID)+".zip")
		if err := os.WriteFile(dst, zipData, 0o600); err != nil {
			return nil, fmt.Errorf("Position-Pack konnte nicht gespeichert werden: %w", err)
		}
	}
	pm.packs[pack.Meta.ID] = pack
	return pack, nil
}

// Save aktualisiert ein Position-Pack und speichert es als ZIP.
func (pm *PackManager) Save(pack *PositionPack) error {
	if err := pack.Validate(); err != nil {
		return err
	}
	markPackOrigin(pack)
	zipBytes, err := CreateZipPack(pack)
	if err != nil {
		return err
	}
	pack.ZipSize = int64(len(zipBytes))

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.dir != "" {
		dst := filepath.Join(pm.dir, string(pack.Meta.ID)+".zip")
		if err := os.WriteFile(dst, zipBytes, 0o600); err != nil {
			return fmt.Errorf("Position-Pack konnte nicht gespeichert werden: %w", err)
		}
	}
	pm.packs[pack.Meta.ID] = pack
	return nil
}

// RegisterMemory registriert ein Position-Pack direkt im Speicher (z. B. für Node-Synchronisation).
func (pm *PackManager) RegisterMemory(pack *PositionPack) error {
	if err := pack.Validate(); err != nil {
		return err
	}
	markPackOrigin(pack)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.packs[pack.Meta.ID] = pack
	return nil
}

// Delete entfernt ein benutzerdefiniertes Position-Pack oder setzt ein überschriebenes Standard-Pack zurück.
func (pm *PackManager) Delete(id ID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pack, ok := pm.packs[id]
	if !ok {
		return ErrNotFound
	}
	defaultP, isDefault := defaultPack(id)
	if isDefault && !pack.Overridden {
		return errors.New("Standard-Position-Packs können nicht gelöscht werden")
	}

	if pm.dir != "" {
		dst := filepath.Join(pm.dir, string(id)+".zip")
		_ = os.Remove(dst)
		_ = os.Remove(filepath.Join(pm.dir, string(id)+".positionpack"))
	}
	if isDefault {
		cloned := *defaultP
		pm.packs[id] = &cloned
	} else {
		delete(pm.packs, id)
	}
	return nil
}

// Export liefert die ZIP-Bytes eines Position-Packs.
func (pm *PackManager) Export(id ID) ([]byte, error) {
	pm.mu.RLock()
	pack, ok := pm.packs[id]
	pm.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return CreateZipPack(pack)
}

func defaultPack(id ID) (*PositionPack, bool) {
	for _, p := range DefaultPacks() {
		if p.Meta.ID == id {
			return p, true
		}
	}
	return nil, false
}

func isDefaultID(id ID) bool {
	_, ok := defaultPack(id)
	return ok
}

func markPackOrigin(pack *PositionPack) {
	if isDefaultID(pack.Meta.ID) {
		pack.Custom = false
		pack.Overridden = true
	} else {
		pack.Custom = true
		pack.Overridden = false
	}
}

// ---------------------------------------------------------------- Default Packs

// DefaultPacks liefert die mitgelieferten Standard-Position-Packs.
func DefaultPacks() []*PositionPack {
	return []*PositionPack{
		defaultPaperPack(),
		defaultFabricPack(),
		defaultPurpurPack(),
		defaultVelocityPack(),
		defaultNeoForgePack(),
		defaultFoliaPack(),
		defaultLeafPack(),
		defaultForgePack(),
		defaultVanillaPack(),
	}
}

func defaultPaperPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Paper,
			Name:         "Paper",
			Description:  "Hochperformante Minecraft-Server-Software mit Plugin-Unterstützung.",
			Version:      "1.0.0",
			Author:       "PaperMC / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtPlugins,
			DefaultPort:  25565,
			Startup:      javaStartup + " nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "paper"},
			Backend: PackBackend{
				Supported:       true,
				VoiceConfigFile: "plugins/voicechat/voicechat-server.properties",
			},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Restart: true,
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
					{
						File:   "config/paper-global.yml",
						Format: "yaml",
						Set: []PackSetOp{
							{Path: []string{"proxies", "velocity", "enabled"}, Value: true},
							{Path: []string{"proxies", "velocity", "online-mode"}, Value: true},
							{Path: []string{"proxies", "velocity", "secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
			Voicechat: &PackAutoconfigVoice{
				File:   "plugins/voicechat/voicechat-server.properties",
				Format: "properties",
				Set: []PackSetOp{
					{Path: []string{"port"}, Value: "{{VOICE_PORT}}"},
					{Path: []string{"voice_host"}, Value: "{{VOICE_HOST}}"},
				},
			},
		},
	}
}

func defaultFabricPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Fabric,
			Name:         "Fabric",
			Description:  "Leichtgewichtige, modulare Modding-Toolchain für Minecraft.",
			Version:      "1.0.0",
			Author:       "FabricMC / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtMods,
			DefaultPort:  25565,
			Startup:      javaStartup + " nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "fabric"},
			Backend: PackBackend{
				Supported:       true,
				ProxyMod:        "fabricproxy-lite",
				VoiceConfigFile: "config/voicechat/voicechat-server.properties",
			},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Restart: true,
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
					{
						File:   "config/FabricProxy-Lite.toml",
						Format: "toml",
						Create: true,
						Set: []PackSetOp{
							{Path: []string{"hackOnlineMode"}, Value: true},
							{Path: []string{"hackEarlySend"}, Value: false},
							{Path: []string{"hackMessageChain"}, Value: true},
							{Path: []string{"disconnectMessage"}, Value: "Dieser Server ist nur über den Proxy erreichbar."},
							{Path: []string{"secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
			Voicechat: &PackAutoconfigVoice{
				File:   "config/voicechat/voicechat-server.properties",
				Format: "properties",
				Set: []PackSetOp{
					{Path: []string{"port"}, Value: "{{VOICE_PORT}}"},
					{Path: []string{"voice_host"}, Value: "{{VOICE_HOST}}"},
				},
			},
		},
	}
}

func defaultPurpurPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Purpur,
			Name:         "Purpur",
			Description:  "Paper-Abspaltung mit Fokus auf Konfigurierbarkeit und Gameplay-Optionen.",
			Version:      "1.0.0",
			Author:       "PurpurMC / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtPlugins,
			DefaultPort:  25565,
			Startup:      javaStartup + " nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "purpur"},
			Backend: PackBackend{
				Supported:       true,
				VoiceConfigFile: "plugins/voicechat/voicechat-server.properties",
			},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Restart: true,
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
					{
						File:   "config/paper-global.yml",
						Format: "yaml",
						Set: []PackSetOp{
							{Path: []string{"proxies", "velocity", "enabled"}, Value: true},
							{Path: []string{"proxies", "velocity", "online-mode"}, Value: true},
							{Path: []string{"proxies", "velocity", "secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
			Voicechat: &PackAutoconfigVoice{
				File:   "plugins/voicechat/voicechat-server.properties",
				Format: "properties",
				Set: []PackSetOp{
					{Path: []string{"port"}, Value: "{{VOICE_PORT}}"},
					{Path: []string{"voice_host"}, Value: "{{VOICE_HOST}}"},
				},
			},
		},
	}
}

func defaultVelocityPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Velocity,
			Name:         "Velocity",
			Description:  "Moderner, extrem schneller Minecraft-Proxy von PaperMC.",
			Version:      "1.0.0",
			Author:       "PaperMC / Lodestone",
			Kind:         KindProxy,
			Extensions:   ExtPlugins,
			DefaultPort:  25565,
			Startup:      javaStartup,
			StopCommand:  "end",
			ReadyPattern: `Done \(|Listening on`,
			Java:         PackJava{Strategy: "velocity", Major: 21},
			Provider:     PackProvider{Type: "velocity"},
			Backend:      PackBackend{Supported: false},
		},
		Autoconfig: PackAutoconfig{
			Proxy: &PackAutoconfigTarget{
				Restart: true,
				Reload:  "velocity reload",
				Edits: []PackEdit{
					{
						File:   "velocity.toml",
						Format: "toml",
						Set: []PackSetOp{
							{Path: []string{"bind"}, Value: "0.0.0.0:{{PROXY_PORT}}"},
							{Path: []string{"player-info-forwarding-mode"}, Value: "modern"},
						},
					},
				},
				Writes: []PackWrite{
					{Path: "forwarding.secret", Content: "{{SECRET}}"},
				},
			},
		},
	}
}

func defaultNeoForgePack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           NeoForge,
			Name:         "NeoForge",
			Description:  "Moderne Modding-Plattform für Minecraft.",
			Version:      "1.0.0",
			Author:       "NeoForged / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtMods,
			DefaultPort:  25565,
			Startup:      installerStartup,
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "neoforge"},
			Backend: PackBackend{
				Supported:       true,
				ProxyMod:        "proxy-compatible-forge",
				VoiceConfigFile: "config/voicechat/voicechat-server.properties",
			},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Restart: true,
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
					{
						File:   "config/proxy-compatible-forge.toml",
						Format: "toml",
						Create: true,
						Set: []PackSetOp{
							{Path: []string{"forwarding", "enabled"}, Value: true},
							{Path: []string{"forwarding", "mode"}, Value: "MODERN"},
							{Path: []string{"forwarding", "secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
			Voicechat: &PackAutoconfigVoice{
				File:   "config/voicechat/voicechat-server.properties",
				Format: "properties",
				Set: []PackSetOp{
					{Path: []string{"port"}, Value: "{{VOICE_PORT}}"},
					{Path: []string{"voice_host"}, Value: "{{VOICE_HOST}}"},
				},
			},
		},
	}
}

func defaultFoliaPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Folia,
			Name:         "Folia",
			Description:  "Multi-Threaded Paper-Server für sehr große Spielerzahlen.",
			Version:      "1.0.0",
			Author:       "PaperMC / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtPlugins,
			DefaultPort:  25565,
			Startup:      javaStartup + " nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "folia"},
			Backend: PackBackend{
				Supported:       true,
				VoiceConfigFile: "plugins/voicechat/voicechat-server.properties",
			},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Restart: true,
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
					{
						File:   "config/paper-global.yml",
						Format: "yaml",
						Set: []PackSetOp{
							{Path: []string{"proxies", "velocity", "enabled"}, Value: true},
							{Path: []string{"proxies", "velocity", "online-mode"}, Value: true},
							{Path: []string{"proxies", "velocity", "secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
		},
	}
}

func defaultLeafPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Leaf,
			Name:         "Leaf",
			Description:  "Gabelung von Paper und Gale für maximale Performance.",
			Version:      "1.0.0",
			Author:       "LeafMC / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtPlugins,
			DefaultPort:  25565,
			Startup:      javaStartup + " nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "leaf"},
			Backend: PackBackend{
				Supported:       true,
				VoiceConfigFile: "plugins/voicechat/voicechat-server.properties",
			},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Restart: true,
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
					{
						File:   "config/paper-global.yml",
						Format: "yaml",
						Set: []PackSetOp{
							{Path: []string{"proxies", "velocity", "enabled"}, Value: true},
							{Path: []string{"proxies", "velocity", "online-mode"}, Value: true},
							{Path: []string{"proxies", "velocity", "secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
		},
	}
}

func defaultForgePack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Forge,
			Name:         "Forge",
			Description:  "Der klassische Mod-Loader für Minecraft.",
			Version:      "1.0.0",
			Author:       "Minecraft Forge / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtMods,
			DefaultPort:  25565,
			Startup:      installerStartup,
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "forge"},
			Backend:      PackBackend{Supported: false},
		},
	}
}

func defaultVanillaPack() *PositionPack {
	return &PositionPack{
		Meta: PackMeta{
			ID:           Vanilla,
			Name:         "Vanilla",
			Description:  "Offizielle Server-Software von Mojang ohne Modifikationen.",
			Version:      "1.0.0",
			Author:       "Mojang / Lodestone",
			Kind:         KindServer,
			Extensions:   ExtNone,
			DefaultPort:  25565,
			Startup:      javaStartup + " nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         PackJava{Strategy: "mojang", Major: 21},
			Provider:     PackProvider{Type: "vanilla"},
			Backend:      PackBackend{Supported: false},
		},
	}
}
