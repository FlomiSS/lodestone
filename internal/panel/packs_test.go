package panel

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/lodestone-mc/lodestone/internal/software"
)

func TestPositionPacksListAndExport(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	// Packs auflisten
	var summaries []software.PackSummary
	data := admin.expect(200, "GET", "/api/packs", nil)
	if err := json.Unmarshal(data, &summaries); err != nil {
		t.Fatalf("Unmarshal summaries: %v", err)
	}

	if len(summaries) < 9 {
		t.Fatalf("zu wenige Position-Packs: %d", len(summaries))
	}

	foundPaper := false
	foundFabric := false
	foundVelocity := false
	for _, s := range summaries {
		if s.ID == "paper" {
			foundPaper = true
			if !s.CanBeBackend || !s.CanVoice || !s.HasAutoconfig {
				t.Errorf("Paper-Eigenschaften falsch: %+v", s)
			}
		}
		if s.ID == "fabric" {
			foundFabric = true
			if !s.CanBeBackend || !s.HasAutoconfig {
				t.Errorf("Fabric-Eigenschaften falsch: %+v", s)
			}
		}
		if s.ID == "velocity" {
			foundVelocity = true
			if s.Kind != "proxy" {
				t.Errorf("Velocity-Kind falsch: %s", s.Kind)
			}
		}
	}
	if !foundPaper || !foundFabric || !foundVelocity {
		t.Fatalf("Standard-Packs nicht vollständig: paper=%v fabric=%v velocity=%v", foundPaper, foundFabric, foundVelocity)
	}

	// Einzelnes Pack abrufen
	var detail struct {
		Meta       software.PackMeta       `json:"meta"`
		Autoconfig software.PackAutoconfig `json:"autoconfig"`
		Script     string                  `json:"script"`
	}
	detailData := admin.expect(200, "GET", "/api/packs/paper", nil)
	if err := json.Unmarshal(detailData, &detail); err != nil {
		t.Fatalf("Unmarshal detail: %v", err)
	}
	if detail.Meta.ID != "paper" || detail.Autoconfig.Backend == nil {
		t.Fatalf("Paper-Details unvollständig: %+v", detail)
	}

	// Pack als ZIP exportieren
	status, zipBytes := admin.do("GET", "/api/packs/paper/export", nil)
	if status != 200 {
		t.Fatalf("Export fehlgeschlagen mit Status %d", status)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("Exportiertes ZIP ungültig: %v", err)
	}
	foundMeta := false
	for _, f := range zr.File {
		if f.Name == "meta.json" {
			foundMeta = true
		}
	}
	if !foundMeta {
		t.Error("meta.json im exportierten ZIP nicht gefunden")
	}
}

