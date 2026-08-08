package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

// MemoryRepository define todas las operaciones de base de datos para memorias.
//
// El método más complejo es Upsert — implementa la lógica de sincronización.
// Los demás son operaciones CRUD estándar.
type MemoryRepository interface {
	// ProjectExists reports whether the canonical project identity is registered.
	// Callers use it only for explicit project filters; an omitted filter remains global.
	ProjectExists(ctx context.Context, canonicalProjectKey string) (bool, error)

	// Create inserta una nueva memoria. Devuelve la memoria con los campos
	// generados por el servidor (ID, SyncedAt, CreatedAt, UpdatedAt).
	Create(ctx context.Context, mem *model.Memory) (*model.Memory, error)

	// GetByID busca una memoria por su UUID de servidor.
	// Devuelve ErrNotFound si no existe.
	GetByID(ctx context.Context, id string) (*model.Memory, error)

	// GetBySyncID busca una memoria por el UUID generado por el daemon.
	// Devuelve nil (sin error) si no existe — usado en la lógica de upsert
	// para saber si es insert o update.
	GetBySyncID(ctx context.Context, syncID string) (*model.Memory, error)

	// List devuelve memorias paginadas según el filtro.
	// Si filter.Project está vacío, devuelve de todos los proyectos.
	// Si filter.Limit es 0, usa el default (20).
	List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, error)

	// Count devuelve el total de memorias que coinciden con el filtro.
	// Necesario para la paginación (el cliente necesita saber cuántas páginas hay).
	Count(ctx context.Context, filter model.MemoryFilter) (int64, error)

	// Search realiza búsqueda de texto completo (FTS) con ranking BM25.
	// Usa el índice tsvector de PostgreSQL, que es mucho más eficiente
	// que un LIKE '%query%' y soporta relevancia.
	Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error)

	CountSearch(ctx context.Context, query string, filter model.MemoryFilter) (int64, error)

	// Upsert es el corazón del protocolo de sincronización.
	// Implementa estas 4 reglas en orden:
	//
	//   1. sync_id NO existe → INSERT (memoria nueva)
	//      → devuelve (memoria, true, nil)  [true = fue insertada]
	//
	//   2. sync_id existe + topic_key IS NULL → SKIP (memoria inmutable)
	//      → devuelve (existente, false, nil)
	//
	//   3. sync_id existe + incoming.UpdatedAt <= existing.UpdatedAt → SKIP (servidor gana)
	//      → devuelve (nil, false, nil)  [nil indica "conflicto, servidor ganó"]
	//
	//   4. sync_id existe + incoming.UpdatedAt > existing.UpdatedAt → UPDATE (cliente gana)
	//      → devuelve (actualizada, false, nil)
	//
	// El SyncService interpreta el resultado para contar pushed y conflicts.
	Upsert(ctx context.Context, mem *model.Memory) (*model.Memory, bool, error)

	// PullSince devuelve una página de memorias del proyecto actualizadas después de
	// 'since', ordenadas por (synced_at ASC, sync_id ASC) para paginación por keyset.
	// excludeSyncIDs filtra las memorias que acaban de ser enviadas por el cliente
	// (para no devolverlas de vuelta en el mismo sync).
	// Si since es el tiempo cero (time.Time{}), el barrido arranca desde el principio.
	//
	// cursor, si no es su valor cero (cursor.IsZero()), reanuda la paginación DESPUÉS
	// de la posición (cursor.SyncedAt, cursor.SyncID) — estrictamente mayor, en el
	// orden (synced_at, sync_id). since y cursor se combinan: since fija el punto de
	// arranque del primer sync incremental, cursor avanza páginas subsiguientes dentro
	// de ese barrido. limit acota cuántas filas se devuelven (ya clampeado por el
	// caller vía model.ClampPullLimit — este método no vuelve a clampear).
	//
	// Devuelve hasMore=true cuando existen más filas después de la última devuelta
	// (implementado internamente con un fetch de limit+1 y trim a limit). Cuando
	// hasMore es true, el caller debe construir el próximo cursor a partir del último
	// elemento devuelto (SyncedAt, SyncID) para pedir la siguiente página.
	PullSince(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, cursor model.PullCursor, limit int) (memories []*model.Memory, hasMore bool, err error)

	ApplyMemoryMutation(ctx context.Context, mutation model.MutationEnvelope) (*model.MutationApplyResult, error)
	ListMemoryMutations(ctx context.Context, project string, cursor model.MutationCursor, limit int) (*model.MutationBatch, error)
	ListActivityFeed(ctx context.Context, query model.ActivityFeedRepositoryQuery) ([]model.ActivityJournalRow, error)

	// CountByProject returns memory counts grouped by project, ordered by count DESC.
	// Soft-deleted memories (deleted_at IS NOT NULL) are excluded.
	// Returns []ProjectCount{} (not nil) when empty.
	CountByProject(ctx context.Context, filter model.MemoryFilter) ([]model.ProjectCount, error)

	// CountLiveActivity counts memories synced in the given window and returns the newest sync_id.
	// Returns count=0, newestSyncID="" when the window is empty.
	CountLiveActivity(ctx context.Context, since time.Time) (count int, newestSyncID string, err error)

	// CountGrowthByMonth returns cumulative memory counts by month (ascending) over the last N months.
	// Uses created_at (not synced_at). Returns []MonthCount{} when empty.
	CountGrowthByMonth(ctx context.Context, months int) ([]model.MonthCount, error)
}
