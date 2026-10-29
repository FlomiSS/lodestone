package node

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func write(t *testing.T, files *FS, name, content string) {
	t.Helper()
	if dir := parentDir(name); dir != "" {
		if err := files.Mkdir(dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeFile(files, name, content); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, files *FS, name string) string {
	t.Helper()
	data, err := files.Read(name, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditProperties(t *testing.T) {
	files, _ := newFS(t)
	write(t, files, "server.properties", "#Minecraft server properties\r\nmotd=Hallo\r\nonline-mode=true\r\nserver-port=25565\r\n")
	edit := Edit{File: "server.properties", Format: "properties", Set: []SetOp{
		{Path: []string{"online-mode"}, Value: false},
		{Path: []string{"max-players"}, Value: float64(20)}, // JSON liefert Zahlen als float64
	}}
	changed, missing, err := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{edit}})
	if err != nil || !changed || len(missing) != 0 {
		t.Fatalf("anwenden: %v %v %v", changed, missing, err)
	}
	got := read(t, files, "server.properties")
	if got != "#Minecraft server properties\r\nmotd=Hallo\r\nonline-mode=false\r\nserver-port=25565\r\nmax-players=20\r\n" {
		t.Fatalf("Properties: %q", got)
	}
	if changed, _, _ := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{edit}}); changed {
		t.Fatal("ein zweites Anwenden darf nichts mehr ändern")
	}
	_, _, _ = ApplyAutoconfig(files, Autoconfig{Edits: []Edit{{File: "server.properties", Format: "properties", Delete: [][]string{{"motd"}}}}})
	if strings.Contains(read(t, files, "server.properties"), "motd") {
		t.Fatal("motd wurde nicht entfernt")
	}
}

