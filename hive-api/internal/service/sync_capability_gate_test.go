package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mutation pull used to hand every event of a project to every daemon on it,
// with no gate. The only version signal a request carries is protocol_version,
// and the daemon hardcodes it to 2 — identical for a daemon that understands
// reproject and one that does not — so the server could not tell them apart and
// sent reproject to both.
//
// A daemon that does not understand the op errors out of its apply loop, which
// aborts the batch BEFORE it advances its mutation cursor and before it acks the
// mutations it just pushed. It keeps sending, but it stops receiving teammates'
// edits and deletes, and it never durably confirms its own pushed mutations.
// Permanent, silent degradation.
//
// So the client declares what it understands, and the server withholds the rest.
// Capabilities travelled server -> client only, which is the wrong direction for
// this question.
func TestSync_Push_WithholdsReprojectFromADaemonThatDidNotDeclareIt(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pushed := model.MutationEnvelope{
		EventID: "c00e8400-e29b-41d4-a716-446655440001", EntityType: model.MutationEntityMemory,
		EntitySyncID: "c00e8400-e29b-41d4-a716-446655440101", Project: "jarvis-dev",
		Op: model.MutationOpDelete, OccurredAt: now,
	}
	remoteDelete := model.MutationEnvelope{
		EventID: "c00e8400-e29b-41d4-a716-446655440002", EntityType: model.MutationEntityMemory,
		EntitySyncID: "c00e8400-e29b-41d4-a716-446655440102", Project: "jarvis-dev",
		Op: model.MutationOpDelete, OccurredAt: now, Sequence: 7,
	}
	remoteReproject := model.MutationEnvelope{
		EventID: "c00e8400-e29b-41d4-a716-446655440003", EntityType: model.MutationEntityMemory,
		EntitySyncID: "c00e8400-e29b-41d4-a716-446655440103", Project: "jarvis-dev",
		Op: model.MutationOpReproject, OccurredAt: now, Sequence: 8,
		Reproject: &model.ReprojectPayload{FromProject: "Jarvis.Dev", ToProject: "jarvis-dev"},
	}
	next := model.MutationCursor{Sequence: 8, EventID: remoteReproject.EventID}

	mockRepo.On("ApplyMemoryMutation", ctx, pushed).
		Return(&model.MutationApplyResult{EventID: pushed.EventID, Op: pushed.Op, Applied: true}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{remoteDelete, remoteReproject}, Next: next}, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{
		Project:         "jarvis-dev",
		ProtocolVersion: model.MutationProtocolVersion,
		Mutations:       []model.MutationEnvelope{pushed},
	}, "user-1")

	require.NoError(t, err)
	require.Len(t, resp.PulledMutations, 1, "an undeclared op must not enter this daemon's pull stream")
	assert.Equal(t, remoteDelete.EventID, resp.PulledMutations[0].EventID,
		"everything the daemon does understand must keep flowing")
	require.NotNil(t, resp.NextMutationCursor)
	assert.Equal(t, next, *resp.NextMutationCursor,
		"the cursor still advances past the withheld event — withholding must not stall the stream")
}

// The other half: a daemon that declares the capability gets the event. Without
// this the gate would be a permanent mute rather than a negotiation.
func TestSync_Push_DeliversReprojectToADaemonThatDeclaredTheCapability(t *testing.T) {
	svc, mockRepo, _ := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pushed := model.MutationEnvelope{
		EventID: "c10e8400-e29b-41d4-a716-446655440001", EntityType: model.MutationEntityMemory,
		EntitySyncID: "c10e8400-e29b-41d4-a716-446655440101", Project: "jarvis-dev",
		Op: model.MutationOpDelete, OccurredAt: now,
	}
	remoteReproject := model.MutationEnvelope{
		EventID: "c10e8400-e29b-41d4-a716-446655440003", EntityType: model.MutationEntityMemory,
		EntitySyncID: "c10e8400-e29b-41d4-a716-446655440103", Project: "jarvis-dev",
		Op: model.MutationOpReproject, OccurredAt: now, Sequence: 8,
		Reproject: &model.ReprojectPayload{FromProject: "Jarvis.Dev", ToProject: "jarvis-dev"},
	}

	mockRepo.On("ApplyMemoryMutation", ctx, pushed).
		Return(&model.MutationApplyResult{EventID: pushed.EventID, Op: pushed.Op, Applied: true}, nil)
	mockRepo.On("ListMemoryMutations", ctx, "jarvis-dev", model.MutationCursor{}, 100).
		Return(&model.MutationBatch{Events: []model.MutationEnvelope{remoteReproject}}, nil)

	resp, err := svc.Push(ctx, model.SyncRequest{
		Project:          "jarvis-dev",
		ProtocolVersion:  model.MutationProtocolVersion,
		SyncCapabilities: []string{model.SyncCapabilityReproject},
		Mutations:        []model.MutationEnvelope{pushed},
	}, "user-1")

	require.NoError(t, err)
	require.Len(t, resp.PulledMutations, 1)
	assert.Equal(t, remoteReproject.EventID, resp.PulledMutations[0].EventID)
}
