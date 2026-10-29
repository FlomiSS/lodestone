package panel

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const schemaMore = `
CREATE TABLE IF NOT EXISTS schedules (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  steps TEXT NOT NULL,
  kind TEXT NOT NULL,
  every_minutes INTEGER NOT NULL DEFAULT 0,
  at_minute INTEGER NOT NULL DEFAULT 0,
  weekdays INTEGER NOT NULL DEFAULT 127,
  warn_minutes INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_run INTEGER NOT NULL DEFAULT 0,
  last_status TEXT NOT NULL DEFAULT '',
  last_message TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS schedules_project ON schedules(project_id);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  project_id TEXT NOT NULL,
  server_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  level TEXT NOT NULL,
  message TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS events_project ON events(project_id, ts);
CREATE TABLE IF NOT EXISTS webhooks (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  kind TEXT NOT NULL,
  events TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_status TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  ip TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_ts ON audit(ts);
CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  prefix TEXT NOT NULL,
  hash TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL,
  last_used INTEGER NOT NULL DEFAULT 0,
  expires_at INTEGER NOT NULL DEFAULT 0
);
`

// moreColumns sind Spalten, die erst nach Version 0.1 dazukamen.
var moreColumns = []struct{ table, name, definition string }{
	{"users", "totp_secret", "TEXT NOT NULL DEFAULT ''"},
	{"users", "totp_enabled", "INTEGER NOT NULL DEFAULT 0"},
	{"users", "totp_recovery", "TEXT NOT NULL DEFAULT ''"},
	{"sessions", "ip", "TEXT NOT NULL DEFAULT ''"},
	{"servers", "auto_restart", "INTEGER NOT NULL DEFAULT 0"},
	{"projects", "status_public", "INTEGER NOT NULL DEFAULT 0"},
	{"projects", "status_slug", "TEXT NOT NULL DEFAULT ''"},
	{"projects", "motd", "TEXT NOT NULL DEFAULT ''"},
}

// ---------------------------------------------------------------- Zeitpläne

// Step ist ein Schritt eines Zeitplans.
type Step struct {
	// Action: backup, command, restart, start, stop oder wait.
	Action string `json:"action"`
	// Value ist der Befehl (command), die Bezeichnung (backup) oder die Sekunden (wait).
	Value string `json:"value,omitempty"`
}

// Schedule führt zu festen Zeiten Schritte auf einem Server aus.
type Schedule struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	ServerID  string `json:"serverId"`
	Name      string `json:"name"`
	Steps     []Step `json:"steps"`
	// Kind: interval (alle EveryMinutes), daily (um AtMinute) oder weekly (an den Wochentagen um AtMinute).
	Kind         string `json:"kind"`
	EveryMinutes int    `json:"everyMinutes"`
	AtMinute     int    `json:"atMinute"`
	// Weekdays ist eine Bitmaske, Bit 0 = Montag … Bit 6 = Sonntag.
	Weekdays    int    `json:"weekdays"`
	WarnMinutes int    `json:"warnMinutes"`
	Enabled     bool   `json:"enabled"`
	LastRun     int64  `json:"lastRun"`
	LastStatus  string `json:"lastStatus"`
	LastMessage string `json:"lastMessage"`
	CreatedAt   int64  `json:"createdAt"`
}

const scheduleColumns = "id, project_id, server_id, name, steps, kind, every_minutes, at_minute, weekdays, warn_minutes, enabled, last_run, last_status, last_message, created_at"

func scanSchedule(row interface{ Scan(...any) error }) (Schedule, error) {
	var schedule Schedule
	var steps string
	var enabled int
	if err := row.Scan(&schedule.ID, &schedule.ProjectID, &schedule.ServerID, &schedule.Name, &steps, &schedule.Kind, &schedule.EveryMinutes, &schedule.AtMinute,
		&schedule.Weekdays, &schedule.WarnMinutes, &enabled, &schedule.LastRun, &schedule.LastStatus, &schedule.LastMessage, &schedule.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Schedule{}, ErrNotFound
		}
		return Schedule{}, err
	}
	schedule.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(steps), &schedule.Steps); err != nil {
		schedule.Steps = []Step{}
	}
	return schedule, nil
}

