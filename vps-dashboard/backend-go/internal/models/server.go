package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Server Registry errors.
var (
	// ErrServerDuplicateName is returned when an INSERT/UPDATE would
	// violate the servers.name UNIQUE constraint.
	ErrServerDuplicateName = errors.New("server: duplicate name")

	// ErrServerNotFound is returned when an id (or name) lookup yields
	// no rows or when an UPDATE/DELETE affects zero rows.
	ErrServerNotFound = errors.New("server: not found")

	// ErrTagNotFound is returned when a tag lookup yields no rows.
	ErrTagNotFound = errors.New("tag: not found")
)

// Server status values accepted by the servers.status CHECK constraint.
const (
	ServerStatusOnline   = "online"
	ServerStatusDegraded = "degraded"
	ServerStatusOffline  = "offline"
	ServerStatusUnknown  = "unknown"
)

// SSH credential types accepted by servers.credential_type.
const (
	ServerCredentialSSHKey  = "ssh_key"
	ServerCredentialPassword = "password"
	ServerCredentialAgent   = "agent"
)

// Server environments (aligned with Project environments).
const (
	ServerEnvDevelopment = "development"
	ServerEnvStaging     = "staging"
	ServerEnvProduction  = "production"
)

// Server is the registry shape exposed over the API: the central
// identity of every managed host (PROJECT ARCHITECTURE.md §7).
//
// CredentialRef is a *reference* (e.g. an ssh key name or keychain
// alias) — never secret material itself. Phase 2 will resolve it
// against a secure credential store.
type Server struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Hostname           string    `json:"hostname"`
	IPAddress          string    `json:"ip_address"`
	SSHPort            int       `json:"ssh_port"`
	SSHUsername        string    `json:"ssh_username"`
	CredentialType     string    `json:"credential_type"`
	CredentialRef      string    `json:"credential_ref"`
	CredentialPassword string    `json:"credential_password,omitempty"` // direct password (when type=password)
	OperatingSystem    string    `json:"operating_system"`
	Architecture       string    `json:"architecture"`
	Provider           string    `json:"provider"`
	ProviderInstanceID string    `json:"provider_instance_id"`
	Environment        string    `json:"environment"`
	Status             string    `json:"status"`
	StatusDetail       string    `json:"status_detail"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	Notes              string    `json:"notes"`
	Enabled            bool      `json:"enabled"`
	Tags               []string  `json:"tags"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Tag is a normalised tag catalogue row.
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ServerFilter narrows the result of ServerRepo.List.
// Empty fields mean "no filter on this field".
type ServerFilter struct {
	Search      string
	Environment string
	Tag         string
	Status      string
	Provider    string
	EnabledOnly bool
}

// ServerRepo persists and retrieves Server rows.
type ServerRepo struct {
	DB *sql.DB
}

// NewServerRepo constructs a ServerRepo bound to the given *sql.DB.
func NewServerRepo(db *sql.DB) *ServerRepo {
	return &ServerRepo{DB: db}
}

// serverColumns is the canonical SELECT list for the servers table.
const serverColumns = `id, name, hostname, ip_address, ssh_port, ssh_username,
	credential_type, credential_ref, credential_password, operating_system, architecture,
	provider, provider_instance_id, environment, status, status_detail,
	last_seen_at, notes, enabled, created_at, updated_at`

