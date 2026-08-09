package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestSyncService(t *testing.T) (service.SyncService, *repository.MockMemoryRepository, *repository.MockPromptRepository) {
	t.Helper()
	mockMemRepo := &repository.MockMemoryRepository{}
	mockPromptRepo := &repository.MockPromptRepository{}
	mockSessionRepo := &repository.MockSessionRepository{}
	// Existing tests use payloads with empty session_id, so service calls EnsureManualSaveSession.
	// Maybe() allows the call to happen 0 or more times without failing expectations.
	mockSessionRepo.On("EnsureManualSaveSession", mock.Anything, mock.Anything).
		Return("manual-save-jarvis-dev", nil).Maybe()
	svc := service.NewSyncService(mockMemRepo, mockPromptRepo, mockSessionRepo, nil, nil)
	return svc, mockMemRepo, mockPromptRepo
}

func newTestSyncServiceWithSession(t *testing.T) (service.SyncService, *repository.MockMemoryRepository, *repository.MockPromptRepository, *repository.MockSessionRepository) {
	t.Helper()
	mockMemRepo := &repository.MockMemoryRepository{}
	mockPromptRepo := &repository.MockPromptRepository{}
	mockSessionRepo := &repository.MockSessionRepository{}
	svc := service.NewSyncService(mockMemRepo, mockPromptRepo, mockSessionRepo, nil, nil)
	return svc, mockMemRepo, mockPromptRepo, mockSessionRepo
}

func newTestSyncServiceWithAudit(t *testing.T) (service.SyncService, *repository.MockMemoryRepository, *repository.MockPromptRepository, *repository.MockAuditRepository) {
	t.Helper()
	mockMemRepo := &repository.MockMemoryRepository{}
	mockPromptRepo := &repository.MockPromptRepository{}
	mockSessionRepo := &repository.MockSessionRepository{}
	mockAuditRepo := &repository.MockAuditRepository{}
	mockSessionRepo.On("EnsureManualSaveSession", mock.Anything, mock.Anything).
		Return("manual-save-jarvis-dev", nil).Maybe()
	svc := service.NewSyncService(mockMemRepo, mockPromptRepo, mockSessionRepo, mockAuditRepo, nil)
	return svc, mockMemRepo, mockPromptRepo, mockAuditRepo
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
// session_id defaults to "manual-save-jarvis-dev" for payloads without explicit SessionID.
func expectedMem(payload model.SyncMemoryPayload, userID string) *model.Memory {
	var sessID *string
	if payload.SessionID != "" {
		sid := payload.SessionID
		sessID = &sid
	} else {
		defaultID := "manual-save-jarvis-dev"
		sessID = &defaultID
	}
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
		SessionID:     sessID,
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

func TestSyncService_SyncPreservesMutationResultsAfterPull(t *testing.T) {
	ctx := context.Background()
	mem := &repository.MockMemoryRepository{}
	prompt := &repository.MockPromptRepository{}
	session := &repository.MockSessionRepository{}
	blocks := &repository.MockProjectBlockRepository{}
	audit := &repository.MockAuditRepository{}
	locks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, audit)
	tx.Memory, tx.Prompt, tx.Session, tx.ProjectBlocks, tx.ProjectKeyLocks = mem, prompt, session, blocks, locks
	svc := service.NewSyncService(mem, prompt, session, nil, blocks, tx)
	mutation := model.MutationEnvelope{EventID: "event-1", EntityType: model.MutationEntityMemory, EntitySyncID: "memory-1", Project: "jarvis-dev", Op: model.MutationOpDelete}
	locks.On("LockProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil)
	blocks.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound)
	mem.On("ApplyMemoryMutation", ctx, mutation).Return(&model.MutationApplyResult{EventID: mutation.EventID, Applied: true}, nil)
	mem.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, mock.Anything).Return(nil, nil)
	audit.On("Insert", ctx, mock.Anything).Return(nil)
	session.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit).Return(nil, false, nil)
	mem.On("PullSince", ctx, "jarvis-dev", time.Time{}, []string{}, model.PullCursor{}, model.UnboundedPullLimit).Return(nil, false, nil)

	resp, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev", ProtocolVersion: model.MutationProtocolVersion, Mutations: []model.MutationEnvelope{mutation}}, "user-1")

	require.NoError(t, err)
	require.Equal(t, []model.MutationApplyResult{{EventID: mutation.EventID, Applied: true}}, resp.MutationResults)
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

