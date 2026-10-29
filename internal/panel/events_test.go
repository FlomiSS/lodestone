package panel

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
)

func init() { startDelay = 5 * time.Millisecond }

// serverProject legt ein Server-Projekt mit einem Server an.
func serverProject(t *testing.T) (*Panel, *client, *fakeNode, string, ServerDTO, string) {
	p, web, fake, nodeID := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	var project ProjectDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Mein Server", "kind": "server"}), &project)
	var server ServerDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{Name: "Main", NodeID: nodeID, Software: "paper", Version: "1.21.4", EULA: true}), &server)
	return p, admin, fake, nodeID, server, project.ID
}

func (f *fakeNode) setExit(id string, exit *node.ExitInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	view := f.servers[id]
	view.LastExit = exit
	f.servers[id] = view
}

func (f *fakeNode) starts(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, entry := range f.powered {
		if entry == "start:"+id {
			count++
		}
	}
	return count
}

func eventKindsOf(t *testing.T, p *Panel, projectID string) []string {
	t.Helper()
	events, err := p.store.ListEvents(projectID, 100)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- { // älteste zuerst
		kinds = append(kinds, events[i].Kind)
	}
	return kinds
}

func refresh(p *Panel, nodeID string) { p.invalidate(nodeID) }

func TestCrashIsReportedAndAutoRestarted(t *testing.T) {
	p, admin, fake, nodeID, server, projectID := serverProject(t)
	ctx := context.Background()
	admin.expect(200, "PUT", "/api/servers/"+server.ID+"/auto-restart", map[string]bool{"enabled": true})

	p.watchOnce(ctx) // erste Beobachtung: nichts melden
	if kinds := eventKindsOf(t, p, projectID); len(kinds) != 0 {
		t.Fatalf("beim ersten Blick darf nichts gemeldet werden: %v", kinds)
	}

	at := int64(1000)
	exit := func(code int, requested bool) {
		at += 1000
		fake.setExit(server.ID, &node.ExitInfo{Code: code, Requested: requested, At: at})
		refresh(p, nodeID)
		p.watchOnce(ctx)
	}

	// Ein verlangter Stopp und ein sauberes Ende sind keine Abstürze
	exit(137, true)
	exit(0, false)
	if kinds := eventKindsOf(t, p, projectID); len(kinds) != 0 {
		t.Fatalf("kein Absturz erwartet: %v", kinds)
	}

	// Ein Absturz wird gemeldet und der Server neu gestartet
	exit(1, false)
	waitUntil(t, "Neustart nach dem Absturz", func() bool { return fake.starts(server.ID) == 1 })
	waitUntil(t, "Ereignis Neustart", func() bool { return len(eventKindsOf(t, p, projectID)) == 2 })
	if kinds := eventKindsOf(t, p, projectID); kinds[0] != "crash" || kinds[1] != "restart" {
		t.Fatalf("Ereignisse: %v", kinds)
	}

	// Nach dem dritten Versuch gibt der Auto-Restart auf, statt endlos weiterzumachen
	exit(1, false)
	exit(1, false)
	waitUntil(t, "drei Neustarts", func() bool { return fake.starts(server.ID) == maxRestarts })
	exit(1, false)
	time.Sleep(80 * time.Millisecond)
	if got := fake.starts(server.ID); got != maxRestarts {
		t.Fatalf("es darf nicht öfter als %d-mal neu gestartet werden: %d", maxRestarts, got)
	}
	kinds := eventKindsOf(t, p, projectID)
	if kinds[len(kinds)-1] != "crashloop" {
		t.Fatalf("zuletzt muss die Absturzschleife gemeldet sein: %v", kinds)
	}
}

func TestCrashWithoutAutoRestartOnlyReports(t *testing.T) {
	p, _, fake, nodeID, server, projectID := serverProject(t)
	ctx := context.Background()
	p.watchOnce(ctx)
	fake.setExit(server.ID, &node.ExitInfo{Code: 1, At: 5000})
	refresh(p, nodeID)
	p.watchOnce(ctx)
	time.Sleep(60 * time.Millisecond)
	if kinds := eventKindsOf(t, p, projectID); len(kinds) != 1 || kinds[0] != "crash" {
		t.Fatalf("Ereignisse: %v", kinds)
	}
	if fake.starts(server.ID) != 0 {
		t.Fatal("ohne Auto-Restart darf nichts gestartet werden")
	}
}

func TestNodeOutageIsReported(t *testing.T) {
	p, _, _, nodeID, _, projectID := serverProject(t)
	ctx := context.Background()
	p.watchOnce(ctx)
	n, _ := p.store.NodeByID(nodeID)
	good := n.URL
	n.URL = "http://127.0.0.1:1" // niemand hört zu
	_ = p.store.UpdateNode(n)
	refresh(p, nodeID)
	p.watchOnce(ctx)
	n.URL = good
	_ = p.store.UpdateNode(n)
	refresh(p, nodeID)
	p.watchOnce(ctx)
	if kinds := eventKindsOf(t, p, projectID); len(kinds) != 2 || kinds[0] != "node_offline" || kinds[1] != "node_online" {
		t.Fatalf("Ereignisse: %v", kinds)
	}
}

type received struct {
	mu     sync.Mutex
	bodies []string
}

func (r *received) handler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(data))
		r.mu.Unlock()
		w.WriteHeader(status)
	})
}

func (r *received) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bodies)
}

