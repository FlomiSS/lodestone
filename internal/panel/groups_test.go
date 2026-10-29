package panel

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
)

func init() { startPoll = 10 * time.Millisecond }

func ptr[T any](value T) *T { return &value }

// newNetwork legt ein Netzwerk mit Proxy an und liefert die Teile für Tests des Autoscalers.
func newNetwork(t *testing.T) (*Panel, *client, *fakeNode, ProjectDTO) {
	p, web, fake, nodeID := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	var project ProjectDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Netz", "kind": "network"}), &project)
	admin.expect(201, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{Name: "Proxy", NodeID: nodeID, Software: "velocity", Version: "3.5.1"})
	return p, admin, fake, project
}

func instancesOf(t *testing.T, p *Panel, groupID string) []ServerRow {
	t.Helper()
	rows, err := p.store.ListGroupServers(groupID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Zeitüberschreitung: %s", what)
}

func runningBackends(fake *fakeNode) int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	count := 0
	for _, view := range fake.servers {
		if view.State == node.StateRunning && view.Software != "velocity" {
			count++
		}
	}
	return count
}

func TestGroupScalesWithPlayers(t *testing.T) {
	p, admin, fake, project := newNetwork(t)
	var group Group
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/groups", groupRequest{
		Name: "Lobby", Software: "paper", Version: "1.21.4", MemoryMB: 512, MinInstances: ptr(1), MaxInstances: ptr(4), EULA: true,
		PlayersPerInstance: ptr(10), IdleMinutes: ptr(1),
	}), &group)

	// Die Mindestzahl wird sofort angelegt und gestartet
	waitUntil(t, "eine Instanz läuft", func() bool { return len(instancesOf(t, p, group.ID)) == 1 && runningBackends(fake) == 1 })
	rows := instancesOf(t, p, group.ID)
	if rows[0].Slug != "lobby-1" || rows[0].Role != RoleBackend || !rows[0].InTry || rows[0].GroupID != group.ID {
		t.Fatalf("Instanz: %+v", rows[0])
	}
	fake.mu.Lock()
	if view := fake.servers[rows[0].ID]; !view.NoPublish || view.MemoryMB != 512 {
		t.Errorf("Instanz auf dem Node: %+v", view.Spec)
	}
	fake.mu.Unlock()

	// 9 von 10 Spielern = über 80 %: eine zweite Instanz kommt dazu
	p.setLive(rows[0].ID, liveInfo{At: time.Now(), Players: 9, Running: true})
	p.scaleMu.Lock()
	delete(p.scaleLast, group.ID) // die Pause zwischen zwei Skalierungen gilt im Test nicht
	p.scaleMu.Unlock()
	p.reconcileGroup(context.Background(), group)
	waitUntil(t, "zweite Instanz läuft", func() bool { return len(instancesOf(t, p, group.ID)) == 2 && runningBackends(fake) == 2 })

	// Das Netzwerk kennt beide Instanzen
	waitUntil(t, "velocity.toml enthält beide", func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		for id, view := range fake.servers {
			if view.Software != "velocity" {
				continue
			}
			for _, op := range fake.autoconfigs[id].Edits[0].Set {
				if len(op.Path) == 1 && op.Path[0] == "servers" {
					table, _ := op.Value.(map[string]any)
					_, a := table["lobby-1"]
					_, b := table["lobby-2"]
					return a && b
				}
			}
		}
		return false
	})

	// Weniger los: leere, überzählige Instanz wird nach dem Leerlauf gestoppt und gelöscht
	for _, row := range instancesOf(t, p, group.ID) {
		p.setLive(row.ID, liveInfo{At: time.Now(), Players: 0, Running: true})
	}
	p.reconcileGroup(context.Background(), group) // merkt sich den Beginn des Leerlaufs
	p.scaleMu.Lock()
	for id := range p.idleSince {
		p.idleSince[id] = time.Now().Add(-2 * time.Minute)
	}
	p.scaleMu.Unlock()
	p.reconcileGroup(context.Background(), group)
	waitUntil(t, "eine Instanz weg", func() bool { return len(instancesOf(t, p, group.ID)) == 1 })
	if left := instancesOf(t, p, group.ID)[0]; left.Slug != "lobby-1" {
		t.Errorf("die neuere Instanz sollte gehen, geblieben ist %s", left.Slug)
	}
}

func TestGroupValidation(t *testing.T) {
	_, admin, _, project := newNetwork(t)
	path := "/api/projects/" + project.ID + "/groups"
	admin.expect(400, "POST", path, groupRequest{Name: "X", Software: "vanilla", Version: "1.21.4", EULA: true})
	admin.expect(400, "POST", path, groupRequest{Name: "Lobby", Software: "paper", Version: "1.21.4"})
	admin.expect(400, "POST", path, groupRequest{Name: "Lobby", Software: "paper", Version: "1.21.4", EULA: true, MinInstances: ptr(2), MaxInstances: ptr(1)})
	// Der Kurzname darf keinem Server im Weg stehen
	admin.expect(409, "POST", path, groupRequest{Name: "Proxy", Software: "paper", Version: "1.21.4", EULA: true})
}