func (s *Store) CreateSchedule(schedule Schedule) (Schedule, error) {
	schedule.ID, schedule.CreatedAt = newID(), time.Now().UnixMilli()
	steps, _ := json.Marshal(schedule.Steps)
	_, err := s.db.Exec("INSERT INTO schedules ("+scheduleColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		schedule.ID, schedule.ProjectID, schedule.ServerID, schedule.Name, string(steps), schedule.Kind, schedule.EveryMinutes, schedule.AtMinute,
		schedule.Weekdays, schedule.WarnMinutes, boolInt(schedule.Enabled), schedule.LastRun, schedule.LastStatus, schedule.LastMessage, schedule.CreatedAt)
	return schedule, err
}

func (s *Store) ScheduleByID(id string) (Schedule, error) {
	return scanSchedule(s.db.QueryRow("SELECT "+scheduleColumns+" FROM schedules WHERE id = ?", id))
}

// ListSchedules liefert die Zeitpläne eines Projekts; ohne Projekt alle.
func (s *Store) ListSchedules(projectID string) ([]Schedule, error) {
	query, args := "SELECT "+scheduleColumns+" FROM schedules ORDER BY name COLLATE NOCASE", []any{}
	if projectID != "" {
		query, args = "SELECT "+scheduleColumns+" FROM schedules WHERE project_id = ? ORDER BY name COLLATE NOCASE", []any{projectID}
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Schedule{}
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, schedule)
	}
	return list, rows.Err()
}

func (s *Store) UpdateSchedule(schedule Schedule) error {
	steps, _ := json.Marshal(schedule.Steps)
	_, err := s.db.Exec("UPDATE schedules SET name = ?, steps = ?, kind = ?, every_minutes = ?, at_minute = ?, weekdays = ?, warn_minutes = ?, enabled = ? WHERE id = ?",
		schedule.Name, string(steps), schedule.Kind, schedule.EveryMinutes, schedule.AtMinute, schedule.Weekdays, schedule.WarnMinutes, boolInt(schedule.Enabled), schedule.ID)
	return err
}

func (s *Store) RecordScheduleRun(id string, at time.Time, status, message string) error {
	if len(message) > 300 {
		message = message[:300]
	}
	_, err := s.db.Exec("UPDATE schedules SET last_run = ?, last_status = ?, last_message = ? WHERE id = ?", at.UnixMilli(), status, message, id)
	return err
}

func (s *Store) DeleteSchedule(id string) error {
	_, err := s.db.Exec("DELETE FROM schedules WHERE id = ?", id)
	return err
}

// ---------------------------------------------------------------- Ereignisse

// Event ist ein Eintrag im Verlauf eines Projekts (Absturz, Neustart, Backup …).
type Event struct {
	ID        int64  `json:"id"`
	Time      int64  `json:"time"`
	ProjectID string `json:"projectId"`
	ServerID  string `json:"serverId,omitempty"`
	Kind      string `json:"kind"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func (s *Store) AddEvent(event Event) (Event, error) {
	event.Time = time.Now().UnixMilli()
	if len(event.Message) > 500 {
		event.Message = event.Message[:500]
	}
	result, err := s.db.Exec("INSERT INTO events (ts, project_id, server_id, kind, level, message) VALUES (?, ?, ?, ?, ?, ?)",
		event.Time, event.ProjectID, event.ServerID, event.Kind, event.Level, event.Message)
	if err != nil {
		return event, err
	}
	event.ID, _ = result.LastInsertId()
	return event, nil
}

func (s *Store) ListEvents(projectID string, limit int) ([]Event, error) {
	rows, err := s.db.Query("SELECT id, ts, project_id, server_id, kind, level, message FROM events WHERE project_id = ? ORDER BY id DESC LIMIT ?", projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Event{}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Time, &event.ProjectID, &event.ServerID, &event.Kind, &event.Level, &event.Message); err != nil {
			return nil, err
		}
		list = append(list, event)
	}
	return list, rows.Err()
}

func (s *Store) PurgeEvents(before int64) error {
	_, err := s.db.Exec("DELETE FROM events WHERE ts < ?", before)
	return err
}

// ---------------------------------------------------------------- Webhooks

// Webhook ruft eine Adresse auf, wenn etwas im Projekt passiert.
type Webhook struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"projectId"`
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Kind       string   `json:"kind"` // discord oder generic
	Events     []string `json:"events"`
	Enabled    bool     `json:"enabled"`
	LastStatus string   `json:"lastStatus"`
	CreatedAt  int64    `json:"createdAt"`
}

