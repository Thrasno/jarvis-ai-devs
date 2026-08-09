package service_test

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The capability gate and the soft reject are both correct and both completely
// silent, which is how the whole feature can fail while every signal reports
// health.
//
// Concretely: a daemon ships declaring "reproject" instead of
// "mutation.reproject". The server withholds every reproject event from it, the
// daemon finishes its cycle reporting a clean sync, the activity feed filters
// reproject out by op, and the sync audit counts only memories, conflicts and
// prompts. Nothing anywhere says a word. Propagation is dead and the only way to
// find out is to notice, by hand, that a rename never arrived.
//
// A log line on the server is the one place that failure is observable, because
// the server is the only side that knows both what it withheld and what the
// client declared.

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})
	return &logs
}

func TestSync_Push_LogsEveryWithheldMutation(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	logs := captureLog(t)
	ctx := context.Background()
	now := time.Now().UTC()

	remoteReproject := model.MutationEnvelope{
		EventID: "d00e8400-e29b-41d4-a716-446655440003", EntityType: model.MutationEntityMemory,
		EntitySyncID: "d00e8400-e29b-41d4-a716-446655440103", Project: "jarvis-dev",
		Op: model.MutationOpReproject, OccurredAt: now, Sequence: 8,
		Reproject: &model.ReprojectPayload{FromProject: "Jarvis.Dev", ToProject: "jarvis-dev"},
	}
	remoteDelete := model.MutationEnvelope{
		EventID: "d00e8400-e29b-41d4-a716-446655440002", EntityType: model.MutationEntityMemory,
		EntitySyncID: "d00e8400-e29b-41d4-a716-446655440102", Project: "jarvis-dev",
		Op: model.MutationOpDelete, OccurredAt: now, Sequence: 7,
	}

	pushed := model.MutationEnvelope{
		EventID: "d00e8400-e29b-41d4-a716-446655440001", EntityType: model.MutationEntityMemory,
		EntitySyncID: "d00e8400-e29b-41d4-a716-446655440101", Project: "jarvis-dev",
		Op: model.MutationOpDelete, OccurredAt: now,
	}

	mockRepo.On("ApplyMemoryMutation", ctx, pushed).
		Return(&model.MutationApplyResult{EventID: pushed.EventID, Op: pushed.Op, Applied: true}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{remoteDelete, remoteReproject}}, nil)

	_, err := svc.Push(ctx, model.SyncRequest{
		Project:          "jarvis-dev",
		ProtocolVersion:  model.MutationProtocolVersion,
		SyncCapabilities: []string{"reproject"}, // the plausible typo: not "mutation.reproject"
		Mutations:        []model.MutationEnvelope{pushed},
	}, "user-1")
	require.NoError(t, err)

	output := logs.String()
	assert.Contains(t, output, "warn: withheld mutation")
	assert.Contains(t, output, string(model.MutationOpReproject), "the op is what the operator has to recognise")
	assert.Contains(t, output, remoteReproject.EventID, "so the withheld event can be found in the journal")
	assert.Contains(t, output, "jarvis-dev", "which project stopped propagating")
	assert.Contains(t, output, model.SyncCapabilityReproject, "what the client would have had to declare")
	assert.Contains(t, output, "reproject", "and what it actually declared, so a typo is visible side by side")
	assert.NotContains(t, output, remoteDelete.EventID, "an event the client can apply is not withheld and must not be logged")
}

// A daemon that declares the capability must produce no noise at all: an
// operator who greps for this line has to be able to trust that finding nothing
// means nothing was withheld.
func TestSync_Push_LogsNothingWhenEveryMutationIsDeliverable(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	logs := captureLog(t)
	ctx := context.Background()
	now := time.Now().UTC()

	remoteReproject := model.MutationEnvelope{
		EventID: "d10e8400-e29b-41d4-a716-446655440003", EntityType: model.MutationEntityMemory,
		EntitySyncID: "d10e8400-e29b-41d4-a716-446655440103", Project: "jarvis-dev",
		Op: model.MutationOpReproject, OccurredAt: now, Sequence: 8,
		Reproject: &model.ReprojectPayload{FromProject: "Jarvis.Dev", ToProject: "jarvis-dev"},
	}
	pushed := model.MutationEnvelope{
		EventID: "d10e8400-e29b-41d4-a716-446655440001", EntityType: model.MutationEntityMemory,
		EntitySyncID: "d10e8400-e29b-41d4-a716-446655440101", Project: "jarvis-dev",
		Op: model.MutationOpDelete, OccurredAt: now,
	}

	mockRepo.On("ApplyMemoryMutation", ctx, pushed).
		Return(&model.MutationApplyResult{EventID: pushed.EventID, Op: pushed.Op, Applied: true}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{remoteReproject}}, nil)

	_, err := svc.Push(ctx, model.SyncRequest{
		Project:          "jarvis-dev",
		ProtocolVersion:  model.MutationProtocolVersion,
		SyncCapabilities: []string{model.SyncCapabilityReproject},
		Mutations:        []model.MutationEnvelope{pushed},
	}, "user-1")
	require.NoError(t, err)

	assert.Empty(t, logs.String(), "a healthy sync must stay quiet")
}

