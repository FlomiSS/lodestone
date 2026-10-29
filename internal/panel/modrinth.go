package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Modrinth fragt die Modrinth-API ab. Die Adresse ist austauschbar (Tests).
type Modrinth struct {
	Base string
	HTTP *http.Client
}

func NewModrinth() *Modrinth {
	return &Modrinth{Base: "https://api.modrinth.com/v2", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// MFile ist eine Datei einer Modrinth-Version.
type MFile struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Primary  bool   `json:"primary"`
	Size     int64  `json:"size"`
	Hashes   struct {
		SHA1   string `json:"sha1"`
		SHA512 string `json:"sha512"`
	} `json:"hashes"`
}

// MDependency ist eine Abhängigkeit einer Version.
type MDependency struct {
	ProjectID      string `json:"project_id"`
	VersionID      string `json:"version_id"`
	DependencyType string `json:"dependency_type"`
}

// MVersion ist eine Version eines Projekts.
type MVersion struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"project_id"`
	Name          string        `json:"name"`
	VersionNumber string        `json:"version_number"`
	VersionType   string        `json:"version_type"`
	GameVersions  []string      `json:"game_versions"`
	Loaders       []string      `json:"loaders"`
	Files         []MFile       `json:"files"`
	Dependencies  []MDependency `json:"dependencies"`
	DatePublished string        `json:"date_published"`
}

// PrimaryFile ist die Hauptdatei der Version.
func (v MVersion) PrimaryFile() (MFile, bool) {
	for _, file := range v.Files {
		if file.Primary {
			return file, true
		}
	}
	if len(v.Files) > 0 {
		return v.Files[0], true
	}
	return MFile{}, false
}

// MHit ist ein Treffer der Suche.
type MHit struct {
	ProjectID   string   `json:"project_id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Downloads   int      `json:"downloads"`
	IconURL     string   `json:"icon_url"`
	Categories  []string `json:"categories"`
	ProjectType string   `json:"project_type"`
	ServerSide  string   `json:"server_side"`
}

type MSearch struct {
	Hits      []MHit `json:"hits"`
	TotalHits int    `json:"total_hits"`
}

func (m *Modrinth) get(ctx context.Context, path string, query url.Values, out any) error {
	address := m.Base + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	return m.do(request, out)
}

func (m *Modrinth) post(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.Base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return m.do(request, out)
}

func (m *Modrinth) do(request *http.Request, out any) error {
	request.Header.Set("User-Agent", "lodestone-mc/lodestone (self-hosted panel)")
	response, err := m.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("Modrinth ist nicht erreichbar: %w", err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("Modrinth antwortet mit %d", response.StatusCode)
	}
	return json.Unmarshal(data, out)
}

// Search sucht Projekte, die zu den Loadern und der Spielversion passen.
func (m *Modrinth) Search(ctx context.Context, query string, loaders []string, gameVersion string, offset int) (MSearch, error) {
	loaderFacet := make([]string, len(loaders))
	for i, loader := range loaders {
		loaderFacet[i] = "categories:" + loader
	}
	facets := [][]string{loaderFacet}
	if gameVersion != "" {
		facets = append(facets, []string{"versions:" + gameVersion})
	}
	facets = append(facets, []string{"server_side:required", "server_side:optional"})
	encoded, _ := json.Marshal(facets)
	values := url.Values{"query": {query}, "facets": {string(encoded)}, "limit": {"20"}, "offset": {fmt.Sprint(offset)}}
	if query == "" {
		values.Set("index", "downloads")
	}
	var result MSearch
	return result, m.get(ctx, "/search", values, &result)
}

// Versions liefert die Versionen eines Projekts für Loader und Spielversion, die neueste zuerst.
func (m *Modrinth) Versions(ctx context.Context, project string, loaders []string, gameVersion string) ([]MVersion, error) {
	values := url.Values{}
	if len(loaders) > 0 {
		encoded, _ := json.Marshal(loaders)
		values.Set("loaders", string(encoded))
	}
	if gameVersion != "" {
		encoded, _ := json.Marshal([]string{gameVersion})
		values.Set("game_versions", string(encoded))
	}
	var versions []MVersion
	return versions, m.get(ctx, "/project/"+url.PathEscape(project)+"/version", values, &versions)
}

// Best wählt die neueste Release-Version (sonst die neueste überhaupt).
func Best(versions []MVersion) (MVersion, bool) {
	for _, version := range versions {
		if version.VersionType == "release" {
			return version, true
		}
	}
	if len(versions) > 0 {
		return versions[0], true
	}
	return MVersion{}, false
}

