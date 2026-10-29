package panel

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

// ServerDTO ist ein Server, wie ihn die Oberfläche sieht: Zeile aus der Datenbank plus Zustand vom Node.
type ServerDTO struct {
	ServerRow
	NodeName     string     `json:"nodeName"`
	PublicHost   string     `json:"publicHost"`
	Online       bool       `json:"online"`
	Address      string     `json:"address,omitempty"`
	VoiceAddress string     `json:"voiceAddress,omitempty"`
	View         *node.View `json:"view"`
}

type ProjectDTO struct {
	Project
	MyRole   string      `json:"myRole"`
	Members  []Member    `json:"members,omitempty"`
	Servers  []ServerDTO `json:"servers"`
	Groups   []GroupDTO  `json:"groups"`
	Problems []string    `json:"problems,omitempty"`
}

func rankName(rank int) string {
	for name, value := range roleRanks {
		if value == rank {
			return name
		}
	}
	return ""
}

// serverDTOs ergänzt die Zeilen um den Zustand von den Nodes (parallel pro Node durch den Zwischenspeicher).
func (p *Panel) serverDTOs(ctx context.Context, rows []ServerRow) []ServerDTO {
	nodes := map[string]Node{}
	if list, err := p.store.ListNodes(); err == nil {
		for _, n := range list {
			nodes[n.ID] = n
		}
	}
	views := map[string]map[string]node.View{}
	failed := map[string]bool{}
	result := make([]ServerDTO, 0, len(rows))
	for _, row := range rows {
		if _, done := views[row.NodeID]; !done && !failed[row.NodeID] {
			v, err := p.nodeViews(ctx, row.NodeID)
			if err != nil {
				failed[row.NodeID] = true
			} else {
				views[row.NodeID] = v
			}
		}
		n := nodes[row.NodeID]
		dto := ServerDTO{ServerRow: row, NodeName: n.Name, PublicHost: n.PublicHost, Online: !failed[row.NodeID]}
		if view, ok := views[row.NodeID][row.ID]; ok {
			view := view
			dto.View = &view
			dto.Address, dto.VoiceAddress = publicAddresses(n.PublicHost, row, view)
		}
		result = append(result, dto)
	}
	return result
}

func publicAddresses(host string, row ServerRow, view node.View) (address, voice string) {
	if host == "" {
		return "", ""
	}
	info, _ := software.Lookup(software.ID(row.Software))
	for _, mapping := range view.Ports {
		if mapping.Host == 0 {
			continue
		}
		switch {
		case mapping.Protocol == "tcp" && mapping.Container == info.DefaultPort && address == "":
			address = hostPort(host, mapping.Host)
		case mapping.Protocol == "udp" && (mapping.Container == voicePort || row.Role == RoleProxy && mapping.Container == proxyPort):
			voice = hostPort(host, mapping.Host)
		}
	}
	return address, voice
}

func hostPort(host string, port int) string {
	if port == 25565 {
		return host
	}
	return host + ":" + itoa(port)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}

func (p *Panel) projectDTO(ctx context.Context, project Project, rank int, full bool) (ProjectDTO, error) {
	rows, err := p.store.ListServers(project.ID)
	if err != nil {
		return ProjectDTO{}, err
	}
	dto := ProjectDTO{Project: project, MyRole: rankName(rank), Servers: p.serverDTOs(ctx, rows)}
	if dto.Groups, err = p.groupDTOs(ctx, project.ID, dto.Servers); err != nil {
		return ProjectDTO{}, err
	}
	if full {
		if dto.Members, err = p.store.ListMembers(project.ID); err != nil {
			return ProjectDTO{}, err
		}
		if project.Kind == "network" {
			net := make([]NetServer, len(rows))
			for i, row := range rows {
				net[i] = NetServer{Row: row}
			}
			dto.Problems = NetworkProblems(net)
		}
	}
	return dto, nil
}

