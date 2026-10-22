package governance_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func identityAdvanceRequest(t *testing.T, requestProject, documentProject, requestID string) governance.ApplyProgressAdvanceRequest {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: documentProject, Change: "change", BatchID: "apb-a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2", Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	require.NoError(t, err)
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: documentProject, Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	require.NoError(t, err)
	return governance.ApplyProgressAdvanceRequest{Project: requestProject, Change: "change", RequestID: requestID, Snapshot: snapshot, Batches: []applyprogress.Batch{batch}}
}

func TestServiceApplyProgressSpellingsShareOneProject(t *testing.T) {
	for _, tt := range []struct {
		key       string
		spellings []string
	}{
		{key: "myproject", spellings: []string{"MyProject", " MYPROJECT ", "myproject"}},
		{key: "my-project", spellings: []string{"my_project", "My.Project", "my-project"}},
	} {
		t.Run(tt.key, func(t *testing.T) {
			store, service := newSDDService(t)
			ctx := context.Background()
			saveSDDServiceMemory(t, store, tt.spellings[0], "sdd/change/explore", "explore")
			committed, err := service.AdvanceApplyProgress(ctx, identityAdvanceRequest(t, tt.spellings[1], tt.key, "fold-"+tt.key))
			require.NoError(t, err)
			for _, spelling := range tt.spellings {
				state, err := service.GetApplyProgress(ctx, spelling, "change")
				require.NoError(t, err, spelling)
				assert.Equal(t, committed.State.Digest, state.Digest, spelling)
				assert.Equal(t, tt.key, state.ProjectKey, spelling)
			}
		})
	}
}

func TestServiceApplyProgressRetiredAliasUsesTargetGateAndProgress(t *testing.T) {
	store, service := newSDDService(t)
	ctx := context.Background()
	saveSDDServiceMemory(t, store, "target", "sdd/change/explore", "explore")
	require.NoError(t, store.AddAlias(ctx, "retired", "target", "local", "test"))

	_, err := service.GetApplyProgress(ctx, "Retired", "change")
	require.ErrorIs(t, err, db.ErrApplyProgressNotFound)
	key, ok := governance.ProjectKeyFromError(err)
	require.True(t, ok)
	assert.Equal(t, "target", key)

	committed, err := service.AdvanceApplyProgress(ctx, identityAdvanceRequest(t, "retired", "target", "alias-genesis"))
	require.NoError(t, err)
	assert.Equal(t, "target", committed.State.ProjectKey)
	assert.Equal(t, "target", committed.Receipt.ProjectKey)

	state, err := service.GetApplyProgress(ctx, "retired", "change")
	require.NoError(t, err)
	assert.Equal(t, committed.State.Digest, state.Digest)
	evidence, err := service.GetApplyProgressEvidence(ctx, "retired", "change", "apb-a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2", state.Digest)
	require.NoError(t, err)
	assert.Equal(t, "target", evidence.Project)
	receipt, err := service.GetApplyProgressReceipt(ctx, "retired", "change", "alias-genesis")
	require.NoError(t, err)
	assert.Equal(t, "target", receipt.ProjectKey)
	occupancy, err := service.GetApplyProgressSuccessorOccupancy(ctx, "retired", "free")
	require.NoError(t, err)
	assert.Equal(t, "target", occupancy.ProjectKey)

	var retiredRows int
	require.NoError(t, store.RawDB().QueryRow(`SELECT (SELECT COUNT(*) FROM memories WHERE project = 'retired') + (SELECT COUNT(*) FROM sdd_apply_heads WHERE project = 'retired') + (SELECT COUNT(*) FROM sdd_apply_receipts WHERE project = 'retired')`).Scan(&retiredRows))
	assert.Zero(t, retiredRows)
}

func TestServiceApplyProgressGateUsesEffectiveKey(t *testing.T) {
	store, service := newSDDService(t)
	ctx := context.Background()
	// The retired key still has ordinary memories, but the gate must check the
	// project every ingress now redirects to.
	saveSDDServiceMemory(t, store, "retired", "notes", "plain")
	require.NoError(t, store.AddAlias(ctx, "retired", "unknown-target", "local", "test"))

	_, err := service.GetApplyProgress(ctx, "retired", "change")
	require.ErrorIs(t, err, governance.ErrProjectNotFound)
	key, ok := governance.ProjectKeyFromError(err)
	require.True(t, ok)
	assert.Equal(t, "unknown-target", key)
}

func TestServiceApplyProgressErrorsBeforeResolutionCarryNoKey(t *testing.T) {
	_, service := newSDDService(t)
	_, err := service.GetApplyProgress(context.Background(), " ", "change")
	require.ErrorIs(t, err, governance.ErrProjectRequired)
	_, ok := governance.ProjectKeyFromError(err)
	assert.False(t, ok)
}

func TestServiceSDDStoreBindingRetiredAliasBindsTarget(t *testing.T) {
	store, service := newSDDService(t)
	ctx := context.Background()
	require.NoError(t, store.AddAlias(ctx, "retired", "target", "local", "test"))

	missing, found, err := service.GetSDDStoreBinding(ctx, "Retired", "change")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, "target", missing.Project, "a missing binding still reports the effective key")

	request := governance.SDDStoreBindingRequest{Mode: db.SDDStoreModeHive, Provenance: "governance"}
	created, wasCreated, err := service.AdoptSDDStoreBinding(ctx, "retired", "change", request)
	require.NoError(t, err)
	assert.True(t, wasCreated)
	assert.Equal(t, "target", created.Project)

	read, found, err := service.GetSDDStoreBinding(ctx, "retired", "change")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, created, read)
}

func TestServiceSDDChangesAndArtifactsRetiredAliasReadTarget(t *testing.T) {
	store, service := newSDDService(t)
	ctx := context.Background()
	saveSDDServiceMemory(t, store, "target", "sdd/alpha/explore", "alpha explore")
	require.NoError(t, store.AddAlias(ctx, "retired", "target", "local", "test"))

	page, err := service.ListSDDChanges(ctx, governance.SDDChangePageRequest{Project: "Retired", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, page.Changes)
	artifacts, err := service.FetchSDDArtifacts(ctx, "retired", "alpha")
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "alpha explore", artifacts[0].Content)
}
