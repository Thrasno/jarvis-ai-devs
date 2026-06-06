package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
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

type MutationOp string

const (
	MutationOpCreate  MutationOp = "create"
	MutationOpUpdate  MutationOp = "update"
	MutationOpDelete  MutationOp = "delete"
	MutationOpRestore MutationOp = "restore"
)

type MutationEnvelope struct {
	EventID       string                    `json:"event_id"`
	EntityType    string                    `json:"entity_type"`
	EntitySyncID  string                    `json:"entity_sync_id"`
	Project       string                    `json:"project"`
	Op            MutationOp                `json:"op"`
	Sequence      int64                     `json:"sequence"`
	OccurredAt    time.Time                 `json:"occurred_at"`
	ActorID       string                    `json:"actor_id,omitempty"`
	BaseUpdatedAt *time.Time                `json:"base_updated_at,omitempty"`
	Memory        *MutationMemoryPayload    `json:"memory,omitempty"`
	Tombstone     *MutationTombstonePayload `json:"tombstone,omitempty"`
}

type MutationCursor struct {
	Sequence int64  `json:"sequence"`
	EventID  string `json:"event_id"`
}

type MutationMemoryPayload struct {
	SyncID        string    `json:"sync_id"`
	Project       string    `json:"project"`
	TopicKey      *string   `json:"topic_key,omitempty"`
	Category      string    `json:"category"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Tags          []string  `json:"tags"`
	FilesAffected []string  `json:"files_affected"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	SessionID     string    `json:"session_id,omitempty"`
}

type MutationTombstonePayload struct {
	DeletedAt time.Time `json:"deleted_at"`
	DeletedBy string    `json:"deleted_by,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

type memoryMutationRecord struct {
	EventID      string
	EntitySyncID string
	Project      string
	Op           MutationOp
	OccurredAt   string
	ActorID      string
	Payload      mutationPayload
}

type mutationPayload struct {
	Memory    *MutationMemoryPayload    `json:"memory,omitempty"`
	Tombstone *MutationTombstonePayload `json:"tombstone,omitempty"`
}

func memoryPayloadFromModel(mem *models.Memory, syncID, createdBy string, occurredAt time.Time) *MutationMemoryPayload {
	createdAt := mem.CreatedAt
	if createdAt.IsZero() {
		createdAt = occurredAt
	}
	updatedAt := occurredAt
	if !mem.UpdatedAt.IsZero() {
		updatedAt = mem.UpdatedAt
	}
	if createdBy == "" {
		createdBy = mem.CreatedBy
	}
	return &MutationMemoryPayload{
		SyncID:        syncID,
		Project:       mem.Project,
		TopicKey:      mem.TopicKey,
		Category:      mem.Category,
		Title:         mem.Title,
		Content:       mem.Content,
		Tags:          orNil(mem.Tags),
		FilesAffected: orNil(mem.FilesAffected),
		CreatedBy:     createdBy,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		SessionID:     mem.SessionID,
	}
}

func insertMemoryMutation(tx *sql.Tx, record memoryMutationRecord) error {
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return fmt.Errorf("marshal mutation payload: %w", err)
	}
	_, err = tx.Exec(`
INSERT INTO memory_mutations
    (event_id, entity_type, entity_sync_id, project, op, occurred_at, actor_id, payload_json)
VALUES (?, 'memory', ?, ?, ?, ?, ?, ?)`,
		record.EventID, record.EntitySyncID, record.Project, string(record.Op), record.OccurredAt, record.ActorID, string(payload),
	)
	return err
}

// GetUnsynced devuelve todas las memorias que aún no se han enviado al servidor
// (synced_at IS NULL). Son las que hay que incluir en el próximo push.
func (d *DB) GetUnsynced(project string) ([]*models.Memory, error) {
	q := `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
	   created_by, created_at, updated_at, synced_at, session_id
FROM memories
WHERE synced_at IS NULL AND sync_id != '' AND deleted_at IS NULL`

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

func (d *DB) GetPendingMutations(project string, limit int) ([]MutationEnvelope, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
SELECT sequence, event_id, entity_type, entity_sync_id, project, op, occurred_at, actor_id, base_updated_at, payload_json
FROM memory_mutations
WHERE synced_at IS NULL`
	args := []any{}
	if project != "" {
		q += ` AND project = ?`
		args = append(args, project)
	}
	q += ` ORDER BY sequence ASC, event_id ASC LIMIT ?`
	args = append(args, limit)

	rows, err := d.sqlDB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get pending mutations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mutations []MutationEnvelope
	for rows.Next() {
		mutation, err := scanMutationEnvelope(rows)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, mutation)
	}
	return mutations, rows.Err()
}

