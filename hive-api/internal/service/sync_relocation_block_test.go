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

// A session or prompt re-push that names a from_project is a relocation: it
// names two projects while the request declares only one. The quarantine
// precheck must see BOTH, exactly as it does for a reproject — otherwise the
// push carries rows out of a quarantine nothing in the request mentions.
func TestSync_Push_ARelocationIsBlockedByAQuarantineOnEitherEnd(t *testing.T) {
	const source = "Foo.Bar"
	const target = "foo-bar"
	now := time.Now().UTC()

	requests := map[string]model.SyncRequest{
		"session": {
			Project: target,
			Sessions: []model.SyncSessionPayload{{
				ID:          "ba0e8400-e29b-41d4-a716-446655440001",
				SyncID:      "ba0e8400-e29b-41d4-a716-446655440101",
				Project:     target,
				FromProject: source,
				DevID:       "dev-1",
				Client:      "claude",
				StartedAt:   now,
			}},
		},
		"prompt": {
			Project: target,
			Prompts: []model.SyncPromptPayload{{
				SyncID:      "bb0e8400-e29b-41d4-a716-446655440101",
				Project:     target,
				FromProject: source,
				Content:     "a prompt",
				CreatedAt:   now,
			}},
		},
	}

	for kind, req := range requests {
		for _, blocked := range []string{source, target} {
			t.Run(kind+" blocked "+blocked, func(t *testing.T) {
				ctx := context.Background()
				memRepo := &repository.MockMemoryRepository{}
				promptRepo := &repository.MockPromptRepository{}
				sessionRepo := &repository.MockSessionRepository{}
				blockRepo := &repository.MockProjectBlockRepository{}
				svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)

				block := &model.ProjectBlock{
					CommandID:  "cmd-1",
					AckToken:   "ack-token-1",
					Project:    blocked,
					ProjectKey: blocked,
					Reason:     "duplicate",
					BlockedAt:  now,
				}
				blockRepo.On("GetByProjectKey", ctx, blocked).Return(block, nil)
				blockRepo.On("GetByProjectKey", ctx, mock.Anything).Return(nil, repository.ErrNotFound)

				_, err := svc.Push(ctx, req, "user-1")

				require.Error(t, err)
				blockedErr := &service.ProjectBlockedError{}
				require.True(t, errors.As(err, &blockedErr))
				sessionRepo.AssertNotCalled(t, "UpsertSession", mock.Anything, mock.Anything)
				promptRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
			})
		}
	}
}

// The from_project a daemon declares must reach the repository unchanged: it is
// the precondition the ON CONFLICT branch tests, so dropping it silently turns a
// correction into a no-op the daemon would never learn about.
func TestSync_Push_ForwardsTheDeclaredFromProjectToTheRepositories(t *testing.T) {
	ctx := context.Background()
	memRepo := &repository.MockMemoryRepository{}
	promptRepo := &repository.MockPromptRepository{}
	sessionRepo := &repository.MockSessionRepository{}
	blockRepo := &repository.MockProjectBlockRepository{}
	svc := service.NewSyncService(memRepo, promptRepo, sessionRepo, nil, blockRepo)
	now := time.Now().UTC()

	blockRepo.On("GetByProjectKey", ctx, mock.Anything).Return(nil, repository.ErrNotFound)
	sessionRepo.On("UpsertSession", ctx, mock.MatchedBy(func(s *model.Session) bool {
		return s.FromProject == "Foo.Bar"
	})).Return(nil)
	promptRepo.On("Upsert", ctx, mock.MatchedBy(func(p *model.Prompt) bool {
		return p.FromProject == "Foo.Bar"
	})).Return(true, nil)

	_, err := svc.Push(ctx, model.SyncRequest{
		Project: "foo-bar",
		Sessions: []model.SyncSessionPayload{{
			ID:          "bc0e8400-e29b-41d4-a716-446655440001",
			SyncID:      "bc0e8400-e29b-41d4-a716-446655440101",
			Project:     "foo-bar",
			FromProject: "Foo.Bar",
			DevID:       "dev-1",
			Client:      "claude",
			StartedAt:   now,
		}},
		Prompts: []model.SyncPromptPayload{{
			SyncID:      "bd0e8400-e29b-41d4-a716-446655440101",
			Project:     "foo-bar",
			FromProject: "Foo.Bar",
			Content:     "a prompt",
			CreatedAt:   now,
		}},
	}, "user-1")

	require.NoError(t, err)
	sessionRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
}
