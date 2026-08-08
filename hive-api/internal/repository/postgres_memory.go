package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/projectkey"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresMemoryRepository es la implementación de MemoryRepository sobre PostgreSQL.
type postgresMemoryRepository struct {
	db   pgxQuerier
	pool *pgxpool.Pool
}

// NewPostgresMemoryRepository crea la implementación real de MemoryRepository.
func NewPostgresMemoryRepository(pool *pgxpool.Pool) MemoryRepository {
	return &postgresMemoryRepository{db: pool, pool: pool}
}

func newPostgresMemoryRepositoryWithQuerier(db pgxQuerier) MemoryRepository {
	return &postgresMemoryRepository{db: db}
}

func (r *postgresMemoryRepository) ProjectExists(ctx context.Context, canonicalProjectKey string) (bool, error) {
	canonicalProjectKey = projectkey.Canonicalize(canonicalProjectKey)
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_identities WHERE project_key = $1)`, canonicalProjectKey).Scan(&exists)
	return exists, wrapPgError(err, "check project identity")
}

// Create inserta una nueva memoria y devuelve el registro completo (con ID del servidor).
func (r *postgresMemoryRepository) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	if err := r.rejectBlockedProject(ctx, mem.Project); err != nil {
		return nil, err
	}
	const q = `
		INSERT INTO memories
			(sync_id, project, topic_key, category, title, content,
			 tags, files_affected, created_by, created_at, updated_at,
			 origin, session_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, synced_at`

	tagsJSON, err := json.Marshal(orEmptySlice(mem.Tags))
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}
	filesJSON, err := json.Marshal(orEmptySlice(mem.FilesAffected))
	if err != nil {
		return nil, fmt.Errorf("marshal files_affected: %w", err)
	}

	row := r.db.QueryRow(ctx, q,
		mem.SyncID, mem.Project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, tagsJSON, filesJSON,
		mem.CreatedBy, mem.CreatedAt, mem.UpdatedAt,
		mem.Origin, mem.SessionID,
	)

	err = row.Scan(&mem.ID, &mem.SyncedAt)
	if err != nil {
		return nil, wrapPgError(err, "Create memory")
	}
	return mem, nil
}

// GetByID devuelve una memoria por su UUID de servidor.
func (r *postgresMemoryRepository) GetByID(ctx context.Context, id string) (*model.Memory, error) {
	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                  tags, files_affected, created_by, created_at, updated_at,
	                  origin, synced_at, session_id,
	                  deleted_at, deleted_by, delete_reason, restored_at
		           FROM memories
		           WHERE id = $1 AND deleted_at IS NULL
		             AND %s`, unblockedProjectPredicate("memories.project"))
	return r.scanMemory(ctx, q, id)
}

// GetBySyncID devuelve una memoria por su sync_id (generado por el daemon).
// Devuelve nil sin error si no existe — es el único método que hace esto.
func (r *postgresMemoryRepository) GetBySyncID(ctx context.Context, syncID string) (*model.Memory, error) {
	const q = `SELECT id, sync_id, project, topic_key, category, title, content,
	                  tags, files_affected, created_by, created_at, updated_at,
	                  origin, synced_at, session_id,
	                  deleted_at, deleted_by, delete_reason, restored_at
	           FROM memories WHERE sync_id = $1`
	mem, err := r.scanMemory(ctx, q, syncID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil // nil + nil = "no existe", es válido para este método
	}
	return mem, err
}

// List devuelve memorias paginadas según el filtro.
func (r *postgresMemoryRepository) List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 20
	}

	args := []interface{}{limit, filter.Offset}
	where := ""
	argIdx := 3

	where, args, _ = appendMemoryFilterPredicates(where, args, argIdx, filter)

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, session_id,
	                         deleted_at, deleted_by, delete_reason, restored_at
	                  FROM memories WHERE 1=1 %s
	                  ORDER BY created_at DESC, synced_at DESC, id DESC LIMIT $1 OFFSET $2`, where)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "List memories")
	}
	defer rows.Close()

	return r.scanMemoryRows(rows)
}

// Count devuelve el total de memorias que coinciden con el filtro.
func (r *postgresMemoryRepository) Count(ctx context.Context, filter model.MemoryFilter) (int64, error) {
	args := []interface{}{}
	where := ""
	argIdx := 1

	where, args, _ = appendMemoryFilterPredicates(where, args, argIdx, filter)

	q := fmt.Sprintf(`SELECT COUNT(*) FROM memories WHERE 1=1 %s`, where)
	var count int64
	err := r.db.QueryRow(ctx, q, args...).Scan(&count)
	return count, wrapPgError(err, "Count memories")
}

// Search realiza búsqueda FTS con ranking BM25 usando el índice tsvector.
func (r *postgresMemoryRepository) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 20
	}

	args := []interface{}{query, limit, filter.Offset}
	where := ""
	argIdx := 4

	where, args, _ = appendMemoryFilterPredicates(where, args, argIdx, filter)

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, session_id,
	                         deleted_at, deleted_by, delete_reason, restored_at
	                  FROM memories
	                  WHERE search_vector @@ plainto_tsquery('simple', $1) %s
	                  ORDER BY ts_rank(search_vector, plainto_tsquery('simple', $1)) DESC,
	                           created_at DESC, synced_at DESC, id DESC
	                  LIMIT $2 OFFSET $3`, where)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "Search memories")
	}
	defer rows.Close()

	return r.scanMemoryRows(rows)
}

