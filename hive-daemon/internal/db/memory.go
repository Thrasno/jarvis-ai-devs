package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/google/uuid"
)

var (
	ErrMemoryNotFound       = errors.New("memory not found")
	ErrMemoryAlreadyDeleted = errors.New("memory already deleted")
	ErrMemoryNotDeleted     = errors.New("memory not deleted")
)

// SaveMemory always inserts a new row. topic_key is a grouping/context key, not
// an identity key — saving twice with the same topic_key creates two distinct
// rows (Issue #119). sync_id is the idempotency key. Returns the new row's id.
func (d *DB) SaveMemory(mem *models.Memory) (int64, error) {
	if err := mem.Validate(); err != nil {
		return 0, fmt.Errorf("invalid memory: %w", err)
	}

	tagsJSON, err := marshalStringSlice(mem.Tags)
	if err != nil {
		return 0, fmt.Errorf("marshal tags: %w", err)
	}
	filesJSON, err := marshalStringSlice(mem.FilesAffected)
	if err != nil {
		return 0, fmt.Errorf("marshal files_affected: %w", err)
	}

	syncID := uuid.New().String()
	createdBy := detectUsername()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	op := MutationOpCreate

	var sessionID sql.NullString
	if mem.SessionID != "" {
		sessionID = sql.NullString{String: mem.SessionID, Valid: true}
	}

	tx, err := d.sqlDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin save memory: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const q = `
INSERT INTO memories
	(sync_id, project, topic_key, category, title, content, tags, files_affected,
	 created_by, created_at, updated_at, session_id)
VALUES
	(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`

	var id int64
	err = tx.QueryRow(q,
		syncID, mem.Project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, tagsJSON, filesJSON,
		createdBy, now, now, sessionID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save memory: %w", err)
	}
	if err := insertMemoryMutation(tx, memoryMutationRecord{
		EventID:      uuid.New().String(),
		EntitySyncID: syncID,
		Project:      mem.Project,
		Op:           op,
		OccurredAt:   now,
		ActorID:      createdBy,
		Payload: mutationPayload{
			Memory: memoryPayloadFromModel(mem, syncID, createdBy, time.Now().UTC()),
		},
	}); err != nil {
		return 0, fmt.Errorf("journal memory mutation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit save memory: %w", err)
	}
	return id, nil
}

