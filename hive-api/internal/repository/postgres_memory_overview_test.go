package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// newTestMemory creates a minimal valid memory for testing.
func newTestMemory(syncID, project string, sessionID *string, now time.Time) *model.Memory {
	return &model.Memory{
		SyncID:    "550e8400-e29b-41d4-a716-" + padSyncSuffix(syncID),
		Project:   project,
		Category:  model.CatDecision,
		Title:     "test " + syncID,
		Content:   "content for " + syncID,
		CreatedBy: "test",
		CreatedAt: now,
		UpdatedAt: now,
		SessionID: sessionID,
	}
}

func padSyncSuffix(s string) string {
	b := []byte("000000000000")
	copy(b[len(b)-len(s):], s)
	return string(b)
}
