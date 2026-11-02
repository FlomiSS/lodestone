package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

const maxPackZipSize = 32 << 20 // 32 MB

func (p *Panel) listPacks(w http.ResponseWriter, r *http.Request) {
	pm := software.GetPackManager()
	writeJSON(w, http.StatusOK, pm.Summaries())
}

func (p *Panel) getPack(w http.ResponseWriter, r *http.Request) {
	id := software.ID(r.PathValue("id"))
	pm := software.GetPackManager()
	pack, ok := pm.Get(id)
	if !ok {
		writeError(w, fail(http.StatusNotFound, "dieses Position-Pack gibt es nicht"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"meta":       pack.Meta,
		"autoconfig": pack.Autoconfig,
		"script":     pack.Script,
		"custom":     pack.Custom,
		"zipSize":    pack.ZipSize,
	})
}

func (p *Panel) exportPack(w http.ResponseWriter, r *http.Request) {
	id := software.ID(r.PathValue("id"))
	pm := software.GetPackManager()
	zipBytes, err := pm.Export(id)
	if err != nil {
		if errors.Is(err, software.ErrNotFound) {
			writeError(w, fail(http.StatusNotFound, "dieses Position-Pack gibt es nicht"))
			return
		}
		writeError(w, fail(http.StatusInternalServerError, "Export fehlgeschlagen: "+err.Error()))
		return
	}

	fileName := fmt.Sprintf("%s.positionpack.zip", id)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBytes)
}

func (p *Panel) uploadPack(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}

	// Gesamte Anfrage auf maxPackZipSize plus Multipart-Framing-Puffer begrenzen
	r.Body = http.MaxBytesReader(w, r.Body, maxPackZipSize+1024*1024)

	var zipBytes []byte
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxPackZipSize); err != nil {
			writeError(w, fail(http.StatusBadRequest, "ungültiges Formular: "+err.Error()))
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, fail(http.StatusBadRequest, "keine Datei im Feld 'file' gefunden"))
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxPackZipSize+1))
		if err != nil {
			writeError(w, fail(http.StatusBadRequest, "Datei konnte nicht gelesen werden"))
			return
		}
		zipBytes = data
	} else {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxPackZipSize+1))
		if err != nil {
			writeError(w, fail(http.StatusBadRequest, "Anfrage konnte nicht gelesen werden"))
			return
		}
		zipBytes = data
	}

	if len(zipBytes) == 0 {
		writeError(w, fail(http.StatusBadRequest, "die hochgeladene Datei ist leer"))
		return
	}
	if int64(len(zipBytes)) > maxPackZipSize {
		writeError(w, fail(http.StatusBadRequest, "das Position-Pack ist größer als 32 MB"))
		return
	}

	pm := software.GetPackManager()
	pack, err := pm.Add(zipBytes)
	if err != nil {
		writeError(w, fail(http.StatusBadRequest, "ungültiges Position-Pack: "+err.Error()))
		return
	}

	p.audit(r, "Position-Pack hochgeladen", pack.Meta.Name)
	go p.syncPackToNodes(pack)
	writeJSON(w, http.StatusCreated, pack.Summary())
}

type packUpdatePayload struct {
	Meta       software.PackMeta       `json:"meta"`
	Autoconfig software.PackAutoconfig `json:"autoconfig"`
	Script     string                  `json:"script"`
}

func (p *Panel) updatePack(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	id := software.ID(r.PathValue("id"))
	pm := software.GetPackManager()
	existing, ok := pm.Get(id)
	if !ok {
		writeError(w, fail(http.StatusNotFound, "dieses Position-Pack gibt es nicht"))
		return
	}

	var payload packUpdatePayload
	if err := decodeLimit(r, &payload, software.MaxPackJSONSize); err != nil {
		writeError(w, err)
		return
	}

	payload.Meta.ID = id // ID darf nicht geändert werden
	updated := &software.PositionPack{
		Meta:       payload.Meta,
		Autoconfig: payload.Autoconfig,
		Script:     payload.Script,
		Custom:     existing.Custom,
		Overridden: existing.Overridden,
	}

	if err := pm.Save(updated); err != nil {
		writeError(w, fail(http.StatusBadRequest, "Position-Pack konnte nicht gespeichert werden: "+err.Error()))
		return
	}

	p.audit(r, "Position-Pack geändert", updated.Meta.Name)
	go p.syncPackToNodes(updated)
	writeJSON(w, http.StatusOK, updated.Summary())
}

func (p *Panel) deletePack(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	id := software.ID(r.PathValue("id"))
	pm := software.GetPackManager()
	if err := pm.Delete(id); err != nil {
		if errors.Is(err, software.ErrNotFound) {
			writeError(w, fail(http.StatusNotFound, "dieses Position-Pack gibt es nicht"))
			return
		}
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}

	p.audit(r, "Position-Pack gelöscht", string(id))
	go p.deletePackFromNodes(id)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) syncPackToNodes(pack *software.PositionPack) {
	nodes, err := p.store.ListNodes()
	if err != nil {
		p.log.Warn("Nodes für Position-Pack-Sync konnten nicht gelistet werden", "pack", pack.Meta.ID, "error", err)
		return
	}
	for _, n := range nodes {
		client, _, err := p.client(n.ID)
		if err != nil {
			p.log.Warn("Node-Client für Position-Pack-Sync nicht erreichbar", "pack", pack.Meta.ID, "node", n.ID, "error", err)
			continue
		}
		go func(c *NodeClient, nodeID string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := c.Do(ctx, http.MethodPut, "/api/packs/"+string(pack.Meta.ID), pack, nil); err != nil {
				p.log.Warn("Position-Pack konnte nicht auf Node synchronisiert werden", "pack", pack.Meta.ID, "node", nodeID, "error", err)
			}
		}(client, n.ID)
	}
}

func (p *Panel) deletePackFromNodes(id software.ID) {
	nodes, err := p.store.ListNodes()
	if err != nil {
		p.log.Warn("Nodes für Position-Pack-Löschung konnten nicht gelistet werden", "pack", id, "error", err)
		return
	}
	for _, n := range nodes {
		client, _, err := p.client(n.ID)
		if err != nil {
			p.log.Warn("Node-Client für Position-Pack-Löschung nicht erreichbar", "pack", id, "node", n.ID, "error", err)
			continue
		}
		go func(c *NodeClient, nodeID string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := c.Do(ctx, http.MethodDelete, "/api/packs/"+string(id), nil, nil); err != nil {
				p.log.Warn("Position-Pack konnte nicht von Node gelöscht werden", "pack", id, "node", nodeID, "error", err)
			}
		}(client, n.ID)
	}
}

func (p *Panel) ensurePackOnNode(ctx context.Context, client *NodeClient, id software.ID) error {
	pack, ok := software.GetPack(id)
	if !ok || (!pack.Custom && !pack.Overridden) {
		return nil
	}
	if err := client.Do(ctx, http.MethodPut, "/api/packs/"+string(pack.Meta.ID), pack, nil); err != nil {
		p.log.Warn("Position-Pack konnte nicht auf Node synchronisiert werden", "pack", pack.Meta.ID, "error", err)
		return err
	}
	return nil
}