func TestEditYAMLKeepsCommentsAndOtherValues(t *testing.T) {
	files, _ := newFS(t)
	write(t, files, "config/paper-global.yml", `# Paper Global Configuration
_version: 29
proxies:
  # Velocity
  velocity:
    enabled: false
    online-mode: false
    secret: ''
  bungee-cord:
    online-mode: true
timings:
  enabled: true
`)
	edit := Edit{File: "config/paper-global.yml", Format: "yaml", Set: []SetOp{
		{Path: []string{"proxies", "velocity", "enabled"}, Value: true},
		{Path: []string{"proxies", "velocity", "online-mode"}, Value: true},
		{Path: []string{"proxies", "velocity", "secret"}, Value: "geheim123"},
	}}
	changed, _, err := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{edit}})
	if err != nil || !changed {
		t.Fatalf("anwenden: %v %v", changed, err)
	}
	got := read(t, files, "config/paper-global.yml")
	var parsed struct {
		Version int `yaml:"_version"`
		Proxies struct {
			Velocity struct {
				Enabled    bool   `yaml:"enabled"`
				OnlineMode bool   `yaml:"online-mode"`
				Secret     string `yaml:"secret"`
			} `yaml:"velocity"`
			Bungee struct {
				OnlineMode bool `yaml:"online-mode"`
			} `yaml:"bungee-cord"`
		} `yaml:"proxies"`
		Timings struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"timings"`
	}
	if err := yaml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("Ergebnis ist kein YAML: %v\n%s", err, got)
	}
	if parsed.Version != 29 || !parsed.Proxies.Velocity.Enabled || !parsed.Proxies.Velocity.OnlineMode || parsed.Proxies.Velocity.Secret != "geheim123" || !parsed.Proxies.Bungee.OnlineMode || !parsed.Timings.Enabled {
		t.Fatalf("Werte: %+v\n%s", parsed, got)
	}
	if !strings.Contains(got, "# Paper Global Configuration") || !strings.Contains(got, "# Velocity") {
		t.Fatalf("Kommentare gingen verloren:\n%s", got)
	}
	if changed, _, _ := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{edit}}); changed {
		t.Fatal("ein zweites Anwenden darf nichts mehr ändern")
	}
}

func TestEditYAMLCreatesMissingTables(t *testing.T) {
	files, _ := newFS(t)
	write(t, files, "x.yml", "a: 1\n")
	_, _, err := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{{File: "x.yml", Format: "yaml", Set: []SetOp{{Path: []string{"b", "c", "d"}, Value: []any{"x", "y"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	_ = yaml.Unmarshal([]byte(read(t, files, "x.yml")), &parsed)
	if got := parsed["b"].(map[string]any)["c"].(map[string]any)["d"].([]any); len(got) != 2 || got[0] != "x" {
		t.Fatalf("verschachtelte Tabelle: %v", parsed)
	}
}

func TestEditTOMLForVelocity(t *testing.T) {
	files, _ := newFS(t)
	write(t, files, "velocity.toml", `config-version = "2.7"
bind = "0.0.0.0:25577"
player-info-forwarding-mode = "none"
forwarding-secret-file = "forwarding.secret"

[servers]
lobby = "127.0.0.1:30066"
try = ["lobby"]

[forced-hosts]
"lobby.example.com" = ["lobby"]

[advanced]
compression-threshold = 256
`)
	edit := Edit{File: "velocity.toml", Format: "toml", Set: []SetOp{
		{Path: []string{"player-info-forwarding-mode"}, Value: "modern"},
		{Path: []string{"servers"}, Value: map[string]any{"lobby": "lodestone-aaa:25565", "spiele": "lodestone-bbb:25565", "try": []any{"lobby", "spiele"}}},
		{Path: []string{"forced-hosts"}, Value: map[string]any{"spiele.example.com": []any{"spiele"}}},
		{Path: []string{"advanced", "compression-threshold"}, Value: float64(256)},
	}}
	changed, _, err := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{edit}})
	if err != nil || !changed {
		t.Fatalf("anwenden: %v %v", changed, err)
	}
	var parsed struct {
		Mode    string `toml:"player-info-forwarding-mode"`
		Bind    string `toml:"bind"`
		Servers struct {
			Lobby  string   `toml:"lobby"`
			Spiele string   `toml:"spiele"`
			Try    []string `toml:"try"`
		} `toml:"servers"`
		Forced   map[string][]string `toml:"forced-hosts"`
		Advanced struct {
			Compression int64 `toml:"compression-threshold"`
		} `toml:"advanced"`
	}
	got := read(t, files, "velocity.toml")
	if err := toml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("kein TOML: %v\n%s", err, got)
	}
	if parsed.Mode != "modern" || parsed.Bind != "0.0.0.0:25577" || parsed.Servers.Lobby != "lodestone-aaa:25565" || len(parsed.Servers.Try) != 2 || parsed.Servers.Try[1] != "spiele" {
		t.Fatalf("Werte: %+v\n%s", parsed, got)
	}
	if len(parsed.Forced) != 1 || parsed.Forced["spiele.example.com"][0] != "spiele" || parsed.Advanced.Compression != 256 {
		t.Fatalf("Forced-Hosts/Advanced: %+v\n%s", parsed, got)
	}
	if changed, _, _ := ApplyAutoconfig(files, Autoconfig{Edits: []Edit{edit}}); changed {
		t.Fatal("ein zweites Anwenden darf nichts mehr ändern")
	}
}

func TestMissingFilesStayPendingAndCreateWorks(t *testing.T) {
	files, _ := newFS(t)
	changed, missing, err := ApplyAutoconfig(files, Autoconfig{
		Edits: []Edit{
			{File: "velocity.toml", Format: "toml", Set: []SetOp{{Path: []string{"a"}, Value: 1}}},
			{File: "plugins/voicechat/voicechat-server.properties", Format: "properties", Create: true, Set: []SetOp{{Path: []string{"port"}, Value: 24454}}},
		},
		Writes: []FileWrite{{Path: "forwarding.secret", Content: "s3cret"}},
	})
	if err != nil || !changed {
		t.Fatalf("anwenden: %v %v", changed, err)
	}
	if len(missing) != 1 || missing[0] != "velocity.toml" {
		t.Fatalf("fehlende Dateien: %v", missing)
	}
	if got := read(t, files, "plugins/voicechat/voicechat-server.properties"); got != "port=24454\n" {
		t.Fatalf("angelegte Datei: %q", got)
	}
	if read(t, files, "forwarding.secret") != "s3cret" {
		t.Fatal("forwarding.secret fehlt")
	}
	if _, err := files.Read("velocity.toml", 10); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("eine fehlende Datei darf nicht angelegt werden")
	}
}

func TestAutoconfigValidation(t *testing.T) {
	bad := []Autoconfig{
		{Edits: []Edit{{File: "", Format: "yaml"}}},
		{Edits: []Edit{{File: "a", Format: "ini"}}},
		{Edits: []Edit{{File: "a", Format: "properties", Set: []SetOp{{Path: []string{"a", "b"}}}}}},
		{Edits: []Edit{{File: "a", Format: "yaml", Set: []SetOp{{Path: nil}}}}},
		{Writes: []FileWrite{{Path: "", Content: "x"}}},
		{Reload: "a\nb"},
	}
	for i, config := range bad {
		if err := config.Validate(); err == nil {
			t.Errorf("ungültiger Satz %d wurde akzeptiert", i)
		}
	}
	ok := Autoconfig{Edits: []Edit{{File: "a.yml", Format: "yaml", Set: []SetOp{{Path: []string{"a"}, Value: 1}}}}, Reload: "velocity reload"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAutoconfigIsAppliedWhenReadyAndRestartsOnce(t *testing.T) {
	rt := newFakeRuntime()
	manager := newTestManager(t, testConfig(t), rt)
	server := createPaper(t, manager, "Config")

	// Die Datei gibt es noch nicht: der Satz wartet
	result, err := server.SetAutoconfig(Autoconfig{Restart: true, Edits: []Edit{{File: "config/paper-global.yml", Format: "yaml", Set: []SetOp{{Path: []string{"proxies", "velocity", "enabled"}, Value: true}}}}})
	if err != nil || !result.Pending || len(result.Missing) != 1 {
		t.Fatalf("SetAutoconfig: %+v %v", result, err)
	}
	if !server.View().AutoconfigPending {
		t.Fatal("der Satz muss als offen gelten")
	}

	_ = server.Start()
	waitState(t, server, StateRunning)
	// Der Server hat beim Start die Datei angelegt
	write(t, server.FS(), "config/paper-global.yml", "_version: 29\nproxies:\n  velocity:\n    enabled: false\n")
	if res := server.tryApply(true); res.Pending || !res.Changed {
		t.Fatalf("tryApply: %+v", res)
	}
	if server.View().AutoconfigPending {
		t.Fatal("der Satz muss angewendet sein")
	}
	if !strings.Contains(read(t, server.FS(), "config/paper-global.yml"), "enabled: true") {
		t.Fatal("die Änderung fehlt in der Datei")
	}
	waitFor(t, "einmaliger Neustart", func() bool {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		c := rt.containers["c2"]
		return c != nil && c.running && server.View().State == StateRunning
	})
	// Ein erneutes Setzen derselben Werte ändert nichts und startet nicht neu
	again, _ := server.SetAutoconfig(Autoconfig{Restart: true, Edits: []Edit{{File: "config/paper-global.yml", Format: "yaml", Set: []SetOp{{Path: []string{"proxies", "velocity", "enabled"}, Value: true}}}}})
	if again.Changed || again.Pending {
		t.Fatalf("erneutes Setzen: %+v", again)
	}
	if rt.count() != 1 && rt.containers["c3"] != nil {
		t.Fatal("es darf kein weiterer Neustart folgen")
	}
}

func TestAutoconfigSurvivesRestartOfTheDaemon(t *testing.T) {
	cfg := testConfig(t)
	rt := newFakeRuntime()
	first := newTestManager(t, cfg, rt)
	server := createPaper(t, first, "Merken")
	_, _ = server.SetAutoconfig(Autoconfig{Edits: []Edit{{File: "x.yml", Format: "yaml", Set: []SetOp{{Path: []string{"a"}, Value: 1}}}}})
	second := newTestManager(t, cfg, rt)
	again, _ := second.Get(server.View().ID)
	if view := again.View(); view.Autoconfig == nil || view.Autoconfig.Version != 1 || !view.AutoconfigPending {
		t.Fatalf("der Satz muss gespeichert sein: %+v", view.Autoconfig)
	}
}
