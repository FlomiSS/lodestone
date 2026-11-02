package software

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPacksConsistent(t *testing.T) {
	packs := DefaultPacks()
	if len(packs) < 9 {
		t.Fatalf("zu wenige Standard-Position-Packs: %d", len(packs))
	}

	seen := map[ID]bool{}
	for _, pack := range packs {
		if err := pack.Meta.Validate(); err != nil {
			t.Fatalf("Position-Pack %s hat ungültige Metadaten: %v", pack.Meta.ID, err)
		}
		if seen[pack.Meta.ID] {
			t.Fatalf("doppelte Position-Pack-ID: %s", pack.Meta.ID)
		}
		seen[pack.Meta.ID] = true

		info := pack.Info()
		if info.ID != pack.Meta.ID || info.Name != pack.Meta.Name {
			t.Fatalf("Info stimmt nicht mit Meta überein: %+v vs %+v", info, pack.Meta)
		}

		summary := pack.Summary()
		if summary.ID != pack.Meta.ID {
			t.Fatalf("Summary-ID falsch: %s", summary.ID)
		}
	}
}

func TestPackZipReadAndWrite(t *testing.T) {
	original := &PositionPack{
		Meta: PackMeta{
			ID:           "minestom",
			Name:         "Minestom",
			Description:  "Reimplementierung des Minecraft-Servers ohne Vanilla-Code.",
			Version:      "1.2.0",
			Author:       "Minestom / Tester",
			Kind:         KindServer,
			Extensions:   ExtNone,
			DefaultPort:  25565,
			Startup:      "exec java -jar {{JAR}}",
			StopCommand:  "stop",
			ReadyPattern: `Ready!`,
			Java:         PackJava{Strategy: "fixed", Major: 21},
			Backend:      PackBackend{Supported: true},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set: []PackSetOp{
							{Path: []string{"online-mode"}, Value: false},
						},
					},
				},
			},
		},
		Script: "#!/bin/sh\necho 'Minestom ready'\n",
	}

	zipBytes, err := CreateZipPack(original)
	if err != nil {
		t.Fatalf("CreateZipPack fehlgeschlagen: %v", err)
	}

	parsed, err := ReadZipPack(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("ReadZipPack fehlgeschlagen: %v", err)
	}

	if parsed.Meta.ID != original.Meta.ID {
		t.Errorf("ID: %s, erwartet %s", parsed.Meta.ID, original.Meta.ID)
	}
	if parsed.Meta.Name != original.Meta.Name {
		t.Errorf("Name: %s, erwartet %s", parsed.Meta.Name, original.Meta.Name)
	}
	if parsed.Autoconfig.Backend == nil || len(parsed.Autoconfig.Backend.Edits) != 1 {
		t.Errorf("Autoconfig nicht korrekt geladen: %+v", parsed.Autoconfig)
	}
	if parsed.Script != original.Script {
		t.Errorf("Script: %q, erwartet %q", parsed.Script, original.Script)
	}
}

