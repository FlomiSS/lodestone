// lodestone-node ist der Daemon von Lodestone: Er läuft auf jeder Maschine und verwaltet dort die Minecraft-Server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
	"github.com/lodestone-mc/lodestone/internal/software"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "config.json", "Pfad zur Config (wird mit einem zufälligen Token angelegt, wenn sie fehlt)")
	showVersion := flag.Bool("version", false, "Version ausgeben und beenden")
	updateSelf := flag.String("update-self", "", "ID des Containers, der ersetzt werden soll (nur intern vom Updater)")
	updateImage := flag.String("update-image", "", "Image für den neuen Container")
	flag.Parse()
	if *showVersion {
		fmt.Println("lodestone-node", version)
		return
	}
	if *updateSelf != "" {
		if err := node.RunSelfUpdate(*updateSelf, *updateImage); err != nil {
			fmt.Fprintf(os.Stderr, "Update fehlgeschlagen: %v\n", err)
			os.Exit(1)
		}
		return
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(*configPath, logger); err != nil {
		logger.Error("lodestone-node beendet sich", "error", err)
		os.Exit(1)
	}
}

func run(configPath string, logger *slog.Logger) error {
	config, created, err := node.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if created {
		logger.Info("Config angelegt", "path", configPath)
		fmt.Printf("\nToken für das Panel (steht auch in %s):\n  %s\n\n", configPath, config.Token)
	}

	runtime, err := node.NewDockerRuntime(config.DockerHost)
	if err != nil {
		return fmt.Errorf("Docker-Anbindung: %w", err)
	}
	software.InitPackManager(filepath.Join(config.DataDir, "packs"))
	resolver := software.NewResolver()
	manager := node.NewManager(config, runtime, resolver, logger)
	defer manager.Close()
	if err := manager.Load(); err != nil {
		return fmt.Errorf("Server laden: %w", err)
	}
	logger.Info("Server geladen", "count", len(manager.List()), "dataDir", config.DataDir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go manager.KeepReconciling(ctx)

	api := node.NewAPI(config, manager, runtime, resolver, logger, version)
	server := &http.Server{
		Addr:              config.Listen,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() {
		logger.Info("Schnittstelle läuft", "listen", config.Listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	// Die Server laufen in ihren Containern weiter; der Daemon übernimmt sie beim nächsten Start wieder.
	logger.Info("Der Daemon wird beendet, laufende Server bleiben an")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
