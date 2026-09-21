package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/require"
)

func TestRetiredProjectIngressResolvesBeforePersistingState(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	require.NoError(t, d.AddAlias(ctx, "retired", "active", "local", "workspace promotion"))

	_, err = d.SaveMemoryWithManualSession(&models.Memory{Project: "retired", Title: "direct", Content: "memory"})
	require.NoError(t, err)
	require.NoError(t, d.CreateSession("direct-session", "retired", "/repo", "dev", "hook"))
	_, err = d.SavePromptForSession(ctx, "retired", "direct-session", "prompt")
	require.NoError(t, err)
	require.NoError(t, d.SavePassiveObservation(ctx, "", "retired", "hook", "observation"))
	require.NoError(t, d.SavePassiveObservationWithSession(ctx, models.PassiveObservationWrite{
		Session: models.SessionInput{ID: "passive-session", Project: "retired", Client: "hook"}, Source: "hook", Content: "session observation",
	}))
	require.NoError(t, d.SaveSessionFromRemote(&models.Session{ID: "remote-session", SyncID: "remote-sync", Project: "retired", DevID: "remote", Client: "remote", StartedAt: time.Now().UTC()}))
	require.NoError(t, d.SaveFromRemote(&models.Memory{SyncID: "remote-memory", Project: "retired", Title: "remote", Content: "memory", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}))
	applied, err := d.ApplyRemoteMutation(hivedb.MutationEnvelope{EventID: "remote-mutation-event", EntitySyncID: "remote-mutation-memory", Project: "retired", Op: hivedb.MutationOpCreate, OccurredAt: time.Now().UTC(), Memory: &hivedb.MutationMemoryPayload{SyncID: "remote-mutation-memory", Project: "retired", Title: "mutation", Content: "memory", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), SessionID: "remote-session"}})
	require.NoError(t, err)
	require.True(t, applied)

	batch := hivedb.ImportBatch{Sessions: []hivedb.ImportSession{{SourceID: "import-session", Project: "retired", StartedAt: "2026-01-01 00:00:00", ContentHash: "session"}}}
	first, err := d.ImportEngramBatch(ctx, hivedb.ImportRun{ID: "import-one", SourceSystem: "engram", SourcePath: "one", Mode: "execute"}, batch)
	require.NoError(t, err)
	require.Equal(t, 1, first.Counts.Imported)
	retry, err := d.ImportEngramBatch(ctx, hivedb.ImportRun{ID: "import-two", SourceSystem: "engram", SourcePath: "one", Mode: "execute"}, batch)
	require.NoError(t, err)
	require.Equal(t, 1, retry.Counts.Reused)

	now := time.Now().UTC()
	require.NoError(t, d.SetLastSync("retired", now))
	require.NoError(t, d.SetMutationCursor("consumer", "retired", hivedb.MutationCursor{Sequence: 7, EventID: "cursor"}, now))
	require.NoError(t, d.SetPullCursor("consumer", "retired", "memories", hivedb.PullCursor{SyncedAt: now, SyncID: "pull"}, now))
	require.NoError(t, d.RecordSyncAttempt("retired", now))
	require.NoError(t, d.RecordSyncAttemptLog(ctx, hivedb.SyncAttemptLog{AttemptID: "attempt", DevID: "dev", Project: "retired", Client: "test", StartedAt: now, EndedAt: now, Outcome: hivedb.SyncAttemptOutcomeSuccess}))

	for _, table := range []string{"memories", "sessions", "user_prompts", "passive_observations", "import_source_aliases", "sync_state", "mutation_cursors", "pull_cursors", "sync_attempt_logs"} {
		var retired int
		column := "project"
		if table == "import_source_aliases" {
			column = "source_project"
		}
		require.NoErrorf(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+column+` = 'retired'`).Scan(&retired), "count retired %s", table)
		require.Zerof(t, retired, "retired project leaked into %s", table)
	}
	var active int
	require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'active'`).Scan(&active))
	require.Greater(t, active, 0)
}

func TestWorkspacePromotionWaitsForInFlightProjectLease(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, d.CreateSession("source-session", "source", workspace, "dev", "test"))
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "source")
	require.NoError(t, err)
	releaseRead := hivedb.AcquireProjectLifecycleRead("source", "target")
	done := make(chan error, 1)
	go func() {
		_, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("promotion bypassed in-flight sync lease: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	releaseRead()
	require.NoError(t, <-done)
}

func TestWorkspacePromotionDoesNotTreatRejectedCreateAsRemotePresence(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := filepath.Join(t.TempDir(), "workspace")
	_, err = d.EnsureManualSaveSession("source")
	require.NoError(t, err)
	_, err = d.SaveMemory(&models.Memory{Project: "source", SessionID: "manual-save-source", Title: "rejected", Content: "create"})
	require.NoError(t, err)
	pending, err := d.GetPendingMutations("source", 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, d.MarkMutationsRejected([]string{pending[0].EventID}, time.Now().UTC()))
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "source")
	require.NoError(t, err)
	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.True(t, changed)
	pending, err = d.GetPendingMutations("target", 10)
	require.NoError(t, err)
	for _, mutation := range pending {
		require.NotEqual(t, hivedb.MutationOpReproject, mutation.Op)
	}
}

func TestWorkspacePromotionMovesSDDStoreBindingWithoutRewritingIt(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := filepath.Join(t.TempDir(), "workspace")
	binding, created, err := d.AdoptSDDStoreBinding(ctx, "source", "change", hivedb.SDDStoreBindingRequest{Mode: hivedb.SDDStoreModeHybrid, Provenance: "operator"})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, d.CreateSession("source-session", "source", workspace, "dev", "test"))
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "source")
	require.NoError(t, err)
	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.True(t, changed)
	moved, found, err := d.GetSDDStoreBinding(ctx, "target", "change")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, binding.SchemaVersion, moved.SchemaVersion)
	require.Equal(t, binding.Mode, moved.Mode)
	require.Equal(t, binding.Provenance, moved.Provenance)
	_, found, err = d.GetSDDStoreBinding(ctx, "source", "change")
	require.NoError(t, err)
	require.False(t, found)
}

func TestIngressAliasResolutionCanonicalizesBeforeLookup(t *testing.T) {
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	_, err = d.RawDB().Exec(`INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES ('retired-project', 'active-project', 'local', 'promotion')`)
	require.NoError(t, err)
	require.NoError(t, d.CreateSession("mixed-session", "RETIRED_project", "/repo", "dev", "test"))
	require.NoError(t, d.SaveFromRemote(&models.Memory{SyncID: "mixed-remote", Project: "retired project", Title: "remote", Content: "memory", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}))
	for _, table := range []string{"sessions", "memories"} {
		var retired, active int
		require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE project = 'retired-project'`).Scan(&retired))
		require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE project = 'active-project'`).Scan(&active))
		require.Zero(t, retired)
		require.Greater(t, active, 0)
	}
}

func TestWorkspacePromotionResetsSourceSyncNamespaceWithoutOverwritingTarget(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, d.CreateSession("source-session", "source", workspace, "dev", "test"))
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "source")
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, d.SetMutationCursor("consumer", "source", hivedb.MutationCursor{Sequence: 1, EventID: "source"}, now))
	require.NoError(t, d.SetMutationCursor("consumer", "target", hivedb.MutationCursor{Sequence: 2, EventID: "target"}, now))

	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.True(t, changed)
	var sourceRows int
	require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM mutation_cursors WHERE project = 'source'`).Scan(&sourceRows))
	require.Zero(t, sourceRows, "A cursor must not be transplanted into B")
	cursor, err := d.GetMutationCursor("consumer", "target")
	require.NoError(t, err)
	require.Equal(t, int64(2), cursor.Sequence, "B keeps only its own remote namespace position")
	changed, err = d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.False(t, changed, "the completed promotion retry is idempotent")
}

