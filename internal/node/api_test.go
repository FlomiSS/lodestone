package node

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lodestone-mc/lodestone/internal/software"
)

type testAPI struct {
	t       *testing.T
	url     string
	token   string
	manager *Manager
	rt      *fakeRuntime
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	cfg := testConfig(t)
	rt := newFakeRuntime()
	manager := newTestManager(t, cfg, rt)
	api := NewAPI(cfg, manager, rt, fakeSoftware{}, slog.New(slog.NewTextHandler(io.Discard, nil)), "test")
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	return &testAPI{t: t, url: server.URL, token: cfg.Token, manager: manager, rt: rt}
}

func (a *testAPI) do(method, path string, body any, headers ...string) (int, []byte) {
	a.t.Helper()
	var reader io.Reader
	switch value := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(value)
	case []byte:
		reader = bytes.NewReader(value)
	default:
		data, _ := json.Marshal(value)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, a.url+path, reader)
	request.Header.Set("Authorization", "Bearer "+a.token)
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		a.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}

func (a *testAPI) create(name string) string {
	a.t.Helper()
	status, body := a.do("POST", "/api/servers", map[string]any{"name": name, "software": "paper", "version": "1.21.8", "eula": true})
	if status != http.StatusCreated {
		a.t.Fatalf("anlegen: %d %s", status, body)
	}
	var view View
	_ = json.Unmarshal(body, &view)
	server, _ := a.manager.Get(view.ID)
	waitFor(a.t, "Installation", func() bool { return server.View().Installed })
	waitState(a.t, server, StateOffline)
	return view.ID
}

func TestAPIRequiresTheToken(t *testing.T) {
	a := newTestAPI(t)
	for _, auth := range []string{"", "Bearer falsch", "Bearer " + strings.Repeat("t", 31)} {
		request, _ := http.NewRequest("GET", a.url+"/api/servers", nil)
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("mit %q erhalten %d, erwartet 401", auth, response.StatusCode)
		}
	}
	// Der Websocket-Token in der Adresse gilt nur beim Websocket, nicht für andere Routen
	response, _ := http.Get(a.url + "/api/servers?token=" + a.token)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("das Token in der Adresse darf nur beim Websocket gelten, erhalten %d", response.StatusCode)
	}
	response.Body.Close()
	if status, _ := a.do("GET", "/healthz", nil); status != http.StatusOK {
		t.Fatalf("healthz: %d", status)
	}
	unauthenticated, _ := http.Get(a.url + "/healthz")
	if unauthenticated.StatusCode != http.StatusOK {
		t.Fatalf("healthz ohne Anmeldung: %d", unauthenticated.StatusCode)
	}
	unauthenticated.Body.Close()
}

func TestAPIServerLifecycle(t *testing.T) {
	a := newTestAPI(t)
	id := a.create("API-Server")

	status, body := a.do("GET", "/api/servers", nil)
	var list []View
	_ = json.Unmarshal(body, &list)
	if status != 200 || len(list) != 1 || list[0].ID != id || list[0].State != StateOffline {
		t.Fatalf("Liste: %d %s", status, body)
	}

	if status, body := a.do("POST", "/api/servers/"+id+"/power", map[string]string{"action": "start"}); status != http.StatusAccepted {
		t.Fatalf("start: %d %s", status, body)
	}
	server, _ := a.manager.Get(id)
	waitState(t, server, StateRunning)
	if status, _ := a.do("POST", "/api/servers/"+id+"/power", map[string]string{"action": "start"}); status != http.StatusConflict {
		t.Fatalf("doppelter Start muss 409 liefern, erhalten %d", status)
	}
	if status, _ := a.do("POST", "/api/servers/"+id+"/command", map[string]string{"command": "say hi"}); status != http.StatusNoContent {
		t.Fatalf("command: %d", status)
	}
	waitFor(t, "Befehl in den Logs", func() bool {
		_, body := a.do("GET", "/api/servers/"+id+"/logs?lines=50", nil)
		return strings.Contains(string(body), "Befehl: say hi")
	})
	if status, _ := a.do("PATCH", "/api/servers/"+id, map[string]any{"memoryMB": 4096}); status != http.StatusConflict {
		t.Fatalf("ein laufender Server lässt sich nicht ändern, erhalten %d", status)
	}
	if status, _ := a.do("POST", "/api/servers/"+id+"/power", map[string]string{"action": "stop"}); status != http.StatusAccepted {
		t.Fatalf("stop: %d", status)
	}
	waitState(t, server, StateOffline)
	if status, _ := a.do("PATCH", "/api/servers/"+id, map[string]any{"memoryMB": 4096}); status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}
	if status, _ := a.do("DELETE", "/api/servers/"+id, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := a.do("GET", "/api/servers/"+id, nil); status != http.StatusNotFound {
		t.Fatalf("gelöscht, erwartet 404, erhalten %d", status)
	}
}

