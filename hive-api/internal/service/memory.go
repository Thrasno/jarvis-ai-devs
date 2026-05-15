package service

import (
	"context"
	"errors"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

// ErrSyncIDExists se devuelve cuando se intenta crear una memoria con un sync_id
// que ya existe. El handler lo mapea a HTTP 200 devolviendo el registro existente.
var ErrSyncIDExists = errors.New("sync_id ya existe")

// defaultMemoryLimit es cuántas memorias devolver cuando el caller no especifica.
// 20 es un número cómodo — suficiente para una pantalla, no tan grande como para
// sobrecargar la respuesta JSON.
const defaultMemoryLimit = 20

// MemoryService gestiona las operaciones sobre memorias individuales.
// Las operaciones de sincronización (push/pull) están en SyncService.
type MemoryService interface {
	// Create inserta una nueva memoria. Devuelve la memoria con el ID generado.
	Create(ctx context.Context, mem *model.Memory) (*model.Memory, error)

	// GetByID busca una memoria por su UUID de servidor.
	// Devuelve repository.ErrNotFound si no existe.
	GetByID(ctx context.Context, id string) (*model.Memory, error)

	// List devuelve memorias paginadas con el total para la paginación.
	// Si filter.Limit es 0, aplica el default (20).
	// Devuelve: memorias, total de registros que coinciden, error.
	List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, int64, error)

	// Search realiza búsqueda de texto completo en memorias.
	Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error)
}

type memoryService struct {
	repo        repository.MemoryRepository
	sessionRepo repository.SessionRepository
}

// NewMemoryService crea un MemoryService con los repositorios inyectados.
// sessionRepo se requiere para resolver el lazy-fallback `manual-save-{project}`
// cuando el caller no envía session_id (R2-CRIT-2).
func NewMemoryService(repo repository.MemoryRepository, sessionRepo repository.SessionRepository) MemoryService {
	return &memoryService{repo: repo, sessionRepo: sessionRepo}
}

func (s *memoryService) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	existing, err := s.repo.GetBySyncID(ctx, mem.SyncID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, ErrSyncIDExists
	}

	// R3-FIX-2: validate cross-project attribution. Was R2-CRIT-2's lazy-fallback
	// only — now also rejects manual-save-{other}, legacy-pre-lifecycle-{other},
	// and regular sessions whose project differs from mem.Project.
	incoming := ""
	if mem.SessionID != nil {
		incoming = *mem.SessionID
	}
	resolved, err := validateSessionAttribution(ctx, s.sessionRepo, incoming, mem.Project)
	if err != nil {
		return nil, err
	}
	mem.SessionID = &resolved

	return s.repo.Create(ctx, mem)
}

func (s *memoryService) GetByID(ctx context.Context, id string) (*model.Memory, error) {
	return s.repo.GetByID(ctx, id)
}

// List aplica el default de Limit antes de delegar al repo.
// Este es el único lugar donde vive esta regla de negocio.
func (s *memoryService) List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, int64, error) {
	if filter.Limit == 0 {
		filter.Limit = defaultMemoryLimit
	}

	// Ejecutamos List y Count en paralelo para reducir latencia.
	// En lugar de esperar que List termine para luego llamar Count,
	// los lanzamos simultáneamente.
	//
	// Concepto clave de Go: goroutines y channels.
	// Una goroutine es como un hilo ultraligero (puede haber millones).
	// Un channel es un conducto seguro para comunicar goroutines.
	//
	// "go func()" lanza una función en background inmediatamente.
	// El channel "countCh" recibirá el resultado cuando termine.
	type countResult struct {
		count int64
		err   error
	}
	countCh := make(chan countResult, 1) // canal con buffer 1 — no bloquea al escribir

	go func() {
		count, err := s.repo.Count(ctx, filter)
		countCh <- countResult{count, err} // envía resultado al canal
	}()

	// Mientras la goroutine de Count está corriendo, ejecutamos List.
	mems, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	// Ahora esperamos el resultado de Count.
	// "<-countCh" bloquea hasta que haya algo en el canal.
	cr := <-countCh
	if cr.err != nil {
		return nil, 0, cr.err
	}

	return mems, cr.count, nil
}

func (s *memoryService) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error) {
	return s.repo.Search(ctx, query, filter)
}