const webhookColumns = "id, project_id, name, url, kind, events, enabled, last_status, created_at"

func scanWebhook(row interface{ Scan(...any) error }) (Webhook, error) {
	var hook Webhook
	var events string
	var enabled int
	if err := row.Scan(&hook.ID, &hook.ProjectID, &hook.Name, &hook.URL, &hook.Kind, &events, &enabled, &hook.LastStatus, &hook.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Webhook{}, ErrNotFound
		}
		return Webhook{}, err
	}
	hook.Enabled = enabled == 1
	hook.Events = []string{}
	for _, name := range strings.Split(events, ",") {
		if name = strings.TrimSpace(name); name != "" {
			hook.Events = append(hook.Events, name)
		}
	}
	return hook, nil
}

func (s *Store) CreateWebhook(hook Webhook) (Webhook, error) {
	hook.ID, hook.CreatedAt = newID(), time.Now().UnixMilli()
	_, err := s.db.Exec("INSERT INTO webhooks ("+webhookColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		hook.ID, hook.ProjectID, hook.Name, hook.URL, hook.Kind, strings.Join(hook.Events, ","), boolInt(hook.Enabled), hook.LastStatus, hook.CreatedAt)
	return hook, err
}

func (s *Store) WebhookByID(id string) (Webhook, error) {
	return scanWebhook(s.db.QueryRow("SELECT "+webhookColumns+" FROM webhooks WHERE id = ?", id))
}

func (s *Store) ListWebhooks(projectID string) ([]Webhook, error) {
	rows, err := s.db.Query("SELECT "+webhookColumns+" FROM webhooks WHERE project_id = ? ORDER BY name COLLATE NOCASE", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Webhook{}
	for rows.Next() {
		hook, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, hook)
	}
	return list, rows.Err()
}

func (s *Store) UpdateWebhook(hook Webhook) error {
	_, err := s.db.Exec("UPDATE webhooks SET name = ?, url = ?, kind = ?, events = ?, enabled = ? WHERE id = ?",
		hook.Name, hook.URL, hook.Kind, strings.Join(hook.Events, ","), boolInt(hook.Enabled), hook.ID)
	return err
}

func (s *Store) SetWebhookStatus(id, status string) error {
	if len(status) > 200 {
		status = status[:200]
	}
	_, err := s.db.Exec("UPDATE webhooks SET last_status = ? WHERE id = ?", status, id)
	return err
}

func (s *Store) DeleteWebhook(id string) error {
	_, err := s.db.Exec("DELETE FROM webhooks WHERE id = ?", id)
	return err
}

// ---------------------------------------------------------------- Audit-Log

// AuditEntry ist ein Eintrag im Protokoll.
type AuditEntry struct {
	ID       int64  `json:"id"`
	Time     int64  `json:"time"`
	UserID   string `json:"userId,omitempty"`
	Username string `json:"username"`
	Action   string `json:"action"`
	Target   string `json:"target,omitempty"`
	Status   int    `json:"status"`
	IP       string `json:"ip,omitempty"`
}

