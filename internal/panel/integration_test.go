package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lodestone-mc/lodestone/internal/node"
)

// fakeModrinth beantwortet die wenigen Aufrufe, die das Panel macht.
func fakeModrinth(t *testing.T) *Modrinth {
	t.Helper()
	file := func(name string) map[string]any {
		return map[string]any{"url": "https://cdn.modrinth.com/data/x/" + name, "filename": name, "primary": true, "size": 10, "hashes": map[string]string{"sha1": "sha1-" + name, "sha512": "sha512-" + name}}
	}
	versions := map[string]map[string]any{
		"v1":        {"id": "v1", "project_id": "pack1", "name": "Pack 1.0", "version_number": "1.0", "version_type": "release", "game_versions": []string{"1.21.4"}, "loaders": []string{"fabric"}, "files": []any{file("pack-1.0.mrpack")}, "date_published": "2026-01-01T00:00:00Z"},
		"v-forge":   {"id": "v-forge", "project_id": "pack1", "name": "Forge", "version_number": "1.0-forge", "version_type": "release", "game_versions": []string{"1.20.1"}, "loaders": []string{"forge"}, "files": []any{file("pack-forge.mrpack")}},
		"v-other":   {"id": "v-other", "project_id": "pack2", "name": "Anderes", "version_number": "9", "version_type": "release", "game_versions": []string{"1.21.4"}, "loaders": []string{"fabric"}, "files": []any{file("other.mrpack")}},
		"mod-v1":    {"id": "mod-v1", "project_id": "mod1", "name": "Cool 1", "version_number": "1.0.0", "version_type": "release", "game_versions": []string{"1.21.4"}, "loaders": []string{"paper"}, "files": []any{file("cool-1.0.0.jar")}, "date_published": "2025-01-01T00:00:00Z"},
		"mod-v2":    {"id": "mod-v2", "project_id": "mod1", "name": "Cool 2", "version_number": "2.0.0", "version_type": "release", "game_versions": []string{"1.21.4"}, "loaders": []string{"paper"}, "files": []any{file("cool-2.0.0.jar")}, "date_published": "2026-01-01T00:00:00Z"},
		"mod-alien": {"id": "mod-alien", "project_id": "mod2", "name": "Fremd", "version_number": "1", "version_type": "release", "game_versions": []string{"1.21.4"}, "loaders": []string{"paper"}, "files": []any{file("alien.jar")}},
	}
	projects := map[string]map[string]any{
		"pack1": {"id": "pack1", "slug": "mypack", "title": "My Pack", "project_type": "modpack", "icon_url": "https://cdn.modrinth.com/icon.png", "downloads": 5, "license": map[string]string{"id": "MIT", "name": "MIT"}},
		"pack2": {"id": "pack2", "slug": "other", "title": "Other Pack", "project_type": "modpack"},
		"mod1":  {"id": "mod1", "slug": "cool-mod", "title": "Cool Mod", "description": "Macht Dinge", "project_type": "mod", "downloads": 1000, "source_url": "https://github.com/x/cool", "issues_url": "https://github.com/x/cool/issues", "license": map[string]string{"id": "MIT", "name": "MIT-Lizenz"}},
		"mod2":  {"id": "mod2", "slug": "alien", "title": "Alien", "project_type": "mod"},
	}
	byProject := map[string][]string{"pack1": {"v1", "v-forge"}, "mod1": {"mod-v2", "mod-v1"}}

	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, value any) { writeJSON(w, 200, value) }
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"total_hits": 1, "hits": []any{map[string]any{"project_id": "pack1", "slug": "mypack", "title": "My Pack", "project_type": "modpack", "server_side": "required"}}})
	})
	mux.HandleFunc("GET /project/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		for _, project := range projects {
			if project["id"] == id || project["slug"] == id {
				reply(w, project)
				return
			}
		}
		writeJSON(w, 404, map[string]string{})
	})
	mux.HandleFunc("GET /project/{id}/version", func(w http.ResponseWriter, r *http.Request) {
		list := []any{}
		key := r.PathValue("id")
		for id, project := range projects { // wie bei Modrinth: ID oder Slug
			if project["slug"] == key {
				key = id
			}
		}
		for _, id := range byProject[key] {
			list = append(list, versions[id])
		}
		reply(w, list)
	})
	mux.HandleFunc("GET /version/{id}", func(w http.ResponseWriter, r *http.Request) {
		if version, ok := versions[r.PathValue("id")]; ok {
			reply(w, version)
			return
		}
		writeJSON(w, 404, map[string]string{})
	})
	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		_ = json.Unmarshal([]byte(r.URL.Query().Get("ids")), &ids)
		list := []any{}
		for _, id := range ids {
			if project, ok := projects[id]; ok {
				list = append(list, project)
			}
		}
		reply(w, list)
	})
	mux.HandleFunc("POST /version_files", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Hashes []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		result := map[string]any{}
		for _, hash := range body.Hashes {
			switch hash {
			case "sha1-installed":
				result[hash] = versions["mod-v1"]
			case "sha1-alien":
				result[hash] = versions["mod-alien"]
			}
		}
		reply(w, result)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return &Modrinth{Base: server.URL, HTTP: server.Client()}
}

