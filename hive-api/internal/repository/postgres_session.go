package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Verify interface compliance at compile time.
var _ SessionRepository = (*postgresSessionRepository)(nil)

type postgresSessionRepository struct {
	db pgxQuerier
}

// NewPostgresSessionRepository crea la implementación real de SessionRepository.
func NewPostgresSessionRepository(pool *pgxpool.Pool) SessionRepository {
	return newPostgresSessionRepositoryWithQuerier(pool)
}

func newPostgresSessionRepositoryWithQuerier(db pgxQuerier) SessionRepository {
	return &postgresSessionRepository{db: db}
}

// CreateSession inserta una nueva sesión usando ON CONFLICT (sync_id) DO NOTHING
// para sesiones normales — el daemon puede reenviar el mismo sync sin duplicar.
func (r *postgresSessionRepository) CreateSession(ctx context.Context, s *model.Session) error {
	const q = `
		INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (sync_id) DO NOTHING`

	_, err := r.db.Exec(ctx, q,
		s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
		s.StartedAt, s.EndedAt, s.Summary,
	)
	return wrapPgError(err, "CreateSession")
}

// UpsertSession upserts a session from the sync wire.
//
// Three conflict patterns (Decision 12 — refined CRIT-4):
//   - manual-save-*: conflict on (id), keep LEAST(started_at) so concurrent
//     daemons converge to the earliest seen start.
//   - legacy-pre-lifecycle-*: conflict on (id), DO NOTHING — the migration
//     created the canonical row; daemons re-pushing the same id must not
//     overwrite it (and the LEAST semantics do not apply because each daemon's
//     local sentinel is independently backfilled to MIN(memories.created_at)).
//   - Regular sessions (UUID-style id): conflict on (sync_id), DO NOTHING.
func (r *postgresSessionRepository) UpsertSession(ctx context.Context, s *model.Session) error {
	if strings.HasPrefix(s.ID, "manual-save-") {
		// R3-FIX-1 — refresh dev_id/client when EXCLUDED carries an attributed value
		// (different from placeholder defaults 'unknown'/'manual') so the lazy-fallback
		// row created by the server doesn't permanently mask the daemon's HIVE_DEV_ID.
		// Conversely, a placeholder push must NOT downgrade an already-attributed row.
		// R4-FIX-3 — also reject EMPTY string as a refresh source. Pre-fix only
		// 'unknown'/'manual' were rejected, so a daemon (or test) pushing
		// dev_id='' silently corrupted an already-attributed row to ''.
		const q = `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE
			  SET started_at = LEAST(sessions.started_at, EXCLUDED.started_at),
			      dev_id     = CASE WHEN EXCLUDED.dev_id <> 'unknown' AND EXCLUDED.dev_id <> '' THEN EXCLUDED.dev_id ELSE sessions.dev_id END,
			      client     = CASE WHEN EXCLUDED.client <> 'manual'  AND EXCLUDED.client <> '' THEN EXCLUDED.client ELSE sessions.client END,
			      updated_at = now()`

		_, err := r.db.Exec(ctx, q,
			s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
			s.StartedAt, s.EndedAt, s.Summary,
		)
		return wrapPgError(err, "UpsertSession(manual-save)")
	}

	if strings.HasPrefix(s.ID, "legacy-pre-lifecycle-") {
		const q = `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO NOTHING`

		_, err := r.db.Exec(ctx, q,
			s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
			s.StartedAt, s.EndedAt, s.Summary,
		)
		return wrapPgError(err, "UpsertSession(legacy-pre-lifecycle)")
	}

	const q = `
		INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (sync_id) DO NOTHING`

	_, err := r.db.Exec(ctx, q,
		s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
		s.StartedAt, s.EndedAt, s.Summary,
	)
	return wrapPgError(err, "UpsertSession")
}

// isSentinelID returns true for manual-save-* and legacy-pre-lifecycle-* ids.
// Used for sentinel detection by callers that don't need the per-prefix dispatch.
func isSentinelID(id string) bool {
	return strings.HasPrefix(id, "manual-save-") || strings.HasPrefix(id, "legacy-pre-lifecycle-")
}