func TestPositionPacksUploadAndServerCreation(t *testing.T) {
	p, web, fake, nodeID := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	// Eigenes Position-Pack bauen
	customPack := &software.PositionPack{
		Meta: software.PackMeta{
			ID:           "sponge",
			Name:         "SpongeVanilla",
			Description:  "Plugin-Plattform für Minecraft.",
			Version:      "1.0.0",
			Kind:         software.KindServer,
			Extensions:   software.ExtPlugins,
			DefaultPort:  25565,
			Startup:      "exec java -jar {{JAR}} nogui",
			StopCommand:  "stop",
			ReadyPattern: `Done \(`,
			Java:         software.PackJava{Strategy: "mojang", Major: 21},
			Provider:     software.PackProvider{Type: "vanilla"},
			Backend: software.PackBackend{
				Supported:       true,
				VoiceConfigFile: "config/sponge/voicechat.properties",
			},
		},
		Autoconfig: software.PackAutoconfig{
			Backend: &software.PackAutoconfigTarget{
				Restart: true,
				Edits: []software.PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set:    []software.PackSetOp{{Path: []string{"online-mode"}, Value: false}},
					},
					{
						File:   "config/sponge/global.conf",
						Format: "yaml",
						Create: true,
						Set: []software.PackSetOp{
							{Path: []string{"modules", "bungeecord"}, Value: true},
							{Path: []string{"bungeecord", "secret"}, Value: "{{SECRET}}"},
						},
					},
				},
			},
		},
		Script: "#!/bin/sh\necho 'Sponge pack initialized'\n",
	}

	zipData, err := software.CreateZipPack(customPack)
	if err != nil {
		t.Fatalf("CreateZipPack: %v", err)
	}

	// Als multipart-form-data hochladen
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sponge.positionpack.zip")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = part.Write(zipData)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", web.URL+"/api/packs/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeader, "1")
	resp, err := admin.http.Do(req)
	if err != nil {
		t.Fatalf("Upload-Request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Upload: Status %d, erwartet 201", resp.StatusCode)
	}

	// Warten bis der asynchrone Sync von uploadPack beim Node angekommen ist
	waitUntil(t, "uploadPack synchronisiert", func() bool {
		for _, id := range fake.getPackPuts() {
			if id == "sponge" {
				return true
			}
		}
		return false
	})
	fake.clearPackPuts()

	// Prüfen, ob Pack in der Softwareliste auftaucht
	var softwareList []software.Info
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/software", nil), &softwareList)
	foundSponge := false
	for _, sw := range softwareList {
		if sw.ID == "sponge" {
			foundSponge = true
			if !sw.CanBeBackend || !sw.Custom {
				t.Errorf("Software-Info für Sponge fehlerhaft: %+v", sw)
			}
		}
	}
	if !foundSponge {
		t.Fatal("Sponge taucht nicht in /api/software auf")
	}

	// Projekt anlegen und Server mit neuem Pack erstellen
	var project struct{ ID string }
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Sponge-Projekt", "kind": "server"}), &project)

	// 1. createServer: fehlgeschlagener Sync verhindert Servererstellung
	fake.setPackFails(true)
	admin.expect(500, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{
		Name:     "Sponge-Fail",
		NodeID:   nodeID,
		Software: "sponge",
		Version:  "1.21.4",
		EULA:     true,
	})
	fake.mu.Lock()
	if len(fake.servers) != 0 {
		t.Fatalf("Server darf bei fehlgeschlagenem Pack-Sync nicht angelegt werden: %+v", fake.servers)
	}
	fake.mu.Unlock()
	puts := fake.getPackPuts()
	if len(puts) == 0 || puts[len(puts)-1] != "sponge" {
		t.Fatalf("ensurePackOnNode hat keinen PUT für sponge gesendet: %+v", puts)
	}

	// 2. createServer: erfolgreicher Sync legt den Server an
	fake.setPackFails(false)
	fake.clearPackPuts()

	var server struct{ ID, Software string }
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+project.ID+"/servers", serverRequest{
		Name:     "Sponge-1",
		NodeID:   nodeID,
		Software: "sponge",
		Version:  "1.21.4",
		EULA:     true,
	}), &server)

	if server.Software != "sponge" {
		t.Fatalf("Erstellter Server hat falsche Software: %s", server.Software)
	}

	fake.mu.Lock()
	nodeView, ok := fake.servers[server.ID]
	fake.mu.Unlock()
	if !ok || nodeView.Spec.Software != "sponge" {
		t.Fatalf("Server auf dem Node nicht angelegt: %+v", nodeView)
	}
	puts = fake.getPackPuts()
	if len(puts) == 0 || puts[len(puts)-1] != "sponge" {
		t.Fatalf("Erfolgreiches createServer hat keinen PUT für sponge gesendet: %+v", puts)
	}

	// 3. createInstance (Gruppen-Skalierung): Netzwerk-Projekt und Gruppe anlegen
	var netProject struct{ ID string }
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Sponge-Netz", "kind": "network"}), &netProject)
	admin.expect(201, "POST", "/api/projects/"+netProject.ID+"/servers", serverRequest{
		Name:     "Proxy",
		NodeID:   nodeID,
		Software: "velocity",
		Version:  "3.5.1",
	})

	var group Group
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/projects/"+netProject.ID+"/groups", groupRequest{
		Name:         "Sponge-Gruppe",
		Software:     "sponge",
		Version:      "1.21.4",
		MemoryMB:     512,
		MinInstances: ptr(0),
		MaxInstances: ptr(2),
		EULA:         true,
	}), &group)

	// createInstance: fehlgeschlagener Sync verhindert Instanzerstellung
	fake.setPackFails(true)
	fake.clearPackPuts()
	admin.expect(500, "POST", "/api/groups/"+group.ID+"/instances", nil)
	rows, err := p.store.ListGroupServers(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("Instanz darf bei fehlgeschlagenem Pack-Sync nicht angelegt werden: %+v", rows)
	}
	puts = fake.getPackPuts()
	if len(puts) == 0 || puts[len(puts)-1] != "sponge" {
		t.Fatalf("createInstance hat keinen PUT für sponge gesendet: %+v", puts)
	}

	// createInstance: erfolgreicher Sync legt Instanz an
	fake.setPackFails(false)
	fake.clearPackPuts()
	var instance ServerDTO
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/groups/"+group.ID+"/instances", nil), &instance)
	if instance.Software != "sponge" {
		t.Fatalf("Instanz hat unerwartete Software: %s", instance.Software)
	}
	rows, err = p.store.ListGroupServers(group.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Instanz wurde nicht in DB angelegt: %+v", rows)
	}
	puts = fake.getPackPuts()
	if len(puts) == 0 || puts[len(puts)-1] != "sponge" {
		t.Fatalf("createInstance hat nach Freigabe keinen PUT für sponge gesendet: %+v", puts)
	}
}