func (p *Panel) listProjects(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	projects, err := p.store.ListProjects(user.ID, user.IsAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	result := make([]ProjectDTO, 0, len(projects))
	for _, project := range projects {
		rank, err := p.projectRank(user, project.ID)
		if err != nil {
			continue
		}
		dto, err := p.projectDTO(r.Context(), project, rank, false)
		if err != nil {
			writeError(w, err)
			return
		}
		result = append(result, dto)
	}
	writeJSON(w, http.StatusOK, result)
}

func (p *Panel) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Description string `json:"description"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 48 {
		writeError(w, fail(http.StatusBadRequest, "der Name braucht 1 bis 48 Zeichen"))
		return
	}
	if body.Kind != "server" && body.Kind != "network" {
		writeError(w, fail(http.StatusBadRequest, "die Art muss server oder network sein"))
		return
	}
	if len(body.Description) > 500 {
		writeError(w, fail(http.StatusBadRequest, "die Beschreibung ist zu lang"))
		return
	}
	secret, err := randomToken()
	if err != nil {
		writeError(w, err)
		return
	}
	user := userFrom(r.Context())
	project, err := p.store.CreateProject(Project{Name: body.Name, Kind: body.Kind, Description: strings.TrimSpace(body.Description), Secret: secret[:24], OwnerID: user.ID})
	if err != nil {
		writeError(w, err)
		return
	}
	dto, err := p.projectDTO(r.Context(), project, rankOwner, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

func (p *Panel) getProject(w http.ResponseWriter, r *http.Request) {
	project, rank, err := p.requireProject(r, r.PathValue("id"), rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	dto, err := p.projectDTO(r.Context(), project, rank, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (p *Panel) updateProject(w http.ResponseWriter, r *http.Request) {
	project, rank, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Name != nil {
		project.Name = strings.TrimSpace(*body.Name)
	}
	if body.Description != nil {
		project.Description = strings.TrimSpace(*body.Description)
	}
	if project.Name == "" || len(project.Name) > 48 || len(project.Description) > 500 {
		writeError(w, fail(http.StatusBadRequest, "Name oder Beschreibung sind ungültig"))
		return
	}
	if err := p.store.UpdateProject(project); err != nil {
		writeError(w, err)
		return
	}
	dto, err := p.projectDTO(r.Context(), project, rank, true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (p *Panel) deleteProject(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankOwner)
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := p.store.ListServers(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	force := r.URL.Query().Get("force") == "1"
	groups, _ := p.store.ListGroups(project.ID)
	for _, group := range groups { // der Autoscaler darf nichts nachstarten
		_ = p.store.DeleteGroup(group.ID)
	}
	for _, row := range rows {
		if err := p.removeFromNode(r.Context(), row); err != nil && !force {
			writeError(w, fail(http.StatusBadGateway, "„"+row.Name+"“ ließ sich auf dem Node nicht löschen: "+err.Error()+" (mit force=1 trotzdem entfernen)"))
			return
		}
	}
	for _, row := range rows {
		_ = p.store.DeleteServerMetrics(row.ID)
	}
	if err := p.store.DeleteProject(project.ID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeFromNode löscht den Server auf dem Node (ein schon fehlender gilt als gelöscht).
func (p *Panel) removeFromNode(ctx context.Context, row ServerRow) error {
	client, _, err := p.client(row.NodeID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err = client.Do(ctx, http.MethodDelete, "/api/servers/"+row.ID, nil, nil)
	var nodeErr *NodeError
	if errors.As(err, &nodeErr) && nodeErr.Status == http.StatusNotFound {
		err = nil
	}
	p.invalidate(row.NodeID)
	return err
}

// ---------------------------------------------------------------- Mitglieder

func (p *Panel) setMember(w http.ResponseWriter, r *http.Request) {
	project, rank, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	want, ok := roleRanks[body.Role]
	if !ok {
		writeError(w, fail(http.StatusBadRequest, "die Rolle muss viewer, operator, admin oder owner sein"))
		return
	}
	if want > rank {
		writeError(w, fail(http.StatusForbidden, "du kannst keine höhere Rolle vergeben, als du selbst hast"))
		return
	}
	target, err := p.store.UserByID(r.PathValue("user"))
	if err != nil {
		writeError(w, fail(http.StatusNotFound, "den Benutzer gibt es nicht"))
		return
	}
	if current, err := p.store.MemberRole(project.ID, target.ID); err == nil {
		if roleRanks[current] > rank {
			writeError(w, fail(http.StatusForbidden, "du kannst die Rolle eines höher gestellten Mitglieds nicht ändern"))
			return
		}
		if current == "owner" && body.Role != "owner" {
			if owners, _ := p.store.CountOwners(project.ID); owners <= 1 {
				writeError(w, fail(http.StatusConflict, "ein Projekt braucht mindestens einen Eigentümer"))
				return
			}
		}
	}
	if err := p.store.SetMember(project.ID, target.ID, body.Role); err != nil {
		writeError(w, err)
		return
	}
	members, err := p.store.ListMembers(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (p *Panel) removeMember(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	targetID := r.PathValue("user")
	// Jeder darf selbst gehen; andere entfernen setzt Admin voraus
	minimum := rankAdmin
	if targetID == user.ID {
		minimum = rankViewer
	}
	project, rank, err := p.requireProject(r, r.PathValue("id"), minimum)
	if err != nil {
		writeError(w, err)
		return
	}
	role, err := p.store.MemberRole(project.ID, targetID)
	if err != nil {
		writeError(w, errNotFound)
		return
	}
	if roleRanks[role] > rank {
		writeError(w, fail(http.StatusForbidden, "du kannst kein höher gestelltes Mitglied entfernen"))
		return
	}
	if role == "owner" {
		if owners, _ := p.store.CountOwners(project.ID); owners <= 1 {
			writeError(w, fail(http.StatusConflict, "ein Projekt braucht mindestens einen Eigentümer"))
			return
		}
	}
	if err := p.store.RemoveMember(project.ID, targetID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