func TestAPIRejectsBadInput(t *testing.T) {
	a := newTestAPI(t)
	cases := []struct {
		name string
		body any
		want int
	}{
		{"unbekannte Software", map[string]any{"software": "nope", "version": "1"}, 400},
		{"Pfad-Trick in der ID", map[string]any{"id": "../x", "software": "paper", "version": "1"}, 400},
		{"unbekanntes Feld", map[string]any{"software": "paper", "version": "1", "root": true}, 400},
		{"kaputtes JSON", "{", 400},
	}
	for _, c := range cases {
		if status, body := a.do("POST", "/api/servers", c.body); status != c.want {
			t.Errorf("%s: %d %s", c.name, status, body)
		}
	}
	if status, _ := a.do("POST", "/api/servers/unbekannt/power", map[string]string{"action": "start"}); status != 404 {
		t.Fatalf("unbekannter Server: %d", status)
	}
	id := a.create("Eingaben")
	if status, _ := a.do("POST", "/api/servers/"+id+"/power", map[string]string{"action": "format-c"}); status != 400 {
		t.Fatalf("unbekannte Aktion: %d", status)
	}
	if status, _ := a.do("POST", "/api/servers", map[string]any{"id": id, "software": "paper", "version": "1.21.8"}); status != 409 {
		t.Fatalf("doppelte ID muss 409 liefern: %d", status)
	}
}

func TestAPIFiles(t *testing.T) {
	a := newTestAPI(t)
	id := a.create("Dateien")
	base := "/api/servers/" + id + "/files"

	if status, body := a.do("PUT", base+"/content?path="+url.QueryEscape("plugins/x/config.yml")+"&create=1", "a: 1\n"); status != http.StatusConflict && status != http.StatusNotFound && status != 500 {
		t.Logf("Schreiben in einen fehlenden Ordner: %d %s", status, body)
	}
	if status, body := a.do("POST", base+"/mkdir", map[string]string{"path": "plugins/x"}); status != 204 {
		t.Fatalf("mkdir: %d %s", status, body)
	}
	if status, body := a.do("PUT", base+"/content?path="+url.QueryEscape("plugins/x/config.yml")+"&create=1", "a: 1\n"); status != 204 {
		t.Fatalf("anlegen: %d %s", status, body)
	}
	if status, body := a.do("GET", base+"/content?path=plugins/x/config.yml", nil); status != 200 || string(body) != "a: 1\n" {
		t.Fatalf("lesen: %d %q", status, body)
	}
	if status, _ := a.do("PUT", base+"/content?path=plugins/x/config.yml&create=1", "b"); status != 409 {
		t.Fatalf("create darf nicht überschreiben: %d", status)
	}
	if status, _ := a.do("PUT", base+"/content?path=plugins/x/config.yml", "b: 2\n"); status != 204 {
		t.Fatalf("überschreiben: %d", status)
	}

	status, body := a.do("GET", base+"?path=plugins", nil)
	var entries []Entry
	_ = json.Unmarshal(body, &entries)
	if status != 200 || len(entries) != 1 || entries[0].Name != "x" || !entries[0].Dir {
		t.Fatalf("Liste: %d %s", status, body)
	}

	// Ausbruchsversuche
	for _, evil := range []string{"../../../etc/passwd", "..%2f..%2fsecret", "/etc/passwd", "plugins/../../x"} {
		if status, body := a.do("GET", base+"/content?path="+evil, nil); status == 200 {
			t.Fatalf("%q wurde ausgeliefert: %s", evil, body)
		}
	}
	if status, _ := a.do("DELETE", base+"?path=.&recursive=1", nil); status == 204 {
		t.Fatal("der Serverordner selbst darf nicht gelöscht werden")
	}

	// Upload und Download
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, _ := writer.CreateFormFile("file", `..\..\evil.jar`)
	_, _ = part.Write([]byte("PK-binär"))
	writer.Close()
	if status, body := a.do("POST", base+"/upload?path=plugins", buffer.Bytes(), "Content-Type", writer.FormDataContentType()); status != 201 {
		t.Fatalf("upload: %d %s", status, body)
	}
	if status, body := a.do("GET", base+"/download?path=plugins/evil.jar", nil); status != 200 || string(body) != "PK-binär" {
		t.Fatalf("der Dateiname muss auf den Namen gekürzt landen: %d %q", status, body)
	}
	if status, _ := a.do("POST", base+"/rename", map[string]string{"from": "plugins/evil.jar", "to": "plugins/ok.jar"}); status != 204 {
		t.Fatalf("rename: %d", status)
	}
	if status, _ := a.do("DELETE", base+"?path=plugins", nil); status != 500 && status != 409 {
		t.Logf("ein nicht leerer Ordner ohne recursive: %d", status)
	}
	if status, _ := a.do("DELETE", base+"?path=plugins&recursive=1", nil); status != 204 {
		t.Fatalf("recursive löschen: %d", status)
	}
}

