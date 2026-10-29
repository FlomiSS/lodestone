package panel

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------- Gruppen

// Group ist eine Vorlage für Server eines Netzwerks. Das Panel hält zwischen min und max Instanzen am Laufen.
type Group struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Software  string `json:"software"`
	Version   string `json:"version"`
	MemoryMB  int    `json:"memoryMB"`
	// MinInstances und MaxInstances begrenzen die Zahl der Instanzen.
	MinInstances int `json:"minInstances"`
	MaxInstances int `json:"maxInstances"`
	// PlayersPerInstance ist die Zahl der Spieler, für die eine Instanz gedacht ist; ab 80 % Auslastung kommt eine dazu.
	PlayersPerInstance int `json:"playersPerInstance"`
	// IdleMinutes sagt, wie lange eine leere Instanz überzählig sein darf, bevor sie gestoppt wird.
	IdleMinutes int  `json:"idleMinutes"`
	InTry       bool `json:"inTry"`
	// Dynamic: Instanzen werden beim Herunterskalieren samt Dateien gelöscht. Sonst werden sie nur gestoppt.
	Dynamic bool `json:"dynamic"`
	// Plugins sind Modrinth-Projekte, die auf jede Instanz kommen.
	Plugins   []string `json:"plugins"`
	CreatedAt int64    `json:"createdAt"`
}

const groupColumns = "id, project_id, name, slug, software, version, memory_mb, min_instances, max_instances, players_per_instance, idle_minutes, in_try, dynamic, plugins, created_at"

func scanGroup(row interface{ Scan(...any) error }) (Group, error) {
	var group Group
	var inTry, dynamic int
	var plugins string
	if err := row.Scan(&group.ID, &group.ProjectID, &group.Name, &group.Slug, &group.Software, &group.Version, &group.MemoryMB,
		&group.MinInstances, &group.MaxInstances, &group.PlayersPerInstance, &group.IdleMinutes, &inTry, &dynamic, &plugins, &group.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Group{}, ErrNotFound
		}
		return Group{}, err
	}
	group.InTry, group.Dynamic = inTry == 1, dynamic == 1
	group.Plugins = []string{}
	for _, plugin := range strings.Split(plugins, ",") {
		if plugin = strings.TrimSpace(plugin); plugin != "" {
			group.Plugins = append(group.Plugins, plugin)
		}
	}
	return group, nil
}

func (s *Store) CreateGroup(group Group) (Group, error) {
	group.ID, group.CreatedAt = newID(), time.Now().UnixMilli()
	_, err := s.db.Exec("INSERT INTO server_groups ("+groupColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		group.ID, group.ProjectID, group.Name, group.Slug, group.Software, group.Version, group.MemoryMB, group.MinInstances, group.MaxInstances,
		group.PlayersPerInstance, group.IdleMinutes, boolInt(group.InTry), boolInt(group.Dynamic), strings.Join(group.Plugins, ","), group.CreatedAt)
	if isUnique(err) {
		return Group{}, fmt.Errorf("%w: eine Gruppe mit diesem Namen gibt es schon", ErrExists)
	}
	return group, err
}

func (s *Store) GroupByID(id string) (Group, error) {
	return scanGroup(s.db.QueryRow("SELECT "+groupColumns+" FROM server_groups WHERE id = ?", id))
}

