package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// DockerRuntime setzt Runtime mit der Docker Engine API um.
type DockerRuntime struct {
	cli *client.Client
}

// NewDockerRuntime verbindet mit Docker (DOCKER_HOST oder dem Standard der Plattform). Es wird noch nicht geprüft,
// ob der Dienst läuft; das tut Info.
func NewDockerRuntime(host string) (*DockerRuntime, error) {
	options := []client.Opt{client.FromEnv}
	if host == "" && os.Getenv("DOCKER_HOST") == "" {
		host = defaultHost()
	}
	if host != "" {
		options = append(options, client.WithHost(host))
	}
	cli, err := client.New(options...)
	if err != nil {
		return nil, err
	}
	return &DockerRuntime{cli: cli}, nil
}

// defaultHost wählt unter Windows die Pipe, die es wirklich gibt: Docker Desktop nutzt je nach Version
// dockerDesktopLinuxEngine (wie das docker-Programm) oder die ältere docker_engine. Sonst gilt der Standard des SDK.
func defaultHost() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	for _, name := range []string{"docker_engine", "dockerDesktopLinuxEngine"} {
		if _, err := os.Stat(`\\.\pipe\` + name); err == nil {
			return "npipe:////./pipe/" + name
		}
	}
	return ""
}

func (d *DockerRuntime) Info(ctx context.Context) (SystemInfo, error) {
	result, err := d.cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return SystemInfo{}, err
	}
	info := result.Info
	return SystemInfo{DockerVersion: info.ServerVersion, OS: info.OperatingSystem, Architecture: info.Architecture, CPUs: info.NCPU, MemoryBytes: info.MemTotal}, nil
}

func (d *DockerRuntime) EnsureNetwork(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	if _, err := d.cli.NetworkInspect(ctx, name, client.NetworkInspectOptions{}); err == nil {
		return nil
	} else if !cerrdefs.IsNotFound(err) {
		return err
	}
	_, err := d.cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge", Labels: map[string]string{LabelServer + ".network": "true"}})
	if err != nil && cerrdefs.IsConflict(err) {
		return nil // ein anderer Aufruf war schneller
	}
	return err
}

func (d *DockerRuntime) EnsureImage(ctx context.Context, image string) error {
	if _, err := d.cli.ImageInspect(ctx, image); err == nil {
		return nil
	} else if !cerrdefs.IsNotFound(err) {
		return err
	}
	response, err := d.cli.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("Image %s konnte nicht geladen werden: %w", image, err)
	}
	defer response.Close()
	return response.Wait(ctx)
}

func (d *DockerRuntime) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	exposed := network.PortSet{}
	bindings := network.PortMap{}
	for _, mapping := range spec.Ports {
		protocol := network.TCP
		if mapping.Protocol == "udp" {
			protocol = network.UDP
		}
		port, ok := network.PortFrom(uint16(mapping.Container), protocol)
		if !ok {
			return "", fmt.Errorf("ungültiger Port %d", mapping.Container)
		}
		exposed[port] = struct{}{}
		bindings[port] = []network.PortBinding{{HostPort: fmt.Sprint(mapping.Host)}}
	}
	pids := int64(4096)
	init := true
	result, err := d.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: spec.Name,
		Config: &container.Config{
			Image:        spec.Image,
			Cmd:          spec.Cmd,
			Env:          spec.Env,
			WorkingDir:   spec.WorkDir,
			User:         spec.User,
			Labels:       spec.Labels,
			ExposedPorts: exposed,
			Tty:          true,
			OpenStdin:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
		},
		HostConfig: &container.HostConfig{
			NetworkMode:   container.NetworkMode(spec.Network),
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(spec.Restart)},
			Binds:         spec.Binds,
			PortBindings:  bindings,
			Init:          &init,
			// Ein Minecraft-Server braucht kaum Rechte: alles weg, nur das Nötigste zurück.
			CapDrop:     []string{"ALL"},
			CapAdd:      []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID", "NET_BIND_SERVICE"},
			SecurityOpt: []string{"no-new-privileges"},
			Resources: container.Resources{
				Memory:     spec.MemoryBytes,
				MemorySwap: spec.MemoryBytes, // kein Swap
				NanoCPUs:   spec.NanoCPUs,
				PidsLimit:  &pids,
			},
		},
	})
	if err != nil {
		return "", err
	}
	return result.ID, nil
}

// Logs liest die letzten Zeilen (der Container hat ein TTY, die Ausgabe kommt deshalb ohne Kopfzeilen).
func (d *DockerRuntime) Logs(ctx context.Context, id string, lines int) (string, error) {
	result, err := d.cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: fmt.Sprint(lines)})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	defer result.Close()
	data, err := io.ReadAll(io.LimitReader(result, 256<<10))
	return string(data), err
}

func (d *DockerRuntime) Start(ctx context.Context, id string) error {
	_, err := d.cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
	return err
}

func (d *DockerRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	_, err := d.cli.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &seconds})
	return ignoreGone(err)
}

func (d *DockerRuntime) Kill(ctx context.Context, id string) error {
	_, err := d.cli.ContainerKill(ctx, id, client.ContainerKillOptions{})
	return ignoreGone(err)
}

func (d *DockerRuntime) Remove(ctx context.Context, id string) error {
	_, err := d.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
	return ignoreGone(err)
}

// ignoreGone behandelt "gibt es nicht" und "läuft nicht" als Erfolg: Das Ziel ist ohnehin erreicht.
func ignoreGone(err error) error {
	if err == nil || cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) || cerrdefs.IsNotModified(err) {
		return nil
	}
	return err
}

func (d *DockerRuntime) Inspect(ctx context.Context, id string) (ContainerState, error) {
	result, err := d.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if cerrdefs.IsNotFound(err) {
		return ContainerState{}, nil
	}
	if err != nil {
		return ContainerState{}, err
	}
	state := result.Container.State
	if state == nil {
		return ContainerState{Exists: true}, nil
	}
	return ContainerState{Exists: true, Running: state.Running, ExitCode: state.ExitCode}, nil
}

type attached struct {
	reader io.Reader
	closer func()
	writer io.Writer
}

func (a *attached) Read(p []byte) (int, error)  { return a.reader.Read(p) }
func (a *attached) Write(p []byte) (int, error) { return a.writer.Write(p) }
func (a *attached) Close() error                { a.closer(); return nil }

func (d *DockerRuntime) Attach(ctx context.Context, id string, replay bool) (io.ReadWriteCloser, error) {
	result, err := d.cli.ContainerAttach(ctx, id, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true, Logs: replay})
	if err != nil {
		return nil, err
	}
	response := result.HijackedResponse
	return &attached{reader: response.Reader, writer: response.Conn, closer: response.Close}, nil
}

func (d *DockerRuntime) Wait(ctx context.Context, id string) (int, error) {
	wait := d.cli.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case result := <-wait.Result:
		return int(result.StatusCode), nil
	case err := <-wait.Error:
		if cerrdefs.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (d *DockerRuntime) StatsStream(ctx context.Context, id string, report func(Stats)) error {
	result, err := d.cli.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: true})
	if err != nil {
		return err
	}
	defer result.Body.Close()
	decoder := json.NewDecoder(result.Body)
	var previous container.StatsResponse
	first := true
	for {
		var sample container.StatsResponse
		if err := decoder.Decode(&sample); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		stats := Stats{MemoryBytes: memoryUsage(sample), MemoryLimit: sample.MemoryStats.Limit}
		for _, net := range sample.Networks {
			stats.NetRx += net.RxBytes
			stats.NetTx += net.TxBytes
		}
		if !first {
			cpuDelta := float64(sample.CPUStats.CPUUsage.TotalUsage) - float64(previous.CPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(sample.CPUStats.SystemUsage) - float64(previous.CPUStats.SystemUsage)
			cpus := float64(sample.CPUStats.OnlineCPUs)
			if cpus == 0 {
				cpus = float64(len(sample.CPUStats.CPUUsage.PercpuUsage))
			}
			if systemDelta > 0 && cpuDelta > 0 && cpus > 0 {
				stats.CPUPercent = cpuDelta / systemDelta * cpus * 100
			}
		}
		first = false
		previous = sample
		report(stats)
	}
}

// memoryUsage rechnet den Zwischenspeicher heraus, wie es "docker stats" auch tut.
func memoryUsage(sample container.StatsResponse) uint64 {
	usage := sample.MemoryStats.Usage
	for _, key := range []string{"inactive_file", "total_inactive_file", "cache"} {
		if cache, ok := sample.MemoryStats.Stats[key]; ok && cache < usage {
			return usage - cache
		}
	}
	return usage
}

func (d *DockerRuntime) List(ctx context.Context) ([]ContainerRef, error) {
	result, err := d.cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("label", LabelServer)})
	if err != nil {
		return nil, err
	}
	refs := make([]ContainerRef, 0, len(result.Items))
	for _, item := range result.Items {
		refs = append(refs, ContainerRef{ID: item.ID, ServerID: item.Labels[LabelServer], Running: string(item.State) == "running"})
	}
	return refs, nil
}
