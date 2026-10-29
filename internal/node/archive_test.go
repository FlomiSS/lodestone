package node

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestArchiveExtractRoundTrip(t *testing.T) {
	for _, name := range []string{"backup.zip", "backup.tar.gz"} {
		t.Run(name, func(t *testing.T) {
			files, _ := newFS(t)
			files.Mkdir("world/region")
			files.Write("world/region/r.mca", strings.NewReader("data"), 1<<20, true)
			files.Write("a.txt", strings.NewReader("hallo"), 1<<20, true)
			if err := files.Archive([]string{"world", "a.txt"}, name); err != nil {
				t.Fatal(err)
			}
			if err := files.Archive([]string{"a.txt"}, name); !errors.Is(err, fs.ErrExist) {
				t.Fatalf("vorhandenes Archiv: %v", err)
			}
			if err := files.Extract(name, "out"); err != nil {
				t.Fatal(err)
			}
			if data, _ := files.Read("out/world/region/r.mca", 1<<20); string(data) != "data" {
				t.Fatalf("Inhalt: %q", data)
			}
			if data, _ := files.Read("out/a.txt", 1<<20); string(data) != "hallo" {
				t.Fatalf("Inhalt: %q", data)
			}
			if err := files.Extract(name, "out"); !errors.Is(err, fs.ErrExist) {
				t.Fatalf("Überschreiben muss scheitern: %v", err)
			}
		})
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	files, _ := newFS(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../evil.txt")
	w.Write([]byte("x"))
	zw.Close()
	files.Write("bad.zip", &buf, 1<<20, true)
	if err := files.Extract("bad.zip", "out"); err == nil {
		t.Fatal("Pfad nach außen wurde akzeptiert")
	}
	if files.Exists("evil.txt") {
		t.Fatal("Datei außerhalb entpackt")
	}
}

func TestMoveManyAndRemoveMany(t *testing.T) {
	files, _ := newFS(t)
	files.Mkdir("dest")
	files.Mkdir("dir")
	files.Write("a.txt", strings.NewReader("a"), 1<<20, true)
	files.Write("dir/b.txt", strings.NewReader("b"), 1<<20, true)
	if err := files.MoveMany([]string{"a.txt", "dir"}, "dest"); err != nil {
		t.Fatal(err)
	}
	if !files.Exists("dest/a.txt") || !files.Exists("dest/dir/b.txt") || files.Exists("a.txt") {
		t.Fatal("verschieben fehlgeschlagen")
	}
	if err := files.MoveMany([]string{"dest"}, "dest/dir"); err == nil {
		t.Fatal("in sich selbst verschieben muss scheitern")
	}
	files.Write("a.txt", strings.NewReader("neu"), 1<<20, true)
	if err := files.MoveMany([]string{"a.txt"}, "dest"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Ziel vorhanden: %v", err)
	}
	if err := files.RemoveMany([]string{"a.txt", "dest"}); err != nil {
		t.Fatal(err)
	}
	if files.Exists("a.txt") || files.Exists("dest") {
		t.Fatal("löschen fehlgeschlagen")
	}
}
