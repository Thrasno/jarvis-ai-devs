package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresMemoryRepository_ListActivityFeed_OrdersNewestFirstAndContinuesEqualTimestamps(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, RunMigrations(pool, migrations.MemoryMutationsSQL))
	repo := NewPostgresMemoryRepository(pool)
	sessionID := ensureManualSavePtr(t, pool, "activity-feed-order")
	occurredAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)

	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "850e8400-e29b-41d4-a716-446655440001",
		EntitySyncID: "850e8400-e29b-41d4-a716-446655440101",
		Project:      "activity-feed-order",
		Op:           model.MutationOpCreate,
		OccurredAt:   occurredAt,
		Title:        "first same-time event",
		SessionID:    *sessionID,
	})
	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "850e8400-e29b-41d4-a716-446655440002",
		EntitySyncID: "850e8400-e29b-41d4-a716-446655440102",
		Project:      "activity-feed-order",
		Op:           model.MutationOpCreate,
		OccurredAt:   occurredAt,
		Title:        "second same-time event",
		SessionID:    *sessionID,
	})
	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "850e8400-e29b-41d4-a716-446655440003",
		EntitySyncID: "850e8400-e29b-41d4-a716-446655440103",
		Project:      "activity-feed-order",
		Op:           model.MutationOpCreate,
		OccurredAt:   occurredAt,
		Title:        "third same-time event",
		SessionID:    *sessionID,
	})

	firstPage, err := repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	assert.Equal(t, []string{
		"850e8400-e29b-41d4-a716-446655440003",
		"850e8400-e29b-41d4-a716-446655440002",
	}, activityEventIDs(firstPage))

	last := firstPage[len(firstPage)-1]
	nextPage, err := repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{
		Limit: 2,
		Cursor: &model.ActivityFeedCursor{
			OccurredAt: last.OccurredAt,
			Sequence:   last.Sequence,
			EventID:    last.EventID,
		},
	})
	require.NoError(t, err)
	require.Len(t, nextPage, 1)
	assert.Equal(t, "850e8400-e29b-41d4-a716-446655440001", nextPage[0].EventID)
}

func TestPostgresMemoryRepository_ListActivityFeed_FiltersUnsupportedRowsBeforeLookahead(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, RunMigrations(pool, migrations.MemoryMutationsSQL))
	require.NoError(t, allowSyncActivityMutationForTest(ctx, pool))
	repo := NewPostgresMemoryRepository(pool)
	sessionID := ensureManualSavePtr(t, pool, "activity-feed-filter")
	base := time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC)

	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "860e8400-e29b-41d4-a716-446655440001",
		EntitySyncID: "860e8400-e29b-41d4-a716-446655440101",
		Project:      "activity-feed-filter",
		Op:           model.MutationOpCreate,
		OccurredAt:   base,
		Title:        "oldest supported",
		SessionID:    *sessionID,
	})
	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "860e8400-e29b-41d4-a716-446655440002",
		EntitySyncID: "860e8400-e29b-41d4-a716-446655440102",
		Project:      "activity-feed-filter",
		Op:           model.MutationOpCreate,
		OccurredAt:   base.Add(time.Minute),
		Title:        "middle supported",
		SessionID:    *sessionID,
	})
	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "860e8400-e29b-41d4-a716-446655440003",
		EntitySyncID: "860e8400-e29b-41d4-a716-446655440103",
		Project:      "activity-feed-filter",
		Op:           model.MutationOpCreate,
		OccurredAt:   base.Add(2 * time.Minute),
		Title:        "newest supported",
		SessionID:    *sessionID,
	})
	insertUnsupportedActivityMutation(t, ctx, pool, "860e8400-e29b-41d4-a716-446655440004", "activity-feed-filter", model.MutationOpRestore, base.Add(3*time.Minute))
	insertUnsupportedActivityMutation(t, ctx, pool, "860e8400-e29b-41d4-a716-446655440005", "activity-feed-filter", model.MutationOp("sync"), base.Add(4*time.Minute))

	rows, err := repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 3})
	require.NoError(t, err)
	require.Len(t, rows, 3, "unsupported restore/sync rows must not consume the repository lookahead")
	assert.Equal(t, []string{
		"860e8400-e29b-41d4-a716-446655440003",
		"860e8400-e29b-41d4-a716-446655440002",
		"860e8400-e29b-41d4-a716-446655440001",
	}, activityEventIDs(rows))
}

