package node

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, _ := writer.Create(name)
		_, _ = entry.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func put(t *testing.T, server *Server, name string, data []byte) {
	t.Helper()
	if dir := parentDir(name); dir != "" {
		_ = server.FS().Mkdir(dir)
	}
	if err := server.FS().Write(name, bytes.NewReader(data), 1<<26, true); err != nil {
		t.Fatal(err)
	}
}

func TestAddonsAreDescribedAndHashed(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Addons")
	luck := jar(t, map[string]string{"plugin.yml": "name: LuckPerms\nversion: '5.4.150'\nmain: me.lucko.Main\n"})
	put(t, server, "plugins/LuckPerms.jar", luck)
	put(t, server, "plugins/Proxy.jar.disabled", jar(t, map[string]string{"velocity-plugin.json": `{"id":"viaversion","name":"ViaVersion","version":"5.2.1"}`}))
	put(t, server, "mods/fabric-api.jar", jar(t, map[string]string{"fabric.mod.json": `{"id":"fabric-api","name":"Fabric API","version":"0.100.0"}`}))
	put(t, server, "mods/forge.jar", jar(t, map[string]string{"META-INF/mods.toml": "[[mods]]\nmodId=\"jei\"\nversion=\"1.2.3\"\ndisplayName=\"Just Enough Items\"\n"}))
	put(t, server, "plugins/kaputt.jar", []byte("kein zip"))
	put(t, server, "plugins/readme.txt", []byte("egal"))

	addons, err := server.Addons()
	if err != nil {
		t.Fatal(err)
	}
	byFile := map[string]Addon{}
	for _, addon := range addons {
		byFile[addon.File] = addon
	}
	if len(addons) != 5 {
		t.Fatalf("erwartet 5 Addons (ohne readme.txt), erhalten %d: %+v", len(addons), addons)
	}
	sum := sha1.Sum(luck)
	l := byFile["plugins/LuckPerms.jar"]
	if l.Name != "LuckPerms" || l.Version != "5.4.150" || l.Kind != "paper" || !l.Enabled || l.SHA1 != hex.EncodeToString(sum[:]) {
		t.Fatalf("LuckPerms: %+v", l)
	}
	if v := byFile["plugins/Proxy.jar.disabled"]; v.Kind != "velocity" || v.Name != "ViaVersion" || v.Enabled {
		t.Fatalf("Velocity-Plugin (deaktiviert): %+v", v)
	}
	if f := byFile["mods/fabric-api.jar"]; f.Kind != "fabric" || f.Version != "0.100.0" || f.ID != "fabric-api" {
		t.Fatalf("Fabric: %+v", f)
	}
	if f := byFile["mods/forge.jar"]; f.Kind != "forge" || f.Name != "Just Enough Items" || f.Version != "1.2.3" {
		t.Fatalf("Forge: %+v", f)
	}
	if k := byFile["plugins/kaputt.jar"]; k.Kind != "unknown" || k.Name != "kaputt" {
		t.Fatalf("defekte Jar: %+v", k)
	}
}