// EndSession actualiza ended_at y summary de una sesión existente.
func (r *postgresSessionRepository) EndSession(ctx context.Context, sessionID, summary string) error {
	const q = `
		UPDATE sessions
		SET ended_at = now(), summary = $2
		WHERE id = $1`

	tag, err := r.db.Exec(ctx, q, sessionID, summary)
	if err != nil {
		return wrapPgError(err, "EndSession")
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSession devuelve una sesión por ID literal.
func (r *postgresSessionRepository) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	const q = `
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at, created_at, updated_at
		FROM sessions
		WHERE id = $1`

	row := r.db.QueryRow(ctx, q, sessionID)
	s, err := scanSession(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrapPgError(err, "GetSession")
	}
	return s, nil
}

// EnsureManualSaveSession crea lazily la sesión 'manual-save-{project}'.
// ON CONFLICT (id) DO NOTHING garantiza idempotencia y seguridad bajo concurrencia.
// La semántica de 'manual-save-*': client='manual', ended_at=NULL forever.
func (r *postgresSessionRepository) EnsureManualSaveSession(ctx context.Context, project string) (string, error) {
	id := "manual-save-" + project

	const q = `
		INSERT INTO sessions (id, project, dev_id, client, started_at)
		VALUES ($1, $2, 'unknown', 'manual', now())
		ON CONFLICT (id) DO NOTHING`

	_, err := r.db.Exec(ctx, q, id, project)
	if err != nil {
		return "", wrapPgError(err, "EnsureManualSaveSession")
	}
	return id, nil
}

// ListSessionsByProject devuelve todas las sesiones de un proyecto ordenadas por started_at DESC.
func (r *postgresSessionRepository) ListSessionsByProject(ctx context.Context, project string) ([]model.Session, error) {
	const q = `
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at, created_at, updated_at
		FROM sessions
		WHERE project = $1
		ORDER BY started_at DESC`

	rows, err := r.db.Query(ctx, q, project)
	if err != nil {
		return nil, wrapPgError(err, "ListSessionsByProject")
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, wrapPgError(err, "scan session row")
		}
		sessions = append(sessions, *s)
	}
	return sessions, rows.Err()
}

// ListSessionsSince devuelve una página de sesiones del proyecto cuyo synced_at >=
// since, ordenadas por (synced_at ASC, sync_id ASC) para paginación por keyset.
// Cuando since es zero, el barrido arranca desde el principio. El filtro por
// project previene leak de tenants (R2-CRIT-4).
//
// cursor y limit siguen la misma semántica que postgresMemoryRepository.PullSince:
// cursor (si no es cursor.IsZero()) reanuda estrictamente después de
// (cursor.SyncedAt, cursor.SyncID); limit <= 0 (model.UnboundedPullLimit)
// significa "sin LIMIT" — barrido completo en una sola página, hasMore=false
// (backward-compat PR 2a, ver postgresMemoryRepository.PullSince); limit > 0
// pide limit+1 y recorta a limit.
//
// NOTA (ordering-semantics change): el ORDER BY de esta consulta cambió de
// started_at ASC a (synced_at ASC, sync_id ASC) respecto a la versión anterior
// a la paginación por keyset — necesario para que el cursor componga con el
// índice compuesto idx_sessions_project_synced_at_sync_id (migración 011). Cualquier
// consumidor que asumiera orden por started_at debe revisar esa suposición;
// el pull ahora ordena por momento de sincronización, no por inicio de sesión.
//
// El filtro de watermark usa `synced_at >= since`, igual que
// postgresMemoryRepository.PullSince, para no perder sesiones con synced_at
// exactamente igual a `since`; el cursor compuesto (synced_at, sync_id) reanuda
// estrictamente después de la última fila vista y evita duplicados.
func (r *postgresSessionRepository) ListSessionsSince(ctx context.Context, project string, since time.Time, cursor model.PullCursor, limit int) ([]*model.Session, bool, error) {
	args := []interface{}{project}
	where := "project = $1 AND " + unblockedProjectPredicate("sessions.project")
	argIdx := 2

	if !since.IsZero() {
		where += fmt.Sprintf(" AND synced_at >= $%d", argIdx)
		args = append(args, since)
		argIdx++
	}

	if !cursor.IsZero() {
		where += fmt.Sprintf(" AND (synced_at, sync_id) > ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cursor.SyncedAt, cursor.SyncID)
		argIdx += 2
	}

	unbounded := limit <= 0

	q := fmt.Sprintf(`
		SELECT id, sync_id, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at, created_at, updated_at
		FROM sessions
		WHERE %s
		ORDER BY synced_at ASC, sync_id ASC`, where)
	if !unbounded {
		fetchLimit := limit + 1
		q += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, fetchLimit)
	}

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, false, wrapPgError(err, "ListSessionsSince")
	}
	defer rows.Close()

	var sessions []*model.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, false, wrapPgError(err, "ListSessionsSince scan")
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, false, wrapPgError(err, "ListSessionsSince rows")
	}

	if unbounded {
		return sessions, false, nil
	}

	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}

	return sessions, hasMore, nil
}

// scanSession escanea una fila de sesión desde un pgx.Row o pgx.Rows.
func scanSession(row pgx.Row) (*model.Session, error) {
	s := &model.Session{}
	err := row.Scan(
		&s.ID, &s.SyncID, &s.Project, &s.Directory, &s.DevID, &s.Client,
		&s.StartedAt, &s.EndedAt, &s.Summary, &s.SyncedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}
