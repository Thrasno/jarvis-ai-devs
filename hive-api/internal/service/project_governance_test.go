package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProjectGovernanceService_BlockProjectValidatesAndAudits(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	auditRepo := &repository.MockAuditRepository{}
	tx := repository.NewMockTxManager(nil, auditRepo)
	tx.ProjectBlocks = blockRepo
	tx.ProjectKeyLocks = &repository.MockProjectKeyLockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, auditRepo, tx)
	actor := model.AdminActor{UserID: "admin-1", Username: "admin"}
	req := model.ProjectBlockRequest{
		Action:       model.ProjectBlockActionBlock,
		Reason:       "duplicate garbage project",
		Confirmation: "jarvis-dev",
		ExportMarker: "export-2026-07-05",
	}
	blockedAt := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	tx.ProjectKeyLocks.(*repository.MockProjectKeyLockRepository).On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil)
	blockRepo.On("BlockProject", ctx, mock.MatchedBy(func(create model.ProjectBlockCreate) bool {
		return create.Project == "Jarvis Dev" &&
			create.CanonicalProjectKey == "jarvis-dev" &&
			create.ActorUserID == "admin-1" &&
			create.Reason == req.Reason &&
			create.ExportMarker == req.ExportMarker &&
			create.Action == model.ProjectBlockActionBlock
	})).Return(&model.ProjectBlock{CommandID: "cmd-1", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Action: model.ProjectBlockActionBlock, Generation: 1, Reason: req.Reason, BlockedAt: blockedAt}, nil)
	auditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionProjectBlock &&
			entry.Outcome == model.AuditOutcomeSuccess &&
			entry.ActorUserID != nil && *entry.ActorUserID == "admin-1" &&
			entry.Project != nil && *entry.Project == "jarvis-dev" &&
			entry.Metadata["action"] == model.ProjectBlockActionBlock &&
			entry.Metadata["generation"] == int64(1) &&
			entry.Metadata["actor"] == "admin-1" &&
			entry.Metadata["export_marker"] == nil
	})).Return(nil)

	got, err := svc.BlockProject(ctx, actor, "Jarvis Dev", req)
	require.NoError(t, err)
	require.Equal(t, "cmd-1", got.CommandID)
	require.Equal(t, "jarvis-dev", got.CanonicalProjectKey)
	blockRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestProjectGovernanceService_QuarantineProgressUsesReadOnlySnapshot(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = blockRepo
	blockRepo.On("QuarantineProgress", ctx, "org-repo", int64(3), "", 20).Return(model.QuarantineProgressResponse{
		CanonicalProjectKey: "org-repo", Generation: 3,
		Totals:   model.QuarantineProgressTotals{Active: 1, Acknowledged: 1},
		Progress: []model.QuarantineProgressRow{{Username: "ada", State: model.ProjectBlockAckApplied}},
	}, nil)

	got, err := service.NewProjectGovernanceService(blockRepo, nil, tx).QuarantineProgress(ctx, "Org/Repo", 3, "", 20)
	require.NoError(t, err)
	require.Equal(t, "org-repo", got.CanonicalProjectKey)
	require.Equal(t, []model.QuarantineProgressRow{{Username: "ada", State: model.ProjectBlockAckApplied}}, got.Progress)
	blockRepo.AssertExpectations(t)
}

func TestProjectGovernanceService_BlockProjectReturnsAuditFailure(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	auditRepo := &repository.MockAuditRepository{}
	tx := repository.NewMockTxManager(nil, auditRepo)
	tx.ProjectBlocks = blockRepo
	tx.ProjectKeyLocks = &repository.MockProjectKeyLockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, auditRepo, tx)
	req := model.ProjectBlockRequest{Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "jarvis-dev", ExportMarker: "export-1"}
	block := &model.ProjectBlock{CommandID: "cmd-1", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: req.Reason, BlockedAt: time.Now().UTC()}

	tx.ProjectKeyLocks.(*repository.MockProjectKeyLockRepository).On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil)
	blockRepo.On("BlockProject", ctx, mock.AnythingOfType("model.ProjectBlockCreate")).Return(block, nil)
	auditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(errors.New("audit unavailable"))

	_, err := svc.BlockProject(ctx, model.AdminActor{UserID: "admin-1"}, "Jarvis Dev", req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "audit")
}