func TestGroupBlockedWhenNodeIsFull(t *testing.T) {
	p, admin, fake, project := newNetwork(t)
	fake.mu.Lock()
	fake.servers["riese"] = node.View{Spec: node.Spec{ID: "riese", MemoryMB: 60000}, State: node.StateRunning}
	fake.mu.Unlock()
	var group Group
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/groups", groupRequest{Name: "Lobby", Software: "paper", Version: "1.21.4", MemoryMB: 8192, MinInstances: ptr(1), EULA: true}), &group)
	p.reconcileGroup(context.Background(), group)
	if rows := instancesOf(t, p, group.ID); len(rows) != 0 {
		t.Fatalf("es darf nichts angelegt werden: %+v", rows)
	}
	if p.groupNote(group.ID) == "" {
		t.Error("die Gruppe sollte erklären, warum sie nicht skaliert")
	}
}

func TestVoicechatOverProxy(t *testing.T) {
	p, admin, fake, project := newNetwork(t)
	nodes, _ := p.store.ListNodes()
	var lobby ServerDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{Name: "Lobby", NodeID: nodes[0].ID, Software: "paper", Version: "1.21.4", EULA: true}), &lobby)

	admin.expect(200, "PUT", "/api/projects/"+project.ID+"/voicechat", map[string]bool{"enabled": true})

	rows, _ := p.store.ListServers(project.ID)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, row := range rows {
		view := fake.servers[row.ID]
		if row.Role == RoleProxy {
			if !hasUDPPort(view.Ports, proxyPort) {
				t.Errorf("der Proxy braucht einen UDP-Port: %+v", view.Ports)
			}
			found := false
			for _, edit := range fake.autoconfigs[row.ID].Edits {
				found = found || edit.File == "plugins/voicechat/voicechat-proxy.properties"
			}
			if !found {
				t.Errorf("keine Konfiguration des Voicechat-Proxys: %+v", fake.autoconfigs[row.ID].Edits)
			}
		} else if hasUDPPort(view.Ports, voicePort) {
			t.Errorf("Server hinter dem Proxy brauchen keinen eigenen UDP-Port: %+v", view.Ports)
		}
		if updated, _ := p.store.ServerByID(row.ID); !updated.Voicechat {
			t.Errorf("%s müsste Voicechat haben", row.Name)
		}
	}
	for _, edit := range fake.autoconfigs[lobby.ID].Edits {
		if edit.File == "plugins/voicechat/voicechat-server.properties" {
			t.Error("im Proxy-Modus bekommt der Server kein voice_host")
		}
	}
}

func TestMetricsAreBucketed(t *testing.T) {
	now := time.Now()
	samples := []Sample{
		{Time: now.Add(-50 * time.Minute).UnixMilli(), CPU: 10, Memory: 100, Players: 2},
		{Time: now.Add(-50*time.Minute + 5*time.Second).UnixMilli(), CPU: 30, Memory: 300, Players: 4},
		{Time: now.Add(-5 * time.Minute).UnixMilli(), CPU: 50, Memory: 500, Players: 1},
	}
	series := buildSeries("1h", now, map[string][]Sample{"a": samples}, "")
	if series.Samples != 2 {
		t.Fatalf("zwei belegte Fenster erwartet, %d", series.Samples)
	}
	if series.PeakPlayers != 3 || series.PeakCPU != 50 || series.PeakMemory != 500 {
		t.Errorf("Zusammenfassung: %+v", series)
	}
	nulls := 0
	for _, value := range series.CPU {
		if value == nil {
			nulls++
		}
	}
	if nulls != chartPoints-2 {
		t.Errorf("leere Fenster müssen null sein: %d", nulls)
	}

	// Zwei Server: CPU addiert sich, Spieler kommen nur vom gewählten (Proxy)
	two := buildSeries("1h", now, map[string][]Sample{
		"proxy":   {{Time: now.Add(-time.Minute).UnixMilli(), CPU: 5, Memory: 10, Players: 7}},
		"backend": {{Time: now.Add(-time.Minute).UnixMilli(), CPU: 20, Memory: 20, Players: 7}},
	}, "proxy")
	if two.PeakCPU != 25 || two.PeakPlayers != 7 {
		t.Errorf("Netzwerk-Reihe: %+v", two)
	}
}

