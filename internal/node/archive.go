package node

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const (
	maxExtractBytes   = int64(8) << 30 // Obergrenze für entpackte Daten (Schutz vor Zip-Bomben)
	maxExtractEntries = 200_000
	maxBulkPaths      = 5000
)

// cleanMany bereinigt eine Liste von Pfaden; der Serverordner selbst und Duplikate sind nicht erlaubt.
func cleanMany(names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, errors.New("keine Einträge ausgewählt")
	}
	if len(names) > maxBulkPaths {
		return nil, errors.New("zu viele Einträge auf einmal")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		rel, err := clean(name)
		if err != nil {
			return nil, err
		}
		if rel == "." {
			return nil, errors.New("der Serverordner selbst kann nicht ausgewählt werden")
		}
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out, nil
}

// RemoveMany löscht mehrere Einträge (Ordner samt Inhalt).
func (f *FS) RemoveMany(names []string) error {
	rels, err := cleanMany(names)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if err := translate(f.root.RemoveAll(rel)); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// MoveMany verschiebt Einträge in einen Zielordner. Nichts wird überschrieben; vorab wird alles geprüft,
// damit nicht die Hälfte verschoben wird.
func (f *FS) MoveMany(names []string, dest string) error {
	rels, err := cleanMany(names)
	if err != nil {
		return err
	}
	dir, err := clean(dest)
	if err != nil {
		return err
	}
	if dir != "." {
		info, err := f.root.Stat(dir)
		if err != nil {
			return translate(err)
		}
		if !info.IsDir() {
			return errors.New("das Ziel ist kein Ordner")
		}
	}
	type move struct{ from, to string }
	moves := make([]move, 0, len(rels))
	targets := map[string]bool{}
	for _, rel := range rels {
		if _, err := f.root.Lstat(rel); err != nil {
			return fmt.Errorf("%s: %w", rel, translate(err))
		}
		if dir == rel || strings.HasPrefix(dir+"/", rel+"/") {
			return fmt.Errorf("%s kann nicht in sich selbst verschoben werden", rel)
		}
		target := path.Join(dir, path.Base(rel))
		if target == rel {
			continue // liegt schon dort
		}
		if targets[target] {
			return fmt.Errorf("mehrere Einträge heißen %q", path.Base(rel))
		}
		if _, err := f.root.Lstat(target); err == nil {
			return fmt.Errorf("%s: %w", target, fs.ErrExist)
		}
		targets[target] = true
		moves = append(moves, move{rel, target})
	}
	for _, m := range moves {
		if err := translate(f.root.Rename(m.from, m.to)); err != nil {
			return fmt.Errorf("%s: %w", m.from, err)
		}
	}
	return nil
}

// Archive packt Einträge in ein Archiv (.zip, .tar.gz oder .tgz nach Endung von dest). Symlinks werden übersprungen.
func (f *FS) Archive(names []string, dest string) error {
	rels, err := cleanMany(names)
	if err != nil {
		return err
	}
	out, err := clean(dest)
	if err != nil {
		return err
	}
	if out == "." {
		return errors.New("ungültiger Archivname")
	}
	lower := strings.ToLower(out)
	var kind string
	switch {
	case strings.HasSuffix(lower, ".zip"):
		kind = "zip"
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		kind = "tgz"
	default:
		return errors.New("das Archiv muss auf .zip, .tar.gz oder .tgz enden")
	}
	if dir := path.Dir(out); dir != "." {
		if err := f.root.MkdirAll(dir, 0o755); err != nil {
			return translate(err)
		}
	}
	file, err := f.root.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fs.ErrExist
		}
		return translate(err)
	}
	var zw *zip.Writer
	var tw *tar.Writer
	var gz *gzip.Writer
	add := func(rel string, info fs.FileInfo) error {
		if rel == out {
			return nil // das Archiv nicht in sich selbst packen
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if kind == "tgz" {
			th, terr := tar.FileInfoHeader(info, "")
			if terr != nil {
				return terr
			}
			th.Name = rel
			if info.IsDir() {
				th.Name += "/"
			}
			if err := tw.WriteHeader(th); err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			header.Name = rel
			if info.IsDir() {
				header.Name += "/"
			} else {
				header.Method = zip.Deflate
			}
			w, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			return f.copyTo(w, rel)
		}
		if info.IsDir() {
			return nil
		}
		return f.copyTo(tw, rel)
	}
	if kind == "zip" {
		zw = zip.NewWriter(file)
	} else {
		gz = gzip.NewWriter(file)
		tw = tar.NewWriter(gz)
	}

	walkErr := func() error {
		for _, rel := range rels {
			info, err := f.root.Lstat(rel)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, translate(err))
			}
			if err := add(rel, info); err != nil {
				return err
			}
			if !info.IsDir() {
				continue
			}
			err = fs.WalkDir(f.root.FS(), rel, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if p == rel {
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				return add(p, info)
			})
			if err != nil {
				return err
			}
		}
		return nil
	}()

	var closeErr error
	if kind == "zip" {
		closeErr = zw.Close()
	} else {
		closeErr = errors.Join(tw.Close(), gz.Close())
	}
	closeErr = errors.Join(closeErr, file.Close())
	if err := errors.Join(walkErr, closeErr); err != nil {
		_ = f.root.Remove(out)
		return translate(err)
	}
	return nil
}