// ListGroups liefert die Gruppen eines Projekts; ohne Projekt alle.
func (s *Store) ListGroups(projectID string) ([]Group, error) {
	query, args := "SELECT "+groupColumns+" FROM server_groups ORDER BY name COLLATE NOCASE", []any{}
	if projectID != "" {
		query, args = "SELECT "+groupColumns+" FROM server_groups WHERE project_id = ? ORDER BY name COLLATE NOCASE", []any{projectID}
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []Group{}
	for rows.Next() {
		group, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) UpdateGroup(group Group) error {
	_, err := s.db.Exec("UPDATE server_groups SET name = ?, memory_mb = ?, min_instances = ?, max_instances = ?, players_per_instance = ?, idle_minutes = ?, in_try = ?, dynamic = ?, plugins = ?, version = ? WHERE id = ?",
		group.Name, group.MemoryMB, group.MinInstances, group.MaxInstances, group.PlayersPerInstance, group.IdleMinutes, boolInt(group.InTry), boolInt(group.Dynamic), strings.Join(group.Plugins, ","), group.Version, group.ID)
	return err
}

func (s *Store) DeleteGroup(id string) error {
	_, err := s.db.Exec("DELETE FROM server_groups WHERE id = ?", id)
	return err
}

// ListGroupServers liefert die Instanzen einer Gruppe.
func (s *Store) ListGroupServers(groupID string) ([]ServerRow, error) {
	rows, err := s.db.Query("SELECT "+serverColumns+" FROM servers WHERE group_id = ? ORDER BY created_at", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := []ServerRow{}
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

// ---------------------------------------------------------------- Messwerte

// Sample ist ein Messpunkt eines Servers.
type Sample struct {
	Time    int64   `json:"t"`
	CPU     float64 `json:"cpu"`
	Memory  int64   `json:"mem"`
	Players int     `json:"players"`
}

func (s *Store) AddSamples(serverIDs []string, samples []Sample) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare("INSERT OR REPLACE INTO metrics (server_id, ts, cpu, mem, players) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer statement.Close()
	for i, sample := range samples {
		if _, err := statement.Exec(serverIDs[i], sample.Time, sample.CPU, sample.Memory, sample.Players); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Samples liefert die Messpunkte der Server seit "since", aufsteigend. Bei mehreren Servern kommen alle Reihen.
func (s *Store) Samples(serverID string, since int64) ([]Sample, error) {
	rows, err := s.db.Query("SELECT ts, cpu, mem, players FROM metrics WHERE server_id = ? AND ts >= ? ORDER BY ts", serverID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples := []Sample{}
	for rows.Next() {
		var sample Sample
		if err := rows.Scan(&sample.Time, &sample.CPU, &sample.Memory, &sample.Players); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *Store) PurgeMetrics(before int64) error {
	if _, err := s.db.Exec("DELETE FROM metrics WHERE ts < ?", before); err != nil {
		return err
	}
	_, err := s.db.Exec("DELETE FROM seen_players WHERE last_seen < ?", before-30*24*int64(time.Hour/time.Millisecond))
	return err
}

func (s *Store) DeleteServerMetrics(serverID string) error {
	if _, err := s.db.Exec("DELETE FROM metrics WHERE server_id = ?", serverID); err != nil {
		return err
	}
	_, err := s.db.Exec("DELETE FROM seen_players WHERE server_id = ?", serverID)
	return err
}

// ---------------------------------------------------------------- Gesehene Spieler

type SeenPlayer struct {
	Name      string `json:"name"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	Seconds   int64  `json:"seconds"`
}

// TouchPlayers merkt sich, dass die Spieler jetzt online sind; seconds ist die Zeit seit der letzten Messung.
func (s *Store) TouchPlayers(serverID string, names []string, now int64, seconds int64) error {
	if len(names) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range names {
		if _, err := tx.Exec(`INSERT INTO seen_players (server_id, name, first_seen, last_seen, seconds) VALUES (?, ?, ?, ?, 0)
			ON CONFLICT(server_id, name) DO UPDATE SET last_seen = excluded.last_seen, seconds = seconds + ?`, serverID, name, now, now, seconds); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SeenPlayers(serverID string, limit int) ([]SeenPlayer, error) {
	rows, err := s.db.Query("SELECT name, first_seen, last_seen, seconds FROM seen_players WHERE server_id = ? ORDER BY last_seen DESC LIMIT ?", serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	players := []SeenPlayer{}
	for rows.Next() {
		var player SeenPlayer
		if err := rows.Scan(&player.Name, &player.FirstSeen, &player.LastSeen, &player.Seconds); err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	return players, rows.Err()
}
