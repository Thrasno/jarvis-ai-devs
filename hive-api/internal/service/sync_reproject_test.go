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

// A reproject names two projects, and a quarantine applies to a literal. Both
// ends must therefore be checked: quarantining only the project a push declares
// would let a reproject carry rows OUT of a quarantined project, and checking
// only the source would let it carry rows INTO one.
func TestSync_Push_ReprojectIsBlockedByAQuarantineOnEitherEnd(t *testing.T) {
	const source = "Foo.Bar"
	const target = "foo-bar"

	reprojectRequest := model.SyncRequest{
		Project:         target,
		ProtocolVersion: model.MutationProtocolVersion,
		Mutations: []model.MutationEnvelope{{
			EventID:      "aa0e8400-e29b-41d4-a716-446655440001",
			EntityType:   model.MutationEntityMemory,
			EntitySyncID: "aa0e8400-e29b-41d4-a716-446655440101",
			Project:      target,
			Op:           model.MutationOpReproject,
			OccurredAt:   time.Now().UTC(),
			Reproject:    &model.ReprojectPayload{FromProject: source, ToProject: target},
		}},
	}

	for _, blocked := range []string{source, target} {
		t.Run("blocked "+blocked, func(t *testing.T) {
			ctx := context.Background()
			memRepo := &repository.MockMemoryRepository{}
			promptRepo := &repository.MockPromptRepository{}
			sessionRepo := &repository.MockSessionRepository{}
			blockRepo := &repository.MockProjectBlockRepository{}
			svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)

			block := &model.ProjectBlock{
				CommandID:           "cmd-1",
				AckToken:            "ack-token-1",
				Project:             blocked,
				CanonicalProjectKey: blocked,
				Reason:              "duplicate",
				BlockedAt:           time.Now().UTC(),
			}
			blockRepo.On("GetByCanonicalKey", ctx, blocked).Return(block, nil)
			blockRepo.On("GetByCanonicalKey", ctx, mock.Anything).Return(nil, repository.ErrNotFound)

			_, err := svc.Push(ctx, reprojectRequest, "user-1")

			require.Error(t, err)
			blockedErr := &service.ProjectBlockedError{}
			require.True(t, errors.As(err, &blockedErr))
			memRepo.AssertNotCalled(t, "ApplyMemoryMutation", mock.Anything, mock.Anything)
		})
	}
}