func TestProjectGovernanceService_BlockProjectAuditFailureDoesNotLeaveActiveBlock(t *testing.T) {
	ctx := context.Background()
	outerBlockRepo := &repository.MockProjectBlockRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txAuditRepo := &repository.MockAuditRepository{}
	tx := repository.NewMockTxManager(nil, txAuditRepo)
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = &repository.MockProjectKeyLockRepository{}
	svc := service.NewProjectGovernanceService(outerBlockRepo, nil, tx)
	req := model.ProjectBlockRequest{Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "jarvis-dev", ExportMarker: "export-1"}
	block := &model.ProjectBlock{CommandID: "cmd-1", Project: "jarvis-dev", CanonicalProjectKey: "jarvis-dev", Reason: req.Reason, BlockedAt: time.Now().UTC()}

	tx.ProjectKeyLocks.(*repository.MockProjectKeyLockRepository).On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil)
	txBlockRepo.On("BlockProject", ctx, mock.AnythingOfType("model.ProjectBlockCreate")).Return(block, nil)
	txAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(errors.New("audit unavailable"))

	_, err := svc.BlockProject(ctx, model.AdminActor{UserID: "admin-1"}, "jarvis-dev", req)
	require.Error(t, err)
	require.True(t, tx.RolledBack)
	require.False(t, tx.Committed)
	outerBlockRepo.AssertNotCalled(t, "BlockProject", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_AcknowledgeValidatesStatusAndCommand(t *testing.T) {
	ctx := context.Background()
	outerBlockRepo := &repository.MockProjectBlockRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewProjectGovernanceService(outerBlockRepo, nil, tx)
	subject := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}
	ack := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-delivery", Status: model.ProjectBlockAckApplied, AckSubject: subject}
	block := &model.ProjectBlock{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev"}
	txLocks.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil).Once()
	txBlockRepo.On("GetAckDelivery", ctx, "jarvis-dev", "cmd-1", subject).Return(&model.ProjectBlockAckDelivery{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-delivery", AckSubject: subject}, nil).Once()
	txBlockRepo.On("RecordAck", ctx, mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CommandID == ack.CommandID && got.CanonicalProjectKey == ack.CanonicalProjectKey && got.AckToken == ack.AckToken && got.Status == ack.Status && got.AckSubject == subject && !got.AppliedAt.IsZero()
	})).Return(model.ProjectBlockAck{CommandID: ack.CommandID, CanonicalProjectKey: ack.CanonicalProjectKey, AckToken: ack.AckToken, Status: ack.Status, AppliedAt: time.Now().UTC()}, nil).Once()

	got, err := svc.Acknowledge(ctx, ack)
	require.NoError(t, err)
	require.Equal(t, model.ProjectBlockAckApplied, got.Status)
	require.True(t, tx.Committed)
	outerBlockRepo.AssertNotCalled(t, "GetByCanonicalKey", mock.Anything, mock.Anything)
	txBlockRepo.AssertExpectations(t)
}

func TestProjectGovernanceService_AcknowledgeCanonicalizesEquivalentProjectKey(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	locks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = blockRepo
	tx.ProjectKeyLocks = locks
	svc := service.NewProjectGovernanceService(blockRepo, nil, tx)
	ack := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: " Jarvis_Dev ", AckToken: "token", Status: model.ProjectBlockAckApplied, AckSubject: model.ProjectBlockAckSubject{AuthSubject: "user-1"}}

	locks.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(&model.ProjectBlock{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev"}, nil).Once()
	blockRepo.On("GetAckDelivery", ctx, "jarvis-dev", "cmd-1", ack.AckSubject).Return(&model.ProjectBlockAckDelivery{AckToken: "token"}, nil).Once()
	blockRepo.On("RecordAck", ctx, mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CanonicalProjectKey == "jarvis-dev"
	})).Return(model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", Status: model.ProjectBlockAckApplied}, nil).Once()

	got, err := svc.Acknowledge(ctx, ack)
	require.NoError(t, err)
	require.Equal(t, "jarvis-dev", got.CanonicalProjectKey)
	blockRepo.AssertExpectations(t)
}