// Create inserts a new server. If s.ID is empty a UUID is generated.
// Tags are attached transactionally via server_tags; unknown tag names
// are created on demand in the tags catalogue.
func (r *ServerRepo) Create(ctx context.Context, s Server) (Server, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.SSHPort == 0 {
		s.SSHPort = 22
	}
	if s.CredentialType == "" {
		s.CredentialType = ServerCredentialSSHKey
	}
	if s.Environment == "" {
		s.Environment = ServerEnvProduction
	}
	if s.Status == "" {
		s.Status = ServerStatusUnknown
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Server{}, fmt.Errorf("server: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO servers (
			id, name, hostname, ip_address, ssh_port, ssh_username,
			credential_type, credential_ref, credential_password, operating_system, architecture,
			provider, provider_instance_id, environment, status, status_detail,
			last_seen_at, notes, enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		s.ID, s.Name, s.Hostname, s.IPAddress, s.SSHPort, s.SSHUsername,
		s.CredentialType, s.CredentialRef, s.CredentialPassword, s.OperatingSystem, s.Architecture,
		s.Provider, s.ProviderInstanceID, s.Environment, s.Status, s.StatusDetail,
		lastSeenRaw(s.LastSeenAt), s.Notes, enabledInt(s.Enabled),
	); err != nil {
		if isUniqueViolation(err) {
			return Server{}, ErrServerDuplicateName
		}
		return Server{}, fmt.Errorf("server: insert: %w", err)
	}

	if err := attachServerTags(ctx, tx, s.ID, s.Tags); err != nil {
		return Server{}, err
	}

	if err := tx.Commit(); err != nil {
		return Server{}, fmt.Errorf("server: commit: %w", err)
	}
	return r.Get(ctx, s.ID)
}

// Update replaces every persisted column on the row with id == s.ID
// (except created_at and status/last_seen, which are owned by the
// monitoring pipeline). updated_at is bumped to now. Tag membership is
// replaced wholesale. Returns ErrServerNotFound when no row matches.
func (r *ServerRepo) Update(ctx context.Context, s Server) (Server, error) {
	if s.SSHPort == 0 {
		s.SSHPort = 22
	}
	if s.CredentialType == "" {
		s.CredentialType = ServerCredentialSSHKey
	}
	if s.Environment == "" {
		s.Environment = ServerEnvProduction
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Server{}, fmt.Errorf("server: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE servers SET
			name = ?, hostname = ?, ip_address = ?, ssh_port = ?, ssh_username = ?,
			credential_type = ?, credential_ref = ?, credential_password = ?,
			operating_system = ?, architecture = ?,
			provider = ?, provider_instance_id = ?, environment = ?, notes = ?,
			enabled = ?, updated_at = datetime('now')
		WHERE id = ?
	`,
		s.Name, s.Hostname, s.IPAddress, s.SSHPort, s.SSHUsername,
		s.CredentialType, s.CredentialRef, s.CredentialPassword,
		s.OperatingSystem, s.Architecture,
		s.Provider, s.ProviderInstanceID, s.Environment, s.Notes,
		enabledInt(s.Enabled), s.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Server{}, ErrServerDuplicateName
		}
		return Server{}, fmt.Errorf("server: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Server{}, fmt.Errorf("server: rows affected: %w", err)
	}
	if n == 0 {
		return Server{}, ErrServerNotFound
	}

	// Replace tag membership wholesale.
	if _, err := tx.ExecContext(ctx, `DELETE FROM server_tags WHERE server_id = ?`, s.ID); err != nil {
		return Server{}, fmt.Errorf("server: clear tags: %w", err)
	}
	if err := attachServerTags(ctx, tx, s.ID, s.Tags); err != nil {
		return Server{}, err
	}

	if err := tx.Commit(); err != nil {
		return Server{}, fmt.Errorf("server: commit: %w", err)
	}
	return r.Get(ctx, s.ID)
}

// Delete removes the server with the given id. Tag memberships cascade
// in SQL; orphaned tags remain in the catalogue for reuse. Returns
// ErrServerNotFound when no row matches.
func (r *ServerRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("server: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("server: rows affected: %w", err)
	}
	if n == 0 {
		return ErrServerNotFound
	}
	return nil
}

// Get returns the server with the given id (tags included) or
// ErrServerNotFound.
func (r *ServerRepo) Get(ctx context.Context, id string) (Server, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE id = ?`, id)
	s, err := scanServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrServerNotFound
	}
	if err != nil {
		return Server{}, err
	}
	tags, err := r.TagsFor(ctx, []string{s.ID})
	if err != nil {
		return Server{}, err
	}
	s.Tags = tags[s.ID]
	return s, nil
}

// GetByName returns the server with a matching name or ErrServerNotFound.
func (r *ServerRepo) GetByName(ctx context.Context, name string) (Server, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE name = ?`, name)
	s, err := scanServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrServerNotFound
	}
	if err != nil {
		return Server{}, err
	}
	tags, err := r.TagsFor(ctx, []string{s.ID})
	if err != nil {
		return Server{}, err
	}
	s.Tags = tags[s.ID]
	return s, nil
}

// Count returns the total number of registered servers.
func (r *ServerRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers`).Scan(&n); err != nil {
		return 0, fmt.Errorf("server: count: %w", err)
	}
	return n, nil
}