func TestPullDownloadsOnlyFromAllowedHostsAndPaths(t *testing.T) {
	content := jar(t, map[string]string{"plugin.yml": "name: Test\nversion: 1\n"})
	sum := sha1.Sum(content)
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(content) })
	mux.HandleFunc("/redirect.jar", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example.com/x.jar", http.StatusFound)
	})
	mux.HandleFunc("/missing.jar", http.NotFound)
	tls := httptest.NewTLSServer(mux)
	defer tls.Close()

	cfg := testConfig(t)
	cfg.PullHosts = []string{"127.0.0.1"}
	manager := newTestManager(t, cfg, newFakeRuntime())
	manager.pullClient = tls.Client()
	server := createPaper(t, manager, "Pull")
	ctx := context.Background()

	if err := manager.Pull(ctx, server, PullRequest{URL: tls.URL + "/ok.jar", Path: "plugins/Test.jar", SHA1: hex.EncodeToString(sum[:])}); err != nil {
		t.Fatal(err)
	}
	if data, _ := server.FS().Read("plugins/Test.jar", 1<<20); !bytes.Equal(data, content) {
		t.Fatal("Inhalt der geladenen Datei stimmt nicht")
	}
	if server.FS().Exists("plugins/Test.jar.lodestone-part") {
		t.Fatal("die Hilfsdatei muss verschwinden")
	}
	if err := manager.Pull(ctx, server, PullRequest{URL: tls.URL + "/ok.jar", Path: "plugins/Test.jar"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("vorhandene Datei ohne overwrite: %v", err)
	}
	if err := manager.Pull(ctx, server, PullRequest{URL: tls.URL + "/ok.jar", Path: "plugins/Test.jar", Overwrite: true}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	bad := []PullRequest{
		{URL: tls.URL + "/ok.jar", Path: "server.properties"},
		{URL: tls.URL + "/ok.jar", Path: "plugins/x.txt"},
		{URL: tls.URL + "/ok.jar", Path: "../x.jar"},
		{URL: tls.URL + "/ok.jar", Path: "plugins"},
		{URL: "http://127.0.0.1/ok.jar", Path: "plugins/a.jar"},                          // kein https
		{URL: "https://evil.example.com/ok.jar", Path: "plugins/a.jar"},                  // Host nicht erlaubt
		{URL: tls.URL + "/ok.jar", Path: "plugins/b.jar", SHA1: strings.Repeat("0", 40)}, // falsche Prüfsumme
		{URL: tls.URL + "/redirect.jar", Path: "plugins/c.jar"},                          // Weiterleitung auf fremden Host
		{URL: tls.URL + "/missing.jar", Path: "plugins/d.jar"},                           // 404
	}
	for i, request := range bad {
		if err := manager.Pull(ctx, server, request); err == nil {
			t.Errorf("Anfrage %d (%+v) wurde akzeptiert", i, request)
		}
	}
	for _, name := range []string{"plugins/a.jar", "plugins/b.jar", "plugins/c.jar", "plugins/d.jar", "plugins/b.jar.lodestone-part"} {
		if server.FS().Exists(name) {
			t.Errorf("%s darf nach einem Fehler nicht existieren", name)
		}
	}
}

// ---------------------------------------------------------------- Backups

func waitBackup(t *testing.T, server *Server, count int) []Backup {
	t.Helper()
	var list []Backup
	waitFor(t, "Backups fertig", func() bool {
		list, _ = server.ListBackups()
		if len(list) != count {
			return false
		}
		for _, backup := range list {
			if backup.Creating {
				return false
			}
		}
		return true
	})
	return list
}

func TestBackupCreateRestoreAndPrune(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Sicherung")
	put(t, server, "world/region/r.0.0.mca", []byte("welt-daten"))
	put(t, server, "server.properties", []byte("motd=alt\n"))

	backup, err := server.CreateBackup("Vor dem Update!")
	if err != nil || !backup.Creating || !strings.HasSuffix(backup.Name, "-vor-dem-update.tar.gz") {
		t.Fatalf("CreateBackup: %+v %v", backup, err)
	}
	if _, err := server.CreateBackup("noch eins"); err == nil {
		t.Fatal("zwei Backups gleichzeitig dürfen nicht möglich sein")
	}
	list := waitBackup(t, server, 1)
	if list[0].Size == 0 || list[0].Label != "vor-dem-update" {
		t.Fatalf("Backup-Eintrag: %+v", list[0])
	}

	// Danach ändern wir den Server …
	_ = server.FS().Write("server.properties", strings.NewReader("motd=neu\n"), 100, false)
	_ = server.FS().Remove("world", true)
	put(t, server, "extra.txt", []byte("neu dazugekommen"))

	// … und stellen das Backup wieder her
	if err := server.RestoreBackup(list[0].Name); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateOffline)
	if got := read(t, server.FS(), "server.properties"); got != "motd=alt\n" {
		t.Fatalf("server.properties = %q", got)
	}
	if got := read(t, server.FS(), "world/region/r.0.0.mca"); got != "welt-daten" {
		t.Fatalf("Welt = %q", got)
	}
	if server.FS().Exists("extra.txt") || server.FS().Exists(".lodestone-restore") {
		t.Fatal("neue Dateien und der Hilfsordner müssen weg sein")
	}
	if !server.FS().Exists("server.jar") {
		t.Fatal("server.jar muss aus dem Backup zurückkommen")
	}

	// Aufbewahrung: nur die neuesten BackupKeep bleiben (innerhalb einer Sekunde entscheidet der Name)
	if err := server.DeleteBackup(list[0].Name); err != nil {
		t.Fatal(err)
	}
	keep := 2
	if err := server.Update(Patch{BackupKeep: &keep}); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"p1", "p2", "p3"} {
		if _, err := server.CreateBackup(label); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "Backup fertig", func() bool { return !server.backupBusy.Load() })
	}
	list = waitBackup(t, server, 2)
	if list[0].Label != "p3" || list[1].Label != "p2" {
		t.Fatalf("das älteste Backup (p1) hätte gelöscht werden müssen: %+v", list)
	}
}

func TestBackupOfRunningServerPausesSaving(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Live")
	_ = server.Start()
	waitState(t, server, StateRunning)
	if _, err := server.CreateBackup(""); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, server, 1)
	waitFor(t, "save-on nach dem Backup", func() bool { return hasLine(server, "Befehl: save-on") })
	if !hasLine(server, "Befehl: save-off") || !hasLine(server, "Befehl: save-all flush") {
		t.Fatal("ein laufender Server muss vor dem Backup das Speichern anhalten")
	}
}

