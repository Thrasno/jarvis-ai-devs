package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresMemoryRepository es la implementación de MemoryRepository sobre PostgreSQL.
type postgresMemoryRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresMemoryRepository crea la implementación real de MemoryRepository.
func NewPostgresMemoryRepository(pool *pgxpool.Pool) MemoryRepository {
	return &postgresMemoryRepository{pool: pool}
}

// Create inserta una nueva memoria y devuelve el registro completo (con ID del servidor).
func (r *postgresMemoryRepository) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
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

	row := r.pool.QueryRow(ctx, q,
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
	const q = `SELECT id, sync_id, project, topic_key, category, title, content,
	                  tags, files_affected, created_by, created_at, updated_at,
	                  origin, synced_at, session_id,
	                  deleted_at, deleted_by, delete_reason, restored_at
	           FROM memories WHERE id = $1 AND deleted_at IS NULL`
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

	if filter.Project != "" {
		where += fmt.Sprintf(" AND project = $%d", argIdx)
		args = append(args, filter.Project)
		argIdx++
	}
	if filter.Category != nil {
		where += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, *filter.Category)
	}

	where += " AND deleted_at IS NULL"

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, session_id,
	                         deleted_at, deleted_by, delete_reason, restored_at
	                  FROM memories WHERE 1=1 %s
	                  ORDER BY synced_at DESC LIMIT $1 OFFSET $2`, where)

	rows, err := r.pool.Query(ctx, q, args...)
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

	if filter.Project != "" {
		where += fmt.Sprintf(" AND project = $%d", argIdx)
		args = append(args, filter.Project)
		argIdx++
	}
	if filter.Category != nil {
		where += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, *filter.Category)
	}

	where += " AND deleted_at IS NULL"

	q := fmt.Sprintf(`SELECT COUNT(*) FROM memories WHERE 1=1 %s`, where)
	var count int64
	err := r.pool.QueryRow(ctx, q, args...).Scan(&count)
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

	if filter.Project != "" {
		where += fmt.Sprintf(" AND project = $%d", argIdx)
		args = append(args, filter.Project)
	}

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, session_id,
	                         deleted_at, deleted_by, delete_reason, restored_at
	                  FROM memories
	                  WHERE search_vector @@ plainto_tsquery('simple', $1) AND deleted_at IS NULL %s
	                  ORDER BY ts_rank(search_vector, plainto_tsquery('simple', $1)) DESC
	                  LIMIT $2 OFFSET $3`, where)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "Search memories")
	}
	defer rows.Close()

	return r.scanMemoryRows(rows)
}

// Upsert implementa el algoritmo de 4 ramas del protocolo de sync.
// Ver la documentación en la interfaz MemoryRepository para los detalles de cada rama.
func (r *postgresMemoryRepository) Upsert(ctx context.Context, mem *model.Memory) (*model.Memory, bool, error) {
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

	row := r.pool.QueryRow(ctx, q,
		mem.TopicKey, mem.Category, mem.Title, mem.Content,
		tagsJSON, filesJSON, mem.UpdatedAt,
		mem.SessionID, id,
	)

	return scanMemoryRow(row)
}

// PullSince devuelve las memorias del proyecto actualizadas después de 'since'.
func (r *postgresMemoryRepository) PullSince(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error) {
	args := []interface{}{project}
	where := "project = $1 AND deleted_at IS NULL"
	argIdx := 2

	if !since.IsZero() {
		// >= para no perder memorias con synced_at exactamente igual a 'since'
		where += fmt.Sprintf(" AND synced_at >= $%d", argIdx)
		args = append(args, since)
		argIdx++
	}

	if len(excludeSyncIDs) > 0 {
		where += fmt.Sprintf(" AND sync_id != ALL($%d)", argIdx)
		args = append(args, excludeSyncIDs)
	}

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, session_id,
	                         deleted_at, deleted_by, delete_reason, restored_at
	                  FROM memories WHERE %s ORDER BY synced_at ASC`, where)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "PullSince")
	}
	defer rows.Close()

	return r.scanMemoryRows(rows)
}

func (r *postgresMemoryRepository) ApplyMemoryMutation(ctx context.Context, mutation model.MutationEnvelope) (*model.MutationApplyResult, error) {
	if mutation.EventID == "" || mutation.EntityType != model.MutationEntityMemory || mutation.EntitySyncID == "" || mutation.Project == "" {
		return nil, fmt.Errorf("invalid memory mutation envelope")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, wrapPgError(err, "begin memory mutation")
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var existingSequence int64
	err = tx.QueryRow(ctx, `SELECT sequence FROM memory_mutations WHERE event_id = $1`, mutation.EventID).Scan(&existingSequence)
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
	if err := tx.Commit(ctx); err != nil {
		return nil, wrapPgError(err, "commit memory mutation")
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

	rows, err := r.pool.Query(ctx, `
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

// --- helpers privados ---

// scanMemory ejecuta una query de fila única y escanea el resultado.
func (r *postgresMemoryRepository) scanMemory(ctx context.Context, query string, arg interface{}) (*model.Memory, error) {
	row := r.pool.QueryRow(ctx, query, arg)
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
