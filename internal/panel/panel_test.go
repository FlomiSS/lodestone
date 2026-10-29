package panel

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lodestone-mc/lodestone/internal/node"
)

// fakeNode ist ein Node, der nur merkt, was man ihm schickt.
type fakeNode struct {
	mu          sync.Mutex
	servers     map[string]node.View
	autoconfigs map[string]node.Autoconfig
	deleted     []string
	powered     []string
}

func newFakeNode(t *testing.T) (*fakeNode, *httptest.Server) {
	fake := &fakeNode{servers: map[string]node.View{}, autoconfigs: map[string]node.Autoconfig{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"version": "test", "docker": map[string]any{"dockerVersion": "x", "memoryBytes": int64(64) << 30}})
	})
	mux.HandleFunc("GET /api/servers", func(w http.ResponseWriter, _ *http.Request) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		list := []node.View{}
		for _, view := range fake.servers {
			list = append(list, view)
		}
		writeJSON(w, 200, list)
	})
	mux.HandleFunc("POST /api/servers", func(w http.ResponseWriter, r *http.Request) {
		var spec node.Spec
		_ = json.NewDecoder(r.Body).Decode(&spec)
		for i := range spec.Ports {
			if spec.Ports[i].Host == 0 {
				spec.Ports[i].Host = 28000 + len(fake.servers)*2 + i
			}
		}
		if len(spec.Ports) == 0 && !spec.NoPublish {
			spec.Ports = []node.PortMapping{{Container: 25565, Host: 28000 + len(fake.servers), Protocol: "tcp"}}
		}
		spec.Installed = true
		view := node.View{Spec: spec, State: "offline"}
		fake.mu.Lock()
		fake.servers[spec.ID] = view
		fake.mu.Unlock()
		writeJSON(w, 201, view)
	})
	mux.HandleFunc("GET /api/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		view, ok := fake.servers[r.PathValue("id")]
		fake.mu.Unlock()
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "nicht gefunden"})
			return
		}
		writeJSON(w, 200, view)
	})
	mux.HandleFunc("POST /api/servers/{id}/power", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Action string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		fake.mu.Lock()
		view, ok := fake.servers[r.PathValue("id")]
		if ok {
			if body.Action == "start" {
				view.State = node.StateRunning
			} else {
				view.State = node.StateOffline
			}
			fake.servers[r.PathValue("id")] = view
			fake.powered = append(fake.powered, body.Action+":"+r.PathValue("id"))
		}
		fake.mu.Unlock()
		writeJSON(w, 202, view)
	})
	mux.HandleFunc("PATCH /api/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		var patch node.Patch
		_ = json.NewDecoder(r.Body).Decode(&patch)
		fake.mu.Lock()
		view := fake.servers[r.PathValue("id")]
		if patch.Ports != nil {
			ports := *patch.Ports
			for i := range ports {
				if ports[i].Host == 0 {
					ports[i].Host = 28500 + i + len(fake.servers)
				}
			}
			view.Ports = ports
		}
		fake.servers[r.PathValue("id")] = view
		fake.mu.Unlock()
		writeJSON(w, 200, view)
	})
	mux.HandleFunc("PUT /api/servers/{id}/autoconfig", func(w http.ResponseWriter, r *http.Request) {
		var config node.Autoconfig
		_ = json.NewDecoder(r.Body).Decode(&config)
		fake.mu.Lock()
		fake.autoconfigs[r.PathValue("id")] = config
		fake.mu.Unlock()
		writeJSON(w, 200, map[string]any{"pending": true})
	})
	mux.HandleFunc("DELETE /api/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		delete(fake.servers, r.PathValue("id"))
		fake.deleted = append(fake.deleted, r.PathValue("id"))
		fake.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/servers/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, []string{"zeile"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return fake, server
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, headers ...string) (int, []byte) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, c.base+path, reader)
	if method != http.MethodGet {
		request.Header.Set(csrfHeader, "1")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i+1] == "" {
			request.Header.Del(headers[i])
		} else {
			request.Header.Set(headers[i], headers[i+1])
		}
	}
	response, err := c.http.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}

func (c *client) expect(want int, method, path string, body any) []byte {
	c.t.Helper()
	status, data := c.do(method, path, body)
	if status != want {
		c.t.Fatalf("%s %s: Status %d, erwartet %d: %s", method, path, status, want, data)
	}
	return data
}

func setupPanel(t *testing.T) (*Panel, *httptest.Server, *fakeNode, string) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	p := New(Config{Version: "test"}, store, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	web := httptest.NewServer(p.Handler())
	t.Cleanup(web.Close)
	fake, fakeServer := newFakeNode(t)
	created, err := store.CreateNode(Node{Name: "test", URL: fakeServer.URL, Token: "token", PublicHost: "mc.example.org"})
	if err != nil {
		t.Fatal(err)
	}
	return p, web, fake, created.ID
}