func (r *postgresMemoryRepository) CountSearch(ctx context.Context, query string, filter model.MemoryFilter) (int64, error) {
	args := []interface{}{query}
	where := ""
	argIdx := 2

	where, args, _ = appendMemoryFilterPredicates(where, args, argIdx, filter)

	q := fmt.Sprintf(`SELECT COUNT(*) FROM memories WHERE search_vector @@ plainto_tsquery('simple', $1) %s`, where)
	var count int64
	err := r.db.QueryRow(ctx, q, args...).Scan(&count)
	return count, wrapPgError(err, "CountSearch memories")
}

// Upsert implementa el algoritmo de 4 ramas del protocolo de sync.
// Ver la documentación en la interfaz MemoryRepository para los detalles de cada rama.
func (r *postgresMemoryRepository) Upsert(ctx context.Context, mem *model.Memory) (*model.Memory, bool, error) {
	if err := r.rejectBlockedProject(ctx, mem.Project); err != nil {
		return nil, false, err
	}
	// Buscamos si ya existe una memoria con este sync_id
	existing, err := r.GetBySyncID(ctx, mem.SyncID)
	if err != nil {
		return nil, false, err
	}

	// Rama 1: sync_id NO existe → INSERT
	if existing == nil {
		created, err := r.Create(ctx, mem)
		if err != nil {
			return nil, false, err
		}
		return created, true, nil
	}

	// Rama 2: sync_id existe + topic_key IS NULL → SKIP (memoria inmutable)
	if existing.TopicKey == nil {
		return existing, false, nil
	}

	// Rama 3: sync_id existe + incoming.UpdatedAt <= existing.UpdatedAt → SKIP (servidor gana)
	if !mem.UpdatedAt.After(existing.UpdatedAt) {
		return nil, false, nil
	}

	// Rama 4: sync_id existe + incoming.UpdatedAt > existing.UpdatedAt → UPDATE (cliente gana)
	updated, err := r.update(ctx, existing.ID, mem)
	if err != nil {
		return nil, false, err
	}
	return updated, false, nil
}

// update aplica los cambios del cliente sobre una memoria existente.
func (r *postgresMemoryRepository) update(ctx context.Context, id string, mem *model.Memory) (*model.Memory, error) {
	const q = `UPDATE memories
	           SET topic_key=$1, category=$2, title=$3, content=$4,
	               tags=$5, files_affected=$6, updated_at=$7,
	               session_id=$8, synced_at=now()
	           WHERE id=$9
	           RETURNING id, sync_id, project, topic_key, category, title, content,
	                     tags, files_affected, created_by, created_at, updated_at,
	                     origin, synced_at, session_id,
	                     deleted_at, deleted_by, delete_reason, restored_at`

	tagsJSON, _ := json.Marshal(orEmptySlice(mem.Tags))
	filesJSON, _ := json.Marshal(orEmptySlice(mem.FilesAffected))

	row := r.db.QueryRow(ctx, q,
		mem.TopicKey, mem.Category, mem.Title, mem.Content,
		tagsJSON, filesJSON, mem.UpdatedAt,
		mem.SessionID, id,
	)

	return scanMemoryRow(row)
}

