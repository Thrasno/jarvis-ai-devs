package db

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/require"
)

// identityApplyProgressRequest builds the shared genesis request fixture for an
// explicit canonical project so identity tests can pick both the request key and
// the immutable document key independently.
func identityApplyProgressRequest(t *testing.T, requestProject, documentProject, requestID, batchID string) ApplyProgressAdvance {
	t.Helper()
	request := applyProgressRequest(t, requestID, 0, 0, "", batchID)
	request.Project = requestProject
	var err error
	request.Batches[0].Project = documentProject
	request.Batches[0], _, err = applyprogress.SealBatch(request.Batches[0])
	require.NoError(t, err)
	request.Snapshot.Project = documentProject
	request.Snapshot.Batches[0].SHA256 = request.Batches[0].SHA256
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	require.NoError(t, err)
	return request
}

// countProjectRows reports every apply-progress and binding row keyed by project.
func countProjectRows(t *testing.T, store *DB, project string) int {
	t.Helper()
	total := 0
	for _, table := range []string{"memories", "sdd_apply_heads", "sdd_apply_receipts", "sdd_store_bindings", "project_identities"} {
		column := "project"
		if table == "project_identities" {
			column = "project_key"
		}
		var count int
		require.NoError(t, store.RawDB().QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+" = ?", project).Scan(&count))
		total += count
	}
	return total
}

func TestApplyProgressSpellingsThatFoldToOneKeyShareProgress(t *testing.T) {
	for _, tt := range []struct {
		name      string
		key       string
		spellings []string
	}{
		{name: "case", key: "myproject", spellings: []string{"MyProject", " MYPROJECT ", "myproject"}},
		{name: "separators", key: "my-project", spellings: []string{"my_project", "My.Project", "my-project"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			committed, err := store.AdvanceApplyProgress(identityApplyProgressRequest(t, tt.spellings[0], tt.key, "fold-"+tt.name, "apb-a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"))
			require.NoError(t, err)
			require.Equal(t, tt.key, committed.State.ProjectKey)
			for _, spelling := range tt.spellings {
				got, err := store.GetApplyProgress(spelling, "change")
				require.NoError(t, err, spelling)
				require.Equal(t, committed.State.Digest, got.Digest, spelling)
				require.Equal(t, tt.key, got.ProjectKey, spelling)
			}
		})
	}
}

func TestApplyProgressRetiredAliasReadsAndWritesTargetProgress(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	genesis := identityApplyProgressRequest(t, "target", "target", "alias-genesis", "apb-b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1")
	committed, err := store.AdvanceApplyProgress(genesis)
	require.NoError(t, err)
	require.NoError(t, store.AddAlias(ctx, "retired", "target", "local", "test"))

	key, err := store.ResolveProjectIngress(ctx, " Retired ")
	require.NoError(t, err)
	require.Equal(t, "target", key)

	state, err := store.GetApplyProgress("Retired", "change")
	require.NoError(t, err)
	require.Equal(t, committed.State.Digest, state.Digest)
	require.Equal(t, "target", state.ProjectKey)

	evidence, err := store.GetApplyProgressEvidence("retired", "change", genesis.Batches[0].BatchID, state.Digest)
	require.NoError(t, err)
	require.Equal(t, "target", evidence.Project)

	receipt, err := store.GetApplyProgressReceipt("retired", "change", "alias-genesis")
	require.NoError(t, err)
	require.Equal(t, committed.Receipt.RequestID, receipt.RequestID)
	require.Equal(t, "target", receipt.ProjectKey)

	occupancy, err := store.GetApplyProgressSuccessorOccupancy("retired", "change")
	require.NoError(t, err)
	require.Equal(t, ApplyProgressSuccessorOccupancy{Occupied: true, Category: "memory", ProjectKey: "target"}, occupancy)

	next := applyProgressSuccessor(t, state, "alias-next", "", "entry-next")
	next.Project = "Retired"
	advanced, err := store.AdvanceApplyProgress(next)
	require.NoError(t, err)
	require.Equal(t, "committed", advanced.Outcome)
	require.Equal(t, "target", advanced.State.ProjectKey)

	stale := applyProgressSuccessor(t, state, "alias-stale", "", "entry-stale")
	stale.Project = "retired"
	conflict, err := store.AdvanceApplyProgress(stale)
	require.NoError(t, err)
	require.Equal(t, "conflict", conflict.Outcome)
	require.Equal(t, "target", conflict.State.ProjectKey)

	got, err := store.GetApplyProgress("target", "change")
	require.NoError(t, err)
	require.Equal(t, advanced.State.Digest, got.Digest)
	require.Zero(t, countProjectRows(t, store, "retired"), "a retired key must never own apply-progress rows")
}