func (d *DB) MarkMutationsSynced(eventIDs []string, at time.Time) error {
	if len(eventIDs) == 0 {
		return nil
	}
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin mark mutations synced: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	formatted := at.UTC().Format("2006-01-02 15:04:05")
	for _, eventID := range eventIDs {
		if _, err := tx.Exec(`UPDATE memory_mutations SET synced_at = ? WHERE event_id = ?`, formatted, eventID); err != nil {
			return fmt.Errorf("mark mutation synced %s: %w", eventID, err)
		}
	}
	return tx.Commit()
}

func (d *DB) GetMutationCursor(consumer, project string) (MutationCursor, error) {
	var cursor MutationCursor
	err := d.sqlDB.QueryRow(`
SELECT sequence, event_id
FROM mutation_cursors
WHERE consumer = ? AND project = ?`, consumer, project).Scan(&cursor.Sequence, &cursor.EventID)
	if errors.Is(err, sql.ErrNoRows) {
		return MutationCursor{}, nil
	}
	if err != nil {
		return MutationCursor{}, fmt.Errorf("get mutation cursor: %w", err)
	}
	return cursor, nil
}

func (d *DB) SetMutationCursor(consumer, project string, cursor MutationCursor, at time.Time) error {
	_, err := d.sqlDB.Exec(`
INSERT INTO mutation_cursors (consumer, project, sequence, event_id, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(consumer, project) DO UPDATE SET
    sequence = excluded.sequence,
    event_id = excluded.event_id,
    updated_at = excluded.updated_at`,
		consumer, project, cursor.Sequence, cursor.EventID, at.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("set mutation cursor: %w", err)
	}
	return nil
}