// GetMemory retrieves an active memory by its id.
// Returns an error if not found or tombstoned.
func (d *DB) GetMemory(id int64) (*models.Memory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
	   created_by, created_at, session_id
FROM memories WHERE id = ? AND deleted_at IS NULL`

	row := d.sqlDB.QueryRow(q, id)
	mem, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("memory not found: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	return mem, nil
}

// ListMemories returns active memories for a project, ordered by created_at DESC.
func (d *DB) ListMemories(project string, limit int) ([]*models.Memory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
	   created_by, created_at, session_id
FROM memories
WHERE project = ? AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT ?`

	rows, err := d.sqlDB.Query(q, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan memory row: %w", err)
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

type DeletedMemory struct {
	Memory       *models.Memory
	DeletedAt    time.Time
	DeletedBy    string
	DeleteReason string
	RestoredAt   time.Time
}

func (d *DB) GetDeletedMemory(id int64) (*DeletedMemory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
	   created_by, created_at, session_id,
	   deleted_at, deleted_by, delete_reason, restored_at
FROM memories WHERE id = ? AND deleted_at IS NOT NULL`

	var deletedAt, deletedBy, reason, restoredAt sql.NullString
	mem, err := scanMemoryWithExtra(d.sqlDB.QueryRow(q, id), &deletedAt, &deletedBy, &reason, &restoredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("deleted memory not found: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get deleted memory: %w", err)
	}

	deleted := &DeletedMemory{Memory: mem}
	if deletedAt.Valid {
		deleted.DeletedAt, _ = parseTimeStr(deletedAt.String)
	}
	if deletedBy.Valid {
		deleted.DeletedBy = deletedBy.String
	}
	if reason.Valid {
		deleted.DeleteReason = reason.String
	}
	if restoredAt.Valid {
		deleted.RestoredAt, _ = parseTimeStr(restoredAt.String)
	}
	return deleted, nil
}

func (d *DB) DeleteMemory(id int64, actorID, reason string) error {
	if actorID == "" {
		actorID = detectUsername()
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin delete memory: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var syncID, project string
	var deletedAt sql.NullString
	err = tx.QueryRow(`SELECT sync_id, project, deleted_at FROM memories WHERE id = ?`, id).Scan(&syncID, &project, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: id=%d", ErrMemoryNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("load memory for delete: %w", err)
	}
	if deletedAt.Valid {
		return fmt.Errorf("%w: id=%d", ErrMemoryAlreadyDeleted, id)
	}

	result, err := tx.Exec(`
UPDATE memories
SET deleted_at = ?, deleted_by = ?, delete_reason = ?, restored_at = NULL, updated_at = ?, synced_at = NULL
WHERE id = ? AND deleted_at IS NULL`, now, actorID, reason, now, id)
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: id=%d", ErrMemoryAlreadyDeleted, id)
	}

	if err := insertMemoryMutation(tx, memoryMutationRecord{
		EventID:      uuid.New().String(),
		EntitySyncID: syncID,
		Project:      project,
		Op:           MutationOpDelete,
		OccurredAt:   now,
		ActorID:      actorID,
		Payload: mutationPayload{
			Tombstone: &MutationTombstonePayload{DeletedAt: parseTimeOrZero(now), DeletedBy: actorID, Reason: reason},
		},
	}); err != nil {
		return fmt.Errorf("journal delete mutation: %w", err)
	}
	return tx.Commit()
}

func (d *DB) RestoreMemory(id int64, actorID string) error {
	if actorID == "" {
		actorID = detectUsername()
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin restore memory: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var syncID, project string
	var deletedAt sql.NullString
	err = tx.QueryRow(`SELECT sync_id, project, deleted_at FROM memories WHERE id = ?`, id).Scan(&syncID, &project, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: id=%d", ErrMemoryNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("load memory for restore: %w", err)
	}
	if !deletedAt.Valid {
		return fmt.Errorf("%w: id=%d", ErrMemoryNotDeleted, id)
	}

	result, err := tx.Exec(`
UPDATE memories
SET deleted_at = NULL, deleted_by = NULL, delete_reason = NULL, restored_at = ?, updated_at = ?, synced_at = NULL
WHERE id = ? AND deleted_at IS NOT NULL`, now, now, id)
	if err != nil {
		return fmt.Errorf("restore memory: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: id=%d", ErrMemoryNotDeleted, id)
	}

	if err := insertMemoryMutation(tx, memoryMutationRecord{
		EventID:      uuid.New().String(),
		EntitySyncID: syncID,
		Project:      project,
		Op:           MutationOpRestore,
		OccurredAt:   now,
		ActorID:      actorID,
		Payload:      mutationPayload{},
	}); err != nil {
		return fmt.Errorf("journal restore mutation: %w", err)
	}
	return tx.Commit()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanMemory(s scanner) (*models.Memory, error) {
	return scanMemoryWithExtra(s)
}

func scanMemoryWithExtra(s scanner, extra ...any) (*models.Memory, error) {
	var (
		mem          models.Memory
		topicKey     sql.NullString
		tagsJSON     string
		filesJSON    string
		createdAtStr string
	)

	dest := []any{
		&mem.ID, &mem.SyncID, &mem.Project, &topicKey,
		&mem.Category, &mem.Title, &mem.Content,
		&tagsJSON, &filesJSON,
		&mem.CreatedBy, &createdAtStr,
		&mem.SessionID,
	}
	dest = append(dest, extra...)
	err := s.Scan(dest...)
	if err != nil {
		return nil, err
	}

	if topicKey.Valid {
		mem.TopicKey = &topicKey.String
	}

	if err := json.Unmarshal([]byte(tagsJSON), &mem.Tags); err != nil {
		mem.Tags = nil
	}
	if err := json.Unmarshal([]byte(filesJSON), &mem.FilesAffected); err != nil {
		mem.FilesAffected = nil
	}

	mem.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
	if err != nil {
		mem.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}

	return &mem, nil
}

func marshalStringSlice(s []string) (string, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseTimeOrZero(value string) time.Time {
	parsed, err := parseTimeStr(value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
