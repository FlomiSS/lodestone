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
