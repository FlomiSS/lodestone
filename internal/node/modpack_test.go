package node

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type packFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       map[string]string `json:"env,omitempty"`
	Downloads []string          `json:"downloads"`
	FileSize  int               `json:"fileSize"`
}

func hashes(data []byte) map[string]string {
	one, five := sha1.Sum(data), sha512.Sum512(data)
	return map[string]string{"sha1": hex.EncodeToString(one[:]), "sha512": hex.EncodeToString(five[:])}
}

// modpackFixture baut einen Modpack-Server und liefert die Adressen der Dateien.
func modpackFixture(t *testing.T, files func(base string, mods map[string][]byte) []packFile, extras map[string]string) (server *Server, manager *Manager, packURL string) {
	t.Helper()
	mods := map[string][]byte{
		"/a.jar": jar(t, map[string]string{"fabric.mod.json": `{"id":"a","version":"1"}`}),
		"/b.jar": jar(t, map[string]string{"fabric.mod.json": `{"id":"b","version":"2"}`}),
		"/c.jar": []byte("nur fuer den Client"),
	}
	mux := http.NewServeMux()
	for name, data := range mods {
		data := data
		mux.HandleFunc(name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) })
	}
	tls := httptest.NewTLSServer(mux)
	t.Cleanup(tls.Close)

	index, _ := json.Marshal(map[string]any{
		"formatVersion": 1, "game": "minecraft", "versionId": "1.2.3", "name": "Testpack",
		"files":        files(tls.URL, mods),
		"dependencies": map[string]string{"minecraft": "1.21.4", "fabric-loader": "0.16.0"},
	})
	archive := map[string]string{"modrinth.index.json": string(index)}
	for name, content := range extras {
		archive[name] = content
	}
	pack := jar(t, archive)
	mux.HandleFunc("/pack.mrpack", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(pack) })

	cfg := testConfig(t)
	cfg.PullHosts = []string{"127.0.0.1"}
	manager = newTestManager(t, cfg, newFakeRuntime())
	manager.pullClient = tls.Client()
	return createPaper(t, manager, "Modpack"), manager, tls.URL + "/pack.mrpack"
}

func waitModpack(t *testing.T, server *Server) ModpackStatus {
	t.Helper()
	var status ModpackStatus
	waitFor(t, "Modpack fertig", func() bool {
		status = server.ModpackStatus()
		return status.State == "done" || status.State == "failed"
	})
	return status
}

func TestModpackWithEscapingPathsFails(t *testing.T) {
	server, manager, packURL := modpackFixture(t, func(base string, mods map[string][]byte) []packFile {
		return []packFile{
			{Path: "mods/a.jar", Hashes: hashes(mods["/a.jar"]), Downloads: []string{base + "/a.jar"}, FileSize: len(mods["/a.jar"]), Env: map[string]string{"client": "required", "server": "required"}},
			{Path: "mods/b.jar", Hashes: hashes(mods["/b.jar"]), Downloads: []string{"https://evil.example.com/b.jar", base + "/b.jar"}, FileSize: len(mods["/b.jar"])},
			{Path: "mods/client-only.jar", Hashes: hashes(mods["/c.jar"]), Downloads: []string{base + "/c.jar"}, Env: map[string]string{"client": "required", "server": "unsupported"}},
		}
	}, map[string]string{
		"overrides/config/pack/settings.json":    `{"a":1}`,
		"overrides/server.properties":            "motd=aus overrides\n",
		"server-overrides/server.properties":     "motd=aus server-overrides\n",
		"overrides/../../etc/evil":               "boese",
		"overrides/.lodestone-modpack.mrpack":    "boese",
		"overrides/config/../../../ausbruch.txt": "boese",
	})
	if err := manager.InstallModpack(server, ModpackRequest{URL: packURL}); err != nil {
		t.Fatal(err)
	}
	status := waitModpack(t, server)
	if status.State != "failed" || !strings.Contains(status.Error, "ungültiger Pfad") {
		t.Fatalf("ein Archiv mit Pfaden aus dem Serverordner heraus muss scheitern: %+v", status)
	}
	if server.FS().Exists("../ausbruch.txt") {
		t.Fatal("es darf nichts außerhalb des Serverordners entstehen")
	}
}

