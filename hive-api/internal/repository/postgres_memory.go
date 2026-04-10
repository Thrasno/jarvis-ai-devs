package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
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
			 origin, confidence, impact_score)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
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
		mem.Origin, mem.Confidence, mem.ImpactScore,
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
	                  origin, synced_at, confidence, impact_score
	           FROM memories WHERE id = $1`
	return r.scanMemory(ctx, q, id)
}

// GetBySyncID devuelve una memoria por su sync_id (generado por el daemon).
// Devuelve nil sin error si no existe — es el único método que hace esto.
func (r *postgresMemoryRepository) GetBySyncID(ctx context.Context, syncID string) (*model.Memory, error) {
	const q = `SELECT id, sync_id, project, topic_key, category, title, content,
	                  tags, files_affected, created_by, created_at, updated_at,
	                  origin, synced_at, confidence, impact_score
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

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, confidence, impact_score
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
	                         origin, synced_at, confidence, impact_score
	                  FROM memories
	                  WHERE search_vector @@ plainto_tsquery('spanish', $1) %s
	                  ORDER BY ts_rank(search_vector, plainto_tsquery('spanish', $1)) DESC
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
	               confidence=$8, impact_score=$9, synced_at=now()
	           WHERE id=$10
	           RETURNING id, sync_id, project, topic_key, category, title, content,
	                     tags, files_affected, created_by, created_at, updated_at,
	                     origin, synced_at, confidence, impact_score`

	tagsJSON, _ := json.Marshal(orEmptySlice(mem.Tags))
	filesJSON, _ := json.Marshal(orEmptySlice(mem.FilesAffected))

	row := r.pool.QueryRow(ctx, q,
		mem.TopicKey, mem.Category, mem.Title, mem.Content,
		tagsJSON, filesJSON, mem.UpdatedAt,
		mem.Confidence, mem.ImpactScore, id,
	)

	return scanMemoryRow(row)
}

// PullSince devuelve las memorias del proyecto actualizadas después de 'since'.
func (r *postgresMemoryRepository) PullSince(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error) {
	args := []interface{}{project}
	where := "project = $1"
	argIdx := 2

	if !since.IsZero() {
		where += fmt.Sprintf(" AND synced_at > $%d", argIdx)
		args = append(args, since)
		argIdx++
	}

	if len(excludeSyncIDs) > 0 {
		where += fmt.Sprintf(" AND sync_id != ALL($%d)", argIdx)
		args = append(args, excludeSyncIDs)
	}

	q := fmt.Sprintf(`SELECT id, sync_id, project, topic_key, category, title, content,
	                         tags, files_affected, created_by, created_at, updated_at,
	                         origin, synced_at, confidence, impact_score
	                  FROM memories WHERE %s ORDER BY synced_at ASC`, where)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "PullSince")
	}
	defer rows.Close()

	return r.scanMemoryRows(rows)
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
		&mem.Confidence, &mem.ImpactScore,
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
			&mem.Confidence, &mem.ImpactScore,
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
