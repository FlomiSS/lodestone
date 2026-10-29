package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

// Config ist die Einstellung des Panels.
type Config struct {
	Listen string
	// SecureCookies setzt das Secure-Flag am Sitzungs-Cookie (hinter HTTPS).
	SecureCookies bool
	// TrustProxy vertraut X-Forwarded-For für die IP bei der Anmelde-Drosselung.
	TrustProxy bool
	Version    string
}

// Panel ist das Backend: HTTP-Schnittstelle, Datenbank und die Verbindung zu den Nodes.
type Panel struct {
	cfg      Config
	store    *Store
	log      *slog.Logger
	throttle *throttle
	sw       *software.Resolver
	modrinth *Modrinth
	web      http.Handler

	viewMu sync.Mutex
	views  map[string]viewEntry

	liveMu sync.Mutex
	live   map[string]liveInfo

	// preparing sind neue Instanzen, die noch ihre Plugins bekommen und gleich gestartet werden; sie zählen als aktiv
	preparing sync.Map

	scaleMu   sync.Mutex
	scaleLast map[string]time.Time   // letzte Skalierung je Gruppe
	idleSince map[string]time.Time   // seit wann eine Instanz leer ist
	restarts  map[string][]time.Time // Neustart-Versuche abgestürzter Instanzen

	// nodeClient lässt sich in Tests ersetzen.
	nodeClient func(Node) *NodeClient
}

type viewEntry struct {
	at    time.Time
	views map[string]node.View
	err   error
}

// New baut das Panel. web ist die Oberfläche (kann nil sein).
func New(cfg Config, store *Store, logger *slog.Logger, web http.Handler) *Panel {
	return &Panel{
		cfg: cfg, store: store, log: logger, web: web,
		throttle: newThrottle(), sw: software.NewResolver(), modrinth: NewModrinth(),
		views: map[string]viewEntry{}, nodeClient: newNodeClient,
		live: map[string]liveInfo{}, scaleLast: map[string]time.Time{}, idleSince: map[string]time.Time{}, restarts: map[string][]time.Time{},
	}
}

