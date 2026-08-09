package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSyncService_SyncRunsPrecheckWritesAndPullInsideProjectKeyTransaction(t *testing.T) {
	ctx := context.Background()
	outerMemRepo := &repository.MockMemoryRepository{}
	outerPromptRepo := &repository.MockPromptRepository{}
	outerSessionRepo := &repository.MockSessionRepository{}
	outerBlockRepo := &repository.MockProjectBlockRepository{}
	txMemRepo := &repository.MockMemoryRepository{}
	txPromptRepo := &repository.MockPromptRepository{}
	txSessionRepo := &repository.MockSessionRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txAuditRepo := &repository.MockAuditRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, txAuditRepo)
	tx.Memory = txMemRepo
	tx.Prompt = txPromptRepo
	tx.Session = txSessionRepo
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewSyncService(outerMemRepo, outerPromptRepo, outerSessionRepo, nil, outerBlockRepo, tx)
	now := time.Now().UTC()
	payload := makePayload("tx-sync-memory", now)

	txLocks.On("LockProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByProjectKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound).Once()
	txSessionRepo.On("EnsureManualSaveSession", ctx, "jarvis-dev").Return("manual-save-jarvis-dev", nil).Once()
	txMemRepo.On("Upsert", ctx, expectedMem(payload, "user-1")).Return(&model.Memory{ID: "server-id", SyncID: payload.SyncID}, true, nil).Once()
	txAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry != nil && entry.Action == model.AuditActionSyncPush && entry.EntryCount == 1
	})).Return(nil).Once()
	txSessionRepo.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit).Return([]*model.Session{{ID: "server-session", Project: "jarvis-dev"}}, false, nil).Once()
	txMemRepo.On("PullSince", ctx, "jarvis-dev", []time.Time{time.Time{}}[0], []string{payload.SyncID}, model.PullCursor{}, model.UnboundedPullLimit).Return([]*model.Memory{{ID: "remote-memory", SyncID: "remote-sync"}}, false, nil).Once()

	resp, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev", Memories: []model.SyncMemoryPayload{payload}}, "user-1")

	require.NoError(t, err)
	require.True(t, tx.Committed)
	require.Equal(t, 1, resp.Pushed)
	require.Len(t, resp.Pulled, 1)
	require.Len(t, resp.PulledSessions, 1)
	outerMemRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	outerMemRepo.AssertNotCalled(t, "PullSince", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	outerBlockRepo.AssertNotCalled(t, "GetByProjectKey", mock.Anything, mock.Anything)
}

func TestSyncService_SyncRequiresTransactionManager(t *testing.T) {
	ctx := context.Background()
	svc := service.NewSyncService(&repository.MockMemoryRepository{}, &repository.MockPromptRepository{}, &repository.MockSessionRepository{}, nil, &repository.MockProjectBlockRepository{})

	_, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev"}, "user-1")

	require.ErrorIs(t, err, service.ErrProjectBlockUnavailable)
}

