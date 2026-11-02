package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

type serverRequest struct {
	Name       string  `json:"name"`
	NodeID     string  `json:"nodeId"`
	Software   string  `json:"software"`
	Version    string  `json:"version"`
	MemoryMB   int     `json:"memoryMB"`
	CPU        float64 `json:"cpu"`
	EULA       bool    `json:"eula"`
	Voicechat  bool    `json:"voicechat"`
	InTry      *bool   `json:"inTry"`
	ForcedHost string  `json:"forcedHost"`
	// Modpack legt den Server aus einem Modrinth-Modpack an (Fabric); Software und Version ergeben sich daraus.
	Modpack *modpackRef `json:"modpack"`
}

func (p *Panel) createServer(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body serverRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 48 {
		writeError(w, fail(http.StatusBadRequest, "der Name braucht 1 bis 48 Zeichen"))
		return
	}
	var pack *resolvedPack
	if body.Modpack != nil {
		if !body.EULA {
			writeError(w, fail(http.StatusBadRequest, "akzeptiere die Minecraft-EULA"))
			return
		}
		if pack, err = p.resolveModpack(r.Context(), *body.Modpack); err != nil {
			writeError(w, err)
			return
		}
		body.Software, body.Version = "fabric", pack.Info.Minecraft
		if body.MemoryMB == 0 {
			body.MemoryMB = 4096 // Modpacks brauchen deutlich mehr als ein einzelner Server
		}
	}
	info, ok := software.Lookup(software.ID(body.Software))
	if !ok {
		writeError(w, fail(http.StatusBadRequest, "unbekannte Software"))
		return
	}
	existing, err := p.store.ListServers(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	row := ServerRow{ProjectID: project.ID, Name: body.Name, Slug: Slugify(body.Name), Software: body.Software, NodeID: body.NodeID,
		Voicechat: body.Voicechat, ForcedHost: strings.ToLower(strings.TrimSpace(body.ForcedHost))}
	if row.Slug == "" {
		writeError(w, fail(http.StatusBadRequest, "der Name braucht mindestens einen Buchstaben oder eine Ziffer"))
		return
	}
	for _, other := range existing {
		if other.Slug == row.Slug {
			writeError(w, fail(http.StatusConflict, "ein Server mit diesem Namen gibt es im Projekt schon"))
			return
		}
	}
	spec := node.Spec{Name: body.Name, Software: info.ID, Version: body.Version, MemoryMB: body.MemoryMB, CPU: body.CPU, EULA: body.EULA}

	switch project.Kind {
	case "server":
		if len(existing) > 0 {
			writeError(w, fail(http.StatusConflict, "ein Server-Projekt hat genau einen Server – lege ein Netzwerk an, wenn du mehrere brauchst"))
			return
		}
		row.Role = RoleStandalone
		if row.Voicechat {
			writeError(w, fail(http.StatusBadRequest, "Voicechat-Autokonfiguration gibt es nur für Server in einem Netzwerk"))
			return
		}
	case "network":
		if len(existing) > 0 {
			row.NodeID = existing[0].NodeID // ein Netzwerk liegt auf einem Node, damit die Server sich im Docker-Netz finden
		}
		switch {
		case info.ID == software.Velocity:
			for _, other := range existing {
				if other.Role == RoleProxy {
					writeError(w, fail(http.StatusConflict, "das Netzwerk hat schon einen Proxy"))
					return
				}
			}
			row.Role = RoleProxy
			if row.Voicechat {
				writeError(w, fail(http.StatusBadRequest, "Voicechat gehört auf die Server hinter dem Proxy"))
				return
			}
			if project.Voicechat {
				// Voicechat über den Proxy: er braucht neben dem TCP- auch einen UDP-Port
				spec.Ports = []node.PortMapping{{Container: proxyPort, Protocol: "tcp"}, {Container: proxyPort, Protocol: "udp"}}
				row.Voicechat = true
			}
		case CanBeBackend(info.ID):
			row.Role = RoleBackend
			row.InTry = body.InTry == nil || *body.InTry
			row.TryPosition = len(existing)
			spec.NoPublish = true
			if project.Voicechat {
				row.Voicechat = true // das Plugin kommt auf den Server, der Proxy nimmt den Voicechat an
			} else if row.Voicechat {
				spec.Ports = []node.PortMapping{{Container: voicePort, Protocol: "udp"}}
			}
		default:
			writeError(w, fail(http.StatusBadRequest, info.Name+" kann nicht hinter Velocity laufen (Paper, Purpur, Folia, Leaf, Fabric und NeoForge gehen)"))
			return
		}
	}

	if row.NodeID == "" {
		nodes, err := p.store.ListNodes()
		if err != nil {
			writeError(w, err)
			return
		}
		if len(nodes) != 1 {
			writeError(w, fail(http.StatusBadRequest, "wähle einen Node aus"))
			return
		}
		row.NodeID = nodes[0].ID
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, fail(http.StatusBadRequest, "den Node gibt es nicht"))
		return
	}

	if pack != nil {
		row.Modpack = &pack.Info
	}
	row.ID = newID()
	spec.ID = row.ID
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := p.ensurePackOnNode(ctx, client, spec.Software); err != nil {
		writeError(w, err)
		return
	}

	var view node.View
	if err := client.Do(ctx, http.MethodPost, "/api/servers", spec, &view); err != nil {
		writeError(w, err)
		return
	}
	p.invalidate(row.NodeID)
	created, err := p.store.CreateServer(row)
	if err != nil {
		_ = client.Do(ctx, http.MethodDelete, "/api/servers/"+row.ID, nil, nil)
		writeError(w, err)
		return
	}
	if pack != nil {
		go p.installModpackWhenReady(created, pack)
	}
	if project.Kind == "network" {
		go p.afterNetworkChange(project.ID, created.ID)
	} else if info.ID == software.Velocity {
		go func() {
			bg, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			config := node.Autoconfig{Edits: []node.Edit{VelocityBind()}, Restart: true, Reload: velocityReload}
			if err := client.Do(bg, http.MethodPut, "/api/servers/"+created.ID+"/autoconfig", config, nil); err != nil {
				p.log.Warn("Velocity-Port nicht festgelegt", "server", created.Name, "error", err)
			}
		}()
	}
	dtos := p.serverDTOs(r.Context(), []ServerRow{created})
	writeJSON(w, http.StatusCreated, dtos[0])
}

