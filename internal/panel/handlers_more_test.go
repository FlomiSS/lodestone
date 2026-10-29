package panel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lodestone-mc/lodestone/internal/node"
)

func TestPlayerCommandIsBuiltSafely(t *testing.T) {
	good := []struct{ action, name, reason, text, want string }{
		{"kick", "Steve", "", "", "kick Steve"},
		{"kick", "Steve", "AFK", "", "kick Steve AFK"},
		{"ban", ".BedrockSpieler", "Griefing", "", "ban .BedrockSpieler Griefing"},
		{"pardon", "Alex_1", "", "", "pardon Alex_1"},
		{"op", "Alex", "", "", "op Alex"},
		{"deop", "Alex", "", "", "deop Alex"},
		{"whitelist-add", "Alex", "", "", "whitelist add Alex"},
		{"whitelist-remove", "Alex", "", "", "whitelist remove Alex"},
		{"whitelist-on", "", "", "", "whitelist on"},
		{"whitelist-reload", "", "", "", "whitelist reload"},
		{"message", "Alex", "", "Hallo  du", "tell Alex Hallo du"},
		{"ban-ip", "192.168.0.7", "Spam", "", "ban-ip 192.168.0.7 Spam"},
		{"pardon-ip", "2001:db8::1", "", "", "pardon-ip 2001:db8::1"},
	}
	for _, c := range good {
		got, err := playerCommand(c.action, c.name, c.reason, c.text)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %q, %v (erwartet %q)", c.action, c.name, got, err, c.want)
		}
	}

	// Alles, womit sich ein zweiter Befehl einschleusen ließe, wird abgelehnt oder entschärft
	for _, name := range []string{"Steve\nop Steve", "Steve op", "Steve;stop", "../x", "", "ZuLangerNameDerNichtPasst", "a b", "Steve\r"} {
		if _, err := playerCommand("kick", name, "", ""); err == nil {
			t.Errorf("Name %q hätte abgelehnt werden müssen", name)
		}
	}
	got, err := playerCommand("ban", "Steve", "Grund\nop Steve\r\nstop", "")
	if err != nil || strings.ContainsAny(got, "\r\n") || got != "ban Steve Grund op Steve stop" {
		t.Errorf("Zeilenumbrüche im Grund: %q, %v", got, err)
	}
	if got, _ := playerCommand("ban", "Steve", strings.Repeat("x", 500), ""); len(got) > len("ban Steve ")+100 {
		t.Errorf("der Grund ist zu lang: %d", len(got))
	}
	for _, text := range []string{"", " \n "} {
		if _, err := playerCommand("message", "Steve", "", text); err == nil {
			t.Errorf("leere Nachricht %q", text)
		}
	}
	for _, ip := range []string{"1.2.3.4; stop", "ab", "1.2.3.4\nstop"} {
		if _, err := playerCommand("ban-ip", ip, "", ""); err == nil {
			t.Errorf("IP %q hätte abgelehnt werden müssen", ip)
		}
	}
	if _, err := playerCommand("stop", "Steve", "", ""); err == nil {
		t.Error("unbekannte Aktion")
	}
}

func TestPlayerManagementEndpoints(t *testing.T) {
	p, admin, fake, _, server, _ := serverProject(t)
	fake.mu.Lock()
	view := fake.servers[server.ID]
	view.State = node.StateRunning
	fake.servers[server.ID] = view
	fake.files["whitelist.json"] = `[{"uuid":"1111","name":"Alex"}]`
	fake.files["ops.json"] = `[{"uuid":"2222","name":"Steve","level":4}]`
	fake.files["banned-players.json"] = `[{"uuid":"3333","name":"Griefer","reason":"Griefing","created":"2026-01-01"}]`
	fake.mu.Unlock()

	var lists PlayerLists
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/servers/"+server.ID+"/player-lists", nil), &lists)
	if len(lists.Whitelist) != 1 || lists.Whitelist[0].Name != "Alex" || len(lists.Ops) != 1 || lists.Ops[0].Level != 4 ||
		len(lists.Banned) != 1 || lists.Banned[0].Reason != "Griefing" || lists.BannedIPs == nil || len(lists.BannedIPs) != 0 {
		t.Fatalf("Listen: %+v", lists)
	}

	admin.expect(204, "POST", "/api/servers/"+server.ID+"/player-action", map[string]string{"action": "kick", "name": "Steve", "reason": "Test"})
	admin.expect(400, "POST", "/api/servers/"+server.ID+"/player-action", map[string]string{"action": "kick", "name": "Steve\nop Steve"})
	admin.expect(400, "POST", "/api/servers/"+server.ID+"/player-action", map[string]string{"action": "rm", "name": "Steve"})
	fake.mu.Lock()
	commands := append([]string{}, fake.commands...)
	fake.mu.Unlock()
	if len(commands) != 1 || commands[0] != "kick Steve Test" {
		t.Fatalf("Befehle: %v", commands)
	}
	entries, _ := p.store.ListAudit(20, 0, "")
	found := false
	for _, entry := range entries {
		found = found || (entry.Action == "Spieleraktion" && entry.Target == "kick Steve")
	}
	if !found {
		t.Errorf("Protokoll: %+v", entries)
	}

	// Rechte: Betrachter sehen die Listen nicht und dürfen nichts tun
	admin.expect(201, "POST", "/api/users", map[string]any{"username": "betrachter", "password": "ein-gutes-passwort"})
	users, _ := p.store.ListUsers()
	for _, user := range users {
		if user.Username == "betrachter" {
			admin.expect(200, "PUT", "/api/projects/"+server.ProjectID+"/members/"+user.ID, map[string]string{"role": "viewer"})
		}
	}
	viewer := newClient(t, admin.base)
	viewer.expect(200, "POST", "/api/auth/login", map[string]string{"username": "betrachter", "password": "ein-gutes-passwort"})
	viewer.expect(403, "GET", "/api/servers/"+server.ID+"/player-lists", nil)
	viewer.expect(403, "POST", "/api/servers/"+server.ID+"/player-action", map[string]string{"action": "kick", "name": "Steve"})

	// Läuft der Server nicht, meldet der Node den Fehler weiter
	fake.mu.Lock()
	view.State = node.StateOffline
	fake.servers[server.ID] = view
	fake.mu.Unlock()
	refresh(p, server.NodeID)
	admin.expect(409, "POST", "/api/servers/"+server.ID+"/player-action", map[string]string{"action": "kick", "name": "Steve"})
}