// ProjectInfo ist die Beschreibung eines Projekts.
type ProjectInfo struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	IconURL     string   `json:"icon_url"`
	Downloads   int      `json:"downloads"`
	Followers   int      `json:"followers"`
	ProjectType string   `json:"project_type"`
	Categories  []string `json:"categories"`
	ClientSide  string   `json:"client_side"`
	ServerSide  string   `json:"server_side"`
	SourceURL   string   `json:"source_url"`
	IssuesURL   string   `json:"issues_url"`
	WikiURL     string   `json:"wiki_url"`
	Updated     string   `json:"updated"`
	License     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"license"`
}

// Link ist die Seite des Projekts auf modrinth.com.
func (p ProjectInfo) Link() string {
	kind := p.ProjectType
	if kind == "" {
		kind = "project"
	}
	return "https://modrinth.com/" + kind + "/" + p.Slug
}

func (m *Modrinth) Project(ctx context.Context, id string) (ProjectInfo, error) {
	var info ProjectInfo
	return info, m.get(ctx, "/project/"+url.PathEscape(id), nil, &info)
}

// Projects holt mehrere Projekte auf einmal (nach ID oder Slug).
func (m *Modrinth) Projects(ctx context.Context, ids []string) (map[string]ProjectInfo, error) {
	result := map[string]ProjectInfo{}
	if len(ids) == 0 {
		return result, nil
	}
	encoded, _ := json.Marshal(ids)
	var list []ProjectInfo
	if err := m.get(ctx, "/projects", url.Values{"ids": {string(encoded)}}, &list); err != nil {
		return nil, err
	}
	for _, info := range list {
		result[info.ID] = info
	}
	return result, nil
}

// Version holt eine Version nach ID.
func (m *Modrinth) Version(ctx context.Context, id string) (MVersion, error) {
	var version MVersion
	return version, m.get(ctx, "/version/"+url.PathEscape(id), nil, &version)
}

// SearchModpacks sucht Modpacks, die sich auf einem Fabric-Server betreiben lassen (nur dort kann Lodestone sie
// installieren).
func (m *Modrinth) SearchModpacks(ctx context.Context, query string, offset int) (MSearch, error) {
	facets := [][]string{{"project_type:modpack"}, {"categories:fabric"}, {"server_side:required", "server_side:optional"}}
	encoded, _ := json.Marshal(facets)
	values := url.Values{"query": {query}, "facets": {string(encoded)}, "limit": {"20"}, "offset": {fmt.Sprint(offset)}}
	if query == "" {
		values.Set("index", "downloads")
	}
	var result MSearch
	return result, m.get(ctx, "/search", values, &result)
}

// ByHashes ordnet Dateien (SHA-1) ihren Versionen zu.
func (m *Modrinth) ByHashes(ctx context.Context, hashes []string) (map[string]MVersion, error) {
	result := map[string]MVersion{}
	if len(hashes) == 0 {
		return result, nil
	}
	err := m.post(ctx, "/version_files", map[string]any{"hashes": hashes, "algorithm": "sha1"}, &result)
	return result, err
}

// LatestForHashes liefert zu jeder Datei die neueste passende Version.
func (m *Modrinth) LatestForHashes(ctx context.Context, hashes, loaders []string, gameVersion string) (map[string]MVersion, error) {
	result := map[string]MVersion{}
	if len(hashes) == 0 {
		return result, nil
	}
	body := map[string]any{"hashes": hashes, "algorithm": "sha1", "loaders": loaders}
	if gameVersion != "" {
		body["game_versions"] = []string{gameVersion}
	}
	err := m.post(ctx, "/version_files/update", body, &result)
	return result, err
}

// loadersFor sagt, welche Modrinth-Loader zu einer Software passen.
func loadersFor(id string) []string {
	switch id {
	case "paper":
		return []string{"paper", "spigot", "bukkit"}
	case "purpur":
		return []string{"purpur", "paper", "spigot", "bukkit"}
	case "folia":
		return []string{"folia"}
	case "fabric":
		return []string{"fabric"}
	case "neoforge":
		return []string{"neoforge"}
	case "velocity":
		return []string{"velocity"}
	}
	return nil
}

// addonDir sagt, wohin Erweiterungen einer Software gehören.
func addonDir(id string) string {
	switch id {
	case "fabric", "neoforge":
		return "mods"
	case "vanilla":
		return ""
	}
	return "plugins"
}

var errNoVersion = errors.New("es gibt keine Version, die zu dieser Software und Spielversion passt")

// safeFilename verhindert, dass ein Dateiname aus der API aus dem Ordner ausbricht.
func safeFilename(name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." || !strings.HasSuffix(strings.ToLower(name), ".jar") {
		return "", false
	}
	return name, true
}
