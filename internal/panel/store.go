// Package panel ist das Backend von Lodestone: Benutzer, Projekte, Nodes und die Verwaltung der Server darauf.
package panel

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // reiner Go-Treiber, kein cgo nötig
)

// Store ist die Datenbank (SQLite).
type Store struct{ db *sql.DB }

var (
	ErrNotFound = errors.New("nicht gefunden")
	ErrExists   = errors.New("gibt es schon")
)

func newID() string {
	buffer := make([]byte, 6)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

// OpenStore öffnet die Datenbank und legt das Schema an.
func OpenStore(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	store := &Store{db: db}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("Schema anlegen: %w", err)
	}
	// Spalten, die erst später dazukamen
	for _, column := range []struct{ table, name, definition string }{
		{"projects", "voicechat", "INTEGER NOT NULL DEFAULT 0"},
		{"servers", "group_id", "TEXT NOT NULL DEFAULT ''"},
		{"servers", "public_address", "TEXT NOT NULL DEFAULT ''"},
		{"servers", "modpack", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := store.ensureColumn(column.table, column.name, column.definition); err != nil {
			db.Close()
			return nil, fmt.Errorf("Spalte %s.%s: %w", column.table, column.name, err)
		}
	}
	return store, nil
}

// ensureColumn legt eine Spalte an, falls sie in einer älteren Datenbank fehlt.
func (s *Store) ensureColumn(table, name, definition string) error {
	rows, err := s.db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			rows.Close()
			return err
		}
		found = found || column == name
	}
	rows.Close()
	if found {
		return nil
	}
	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, definition))
	return err
}

