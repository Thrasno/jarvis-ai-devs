package service

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
)

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
	repo repository.MemoryRepository
}

// NewMemoryService crea un MemoryService con el repositorio inyectado.
func NewMemoryService(repo repository.MemoryRepository) MemoryService {
	return &memoryService{repo: repo}
}

func (s *memoryService) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	// Sin lógica adicional por ahora — el repo se encarga de generar ID, timestamps, etc.
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
