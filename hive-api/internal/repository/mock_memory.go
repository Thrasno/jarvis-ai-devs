package repository

// MockMemoryRepository es el doble de test para MemoryRepository.
// Mismo patrón que MockUserRepository — embebe mock.Mock de testify.
//
// El método más interesante es Upsert, que devuelve 3 valores:
//   (*model.Memory, bool, error)
// El bool indica si fue una inserción nueva (true) o update/skip (false).
// Lo gestionamos como un argumento más en la lista de returns.

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

// MockMemoryRepository implementa MemoryRepository con datos falsos en memoria.
// Úsalo en los tests de MemoryService, SyncService y AdminService.
type MockMemoryRepository struct {
	mock.Mock
}

// Verificación en tiempo de compilación — si falta algún método de la interfaz,
// el compilador dice exactamente cuál falta. Sin esto, el error aparece en los tests.
var _ MemoryRepository = (*MockMemoryRepository)(nil)

func (m *MockMemoryRepository) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	args := m.Called(ctx, mem)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}

func (m *MockMemoryRepository) GetByID(ctx context.Context, id string) (*model.Memory, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}

func (m *MockMemoryRepository) GetBySyncID(ctx context.Context, syncID string) (*model.Memory, error) {
	args := m.Called(ctx, syncID)
	if args.Get(0) == nil {
		// GetBySyncID devuelve nil sin error cuando la memoria no existe.
		// Es la única excepción al patrón: nil + nil es válido aquí.
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}

func (m *MockMemoryRepository) List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Memory), args.Error(1)
}

func (m *MockMemoryRepository) Count(ctx context.Context, filter model.MemoryFilter) (int64, error) {
	args := m.Called(ctx, filter)
	// args.Get(0) devuelve interface{} — necesitamos cast a int64.
	// No usamos args.Int(0) porque ese método solo soporta int, no int64.
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockMemoryRepository) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error) {
	args := m.Called(ctx, query, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Memory), args.Error(1)
}

func (m *MockMemoryRepository) CountSearch(ctx context.Context, query string, filter model.MemoryFilter) (int64, error) {
	args := m.Called(ctx, query, filter)
	return args.Get(0).(int64), args.Error(1)
}

// Upsert es el más complejo porque devuelve 3 valores.
// En el test configuramos con:
//
//	mockRepo.On("Upsert", ctx, mem).Return(savedMem, true, nil)
//
// Los tres valores mapean exactamente a las 3 posiciones de args:
//
//	args.Get(0) → *model.Memory (la memoria guardada, o nil)
//	args.Bool(1) → bool (true = fue INSERT, false = UPDATE o SKIP)
//	args.Error(2) → error
func (m *MockMemoryRepository) Upsert(ctx context.Context, mem *model.Memory) (*model.Memory, bool, error) {
	args := m.Called(ctx, mem)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*model.Memory), args.Bool(1), args.Error(2)
}

func (m *MockMemoryRepository) PullSince(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, cursor model.PullCursor, limit int) ([]*model.Memory, bool, error) {
	args := m.Called(ctx, project, since, excludeSyncIDs, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).([]*model.Memory), args.Bool(1), args.Error(2)
}

func (m *MockMemoryRepository) ApplyMemoryMutation(ctx context.Context, mutation model.MutationEnvelope) (*model.MutationApplyResult, error) {
	args := m.Called(ctx, mutation)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.MutationApplyResult), args.Error(1)
}

func (m *MockMemoryRepository) ListMemoryMutations(ctx context.Context, project string, cursor model.MutationCursor, limit int) (*model.MutationBatch, error) {
	args := m.Called(ctx, project, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.MutationBatch), args.Error(1)
}

func (m *MockMemoryRepository) ListActivityFeed(ctx context.Context, query model.ActivityFeedRepositoryQuery) ([]model.ActivityJournalRow, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.ActivityJournalRow), args.Error(1)
}

func (m *MockMemoryRepository) CountByProject(ctx context.Context, filter model.MemoryFilter) ([]model.ProjectCount, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.ProjectCount), args.Error(1)
}

func (m *MockMemoryRepository) CountLiveActivity(ctx context.Context, since time.Time) (int, string, error) {
	args := m.Called(ctx, since)
	return args.Int(0), args.String(1), args.Error(2)
}

func (m *MockMemoryRepository) CountGrowthByMonth(ctx context.Context, months int) ([]model.MonthCount, error) {
	args := m.Called(ctx, months)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.MonthCount), args.Error(1)
}