// Handler liefert alle Routen.
func (p *Panel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) })

	// Anmeldung
	mux.HandleFunc("GET /api/auth/status", p.authStatus)
	mux.HandleFunc("POST /api/auth/setup", p.setup)
	mux.HandleFunc("POST /api/auth/login", p.login)
	mux.HandleFunc("POST /api/auth/logout", p.authenticate(p.logout))
	mux.HandleFunc("GET /api/me", p.authenticate(p.me))
	mux.HandleFunc("POST /api/me/password", p.authenticate(p.changeOwnPassword))

	// Benutzer
	mux.HandleFunc("GET /api/users", p.authenticate(p.listUsers))
	mux.HandleFunc("POST /api/users", p.authenticate(p.createUser))
	mux.HandleFunc("PATCH /api/users/{id}", p.authenticate(p.updateUser))
	mux.HandleFunc("DELETE /api/users/{id}", p.authenticate(p.deleteUser))
	mux.HandleFunc("POST /api/users/{id}/password", p.authenticate(p.resetPassword))

	// Nodes und Software
	mux.HandleFunc("GET /api/nodes", p.authenticate(p.listNodes))
	mux.HandleFunc("POST /api/nodes", p.authenticate(p.createNode))
	mux.HandleFunc("PATCH /api/nodes/{id}", p.authenticate(p.updateNode))
	mux.HandleFunc("DELETE /api/nodes/{id}", p.authenticate(p.deleteNode))
	mux.HandleFunc("GET /api/nodes/{id}/playit", p.authenticate(p.nodePlayit))
	mux.HandleFunc("PUT /api/nodes/{id}/playit", p.authenticate(p.nodePlayit))
	mux.HandleFunc("DELETE /api/nodes/{id}/playit", p.authenticate(p.nodePlayit))
	mux.HandleFunc("GET /api/modpacks/search", p.authenticate(p.modpackSearch))
	mux.HandleFunc("GET /api/modpacks/{project}", p.authenticate(p.modpackVersions))
	mux.HandleFunc("GET /api/software", p.authenticate(p.listSoftware))
	mux.HandleFunc("GET /api/software/{id}/versions", p.authenticate(p.softwareVersions))

	// Projekte
	mux.HandleFunc("GET /api/projects", p.authenticate(p.listProjects))
	mux.HandleFunc("POST /api/projects", p.authenticate(p.createProject))
	mux.HandleFunc("GET /api/projects/{id}", p.authenticate(p.getProject))
	mux.HandleFunc("PATCH /api/projects/{id}", p.authenticate(p.updateProject))
	mux.HandleFunc("DELETE /api/projects/{id}", p.authenticate(p.deleteProject))
	mux.HandleFunc("PUT /api/projects/{id}/members/{user}", p.authenticate(p.setMember))
	mux.HandleFunc("DELETE /api/projects/{id}/members/{user}", p.authenticate(p.removeMember))
	mux.HandleFunc("POST /api/projects/{id}/servers", p.authenticate(p.createServer))
	mux.HandleFunc("PUT /api/projects/{id}/network", p.authenticate(p.putNetwork))
	mux.HandleFunc("POST /api/projects/{id}/network/sync", p.authenticate(p.syncNetworkNow))
	mux.HandleFunc("GET /api/projects/{id}/metrics", p.authenticate(p.projectMetrics))
	mux.HandleFunc("PUT /api/projects/{id}/voicechat", p.authenticate(p.putVoicechat))
	mux.HandleFunc("POST /api/projects/{id}/groups", p.authenticate(p.createGroup))
	mux.HandleFunc("PATCH /api/groups/{id}", p.authenticate(p.updateGroup))
	mux.HandleFunc("DELETE /api/groups/{id}", p.authenticate(p.deleteGroup))
	mux.HandleFunc("POST /api/groups/{id}/instances", p.authenticate(p.addInstance))

	// Server
	mux.HandleFunc("GET /api/servers/{id}", p.authenticate(p.getServer))
	mux.HandleFunc("PATCH /api/servers/{id}", p.authenticate(p.patchServer))
	mux.HandleFunc("DELETE /api/servers/{id}", p.authenticate(p.deleteServer))
	mux.HandleFunc("GET /api/servers/{id}/ws", p.authenticate(p.consoleSocket))
	mux.HandleFunc("GET /api/servers/{id}/metrics", p.authenticate(p.serverMetrics))
	mux.HandleFunc("GET /api/servers/{id}/player-history", p.authenticate(p.serverPlayerHistory))
	mux.HandleFunc("GET /api/servers/{id}/modrinth/search", p.authenticate(p.modrinthSearch))
	mux.HandleFunc("GET /api/servers/{id}/modrinth/project/{project}", p.authenticate(p.modrinthProject))
	mux.HandleFunc("POST /api/servers/{id}/modrinth/install", p.authenticate(p.modrinthInstall))
	mux.HandleFunc("GET /api/servers/{id}/addons/details", p.authenticate(p.addonsDetails))
	mux.HandleFunc("POST /api/servers/{id}/addons/switch", p.authenticate(p.addonsSwitch))
	mux.HandleFunc("PUT /api/servers/{id}/public-address", p.authenticate(p.putPublicAddress))
	mux.HandleFunc("POST /api/servers/{id}/addons/check", p.authenticate(p.addonsCheck))
	mux.HandleFunc("POST /api/servers/{id}/addons/update", p.authenticate(p.addonsUpdate))
	mux.HandleFunc("/api/servers/{id}/{rest...}", p.authenticate(p.proxyServer))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unbekannte Adresse"})
	})
	if p.web != nil {
		mux.Handle("/", p.web)
	}
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			header.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------- Hilfen

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func fail(status int, message string) error { return &httpError{status, message} }

var (
	errForbidden = fail(http.StatusForbidden, "dafür fehlt dir die Berechtigung")
	errNotFound  = fail(http.StatusNotFound, "nicht gefunden")
)

