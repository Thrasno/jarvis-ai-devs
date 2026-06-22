package repository

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testMemCounter int64

// --- CountByProject tests ---

func TestPostgresMemoryRepository_CountByProject_ReturnsCountsDescending(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	now := time.Now().UTC()

	sessA := ensureManualSavePtr(t, pool, "proj-A")
	sessB := ensureManualSavePtr(t, pool, "proj-B")
	sessC := ensureManualSavePtr(t, pool, "proj-C")

	// proj-A: 3 memories, proj-B: 1 memory, proj-C: 2 memories
	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, newTestMemory("count-a-"+string(rune('0'+i)), "proj-A", sessA, now))
		require.NoError(t, err)
	}
	_, err := repo.Create(ctx, newTestMemory("count-b-0", "proj-B", sessB, now))
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := repo.Create(ctx, newTestMemory("count-c-"+string(rune('0'+i)), "proj-C", sessC, now))
		require.NoError(t, err)
	}

	result, err := repo.CountByProject(ctx, model.MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, result, 3)

	// Must be sorted DESC by count: A(3), C(2), B(1)
	assert.Equal(t, "proj-A", result[0].Project)
	assert.EqualValues(t, 3, result[0].Count)
	assert.Equal(t, "proj-C", result[1].Project)
	assert.EqualValues(t, 2, result[1].Count)
	assert.Equal(t, "proj-B", result[2].Project)
	assert.EqualValues(t, 1, result[2].Count)
}

func TestPostgresMemoryRepository_CountByProject_ExcludesSoftDeleted(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	now := time.Now().UTC()

	sessA := ensureManualSavePtr(t, pool, "proj-del")

	// Create 2 memories, then soft-delete 1
	active, err := repo.Create(ctx, newTestMemory("del-active", "proj-del", sessA, now))
	require.NoError(t, err)
	deleted, err := repo.Create(ctx, newTestMemory("del-deleted", "proj-del", sessA, now))
	require.NoError(t, err)

	// Soft-delete via memory_mutations tombstone
	deletedAt := now.Add(time.Second)
	_, err = repo.ApplyMemoryMutation(ctx, model.MutationEnvelope{
		EventID:      "del-event-1",
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: deleted.SyncID,
		Project:      "proj-del",
		Op:           model.MutationOpDelete,
		OccurredAt:   deletedAt,
	})
	require.NoError(t, err)
	_ = active

	result, err := repo.CountByProject(ctx, model.MemoryFilter{})
	require.NoError(t, err)

	var found bool
	for _, pc := range result {
		if pc.Project == "proj-del" {
			found = true
			assert.EqualValues(t, 1, pc.Count, "soft-deleted memory must be excluded")
		}
	}
	assert.True(t, found, "proj-del should appear with count 1")
}

func TestPostgresMemoryRepository_CountByProject_EmptyTable(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)

	result, err := repo.CountByProject(ctx, model.MemoryFilter{})
	require.NoError(t, err)
	assert.NotNil(t, result, "must return empty slice, not nil")
	assert.Empty(t, result)
}

// --- CountLiveActivity tests ---

func TestPostgresMemoryRepository_CountLiveActivity_ReturnsCountAndNewest(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	now := time.Now().UTC()
	sess := ensureManualSavePtr(t, pool, "live-proj")

	// Insert 3 memories to be in the activity window
	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, newTestMemory("live-"+string(rune('a'+i)), "live-proj", sess, now))
		require.NoError(t, err)
	}

	since := now.Add(-2 * time.Hour)
	count, newestSyncID, err := repo.CountLiveActivity(ctx, since)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.NotEmpty(t, newestSyncID, "newestSyncID must be a non-empty UUID string")
}

func TestPostgresMemoryRepository_CountLiveActivity_EmptyWindow(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)

	// Use a future since time so no memories fall in the window
	since := time.Now().UTC().Add(time.Hour)
	count, newestSyncID, err := repo.CountLiveActivity(ctx, since)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Equal(t, "", newestSyncID)
}

// --- CountGrowthByMonth tests ---

func TestPostgresMemoryRepository_CountGrowthByMonth_ReturnsAscendingPoints(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)
	now := time.Now().UTC()
	sess := ensureManualSavePtr(t, pool, "growth-proj")

	// Insert 2 memories with created_at in the current month
	for i := 0; i < 2; i++ {
		mem := newTestMemory("growth-"+string(rune('a'+i)), "growth-proj", sess, now)
		_, err := repo.Create(ctx, mem)
		require.NoError(t, err)
	}

	result, err := repo.CountGrowthByMonth(ctx, 5)
	require.NoError(t, err)
	assert.Len(t, result, 5, "must return exactly 5 points")

	// Points must be in ascending order (oldest first)
	for i := 1; i < len(result); i++ {
		assert.GreaterOrEqual(t, result[i].Value, result[i-1].Value,
			"cumulative values must be non-decreasing (ascending)")
	}

	// The last point (most recent month) must reflect the 2 memories we added
	last := result[len(result)-1]
	assert.GreaterOrEqual(t, last.Value, 2, "last point must include at least the 2 seeded memories")
}

func TestPostgresMemoryRepository_CountGrowthByMonth_EmptyTable(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresMemoryRepository(pool)

	result, err := repo.CountGrowthByMonth(ctx, 5)
	require.NoError(t, err)
	assert.Len(t, result, 5, "must return exactly 5 points even when table is empty")
	for _, pt := range result {
		assert.Equal(t, 0, pt.Value, "all cumulative values must be 0 for empty table")
	}
}

// newTestMemory creates a minimal valid memory for testing.
// label is used only for human-readable Title/Content; the SyncID is a valid UUID generated from a counter.
func newTestMemory(label, project string, sessionID *string, now time.Time) *model.Memory {
	n := atomic.AddInt64(&testMemCounter, 1)
	syncID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012x", n)
	return &model.Memory{
		SyncID:    syncID,
		Project:   project,
		Category:  model.CatDecision,
		Title:     "test " + label,
		Content:   "content for " + label,
		CreatedBy: "test",
		CreatedAt: now,
		UpdatedAt: now,
		SessionID: sessionID,
	}
}
