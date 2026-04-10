package service

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
)

// SyncService gestiona la sincronización bidireccional entre el daemon local
// y el servidor central.
//
// Protocolo de sync:
//   1. El daemon envía sus memorias locales (Push).
//      El servidor decide cuáles acepta según el algoritmo de 4 ramas.
//   2. El servidor envía las memorias que el daemon no tiene (Pull).
//      El daemon las guarda en su SQLite local.
//
// Push y Pull son independientes: el daemon puede hacer solo Push, solo Pull,
// o ambos en secuencia. El orden recomendado es Push primero, luego Pull,
// para que el Pull excluya las memorias que acaban de ser enviadas.
type SyncService interface {
	// Push recibe un batch de memorias del daemon y las persiste en el servidor.
	// Devuelve estadísticas: cuántas se guardaron y cuántas generaron conflicto.
	//
	// La lógica de resolución de conflictos está en MemoryRepository.Upsert.
	// SyncService interpreta el resultado para contar pushed vs conflicts.
	Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error)

	// Pull devuelve las memorias del servidor actualizadas después de 'since'.
	// excludeSyncIDs evita devolver memorias que el daemon acaba de enviar.
	Pull(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error)
}

type syncService struct {
	repo repository.MemoryRepository
}

// NewSyncService crea el SyncService con el repositorio inyectado.
func NewSyncService(repo repository.MemoryRepository) SyncService {
	return &syncService{repo: repo}
}

// Push procesa el batch de memorias del cliente.
//
// Iteramos cada memoria y llamamos a repo.Upsert. El repositorio implementa
// las 4 ramas del algoritmo. Nosotros solo contamos los resultados:
//
//   Upsert devuelve (mem, true,  nil) → fue INSERT       → pushed++
//   Upsert devuelve (mem, false, nil) → fue UPDATE       → pushed++
//   Upsert devuelve (nil, false, nil) → fue SKIP         → conflicts++
//   Upsert devuelve (_,   _,    err)  → error de BD      → propagamos error
//
// El campo CreatedBy se asigna aquí, en el service — el repositorio no sabe
// quién está haciendo el sync. Ese dato viene del JWT (validado por el middleware).
func (s *syncService) Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	var pushed, conflicts int

	for _, payload := range req.Memories {
		// Construimos el model.Memory a partir del payload del cliente.
		// El payload tiene los campos que envía el daemon; lo adaptamos
		// al modelo interno del servidor.
		mem := &model.Memory{
			SyncID:        payload.SyncID,
			Project:       payload.Project,
			TopicKey:      payload.TopicKey,
			Category:      payload.Category,
			Title:         payload.Title,
			Content:       payload.Content,
			Tags:          payload.Tags,
			FilesAffected: payload.FilesAffected,
			CreatedBy:     userID, // sobreescribimos con el ID del JWT, no el del payload
			CreatedAt:     payload.CreatedAt,
			UpdatedAt:     payload.UpdatedAt,
			Confidence:    payload.Confidence,
			ImpactScore:   payload.ImpactScore,
		}

		saved, wasInsert, err := s.repo.Upsert(ctx, mem)
		if err != nil {
			return nil, err
		}

		if saved == nil {
			// nil → el servidor rechazó la memoria (Ramas 2 y 3).
			// El daemon sabe que su versión fue ignorada.
			conflicts++
		} else {
			// Non-nil → la memoria fue guardada (Ramas 1 y 4).
			// wasInsert distingue INSERT de UPDATE, pero ambos cuentan como pushed.
			_ = wasInsert
			pushed++
		}
	}

	return &model.SyncResponse{
		Pushed:    pushed,
		Conflicts: conflicts,
	}, nil
}

// Pull delega directamente al repositorio.
// No tiene lógica adicional — la complejidad está en el repo (filtrado por fecha y exclusiones).
func (s *syncService) Pull(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error) {
	return s.repo.PullSince(ctx, project, since, excludeSyncIDs)
}