func TestSamplesAndSeenPlayers(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UnixMilli()
	if err := store.AddSamples([]string{"s1", "s1"}, []Sample{{Time: now - 60000, CPU: 1, Memory: 2, Players: 3}, {Time: now, CPU: 4, Memory: 5, Players: 6}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Samples("s1", now-120000)
	if err != nil || len(got) != 2 || got[1].Players != 6 {
		t.Fatalf("Samples: %+v %v", got, err)
	}
	_ = store.TouchPlayers("s1", []string{"Alex", "Steve"}, now, 30)
	_ = store.TouchPlayers("s1", []string{"alex"}, now+30000, 30)
	players, _ := store.SeenPlayers("s1", 10)
	if len(players) != 2 {
		t.Fatalf("Namen sind nicht groß/klein-empfindlich: %+v", players)
	}
	for _, player := range players {
		if player.Name == "Alex" && player.Seconds != 30 {
			t.Errorf("Spielzeit von Alex: %d", player.Seconds)
		}
	}
	if err := store.PurgeMetrics(now + 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Samples("s1", 0); len(got) != 0 {
		t.Errorf("alte Messwerte müssen weg: %+v", got)
	}
}

func TestOldDatabaseIsMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// So sah die Datenbank von Version 0.1 aus: ohne die neuen Spalten
	for _, statement := range []string{"ALTER TABLE projects DROP COLUMN voicechat", "ALTER TABLE servers DROP COLUMN group_id"} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	store.Close()
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("Migration: %v", err)
	}
	defer reopened.Close()
	user, _ := reopened.CreateUser("a-b-c", "x", true)
	project, err := reopened.CreateProject(Project{Name: "p", Kind: "network", Secret: "s", OwnerID: user.ID, Voicechat: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.ProjectByID(project.ID); !got.Voicechat {
		t.Error("die neue Spalte funktioniert nicht")
	}
}

// Eine neue Instanz bekommt noch ihre Plugins und ist deshalb kurz aus; sie zählt trotzdem schon zur Mindestzahl.
func TestGroupDoesNotOvershootWhilePreparing(t *testing.T) {
	p, admin, fake, project := newNetwork(t)
	var group Group
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/groups", groupRequest{Name: "Lobby", Software: "paper", Version: "1.21.4", MemoryMB: 512, MinInstances: ptr(1), MaxInstances: ptr(3), EULA: true}), &group)
	for i := 0; i < 40; i++ {
		p.reconcileGroup(context.Background(), group)
		time.Sleep(5 * time.Millisecond)
	}
	waitUntil(t, "die Instanz läuft", func() bool { return runningBackends(fake) >= 1 })
	if rows := instancesOf(t, p, group.ID); len(rows) != 1 {
		t.Fatalf("es darf nur eine Instanz geben: %d", len(rows))
	}
}

// Stürzt eine Instanz ab, wird sie neu gestartet statt ersetzt – und nach mehreren Versuchen bleibt es dabei, statt
// immer neue Instanzen anzulegen.
func TestGroupRestartsCrashedInstanceInsteadOfSpawningNew(t *testing.T) {
	p, admin, fake, project := newNetwork(t)
	var group Group
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/groups", groupRequest{Name: "Lobby", Software: "paper", Version: "1.21.4", MemoryMB: 512, MinInstances: ptr(1), MaxInstances: ptr(3), EULA: true}), &group)
	waitUntil(t, "die Instanz läuft", func() bool { return runningBackends(fake) == 1 && len(instancesOf(t, p, group.ID)) == 1 })
	id := instancesOf(t, p, group.ID)[0].ID

	crash := func() {
		fake.mu.Lock()
		view := fake.servers[id]
		view.State = node.StateOffline
		fake.servers[id] = view
		fake.mu.Unlock()
		p.invalidate(view.Spec.ID)
		for nodeID := range p.views {
			p.invalidate(nodeID)
		}
		p.scaleMu.Lock()
		for k, times := range p.restarts { // die Pause zwischen Versuchen überspringen
			for i := range times {
				times[i] = times[i].Add(-time.Minute)
			}
			p.restarts[k] = times
		}
		p.scaleMu.Unlock()
		p.reconcileGroup(context.Background(), group)
	}
	for attempt := 1; attempt <= maxRestarts; attempt++ {
		crash()
		if rows := instancesOf(t, p, group.ID); len(rows) != 1 {
			t.Fatalf("Versuch %d: es darf keine neue Instanz entstehen (%d)", attempt, len(rows))
		}
		waitUntil(t, "neu gestartet", func() bool { return runningBackends(fake) == 1 })
	}
	crash() // der vierte Absturz: jetzt wird nichts mehr versucht
	if rows := instancesOf(t, p, group.ID); len(rows) != 1 {
		t.Fatalf("keine neue Instanz bei Absturzschleife: %d", len(rows))
	}
	if runningBackends(fake) != 0 {
		t.Error("die Instanz sollte nicht noch einmal gestartet werden")
	}
	if p.groupNote(group.ID) == "" {
		t.Error("die Gruppe soll die Absturzschleife melden")
	}
}

func TestFitsMemory(t *testing.T) {
	cases := []struct {
		name             string
		memory, used     int
		total, available int64
		want             bool
	}{
		{"genug Platz", 1536, 2048, 16000, 9000, true},
		{"Limits zu hoch", 4096, 10000, 16000, 9000, false},
		{"Host fast voll, obwohl die Limits passen", 1536, 2048, 16000, 2000, false},
		{"freier Speicher unbekannt", 1536, 2048, 16000, 0, true},
		{"gerade genug", 1024, 0, 16000, 1024*5/4 + hostReserveMB, true},
	}
	for _, c := range cases {
		if got, why := fitsMemory(c.memory, c.used, c.total, c.available); got != c.want {
			t.Errorf("%s: %v (%s)", c.name, got, why)
		}
	}
}
