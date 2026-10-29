package panel

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
)

const (
	sampleEvery     = 30 * time.Second
	metricsKeepDays = 14
)

// liveInfo ist, was die letzte Messung über einen Server wusste.
type liveInfo struct {
	At         time.Time
	Players    int
	MaxPlayers int
	Names      []string
	Running    bool
}

// ---------------------------------------------------------------- Messung

// sampleLoop misst alle 30 Sekunden CPU, Speicher und Spieler aller Server und speichert sie.
func (p *Panel) sampleLoop(ctx context.Context) {
	ticker := time.NewTicker(sampleEvery)
	defer ticker.Stop()
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-purge.C:
			if err := p.store.PurgeMetrics(time.Now().Add(-metricsKeepDays * 24 * time.Hour).UnixMilli()); err != nil {
				p.log.Warn("Messwerte aufräumen fehlgeschlagen", "error", err)
			}
		case <-ticker.C:
			p.sampleOnce(ctx)
		}
	}
}

func (p *Panel) sampleOnce(ctx context.Context) {
	rows, err := p.store.ListServers("")
	if err != nil || len(rows) == 0 {
		return
	}
	now := time.Now()
	type result struct {
		row    ServerRow
		view   node.View
		info   node.PlayerInfo
		hasPly bool
	}
	results := make([]result, 0, len(rows))
	var mu sync.Mutex
	var group sync.WaitGroup
	limit := make(chan struct{}, 8)
	viewsByNode := map[string]map[string]node.View{}
	for _, row := range rows {
		if _, done := viewsByNode[row.NodeID]; !done {
			views, err := p.nodeViews(ctx, row.NodeID)
			if err != nil {
				views = nil
			}
			viewsByNode[row.NodeID] = views
		}
	}
	for _, row := range rows {
		view, ok := viewsByNode[row.NodeID][row.ID]
		if !ok || (view.State != node.StateRunning && view.State != node.StateStarting) {
			p.setLive(row.ID, liveInfo{At: now})
			continue
		}
		group.Add(1)
		limit <- struct{}{}
		go func() {
			defer func() { <-limit; group.Done() }()
			entry := result{row: row, view: view}
			if view.State == node.StateRunning {
				if client, _, err := p.client(row.NodeID); err == nil {
					callCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
					var info node.PlayerInfo
					if err := client.Do(callCtx, http.MethodGet, "/api/servers/"+row.ID+"/players", nil, &info); err == nil {
						entry.info, entry.hasPly = info, true
					}
					cancel()
				}
			}
			mu.Lock()
			results = append(results, entry)
			mu.Unlock()
		}()
	}
	group.Wait()

	ids := make([]string, 0, len(results))
	samples := make([]Sample, 0, len(results))
	for _, entry := range results {
		sample := Sample{Time: now.UnixMilli(), Players: entry.info.Online}
		if entry.view.Stats != nil {
			sample.CPU, sample.Memory = entry.view.Stats.CPUPercent, int64(entry.view.Stats.MemoryBytes)
		}
		ids = append(ids, entry.row.ID)
		samples = append(samples, sample)
		p.setLive(entry.row.ID, liveInfo{At: now, Players: entry.info.Online, MaxPlayers: entry.info.Max, Names: entry.info.Names, Running: true})
		if entry.hasPly && len(entry.info.Names) > 0 {
			_ = p.store.TouchPlayers(entry.row.ID, entry.info.Names, now.UnixMilli(), int64(sampleEvery/time.Second))
		}
	}
	if err := p.store.AddSamples(ids, samples); err != nil {
		p.log.Warn("Messwerte speichern fehlgeschlagen", "error", err)
	}
}

func (p *Panel) setLive(id string, info liveInfo) {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	if p.live == nil {
		p.live = map[string]liveInfo{}
	}
	p.live[id] = info
}

func (p *Panel) liveOf(id string) liveInfo {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	return p.live[id]
}

// ---------------------------------------------------------------- Auswertung

var rangeDurations = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "14d": 14 * 24 * time.Hour,
}

const chartPoints = 120

// Series ist ein Verlauf in gleich breiten Zeitfenstern. null heißt: in dem Fenster gab es keine Messung.
type Series struct {
	Range   string     `json:"range"`
	Times   []int64    `json:"times"`
	CPU     []*float64 `json:"cpu"`
	Memory  []*float64 `json:"memory"`
	Players []*float64 `json:"players"`
	// Zusammenfassung über den Zeitraum
	PeakPlayers int     `json:"peakPlayers"`
	AvgPlayers  float64 `json:"avgPlayers"`
	AvgCPU      float64 `json:"avgCpu"`
	PeakCPU     float64 `json:"peakCpu"`
	PeakMemory  int64   `json:"peakMemory"`
	Samples     int     `json:"samples"`
}