func TestPromotionDispatchesReprojectBeforeServerHeldPendingFollowups(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := filepath.Join(t.TempDir(), "workspace")
	_, err = d.EnsureManualSaveSession("source")
	require.NoError(t, err)
	nowTime := time.Now().UTC()
	require.NoError(t, d.SaveFromRemote(&models.Memory{SyncID: "remote-server-held", Project: "source", SessionID: "manual-save-source", Title: "server-held", Content: "memory", CreatedAt: nowTime, UpdatedAt: nowTime}))
	var id int64
	require.NoError(t, d.RawDB().QueryRow(`SELECT id FROM memories WHERE sync_id = 'remote-server-held'`).Scan(&id))
	memory, err := d.GetMemory(id)
	require.NoError(t, err)
	_, err = d.RawDB().Exec(`UPDATE memories SET synced_at = NULL WHERE sync_id = ?`, memory.SyncID)
	require.NoError(t, err)
	for _, op := range []string{"update", "delete", "restore"} {
		_, err = d.RawDB().Exec(`INSERT INTO memory_mutations (event_id, entity_sync_id, project, op, payload_json) VALUES (?, ?, 'source', ?, '{}')`, "pending-"+op, memory.SyncID, op)
		require.NoError(t, err)
	}
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "source")
	require.NoError(t, err)
	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.True(t, changed)
	pending, err := d.GetPendingMutations("target", 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(pending), 4)
	require.Equal(t, hivedb.MutationOpReproject, pending[0].Op)
	require.Equal(t, memory.SyncID, pending[0].EntitySyncID)
	for _, mutation := range pending[1:] {
		if mutation.EntitySyncID == memory.SyncID {
			require.NotEqual(t, hivedb.MutationOpReproject, mutation.Op)
		}
	}
	changed, err = d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.False(t, changed, "completed promotion retry must not enqueue another reproject")
}