func TestProjectGovernanceService_AcknowledgeRejectsDifferentSignedSubject(t *testing.T) {
	ctx := context.Background()
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewProjectGovernanceService(&repository.MockProjectBlockRepository{}, nil, tx)
	deliverySubject := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}
	attackerSubject := model.ProjectBlockAckSubject{AuthSubject: "user-2", DaemonID: "daemon-2", Client: "hive-daemon"}
	ack := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-delivery", Status: model.ProjectBlockAckApplied, AckSubject: attackerSubject}
	block := &model.ProjectBlock{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev"}
	txLocks.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil).Once()
	txBlockRepo.On("GetAckDelivery", ctx, "jarvis-dev", "cmd-1", attackerSubject).Return(nil, repository.ErrNotFound).Once()
	// A delivery exists for a different subject, but it must not authorize this caller.
	_ = deliverySubject

	_, err := svc.Acknowledge(ctx, ack)
	require.ErrorIs(t, err, service.ErrProjectBlockInvalidRequest)
	require.True(t, tx.RolledBack)
	txBlockRepo.AssertNotCalled(t, "RecordAck", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_AcknowledgeAllowsSameAccountWithDifferentDaemonMetadata(t *testing.T) {
	ctx := context.Background()
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewProjectGovernanceService(&repository.MockProjectBlockRepository{}, nil, tx)
	deliverySubject := model.ProjectBlockAckSubject{AuthSubject: "user-1"}
	ackSubject := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-after-reconfigure", Client: "hive-daemon"}
	ack := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "account-token", Status: model.ProjectBlockAckApplied, AckSubject: ackSubject}
	block := &model.ProjectBlock{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev"}
	txLocks.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil).Once()
	txBlockRepo.On("GetAckDelivery", ctx, "jarvis-dev", "cmd-1", ackSubject).Return(&model.ProjectBlockAckDelivery{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "account-token", AckSubject: deliverySubject}, nil).Once()
	txBlockRepo.On("RecordAck", ctx, mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CommandID == ack.CommandID && got.AckToken == ack.AckToken && got.AckSubject == ackSubject
	})).Return(model.ProjectBlockAck{CommandID: ack.CommandID, CanonicalProjectKey: ack.CanonicalProjectKey, AckToken: ack.AckToken, Status: ack.Status, AckSubject: ackSubject, AppliedAt: time.Now().UTC()}, nil).Once()

	got, err := svc.Acknowledge(ctx, ack)
	require.NoError(t, err)
	require.Equal(t, model.ProjectBlockAckApplied, got.Status)
	require.True(t, tx.Committed)
}

func TestProjectGovernanceService_AcknowledgeRejectsReblockedCommandUnderLock(t *testing.T) {
	ctx := context.Background()
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	svc := service.NewProjectGovernanceService(&repository.MockProjectBlockRepository{}, nil, tx)
	ack := model.ProjectBlockAck{CommandID: "old-command", CanonicalProjectKey: "jarvis-dev", AckToken: "old-token", Status: model.ProjectBlockAckApplied, AckSubject: model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}}
	reblocked := &model.ProjectBlock{CommandID: "new-command", CanonicalProjectKey: "jarvis-dev"}
	txLocks.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(reblocked, nil).Once()

	_, err := svc.Acknowledge(ctx, ack)

	require.ErrorIs(t, err, service.ErrProjectBlockInvalidRequest)
	require.True(t, tx.RolledBack)
	txBlockRepo.AssertNotCalled(t, "RecordAck", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_AcknowledgeRejectsForgedCommand(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = blockRepo
	tx.ProjectKeyLocks = &repository.MockProjectKeyLockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, nil, tx)
	ack := model.ProjectBlockAck{CommandID: "forged-cmd", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AckSubject: model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}}
	block := &model.ProjectBlock{CommandID: "real-cmd", AckToken: "ack-token-1", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate"}

	tx.ProjectKeyLocks.(*repository.MockProjectKeyLockRepository).On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil)

	_, err := svc.Acknowledge(ctx, ack)
	require.ErrorIs(t, err, service.ErrProjectBlockInvalidRequest)
	blockRepo.AssertNotCalled(t, "RecordAck", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_AcknowledgeRejectsForgedAckToken(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = blockRepo
	tx.ProjectKeyLocks = &repository.MockProjectKeyLockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, nil, tx)
	subject := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}
	ack := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "stolen-or-guessed-token", Status: model.ProjectBlockAckApplied, AckSubject: subject}
	block := &model.ProjectBlock{CommandID: "cmd-1", AckToken: "real-token-only-sync-caller-saw", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate"}

	tx.ProjectKeyLocks.(*repository.MockProjectKeyLockRepository).On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil)
	blockRepo.On("GetAckDelivery", ctx, "jarvis-dev", "cmd-1", subject).Return(&model.ProjectBlockAckDelivery{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "real-token-only-sync-caller-saw", AckSubject: subject}, nil)

	_, err := svc.Acknowledge(ctx, ack)
	require.ErrorIs(t, err, service.ErrProjectBlockInvalidRequest)
	blockRepo.AssertNotCalled(t, "RecordAck", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_StatusIncludesLatestAck(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, nil, nil)
	block := &model.ProjectBlock{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate", BlockedAt: time.Now().UTC()}
	ack := &model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied}

	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil)
	blockRepo.On("LatestAckForCommand", ctx, "jarvis-dev", "cmd-1").Return(ack, nil)

	status, err := svc.Status(ctx, "Jarvis Dev")
	require.NoError(t, err)
	require.NotNil(t, status.Ack)
	require.Equal(t, model.ProjectBlockAckApplied, status.Ack.Status)
	body, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(body)), "ack_token", "admin status must not expose ACK delivery tokens")
}

