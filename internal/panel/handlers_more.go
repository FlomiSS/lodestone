package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
)

// ---------------------------------------------------------------- Spielerverwaltung

var (
	playerName = regexp.MustCompile(`^\.?[A-Za-z0-9_]{1,16}$`) // Java-Namen; der Punkt vorweg ist die Kennung von Floodgate (Bedrock)
	ipAddress  = regexp.MustCompile(`^[0-9A-Fa-f:.]{3,45}$`)
)

// PlayerLists sind die Listen, die ein Minecraft-Server als Dateien führt.
type PlayerLists struct {
	Whitelist []ListedPlayer `json:"whitelist"`
	Ops       []ListedPlayer `json:"ops"`
	Banned    []ListedPlayer `json:"banned"`
	BannedIPs []BannedIP     `json:"bannedIps"`
}

type ListedPlayer struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Level   int    `json:"level,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Created string `json:"created,omitempty"`
	Source  string `json:"source,omitempty"`
	Expires string `json:"expires,omitempty"`
}

type BannedIP struct {
	IP      string `json:"ip"`
	Reason  string `json:"reason,omitempty"`
	Created string `json:"created,omitempty"`
	Source  string `json:"source,omitempty"`
	Expires string `json:"expires,omitempty"`
}

func readJSONFile(ctx context.Context, client *NodeClient, id, name string, into any) {
	data, err := client.Raw(ctx, http.MethodGet, "/api/servers/"+id+"/files/content?path="+name, nil)
	if err != nil {
		return // fehlt die Datei, ist die Liste leer
	}
	_ = json.Unmarshal(data, into)
}

func (p *Panel) playerLists(w http.ResponseWriter, r *http.Request) {
	row, _, _, err := p.requireServer(r, rankOperator)
	if err != nil {
		writeError(w, err)
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	lists := PlayerLists{Whitelist: []ListedPlayer{}, Ops: []ListedPlayer{}, Banned: []ListedPlayer{}, BannedIPs: []BannedIP{}}
	readJSONFile(r.Context(), client, row.ID, "whitelist.json", &lists.Whitelist)
	readJSONFile(r.Context(), client, row.ID, "ops.json", &lists.Ops)
	readJSONFile(r.Context(), client, row.ID, "banned-players.json", &lists.Banned)
	readJSONFile(r.Context(), client, row.ID, "banned-ips.json", &lists.BannedIPs)
	for _, list := range []*[]ListedPlayer{&lists.Whitelist, &lists.Ops, &lists.Banned} {
		if *list == nil {
			*list = []ListedPlayer{}
		}
	}
	if lists.BannedIPs == nil {
		lists.BannedIPs = []BannedIP{}
	}
	writeJSON(w, http.StatusOK, lists)
}

// cleanText macht aus Eingaben einen Text, der als ein Teil eines Befehls sicher ist (keine Zeilenumbrüche, Länge begrenzt).
func cleanText(text string, limit int) string {
	text = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > limit {
		text = text[:limit]
	}
	return text
}

// playerCommand baut aus einer Aktion den Konsolenbefehl. Namen und Adressen werden streng geprüft, damit sich in den
// Befehl nichts einschleusen lässt.
func playerCommand(action, name, reason, text string) (string, error) {
	needName := func() error {
		if !playerName.MatchString(name) {
			return errors.New("ungültiger Spielername (erlaubt: 1 bis 16 Buchstaben, Ziffern, _)")
		}
		return nil
	}
	reason = cleanText(reason, 100)
	switch action {
	case "kick", "ban":
		if err := needName(); err != nil {
			return "", err
		}
		return strings.TrimSpace(action + " " + name + " " + reason), nil
	case "pardon", "op", "deop":
		if err := needName(); err != nil {
			return "", err
		}
		return action + " " + name, nil
	case "whitelist-add", "whitelist-remove":
		if err := needName(); err != nil {
			return "", err
		}
		return "whitelist " + strings.TrimPrefix(action, "whitelist-") + " " + name, nil
	case "whitelist-on", "whitelist-off", "whitelist-reload":
		return "whitelist " + strings.TrimPrefix(action, "whitelist-"), nil
	case "message":
		if err := needName(); err != nil {
			return "", err
		}
		text = cleanText(text, 200)
		if text == "" {
			return "", errors.New("die Nachricht ist leer")
		}
		return "tell " + name + " " + text, nil
	case "ban-ip", "pardon-ip":
		if !ipAddress.MatchString(name) {
			return "", errors.New("ungültige IP-Adresse")
		}
		return strings.TrimSpace(action + " " + name + " " + reason), nil
	}
	return "", fmt.Errorf("unbekannte Aktion %q", action)
}

func (p *Panel) playerAction(w http.ResponseWriter, r *http.Request) {
	row, _, _, err := p.requireServer(r, rankOperator)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Action string `json:"action"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
		Text   string `json:"text"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if row.Software == "velocity" {
		writeError(w, fail(http.StatusBadRequest, "Ein Proxy kennt diese Befehle nicht – nutze den Server, auf dem der Spieler ist"))
		return
	}
	command, err := playerCommand(body.Action, strings.TrimSpace(body.Name), body.Reason, body.Text)
	if err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := client.Do(r.Context(), http.MethodPost, "/api/servers/"+row.ID+"/command", map[string]string{"command": command}, nil); err != nil {
		writeError(w, err)
		return
	}
	p.audit(r, "Spieleraktion", body.Action+" "+body.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- MOTD des Netzwerks

func (p *Panel) putMOTD(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	if project.Kind != "network" {
		writeError(w, fail(http.StatusBadRequest, "das ist kein Netzwerk"))
		return
	}
	var body struct {
		MOTD string `json:"motd"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	motd := strings.ReplaceAll(strings.TrimSpace(body.MOTD), "\r", "")
	if len(motd) > 400 || strings.Count(motd, "\n") > 1 || strings.ContainsRune(motd, 0) {
		writeError(w, fail(http.StatusBadRequest, "die MOTD darf höchstens zwei Zeilen und 400 Zeichen haben"))
		return
	}
	extras, err := p.store.Extras(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	extras.MOTD = motd
	if err := p.store.SetExtras(project.ID, extras); err != nil {
		writeError(w, err)
		return
	}
	result, err := p.syncNetwork(r.Context(), project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------- Öffentliche Statusseite

var statusSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}$`)

func (p *Panel) putStatus(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Public bool   `json:"public"`
		Slug   string `json:"slug"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	extras, err := p.store.Extras(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	slug := strings.ToLower(strings.TrimSpace(body.Slug))
	if slug == "" {
		slug = extras.StatusSlug
	}
	if slug == "" && body.Public {
		slug = Slugify(project.Name)
		if len(slug) < 3 {
			slug += "-" + project.ID[:4]
		}
	}
	if slug != "" && !statusSlug.MatchString(slug) {
		writeError(w, fail(http.StatusBadRequest, "die Adresse braucht 3 bis 40 Zeichen: Kleinbuchstaben, Ziffern und Bindestrich"))
		return
	}
	extras.StatusPublic, extras.StatusSlug = body.Public, slug
	if err := p.store.SetExtras(project.ID, extras); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, extras)
}