func TestSync_Push_AuditFailureFailsSync(t *testing.T) {
	svc, mockRepo, mockPromptRepo, mockAuditRepo := newTestSyncServiceWithAudit(t)
	ctx := context.Background()

	payload := makePayload("sync-id-audit-best-effort", time.Now())
	expected := expectedMem(payload, "user-1")
	saved := &model.Memory{ID: "server-uuid", SyncID: payload.SyncID}
	auditErr := errors.New("audit insert failed")

	mockRepo.On("Upsert", ctx, expected).Return(saved, true, nil)
	mockPromptRepo.On("Upsert", ctx, mock.MatchedBy(func(p *model.Prompt) bool {
		return p.SyncID == "prompt-1" && p.Project == "jarvis-dev" && p.CreatedBy == "user-1"
	})).Return(true, nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionSyncPush &&
			entry.Outcome == model.AuditOutcomeSuccess &&
			entry.EntryCount == 2 &&
			entry.ActorUserID != nil && *entry.ActorUserID == "user-1" &&
			entry.Project != nil && *entry.Project == "jarvis-dev" &&
			entry.Metadata["pushed_count"] == 1 &&
			entry.Metadata["conflict_count"] == 0 &&
			entry.Metadata["prompt_count"] == 1
	})).Return(auditErr)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{payload},
		Prompts: []model.SyncPromptPayload{{
			SyncID:    "prompt-1",
			Project:   "jarvis-dev",
			Content:   "prompt content",
			CreatedAt: time.Now(),
		}},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.Error(t, err)
	require.Contains(t, err.Error(), "sync audit")
	require.Nil(t, resp)
	mockRepo.AssertExpectations(t)
	mockPromptRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSync_Push_AuditReceivesBatchCountsForPushAndConflict(t *testing.T) {
	svc, mockRepo, mockPromptRepo, mockAuditRepo := newTestSyncServiceWithAudit(t)
	ctx := context.Background()

	pushedPayload := makePayload("sync-id-pushed", time.Now())
	conflictPayload := makePayload("sync-id-conflict-audit", time.Now().Add(-time.Hour))

	mockRepo.On("Upsert", ctx, expectedMem(pushedPayload, "user-1")).
		Return(&model.Memory{ID: "server-uuid", SyncID: pushedPayload.SyncID}, true, nil)
	mockRepo.On("Upsert", ctx, expectedMem(conflictPayload, "user-1")).
		Return(nil, false, nil)
	mockPromptRepo.On("Upsert", ctx, mock.MatchedBy(func(p *model.Prompt) bool {
		return p.SyncID == "prompt-1" && p.Project == "jarvis-dev" && p.CreatedBy == "user-1"
	})).Return(true, nil)

	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionSyncPush &&
			entry.Outcome == model.AuditOutcomeSuccess &&
			entry.EntryCount == 3 &&
			entry.ReasonCode == nil &&
			entry.Metadata["pushed_count"] == 1 &&
			entry.Metadata["conflict_count"] == 1 &&
			entry.Metadata["prompt_count"] == 1 &&
			entry.Metadata["content"] == nil &&
			entry.Metadata["sync_id"] == nil &&
			entry.Metadata["title"] == nil &&
			entry.Metadata["memories"] == nil &&
			entry.Metadata["prompts"] == nil &&
			entry.Metadata["raw_payload"] == nil
	})).Return(nil).Once()
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionSyncConflict &&
			entry.Outcome == model.AuditOutcomeConflict &&
			entry.EntryCount == 1 &&
			entry.ReasonCode != nil && *entry.ReasonCode == "memory_conflict" &&
			entry.Metadata["pushed_count"] == 1 &&
			entry.Metadata["conflict_count"] == 1 &&
			entry.Metadata["prompt_count"] == 1 &&
			entry.Metadata["reason_code"] == "memory_conflict" &&
			entry.Metadata["content"] == nil &&
			entry.Metadata["sync_id"] == nil &&
			entry.Metadata["title"] == nil &&
			entry.Metadata["memories"] == nil &&
			entry.Metadata["prompts"] == nil &&
			entry.Metadata["raw_payload"] == nil
	})).Return(nil).Once()

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Memories: []model.SyncMemoryPayload{pushedPayload, conflictPayload},
		Prompts: []model.SyncPromptPayload{{
			SyncID:    "prompt-1",
			Project:   "jarvis-dev",
			Content:   "prompt content",
			CreatedAt: time.Now(),
		}},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	assert.Equal(t, 1, resp.Conflicts)
	assert.Equal(t, 1, resp.PromptsPushed)
	mockRepo.AssertExpectations(t)
	mockPromptRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

// --- Tests de PullAll (memorias) ---

