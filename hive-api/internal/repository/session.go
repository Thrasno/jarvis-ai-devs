package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
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

	// ListSessionsSince devuelve las sesiones del proyecto cuyo synced_at > since,
	// ordenadas por started_at ASC. Cuando since es zero, devuelve todas las sesiones
	// del proyecto (primer sync incremental). El filtro por project es obligatorio:
	// sin él, un daemon recibiría sesiones de otros proyectos (R2-CRIT-4 — tenant leak).
	ListSessionsSince(ctx context.Context, project string, since time.Time) ([]*model.Session, error)
}