// PullSince devuelve una página de memorias del proyecto actualizadas después de
// 'since', ordenadas por (synced_at ASC, sync_id ASC) para paginación por keyset.
//
// cursor (si no es cursor.IsZero()) reanuda estrictamente después de
// (cursor.SyncedAt, cursor.SyncID) usando la comparación de tupla de Postgres
// `(synced_at, sync_id) > ($ts, $id)`, que respeta el orden compuesto sin el
// problema de "empates en synced_at" que tendría comparar solo por timestamp.
//
// limit <= 0 (model.UnboundedPullLimit) significa "sin LIMIT" — barrido legado
// completo en una sola página, hasMore siempre false. Este es el contrato de
// backward-compat de PR 2a: un cliente que nunca optó a paginación (pull_limit
// ausente/0) debe obtener EXACTAMENTE el comportamiento pre-2a — nunca le
// recortamos filas que no sabe cómo reanudar.
//
// limit > 0 pide limit+1 filas: si vuelven limit+1, hay más páginas — se
// recorta a limit antes de devolver.
func (r *postgresMemoryRepository) PullSince(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, cursor model.PullCursor, limit int) ([]*model.Memory, bool, error) {
	args := []interface{}{projectkey.Canonicalize(project)}
	where := resolvedProjectKeyExpr("memories.project") + " = $1 AND deleted_at IS NULL AND " + unblockedProjectPredicate("memories.project")
	argIdx := 2

	if !since.IsZero() {
		// >= para no perder memorias con synced_at exactamente igual a 'since'
		where += fmt.Sprintf(" AND synced_at >= $%d", argIdx)
		args = append(args, since)
		argIdx++
	}

	if !cursor.IsZero() {
		where += fmt.Sprintf(" AND (synced_at, sync_id) > ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cursor.SyncedAt, cursor.SyncID)
		argIdx += 2
	}

	if len(excludeSyncIDs) > 0 {
		where += fmt.Sprintf(" AND sync_id != ALL($%d)", argIdx)
		args = append(args, excludeSyncIDs)
		argIdx++
	}

	unbounded := limit <= 0

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, session_id,
	                         deleted_at, deleted_by, delete_reason, restored_at
	                  FROM memories WHERE %s ORDER BY synced_at ASC, sync_id ASC`, where)
	if !unbounded {
		fetchLimit := limit + 1
		q += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, fetchLimit)
	}

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, false, wrapPgError(err, "PullSince")
	}
	defer rows.Close()

	memories, err := r.scanMemoryRows(rows)
	if err != nil {
		return nil, false, err
	}

	if unbounded {
		return memories, false, nil
	}

	hasMore := len(memories) > limit
	if hasMore {
		memories = memories[:limit]
	}

	return memories, hasMore, nil
}

func (r *postgresMemoryRepository) ApplyMemoryMutation(ctx context.Context, mutation model.MutationEnvelope) (*model.MutationApplyResult, error) {
	if mutation.EventID == "" || mutation.EntityType != model.MutationEntityMemory || mutation.EntitySyncID == "" || mutation.Project == "" {
		return nil, fmt.Errorf("invalid memory mutation envelope")
	}
	if r.pool == nil {
		return r.applyMemoryMutationInTx(ctx, r.db, mutation)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, wrapPgError(err, "begin memory mutation")
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	return r.applyMemoryMutationInTx(ctx, tx, mutation)
}

func (r *postgresMemoryRepository) applyMemoryMutationInTx(ctx context.Context, tx mutationTx, mutation model.MutationEnvelope) (*model.MutationApplyResult, error) {
	var existingSequence int64
	err := tx.QueryRow(ctx, `SELECT sequence FROM memory_mutations WHERE event_id = $1`, mutation.EventID).Scan(&existingSequence)
	if err == nil {
		return &model.MutationApplyResult{EventID: mutation.EventID, Op: mutation.Op, Duplicate: true, Applied: false, Sequence: existingSequence}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapPgError(err, "check mutation event")
	}
	if mutation.Memory != nil && mutation.Memory.Project != "" && mutation.Memory.Project != mutation.Project {
		return rejectedMutationResult(mutation, "project mismatch between mutation envelope and memory payload"), nil
	}

	existing, err := memoryBySyncIDForUpdate(ctx, tx, mutation.EntitySyncID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Project != mutation.Project {
		return rejectedMutationResult(mutation, "project mismatch for existing memory row"), nil
	}

	var changed bool
	switch mutation.Op {
	case model.MutationOpCreate:
		changed, err = r.applyCreateMutation(ctx, tx, mutation)
	case model.MutationOpUpdate:
		changed, err = r.applyUpdateMutation(ctx, tx, mutation)
	case model.MutationOpDelete:
		changed, err = r.applyDeleteMutation(ctx, tx, mutation)
	case model.MutationOpRestore:
		changed, err = r.applyRestoreMutation(ctx, tx, mutation)
	default:
		return nil, fmt.Errorf("unsupported memory mutation op %q", mutation.Op)
	}
	if err != nil {
		return nil, err
	}
	if !changed {
		return &model.MutationApplyResult{EventID: mutation.EventID, Op: mutation.Op, Applied: false}, nil
	}

	sequence, err := insertMemoryMutation(ctx, tx, mutation)
	if err != nil {
		return nil, err
	}
	if r.pool != nil {
		if committer, ok := tx.(interface{ Commit(context.Context) error }); ok {
			if err := committer.Commit(ctx); err != nil {
				return nil, wrapPgError(err, "commit memory mutation")
			}
		}
	}

	return &model.MutationApplyResult{EventID: mutation.EventID, Op: mutation.Op, Applied: true, Sequence: sequence}, nil
}

func rejectedMutationResult(mutation model.MutationEnvelope, reason string) *model.MutationApplyResult {
	return &model.MutationApplyResult{EventID: mutation.EventID, Op: mutation.Op, Applied: false, Rejected: true, Reason: reason}
}

func (r *postgresMemoryRepository) ListMemoryMutations(ctx context.Context, project string, cursor model.MutationCursor, limit int) (*model.MutationBatch, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, `
		SELECT sequence, event_id::text, entity_type, entity_sync_id::text, project, op,
		       occurred_at, COALESCE(actor_id, ''), base_updated_at, memory, tombstone
		FROM memory_mutations
		WHERE project = $1
		  AND (sequence > $2 OR (sequence = $2 AND event_id::text > $3))
		ORDER BY sequence ASC, event_id ASC
		LIMIT $4`, project, cursor.Sequence, cursor.EventID, limit)
	if err != nil {
		return nil, wrapPgError(err, "list memory mutations")
	}
	defer rows.Close()

	batch := &model.MutationBatch{Events: []model.MutationEnvelope{}, Next: cursor}
	for rows.Next() {
		var event model.MutationEnvelope
		var op string
		var memoryRaw, tombstoneRaw []byte
		err := rows.Scan(&event.Sequence, &event.EventID, &event.EntityType, &event.EntitySyncID,
			&event.Project, &op, &event.OccurredAt, &event.ActorID, &event.BaseUpdatedAt, &memoryRaw, &tombstoneRaw)
		if err != nil {
			return nil, wrapPgError(err, "scan memory mutation")
		}
		event.Op = model.MutationOp(op)
		if len(memoryRaw) > 0 {
			_ = json.Unmarshal(memoryRaw, &event.Memory)
		}
		if len(tombstoneRaw) > 0 {
			_ = json.Unmarshal(tombstoneRaw, &event.Tombstone)
		}
		batch.Events = append(batch.Events, event)
		batch.Next = model.MutationCursor{Sequence: event.Sequence, EventID: event.EventID}
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate memory mutations")
	}

	return batch, nil
}

func (r *postgresMemoryRepository) ListActivityFeed(ctx context.Context, query model.ActivityFeedRepositoryQuery) ([]model.ActivityJournalRow, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}

	args := []interface{}{}
	cursorClause := ""
	if query.Cursor != nil {
		args = append(args, query.Cursor.OccurredAt, query.Cursor.Sequence, query.Cursor.EventID)
		cursorClause = `AND (mm.occurred_at, mm.sequence, mm.event_id) < ($1, $2, $3::uuid)`
	}
	args = append(args, limit)
	limitPlaceholder := len(args)

	q := fmt.Sprintf(`
		SELECT mm.event_id::text, mm.entity_type, mm.entity_sync_id::text, mm.project, mm.op,
		       mm.sequence, mm.occurred_at, COALESCE(mm.actor_id, ''), mm.memory, mm.tombstone,
		       COALESCE(mem.project, ''), COALESCE(mem.category::text, ''), COALESCE(mem.title, ''),
		       COALESCE(mem.content, ''), COALESCE(mem.created_by, '')
			FROM memory_mutations mm
			LEFT JOIN memories mem ON mem.sync_id = mm.entity_sync_id AND mem.project = mm.project
			WHERE mm.entity_type = 'memory'
			  AND mm.op IN ('create', 'update', 'delete')
			  AND %s
			  %s
		ORDER BY mm.occurred_at DESC, mm.sequence DESC, mm.event_id DESC
		LIMIT $%d`, unblockedProjectPredicate("mm.project"), cursorClause, limitPlaceholder)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "list activity feed")
	}
	defer rows.Close()

	feed := []model.ActivityJournalRow{}
	for rows.Next() {
		row, err := scanActivityJournalRow(rows)
		if err != nil {
			return nil, err
		}
		feed = append(feed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate activity feed")
	}
	return feed, nil
}

func scanActivityJournalRow(rows pgx.Rows) (model.ActivityJournalRow, error) {
	var row model.ActivityJournalRow
	var op string
	var memoryRaw, tombstoneRaw []byte
	var memoryProject, memoryCategory, memoryTitle, memoryContent, memoryCreatedBy string

	err := rows.Scan(&row.EventID, &row.EntityType, &row.EntitySyncID, &row.Project, &op,
		&row.Sequence, &row.OccurredAt, &row.ActorID, &memoryRaw, &tombstoneRaw,
		&memoryProject, &memoryCategory, &memoryTitle, &memoryContent, &memoryCreatedBy)
	if err != nil {
		return model.ActivityJournalRow{}, wrapPgError(err, "scan activity feed row")
	}
	row.Op = model.MutationOp(op)

	if len(memoryRaw) > 0 && string(memoryRaw) != "null" {
		var payload model.MemoryPayload
		if err := json.Unmarshal(memoryRaw, &payload); err != nil {
			return model.ActivityJournalRow{}, fmt.Errorf("unmarshal activity memory payload: %w", err)
		}
		row.Memory = &payload
	}
	if row.Memory == nil && (memoryProject != "" || memoryCategory != "" || memoryTitle != "") {
		row.Memory = &model.MemoryPayload{
			SyncID:    row.EntitySyncID,
			Project:   memoryProject,
			Category:  model.MemoryCategory(memoryCategory),
			Title:     memoryTitle,
			Content:   memoryContent,
			CreatedBy: memoryCreatedBy,
		}
	}
	if len(tombstoneRaw) > 0 && string(tombstoneRaw) != "null" {
		var payload model.TombstonePayload
		if err := json.Unmarshal(tombstoneRaw, &payload); err != nil {
			return model.ActivityJournalRow{}, fmt.Errorf("unmarshal activity tombstone payload: %w", err)
		}
		row.Tombstone = &payload
	}

	return row, nil
}

type mutationTx interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
}

func (r *postgresMemoryRepository) applyCreateMutation(ctx context.Context, tx mutationTx, mutation model.MutationEnvelope) (bool, error) {
	if mutation.Memory == nil {
		return false, fmt.Errorf("create mutation requires memory payload")
	}
	existing, err := memoryBySyncIDForUpdate(ctx, tx, mutation.EntitySyncID)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return false, nil
	}

	mem := memoryFromPayload(mutation.Memory)
	if mem.SyncID == "" {
		mem.SyncID = mutation.EntitySyncID
	}
	mem.Project = mutation.Project
	if mem.CreatedAt.IsZero() {
		mem.CreatedAt = mutation.OccurredAt
	}
	if mem.UpdatedAt.IsZero() {
		mem.UpdatedAt = mutation.OccurredAt
	}
	tagsJSON, err := json.Marshal(orEmptySlice(mem.Tags))
	if err != nil {
		return false, fmt.Errorf("marshal mutation tags: %w", err)
	}
	filesJSON, err := json.Marshal(orEmptySlice(mem.FilesAffected))
	if err != nil {
		return false, fmt.Errorf("marshal mutation files_affected: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO memories
			(sync_id, project, topic_key, category, title, content, tags, files_affected,
			 created_by, created_at, updated_at, session_id, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())`,
		mem.SyncID, mem.Project, mem.TopicKey, mem.Category, mem.Title, mem.Content,
		tagsJSON, filesJSON, mem.CreatedBy, mem.CreatedAt, mem.UpdatedAt,
		mem.SessionID)
	if err != nil {
		return false, wrapPgError(err, "apply create memory mutation")
	}
	return true, nil
}

