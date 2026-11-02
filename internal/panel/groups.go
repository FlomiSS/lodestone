package panel

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

const (
	maxRestarts    = 3                // so oft wird eine abgestürzte Instanz innerhalb von restartWindow neu gestartet
	restartWindow  = 15 * time.Minute //
	restartPause   = 30 * time.Second // Mindestabstand zwischen zwei Neustarts
	scaleEvery     = 15 * time.Second
	scaleCooldown  = 45 * time.Second
	maxGroupSize   = 20
	maxStartPerRun = 3
	fillUp         = 0.8 // ab dieser Auslastung kommt eine Instanz dazu
	fillDown       = 0.6 // nur wenn der Rest danach höchstens so voll wäre, wird eine entfernt
	memoryCeiling  = 0.85
	hostReserveMB  = 1536 // soviel soll auf dem Host nach dem Start eines Servers noch wirklich frei sein
)

// GroupDTO ist eine Gruppe samt Zustand für die Oberfläche.
type GroupDTO struct {
	Group
	Instances []string `json:"instances"`
	Active    int      `json:"active"`
	Players   int      `json:"players"`
	Note      string   `json:"note,omitempty"`
}

var (
	noteMu     sync.Mutex // eigener Schloss, damit die Oberfläche nicht auf einen laufenden Abgleich wartet
	groupNotes = map[string]string{}
	// startPoll ist der Takt, in dem auf Installation oder Stopp gewartet wird (in Tests kürzer).
	startPoll = 3 * time.Second
)

func (p *Panel) groupNote(id string) string {
	noteMu.Lock()
	defer noteMu.Unlock()
	return groupNotes[id]
}

func (p *Panel) groupDTOs(ctx context.Context, projectID string, servers []ServerDTO) ([]GroupDTO, error) {
	groups, err := p.store.ListGroups(projectID)
	if err != nil {
		return nil, err
	}
	result := make([]GroupDTO, 0, len(groups))
	for _, group := range groups {
		dto := GroupDTO{Group: group, Instances: []string{}, Note: p.groupNote(group.ID)}
		for _, server := range servers {
			if server.GroupID != group.ID {
				continue
			}
			dto.Instances = append(dto.Instances, server.ID)
			if server.View != nil && activeState(server.View.State) {
				dto.Active++
			}
			dto.Players += p.liveOf(server.ID).Players
		}
		result = append(result, dto)
	}
	return result, nil
}