func TestModpackSearchAndVersions(t *testing.T) {
	p, admin, _, _, _, _ := serverProject(t)
	p.modrinth = fakeModrinth(t)

	var search MSearch
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/modpacks/search?q=pack", nil), &search)
	if len(search.Hits) != 1 || search.Hits[0].ProjectType != "modpack" {
		t.Fatalf("Suche: %+v", search)
	}
	var detail struct {
		Link     string
		Versions []struct {
			ID           string
			GameVersions []string
		}
	}
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/modpacks/mypack", nil), &detail)
	if detail.Link != "https://modrinth.com/modpack/mypack" || len(detail.Versions) != 2 {
		t.Fatalf("Versionen: %+v", detail)
	}
	admin.expect(400, "GET", "/api/modpacks/cool-mod", nil) // kein Modpack
}

func TestServerFromModpack(t *testing.T) {
	p, web, fake, nodeID := setupPanel(t)
	p.modrinth = fakeModrinth(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	var project ProjectDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Pack", "kind": "server"}), &project)
	path := "/api/projects/" + project.ID + "/servers"

	// Ablehnungen: EULA, fremde Version, Forge
	admin.expect(400, "POST", path, serverRequest{Name: "Pack", NodeID: nodeID, Modpack: &modpackRef{Project: "mypack", VersionID: "v1"}})
	admin.expect(400, "POST", path, serverRequest{Name: "Pack", NodeID: nodeID, EULA: true, Modpack: &modpackRef{Project: "mypack", VersionID: "v-other"}})
	body := admin.expect(400, "POST", path, serverRequest{Name: "Pack", NodeID: nodeID, EULA: true, Modpack: &modpackRef{Project: "mypack", VersionID: "v-forge"}})
	if !strings.Contains(string(body), "Fabric") || !strings.Contains(string(body), "forge") {
		t.Errorf("Meldung zu Forge: %s", body)
	}
	admin.expect(404, "POST", path, serverRequest{Name: "Pack", NodeID: nodeID, EULA: true, Modpack: &modpackRef{Project: "mypack", VersionID: "gibtsnicht"}})
	admin.expect(400, "POST", path, serverRequest{Name: "Pack", NodeID: nodeID, EULA: true, Modpack: &modpackRef{Project: "mypack", VersionID: "../x"}})
	fake.mu.Lock()
	created := len(fake.servers)
	fake.mu.Unlock()
	if created != 0 {
		t.Fatalf("bei Ablehnung darf auf dem Node nichts entstehen: %d", created)
	}

	var server ServerDTO
	_ = json.Unmarshal(admin.expect(201, "POST", path, serverRequest{Name: "Pack", NodeID: nodeID, EULA: true, Software: "paper", Version: "9.9", Modpack: &modpackRef{Project: "mypack", VersionID: "v1"}}), &server)
	if server.Software != "fabric" || server.View == nil || server.View.Version != "1.21.4" || server.View.MemoryMB != 4096 {
		t.Fatalf("Software und Version kommen aus dem Modpack, nicht aus der Anfrage: %+v %+v", server.ServerRow, server.View)
	}
	if server.Modpack == nil || server.Modpack.Title != "My Pack" || server.Modpack.VersionNumber != "1.0" || server.Modpack.Slug != "mypack" {
		t.Fatalf("Modpack-Angaben: %+v", server.Modpack)
	}

	// Der Panel schickt dem Node das Modpack, sobald der Server installiert ist
	waitUntil(t, "Modpack beim Node", func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.modpacks) == 1
	})
	fake.mu.Lock()
	request := fake.modpacks[0]
	fake.mu.Unlock()
	if request.URL != "https://cdn.modrinth.com/data/x/pack-1.0.mrpack" || request.SHA1 != "sha1-pack-1.0.mrpack" || request.SHA512 != "sha512-pack-1.0.mrpack" {
		t.Errorf("Anfrage an den Node: %+v", request)
	}
}

