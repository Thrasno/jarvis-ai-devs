package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

// SessionRepository define las operaciones de base de datos para sesiones.
type SessionRepository interface {
	// CreateSession inserta una nueva sesión. Usa ON CONFLICT (sync_id) DO NOTHING
	// para sesiones normales (idempotencia de sync).
	CreateSession(ctx context.Context, session *model.Session) error

	// UpsertSession upserts a session arriving via the sync wire.
	// For 'manual-save-*' and 'legacy-pre-lifecycle-*' ids (sentinel ids), the conflict
	// target is (id) and it keeps MIN(started_at) — Decision 12.
	// For regular sessions, the conflict target is (sync_id) and it does nothing on conflict.
	UpsertSession(ctx context.Context, session *model.Session) error

	// EndSession marca una sesión como terminada con un resumen opcional.
	// Devuelve ErrNotFound si la sesión no existe.
	EndSession(ctx context.Context, sessionID, summary string) error

	// GetSession devuelve una sesión por su ID literal.
	// Devuelve ErrNotFound si no existe.
	GetSession(ctx context.Context, sessionID string) (*model.Session, error)

	// EnsureManualSaveSession crea lazily la sesión 'manual-save-{project}' si no existe.
	// Idempotente: ON CONFLICT (id) DO NOTHING. Devuelve el id de la sesión.
	// Seguro bajo concurrencia: múltiples goroutines pueden llamarlo para el mismo proyecto.
	EnsureManualSaveSession(ctx context.Context, project string) (sessionID string, err error)

	// ListSessionsByProject devuelve todas las sesiones de un proyecto, ordenadas por started_at DESC.
	ListSessionsByProject(ctx context.Context, project string) ([]model.Session, error)

	// ListSessionsSince devuelve una página de sesiones del proyecto cuyo synced_at
	// > since, ordenadas por (synced_at ASC, sync_id ASC) para paginación por keyset.
	// Cuando since es zero, el barrido arranca desde el principio (primer sync
	// incremental). El filtro por project es obligatorio: sin él, un daemon recibiría
	// sesiones de otros proyectos (R2-CRIT-4 — tenant leak).
	//
	// cursor y limit siguen exactamente la misma semántica que
	// MemoryRepository.PullSince: cursor (si no es cursor.IsZero()) reanuda
	// estrictamente después de (cursor.SyncedAt, cursor.SyncID); limit acota el
	// tamaño de página (ya clampeado por el caller); hasMore indica si quedan más
	// filas después de la última devuelta.
	//
	// NOTA: el ordenamiento de salida cambia de started_at ASC a (synced_at, sync_id)
	// ASC respecto a la firma anterior — necesario para que el cursor componga con el
	// índice compuesto (project, synced_at, sync_id). El daemon consume sesiones por lote, no
	// depende de un orden de started_at estable entre páginas.
	ListSessionsSince(ctx context.Context, project string, since time.Time, cursor model.PullCursor, limit int) (sessions []*model.Session, hasMore bool, err error)
}
