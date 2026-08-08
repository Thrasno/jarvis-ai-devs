package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/google/uuid"
)

var (
	ErrMemoryNotFound                  = errors.New("memory not found")
	ErrMemoryAlreadyDeleted            = errors.New("memory already deleted")
	ErrMemoryNotDeleted                = errors.New("memory not deleted")
	ErrGuardedMutationIdentityMismatch = errors.New("guarded mutation identity mismatch")
	ErrGuardedMutationRequestConflict  = errors.New("guarded mutation request conflicts with existing receipt")
)

const (
	GuardedMutationLocalCommitted = "committed"
	GuardedMutationSharedPending  = "pending"
)

// GuardedMemoryMutation is the compare-and-swap input accepted at the database
// boundary after backup and confirmation checks have passed.
type GuardedMemoryMutation struct {
	RequestID       string
	Operation       MutationOp
	TargetID        int64
	ExpectedProject string
	ExpectedSyncID  string
	ActorID         string
	Reason          string
}

// MutationReceipt separates the committed local change from asynchronous shared propagation.
type MutationReceipt struct {
	RequestID    string `json:"request_id"`
	Operation    string `json:"operation"`
	TargetID     int64  `json:"target_id"`
	Project      string `json:"project"`
	EntitySyncID string `json:"entity_sync_id"`
	EventID      string `json:"event_id"`
	LocalStatus  string `json:"local_status"`
	SharedStatus string `json:"shared_status"`
}

// SaveMemory always inserts a new row. topic_key is a grouping/context key, not
// an identity key — saving twice with the same topic_key creates two distinct
// rows (Issue #119). sync_id is the idempotency key. Returns the new row's id.
func (d *DB) SaveMemory(mem *models.Memory) (int64, error) {
	return d.saveMemory(mem, nil)
}

