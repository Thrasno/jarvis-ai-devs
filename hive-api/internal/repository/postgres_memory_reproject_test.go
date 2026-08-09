package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reproject op is how the daemon tells the server "this memory's project
// literal changed". Every other mutation op uses `project` to FIND its row and
// can never change it; reproject is the one op whose entire purpose is to change
// it, so it is the one op that does not compare the envelope's project against
// the stored one.
//
// What replaces that comparison is `WHERE sync_id = $1 AND project = $from`: the
// caller must name the literal the row currently holds, or the statement matches
// nothing. That is what makes the op safe to expose, and it is what makes replay
// a silent no-op instead of a wrong move.

const (
	reprojectFrom = "Foo.Bar"
	reprojectTo   = "foo-bar"
)

func seedReprojectMemory(ctx context.Context, t *testing.T, pool *pgxpool.Pool, syncID string) *model.Memory {
	t.Helper()
	repo := NewPostgresMemoryRepository(pool)
	sessionID := ensureManualSavePtr(t, pool, reprojectFrom)
	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	mem, err := repo.Create(ctx, &model.Memory{
		SyncID:    syncID,
		Project:   reprojectFrom,
		TopicKey:  stringPtr("reproject/topic"),
		Category:  model.CatDecision,
		Title:     "moved memory",
		Content:   "moved content",
		CreatedBy: "tester",
		CreatedAt: now,
		UpdatedAt: now,
		SessionID: sessionID,
	})
	require.NoError(t, err)
	return mem
}

func reprojectEvent(eventID, syncID, from, to string) model.MutationEnvelope {
	return model.MutationEnvelope{
		EventID:      eventID,
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: syncID,
		Project:      to,
		Op:           model.MutationOpReproject,
		OccurredAt:   time.Now().UTC(),
		Reproject:    &model.ReprojectPayload{FromProject: from, ToProject: to},
	}
}

func TestApplyMemoryMutation_ReprojectMovesTheMemoryAndJournalsIt(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8a0e8400-e29b-41d4-a716-446655440101"
	before := seedReprojectMemory(ctx, t, pool, syncID)

	event := reprojectEvent("8a0e8400-e29b-41d4-a716-446655440001", syncID, reprojectFrom, reprojectTo)
	result, err := repo.ApplyMemoryMutation(ctx, event)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Applied)
	assert.False(t, result.Rejected)

	after, err := repo.GetBySyncID(ctx, syncID)
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.Equal(t, reprojectTo, after.Project, "the memory must now live under the target project")
	assert.Equal(t, before.Title, after.Title, "reproject changes identity, not content")
	assert.Equal(t, before.Content, after.Content)
	assert.Equal(t, before.CreatedBy, after.CreatedBy)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at is not a sync timestamp and must not move")
	assert.True(t, after.SyncedAt.After(before.SyncedAt),
		"synced_at must be bumped — it is how pullers on the new name discover the row")

	// The journal entry lands under the target project, carrying both literals so
	// a consumer replaying the stream can follow the move.
	batch, err := repo.ListMemoryMutations(ctx, reprojectTo, model.MutationCursor{}, 10)
	require.NoError(t, err)
	require.Len(t, batch.Events, 1)
	assert.Equal(t, model.MutationOpReproject, batch.Events[0].Op)
	require.NotNil(t, batch.Events[0].Reproject)
	assert.Equal(t, reprojectFrom, batch.Events[0].Reproject.FromProject)
	assert.Equal(t, reprojectTo, batch.Events[0].Reproject.ToProject)

	sourceBatch, err := repo.ListMemoryMutations(ctx, reprojectFrom, model.MutationCursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, sourceBatch.Events, "the source project's journal records nothing")
}

// TestApplyMemoryMutation_ReprojectIsDiscoverableByTheTargetProjectPull proves
// the propagation mechanism rather than assuming it: synced_at = now() puts the
// row inside the normal PullSince window of a puller on the new name, and out of
// reach of one on the old name.
func TestApplyMemoryMutation_ReprojectIsDiscoverableByTheTargetProjectPull(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8b0e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	// A puller's watermark from before the move.
	since := time.Now().UTC()

	_, err := repo.ApplyMemoryMutation(ctx, reprojectEvent("8b0e8400-e29b-41d4-a716-446655440001", syncID, reprojectFrom, reprojectTo))
	require.NoError(t, err)

	target, _, err := repo.PullSince(ctx, reprojectTo, since, nil, model.PullCursor{}, 0)
	require.NoError(t, err)
	require.Len(t, target, 1, "a puller on the new name must see the row through its normal since window")
	assert.Equal(t, syncID, target[0].SyncID)

	source, _, err := repo.PullSince(ctx, reprojectFrom, since, nil, model.PullCursor{}, 0)
	require.NoError(t, err)
	assert.Empty(t, source, "the old name no longer owns the row")
}

