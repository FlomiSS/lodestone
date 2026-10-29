package node

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

const testSecret = "a1b2c3d4e5f60718293a4b5c6d7e8f90"

func TestPlayitLifecycle(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	ctx := context.Background()

	if status := manager.PlayitStatus(ctx); status.Configured || status.Running {
		t.Fatalf("ohne Einrichtung: %+v", status)
	}
	for _, bad := range []string{"", "kurz", "mit leerzeichen und viel zu lang abc", "../../etc/passwd-und-mehr"} {
		if err := manager.ConfigurePlayit(ctx, bad); !errors.Is(err, errBadRequest) {
			t.Errorf("%q hätte abgelehnt werden müssen: %v", bad, err)
		}
	}

	if err := manager.ConfigurePlayit(ctx, "  "+testSecret+"\n"); err != nil {
		t.Fatal(err)
	}
	container := rt.find(playitContainer)
	if container == nil || !container.running {
		t.Fatal("der Agent muss laufen")
	}
	spec := container.spec
	if spec.Network != "host" || spec.Restart != "unless-stopped" || spec.Labels[LabelService] != "playit" || spec.Labels[LabelServer] != "" {
		t.Errorf("Container-Spec: %+v", spec)
	}
	if len(spec.Env) != 1 || spec.Env[0] != "SECRET_KEY="+testSecret {
		t.Errorf("Umgebung: %v", spec.Env)
	}
	if spec.Image != DefaultPlayitImage {
		t.Errorf("Image: %s", spec.Image)
	}
	info, err := os.Stat(manager.playitPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 && !strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
		t.Errorf("die Schlüsseldatei darf nur für den Daemon lesbar sein: %v", info.Mode())
	}

	// Der Schlüssel taucht in der Anzeige nicht auf, auch wenn der Agent ihn ausgibt
	container.say("Agent gestartet mit " + testSecret)
	container.say("Tunnel bereit")
	status := manager.PlayitStatus(ctx)
	if !status.Configured || !status.Running || len(status.Logs) < 2 {
		t.Fatalf("Zustand: %+v", status)
	}
	for _, line := range status.Logs {
		if strings.Contains(line, testSecret) {
			t.Errorf("der Schlüssel steht in der Ausgabe: %q", line)
		}
	}

	// Ein neuer Schlüssel ersetzt den Container
	other := "ffffffffffffffffffffffffffffffff"
	if err := manager.ConfigurePlayit(ctx, other); err != nil {
		t.Fatal(err)
	}
	if rt.count() != 1 || rt.find(playitContainer).spec.Env[0] != "SECRET_KEY="+other {
		t.Errorf("der alte Container muss ersetzt sein (%d Container)", rt.count())
	}

	// Nach einem Neustart des Daemons wird ein fehlender oder gestoppter Agent wieder gestartet
	_ = rt.Remove(ctx, playitContainer)
	if err := manager.EnsurePlayit(ctx); err != nil {
		t.Fatal(err)
	}
	if c := rt.find(playitContainer); c == nil || !c.running {
		t.Fatal("EnsurePlayit muss den Agenten wieder anlegen")
	}

	if err := manager.RemovePlayit(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.find(playitContainer) != nil || manager.PlayitStatus(ctx).Configured {
		t.Error("nach dem Entfernen darf nichts übrig sein")
	}
	if err := manager.EnsurePlayit(ctx); err != nil || rt.find(playitContainer) != nil {
		t.Errorf("ohne Schlüssel startet EnsurePlayit nichts: %v", err)
	}
}

func TestPlayitIsNotTreatedAsAServer(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	if err := manager.ConfigurePlayit(context.Background(), testSecret); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(manager.List()) != 0 {
		t.Fatal("der Agent ist kein Server")
	}
}
