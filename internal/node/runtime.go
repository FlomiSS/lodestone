package node

import (
	"context"
	"io"
	"time"
)

// ContainerSpec beschreibt, wie ein Server-Container gebaut wird.
type ContainerSpec struct {
	Name        string
	Image       string
	Cmd         []string
	Env         []string
	WorkDir     string
	Binds       []string
	Ports       []PortMapping
	MemoryBytes int64
	NanoCPUs    int64
	User        string
	Labels      map[string]string
	// Network ist das Docker-Netzwerk, in dem der Container läuft (leer = Standard, "host" = Netz der Maschine).
	Network string
	// Restart ist die Neustart-Regel von Docker (z. B. "unless-stopped"); leer heißt: nie.
	Restart string
}

// ContainerState ist der Zustand eines Containers.
type ContainerState struct {
	Exists   bool
	Running  bool
	ExitCode int
}

// ContainerRef verweist auf einen vorhandenen Container eines Servers.
type ContainerRef struct {
	ID       string
	ServerID string
	Running  bool
}

// Stats sind Messwerte eines laufenden Containers.
type Stats struct {
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryBytes uint64  `json:"memoryBytes"`
	MemoryLimit uint64  `json:"memoryLimit"`
	NetRx       uint64  `json:"netRx"`
	NetTx       uint64  `json:"netTx"`
}

// SystemInfo beschreibt die Maschine und den Docker-Dienst.
type SystemInfo struct {
	DockerVersion string `json:"dockerVersion"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	CPUs          int    `json:"cpus"`
	MemoryBytes   int64  `json:"memoryBytes"`
}

// Runtime ist alles, was der Daemon von der Container-Laufzeit braucht. Die Umsetzung für Docker steht in docker.go,
// die Tests nutzen eine Attrappe.
type Runtime interface {
	Info(ctx context.Context) (SystemInfo, error)
	// EnsureNetwork legt das Netzwerk an, falls es fehlt.
	EnsureNetwork(ctx context.Context, name string) error
	// EnsureImage lädt das Image, falls es noch fehlt.
	EnsureImage(ctx context.Context, image string) error
	Create(ctx context.Context, spec ContainerSpec) (id string, err error)
	Start(ctx context.Context, id string) error
	// Stop beendet den Container; nach timeout wird er hart beendet.
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Kill(ctx context.Context, id string) error
	Remove(ctx context.Context, id string) error
	Inspect(ctx context.Context, id string) (ContainerState, error)
	// Attach verbindet mit Ein- und Ausgabe des Containers (roher Strom, mit TTY). Mit replay kommt zuerst die
	// bisherige Ausgabe, damit ein neu gestarteter Daemon den Verlauf nicht verliert.
	Attach(ctx context.Context, id string, replay bool) (io.ReadWriteCloser, error)
	// Wait blockiert, bis der Container nicht mehr läuft, und gibt den Exit-Code zurück.
	Wait(ctx context.Context, id string) (int, error)
	// StatsStream meldet Messwerte, bis ctx endet oder der Container stoppt.
	StatsStream(ctx context.Context, id string, report func(Stats)) error
	// List findet alle Container, die dieser Daemon angelegt hat (Label lodestone.server).
	List(ctx context.Context) ([]ContainerRef, error)
	// Logs liefert die letzten Zeilen der Ausgabe eines Containers.
	Logs(ctx context.Context, id string, lines int) (string, error)
}

// Labels, mit denen der Daemon seine Container markiert.
const (
	LabelServer = "lodestone.server"
	// LabelService markiert Hilfsdienste des Daemons (z. B. den playit-Agent); sie sind keine Minecraft-Server.
	LabelService = "lodestone.service"
)
