package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

const maxSyncLastErrorRunes = 500

type SyncHealth struct {
	Project             string
	LastAttemptAt       time.Time
	LastSuccessAt       time.Time
	LastFailureAt       time.Time
	BackoffUntil        time.Time
	ConsecutiveFailures int
	LastError           string
}

// GetUnsynced devuelve todas las memorias que aún no se han enviado al servidor
// (synced_at IS NULL). Son las que hay que incluir en el próximo push.
func (d *DB) GetUnsynced(project string) ([]*models.Memory, error) {
	q := `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
       created_by, created_at, updated_at, synced_at, confidence, impact_score
FROM memories
WHERE synced_at IS NULL AND sync_id != ''`

	args := []any{}
	if project != "" {
		q += " AND project = ?"
		args = append(args, project)
	}
	q += " ORDER BY created_at ASC"

	rows, err := d.sqlDB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get unsynced: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanSyncRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced row: %w", err)
		}
		results = append(results, mem)
	}
	return results, rows.Err()
}

// MarkSynced marca una memoria como sincronizada con el servidor.
func (d *DB) MarkSynced(syncID string, at time.Time) error {
	result, err := d.sqlDB.Exec(
		`UPDATE memories SET synced_at = ? WHERE sync_id = ?`,
		at.UTC().Format("2006-01-02 15:04:05"), syncID,
	)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		logger.Log.Printf("warn: MarkSynced: no row found for sync_id %s", syncID)
	}
	return nil
}

