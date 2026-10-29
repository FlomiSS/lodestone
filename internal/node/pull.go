package node

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const maxPullBytes = 256 << 20

// PullRequest lädt eine Plugin- oder Mod-Datei von einer Adresse in den Serverordner.
type PullRequest struct {
	URL       string `json:"url"`
	Path      string `json:"path"` // z. B. plugins/LuckPerms.jar
	SHA1      string `json:"sha1,omitempty"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

func allowedHost(host string, allowed []string) bool {
	host = strings.ToLower(host)
	for _, entry := range allowed {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "*" || host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}

// validatePullPath erlaubt nur .jar-Dateien in plugins/ oder mods/, damit sich damit keine Config überschreiben lässt.
func validatePullPath(name string) (string, error) {
	rel, err := clean(name)
	if err != nil {
		return "", err
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 2 || (parts[0] != "plugins" && parts[0] != "mods") || !strings.HasSuffix(strings.ToLower(rel), ".jar") {
		return "", badRequest("Dateien lassen sich nur als .jar in plugins/ oder mods/ laden")
	}
	return rel, nil
}

// Pull lädt die Datei. Jede Weiterleitung wird erneut gegen die erlaubten Hosts geprüft, es gilt nur https.
func (m *Manager) Pull(ctx context.Context, s *Server, request PullRequest) error {
	rel, err := validatePullPath(request.Path)
	if err != nil {
		return err
	}
	address, err := url.Parse(request.URL)
	if err != nil || address.Host == "" {
		return badRequest("ungültige Adresse")
	}
	if !m.checkPullURL(address) {
		return badRequest("von %q darf nichts geladen werden (erlaubt: https und die Hosts in der Config)", address.Hostname())
	}
	if s.fs.Exists(rel) && !request.Overwrite {
		return fmt.Errorf("%w: %s gibt es schon", ErrConflict, rel)
	}
	if dir := path.Dir(rel); dir != "." {
		if err := s.fs.Mkdir(dir); err != nil {
			return err
		}
	}

	client := &http.Client{}
	if m.pullClient != nil {
		copy := *m.pullClient
		client = &copy
	}
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("zu viele Weiterleitungen")
		}
		if !m.checkPullURL(next.URL) {
			return fmt.Errorf("die Weiterleitung auf %q ist nicht erlaubt", next.URL.Hostname())
		}
		return nil
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return badRequest("ungültige Adresse")
	}
	httpRequest.Header.Set("User-Agent", "Lodestone")
	response, err := client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("Download fehlgeschlagen: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Download fehlgeschlagen: Status %d", response.StatusCode)
	}
	if response.ContentLength > maxPullBytes {
		return fmt.Errorf("die Datei ist größer als %d MB", maxPullBytes>>20)
	}

	part := rel + ".lodestone-part"
	_ = s.fs.Remove(part, false)
	hash := sha1.New()
	if err := s.fs.Write(part, io.TeeReader(response.Body, hash), maxPullBytes, true); err != nil {
		return err
	}
	if want := strings.ToLower(strings.TrimSpace(request.SHA1)); want != "" && hex.EncodeToString(hash.Sum(nil)) != want {
		_ = s.fs.Remove(part, false)
		return errors.New("die Prüfsumme der Datei stimmt nicht")
	}
	if request.Overwrite {
		_ = s.fs.Remove(rel, false)
	}
	if err := s.fs.Rename(part, rel); err != nil {
		_ = s.fs.Remove(part, false)
		return err
	}
	return nil
}

func (m *Manager) checkPullURL(address *url.URL) bool {
	if address.Scheme != "https" && !(m.insecurePull && address.Scheme == "http") {
		return false
	}
	return allowedHost(address.Hostname(), m.cfg.PullHosts)
}