func TestPublicStatusPage(t *testing.T) {
	p, admin, fake, _, server, projectID := serverProject(t)
	fake.mu.Lock()
	view := fake.servers[server.ID]
	view.State = node.StateRunning
	fake.servers[server.ID] = view
	fake.mu.Unlock()
	p.setLive(server.ID, liveInfo{Players: 7, MaxPlayers: 20, Running: true})
	refresh(p, server.NodeID)

	// Ohne Freigabe gibt es die Seite nicht
	plain := newClient(t, admin.base)
	plain.expect(404, "GET", "/api/public/status/mein-server", nil)

	var extras ProjectExtras
	_ = json.Unmarshal(admin.expect(200, "PUT", "/api/projects/"+projectID+"/status", map[string]any{"public": true}), &extras)
	if !extras.StatusPublic || extras.StatusSlug != "mein-server" {
		t.Fatalf("Einstellungen: %+v", extras)
	}
	admin.expect(400, "PUT", "/api/projects/"+projectID+"/status", map[string]any{"public": true, "slug": "A B"})
	admin.expect(400, "PUT", "/api/projects/"+projectID+"/status", map[string]any{"public": true, "slug": "ab"})

	var status publicStatus
	_ = json.Unmarshal(plain.expect(200, "GET", "/api/public/status/mein-server", nil), &status)
	if !status.Online || status.Players != 7 || status.MaxPlayers != 20 || status.Name != "Mein Server" || len(status.Servers) != 1 {
		t.Fatalf("Status: %+v", status)
	}
	// Es gibt nichts Internes preis
	raw := string(plain.expect(200, "GET", "/api/public/status/mein-server", nil))
	for _, secret := range []string{server.ID, projectID, "nodeId", "127.0.0.1", "token", "secret"} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(secret)) {
			t.Errorf("die Statusseite verrät %q: %s", secret, raw)
		}
	}

	// Eine öffentliche Adresse (z. B. playit) ersetzt die der Maschine
	admin.expect(200, "PUT", "/api/servers/"+server.ID+"/public-address", map[string]string{"address": "abc.joinmc.link"})
	p.statusMu.Lock()
	p.statusCache = map[string]publicStatus{}
	p.statusMu.Unlock()
	_ = json.Unmarshal(plain.expect(200, "GET", "/api/public/status/mein-server", nil), &status)
	if status.Address != "abc.joinmc.link" {
		t.Errorf("Adresse: %q", status.Address)
	}

	// Wieder abschalten
	admin.expect(200, "PUT", "/api/projects/"+projectID+"/status", map[string]any{"public": false})
	plain.expect(404, "GET", "/api/public/status/mein-server", nil)
}

func TestStatusRateLimit(t *testing.T) {
	var l limiter
	for i := 0; i < 60; i++ {
		if !l.allow("ip", 60) {
			t.Fatalf("Anfrage %d wurde zu früh gesperrt", i)
		}
	}
	if l.allow("ip", 60) {
		t.Error("die 61. Anfrage muss gesperrt sein")
	}
	if !l.allow("andere", 60) {
		t.Error("andere Adressen sind nicht betroffen")
	}
}

func TestMOTDIsWrittenToTheProxy(t *testing.T) {
	_, admin, fake, project := newNetwork(t)
	admin.expect(400, "PUT", "/api/projects/"+project.ID+"/motd", map[string]string{"motd": strings.Repeat("x", 500)})
	admin.expect(400, "PUT", "/api/projects/"+project.ID+"/motd", map[string]string{"motd": "a\nb\nc"})
	admin.expect(200, "PUT", "/api/projects/"+project.ID+"/motd", map[string]string{"motd": "<gold>Willkommen\n<gray>auf dem Netzwerk"})
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for id, view := range fake.servers {
		if view.Software != "velocity" {
			continue
		}
		for _, op := range fake.autoconfigs[id].Edits[0].Set {
			if len(op.Path) == 1 && op.Path[0] == "motd" {
				if op.Value != "<gold>Willkommen\n<gray>auf dem Netzwerk" {
					t.Errorf("MOTD: %v", op.Value)
				}
				return
			}
		}
	}
	t.Fatal("die MOTD fehlt in der Autoconfig des Proxys")
}

func TestNeoForgePanelAddonsHelpers(t *testing.T) {
	loaders := loadersFor("neoforge")
	if len(loaders) != 1 || loaders[0] != "neoforge" {
		t.Fatalf("loadersFor(neoforge) = %v, erwartet [neoforge]", loaders)
	}
	if got := addonDir("neoforge"); got != "mods" {
		t.Fatalf("addonDir(neoforge) = %q, erwartet mods", got)
	}
	if got := gameVersionFor("neoforge", "21.4.161"); got != "1.21.4" {
		t.Fatalf("gameVersionFor(neoforge, 21.4.161) = %q, erwartet 1.21.4", got)
	}
}