func TestProjectGovernanceService_StatusIgnoresStaleAckForPreviousCommand(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, nil, nil)
	block := &model.ProjectBlock{CommandID: "new-command", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate", BlockedAt: time.Now().UTC()}

	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil)
	blockRepo.On("LatestAckForCommand", ctx, "jarvis-dev", "new-command").Return(nil, repository.ErrNotFound)

	status, err := svc.Status(ctx, "Jarvis Dev")
	require.NoError(t, err)
	require.Nil(t, status.Ack)
	blockRepo.AssertNotCalled(t, "LatestAck", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_BlockProjectRejectsMissingGuard(t *testing.T) {
	svc := service.NewProjectGovernanceService(&repository.MockProjectBlockRepository{}, nil, nil)
	_, err := svc.BlockProject(context.Background(), model.AdminActor{UserID: "admin-1"}, "jarvis-dev", model.ProjectBlockRequest{})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrProjectBlockInvalidRequest))
}

func TestProjectGovernanceService_RejectsPurgeIntentWithoutMutation(t *testing.T) {
	blockRepo := &repository.MockProjectBlockRepository{}
	tx := repository.NewMockTxManager(nil, nil)
	tx.ProjectBlocks = blockRepo
	svc := service.NewProjectGovernanceService(blockRepo, nil, tx)

	_, err := svc.BlockProject(context.Background(), model.AdminActor{UserID: "admin-1"}, "jarvis-dev", model.ProjectBlockRequest{
		Action:       model.ProjectBlockActionPurgeIntent,
		Reason:       "not supported",
		Confirmation: "jarvis-dev",
	})

	require.ErrorIs(t, err, service.ErrProjectBlockInvalidRequest)
	require.False(t, tx.Committed)
	blockRepo.AssertNotCalled(t, "BlockProject", mock.Anything, mock.Anything)
}

func TestProjectGovernanceService_AuditRecordsTruthfulQuarantineTransition(t *testing.T) {
	ctx := context.Background()
	blockRepo := &repository.MockProjectBlockRepository{}
	auditRepo := &repository.MockAuditRepository{}
	tx := repository.NewMockTxManager(nil, auditRepo)
	tx.ProjectBlocks = blockRepo
	tx.ProjectKeyLocks = &repository.MockProjectKeyLockRepository{}
	svc := service.NewProjectGovernanceService(blockRepo, auditRepo, tx)
	req := model.ProjectBlockRequest{Action: model.ProjectBlockActionBlock, Reason: "policy", Confirmation: "jarvis-dev"}

	tx.ProjectKeyLocks.(*repository.MockProjectKeyLockRepository).On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil)
	blockRepo.On("BlockProject", ctx, mock.AnythingOfType("model.ProjectBlockCreate")).Return(&model.ProjectBlock{CommandID: "cmd-1", Project: "jarvis-dev", CanonicalProjectKey: "jarvis-dev", Action: model.ProjectBlockActionBlock, Generation: 1}, nil)
	auditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Metadata["action"] == model.ProjectBlockActionBlock &&
			entry.Metadata["generation"] == int64(1) &&
			entry.Metadata["project"] == "jarvis-dev" &&
			entry.Metadata["actor"] == "admin-1" &&
			entry.Metadata["export_marker"] == nil
	})).Return(nil)

	_, err := svc.BlockProject(ctx, model.AdminActor{UserID: "admin-1"}, "jarvis-dev", req)
	require.NoError(t, err)
	auditRepo.AssertExpectations(t)
}