func TestApplyProgressRetiredAliasGenesisLandsUnderTarget(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, store.AddAlias(ctx, "retired", "target", "local", "test"))

	_, err := store.GetApplyProgress("retired", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)

	committed, err := store.AdvanceApplyProgress(identityApplyProgressRequest(t, "retired", "target", "alias-fresh", "apb-c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"))
	require.NoError(t, err)
	require.Equal(t, "target", committed.State.ProjectKey)

	_, err = store.AdvanceApplyProgress(identityApplyProgressRequest(t, "retired", "retired", "alias-retired-document", "apb-c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2"))
	require.ErrorIs(t, err, ErrApplyProgressInvalid, "documents signed under a retired key cannot be stored under its target")
	require.Zero(t, countProjectRows(t, store, "retired"))
}

func TestApplyProgressConflictWithoutHeadReportsResolvedKey(t *testing.T) {
	store := openTestDB(t)
	request := identityApplyProgressRequest(t, "Project", "project", "missing-head", "apb-c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3")
	request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest = 1, 1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request.Snapshot.Generation, request.Snapshot.Revision, request.Snapshot.PreviousDigest = 1, 2, request.ExpectedDigest
	var err error
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	require.NoError(t, err)
	result, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, "conflict", result.Outcome)
	require.Equal(t, "project", result.State.ProjectKey)
}

func TestPublishApplyProgressSuccessorResolvesRetiredAlias(t *testing.T) {
	store, seal := sealedPublishFixture(t)
	_, err := store.RawDB().Exec(`INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by) VALUES ('retired', 'project', 'local', 'test', CURRENT_TIMESTAMP, 'test')`)
	require.NoError(t, err)

	result, err := store.PublishApplyProgressSuccessor("Retired", "change")
	require.NoError(t, err)
	require.Equal(t, "committed", result.Outcome)
	require.Equal(t, "project", result.State.ProjectKey)
	require.Equal(t, seal.Digest, result.State.Snapshot.Supersedes.SealDigest)
	require.Zero(t, countProjectRows(t, store, "retired"))
}

func TestAddAliasRefusesSourceWithImmutableApplyProgress(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name string
		seed func(t *testing.T, store *DB)
	}{
		{name: "head", seed: func(t *testing.T, store *DB) {
			_, err := store.AdvanceApplyProgress(identityApplyProgressRequest(t, "source", "source", "guard-head", "apb-d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1"))
			require.NoError(t, err)
		}},
		{name: "receipt", seed: func(t *testing.T, store *DB) {
			_, err := store.RawDB().Exec(`INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES ('guard-receipt', 'source', 'change', 'digest', '{}')`)
			require.NoError(t, err)
		}},
		{name: "snapshot document", seed: func(t *testing.T, store *DB) {
			request := identityApplyProgressRequest(t, "source", "source", "guard-snapshot", "apb-d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2")
			topic := "sdd/change/apply-progress/v2"
			insertSDDMemory(t, store, "source", &topic, "snapshot", canonicalSnapshotContent(t, request.Snapshot), "2026-10-20 10:00:00", false)
		}},
		{name: "evidence document", seed: func(t *testing.T, store *DB) {
			request := identityApplyProgressRequest(t, "source", "source", "guard-evidence", "apb-d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3")
			topic := "sdd/change/apply-evidence/" + request.Batches[0].BatchID
			insertSDDMemory(t, store, "source", &topic, "evidence", canonicalBatchContent(t, request.Batches[0]), "2026-10-20 10:00:00", false)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			tt.seed(t, store)
			before := countProjectRows(t, store, "source")

			err := store.AddAlias(ctx, "source", "target", "local", "manual")

			require.ErrorIs(t, err, ErrAliasSourceHasImmutableApplyProgress)
			aliases, err := store.ListAliases(ctx)
			require.NoError(t, err)
			require.Empty(t, aliases)
			require.Equal(t, before, countProjectRows(t, store, "source"))
			require.Zero(t, countProjectRows(t, store, "target"))
		})
	}
}

