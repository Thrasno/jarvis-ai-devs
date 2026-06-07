package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpen_HiveWarningsSchemaExists(t *testing.T) {
	d := openTestDB(t)

	for _, tt := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "hive_warnings"},
		{kind: "index", name: "idx_hive_warnings_created_at"},
		{kind: "index", name: "idx_hive_warnings_resolution_state"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type = ? AND name = ?", tt.kind, tt.name,
			).Scan(&name)
			require.NoErrorf(t, err, "%s %s should exist", tt.kind, tt.name)
		})
	}

	for _, column := range []string{"id", "created_at", "severity", "source", "message", "resolution_state", "resolved_at"} {
		t.Run("column "+column, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM pragma_table_info('hive_warnings') WHERE name = ?", column,
			).Scan(&name)
			require.NoErrorf(t, err, "column %s should exist", column)
		})
	}
}

func TestDB_HiveWarningsPersistence(t *testing.T) {
	d := openTestDB(t)

	first, err := d.SaveHiveWarning(HiveWarningInput{
		Severity: "warning",
		Source:   "startup",
		Message:  "sync disabled; running local-only",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), first.ID)
	require.Equal(t, "warning", first.Severity)
	require.Equal(t, "startup", first.Source)
	require.Equal(t, "sync disabled; running local-only", first.Message)
	require.Equal(t, "active", first.ResolutionState)
	require.Nil(t, first.ResolvedAt)
	require.False(t, first.CreatedAt.IsZero())

	_, err = d.SaveHiveWarning(HiveWarningInput{
		Severity: "critical",
		Source:   "config",
		Message:  "sync token is expired",
	})
	require.NoError(t, err)

	warnings, err := d.ListHiveWarnings(HiveWarningFilter{ResolutionState: "active"})
	require.NoError(t, err)
	require.Len(t, warnings, 2)
	// Newer warnings sort first: ORDER BY created_at DESC, id DESC.
	require.Equal(t, "critical", warnings[0].Severity)
	require.Equal(t, "warning", warnings[1].Severity)
}

func TestDB_HiveWarningsPersistAcrossFileBackedReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	d, err := Open(dbPath)
	require.NoError(t, err)

	active, err := d.SaveHiveWarning(HiveWarningInput{
		Severity: "warning",
		Source:   "startup",
		Message:  "sync disabled; running local-only",
	})
	require.NoError(t, err)

	resolved, err := d.SaveHiveWarning(HiveWarningInput{
		Severity: "critical",
		Source:   "config",
		Message:  "sync token is expired",
	})
	require.NoError(t, err)
	resolvedAt := time.Date(2026, 6, 6, 18, 30, 0, 0, time.UTC)
	require.NoError(t, d.ResolveHiveWarning(resolved.ID, resolvedAt))
	require.NoError(t, d.Close())

	reopened, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	activeWarnings, err := reopened.ListHiveWarnings(HiveWarningFilter{ResolutionState: "active"})
	require.NoError(t, err)
	require.Len(t, activeWarnings, 1)
	require.Equal(t, active.ID, activeWarnings[0].ID)
	require.Equal(t, "warning", activeWarnings[0].Severity)
	require.Equal(t, "startup", activeWarnings[0].Source)
	require.Equal(t, "sync disabled; running local-only", activeWarnings[0].Message)
	require.Equal(t, "active", activeWarnings[0].ResolutionState)
	require.Nil(t, activeWarnings[0].ResolvedAt)

	resolvedWarnings, err := reopened.ListHiveWarnings(HiveWarningFilter{ResolutionState: "resolved"})
	require.NoError(t, err)
	require.Len(t, resolvedWarnings, 1)
	require.Equal(t, resolved.ID, resolvedWarnings[0].ID)
	require.Equal(t, "critical", resolvedWarnings[0].Severity)
	require.Equal(t, "config", resolvedWarnings[0].Source)
	require.Equal(t, "sync token is expired", resolvedWarnings[0].Message)
	require.Equal(t, "resolved", resolvedWarnings[0].ResolutionState)
	require.NotNil(t, resolvedWarnings[0].ResolvedAt)
	require.Equal(t, resolvedAt, *resolvedWarnings[0].ResolvedAt)

}

func TestDB_HiveWarningsResolutionStateConstraint(t *testing.T) {
	d := openTestDB(t)

	_, err := d.sqlDB.Exec(`
INSERT INTO hive_warnings (severity, source, message, resolution_state)
VALUES (?, ?, ?, ?)`, "warning", "test", "invalid state", "dismissed")
	require.Error(t, err)
}

func TestDB_ResolveHiveWarningUpdatesResolutionState(t *testing.T) {
	d := openTestDB(t)

	warning, err := d.SaveHiveWarning(HiveWarningInput{
		Severity: "warning",
		Source:   "sync",
		Message:  "remote sync failed",
	})
	require.NoError(t, err)

	resolvedAt := time.Date(2026, 6, 6, 18, 30, 0, 0, time.UTC)
	require.NoError(t, d.ResolveHiveWarning(warning.ID, resolvedAt))

	active, err := d.ListHiveWarnings(HiveWarningFilter{ResolutionState: "active"})
	require.NoError(t, err)
	require.Empty(t, active)

	resolved, err := d.ListHiveWarnings(HiveWarningFilter{ResolutionState: "resolved"})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	require.Equal(t, "resolved", resolved[0].ResolutionState)
	require.NotNil(t, resolved[0].ResolvedAt)
	require.Equal(t, resolvedAt, *resolved[0].ResolvedAt)
}