func (s *Store) AddAudit(entry AuditEntry) error {
	entry.Username, entry.Action, entry.Target = clip(entry.Username, 64), clip(entry.Action, 200), clip(entry.Target, 200)
	_, err := s.db.Exec("INSERT INTO audit (ts, user_id, username, action, target, status, ip) VALUES (?, ?, ?, ?, ?, ?, ?)",
		time.Now().UnixMilli(), entry.UserID, entry.Username, entry.Action, entry.Target, entry.Status, entry.IP)
	return err
}

func clip(text string, limit int) string {
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

// ListAudit liefert Einträge, neueste zuerst; before (0 = ab dem neuesten) ist die ID für die nächste Seite.
func (s *Store) ListAudit(limit int, before int64, username string) ([]AuditEntry, error) {
	query := "SELECT id, ts, user_id, username, action, target, status, ip FROM audit WHERE (? = 0 OR id < ?)"
	args := []any{before, before}
	if username != "" {
		query += " AND username = ? COLLATE NOCASE"
		args = append(args, username)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []AuditEntry{}
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Time, &entry.UserID, &entry.Username, &entry.Action, &entry.Target, &entry.Status, &entry.IP); err != nil {
			return nil, err
		}
		list = append(list, entry)
	}
	return list, rows.Err()
}

func (s *Store) PurgeAudit(before int64) error {
	_, err := s.db.Exec("DELETE FROM audit WHERE ts < ?", before)
	return err
}

// ---------------------------------------------------------------- API-Schlüssel