// Close schließt die Datenbank.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  disabled INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  user_agent TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  url TEXT NOT NULL,
  token TEXT NOT NULL,
  public_host TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  secret TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS members (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  PRIMARY KEY (project_id, user_id)
);
CREATE TABLE IF NOT EXISTS servers (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL REFERENCES nodes(id),
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  role TEXT NOT NULL,
  software TEXT NOT NULL,
  in_try INTEGER NOT NULL DEFAULT 0,
  try_position INTEGER NOT NULL DEFAULT 0,
  forced_host TEXT NOT NULL DEFAULT '',
  voicechat INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  UNIQUE (project_id, slug)
);
CREATE INDEX IF NOT EXISTS servers_project ON servers(project_id);
CREATE INDEX IF NOT EXISTS servers_node ON servers(node_id);
CREATE TABLE IF NOT EXISTS server_groups (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  software TEXT NOT NULL,
  version TEXT NOT NULL,
  memory_mb INTEGER NOT NULL,
  min_instances INTEGER NOT NULL,
  max_instances INTEGER NOT NULL,
  players_per_instance INTEGER NOT NULL,
  idle_minutes INTEGER NOT NULL,
  in_try INTEGER NOT NULL DEFAULT 1,
  dynamic INTEGER NOT NULL DEFAULT 1,
  plugins TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (project_id, slug)
);
CREATE TABLE IF NOT EXISTS metrics (
  server_id TEXT NOT NULL,
  ts INTEGER NOT NULL,
  cpu REAL NOT NULL,
  mem INTEGER NOT NULL,
  players INTEGER NOT NULL,
  PRIMARY KEY (server_id, ts)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS seen_players (
  server_id TEXT NOT NULL,
  name TEXT NOT NULL COLLATE NOCASE,
  first_seen INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  seconds INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (server_id, name)
) WITHOUT ROWID;
`

// ---------------------------------------------------------------- Benutzer

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	IsAdmin      bool   `json:"isAdmin"`
	Disabled     bool   `json:"disabled"`
	CreatedAt    int64  `json:"createdAt"`
}

const userColumns = "id, username, password_hash, is_admin, disabled, created_at"

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var user User
	var admin, disabled int
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &admin, &disabled, &user.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	user.IsAdmin, user.Disabled = admin == 1, disabled == 1
	return user, nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *Store) CountUsers() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *Store) CountAdmins() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1 AND disabled = 0").Scan(&count)
	return count, err
}

func (s *Store) CreateUser(username, hash string, admin bool) (User, error) {
	user := User{ID: newID(), Username: username, PasswordHash: hash, IsAdmin: admin, CreatedAt: time.Now().UnixMilli()}
	_, err := s.db.Exec("INSERT INTO users (id, username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?)",
		user.ID, user.Username, user.PasswordHash, boolInt(admin), user.CreatedAt)
	if isUnique(err) {
		return User{}, fmt.Errorf("%w: der Benutzername ist vergeben", ErrExists)
	}
	return user, err
}

func modpackText(info *ModpackInfo) string {
	if info == nil {
		return ""
	}
	data, err := json.Marshal(info)
	if err != nil {
		return ""
	}
	return string(data)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) UserByID(id string) (User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

func (s *Store) UserByName(name string) (User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE username = ?", name))
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query("SELECT " + userColumns + " FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUser(id string, admin, disabled bool) error {
	_, err := s.db.Exec("UPDATE users SET is_admin = ?, disabled = ? WHERE id = ?", boolInt(admin), boolInt(disabled), id)
	return err
}

func (s *Store) SetPassword(id, hash string) error {
	_, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	return err
}

func (s *Store) DeleteUser(id string) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// ---------------------------------------------------------------- Sitzungen

func (s *Store) CreateSession(hash, userID, agent string, expires time.Time) error {
	_, err := s.db.Exec("INSERT INTO sessions (token_hash, user_id, expires_at, created_at, user_agent) VALUES (?, ?, ?, ?, ?)",
		hash, userID, expires.UnixMilli(), time.Now().UnixMilli(), agent)
	return err
}

// SessionUser liefert den Benutzer einer gültigen Sitzung.
func (s *Store) SessionUser(hash string) (User, error) {
	row := s.db.QueryRow("SELECT u.id, u.username, u.password_hash, u.is_admin, u.disabled, u.created_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ? AND u.disabled = 0",
		hash, time.Now().UnixMilli())
	return scanUser(row)
}

func (s *Store) DeleteSession(hash string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", hash)
	return err
}

func (s *Store) DeleteUserSessions(userID string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

func (s *Store) PurgeSessions() error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", time.Now().UnixMilli())
	return err
}

// ---------------------------------------------------------------- Nodes

type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Token      string `json:"-"`
	PublicHost string `json:"publicHost"`
	CreatedAt  int64  `json:"createdAt"`
}

func scanNode(row interface{ Scan(...any) error }) (Node, error) {
	var node Node
	if err := row.Scan(&node.ID, &node.Name, &node.URL, &node.Token, &node.PublicHost, &node.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Node{}, ErrNotFound
		}
		return Node{}, err
	}
	return node, nil
}

const nodeColumns = "id, name, url, token, public_host, created_at"

func (s *Store) CreateNode(node Node) (Node, error) {
	node.ID, node.CreatedAt = newID(), time.Now().UnixMilli()
	_, err := s.db.Exec("INSERT INTO nodes ("+nodeColumns+") VALUES (?, ?, ?, ?, ?, ?)", node.ID, node.Name, node.URL, node.Token, node.PublicHost, node.CreatedAt)
	if isUnique(err) {
		return Node{}, fmt.Errorf("%w: der Name ist vergeben", ErrExists)
	}
	return node, err
}

func (s *Store) NodeByID(id string) (Node, error) {
	return scanNode(s.db.QueryRow("SELECT "+nodeColumns+" FROM nodes WHERE id = ?", id))
}

func (s *Store) ListNodes() ([]Node, error) {
	rows, err := s.db.Query("SELECT " + nodeColumns + " FROM nodes ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []Node{}
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) UpdateNode(node Node) error {
	_, err := s.db.Exec("UPDATE nodes SET name = ?, url = ?, token = ?, public_host = ? WHERE id = ?", node.Name, node.URL, node.Token, node.PublicHost, node.ID)
	if isUnique(err) {
		return fmt.Errorf("%w: der Name ist vergeben", ErrExists)
	}
	return err
}

func (s *Store) DeleteNode(id string) error {
	_, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", id)
	return err
}

func (s *Store) CountServersOnNode(id string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM servers WHERE node_id = ?", id).Scan(&count)
	return count, err
}

// ---------------------------------------------------------------- Projekte

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // server oder network
	Description string `json:"description"`
	Secret      string `json:"-"`
	OwnerID     string `json:"ownerId"`
	CreatedAt   int64  `json:"createdAt"`
	// Voicechat heißt: Simple Voice Chat läuft über den Proxy (ein UDP-Port), die Server bekommen das Plugin.
	Voicechat bool `json:"voicechat"`
}

const projectColumns = "id, name, kind, description, secret, owner_id, created_at, voicechat"

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var project Project
	var voice int
	if err := row.Scan(&project.ID, &project.Name, &project.Kind, &project.Description, &project.Secret, &project.OwnerID, &project.CreatedAt, &voice); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, ErrNotFound
		}
		return Project{}, err
	}
	project.Voicechat = voice == 1
	return project, nil
}

func (s *Store) CreateProject(project Project) (Project, error) {
	project.ID, project.CreatedAt = newID(), time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO projects ("+projectColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		project.ID, project.Name, project.Kind, project.Description, project.Secret, project.OwnerID, project.CreatedAt, boolInt(project.Voicechat)); err != nil {
		return Project{}, err
	}
	if _, err := tx.Exec("INSERT INTO members (project_id, user_id, role) VALUES (?, ?, 'owner')", project.ID, project.OwnerID); err != nil {
		return Project{}, err
	}
	return project, tx.Commit()
}

func (s *Store) ProjectByID(id string) (Project, error) {
	return scanProject(s.db.QueryRow("SELECT "+projectColumns+" FROM projects WHERE id = ?", id))
}

// ListProjects liefert die Projekte eines Benutzers; ein Administrator sieht alle.
func (s *Store) ListProjects(userID string, all bool) ([]Project, error) {
	query := "SELECT " + projectColumns + " FROM projects ORDER BY name COLLATE NOCASE"
	args := []any{}
	if !all {
		query = "SELECT p.id, p.name, p.kind, p.description, p.secret, p.owner_id, p.created_at, p.voicechat FROM projects p JOIN members m ON m.project_id = p.id WHERE m.user_id = ? ORDER BY p.name COLLATE NOCASE"
		args = append(args, userID)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateProject(project Project) error {
	_, err := s.db.Exec("UPDATE projects SET name = ?, description = ?, secret = ?, voicechat = ? WHERE id = ?", project.Name, project.Description, project.Secret, boolInt(project.Voicechat), project.ID)
	return err
}

func (s *Store) DeleteProject(id string) error {
	_, err := s.db.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

// ---------------------------------------------------------------- Mitglieder

type Member struct {
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Role      string `json:"role"`
}

func (s *Store) ListMembers(projectID string) ([]Member, error) {
	rows, err := s.db.Query("SELECT m.project_id, m.user_id, u.username, m.role FROM members m JOIN users u ON u.id = m.user_id WHERE m.project_id = ? ORDER BY u.username", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []Member{}
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ProjectID, &member.UserID, &member.Username, &member.Role); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) MemberRole(projectID, userID string) (string, error) {
	var role string
	err := s.db.QueryRow("SELECT role FROM members WHERE project_id = ? AND user_id = ?", projectID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

func (s *Store) SetMember(projectID, userID, role string) error {
	_, err := s.db.Exec("INSERT INTO members (project_id, user_id, role) VALUES (?, ?, ?) ON CONFLICT(project_id, user_id) DO UPDATE SET role = excluded.role", projectID, userID, role)
	return err
}

func (s *Store) RemoveMember(projectID, userID string) error {
	_, err := s.db.Exec("DELETE FROM members WHERE project_id = ? AND user_id = ?", projectID, userID)
	return err
}

func (s *Store) CountOwners(projectID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM members WHERE project_id = ? AND role = 'owner'", projectID).Scan(&count)
	return count, err
}

// ---------------------------------------------------------------- Server

type ServerRow struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	NodeID      string `json:"nodeId"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Role        string `json:"role"` // standalone, proxy, backend
	Software    string `json:"software"`
	InTry       bool   `json:"inTry"`
	TryPosition int    `json:"tryPosition"`
	ForcedHost  string `json:"forcedHost"`
	Voicechat   bool   `json:"voicechat"`
	// GroupID ist die Gruppe, zu der der Server als Instanz gehört (leer bei einem normalen Server).
	GroupID string `json:"groupId,omitempty"`
	// PublicAddress ist die Adresse, unter der Spieler den Server erreichen, wenn sie nicht die der Maschine ist (z. B.
	// ein playit-Tunnel). Sie wird nur angezeigt.
	PublicAddress string `json:"publicAddress,omitempty"`
	// Modpack ist das Modrinth-Modpack, aus dem der Server angelegt wurde.
	Modpack   *ModpackInfo `json:"modpack,omitempty"`
	CreatedAt int64        `json:"createdAt"`
}