func TestApplyMemoryMutation_ReprojectReplayIsANoOp(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8c0e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	first := reprojectEvent("8c0e8400-e29b-41d4-a716-446655440001", syncID, reprojectFrom, reprojectTo)
	applied, err := repo.ApplyMemoryMutation(ctx, first)
	require.NoError(t, err)
	require.True(t, applied.Applied)

	t.Run("same event_id is a duplicate", func(t *testing.T) {
		result, err := repo.ApplyMemoryMutation(ctx, first)
		require.NoError(t, err)
		assert.True(t, result.Duplicate)
		assert.False(t, result.Applied)
	})

	t.Run("a fresh event replaying the same move matches nothing", func(t *testing.T) {
		replay := reprojectEvent("8c0e8400-e29b-41d4-a716-446655440002", syncID, reprojectFrom, reprojectTo)
		result, err := repo.ApplyMemoryMutation(ctx, replay)
		require.NoError(t, err, "a replay is not an error")
		require.NotNil(t, result)
		assert.False(t, result.Applied, "the row already moved: zero rows matched")
		assert.False(t, result.Rejected, "a no-op replay is not a rejection")
		assert.True(t, result.Duplicate, "and it must still be ackable, or it retries forever")
		assertNoMemoryMutationRow(t, pool, replay.EventID)
	})

	after, err := repo.GetBySyncID(ctx, syncID)
	require.NoError(t, err)
	assert.Equal(t, reprojectTo, after.Project)
}

// TestApplyMemoryMutation_ReprojectRequiresTheStoredFromProject is the property
// that replaces the generic anti-hijack guard for this op: a caller who does not
// name the literal the row currently holds moves nothing.
func TestApplyMemoryMutation_ReprojectRequiresTheStoredFromProject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8d0e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	event := reprojectEvent("8d0e8400-e29b-41d4-a716-446655440001", syncID, "some-other-project", reprojectTo)
	result, err := repo.ApplyMemoryMutation(ctx, event)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Applied)
	assertNoMemoryMutationRow(t, pool, event.EventID)

	after, err := repo.GetBySyncID(ctx, syncID)
	require.NoError(t, err)
	assert.Equal(t, reprojectFrom, after.Project, "the row must not have moved")
}

func TestApplyMemoryMutation_ReprojectRejectsMalformedInstructions(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8e0e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	base := func(eventID string) model.MutationEnvelope {
		return model.MutationEnvelope{
			EventID:      eventID,
			EntityType:   model.MutationEntityMemory,
			EntitySyncID: syncID,
			Project:      reprojectTo,
			Op:           model.MutationOpReproject,
			OccurredAt:   time.Now().UTC(),
		}
	}

	tests := []struct {
		name   string
		mutate func(*model.MutationEnvelope)
		reason string
	}{
		{
			name:   "no reproject payload",
			mutate: func(m *model.MutationEnvelope) { m.Reproject = nil },
			reason: "reproject",
		},
		{
			name:   "empty from_project",
			mutate: func(m *model.MutationEnvelope) { m.Reproject = &model.ReprojectPayload{ToProject: reprojectTo} },
			reason: "reproject",
		},
		{
			name: "from equals to",
			mutate: func(m *model.MutationEnvelope) {
				m.Reproject = &model.ReprojectPayload{FromProject: reprojectTo, ToProject: reprojectTo}
			},
			reason: "reproject",
		},
		{
			name: "to_project disagrees with the envelope project",
			mutate: func(m *model.MutationEnvelope) {
				m.Reproject = &model.ReprojectPayload{FromProject: reprojectFrom, ToProject: "third-project"}
			},
			reason: "reproject",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := base("8e0e8400-e29b-41d4-a716-44665544000" + string(rune('1'+i)))
			tt.mutate(&event)

			result, err := repo.ApplyMemoryMutation(ctx, event)
			require.NoError(t, err, "a malformed instruction must not fail the whole batch")
			require.NotNil(t, result)
			assert.True(t, result.Rejected)
			assert.False(t, result.Applied)
			assert.Contains(t, result.Reason, tt.reason)
			assertNoMemoryMutationRow(t, pool, event.EventID)

			after, err := repo.GetBySyncID(ctx, syncID)
			require.NoError(t, err)
			assert.Equal(t, reprojectFrom, after.Project)
		})
	}
}

