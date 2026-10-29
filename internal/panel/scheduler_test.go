package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/lodestone-mc/lodestone/internal/node"
)

func init() {
	waitUnit = time.Millisecond
	restartSettle = time.Millisecond
	pollEvery = 5 * time.Millisecond
}

func berlin(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func TestNextRun(t *testing.T) {
	loc := berlin(t)
	at := func(year int, month time.Month, day, hour, minute int) time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, loc)
	}

	daily := Schedule{Kind: "daily", AtMinute: 4 * 60}
	if got := nextRun(daily, at(2026, 11, 2, 3, 0), loc); !got.Equal(at(2026, 11, 2, 4, 0)) {
		t.Errorf("täglich, vor 04:00: %v", got)
	}
	if got := nextRun(daily, at(2026, 11, 2, 4, 0), loc); !got.Equal(at(2026, 11, 3, 4, 0)) {
		t.Errorf("täglich, genau 04:00 → morgen: %v", got)
	}

	// Wochentage: Montag und Freitag um 18:30
	weekly := Schedule{Kind: "weekly", AtMinute: 18*60 + 30, Weekdays: 1<<0 | 1<<4}
	if got := nextRun(weekly, at(2026, 11, 2, 19, 0), loc); got.Weekday() != time.Friday || !got.Equal(at(2026, 11, 6, 18, 30)) { // Montag, schon nach 18:30
		t.Errorf("wöchentlich: %v (%s)", got, got.Weekday())
	}
	if got := nextRun(weekly, at(2026, 11, 6, 19, 0), loc); got.Weekday() != time.Monday || !got.Equal(at(2026, 11, 9, 18, 30)) {
		t.Errorf("wöchentlich über das Wochenende: %v", got)
	}
	sunday := Schedule{Kind: "weekly", AtMinute: 60, Weekdays: 1 << 6}
	if got := nextRun(sunday, at(2026, 11, 2, 0, 0), loc); got.Weekday() != time.Sunday {
		t.Errorf("Sonntag ist Bit 6: %v", got.Weekday())
	}

	// Zeitumstellung: Am 25.10.2026 endet die Sommerzeit; 04:00 bleibt 04:00 Ortszeit
	if got := nextRun(daily, at(2026, 10, 24, 5, 0), loc); got.Hour() != 4 || got.Day() != 25 {
		t.Errorf("Umstellung: %v", got)
	}

	interval := Schedule{Kind: "interval", EveryMinutes: 90}
	if got := nextRun(interval, at(2026, 11, 2, 10, 0), loc); !got.Equal(at(2026, 11, 2, 11, 30)) {
		t.Errorf("Intervall: %v", got)
	}
}

func TestSchedulePlanning(t *testing.T) {
	p := &Panel{cfg: Config{Location: berlin(t)}, warnedKeys: map[string]bool{}}
	loc := p.location()
	now := time.Date(2026, 11, 2, 3, 56, 0, 0, loc)
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, loc).UnixMilli()

	restart := Schedule{ID: "r", Kind: "daily", AtMinute: 4 * 60, Enabled: true, CreatedAt: created, WarnMinutes: 5, Steps: []Step{{Action: "restart"}}}
	disabled := Schedule{ID: "d", Kind: "daily", AtMinute: 3 * 60, Enabled: false, CreatedAt: created, Steps: []Step{{Action: "backup"}}}
	missed := Schedule{ID: "m", Kind: "daily", AtMinute: 60, Enabled: true, CreatedAt: created, Steps: []Step{{Action: "backup"}}}      // 01:00, vor fast 3 Stunden
	late := Schedule{ID: "l", Kind: "daily", AtMinute: 3*60 + 30, Enabled: true, CreatedAt: created, Steps: []Step{{Action: "backup"}}} // 03:30, vor 26 Minuten
	every := Schedule{ID: "i", Kind: "interval", EveryMinutes: 30, Enabled: true, CreatedAt: now.Add(-3 * time.Hour).UnixMilli(), Steps: []Step{{Action: "command", Value: "save-all"}}}
	list := []Schedule{restart, disabled, missed, late, every}

	got := p.plan(now, list)
	// 04:00 ist in 4 Minuten: Vorwarnung "5 Minuten" (Fenster ist schon offen) – die Minute-Warnung noch nicht
	if len(got.warn) != 1 || got.warn[0].minutes != 5 || got.warn[0].schedule.ID != "r" {
		t.Fatalf("Vorwarnungen: %+v", got.warn)
	}
	// 03:30 liegt innerhalb der Toleranz und läuft nach; 01:00 ist zu lange her; das Intervall läuft immer nach
	ids := func(list []Schedule) string {
		out := ""
		for _, s := range list {
			out += s.ID
		}
		return out
	}
	if ids(got.run) != "li" || ids(got.skip) != "m" {
		t.Fatalf("run=%s skip=%s", ids(got.run), ids(got.skip))
	}

	// Eine gesendete Vorwarnung kommt nicht noch einmal; eine Minute vorher kommt die zweite
	due := p.dueAt(restart)
	p.markWarned(restartKey(restart, due, 5))
	if got := p.plan(now, []Schedule{restart}); len(got.warn) != 0 {
		t.Fatalf("doppelte Vorwarnung: %+v", got.warn)
	}
	if got := p.plan(due.Add(-30*time.Second), []Schedule{restart}); len(got.warn) != 1 || got.warn[0].minutes != 1 {
		t.Fatalf("Warnung kurz vor dem Neustart: %+v", got.warn)
	}
	// Ohne Neustart oder Stopp gibt es keine Vorwarnung
	backup := restart
	backup.ID, backup.Steps = "b", []Step{{Action: "backup"}}
	if got := p.plan(now, []Schedule{backup}); len(got.warn) != 0 {
		t.Fatalf("Backups warnen nicht: %+v", got.warn)
	}
}

