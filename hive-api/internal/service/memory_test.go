package service_test

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestMemoryService helper análogo al de auth.
func newTestMemoryService(t *testing.T) (service.MemoryService, *repository.MockMemoryRepository) {
	t.Helper()
	mockRepo := &repository.MockMemoryRepository{}
	svc := service.NewMemoryService(mockRepo)
	return svc, mockRepo
}

// TestCreateMemory_Success verifica que Create hace el lookup de sync_id primero
// y luego inserta cuando no existe.
func TestCreateMemory_Success(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	input := &model.Memory{
		SyncID:   "sync-abc-123",
		Project:  "jarvis-dev",
		Title:    "Test memory",
		Content:  "Contenido de prueba",
		Category: model.CatDecision,
	}
	saved := &model.Memory{
		ID:      "mem-uuid-123",
		SyncID:  "sync-abc-123",
		Project: "jarvis-dev",
		Title:   "Test memory",
		Content: "Contenido de prueba",
	}

	// GetBySyncID devuelve nil (no existe) → procedemos con Create
	mockRepo.On("GetBySyncID", ctx, "sync-abc-123").Return(nil, nil)
	mockRepo.On("Create", ctx, input).Return(saved, nil)

	result, err := svc.Create(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "mem-uuid-123", result.ID)
	mockRepo.AssertExpectations(t)
}

// TestCreateMemory_DuplicateSyncID verifica que Create devuelve el registro existente
// con ErrSyncIDExists cuando el sync_id ya existe — sin crear duplicados.
func TestCreateMemory_DuplicateSyncID(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	existing := &model.Memory{ID: "existing-uuid", SyncID: "dup-sync-id", Title: "already there"}
	input := &model.Memory{SyncID: "dup-sync-id", Title: "new attempt"}

	mockRepo.On("GetBySyncID", ctx, "dup-sync-id").Return(existing, nil)
	// Create NO debe llamarse — devolvemos el existente
	mockRepo.AssertNotCalled(t, "Create")

	result, err := svc.Create(ctx, input)

	assert.ErrorIs(t, err, service.ErrSyncIDExists)
	assert.Equal(t, "existing-uuid", result.ID)
	mockRepo.AssertExpectations(t)
}

// TestGetByID_Success verifica recuperación por ID.
func TestGetByID_Success(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	mem := &model.Memory{ID: "abc-123", Title: "Algo"}
	mockRepo.On("GetByID", ctx, "abc-123").Return(mem, nil)

	result, err := svc.GetByID(ctx, "abc-123")

	require.NoError(t, err)
	assert.Equal(t, "abc-123", result.ID)
	mockRepo.AssertExpectations(t)
}

// TestGetByID_NotFound verifica que ErrNotFound se propaga correctamente.
func TestGetByID_NotFound(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	mockRepo.On("GetByID", ctx, "no-existe").Return(nil, repository.ErrNotFound)

	result, err := svc.GetByID(ctx, "no-existe")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, repository.ErrNotFound)
	mockRepo.AssertExpectations(t)
}

// TestList_AppliesDefaultLimit verifica que si Limit=0, el service lo sustituye por 20.
// Esta es la única lógica de negocio no trivial del MemoryService.
func TestList_AppliesDefaultLimit(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	// El caller pasa Limit=0 (sin especificar)
	inputFilter := model.MemoryFilter{Project: "jarvis-dev", Limit: 0}

	// El service debe llamar al repo con Limit=20 (el default)
	expectedFilter := model.MemoryFilter{Project: "jarvis-dev", Limit: 20}

	mockRepo.On("List", ctx, expectedFilter).Return([]*model.Memory{}, nil)
	mockRepo.On("Count", ctx, expectedFilter).Return(int64(0), nil)

	result, total, err := svc.List(ctx, inputFilter)

	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, result)
	mockRepo.AssertExpectations(t)
}

// TestList_RespectsExplicitLimit verifica que un Limit explícito no se sobreescribe.
func TestList_RespectsExplicitLimit(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	filter := model.MemoryFilter{Project: "jarvis-dev", Limit: 5}
	mems := []*model.Memory{{ID: "1"}, {ID: "2"}}

	mockRepo.On("List", ctx, filter).Return(mems, nil)
	mockRepo.On("Count", ctx, filter).Return(int64(2), nil)

	result, total, err := svc.List(ctx, filter)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, int64(2), total)
	mockRepo.AssertExpectations(t)
}

// TestSearch_DelegatesToRepo verifica que Search pasa query y filter al repo.
func TestSearch_DelegatesToRepo(t *testing.T) {
	svc, mockRepo := newTestMemoryService(t)
	ctx := context.Background()

	filter := model.MemoryFilter{Project: "jarvis-dev"}
	mems := []*model.Memory{{ID: "1", Title: "Auth bug fix"}}

	mockRepo.On("Search", ctx, "auth", filter).Return(mems, nil)

	result, err := svc.Search(ctx, "auth", filter)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	mockRepo.AssertExpectations(t)
}