// TestApplyMemoryMutation_ReprojectMovesATombstonedMemory: identity is a
// property of the row, not of its liveness. Leaving tombstones behind would
// strand a delete under the old name and let a later restore resurface the
// memory under a project the daemon no longer uses.
func TestApplyMemoryMutation_ReprojectMovesATombstonedMemory(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8f0e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	now := time.Now().UTC()
	_, err := repo.ApplyMemoryMutation(ctx, model.MutationEnvelope{
		EventID:      "8f0e8400-e29b-41d4-a716-446655440001",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: syncID,
		Project:      reprojectFrom,
		Op:           model.MutationOpDelete,
		OccurredAt:   now,
		Tombstone:    &model.TombstonePayload{DeletedAt: now, DeletedBy: "tester", Reason: "obsolete"},
	})
	require.NoError(t, err)

	result, err := repo.ApplyMemoryMutation(ctx, reprojectEvent("8f0e8400-e29b-41d4-a716-446655440002", syncID, reprojectFrom, reprojectTo))
	require.NoError(t, err)
	assert.True(t, result.Applied)

	after, err := repo.GetBySyncID(ctx, syncID)
	require.NoError(t, err)
	assert.Equal(t, reprojectTo, after.Project)
	require.NotNil(t, after.DeletedAt, "the tombstone travels with the row")
}

// TestListActivityFeed_ExcludesReproject keeps the human-facing feed about what
// happened to knowledge. A reproject is a rename of the container, not an event
// in the life of a memory, and the feed's op filter already says so.
func TestListActivityFeed_ExcludesReproject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8a1e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	_, err := repo.ApplyMemoryMutation(ctx, reprojectEvent("8a1e8400-e29b-41d4-a716-446655440001", syncID, reprojectFrom, reprojectTo))
	require.NoError(t, err)

	feed, err := repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 20})
	require.NoError(t, err)
	for _, row := range feed {
		assert.NotEqual(t, model.MutationOpReproject, row.Op, "reproject must not surface in the activity feed")
	}
}

