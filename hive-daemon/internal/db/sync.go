package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

const maxSyncLastErrorRunes = 500

func canonicalSyncStateProject(project string) string {
	if project == "__auth__" {
		return project
	}
	return projectidentity.Canonical(project).String()
}

type SyncHealth struct {
	Project             string    `json:"project"`
	LastAttemptAt       time.Time `json:"last_attempt_at"`
	LastSuccessAt       time.Time `json:"last_success_at"`
	LastFailureAt       time.Time `json:"last_failure_at"`
	BackoffUntil        time.Time `json:"backoff_until"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastError           string    `json:"last_error"`

	// LastDrainState/LastDrainReason/LastDrainRemaining (PR 3, task 3.4,
	// hive-sync-batched-drain) persist the most recent Drain outcome for this
	// project — see RecordDrainOutcome. Empty/zero when no Drain call has
	// ever recorded an outcome for this project (fresh sync_state row, or a
	// DB created before this migration).
	LastDrainState     string `json:"last_drain_state"`
	LastDrainReason    string `json:"last_drain_reason"`
	LastDrainRemaining int    `json:"last_drain_remaining"`
}

type MutationOp string

const (
	MutationOpCreate  MutationOp = "create"
	MutationOpUpdate  MutationOp = "update"
	MutationOpDelete  MutationOp = "delete"
	MutationOpRestore MutationOp = "restore"

	// MutationOpReproject moves a memory the server already holds from one
	// project literal to another. It is the only op that changes a row's
	// project, and it carries no content: the daemon is the sole authority on
	// project identity, so when its local identity migration folds a spelling
	// variant it must tell the server which name the row moved from and to.
	MutationOpReproject MutationOp = "reproject"
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
	Reproject     *MutationReprojectPayload `json:"reproject,omitempty"`
}

type MutationCursor struct {
	Sequence int64  `json:"sequence"`
	EventID  string `json:"event_id"`
}

// PullCursor is the keyset pagination cursor for the legacy (row-state) pull
// channels — pulled memories and pulled sessions (PR 2a/2b,
// hive-sync-batched-drain). It mirrors hive-api's model.PullCursor exactly
// (same field names/types/JSON tags) so the wire payload round-trips without
// translation: (synced_at, sync_id) forms a strictly increasing, gap-free
// key when combined with `ORDER BY synced_at ASC, sync_id ASC` on the
// server side. Declared here (not in internal/sync) so GetPullCursor/
// SetPullCursor can persist it directly — internal/sync imports internal/db,
// so the type must live on this side of that dependency edge; internal/sync
// re-exports it as a type alias for callers in that package.
type PullCursor struct {
	SyncedAt time.Time `json:"synced_at"`
	SyncID   string    `json:"sync_id"`
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

// MutationReprojectPayload names both ends of a project move.
//
// FromProject is not redundant with the project the server already stores: the
// server applies the move only to a row that currently holds FromProject, so a
// replay after the row already moved matches nothing instead of dragging some
// other row out of some other project. ToProject duplicates the envelope's
// Project on purpose — the server rejects an envelope whose two disagree.
type MutationReprojectPayload struct {
	FromProject string `json:"from_project"`
	ToProject   string `json:"to_project"`
}

type memoryMutationRecord struct {
	EventID      string
	RequestID    string
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
	Reproject *MutationReprojectPayload `json:"reproject,omitempty"`
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
    (event_id, request_id, entity_type, entity_sync_id, project, op, occurred_at, actor_id, payload_json)
VALUES (?, NULLIF(?, ''), 'memory', ?, ?, ?, ?, ?, ?)`,
		record.EventID, record.RequestID, record.EntitySyncID, record.Project, string(record.Op), record.OccurredAt, record.ActorID, string(payload),
	)
	return err
}

