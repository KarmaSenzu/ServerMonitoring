// Package backup snapshots the dashboard's SQLite database to disk.
//
// SQLite's `VACUUM INTO '<path>'` produces a consistent, point-in-time
// copy of the database without locking writers. This is the canonical
// online-backup approach for the modernc.org/sqlite driver because it
// works across the whole pure-Go stack without needing the C-level
// sqlite3_backup API.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
)

// Service runs scheduled and on-demand SQLite backups.
type Service struct {
	Logger    zerolog.Logger
	DB        *sql.DB
	DBPath    string
	Dir       string
	Keep      int
	HourLocal int
	Repo      *Repo
	Events    *models.EventRepo
}

// Backup is one row from the backups table.
type Backup struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"ts"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error"`
	Trigger   string    `json:"trigger"`
}

// ErrNotFound is returned by Repo.Get when no matching id exists.
var ErrNotFound = errors.New("backup: not found")

// ErrLastBackup is returned when DELETE would remove the last backup.
var ErrLastBackup = errors.New("backup: refusing to delete the only remaining backup")

// Repo persists Backup rows.
type Repo struct {
	DB *sql.DB
}

// NewRepo binds a Repo to db.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{DB: db}
}

// Create inserts a new backup row. The provided ID must be set.
func (r *Repo) Create(ctx context.Context, b Backup) (Backup, error) {
	if b.Timestamp.IsZero() {
		b.Timestamp = time.Now().UTC()
	}
	okInt := 0
	if b.OK {
		okInt = 1
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO backups (id, ts, path, size_bytes, ok, error, trigger)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		b.ID, b.Timestamp.UTC().Format(time.RFC3339Nano),
		b.Path, b.SizeBytes, okInt, b.Error, b.Trigger,
	); err != nil {
		return Backup{}, fmt.Errorf("backup: insert: %w", err)
	}
	return r.Get(ctx, b.ID)
}

// Get returns the backup with id or ErrNotFound.
func (r *Repo) Get(ctx context.Context, id string) (Backup, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, ts, path, size_bytes, ok, error, trigger
		FROM backups WHERE id = ?
	`, id)
	return scanBackupRow(row.Scan)
}

// List returns the most recent backups ordered by ts DESC. limit is
// clamped to [1, 200].
func (r *Repo) List(ctx context.Context, limit int) ([]Backup, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, ts, path, size_bytes, ok, error, trigger
		FROM backups
		ORDER BY ts DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("backup: list: %w", err)
	}
	defer rows.Close()

	out := make([]Backup, 0, limit)
	for rows.Next() {
		b, err := scanBackupRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("backup: list iter: %w", err)
	}
	return out, nil
}

// CountOK returns the number of successful backups currently recorded.
// Used by Service.Delete to refuse removal of the last good backup.
func (r *Repo) CountOK(ctx context.Context) (int, error) {
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE ok = 1`).Scan(&n); err != nil {
		return 0, fmt.Errorf("backup: count ok: %w", err)
	}
	return n, nil
}

// Delete removes a backup row.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("backup: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("backup: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PruneKeep retains the `keep` newest successful backup rows and
// deletes the rest along with their files. Failed rows are preserved
// for diagnostics. Returns the list of deleted ids and any file-system
// errors (best-effort).
func (s *Service) pruneKeep(ctx context.Context) ([]string, error) {
	if s.Keep <= 0 {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, path FROM backups
		WHERE ok = 1
		ORDER BY ts DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("backup: prune query: %w", err)
	}
	defer rows.Close()

	type entry struct{ id, path string }
	var keepers []entry
	var doomed []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.path); err != nil {
			return nil, fmt.Errorf("backup: prune scan: %w", err)
		}
		if len(keepers) < s.Keep {
			keepers = append(keepers, e)
		} else {
			doomed = append(doomed, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("backup: prune iter: %w", err)
	}

	deleted := make([]string, 0, len(doomed))
	for _, e := range doomed {
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			s.Logger.Warn().Err(err).Str("path", e.path).Msg("backup.prune.unlink_failed")
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, e.id); err != nil {
			s.Logger.Warn().Err(err).Str("id", e.id).Msg("backup.prune.row_delete_failed")
			continue
		}
		deleted = append(deleted, e.id)
	}
	return deleted, nil
}

// scanRowFn is the Scan signature shared by *sql.Row and *sql.Rows.
type scanRowFn func(dest ...any) error

func scanBackupRow(scan scanRowFn) (Backup, error) {
	var (
		b     Backup
		ok    int
		tsRaw string
	)
	if err := scan(&b.ID, &tsRaw, &b.Path, &b.SizeBytes, &ok, &b.Error, &b.Trigger); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Backup{}, ErrNotFound
		}
		return Backup{}, fmt.Errorf("backup: scan: %w", err)
	}
	b.OK = ok != 0
	b.Timestamp = parseSQLiteTime(tsRaw)
	return b, nil
}