func TestAuthFlow(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	c := newClient(t, web.URL)

	// vor der Einrichtung ist nichts erlaubt
	if status, _ := c.do("GET", "/api/projects", nil); status != 401 {
		t.Fatalf("ohne Anmeldung: %d", status)
	}
	var state struct{ SetupRequired bool }
	_ = json.Unmarshal(c.expect(200, "GET", "/api/auth/status", nil), &state)
	if !state.SetupRequired {
		t.Fatal("Einrichtung sollte nötig sein")
	}
	c.expect(400, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "kurz"})
	c.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	c.expect(409, "POST", "/api/auth/setup", map[string]string{"username": "zweiter", "password": "ein-gutes-passwort"})

	// die Sitzung gilt; schreibende Anfragen brauchen den Schutz-Header
	c.expect(200, "GET", "/api/me", nil)
	if status, _ := c.do("POST", "/api/projects", map[string]string{"name": "x", "kind": "server"}, csrfHeader, ""); status != 403 {
		t.Errorf("ohne CSRF-Header: %d", status)
	}

	c.expect(204, "POST", "/api/auth/logout", nil)
	c.expect(401, "GET", "/api/me", nil)

	other := newClient(t, web.URL)
	other.expect(401, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "falsch-falsch"})
	other.expect(200, "POST", "/api/auth/login", map[string]string{"username": "ADMIN", "password": "ein-gutes-passwort"})
	other.expect(200, "GET", "/api/me", nil)
}

func TestLoginThrottle(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	c := newClient(t, web.URL)
	c.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	other := newClient(t, web.URL)
	for i := 0; i < throttleLimit; i++ {
		other.expect(401, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "falsch-falsch"})
	}
	// jetzt ist auch das richtige Passwort gesperrt
	other.expect(429, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
}

func TestProjectPermissions(t *testing.T) {
	_, web, _, nodeID := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	for _, name := range []string{"viewer", "operator", "fremd"} {
		admin.expect(201, "POST", "/api/users", map[string]any{"username": name, "password": "ein-gutes-passwort"})
	}
	var users []User
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/users", nil), &users)
	ids := map[string]string{}
	for _, user := range users {
		ids[user.Username] = user.ID
	}

	var project ProjectDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Mein Server", "kind": "server"}), &project)
	admin.expect(200, "PUT", "/api/projects/"+project.ID+"/members/"+ids["viewer"], map[string]string{"role": "viewer"})
	admin.expect(200, "PUT", "/api/projects/"+project.ID+"/members/"+ids["operator"], map[string]string{"role": "operator"})

	var created ServerDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/servers",
		serverRequest{Name: "Main", NodeID: nodeID, Software: "paper", Version: "1.21.4", EULA: true}), &created)
	// ein Server-Projekt hat genau einen Server
	admin.expect(409, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{Name: "Zwei", NodeID: nodeID, Software: "paper", Version: "1.21.4", EULA: true})

	login := func(name string) *client {
		c := newClient(t, web.URL)
		c.expect(200, "POST", "/api/auth/login", map[string]string{"username": name, "password": "ein-gutes-passwort"})
		return c
	}
	viewer, operator, stranger := login("viewer"), login("operator"), login("fremd")

	// wer nicht Mitglied ist, sieht das Projekt gar nicht
	stranger.expect(404, "GET", "/api/projects/"+project.ID, nil)
	stranger.expect(404, "GET", "/api/servers/"+created.ID, nil)
	if list := stranger.expect(200, "GET", "/api/projects", nil); strings.Contains(string(list), project.ID) {
		t.Error("Fremder sieht das Projekt in der Liste")
	}

	// Betrachter: lesen ja, Konsole/Einstellungen nein
	viewer.expect(200, "GET", "/api/servers/"+created.ID, nil)
	viewer.expect(200, "GET", "/api/servers/"+created.ID+"/logs", nil)
	viewer.expect(403, "POST", "/api/servers/"+created.ID+"/power", map[string]string{"action": "start"})
	viewer.expect(403, "PATCH", "/api/servers/"+created.ID, map[string]any{"memoryMB": 512})
	viewer.expect(403, "DELETE", "/api/servers/"+created.ID, nil)
	viewer.expect(403, "GET", "/api/servers/"+created.ID+"/files?path=/", nil) // Dateien sind nicht für Betrachter

	// Operator: Konsole ja (der Fake-Node kennt power nicht, aber die Berechtigung greift davor), Einstellungen nein
	if status, _ := operator.do("POST", "/api/servers/"+created.ID+"/power", map[string]string{"action": "start"}); status == 403 {
		t.Error("Operator darf Server starten")
	}
	operator.expect(403, "PATCH", "/api/servers/"+created.ID, map[string]any{"memoryMB": 512})
	operator.expect(403, "PUT", "/api/projects/"+project.ID+"/members/"+ids["fremd"], map[string]string{"role": "viewer"})

	// Rolle, die man nicht hat, kann man nicht vergeben; autoconfig ist nie direkt erreichbar
	admin.expect(404, "PUT", "/api/servers/"+created.ID+"/autoconfig", map[string]any{})
	operator.expect(403, "DELETE", "/api/projects/"+project.ID, nil)

	// Nur Administratoren verwalten Benutzer und Nodes
	viewer.expect(403, "POST", "/api/users", map[string]any{"username": "neu-neu", "password": "ein-gutes-passwort"})
	viewer.expect(403, "POST", "/api/nodes", map[string]any{"name": "x", "url": "http://x", "token": "y"})
}