// CountUnsyncedMemories returns the global count of memories that have not yet
// been pushed to the server. Predicate is identical to GetUnsynced("") so the
// two are always consistent.
func (d *DB) CountUnsyncedMemories(ctx context.Context) (int, error) {
	var n int
	err := d.sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM memories
WHERE synced_at IS NULL AND sync_id != '' AND deleted_at IS NULL`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unsynced memories: %w", err)
	}
	return n, nil
}

// CountUnsyncedPrompts returns the global count of prompts (across all projects)
// that have not yet been pushed to the server. Intentionally omits the project
// clause so it counts globally — unlike GetUnsyncedPrompts which guards against
// an empty project with an early nil return.
func (d *DB) CountUnsyncedPrompts(ctx context.Context) (int, error) {
	var n int
	err := d.sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM user_prompts
WHERE synced_at IS NULL AND sync_id != ''`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unsynced prompts: %w", err)
	}
	return n, nil
}

// CountUnsyncedSessions returns the global count of sessions (across all
// projects) that have not yet been pushed to the server. The non-empty
// sync_id predicate matches the behavior of CountUnsyncedMemories and
// CountUnsyncedPrompts: sessions without a sync_id were never queued for sync
// and must not be counted as pending.
func (d *DB) CountUnsyncedSessions(ctx context.Context) (int, error) {
	var n int
	err := d.sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM sessions
WHERE synced_at IS NULL AND sync_id != ''`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unsynced sessions: %w", err)
	}
	return n, nil
}

// GetUnsynced devuelve todas las memorias que aún no se han enviado al servidor
// (synced_at IS NULL). Son las que hay que incluir en el próximo push.
func (d *DB) GetUnsynced(project string) ([]*models.Memory, error) {
	if project != "" {
		blocked, err := d.IsProjectBlocked(context.Background(), project)
		if err != nil {
			return nil, fmt.Errorf("get unsynced block check: %w", err)
		}
		if blocked {
			return []*models.Memory{}, nil
		}
	}
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

// GetUnsyncedPage returns at most `limit` memories that have not yet been
// pushed to the server (synced_at IS NULL), ordered by created_at ASC with
// id ASC as a secondary tiebreaker. This is the paged counterpart to
// GetUnsynced (PR 1b-iii, hive-sync-batched-drain): syncBatchStep uses this
// instead of the unpaged getter so a single push batch never exceeds
// syncPageSize, while the ORDER BY keeps paging stable across repeated calls
// as earlier rows get marked synced. created_at has only second-level
// granularity, and this table is also served by
// idx_memories_project_active (project, created_at DESC), so without the
// secondary id ASC key a created_at tie can be returned in descending id
// order instead of oldest-first, letting rows straddling a page boundary be
// skipped or duplicated as the synced_at IS NULL filter shifts between
// fetches. GetUnsynced itself is left untouched for any other existing
// callers.
func (d *DB) GetUnsyncedPage(project string, limit int) ([]*models.Memory, error) {
	if project != "" {
		blocked, err := d.IsProjectBlocked(context.Background(), project)
		if err != nil {
			return nil, fmt.Errorf("get unsynced page block check: %w", err)
		}
		if blocked {
			return []*models.Memory{}, nil
		}
	}
	if limit <= 0 {
		limit = 100
	}
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
	q += " ORDER BY created_at ASC, id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := d.sqlDB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get unsynced page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanSyncRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced page row: %w", err)
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

// MarkMemoriesSyncedBySyncID marks legacy memory rows as synced by
// correlating their sync_id, mirroring MarkMutationsSynced. This is used to
// ack legacy memories once the server has durably confirmed the
// corresponding mutation in mutation-sync-v2 mode, so GetUnsynced stops
// re-emitting them.
func (d *DB) MarkMemoriesSyncedBySyncID(syncIDs []string, at time.Time) error {
	if len(syncIDs) == 0 {
		return nil
	}
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin mark memories synced: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	formatted := at.UTC().Format("2006-01-02 15:04:05")
	for _, syncID := range syncIDs {
		result, err := tx.Exec(`UPDATE memories SET synced_at = ? WHERE sync_id = ?`, formatted, syncID)
		if err != nil {
			return fmt.Errorf("mark memory synced %s: %w", syncID, err)
		}
		if n, _ := result.RowsAffected(); n == 0 {
			logger.Log.Printf("warn: MarkMemoriesSyncedBySyncID: no row found for sync_id %s", syncID)
		}
	}
	return tx.Commit()
}

func (d *DB) GetPendingMutations(project string, limit int) ([]MutationEnvelope, error) {
	if project != "" {
		blocked, err := d.IsProjectBlocked(context.Background(), project)
		if err != nil {
			return nil, fmt.Errorf("get pending mutations block check: %w", err)
		}
		if blocked {
			return []MutationEnvelope{}, nil
		}
	}
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

// MarkMutationsRejected stops retrying terminal Hive API rejections without
// claiming that the guarded local change was shared successfully.
func (d *DB) MarkMutationsRejected(eventIDs []string, at time.Time) error {
	if len(eventIDs) == 0 {
		return nil
	}
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin mark mutations rejected: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	formatted := at.UTC().Format("2006-01-02 15:04:05")
	for _, eventID := range eventIDs {
		if _, err := tx.Exec(`UPDATE memory_mutations SET synced_at = ? WHERE event_id = ?`, formatted, eventID); err != nil {
			return fmt.Errorf("mark rejected mutation %s: %w", eventID, err)
		}
		if _, err := tx.Exec(`UPDATE mutation_receipts SET shared_status = 'failed' WHERE event_id = ? AND shared_status = 'pending'`, eventID); err != nil {
			return fmt.Errorf("mark rejected mutation receipt %s: %w", eventID, err)
		}
	}
	return tx.Commit()
}

// MarkMutationReceiptsLegacyUnsupported records that a guarded mutation could
// not be propagated by a legacy row-state peer. The local mutation remains
// pending and retryable; this status must never claim shared completion.
func (d *DB) MarkMutationReceiptsLegacyUnsupported(eventIDs []string) error {
	for _, eventID := range eventIDs {
		if _, err := d.sqlDB.Exec(`UPDATE mutation_receipts SET shared_status = 'legacy_unsupported' WHERE event_id = ? AND shared_status = 'pending'`, eventID); err != nil {
			return fmt.Errorf("mark mutation receipt legacy unsupported %s: %w", eventID, err)
		}
	}
	return nil
}

// MarkMutationsAndMemoriesSynced acks the given mutation journal event_ids
// AND the correlated legacy memories.sync_id rows in a single transaction.
// This is the atomic counterpart to calling MarkMutationsSynced followed by
// MarkMemoriesSyncedBySyncID separately: if either half fails, the whole
// transaction rolls back, so a pending mutation is never left "confirmed"
// while its correlated legacy row stays unsynced forever. On the next Sync
// call, GetPendingMutations will re-derive and retry both halves together.
func (d *DB) MarkMutationsAndMemoriesSynced(eventIDs []string, syncIDs []string, at time.Time) error {
	if len(eventIDs) == 0 && len(syncIDs) == 0 {
		return nil
	}
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin mark mutations and memories synced: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	formatted := at.UTC().Format("2006-01-02 15:04:05")

	for _, eventID := range eventIDs {
		if _, err := tx.Exec(`UPDATE memory_mutations SET synced_at = ? WHERE event_id = ?`, formatted, eventID); err != nil {
			return fmt.Errorf("mark mutation synced %s: %w", eventID, err)
		}
	}

	for _, syncID := range syncIDs {
		result, err := tx.Exec(`UPDATE memories SET synced_at = ? WHERE sync_id = ?`, formatted, syncID)
		if err != nil {
			return fmt.Errorf("mark memory synced %s: %w", syncID, err)
		}
		if n, _ := result.RowsAffected(); n == 0 {
			logger.Log.Printf("warn: MarkMutationsAndMemoriesSynced: no memory row found for sync_id %s", syncID)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mark mutations and memories synced: %w", err)
	}
	return nil
}

func (d *DB) GetMutationCursor(consumer, project string) (MutationCursor, error) {
	project = projectidentity.Canonical(project).String()
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
	project = projectidentity.Canonical(project).String()
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

// pullCursorTimeLayout stores PullCursor.SyncedAt with full (nanosecond)
// precision, unlike formatSQLiteTime's second-level truncation used
// elsewhere in this file. PullCursor.SyncedAt is an opaque resume token
// echoed back to hive-api's keyset pagination verbatim (see PullCursor's
// doc) — it is never queried or ordered by SQLite itself, only stored and
// round-tripped, so truncating it to seconds would risk resuming a page one
// second early/late and re-fetching or skipping rows that share that second.
const pullCursorTimeLayout = time.RFC3339Nano

// GetPullCursor returns the persisted bounded-pull resume position for
// (consumer, project, channel) — PR 2a/2b, hive-sync-batched-drain task 2.7.
// channel is "memories" or "sessions": the two legacy pull channels
// paginate independently, so each gets its own row. A missing cursor
// (never synced, or first bounded pull for this project) returns the zero
// value, matching GetMutationCursor's contract.
func (d *DB) GetPullCursor(consumer, project, channel string) (PullCursor, error) {
	project = projectidentity.Canonical(project).String()
	var cursor PullCursor
	var syncedAt string
	err := d.sqlDB.QueryRow(`
SELECT synced_at, sync_id
FROM pull_cursors
WHERE consumer = ? AND project = ? AND channel = ?`, consumer, project, channel).Scan(&syncedAt, &cursor.SyncID)
	if errors.Is(err, sql.ErrNoRows) {
		return PullCursor{}, nil
	}
	if err != nil {
		return PullCursor{}, fmt.Errorf("get pull cursor: %w", err)
	}
	if syncedAt != "" {
		parsed, parseErr := time.Parse(pullCursorTimeLayout, syncedAt)
		if parseErr != nil {
			return PullCursor{}, fmt.Errorf("get pull cursor: parse synced_at: %w", parseErr)
		}
		cursor.SyncedAt = parsed
	}
	return cursor, nil
}

// SetPullCursor persists the bounded-pull resume position for (consumer,
// project, channel), upserting on repeated calls for the same key — PR
// 2a/2b, hive-sync-batched-drain task 2.7. Mirrors SetMutationCursor's
// ON CONFLICT DO UPDATE shape, keyed one level deeper to keep the memories
// and sessions pull channels independent for the same project.
func (d *DB) SetPullCursor(consumer, project, channel string, cursor PullCursor, at time.Time) error {
	project = projectidentity.Canonical(project).String()
	_, err := d.sqlDB.Exec(`
INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(consumer, project, channel) DO UPDATE SET
    synced_at = excluded.synced_at,
    sync_id = excluded.sync_id,
    updated_at = excluded.updated_at`,
		consumer, project, channel, cursor.SyncedAt.UTC().Format(pullCursorTimeLayout), cursor.SyncID, formatSQLiteTime(at),
	)
	if err != nil {
		return fmt.Errorf("set pull cursor: %w", err)
	}
	return nil
}

// ClearPullCursor deletes the bounded-pull resume position for one channel.
// Deleting a missing row is a successful no-op.
func (d *DB) ClearPullCursor(consumer, project, channel string) error {
	project = projectidentity.Canonical(project).String()
	_, err := d.sqlDB.Exec(`
DELETE FROM pull_cursors
WHERE consumer = ? AND project = ? AND channel = ?`, consumer, project, channel)
	if err != nil {
		return fmt.Errorf("clear pull cursor: %w", err)
	}
	return nil
}

func (d *DB) ApplyRemoteMutation(event MutationEnvelope) (bool, error) {
	if event.EventID == "" {
		return false, fmt.Errorf("event_id is required")
	}
	// EntityType == "" defaults to "memory" here. This convention is mirrored
	// by confirmedMemorySyncIDs in internal/sync/syncer.go, which treats an
	// empty EntityType the same way when correlating confirmed mutations
	// back to legacy memories.sync_id rows. Today EntityType is only ever ""
	// or "memory", so both sites agree — if a second entity type is ever
	// introduced, update both call sites together.
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

	// Resolve alias inside the transaction so the lookup is atomic with the
	// subsequent writes. If event.Project is a known alias source, rewrite to
	// the canonical target project name.
	var aliasTarget string
	aliasErr := tx.QueryRow(
		`SELECT target_project FROM project_aliases WHERE source_project = ? LIMIT 1`, event.Project,
	).Scan(&aliasTarget)
	if aliasErr != nil && !errors.Is(aliasErr, sql.ErrNoRows) {
		return false, fmt.Errorf("ApplyRemoteMutation resolve alias: %w", aliasErr)
	}
	if aliasErr == nil {
		event.Project = aliasTarget
	}
	rawProject := event.Project
	canonicalProject, err := registerProjectIdentity(context.Background(), tx, rawProject)
	if err != nil {
		return false, err
	}
	event.Project = canonicalProject
	if event.Memory != nil && event.Memory.SessionID == "manual-save-"+rawProject {
		event.Memory.SessionID = "manual-save-" + event.Project
	}
	if err := ensureProjectWritableInTx(tx, event.Project); err != nil {
		return false, err
	}

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

	// Resolve alias using a direct SQL query so this function does not depend on
	// context.Background() via d.ResolveAlias. If mem.Project is a known alias
	// source, rewrite to the canonical target project name.
	// Use a local variable to avoid mutating the caller's *models.Memory.
	project := mem.Project
	var aliasTarget string
	aliasErr := d.sqlDB.QueryRow(
		`SELECT target_project FROM project_aliases WHERE source_project = ? LIMIT 1`, project,
	).Scan(&aliasTarget)
	if aliasErr != nil && !errors.Is(aliasErr, sql.ErrNoRows) {
		return fmt.Errorf("SaveFromRemote resolve alias: %w", aliasErr)
	}
	if aliasErr == nil {
		project = aliasTarget
		// Note: session_id is kept as-is even when the project is remapped via alias.
		// The FK on memories.session_id checks ID existence only, not project match.
		// All memory reads filter by memories.project directly, so the mismatch is harmless.
		// Remapping sessions on sync receive would require creating artificial sessions
		// under the target project, which adds noise to KnownProjects and session history.
	}
	rawProject := project
	canonicalProject, err := registerProjectIdentity(context.Background(), d.sqlDB, rawProject)
	if err != nil {
		return err
	}
	project = canonicalProject
	if err := d.ensureProjectWritable(context.Background(), project); err != nil {
		return err
	}

	// R2-CRIT-3: resolve session_id BEFORE the INSERT. memories.session_id is NOT NULL,
	// and `INSERT OR IGNORE` would silently drop the row on any constraint failure.
	sessionID := mem.SessionID
	if sessionID == "manual-save-"+rawProject {
		sessionID = "manual-save-" + project
	}
	if sessionID == "" {
		resolved, err := d.EnsureManualSaveSession(project)
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
		mem.SyncID, project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, string(tagsJSON), string(filesJSON),
		mem.CreatedBy, createdAt, updatedAt, now, sessionID,
	)
	return err
}

// GetLastSync devuelve el timestamp del último sync exitoso para un proyecto.
func (d *DB) GetLastSync(project string) (time.Time, error) {
	project = canonicalSyncStateProject(project)
	var ts sql.NullString
	err := d.sqlDB.QueryRow(
		`SELECT last_sync_at FROM sync_state WHERE project = ?`, project,
	).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) || !ts.Valid {
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
	project = canonicalSyncStateProject(project)
	var (
		health                                SyncHealth
		lastAttempt, lastSuccess, lastFailure sql.NullString
		backoffUntil                          sql.NullString
		lastError                             sql.NullString
		consecutiveFailures                   sql.NullInt64
		lastDrainState, lastDrainReason       sql.NullString
		lastDrainRemaining                    sql.NullInt64
	)

	health.Project = project
	err := d.sqlDB.QueryRow(`
SELECT last_attempt_at, last_success_at, last_failure_at, consecutive_failures, backoff_until, last_error,
       last_drain_state, last_drain_reason, last_drain_remaining
FROM sync_state WHERE project = ?`, project).Scan(
		&lastAttempt,
		&lastSuccess,
		&lastFailure,
		&consecutiveFailures,
		&backoffUntil,
		&lastError,
		&lastDrainState,
		&lastDrainReason,
		&lastDrainRemaining,
	)
	if errors.Is(err, sql.ErrNoRows) {
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
	if lastDrainState.Valid {
		health.LastDrainState = lastDrainState.String
	}
	if lastDrainReason.Valid {
		health.LastDrainReason = lastDrainReason.String
	}
	if lastDrainRemaining.Valid {
		health.LastDrainRemaining = int(lastDrainRemaining.Int64)
	}

	return health, nil
}

func (d *DB) ListGovernanceSyncHealth(ctx context.Context) ([]SyncHealth, error) {
	rows, err := d.sqlDB.QueryContext(ctx, `
SELECT project, last_attempt_at, last_success_at, last_failure_at, consecutive_failures, backoff_until, last_error,
       last_drain_state, last_drain_reason, last_drain_remaining
FROM sync_state
WHERE project != '__auth__'
ORDER BY project`)
	if err != nil {
		return nil, fmt.Errorf("list governance sync health: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var health []SyncHealth
	for rows.Next() {
		item, err := scanSyncHealth(rows)
		if err != nil {
			return nil, err
		}
		health = append(health, item)
	}
	return health, rows.Err()
}

// RecordDrainOutcome persists the most recently recorded Drain outcome for
// project (PR 3, task 3.4, hive-sync-batched-drain). Unlike RecordSyncSuccess/
// RecordSyncFailure this always OVERWRITES the previous drain fields — a
// drain outcome is a point-in-time snapshot ("what happened on the last
// Drain call"), not an accumulating counter, so the newest call always wins.
// reason is passed through as-is (empty string for DrainReasonNone) and
// stored as NULL only when the row does not exist yet — an explicit empty
// string is stored as empty string, not NULL, so GetSyncHealth's Valid check
// still reports the row as present.
func (d *DB) RecordDrainOutcome(project, state, reason string, remaining int) error {
	if _, err := d.sqlDB.Exec(`
INSERT OR IGNORE INTO sync_state (project, consecutive_failures, last_error)
VALUES (?, 0, '')`, project); err != nil {
		return err
	}

	_, err := d.sqlDB.Exec(`
UPDATE sync_state SET
	last_drain_state = ?,
	last_drain_reason = ?,
	last_drain_remaining = ?
WHERE project = ?`,
		state, reason, remaining, project,
	)
	return err
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

// ClearJWT removes only the cached daemon auth session.
func (d *DB) ClearJWT() error {
	_, err := d.sqlDB.Exec(`UPDATE sync_state SET jwt_token = NULL, jwt_expires_at = NULL WHERE project = '__auth__'`)
	return err
}

// --- helpers privados ---

type syncScanner interface {
	Scan(...any) error
}

func scanSyncHealth(s syncScanner) (SyncHealth, error) {
	var (
		health                                SyncHealth
		lastAttempt, lastSuccess, lastFailure sql.NullString
		backoffUntil                          sql.NullString
		lastError                             sql.NullString
		consecutiveFailures                   sql.NullInt64
		lastDrainState, lastDrainReason       sql.NullString
		lastDrainRemaining                    sql.NullInt64
	)
	if err := s.Scan(
		&health.Project, &lastAttempt, &lastSuccess, &lastFailure, &consecutiveFailures, &backoffUntil, &lastError,
		&lastDrainState, &lastDrainReason, &lastDrainRemaining,
	); err != nil {
		return SyncHealth{}, fmt.Errorf("scan sync health: %w", err)
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
	if lastDrainState.Valid {
		health.LastDrainState = lastDrainState.String
	}
	if lastDrainReason.Valid {
		health.LastDrainReason = lastDrainReason.String
	}
	if lastDrainRemaining.Valid {
		health.LastDrainRemaining = int(lastDrainRemaining.Int64)
	}
	return health, nil
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
	var payload mutationPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err == nil {
		mutation.Memory = payload.Memory
		mutation.Tombstone = payload.Tombstone
		mutation.Reproject = payload.Reproject
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
	project = canonicalSyncStateProject(project)
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

	// Primary rule: cut at "): " — this separator appears after the status code
	// or parenthesised context and avoids false cuts on HTTPS URLs (which contain
	// "://" but not "): ").
	if head, _, found := strings.Cut(trimmed, "): "); found {
		return strings.TrimSpace(head) + ")"
	}

	// Fallback: if the message contains a newline and the content after the first
	// newline starts with an HTML/JSON body indicator, truncate at the newline.
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		remainder := trimmed[idx+1:]
		if len(remainder) > 0 && (remainder[0] == '<' || remainder[0] == '{' || remainder[0] == '[') {
			return strings.TrimSpace(trimmed[:idx])
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
