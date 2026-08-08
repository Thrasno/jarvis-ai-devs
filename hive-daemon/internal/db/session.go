package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

// ErrSessionNotFound is returned when a requested session does not exist.
// Callers use errors.Is for comparison.
var ErrSessionNotFound = errors.New("session not found")

// CreateSession inserts a new session row.
// Returns an error if the id already exists (use EnsureManualSaveSession for idempotent inserts).
// BUG-DEVID-EMPTY: an empty devID (after trimming) falls back to resolveDevID()
// here so that NO insert caller — MCP or hook path — can ever persist a session
// with an empty dev_id. hive-api rejects empty dev_id via binding:"required", and a
// single poisoned row blocks the whole batched sync push.
func (d *DB) CreateSession(id, project, directory, devID, client string) error {
	canonicalProject, err := registerProjectIdentity(context.Background(), d.sqlDB, project)
	if err != nil {
		return err
	}
	project = canonicalProject
	if err := d.ensureProjectWritable(context.Background(), project); err != nil {
		return err
	}
	if strings.TrimSpace(devID) == "" {
		devID = resolveDevID()
	}
	_, err = d.sqlDB.Exec(`
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
	blocked, err := d.IsProjectBlocked(context.Background(), s.Project)
	if err != nil {
		return nil, fmt.Errorf("get session block check: %w", err)
	}
	if blocked {
		return nil, ErrSessionNotFound
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
	var project string
	err := d.sqlDB.QueryRow(`SELECT project FROM sessions WHERE id = ?`, id).Scan(&project)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("load session for end: %w", err)
	}
	if err := d.ensureProjectWritable(context.Background(), project); err != nil {
		return err
	}
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
	project = canonicalProjectKey(project)
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("list sessions block check: %w", err)
	}
	if blocked {
		return []*models.Session{}, nil
	}
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
	canonicalProject, err := registerProjectIdentity(context.Background(), d.sqlDB, project)
	if err != nil {
		return "", err
	}
	project = canonicalProject
	if err := d.ensureProjectWritable(context.Background(), project); err != nil {
		return "", err
	}
	id := "manual-save-" + project
	devID := resolveDevID()

	_, err = d.sqlDB.Exec(`
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
	project = canonicalProjectKey(project)
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("list unsynced sessions block check: %w", err)
	}
	if blocked {
		return []*models.Session{}, nil
	}
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

// ListUnsyncedSessionsPage returns at most `limit` unsynced sessions for the
// project (synced_at IS NULL), ordered by created_at ASC with id ASC as a
// secondary tiebreaker. This is the paged counterpart to
// ListUnsyncedSessions (PR 1b-iii, hive-sync-batched-drain): syncBatchStep
// uses this instead of the unpaged getter so a single push batch never
// exceeds syncPageSize, while the ORDER BY keeps paging stable across
// repeated calls as earlier rows get marked synced. created_at has only
// second-level granularity, so without the secondary id ASC key, rows
// created within the same second could be returned in a nondeterministic
// order across pages and be skipped or duplicated as the synced_at IS NULL
// filter shifts between fetches. ListUnsyncedSessions itself is left
// untouched for any other existing callers.
func (d *DB) ListUnsyncedSessionsPage(project string, limit int) ([]*models.Session, error) {
	project = canonicalProjectKey(project)
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("list unsynced sessions page block check: %w", err)
	}
	if blocked {
		return []*models.Session{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.sqlDB.Query(`
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at
		FROM sessions WHERE project = ? AND synced_at IS NULL
		ORDER BY created_at ASC, id ASC LIMIT ?`, project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list unsynced sessions page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced session page: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// MarkSessionSynced sets synced_at for a session identified by id.
func (d *DB) MarkSessionSynced(id string, at time.Time) error {
	result, err := d.sqlDB.Exec(
		`UPDATE sessions SET synced_at = ? WHERE id = ?`,
		at.UTC().Format("2006-01-02 15:04:05"), id,
	)
	if err != nil {
		return fmt.Errorf("mark session synced: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark session synced rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// SaveSessionFromRemote upserts a session received from the server.
// Uses the sync_id for conflict detection — ON CONFLICT(id) keeps first-arriving
// sentinel rows intact for legacy-pre-lifecycle-* IDs.
func (d *DB) SaveSessionFromRemote(s *models.Session) error {
	if err := d.ensureProjectWritable(context.Background(), s.Project); err != nil {
		return err
	}
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