func (r *postgresMemoryRepository) applyUpdateMutation(ctx context.Context, tx mutationTx, mutation model.MutationEnvelope) (bool, error) {
	if mutation.Memory == nil {
		return false, fmt.Errorf("update mutation requires memory payload")
	}
	existing, err := memoryBySyncIDForUpdate(ctx, tx, mutation.EntitySyncID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, ErrNotFound
	}
	if existing.DeletedAt != nil {
		return false, ErrMemoryTombstoned
	}
	mem := memoryFromPayload(mutation.Memory)
	if !mem.UpdatedAt.IsZero() && !mem.UpdatedAt.After(existing.UpdatedAt) {
		return false, nil
	}
	if mem.UpdatedAt.IsZero() {
		mem.UpdatedAt = mutation.OccurredAt
	}

	tagsJSON, err := json.Marshal(orEmptySlice(mem.Tags))
	if err != nil {
		return false, fmt.Errorf("marshal mutation tags: %w", err)
	}
	filesJSON, err := json.Marshal(orEmptySlice(mem.FilesAffected))
	if err != nil {
		return false, fmt.Errorf("marshal mutation files_affected: %w", err)
	}

	cmd, err := tx.Exec(ctx, `
		UPDATE memories
		SET topic_key=$1, category=$2, title=$3, content=$4,
		    tags=$5, files_affected=$6, updated_at=$7,
		    session_id=COALESCE(NULLIF($8, ''), session_id), synced_at=now()
		WHERE sync_id=$9 AND project=$10 AND deleted_at IS NULL`,
		mem.TopicKey, mem.Category, mem.Title, mem.Content, tagsJSON, filesJSON,
		mem.UpdatedAt, mem.SessionID, mutation.EntitySyncID, mutation.Project)
	if err != nil {
		return false, wrapPgError(err, "apply update memory mutation")
	}
	return cmd.RowsAffected() > 0, nil
}