func TestAPIWebsocketStreamsConsoleAndAcceptsCommands(t *testing.T) {
	a := newTestAPI(t)
	id := a.create("Konsole")
	server, _ := a.manager.Get(id)
	_ = server.Start()
	waitState(t, server, StateRunning)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(a.url, "http") + "/api/servers/" + id + "/ws?token=" + a.token
	conn, _, err := websocket.Dial(ctx, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	var first hello
	if err := wsjson.Read(ctx, conn, &first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "hello" || first.State != StateRunning || len(first.Lines) == 0 {
		t.Fatalf("hello: %+v", first)
	}
	if err := wsjson.Write(ctx, conn, clientMessage{Type: "command", Data: "say ws"}); err != nil {
		t.Fatal(err)
	}
	for {
		var event Event
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			t.Fatalf("Warten auf das Echo: %v", err)
		}
		if event.Type == "line" && strings.Contains(event.Line.Text, "Befehl: say ws") {
			break
		}
	}
	_ = server.Stop()
	for {
		var event Event
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			t.Fatalf("Warten auf den Zustand offline: %v", err)
		}
		if event.Type == "state" && event.State == StateOffline {
			break
		}
	}

	// ohne Token kein Websocket
	if _, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(a.url, "http")+"/api/servers/"+id+"/ws", nil); err == nil || response == nil || response.StatusCode != 401 {
		t.Fatalf("Websocket ohne Token muss mit 401 abgelehnt werden")
	}
}

func TestAPISoftwareAndSystem(t *testing.T) {
	a := newTestAPI(t)
	status, body := a.do("GET", "/api/software", nil)
	if status != 200 || !strings.Contains(string(body), `"velocity"`) || !strings.Contains(string(body), `"fabric"`) {
		t.Fatalf("Software: %d %s", status, body)
	}
	if status, body := a.do("GET", "/api/software/paper/versions", nil); status != 200 || !strings.Contains(string(body), "1.21.8") {
		t.Fatalf("Versionen: %d %s", status, body)
	}
	if status, _ := a.do("GET", "/api/software/unbekannt/versions", nil); status != 404 {
		t.Fatalf("unbekannte Software: %d", status)
	}
	if status, body := a.do("GET", "/api/system", nil); status != 200 || !strings.Contains(string(body), `"dockerVersion":"fake"`) {
		t.Fatalf("System: %d %s", status, body)
	}
}

func TestReadMeminfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meminfo")
	if err := os.WriteFile(path, []byte("MemTotal:       16000000 kB\nMemFree:         100000 kB\nMemAvailable:    8000000 kB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readMeminfo(path, "MemAvailable"); got != 8000000*1024 {
		t.Errorf("MemAvailable = %d", got)
	}
	if got := readMeminfo(path, "Gibtsnicht"); got != 0 {
		t.Errorf("unbekannter Schlüssel = %d", got)
	}
	if got := readMeminfo(filepath.Join(t.TempDir(), "fehlt"), "MemAvailable"); got != 0 {
		t.Errorf("fehlende Datei = %d", got)
	}
}