func TestNetworkLifecycle(t *testing.T) {
	_, web, fake, nodeID := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	var project ProjectDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Netz", "kind": "network"}), &project)
	add := func(request serverRequest) ServerDTO {
		var dto ServerDTO
		_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/servers", request), &dto)
		return dto
	}
	proxy := add(serverRequest{Name: "Proxy", NodeID: nodeID, Software: "velocity", Version: "3.4.0"})
	lobby := add(serverRequest{Name: "Lobby", NodeID: nodeID, Software: "paper", Version: "1.21.4", EULA: true})
	survival := add(serverRequest{Name: "Survival", NodeID: nodeID, Software: "paper", Version: "1.21.4", EULA: true, Voicechat: true})

	if proxy.Role != RoleProxy || lobby.Role != RoleBackend {
		t.Fatalf("Rollen: %s %s", proxy.Role, lobby.Role)
	}
	// Backends sind nicht nach außen veröffentlicht, außer dem Voicechat-Port
	fake.mu.Lock()
	if view := fake.servers[lobby.ID]; !view.NoPublish || len(view.Ports) != 0 {
		t.Errorf("Lobby sollte nicht veröffentlicht sein: %+v", view.Spec)
	}
	if view := fake.servers[survival.ID]; len(view.Ports) != 1 || view.Ports[0].Protocol != "udp" || view.Ports[0].Container != 24454 {
		t.Errorf("Survival Ports: %+v", view.Ports)
	}
	fake.mu.Unlock()

	// ein zweiter Proxy, Vanilla und Velocity als Backend gehen nicht
	admin.expect(409, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{Name: "Proxy2", NodeID: nodeID, Software: "velocity", Version: "3.4.0"})
	admin.expect(400, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{Name: "Plain", NodeID: nodeID, Software: "vanilla", Version: "1.21.4", EULA: true})

	// Reihenfolge setzen: Survival zuerst, Lobby aus dem Try, Forced Host für Survival
	var result SyncResult
	_ = json.Unmarshal(admin.expect(200, "PUT", "/api/projects/"+project.ID+"/network", map[string]any{"servers": []networkEntry{
		{ID: survival.ID, InTry: true, ForcedHost: "Survival.Example.org", Voicechat: true},
		{ID: lobby.ID, InTry: false},
	}}), &result)
	if len(result.Failed) != 0 {
		t.Fatalf("Abgleich: %+v", result)
	}

	fake.mu.Lock()
	config := fake.autoconfigs[proxy.ID]
	fake.mu.Unlock()
	velocity := editFor(t, config, "velocity.toml")
	table := setValue(t, velocity, "servers").(map[string]any)
	try, _ := table["try"].([]any)
	if len(try) != 1 || try[0] != "survival" {
		t.Errorf("try = %v", table["try"])
	}
	forced := setValue(t, velocity, "forced-hosts").(map[string]any)
	if _, ok := forced["survival.example.org"]; !ok {
		t.Errorf("forced-hosts = %v", forced)
	}
	fake.mu.Lock()
	survivalConfig, ok := fake.autoconfigs[survival.ID]
	fake.mu.Unlock()
	if !ok {
		t.Fatal("Survival hat keine Autoconfig bekommen")
	}
	voice := editFor(t, survivalConfig, "plugins/voicechat/voicechat-server.properties")
	if got := setValue(t, voice, "voice_host"); !strings.HasPrefix(got.(string), "mc.example.org:") {
		t.Errorf("voice_host = %v", got)
	}

	// Server löschen entfernt ihn auch auf dem Node
	admin.expect(204, "DELETE", "/api/servers/"+lobby.ID, nil)
	fake.mu.Lock()
	if len(fake.deleted) != 1 || fake.deleted[0] != lobby.ID {
		t.Errorf("gelöscht auf dem Node: %v", fake.deleted)
	}
	fake.mu.Unlock()

	admin.expect(204, "DELETE", "/api/projects/"+project.ID, nil)
	fake.mu.Lock()
	if len(fake.servers) != 0 {
		t.Errorf("auf dem Node bleiben Server: %v", fake.servers)
	}
	fake.mu.Unlock()
}