func writeError(w http.ResponseWriter, err error) {
	var httpErr *httpError
	var nodeErr *NodeError
	switch {
	case errors.As(err, &httpErr):
		writeJSON(w, httpErr.status, map[string]string{"error": httpErr.message})
	case errors.As(err, &nodeErr):
		status := nodeErr.Status
		if status == http.StatusUnauthorized {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]string{"error": nodeErr.Message})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "nicht gefunden"})
	case errors.Is(err, ErrExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNodeOffline):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	case errors.Is(err, fs.ErrNotExist):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "nicht gefunden"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "interner Fehler: " + err.Error()})
	}
}

func decode(r *http.Request, into any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fail(http.StatusBadRequest, "ungültige Anfrage: "+err.Error())
	}
	return nil
}

// ---------------------------------------------------------------- Rechte

// Rollen in einem Projekt, aufsteigend.
const (
	rankNone = iota
	rankViewer
	rankOperator
	rankAdmin
	rankOwner
)

var roleRanks = map[string]int{"viewer": rankViewer, "operator": rankOperator, "admin": rankAdmin, "owner": rankOwner}

// projectRank ist die Stufe des Benutzers im Projekt. Globale Administratoren sind überall Eigentümer.
func (p *Panel) projectRank(user User, projectID string) (int, error) {
	if user.IsAdmin {
		if _, err := p.store.ProjectByID(projectID); err != nil {
			return rankNone, err
		}
		return rankOwner, nil
	}
	role, err := p.store.MemberRole(projectID, user.ID)
	if errors.Is(err, ErrNotFound) {
		return rankNone, errNotFound // Projekte, die man nicht sieht, gibt es nicht
	}
	if err != nil {
		return rankNone, err
	}
	return roleRanks[role], nil
}

// requireProject prüft die Mindeststufe und liefert das Projekt.
func (p *Panel) requireProject(r *http.Request, projectID string, minimum int) (Project, int, error) {
	user := userFrom(r.Context())
	rank, err := p.projectRank(user, projectID)
	if err != nil {
		return Project{}, 0, err
	}
	if rank < minimum {
		return Project{}, 0, errForbidden
	}
	project, err := p.store.ProjectByID(projectID)
	return project, rank, err
}

// requireServer löst {id} auf und prüft die Mindeststufe im Projekt des Servers.
func (p *Panel) requireServer(r *http.Request, minimum int) (ServerRow, Project, int, error) {
	row, err := p.store.ServerByID(r.PathValue("id"))
	if err != nil {
		return ServerRow{}, Project{}, 0, errNotFound
	}
	project, rank, err := p.requireProject(r, row.ProjectID, minimum)
	return row, project, rank, err
}

func requireAdmin(r *http.Request) error {
	if !userFrom(r.Context()).IsAdmin {
		return fail(http.StatusForbidden, "dafür braucht es einen Administrator")
	}
	return nil
}

// ---------------------------------------------------------------- Nodes ansprechen

func (p *Panel) client(nodeID string) (*NodeClient, Node, error) {
	n, err := p.store.NodeByID(nodeID)
	if err != nil {
		return nil, Node{}, err
	}
	return p.nodeClient(n), n, nil
}

const viewTTL = 1500 * time.Millisecond

// nodeViews liefert den Zustand aller Server auf einem Node (kurz zwischengespeichert, damit Listen den Node nicht fluten).
func (p *Panel) nodeViews(ctx context.Context, nodeID string) (map[string]node.View, error) {
	p.viewMu.Lock()
	entry, ok := p.views[nodeID]
	p.viewMu.Unlock()
	if ok && time.Since(entry.at) < viewTTL {
		return entry.views, entry.err
	}
	client, _, err := p.client(nodeID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var list []node.View
	err = client.Do(ctx, http.MethodGet, "/api/servers", nil, &list)
	views := map[string]node.View{}
	for _, view := range list {
		views[view.ID] = view
	}
	p.viewMu.Lock()
	p.views[nodeID] = viewEntry{at: time.Now(), views: views, err: err}
	p.viewMu.Unlock()
	return views, err
}

func (p *Panel) invalidate(nodeID string) {
	p.viewMu.Lock()
	delete(p.views, nodeID)
	p.viewMu.Unlock()
}