func TestSystemUpdateEndpoints(t *testing.T) {
	a := newTestAPI(t)
	status, body := a.do("POST", "/api/system/update", map[string]string{"version": "0.3.0"})
	if status != 200 || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("system/update: %d %s", status, body)
	}
	status, body = a.do("POST", "/api/system/update-panel", map[string]string{"version": "v0.3.0-rc.1"})
	if status != 200 || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("system/update-panel: %d %s", status, body)
	}
	status, body = a.do("POST", "/api/system/update", map[string]string{"version": "dev-1a2b3c4"})
	if status != 200 || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("system/update auf einen Dev-Build: %d %s", status, body)
	}
	// Alles, was kein Release-Tag oder Dev-Build ist, wird abgelehnt (die Version landet im Namen eines Images).
	for _, version := range []string{"", "latest", "main", "0.3", "0.3.0/../x", "0.3.0 --privileged", "ghcr.io/evil/img:1.0.0",
		"dev", "dev-", "dev-1a2b3c", "dev-1A2B3C4", "dev-1a2b3c4/../x", "dev-1a2b3c4 --privileged", "dev-" + strings.Repeat("a", 41)} {
		if status, _ := a.do("POST", "/api/system/update", map[string]string{"version": version}); status != 400 {
			t.Errorf("Version %q: %d, erwartet 400", version, status)
		}
	}
	// Ein fremdes Image lässt sich nicht mehr mitschicken.
	if status, _ := a.do("POST", "/api/system/update", map[string]string{"version": "0.3.0", "image": "evil:1"}); status != 400 {
		t.Errorf("unbekanntes Feld image: %d, erwartet 400", status)
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", a.url+"/api/system/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("erwarte 401 ohne Token, got %d", resp.StatusCode)
	}
}

