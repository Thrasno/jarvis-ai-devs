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

func TestSync_Push_BlockedProjectReturnsCommandWithoutPersisting(t *testing.T) {
	ctx := context.Background()
	memRepo := &repository.MockMemoryRepository{}
	promptRepo := &repository.MockPromptRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)
	blockedAt := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	block := &model.ProjectBlock{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate", BlockedAt: blockedAt}

	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil)

	_, err := svc.Push(ctx, model.SyncRequest{Project: "Jarvis Dev", Memories: []model.SyncMemoryPayload{makePayload("11111111-1111-1111-1111-111111111111", time.Now())}}, "user-1")
	require.Error(t, err)
	blockedErr := &service.ProjectBlockedError{}
	require.True(t, errors.As(err, &blockedErr))
	require.Equal(t, "cmd-1", blockedErr.Command.CommandID)
	require.Equal(t, "ack-token-1", blockedErr.Command.AckToken)
	require.Equal(t, "duplicate", blockedErr.Command.Reason)
	memRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	promptRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	sessionRepo.AssertNotCalled(t, "UpsertSession", mock.Anything, mock.Anything)
}

func TestSync_Push_AllowsUnblockedProject(t *testing.T) {
	ctx := context.Background()
	memRepo := &repository.MockMemoryRepository{}
	promptRepo := &repository.MockPromptRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)
	payload := makePayload("22222222-2222-2222-2222-222222222222", time.Now())
	expected := expectedMem(payload, "user-1")

	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound)
	sessionRepo.On("EnsureManualSaveSession", mock.Anything, mock.Anything).Return("manual-save-jarvis-dev", nil)
	memRepo.On("Upsert", ctx, expected).Return(&model.Memory{ID: "server-1", SyncID: payload.SyncID}, true, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev", Memories: []model.SyncMemoryPayload{payload}}, "user-1")
	require.NoError(t, err)
	require.Equal(t, 1, resp.Pushed)
}

func TestSync_Push_MapsRepositoryBlockedErrorToCommand(t *testing.T) {
	ctx := context.Background()
	memRepo := &repository.MockMemoryRepository{}
	promptRepo := &repository.MockPromptRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)
	payload := makePayload("33333333-3333-3333-3333-333333333333", time.Now())
	block := &model.ProjectBlock{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "jarvis-dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate", BlockedAt: time.Now().UTC()}
	expected := expectedMem(payload, "user-1")

	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound).Once()
	sessionRepo.On("EnsureManualSaveSession", mock.Anything, mock.Anything).Return("manual-save-jarvis-dev", nil)
	memRepo.On("Upsert", ctx, expected).Return(nil, false, repository.ErrProjectBlocked)
	blockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil).Once()

	_, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev", Memories: []model.SyncMemoryPayload{payload}}, "user-1")
	require.Error(t, err)
	blockedErr := &service.ProjectBlockedError{}
	require.True(t, errors.As(err, &blockedErr))
	require.Equal(t, "cmd-1", blockedErr.Command.CommandID)
	require.Equal(t, "ack-token-1", blockedErr.Command.AckToken)
	require.Equal(t, "duplicate", blockedErr.Command.Reason)
}

func TestSync_Push_PrechecksEveryPayloadProjectBeforeWriting(t *testing.T) {
	ctx := context.Background()
	memRepo := &repository.MockMemoryRepository{}
	promptRepo := &repository.MockPromptRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)
	blockedAt := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	block := &model.ProjectBlock{CommandID: "cmd-blocked", AckToken: "ack-token-blocked", Project: "Blocked Project", CanonicalProjectKey: "blocked-project", Reason: "duplicate", BlockedAt: blockedAt}
	payload := makePayload("44444444-4444-4444-4444-444444444444", time.Now())
	payload.Project = "Blocked Project"

	blockRepo.On("GetByCanonicalKey", ctx, "visible-project").Return(nil, repository.ErrNotFound).Once()
	blockRepo.On("GetByCanonicalKey", ctx, "blocked-project").Return(block, nil).Once()

	_, err := svc.Push(ctx, model.SyncRequest{
		Project:  "visible-project",
		Sessions: []model.SyncSessionPayload{{ID: "visible-session", SyncID: "visible-session", Project: "visible-project", StartedAt: time.Now()}},
		Memories: []model.SyncMemoryPayload{payload},
	}, "user-1")
	require.Error(t, err)
	blockedErr := &service.ProjectBlockedError{}
	require.True(t, errors.As(err, &blockedErr))
	require.Equal(t, "cmd-blocked", blockedErr.Command.CommandID)
	require.Equal(t, "ack-token-blocked", blockedErr.Command.AckToken)
	require.Equal(t, "duplicate", blockedErr.Command.Reason)
	sessionRepo.AssertNotCalled(t, "UpsertSession", mock.Anything, mock.Anything)
	memRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	promptRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}