// SaveMemoryWithManualSession atomically creates the project's manual session
// when needed and saves the memory attributed to it.
func (d *DB) SaveMemoryWithManualSession(mem *models.Memory) (int64, error) {
	return d.saveMemory(mem, func(tx *sql.Tx) error {
		mem.SessionID = "manual-save-" + mem.Project
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO sessions
			    (id, sync_id, project, directory, dev_id, client)
			VALUES (?, lower(hex(randomblob(16))), ?, '', ?, 'manual')`,
			mem.SessionID, mem.Project, resolveDevID(),
		)
		if err != nil {
			return fmt.Errorf("ensure manual save session: %w", err)
		}
		return nil
	})
}

func (d *DB) saveMemory(mem *models.Memory, prepareTx func(*sql.Tx) error) (int64, error) {
	if err := mem.Validate(); err != nil {
		return 0, fmt.Errorf("invalid memory: %w", err)
	}
	rawProject := mem.Project
	mem.Project = canonicalProjectKey(rawProject)
	if mem.Project == "" {
		return 0, fmt.Errorf("invalid memory: project is required")
	}
	if mem.SessionID == "manual-save-"+rawProject {
		mem.SessionID = "manual-save-" + mem.Project
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

	tx, err := d.sqlDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin save memory: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := registerProjectIdentity(context.Background(), tx, rawProject); err != nil {
		return 0, err
	}
	if err := ensureProjectWritableInTx(tx, mem.Project); err != nil {
		return 0, err
	}
	if prepareTx != nil {
		if err := prepareTx(tx); err != nil {
			return 0, err
		}
	}

	var sessionID sql.NullString
	if mem.SessionID != "" {
		sessionID = sql.NullString{String: mem.SessionID, Valid: true}
	}

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
	if mem.PromptID > 0 {
		if _, err := tx.Exec(
			`INSERT INTO memory_prompt_links (memory_id, prompt_id) VALUES (?, ?)`,
			id, mem.PromptID,
		); err != nil {
			return 0, fmt.Errorf("link memory prompt: %w", err)
		}
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
	blocked, err := d.IsProjectBlocked(context.Background(), mem.Project)
	if err != nil {
		return nil, fmt.Errorf("get memory block check: %w", err)
	}
	if blocked {
		return nil, fmt.Errorf("memory not found: id=%d", id)
	}
	return mem, nil
}

// ListMemories returns active memories for a project, ordered by created_at DESC.
func (d *DB) ListMemories(project string, limit int) ([]*models.Memory, error) {
	project = canonicalProjectKey(project)
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("list memories block check: %w", err)
	}
	if blocked {
		return []*models.Memory{}, nil
	}
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

// LatestMemoryTimestamp returns the created_at of the most recent active memory
// for a project. The bool reports whether any memory exists; when false, the
// returned time is the zero value. Blocked projects are treated as having no
// memories (found=false), inheriting ListMemories' blocked-project handling.
func (d *DB) LatestMemoryTimestamp(project string) (time.Time, bool, error) {
	mems, err := d.ListMemories(project, 1)
	if err != nil {
		return time.Time{}, false, err
	}
	if len(mems) == 0 {
		return time.Time{}, false, nil
	}
	return mems[0].CreatedAt, true, nil
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
	if err := ensureProjectWritableInTx(tx, project); err != nil {
		return err
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
	if err := ensureProjectWritableInTx(tx, project); err != nil {
		return err
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

// ExecuteGuardedMemoryMutation commits identity validation, the soft mutation,
// journal event, and receipt in one SQLite transaction. Reusing a request ID
// returns the original receipt only when every safety-relevant input matches.
func (d *DB) ExecuteGuardedMemoryMutation(req GuardedMemoryMutation) (MutationReceipt, error) {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.ExpectedProject = strings.TrimSpace(req.ExpectedProject)
	req.ExpectedSyncID = strings.TrimSpace(req.ExpectedSyncID)
	req.ActorID = strings.TrimSpace(req.ActorID)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.RequestID == "" || req.TargetID <= 0 || req.ExpectedProject == "" || req.ExpectedSyncID == "" || req.Reason == "" {
		return MutationReceipt{}, ErrGuardedMutationIdentityMismatch
	}
	if req.Operation != MutationOpDelete && req.Operation != MutationOpRestore {
		return MutationReceipt{}, fmt.Errorf("unsupported guarded mutation operation %q", req.Operation)
	}
	if req.ActorID == "" {
		req.ActorID = detectUsername()
	}
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return MutationReceipt{}, fmt.Errorf("begin guarded mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing MutationReceipt
	var existingActor, existingReason string
	err = tx.QueryRow(`SELECT request_id, operation, target_id, project, entity_sync_id, event_id, local_status, shared_status, actor_id, reason FROM mutation_receipts WHERE request_id = ?`, req.RequestID).Scan(
		&existing.RequestID, &existing.Operation, &existing.TargetID, &existing.Project, &existing.EntitySyncID, &existing.EventID, &existing.LocalStatus, &existing.SharedStatus, &existingActor, &existingReason)
	if err == nil {
		if existing.Operation != string(req.Operation) || existing.TargetID != req.TargetID || existing.Project != req.ExpectedProject || existing.EntitySyncID != req.ExpectedSyncID || existingActor != req.ActorID || existingReason != req.Reason {
			return MutationReceipt{}, ErrGuardedMutationRequestConflict
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MutationReceipt{}, fmt.Errorf("load guarded receipt: %w", err)
	}

	var project, syncID string
	var deletedAt sql.NullString
	err = tx.QueryRow(`SELECT project, sync_id, deleted_at FROM memories WHERE id = ?`, req.TargetID).Scan(&project, &syncID, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MutationReceipt{}, fmt.Errorf("%w: id=%d", ErrMemoryNotFound, req.TargetID)
	}
	if err != nil {
		return MutationReceipt{}, fmt.Errorf("load guarded target: %w", err)
	}
	if project != req.ExpectedProject || syncID != req.ExpectedSyncID {
		return MutationReceipt{}, ErrGuardedMutationIdentityMismatch
	}
	if err := ensureProjectWritableInTx(tx, project); err != nil {
		return MutationReceipt{}, err
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	switch req.Operation {
	case MutationOpDelete:
		if deletedAt.Valid {
			return MutationReceipt{}, fmt.Errorf("%w: id=%d", ErrMemoryAlreadyDeleted, req.TargetID)
		}
		if _, err = tx.Exec(`UPDATE memories SET deleted_at=?, deleted_by=?, delete_reason=?, restored_at=NULL, updated_at=?, synced_at=NULL WHERE id=? AND deleted_at IS NULL`, now, req.ActorID, req.Reason, now, req.TargetID); err != nil {
			return MutationReceipt{}, fmt.Errorf("delete guarded memory: %w", err)
		}
	case MutationOpRestore:
		if !deletedAt.Valid {
			return MutationReceipt{}, fmt.Errorf("%w: id=%d", ErrMemoryNotDeleted, req.TargetID)
		}
		if _, err = tx.Exec(`UPDATE memories SET deleted_at=NULL, deleted_by=NULL, delete_reason=NULL, restored_at=?, updated_at=?, synced_at=NULL WHERE id=? AND deleted_at IS NOT NULL`, now, now, req.TargetID); err != nil {
			return MutationReceipt{}, fmt.Errorf("restore guarded memory: %w", err)
		}
	}
	eventID := uuid.NewString()
	if err := insertMemoryMutation(tx, memoryMutationRecord{EventID: eventID, RequestID: req.RequestID, EntitySyncID: syncID, Project: project, Op: req.Operation, OccurredAt: now, ActorID: req.ActorID, Payload: mutationPayload{Tombstone: &MutationTombstonePayload{DeletedAt: parseTimeOrZero(now), DeletedBy: req.ActorID, Reason: req.Reason}}}); err != nil {
		return MutationReceipt{}, fmt.Errorf("journal guarded mutation: %w", err)
	}
	receipt := MutationReceipt{RequestID: req.RequestID, Operation: string(req.Operation), TargetID: req.TargetID, Project: project, EntitySyncID: syncID, EventID: eventID, LocalStatus: GuardedMutationLocalCommitted, SharedStatus: GuardedMutationSharedPending}
	if _, err := tx.Exec(`INSERT INTO mutation_receipts (request_id, operation, target_id, project, entity_sync_id, event_id, actor_id, reason, local_status, shared_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, receipt.RequestID, receipt.Operation, receipt.TargetID, receipt.Project, receipt.EntitySyncID, receipt.EventID, req.ActorID, req.Reason, receipt.LocalStatus, receipt.SharedStatus); err != nil {
		return MutationReceipt{}, fmt.Errorf("store guarded receipt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MutationReceipt{}, fmt.Errorf("commit guarded mutation: %w", err)
	}
	return receipt, nil
}

// MutationReceipt returns a receipt only when the caller proves the same target
// identity used to create it; this prevents a stale UI response being applied to
// another local row.
func (d *DB) MutationReceipt(requestID string, targetID int64, project, syncID string) (MutationReceipt, error) {
	var receipt MutationReceipt
	var syncedAt sql.NullString
	err := d.sqlDB.QueryRow(`SELECT r.request_id, r.operation, r.target_id, r.project, r.entity_sync_id, r.event_id, r.local_status, r.shared_status, m.synced_at FROM mutation_receipts r LEFT JOIN memory_mutations m ON m.event_id = r.event_id WHERE r.request_id = ?`, strings.TrimSpace(requestID)).Scan(
		&receipt.RequestID, &receipt.Operation, &receipt.TargetID, &receipt.Project, &receipt.EntitySyncID, &receipt.EventID, &receipt.LocalStatus, &receipt.SharedStatus, &syncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MutationReceipt{}, ErrMemoryNotFound
	}
	if err != nil {
		return MutationReceipt{}, fmt.Errorf("load mutation receipt: %w", err)
	}
	if receipt.TargetID != targetID || receipt.Project != strings.TrimSpace(project) || receipt.EntitySyncID != strings.TrimSpace(syncID) {
		return MutationReceipt{}, ErrGuardedMutationIdentityMismatch
	}
	if syncedAt.Valid && syncedAt.String != "" && receipt.SharedStatus == GuardedMutationSharedPending {
		receipt.SharedStatus = "completed"
	}
	return receipt, nil
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