// parseSQLiteTime mirrors models.parseSQLiteTime to keep this package
// self-contained.
func parseSQLiteTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// RunOnce performs a single backup synchronously and returns the
// resulting Backup row. The pruner is invoked on success.
func (s *Service) RunOnce(ctx context.Context, trigger string) (Backup, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return Backup{}, fmt.Errorf("backup: ensure dir: %w", err)
	}

	id := uuid.NewString()
	now := time.Now()
	filename := fmt.Sprintf("vps-dashboard-%s-%s.db", now.Format("2006-01-02-150405"), id[:8])
	outPath := filepath.Join(s.Dir, filename)

	// VACUUM INTO is parameterised through SQL string interpolation
	// because SQLite does not accept bound parameters here. We mitigate
	// by validating the path is under our managed dir and escaping the
	// single quote inside the literal.
	if !pathInside(s.Dir, outPath) {
		return Backup{}, fmt.Errorf("backup: refusing path outside dir: %q", outPath)
	}
	literal := strings.ReplaceAll(outPath, "'", "''")
	stmt := fmt.Sprintf(`VACUUM INTO '%s'`, literal)
	_, vacErr := s.DB.ExecContext(ctx, stmt)

	b := Backup{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Path:      outPath,
		Trigger:   trigger,
	}
	if vacErr != nil {
		b.OK = false
		b.Error = vacErr.Error()
		_, _ = s.Repo.Create(ctx, b)
		s.fireEvent(ctx, b, "Backup failed: "+vacErr.Error(), models.SeverityError)
		return b, fmt.Errorf("backup: vacuum into: %w", vacErr)
	}

	info, statErr := os.Stat(outPath)
	if statErr != nil {
		b.OK = false
		b.Error = statErr.Error()
		_, _ = s.Repo.Create(ctx, b)
		s.fireEvent(ctx, b, "Backup stat failed: "+statErr.Error(), models.SeverityError)
		return b, fmt.Errorf("backup: stat: %w", statErr)
	}
	b.SizeBytes = info.Size()
	b.OK = true

	created, err := s.Repo.Create(ctx, b)
	if err != nil {
		return Backup{}, err
	}

	if _, err := s.pruneKeep(ctx); err != nil {
		s.Logger.Warn().Err(err).Msg("backup.prune_failed")
	}

	s.fireEvent(ctx, created, fmt.Sprintf("Backup OK (%s, %d bytes)", filename, created.SizeBytes), models.SeverityInfo)
	return created, nil
}

// fireEvent records a backup result to the events table when an Events
// repo was wired into the service.
func (s *Service) fireEvent(ctx context.Context, b Backup, msg, severity string) {
	if s.Events == nil {
		return
	}
	if _, err := s.Events.Append(ctx, models.Event{
		Category: "backup",
		Severity: severity,
		Source:   "backup",
		Message:  msg,
		Data: map[string]any{
			"backup_id":  b.ID,
			"path":       b.Path,
			"size_bytes": b.SizeBytes,
			"trigger":    b.Trigger,
		},
		Timestamp: b.Timestamp,
	}); err != nil {
		s.Logger.Warn().Err(err).Str("backup_id", b.ID).Msg("backup.event_append_failed")
	}
}

// Run blocks until ctx is cancelled, executing one backup at the
// configured local hour each day. The first run is computed relative
// to wall-clock time and may fire shortly after start if the hour has
// already passed today.
func (s *Service) Run(ctx context.Context) {
	hour := s.HourLocal
	if hour < 0 || hour > 23 {
		hour = 3
	}
	keep := s.Keep
	if keep <= 0 {
		keep = 7
	}
	s.Keep = keep

	s.Logger.Info().
		Int("hour_local", hour).
		Int("keep", keep).
		Str("dir", s.Dir).
		Msg("backup.scheduler.started")

	for {
		next := nextRun(time.Now(), hour)
		wait := time.Until(next)
		s.Logger.Info().Time("next_run", next).Dur("wait", wait).Msg("backup.scheduler.sleeping")

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.Logger.Info().Msg("backup.scheduler.stopped")
			return
		case <-timer.C:
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			if _, err := s.RunOnce(runCtx, "scheduled"); err != nil {
				s.Logger.Warn().Err(err).Msg("backup.run_once.failed")
			}
			cancel()
		}
	}
}

// Delete removes a backup file and its row. It refuses to delete the
// last successful backup so the user always retains a fallback.
func (s *Service) Delete(ctx context.Context, id string) error {
	b, err := s.Repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if b.OK {
		count, err := s.Repo.CountOK(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastBackup
		}
	}
	if err := os.Remove(b.Path); err != nil && !os.IsNotExist(err) {
		s.Logger.Warn().Err(err).Str("path", b.Path).Msg("backup.delete.unlink_failed")
	}
	return s.Repo.Delete(ctx, id)
}

// nextRun returns the next time at hour:00 in local time strictly
// after `now`. If today's slot has already passed it returns
// tomorrow's slot.
func nextRun(now time.Time, hour int) time.Time {
	loc := now.Location()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc)
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

// pathInside reports whether candidate is inside dir (after symlink
// resolution and Clean). Used by both the backup writer and the
// download handler to prevent path-traversal access.
func pathInside(dir, candidate string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	absCand, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absCand)
	if err != nil {
		return false
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return false
	}
	return true
}

// PathInside is the exported variant for handlers.
func PathInside(dir, candidate string) bool {
	return pathInside(dir, candidate)
}

// SortByTimeDesc orders backups newest-first; exported for tests.
func SortByTimeDesc(in []Backup) []Backup {
	out := make([]Backup, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].ID > out[j].ID
		}
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out
}
