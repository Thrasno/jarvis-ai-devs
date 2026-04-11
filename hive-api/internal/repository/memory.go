package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
)

// MemoryRepository define todas las operaciones de base de datos para memorias.
//
// El método más complejo es Upsert — implementa la lógica de sincronización.
// Los demás son operaciones CRUD estándar.
type MemoryRepository interface {
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

	// PullSince devuelve las memorias del proyecto actualizadas después de 'since'.
	// excludeSyncIDs filtra las memorias que acaban de ser enviadas por el cliente
	// (para no devolverlas de vuelta en el mismo sync).
	// Si since es el tiempo cero (time.Time{}), devuelve todas las memorias del proyecto.
	PullSince(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error)
}