func TestAddAliasAllowsTargetWithImmutableApplyProgress(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	_, err := store.AdvanceApplyProgress(identityApplyProgressRequest(t, "target", "target", "guard-target", "apb-e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1"))
	require.NoError(t, err)
	insertSDDMemory(t, store, "source", nil, "plain", "plain memory", "2026-10-20 10:00:00", false)

	require.NoError(t, store.AddAlias(ctx, "source", "target", "local", "manual"))
	require.NoError(t, store.AddAlias(ctx, "source", "target", "local", "manual"), "idempotent re-insert")
	target, found, err := store.ResolveAlias(ctx, "source")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "target", target)
}

func TestSDDStoreBindingRetiredAliasBindsTarget(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, store.AddAlias(ctx, "retired", "target", "local", "test"))
	request := SDDStoreBindingRequest{Mode: SDDStoreModeHive, Provenance: "governance"}

	created, wasCreated, err := store.AdoptSDDStoreBinding(ctx, " Retired ", "change", request)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, "target", created.Project)

	replayed, wasCreated, err := store.AdoptSDDStoreBinding(ctx, "retired", "change", request)
	require.NoError(t, err)
	require.False(t, wasCreated)
	require.Equal(t, created, replayed)

	_, _, err = store.AdoptSDDStoreBinding(ctx, "retired", "change", SDDStoreBindingRequest{Mode: SDDStoreModeHybrid, Provenance: "other"})
	var conflict *SDDStoreBindingConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, created, conflict.Existing)

	read, found, err := store.GetSDDStoreBinding(ctx, "RETIRED", "change")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, created, read)
	require.Zero(t, countProjectRows(t, store, "retired"))
}

func TestSDDStoreBindingNewAdoptRespectsProjectBlockButReplayDoesNot(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	request := SDDStoreBindingRequest{Mode: SDDStoreModeHive, Provenance: "governance"}
	created, _, err := store.AdoptSDDStoreBinding(ctx, "project", "existing", request)
	require.NoError(t, err)
	_, err = store.RawDB().Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES ('project', 'project', 'block-binding', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	replayed, wasCreated, err := store.AdoptSDDStoreBinding(ctx, "project", "existing", request)
	require.NoError(t, err)
	require.False(t, wasCreated)
	require.Equal(t, created, replayed)

	_, _, err = store.AdoptSDDStoreBinding(ctx, "project", "fresh", request)
	require.ErrorIs(t, err, ErrProjectBlocked)
	_, found, err := store.GetSDDStoreBinding(ctx, "project", "fresh")
	require.NoError(t, err)
	require.False(t, found)
}

func TestRemoteImmutableApplyProgressRemappedByAliasIsRejectedDistinctly(t *testing.T) {
	request := identityApplyProgressRequest(t, "retired", "retired", "remote-remapped", "apb-f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1")
	snapshotContent := canonicalSnapshotContent(t, request.Snapshot)
	for _, tt := range []struct {
		name    string
		content string
		want    ImmutableRemoteCreateRejectionCode
		source  string
	}{
		{name: "valid document under retired key", content: snapshotContent, want: ImmutableRemoteCreateRejectionProjectRemapped, source: "retired"},
		{name: "malformed document under retired key", content: `{"schema":"broken"}`, want: ImmutableRemoteCreateRejectionInvalidDocument},
	} {
		t.Run(tt.name+" via mutation", func(t *testing.T) {
			store := openTestDB(t)
			require.NoError(t, store.AddAlias(context.Background(), "retired", "target", "local", "test"))
			event := immutableCreateEvent("remapped-event", "remapped-sync", "sdd/change/apply-progress/v2", tt.content)
			event.Project, event.Memory.Project = "Retired", "Retired"

			applied, err := store.ApplyRemoteMutation(event)

			require.False(t, applied)
			var rejected *RemoteImmutableCreateRejectedError
			require.ErrorAs(t, err, &rejected)
			require.Equal(t, tt.want, rejected.RejectionCode)
			require.Equal(t, "target", rejected.CanonicalProject)
			require.Equal(t, tt.source, rejected.SourceProject)
			require.NotContains(t, rejected.Error(), tt.content)
			var memories int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, "remapped-sync").Scan(&memories))
			require.Zero(t, memories)
			require.Zero(t, countProjectRows(t, store, "retired"))
			require.Zero(t, countProjectRows(t, store, "target"))
		})
		t.Run(tt.name+" via legacy pull", func(t *testing.T) {
			store := openTestDB(t)
			require.NoError(t, store.AddAlias(context.Background(), "retired", "target", "local", "test"))
			topic := "sdd/change/apply-progress/v2"
			err := store.SaveFromRemote(&models.Memory{SyncID: "remapped-pull", Project: "Retired", TopicKey: &topic, Title: "immutable apply progress", Content: tt.content, SessionID: "manual-save-retired"})

			var rejected *RemoteImmutableCreateRejectedError
			require.ErrorAs(t, err, &rejected)
			require.Equal(t, tt.want, rejected.RejectionCode)
			require.Equal(t, "target", rejected.CanonicalProject)
			require.Equal(t, tt.source, rejected.SourceProject)
			var memories int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, "remapped-pull").Scan(&memories))
			require.Zero(t, memories)
		})
	}
}

