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
	case "paper", "purpur", "folia":
		return true
	}
	return false
}

// CanBeBackend sagt, ob ein Server hinter Velocity laufen kann (er muss "modern forwarding" verstehen).
func CanBeBackend(id software.ID) bool { return IsPaperFamily(id) || id == "fabric" }

// NetServer sind die Angaben, die die Rezepte über einen Server des Netzwerks brauchen.
type NetServer struct {
	Row ServerRow
	// Port im Container, unter dem der Server im Docker-Netzwerk antwortet.
	InternalPort int
	// PublicHost ist die Adresse der Maschine, auf der der Server läuft.
	PublicHost string
	// VoiceHostPort ist der veröffentlichte UDP-Port für Voicechat (0, wenn es keinen gibt).
	VoiceHostPort int
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

	edits := []node.Edit{{
		File:   "velocity.toml",
		Format: "toml",
		Set: []node.SetOp{
			{Path: []string{"bind"}, Value: fmt.Sprintf("0.0.0.0:%d", proxyPort)},
			{Path: []string{"player-info-forwarding-mode"}, Value: "modern"},
			{Path: []string{"servers"}, Value: table},
			{Path: []string{"forced-hosts"}, Value: forced},
		},
	}}
	if voiceProxy && proxy.VoiceHostPort > 0 && proxy.PublicHost != "" {
		// Der Proxy nimmt den Voicechat aller Server über einen UDP-Port an und reicht ihn weiter
		edits = append(edits, node.Edit{
			File:   "plugins/voicechat/voicechat-proxy.properties",
			Format: "properties",
			Create: true,
			Set: []node.SetOp{
				{Path: []string{"port"}, Value: -1},
				{Path: []string{"voice_host"}, Value: proxy.PublicHost + ":" + strconv.Itoa(proxy.VoiceHostPort)},
			},
		})
	}
	return node.Autoconfig{
		Edits:   edits,
		Writes:  []node.FileWrite{{Path: "forwarding.secret", Content: secret}},
		Restart: true,
		Reload:  velocityReload,
	}
}

func backendAutoconfig(secret string, server NetServer, voiceProxy bool) (node.Autoconfig, bool) {
	edits := []node.Edit{{
		File:   "server.properties",
		Format: "properties",
		Set:    []node.SetOp{{Path: []string{"online-mode"}, Value: false}},
	}}
	var voiceFile string
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
		voiceFile = "plugins/voicechat/voicechat-server.properties"
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
		voiceFile = "config/voicechat/voicechat-server.properties"
	default:
		return node.Autoconfig{}, false
	}

	if !voiceProxy && server.Row.Voicechat && server.VoiceHostPort > 0 && server.PublicHost != "" {
		edits = append(edits, node.Edit{
			File:   voiceFile,
			Format: "properties",
			Set: []node.SetOp{
				{Path: []string{"port"}, Value: voicePort},
				{Path: []string{"voice_host"}, Value: server.PublicHost + ":" + strconv.Itoa(server.VoiceHostPort)},
			},
		})
	}
	return node.Autoconfig{Edits: edits, Restart: true}, true
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
