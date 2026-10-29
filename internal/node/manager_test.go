package node

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createPaper(t *testing.T, manager *Manager, name string) *Server {
	t.Helper()
	server, err := manager.Create(Spec{Name: name, Software: "paper", Version: "1.21.8", EULA: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Installation", func() bool { return server.View().Installed })
	waitState(t, server, StateOffline)
	return server
}

func TestInstallStartCommandStop(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Lobby")

	view := server.View()
	if view.JavaMajor != 21 || len(view.Ports) != 1 || view.Ports[0].Host < 41000 {
		t.Fatalf("Ansicht nach der Installation: %+v", view)
	}
	jar := filepath.Join(manager.cfg.DataDir, "servers", view.ID, "data", "server.jar")
	if data, err := os.ReadFile(jar); err != nil || string(data) != "jar:fake-1.21.8.jar" {
		t.Fatalf("server.jar: %q %v", data, err)
	}
	if eula, _ := server.FS().Read("eula.txt", 1024); !strings.Contains(string(eula), "eula=true") {
		t.Fatalf("eula.txt: %q", eula)
	}

	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); !errors.Is(err, ErrBadState) {
		t.Fatalf("doppeltes Starten muss scheitern, erhalten %v", err)
	}
	waitState(t, server, StateRunning)
	waitFor(t, "Messwerte", func() bool { return server.View().Stats != nil })

	spec := rt.containers["c1"].spec
	if spec.Image != "eclipse-temurin:21-jre" || spec.MemoryBytes != 2048<<20 || spec.WorkDir != "/data" || spec.Labels[LabelServer] != view.ID {
		t.Fatalf("Container-Spec: %+v", spec)
	}
	if !strings.Contains(spec.Cmd[2], "exec java -Xms1536M -Xmx1536M") || !strings.Contains(spec.Cmd[2], "-jar server.jar nogui") {
		t.Fatalf("Startzeile: %s", spec.Cmd[2])
	}

	if err := server.Command("say hallo"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Echo des Befehls", func() bool { return hasLine(server, "Befehl: say hallo") })
	if err := server.Command("bad\ncommand"); err == nil {
		t.Fatal("ein Befehl mit Zeilenumbruch muss abgelehnt werden")
	}

	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	waitFor(t, "Endemeldung", func() bool { return hasLine(server, "Server beendet (Exit-Code 0)") })
	if err := server.Command("x"); !errors.Is(err, ErrBadState) {
		t.Fatalf("Befehl an einen gestoppten Server: %v", err)
	}
}

func TestStopFallsBackToKill(t *testing.T) {
	rt := newFakeRuntime()
	rt.ignoreStop = true
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Stur")
	_ = server.Start()
	waitState(t, server, StateRunning)
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	waitFor(t, "Hinweis und Exit-Code 137", func() bool {
		return hasLine(server, "reagiert nicht") && hasLine(server, "Exit-Code 137")
	})
}

func TestRestartAndKill(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Neustart")
	_ = server.Start()
	waitState(t, server, StateRunning)

	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "zweiter Container läuft", func() bool {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		c := rt.containers["c2"]
		return c != nil && c.running && server.View().State == StateRunning
	})
	if err := server.Kill(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	if err := server.Kill(); !errors.Is(err, ErrBadState) {
		t.Fatalf("Kill eines gestoppten Servers: %v", err)
	}
	if err := server.Stop(); !errors.Is(err, ErrBadState) {
		t.Fatalf("Stop eines gestoppten Servers: %v", err)
	}
}

func TestFailedInstallKeepsServerOfflineAndRetries(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server, err := manager.Create(Spec{Name: "Kaputt", Software: "paper", Version: "0.0"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Fehlermeldung", func() bool { return server.View().LastError != "" })
	if server.View().Installed || server.View().State != StateOffline {
		t.Fatalf("Ansicht: %+v", server.View())
	}
	// Mit einer gültigen Version klappt Neuinstallieren
	if err := server.Reinstall("paper", "1.21.8"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Installation", func() bool { return server.View().Installed })
	if view := server.View(); view.Version != "1.21.8" || view.LastError != "" {
		t.Fatalf("nach Reinstall: %+v", view)
	}
}

func TestUpdateOnlyWhenOffline(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Einstellungen")
	memory, name := 4096, "Neu benannt"
	if err := server.Update(Patch{MemoryMB: &memory, Name: &name}); err != nil {
		t.Fatal(err)
	}
	if view := server.View(); view.MemoryMB != 4096 || view.Name != "Neu benannt" {
		t.Fatalf("nach Update: %+v", view)
	}
	tooSmall := 1
	if err := server.Update(Patch{MemoryMB: &tooSmall}); err == nil {
		t.Fatal("ungültiger Speicher wurde akzeptiert")
	}
	_ = server.Start()
	waitState(t, server, StateRunning)
	if err := server.Update(Patch{MemoryMB: &memory}); !errors.Is(err, ErrBadState) {
		t.Fatalf("Update eines laufenden Servers: %v", err)
	}
}

func TestSettingsSurviveRestartOfTheDaemonAndRunningServerIsAdopted(t *testing.T) {
	cfg := testConfig(t)
	rt := newFakeRuntime()
	first := newTestManager(t, cfg, rt)
	server := createPaper(t, first, "Bleibt")
	id := server.View().ID
	_ = server.Start()
	waitState(t, server, StateRunning)
	waitFor(t, "Startmeldung", func() bool { return hasLine(server, "Done (") })

	// Der Daemon startet neu, der Container läuft weiter
	second := newTestManager(t, cfg, rt)
	adopted, ok := second.Get(id)
	if !ok || adopted.View().Name != "Bleibt" || !adopted.View().Installed {
		t.Fatalf("Server nach dem Neustart: %v %+v", ok, adopted)
	}
	if adopted.View().State != StateOffline {
		t.Fatal("vor dem Abgleich ist der Zustand unbekannt, also offline")
	}
	if err := second.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitState(t, adopted, StateRunning)
	waitFor(t, "alter Verlauf wird wiederhergestellt", func() bool { return hasLine(adopted, "Done (1.234s)") })
	if err := adopted.Command("say wieder da"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Befehl nach der Übernahme", func() bool { return hasLine(adopted, "Befehl: say wieder da") })
	_ = adopted.Stop()
	waitState(t, adopted, StateOffline)
}

func TestPortsAreUniqueAndFreedOnDelete(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	one := createPaper(t, manager, "Eins")
	two := createPaper(t, manager, "Zwei")
	a, b := one.View().Ports[0].Host, two.View().Ports[0].Host
	if a == b || a == 0 || b == 0 {
		t.Fatalf("Ports müssen verschieden sein: %d %d", a, b)
	}
	if _, err := manager.Create(Spec{Name: "Kollision", Software: "paper", Version: "1.21.8", Ports: []PortMapping{{Container: 25565, Host: a}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("doppelter Port muss ErrConflict liefern, erhalten %v", err)
	}
	if err := manager.Delete(one.View().ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Get(one.View().ID); ok {
		t.Fatal("gelöschter Server ist noch da")
	}
	if _, err := os.Stat(filepath.Join(manager.cfg.DataDir, "servers", one.View().ID)); !os.IsNotExist(err) {
		t.Fatalf("Ordner des gelöschten Servers existiert noch: %v", err)
	}
	again, err := manager.Create(Spec{Name: "Wieder", Software: "paper", Version: "1.21.8", Ports: []PortMapping{{Container: 25565, Host: a}}})
	if err != nil || again.View().Ports[0].Host != a {
		t.Fatalf("freigegebener Port muss wieder nutzbar sein: %v", err)
	}
	waitFor(t, "Installation", func() bool { return again.View().Installed })
}

func TestDeleteStopsRunningServer(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Läuft")
	_ = server.Start()
	waitState(t, server, StateRunning)
	if err := manager.Delete(server.View().ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Container entfernt", func() bool { return rt.count() == 0 })
}

func TestVelocityUsesItsOwnStopCommandAndPort(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	proxy, err := manager.Create(Spec{Name: "Proxy", Software: "velocity", Version: "3.4.0-SNAPSHOT"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Installation", func() bool { return proxy.View().Installed })
	waitState(t, proxy, StateOffline)
	if proxy.View().Ports[0].Container != 25565 {
		t.Fatalf("Velocity-Port: %+v", proxy.View().Ports)
	}
	// Velocity bekommt keine EULA-Datei
	if _, err := proxy.FS().Read("eula.txt", 10); err == nil {
		t.Fatal("ein Proxy braucht keine eula.txt")
	}
	_ = proxy.Start()
	waitState(t, proxy, StateRunning)
	if cmd := rt.containers["c1"].spec.Cmd[2]; strings.Contains(cmd, "nogui") {
		t.Fatalf("Velocity wird ohne nogui gestartet: %s", cmd)
	}
	_ = proxy.Stop()
	waitState(t, proxy, StateOffline)
	waitFor(t, "end statt stop", func() bool { return hasLine(proxy, "Stopping server") })
}

func TestStartWithoutDockerFailsCleanly(t *testing.T) {
	rt := &brokenRuntime{fakeRuntime: newFakeRuntime()}
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "OhneDocker")
	_ = server.Start()
	waitFor(t, "Fehler wird gemeldet", func() bool { return server.View().LastError != "" })
	waitState(t, server, StateOffline)
	if !hasLine(server, "Start fehlgeschlagen") {
		t.Fatal("der Fehler gehört in die Konsole")
	}
	time.Sleep(10 * time.Millisecond)
}

// brokenRuntime verhält sich wie ein Docker, das nicht läuft.
type brokenRuntime struct{ *fakeRuntime }

func (b *brokenRuntime) EnsureImage(_ context.Context, _ string) error {
	return errors.New("Cannot connect to the Docker daemon")
}

func TestLastExitTellsCrashFromRequestedStop(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Absturz")
	if server.View().LastExit != nil {
		t.Fatal("vor dem ersten Lauf gibt es kein Ende")
	}

	// Ein Absturz: der Prozess endet von allein mit Code 1
	_ = server.Start()
	waitState(t, server, StateRunning)
	rt.mu.Lock()
	crashed := rt.containers["c1"]
	rt.mu.Unlock()
	rt.finish(crashed, 1)
	waitState(t, server, StateOffline)
	exit := server.View().LastExit
	if exit == nil || exit.Code != 1 || exit.Requested || exit.At == 0 {
		t.Fatalf("Absturz: %+v", exit)
	}

	// Ein verlangter Stopp ist kein Absturz, auch wenn der Prozess hart beendet werden musste (Code 137)
	rt.ignoreStop = true
	_ = server.Start()
	waitState(t, server, StateRunning)
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	if exit := server.View().LastExit; exit == nil || exit.Code != 137 || !exit.Requested {
		t.Fatalf("Stopp: %+v", exit)
	}

	// Kill über die Schnittstelle zählt ebenfalls als verlangt
	rt.ignoreStop = false
	_ = server.Start()
	waitState(t, server, StateRunning)
	if err := server.Kill(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	if exit := server.View().LastExit; exit == nil || !exit.Requested {
		t.Fatalf("Kill: %+v", exit)
	}
}