func TestWebhooksDeliverAndAreValidated(t *testing.T) {
	p, admin, fake, nodeID, server, projectID := serverProject(t)
	receiver := &received{}
	target := httptest.NewServer(receiver.handler(204))
	defer target.Close()

	// Mit Standardeinstellungen ist das interne Netz gesperrt – auch http und Weiterleitungen
	admin.expect(400, "POST", "/api/projects/"+projectID+"/webhooks", map[string]any{"name": "intern", "url": target.URL, "kind": "generic"})
	admin.expect(400, "POST", "/api/projects/"+projectID+"/webhooks", map[string]any{"name": "x", "url": "https://example.org/x", "kind": "slack"})
	admin.expect(400, "POST", "/api/projects/"+projectID+"/webhooks", map[string]any{"name": "x", "url": "https://example.org/x", "kind": "generic", "events": []string{"gibtsnicht"}})

	// In dieser Umgebung erlauben wir interne Ziele (Testserver auf 127.0.0.1)
	p.cfg.AllowInternalWebhooks = true
	p.hookClient = newHookClient(true)
	var generic, discord Webhook
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+projectID+"/webhooks", map[string]any{"name": "Allgemein", "url": target.URL + "/generic", "kind": "generic", "events": []string{"crash"}}), &generic)
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+projectID+"/webhooks", map[string]any{"name": "Discord", "url": target.URL + "/discord", "kind": "discord"}), &discord)
	if len(discord.Events) != len(defaultHookEvents) {
		t.Errorf("ohne Auswahl gelten die Standard-Ereignisse: %v", discord.Events)
	}

	ctx := context.Background()
	p.watchOnce(ctx)
	fake.setExit(server.ID, &node.ExitInfo{Code: 1, At: 9000})
	refresh(p, nodeID)
	p.watchOnce(ctx)
	waitUntil(t, "beide Webhooks", func() bool { return receiver.count() == 2 })

	var sawGeneric, sawDiscord bool
	receiver.mu.Lock()
	for _, body := range receiver.bodies {
		var parsed map[string]any
		_ = json.Unmarshal([]byte(body), &parsed)
		if parsed["event"] == "crash" && parsed["project"] == "Mein Server" && parsed["server"] == "Main" {
			sawGeneric = true
		}
		if embeds, ok := parsed["embeds"].([]any); ok && len(embeds) == 1 && strings.Contains(embeds[0].(map[string]any)["title"].(string), "Absturz") {
			sawDiscord = true
		}
	}
	receiver.mu.Unlock()
	if !sawGeneric || !sawDiscord {
		t.Errorf("Inhalte: %v", receiver.bodies)
	}

	// Test-Knopf und Statusanzeige
	var result struct {
		OK     bool
		Status string
	}
	_ = json.Unmarshal(admin.expect(200, "POST", "/api/webhooks/"+generic.ID+"/test", nil), &result)
	if !result.OK || !strings.HasPrefix(result.Status, "OK") {
		t.Errorf("Test: %+v", result)
	}
	hooks, _ := p.store.ListWebhooks(projectID)
	for _, hook := range hooks {
		if !strings.HasPrefix(hook.LastStatus, "OK") {
			t.Errorf("Status von %s: %q", hook.Name, hook.LastStatus)
		}
	}

	// Ein Webhook, der nicht antwortet, wird als Fehler vermerkt
	failing := httptest.NewServer(receiver.handler(500))
	defer failing.Close()
	broken, _ := p.store.CreateWebhook(Webhook{ProjectID: projectID, Name: "kaputt", URL: failing.URL, Kind: "generic", Events: []string{"crash"}, Enabled: true})
	if status := p.deliver(broken, Event{Kind: "crash", Level: "error", Message: "x", Time: time.Now().UnixMilli()}, "p", ""); !strings.HasPrefix(status, "Fehler") {
		t.Errorf("Antwort 500: %q", status)
	}
}

func TestWebhookClientBlocksInternalTargets(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("das interne Ziel wurde erreicht") }))
	defer target.Close()
	p := &Panel{hookClient: newHookClient(false)}
	status := p.deliver(Webhook{Kind: "generic", URL: target.URL}, Event{Kind: "crash", Time: time.Now().UnixMilli()}, "p", "")
	if !strings.HasPrefix(status, "Fehler") || !strings.Contains(status, "gesperrt") {
		t.Errorf("Status: %q", status)
	}
	// Eine Weiterleitung auf ein internes Ziel wird nicht verfolgt
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirector.Close()
	p.hookClient = newHookClient(true) // damit der Weiterleiter selbst erreichbar ist
	if status := p.deliver(Webhook{Kind: "generic", URL: redirector.URL}, Event{Kind: "crash", Time: time.Now().UnixMilli()}, "p", ""); !strings.HasPrefix(status, "Fehler") {
		t.Errorf("Weiterleitung: %q", status)
	}
}

func TestInternalAddressDetection(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "192.168.178.43": true, "172.16.0.1": true, "169.254.169.254": true, "100.64.0.1": true,
		"0.0.0.0": true, "::1": true, "fe80::1": true, "fd00::1": true, "::ffff:10.0.0.1": true,
		"8.8.8.8": false, "1.1.1.1": false, "2606:4700:4700::1111": false, "100.128.0.1": false,
	}
	for address, want := range cases {
		ip := netParse(t, address)
		if got := isInternalIP(ip); got != want {
			t.Errorf("isInternalIP(%s) = %v, erwartet %v", address, got, want)
		}
	}
}

func netParse(t *testing.T, address string) net.IP {
	t.Helper()
	ip := net.ParseIP(address)
	if ip == nil {
		t.Fatalf("keine Adresse: %s", address)
	}
	return ip
}