func restartKey(s Schedule, due time.Time, minutes int) string {
	return fmt.Sprintf("%s|%d|%d", s.ID, due.UnixMilli(), minutes)
}

func TestRunScheduleExecutesStepsInOrder(t *testing.T) {
	p, admin, fake, nodeID, server, projectID := serverProject(t)
	// der Server läuft
	fake.mu.Lock()
	view := fake.servers[server.ID]
	view.State = node.StateRunning
	fake.servers[server.ID] = view
	fake.mu.Unlock()

	var created scheduleDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+projectID+"/schedules", map[string]any{
		"serverId": server.ID, "name": "Nachts", "kind": "daily", "atMinute": 240, "warnMinutes": 5,
		"steps": []Step{{Action: "command", Value: "say Backup startet"}, {Action: "backup", Value: "nachts"}, {Action: "wait", Value: "1"}, {Action: "restart"}},
	}), &created)
	if created.NextRun == 0 || created.ServerName != "Main" {
		t.Fatalf("angelegt: %+v", created)
	}

	schedule, _ := p.store.ScheduleByID(created.ID)
	refresh(p, nodeID)
	p.runSchedule(context.Background(), schedule)

	after, _ := p.store.ScheduleByID(created.ID)
	if after.LastStatus != "ok" || after.LastRun == 0 {
		t.Fatalf("Ergebnis: %+v", after)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.commands) != 1 || fake.commands[0] != "say Backup startet" {
		t.Errorf("Befehle: %v", fake.commands)
	}
	if len(fake.backups) != 1 || !strings.Contains(fake.backups[0].Name, "nachts") {
		t.Errorf("Backups: %+v", fake.backups)
	}
	if len(fake.powered) == 0 || fake.powered[len(fake.powered)-1] != "restart:"+server.ID {
		t.Errorf("Neustart: %v", fake.powered)
	}
	kinds := eventKindsOfLocked(t, p, projectID)
	if len(kinds) != 1 || kinds[0] != "backup" {
		t.Errorf("Ereignisse: %v", kinds)
	}
}

// eventKindsOfLocked gibt acht, dass der Fake-Node nicht gesperrt ist, während die Ereignisse gelesen werden.
func eventKindsOfLocked(t *testing.T, p *Panel, projectID string) []string {
	return eventKindsOf(t, p, projectID)
}

func TestScheduleFailureStopsAndReports(t *testing.T) {
	p, admin, fake, _, server, projectID := serverProject(t)
	var created scheduleDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+projectID+"/schedules", map[string]any{
		"serverId": server.ID, "name": "Befehl", "kind": "interval", "everyMinutes": 10,
		"steps": []Step{{Action: "command", Value: "save-all"}, {Action: "backup"}},
	}), &created)
	schedule, _ := p.store.ScheduleByID(created.ID)
	p.runSchedule(context.Background(), schedule) // der Server ist aus: der Befehl scheitert

	after, _ := p.store.ScheduleByID(created.ID)
	if after.LastStatus != "failed" || !strings.Contains(after.LastMessage, "Schritt 1") {
		t.Fatalf("Ergebnis: %+v", after)
	}
	fake.mu.Lock()
	backups := len(fake.backups)
	fake.mu.Unlock()
	if backups != 0 {
		t.Error("nach einem fehlgeschlagenen Schritt darf nichts weiterlaufen")
	}
	if kinds := eventKindsOf(t, p, projectID); len(kinds) != 1 || kinds[0] != "schedule_failed" {
		t.Errorf("Ereignisse: %v", kinds)
	}
}

