package panel

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

type nodeDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	PublicHost  string `json:"publicHost"`
	Online      bool   `json:"online"`
	Error       string `json:"error,omitempty"`
	System      any    `json:"system,omitempty"`
	ServerCount int    `json:"serverCount"`
	Version     string `json:"version,omitempty"`
	// MemoryAvailable ist der freie Arbeitsspeicher der Maschine in Bytes.
	MemoryAvailable int64 `json:"memoryAvailable,omitempty"`
	// MemoryCommitted ist der Arbeitsspeicher, den die laufenden Server auf dem Node als Limit haben (Bytes).
	MemoryCommitted int64 `json:"memoryCommitted,omitempty"`
}

func (p *Panel) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := p.store.ListNodes()
	if err != nil {
		writeError(w, err)
		return
	}
	admin := userFrom(r.Context()).IsAdmin
	result := make([]nodeDTO, len(nodes))
	var group sync.WaitGroup
	for i, n := range nodes {
		result[i] = nodeDTO{ID: n.ID, Name: n.Name, PublicHost: n.PublicHost}
		if admin {
			result[i].URL = n.URL
		}
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			var system struct {
				Version         string `json:"version"`
				Docker          any    `json:"docker"`
				Error           string `json:"dockerError"`
				DataDir         string `json:"dataDir"`
				MemoryAvailable int64  `json:"memoryAvailable"`
			}
			if err := p.nodeClient(n).Do(ctx, http.MethodGet, "/api/system", nil, &system); err != nil {
				result[i].Error = err.Error()
				return
			}
			result[i].Online, result[i].System, result[i].Version, result[i].MemoryAvailable = true, system.Docker, system.Version, system.MemoryAvailable
			if views, err := p.nodeViews(ctx, n.ID); err == nil {
				for _, view := range views {
					if activeState(view.State) {
						result[i].MemoryCommitted += int64(view.MemoryMB) << 20
					}
				}
			}
			if system.Error != "" {
				result[i].Online, result[i].Error = false, "Docker: "+system.Error
			}
		}()
	}
	group.Wait()
	for i, n := range nodes {
		if count, err := p.store.CountServersOnNode(n.ID); err == nil {
			result[i].ServerCount = count
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type nodeBody struct {
	Name       *string `json:"name"`
	URL        *string `json:"url"`
	Token      *string `json:"token"`
	PublicHost *string `json:"publicHost"`
}

func validNodeURL(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func (p *Panel) createNode(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body nodeBody
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	n := Node{}
	if body.Name != nil {
		n.Name = strings.TrimSpace(*body.Name)
	}
	if body.URL != nil {
		n.URL = strings.TrimRight(strings.TrimSpace(*body.URL), "/")
	}
	if body.Token != nil {
		n.Token = strings.TrimSpace(*body.Token)
	}
	if body.PublicHost != nil {
		n.PublicHost = strings.TrimSpace(*body.PublicHost)
	}
	if n.Name == "" || len(n.Name) > 48 {
		writeError(w, fail(http.StatusBadRequest, "der Name braucht 1 bis 48 Zeichen"))
		return
	}
	if !validNodeURL(n.URL) || n.Token == "" {
		writeError(w, fail(http.StatusBadRequest, "Adresse (http://…) und Token des Nodes sind nötig"))
		return
	}
	// Erreichbarkeit und Token gleich prüfen, damit Tippfehler auffallen
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := p.nodeClient(n).Do(ctx, http.MethodGet, "/api/system", nil, nil); err != nil {
		writeError(w, fail(http.StatusBadGateway, "der Node antwortet nicht oder lehnt das Token ab: "+err.Error()))
		return
	}
	created, err := p.store.CreateNode(n)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, nodeDTO{ID: created.ID, Name: created.Name, URL: created.URL, PublicHost: created.PublicHost})
}

func (p *Panel) updateNode(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body nodeBody
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	n, err := p.store.NodeByID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if body.Name != nil {
		n.Name = strings.TrimSpace(*body.Name)
	}
	if body.URL != nil {
		n.URL = strings.TrimRight(strings.TrimSpace(*body.URL), "/")
	}
	if body.Token != nil && strings.TrimSpace(*body.Token) != "" {
		n.Token = strings.TrimSpace(*body.Token)
	}
	if body.PublicHost != nil {
		n.PublicHost = strings.TrimSpace(*body.PublicHost)
	}
	if n.Name == "" || len(n.Name) > 48 || !validNodeURL(n.URL) {
		writeError(w, fail(http.StatusBadRequest, "Name und Adresse sind ungültig"))
		return
	}
	if err := p.store.UpdateNode(n); err != nil {
		writeError(w, err)
		return
	}
	p.invalidate(n.ID)
	writeJSON(w, http.StatusOK, nodeDTO{ID: n.ID, Name: n.Name, URL: n.URL, PublicHost: n.PublicHost})
}

func (p *Panel) deleteNode(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	id := r.PathValue("id")
	count, err := p.store.CountServersOnNode(id)
	if err != nil {
		writeError(w, err)
		return
	}
	if count > 0 {
		writeError(w, fail(http.StatusConflict, "auf dem Node laufen noch "+strconv.Itoa(count)+" Server – lösche sie zuerst"))
		return
	}
	if err := p.store.DeleteNode(id); err != nil {
		writeError(w, err)
		return
	}
	p.invalidate(id)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- Software

func (p *Panel) listSoftware(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, software.Catalog())
}

type versionsEntry struct {
	at       time.Time
	versions []string
}

var (
	versionsMu    sync.Mutex
	versionsCache = map[string]versionsEntry{}
)

func (p *Panel) softwareVersions(w http.ResponseWriter, r *http.Request) {
	id := software.ID(r.PathValue("id"))
	if _, ok := software.Lookup(id); !ok {
		writeError(w, errNotFound)
		return
	}
	prerelease := r.URL.Query().Get("prerelease") == "1"
	key := string(id) + "|" + strconv.FormatBool(prerelease)
	versionsMu.Lock()
	entry, ok := versionsCache[key]
	versionsMu.Unlock()
	if ok && time.Since(entry.at) < 10*time.Minute {
		writeJSON(w, http.StatusOK, entry.versions)
		return
	}
	versions, err := p.sw.Versions(r.Context(), id, prerelease)
	if err != nil {
		if ok {
			writeJSON(w, http.StatusOK, entry.versions) // lieber alte Liste als nichts
			return
		}
		writeError(w, fail(http.StatusBadGateway, "Der Anbieter ist nicht erreichbar: "+err.Error()))
		return
	}
	versionsMu.Lock()
	versionsCache[key] = versionsEntry{at: time.Now(), versions: versions}
	versionsMu.Unlock()
	writeJSON(w, http.StatusOK, versions)
}