func (f *FS) copyTo(w io.Writer, rel string) error {
	src, err := f.root.Open(rel)
	if err != nil {
		return translate(err)
	}
	defer src.Close()
	_, err = io.Copy(w, src)
	return err
}

// Extract entpackt ein Archiv (.zip, .tar, .tar.gz, .tgz) in den Ordner dest. Vorhandene Dateien werden nie
// überschrieben, Symlinks und Pfade nach außen werden verworfen.
func (f *FS) Extract(name, dest string) error {
	rel, err := clean(name)
	if err != nil {
		return err
	}
	dir, err := clean(dest)
	if err != nil {
		return err
	}
	if rel == "." {
		return errors.New("ungültiges Archiv")
	}
	src, err := f.root.Open(rel)
	if err != nil {
		return translate(err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("das ist ein Ordner")
	}
	if dir != "." {
		if err := f.root.MkdirAll(dir, 0o755); err != nil {
			return translate(err)
		}
	}
	budget := &extractBudget{bytes: maxExtractBytes, entries: maxExtractEntries}
	lower := strings.ToLower(rel)
	switch {
	case strings.HasSuffix(lower, ".zip"), strings.HasSuffix(lower, ".jar"):
		return f.extractZip(src, info.Size(), dir, budget)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		gz, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("kein gültiges gzip-Archiv: %w", err)
		}
		defer gz.Close()
		return f.extractTar(gz, dir, budget)
	case strings.HasSuffix(lower, ".tar"):
		return f.extractTar(src, dir, budget)
	}
	return errors.New("nur .zip, .tar, .tar.gz und .tgz können entpackt werden")
}

type extractBudget struct {
	bytes   int64
	entries int
}

func (b *extractBudget) entry() error {
	b.entries--
	if b.entries < 0 {
		return errors.New("das Archiv enthält zu viele Einträge")
	}
	return nil
}

// target liefert den sicheren Zielpfad für einen Archiveintrag ("" = überspringen).
func target(dir, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("unzulässiger Pfad im Archiv: %s", name)
		}
	}
	rel, err := clean(name)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", nil
	}
	return path.Join(dir, rel), nil
}

func (f *FS) place(rel string, r io.Reader, perm fs.FileMode, budget *extractBudget) error {
	if parent := path.Dir(rel); parent != "." {
		if err := f.root.MkdirAll(parent, 0o755); err != nil {
			return translate(err)
		}
	}
	file, err := f.root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm|0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s: %w", rel, fs.ErrExist)
		}
		return translate(err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(r, budget.bytes+1))
	closeErr := file.Close()
	budget.bytes -= written
	if copyErr == nil && budget.bytes < 0 {
		copyErr = errors.New("das Archiv ist entpackt zu groß")
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = f.root.Remove(rel)
		return err
	}
	return nil
}

func (f *FS) extractZip(src io.ReaderAt, size int64, dir string, budget *extractBudget) error {
	reader, err := zip.NewReader(src, size)
	if err != nil {
		return fmt.Errorf("kein gültiges Zip-Archiv: %w", err)
	}
	for _, entry := range reader.File {
		if err := budget.entry(); err != nil {
			return err
		}
		rel, err := target(dir, entry.Name)
		if err != nil {
			return err
		}
		if rel == "" || entry.Mode()&fs.ModeSymlink != 0 {
			continue
		}
		if entry.FileInfo().IsDir() {
			if err := f.root.MkdirAll(rel, 0o755); err != nil {
				return translate(err)
			}
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		err = f.place(rel, rc, entry.Mode().Perm()&0o755, budget)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *FS) extractTar(src io.Reader, dir string, budget *extractBudget) error {
	reader := tar.NewReader(src)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("kein gültiges Tar-Archiv: %w", err)
		}
		if err := budget.entry(); err != nil {
			return err
		}
		rel, err := target(dir, header.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := f.root.MkdirAll(rel, 0o755); err != nil {
				return translate(err)
			}
		case tar.TypeReg:
			if err := f.place(rel, reader, fs.FileMode(header.Mode).Perm()&0o755, budget); err != nil {
				return err
			}
		}
	}
}