// List returns servers matching the supplied filter, sorted by name
// ASC. Tag filtering joins through server_tags so the normalized
// many-to-many representation is interpreted correctly.
func (r *ServerRepo) List(ctx context.Context, filter ServerFilter) ([]Server, error) {
	var (
		clauses []string
		args    []any
	)

	if s := strings.TrimSpace(filter.Search); s != "" {
		like := "%" + s + "%"
		clauses = append(clauses, "(name LIKE ? OR hostname LIKE ? OR ip_address LIKE ? OR notes LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if e := strings.TrimSpace(filter.Environment); e != "" {
		clauses = append(clauses, "environment = ?")
		args = append(args, e)
	}
	if st := strings.TrimSpace(filter.Status); st != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, st)
	}
	if p := strings.TrimSpace(filter.Provider); p != "" {
		clauses = append(clauses, "provider = ?")
		args = append(args, p)
	}
	if filter.EnabledOnly {
		clauses = append(clauses, "enabled = 1")
	}

	q := `SELECT ` + serverColumns + ` FROM servers`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY name ASC"

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("server: list: %w", err)
	}
	defer rows.Close()

	out := make([]Server, 0, 16)
	ids := make([]string, 0, 16)
	for rows.Next() {
		s, err := scanServerRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		ids = append(ids, s.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server: list iter: %w", err)
	}

	// Batch-load tags for the whole page in one query instead of N+1.
	tagMap, err := r.TagsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Tags = tagMap[out[i].ID]
	}

	// In-memory tag filter (exact match against the tag name).
	if t := strings.TrimSpace(filter.Tag); t != "" {
		filtered := make([]Server, 0, len(out))
		for _, s := range out {
			for _, tag := range s.Tags {
				if tag == t {
					filtered = append(filtered, s)
					break
				}
			}
		}
		out = filtered
	}

	return out, nil
}

// TagsFor returns a serverID → tag names map for the given server ids.
// Empty input returns an empty map.
func (r *ServerRepo) TagsFor(ctx context.Context, serverIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(serverIDs))
	if len(serverIDs) == 0 {
		return out, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(serverIDs)), ",")
	q := `
		SELECT st.server_id, t.name
		FROM server_tags st
		JOIN tags t ON t.id = st.tag_id
		WHERE st.server_id IN (` + placeholders + `)
		ORDER BY t.name ASC
	`
	args := make([]any, len(serverIDs))
	for i, id := range serverIDs {
		args[i] = id
	}

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("server: tags for: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var serverID, tagName string
		if err := rows.Scan(&serverID, &tagName); err != nil {
			return nil, fmt.Errorf("server: tags scan: %w", err)
		}
		out[serverID] = append(out[serverID], tagName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server: tags iter: %w", err)
	}
	return out, nil
}

// ListTags returns the full tag catalogue ordered by name.
func (r *ServerRepo) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, name FROM tags ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("server: list tags: %w", err)
	}
	defer rows.Close()
	out := make([]Tag, 0, 8)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("server: tag scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server: tags iter: %w", err)
	}
	return out, nil
}