// APIKey ist ein persönlicher Zugriffsschlüssel (der Klartext wird nur beim Anlegen gezeigt).
type APIKey struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	CreatedAt int64  `json:"createdAt"`
	LastUsed  int64  `json:"lastUsed"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Store) CreateAPIKey(key APIKey, hash string) (APIKey, error) {
	key.ID, key.CreatedAt = newID(), time.Now().UnixMilli()
	_, err := s.db.Exec("INSERT INTO api_keys (id, user_id, name, prefix, hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		key.ID, key.UserID, key.Name, key.Prefix, hash, key.CreatedAt, key.ExpiresAt)
	return key, err
}

func (s *Store) ListAPIKeys(userID string) ([]APIKey, error) {
	rows, err := s.db.Query("SELECT id, user_id, name, prefix, created_at, last_used, expires_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []APIKey{}
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.CreatedAt, &key.LastUsed, &key.ExpiresAt); err != nil {
			return nil, err
		}
		list = append(list, key)
	}
	return list, rows.Err()
}

func (s *Store) DeleteAPIKey(userID, id string) error {
	result, err := s.db.Exec("DELETE FROM api_keys WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// APIKeyUser liefert den Benutzer zu einem Schlüssel (Hash) und merkt sich die Nutzung.
func (s *Store) APIKeyUser(hash string) (User, error) {
	row := s.db.QueryRow("SELECT u.id, u.username, u.password_hash, u.is_admin, u.disabled, u.created_at, k.id, k.expires_at FROM api_keys k JOIN users u ON u.id = k.user_id WHERE k.hash = ? AND u.disabled = 0", hash)
	var user User
	var admin, disabled int
	var keyID string
	var expires int64
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &admin, &disabled, &user.CreatedAt, &keyID, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if expires > 0 && expires < time.Now().UnixMilli() {
		return User{}, ErrNotFound
	}
	user.IsAdmin, user.Disabled = admin == 1, disabled == 1
	_, _ = s.db.Exec("UPDATE api_keys SET last_used = ? WHERE id = ?", time.Now().UnixMilli(), keyID)
	return user, nil
}

// ---------------------------------------------------------------- Zwei-Faktor und Sitzungen

// TotpState liefert den verschlüsselten Schlüssel, ob 2FA aktiv ist und die Hashes der Wiederherstellungscodes.
func (s *Store) TotpState(userID string) (secret string, enabled bool, recovery []string, err error) {
	var on int
	var codes string
	err = s.db.QueryRow("SELECT totp_secret, totp_enabled, totp_recovery FROM users WHERE id = ?", userID).Scan(&secret, &on, &codes)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil, ErrNotFound
	}
	for _, code := range strings.Split(codes, ",") {
		if code != "" {
			recovery = append(recovery, code)
		}
	}
	return secret, on == 1, recovery, err
}

func (s *Store) SetTotp(userID, secret string, enabled bool, recovery []string) error {
	_, err := s.db.Exec("UPDATE users SET totp_secret = ?, totp_enabled = ?, totp_recovery = ? WHERE id = ?", secret, boolInt(enabled), strings.Join(recovery, ","), userID)
	return err
}

// SessionInfo beschreibt eine angemeldete Sitzung.
type SessionInfo struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	UserAgent string `json:"userAgent"`
	IP        string `json:"ip"`
	Current   bool   `json:"current"`
}

func (s *Store) ListSessions(userID, currentHash string) ([]SessionInfo, error) {
	rows, err := s.db.Query("SELECT token_hash, created_at, expires_at, user_agent, ip FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC", userID, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []SessionInfo{}
	for rows.Next() {
		var info SessionInfo
		var hash string
		if err := rows.Scan(&hash, &info.CreatedAt, &info.ExpiresAt, &info.UserAgent, &info.IP); err != nil {
			return nil, err
		}
		info.ID, info.Current = hash[:16], hash == currentHash
		list = append(list, info)
	}
	return list, rows.Err()
}

// DeleteSessionByID beendet eine Sitzung des Benutzers anhand der Kurz-ID.
func (s *Store) DeleteSessionByID(userID, id string) error {
	if len(id) != 16 {
		return ErrNotFound
	}
	result, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ? AND substr(token_hash, 1, 16) = ?", userID, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetSessionIP(hash, ip string) {
	_, _ = s.db.Exec("UPDATE sessions SET ip = ? WHERE token_hash = ?", ip, hash)
}

// ---------------------------------------------------------------- Projekt- und Server-Zusätze

func (s *Store) SetAutoRestart(serverID string, enabled bool) error {
	_, err := s.db.Exec("UPDATE servers SET auto_restart = ? WHERE id = ?", boolInt(enabled), serverID)
	return err
}

func (s *Store) AutoRestart(serverID string) bool {
	var on int
	_ = s.db.QueryRow("SELECT auto_restart FROM servers WHERE id = ?", serverID).Scan(&on)
	return on == 1
}

// ProjectExtras sind Einstellungen, die nicht in Project stehen.
type ProjectExtras struct {
	StatusPublic bool   `json:"statusPublic"`
	StatusSlug   string `json:"statusSlug"`
	MOTD         string `json:"motd"`
}

func (s *Store) Extras(projectID string) (ProjectExtras, error) {
	var extras ProjectExtras
	var public int
	err := s.db.QueryRow("SELECT status_public, status_slug, motd FROM projects WHERE id = ?", projectID).Scan(&public, &extras.StatusSlug, &extras.MOTD)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectExtras{}, ErrNotFound
	}
	extras.StatusPublic = public == 1
	return extras, err
}

func (s *Store) SetExtras(projectID string, extras ProjectExtras) error {
	_, err := s.db.Exec("UPDATE projects SET status_public = ?, status_slug = ?, motd = ? WHERE id = ?", boolInt(extras.StatusPublic), extras.StatusSlug, extras.MOTD, projectID)
	if isUnique(err) {
		return fmt.Errorf("%w: diese Adresse der Statusseite ist vergeben", ErrExists)
	}
	return err
}

// ProjectByStatusSlug findet ein Projekt mit öffentlicher Statusseite.
func (s *Store) ProjectByStatusSlug(slug string) (Project, error) {
	return scanProject(s.db.QueryRow("SELECT "+projectColumns+" FROM projects WHERE status_public = 1 AND status_slug = ?", slug))
}