type publicServer struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Players int    `json:"players"`
	Max     int    `json:"maxPlayers,omitempty"`
}

type publicStatus struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Online      bool           `json:"online"`
	Address     string         `json:"address,omitempty"`
	Players     int            `json:"players"`
	MaxPlayers  int            `json:"maxPlayers,omitempty"`
	Version     string         `json:"version,omitempty"`
	Servers     []publicServer `json:"servers"`
	Updated     int64          `json:"updated"`
}

type limiter struct {
	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

// allow erlaubt höchstens limit Anfragen pro Minute und Schlüssel.
func (l *limiter) allow(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]*window{}
	}
	now := time.Now()
	w := l.windows[key]
	if w == nil || now.Sub(w.start) > time.Minute {
		if len(l.windows) > 5000 {
			l.windows = map[string]*window{}
		}
		w = &window{start: now}
		l.windows[key] = w
	}
	w.count++
	return w.count <= limit
}

// publicStatus liefert die Daten der Statusseite. Es gibt nur her, was auf der Seite stehen darf: keine IDs, keine
// internen Adressen, keine Konfiguration.
func (p *Panel) publicStatus(w http.ResponseWriter, r *http.Request) {
	if !p.statusLimit.allow(p.clientIP(r), 60) {
		writeError(w, fail(http.StatusTooManyRequests, "zu viele Anfragen"))
		return
	}
	project, err := p.store.ProjectByStatusSlug(r.PathValue("slug"))
	if err != nil {
		writeError(w, errNotFound)
		return
	}
	p.statusMu.Lock()
	cached, ok := p.statusCache[project.ID]
	p.statusMu.Unlock()
	if !ok || time.Since(time.UnixMilli(cached.Updated)) > 15*time.Second {
		cached = p.buildStatus(r.Context(), project)
		p.statusMu.Lock()
		p.statusCache[project.ID] = cached
		p.statusMu.Unlock()
	}
	w.Header().Set("Cache-Control", "public, max-age=10")
	w.Header().Set("Access-Control-Allow-Origin", "*") // lesbar von jeder Seite (Widget)
	writeJSON(w, http.StatusOK, cached)
}

func (p *Panel) buildStatus(ctx context.Context, project Project) publicStatus {
	rows, _ := p.store.ListServers(project.ID)
	dtos := p.serverDTOs(ctx, rows)
	status := publicStatus{Name: project.Name, Description: project.Description, Servers: []publicServer{}, Updated: time.Now().UnixMilli()}
	var front *ServerDTO
	for i := range dtos {
		if dtos[i].Role != RoleBackend {
			front = &dtos[i]
		}
	}
	for _, dto := range dtos {
		running := dto.View != nil && dto.View.State == node.StateRunning
		live := p.liveOf(dto.ID)
		if dto.Role == RoleBackend || project.Kind == "server" {
			status.Servers = append(status.Servers, publicServer{Name: dto.Name, Online: running, Players: live.Players, Max: live.MaxPlayers})
		}
	}
	if front != nil {
		live := p.liveOf(front.ID)
		status.Online = front.View != nil && front.View.State == node.StateRunning
		status.Players, status.MaxPlayers = live.Players, live.MaxPlayers
		status.Address = front.Address
		if front.PublicAddress != "" {
			status.Address = front.PublicAddress
		}
		if front.View != nil {
			status.Version = front.View.Version
		}
		if !status.Online {
			status.Players = 0
		}
	}
	return status
}