// SetStatus updates status, status_detail and last_seen_at for a
// server. Intended for the monitoring pipeline (Phase 3); exposed now
// so the contract is in place.
func (r *ServerRepo) SetStatus(ctx context.Context, id, status, detail string, lastSeen time.Time) error {
	res, err := r.DB.ExecContext(ctx, `
		UPDATE servers SET
			status = ?, status_detail = ?, last_seen_at = ?, updated_at = datetime('now')
		WHERE id = ?
	`, status, detail, lastSeenRaw(lastSeen), id)
	if err != nil {
		return fmt.Errorf("server: set status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("server: rows affected: %w", err)
	}
	if n == 0 {
		return ErrServerNotFound
	}
	return nil
}

// UpdateSystemInfo updates only the auto-detected metadata fields
// (operating_system, architecture) of a server. This is called after
// the first successful SSH connection to auto-populate these fields
// so users don't have to fill them manually.
//
// Only non-empty values are written, so this won't overwrite existing
// values with empty strings if detection fails for some reason.
func (r *ServerRepo) UpdateSystemInfo(ctx context.Context, id, osName, arch string) error {
	if osName == "" && arch == "" {
		return nil // Nothing to update
	}

	// Build dynamic SET clause based on which fields are non-empty.
	setClause := "updated_at = datetime('now')"
	args := []interface{}{}
	if osName != "" {
		setClause = "operating_system = ?, " + setClause
		args = append(args, osName)
	}
	if arch != "" {
		setClause = "architecture = ?, " + setClause
		args = append(args, arch)
	}
	args = append(args, id)

	_, err := r.DB.ExecContext(ctx,
		`UPDATE servers SET `+setClause+` WHERE id = ?`,
		args...)
	if err != nil {
		return fmt.Errorf("server: update system info: %w", err)
	}
	return nil
}

// attachServerTags replaces the tag membership of a server within the
// given transaction. Unknown tag names are inserted into the tags
// catalogue (INSERT OR IGNORE) before being linked.
func attachServerTags(ctx context.Context, tx *sql.Tx, serverID string, tags []string) error {
	cleaned, err := NormalizeServerTags(tags)
	if err != nil {
		return err
	}
	for _, name := range cleaned {
		tagID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(id, name) VALUES (?, ?)`, tagID, name); err != nil {
			return fmt.Errorf("server: upsert tag %q: %w", name, err)
		}
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&id); err != nil {
			return fmt.Errorf("server: resolve tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO server_tags(server_id, tag_id) VALUES (?, ?)`, serverID, id); err != nil {
			return fmt.Errorf("server: link tag %q: %w", name, err)
		}
	}
	return nil
}

// scanServer adapts a *sql.Row scan into ErrServerNotFound semantics.
func scanServer(row *sql.Row) (Server, error) {
	s, err := scanServerRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrServerNotFound
	}
	if err != nil {
		return Server{}, err
	}
	return s, nil
}

func scanServerRow(s rowScanner) (Server, error) {
	var (
		srv          Server
		enabled      int
		lastSeenRaw  string
		createdRaw   string
		updatedRaw   string
	)
	err := s.Scan(
		&srv.ID, &srv.Name, &srv.Hostname, &srv.IPAddress, &srv.SSHPort, &srv.SSHUsername,
		&srv.CredentialType, &srv.CredentialRef, &srv.CredentialPassword, &srv.OperatingSystem, &srv.Architecture,
		&srv.Provider, &srv.ProviderInstanceID, &srv.Environment, &srv.Status, &srv.StatusDetail,
		&lastSeenRaw, &srv.Notes, &enabled, &createdRaw, &updatedRaw,
	)
	if err != nil {
		return Server{}, err
	}
	srv.Enabled = boolFromInt(enabled)
	srv.LastSeenAt = parseSQLiteTime(lastSeenRaw)
	srv.CreatedAt = parseSQLiteTime(createdRaw)
	srv.UpdatedAt = parseSQLiteTime(updatedRaw)
	if srv.Tags == nil {
		srv.Tags = []string{}
	}
	return srv, nil
}

// lastSeenRaw renders a time as the SQLite TEXT layout, or "" when zero.
func lastSeenRaw(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(sqliteTSLayout)
}

