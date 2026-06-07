package governance

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/stretchr/testify/require"
)

func TestWarningsServiceRecordsAndListsWarnings(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	service := NewWarningsService(store)

	warning, err := service.Record(context.Background(), WarningInput{
		Severity: "warning",
		Source:   "startup",
		Message:  "sync disabled; running local-only",
	})
	require.NoError(t, err)
	require.Equal(t, "active", warning.ResolutionState)

	got, err := service.List(context.Background(), WarningFilter{ResolutionState: "active"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, warning.ID, got[0].ID)
	require.Equal(t, "warning", got[0].Severity)
	require.Equal(t, "startup", got[0].Source)
	require.Equal(t, "sync disabled; running local-only", got[0].Message)
}

func TestWarningsServiceRejectsIncompleteWarnings(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	service := NewWarningsService(store)

	_, err = service.Record(context.Background(), WarningInput{
		Severity: "warning",
		Source:   "startup",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "message is required")
}