func TestSyncService_SyncRollsBackWhenRequiredAuditFails(t *testing.T) {
	ctx := context.Background()
	txMemRepo := &repository.MockMemoryRepository{}
	txPromptRepo := &repository.MockPromptRepository{}
	txSessionRepo := &repository.MockSessionRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txAuditRepo := &repository.MockAuditRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, txAuditRepo)
	tx.Memory = txMemRepo
	tx.Prompt = txPromptRepo
	tx.Session = txSessionRepo
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewSyncService(&repository.MockMemoryRepository{}, &repository.MockPromptRepository{}, &repository.MockSessionRepository{}, nil, &repository.MockProjectBlockRepository{}, tx)
	now := time.Now().UTC()
	payload := makePayload("audit-failure-memory", now)

	txLocks.On("LockProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByProjectKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound).Once()
	txSessionRepo.On("EnsureManualSaveSession", ctx, "jarvis-dev").Return("manual-save-jarvis-dev", nil).Once()
	txMemRepo.On("Upsert", ctx, expectedMem(payload, "user-1")).Return(&model.Memory{ID: "server-id", SyncID: payload.SyncID}, true, nil).Once()
	txAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(errors.New("audit unavailable")).Once()

	_, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev", Memories: []model.SyncMemoryPayload{payload}}, "user-1")

	require.Error(t, err)
	require.Contains(t, err.Error(), "sync audit")
	require.True(t, tx.RolledBack)
	require.False(t, tx.Committed)
	txMemRepo.AssertNotCalled(t, "PullSince", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	txSessionRepo.AssertNotCalled(t, "ListSessionsSince", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestSyncService_SyncRequiresTransactionScopedAuditRepository(t *testing.T) {
	ctx := context.Background()
	txMemRepo := &repository.MockMemoryRepository{}
	txPromptRepo := &repository.MockPromptRepository{}
	txSessionRepo := &repository.MockSessionRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.Memory = txMemRepo
	tx.Prompt = txPromptRepo
	tx.Session = txSessionRepo
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewSyncService(&repository.MockMemoryRepository{}, &repository.MockPromptRepository{}, &repository.MockSessionRepository{}, nil, &repository.MockProjectBlockRepository{}, tx)

	_, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev"}, "user-1")

	require.ErrorIs(t, err, service.ErrProjectBlockUnavailable)
	require.True(t, tx.RolledBack)
	require.False(t, tx.Committed)
	txLocks.AssertNotCalled(t, "LockProjectKeys", mock.Anything, mock.Anything)
	txBlockRepo.AssertNotCalled(t, "GetByProjectKey", mock.Anything, mock.Anything)
	txMemRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}

func TestSyncService_SyncRollsBackWhenConcurrentBlockAppearsAfterLock(t *testing.T) {
	ctx := context.Background()
	txMemRepo := &repository.MockMemoryRepository{}
	txPromptRepo := &repository.MockPromptRepository{}
	txSessionRepo := &repository.MockSessionRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txAuditRepo := &repository.MockAuditRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, txAuditRepo)
	tx.Memory = txMemRepo
	tx.Prompt = txPromptRepo
	tx.Session = txSessionRepo
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewSyncService(&repository.MockMemoryRepository{}, &repository.MockPromptRepository{}, &repository.MockSessionRepository{}, nil, &repository.MockProjectBlockRepository{}, tx)
	block := &model.ProjectBlock{CommandID: "cmd-block", AckToken: "ack-token-tx", Project: "jarvis-dev", ProjectKey: "jarvis-dev", Reason: "blocked", BlockedAt: time.Now().UTC()}
	payload := makePayload("tx-blocked-memory", time.Now().UTC())

	txLocks.On("LockProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByProjectKey", ctx, "jarvis-dev").Return(block, nil).Once()

	_, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev", Memories: []model.SyncMemoryPayload{payload}}, "user-1")

	require.Error(t, err)
	blockedErr := &service.ProjectBlockedError{}
	require.True(t, errors.As(err, &blockedErr))
	require.Equal(t, "ack-token-tx", blockedErr.Command.AckToken)
	require.Equal(t, "blocked", blockedErr.Command.Reason)
	require.True(t, tx.RolledBack)
	require.False(t, tx.Committed)
	txSessionRepo.AssertNotCalled(t, "UpsertSession", mock.Anything, mock.Anything)
	txMemRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	txPromptRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_BlockProjectLocksProjectKeyBeforeWrites(t *testing.T) {
	ctx := context.Background()
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txAuditRepo := &repository.MockAuditRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, txAuditRepo)
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewProjectGovernanceService(&repository.MockProjectBlockRepository{}, nil, tx)
	req := model.ProjectBlockRequest{Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "jarvis-dev", ExportMarker: "export-1"}
	block := &model.ProjectBlock{CommandID: "cmd-1", Project: "jarvis-dev", ProjectKey: "jarvis-dev", Reason: req.Reason, BlockedAt: time.Now().UTC()}
	callOrder := []string{}

	txLocks.On("LockProjectKeys", ctx, []string{"jarvis-dev"}).Run(func(mock.Arguments) { callOrder = append(callOrder, "lock") }).Return(nil).Once()
	txBlockRepo.On("BlockProject", ctx, mock.AnythingOfType("model.ProjectBlockCreate")).Run(func(mock.Arguments) { callOrder = append(callOrder, "block") }).Return(block, nil).Once()
	txAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(nil).Once()

	_, err := svc.BlockProject(ctx, model.AdminActor{UserID: "admin-1"}, "jarvis-dev", req)

	require.NoError(t, err)
	require.Equal(t, []string{"lock", "block"}, callOrder)
}