func TestRestoreRejectsMaliciousArchiveAndKeepsFiles(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Angriff")
	put(t, server, "wichtig.txt", []byte("bleibt"))

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(gz)
	for _, name := range []string{"ok.txt", "../ausbruch.txt"} {
		_ = archive.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 3, Typeflag: tar.TypeReg})
		_, _ = archive.Write([]byte("abc"))
	}
	archive.Close()
	gz.Close()
	dir := server.backupDir()
	_ = os.MkdirAll(dir, 0o755)
	name := "20260101-000000-boese.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, name), buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := server.RestoreBackup(name); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Wiederherstellung beendet", func() bool { return server.View().State == StateOffline && server.View().LastError != "" })
	if got := read(t, server.FS(), "wichtig.txt"); got != "bleibt" {
		t.Fatal("die bisherigen Dateien müssen nach einem Fehler unverändert sein")
	}
	if server.FS().Exists("ok.txt") || server.FS().Exists(".lodestone-restore") {
		t.Fatal("es darf nichts vom bösen Archiv übrig bleiben")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(server.dir), "ausbruch.txt")); err == nil {
		t.Fatal("das Archiv hat außerhalb des Serverordners geschrieben")
	}
	if _, err := os.Stat(filepath.Join(server.dir, "ausbruch.txt")); err == nil {
		t.Fatal("das Archiv hat außerhalb des Datenordners geschrieben")
	}
}

func TestBackupNamesAreValidated(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Namen")
	for _, name := range []string{"../../spec.json", "x.tar.gz", "20260101-000000.tar.gz/../../x", `..\a.tar.gz`} {
		if _, err := server.BackupFile(name); err == nil {
			t.Errorf("Name %q wurde akzeptiert", name)
		}
	}
}

func TestRestoreOnlyWhenOffline(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Laufend")
	_, _ = server.CreateBackup("x")
	list := waitBackup(t, server, 1)
	_ = server.Start()
	waitState(t, server, StateRunning)
	if err := server.RestoreBackup(list[0].Name); !errors.Is(err, ErrBadState) {
		t.Fatalf("Wiederherstellen eines laufenden Servers: %v", err)
	}
}

// ---------------------------------------------------------------- Spielerabfrage

func fakeStatusServer(t *testing.T, payload string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				buffer := make([]byte, 512)
				_, _ = conn.Read(buffer) // Handshake und Statusanfrage
				body := &bytesWriter{}
				writeVarInt(body, len(payload))
				body.data = append(body.data, payload...)
				_, _ = conn.Write(packet(0x00, body.data))
			}()
		}
	}()
	return listener.Addr().String()
}

func TestQueryStatus(t *testing.T) {
	address := fakeStatusServer(t, `{"version":{"name":"Paper 1.21.8","protocol":772},"players":{"max":20,"online":2,"sample":[{"name":"Steve","id":"1"},{"name":"Alex","id":"2"}]},"description":{"text":"Hallo ","extra":[{"text":"Welt"}]}}`)
	status, latency, err := queryStatus(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	if status.Players.Online != 2 || status.Players.Max != 20 || status.Version.Name != "Paper 1.21.8" || len(status.Players.Sample) != 2 || status.Players.Sample[1].Name != "Alex" {
		t.Fatalf("Status: %+v", status)
	}
	if motdText(status.Description) != "Hallo Welt" || latency < 0 {
		t.Fatalf("MOTD %q, Latenz %v", motdText(status.Description), latency)
	}
	if motdText([]byte(`"Nur Text"`)) != "Nur Text" {
		t.Fatal("einfacher MOTD-Text")
	}
	if _, _, err := queryStatus(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("ohne Server muss es einen Fehler geben")
	}
}

func TestVarIntRoundTrip(t *testing.T) {
	for _, value := range []int{0, 1, 127, 128, 255, 25565, 2097151, 765} {
		buffer := &bytesWriter{}
		writeVarInt(buffer, value)
		got, err := readVarInt(&byteReader{data: buffer.data})
		if err != nil || got != value {
			t.Fatalf("%d -> %d (%v)", value, got, err)
		}
	}
	_ = binary.BigEndian
}

func TestPlayersNeedsARunningServer(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Spieler")
	if _, err := server.Players(context.Background()); !errors.Is(err, ErrBadState) {
		t.Fatalf("Spieler eines gestoppten Servers: %v", err)
	}
}

func TestDeleteRemovesBackupsToo(t *testing.T) {
	manager := newTestManager(t, testConfig(t), newFakeRuntime())
	server := createPaper(t, manager, "Mit Backup")
	if _, err := server.CreateBackup("x"); err != nil {
		t.Fatal(err)
	}
	waitBackup(t, server, 1)
	dir := server.backupDir()
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete(server.View().ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Backup-Ordner muss mit dem Server verschwinden: %v", err)
	}
}