func TestFailedBackupIsReported(t *testing.T) {
	p, admin, fake, _, server, projectID := serverProject(t)
	fake.mu.Lock()
	fake.backupFails = true // der Node nimmt das Backup an, es entsteht aber nie
	fake.mu.Unlock()
	var created scheduleDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+projectID+"/schedules", map[string]any{
		"serverId": server.ID, "name": "Backup", "kind": "daily", "atMinute": 60, "steps": []Step{{Action: "backup"}},
	}), &created)
	schedule, _ := p.store.ScheduleByID(created.ID)
	p.runSchedule(context.Background(), schedule)
	kinds := eventKindsOf(t, p, projectID)
	if len(kinds) != 2 || kinds[0] != "schedule_failed" || kinds[1] != "backup_failed" {
		t.Fatalf("Ereignisse: %v", kinds)
	}
}

func TestScheduleValidationAndPermissions(t *testing.T) {
	p, admin, _, _, server, projectID := serverProject(t)
	path := "/api/projects/" + projectID + "/schedules"
	good := map[string]any{"serverId": server.ID, "name": "x", "kind": "daily", "atMinute": 60, "steps": []Step{{Action: "backup"}}}
	with := func(key string, value any) map[string]any {
		copy := map[string]any{}
		for k, v := range good {
			copy[k] = v
		}
		copy[key] = value
		return copy
	}
	admin.expect(400, "POST", path, with("steps", []Step{}))
	admin.expect(400, "POST", path, with("steps", []Step{{Action: "rm -rf"}}))
	admin.expect(400, "POST", path, with("steps", []Step{{Action: "command", Value: "say a\nstop"}}))
	admin.expect(400, "POST", path, with("steps", []Step{{Action: "wait", Value: "9999"}}))
	admin.expect(400, "POST", path, with("kind", "monthly"))
	admin.expect(400, "POST", path, with("atMinute", 2000))
	admin.expect(400, "POST", path, with("kind", "interval"))
	weekly := with("kind", "weekly")
	weekly["weekdays"] = 0 // kein Wochentag gewählt
	admin.expect(400, "POST", path, weekly)
	admin.expect(400, "POST", path, with("serverId", "fremd"))
	admin.expect(400, "POST", path, with("warnMinutes", 90))

	// Nur Admins legen an; Betrachter sehen die Liste, Operatoren starten einen Lauf
	for _, name := range []string{"betrachter", "operator"} {
		admin.expect(201, "POST", "/api/users", map[string]any{"username": name, "password": "ein-gutes-passwort"})
	}
	users, _ := p.store.ListUsers()
	ids := map[string]string{}
	for _, user := range users {
		ids[user.Username] = user.ID
	}
	admin.expect(200, "PUT", "/api/projects/"+projectID+"/members/"+ids["betrachter"], map[string]string{"role": "viewer"})
	admin.expect(200, "PUT", "/api/projects/"+projectID+"/members/"+ids["operator"], map[string]string{"role": "operator"})
	var created scheduleDTO
	_ = json.Unmarshal(admin.expect(201, "POST", path, good), &created)

	login := func(name string) *client {
		c := newClient(t, admin.base)
		c.expect(200, "POST", "/api/auth/login", map[string]string{"username": name, "password": "ein-gutes-passwort"})
		return c
	}
	viewer, operator := login("betrachter"), login("operator")
	viewer.expect(200, "GET", path, nil)
	viewer.expect(403, "POST", path, good)
	viewer.expect(403, "DELETE", "/api/schedules/"+created.ID, nil)
	viewer.expect(403, "POST", "/api/schedules/"+created.ID+"/run", nil)
	operator.expect(403, "PATCH", "/api/schedules/"+created.ID, map[string]any{"enabled": false})
	operator.expect(202, "POST", "/api/schedules/"+created.ID+"/run", nil)

	admin.expect(200, "PATCH", "/api/schedules/"+created.ID, map[string]any{"enabled": false, "name": "neu"})
	updated, _ := p.store.ScheduleByID(created.ID)
	if updated.Enabled || updated.Name != "neu" {
		t.Errorf("Änderung: %+v", updated)
	}
	admin.expect(204, "DELETE", "/api/schedules/"+created.ID, nil)
	admin.expect(404, "PATCH", "/api/schedules/"+created.ID, map[string]any{"enabled": true})

	// Fremde sehen nichts
	admin.expect(201, "POST", "/api/users", map[string]any{"username": "fremder", "password": "ein-gutes-passwort"})
	login("fremder").expect(404, "GET", path, nil)
}