func (d *DB) ApplyRemoteMutation(event MutationEnvelope) (bool, error) {
	if event.EventID == "" {
		return false, fmt.Errorf("event_id is required")
	}
	if event.EntityType == "" {
		event.EntityType = "memory"
	}
	if event.EntityType != "memory" {
		return false, fmt.Errorf("unsupported mutation entity_type %q", event.EntityType)
	}
	if event.EntitySyncID == "" {
		return false, fmt.Errorf("entity_sync_id is required")
	}
	if event.Project == "" {
		return false, fmt.Errorf("project is required")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	tx, err := d.sqlDB.Begin()
	if err != nil {
		return false, fmt.Errorf("begin apply remote mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing string
	err = tx.QueryRow(`SELECT event_id FROM memory_mutations WHERE event_id = ?`, event.EventID).Scan(&existing)
	if err == nil {
		return false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("check remote mutation idempotency: %w", err)
	}

	switch event.Op {
	case MutationOpCreate:
		if event.Memory == nil {
			return false, fmt.Errorf("memory payload required for %s mutation", event.Op)
		}
		if err := ensureMutationSession(tx, event.Project, event.Memory.SessionID); err != nil {
			return false, err
		}
		createdAt := event.OccurredAt.UTC().Format("2006-01-02 15:04:05")
		var existingID int64
		lookupErr := tx.QueryRow(`SELECT id FROM memories WHERE sync_id = ?`, event.EntitySyncID).Scan(&existingID)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return false, fmt.Errorf("lookup remote memory: %w", lookupErr)
		}
		if errors.Is(lookupErr, sql.ErrNoRows) {
			_, err = tx.Exec(`
INSERT INTO memories
	(sync_id, project, topic_key, category, title, content, tags, files_affected,
	 created_by, created_at, updated_at, synced_at, session_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				event.EntitySyncID, event.Project, event.Memory.TopicKey, event.Memory.Category,
				event.Memory.Title, event.Memory.Content, mustMarshalStrings(event.Memory.Tags), mustMarshalStrings(event.Memory.FilesAffected),
				event.Memory.CreatedBy, createdAt, createdAt, createdAt, event.Memory.SessionID,
			)
		} else {
			_, err = tx.Exec(`
UPDATE memories SET
	topic_key = ?, category = ?, title = ?, content = ?, tags = ?, files_affected = ?,
	updated_at = ?, synced_at = ?, session_id = ?
WHERE sync_id = ?`,
				event.Memory.TopicKey, event.Memory.Category, event.Memory.Title, event.Memory.Content,
				mustMarshalStrings(event.Memory.Tags), mustMarshalStrings(event.Memory.FilesAffected),
				createdAt, createdAt, event.Memory.SessionID,
				event.EntitySyncID,
			)
		}
	case MutationOpUpdate:
		if event.Memory == nil {
			return false, fmt.Errorf("memory payload required for %s mutation", event.Op)
		}
		createdAt := event.OccurredAt.UTC().Format("2006-01-02 15:04:05")
		var existingID int64
		var deletedAt sql.NullString
		lookupErr := tx.QueryRow(`SELECT id, deleted_at FROM memories WHERE sync_id = ?`, event.EntitySyncID).Scan(&existingID, &deletedAt)
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return false, fmt.Errorf("memory not found for update: sync_id=%s", event.EntitySyncID)
		}
		if lookupErr != nil {
			return false, fmt.Errorf("lookup remote memory: %w", lookupErr)
		}
		if deletedAt.Valid {
			return false, fmt.Errorf("memory is deleted; explicit restore required before update")
		}
		if err := ensureMutationSession(tx, event.Project, event.Memory.SessionID); err != nil {
			return false, err
		}
		_, err = tx.Exec(`
UPDATE memories SET
	topic_key = ?, category = ?, title = ?, content = ?, tags = ?, files_affected = ?,
	updated_at = ?, synced_at = ?, session_id = ?
WHERE sync_id = ? AND deleted_at IS NULL`,
			event.Memory.TopicKey, event.Memory.Category, event.Memory.Title, event.Memory.Content,
			mustMarshalStrings(event.Memory.Tags), mustMarshalStrings(event.Memory.FilesAffected),
			createdAt, createdAt, event.Memory.SessionID,
			event.EntitySyncID,
		)
	case MutationOpDelete:
		deletedAt := event.OccurredAt.UTC().Format("2006-01-02 15:04:05")
		deletedBy := event.ActorID
		reason := ""
		if event.Tombstone != nil {
			if !event.Tombstone.DeletedAt.IsZero() {
				deletedAt = event.Tombstone.DeletedAt.UTC().Format("2006-01-02 15:04:05")
			}
			deletedBy = event.Tombstone.DeletedBy
			reason = event.Tombstone.Reason
		}
		var result sql.Result
		result, err = tx.Exec(`UPDATE memories SET deleted_at = ?, deleted_by = ?, delete_reason = ?, restored_at = NULL, updated_at = ?, synced_at = ? WHERE sync_id = ?`, deletedAt, deletedBy, reason, deletedAt, deletedAt, event.EntitySyncID)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows == 0 {
				return false, fmt.Errorf("memory not found for delete: sync_id=%s", event.EntitySyncID)
			}
		}
	case MutationOpRestore:
		restoredAt := event.OccurredAt.UTC().Format("2006-01-02 15:04:05")
		var result sql.Result
		result, err = tx.Exec(`UPDATE memories SET deleted_at = NULL, deleted_by = NULL, delete_reason = NULL, restored_at = ?, updated_at = ?, synced_at = ? WHERE sync_id = ? AND deleted_at IS NOT NULL`, restoredAt, restoredAt, restoredAt, event.EntitySyncID)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows == 0 {
				return false, fmt.Errorf("memory not deleted for restore: sync_id=%s", event.EntitySyncID)
			}
		}
	default:
		return false, fmt.Errorf("unsupported mutation op %q", event.Op)
	}
	if err != nil {
		return false, fmt.Errorf("apply remote %s mutation: %w", event.Op, err)
	}

	payload := mutationPayload{}
	if event.Memory != nil {
		payload.Memory = event.Memory
	}
	if event.Tombstone != nil {
		payload.Tombstone = &MutationTombstonePayload{
			DeletedAt: event.Tombstone.DeletedAt,
			DeletedBy: event.Tombstone.DeletedBy,
			Reason:    event.Tombstone.Reason,
		}
	}
	if err := insertMemoryMutation(tx, memoryMutationRecord{
		EventID:      event.EventID,
		EntitySyncID: event.EntitySyncID,
		Project:      event.Project,
		Op:           event.Op,
		OccurredAt:   event.OccurredAt.UTC().Format("2006-01-02 15:04:05"),
		ActorID:      event.ActorID,
		Payload:      payload,
	}); err != nil {
		return false, fmt.Errorf("record remote mutation: %w", err)
	}
	return true, tx.Commit()
}

// SaveFromRemote guarda una memoria recibida del servidor (pull).
// La marca como ya sincronizada para no reenviarla en el próximo push.
// INSERT OR IGNORE: si el sync_id ya existe localmente, no tocamos nada.
//
// R2-CRIT-3 — si la memoria llega sin session_id (legacy o bug del servidor),
// resolvemos defensivamente a `manual-save-{project}` para que el INSERT no quede
// silenciosamente descartado por la combinación de NOT NULL + INSERT OR IGNORE.
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

	// R2-CRIT-3: resolve session_id BEFORE the INSERT. memories.session_id is NOT NULL,
	// and `INSERT OR IGNORE` would silently drop the row on any constraint failure.
	sessionID := mem.SessionID
	if sessionID == "" {
		resolved, err := d.EnsureManualSaveSession(mem.Project)
		if err != nil {
			return fmt.Errorf("ensure manual-save session for remote insert: %w", err)
		}
		sessionID = resolved
		logger.Log.Printf("warn: SaveFromRemote(%s) had empty session_id; lazy-resolved to %q", mem.SyncID, sessionID)
	}

	_, err = d.sqlDB.Exec(`
INSERT OR IGNORE INTO memories
	(sync_id, project, topic_key, category, title, content, tags, files_affected,
	 created_by, created_at, updated_at, synced_at, session_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mem.SyncID, mem.Project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, string(tagsJSON), string(filesJSON),
		mem.CreatedBy, createdAt, updatedAt, now, sessionID,
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
		sessionID    sql.NullString
	)

	err := s.Scan(
		&mem.ID, &mem.SyncID, &mem.Project, &topicKey,
		&mem.Category, &mem.Title, &mem.Content,
		&tagsJSON, &filesJSON,
		&mem.CreatedBy, &createdAtStr, &updatedAtStr, &syncedAtStr,
		&sessionID,
	)
	if err != nil {
		return nil, err
	}

	if topicKey.Valid {
		mem.TopicKey = &topicKey.String
	}
	if sessionID.Valid {
		mem.SessionID = sessionID.String
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

func scanMutationEnvelope(s syncScanner) (MutationEnvelope, error) {
	var (
		mutation      MutationEnvelope
		op            string
		occurredAtStr string
		baseUpdatedAt sql.NullString
		payloadJSON   string
	)
	err := s.Scan(
		&mutation.Sequence,
		&mutation.EventID,
		&mutation.EntityType,
		&mutation.EntitySyncID,
		&mutation.Project,
		&op,
		&occurredAtStr,
		&mutation.ActorID,
		&baseUpdatedAt,
		&payloadJSON,
	)
	if err != nil {
		return MutationEnvelope{}, fmt.Errorf("scan mutation row: %w", err)
	}
	mutation.Op = MutationOp(op)
	mutation.OccurredAt, _ = parseTimeStr(occurredAtStr)
	if baseUpdatedAt.Valid {
		parsed, err := parseTimeStr(baseUpdatedAt.String)
		if err == nil {
			mutation.BaseUpdatedAt = &parsed
		}
	}
	var payload struct {
		Memory    *MutationMemoryPayload    `json:"memory"`
		Tombstone *MutationTombstonePayload `json:"tombstone"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err == nil {
		mutation.Memory = payload.Memory
		mutation.Tombstone = payload.Tombstone
	}
	return mutation, nil
}

func ensureMutationSession(tx *sql.Tx, project, sessionID string) error {
	if sessionID == "" {
		sessionID = "manual-save-" + project
	}
	_, err := tx.Exec(`
INSERT OR IGNORE INTO sessions
    (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
VALUES (?, lower(hex(randomblob(16))), ?, '', 'remote', 'remote', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'Remote mutation fallback session.')`,
		sessionID, project,
	)
	if err != nil {
		return fmt.Errorf("ensure mutation session: %w", err)
	}
	return nil
}

func mustMarshalStrings(values []string) string {
	encoded, err := json.Marshal(orNil(values))
	if err != nil {
		return "[]"
	}
	return string(encoded)
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