func (r *postgresMemoryRepository) applyDeleteMutation(ctx context.Context, tx mutationTx, mutation model.MutationEnvelope) (bool, error) {
	existing, err := memoryBySyncIDForUpdate(ctx, tx, mutation.EntitySyncID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, ErrNotFound
	}
	deletedAt := mutation.OccurredAt
	var deletedBy, reason string
	if mutation.Tombstone != nil {
		if !mutation.Tombstone.DeletedAt.IsZero() {
			deletedAt = mutation.Tombstone.DeletedAt
		}
		deletedBy = mutation.Tombstone.DeletedBy
		reason = mutation.Tombstone.Reason
	}
	cmd, err := tx.Exec(ctx, `
		UPDATE memories
		SET deleted_at=$1, deleted_by=NULLIF($2, ''), delete_reason=NULLIF($3, ''), synced_at=now()
		WHERE sync_id=$4 AND project=$5`, deletedAt, deletedBy, reason, mutation.EntitySyncID, mutation.Project)
	if err != nil {
		return false, wrapPgError(err, "apply delete memory mutation")
	}
	return cmd.RowsAffected() > 0, nil
}

func (r *postgresMemoryRepository) applyRestoreMutation(ctx context.Context, tx mutationTx, mutation model.MutationEnvelope) (bool, error) {
	existing, err := memoryBySyncIDForUpdate(ctx, tx, mutation.EntitySyncID)
	if err != nil {
		return false, err
	}
	if existing == nil || existing.DeletedAt == nil {
		return false, ErrNotFound
	}

	if mutation.Memory != nil {
		mem := memoryFromPayload(mutation.Memory)
		if mem.UpdatedAt.IsZero() {
			mem.UpdatedAt = mutation.OccurredAt
		}
		tagsJSON, err := json.Marshal(orEmptySlice(mem.Tags))
		if err != nil {
			return false, fmt.Errorf("marshal mutation tags: %w", err)
		}
		filesJSON, err := json.Marshal(orEmptySlice(mem.FilesAffected))
		if err != nil {
			return false, fmt.Errorf("marshal mutation files_affected: %w", err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE memories
			SET topic_key=$1, category=$2, title=$3, content=$4,
			    tags=$5, files_affected=$6, updated_at=$7,
			    session_id=COALESCE(NULLIF($8, ''), session_id),
			    deleted_at=NULL, deleted_by=NULL, delete_reason=NULL, restored_at=$9, synced_at=now()
			WHERE sync_id=$10 AND project=$11`,
			mem.TopicKey, mem.Category, mem.Title, mem.Content, tagsJSON, filesJSON,
			mem.UpdatedAt, mem.SessionID, mutation.OccurredAt, mutation.EntitySyncID, mutation.Project)
		if err != nil {
			return false, wrapPgError(err, "apply restore memory mutation")
		}
		return true, nil
	}

	cmd, err := tx.Exec(ctx, `
		UPDATE memories
		SET deleted_at=NULL, deleted_by=NULL, delete_reason=NULL, restored_at=$1, synced_at=now()
		WHERE sync_id=$2 AND project=$3 AND deleted_at IS NOT NULL`, mutation.OccurredAt, mutation.EntitySyncID, mutation.Project)
	if err != nil {
		return false, wrapPgError(err, "apply restore memory mutation")
	}
	return cmd.RowsAffected() > 0, nil
}

func insertMemoryMutation(ctx context.Context, tx mutationTx, mutation model.MutationEnvelope) (int64, error) {
	memoryJSON, err := json.Marshal(mutation.Memory)
	if err != nil {
		return 0, fmt.Errorf("marshal mutation memory payload: %w", err)
	}
	tombstoneJSON, err := json.Marshal(mutation.Tombstone)
	if err != nil {
		return 0, fmt.Errorf("marshal mutation tombstone payload: %w", err)
	}

	var sequence int64
	err = tx.QueryRow(ctx, `
		INSERT INTO memory_mutations
			(event_id, entity_type, entity_sync_id, project, op, occurred_at, actor_id, base_updated_at, memory, tombstone)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7, ''),$8,$9,$10)
		RETURNING sequence`,
		mutation.EventID, mutation.EntityType, mutation.EntitySyncID, mutation.Project,
		mutation.Op, mutation.OccurredAt, mutation.ActorID, mutation.BaseUpdatedAt, memoryJSON, tombstoneJSON).Scan(&sequence)
	if err != nil {
		return 0, wrapPgError(err, "insert memory mutation")
	}
	return sequence, nil
}

func memoryBySyncIDForUpdate(ctx context.Context, tx mutationTx, syncID string) (*model.Memory, error) {
	row := tx.QueryRow(ctx, `SELECT id, sync_id, project, topic_key, category, title, content,
		                         tags, files_affected, created_by, created_at, updated_at,
		                         origin, synced_at, session_id,
		                         deleted_at, deleted_by, delete_reason, restored_at
		                  FROM memories WHERE sync_id = $1 FOR UPDATE`, syncID)
	mem, err := scanMemoryRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, wrapPgError(err, "select memory for mutation")
	}
	return mem, nil
}

func memoryFromPayload(payload *model.MemoryPayload) *model.Memory {
	var sessionID *string
	if payload.SessionID != "" {
		sessionID = &payload.SessionID
	}
	return &model.Memory{
		SyncID:        payload.SyncID,
		Project:       payload.Project,
		TopicKey:      payload.TopicKey,
		Category:      payload.Category,
		Title:         payload.Title,
		Content:       payload.Content,
		Tags:          payload.Tags,
		FilesAffected: payload.FilesAffected,
		CreatedBy:     payload.CreatedBy,
		CreatedAt:     payload.CreatedAt,
		UpdatedAt:     payload.UpdatedAt,
		SessionID:     sessionID,
	}
}

// CountByProject returns memory counts grouped by project, DESC by count.
// Soft-deleted memories are excluded. Returns []ProjectCount{} (not nil) when empty.
func (r *postgresMemoryRepository) CountByProject(ctx context.Context, filter model.MemoryFilter) ([]model.ProjectCount, error) {
	q := fmt.Sprintf(`
		SELECT project, COUNT(*) AS cnt
		FROM memories
		WHERE deleted_at IS NULL
		  AND %s
		GROUP BY project
		ORDER BY COUNT(*) DESC`, unblockedProjectPredicate("memories.project"))

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, wrapPgError(err, "CountByProject")
	}
	defer rows.Close()

	result := []model.ProjectCount{}
	for rows.Next() {
		var pc model.ProjectCount
		if err := rows.Scan(&pc.Project, &pc.Count); err != nil {
			return nil, wrapPgError(err, "scan CountByProject row")
		}
		result = append(result, pc)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate CountByProject rows")
	}
	return result, nil
}

// CountLiveActivity counts memories synced within the given window and returns the newest sync_id.
func (r *postgresMemoryRepository) CountLiveActivity(ctx context.Context, since time.Time) (int, string, error) {
	q := fmt.Sprintf(`
			SELECT COUNT(*) AS c,
			       COALESCE(
			         (SELECT sync_id::text FROM memories
			          WHERE synced_at >= $1 AND deleted_at IS NULL
			            AND %s
			          ORDER BY synced_at DESC, id DESC LIMIT 1),
			         '') AS newest
			FROM memories
			WHERE synced_at >= $1 AND deleted_at IS NULL
			  AND %s`, unblockedProjectPredicate("memories.project"), unblockedProjectPredicate("memories.project"))

	var count int
	var newest string
	err := r.db.QueryRow(ctx, q, since).Scan(&count, &newest)
	if err != nil {
		return 0, "", wrapPgError(err, "CountLiveActivity")
	}
	return count, newest, nil
}

// CountGrowthByMonth returns cumulative memory counts by month (ascending) over the last N months.
// Uses created_at (not synced_at) to reflect knowledge accumulation.
func (r *postgresMemoryRepository) CountGrowthByMonth(ctx context.Context, months int) ([]model.MonthCount, error) {
	q := fmt.Sprintf(`
		WITH months AS (
		  SELECT date_trunc('month', now()) - (n || ' months')::interval AS m
		  FROM generate_series($1 - 1, 0, -1) AS n
		)
			SELECT months.m,
			       (SELECT COUNT(*) FROM memories
			        WHERE deleted_at IS NULL
			          AND created_at < months.m + interval '1 month'
			          AND %s) AS cumulative
			FROM months ORDER BY months.m ASC`, unblockedProjectPredicate("memories.project"))

	rows, err := r.db.Query(ctx, q, months)
	if err != nil {
		return nil, wrapPgError(err, "CountGrowthByMonth")
	}
	defer rows.Close()

	result := []model.MonthCount{}
	for rows.Next() {
		var m time.Time
		var cumulative int64
		if err := rows.Scan(&m, &cumulative); err != nil {
			return nil, wrapPgError(err, "scan CountGrowthByMonth row")
		}
		result = append(result, model.MonthCount{
			Label: m.Format("Jan 2006"),
			Value: int(cumulative),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate CountGrowthByMonth rows")
	}
	return result, nil
}

// --- helpers privados ---

func appendMemoryFilterPredicates(where string, args []interface{}, argIdx int, filter model.MemoryFilter) (string, []interface{}, int) {
	if filter.Project != "" {
		where += fmt.Sprintf(" AND (project = $%d OR EXISTS (SELECT 1 FROM project_identity_spellings pis WHERE pis.spelling = project AND pis.project_key = $%d))", argIdx, argIdx)
		args = append(args, filter.Project)
		argIdx++
	}
	if filter.Category != nil {
		where += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, *filter.Category)
		argIdx++
	}
	if filter.CreatedFrom != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *filter.CreatedFrom)
		argIdx++
	}
	if filter.CreatedUntil != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *filter.CreatedUntil)
		argIdx++
	}

	where += " AND deleted_at IS NULL AND " + unblockedProjectPredicate("memories.project")
	return where, args, argIdx
}

func (r *postgresMemoryRepository) rejectBlockedProject(ctx context.Context, project string) error {
	return checkProjectBlocked(ctx, r.db, project)
}

// scanMemory ejecuta una query de fila única y escanea el resultado.
func (r *postgresMemoryRepository) scanMemory(ctx context.Context, query string, arg interface{}) (*model.Memory, error) {
	row := r.db.QueryRow(ctx, query, arg)
	mem, err := scanMemoryRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrapPgError(err, "scanMemory")
	}
	return mem, nil
}

// scanMemoryRow escanea una fila de memoria desde un pgx.Row.
func scanMemoryRow(row pgx.Row) (*model.Memory, error) {
	mem := &model.Memory{}
	var tagsRaw, filesRaw []byte

	err := row.Scan(
		&mem.ID, &mem.SyncID, &mem.Project, &mem.TopicKey,
		&mem.Category, &mem.Title, &mem.Content,
		&tagsRaw, &filesRaw,
		&mem.CreatedBy, &mem.CreatedAt, &mem.UpdatedAt,
		&mem.Origin, &mem.SyncedAt,
		&mem.SessionID,
		&mem.DeletedAt, &mem.DeletedBy, &mem.DeleteReason, &mem.RestoredAt,
	)
	if err != nil {
		return nil, err
	}

	// Deserializamos los campos JSONB de vuelta a slices de strings
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &mem.Tags)
	}
	if len(filesRaw) > 0 {
		_ = json.Unmarshal(filesRaw, &mem.FilesAffected)
	}

	return mem, nil
}

// scanMemoryRows itera sobre un conjunto de filas y devuelve todos los resultados.
func (r *postgresMemoryRepository) scanMemoryRows(rows pgx.Rows) ([]*model.Memory, error) {
	var mems []*model.Memory
	for rows.Next() {
		mem := &model.Memory{}
		var tagsRaw, filesRaw []byte
		err := rows.Scan(
			&mem.ID, &mem.SyncID, &mem.Project, &mem.TopicKey,
			&mem.Category, &mem.Title, &mem.Content,
			&tagsRaw, &filesRaw,
			&mem.CreatedBy, &mem.CreatedAt, &mem.UpdatedAt,
			&mem.Origin, &mem.SyncedAt,
			&mem.SessionID,
			&mem.DeletedAt, &mem.DeletedBy, &mem.DeleteReason, &mem.RestoredAt,
		)
		if err != nil {
			return nil, wrapPgError(err, "scan memory row")
		}
		if len(tagsRaw) > 0 {
			_ = json.Unmarshal(tagsRaw, &mem.Tags)
		}
		if len(filesRaw) > 0 {
			_ = json.Unmarshal(filesRaw, &mem.FilesAffected)
		}
		mems = append(mems, mem)
	}
	return mems, rows.Err()
}

// wrapPgError envuelve errores de pgx con contexto adicional.
// En producción esto se logearía con el request ID para trazabilidad.
func wrapPgError(err error, op string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%s: unique constraint: %w", op, ErrConflict)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// orEmptySlice devuelve el slice original si tiene elementos, o un slice vacío.
// Evita guardar JSON null en la BD — siempre guardamos [] para arrays vacíos.
func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
