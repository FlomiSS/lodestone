package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

var errAlreadyInstalled = errors.New("ist schon installiert")

// gameVersionFor liefert die Spielversion, nach der Modrinth gefiltert wird. Velocity-Plugins hängen nicht an einer.
func gameVersionFor(softwareName, version string) string {
	if softwareName == "velocity" {
		return ""
	}
	if softwareName == "neoforge" {
		return software.NeoForgeToMinecraft(version)
	}
	return version
}

// installedProjects nennt die Modrinth-Projekte, die auf dem Server schon liegen (SHA-1 der Dateien).
func (p *Panel) installedProjects(ctx context.Context, client *NodeClient, row ServerRow) map[string]bool {
	found := map[string]bool{}
	var addons []node.Addon
	if err := client.Do(ctx, http.MethodGet, "/api/servers/"+row.ID+"/addons", nil, &addons); err != nil {
		return found
	}
	hashes := []string{}
	for _, addon := range addons {
		if addon.SHA1 != "" {
			hashes = append(hashes, addon.SHA1)
		}
	}
	versions, err := p.modrinth.ByHashes(ctx, hashes)
	if err != nil {
		return found
	}
	for _, version := range versions {
		found[version.ProjectID] = true
	}
	return found
}

// installFromModrinth lädt die passende Version (oder die gewünschte) samt benötigten Abhängigkeiten auf den Server.
func (p *Panel) installFromModrinth(ctx context.Context, row ServerRow, project, versionID string, withDeps bool) ([]string, error) {
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return nil, err
	}
	dir := addonDir(row.Software)
	if dir == "" {
		return nil, fail(http.StatusBadRequest, "Vanilla kennt weder Plugins noch Mods")
	}
	views, err := p.nodeViews(ctx, row.NodeID)
	if err != nil {
		return nil, err
	}
	view, ok := views[row.ID]
	if !ok {
		return nil, errNotFound
	}
	installed := p.installedProjects(ctx, client, row)
	return p.installVersion(ctx, client, row, dir, gameVersionFor(row.Software, view.Version), project, versionID, withDeps, installed)
}

func (p *Panel) installVersion(ctx context.Context, client *NodeClient, row ServerRow, dir, gameVersion, project, versionID string, withDeps bool, installed map[string]bool) ([]string, error) {
	versions, err := p.modrinth.Versions(ctx, project, loadersFor(row.Software), gameVersion)
	if err != nil {
		return nil, err
	}
	var chosen MVersion
	var ok bool
	if versionID != "" {
		found := false
		for _, version := range versions {
			if version.ID == versionID {
				chosen, found = version, true
			}
		}
		if !found {
			return nil, errNoVersion
		}
	} else if chosen, ok = Best(versions); !ok {
		return nil, errNoVersion
	}
	if installed[chosen.ProjectID] {
		return nil, errAlreadyInstalled
	}
	file, hasFile := chosen.PrimaryFile()
	name, safe := safeFilename(file.Filename)
	if !hasFile || !safe {
		return nil, fail(http.StatusBadGateway, "die Version enthält keine passende .jar-Datei")
	}
	pull := node.PullRequest{URL: file.URL, Path: dir + "/" + name, SHA1: file.Hashes.SHA1}
	if err := p.pull(ctx, client, row.ID, pull); err != nil {
		return nil, err
	}
	installed[chosen.ProjectID] = true
	done := []string{name}
	if withDeps {
		for _, dependency := range chosen.Dependencies {
			if dependency.DependencyType != "required" || dependency.ProjectID == "" || installed[dependency.ProjectID] {
				continue
			}
			more, err := p.installVersion(ctx, client, row, dir, gameVersion, dependency.ProjectID, dependency.VersionID, false, installed)
			if err != nil && !errors.Is(err, errAlreadyInstalled) {
				return done, fmt.Errorf("benötigtes Projekt %s: %w", dependency.ProjectID, err)
			}
			done = append(done, more...)
		}
	}
	return done, nil
}

func (p *Panel) pull(ctx context.Context, client *NodeClient, serverID string, request node.PullRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	err := client.Do(ctx, http.MethodPost, "/api/servers/"+serverID+"/files/pull", request, nil)
	var nodeErr *NodeError
	if errors.As(err, &nodeErr) && nodeErr.Status == http.StatusConflict {
		return errAlreadyInstalled
	}
	return err
}

// ---------------------------------------------------------------- Modrinth-Routen

func (p *Panel) modrinthContext(r *http.Request, minimum int) (ServerRow, string, error) {
	row, _, _, err := p.requireServer(r, minimum)
	if err != nil {
		return row, "", err
	}
	if addonDir(row.Software) == "" {
		return row, "", fail(http.StatusBadRequest, "Vanilla kennt weder Plugins noch Mods")
	}
	views, err := p.nodeViews(r.Context(), row.NodeID)
	if err != nil {
		return row, "", err
	}
	view, ok := views[row.ID]
	if !ok {
		return row, "", errNotFound
	}
	return row, gameVersionFor(row.Software, view.Version), nil
}