func TestPostgresMemoryRepository_ListActivityFeed_ReturnsEmptyFeed(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, RunMigrations(pool, migrations.MemoryMutationsSQL))
	repo := NewPostgresMemoryRepository(pool)

	rows, err := repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 10})

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestPostgresMemoryRepository_ListActivityFeedExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, RunMigrations(pool, migrations.MemoryMutationsSQL))
	require.NoError(t, RunMigrations(pool, migrations.ProjectBlocksSQL))
	require.NoError(t, RunMigrations(pool, migrations.QuarantineContractSQL))
	repo := NewPostgresMemoryRepository(pool)
	blockRepo := NewPostgresProjectBlockRepository(pool)
	visibleSessionID := ensureManualSavePtr(t, pool, "visible-activity")
	blockedSessionID := ensureManualSavePtr(t, pool, "Blocked Activity")
	base := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "870e8400-e29b-41d4-a716-446655440001",
		EntitySyncID: "870e8400-e29b-41d4-a716-446655440101",
		Project:      "visible-activity",
		Op:           model.MutationOpCreate,
		OccurredAt:   base,
		Title:        "visible event",
		SessionID:    *visibleSessionID,
	})
	seedActivityMutation(t, ctx, repo, activityMutationSeed{
		EventID:      "870e8400-e29b-41d4-a716-446655440002",
		EntitySyncID: "870e8400-e29b-41d4-a716-446655440102",
		Project:      "Blocked Activity",
		Op:           model.MutationOpCreate,
		OccurredAt:   base.Add(time.Minute),
		Title:        "blocked event",
		SessionID:    *blockedSessionID,
	})
	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Activity",
		CanonicalProjectKey: "Blocked Activity",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-activity",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	rows, err := repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, []string{"870e8400-e29b-41d4-a716-446655440001"}, activityEventIDs(rows))
}

func TestMigration008_ActivityFeedIndexIsIdempotent(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	require.NoError(t, RunMigrations(pool, migrations.MemoryMutationsSQL))
	require.NoError(t, RunMigrations(pool, migrations.ActivityFeedIndexSQL))
	require.NoError(t, RunMigrations(pool, migrations.ActivityFeedIndexSQL))

	var definition string
	err := pool.QueryRow(context.Background(), `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = 'public' AND indexname = 'idx_memory_mutations_activity_feed_recency'`).Scan(&definition)
	require.NoError(t, err)
	assert.Contains(t, definition, "occurred_at DESC")
	assert.Contains(t, definition, "sequence DESC")
	assert.Contains(t, definition, "event_id DESC")
	assert.Contains(t, definition, "entity_type")
	assert.Contains(t, definition, "op")
}

type activityMutationSeed struct {
	EventID      string
	EntitySyncID string
	Project      string
	Op           model.MutationOp
	OccurredAt   time.Time
	Title        string
	SessionID    string
}

func seedActivityMutation(t *testing.T, ctx context.Context, repo MemoryRepository, seed activityMutationSeed) {
	t.Helper()

	mutation := model.MutationEnvelope{
		EventID:      seed.EventID,
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: seed.EntitySyncID,
		Project:      seed.Project,
		Op:           seed.Op,
		OccurredAt:   seed.OccurredAt,
		ActorID:      "activity-tester",
	}
	if seed.Op == model.MutationOpDelete {
		mutation.Tombstone = &model.TombstonePayload{DeletedAt: seed.OccurredAt, DeletedBy: "activity-tester", Reason: "cleanup"}
	} else {
		mutation.Memory = &model.MemoryPayload{
			SyncID:    seed.EntitySyncID,
			Project:   seed.Project,
			TopicKey:  stringPtr("activity/" + seed.EventID),
			Category:  model.CatDecision,
			Title:     seed.Title,
			Content:   seed.Title + " content",
			CreatedBy: "activity-tester",
			CreatedAt: seed.OccurredAt,
			UpdatedAt: seed.OccurredAt,
			SessionID: seed.SessionID,
		}
	}

	result, err := repo.ApplyMemoryMutation(ctx, mutation)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Applied, "seed mutation %s should be applied", seed.EventID)
}

func activityEventIDs(rows []model.ActivityJournalRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.EventID)
	}
	return ids
}

func allowSyncActivityMutationForTest(ctx context.Context, pool queryExecer) error {
	_, err := pool.Exec(ctx, `ALTER TABLE memory_mutations DROP CONSTRAINT IF EXISTS chk_memory_mutations_op`)
	return err
}

func insertUnsupportedActivityMutation(t *testing.T, ctx context.Context, pool queryExecer, eventID string, project string, op model.MutationOp, occurredAt time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_mutations (event_id, entity_type, entity_sync_id, project, op, occurred_at, actor_id)
		VALUES ($1, 'memory', '860e8400-e29b-41d4-a716-446655440999', $2, $3, $4, 'activity-tester')`, eventID, project, op, occurredAt)
	require.NoError(t, err)
}

type queryExecer interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
}
