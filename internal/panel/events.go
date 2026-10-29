package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
)

// Arten von Ereignissen, auf die Webhooks hören können.
var eventKinds = []string{"crash", "restart", "crashloop", "backup", "backup_failed", "schedule_failed", "node_offline", "node_online", "lowmemory", "scale"}

// defaultHookEvents sind die Ereignisse, die ein neuer Webhook bekommt, wenn nichts gewählt ist.
var defaultHookEvents = []string{"crash", "crashloop", "backup_failed", "schedule_failed", "node_offline", "lowmemory"}

const (
	watchEvery  = 10 * time.Second
	lowMemoryMB = 1536
	eventKeep   = 30 * 24 * time.Hour
)

// ---------------------------------------------------------------- Ereignisse und Benachrichtigungen

// emit trägt ein Ereignis in den Verlauf des Projekts ein und schickt es an die passenden Webhooks.
func (p *Panel) emit(projectID, serverID, kind, level, message string) {
	if projectID == "" {
		return
	}
	event, err := p.store.AddEvent(Event{ProjectID: projectID, ServerID: serverID, Kind: kind, Level: level, Message: message})
	if err != nil {
		p.log.Warn("Ereignis nicht gespeichert", "error", err)
		return
	}
	go p.notify(event)
}

func (p *Panel) notify(event Event) {
	hooks, err := p.store.ListWebhooks(event.ProjectID)
	if err != nil {
		return
	}
	project, _ := p.store.ProjectByID(event.ProjectID)
	server := ""
	if event.ServerID != "" {
		if row, err := p.store.ServerByID(event.ServerID); err == nil {
			server = row.Name
		}
	}
	for _, hook := range hooks {
		if !hook.Enabled || !slices.Contains(hook.Events, event.Kind) {
			continue
		}
		status := p.deliver(hook, event, project.Name, server)
		_ = p.store.SetWebhookStatus(hook.ID, status)
	}
}

// webhookBody baut den Inhalt für Discord oder ein allgemeines JSON.
func webhookBody(hook Webhook, event Event, project, server string) ([]byte, error) {
	at := time.UnixMilli(event.Time).UTC().Format(time.RFC3339)
	if hook.Kind == "discord" {
		color := 0x3b82f6
		switch event.Level {
		case "error":
			color = 0xef4444
		case "warn":
			color = 0xf59e0b
		}
		title := titles[event.Kind]
		if title == "" {
			title = event.Kind
		}
		if server != "" {
			title += " · " + server
		}
		return json.Marshal(map[string]any{
			"username": "Lodestone",
			"embeds": []map[string]any{{
				"title": title, "description": event.Message, "color": color, "timestamp": at,
				"footer": map[string]string{"text": project},
			}},
		})
	}
	return json.Marshal(map[string]any{"event": event.Kind, "level": event.Level, "project": project, "server": server, "message": event.Message, "time": at})
}

var titles = map[string]string{
	"crash": "Absturz", "restart": "Neustart", "crashloop": "Absturzschleife", "backup": "Backup", "backup_failed": "Backup fehlgeschlagen",
	"schedule_failed": "Zeitplan fehlgeschlagen", "node_offline": "Node nicht erreichbar", "node_online": "Node wieder erreichbar",
	"lowmemory": "Wenig Arbeitsspeicher", "scale": "Skalierung", "test": "Test",
}