// The soft reject is the right call — a hard error poisoned the whole batch —
// but it is destructive in a way the hard error was not: MarkMutationsRejected
// drops the event locally and the daemon never surfaces result.Reason, so the
// event is gone for good and nobody is told why. The server-side log is the only
// surviving record of what was discarded.
func TestSync_Push_LogsEveryRejectedMutationWithItsReason(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	logs := captureLog(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pushed := model.MutationEnvelope{
		EventID: "d20e8400-e29b-41d4-a716-446655440001", EntityType: model.MutationEntityMemory,
		EntitySyncID: "d20e8400-e29b-41d4-a716-446655440101", Project: "jarvis-dev",
		Op: model.MutationOp("mutation.reproject"), OccurredAt: now,
	}
	const reason = `unsupported memory mutation op "mutation.reproject"`

	mockRepo.On("ApplyMemoryMutation", ctx, pushed).
		Return(&model.MutationApplyResult{EventID: pushed.EventID, Op: pushed.Op, Rejected: true, Reason: reason}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{}, nil)

	_, err := svc.Push(ctx, model.SyncRequest{
		Project:         "jarvis-dev",
		ProtocolVersion: model.MutationProtocolVersion,
		Mutations:       []model.MutationEnvelope{pushed},
	}, "user-1")
	require.NoError(t, err)

	output := logs.String()
	assert.Contains(t, output, "warn: rejected mutation")
	assert.Contains(t, output, pushed.EventID, "the daemon drops this event — the id is the only way back to it")
	assert.Contains(t, output, "jarvis-dev")
	assert.Contains(t, output, reason, "the reason never reaches a human on the client side")
}

// The envelope-level mismatch is rejected before the repository ever sees it,
// and it is discarded by the same daemon path, so it needs the same line.
func TestSync_Push_LogsAProjectMismatchRejectionToo(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	logs := captureLog(t)
	ctx := context.Background()

	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{}, nil)

	_, err := svc.Push(ctx, model.SyncRequest{
		Project:         "jarvis-dev",
		ProtocolVersion: model.MutationProtocolVersion,
		Mutations: []model.MutationEnvelope{{
			EventID: "d30e8400-e29b-41d4-a716-446655440001", EntityType: model.MutationEntityMemory,
			EntitySyncID: "d30e8400-e29b-41d4-a716-446655440101", Project: "other-project",
			Op: model.MutationOpDelete, OccurredAt: time.Now().UTC(),
		}},
	}, "user-1")
	require.NoError(t, err)

	output := logs.String()
	assert.Contains(t, output, "warn: rejected mutation")
	assert.Contains(t, output, "d30e8400-e29b-41d4-a716-446655440001")
	assert.Contains(t, output, "mutation project does not match sync project")
}

// Sessions confine their relocation target to req.Project with a hard error, so
// a push cannot write a row into a project the request never claimed. Prompts
// had no equivalent check.
//
// Quarantine and the project-key lock still cover both ends — syncRequestProjects
// collects payload.Project and payload.FromProject for prompts too — so this is
// not an authorization gain. What it fixes is attribution: emitSyncAudit records
// the sync under req.Project alone, so a prompt written into (or relocated
// into) another project was booked against a project that never touched it. The
// audit trail is the thing an operator reads to answer "who moved this", and it
// was quietly wrong.
func TestSync_Push_RejectsAPromptWhoseProjectIsNotTheSyncProject(t *testing.T) {
	svc, _, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()

	_, err := svc.Push(ctx, model.SyncRequest{
		Project: "beta",
		Prompts: []model.SyncPromptPayload{{
			SyncID:    "e00e8400-e29b-41d4-a716-446655440001",
			Project:   "alpha",
			Content:   "a prompt attributed to a project this request never claimed",
			CreatedAt: time.Now().UTC(),
		}},
	}, "user-1")

	require.Error(t, err, "a prompt payload naming another project must be rejected")
	assert.ErrorIs(t, err, service.ErrPromptProjectMismatch)
	mockPromptRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}

// The relocation target is the same question in its more dangerous form: a
// from_project pointing at a third project asks the server to pull a row out of
// a project the request never named, and book it against req.Project.
func TestSync_Push_AcceptsAPromptRelocationIntoTheSyncProject(t *testing.T) {
	svc, _, mockPromptRepo := newTestSyncService(t)
	ctx := context.Background()
	createdAt := time.Now().UTC()

	mockPromptRepo.On("Upsert", ctx, mock.MatchedBy(func(p *model.Prompt) bool {
		return p.Project == "beta" && p.FromProject == "Beta.Legacy"
	})).Return(true, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{
		Project: "beta",
		Prompts: []model.SyncPromptPayload{{
			SyncID:      "e10e8400-e29b-41d4-a716-446655440001",
			Project:     "beta",
			FromProject: "Beta.Legacy",
			Content:     "a genuine identity correction still goes through",
			CreatedAt:   createdAt,
		}},
	}, "user-1")

	require.NoError(t, err)
	assert.Equal(t, 1, resp.PromptsPushed)
	mockPromptRepo.AssertExpectations(t)
}
