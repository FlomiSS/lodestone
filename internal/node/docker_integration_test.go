//go:build docker

package node

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestDockerEndToEnd läuft gegen ein echtes Docker: Der "Server" ist ein kleines Shell-Programm in Alpine, das wie
// Minecraft "Done" meldet, Befehle zurückgibt und bei "stop" endet. So lässt sich der ganze Weg prüfen (Image laden,
// Container mit Limits und Ports anlegen, Konsole, Messwerte, sauber stoppen), ohne Minecraft herunterzuladen.
//
//	go test -tags docker ./internal/node -run Docker -v
func TestDockerEndToEnd(t *testing.T) {
	runtime, err := NewDockerRuntime("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	info, err := runtime.Info(ctx)
	if err != nil {
		t.Skipf("Docker ist nicht erreichbar: %v", err)
	}
	t.Logf("Docker %s auf %s (%d CPUs)", info.DockerVersion, info.OS, info.CPUs)

	manager := newTestManager(t, testConfig(t), runtime)
	manager.sw = fakeSoftware{}
	server, err := manager.Create(Spec{
		Name:     "Docker-Test",
		Software: "paper",
		Version:  "1.21.8",
		MemoryMB: 256,
		Image:    "alpine:3.20",
		Startup:  `echo 'Done (0.1s)! Docker-Test'; while read line; do echo "got: $line"; if [ "$line" = stop ]; then echo bye; exit 0; fi; done`,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Delete(server.ID()) })
	waitFor(t, "Installation", func() bool { return server.View().Installed })
	waitState(t, server, StateOffline)

	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Minute) // das erste Mal wird das Image geladen
	for server.View().State != StateRunning {
		if time.Now().After(deadline) {
			t.Fatalf("Server wurde nicht bereit: %+v\n%s", server.View(), dump(server))
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := server.Command("hallo docker"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Echo aus dem Container", func() bool { return hasLine(server, "got: hallo docker") })
	waitFor(t, "Messwerte", func() bool { return server.View().Stats != nil && server.View().Stats.MemoryLimit > 0 })
	if limit := server.View().Stats.MemoryLimit; limit != 256<<20 {
		t.Errorf("Speicherlimit = %d, erwartet %d", limit, 256<<20)
	}

	// Dateien im Serverordner sind im Container unter /data sichtbar (gleicher Ordner)
	if err := server.FS().Write("marker.txt", strings.NewReader("vom Host"), 1024, true); err != nil {
		t.Fatal(err)
	}

	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	if !hasLine(server, "bye") || !hasLine(server, "Exit-Code 0") {
		t.Fatalf("der Server hat sich nicht sauber beendet:\n%s", dump(server))
	}

	// Neustart des Daemons: neuer Manager findet den (gestoppten) Container wieder und startet erneut
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateRunning)
	if err := server.Kill(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
}

func dump(server *Server) string {
	var out strings.Builder
	for _, line := range server.Logs(0) {
		out.WriteString(line.Text + "\n")
	}
	return out.String()
}