func TestWorkspacePromotionReconcilesIngressInventoryAndRelocationProvenance(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := filepath.Join(t.TempDir(), "workspace")

	require.NoError(t, d.CreateSession("synced-session", "source", workspace, "dev", "hook"))
	require.NoError(t, d.CreateSession("unsynced-session", "source", workspace, "dev", "hook"))
	_, err = d.SavePromptForSession(ctx, "source", "synced-session", "synced prompt")
	require.NoError(t, err)
	_, err = d.SavePromptForSession(ctx, "source", "unsynced-session", "unsynced prompt")
	require.NoError(t, err)
	_, err = d.EnsureManualSaveSession("source")
	require.NoError(t, err)
	memoryID, err := d.SaveMemory(&models.Memory{Project: "source", SessionID: "manual-save-source", Title: "synced", Content: "memory"})
	require.NoError(t, err)
	memory, err := d.GetMemory(memoryID)
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, d.MarkSynced(memory.SyncID, now))
	require.NoError(t, d.MarkSessionSynced("synced-session", now))
	var syncedPrompt string
	require.NoError(t, d.RawDB().QueryRow(`SELECT sync_id FROM user_prompts WHERE content = 'synced prompt'`).Scan(&syncedPrompt))
	require.NoError(t, d.MarkPromptSynced(ctx, syncedPrompt, now))
	require.NoError(t, d.SavePassiveObservation(ctx, "", "source", "hook", "passive"))
	require.NoError(t, d.SetLastSync("source", now))
	require.NoError(t, d.SetMutationCursor("consumer", "source", hivedb.MutationCursor{Sequence: 3, EventID: "mutation"}, now))
	require.NoError(t, d.SetPullCursor("consumer", "source", "memories", hivedb.PullCursor{SyncedAt: now, SyncID: "pull"}, now))
	require.NoError(t, d.RecordSyncAttemptLog(ctx, hivedb.SyncAttemptLog{AttemptID: "promotion-attempt", DevID: "dev", Project: "source", Client: "test", StartedAt: now, EndedAt: now, Outcome: hivedb.SyncAttemptOutcomeSuccess}))
	_, err = d.ImportEngramBatch(ctx, hivedb.ImportRun{ID: "promotion-import", SourceSystem: "engram", SourcePath: "source", Mode: "execute"}, hivedb.ImportBatch{Sessions: []hivedb.ImportSession{{SourceID: "source-import", Project: "source", StartedAt: "2026-01-01 00:00:00"}}})
	require.NoError(t, err)
	require.NoError(t, d.SavePassiveObservation(ctx, "", "source", "hook", "passive"))
	_, err = d.RawDB().Exec(`INSERT INTO memory_mutations (event_id, entity_sync_id, project, op, payload_json) VALUES ('pending-reproject', 'pending-reproject-memory', 'source', 'reproject', '{"reproject":{"from_project":"other","to_project":"source"}}')`)
	require.NoError(t, err)
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "source")
	require.NoError(t, err)

	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
	require.NoError(t, err)
	require.True(t, changed)
	// A delayed pull can still carry the source manual session id. Promotion
	// rekeys that session's project but intentionally preserves its stable id.
	require.NoError(t, d.SaveFromRemote(&models.Memory{SyncID: "delayed-manual-source", Project: "source", SessionID: "manual-save-source", Title: "delayed", Content: "remote", CreatedAt: now, UpdatedAt: now}))
	var delayedSession string
	require.NoError(t, d.RawDB().QueryRow(`SELECT session_id FROM memories WHERE sync_id = 'delayed-manual-source'`).Scan(&delayedSession))
	require.Equal(t, "manual-save-source", delayedSession)

	for _, table := range []string{"memories", "sessions", "user_prompts", "passive_observations", "sync_state", "mutation_cursors", "pull_cursors", "sync_attempt_logs", "import_source_aliases"} {
		column := "project"
		if table == "import_source_aliases" {
			column = "source_project"
		}
		var sourceRows int
		require.NoErrorf(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+column+` = 'source'`).Scan(&sourceRows), "count source %s", table)
		require.Zerof(t, sourceRows, "source rows remain in %s", table)
	}
	var sessionFrom, promptFrom string
	require.NoError(t, d.RawDB().QueryRow(`SELECT sync_from_project FROM sessions WHERE id = 'synced-session'`).Scan(&sessionFrom))
	require.Equal(t, "source", sessionFrom)
	require.NoError(t, d.RawDB().QueryRow(`SELECT sync_from_project FROM user_prompts WHERE sync_id = ?`, syncedPrompt).Scan(&promptFrom))
	require.Equal(t, "source", promptFrom)
	pending, err := d.GetPendingMutations("target", 100)
	require.NoError(t, err)
	foundReproject, foundCreate, foundRekeyedReproject := false, false, false
	for _, mutation := range pending {
		if mutation.EntitySyncID == memory.SyncID && mutation.Project == "target" && mutation.Op == hivedb.MutationOpReproject && mutation.Reproject != nil && mutation.Reproject.FromProject == "source" && mutation.Reproject.ToProject == "target" {
			foundReproject = true
		}
		if mutation.EntitySyncID == memory.SyncID && mutation.Project == "target" && mutation.Memory != nil && mutation.Memory.Project == "target" {
			foundCreate = true
		}
		if mutation.EventID == "pending-reproject" && mutation.Project == "target" && mutation.Reproject != nil && mutation.Reproject.FromProject == "other" && mutation.Reproject.ToProject == "target" {
			foundRekeyedReproject = true
		}
	}
	require.True(t, foundReproject, "synced memory must receive a target reproject mutation")
	require.True(t, foundCreate, "pending create payload must carry the promoted target")
	require.True(t, foundRekeyedReproject, "pending reproject payload must be accepted under target coordinates")
}