func TestPositionPacksDeleteAndPermissions(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	// Weiteren Benutzer anlegen (kein Admin)
	admin.expect(201, "POST", "/api/users", map[string]any{"username": "bob", "password": "passwort-fuer-bob", "isAdmin": false})
	bob := newClient(t, web.URL)
	bob.expect(200, "POST", "/api/auth/login", map[string]string{"username": "bob", "password": "passwort-fuer-bob"})

	// Bob darf Packs ansehen
	bob.expect(200, "GET", "/api/packs", nil)

	// Bob darf nicht hochladen oder löschen oder ändern
	bob.expect(403, "POST", "/api/packs/upload", []byte("xyz"))
	bob.expect(403, "DELETE", "/api/packs/paper", nil)
	bob.expect(403, "PUT", "/api/packs/paper", map[string]any{"meta": map[string]any{"name": "Test"}})

	// Update-Prüfungen
	// Unbekannte ID liefert 404
	admin.expect(404, "PUT", "/api/packs/unbekanntes-pack-xyz", map[string]any{"meta": map[string]any{"name": "Test"}})
	// Unbekanntes Feld liefert 400
	admin.expect(400, "PUT", "/api/packs/paper", map[string]any{"unerlaubtesFeld": "ungueltig"})
	// Erfolgreiches Update
	validUpdate := map[string]any{
		"meta": map[string]any{
			"id":           "paper",
			"name":         "Paper Modifiziert",
			"kind":         "server",
			"extensions":   "plugins",
			"defaultPort":  25565,
			"startup":      "exec java -jar {{JAR}}",
			"stopCommand":  "stop",
			"readyPattern": "Done",
		},
		"autoconfig": map[string]any{},
		"script":     "",
	}
	admin.expect(200, "PUT", "/api/packs/paper", validUpdate)

	// Überschriebenes Standard-Pack löschen setzt auf Standard zurück (204)
	admin.expect(204, "DELETE", "/api/packs/paper", nil)
	admin.expect(200, "GET", "/api/packs/paper", nil)
	// Nicht mehr überschriebenes Standard-Pack darf nicht gelöscht werden (400)
	admin.expect(400, "DELETE", "/api/packs/paper", nil)

	// Custom Pack hochladen und löschen
	testPack := &software.PositionPack{
		Meta: software.PackMeta{
			ID:           "temp-pack",
			Name:         "Temp",
			Kind:         software.KindServer,
			Extensions:   software.ExtNone,
			DefaultPort:  25565,
			Startup:      "exec java -jar {{JAR}}",
			StopCommand:  "stop",
			ReadyPattern: "Done",
		},
	}
	zipBytes, _ := software.CreateZipPack(testPack)
	req, _ := http.NewRequest("POST", web.URL+"/api/packs/upload", bytes.NewReader(zipBytes))
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set(csrfHeader, "1")
	resp, err := admin.http.Do(req)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Upload status = %d, want 201", resp.StatusCode)
	}

	// Löschen
	admin.expect(204, "DELETE", "/api/packs/temp-pack", nil)
	admin.expect(404, "GET", "/api/packs/temp-pack", nil)
}