func TestAPIPackRoutes(t *testing.T) {
	tempDir := t.TempDir()
	software.InitPackManager(tempDir)
	t.Cleanup(func() {
		software.InitPackManager("")
	})

	a := newTestAPI(t)

	// 1. Auth check
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequestWithContext(context.Background(), method, a.url+"/api/packs/custom-node-test", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s ohne Token: erwartet 401, erhalten %d", method, resp.StatusCode)
		}
	}

	// 2. Ungültiges JSON bei PUT
	status, _ := a.do(http.MethodPut, "/api/packs/test-pack", "{ungueltiges json")
	if status != http.StatusBadRequest {
		t.Fatalf("PUT ungültiges JSON: erwartet 400, erhalten %d", status)
	}

	// 3. Body größer als software.MaxPackJSONSize (8 MB)
	oversized := append([]byte(`{"meta":{"name":"`), bytes.Repeat([]byte("a"), software.MaxPackJSONSize+100)...)
	oversized = append(oversized, []byte(`"}}`)...)
	status, _ = a.do(http.MethodPut, "/api/packs/oversized-pack", oversized)
	if status != http.StatusBadRequest {
		t.Fatalf("PUT oversized body: erwartet 400, erhalten %d", status)
	}

	// 3b. Pack mit Skript größer als MaxPackScriptSize (1 MB) wird abgewiesen
	oversizedScriptPack := software.PositionPack{
		Meta: software.PackMeta{
			ID:         "oversized-script",
			Name:       "Oversized Script",
			Kind:       software.KindServer,
			Extensions: software.ExtPlugins,
		},
		Script: strings.Repeat("# script\n", 150000), // ca. 1.35 MB > 1 MB
	}
	status, _ = a.do(http.MethodPut, "/api/packs/oversized-script", oversizedScriptPack)
	if status != http.StatusBadRequest {
		t.Fatalf("PUT oversized script (>1MB): erwartet 400, erhalten %d", status)
	}

	// 3c. Gültiges Pack größer als maxJSONBody (1 MB), aber mit zulässigen Eintragsgrößen (Script < 1 MB, Autoconfig < 2 MB)
	scriptContent := strings.Repeat("# line\n", 100000) // ~700 KB (< 1 MB)
	largeEdits := make([]software.PackEdit, 500)
	for i := range largeEdits {
		largeEdits[i] = software.PackEdit{
			File:   "config.yml",
			Format: "yaml",
			Set:    []software.PackSetOp{{Path: []string{strings.Repeat("k", 20)}, Value: strings.Repeat("x", 800)}},
		}
	}
	largePack := software.PositionPack{
		Meta: software.PackMeta{
			ID:         "large-pack",
			Name:       "Large Pack",
			Kind:       software.KindServer,
			Extensions: software.ExtPlugins,
		},
		Script: scriptContent,
		Autoconfig: software.PackAutoconfig{
			Backend: &software.PackAutoconfigTarget{
				Edits: largeEdits,
			},
		},
	}
	status, _ = a.do(http.MethodPut, "/api/packs/large-pack", largePack)
	if status != http.StatusOK {
		t.Fatalf("PUT large pack (>1MB): erwartet 200, erhalten %d", status)
	}

	// Sicherstellen, dass das Pack beim Neuladen/Exportieren vollständig und unversehrt bleibt (Reload-Contract)
	exported, err := software.GetPackManager().Export("large-pack")
	if err != nil {
		t.Fatalf("Export large-pack: %v", err)
	}
	reloaded, err := software.ReadZipPack(bytes.NewReader(exported), int64(len(exported)))
	if err != nil {
		t.Fatalf("ReadZipPack large-pack: %v", err)
	}
	if reloaded.Script != scriptContent {
		t.Fatalf("Script wurde beim Neuladen abgeschnitten oder verändert: len %d vs %d", len(reloaded.Script), len(scriptContent))
	}
	if reloaded.Autoconfig.Backend == nil || len(reloaded.Autoconfig.Backend.Edits) != len(largeEdits) {
		t.Fatalf("Autoconfig wurde beim Neuladen verändert")
	}
	_, _ = a.do(http.MethodDelete, "/api/packs/large-pack", nil)

	// 4. Ungültige Metadaten
	status, _ = a.do(http.MethodPut, "/api/packs/bad-pack", software.PositionPack{
		Meta: software.PackMeta{ID: "bad-pack", Name: ""},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("PUT ungültige Metadaten: erwartet 400, erhalten %d", status)
	}

	// 5. 404 bei DELETE von unbekanntem Pack
	status, _ = a.do(http.MethodDelete, "/api/packs/unbekanntes-pack-xyz", nil)
	if status != http.StatusNotFound {
		t.Fatalf("DELETE unbekanntes Pack: erwartet 404, erhalten %d", status)
	}

	// 6. 400 bei DELETE von nicht-überschriebenem Standard-Pack
	status, _ = a.do(http.MethodDelete, "/api/packs/paper", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("DELETE Standard-Pack: erwartet 400, erhalten %d", status)
	}

	// 7. Gültiges PUT für Custom Pack
	customPack := software.PositionPack{
		Meta: software.PackMeta{
			ID:         "custom-node-pack",
			Name:       "Custom Node Pack",
			Kind:       software.KindServer,
			Extensions: software.ExtPlugins,
		},
	}
	status, body := a.do(http.MethodPut, "/api/packs/custom-node-pack", customPack)
	if status != http.StatusOK {
		t.Fatalf("PUT custom pack: erwartet 200, erhalten %d: %s", status, body)
	}
	var summary software.PackSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary.ID != "custom-node-pack" || !summary.Custom || summary.Overridden {
		t.Fatalf("unerwartete Summary für Custom Pack: %+v", summary)
	}

	// 8. GET /api/packs enthält das neue Custom Pack
	status, body = a.do(http.MethodGet, "/api/packs", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/packs: erwartet 200, erhalten %d", status)
	}
	var summaries []software.PackSummary
	if err := json.Unmarshal(body, &summaries); err != nil {
		t.Fatalf("unmarshal summaries: %v", err)
	}
	found := false
	for _, s := range summaries {
		if s.ID == "custom-node-pack" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom-node-pack nicht in GET /api/packs gefunden")
	}

	// 9. Standard-Pack überschreiben (PUT /api/packs/paper)
	paperPack := software.PositionPack{
		Meta: software.PackMeta{
			ID:          "paper",
			Name:        "Modifiziertes Paper",
			Kind:        software.KindServer,
			Extensions:  software.ExtPlugins,
			Description: "Überschriebenes Standard-Pack",
		},
	}
	status, body = a.do(http.MethodPut, "/api/packs/paper", paperPack)
	if status != http.StatusOK {
		t.Fatalf("PUT override paper: erwartet 200, erhalten %d: %s", status, body)
	}
	var paperSummary software.PackSummary
	if err := json.Unmarshal(body, &paperSummary); err != nil {
		t.Fatalf("unmarshal paper summary: %v", err)
	}
	if paperSummary.Custom || !paperSummary.Overridden {
		t.Fatalf("überschriebenes Paper sollte Custom=false, Overridden=true sein: %+v", paperSummary)
	}

	// 10. DELETE von überschriebenem Standard-Pack setzt zurück (204)
	status, _ = a.do(http.MethodDelete, "/api/packs/paper", nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE override paper: erwartet 204, erhalten %d", status)
	}
	// Nach Reset schlägt DELETE wieder mit 400 fehl
	status, _ = a.do(http.MethodDelete, "/api/packs/paper", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("DELETE reset paper: erwartet 400, erhalten %d", status)
	}

	// 11. DELETE von Custom Pack (204)
	status, _ = a.do(http.MethodDelete, "/api/packs/custom-node-pack", nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE custom pack: erwartet 204, erhalten %d", status)
	}
	// Erneutes DELETE liefert 404
	status, _ = a.do(http.MethodDelete, "/api/packs/custom-node-pack", nil)
	if status != http.StatusNotFound {
		t.Fatalf("DELETE custom pack erneut: erwartet 404, erhalten %d", status)
	}
}