func (p *Panel) getServer(w http.ResponseWriter, r *http.Request) {
	row, _, rank, err := p.requireServer(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	dto := p.serverDTOs(r.Context(), []ServerRow{row})[0]
	writeJSON(w, http.StatusOK, map[string]any{"server": dto, "myRole": rankName(rank)})
}

func (p *Panel) patchServer(w http.ResponseWriter, r *http.Request) {
	row, _, _, err := p.requireServer(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var patch node.Patch
	if err := decode(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	var view node.View
	if err := client.Do(r.Context(), http.MethodPatch, "/api/servers/"+row.ID, patch, &view); err != nil {
		writeError(w, err)
		return
	}
	p.invalidate(row.NodeID)
	if patch.Name != nil && view.Name != row.Name {
		row.Name = view.Name
		if err := p.store.UpdateServer(row); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, p.serverDTOs(r.Context(), []ServerRow{row})[0])
}

func (p *Panel) deleteServer(w http.ResponseWriter, r *http.Request) {
	row, project, _, err := p.requireServer(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.removeFromNode(r.Context(), row); err != nil && r.URL.Query().Get("force") != "1" {
		writeError(w, err)
		return
	}
	if err := p.store.DeleteServer(row.ID); err != nil {
		writeError(w, err)
		return
	}
	_ = p.store.DeleteServerMetrics(row.ID)
	if project.Kind == "network" {
		go p.afterNetworkChange(project.ID, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- Proxy zum Node

// requiredRank sagt, welche Stufe eine Anfrage an den Node braucht; ok ist false für alles, was nicht durchgereicht wird.
func requiredRank(method, rest string) (rank int, ok bool) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	switch parts[0] {
	case "logs", "players", "addons", "modpack":
		return rankViewer, method == http.MethodGet && len(parts) == 1
	case "power", "command":
		return rankOperator, method == http.MethodPost && len(parts) == 1
	case "reinstall":
		return rankAdmin, method == http.MethodPost && len(parts) == 1
	case "files":
		return rankOperator, len(parts) <= 2
	case "backups":
		switch {
		case len(parts) == 1 && method == http.MethodGet:
			return rankViewer, true
		case len(parts) == 1 && method == http.MethodPost:
			return rankOperator, true
		case len(parts) == 2 && method == http.MethodDelete:
			return rankOperator, true
		case len(parts) == 3 && (parts[2] == "download" && method == http.MethodGet || parts[2] == "restore" && method == http.MethodPost):
			return rankOperator, true
		}
	}
	return 0, false
}

func (p *Panel) proxyServer(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	need, ok := requiredRank(r.Method, rest)
	if !ok {
		writeError(w, errNotFound)
		return
	}
	row, _, _, err := p.requireServer(r, need)
	if err != nil {
		writeError(w, err)
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	client.Proxy(w, r, "/api/servers/"+row.ID+"/"+rest)
	if r.Method != http.MethodGet {
		p.invalidate(row.NodeID)
	}
}

// consoleSocket verbindet den Browser mit der Konsole auf dem Node. Befehle gelten erst ab der Rolle „operator“.
func (p *Panel) consoleSocket(w http.ResponseWriter, r *http.Request) {
	row, _, rank, err := p.requireServer(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	if !sameOrigin(r) {
		writeError(w, fail(http.StatusForbidden, "falsche Herkunft"))
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	upstream, err := client.DialWS(r.Context(), "/api/servers/"+row.ID+"/ws")
	if err != nil {
		writeError(w, err)
		return
	}
	defer upstream.CloseNow()
	browser, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // Herkunft oben geprüft
	if err != nil {
		return
	}
	defer browser.CloseNow()
	browser.SetReadLimit(64 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() { // Node -> Browser
		defer cancel()
		for {
			kind, data, err := upstream.Read(ctx)
			if err != nil {
				return
			}
			if err := browser.Write(ctx, kind, data); err != nil {
				return
			}
		}
	}()
	go func() { // Browser -> Node
		defer cancel()
		for {
			kind, data, err := browser.Read(ctx)
			if err != nil {
				return
			}
			if rank < rankOperator {
				continue
			}
			if err := upstream.Write(ctx, kind, data); err != nil {
				return
			}
		}
	}()
	<-ctx.Done()
	browser.Close(websocket.StatusNormalClosure, "")
}

// ---------------------------------------------------------------- Netzwerk

type networkEntry struct {
	ID         string `json:"id"`
	InTry      bool   `json:"inTry"`
	ForcedHost string `json:"forcedHost"`
	Voicechat  bool   `json:"voicechat"`
}

// putNetwork speichert Reihenfolge (die Reihenfolge der Liste), Forced Hosts und Voicechat und schreibt die Konfiguration
// in den Proxy und die Server.
func (p *Panel) putNetwork(w http.ResponseWriter, r *http.Request) {
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
		Servers []networkEntry `json:"servers"`
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
	byID := map[string]ServerRow{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	hosts := map[string]string{}
	var warnings []string
	for i, entry := range body.Servers {
		row, ok := byID[entry.ID]
		if !ok || row.Role != RoleBackend {
			writeError(w, fail(http.StatusBadRequest, "unbekannter Server in der Liste"))
			return
		}
		host := strings.ToLower(strings.TrimSpace(entry.ForcedHost))
		if strings.ContainsAny(host, " \t\"'\\") || len(host) > 253 {
			writeError(w, fail(http.StatusBadRequest, "ungültiger Forced Host: "+entry.ForcedHost))
			return
		}
		if other, taken := hosts[host]; host != "" && taken && other != row.ID {
			// derselbe Host darf mehrere Server haben (Velocity nimmt dann der Reihe nach), also nur ein Hinweis
			warnings = append(warnings, host+" zeigt auf mehrere Server")
		}
		hosts[host] = row.ID
		if !project.Voicechat && entry.Voicechat != row.Voicechat {
			if err := p.setVoicechat(r.Context(), row, entry.Voicechat); err != nil {
				writeError(w, err)
				return
			}
			row.Voicechat = entry.Voicechat
		}
		row.InTry, row.ForcedHost, row.TryPosition = entry.InTry, host, i
		if err := p.store.UpdateServer(row); err != nil {
			writeError(w, err)
			return
		}
		delete(byID, entry.ID)
	}
	for _, left := range byID { // nicht genannte Backends behalten ihre Einstellungen, rutschen aber ans Ende
		if left.Role == RoleBackend {
			left.TryPosition += len(body.Servers)
			_ = p.store.UpdateServer(left)
		}
	}
	result, err := p.syncNetwork(r.Context(), project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	result.Warnings = append(result.Warnings, warnings...)
	writeJSON(w, http.StatusOK, result)
}

func (p *Panel) syncNetworkNow(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	if project.Kind != "network" {
		writeError(w, fail(http.StatusBadRequest, "das ist kein Netzwerk"))
		return
	}
	result, err := p.syncNetwork(r.Context(), project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// SyncResult sagt, was beim Abgleich passiert ist.
type SyncResult struct {
	Applied  []string `json:"applied"`
	Pending  []string `json:"pending"`
	Failed   []string `json:"failed"`
	Warnings []string `json:"warnings,omitempty"`
}

// syncNetwork schickt jedem Server des Netzwerks seine Autoconfig.
func (p *Panel) syncNetwork(ctx context.Context, projectID string) (SyncResult, error) {
	result := SyncResult{Applied: []string{}, Pending: []string{}, Failed: []string{}}
	project, err := p.store.ProjectByID(projectID)
	if err != nil {
		return result, err
	}
	rows, err := p.store.ListServers(projectID)
	if err != nil {
		return result, err
	}
	p.invalidateProject(rows)
	dtos := p.serverDTOs(ctx, rows)
	net := make([]NetServer, len(rows))
	for i, dto := range dtos {
		net[i] = NetServer{Row: dto.ServerRow, PublicHost: dto.PublicHost, InternalPort: 25565, VoiceAddress: dto.PublicVoiceAddress}
		if dto.View != nil {
			for _, mapping := range dto.View.Ports {
				if mapping.Protocol != "udp" {
					continue
				}
				if (dto.Role == RoleProxy && mapping.Container == proxyPort) || (dto.Role != RoleProxy && mapping.Container == voicePort) {
					net[i].VoiceHostPort = mapping.Host
				}
			}
		}
	}
	if extras, err := p.store.Extras(projectID); err == nil && extras.MOTD != "" {
		for i := range net {
			if net[i].Row.Role == RoleProxy {
				net[i].MOTD = extras.MOTD
			}
		}
	}
	configs := BuildAutoconfigs(project.Secret, net, project.Voicechat)
	for _, dto := range dtos {
		config, ok := configs[dto.ID]
		if !ok {
			continue
		}
		if !dto.Online || dto.View == nil {
			result.Failed = append(result.Failed, dto.Name+" (Node nicht erreichbar)")
			continue
		}
		client, _, err := p.client(dto.NodeID)
		if err != nil {
			result.Failed = append(result.Failed, dto.Name)
			continue
		}
		var answer struct {
			Pending bool `json:"pending"`
		}
		callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = client.Do(callCtx, http.MethodPut, "/api/servers/"+dto.ID+"/autoconfig", config, &answer)
		cancel()
		switch {
		case err != nil:
			result.Failed = append(result.Failed, dto.Name+": "+err.Error())
		case answer.Pending:
			result.Pending = append(result.Pending, dto.Name)
		default:
			result.Applied = append(result.Applied, dto.Name)
		}
	}
	p.invalidateProject(rows)
	return result, nil
}

func (p *Panel) invalidateProject(rows []ServerRow) {
	for _, row := range rows {
		p.invalidate(row.NodeID)
	}
}

// afterNetworkChange installiert, was ein neuer Server braucht, und gleicht dann das Netzwerk ab.
func (p *Panel) afterNetworkChange(projectID, serverID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if serverID != "" {
		row, err := p.store.ServerByID(serverID)
		if err == nil {
			if err := p.installRequirements(ctx, row); err != nil {
				p.log.Warn("Erweiterungen für den neuen Server nicht installiert", "server", row.Name, "error", err)
			}
		}
	}
	if _, err := p.syncNetwork(ctx, projectID); err != nil {
		p.log.Warn("Netzwerk-Abgleich fehlgeschlagen", "project", projectID, "error", err)
	}
}

// installRequirements lädt den Proxy-Mod (FabricProxy-Lite bei Fabric, Proxy Compatible Forge bei NeoForge) und Simple Voice
// Chat (wenn gewünscht).
func (p *Panel) installRequirements(ctx context.Context, row ServerRow) error {
	var errs []error
	if mod := ProxyMod(software.ID(row.Software)); row.Role == RoleBackend && mod != "" {
		if _, err := p.installFromModrinth(ctx, row, mod, "", true); err != nil && !errors.Is(err, errAlreadyInstalled) {
			errs = append(errs, fmt.Errorf("%s: %w", mod, err))
		}
	}
	if row.Voicechat || row.Role == RoleProxy && p.projectVoicechat(row.ProjectID) {
		if _, err := p.installFromModrinth(ctx, row, "simple-voice-chat", "", true); err != nil && !errors.Is(err, errAlreadyInstalled) {
			errs = append(errs, fmt.Errorf("Simple Voice Chat: %w", err))
		}
	}
	return errors.Join(errs...)
}

// setVoicechat schaltet Voicechat für einen Server ein oder aus (UDP-Port, Plugin/Mod). Der Server muss dafür aus sein.
func (p *Panel) setVoicechat(ctx context.Context, row ServerRow, enable bool) error {
	if VoiceConfigFile(software.ID(row.Software)) == "" {
		return fail(http.StatusBadRequest, "Voicechat gibt es nur für Paper, Purpur, Folia, Leaf, Fabric und NeoForge")
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return err
	}
	var view node.View
	if err := client.Do(ctx, http.MethodGet, "/api/servers/"+row.ID, nil, &view); err != nil {
		return err
	}
	ports := []node.PortMapping{}
	for _, mapping := range view.Ports {
		if mapping.Protocol == "udp" && mapping.Container == voicePort {
			continue
		}
		ports = append(ports, mapping)
	}
	if enable {
		ports = append(ports, node.PortMapping{Container: voicePort, Protocol: "udp"})
	}
	patch := node.Patch{Ports: &ports}
	if len(ports) == 0 {
		noPublish := true
		patch.NoPublish = &noPublish
	}
	if err := client.Do(ctx, http.MethodPatch, "/api/servers/"+row.ID, patch, nil); err != nil {
		var nodeErr *NodeError
		if errors.As(err, &nodeErr) && nodeErr.Status == http.StatusConflict {
			return fail(http.StatusConflict, "„"+row.Name+"“ muss ausgeschaltet sein, damit Voicechat umgestellt werden kann")
		}
		return err
	}
	p.invalidate(row.NodeID)
	if enable {
		row.Voicechat = true
		go func() {
			bg, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			if _, err := p.installFromModrinth(bg, row, "simple-voice-chat", "", true); err != nil {
				p.log.Warn("Simple Voice Chat nicht installiert", "server", row.Name, "error", err)
			}
		}()
	}
	return nil
}