// recentRestarts sind die Neustart-Versuche der letzten Minuten (Aufrufer hält scaleMu).
func (p *Panel) recentRestarts(id string, now time.Time) []time.Time {
	kept := p.restarts[id][:0:0]
	for _, at := range p.restarts[id] {
		if now.Sub(at) < restartWindow {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(p.restarts, id)
	} else {
		p.restarts[id] = kept
	}
	return kept
}

func activeState(state node.State) bool {
	switch state {
	case node.StateRunning, node.StateStarting, node.StateInstalling, node.StateRestoring:
		return true
	}
	return false
}

// ---------------------------------------------------------------- Anfragen

type groupRequest struct {
	Name               string   `json:"name"`
	Software           string   `json:"software"`
	Version            string   `json:"version"`
	MemoryMB           int      `json:"memoryMB"`
	MinInstances       *int     `json:"minInstances"`
	MaxInstances       *int     `json:"maxInstances"`
	PlayersPerInstance *int     `json:"playersPerInstance"`
	IdleMinutes        *int     `json:"idleMinutes"`
	InTry              *bool    `json:"inTry"`
	Dynamic            *bool    `json:"dynamic"`
	Plugins            []string `json:"plugins"`
	EULA               bool     `json:"eula"`
}

func (r groupRequest) validateLimits(g *Group) error {
	if r.MinInstances != nil {
		g.MinInstances = *r.MinInstances
	}
	if r.MaxInstances != nil {
		g.MaxInstances = *r.MaxInstances
	}
	if r.PlayersPerInstance != nil {
		g.PlayersPerInstance = *r.PlayersPerInstance
	}
	if r.IdleMinutes != nil {
		g.IdleMinutes = *r.IdleMinutes
	}
	if r.InTry != nil {
		g.InTry = *r.InTry
	}
	if r.Dynamic != nil {
		g.Dynamic = *r.Dynamic
	}
	if r.MemoryMB != 0 {
		g.MemoryMB = r.MemoryMB
	}
	if r.Plugins != nil {
		g.Plugins = nil
		for _, plugin := range r.Plugins {
			plugin = strings.ToLower(strings.TrimSpace(plugin))
			if plugin == "" {
				continue
			}
			if strings.ContainsAny(plugin, " ,/?#\\") || len(plugin) > 64 {
				return fail(http.StatusBadRequest, "ungültiges Plugin: "+plugin)
			}
			g.Plugins = append(g.Plugins, plugin)
		}
		if len(g.Plugins) > 20 {
			return fail(http.StatusBadRequest, "höchstens 20 Plugins")
		}
	}
	switch {
	case g.MinInstances < 0 || g.MinInstances > maxGroupSize:
		return fail(http.StatusBadRequest, fmt.Sprintf("die Mindestzahl muss zwischen 0 und %d liegen", maxGroupSize))
	case g.MaxInstances < 1 || g.MaxInstances > maxGroupSize || g.MaxInstances < g.MinInstances:
		return fail(http.StatusBadRequest, fmt.Sprintf("die Höchstzahl muss zwischen 1 und %d und mindestens so groß wie die Mindestzahl sein", maxGroupSize))
	case g.PlayersPerInstance < 1 || g.PlayersPerInstance > 1000:
		return fail(http.StatusBadRequest, "Spieler pro Instanz: 1 bis 1000")
	case g.IdleMinutes < 1 || g.IdleMinutes > 1440:
		return fail(http.StatusBadRequest, "Leerlauf: 1 bis 1440 Minuten")
	case g.MemoryMB < 256 || g.MemoryMB > 65536:
		return fail(http.StatusBadRequest, "Arbeitsspeicher: 256 bis 65536 MB")
	}
	return nil
}

func (p *Panel) createGroup(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	if project.Kind != "network" {
		writeError(w, fail(http.StatusBadRequest, "Gruppen gibt es nur in Netzwerken"))
		return
	}
	var body groupRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	slug := Slugify(body.Name)
	if body.Name == "" || len(body.Name) > 32 || slug == "" {
		writeError(w, fail(http.StatusBadRequest, "der Name braucht 1 bis 32 Zeichen mit mindestens einem Buchstaben oder einer Ziffer"))
		return
	}
	if _, ok := software.Lookup(software.ID(body.Software)); !ok || !CanBeBackend(software.ID(body.Software)) {
		writeError(w, fail(http.StatusBadRequest, "Gruppen gibt es für Paper, Purpur, Folia, Leaf, Fabric und NeoForge"))
		return
	}
	if strings.TrimSpace(body.Version) == "" || !body.EULA {
		writeError(w, fail(http.StatusBadRequest, "Version angeben und die Minecraft-EULA akzeptieren"))
		return
	}
	group := Group{ProjectID: project.ID, Name: body.Name, Slug: slug, Software: body.Software, Version: strings.TrimSpace(body.Version),
		MemoryMB: 1024, MinInstances: 1, MaxInstances: 4, PlayersPerInstance: 20, IdleMinutes: 5, InTry: true, Dynamic: true, Plugins: []string{}}
	if err := body.validateLimits(&group); err != nil {
		writeError(w, err)
		return
	}
	// Der Kurzname darf keinem Server und keiner anderen Gruppe im Weg stehen (Instanzen heißen name-1, name-2 …)
	servers, err := p.store.ListServers(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, server := range servers {
		if server.Slug == slug || strings.HasPrefix(server.Slug, slug+"-") {
			writeError(w, fail(http.StatusConflict, "der Name kollidiert mit dem Server „"+server.Name+"“"))
			return
		}
	}
	if len(servers) == 0 {
		writeError(w, fail(http.StatusConflict, "lege zuerst den Proxy an"))
		return
	}
	created, err := p.store.CreateGroup(group)
	if err != nil {
		writeError(w, err)
		return
	}
	go p.reconcileGroup(context.Background(), created)
	dtos := p.serverDTOs(r.Context(), servers)
	list, _ := p.groupDTOs(r.Context(), project.ID, dtos)
	for _, entry := range list {
		if entry.ID == created.ID {
			writeJSON(w, http.StatusCreated, entry)
			return
		}
	}
	writeJSON(w, http.StatusCreated, created)
}

func (p *Panel) requireGroup(r *http.Request, minimum int) (Group, Project, error) {
	group, err := p.store.GroupByID(r.PathValue("id"))
	if err != nil {
		return Group{}, Project{}, errNotFound
	}
	project, _, err := p.requireProject(r, group.ProjectID, minimum)
	return group, project, err
}

func (p *Panel) updateGroup(w http.ResponseWriter, r *http.Request) {
	group, _, err := p.requireGroup(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body groupRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if name := strings.TrimSpace(body.Name); name != "" && len(name) <= 32 {
		group.Name = name
	}
	if version := strings.TrimSpace(body.Version); version != "" {
		group.Version = version
	}
	if err := body.validateLimits(&group); err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.UpdateGroup(group); err != nil {
		writeError(w, err)
		return
	}
	go p.reconcileGroup(context.Background(), group)
	writeJSON(w, http.StatusOK, group)
}

func (p *Panel) deleteGroup(w http.ResponseWriter, r *http.Request) {
	group, project, err := p.requireGroup(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	p.scaleMu.Lock()
	instances, err := p.store.ListGroupServers(group.ID)
	if err == nil {
		// Die Gruppe verschwindet zuerst, damit der Autoscaler nichts nachstartet
		err = p.store.DeleteGroup(group.ID)
	}
	p.scaleMu.Unlock()
	if err != nil {
		writeError(w, err)
		return
	}
	var failed []string
	for _, row := range instances {
		if err := p.removeFromNode(r.Context(), row); err != nil {
			failed = append(failed, row.Name)
			continue
		}
		_ = p.store.DeleteServer(row.ID)
		_ = p.store.DeleteServerMetrics(row.ID)
	}
	go p.afterNetworkChange(project.ID, "")
	if len(failed) > 0 {
		writeError(w, fail(http.StatusBadGateway, "Die Gruppe ist gelöscht, diese Instanzen ließen sich auf dem Node nicht entfernen: "+strings.Join(failed, ", ")))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) addInstance(w http.ResponseWriter, r *http.Request) {
	group, project, err := p.requireGroup(r, rankOperator)
	if err != nil {
		writeError(w, err)
		return
	}
	p.scaleMu.Lock()
	instances, _ := p.store.ListGroupServers(group.ID)
	p.scaleMu.Unlock()
	if len(instances) >= group.MaxInstances {
		writeError(w, fail(http.StatusConflict, "die Höchstzahl der Instanzen ist erreicht"))
		return
	}
	row, err := p.createInstance(r.Context(), project, group)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p.serverDTOs(r.Context(), []ServerRow{row})[0])
}

// ---------------------------------------------------------------- Instanzen

// groupNode ist der Node, auf dem das Netzwerk liegt.
func (p *Panel) groupNode(projectID string) (string, error) {
	servers, err := p.store.ListServers(projectID)
	if err != nil {
		return "", err
	}
	if len(servers) == 0 {
		return "", errors.New("das Netzwerk hat noch keinen Server")
	}
	return servers[0].NodeID, nil
}

// canFit prüft, ob auf dem Node noch Platz für einen Server mit dieser Größe ist, damit der Autoscaler die Maschine
// nicht an die Wand fährt.
func (p *Panel) canFit(ctx context.Context, nodeID string, memoryMB int) (bool, string) {
	client, _, err := p.client(nodeID)
	if err != nil {
		return false, "Node nicht gefunden"
	}
	var system struct {
		Docker          *node.SystemInfo `json:"docker"`
		MemoryAvailable int64            `json:"memoryAvailable"`
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Do(callCtx, http.MethodGet, "/api/system", nil, &system); err != nil || system.Docker == nil {
		return false, "Node nicht erreichbar"
	}
	views, err := p.nodeViews(ctx, nodeID)
	if err != nil {
		return false, "Node nicht erreichbar"
	}
	used := 0
	for _, view := range views {
		if activeState(view.State) {
			used += view.MemoryMB
		}
	}
	return fitsMemory(memoryMB, used, system.Docker.MemoryBytes>>20, system.MemoryAvailable>>20)
}

// fitsMemory entscheidet, ob ein neuer Server mit memoryMB noch Platz hat. Zwei Grenzen gelten: Die Container-Limits
// aller aktiven Server bleiben unter 85 % des Arbeitsspeichers, und auf dem Host müssen nach dem Start noch mindestens
// 1,5 GB wirklich frei sein (der Server braucht über seinen Heap hinaus Platz, und auf der Maschine laufen auch andere
// Dienste). Ist der freie Speicher unbekannt (0), gilt nur die erste Grenze.
func fitsMemory(memoryMB, usedLimitsMB int, totalMB, availableMB int64) (bool, string) {
	limit := int(float64(totalMB) * memoryCeiling)
	if usedLimitsMB+memoryMB > limit {
		return false, fmt.Sprintf("Zu wenig Arbeitsspeicher auf dem Node (%d von %d MB für Server vergeben)", usedLimitsMB, limit)
	}
	if availableMB > 0 {
		needed := int64(float64(memoryMB)*1.25) + hostReserveMB
		if availableMB < needed {
			return false, fmt.Sprintf("Auf dem Node sind nur noch %d MB Arbeitsspeicher frei (nötig: %d MB inklusive Reserve)", availableMB, needed)
		}
	}
	return true, ""
}

// createInstance legt eine neue Instanz der Gruppe an und startet sie, sobald sie installiert ist.
func (p *Panel) createInstance(ctx context.Context, project Project, group Group) (ServerRow, error) {
	nodeID, err := p.groupNode(project.ID)
	if err != nil {
		return ServerRow{}, fail(http.StatusConflict, err.Error())
	}
	if fits, why := p.canFit(ctx, nodeID, group.MemoryMB); !fits {
		return ServerRow{}, fail(http.StatusConflict, why)
	}
	servers, err := p.store.ListServers(project.ID)
	if err != nil {
		return ServerRow{}, err
	}
	taken := map[string]bool{}
	position := 0
	for _, server := range servers {
		taken[server.Slug] = true
		if server.TryPosition >= position {
			position = server.TryPosition + 1
		}
	}
	number := 1
	for taken[fmt.Sprintf("%s-%d", group.Slug, number)] {
		number++
	}
	slug := fmt.Sprintf("%s-%d", group.Slug, number)
	row := ServerRow{ID: newID(), ProjectID: project.ID, NodeID: nodeID, Name: fmt.Sprintf("%s-%d", group.Name, number), Slug: slug, Role: RoleBackend,
		Software: group.Software, InTry: group.InTry, TryPosition: position, GroupID: group.ID, Voicechat: project.Voicechat}
	spec := node.Spec{ID: row.ID, Name: row.Name, Software: software.ID(group.Software), Version: group.Version, MemoryMB: group.MemoryMB, EULA: true, NoPublish: true}
	client, _, err := p.client(nodeID)
	if err != nil {
		return ServerRow{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := p.ensurePackOnNode(callCtx, client, spec.Software); err != nil {
		return ServerRow{}, err
	}
	if err := client.Do(callCtx, http.MethodPost, "/api/servers", spec, nil); err != nil {
		return ServerRow{}, err
	}
	p.invalidate(nodeID)
	created, err := p.store.CreateServer(row)
	if err != nil {
		_ = client.Do(callCtx, http.MethodDelete, "/api/servers/"+row.ID, nil, nil)
		return ServerRow{}, err
	}
	p.preparing.Store(created.ID, true)
	go p.prepareInstance(created, group, project)
	return created, nil
}

// prepareInstance installiert die Plugins der Gruppe, gleicht das Netzwerk ab und startet die Instanz.
func (p *Panel) prepareInstance(row ServerRow, group Group, project Project) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	defer p.preparing.Delete(row.ID)
	p.log.Info("Instanz wird vorbereitet", "server", row.Name, "gruppe", group.Name)
	if err := p.installRequirements(ctx, row); err != nil {
		p.log.Warn("Erweiterungen der Instanz nicht installiert", "server", row.Name, "error", err)
	}
	for _, plugin := range group.Plugins {
		if _, err := p.installFromModrinth(ctx, row, plugin, "", true); err != nil && !errors.Is(err, errAlreadyInstalled) {
			p.log.Warn("Plugin der Gruppe nicht installiert", "server", row.Name, "plugin", plugin, "error", err)
		}
	}
	if _, err := p.syncNetwork(ctx, project.ID); err != nil {
		p.log.Warn("Netzwerk-Abgleich fehlgeschlagen", "error", err)
	}
	p.startWhenReady(ctx, row)
	p.log.Info("Instanz ist bereit", "server", row.Name)
}

// startWhenReady wartet, bis die Installation fertig ist, und startet den Server.
func (p *Panel) startWhenReady(ctx context.Context, row ServerRow) {
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return
	}
	for attempt := 0; attempt < 120; attempt++ {
		var view node.View
		if err := client.Do(ctx, http.MethodGet, "/api/servers/"+row.ID, nil, &view); err != nil {
			var nodeErr *NodeError
			if errors.As(err, &nodeErr) && nodeErr.Status == http.StatusNotFound {
				return // inzwischen gelöscht
			}
		} else if view.State == node.StateOffline && view.Installed {
			if err := client.Do(ctx, http.MethodPost, "/api/servers/"+row.ID+"/power", map[string]string{"action": "start"}, nil); err != nil {
				p.log.Warn("Instanz ließ sich nicht starten", "server", row.Name, "error", err)
			}
			p.invalidate(row.NodeID)
			return
		} else if view.State == node.StateRunning || view.State == node.StateStarting {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(startPoll):
		}
	}
}

// ---------------------------------------------------------------- Autoscaler

func (p *Panel) scaleLoop(ctx context.Context) {
	ticker := time.NewTicker(scaleEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			groups, err := p.store.ListGroups("")
			if err != nil {
				continue
			}
			for _, group := range groups {
				p.reconcileGroup(ctx, group)
			}
		}
	}
}

// reconcileGroup bringt die Zahl der Instanzen einer Gruppe auf den Bedarf.
func (p *Panel) reconcileGroup(ctx context.Context, group Group) {
	p.scaleMu.Lock()
	defer p.scaleMu.Unlock()
	// Die Gruppe kann inzwischen gelöscht oder geändert sein
	current, err := p.store.GroupByID(group.ID)
	if err != nil {
		return
	}
	group = current
	project, err := p.store.ProjectByID(group.ProjectID)
	if err != nil {
		return
	}
	instances, err := p.store.ListGroupServers(group.ID)
	if err != nil {
		return
	}
	views, err := p.nodeViewsFor(ctx, instances)
	if err != nil {
		p.setNoteLocked(group.ID, "Node nicht erreichbar")
		return
	}

	type state struct {
		row     ServerRow
		view    node.View
		known   bool
		active  bool
		restart bool // aus, soll aber laufen: wird neu gestartet
		players int
	}
	states := make([]state, 0, len(instances))
	active, players := 0, 0
	now := time.Now()
	crashLoop := ""
	for _, row := range instances {
		view, known := views[row.ID]
		entry := state{row: row, view: view, known: known, players: p.liveOf(row.ID).Players}
		_, preparing := p.preparing.Load(row.ID)
		entry.active = known && (activeState(view.State) || preparing)
		// Eine abgestürzte Instanz einer dynamischen Gruppe starten wir neu, statt eine neue anzulegen. Stürzt sie immer
		// wieder ab, legen wir nichts Weiteres an (der Fehler liegt dann an der Vorlage, nicht an der Auslastung).
		if known && !entry.active && group.Dynamic && view.State == node.StateOffline && view.Installed {
			if len(p.recentRestarts(row.ID, now)) < maxRestarts {
				entry.active, entry.restart = true, true
			} else {
				crashLoop = row.Name
			}
		}
		if entry.active {
			active++
			players += entry.players
		}
		states = append(states, entry)
	}

	changed := false

	restartNote := ""
	for _, entry := range states {
		if !entry.restart {
			continue
		}
		recent := p.recentRestarts(entry.row.ID, now)
		if len(recent) > 0 && now.Sub(recent[len(recent)-1]) < restartPause {
			continue
		}
		if fits, why := p.canFit(ctx, entry.row.NodeID, entry.view.MemoryMB); !fits {
			restartNote = why
			continue
		}
		if client, _, err := p.client(entry.row.NodeID); err == nil {
			p.restarts[entry.row.ID] = append(recent, now)
			if err := client.Do(ctx, http.MethodPost, "/api/servers/"+entry.row.ID+"/power", map[string]string{"action": "start"}, nil); err != nil {
				p.log.Warn("Instanz ließ sich nicht neu starten", "server", entry.row.Name, "error", err)
			}
			p.invalidate(entry.row.NodeID)
		}
	}

	desired := int(math.Ceil(float64(players) / (float64(group.PlayersPerInstance) * fillUp)))
	if desired < group.MinInstances {
		desired = group.MinInstances
	}
	if desired > group.MaxInstances {
		desired = group.MaxInstances
	}

	// Hochskalieren
	note := restartNote
	if crashLoop != "" {
		note = crashLoop + " stürzt immer wieder ab (siehe Konsole). Solange legt das Panel keine weitere Instanz an."
	}
	started := 0
	for crashLoop == "" && active < desired && len(instances)+started < group.MaxInstances && started < maxStartPerRun {
		// Bei Auslastung (nicht bei der Mindestzahl) gilt eine Pause, damit nicht auf einen Schlag viele entstehen
		if active >= group.MinInstances && now.Sub(p.scaleLast[group.ID]) < scaleCooldown {
			break
		}
		// Zuerst eine vorhandene, gestoppte Instanz nutzen (bei statischen Gruppen)
		woke := false
		for _, entry := range states {
			if entry.known && !entry.active && entry.view.State == node.StateOffline && entry.view.Installed && !group.Dynamic {
				if client, _, err := p.client(entry.row.NodeID); err == nil {
					if fits, why := p.canFit(ctx, entry.row.NodeID, entry.view.MemoryMB); !fits {
						note = why
						break
					}
					if client.Do(ctx, http.MethodPost, "/api/servers/"+entry.row.ID+"/power", map[string]string{"action": "start"}, nil) == nil {
						woke = true
						p.invalidate(entry.row.NodeID)
						for i := range states {
							if states[i].row.ID == entry.row.ID {
								states[i].active = true
							}
						}
					}
				}
				break
			}
		}
		if !woke && note == "" {
			if _, err := p.createInstance(ctx, project, group); err != nil {
				note = err.Error()
				break
			}
			started++
			changed = true
		}
		if note != "" {
			break
		}
		active++
		p.scaleLast[group.ID] = now
	}

	// Herunterskalieren: eine leere, überzählige Instanz nach dem Leerlauf entfernen
	floor := group.MinInstances
	if desired > floor {
		floor = desired
	}
	if active > floor {
		remaining := float64(active-1) * float64(group.PlayersPerInstance) * fillDown
		if float64(players) <= remaining {
			sort.SliceStable(states, func(i, j int) bool { return states[i].row.CreatedAt > states[j].row.CreatedAt })
			for _, entry := range states {
				if !entry.active || !entry.known || entry.view.State != node.StateRunning {
					continue
				}
				if entry.players > 0 {
					delete(p.idleSince, entry.row.ID)
					continue
				}
				since, ok := p.idleSince[entry.row.ID]
				if !ok {
					p.idleSince[entry.row.ID] = now
					continue
				}
				if now.Sub(since) < time.Duration(group.IdleMinutes)*time.Minute {
					continue
				}
				if p.retireInstance(ctx, group, entry.row) {
					delete(p.idleSince, entry.row.ID)
					changed = true
				}
				break
			}
		}
	}
	for _, entry := range states { // wer wieder Spieler hat, ist nicht mehr leer
		if entry.players > 0 {
			delete(p.idleSince, entry.row.ID)
		}
	}

	p.setNoteLocked(group.ID, note)
	if changed {
		go func() {
			bg, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := p.syncNetwork(bg, project.ID); err != nil {
				p.log.Warn("Netzwerk-Abgleich nach Skalierung fehlgeschlagen", "error", err)
			}
		}()
	}
}

func (p *Panel) setNoteLocked(id, note string) {
	noteMu.Lock()
	defer noteMu.Unlock()
	if note == "" {
		delete(groupNotes, id)
	} else {
		groupNotes[id] = note
	}
}

// retireInstance fährt eine Instanz herunter und löscht sie (dynamische Gruppe) oder lässt sie gestoppt liegen.
func (p *Panel) retireInstance(ctx context.Context, group Group, row ServerRow) bool {
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return false
	}
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := client.Do(stopCtx, http.MethodPost, "/api/servers/"+row.ID+"/power", map[string]string{"action": "stop"}, nil); err != nil {
		return false
	}
	p.invalidate(row.NodeID)
	if !group.Dynamic {
		return true
	}
	// Erst löschen, wenn der Server wirklich aus ist
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		for attempt := 0; attempt < 60; attempt++ {
			var view node.View
			if err := client.Do(bg, http.MethodGet, "/api/servers/"+row.ID, nil, &view); err == nil && view.State == node.StateOffline {
				if err := p.removeFromNode(bg, row); err == nil {
					_ = p.store.DeleteServer(row.ID)
					_ = p.store.DeleteServerMetrics(row.ID)
					if _, err := p.syncNetwork(bg, row.ProjectID); err != nil {
						p.log.Warn("Netzwerk-Abgleich fehlgeschlagen", "error", err)
					}
				}
				return
			}
			select {
			case <-bg.Done():
				return
			case <-time.After(startPoll):
			}
		}
	}()
	return true
}

// nodeViewsFor holt die Zustände der Server aller betroffenen Nodes.
func (p *Panel) nodeViewsFor(ctx context.Context, rows []ServerRow) (map[string]node.View, error) {
	all := map[string]node.View{}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.NodeID] {
			continue
		}
		seen[row.NodeID] = true
		views, err := p.nodeViews(ctx, row.NodeID)
		if err != nil {
			return nil, err
		}
		for id, view := range views {
			all[id] = view
		}
	}
	return all, nil
}

