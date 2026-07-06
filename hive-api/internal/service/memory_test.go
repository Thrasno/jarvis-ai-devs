package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newTestMemoryService helper análogo al de auth.
func newTestMemoryService(t *testing.T) (service.MemoryService, *repository.MockMemoryRepository) {
	t.Helper()
	mockRepo := &repository.MockMemoryRepository{}
	mockSessionRepo := &repository.MockSessionRepository{}
	// Existing tests don't trigger the lazy-fallback path; allow it to be called 0+ times.
	mockSessionRepo.On("EnsureManualSaveSession", mock.Anything, mock.Anything).
		Return("manual-save-jarvis-dev", nil).Maybe()
	svc := service.NewMemoryService(mockRepo, mockSessionRepo, nil)
	return svc, mockRepo
}

// newTestMemoryServiceWithSession exposes the session mock for R2-CRIT-2 tests.
func newTestMemoryServiceWithSession(t *testing.T) (service.MemoryService, *repository.MockMemoryRepository, *repository.MockSessionRepository) {
	t.Helper()
	mockRepo := &repository.MockMemoryRepository{}
	mockSessionRepo := &repository.MockSessionRepository{}
	svc := service.NewMemoryService(mockRepo, mockSessionRepo, nil)
	return svc, mockRepo, mockSessionRepo
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

// R2-CRIT-2 — service.Create must mirror sync resolver: empty SessionID lazily
// resolves to manual-save-{project} BEFORE repo.Create. Direct REST POST /memories
// otherwise fails the memories.session_id NOT NULL constraint.

func TestCreateMemory_WithoutSessionID_LazyCreatesManualSave(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	input := &model.Memory{
		SyncID:   "r2c2-sync-1",
		Project:  "myproj",
		Title:    "No session",
		Content:  "content",
		Category: model.CatDecision,
	}

	mockRepo.On("GetBySyncID", ctx, "r2c2-sync-1").Return(nil, nil)
	mockSessionRepo.On("EnsureManualSaveSession", ctx, "myproj").
		Return("manual-save-myproj", nil).Once()
	mockRepo.On("Create", ctx, mock.MatchedBy(func(m *model.Memory) bool {
		return m.SessionID != nil && *m.SessionID == "manual-save-myproj"
	})).Return(&model.Memory{ID: "saved-1", SessionID: stringPtrSvc("manual-save-myproj")}, nil)

	result, err := svc.Create(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result.SessionID)
	assert.Equal(t, "manual-save-myproj", *result.SessionID)
	mockSessionRepo.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestCreateMemory_WithExplicitSessionID_NoLazyCreate(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	provided := "sess-explicit-99"
	input := &model.Memory{
		SyncID:    "r2c2-sync-2",
		Project:   "myproj",
		Title:     "Explicit",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &provided,
	}

	mockRepo.On("GetBySyncID", ctx, "r2c2-sync-2").Return(nil, nil)
	// R3-FIX-2: explicit non-sentinel sessions are now validated via GetSession;
	// the session must exist AND its project must match the request project.
	mockSessionRepo.On("GetSession", ctx, provided).Return(&model.Session{
		ID:      provided,
		Project: "myproj",
	}, nil)
	// EnsureManualSaveSession MUST NOT be called when caller already supplied session_id.
	mockRepo.On("Create", ctx, mock.MatchedBy(func(m *model.Memory) bool {
		return m.SessionID != nil && *m.SessionID == "sess-explicit-99"
	})).Return(&model.Memory{ID: "saved-2", SessionID: &provided}, nil)

	_, err := svc.Create(ctx, input)
	require.NoError(t, err)
	mockSessionRepo.AssertNotCalled(t, "EnsureManualSaveSession", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

// stringPtrSvc avoids an import-cycle with the repository tests' helper.
func stringPtrSvc(s string) *string { return &s }

// R3-FIX-2 — POST /memories MUST validate that the explicit session_id belongs
// to the same project as the memory. Without this, the direct REST path leaks
// memories across projects (the sync path was already protected via
// resolveSessionID; the REST path was not).

func TestCreateMemory_ManualSaveOtherProjectRejected(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	other := "manual-save-other"
	input := &model.Memory{
		SyncID:    "r3f2-sync-1",
		Project:   "this",
		Title:     "Cross-project leak attempt",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &other,
	}

	mockRepo.On("GetBySyncID", ctx, "r3f2-sync-1").Return(nil, nil)

	_, err := svc.Create(ctx, input)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrSessionProjectMismatch)
	mockRepo.AssertNotCalled(t, "Create")
	mockSessionRepo.AssertNotCalled(t, "EnsureManualSaveSession", mock.Anything, mock.Anything)
}

func TestCreateMemory_LegacySentinelOtherProjectRejected(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	other := "legacy-pre-lifecycle-other"
	input := &model.Memory{
		SyncID:    "r3f2-sync-2",
		Project:   "this",
		Title:     "Legacy sentinel cross-project",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &other,
	}

	mockRepo.On("GetBySyncID", ctx, "r3f2-sync-2").Return(nil, nil)

	_, err := svc.Create(ctx, input)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrSessionProjectMismatch)
	mockRepo.AssertNotCalled(t, "Create")
	mockSessionRepo.AssertNotCalled(t, "EnsureManualSaveSession", mock.Anything, mock.Anything)
}

func TestCreateMemory_RegularSessionDifferentProjectRejected(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	regular := "sess-uuid-regular"
	input := &model.Memory{
		SyncID:    "r3f2-sync-3",
		Project:   "this",
		Title:     "Regular session cross-project",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &regular,
	}

	mockRepo.On("GetBySyncID", ctx, "r3f2-sync-3").Return(nil, nil)
	mockSessionRepo.On("GetSession", ctx, regular).Return(&model.Session{
		ID:      regular,
		Project: "different-project",
	}, nil)

	_, err := svc.Create(ctx, input)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrSessionProjectMismatch)
	mockRepo.AssertNotCalled(t, "Create")
}

func TestCreateMemory_ManualSaveSameProjectAccepted(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	matching := "manual-save-this"
	input := &model.Memory{
		SyncID:    "r3f2-sync-4",
		Project:   "this",
		Title:     "OK",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &matching,
	}

	mockRepo.On("GetBySyncID", ctx, "r3f2-sync-4").Return(nil, nil)
	mockSessionRepo.On("EnsureManualSaveSession", ctx, "this").Return("manual-save-this", nil).Once()
	mockRepo.On("Create", ctx, mock.MatchedBy(func(m *model.Memory) bool {
		return m.SessionID != nil && *m.SessionID == "manual-save-this"
	})).Return(&model.Memory{ID: "saved-r3f2-4"}, nil)

	_, err := svc.Create(ctx, input)
	require.NoError(t, err)
}

func TestCreateMemory_RegularSessionSameProjectAccepted(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	regular := "sess-uuid-ok"
	input := &model.Memory{
		SyncID:    "r3f2-sync-5",
		Project:   "this",
		Title:     "OK regular",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &regular,
	}

	mockRepo.On("GetBySyncID", ctx, "r3f2-sync-5").Return(nil, nil)
	mockSessionRepo.On("GetSession", ctx, regular).Return(&model.Session{
		ID:      regular,
		Project: "this",
	}, nil)
	mockRepo.On("Create", ctx, mock.MatchedBy(func(m *model.Memory) bool {
		return m.SessionID != nil && *m.SessionID == regular
	})).Return(&model.Memory{ID: "saved-r3f2-5"}, nil)

	_, err := svc.Create(ctx, input)
	require.NoError(t, err)
}

func TestCreateMemory_RegularSessionUnknownReturnsErrSessionNotFound(t *testing.T) {
	svc, mockRepo, mockSessionRepo := newTestMemoryServiceWithSession(t)
	ctx := context.Background()

	missing := "sess-uuid-missing"
	input := &model.Memory{
		SyncID:    "r3f2-sync-6",
		Project:   "this",
		Title:     "Unknown session",
		Content:   "content",
		Category:  model.CatDecision,
		SessionID: &missing,
	}

	mockRepo.On("GetBySyncID", ctx, "r3f2-sync-6").Return(nil, nil)
	mockSessionRepo.On("GetSession", ctx, missing).Return(nil, repository.ErrNotFound)

	_, err := svc.Create(ctx, input)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrSessionNotFound)
	mockRepo.AssertNotCalled(t, "Create")
}