// Server name/tag validation rules, mirroring project conventions.
var (
	serverNameRE  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\- ]{0,63}$`)
	serverTagRE   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\- ]{0,31}$`)
	serverUserRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.\-]{0,31}$`)
	serverArchRE  = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{0,16}$`)
	serverProvRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{0,31}$`)
)

// Validate enforces the field-level rules for the Server Registry.
// It mutates s in place to apply trivial canonicalisations
// (lowercasing the hostname, trimming whitespace, dedup tags).
func (s *Server) Validate() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("name: required")
	}
	if !serverNameRE.MatchString(s.Name) {
		return fmt.Errorf("name: invalid format")
	}

	s.Hostname = strings.TrimSpace(s.Hostname)
	if s.Hostname == "" {
		return fmt.Errorf("hostname: required")
	}
	if len(s.Hostname) > 253 {
		return fmt.Errorf("hostname: too long")
	}
	// Accept either a hostname or an IPv4/IPv6 literal.
	if !hostnameRE.MatchString(s.Hostname) && !isIPLiteral(s.Hostname) {
		return fmt.Errorf("hostname: invalid hostname or IP")
	}

	s.IPAddress = strings.TrimSpace(s.IPAddress)
	if s.IPAddress != "" && !isIPLiteral(s.IPAddress) {
		return fmt.Errorf("ip_address: invalid IPv4 or IPv6 literal")
	}

	if s.SSHPort == 0 {
		s.SSHPort = 22
	}
	if s.SSHPort < 1 || s.SSHPort > 65535 {
		return fmt.Errorf("ssh_port: must be between 1 and 65535")
	}

	s.SSHUsername = strings.TrimSpace(s.SSHUsername)
	if s.SSHUsername == "" {
		return fmt.Errorf("ssh_username: required")
	}
	if !serverUserRE.MatchString(s.SSHUsername) {
		return fmt.Errorf("ssh_username: invalid format")
	}

	s.CredentialType = strings.TrimSpace(strings.ToLower(s.CredentialType))
	switch s.CredentialType {
	case ServerCredentialSSHKey, ServerCredentialPassword, ServerCredentialAgent:
	case "":
		s.CredentialType = ServerCredentialSSHKey
	default:
		return fmt.Errorf("credential_type: must be ssh_key|password|agent")
	}
	// credential_ref names the credential *reference*; it must never
	// contain secret material, but the reference string itself is
	// opaque to the registry and only resolved by the SSH engine.
	s.CredentialRef = strings.TrimSpace(s.CredentialRef)
	if len(s.CredentialRef) > 128 {
		return fmt.Errorf("credential_ref: too long (max 128)")
	}

	s.OperatingSystem = strings.TrimSpace(s.OperatingSystem)
	if len(s.OperatingSystem) > 64 {
		return fmt.Errorf("operating_system: too long (max 64)")
	}
	s.Architecture = strings.TrimSpace(strings.ToLower(s.Architecture))
	if s.Architecture != "" && !serverArchRE.MatchString(s.Architecture) {
		return fmt.Errorf("architecture: invalid format")
	}

	s.Provider = strings.TrimSpace(strings.ToLower(s.Provider))
	if s.Provider != "" && !serverProvRE.MatchString(s.Provider) {
		return fmt.Errorf("provider: invalid format")
	}
	s.ProviderInstanceID = strings.TrimSpace(s.ProviderInstanceID)
	if len(s.ProviderInstanceID) > 128 {
		return fmt.Errorf("provider_instance_id: too long (max 128)")
	}

	s.Environment = strings.TrimSpace(strings.ToLower(s.Environment))
	if s.Environment == "" {
		s.Environment = ServerEnvProduction
	}
	switch s.Environment {
	case ServerEnvDevelopment, ServerEnvStaging, ServerEnvProduction:
	default:
		return fmt.Errorf("environment: must be development|staging|production")
	}

	// Status is owned by the monitoring pipeline; however Validate is
	// also called on create where unknown is the sane default.
	s.Status = strings.TrimSpace(strings.ToLower(s.Status))
	switch s.Status {
	case ServerStatusOnline, ServerStatusDegraded, ServerStatusOffline, ServerStatusUnknown, "":
		if s.Status == "" {
			s.Status = ServerStatusUnknown
		}
	default:
		return fmt.Errorf("status: must be online|degraded|offline|unknown")
	}
	s.StatusDetail = strings.TrimSpace(s.StatusDetail)
	if len(s.StatusDetail) > 256 {
		return fmt.Errorf("status_detail: too long (max 256)")
	}

	s.Notes = strings.TrimSpace(s.Notes)
	if len(s.Notes) > 2000 {
		return fmt.Errorf("notes: too long (max 2000)")
	}

	cleaned, err := NormalizeServerTags(s.Tags)
	if err != nil {
		return err
	}
	s.Tags = cleaned

	return nil
}

// NormalizeServerTags trims, dedupes (case-insensitive), and validates
// a tag list, returning the canonical ordering.
func NormalizeServerTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}
	if len(tags) > 20 {
		return nil, fmt.Errorf("tags: max 20")
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !serverTagRE.MatchString(t) {
			return nil, fmt.Errorf("tags: %q is invalid", t)
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}

// isIPLiteral reports whether s parses as an IPv4 or IPv6 literal.
func isIPLiteral(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}