// TestSync_PullAll_FirstSync verifica el primer sync (since = zero time) → devuelve todo.
func TestSync_PullAll_FirstSync(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	serverMems := []*model.Memory{
		{ID: "srv-1", SyncID: "sync-a"},
		{ID: "srv-2", SyncID: "sync-b"},
	}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, 100).Return([]*model.Session{}, false, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", time.Time{}, mock.MatchedBy(func(ids []string) bool {
		return ids == nil || len(ids) == 0
	}), model.PullCursor{}, 100).Return(serverMems, false, nil)

	res, err := svc.PullAll(ctx, "jarvis-dev", time.Time{}, nil, 100, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	assert.Len(t, res.Memories, 2)
	mockRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

// TestSync_PullAll_WithExclusions verifica que los sync_ids excluidos no se devuelven.
func TestSync_PullAll_WithExclusions(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	exclude := []string{"sync-a", "sync-b"}
	serverMems := []*model.Memory{
		{ID: "srv-3", SyncID: "sync-c"},
	}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", since, model.PullCursor{}, 100).Return([]*model.Session{}, false, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", since, exclude, model.PullCursor{}, 100).Return(serverMems, false, nil)

	res, err := svc.PullAll(ctx, "jarvis-dev", since, exclude, 100, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	assert.Len(t, res.Memories, 1)
	assert.Equal(t, "sync-c", res.Memories[0].SyncID)
	mockRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

// R2-CRIT-4 — PullAll must filter sessions by the requesting project. Without
// this, a daemon syncing project "alpha" receives sessions from "beta", leaking
// tenant data across projects.
func TestPullAll_FiltersSessionsByProject(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	// Service must invoke ListSessionsSince(ctx, "alpha", since). The mock will
	// fail (unexpected call) if the project arg is missing or wrong.
	alphaSessions := []*model.Session{{ID: "alpha-sess-1", Project: "alpha"}}
	mockSessionRepo.On("ListSessionsSince", ctx, "alpha", time.Time{}, model.PullCursor{}, 100).Return(alphaSessions, false, nil)
	mockRepo.On("PullSince", ctx, "alpha", time.Time{}, mock.Anything, model.PullCursor{}, 100).Return([]*model.Memory{}, false, nil)

	res, err := svc.PullAll(ctx, "alpha", time.Time{}, nil, 100, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	require.Len(t, res.Sessions, 1)
	assert.Equal(t, "alpha", res.Sessions[0].Project, "only alpha sessions must be returned")
	mockSessionRepo.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

// ─── PR 2a: bounded legacy pull pagination ────────────────────────────────────

// TestSync_PullAll_ForwardsLimitAndCursorsToRepositories verifies that PullAll
// threads the caller-supplied limit and per-channel cursors straight through to
// PullSince/ListSessionsSince without modification (the handler is responsible
// for clamping — PullAll must not re-clamp).
func TestSync_PullAll_ForwardsLimitAndCursorsToRepositories(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	memCursor := model.PullCursor{SyncedAt: since.Add(time.Minute), SyncID: "mem-cursor-1"}
	sessCursor := model.PullCursor{SyncedAt: since.Add(2 * time.Minute), SyncID: "sess-cursor-1"}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", since, sessCursor, 7).
		Return([]*model.Session{}, false, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", since, mock.Anything, memCursor, 7).
		Return([]*model.Memory{}, false, nil)

	_, err := svc.PullAll(ctx, "jarvis-dev", since, nil, 7, memCursor, sessCursor)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

// CRITICAL FIX #1 — TestSync_PullAll_UnboundedLimitForwardedAsIsToRepositories
// verifies that PullAll forwards model.UnboundedPullLimit (0) straight through
// to PullSince/ListSessionsSince without substituting the old 100 default. This
// is the service-layer half of the backward-compat contract: when the handler
// resolves "client did not opt into pull_limit" to UnboundedPullLimit, PullAll
// must not re-introduce a bounded page — the repositories are responsible for
// interpreting limit<=0 as "no LIMIT clause".
func TestSync_PullAll_UnboundedLimitForwardedAsIsToRepositories(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	serverMems := make([]*model.Memory, 0, 150)
	for i := 0; i < 150; i++ {
		serverMems = append(serverMems, &model.Memory{ID: fmt.Sprintf("srv-%d", i), SyncID: fmt.Sprintf("sync-%d", i)})
	}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit).
		Return([]*model.Session{}, false, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", time.Time{}, mock.Anything, model.PullCursor{}, model.UnboundedPullLimit).
		Return(serverMems, false, nil)

	res, err := svc.PullAll(ctx, "jarvis-dev", time.Time{}, nil, model.UnboundedPullLimit, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	assert.Len(t, res.Memories, 150, "unbounded pull must not be capped at the old 100-row default")
	assert.False(t, res.MemoriesHasMore, "unbounded pull always reports hasMore=false")
	assert.Nil(t, res.NextPullCursor)
	mockRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

// TestSync_PullAll_MemoriesHasMoreSetsNextCursorFromLastRow verifies that when
// the memory repository reports hasMore=true, PullAll surfaces
// MemoriesHasMore=true and builds NextPullCursor from the last returned memory's
// (SyncedAt, SyncID) — this is exactly what the daemon needs to request the
// next page.
func TestSync_PullAll_MemoriesHasMoreSetsNextCursorFromLastRow(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	lastSyncedAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	serverMems := []*model.Memory{
		{ID: "srv-1", SyncID: "sync-a", SyncedAt: lastSyncedAt.Add(-time.Minute)},
		{ID: "srv-2", SyncID: "sync-b", SyncedAt: lastSyncedAt},
	}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, 2).
		Return([]*model.Session{}, false, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", time.Time{}, mock.Anything, model.PullCursor{}, 2).
		Return(serverMems, true, nil)

	res, err := svc.PullAll(ctx, "jarvis-dev", time.Time{}, nil, 2, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	assert.True(t, res.MemoriesHasMore)
	require.NotNil(t, res.NextPullCursor)
	assert.Equal(t, "sync-b", res.NextPullCursor.SyncID)
	assert.Equal(t, lastSyncedAt, res.NextPullCursor.SyncedAt)
}

// TestSync_PullAll_SessionsHasMoreSetsNextSessionCursorFromLastRow mirrors the
// memories case for the sessions channel — independent cursor, independent
// has-more flag.
func TestSync_PullAll_SessionsHasMoreSetsNextSessionCursorFromLastRow(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	lastSyncedAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	serverSessions := []*model.Session{
		{ID: "sess-1", SyncID: "sess-sync-a", SyncedAt: lastSyncedAt.Add(-time.Minute)},
		{ID: "sess-2", SyncID: "sess-sync-b", SyncedAt: lastSyncedAt},
	}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, 2).
		Return(serverSessions, true, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", time.Time{}, mock.Anything, model.PullCursor{}, 2).
		Return([]*model.Memory{}, false, nil)

	res, err := svc.PullAll(ctx, "jarvis-dev", time.Time{}, nil, 2, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	assert.True(t, res.SessionsHasMore)
	require.NotNil(t, res.NextSessionCursor)
	assert.Equal(t, "sess-sync-b", res.NextSessionCursor.SyncID)
	assert.Equal(t, lastSyncedAt, res.NextSessionCursor.SyncedAt)
	assert.False(t, res.MemoriesHasMore)
	assert.Nil(t, res.NextPullCursor)
}

// TestSync_PullAll_NoMoreRowsOmitsNextCursors verifies the common case (backlog
// fits in one page): hasMore is false and no next cursor is built for either
// channel — this is what makes the response's omitempty JSON fields disappear
// for old and new daemons alike when there's nothing left to page through.
func TestSync_PullAll_NoMoreRowsOmitsNextCursors(t *testing.T) {
	svc, mockRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, 100).
		Return([]*model.Session{{ID: "sess-1", SyncID: "s1"}}, false, nil)
	mockRepo.On("PullSince", ctx, "jarvis-dev", time.Time{}, mock.Anything, model.PullCursor{}, 100).
		Return([]*model.Memory{{ID: "m1", SyncID: "sync-m1"}}, false, nil)

	res, err := svc.PullAll(ctx, "jarvis-dev", time.Time{}, nil, 100, model.PullCursor{}, model.PullCursor{})

	require.NoError(t, err)
	assert.False(t, res.MemoriesHasMore)
	assert.False(t, res.SessionsHasMore)
	assert.Nil(t, res.NextPullCursor)
	assert.Nil(t, res.NextSessionCursor)
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

// ─── T4.3: sessions processed before memories ─────────────────────────────────

// makeSyncSessionPayload constructs a minimal SyncSessionPayload for tests.
func makeSyncSessionPayload(id, syncID string) model.SyncSessionPayload {
	return model.SyncSessionPayload{
		ID:      id,
		SyncID:  syncID,
		Project: "jarvis-dev",
		DevID:   "dev@host",
		Client:  "claude-code",
	}
}

// TestSync_Push_SessionsUpsertedBeforeMemories verifies T4.3: sessions are processed
// before memories, satisfying the FK constraint.
// The memory uses a session_id that is in the sessions payload.
func TestSync_Push_SessionsUpsertedBeforeMemories(t *testing.T) {
	svc, mockMemRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	sess := makeSyncSessionPayload("sess-abc", "11112222-3333-4444-5555-666677778888")
	payload := makePayload("client-mem-1", time.Now())
	payload.SessionID = "sess-abc" // memory references the session in the payload

	expectedSession := &model.Session{
		ID:        sess.ID,
		SyncID:    sess.SyncID,
		Project:   sess.Project,
		Directory: sess.Directory,
		DevID:     sess.DevID,
		Client:    sess.Client,
		StartedAt: sess.StartedAt,
		EndedAt:   sess.EndedAt,
		Summary:   sess.Summary,
	}
	sessID := "sess-abc"
	expectedMemory := expectedMem(payload, "user-1")
	expectedMemory.SessionID = &sessID
	savedMem := &model.Memory{ID: "server-uuid", SyncID: "client-mem-1"}

	var callOrder []string
	mockSessionRepo.On("UpsertSession", ctx, expectedSession).
		Run(func(args mock.Arguments) { callOrder = append(callOrder, "session") }).
		Return(nil)
	mockMemRepo.On("Upsert", ctx, expectedMemory).
		Run(func(args mock.Arguments) { callOrder = append(callOrder, "memory") }).
		Return(savedMem, true, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Sessions: []model.SyncSessionPayload{sess},
		Memories: []model.SyncMemoryPayload{payload},
	}

	resp, err := svc.Push(ctx, req, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	require.Equal(t, []string{"session", "memory"}, callOrder,
		"sessions must be upserted BEFORE memories")
	mockSessionRepo.AssertExpectations(t)
	mockMemRepo.AssertExpectations(t)
}

// TestSync_Push_ManualSaveSessionUpsert verifies Decision 12: manual-save-* conflict
// uses LEAST(started_at).
func TestSync_Push_ManualSaveSessionUpsert(t *testing.T) {
	svc, _, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	sess := makeSyncSessionPayload("manual-save-jarvis-dev", "aaaabbbb-0000-0000-0000-000000000001")

	expectedSession := &model.Session{
		ID:      "manual-save-jarvis-dev",
		SyncID:  sess.SyncID,
		Project: "jarvis-dev",
		DevID:   "dev@host",
		Client:  "claude-code",
	}
	mockSessionRepo.On("UpsertSession", ctx, expectedSession).Return(nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Sessions: []model.SyncSessionPayload{sess},
		Memories: []model.SyncMemoryPayload{},
	}

	resp, err := svc.Push(ctx, req, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Pushed)
	mockSessionRepo.AssertExpectations(t)
}

// ─── T4.4: backward compat — no sessions key, no session_id → lazy create ──────

// TestSync_Push_NoSessions_NoSessionID_LazyCreatesManualSave verifies T4.4:
// a memory with no session_id AND no sessions in payload causes the server to
// lazy-create manual-save-{project} and fill memory.session_id.
func TestSync_Push_NoSessions_NoSessionID_LazyCreatesManualSave(t *testing.T) {
	svc, mockMemRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	payload := makePayload("client-mem-no-sess", time.Now())
	// memory has NO session_id set

	mockSessionRepo.On("EnsureManualSaveSession", ctx, "jarvis-dev").
		Return("manual-save-jarvis-dev", nil)

	// The memory passed to Upsert should have SessionID filled in
	manualSaveID := "manual-save-jarvis-dev"
	expectedMem := expectedMem(payload, "user-1")
	expectedMem.SessionID = &manualSaveID
	savedMem := &model.Memory{ID: "server-uuid", SyncID: "client-mem-no-sess"}
	mockMemRepo.On("Upsert", ctx, expectedMem).Return(savedMem, true, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Sessions: nil, // old daemon, no sessions key
		Memories: []model.SyncMemoryPayload{payload},
	}

	resp, err := svc.Push(ctx, req, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	mockSessionRepo.AssertExpectations(t)
	mockMemRepo.AssertExpectations(t)
}

// ─── T4.5: unknown session_id not in payload → 422 ───────────────────────────

// ─── T4.10 SC-14: regular session idempotent re-push ─────────────────────────

// TestSyncService_Push_RegularSessionRePushIdempotent verifies SC-14:
// pushing the same regular session twice triggers ON CONFLICT (sync_id) DO NOTHING
// on the second call — UpsertSession is called twice but both succeed with no error,
// and the service returns no error.
func TestSyncService_Push_RegularSessionRePushIdempotent(t *testing.T) {
	svc, _, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	sess := makeSyncSessionPayload("sess-regular-001", "12340000-0000-0000-0000-000000000001")
	expectedSession := &model.Session{
		ID:      sess.ID,
		SyncID:  sess.SyncID,
		Project: sess.Project,
		DevID:   sess.DevID,
		Client:  sess.Client,
	}

	// First push: UpsertSession is called, succeeds (INSERT)
	mockSessionRepo.On("UpsertSession", ctx, expectedSession).Return(nil).Once()
	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Sessions: []model.SyncSessionPayload{sess},
		Memories: []model.SyncMemoryPayload{},
	}
	resp, err := svc.Push(ctx, req, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Pushed)

	// Second push: UpsertSession is called again, still succeeds (ON CONFLICT DO NOTHING)
	mockSessionRepo.On("UpsertSession", ctx, expectedSession).Return(nil).Once()
	resp2, err := svc.Push(ctx, req, "user-1")
	require.NoError(t, err, "second push of same session must not error")
	assert.Equal(t, 0, resp2.Pushed)
	mockSessionRepo.AssertExpectations(t)
}

// ─── T4.9: Pull returns sessions before memories ─────────────────────────────

// TestSyncService_Pull_CallsListSessionsSince verifies T4.9: Pull calls
// sessionRepo.ListSessionsSince with the same since cutoff used for memories,
// and the returned sessions appear in PullResult.Sessions.
func TestSyncService_Pull_CallsListSessionsSince(t *testing.T) {
	svc, mockMemRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	since := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	sess1 := &model.Session{ID: "sess-from-server", Project: "jarvis-dev", DevID: "dev", Client: "claude-code"}
	mem1 := &model.Memory{ID: "srv-mem-1", SyncID: "sync-m1"}

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", since, model.PullCursor{}, 100).Return([]*model.Session{sess1}, false, nil)
	mockMemRepo.On("PullSince", ctx, "jarvis-dev", since, []string(nil), model.PullCursor{}, 100).Return([]*model.Memory{mem1}, false, nil)

	result, err := svc.PullAll(ctx, "jarvis-dev", since, nil, 100, model.PullCursor{}, model.PullCursor{})
	require.NoError(t, err)
	require.Len(t, result.Sessions, 1)
	require.Len(t, result.Memories, 1)
	assert.Equal(t, "sess-from-server", result.Sessions[0].ID)
	assert.Equal(t, "sync-m1", result.Memories[0].SyncID)
	mockSessionRepo.AssertExpectations(t)
	mockMemRepo.AssertExpectations(t)
}

// TestSyncService_Pull_EmptySessions verifies PullAll returns empty slices
// when no sessions or memories changed since the cutoff.
func TestSyncService_Pull_EmptySessions(t *testing.T) {
	svc, mockMemRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	since := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	mockSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", since, model.PullCursor{}, 100).Return([]*model.Session{}, false, nil)
	mockMemRepo.On("PullSince", ctx, "jarvis-dev", since, []string(nil), model.PullCursor{}, 100).Return([]*model.Memory{}, false, nil)

	result, err := svc.PullAll(ctx, "jarvis-dev", since, nil, 100, model.PullCursor{}, model.PullCursor{})
	require.NoError(t, err)
	assert.Empty(t, result.Sessions)
	assert.Empty(t, result.Memories)
	mockSessionRepo.AssertExpectations(t)
	mockMemRepo.AssertExpectations(t)
}

// ─── T4.11 Fix W-S4-2: legacy-pre-lifecycle-* sentinel preserved, not substituted ──

// TestSyncService_Push_LegacySentinelPreservedNotSubstituted verifies T4.11:
// a memory arriving with session_id = 'legacy-pre-lifecycle-{project}' must NOT have
// its session_id replaced with 'manual-save-{project}'. The legacy sentinel id must be
// preserved exactly.
//
// The realistic flow: the push payload includes the legacy sentinel session in sessions[],
// so it gets upserted first (FK satisfied). Then the memory uses that exact session_id.
func TestSyncService_Push_LegacySentinelPreservedNotSubstituted(t *testing.T) {
	svc, mockMemRepo, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	legacySentinelID := "legacy-pre-lifecycle-jarvis-dev"

	// The legacy sentinel session arrives in the sessions[] payload.
	sess := model.SyncSessionPayload{
		ID:      legacySentinelID,
		SyncID:  "deadbeef-0000-0000-0000-000000000001",
		Project: "jarvis-dev",
		DevID:   "legacy",
		Client:  "legacy",
	}
	payload := makePayload("mem-with-legacy-sentinel", time.Now())
	payload.SessionID = legacySentinelID

	expectedSession := &model.Session{
		ID:      sess.ID,
		SyncID:  sess.SyncID,
		Project: sess.Project,
		DevID:   sess.DevID,
		Client:  sess.Client,
	}
	// UpsertSession is called once for the legacy sentinel (from sessions[] Fase 1).
	mockSessionRepo.On("UpsertSession", ctx, expectedSession).Return(nil)

	// The memory must be stored with session_id = legacy sentinel, NOT manual-save-*.
	expectedMemory := expectedMem(payload, "user-1")
	sid := legacySentinelID
	expectedMemory.SessionID = &sid
	savedMem := &model.Memory{ID: "server-uuid", SyncID: "mem-with-legacy-sentinel"}
	mockMemRepo.On("Upsert", ctx, expectedMemory).Return(savedMem, true, nil)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Sessions: []model.SyncSessionPayload{sess},
		Memories: []model.SyncMemoryPayload{payload},
	}

	resp, err := svc.Push(ctx, req, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)

	// EnsureManualSaveSession must NOT have been called — legacy sentinel must NOT be substituted.
	mockSessionRepo.AssertNotCalled(t, "EnsureManualSaveSession", mock.Anything, mock.Anything)
	mockSessionRepo.AssertExpectations(t)
	mockMemRepo.AssertExpectations(t)
}

// TestSync_Push_UnknownSessionID_NotSentinel_ReturnsError verifies T4.5:
// a memory with a non-sentinel session_id that does NOT exist server-side
// and is NOT in the sessions payload is rejected.
func TestSync_Push_UnknownSessionID_NotSentinel_ReturnsError(t *testing.T) {
	svc, _, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	payload := makePayload("client-mem-bad-sess", time.Now())
	payload.SessionID = "some-unknown-uuid-that-does-not-exist"

	// GetSession returns ErrNotFound — the session doesn't exist
	mockSessionRepo.On("GetSession", ctx, "some-unknown-uuid-that-does-not-exist").
		Return(nil, repository.ErrNotFound)

	req := model.SyncRequest{
		Project:  "jarvis-dev",
		Sessions: nil,
		Memories: []model.SyncMemoryPayload{payload},
	}

	_, err := svc.Push(ctx, req, "user-1")
	require.Error(t, err, "unknown non-sentinel session_id must be rejected")
	mockSessionRepo.AssertExpectations(t)
}

// Suspect-B — when a memory arrives with session_id = manual-save-otherproj
// while req.Project = currentproj, the resolver must REJECT the mismatch
// instead of silently rewriting the id to manual-save-currentproj. A silent
// rewrite would mis-attribute memories across projects.
func TestSyncService_Push_ManualSaveProjectMismatchRejected(t *testing.T) {
	svc, _, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	payload := makePayload("client-mem-mismatch", time.Now())
	payload.Project = "current-proj"
	payload.SessionID = "manual-save-other-proj"

	req := model.SyncRequest{
		Project:  "current-proj",
		Sessions: nil,
		Memories: []model.SyncMemoryPayload{payload},
	}

	_, err := svc.Push(ctx, req, "user-1")
	require.Error(t, err, "manual-save-* with mismatched project must be rejected, not silently rewritten")
	require.Contains(t, err.Error(), "mismatch",
		"error must explicitly mention the project mismatch so callers can diagnose")

	// EnsureManualSaveSession must NOT have been called for current-proj — that
	// was the silent rewrite the resolver used to do.
	mockSessionRepo.AssertNotCalled(t, "EnsureManualSaveSession", mock.Anything, mock.Anything)
}

// R4-FIX-2 — closes the cross-project leak when a session in the push payload
// declares a project that DIFFERS from req.Project. Pre-fix, Fase 1 upserted
// the session anyway and resolveSessionID short-circuited via inPayload[id],
// allowing a malicious daemon to attribute a session to one project while the
// memory referenced it from another. The fix validates project equality BEFORE
// calling UpsertSession and returns ErrSessionProjectMismatch.
func TestSyncService_Push_SessionInPayloadProjectMismatch_Rejected(t *testing.T) {
	svc, _, _, mockSessionRepo := newTestSyncServiceWithSession(t)
	ctx := context.Background()

	// Session payload claims project=alpha, but the request says project=beta.
	leak := makeSyncSessionPayload("X", "deadbeef-cafe-0000-0000-000000000001")
	leak.Project = "alpha"

	req := model.SyncRequest{
		Project:  "beta",
		Sessions: []model.SyncSessionPayload{leak},
		Memories: []model.SyncMemoryPayload{},
	}

	_, err := svc.Push(ctx, req, "user-1")
	require.Error(t, err, "session payload with project != req.Project must be rejected")
	require.True(t, errors.Is(err, service.ErrSessionProjectMismatch),
		"error must wrap ErrSessionProjectMismatch so handler maps to 400")

	// UpsertSession MUST NOT have been called — validation runs first to avoid
	// any partial commit of the cross-project session row.
	mockSessionRepo.AssertNotCalled(t, "UpsertSession", mock.Anything, mock.Anything)
}

func TestSyncService_Push_MutationProtocolV2AppliesBatchAndPreservesLegacyCounts(t *testing.T) {
	svc, mockRepo, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	legacyPayload := makePayload("legacy-sync-id", now)

	mutation := model.MutationEnvelope{
		EventID:      "950e8400-e29b-41d4-a716-446655440001",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: "950e8400-e29b-41d4-a716-446655440101",
		Project:      "jarvis-dev",
		Op:           model.MutationOpDelete,
		OccurredAt:   now,
		Tombstone:    &model.TombstonePayload{DeletedAt: now, DeletedBy: "daemon-user", Reason: "obsolete"},
	}
	mockRepo.On("ApplyMemoryMutation", ctx, mutation).
		Return(&model.MutationApplyResult{EventID: mutation.EventID, Op: model.MutationOpDelete, Applied: true}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{Sequence: 41, EventID: "previous"}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{{EventID: "remote-event", Op: model.MutationOpUpdate, Sequence: 42}}, Next: model.MutationCursor{Sequence: 42, EventID: "remote-event"}}, nil)

	req := model.SyncRequest{
		Project:         "jarvis-dev",
		ProtocolVersion: 2,
		MutationCursor:  &model.MutationCursor{Sequence: 41, EventID: "previous"},
		Memories:        []model.SyncMemoryPayload{legacyPayload},
		Mutations:       []model.MutationEnvelope{mutation},
	}

	resp, err := svc.Push(ctx, req, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed, "v2 mutations are authoritative; legacy memories are skipped when mutations are present")
	assert.Equal(t, 0, resp.Conflicts)
	assert.Equal(t, model.CompatibilityModeMutationV2, resp.CompatibilityMode)
	require.NotNil(t, resp.NextMutationCursor)
	assert.Equal(t, int64(42), resp.NextMutationCursor.Sequence)
	require.Len(t, resp.PulledMutations, 1)
	assert.Equal(t, "remote-event", resp.PulledMutations[0].EventID)
	mockRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
	mockPromptRepo.AssertExpectations(t)
}

func TestSyncService_Push_MutationProtocolV2EmptyMutationsReturnsLegacyMode(t *testing.T) {
	svc, mockRepo, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	legacyPayload := makePayload("legacy-empty-mutations-sync-id", now)

	mockRepo.On("Upsert", ctx, expectedMem(legacyPayload, "user-1")).
		Return(&model.Memory{ID: "server-uuid", SyncID: legacyPayload.SyncID}, true, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{
		Project:         "jarvis-dev",
		ProtocolVersion: model.MutationProtocolVersion,
		MutationCursor:  &model.MutationCursor{Sequence: 41, EventID: "previous"},
		Memories:        []model.SyncMemoryPayload{legacyPayload},
		Mutations:       []model.MutationEnvelope{},
	}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	assert.Equal(t, 0, resp.Conflicts)
	assert.Equal(t, model.CompatibilityModeLegacy, resp.CompatibilityMode, "API must report the legacy path it actually processed so daemons ack legacy rows")
	assert.Nil(t, resp.NextMutationCursor)
	assert.Empty(t, resp.PulledMutations)
	mockRepo.AssertNotCalled(t, "ApplyMemoryMutation", mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "ListMemoryMutations", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
	mockPromptRepo.AssertExpectations(t)
}

func TestSyncService_Push_MutationProtocolV2SkipsLegacyRowsWhenMutationRejectsTombstone(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	legacyPayload := makePayload("tombstoned-sync-id", now.Add(2*time.Minute))
	legacyPayload.Title = "stale row title"
	legacyPayload.Content = "stale row content"

	mutation := model.MutationEnvelope{
		EventID:      "960e8400-e29b-41d4-a716-446655440001",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: legacyPayload.SyncID,
		Project:      "jarvis-dev",
		Op:           model.MutationOpUpdate,
		OccurredAt:   now.Add(2 * time.Minute),
		Memory:       &model.MemoryPayload{SyncID: legacyPayload.SyncID, Project: "jarvis-dev", Category: model.CatDecision, Title: "stale row title", Content: "stale row content", UpdatedAt: now.Add(2 * time.Minute)},
	}
	appliedMutation := model.MutationEnvelope{
		EventID:      "960e8400-e29b-41d4-a716-446655440002",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: "960e8400-e29b-41d4-a716-446655440102",
		Project:      "jarvis-dev",
		Op:           model.MutationOpDelete,
		OccurredAt:   now.Add(3 * time.Minute),
	}
	mockRepo.On("ApplyMemoryMutation", ctx, mutation).Return(nil, repository.ErrMemoryTombstoned)
	mockRepo.On("ApplyMemoryMutation", ctx, appliedMutation).
		Return(&model.MutationApplyResult{EventID: appliedMutation.EventID, Op: appliedMutation.Op, Applied: true}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{}, Next: model.MutationCursor{}}, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{
		Project:         "jarvis-dev",
		ProtocolVersion: model.MutationProtocolVersion,
		Memories:        []model.SyncMemoryPayload{legacyPayload},
		Mutations:       []model.MutationEnvelope{mutation, appliedMutation},
	}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	assert.Equal(t, 1, resp.Conflicts, "update on tombstone must be classified as rejected/conflict")
	assert.Equal(t, model.CompatibilityModeMutationV2, resp.CompatibilityMode)
	assert.Equal(t, []model.MutationApplyResult{
		{EventID: mutation.EventID, Op: mutation.Op, Rejected: true, Reason: "mutation target is tombstoned"},
		{EventID: appliedMutation.EventID, Op: appliedMutation.Op, Applied: true},
	}, resp.MutationResults)
	mockRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestSyncService_Push_MutationProtocolV2RejectsMissingTarget(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	mutation := model.MutationEnvelope{
		EventID: "960e8400-e29b-41d4-a716-446655440003", EntityType: model.MutationEntityMemory,
		EntitySyncID: "960e8400-e29b-41d4-a716-446655440103", Project: "jarvis-dev", Op: model.MutationOpRestore,
	}
	mockRepo.On("ApplyMemoryMutation", ctx, mutation).Return(nil, repository.ErrNotFound)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{}, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{
		Project: "jarvis-dev", ProtocolVersion: model.MutationProtocolVersion, Mutations: []model.MutationEnvelope{mutation},
	}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Conflicts)
	assert.Equal(t, []model.MutationApplyResult{{
		EventID: mutation.EventID, Op: mutation.Op, Rejected: true, Reason: "mutation target was not found",
	}}, resp.MutationResults)
	mockRepo.AssertExpectations(t)
}

func TestSyncService_Push_MutationProtocolV2RejectsCrossProjectMutations(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	mutation := model.MutationEnvelope{
		EventID:      "970e8400-e29b-41d4-a716-446655440001",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: "970e8400-e29b-41d4-a716-446655440101",
		Project:      "other-project",
		Op:           model.MutationOpCreate,
		OccurredAt:   now,
		Memory:       &model.MemoryPayload{SyncID: "970e8400-e29b-41d4-a716-446655440101", Project: "other-project", Category: model.CatDecision, Title: "wrong project", Content: "must not apply", UpdatedAt: now},
	}
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{}, Next: model.MutationCursor{}}, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev", ProtocolVersion: model.MutationProtocolVersion, Mutations: []model.MutationEnvelope{mutation}}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 0, resp.Pushed)
	assert.Equal(t, 1, resp.Conflicts, "cross-project mutation must be rejected/classified")
	require.Equal(t, []model.MutationApplyResult{{
		EventID:  mutation.EventID,
		Op:       mutation.Op,
		Rejected: true,
		Reason:   "mutation project does not match sync project",
	}}, resp.MutationResults)
	mockRepo.AssertNotCalled(t, "ApplyMemoryMutation", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestSyncService_Push_MutationProtocolV2RejectsNestedMemoryProjectMismatch(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	mutation := model.MutationEnvelope{
		EventID:      "970e8400-e29b-41d4-a716-446655440002",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: "970e8400-e29b-41d4-a716-446655440102",
		Project:      "jarvis-dev",
		Op:           model.MutationOpCreate,
		OccurredAt:   now,
		Memory: &model.MemoryPayload{
			SyncID:    "970e8400-e29b-41d4-a716-446655440102",
			Project:   "other-project",
			Category:  model.CatDecision,
			Title:     "nested wrong project",
			Content:   "must not apply or journal",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{}, Next: model.MutationCursor{}}, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev", ProtocolVersion: model.MutationProtocolVersion, Mutations: []model.MutationEnvelope{mutation}}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 0, resp.Pushed)
	assert.Equal(t, 1, resp.Conflicts, "nested memory project mismatch must be classified without fake apply")
	mockRepo.AssertNotCalled(t, "ApplyMemoryMutation", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestSyncService_Push_MutationProtocolV2ClassifiesRejectedAndDuplicateEvents(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	tests := []struct {
		name      string
		result    *model.MutationApplyResult
		wantPush  int
		wantConfl int
	}{
		{
			name:      "duplicate replay is safe but not applied again",
			result:    &model.MutationApplyResult{EventID: "dup", Op: model.MutationOpDelete, Duplicate: true, Applied: false},
			wantPush:  0,
			wantConfl: 0,
		},
		{
			name:      "rejected update is counted as conflict",
			result:    &model.MutationApplyResult{EventID: "reject", Op: model.MutationOpUpdate, Applied: false, Rejected: true, Reason: "memory tombstoned"},
			wantPush:  0,
			wantConfl: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutation := model.MutationEnvelope{
				EventID:      tt.result.EventID,
				EntityType:   model.MutationEntityMemory,
				EntitySyncID: "950e8400-e29b-41d4-a716-446655440102",
				Project:      "jarvis-dev",
				Op:           tt.result.Op,
				OccurredAt:   now,
			}
			mockRepo.On("ApplyMemoryMutation", ctx, mutation).Return(tt.result, nil).Once()
			mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
				Return(&model.MutationBatch{Events: []model.MutationEnvelope{}, Next: model.MutationCursor{}}, nil).Once()

			resp, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev", ProtocolVersion: 2, Mutations: []model.MutationEnvelope{mutation}}, "user-1")

			require.NoError(t, err)
			assert.Equal(t, tt.wantPush, resp.Pushed)
			assert.Equal(t, tt.wantConfl, resp.Conflicts)
			assert.Equal(t, model.CompatibilityModeMutationV2, resp.CompatibilityMode)
			require.Len(t, resp.MutationResults, 1)
			assert.Equal(t, tt.result, &resp.MutationResults[0], "the daemon needs per-event acknowledgement for receipt correlation")
		})
	}
	mockRepo.AssertExpectations(t)
}

func TestSyncService_Push_LegacyRequestDoesNotCallMutationRepository(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	payload := makePayload("legacy-only-sync-id", time.Now())
	mockRepo.On("Upsert", ctx, expectedMem(payload, "user-1")).Return(&model.Memory{ID: "server-uuid", SyncID: payload.SyncID}, true, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev", Memories: []model.SyncMemoryPayload{payload}}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Pushed)
	assert.Empty(t, resp.CompatibilityMode)
	mockRepo.AssertNotCalled(t, "ApplyMemoryMutation", mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "ListMemoryMutations", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