func TestCreateMemory_UsesProjectKeyTransactionLock(t *testing.T) {
	ctx := context.Background()
	outerRepo := &repository.MockMemoryRepository{}
	txRepo := &repository.MockMemoryRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	lockRepo := &repository.MockProjectKeyLockRepository{}
	tx := &repository.MockTxManager{
		Memory:          txRepo,
		Session:         sessionRepo,
		ProjectBlocks:   blockRepo,
		ProjectKeyLocks: lockRepo,
	}
	svc := service.NewMemoryService(outerRepo, sessionRepo, blockRepo, tx)

	input := &model.Memory{
		SyncID:   "direct-create-lock-1",
		Project:  "Jarvis Dev",
		Title:    "Locked direct create",
		Content:  "content",
		Category: model.CatDecision,
	}

	lockRepo.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound).Once()
	txRepo.On("GetBySyncID", ctx, "direct-create-lock-1").Return(nil, nil).Once()
	sessionRepo.On("EnsureManualSaveSession", ctx, "Jarvis Dev").Return("manual-save-jarvis-dev", nil).Once()
	txRepo.On("Create", ctx, mock.MatchedBy(func(m *model.Memory) bool {
		return m.SyncID == "direct-create-lock-1" && m.SessionID != nil && *m.SessionID == "manual-save-jarvis-dev"
	})).Return(&model.Memory{ID: "created-direct-lock"}, nil).Once()

	created, err := svc.Create(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "created-direct-lock", created.ID)
	require.True(t, tx.Committed)
	outerRepo.AssertNotCalled(t, "GetBySyncID", mock.Anything, mock.Anything)
	outerRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	lockRepo.AssertExpectations(t)
	blockRepo.AssertExpectations(t)
	txRepo.AssertExpectations(t)
	sessionRepo.AssertExpectations(t)
}

