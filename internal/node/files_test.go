package node

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFS(t *testing.T) (*FS, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "server")
	files, err := OpenFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { files.Close() })
	return files, dir
}

func TestWriteReadListAndRemove(t *testing.T) {
	files, _ := newFS(t)
	if err := files.Mkdir("plugins/config"); err != nil {
		t.Fatal(err)
	}
	if err := files.Write("plugins/config/a.yml", strings.NewReader("key: value\n"), 1<<20, true); err != nil {
		t.Fatal(err)
	}
	data, err := files.Read("plugins/config/a.yml", 1<<20)
	if err != nil || string(data) != "key: value\n" {
		t.Fatalf("lesen: %q %v", data, err)
	}
	if err := files.Write("plugins/config/a.yml", strings.NewReader("k: v\n"), 1<<20, false); err != nil {
		t.Fatal(err)
	}
	if data, _ := files.Read("plugins/config/a.yml", 1<<20); string(data) != "k: v\n" {
		t.Fatalf("überschreiben: %q", data)
	}
	if err := files.Write("plugins/config/a.yml", strings.NewReader("x"), 1<<20, true); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("create darf nichts überschreiben, erhalten %v", err)
	}
	if err := files.Write("plugins/config/missing.yml", strings.NewReader("x"), 1<<20, false); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ohne create muss die Datei da sein, erhalten %v", err)
	}

	entries, err := files.List("plugins")
	if err != nil || len(entries) != 1 || !entries[0].Dir || entries[0].Name != "config" {
		t.Fatalf("Liste: %+v %v", entries, err)
	}
	if err := files.Remove("plugins", false); err == nil {
		t.Fatal("ein nicht leerer Ordner darf nicht ohne recursive gelöscht werden")
	}
	if err := files.Remove("plugins", true); err != nil {
		t.Fatal(err)
	}
	if _, err := files.List("plugins"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("nach dem Löschen: %v", err)
	}
}

func TestPathsCannotLeaveTheServerFolder(t *testing.T) {
	files, dir := newFS(t)
	outside := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(outside, []byte("geheim"), 0o644); err != nil {
		t.Fatal(err)
	}

	// ".." und absolute Pfade landen innerhalb des Ordners und finden das Geheimnis nicht
	for _, name := range []string{"../secret.txt", "../../secret.txt", "/secret.txt", "a/../../secret.txt", `..\secret.txt`} {
		if data, err := files.Read(name, 1<<20); err == nil {
			t.Fatalf("%q las %q außerhalb des Ordners", name, data)
		}
	}
	// schreiben außerhalb ist ebenfalls unmöglich
	if err := files.Write("../escape.txt", strings.NewReader("x"), 1<<20, true); err == nil {
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); statErr == nil {
			t.Fatal("Datei außerhalb des Ordners angelegt")
		}
	}
	if err := files.Remove("../secret.txt", false); err == nil {
		if _, statErr := os.Stat(outside); statErr != nil {
			t.Fatal("Datei außerhalb des Ordners gelöscht")
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("die Datei außerhalb existiert nicht mehr")
	}
}

func TestSymlinkOutOfTheFolderIsBlocked(t *testing.T) {
	files, dir := newFS(t)
	outsideDir := filepath.Join(filepath.Dir(dir), "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("geheim"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(dir, "link")); err != nil {
		t.Skipf("Symlinks sind hier nicht erlaubt: %v", err)
	}
	if data, err := files.Read("link/secret.txt", 1<<20); err == nil {
		t.Fatalf("über den Symlink gelesen: %q", data)
	}
	if _, err := files.List("link"); err == nil {
		t.Fatal("der Symlink darf nicht aus dem Ordner führen")
	} else if !errors.Is(err, ErrOutside) {
		t.Logf("Fehler (erwartet ErrOutside): %v", err)
	}
	if err := files.Write("link/new.txt", strings.NewReader("x"), 1<<20, true); err == nil {
		t.Fatal("über den Symlink geschrieben")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "new.txt")); err == nil {
		t.Fatal("Datei außerhalb angelegt")
	}
}

func TestLimitsAndProtectedRoot(t *testing.T) {
	files, _ := newFS(t)
	if err := files.Write("big.bin", strings.NewReader(strings.Repeat("x", 100)), 10, true); err == nil {
		t.Fatal("zu große Datei wurde akzeptiert")
	}
	if _, err := files.Read("big.bin", 10); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("zu große Datei darf nicht liegen bleiben: %v", err)
	}
	if err := files.Write("ok.txt", strings.NewReader(strings.Repeat("x", 50)), 100, true); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Read("ok.txt", 10); err == nil {
		t.Fatal("Lesen über dem Limit muss scheitern")
	}
	for _, name := range []string{"", ".", "/", "a/.."} {
		if err := files.Remove(name, true); err == nil {
			t.Fatalf("der Serverordner selbst wurde über %q gelöscht", name)
		}
	}
	if err := files.Rename("ok.txt", "."); err == nil {
		t.Fatal("verschieben auf den Serverordner darf nicht gehen")
	}
}

func TestRenameNeverOverwrites(t *testing.T) {
	files, _ := newFS(t)
	_ = files.Write("a.txt", strings.NewReader("a"), 100, true)
	_ = files.Write("b.txt", strings.NewReader("b"), 100, true)
	if err := files.Rename("a.txt", "b.txt"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("erwartet ErrExist, erhalten %v", err)
	}
	if err := files.Rename("a.txt", "c.txt"); err != nil {
		t.Fatal(err)
	}
	if data, _ := files.Read("c.txt", 100); string(data) != "a" {
		t.Fatalf("Inhalt nach dem Umbenennen: %q", data)
	}
	if err := files.Rename("missing.txt", "d.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("erwartet ErrNotExist, erhalten %v", err)
	}
}

func TestSpecValidation(t *testing.T) {
	valid := Spec{Software: "paper", Version: "1.21.8"}
	if err := valid.Normalize(); err != nil {
		t.Fatal(err)
	}
	if valid.MemoryMB != 2048 || len(valid.Ports) != 1 || valid.Ports[0].Container != 25565 || valid.Ports[0].Protocol != "tcp" || valid.ID == "" {
		t.Fatalf("Standardwerte fehlen: %+v", valid)
	}
	velocity := Spec{Software: "velocity", Version: "3.4.0-SNAPSHOT"}
	if err := velocity.Normalize(); err != nil || velocity.Ports[0].Container != 25565 {
		t.Fatalf("Velocity-Standardport: %+v %v", velocity.Ports, err)
	}

	bad := []Spec{
		{Software: "nope", Version: "1"},
		{Software: "paper", Version: ""},
		{Software: "paper", Version: "../1"},
		{ID: "../etc", Software: "paper", Version: "1"},
		{ID: "UPPER", Software: "paper", Version: "1"},
		{Software: "paper", Version: "1", MemoryMB: 10},
		{Software: "paper", Version: "1", CPU: -1},
		{Software: "paper", Version: "1", Ports: []PortMapping{{Container: 0}}},
		{Software: "paper", Version: "1", Ports: []PortMapping{{Container: 25565, Protocol: "icmp"}}},
		{Software: "paper", Version: "1", Ports: []PortMapping{{Container: 25565}, {Container: 25565}}},
		{Software: "paper", Version: "1", Env: map[string]string{"A B": "x"}},
		{Software: "paper", Version: "1", Image: "bad image; rm -rf /"},
	}
	for index, spec := range bad {
		if err := spec.Normalize(); err == nil {
			t.Errorf("ungültige Spec %d wurde akzeptiert: %+v", index, spec)
		}
	}
}