func TestAddonDetailsAndVersionSwitch(t *testing.T) {
	p, admin, fake, nodeID, server, _ := serverProject(t)
	p.modrinth = fakeModrinth(t)
	fake.mu.Lock()
	fake.addons = []node.Addon{
		{File: "cool-1.0.0.jar", Dir: "plugins", Enabled: true, Name: "Cool", Version: "1.0.0", SHA1: "sha1-installed"},
		{File: "eigenbau.jar", Dir: "plugins", Enabled: true, Name: "Eigenbau", Version: "1", SHA1: "sha1-unbekannt"},
	}
	fake.mu.Unlock()
	refresh(p, nodeID)

	var details []addonDetail
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/servers/"+server.ID+"/addons/details", nil), &details)
	if len(details) != 2 {
		t.Fatalf("Details: %+v", details)
	}
	known, unknown := details[0], details[1]
	if !known.Known || known.Title != "Cool Mod" || known.VersionNumber != "1.0.0" || known.Link != "https://modrinth.com/mod/cool-mod" ||
		known.VersionLink != "https://modrinth.com/mod/cool-mod/version/mod-v1" || known.License != "MIT-Lizenz" || known.SourceURL == "" {
		t.Errorf("bekannte Datei: %+v", known)
	}
	if unknown.Known || unknown.Title != "" {
		t.Errorf("eine Datei, die Modrinth nicht kennt, hat keine Angaben: %+v", unknown)
	}

	// Version wechseln: neue Datei laden, alte entfernen
	admin.expect(200, "POST", "/api/servers/"+server.ID+"/addons/switch", map[string]string{"file": "cool-1.0.0.jar", "versionId": "mod-v2"})
	fake.mu.Lock()
	pulled, deleted := append([]string{}, fake.pulled...), append([]string{}, fake.removed...)
	fake.mu.Unlock()
	if len(pulled) != 1 || pulled[0] != "plugins/cool-2.0.0.jar" {
		t.Errorf("geladen: %v", pulled)
	}
	if len(deleted) != 1 || deleted[0] != "plugins/cool-1.0.0.jar" {
		t.Errorf("entfernt: %v", deleted)
	}

	// Ablehnungen: dieselbe Version, ein anderes Projekt, eine Datei von außerhalb von Modrinth, eine fehlende Datei
	admin.expect(409, "POST", "/api/servers/"+server.ID+"/addons/switch", map[string]string{"file": "cool-1.0.0.jar", "versionId": "mod-v1"})
	admin.expect(400, "POST", "/api/servers/"+server.ID+"/addons/switch", map[string]string{"file": "cool-1.0.0.jar", "versionId": "mod-alien"})
	admin.expect(409, "POST", "/api/servers/"+server.ID+"/addons/switch", map[string]string{"file": "eigenbau.jar", "versionId": "mod-v2"})
	admin.expect(404, "POST", "/api/servers/"+server.ID+"/addons/switch", map[string]string{"file": "gibtsnicht.jar", "versionId": "mod-v2"})
}

func TestPublicAddressValidation(t *testing.T) {
	_, admin, _, _, server, _ := serverProject(t)
	path := "/api/servers/" + server.ID + "/public-address"
	for _, good := range []string{"mc.example.org", "abc.joinmc.link:12345", "MC.Example.ORG", ""} {
		admin.expect(200, "PUT", path, map[string]string{"address": good})
	}
	for _, bad := range []string{"http://x.de", "a b", "x.de:99999", "x.de:abc", ":25565", "-x.de", "x.de/pfad", "<script>"} {
		admin.expect(400, "PUT", path, map[string]string{"address": bad})
	}
	var dto ServerDTO
	_ = json.Unmarshal(admin.expect(200, "PUT", path, map[string]string{"address": "Mc.Example.org:25000"}), &dto)
	if dto.PublicAddress != "mc.example.org:25000" {
		t.Errorf("Adresse wird klein geschrieben gespeichert: %q", dto.PublicAddress)
	}
}

func TestPlayitIsAdminOnlyAndKeepsTheKeySecret(t *testing.T) {
	p, admin, fake, nodeID, _, projectID := serverProject(t)
	path := "/api/nodes/" + nodeID + "/playit"

	var status map[string]any
	_ = json.Unmarshal(admin.expect(200, "PUT", path, map[string]string{"secret": "a1b2c3d4e5f60718293a4b5c6d7e8f90"}), &status)
	fake.mu.Lock()
	got := fake.playitSecret
	fake.mu.Unlock()
	if got != "a1b2c3d4e5f60718293a4b5c6d7e8f90" || status["configured"] != true {
		t.Fatalf("Schlüssel beim Node: %q, Antwort: %v", got, status)
	}
	body := string(admin.expect(200, "GET", path, nil))
	if strings.Contains(body, "a1b2c3d4") {
		t.Errorf("die Antwort darf den Schlüssel nicht enthalten: %s", body)
	}
	admin.expect(204, "DELETE", path, nil)

	// Betrachter dürfen nichts davon
	admin.expect(201, "POST", "/api/users", map[string]any{"username": "betrachter", "password": "ein-gutes-passwort"})
	users, _ := p.store.ListUsers()
	for _, user := range users {
		if user.Username == "betrachter" {
			admin.expect(200, "PUT", "/api/projects/"+projectID+"/members/"+user.ID, map[string]string{"role": "viewer"})
		}
	}
	viewer := newClient(t, admin.base)
	viewer.expect(200, "POST", "/api/auth/login", map[string]string{"username": "betrachter", "password": "ein-gutes-passwort"})
	viewer.expect(403, "GET", path, nil)
	viewer.expect(403, "PUT", path, map[string]string{"secret": "a1b2c3d4e5f60718293a4b5c6d7e8f90"})
	viewer.expect(403, "DELETE", path, nil)
}