// SaveFromRemote guarda una memoria recibida del servidor (pull).
// La marca como ya sincronizada para no reenviarla en el próximo push.
// INSERT OR IGNORE: si el sync_id ya existe localmente, no tocamos nada.
func (d *DB) SaveFromRemote(mem *models.Memory) error {
	tagsJSON, err := json.Marshal(orNil(mem.Tags))
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	filesJSON, err := json.Marshal(orNil(mem.FilesAffected))
	if err != nil {
		return fmt.Errorf("marshal files: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	createdAt := mem.CreatedAt.UTC().Format("2006-01-02 15:04:05")
	updatedAt := mem.UpdatedAt.UTC().Format("2006-01-02 15:04:05")

	_, err = d.sqlDB.Exec(`
INSERT OR IGNORE INTO memories
    (sync_id, project, topic_key, category, title, content, tags, files_affected,
     created_by, created_at, updated_at, synced_at, confidence, impact_score)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mem.SyncID, mem.Project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, string(tagsJSON), string(filesJSON),
		mem.CreatedBy, createdAt, updatedAt, now,
		mem.Confidence, mem.ImpactScore,
	)
	return err
}

// GetLastSync devuelve el timestamp del último sync exitoso para un proyecto.
func (d *DB) GetLastSync(project string) (time.Time, error) {
	var ts sql.NullString
	err := d.sqlDB.QueryRow(
		`SELECT last_sync_at FROM sync_state WHERE project = ?`, project,
	).Scan(&ts)
	if err == sql.ErrNoRows || !ts.Valid {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return parseTimeStr(ts.String)
}

// SetLastSync actualiza el timestamp del último sync para un proyecto.
func (d *DB) SetLastSync(project string, at time.Time) error {
	return d.upsertSyncState(project, syncStateUpdate{
		lastSyncAt: timePtr(at),
	})
}

func (d *DB) GetSyncHealth(project string) (SyncHealth, error) {
	var (
		health                                SyncHealth
		lastAttempt, lastSuccess, lastFailure sql.NullString
		backoffUntil                          sql.NullString
		lastError                             sql.NullString
		consecutiveFailures                   sql.NullInt64
	)

	health.Project = project
	err := d.sqlDB.QueryRow(`
SELECT last_attempt_at, last_success_at, last_failure_at, consecutive_failures, backoff_until, last_error
FROM sync_state WHERE project = ?`, project).Scan(
		&lastAttempt,
		&lastSuccess,
		&lastFailure,
		&consecutiveFailures,
		&backoffUntil,
		&lastError,
	)
	if err == sql.ErrNoRows {
		return health, nil
	}
	if err != nil {
		return SyncHealth{}, err
	}

	health.LastAttemptAt = parseNullTime(lastAttempt)
	health.LastSuccessAt = parseNullTime(lastSuccess)
	health.LastFailureAt = parseNullTime(lastFailure)
	health.BackoffUntil = parseNullTime(backoffUntil)
	if consecutiveFailures.Valid {
		health.ConsecutiveFailures = int(consecutiveFailures.Int64)
	}
	if lastError.Valid {
		health.LastError = lastError.String
	}

	return health, nil
}

func (d *DB) RecordSyncAttempt(project string, at time.Time) error {
	return d.upsertSyncState(project, syncStateUpdate{
		lastAttemptAt: timePtr(at),
	})
}

func (d *DB) RecordSyncSuccess(project string, at time.Time) error {
	return d.upsertSyncState(project, syncStateUpdate{
		lastSyncAt:          timePtr(at),
		lastAttemptAt:       timePtr(at),
		lastSuccessAt:       timePtr(at),
		clearLastFailureAt:  true,
		consecutiveFailures: intPtr(0),
		clearBackoffUntil:   true,
		lastError:           stringPtr(""),
	})
}

func (d *DB) RecordSyncFailure(project string, at time.Time, consecutiveFailures int, backoffUntil time.Time, syncErr error) error {
	return d.upsertSyncState(project, syncStateUpdate{
		lastAttemptAt:       timePtr(at),
		lastFailureAt:       timePtr(at),
		consecutiveFailures: intPtr(consecutiveFailures),
		backoffUntil:        timePtr(backoffUntil),
		lastError:           stringPtr(sanitizeSyncLastError(syncErr)),
	})
}

// GetJWT devuelve el JWT almacenado si aún es válido (margen de 1 hora).
func (d *DB) GetJWT() string {
	var token, expires sql.NullString
	err := d.sqlDB.QueryRow(
		`SELECT jwt_token, jwt_expires_at FROM sync_state WHERE project = '__auth__'`,
	).Scan(&token, &expires)
	if err != nil || !token.Valid || !expires.Valid {
		return ""
	}
	exp, err := parseTimeStr(expires.String)
	if err != nil || time.Now().Add(time.Hour).After(exp) {
		return ""
	}
	return token.String
}

// SetJWT almacena el JWT con su fecha de expiración.
func (d *DB) SetJWT(token string, expiresAt time.Time) error {
	_, err := d.sqlDB.Exec(`
INSERT INTO sync_state (project, jwt_token, jwt_expires_at)
VALUES ('__auth__', ?, ?)
ON CONFLICT(project) DO UPDATE SET
    jwt_token      = excluded.jwt_token,
    jwt_expires_at = excluded.jwt_expires_at`,
		token, expiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	return err
}

// --- helpers privados ---

type syncScanner interface {
	Scan(...any) error
}

func scanSyncRow(s syncScanner) (*models.Memory, error) {
	var (
		mem          models.Memory
		topicKey     sql.NullString
		tagsJSON     string
		filesJSON    string
		createdAtStr string
		updatedAtStr string
		syncedAtStr  sql.NullString
	)

	err := s.Scan(
		&mem.ID, &mem.SyncID, &mem.Project, &topicKey,
		&mem.Category, &mem.Title, &mem.Content,
		&tagsJSON, &filesJSON,
		&mem.CreatedBy, &createdAtStr, &updatedAtStr, &syncedAtStr,
		&mem.Confidence, &mem.ImpactScore,
	)
	if err != nil {
		return nil, err
	}

	if topicKey.Valid {
		mem.TopicKey = &topicKey.String
	}

	mem.CreatedAt, _ = parseTimeStr(createdAtStr)
	mem.UpdatedAt, _ = parseTimeStr(updatedAtStr)

	if syncedAtStr.Valid {
		t, _ := parseTimeStr(syncedAtStr.String)
		mem.SyncedAt = &t
	}

	_ = json.Unmarshal([]byte(tagsJSON), &mem.Tags)
	_ = json.Unmarshal([]byte(filesJSON), &mem.FilesAffected)

	return &mem, nil
}

func parseTimeStr(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}

type syncStateUpdate struct {
	lastSyncAt          *string
	lastAttemptAt       *string
	lastSuccessAt       *string
	lastFailureAt       *string
	clearLastFailureAt  bool
	consecutiveFailures *int
	backoffUntil        *string
	clearBackoffUntil   bool
	lastError           *string
}

func (d *DB) upsertSyncState(project string, update syncStateUpdate) error {
	if _, err := d.sqlDB.Exec(`
INSERT OR IGNORE INTO sync_state (project, consecutive_failures, last_error)
VALUES (?, 0, '')`, project); err != nil {
		return err
	}

	_, err := d.sqlDB.Exec(`
UPDATE sync_state SET
	last_sync_at = COALESCE(?, last_sync_at),
	last_attempt_at = COALESCE(?, last_attempt_at),
	last_success_at = COALESCE(?, last_success_at),
	last_failure_at = CASE
		WHEN ? THEN NULL
		ELSE COALESCE(?, last_failure_at)
	END,
	consecutive_failures = COALESCE(?, consecutive_failures),
	backoff_until = CASE
		WHEN ? THEN NULL
		ELSE COALESCE(?, backoff_until)
	END,
	last_error = COALESCE(?, last_error)
WHERE project = ?`,
		nullableString(update.lastSyncAt),
		nullableString(update.lastAttemptAt),
		nullableString(update.lastSuccessAt),
		update.clearLastFailureAt,
		nullableString(update.lastFailureAt),
		nullableInt(update.consecutiveFailures),
		update.clearBackoffUntil,
		nullableString(update.backoffUntil),
		nullableString(update.lastError),
		project,
	)
	return err
}

func parseNullTime(value sql.NullString) time.Time {
	if !value.Valid || value.String == "" {
		return time.Time{}
	}
	parsed, err := parseTimeStr(value.String)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func timePtr(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format("2006-01-02 15:04:05")
	return &formatted
}

func intPtr(value int) *int {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func sanitizeSyncLastError(err error) string {
	if err == nil {
		return ""
	}

	trimmed := strings.TrimSpace(stripHTTPErrorBody(err.Error()))
	if trimmed == "" {
		return ""
	}

	var builder strings.Builder
	count := 0
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			continue
		}
		builder.WriteRune(r)
		count++
		if count >= maxSyncLastErrorRunes {
			break
		}
	}

	return strings.TrimSpace(builder.String())
}

func stripHTTPErrorBody(message string) string {
	trimmed := strings.TrimSpace(message)
	for _, prefix := range []string{"login failed (", "sync failed ("} {
		if strings.HasPrefix(trimmed, prefix) {
			head, _, found := strings.Cut(trimmed, ":")
			if found {
				return strings.TrimSpace(head)
			}
		}
	}
	return trimmed
}

func orNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
