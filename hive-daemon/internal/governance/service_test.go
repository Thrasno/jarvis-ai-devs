package governance

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/require"
)

func TestServiceReturnsReadOnlyGovernanceViews(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"))
	_, err = store.SaveMemory(&models.Memory{Project: "alpha", Title: "Read model", Content: "content", SessionID: "sess-alpha"})
	require.NoError(t, err)
	require.NoError(t, store.RecordSyncFailure("alpha", time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC), 2, time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC), errReadOnlyTest))

	service := NewService(store)

	projects, err := service.Projects(context.Background())
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, "alpha", projects[0].Name)
	require.Equal(t, 1, projects[0].ActiveMemoryCount)

	detail, err := service.Project(context.Background(), "alpha")
	require.NoError(t, err)
	require.Equal(t, "/repo/alpha", detail.Directory)

	memories, err := service.Memories(context.Background(), MemoryFilter{Project: "alpha", Limit: 10})
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, "Read model", memories[0].Title)

	health, err := service.Health(context.Background())
	require.NoError(t, err)
	require.Len(t, health, 1)
	require.Equal(t, 2, health[0].ConsecutiveFailures)
}

func TestServiceRejectsBlankProjectDetailAndMemoryFilter(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	service := NewService(store)

	_, err = service.Project(context.Background(), " ")
	require.ErrorContains(t, err, "project is required")

	_, err = service.Memories(context.Background(), MemoryFilter{})
	require.ErrorContains(t, err, "project is required")
}

type readOnlyTestError string

func (e readOnlyTestError) Error() string { return string(e) }

const errReadOnlyTest readOnlyTestError = "read-only test failure"
