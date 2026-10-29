package node

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lodestone-mc/lodestone/internal/software"
)

const (
	maxJSONBody  = 1 << 20
	maxEditBytes = 5 << 20
)

// API ist die HTTP-Schnittstelle des Daemons. Sie ist mit einem Schlüssel gesichert (Authorization: Bearer ...).
type API struct {
	cfg     Config
	manager *Manager
	rt      Runtime
	sw      Software
	log     *slog.Logger
	version string
}

// NewAPI baut die Schnittstelle.
func NewAPI(cfg Config, manager *Manager, rt Runtime, sw Software, logger *slog.Logger, version string) *API {
	return &API{cfg: cfg, manager: manager, rt: rt, sw: sw, log: logger, version: version}
}

// Handler liefert alle Routen.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) })

	mux.HandleFunc("GET /api/system", a.system)
	mux.HandleFunc("GET /api/software", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, software.Catalog()) })
	mux.HandleFunc("GET /api/software/{id}/versions", a.softwareVersions)

	mux.HandleFunc("GET /api/playit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.manager.PlayitStatus(r.Context()))
	})
	mux.HandleFunc("PUT /api/playit", a.putPlayit)
	mux.HandleFunc("DELETE /api/playit", func(w http.ResponseWriter, r *http.Request) {
		if err := a.manager.RemovePlayit(r.Context()); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/servers", a.listServers)
	mux.HandleFunc("POST /api/servers", a.createServer)
	mux.HandleFunc("GET /api/servers/{id}", a.server(a.getServer))
	mux.HandleFunc("PATCH /api/servers/{id}", a.server(a.patchServer))
	mux.HandleFunc("DELETE /api/servers/{id}", a.deleteServer)
	mux.HandleFunc("POST /api/servers/{id}/reinstall", a.server(a.reinstall))
	mux.HandleFunc("POST /api/servers/{id}/power", a.server(a.power))
	mux.HandleFunc("POST /api/servers/{id}/command", a.server(a.command))
	mux.HandleFunc("GET /api/servers/{id}/logs", a.server(a.logs))
	mux.HandleFunc("GET /api/servers/{id}/ws", a.server(a.stream))

	mux.HandleFunc("POST /api/servers/{id}/modpack", a.server(a.installModpack))
	mux.HandleFunc("GET /api/servers/{id}/modpack", a.server(func(w http.ResponseWriter, _ *http.Request, server *Server) {
		writeJSON(w, http.StatusOK, server.ModpackStatus())
	}))
	mux.HandleFunc("GET /api/servers/{id}/addons", a.server(a.addons))
	mux.HandleFunc("GET /api/servers/{id}/players", a.server(a.players))
	mux.HandleFunc("GET /api/servers/{id}/autoconfig", a.server(a.getAutoconfig))
	mux.HandleFunc("PUT /api/servers/{id}/autoconfig", a.server(a.putAutoconfig))
	mux.HandleFunc("GET /api/servers/{id}/backups", a.server(a.listBackups))
	mux.HandleFunc("POST /api/servers/{id}/backups", a.server(a.createBackup))
	mux.HandleFunc("DELETE /api/servers/{id}/backups/{name}", a.server(a.deleteBackup))
	mux.HandleFunc("GET /api/servers/{id}/backups/{name}/download", a.server(a.downloadBackup))
	mux.HandleFunc("POST /api/servers/{id}/backups/{name}/restore", a.server(a.restoreBackup))

	mux.HandleFunc("GET /api/servers/{id}/files", a.server(a.listFiles))
	mux.HandleFunc("POST /api/servers/{id}/files/pull", a.server(a.pull))
	mux.HandleFunc("GET /api/servers/{id}/files/content", a.server(a.readFile))
	mux.HandleFunc("PUT /api/servers/{id}/files/content", a.server(a.writeFile))
	mux.HandleFunc("POST /api/servers/{id}/files/mkdir", a.server(a.mkdir))
	mux.HandleFunc("POST /api/servers/{id}/files/rename", a.server(a.rename))
	mux.HandleFunc("POST /api/servers/{id}/files/move", a.server(a.move))
	mux.HandleFunc("POST /api/servers/{id}/files/delete", a.server(a.bulkDelete))
	mux.HandleFunc("POST /api/servers/{id}/files/archive", a.server(a.archive))
	mux.HandleFunc("POST /api/servers/{id}/files/extract", a.server(a.extract))
	mux.HandleFunc("DELETE /api/servers/{id}/files", a.server(a.removeFile))
	mux.HandleFunc("POST /api/servers/{id}/files/upload", a.server(a.upload))
	mux.HandleFunc("GET /api/servers/{id}/files/download", a.server(a.download))

	return a.authenticate(mux)
}

// ---------------------------------------------------------------- Hilfen

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, fs.ErrNotExist), errors.Is(err, software.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrBadState), errors.Is(err, ErrConflict), errors.Is(err, fs.ErrExist):
		status = http.StatusConflict
	case errors.Is(err, ErrOutside):
		status = http.StatusForbidden
	case errors.Is(err, errBadRequest):
		status = http.StatusBadRequest
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "interner Fehler: " + message
	}
	writeJSON(w, status, map[string]string{"error": message})
}

