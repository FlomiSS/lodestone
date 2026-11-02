// lodestone-panel ist das Panel: Weboberfläche und Schnittstelle für Benutzer, Projekte und Nodes.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // Zeitzonen auch ohne Datenbank des Systems

	"github.com/lodestone-mc/lodestone/internal/panel"
	"github.com/lodestone-mc/lodestone/internal/panel/webui"
)

var version = "dev"

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func flag(name string) bool {
	switch strings.ToLower(os.Getenv(name)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("Das Panel konnte nicht starten", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	dataDir := env("LODESTONE_DATA_DIR", "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "packs"), 0o700); err != nil {
		return err
	}
	store, err := panel.OpenStore(filepath.Join(dataDir, "lodestone.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	cfg := panel.Config{
		Listen:                env("LODESTONE_LISTEN", ":8480"),
		SecureCookies:         flag("LODESTONE_SECURE_COOKIES"),
		TrustProxy:            flag("LODESTONE_TRUST_PROXY"),
		AllowInternalWebhooks: flag("LODESTONE_WEBHOOK_ALLOW_INTERNAL"),
		Version:               version,
		DataDir:               dataDir,
	}
	if zone := env("LODESTONE_TZ", ""); zone != "" {
		location, err := time.LoadLocation(zone)
		if err != nil {
			return fmt.Errorf("LODESTONE_TZ=%q: %w", zone, err)
		}
		cfg.Location = location
	}
	if err := bootstrap(logger, store); err != nil {
		return err
	}

	p := panel.New(cfg, store, logger, webui.Handler())
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           p.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go p.Maintain(ctx)

	logger.Info("Lodestone Panel läuft", "address", cfg.Listen, "version", version)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// bootstrap legt beim ersten Start den Administrator und den ersten Node an, wenn sie in der Umgebung stehen.
func bootstrap(logger *slog.Logger, store *panel.Store) error {
	count, err := store.CountUsers()
	if err != nil {
		return err
	}
	if name := env("LODESTONE_ADMIN_USER", ""); count == 0 && name != "" {
		password := env("LODESTONE_ADMIN_PASSWORD", "")
		generated := false
		if password == "" {
			buffer := make([]byte, 12)
			_, _ = rand.Read(buffer)
			password, generated = hex.EncodeToString(buffer), true
		}
		hash, err := panel.HashPassword(password)
		if err != nil {
			return err
		}
		if _, err := store.CreateUser(name, hash, true); err != nil {
			return err
		}
		if generated {
			logger.Warn("Administrator angelegt – dieses Passwort wird nur jetzt angezeigt", "user", name, "password", password)
		} else {
			logger.Info("Administrator angelegt", "user", name)
		}
	}

	address, token := env("LODESTONE_NODE_URL", ""), env("LODESTONE_NODE_TOKEN", "")
	if address == "" || token == "" {
		return nil
	}
	node := panel.Node{
		Name:       env("LODESTONE_NODE_NAME", "local"),
		URL:        strings.TrimRight(address, "/"),
		Token:      token,
		PublicHost: env("LODESTONE_NODE_PUBLIC_HOST", ""),
	}
	nodes, err := store.ListNodes()
	if err != nil {
		return err
	}
	for _, existing := range nodes {
		if strings.EqualFold(existing.Name, node.Name) {
			node.ID = existing.ID
			if existing.URL != node.URL || existing.Token != node.Token || existing.PublicHost != node.PublicHost {
				if err := store.UpdateNode(node); err != nil {
					return err
				}
				logger.Info("Node aktualisiert", "name", node.Name)
			}
			return nil
		}
	}
	if _, err := store.CreateNode(node); err != nil {
		return err
	}
	logger.Info("Node angelegt", "name", node.Name, "url", node.URL)
	return nil
}