// ---------------------------------------------------------------- Voicechat über den Proxy

func (p *Panel) projectVoicechat(projectID string) bool {
	project, err := p.store.ProjectByID(projectID)
	return err == nil && project.Voicechat
}

func hasUDPPort(ports []node.PortMapping, container int) bool {
	for _, port := range ports {
		if port.Protocol == "udp" && port.Container == container {
			return true
		}
	}
	return false
}

// putVoicechat schaltet Simple Voice Chat über den Proxy ein oder aus: Der Proxy bekommt das Proxy-Plugin und einen
// UDP-Port, die Server nur das normale Plugin. Spieler brauchen dann nur noch diesen einen Port.
func (p *Panel) putVoicechat(w http.ResponseWriter, r *http.Request) {
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
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	rows, err := p.store.ListServers(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	var proxy *ServerRow
	for i := range rows {
		if rows[i].Role == RoleProxy {
			proxy = &rows[i]
		}
	}
	if proxy == nil {
		writeError(w, fail(http.StatusConflict, "das Netzwerk braucht zuerst einen Proxy"))
		return
	}

	if body.Enabled {
		client, _, err := p.client(proxy.NodeID)
		if err != nil {
			writeError(w, err)
			return
		}
		var view node.View
		if err := client.Do(r.Context(), http.MethodGet, "/api/servers/"+proxy.ID, nil, &view); err != nil {
			writeError(w, err)
			return
		}
		if !hasUDPPort(view.Ports, proxyPort) {
			if view.State != node.StateOffline {
				writeError(w, fail(http.StatusConflict, "Der Proxy muss dafür ausgeschaltet sein: Er bekommt einen zusätzlichen UDP-Port"))
				return
			}
			ports := append(append([]node.PortMapping{}, view.Ports...), node.PortMapping{Container: proxyPort, Protocol: "udp"})
			if len(view.Ports) == 0 {
				ports = append([]node.PortMapping{{Container: proxyPort, Protocol: "tcp"}}, ports...)
			}
			if err := client.Do(r.Context(), http.MethodPatch, "/api/servers/"+proxy.ID, node.Patch{Ports: &ports}, nil); err != nil {
				writeError(w, err)
				return
			}
			p.invalidate(proxy.NodeID)
		}
	}

	project.Voicechat = body.Enabled
	if err := p.store.UpdateProject(project); err != nil {
		writeError(w, err)
		return
	}
	for _, row := range rows {
		switch {
		case row.Role == RoleBackend && row.Voicechat != body.Enabled:
			row.Voicechat = body.Enabled
			_ = p.store.UpdateServer(row)
		case row.Role == RoleProxy && row.Voicechat != body.Enabled:
			row.Voicechat = body.Enabled
			_ = p.store.UpdateServer(row)
		}
	}
	if body.Enabled {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			updated, _ := p.store.ListServers(project.ID)
			for _, row := range updated {
				if row.Voicechat {
					if _, err := p.installFromModrinth(ctx, row, "simple-voice-chat", "", true); err != nil && !errors.Is(err, errAlreadyInstalled) {
						p.log.Warn("Simple Voice Chat nicht installiert", "server", row.Name, "error", err)
					}
				}
			}
			if _, err := p.syncNetwork(ctx, project.ID); err != nil {
				p.log.Warn("Netzwerk-Abgleich fehlgeschlagen", "error", err)
			}
		}()
	}
	result, err := p.syncNetwork(r.Context(), project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
