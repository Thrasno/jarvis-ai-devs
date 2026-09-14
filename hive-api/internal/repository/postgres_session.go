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

// sessionCreateCorrectionConflict preserves CreateSession's correction-only
// behavior. $10 is the from-project precondition.
const sessionCreateCorrectionConflict = `
	ON CONFLICT (sync_id) DO UPDATE
	  SET project = EXCLUDED.project, synced_at = now()
	  WHERE sessions.project = $10`

// sessionLifecycleConflict accepts regular lifecycle updates from the sync wire.
// It deliberately preserves identity and provenance while applying nullable
// ended_at and summary so a reopened session becomes visible to pull clients.
const sessionLifecycleConflict = `
	ON CONFLICT (sync_id) DO UPDATE
	  SET project = EXCLUDED.project,
	      ended_at = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.ended_at ELSE sessions.ended_at END,
	      summary = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.summary ELSE sessions.summary END,
	      synced_at = now()
	  WHERE sessions.project = $10`

// CreateSession inserts a new session. A conflict on sync_id means "this is the
// same session, resent": everything stays idempotent except project, the one
// column the daemon is authority over.
func (r *postgresSessionRepository) CreateSession(ctx context.Context, s *model.Session) error {
	if err := r.rejectRelocationEnds(ctx, s); err != nil {
		return err
	}
	const q = `
		INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)` + sessionCreateCorrectionConflict

	_, err := r.db.Exec(ctx, q,
		s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
		s.StartedAt, s.EndedAt, s.Summary, relocationSource(s.Project, s.FromProject),
	)
	return wrapPgError(err, "CreateSession")
}

// UpsertSession upserts a session from the sync wire.
//
// # Project correction
//
// The daemon is the sole authority on project identity. When its local identity
// migration rewrites its own rows ("Foo.Bar" -> "foo-bar") it re-pushes them
// under the corrected literal; without accepting that, the server would keep the
// old spelling forever and the same session would live under two project names.
// So on a sync_id conflict — "this is the same row, resent" — `project` is taken
// from EXCLUDED. The incoming lifecycle state is also authoritative: ended_at
// (including NULL for reopen) and summary are applied, while identity and
// provenance remain first-write-wins.
//
// # The from-project precondition
//
// Taking EXCLUDED.project is gated on `WHERE sessions.project = $10`. Ordinary
// pushes bind their current project, so their lifecycle fields update in place.
// Explicit relocations bind FromProject; the lifecycle assignments compare the
// stored and excluded projects, so a true relocation moves only project and its
// watermark. A self-relocation retains its empty no-op predicate. Without the
// source gate an explicit correction could relocate whichever row the sync_id
// happened to hit, including one that the quarantine precheck cannot see because
// syncRequestProjects only collects projects named by the request, not the row's
// current project. The gate is the counterpart of applyReprojectMutation's
// `AND project = $3`: name the literal the row holds, or move nothing.
//
// The source end is checked against the quarantine too (rejectRelocationEnds),
// which is the other half of the memory path's guarantee — there it comes from
// syncRequestProjects feeding BOTH reproject ends into the precheck.
//
// # Why the correction also bumps synced_at
//
// synced_at is the propagation mechanism, exactly as it is for a reprojected
// memory. ListSessionsSince filters on `synced_at >= since` and keysets on
// (synced_at, sync_id), so a row whose project moved without its synced_at
// moving stays behind the watermark of every daemon already syncing the target
// project: the session now belongs to them and none of them is ever told.
// Bumping it places the row inside their normal pull window. updated_at stays
// put — it describes the session, not the moment it was synced.
//
// The id-keyed branches below deliberately do NOT take it, and neither does
// EnsureManualSaveSession: their id embeds the project literal, so a genuine
// rename produces a different id — a new row, never a conflict. Accepting
// EXCLUDED.project there would add no correction path and would let a push of
// id="manual-save-A" carrying project="B" move A's sentinel into B. Pinned by
// TestUpsertSession_SentinelBranchesRefuseAProjectMove.
//
// Three conflict patterns (Decision 12 — refined CRIT-4):
//   - manual-save-*: conflict on (id), keep LEAST(started_at) so concurrent
//     daemons converge to the earliest seen start.
//   - legacy-pre-lifecycle-*: conflict on (id), DO NOTHING — the migration
//     created the canonical row; daemons re-pushing the same id must not
//     overwrite it (and the LEAST semantics do not apply because each daemon's
//     local sentinel is independently backfilled to MIN(memories.created_at)).
//   - Regular sessions (UUID-style id): conflict on (sync_id); project and
//     lifecycle state follow the daemon, while identity and provenance do not.
func (r *postgresSessionRepository) UpsertSession(ctx context.Context, s *model.Session) error {
	if err := r.rejectRelocationEnds(ctx, s); err != nil {
		return err
	}
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)` + sessionLifecycleConflict

	predicateProject := s.Project
	if s.FromProject != "" {
		// Preserve an explicit relocation source, including relocationSource's
		// empty no-op sentinel for a self-move.
		predicateProject = relocationSource(s.Project, s.FromProject)
	}

	_, err := r.db.Exec(ctx, q,
		s.ID, s.SyncID, s.Project, s.Directory, s.DevID, s.Client,
		s.StartedAt, s.EndedAt, s.Summary, predicateProject,
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
	if err := r.rejectBlockedProject(ctx, project); err != nil {
		return "", err
	}
	// The id is derived from the literal spelling, not a canonical key: the
	// caller's cross-project attribution check compares against
	// "manual-save-" + project, and the row stores the literal in `project`.
	// Folding here would hand one spelling a session owned by another.
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

// rejectBlockedProject is the session counterpart of the memory and prompt
// write-side checks. Every session write that carries a project literal runs it,
// so a sessions-only sync push cannot land rows inside a quarantine.
//
// EndSession is deliberately excluded: it carries no project literal, only a
// session id, and it can never create a row.
func (r *postgresSessionRepository) rejectBlockedProject(ctx context.Context, project string) error {
	return checkProjectBlocked(ctx, r.db, project)
}

// rejectRelocationEnds checks BOTH ends a session write can touch: the project
// the row is being written into, and — when the write asks to move an existing
// row — the project it is being moved out of. A quarantine must hold in both
// directions, and the source end is the one a sync request never names.
func (r *postgresSessionRepository) rejectRelocationEnds(ctx context.Context, s *model.Session) error {
	if err := r.rejectBlockedProject(ctx, s.Project); err != nil {
		return err
	}
	if s.FromProject == "" || s.FromProject == s.Project {
		return nil
	}
	return r.rejectBlockedProject(ctx, s.FromProject)
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
	where := "sessions.project = $1 AND " + unblockedProjectPredicate("sessions.project")
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
