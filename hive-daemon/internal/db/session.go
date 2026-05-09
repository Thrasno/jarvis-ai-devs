package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

// ErrSessionNotFound is returned when a requested session does not exist.
// Callers use errors.Is for comparison.
var ErrSessionNotFound = errors.New("session not found")

// CreateSession inserts a new session row.
// Returns an error if the id already exists (use EnsureManualSaveSession for idempotent inserts).
func (d *DB) CreateSession(id, project, directory, devID, client string) error {
	_, err := d.sqlDB.Exec(`
		INSERT INTO sessions (id, sync_id, project, directory, dev_id, client)
		VALUES (?, lower(hex(randomblob(16))), ?, ?, ?, ?)`,
		id, project, directory, devID, client,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by id.
// Returns ErrSessionNotFound if the session does not exist.
func (d *DB) GetSession(id string) (*models.Session, error) {
	var s models.Session
	var endedAt, syncedAt, summary sql.NullString
	var startedAtStr string

	err := d.sqlDB.QueryRow(`
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at
		FROM sessions WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.SyncID, &s.Project, &s.Directory, &s.DevID, &s.Client,
		&startedAtStr, &endedAt, &summary, &syncedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	s.StartedAt, _ = parseTimeStr(startedAtStr)
	if summary.Valid {
		s.Summary = summary.String
	}
	if endedAt.Valid && endedAt.String != "" {
		t, _ := parseTimeStr(endedAt.String)
		s.EndedAt = &t
	}
	if syncedAt.Valid && syncedAt.String != "" {
		t, _ := parseTimeStr(syncedAt.String)
		s.SyncedAt = &t
	}

	return &s, nil
}

// EndSession marks a session as ended with the provided summary.
func (d *DB) EndSession(id, summary string) error {
	result, err := d.sqlDB.Exec(`
		UPDATE sessions SET ended_at = CURRENT_TIMESTAMP, summary = ?
		WHERE id = ?`, summary, id,
	)
	if err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// ListSessions returns sessions for a project ordered by started_at DESC, capped at limit.
func (d *DB) ListSessions(project string, limit int) ([]*models.Session, error) {
	rows, err := d.sqlDB.Query(`
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at
		FROM sessions WHERE project = ?
		ORDER BY started_at DESC
		LIMIT ?`, project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

// EnsureManualSaveSession idempotently creates 'manual-save-{project}' and
// returns its id. Uses INSERT OR IGNORE so concurrent calls are safe.
// This session is never auto-closed by AutoCloseStale (exempt by id prefix).
func (d *DB) EnsureManualSaveSession(project string) (string, error) {
	id := "manual-save-" + project
	devID := resolveDevID()

	_, err := d.sqlDB.Exec(`
		INSERT OR IGNORE INTO sessions
		    (id, sync_id, project, directory, dev_id, client)
		VALUES (?, lower(hex(randomblob(16))), ?, '', ?, 'manual')`,
		id, project, devID,
	)
	if err != nil {
		return "", fmt.Errorf("ensure manual save session: %w", err)
	}
	return id, nil
}

// AutoCloseStale closes all open sessions whose started_at is older than
// threshold, except sessions whose id starts with 'manual-save-'.
// nowFn is injectable for test control.
// Returns the number of sessions that were closed.
func (d *DB) AutoCloseStale(threshold time.Duration, nowFn func() time.Time) (int64, error) {
	cutoff := nowFn().Add(-threshold).UTC().Format("2006-01-02 15:04:05")
	result, err := d.sqlDB.Exec(`
		UPDATE sessions
		SET ended_at = CURRENT_TIMESTAMP,
		    summary  = '[auto-closed: daemon restart]'
		WHERE ended_at IS NULL
		  AND started_at < ?
		  AND id NOT LIKE 'manual-save-%'`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("auto close stale sessions: %w", err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// ListUnsyncedSessions returns all sessions for the project that haven't been synced yet
// (synced_at IS NULL). Used by the Syncer to build the sessions[] push payload.
func (d *DB) ListUnsyncedSessions(project string) ([]*models.Session, error) {
	rows, err := d.sqlDB.Query(`
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at
		FROM sessions WHERE project = ? AND synced_at IS NULL`, project,
	)
	if err != nil {
		return nil, fmt.Errorf("list unsynced sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced session: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// MarkSessionSynced sets synced_at for a session identified by id.
func (d *DB) MarkSessionSynced(id string, at time.Time) error {
	_, err := d.sqlDB.Exec(
		`UPDATE sessions SET synced_at = ? WHERE id = ?`,
		at.UTC().Format("2006-01-02 15:04:05"), id,
	)
	if err != nil {
		return fmt.Errorf("mark session synced: %w", err)
	}
	return nil
}

// SaveSessionFromRemote upserts a session received from the server.
// Uses the sync_id for conflict detection — ON CONFLICT(id) keeps first-arriving
// sentinel rows intact for legacy-pre-lifecycle-* IDs.
func (d *DB) SaveSessionFromRemote(s *models.Session) error {
	_, err := d.sqlDB.Exec(`
		INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO NOTHING`,
		s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
		s.StartedAt.UTC().Format("2006-01-02 15:04:05"),
		formatNullableTime(s.EndedAt),
		emptyToNil(s.Summary),
	)
	if err != nil {
		return fmt.Errorf("save session from remote: %w", err)
	}
	return nil
}

// resolveDevID returns the value of HIVE_DEV_ID env var, or 'unknown' if absent.
func resolveDevID() string {
	if v := os.Getenv("HIVE_DEV_ID"); v != "" {
		return v
	}
	return "unknown"
}

// --- helpers ---

func formatNullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func emptyToNil(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

type sessionScanner interface {
	Scan(...any) error
}

func scanSession(s sessionScanner) (*models.Session, error) {
	var sess models.Session
	var endedAt, syncedAt, summary sql.NullString
	var startedAtStr string

	err := s.Scan(
		&sess.ID, &sess.SyncID, &sess.Project, &sess.Directory, &sess.DevID, &sess.Client,
		&startedAtStr, &endedAt, &summary, &syncedAt,
	)
	if err != nil {
		return nil, err
	}

	sess.StartedAt, _ = parseTimeStr(startedAtStr)
	if summary.Valid {
		sess.Summary = summary.String
	}
	if endedAt.Valid && endedAt.String != "" {
		t, _ := parseTimeStr(endedAt.String)
		sess.EndedAt = &t
	}
	if syncedAt.Valid && syncedAt.String != "" {
		t, _ := parseTimeStr(syncedAt.String)
		sess.SyncedAt = &t
	}

	return &sess, nil
}