func TestCreateMemory_RejectsBlockedProjectInsideProjectKeyLock(t *testing.T) {
	ctx := context.Background()
	outerRepo := &repository.MockMemoryRepository{}
	txRepo := &repository.MockMemoryRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	lockRepo := &repository.MockProjectKeyLockRepository{}
	tx := &repository.MockTxManager{
		Memory:          txRepo,
		Session:         sessionRepo,
		ProjectBlocks:   blockRepo,
		ProjectKeyLocks: lockRepo,
	}
	svc := service.NewMemoryService(outerRepo, sessionRepo, blockRepo, tx)

	block := &model.ProjectBlock{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		CommandID:           "command-1",
		Blocked:             true,
		BlockedAt:           time.Now().UTC(),
	}
	lockRepo.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil).Once()

	_, err := svc.Create(ctx, &model.Memory{SyncID: "blocked-direct", Project: "Jarvis Dev", Category: model.CatDecision})
	require.Error(t, err)
	require.True(t, tx.RolledBack)
	txRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	outerRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	lockRepo.AssertExpectations(t)
	blockRepo.AssertExpectations(t)
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
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 1, 31, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
	category := model.CatDecision

	filter := model.MemoryFilter{Project: "jarvis-dev", Category: &category, CreatedFrom: &from, CreatedUntil: &until, Limit: 5, Offset: 1}
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
	expectedFilter := model.MemoryFilter{Project: "jarvis-dev", Limit: 20}
	mems := []*model.Memory{{ID: "1", Title: "Auth bug fix"}}

	mockRepo.On("Search", ctx, "auth", expectedFilter).Return(mems, nil)
	mockRepo.On("CountSearch", ctx, "auth", expectedFilter).Return(int64(7), nil)

	result, total, err := svc.Search(ctx, "auth", filter)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, int64(7), total)
	mockRepo.AssertExpectations(t)
}
