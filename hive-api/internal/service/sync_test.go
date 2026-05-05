package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestSyncService(t *testing.T) (service.SyncService, *repository.MockMemoryRepository, *repository.MockPromptRepository) {
	t.Helper()
	mockMemRepo := &repository.MockMemoryRepository{}
	mockPromptRepo := &repository.MockPromptRepository{}
	svc := service.NewSyncService(mockMemRepo, mockPromptRepo)
	return svc, mockMemRepo, mockPromptRepo
}

// makePayload construye un SyncMemoryPayload mínimo para tests.
func makePayload(syncID string, updatedAt time.Time) model.SyncMemoryPayload {
	return model.SyncMemoryPayload{
		SyncID:    syncID,
		Project:   "jarvis-dev",
		Category:  model.CatDecision,
		Title:     "test",
		Content:   "test content",
		CreatedBy: "daemon-user",
		UpdatedAt: updatedAt,
	}
}

// expectedMem construye el *model.Memory que el service pasa a Upsert
// a partir de un payload dado y el userID del JWT.
// Debe coincidir EXACTAMENTE con lo que construye sync.go — si cambia la
// lógica de construcción allí, hay que actualizar esto también.
func expectedMem(payload model.SyncMemoryPayload, userID string) *model.Memory {
	return &model.Memory{
		SyncID:        payload.SyncID,
		Project:       payload.Project,
		TopicKey:      payload.TopicKey,
		Category:      payload.Category,
		Title:         payload.Title,
		Content:       payload.Content,
		Tags:          payload.Tags,
		FilesAffected: payload.FilesAffected,
		CreatedBy:     userID, // el service sobreescribe con el userID del JWT
		CreatedAt:     payload.CreatedAt,
		UpdatedAt:     payload.UpdatedAt,
		Confidence:    payload.Confidence,
		ImpactScore:   payload.ImpactScore,
	}
}

// --- Tests de Push ---

// TestSync_Push_NewMemory verifica la Rama 1: sync_id desconocido → INSERT.
func TestSync_Push_NewMemory(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()

	payload := makePayload("client-sync-id-new", time.Now())
	expected := expectedMem(payload, "user-1")
	savedMem := &model.Memory{ID: "server-uuid", SyncID: "client-sync-id-new"}

	// Upsert devuelve (savedMem, true, nil) → true = fue INSERT
	mockRepo.On("Upsert", ctx, expected).Return(savedMem, true, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{payload},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	assert.Equal(t, 0, resp.Conflicts)
	mockRepo.AssertExpectations(t)
}

// TestSync_Push_UpdateWins verifica la Rama 4: cliente tiene versión más nueva → UPDATE.
func TestSync_Push_UpdateWins(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()

	payload := makePayload("sync-id-existing", time.Now())
	expected := expectedMem(payload, "user-1")
	updatedResult := &model.Memory{ID: "server-uuid", SyncID: "sync-id-existing"}

	// Upsert devuelve (updated, false, nil) → false = fue UPDATE (no INSERT)
	mockRepo.On("Upsert", ctx, expected).Return(updatedResult, false, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{payload},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	assert.Equal(t, 0, resp.Conflicts)
	mockRepo.AssertExpectations(t)
}

// TestSync_Push_Conflict verifica las Ramas 2 y 3: el servidor rechaza la memoria del cliente.
// Upsert devuelve (nil, false, nil) → nil = servidor ganó.
func TestSync_Push_Conflict(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()

	payload := makePayload("sync-id-conflict", time.Now().Add(-1*time.Hour))
	expected := expectedMem(payload, "user-1")

	// nil como primer return → el servidor rechazó la memoria del cliente
	mockRepo.On("Upsert", ctx, expected).Return(nil, false, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{payload},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 0, resp.Pushed)
	assert.Equal(t, 1, resp.Conflicts)
	mockRepo.AssertExpectations(t)
}

// TestSync_Push_Mixed verifica el caso realista: batch con mix de inserts, updates y conflictos.
func TestSync_Push_Mixed(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()

	p1 := makePayload("id-new", time.Now())
	p2 := makePayload("id-update", time.Now())
	p3 := makePayload("id-conflict", time.Now())

	e1 := expectedMem(p1, "user-1")
	e2 := expectedMem(p2, "user-1")
	e3 := expectedMem(p3, "user-1")

	saved1 := &model.Memory{ID: "srv-1", SyncID: "id-new"}
	saved2 := &model.Memory{ID: "srv-2", SyncID: "id-update"}

	mockRepo.On("Upsert", ctx, e1).Return(saved1, true, nil)  // INSERT
	mockRepo.On("Upsert", ctx, e2).Return(saved2, false, nil) // UPDATE
	mockRepo.On("Upsert", ctx, e3).Return(nil, false, nil)    // CONFLICT

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{p1, p2, p3},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 2, resp.Pushed)    // p1 (insert) + p2 (update)
	assert.Equal(t, 1, resp.Conflicts) // p3 rechazada
	mockRepo.AssertExpectations(t)
}

// --- Tests de Pull ---

// TestSync_Pull_FirstSync verifica el primer sync (since = zero time) → devuelve todo.
func TestSync_Pull_FirstSync(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()

	serverMems := []*model.Memory{
		{ID: "srv-1", SyncID: "sync-a"},
		{ID: "srv-2", SyncID: "sync-b"},
	}

	// Primer sync: since = time.Time{} (zero value), sin IDs a excluir
	mockRepo.On("PullSince", ctx, "jarvis-dev", time.Time{}, mock.MatchedBy(func(ids []string) bool {
		return ids == nil || len(ids) == 0
	})).Return(serverMems, nil)

	mems, err := svc.Pull(ctx, "jarvis-dev", time.Time{}, nil)

	require.NoError(t, err)
	assert.Len(t, mems, 2)
	mockRepo.AssertExpectations(t)
}

// TestSync_Pull_WithExclusions verifica que los sync_ids excluidos no se devuelven.
func TestSync_Pull_WithExclusions(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()

	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	exclude := []string{"sync-a", "sync-b"}
	serverMems := []*model.Memory{
		{ID: "srv-3", SyncID: "sync-c"},
	}

	mockRepo.On("PullSince", ctx, "jarvis-dev", since, exclude).Return(serverMems, nil)

	mems, err := svc.Pull(ctx, "jarvis-dev", since, exclude)

	require.NoError(t, err)
	assert.Len(t, mems, 1)
	assert.Equal(t, "sync-c", mems[0].SyncID)
	mockRepo.AssertExpectations(t)
}

// --- Tests de prompts en Push ---

// makePromptPayload construye un SyncPromptPayload mínimo para tests.
func makePromptPayload(syncID string) model.SyncPromptPayload {
	return model.SyncPromptPayload{
		SyncID:    syncID,
		Project:   "jarvis-dev",
		Content:   "Be concise and direct",
		CreatedAt: time.Now(),
	}
}

// expectedPrompt construye el *model.Prompt que el service pasa a promptRepo.Upsert.
// Debe coincidir EXACTAMENTE con lo que construye Push() — si cambia la lógica allí,
// hay que actualizar esto también.
func expectedPrompt(payload model.SyncPromptPayload, userID string) *model.Prompt {
	return &model.Prompt{
		SyncID:    payload.SyncID,
		Project:   payload.Project,
		Content:   payload.Content,
		CreatedBy: userID,
		CreatedAt: payload.CreatedAt,
	}
}

// TestSync_Push_Prompts_ThreePrompts verifica S11: 3 prompts nuevos → PromptsPushed=3.
// Solo Upsert con saved=true (INSERT real) incrementa PromptsPushed.
func TestSync_Push_Prompts_ThreePrompts(t *testing.T) {
	svc, _, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()

	p1 := makePromptPayload("prompt-sync-id-1")
	p2 := makePromptPayload("prompt-sync-id-2")
	p3 := makePromptPayload("prompt-sync-id-3")

	e1 := expectedPrompt(p1, "user-1")
	e2 := expectedPrompt(p2, "user-1")
	e3 := expectedPrompt(p3, "user-1")

	// Todos se procesan (nuevos inserts)
	mockPromptRepo.On("Upsert", ctx, e1).Return(true, nil)
	mockPromptRepo.On("Upsert", ctx, e2).Return(true, nil)
	mockPromptRepo.On("Upsert", ctx, e3).Return(true, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{},
		Prompts:  []model.SyncPromptPayload{p1, p2, p3},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 3, resp.PromptsPushed)
	assert.Equal(t, 0, resp.Pushed)
	mockPromptRepo.AssertExpectations(t)
}

// TestSync_Push_Prompts_Zero verifica S9 (backward-compat): 0 prompts → PromptsPushed=0.
// Un daemon antiguo que no envía el campo prompts no llama a promptRepo.Upsert en absoluto.
func TestSync_Push_Prompts_Zero(t *testing.T) {
	svc, _, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()

	// Sin prompts en el request — Upsert NO debe llamarse
	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{},
		Prompts:  nil,
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 0, resp.PromptsPushed)
	// mockPromptRepo.Upsert nunca fue llamado — AssertExpectations verifica eso
	mockPromptRepo.AssertExpectations(t)
}

