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
	//
	// filter.Project is a query, never an identity decision. Every literal is a
	// valid project: the daemon owns identity and the API keeps no whitelist, so
	// a project with no matching rows returns an empty result, not an error.
	List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, int64, error)

	// Search realiza búsqueda de texto completo en memorias.
	Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, int64, error)
}

type memoryService struct {
	repo        repository.MemoryRepository
	sessionRepo repository.SessionRepository
	blockRepo   repository.ProjectBlockRepository
	tx          repository.TxManager
}

// NewMemoryService crea un MemoryService con los repositorios inyectados.
// sessionRepo se requiere para resolver el lazy-fallback `manual-save-{project}`
// cuando el caller no envía session_id (R2-CRIT-2).
func NewMemoryService(repo repository.MemoryRepository, sessionRepo repository.SessionRepository, blockRepo repository.ProjectBlockRepository, tx ...repository.TxManager) MemoryService {
	var txManager repository.TxManager
	if len(tx) > 0 {
		txManager = tx[0]
	}
	return &memoryService{repo: repo, sessionRepo: sessionRepo, blockRepo: blockRepo, tx: txManager}
}

func (s *memoryService) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	if s.tx != nil {
		var created *model.Memory
		err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
			if repos.Memory == nil || repos.Session == nil || repos.ProjectBlocks == nil || repos.ProjectKeyLocks == nil {
				return ErrProjectBlockUnavailable
			}
			if err := repos.ProjectKeyLocks.LockProjectKeys(ctx, []string{mem.Project}); err != nil {
				return err
			}
			result, err := s.createWithRepos(ctx, mem, repos.Memory, repos.Session, repos.ProjectBlocks)
			if err != nil {
				return err
			}
			created = result
			return nil
		})
		return created, err
	}
	if s.blockRepo != nil {
		return nil, ErrProjectBlockUnavailable
	}
	return s.createWithRepos(ctx, mem, s.repo, s.sessionRepo, nil)
}

func (s *memoryService) createWithRepos(ctx context.Context, mem *model.Memory, memRepo repository.MemoryRepository, sessionRepo repository.SessionRepository, blockRepo repository.ProjectBlockRepository) (*model.Memory, error) {
	if blockRepo != nil {
		block, err := blockRepo.GetByProjectKey(ctx, mem.Project)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if block != nil {
			return nil, projectBlockedError(block)
		}
	}
	existing, err := memRepo.GetBySyncID(ctx, mem.SyncID)
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
	resolved, err := validateSessionAttribution(ctx, sessionRepo, incoming, mem.Project)
	if err != nil {
		return nil, err
	}
	mem.SessionID = &resolved

	created, err := memRepo.Create(ctx, mem)
	if errors.Is(err, repository.ErrProjectBlocked) {
		return nil, projectBlockedErrorForProject(ctx, blockRepo, mem.Project)
	}
	return created, err
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

func (s *memoryService) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, int64, error) {
	if filter.Limit == 0 {
		filter.Limit = defaultMemoryLimit
	}

	type countResult struct {
		count int64
		err   error
	}
	countCh := make(chan countResult, 1)

	go func() {
		count, err := s.repo.CountSearch(ctx, query, filter)
		countCh <- countResult{count: count, err: err}
	}()

	mems, err := s.repo.Search(ctx, query, filter)
	if err != nil {
		return nil, 0, err
	}

	cr := <-countCh
	if cr.err != nil {
		return nil, 0, cr.err
	}

	return mems, cr.count, nil
}
