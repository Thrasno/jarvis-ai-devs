package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// A daemon must not send a reproject to a server that does not understand it:
// an unknown op is rejected, so the correction would be silently dropped and the
// daemon would believe it had been delivered. So the server says what it can do.
//
// It says it in a capability list, NOT by moving project_identity_version. That
// field is a strict-equality handshake — the daemon errors out when the value
// differs from its own contract version — so bumping it to announce a new
// ability would break every daemon that has not been upgraded, in both
// directions. Capabilities must degrade, not fail; a list an old daemon simply
// ignores does that, a version it compares for equality does not.
func TestSync_Push_AdvertisesTheReprojectCapability(t *testing.T) {
	ctx := context.Background()
	memRepo := &repository.MockMemoryRepository{}
	promptRepo := &repository.MockPromptRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)
	blockRepo.On("GetByProjectKey", ctx, mock.Anything).Return(nil, repository.ErrNotFound)

	resp, err := svc.Push(ctx, model.SyncRequest{Project: "jarvis-dev"}, "user-1")

	require.NoError(t, err)
	require.Contains(t, resp.SyncCapabilities, model.SyncCapabilityReproject)
	require.Equal(t, projectidentity.ContractVersion, resp.ProjectIdentityVersion,
		"the identity contract version is a handshake and must not move to announce a capability")
}

// The capability has to survive the pull half too — Sync, not just Push, is what
// the daemon actually calls.
func TestSync_SyncResponseCarriesTheCapabilityThroughThePull(t *testing.T) {
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

	locks.On("LockProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil)
	blocks.On("GetByProjectKey", ctx, "jarvis-dev").Return(nil, repository.ErrNotFound)
	audit.On("Insert", ctx, mock.Anything).Return(nil)
	session.On("ListSessionsSince", ctx, "jarvis-dev", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit).Return(nil, false, nil)
	mem.On("PullSince", ctx, "jarvis-dev", time.Time{}, []string{}, model.PullCursor{}, model.UnboundedPullLimit).Return(nil, false, nil)

	resp, err := svc.Sync(ctx, model.SyncRequest{Project: "jarvis-dev"}, "user-1")

	require.NoError(t, err)
	require.Contains(t, resp.SyncCapabilities, model.SyncCapabilityReproject)
}
