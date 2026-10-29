package panel

import (
	"testing"

	"github.com/lodestone-mc/lodestone/internal/node"
)

func editFor(t *testing.T, config node.Autoconfig, file string) node.Edit {
	t.Helper()
	for _, edit := range config.Edits {
		if edit.File == file {
			return edit
		}
	}
	t.Fatalf("keine Änderung an %s in %+v", file, config.Edits)
	return node.Edit{}
}

func setValue(t *testing.T, edit node.Edit, path ...string) any {
	t.Helper()
	for _, op := range edit.Set {
		if len(op.Path) == len(path) {
			same := true
			for i := range path {
				same = same && op.Path[i] == path[i]
			}
			if same {
				return op.Value
			}
		}
	}
	t.Fatalf("kein Wert für %v in %s", path, edit.File)
	return nil
}

func TestBuildAutoconfigs(t *testing.T) {
	servers := []NetServer{
		{Row: ServerRow{ID: "aaa", Name: "Proxy", Slug: "proxy", Role: RoleProxy, Software: "velocity"}, InternalPort: 25565},
		{Row: ServerRow{ID: "bbb", Name: "Survival", Slug: "survival", Role: RoleBackend, Software: "paper", InTry: false, TryPosition: 1, ForcedHost: "survival.example.org", Voicechat: true}, VoiceHostPort: 28005, PublicHost: "mc.example.org"},
		{Row: ServerRow{ID: "ccc", Name: "Lobby", Slug: "lobby", Role: RoleBackend, Software: "fabric", InTry: true, TryPosition: 0}},
	}
	configs := BuildAutoconfigs("geheim", servers, false)

	proxy := configs["aaa"]
	velocity := editFor(t, proxy, "velocity.toml")
	if got := setValue(t, velocity, "player-info-forwarding-mode"); got != "modern" {
		t.Errorf("Forwarding-Modus = %v", got)
	}
	table := setValue(t, velocity, "servers").(map[string]any)
	if table["lobby"] != "lodestone-ccc:25565" || table["survival"] != "lodestone-bbb:25565" {
		t.Errorf("Serverliste = %v", table)
	}
	try := table["try"].([]string)
	if len(try) != 1 || try[0] != "lobby" {
		t.Errorf("try = %v", try)
	}
	forced := setValue(t, velocity, "forced-hosts").(map[string]any)
	if hosts := forced["survival.example.org"].([]string); len(hosts) != 1 || hosts[0] != "survival" {
		t.Errorf("forced-hosts = %v", forced)
	}
	if len(proxy.Writes) != 1 || proxy.Writes[0].Path != "forwarding.secret" || proxy.Writes[0].Content != "geheim" {
		t.Errorf("forwarding.secret = %+v", proxy.Writes)
	}
	if proxy.Reload != "velocity reload" || !proxy.Restart {
		t.Errorf("Neu laden/Neustart = %q %v", proxy.Reload, proxy.Restart)
	}

	paper := configs["bbb"]
	if got := setValue(t, editFor(t, paper, "server.properties"), "online-mode"); got != false {
		t.Errorf("online-mode = %v", got)
	}
	global := editFor(t, paper, "config/paper-global.yml")
	if setValue(t, global, "proxies", "velocity", "secret") != "geheim" || setValue(t, global, "proxies", "velocity", "enabled") != true {
		t.Errorf("paper-global.yml = %+v", global)
	}
	voice := editFor(t, paper, "plugins/voicechat/voicechat-server.properties")
	if got := setValue(t, voice, "voice_host"); got != "mc.example.org:28005" {
		t.Errorf("voice_host = %v", got)
	}

	fabric := configs["ccc"]
	lite := editFor(t, fabric, "config/FabricProxy-Lite.toml")
	if !lite.Create || setValue(t, lite, "secret") != "geheim" {
		t.Errorf("FabricProxy-Lite.toml = %+v", lite)
	}
	for _, edit := range fabric.Edits {
		if edit.File == "config/voicechat/voicechat-server.properties" {
			t.Error("Voicechat ist für Lobby nicht eingeschaltet")
		}
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"Survival 1": "survival-1", "  Lobby!! ": "lobby", "ÄÖ": "", "a_b": "a-b"} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, will %q", in, got, want)
		}
	}
}

func TestNetworkProblems(t *testing.T) {
	if len(NetworkProblems(nil)) != 2 {
		t.Errorf("leeres Netzwerk: %v", NetworkProblems(nil))
	}
	ok := []NetServer{{Row: ServerRow{Role: RoleProxy}}, {Row: ServerRow{Role: RoleBackend, InTry: true}}}
	if problems := NetworkProblems(ok); len(problems) != 0 {
		t.Errorf("vollständiges Netzwerk: %v", problems)
	}
}
