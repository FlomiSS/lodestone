// Package webui enthält die gebaute Oberfläche (Next.js Static Export), die in das Panel eingebettet wird.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler liefert die Oberfläche aus. Unbekannte Seiten bekommen die 404-Seite, Dateien mit Hash im Namen werden
// lange zwischengespeichert.
func Handler() http.Handler {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return handlerFor(root)
}

func handlerFor(root fs.FS) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		candidates := []string{name, name + ".html", path.Join(name, "index.html")}
		for _, candidate := range candidates {
			info, err := fs.Stat(root, candidate)
			if err != nil || info.IsDir() {
				continue
			}
			if strings.HasPrefix(candidate, "_next/static/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			if candidate == name {
				files.ServeHTTP(w, r)
				return
			}
			serve(w, r, root, candidate)
			return
		}
		if page, err := fs.ReadFile(root, "404.html"); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(page)
			return
		}
		http.NotFound(w, r)
	})
}

func serve(w http.ResponseWriter, r *http.Request, root fs.FS, name string) {
	data, err := fs.ReadFile(root, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
