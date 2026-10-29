package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (a *API) addons(w http.ResponseWriter, _ *http.Request, server *Server) {
	list, err := server.Addons()
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []Addon{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) players(w http.ResponseWriter, r *http.Request, server *Server) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	info, err := server.Players(ctx)
	if err != nil {
		if errors.Is(err, ErrBadState) {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "der Server antwortet nicht auf die Abfrage: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) getAutoconfig(w http.ResponseWriter, _ *http.Request, server *Server) {
	view := server.View()
	writeJSON(w, http.StatusOK, map[string]any{"autoconfig": view.Autoconfig, "applied": view.AutoconfigApplied, "pending": view.AutoconfigPending})
}

func (a *API) putAutoconfig(w http.ResponseWriter, r *http.Request, server *Server) {
	var config Autoconfig
	if err := decode(r, &config); err != nil {
		writeError(w, err)
		return
	}
	result, err := server.SetAutoconfig(config)
	if err != nil {
		writeError(w, badRequest("%v", err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) pull(w http.ResponseWriter, r *http.Request, server *Server) {
	var request PullRequest
	if err := decode(r, &request); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	if err := a.manager.Pull(ctx, server, request); err != nil {
		if errors.Is(err, errBadRequest) || errors.Is(err, ErrConflict) || errors.Is(err, ErrOutside) {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listBackups(w http.ResponseWriter, _ *http.Request, server *Server) {
	list, err := server.ListBackups()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) createBackup(w http.ResponseWriter, r *http.Request, server *Server) {
	var body struct {
		Label string `json:"label"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &body); err != nil {
			writeError(w, err)
			return
		}
	}
	backup, err := server.CreateBackup(body.Label)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, backup)
}

func (a *API) deleteBackup(w http.ResponseWriter, r *http.Request, server *Server) {
	if err := server.DeleteBackup(r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) downloadBackup(w http.ResponseWriter, r *http.Request, server *Server) {
	file, err := server.BackupFile(r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, file)
}

func (a *API) restoreBackup(w http.ResponseWriter, r *http.Request, server *Server) {
	if err := server.RestoreBackup(r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, server.View())
}

func (a *API) installModpack(w http.ResponseWriter, r *http.Request, server *Server) {
	var request ModpackRequest
	if err := decode(r, &request); err != nil {
		writeError(w, err)
		return
	}
	if err := a.manager.InstallModpack(server, request); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, server.ModpackStatus())
}

func (a *API) putPlayit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Secret string `json:"secret"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	if err := a.manager.ConfigurePlayit(ctx, body.Secret); err != nil {
		if !errors.Is(err, errBadRequest) {
			err = fmt.Errorf("der Agent ließ sich nicht starten: %w", err)
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.manager.PlayitStatus(ctx))
}