func (p *Panel) deliver(hook Webhook, event Event, project, server string) string {
	body, err := webhookBody(hook, event, project, server)
	if err != nil {
		return "Fehler: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return "Fehler: " + err.Error()
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Lodestone")
	response, err := p.hookClient.Do(request)
	if err != nil {
		return "Fehler: " + shorten(err.Error())
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode >= 300 {
		return fmt.Sprintf("Fehler: Antwort %d", response.StatusCode)
	}
	return "OK " + time.Now().Format("02.01. 15:04")
}

func shorten(text string) string {
	if len(text) > 160 {
		return text[:160]
	}
	return text
}

// ---------------------------------------------------------------- Schutz vor Zugriffen auf das interne Netz

func isInternalIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		if v4[0] == 0 || (v4[0] == 100 && v4[1]&0xc0 == 64) { // 0.0.0.0/8 und CGNAT 100.64.0.0/10
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// newHookClient baut den HTTP-Client für Webhooks. Er verbindet sich nie mit internen Adressen (auch nicht, wenn ein
// Name auf eine solche zeigt oder nach einer Weiterleitung), außer allowInternal ist gesetzt. Geprüft wird die Adresse,
// mit der wirklich verbunden wird.
func newHookClient(allowInternal bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 8 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowInternal {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || isInternalIP(ip) {
				return fmt.Errorf("Verbindungen ins interne Netz sind für Webhooks gesperrt (%s)", host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:       12 * time.Second,
		Transport:     &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 8 * time.Second, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ---------------------------------------------------------------- Überwachung

type watcher struct {
	exitSeen map[string]int64
	crashes  map[string][]time.Time
	nodeUp   map[string]bool
	lowMem   map[string]time.Time
	lastMem  time.Time
}

func newWatcher() *watcher {
	return &watcher{exitSeen: map[string]int64{}, crashes: map[string][]time.Time{}, nodeUp: map[string]bool{}, lowMem: map[string]time.Time{}}
}

// watchLoop erkennt Abstürze, Node-Ausfälle und knappen Speicher und reagiert darauf.
func (p *Panel) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(watchEvery)
	defer ticker.Stop()
	purge := time.NewTicker(6 * time.Hour)
	defer purge.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-purge.C:
			_ = p.store.PurgeEvents(time.Now().Add(-eventKeep).UnixMilli())
			_ = p.store.PurgeAudit(time.Now().Add(-90 * 24 * time.Hour).UnixMilli())
		case <-ticker.C:
			p.watchOnce(ctx)
		}
	}
}

func (p *Panel) watchOnce(ctx context.Context) {
	rows, err := p.store.ListServers("")
	if err != nil || len(rows) == 0 {
		return
	}
	w := p.watch
	byNode := map[string][]ServerRow{}
	for _, row := range rows {
		byNode[row.NodeID] = append(byNode[row.NodeID], row)
	}
	nodes, _ := p.store.ListNodes()
	names := map[string]string{}
	for _, n := range nodes {
		names[n.ID] = n.Name
	}
	for nodeID, list := range byNode {
		views, err := p.nodeViews(ctx, nodeID)
		up := err == nil
		if was, known := w.nodeUp[nodeID]; known && was != up {
			kind, level, text := "node_offline", "error", "Der Node „"+names[nodeID]+"“ ist nicht erreichbar."
			if up {
				kind, level, text = "node_online", "info", "Der Node „"+names[nodeID]+"“ ist wieder erreichbar."
			}
			for _, projectID := range distinctProjects(list) {
				p.emit(projectID, "", kind, level, text)
			}
		}
		w.nodeUp[nodeID] = up
		if !up {
			continue
		}
		for _, row := range list {
			view, ok := views[row.ID]
			if !ok {
				continue
			}
			var at int64
			if view.LastExit != nil {
				at = view.LastExit.At
			}
			seen, known := w.exitSeen[row.ID]
			w.exitSeen[row.ID] = max(seen, at)
			if known && view.LastExit != nil && at > seen && view.LastExit.Code != 0 && !view.LastExit.Requested {
				p.handleCrash(ctx, row, view)
			}
		}
		p.checkMemory(ctx, nodeID, names[nodeID], list)
	}
}

func distinctProjects(rows []ServerRow) []string {
	seen := map[string]bool{}
	list := []string{}
	for _, row := range rows {
		if !seen[row.ProjectID] {
			seen[row.ProjectID] = true
			list = append(list, row.ProjectID)
		}
	}
	return list
}

func (p *Panel) handleCrash(ctx context.Context, row ServerRow, view node.View) {
	p.emit(row.ProjectID, row.ID, "crash", "error", fmt.Sprintf("%s ist abgestürzt (Exit-Code %d).", row.Name, view.LastExit.Code))
	if row.GroupID != "" {
		return // Instanzen einer Gruppe startet der Autoscaler
	}
	if !p.store.AutoRestart(row.ID) {
		return
	}
	w := p.watch
	now := time.Now()
	recent := w.crashes[row.ID][:0:0]
	for _, at := range w.crashes[row.ID] {
		if now.Sub(at) < restartWindow {
			recent = append(recent, at)
		}
	}
	if len(recent) >= maxRestarts {
		w.crashes[row.ID] = recent
		p.emit(row.ProjectID, row.ID, "crashloop", "error", fmt.Sprintf("%s stürzt immer wieder ab – der Auto-Restart gibt nach %d Versuchen auf. Schau in die Konsole.", row.Name, maxRestarts))
		return
	}
	w.crashes[row.ID] = append(recent, now)
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return
	}
	go func() {
		time.Sleep(startDelay)
		callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.Do(callCtx, http.MethodPost, "/api/servers/"+row.ID+"/power", map[string]string{"action": "start"}, nil); err != nil {
			p.emit(row.ProjectID, row.ID, "restart", "error", fmt.Sprintf("%s ließ sich nach dem Absturz nicht neu starten: %v", row.Name, err))
			return
		}
		p.invalidate(row.NodeID)
		p.emit(row.ProjectID, row.ID, "restart", "warn", fmt.Sprintf("%s wurde nach dem Absturz neu gestartet (Versuch %d von %d).", row.Name, len(recent)+1, maxRestarts))
	}()
}

// startDelay ist die Pause zwischen Absturz und Neustart (in Tests kürzer).
var startDelay = 3 * time.Second

// checkMemory meldet knappen Arbeitsspeicher eines Nodes (höchstens alle 30 Minuten, nur einmal pro Minute geprüft).
func (p *Panel) checkMemory(ctx context.Context, nodeID, name string, list []ServerRow) {
	w := p.watch
	if time.Since(w.lastMem) < time.Minute {
		return
	}
	client, _, err := p.client(nodeID)
	if err != nil {
		return
	}
	var system struct {
		MemoryAvailable int64 `json:"memoryAvailable"`
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if client.Do(callCtx, http.MethodGet, "/api/system", nil, &system) != nil || system.MemoryAvailable == 0 {
		return
	}
	if system.MemoryAvailable>>20 >= lowMemoryMB || time.Since(w.lowMem[nodeID]) < 30*time.Minute {
		return
	}
	w.lowMem[nodeID] = time.Now()
	for _, projectID := range distinctProjects(list) {
		p.emit(projectID, "", "lowmemory", "warn", fmt.Sprintf("Auf dem Node „%s“ sind nur noch %d MB Arbeitsspeicher frei. Neue Server lassen sich dort bald nicht mehr starten.", name, system.MemoryAvailable>>20))
	}
}

// ---------------------------------------------------------------- Schnittstelle

func (p *Panel) listEvents(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	events, err := p.store.ListEvents(project.ID, 100)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

type webhookRequest struct {
	Name    *string   `json:"name"`
	URL     *string   `json:"url"`
	Kind    *string   `json:"kind"`
	Events  *[]string `json:"events"`
	Enabled *bool     `json:"enabled"`
}

func (p *Panel) applyWebhook(hook *Webhook, body webhookRequest) error {
	if body.Name != nil {
		hook.Name = strings.TrimSpace(*body.Name)
	}
	if body.URL != nil {
		hook.URL = strings.TrimSpace(*body.URL)
	}
	if body.Kind != nil {
		hook.Kind = *body.Kind
	}
	if body.Events != nil {
		hook.Events = *body.Events
	}
	if body.Enabled != nil {
		hook.Enabled = *body.Enabled
	}
	if hook.Name == "" || len(hook.Name) > 48 {
		return fail(http.StatusBadRequest, "der Name braucht 1 bis 48 Zeichen")
	}
	if hook.Kind != "discord" && hook.Kind != "generic" {
		return fail(http.StatusBadRequest, "die Art muss discord oder generic sein")
	}
	address, err := url.Parse(hook.URL)
	if err != nil || address.Host == "" || len(hook.URL) > 500 || (address.Scheme != "https" && !(address.Scheme == "http" && p.cfg.AllowInternalWebhooks)) {
		return fail(http.StatusBadRequest, "die Adresse muss mit https:// beginnen")
	}
	if len(hook.Events) == 0 {
		hook.Events = append([]string{}, defaultHookEvents...)
	}
	for _, kind := range hook.Events {
		if !slices.Contains(eventKinds, kind) {
			return fail(http.StatusBadRequest, "unbekanntes Ereignis: "+kind)
		}
	}
	return nil
}

func (p *Panel) listWebhooks(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	hooks, err := p.store.ListWebhooks(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (p *Panel) createWebhook(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body webhookRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	existing, _ := p.store.ListWebhooks(project.ID)
	if len(existing) >= 10 {
		writeError(w, fail(http.StatusConflict, "höchstens 10 Webhooks pro Projekt"))
		return
	}
	hook := Webhook{ProjectID: project.ID, Kind: "discord", Enabled: true}
	if err := p.applyWebhook(&hook, body); err != nil {
		writeError(w, err)
		return
	}
	created, err := p.store.CreateWebhook(hook)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (p *Panel) requireWebhook(r *http.Request) (Webhook, error) {
	hook, err := p.store.WebhookByID(r.PathValue("id"))
	if err != nil {
		return Webhook{}, errNotFound
	}
	if _, _, err := p.requireProject(r, hook.ProjectID, rankAdmin); err != nil {
		return Webhook{}, err
	}
	return hook, nil
}

func (p *Panel) updateWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := p.requireWebhook(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body webhookRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := p.applyWebhook(&hook, body); err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.UpdateWebhook(hook); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (p *Panel) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := p.requireWebhook(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.DeleteWebhook(hook.ID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testWebhook schickt eine Testnachricht und meldet, was angekommen ist.
func (p *Panel) testWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := p.requireWebhook(r)
	if err != nil {
		writeError(w, err)
		return
	}
	project, _ := p.store.ProjectByID(hook.ProjectID)
	status := p.deliver(hook, Event{Kind: "test", Level: "info", Message: "Das ist eine Testnachricht von Lodestone.", Time: time.Now().UnixMilli()}, project.Name, "")
	_ = p.store.SetWebhookStatus(hook.ID, status)
	writeJSON(w, http.StatusOK, map[string]any{"ok": strings.HasPrefix(status, "OK"), "status": status})
}

func (p *Panel) putAutoRestart(w http.ResponseWriter, r *http.Request) {
	row, _, _, err := p.requireServer(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.SetAutoRestart(row.ID, body.Enabled); err != nil {
		writeError(w, err)
		return
	}
	row.AutoRestart = body.Enabled
	writeJSON(w, http.StatusOK, p.serverDTOs(r.Context(), []ServerRow{row})[0])
}
