package panel

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

// Rollen eines Servers im Projekt.
const (
	RoleStandalone = "standalone"
	RoleProxy      = "proxy"
	RoleBackend    = "backend"
)

const (
	voicePort      = 24454
	velocityReload = "velocity reload"
	// proxyPort ist der Port, auf dem Velocity im Container lauscht (die Standardwerte der Versionen unterscheiden sich,
	// deshalb steht er in der Autoconfig fest).
	proxyPort = 25565
)

// VelocityBind legt fest, auf welchem Port Velocity im Container lauscht – passend zur Port-Zuordnung des Nodes.
func VelocityBind() node.Edit {
	return node.Edit{File: "velocity.toml", Format: "toml", Set: []node.SetOp{{Path: []string{"bind"}, Value: fmt.Sprintf("0.0.0.0:%d", proxyPort)}}}
}

var slugClean = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify macht aus einem Namen den Kurznamen, unter dem der Server im Proxy steht.
func Slugify(name string) string {
	slug := strings.Trim(slugClean.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	return slug
}

// IsPaperFamily sagt, ob die Software ihre Proxy-Einstellung in config/paper-global.yml hat.
func IsPaperFamily(id software.ID) bool {
	switch id {
	case "paper", "purpur", "folia", "leaf":
		return true
	}
	return false
}

// CanBeBackend sagt, ob ein Server hinter Velocity laufen kann (er muss "modern forwarding" verstehen). Fabric und NeoForge
// bekommen das über einen Mod, den Lodestone installiert (siehe ProxyMod).
func CanBeBackend(id software.ID) bool {
	if pack, ok := software.GetPack(id); ok {
		if pack.Meta.Backend.Supported || (pack.Autoconfig.Backend != nil && len(pack.Autoconfig.Backend.Edits) > 0) {
			return true
		}
	}
	return IsPaperFamily(id) || ProxyMod(id) != ""
}

// ProxyMod nennt das Modrinth-Projekt, das einen Mod-Server "modern forwarding" von Velocity beibringt ("" = nicht nötig
// oder nicht möglich).
func ProxyMod(id software.ID) string {
	if pack, ok := software.GetPack(id); ok && pack.Meta.Backend.ProxyMod != "" {
		return pack.Meta.Backend.ProxyMod
	}
	switch id {
	case "fabric", "quilt":
		return "fabricproxy-lite"
	case "neoforge":
		return "proxy-compatible-forge"
	}
	return ""
}

// VoiceConfigFile ist die Einstellungsdatei von Simple Voice Chat bei dieser Software ("" = Voicechat nicht unterstützt).
func VoiceConfigFile(id software.ID) string {
	if pack, ok := software.GetPack(id); ok && pack.Meta.Backend.VoiceConfigFile != "" {
		return pack.Meta.Backend.VoiceConfigFile
	}
	switch {
	case IsPaperFamily(id):
		return "plugins/voicechat/voicechat-server.properties"
	case id == "fabric", id == "neoforge", id == "quilt":
		return "config/voicechat/voicechat-server.properties"
	}
	return ""
}

// NetServer sind die Angaben, die die Rezepte über einen Server des Netzwerks brauchen.
type NetServer struct {
	Row ServerRow
	// Port im Container, unter dem der Server im Docker-Netzwerk antwortet.
	InternalPort int
	// PublicHost ist die Adresse der Maschine, auf der der Server läuft.
	PublicHost string
	// VoiceHostPort ist der veröffentlichte UDP-Port für Voicechat (0, wenn es keinen gibt).
	VoiceHostPort int
	// VoiceAddress ersetzt PublicHost:VoiceHostPort als Adresse des Voicechats (z. B. ein playit-Tunnel).
	VoiceAddress string
	// MOTD ist die Nachricht in der Serverliste (nur am Proxy; leer = Standard von Velocity).
	MOTD string
}

// voiceHost ist die Adresse, die Spieler für den Voicechat benutzen (leer, wenn es keine gibt).
func (s NetServer) voiceHost() string {
	switch {
	case s.VoiceAddress != "":
		return s.VoiceAddress
	case s.VoiceHostPort > 0 && s.PublicHost != "":
		return s.PublicHost + ":" + strconv.Itoa(s.VoiceHostPort)
	}
	return ""
}

func (s NetServer) internalAddress() string {
	port := s.InternalPort
	if port == 0 {
		port = 25565
	}
	return fmt.Sprintf("lodestone-%s:%d", s.Row.ID, port)
}

// BuildAutoconfigs berechnet für jeden Server eines Netzwerks die Dateiänderungen. Der Schlüssel der Antwort ist die
// Server-ID. secret ist das gemeinsame Forwarding-Geheimnis des Projekts.
func BuildAutoconfigs(secret string, servers []NetServer, voiceProxy bool) map[string]node.Autoconfig {
	result := map[string]node.Autoconfig{}

	// Reihenfolge der Backends: erst die im Try, nach Position, dann der Rest nach Name
	backends := []NetServer{}
	for _, server := range servers {
		if server.Row.Role == RoleBackend {
			backends = append(backends, server)
		}
	}
	sort.SliceStable(backends, func(i, j int) bool {
		a, b := backends[i].Row, backends[j].Row
		if a.TryPosition != b.TryPosition {
			return a.TryPosition < b.TryPosition
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	for _, server := range servers {
		switch server.Row.Role {
		case RoleProxy:
			result[server.Row.ID] = proxyAutoconfig(secret, backends, server, voiceProxy)
		case RoleBackend:
			if config, ok := backendAutoconfig(secret, server, voiceProxy); ok {
				result[server.Row.ID] = config
			}
		}
	}
	return result
}

func proxyAutoconfig(secret string, backends []NetServer, proxy NetServer, voiceProxy bool) node.Autoconfig {
	table := map[string]any{}
	try := []string{}
	for _, backend := range backends {
		table[backend.Row.Slug] = backend.internalAddress()
		if backend.Row.InTry {
			try = append(try, backend.Row.Slug)
		}
	}
	table["try"] = try

	forced := map[string]any{}
	for _, backend := range backends {
		host := strings.ToLower(strings.TrimSpace(backend.Row.ForcedHost))
		if host == "" {
			continue
		}
		existing, _ := forced[host].([]string)
		forced[host] = append(existing, backend.Row.Slug)
	}

	var edits []node.Edit
	var writes []node.FileWrite
	restart := true
	reload := velocityReload

	pack, hasPack := software.GetPack(software.ID(proxy.Row.Software))
	if hasPack && pack.Autoconfig.Proxy != nil {
		target := pack.Autoconfig.Proxy
		restart = target.Restart
		if target.Reload != "" {
			reload = interpolateString(target.Reload, secret, proxy)
		}
		for _, pe := range target.Edits {
			edit := node.Edit{
				File:   interpolateString(pe.File, secret, proxy),
				Format: pe.Format,
				Create: pe.Create,
			}
			for _, op := range pe.Set {
				edit.Set = append(edit.Set, node.SetOp{
					Path:  op.Path,
					Value: interpolateValue(op.Value, secret, proxy),
				})
			}
			if edit.File == "velocity.toml" {
				edit.Set = append(edit.Set,
					node.SetOp{Path: []string{"servers"}, Value: table},
					node.SetOp{Path: []string{"forced-hosts"}, Value: forced},
				)
				if proxy.MOTD != "" {
					edit.Set = append(edit.Set, node.SetOp{Path: []string{"motd"}, Value: proxy.MOTD})
				}
			}
			edits = append(edits, edit)
		}
		for _, pw := range target.Writes {
			writes = append(writes, node.FileWrite{
				Path:    interpolateString(pw.Path, secret, proxy),
				Content: interpolateString(pw.Content, secret, proxy),
			})
		}
	} else {
		defaultEdit := node.Edit{
			File:   "velocity.toml",
			Format: "toml",
			Set: []node.SetOp{
				{Path: []string{"bind"}, Value: fmt.Sprintf("0.0.0.0:%d", proxyPort)},
				{Path: []string{"player-info-forwarding-mode"}, Value: "modern"},
				{Path: []string{"servers"}, Value: table},
				{Path: []string{"forced-hosts"}, Value: forced},
			},
		}
		if proxy.MOTD != "" {
			defaultEdit.Set = append(defaultEdit.Set, node.SetOp{Path: []string{"motd"}, Value: proxy.MOTD})
		}
		edits = append(edits, defaultEdit)
		writes = append(writes, node.FileWrite{Path: "forwarding.secret", Content: secret})
	}

	if voice := proxy.voiceHost(); voiceProxy && voice != "" {
		// Der Proxy nimmt den Voicechat aller Server über einen UDP-Port an und reicht ihn weiter
		edits = append(edits, node.Edit{
			File:   "plugins/voicechat/voicechat-proxy.properties",
			Format: "properties",
			Create: true,
			Set: []node.SetOp{
				{Path: []string{"port"}, Value: -1},
				{Path: []string{"voice_host"}, Value: voice},
			},
		})
	}
	return node.Autoconfig{
		Edits:   edits,
		Writes:  writes,
		Restart: restart,
		Reload:  reload,
	}
}

func backendAutoconfig(secret string, server NetServer, voiceProxy bool) (node.Autoconfig, bool) {
	if pack, ok := software.GetPack(software.ID(server.Row.Software)); ok && pack.Autoconfig.Backend != nil && len(pack.Autoconfig.Backend.Edits) > 0 {
		return buildPackBackendAutoconfig(pack, secret, server, voiceProxy)
	}
	edits := []node.Edit{{
		File:   "server.properties",
		Format: "properties",
		Set:    []node.SetOp{{Path: []string{"online-mode"}, Value: false}},
	}}
	voiceFile := VoiceConfigFile(software.ID(server.Row.Software))
	switch {
	case IsPaperFamily(software.ID(server.Row.Software)):
		edits = append(edits, node.Edit{
			File:   "config/paper-global.yml",
			Format: "yaml",
			Set: []node.SetOp{
				{Path: []string{"proxies", "velocity", "enabled"}, Value: true},
				{Path: []string{"proxies", "velocity", "online-mode"}, Value: true},
				{Path: []string{"proxies", "velocity", "secret"}, Value: secret},
			},
		})
	case server.Row.Software == "fabric":
		edits = append(edits, node.Edit{
			File:   "config/FabricProxy-Lite.toml",
			Format: "toml",
			Create: true,
			Set: []node.SetOp{
				{Path: []string{"hackOnlineMode"}, Value: true},
				{Path: []string{"hackEarlySend"}, Value: false},
				{Path: []string{"hackMessageChain"}, Value: true},
				{Path: []string{"disconnectMessage"}, Value: "Dieser Server ist nur über den Proxy erreichbar."},
				{Path: []string{"secret"}, Value: secret},
			},
		})
	case server.Row.Software == "neoforge":
		// Proxy Compatible Forge liest die Datei beim Start; alle drei Werte stehen drin, damit nichts vom Standard abhängt
		edits = append(edits, node.Edit{
			File:   "config/proxy-compatible-forge.toml",
			Format: "toml",
			Create: true,
			Set: []node.SetOp{
				{Path: []string{"forwarding", "enabled"}, Value: true},
				{Path: []string{"forwarding", "mode"}, Value: "MODERN"},
				{Path: []string{"forwarding", "secret"}, Value: secret},
			},
		})
	default:
		return node.Autoconfig{}, false
	}

	if voice := server.voiceHost(); !voiceProxy && server.Row.Voicechat && voice != "" {
		edits = append(edits, node.Edit{
			File:   voiceFile,
			Format: "properties",
			Set: []node.SetOp{
				{Path: []string{"port"}, Value: voicePort},
				{Path: []string{"voice_host"}, Value: voice},
			},
		})
	}
	return node.Autoconfig{Edits: edits, Restart: true}, true
}

func interpolateString(s, secret string, server NetServer) string {
	repl := strings.NewReplacer(
		"{{SECRET}}", secret,
		"{{VOICE_HOST}}", server.voiceHost(),
		"{{VOICE_PORT}}", strconv.Itoa(voicePort),
		"{{PROXY_PORT}}", strconv.Itoa(proxyPort),
		"{{SERVER_NAME}}", server.Row.Name,
		"{{SERVER_SLUG}}", server.Row.Slug,
		"{{INTERNAL_PORT}}", strconv.Itoa(server.InternalPort),
	)
	return repl.Replace(s)
}

func interpolateValue(val any, secret string, server NetServer) any {
	switch v := val.(type) {
	case string:
		return interpolateString(v, secret, server)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = interpolateValue(item, secret, server)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = interpolateValue(item, secret, server)
		}
		return out
	default:
		return val
	}
}

func buildPackBackendAutoconfig(pack *software.PositionPack, secret string, server NetServer, voiceProxy bool) (node.Autoconfig, bool) {
	target := pack.Autoconfig.Backend
	if target == nil {
		return node.Autoconfig{}, false
	}
	var edits []node.Edit
	for _, pe := range target.Edits {
		edit := node.Edit{
			File:   interpolateString(pe.File, secret, server),
			Format: pe.Format,
			Create: pe.Create,
		}
		for _, op := range pe.Set {
			path := make([]string, len(op.Path))
			for i, p := range op.Path {
				path[i] = interpolateString(p, secret, server)
			}
			edit.Set = append(edit.Set, node.SetOp{
				Path:  path,
				Value: interpolateValue(op.Value, secret, server),
			})
		}
		edits = append(edits, edit)
	}

	voice := server.voiceHost()
	if !voiceProxy && server.Row.Voicechat && voice != "" {
		if pack.Autoconfig.Voicechat != nil {
			pv := pack.Autoconfig.Voicechat
			vEdit := node.Edit{
				File:   interpolateString(pv.File, secret, server),
				Format: pv.Format,
			}
			for _, op := range pv.Set {
				path := make([]string, len(op.Path))
				for i, p := range op.Path {
					path[i] = interpolateString(p, secret, server)
				}
				vEdit.Set = append(vEdit.Set, node.SetOp{
					Path:  path,
					Value: interpolateValue(op.Value, secret, server),
				})
			}
			edits = append(edits, vEdit)
		} else if voiceFile := VoiceConfigFile(software.ID(server.Row.Software)); voiceFile != "" {
			edits = append(edits, node.Edit{
				File:   voiceFile,
				Format: "properties",
				Set: []node.SetOp{
					{Path: []string{"port"}, Value: voicePort},
					{Path: []string{"voice_host"}, Value: voice},
				},
			})
		}
	}

	var writes []node.FileWrite
	for _, pw := range target.Writes {
		writes = append(writes, node.FileWrite{
			Path:    interpolateString(pw.Path, secret, server),
			Content: interpolateString(pw.Content, secret, server),
		})
	}
	if strings.TrimSpace(pack.Script) != "" {
		writes = append(writes, node.FileWrite{
			Path:    "autoconfig.sh",
			Content: interpolateString(pack.Script, secret, server),
		})
	}

	return node.Autoconfig{
		Edits:   edits,
		Writes:  writes,
		Restart: target.Restart,
		Reload:  interpolateString(target.Reload, secret, server),
	}, true
}

// NetworkProblems prüft die Zusammenstellung eines Netzwerks und liefert Hinweise für die Oberfläche.
func NetworkProblems(servers []NetServer) []string {
	var problems []string
	proxies, backends, tried := 0, 0, 0
	for _, server := range servers {
		switch server.Row.Role {
		case RoleProxy:
			proxies++
		case RoleBackend:
			backends++
			if server.Row.InTry {
				tried++
			}
		}
	}
	if proxies == 0 {
		problems = append(problems, "Das Netzwerk hat noch keinen Proxy (Velocity).")
	}
	if backends == 0 {
		problems = append(problems, "Es gibt noch keinen Server hinter dem Proxy.")
	}
	if backends > 0 && tried == 0 {
		problems = append(problems, "Kein Server steht in der Beitrittsreihenfolge – Spieler kommen nirgends hin.")
	}
	return problems
}