func TestModpackCleanInstall(t *testing.T) {
	server, manager, packURL := modpackFixture(t, func(base string, mods map[string][]byte) []packFile {
		return []packFile{
			{Path: "mods/a.jar", Hashes: hashes(mods["/a.jar"]), Downloads: []string{base + "/a.jar"}, FileSize: len(mods["/a.jar"]), Env: map[string]string{"client": "required", "server": "required"}},
			{Path: "mods/b.jar", Hashes: hashes(mods["/b.jar"]), Downloads: []string{"https://evil.example.com/b.jar", base + "/b.jar"}, FileSize: len(mods["/b.jar"])},
			{Path: "mods/client-only.jar", Hashes: hashes(mods["/c.jar"]), Downloads: []string{base + "/c.jar"}, Env: map[string]string{"client": "required", "server": "unsupported"}},
		}
	}, map[string]string{
		"overrides/config/pack/settings.json": `{"a":1}`,
		"overrides/server.properties":         "motd=aus overrides\n",
		"server-overrides/server.properties":  "motd=aus server-overrides\n",
	})

	if err := manager.InstallModpack(server, ModpackRequest{URL: packURL}); err != nil {
		t.Fatal(err)
	}
	// Während der Installation lässt sich der Server nicht starten, und eine zweite Installation geht nicht
	if err := manager.InstallModpack(server, ModpackRequest{URL: packURL}); err != nil && !errors.Is(err, ErrBadState) {
		t.Fatalf("zweite Installation: %v", err)
	}
	status := waitModpack(t, server)
	if status.State != "done" {
		t.Fatalf("Installation: %+v", status)
	}
	if status.Name != "Testpack" || status.Version != "1.2.3" || status.Minecraft != "1.21.4" || status.Loader != "fabric-loader" {
		t.Errorf("Angaben des Modpacks: %+v", status)
	}
	for _, name := range []string{"mods/a.jar", "mods/b.jar", "config/pack/settings.json"} {
		if !server.FS().Exists(name) {
			t.Errorf("%s fehlt", name)
		}
	}
	if server.FS().Exists("mods/client-only.jar") {
		t.Error("ein Mod nur für den Client gehört nicht auf den Server")
	}
	if got := read(t, server.FS(), "server.properties"); got != "motd=aus server-overrides\n" {
		t.Errorf("server-overrides müssen gewinnen: %q", got)
	}
	if server.FS().Exists(modpackTempName) {
		t.Error("das heruntergeladene Archiv muss weg sein")
	}
	if status.Done != status.Total || status.Total != 5 {
		t.Errorf("Fortschritt: %d von %d", status.Done, status.Total)
	}
}

func TestModpackRejectsBadChecksumAndHost(t *testing.T) {
	server, manager, packURL := modpackFixture(t, func(base string, mods map[string][]byte) []packFile {
		wrong := hashes([]byte("etwas anderes"))
		return []packFile{{Path: "mods/a.jar", Hashes: wrong, Downloads: []string{base + "/a.jar"}, FileSize: len(mods["/a.jar"])}}
	}, nil)
	if err := manager.InstallModpack(server, ModpackRequest{URL: "https://evil.example.com/pack.mrpack"}); err == nil {
		t.Fatal("von einem nicht erlaubten Host darf nichts geladen werden")
	}
	if err := manager.InstallModpack(server, ModpackRequest{URL: packURL}); err != nil {
		t.Fatal(err)
	}
	status := waitModpack(t, server)
	if status.State != "failed" || !strings.Contains(status.Error, "Prüfsumme") {
		t.Fatalf("falsche Prüfsumme: %+v", status)
	}
	if server.FS().Exists("mods/a.jar") || server.FS().Exists("mods/a.jar.lodestone-part") {
		t.Error("nach einem Prüfsummenfehler darf nichts liegen bleiben")
	}
	// Nach dem Fehler lässt sich der Server wieder starten
	if err := server.Start(); err != nil {
		t.Fatalf("Start nach fehlgeschlagener Installation: %v", err)
	}
}

func TestModpackNeedsIndexAndOfflineServer(t *testing.T) {
	server, manager, _ := modpackFixture(t, func(string, map[string][]byte) []packFile { return nil }, nil)
	// ein Archiv ohne modrinth.index.json
	mux := http.NewServeMux()
	mux.HandleFunc("/leer.mrpack", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jar(t, map[string]string{"irgendwas.txt": "x"}))
	})
	tls := httptest.NewTLSServer(mux)
	defer tls.Close()
	manager.pullClient = tls.Client()
	if err := manager.InstallModpack(server, ModpackRequest{URL: tls.URL + "/leer.mrpack"}); err != nil {
		t.Fatal(err)
	}
	if status := waitModpack(t, server); status.State != "failed" || !strings.Contains(status.Error, "modrinth.index.json") {
		t.Fatalf("Archiv ohne Index: %+v", status)
	}

	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	waitState(t, server, StateRunning)
	if err := manager.InstallModpack(server, ModpackRequest{URL: tls.URL + "/leer.mrpack"}); !errors.Is(err, ErrBadState) {
		t.Fatalf("auf einem laufenden Server darf nichts installiert werden: %v", err)
	}
}

func TestSafeModpackPath(t *testing.T) {
	good := map[string]string{"mods/a.jar": "mods/a.jar", "config\\x\\y.toml": "config/x/y.toml", "./mods//b.jar": "mods/b.jar"}
	for in, want := range good {
		if got, err := safeModpackPath(in); err != nil || got != want {
			t.Errorf("safeModpackPath(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "/etc/passwd", "../x", "mods/../../x", "C:/x", ".lodestone-modpack.mrpack", "spec.json", "a\x00b"} {
		if _, err := safeModpackPath(bad); err == nil {
			t.Errorf("%q hätte abgelehnt werden müssen", bad)
		}
	}
}

func TestStartRefusedWhenHostMemoryIsLow(t *testing.T) {
	cfg := testConfig(t)
	cfg.MemoryReserveMB = 1024
	manager := newTestManager(t, cfg, newFakeRuntime())
	server := createPaper(t, manager, "Knapp")
	manager.availableMemory = func() int64 { return 1500 << 20 } // 2048 MB + Reserve wären nötig
	if err := server.Start(); !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "Zu wenig freier Arbeitsspeicher") {
		t.Fatalf("Start bei knappem Speicher: %v", err)
	}
	manager.availableMemory = func() int64 { return 6000 << 20 }
	if err := server.Start(); err != nil {
		t.Fatalf("Start bei genug Speicher: %v", err)
	}
	waitState(t, server, StateRunning)
}