func TestPackZipValidationErrors(t *testing.T) {
	// 1. Kein gültiges ZIP
	if _, err := ReadZipPack(bytes.NewReader([]byte("kein zip")), 8); err == nil {
		t.Error("Fehler bei ungültigem ZIP erwartet")
	}

	// 2. ZIP ohne meta.json
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("hello.txt")
	_, _ = f.Write([]byte("welt"))
	_ = zw.Close()

	if _, err := ReadZipPack(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil {
		t.Error("Fehler bei fehlender meta.json erwartet")
	}

	// 3. Ungültige ID
	badMeta := PackMeta{
		ID:   "Ungueltige ID mit Leerzeichen!",
		Name: "Test",
		Kind: KindServer,
	}
	if err := badMeta.Validate(); err == nil {
		t.Error("Fehler bei ungültiger ID erwartet")
	}

	// 4. Skript überschreitet MaxPackScriptSize (1 MB)
	oversizedPack := &PositionPack{
		Meta: PackMeta{
			ID:   "oversized",
			Name: "Oversized",
			Kind: KindServer,
		},
		Script: strings.Repeat("a", MaxPackScriptSize+10),
	}
	if err := oversizedPack.Validate(); err == nil {
		t.Error("Validate hätte Skript > 1MB abweisen müssen")
	}
	if _, err := CreateZipPack(oversizedPack); err == nil {
		t.Error("CreateZipPack hätte Skript > 1MB abweisen müssen")
	}

	// 5. ZIP mit autoconfig.sh > 1 MB wird von ReadZipPack abgewiesen (keine stille Trunkierung)
	buf = new(bytes.Buffer)
	zw = zip.NewWriter(buf)
	mf, _ := zw.Create("meta.json")
	_, _ = mf.Write([]byte(`{"id":"bigscript","name":"BigScript","kind":"server"}`))
	sf, _ := zw.Create("autoconfig.sh")
	_, _ = sf.Write(bytes.Repeat([]byte("a"), MaxPackScriptSize+50))
	_ = zw.Close()
	if _, err := ReadZipPack(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil {
		t.Error("ReadZipPack hätte ZIP mit Skript > 1MB abweisen müssen")
	}

	// 6. ZIP mit autoconfig.json > 2 MB wird von ReadZipPack abgewiesen
	buf = new(bytes.Buffer)
	zw = zip.NewWriter(buf)
	mf2, _ := zw.Create("meta.json")
	_, _ = mf2.Write([]byte(`{"id":"bigauto","name":"BigAuto","kind":"server"}`))
	af, _ := zw.Create("autoconfig.json")
	_, _ = af.Write(bytes.Repeat([]byte(" "), MaxPackAutoconfigSize+50))
	_ = zw.Close()
	if _, err := ReadZipPack(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil {
		t.Error("ReadZipPack hätte ZIP mit autoconfig.json > 2MB abweisen müssen")
	}
}

func TestPackManagerLifecycle(t *testing.T) {
	dir := t.TempDir()
	pm := NewPackManager(dir)

	// Standard-Pack abfragen
	paper, ok := pm.Get(Paper)
	if !ok || paper.Meta.Name != "Paper" {
		t.Fatalf("Paper-Position-Pack fehlt: %+v", paper)
	}

	// Neues Position-Pack bauen
	custom := &PositionPack{
		Meta: PackMeta{
			ID:          "purpur-custom",
			Name:        "Purpur Spezial",
			Kind:        KindServer,
			Extensions:  ExtPlugins,
			DefaultPort: 25565,
			Provider:    PackProvider{Type: "purpur"},
			Backend:     PackBackend{Supported: true},
		},
		Autoconfig: PackAutoconfig{
			Backend: &PackAutoconfigTarget{
				Edits: []PackEdit{
					{
						File:   "server.properties",
						Format: "properties",
						Set:    []PackSetOp{{Path: []string{"online-mode"}, Value: false}},
					},
				},
			},
		},
		Script: "#!/bin/sh\necho custom\n",
	}

	zipData, err := CreateZipPack(custom)
	if err != nil {
		t.Fatalf("CreateZipPack: %v", err)
	}

	added, err := pm.Add(zipData)
	if err != nil {
		t.Fatalf("pm.Add: %v", err)
	}
	if !added.Custom {
		t.Errorf("Pack sollte als Custom markiert sein")
	}

	// Datei muss auf der Platte liegen
	if _, err := os.Stat(filepath.Join(dir, "purpur-custom.zip")); err != nil {
		t.Errorf("purpur-custom.zip nicht auf der Platte gefunden: %v", err)
	}

	// Auslesen
	got, ok := pm.Get("purpur-custom")
	if !ok || got.Meta.Name != "Purpur Spezial" {
		t.Fatalf("Get('purpur-custom') fehlgeschlagen: %+v", got)
	}

	// Exportieren
	exported, err := pm.Export("purpur-custom")
	if err != nil || len(exported) == 0 {
		t.Fatalf("Export fehlgeschlagen: %v", err)
	}

	// Löschen
	if err := pm.Delete("purpur-custom"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := pm.Get("purpur-custom"); ok {
		t.Errorf("Pack sollte nach Delete gelöscht sein")
	}
	if _, err := os.Stat(filepath.Join(dir, "purpur-custom.zip")); !os.IsNotExist(err) {
		t.Errorf("Datei sollte von der Platte entfernt sein")
	}

	// Standard-Pack darf nicht gelöscht werden (wenn nicht überschrieben)
	if err := pm.Delete(Paper); err == nil {
		t.Errorf("Löschen von Paper hätte fehlschlagen müssen")
	}

	// Standard-Pack überschreiben
	paperMod := *paper
	paperMod.Meta.Description = "Modifiziertes Paper"
	modZip, err := CreateZipPack(&paperMod)
	if err != nil {
		t.Fatalf("CreateZipPack: %v", err)
	}
	saved, err := pm.Add(modZip)
	if err != nil {
		t.Fatalf("pm.Add modifiziertes Paper: %v", err)
	}
	if saved.Custom {
		t.Errorf("überschriebenes Standard-Pack darf nicht Custom sein")
	}
	if !saved.Overridden {
		t.Errorf("überschriebenes Standard-Pack muss Overridden=true sein")
	}

	// Löschen setzt auf Standard zurück
	if err := pm.Delete(Paper); err != nil {
		t.Fatalf("Löschen von überschriebenem Paper fehlgeschlagen: %v", err)
	}
	restored, ok := pm.Get(Paper)
	if !ok {
		t.Fatalf("Paper muss nach Reset vorhanden bleiben")
	}
	if restored.Overridden {
		t.Errorf("wiederhergestelltes Paper darf nicht Overridden sein")
	}
	if restored.Meta.Description == "Modifiziertes Paper" {
		t.Errorf("Paper-Beschreibung wurde nicht auf Standard zurückgesetzt")
	}

	// Save mit unnormalisierter ID ("Paper") muss trotzdem als Standard-Pack erkannt werden
	caseTest := *restored
	caseTest.Meta.ID = "Paper"
	caseTest.Meta.Description = "Case-Normalisiertes Paper"
	if err := pm.Save(&caseTest); err != nil {
		t.Fatalf("pm.Save mit unnormalisierter ID fehlgeschlagen: %v", err)
	}
	if caseTest.Meta.ID != Paper {
		t.Errorf("ID wurde nicht normalisiert: %q", caseTest.Meta.ID)
	}
	if caseTest.Custom || !caseTest.Overridden {
		t.Errorf("unnormalisiertes Paper wurde nicht als Overridden Standard-Pack markiert: Custom=%v Overridden=%v", caseTest.Custom, caseTest.Overridden)
	}
	_ = pm.Delete(Paper)
}