// ModpackInfo beschreibt das Modpack, mit dem ein Server angelegt wurde.
type ModpackInfo struct {
	ProjectID     string `json:"projectId"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	IconURL       string `json:"iconUrl,omitempty"`
	VersionID     string `json:"versionId"`
	VersionNumber string `json:"versionNumber"`
	Minecraft     string `json:"minecraft"`
}

const serverColumns = "id, project_id, node_id, name, slug, role, software, in_try, try_position, forced_host, voicechat, group_id, public_address, modpack, created_at"

func scanServer(row interface{ Scan(...any) error }) (ServerRow, error) {
	var server ServerRow
	var inTry, voice int
	var modpack string
	if err := row.Scan(&server.ID, &server.ProjectID, &server.NodeID, &server.Name, &server.Slug, &server.Role, &server.Software, &inTry, &server.TryPosition, &server.ForcedHost, &voice, &server.GroupID, &server.PublicAddress, &modpack, &server.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ServerRow{}, ErrNotFound
		}
		return ServerRow{}, err
	}
	server.InTry, server.Voicechat = inTry == 1, voice == 1
	if modpack != "" {
		var info ModpackInfo
		if json.Unmarshal([]byte(modpack), &info) == nil {
			server.Modpack = &info
		}
	}
	return server, nil
}

func (s *Store) CreateServer(server ServerRow) (ServerRow, error) {
	if server.ID == "" {
		server.ID = newID()
	}
	server.CreatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec("INSERT INTO servers ("+serverColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		server.ID, server.ProjectID, server.NodeID, server.Name, server.Slug, server.Role, server.Software, boolInt(server.InTry), server.TryPosition, server.ForcedHost, boolInt(server.Voicechat), server.GroupID, server.PublicAddress, modpackText(server.Modpack), server.CreatedAt)
	if isUnique(err) {
		return ServerRow{}, fmt.Errorf("%w: ein Server mit diesem Namen existiert im Projekt schon", ErrExists)
	}
	return server, err
}

func (s *Store) ServerByID(id string) (ServerRow, error) {
	return scanServer(s.db.QueryRow("SELECT "+serverColumns+" FROM servers WHERE id = ?", id))
}

func (s *Store) ListServers(projectID string) ([]ServerRow, error) {
	query, args := "SELECT "+serverColumns+" FROM servers ORDER BY role, try_position, name COLLATE NOCASE", []any{}
	if projectID != "" {
		query, args = "SELECT "+serverColumns+" FROM servers WHERE project_id = ? ORDER BY role, try_position, name COLLATE NOCASE", []any{projectID}
	}
	rows, err := s.db.Query(query, args...)
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

func (s *Store) UpdateServer(server ServerRow) error {
	_, err := s.db.Exec("UPDATE servers SET name = ?, slug = ?, role = ?, software = ?, in_try = ?, try_position = ?, forced_host = ?, voicechat = ?, node_id = ?, public_address = ?, modpack = ? WHERE id = ?",
		server.Name, server.Slug, server.Role, server.Software, boolInt(server.InTry), server.TryPosition, server.ForcedHost, boolInt(server.Voicechat), server.NodeID, server.PublicAddress, modpackText(server.Modpack), server.ID)
	if isUnique(err) {
		return fmt.Errorf("%w: ein Server mit diesem Namen existiert im Projekt schon", ErrExists)
	}
	return err
}

func (s *Store) DeleteServer(id string) error {
	_, err := s.db.Exec("DELETE FROM servers WHERE id = ?", id)
	return err
}