func (p *Panel) modrinthSearch(w http.ResponseWriter, r *http.Request) {
	row, gameVersion, err := p.modrinthContext(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	result, err := p.modrinth.Search(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), loadersFor(row.Software), gameVersion, max(offset, 0))
	if err != nil {
		writeError(w, fail(http.StatusBadGateway, err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (p *Panel) modrinthProject(w http.ResponseWriter, r *http.Request) {
	row, gameVersion, err := p.modrinthContext(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	id := r.PathValue("project")
	info, err := p.modrinth.Project(r.Context(), id)
	if err != nil {
		writeError(w, fail(http.StatusBadGateway, err.Error()))
		return
	}
	versions, err := p.modrinth.Versions(r.Context(), id, loadersFor(row.Software), gameVersion)
	if err != nil {
		writeError(w, fail(http.StatusBadGateway, err.Error()))
		return
	}
	type brief struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		Number    string   `json:"versionNumber"`
		Type      string   `json:"type"`
		Published string   `json:"published"`
		Games     []string `json:"gameVersions"`
	}
	list := make([]brief, 0, len(versions))
	for _, version := range versions {
		list = append(list, brief{version.ID, version.Name, version.VersionNumber, version.VersionType, version.DatePublished, version.GameVersions})
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": info, "versions": list})
}

func (p *Panel) modrinthInstall(w http.ResponseWriter, r *http.Request) {
	row, _, err := p.modrinthContext(r, rankOperator)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Project   string `json:"project"`
		VersionID string `json:"versionId"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Project == "" || strings.ContainsAny(body.Project, "/?#") {
		writeError(w, fail(http.StatusBadRequest, "kein Projekt angegeben"))
		return
	}
	files, err := p.installFromModrinth(r.Context(), row, body.Project, body.VersionID, true)
	switch {
	case errors.Is(err, errAlreadyInstalled):
		writeError(w, fail(http.StatusConflict, "das ist schon installiert – nutze „Updates prüfen“"))
		return
	case errors.Is(err, errNoVersion):
		writeError(w, fail(http.StatusNotFound, err.Error()))
		return
	case err != nil:
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installed": files})
}

// ---------------------------------------------------------------- Updates

type updateEntry struct {
	File            string `json:"file"`
	Dir             string `json:"dir"`
	Name            string `json:"name"`
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	LatestFile      string `json:"latestFile,omitempty"`
	Known           bool   `json:"known"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

func (p *Panel) latestFor(ctx context.Context, row ServerRow, gameVersion string, addons []node.Addon) (map[string]MVersion, map[string]MVersion, error) {
	hashes := []string{}
	for _, addon := range addons {
		if addon.SHA1 != "" && addon.Enabled {
			hashes = append(hashes, addon.SHA1)
		}
	}
	current, err := p.modrinth.ByHashes(ctx, hashes)
	if err != nil {
		return nil, nil, err
	}
	latest, err := p.modrinth.LatestForHashes(ctx, hashes, loadersFor(row.Software), gameVersion)
	return current, latest, err
}

func (p *Panel) addonsCheck(w http.ResponseWriter, r *http.Request) {
	row, gameVersion, err := p.modrinthContext(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	var addons []node.Addon
	if err := client.Do(r.Context(), http.MethodGet, "/api/servers/"+row.ID+"/addons", nil, &addons); err != nil {
		writeError(w, err)
		return
	}
	current, latest, err := p.latestFor(r.Context(), row, gameVersion, addons)
	if err != nil {
		writeError(w, fail(http.StatusBadGateway, err.Error()))
		return
	}
	result := []updateEntry{}
	for _, addon := range addons {
		entry := updateEntry{File: addon.File, Dir: addon.Dir, Name: addon.Name, Current: addon.Version}
		if version, ok := current[addon.SHA1]; ok {
			entry.Known = true
			if next, ok := latest[addon.SHA1]; ok && next.ID != version.ID {
				entry.UpdateAvailable, entry.Latest = true, next.VersionNumber
				if file, ok := next.PrimaryFile(); ok {
					entry.LatestFile = file.Filename
				}
			}
		}
		result = append(result, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

func (p *Panel) addonsUpdate(w http.ResponseWriter, r *http.Request) {
	row, gameVersion, err := p.modrinthContext(r, rankOperator)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Files []string `json:"files"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	var addons []node.Addon
	if err := client.Do(r.Context(), http.MethodGet, "/api/servers/"+row.ID+"/addons", nil, &addons); err != nil {
		writeError(w, err)
		return
	}
	wanted := map[string]bool{}
	for _, file := range body.Files {
		wanted[file] = true
	}
	selected := []node.Addon{}
	for _, addon := range addons {
		if wanted[addon.File] {
			selected = append(selected, addon)
		}
	}
	current, latest, err := p.latestFor(r.Context(), row, gameVersion, selected)
	if err != nil {
		writeError(w, fail(http.StatusBadGateway, err.Error()))
		return
	}
	type outcome struct {
		File    string `json:"file"`
		Updated bool   `json:"updated"`
		To      string `json:"to,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	results := []outcome{}
	for _, addon := range selected {
		out := outcome{File: addon.File}
		next, ok := latest[addon.SHA1]
		cur := current[addon.SHA1]
		file, hasFile := next.PrimaryFile()
		name, safe := safeFilename(file.Filename)
		switch {
		case !ok || next.ID == cur.ID:
			out.Error = "schon aktuell"
		case !hasFile || !safe:
			out.Error = "die neue Version hat keine passende Datei"
		default:
			err := p.pull(r.Context(), client, row.ID, node.PullRequest{URL: file.URL, Path: addon.Dir + "/" + name, SHA1: file.Hashes.SHA1})
			if err == nil && name != addon.File {
				query := url.Values{"path": {addon.Dir + "/" + addon.File}}
				err = client.Do(r.Context(), http.MethodDelete, "/api/servers/"+row.ID+"/files?"+query.Encode(), nil, nil)
			}
			if err != nil && !errors.Is(err, errAlreadyInstalled) {
				out.Error = err.Error()
			} else {
				out.Updated, out.To = true, next.VersionNumber
			}
		}
		results = append(results, out)
	}
	writeJSON(w, http.StatusOK, results)
}