func TestRemoteImmutableApplyProgressWithoutAliasStillApplies(t *testing.T) {
	store := openTestDB(t)
	request := identityApplyProgressRequest(t, "project", "project", "remote-plain", "apb-f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2")
	event := immutableCreateEvent("plain-event", "plain-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, request.Snapshot))
	event.Project, event.Memory.Project = "Project", "Project"

	applied, err := store.ApplyRemoteMutation(event)

	require.NoError(t, err)
	require.True(t, applied)
	var project string
	require.NoError(t, store.RawDB().QueryRow(`SELECT project FROM memories WHERE sync_id = ?`, "plain-sync").Scan(&project))
	require.Equal(t, "project", project)
}

// TestLegacyCaseOnlyAliasResolvesTheSameInsideAndOutsideTransactions covers a
// historical alias row whose source was stored before canonical storage. Both
// the non-transactional and transactional ingress paths must resolve it alike.
func TestLegacyCaseOnlyAliasResolvesTheSameInsideAndOutsideTransactions(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	_, err := store.RawDB().Exec(`INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by) VALUES ('MyProj', 'target', 'local', 'legacy', CURRENT_TIMESTAMP, 'test')`)
	require.NoError(t, err)

	key, err := store.ResolveProjectIngress(ctx, "MyProj")
	require.NoError(t, err)
	require.Equal(t, "target", key)

	request := SDDStoreBindingRequest{Mode: SDDStoreModeHive, Provenance: "legacy"}
	created, wasCreated, err := store.AdoptSDDStoreBinding(ctx, "MyProj", "change", request)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, "target", created.Project)
	read, found, err := store.GetSDDStoreBinding(ctx, "myproj", "change")
	require.NoError(t, err)
	require.True(t, found, "get must resolve the same alias adopt resolved")
	require.Equal(t, created, read)

	committed, err := store.AdvanceApplyProgress(identityApplyProgressRequest(t, "MyProj", "target", "legacy-alias", "apb-f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3f3"))
	require.NoError(t, err)
	require.Equal(t, "committed", committed.Outcome)
	require.Equal(t, "target", committed.State.ProjectKey)
	state, err := store.GetApplyProgress("MYPROJ", "change")
	require.NoError(t, err)
	require.Equal(t, committed.State.Digest, state.Digest)
	require.Zero(t, countProjectRows(t, store, "myproj"))
}

func TestRemoteImmutableRejectionErrorTextNamesItsCode(t *testing.T) {
	for _, code := range []ImmutableRemoteCreateRejectionCode{ImmutableRemoteCreateRejectionProjectRemapped, ImmutableRemoteCreateRejectionInvalidDocument, ImmutableRemoteCreateRejectionSyncIDConflict} {
		rejected := immutableRemoteCreateRejectedForIdentity("", "sync", "project", "sdd/change/apply-progress/v2", MutationOpCreate, "secret content", code)
		require.Contains(t, rejected.Error(), string(code))
		require.NotContains(t, rejected.Error(), "secret content")
	}
}