func TestContainerNameHelpers(t *testing.T) {
	if !hasContainerNamePrefix([]string{"/lodestone-node-updater-1f2e"}, updaterContainerPrefix) {
		t.Error("Hilfscontainer wurde nicht erkannt")
	}
	if hasContainerNamePrefix([]string{"/lodestone-node-1"}, updaterContainerPrefix) {
		t.Error("der Daemon selbst ist kein Hilfscontainer")
	}
	if !hasContainerName([]string{"/lodestone-panel-1"}, "panel") || hasContainerName([]string{"/panel-extra"}, "panel") {
		t.Error("hasContainerName unterscheidet Compose-Namen falsch")
	}
}

func TestDockerImageHelpers(t *testing.T) {
	for ref, want := range map[string]string{
		"ghcr.io/henny263462/lodestone-panel:latest": "ghcr.io/henny263462/lodestone-panel",
		"ghcr.io/x/lodestone-node@sha256:abc":        "ghcr.io/x/lodestone-node",
		"registry:5000/lodestone-node:v1.0.0":        "registry:5000/lodestone-node",
		"lodestone-node":                             "lodestone-node",
	} {
		if got := imageRepo(ref); got != want {
			t.Errorf("imageRepo(%q) = %q, erwartet %q", ref, got, want)
		}
	}
	if got := releaseImage("ghcr.io/x/lodestone-node:latest", "0.3.0"); got != "ghcr.io/x/lodestone-node:v0.3.0" {
		t.Errorf("releaseImage = %q", got)
	}
	if got := releaseImage("ghcr.io/x/lodestone-node:latest", "v0.3.0"); got != "ghcr.io/x/lodestone-node:v0.3.0" {
		t.Errorf("releaseImage mit v = %q", got)
	}
	if got := releaseImage("ghcr.io/x/lodestone-node:v0.3.0", "dev-1a2b3c4"); got != "ghcr.io/x/lodestone-node:dev-1a2b3c4" {
		t.Errorf("releaseImage für Dev-Build = %q", got)
	}
	// Fremde Container, die zufällig „panel“ oder „node“ heißen (zum Beispiel Pelican), sind kein Ziel.
	for ref, want := range map[string]bool{
		"ghcr.io/pelican-dev/panel:latest":     false,
		"node:22-alpine":                       false,
		"ghcr.io/x/lodestone-panel:latest":     true,
		"lodestone-panel":                      true,
		"ghcr.io/x/not-lodestone-panel:latest": false,
	} {
		if got := isLodestoneImage(ref, panelImageName); got != want {
			t.Errorf("isLodestoneImage(%q) = %v, erwartet %v", ref, got, want)
		}
	}
}