// TestSync_Push_Prompts_UpsertError verifica que un error en promptRepo.Upsert
// se propaga al caller sin continuar iterando.
func TestSync_Push_Prompts_UpsertError(t *testing.T) {
	svc, _, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()

	p1 := makePromptPayload("prompt-will-fail")
	e1 := expectedPrompt(p1, "user-1")

	dbErr := errors.New("connection refused")
	mockPromptRepo.On("Upsert", ctx, e1).Return(false, dbErr)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{},
		Prompts:  []model.SyncPromptPayload{p1},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.Error(t, err)
	assert.Nil(t, resp)
	mockPromptRepo.AssertExpectations(t)
}

// FIX-3 + FIX-8: When Upsert returns saved=false (duplicate), PromptsPushed must NOT increment.
// Batch: 2 new prompts (saved=true) + 1 duplicate (saved=false) → PromptsPushed=2, not 3.
func TestSync_Push_Prompts_DuplicateDoesNotIncrementCounter(t *testing.T) {
	svc, _, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()

	p1 := makePromptPayload("prompt-new-1")
	p2 := makePromptPayload("prompt-new-2")
	p3 := makePromptPayload("prompt-duplicate")

	e1 := expectedPrompt(p1, "user-1")
	e2 := expectedPrompt(p2, "user-1")
	e3 := expectedPrompt(p3, "user-1")

	// p1 and p2 are new inserts; p3 is a duplicate (ON CONFLICT DO NOTHING → saved=false)
	mockPromptRepo.On("Upsert", ctx, e1).Return(true, nil)
	mockPromptRepo.On("Upsert", ctx, e2).Return(true, nil)
	mockPromptRepo.On("Upsert", ctx, e3).Return(false, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{},
		Prompts:  []model.SyncPromptPayload{p1, p2, p3},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	// Only 2 actual inserts → PromptsPushed=2, not 3
	assert.Equal(t, 2, resp.PromptsPushed, "duplicate prompt must not increment PromptsPushed")
	mockPromptRepo.AssertExpectations(t)
}