func TestMigration023_MemoryMutationsAcceptReprojectOp(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_mutations
			(event_id, entity_type, entity_sync_id, project, op, occurred_at, reproject)
		VALUES ('9a0e8400-e29b-41d4-a716-446655440001', 'memory',
		        '9a0e8400-e29b-41d4-a716-446655440101', $1, 'reproject', now(), $2)`,
		reprojectTo, []byte(`{"from_project":"Foo.Bar","to_project":"foo-bar"}`))
	require.NoError(t, err, "migration 023 must widen the op check constraint and add the payload column")

	_, err = pool.Exec(ctx, `
		INSERT INTO memory_mutations
			(event_id, entity_type, entity_sync_id, project, op, occurred_at)
		VALUES ('9a0e8400-e29b-41d4-a716-446655440002', 'memory',
		        '9a0e8400-e29b-41d4-a716-446655440102', $1, 'nonsense', now())`, reprojectTo)
	require.Error(t, err, "the constraint must still reject an op nobody defined")
}

// TestApplyMemoryMutation_UnknownOpIsRejectedNotFatal covers the other half of
// the capability contract.
//
// An op arrives from the wire and is not validated anywhere before the switch,
// so an unknown value is untrusted input, not a broken invariant. It used to
// return a hard error, which failed the ENTIRE mutation batch: one daemon ahead
// of its server could not sync at all, and every well-formed mutation travelling
// with the unknown one was lost too. Rejecting just that event lets the rest of
// the batch through and tells the daemon precisely which event the server did
// not understand.
func TestApplyMemoryMutation_UnknownOpIsRejectedNotFatal(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	const syncID = "8b1e8400-e29b-41d4-a716-446655440101"
	seedReprojectMemory(ctx, t, pool, syncID)

	event := model.MutationEnvelope{
		EventID:      "8b1e8400-e29b-41d4-a716-446655440001",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: syncID,
		Project:      reprojectFrom,
		Op:           model.MutationOp("teleport"),
		OccurredAt:   time.Now().UTC(),
	}

	result, err := repo.ApplyMemoryMutation(ctx, event)

	require.NoError(t, err, "an op this server does not know must not fail the whole batch")
	require.NotNil(t, result)
	assert.True(t, result.Rejected)
	assert.False(t, result.Applied)
	assert.Contains(t, result.Reason, "teleport")
	assertNoMemoryMutationRow(t, pool, event.EventID)

	after, err := repo.GetBySyncID(ctx, syncID)
	require.NoError(t, err)
	assert.Equal(t, reprojectFrom, after.Project, "an unknown op changes nothing")
}

// TestApplyMemoryMutation_ReprojectResultIsAlwaysAckable closes the retry loop.
//
// The daemon acks a mutation on Applied || Duplicate, and drops it on Rejected.
// A reproject that matched zero rows used to return all three false, so it
// matched neither path: it stayed pending and was re-sent on every cycle,
// forever. And "matched zero rows" is not an edge case here — the design calls
// it the idempotent success path, so it is the ROUTINE outcome of a replay.
//
// Every outcome must therefore land in one of the two ackable classes, and the
// two reasons a reproject can match nothing must stay distinguishable: a row
// that is already where the caller wants it is a success, a row that is missing
// or somewhere else entirely is not.
func TestApplyMemoryMutation_ReprojectResultIsAlwaysAckable(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)

	t.Run("already at the target is a success the daemon can ack", func(t *testing.T) {
		const syncID = "8c1e8400-e29b-41d4-a716-446655440101"
		seedReprojectMemory(ctx, t, pool, syncID)
		_, err := repo.ApplyMemoryMutation(ctx, reprojectEvent("8c1e8400-e29b-41d4-a716-446655440001", syncID, reprojectFrom, reprojectTo))
		require.NoError(t, err)

		replay := reprojectEvent("8c1e8400-e29b-41d4-a716-446655440002", syncID, reprojectFrom, reprojectTo)
		result, err := repo.ApplyMemoryMutation(ctx, replay)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Duplicate, "the row is already where the caller wants it")
		assert.False(t, result.Applied)
		assert.False(t, result.Rejected, "already-at-target is not a failure")
		assertNoMemoryMutationRow(t, pool, replay.EventID)
	})

	t.Run("a missing target is a rejection, not a retry", func(t *testing.T) {
		event := reprojectEvent("8c1e8400-e29b-41d4-a716-446655440003",
			"8c1e8400-e29b-41d4-a716-446655440199", reprojectFrom, reprojectTo)
		result, err := repo.ApplyMemoryMutation(ctx, event)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Rejected, "there is nothing to move and retrying will never help")
		assert.False(t, result.Applied)
		assert.False(t, result.Duplicate, "a missing row must stay distinguishable from an already-moved one")
		assert.NotEmpty(t, result.Reason)
		assertNoMemoryMutationRow(t, pool, event.EventID)
	})

	t.Run("a stale from_project is a rejection", func(t *testing.T) {
		const syncID = "8c1e8400-e29b-41d4-a716-446655440102"
		seedReprojectMemory(ctx, t, pool, syncID)

		event := reprojectEvent("8c1e8400-e29b-41d4-a716-446655440004", syncID, "some-other-project", reprojectTo)
		result, err := repo.ApplyMemoryMutation(ctx, event)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Rejected)
		assert.False(t, result.Duplicate)
		assert.NotEmpty(t, result.Reason)

		after, err := repo.GetBySyncID(ctx, syncID)
		require.NoError(t, err)
		assert.Equal(t, reprojectFrom, after.Project, "the row must not have moved")
	})
}

// TestMigration023_DoesNotRevalidateTheJournalOnEveryBoot pins the cost of a
// restart, not a schema shape.
//
// This module has no migration ledger: migrations.Ordered() replays the whole
// slice on every boot. An unguarded DROP CONSTRAINT + ADD CONSTRAINT therefore
// takes ACCESS EXCLUSIVE on memory_mutations and full-scans the journal to
// validate the check — every single boot, at a cost that grows with the journal.
// On a single-instance deploy that is plain downtime.
//
// A constraint's OID is the observable proof: re-adding it mints a new one, so a
// stable OID across a replay means the migration recognised its own work and did
// nothing.
func TestMigration023_DoesNotRevalidateTheJournalOnEveryBoot(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	before := memoryMutationsOpConstraintOID(ctx, t, pool)

	require.NoError(t, RunMigrations(pool, migrations.ReprojectMutationSQL), "migration 023 must replay cleanly")

	after := memoryMutationsOpConstraintOID(ctx, t, pool)
	assert.Equal(t, before, after,
		"a replay must leave the existing constraint alone — dropping and re-adding it re-scans the whole journal under ACCESS EXCLUSIVE")
}

// TestMigration023_UpgradesTheOldFourOpConstraint is the other half: skipping
// the work must never mean skipping the upgrade. A database that predates this
// migration carries a constraint with the SAME NAME and only four ops, so a
// guard that tested the name alone would leave it there and reject every
// reproject the feature exists to journal.
func TestMigration023_UpgradesTheOldFourOpConstraint(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()

	// Rewind to the pre-023 shape: same constraint name, no 'reproject'.
	_, err := pool.Exec(ctx, `
		ALTER TABLE memory_mutations DROP CONSTRAINT chk_memory_mutations_op;
		ALTER TABLE memory_mutations ADD CONSTRAINT chk_memory_mutations_op
			CHECK (op IN ('create', 'update', 'delete', 'restore'))`)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(pool, migrations.ReprojectMutationSQL))

	_, err = pool.Exec(ctx, `
		INSERT INTO memory_mutations
			(event_id, entity_type, entity_sync_id, project, op, occurred_at, reproject)
		VALUES ('9b0e8400-e29b-41d4-a716-446655440001', 'memory',
		        '9b0e8400-e29b-41d4-a716-446655440101', $1, 'reproject', now(), $2)`,
		reprojectTo, []byte(`{"from_project":"Foo.Bar","to_project":"foo-bar"}`))
	require.NoError(t, err, "an old four-op constraint must be replaced, not left in place")
}

func memoryMutationsOpConstraintOID(ctx context.Context, t *testing.T, pool *pgxpool.Pool) uint32 {
	t.Helper()
	var oid uint32
	err := pool.QueryRow(ctx, `
		SELECT oid FROM pg_constraint
		WHERE conrelid = 'memory_mutations'::regclass AND conname = 'chk_memory_mutations_op'`).Scan(&oid)
	require.NoError(t, err, "migration 023 must leave the op check constraint in place")
	return oid
}

// TestApplyMemoryMutation_ReprojectRefusesToCarryAWritePayload closes a door
// nobody was watching.
//
// reprojectInstructionError validated the reproject block and nothing else, and
// the reproject branch skips the memoryBySyncIDForUpdate precondition every
// other op runs. So a caller could send op=reproject with a well-formed
// reproject block AND a memory payload whose project matches the envelope:
// insertMemoryMutation marshals mutation.Memory unconditionally and
// ListMemoryMutations unmarshals it straight back out, so that payload reached
// every daemon pulling the target project.
//
// It was never written to `memories`, which is what makes it dangerous rather
// than merely redundant: invisible to list, search, admin and the quarantine
// export, while carrying the same weight on a client as a `create`.
//
// A reproject changes one column. It has no business carrying content at all.
func TestApplyMemoryMutation_ReprojectRefusesToCarryAWritePayload(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)

	tests := []struct {
		name    string
		eventID string
		syncID  string
		mutate  func(*model.MutationEnvelope)
	}{
		{
			name:    "a memory payload",
			eventID: "8a2e8400-e29b-41d4-a716-446655440001",
			syncID:  "8a2e8400-e29b-41d4-a716-446655440101",
			mutate: func(m *model.MutationEnvelope) {
				m.Memory = &model.MemoryPayload{
					SyncID:  m.EntitySyncID,
					Project: reprojectTo,
					Title:   "smuggled",
					Content: "content no server table ever stores",
				}
			},
		},
		{
			name:    "a tombstone payload",
			eventID: "8a2e8400-e29b-41d4-a716-446655440002",
			syncID:  "8a2e8400-e29b-41d4-a716-446655440102",
			mutate: func(m *model.MutationEnvelope) {
				m.Tombstone = &model.TombstonePayload{DeletedAt: time.Now().UTC(), DeletedBy: "attacker", Reason: "smuggled"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedReprojectMemory(ctx, t, pool, tt.syncID)
			event := reprojectEvent(tt.eventID, tt.syncID, reprojectFrom, reprojectTo)
			tt.mutate(&event)

			result, err := repo.ApplyMemoryMutation(ctx, event)
			require.NoError(t, err, "a malformed instruction must not fail the whole batch")
			require.NotNil(t, result)
			assert.True(t, result.Rejected)
			assert.False(t, result.Applied)
			assert.NotEmpty(t, result.Reason)
			assertNoMemoryMutationRow(t, pool, event.EventID)

			after, err := repo.GetBySyncID(ctx, tt.syncID)
			require.NoError(t, err)
			assert.Equal(t, reprojectFrom, after.Project, "a rejected reproject moves nothing")

			batch, err := repo.ListMemoryMutations(ctx, reprojectTo, model.MutationCursor{}, 10)
			require.NoError(t, err)
			for _, delivered := range batch.Events {
				assert.NotEqual(t, event.EventID, delivered.EventID,
					"the payload must never reach a puller")
			}
		})
	}
}