var errBadRequest = errors.New("ungültige Anfrage")

func badRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errBadRequest, fmt.Sprintf(format, args...))
}

func decode(r *http.Request, into any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return badRequest("%v", err)
	}
	return nil
}

func (a *API) authenticate(next http.Handler) http.Handler {
	expected := []byte(a.cfg.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		// Browser können beim Websocket keine Header setzen; dort gilt das Token der Adresse
		if given == "" && strings.HasSuffix(r.URL.Path, "/ws") {
			given = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(given), expected) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "nicht angemeldet"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// server löst {id} auf und reicht den Server weiter.
func (a *API) server(handler func(http.ResponseWriter, *http.Request, *Server)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		server, ok := a.manager.Get(r.PathValue("id"))
		if !ok {
			writeError(w, ErrNotFound)
			return
		}
		handler(w, r, server)
	}
}

// ---------------------------------------------------------------- System und Software

func (a *API) system(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result := map[string]any{"version": a.version, "dataDir": a.cfg.DataDir, "docker": nil, "memoryAvailable": AvailableMemory()}
	info, err := a.rt.Info(ctx)
	if err != nil {
		result["dockerError"] = err.Error()
	} else {
		result["docker"] = info
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) softwareVersions(w http.ResponseWriter, r *http.Request) {
	id := software.ID(r.PathValue("id"))
	if _, ok := software.Lookup(id); !ok {
		writeError(w, ErrNotFound)
		return
	}
	versions, err := a.sw.Versions(r.Context(), id, r.URL.Query().Get("prerelease") == "1")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Der Anbieter ist nicht erreichbar: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// ---------------------------------------------------------------- Server

func (a *API) listServers(w http.ResponseWriter, _ *http.Request) {
	servers := a.manager.List()
	views := make([]View, 0, len(servers))
	for _, server := range servers {
		views = append(views, server.View())
	}
	writeJSON(w, http.StatusOK, views)
}

func (a *API) createServer(w http.ResponseWriter, r *http.Request) {
	var spec Spec
	if err := decode(r, &spec); err != nil {
		writeError(w, err)
		return
	}
	server, err := a.manager.Create(spec)
	if err != nil {
		if !errors.Is(err, ErrConflict) {
			err = badRequest("%v", err)
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, server.View())
}

func (a *API) getServer(w http.ResponseWriter, _ *http.Request, server *Server) {
	writeJSON(w, http.StatusOK, server.View())
}

func (a *API) patchServer(w http.ResponseWriter, r *http.Request, server *Server) {
	var patch Patch
	if err := decode(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	if err := server.Update(patch); err != nil {
		if !errors.Is(err, ErrBadState) && !errors.Is(err, ErrConflict) {
			err = badRequest("%v", err)
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server.View())
}

func (a *API) deleteServer(w http.ResponseWriter, r *http.Request) {
	if err := a.manager.Delete(r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) reinstall(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Software software.ID `json:"software"`
		Version  string      `json:"version"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.Reinstall(body.Software, body.Version); err != nil {
		if !errors.Is(err, ErrBadState) {
			err = badRequest("%v", err)
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, server.View())
}

func (a *API) power(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Action string `json:"action"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	var err error
	switch body.Action {
	case "start":
		err = server.Start()
	case "stop":
		err = server.Stop()
	case "restart":
		err = server.Restart()
	case "kill":
		err = server.Kill()
	default:
		err = badRequest("unbekannte Aktion %q (start, stop, restart, kill)", body.Action)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, server.View())
}

func (a *API) command(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Command string `json:"command"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.Command(body.Command); err != nil {
		if !errors.Is(err, ErrBadState) {
			err = badRequest("%v", err)
		}
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) logs(w http.ResponseWriter, r *http.Request, server *Server) {
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > consoleLines {
		lines = 200
	}
	writeJSON(w, http.StatusOK, server.Logs(lines))
}

// ---------------------------------------------------------------- Konsole (Websocket)

// Nachricht des Browsers an den Server.
type clientMessage struct {
	Type string `json:"type"` // command
	Data string `json:"data"`
}

// Erste Nachricht des Servers: Verlauf und aktueller Zustand.
type hello struct {
	Type  string `json:"type"`
	State State  `json:"state"`
	Lines []Line `json:"lines"`
	Stats *Stats `json:"stats,omitempty"`
}

func (a *API) stream(w http.ResponseWriter, r *http.Request, server *Server) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // der Schlüssel schützt, nicht die Herkunft
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)

	events, cancel, history, state := server.Subscribe(500)
	defer cancel()
	ctx := r.Context()

	view := server.View()
	if err := wsjson.Write(ctx, conn, hello{Type: "hello", State: state, Lines: history, Stats: view.Stats}); err != nil {
		return
	}

	// Befehle vom Browser
	go func() {
		defer cancel()
		for {
			var message clientMessage
			if err := wsjson.Read(ctx, conn, &message); err != nil {
				return
			}
			if message.Type != "command" {
				continue
			}
			if err := server.Command(message.Data); err != nil {
				_ = wsjson.Write(ctx, conn, Event{Type: "error", Error: err.Error()})
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				conn.Close(websocket.StatusTryAgainLater, "bitte neu verbinden")
				return
			}
			if err := wsjson.Write(ctx, conn, event); err != nil {
				return
			}
		case <-ping.C:
			pingCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pingCtx)
			stop()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// ---------------------------------------------------------------- Dateien

func queryPath(r *http.Request) string { return r.URL.Query().Get("path") }

func (a *API) listFiles(w http.ResponseWriter, r *http.Request, server *Server) {
	entries, err := server.FS().List(queryPath(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (a *API) readFile(w http.ResponseWriter, r *http.Request, server *Server) {
	data, err := server.FS().Read(queryPath(r), maxEditBytes)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func (a *API) writeFile(w http.ResponseWriter, r *http.Request, server *Server) {
	create := r.URL.Query().Get("create") == "1"
	if err := server.FS().Write(queryPath(r), http.MaxBytesReader(w, r.Body, maxEditBytes+1), maxEditBytes, create); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) mkdir(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.FS().Mkdir(body.Path); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) rename(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.FS().Rename(body.From, body.To); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) bulkDelete(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.FS().RemoveMany(body.Paths); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) move(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.FS().MoveMany(body.Paths, body.Dest); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) archive(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.FS().Archive(body.Paths, body.Dest); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) extract(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Path string `json:"path"`
		Dest string `json:"dest"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := server.FS().Extract(body.Path, body.Dest); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) removeFile(w http.ResponseWriter, r *http.Request, server *Server) {
	if err := server.FS().Remove(queryPath(r), r.URL.Query().Get("recursive") == "1"); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// upload nimmt eine oder mehrere Dateien (multipart, Feld "file") entgegen und legt sie im Ordner "path" ab.
func (a *API) upload(w http.ResponseWriter, r *http.Request, server *Server) {
	limit := int64(a.cfg.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, badRequest("kein Multipart-Upload"))
		return
	}
	dir := queryPath(r)
	saved := []string{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, badRequest("%v", err))
			return
		}
		name := path.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
		if part.FormName() != "file" || name == "." || name == "/" || name == "" {
			continue
		}
		target := path.Join(dir, name)
		err = server.FS().Write(target, part, limit, true)
		if errors.Is(err, fs.ErrExist) && r.URL.Query().Get("overwrite") == "1" {
			err = server.FS().Write(target, part, limit, false)
		}
		if err != nil {
			writeError(w, err)
			return
		}
		saved = append(saved, target)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"saved": saved})
}

func (a *API) download(w http.ResponseWriter, r *http.Request, server *Server) {
	file, info, err := server.FS().Open(queryPath(r))
	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