type bucket struct {
	cpuSum, memSum float64
	players        float64
	n              int
}

// buildSeries fasst Messreihen zusammen. Mehrere Server werden je Zeitfenster addiert (CPU, Speicher); bei den
// Spielern zählt die Reihe von playersFrom (z. B. der Proxy), sonst die Summe.
func buildSeries(rangeName string, now time.Time, perServer map[string][]Sample, playersFrom string) Series {
	duration := rangeDurations[rangeName]
	start := now.Add(-duration).UnixMilli()
	width := duration.Milliseconds() / chartPoints
	result := Series{Range: rangeName, Times: make([]int64, chartPoints), CPU: make([]*float64, chartPoints), Memory: make([]*float64, chartPoints), Players: make([]*float64, chartPoints)}
	buckets := make([]map[string]*bucket, chartPoints)
	for i := range result.Times {
		result.Times[i] = start + int64(i)*width + width/2
		buckets[i] = map[string]*bucket{}
	}
	for id, samples := range perServer {
		for _, sample := range samples {
			index := int((sample.Time - start) / width)
			if index < 0 || index >= chartPoints {
				continue
			}
			b := buckets[index][id]
			if b == nil {
				b = &bucket{}
				buckets[index][id] = b
			}
			b.cpuSum += sample.CPU
			b.memSum += float64(sample.Memory)
			b.players += float64(sample.Players)
			b.n++
		}
	}
	var cpuTotal, playerTotal float64
	var counted int
	for i, perID := range buckets {
		if len(perID) == 0 {
			continue
		}
		var cpu, memory, players float64
		for id, b := range perID {
			cpu += b.cpuSum / float64(b.n)
			memory += b.memSum / float64(b.n)
			if playersFrom == "" || playersFrom == id {
				players += b.players / float64(b.n)
			}
		}
		result.CPU[i], result.Memory[i], result.Players[i] = &cpu, &memory, &players
		result.Samples++
		counted++
		cpuTotal += cpu
		playerTotal += players
		if cpu > result.PeakCPU {
			result.PeakCPU = cpu
		}
		if int64(memory) > result.PeakMemory {
			result.PeakMemory = int64(memory)
		}
		if int(players+0.5) > result.PeakPlayers {
			result.PeakPlayers = int(players + 0.5)
		}
	}
	if counted > 0 {
		result.AvgCPU, result.AvgPlayers = cpuTotal/float64(counted), playerTotal/float64(counted)
	}
	return result
}

func rangeParam(r *http.Request) string {
	name := r.URL.Query().Get("range")
	if _, ok := rangeDurations[name]; !ok {
		return "6h"
	}
	return name
}

func (p *Panel) serverMetrics(w http.ResponseWriter, r *http.Request) {
	row, _, _, err := p.requireServer(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	name, now := rangeParam(r), time.Now()
	samples, err := p.store.Samples(row.ID, now.Add(-rangeDurations[name]).UnixMilli())
	if err != nil {
		writeError(w, err)
		return
	}
	series := buildSeries(name, now, map[string][]Sample{row.ID: samples}, "")
	live := p.liveOf(row.ID)
	writeJSON(w, http.StatusOK, map[string]any{"series": series, "maxPlayers": live.MaxPlayers, "players": live.Players, "names": live.Names})
}

func (p *Panel) projectMetrics(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := p.store.ListServers(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	name, now := rangeParam(r), time.Now()
	since := now.Add(-rangeDurations[name]).UnixMilli()
	perServer := map[string][]Sample{}
	playersFrom, online := "", 0
	for _, row := range rows {
		samples, err := p.store.Samples(row.ID, since)
		if err != nil {
			writeError(w, err)
			return
		}
		perServer[row.ID] = samples
		if row.Role == RoleProxy {
			playersFrom = row.ID
		}
	}
	// Im Netzwerk zählt der Proxy (sieht alle Spieler), sonst die Summe der Server
	for _, row := range rows {
		if playersFrom == "" || row.ID == playersFrom {
			online += p.liveOf(row.ID).Players
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": buildSeries(name, now, perServer, playersFrom), "players": online})
}

func (p *Panel) serverPlayerHistory(w http.ResponseWriter, r *http.Request) {
	row, _, _, err := p.requireServer(r, rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	seen, err := p.store.SeenPlayers(row.ID, 200)
	if err != nil {
		writeError(w, err)
		return
	}
	live := p.liveOf(row.ID)
	online := map[string]bool{}
	for _, name := range live.Names {
		online[name] = true
	}
	type entry struct {
		SeenPlayer
		Online bool `json:"online"`
	}
	list := make([]entry, 0, len(seen))
	for _, player := range seen {
		list = append(list, entry{SeenPlayer: player, Online: online[player.Name]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"players": list, "online": live.Players, "max": live.MaxPlayers})
}
