package node

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

// Addon ist ein Plugin oder Mod im Serverordner.
type Addon struct {
	File     string `json:"file"`
	Dir      string `json:"dir"` // plugins oder mods
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Enabled  bool   `json:"enabled"`
	Name     string `json:"name"`
	ID       string `json:"id,omitempty"`
	Version  string `json:"version"`
	// Kind sagt, wofür die Datei gebaut ist: paper, velocity, fabric, forge, neoforge oder unknown.
	Kind string `json:"kind"`
	SHA1 string `json:"sha1"`
}

var addonDirs = []string{"plugins", "mods"}

type addonCache struct {
	mu      sync.Mutex
	entries map[string]cachedAddon
}

type cachedAddon struct {
	size, modified int64
	addon          Addon
}

// Addons listet Plugins und Mods mit Name und Version aus ihrer Beschreibung und der SHA-1 der Datei.
func (s *Server) Addons() ([]Addon, error) {
	var result []Addon
	for _, dir := range addonDirs {
		entries, err := s.fs.List(dir)
		if err != nil {
			continue // den Ordner gibt es (noch) nicht
		}
		for _, entry := range entries {
			if entry.Dir {
				continue
			}
			enabled := strings.HasSuffix(strings.ToLower(entry.Name), ".jar")
			if !enabled && !strings.HasSuffix(strings.ToLower(entry.Name), ".jar.disabled") {
				continue
			}
			file := path.Join(dir, entry.Name)
			addon, ok := s.cachedAddon(file, entry)
			if !ok {
				addon = s.inspectAddon(file, entry)
				s.rememberAddon(file, entry, addon)
			}
			addon.Enabled = enabled
			result = append(result, addon)
		}
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name) })
	return result, nil
}

func (s *Server) cachedAddon(file string, entry Entry) (Addon, bool) {
	s.addons.mu.Lock()
	defer s.addons.mu.Unlock()
	cached, ok := s.addons.entries[file]
	if ok && cached.size == entry.Size && cached.modified == entry.Modified {
		return cached.addon, true
	}
	return Addon{}, false
}

func (s *Server) rememberAddon(file string, entry Entry, addon Addon) {
	s.addons.mu.Lock()
	defer s.addons.mu.Unlock()
	if s.addons.entries == nil {
		s.addons.entries = map[string]cachedAddon{}
	}
	s.addons.entries[file] = cachedAddon{size: entry.Size, modified: entry.Modified, addon: addon}
}

func (s *Server) inspectAddon(file string, entry Entry) Addon {
	addon := Addon{File: file, Dir: path.Dir(file), Size: entry.Size, Modified: entry.Modified, Kind: "unknown",
		Name: strings.TrimSuffix(strings.TrimSuffix(entry.Name, ".disabled"), ".jar")}
	handle, _, err := s.fs.Open(file)
	if err != nil {
		return addon
	}
	defer handle.Close()
	hash := sha1.New()
	if _, err := io.Copy(hash, handle); err == nil {
		addon.SHA1 = hex.EncodeToString(hash.Sum(nil))
	}
	archive, err := zip.NewReader(handle, entry.Size)
	if err != nil {
		return addon
	}
	describeAddon(archive, &addon)
	return addon
}

var (
	yamlName    = regexp.MustCompile(`(?m)^name\s*:\s*['"]?([^'"\r\n#]+?)['"]?\s*$`)
	yamlVersion = regexp.MustCompile(`(?m)^version\s*:\s*['"]?([^'"\r\n#]+?)['"]?\s*$`)
)

func readEntry(archive *zip.Reader, name string) ([]byte, bool) {
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, false
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, 512<<10))
		return data, err == nil
	}
	return nil, false
}

// describeAddon liest die Beschreibung der Jar: plugin.yml (Paper/Spigot), velocity-plugin.json, fabric.mod.json,
// neoforge.mods.toml oder mods.toml.
func describeAddon(archive *zip.Reader, addon *Addon) {
	for _, name := range []string{"paper-plugin.yml", "plugin.yml"} {
		if data, ok := readEntry(archive, name); ok {
			addon.Kind = "paper"
			if m := yamlName.FindSubmatch(data); m != nil {
				addon.Name = strings.TrimSpace(string(m[1]))
			}
			if m := yamlVersion.FindSubmatch(data); m != nil {
				addon.Version = strings.TrimSpace(string(m[1]))
			}
			break
		}
	}
	if data, ok := readEntry(archive, "velocity-plugin.json"); ok {
		var meta struct{ ID, Name, Version string }
		if json.Unmarshal(data, &meta) == nil && meta.ID != "" {
			if addon.Kind == "paper" {
				addon.Kind = "paper+velocity"
			} else {
				addon.Kind = "velocity"
				addon.Name, addon.Version = firstNonEmpty(meta.Name, meta.ID), meta.Version
			}
			addon.ID = meta.ID
		}
	}
	if data, ok := readEntry(archive, "fabric.mod.json"); ok {
		var meta struct{ ID, Name, Version string }
		if json.Unmarshal(data, &meta) == nil && meta.ID != "" {
			addon.Kind, addon.ID = "fabric", meta.ID
			addon.Name, addon.Version = firstNonEmpty(meta.Name, meta.ID), meta.Version
		}
	}
	for _, entry := range []struct{ file, kind string }{{"META-INF/neoforge.mods.toml", "neoforge"}, {"META-INF/mods.toml", "forge"}} {
		data, ok := readEntry(archive, entry.file)
		if !ok {
			continue
		}
		var meta struct {
			Mods []struct {
				ModID       string `toml:"modId"`
				Version     string `toml:"version"`
				DisplayName string `toml:"displayName"`
			} `toml:"mods"`
		}
		if toml.Unmarshal(data, &meta) == nil && len(meta.Mods) > 0 {
			addon.Kind, addon.ID = entry.kind, meta.Mods[0].ModID
			addon.Name, addon.Version = firstNonEmpty(meta.Mods[0].DisplayName, meta.Mods[0].ModID), meta.Mods[0].Version
		}
		break
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ = fs.ErrNotExist
