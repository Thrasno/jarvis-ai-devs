package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── passive_observations table ──────────────────────────────────────────────

func TestPassiveObservationsTableExists(t *testing.T) {
	d := openTestDB(t)

	var name string
	err := d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='passive_observations'",
	).Scan(&name)
	require.NoError(t, err, "passive_observations table should exist after Open")
	assert.Equal(t, "passive_observations", name)
}

func TestPassiveObservationsTableColumns(t *testing.T) {
	d := openTestDB(t)

	expected := []string{
		"id", "session_id", "project", "source", "content", "sync_id", "created_at",
	}

	for _, col := range expected {
		col := col
		t.Run(col, func(t *testing.T) {
			var colName string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM pragma_table_info('passive_observations') WHERE name = ?", col,
			).Scan(&colName)
			require.NoErrorf(t, err, "column %q should exist in passive_observations table", col)
			assert.Equal(t, col, colName)
		})
	}
}

// ─── SavePassiveObservation ───────────────────────────────────────────────────

func TestSavePassiveObservation_HappyPath(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	err := d.SavePassiveObservation(ctx, "sess-1", "my-project", "subagent-stop", "some output content")
	require.NoError(t, err)

	var count int
	err = d.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM passive_observations WHERE session_id = 'sess-1'`,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "one row should be inserted")
}

func TestSavePassiveObservation_InsertsAllFields(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	err := d.SavePassiveObservation(ctx, "sess-abc", "jarvis-dev", "subagent-stop", "hello world")
	require.NoError(t, err)

	var sessionID, project, source, content string
	var syncID *string
	err = d.sqlDB.QueryRow(
		`SELECT session_id, project, source, content, sync_id
		 FROM passive_observations WHERE session_id = 'sess-abc'`,
	).Scan(&sessionID, &project, &source, &content, &syncID)
	require.NoError(t, err)
	assert.Equal(t, "sess-abc", sessionID)
	assert.Equal(t, "jarvis-dev", project)
	assert.Equal(t, "subagent-stop", source)
	assert.Equal(t, "hello world", content)
	assert.Nil(t, syncID, "sync_id should be NULL for hook-originated observations")
}

func TestSavePassiveObservation_EmptySessionID_StoresEmptyString(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Empty session_id is allowed — hook may not have a session ID available.
	err := d.SavePassiveObservation(ctx, "", "my-project", "subagent-stop", "content")
	require.NoError(t, err)

	var count int
	err = d.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM passive_observations WHERE session_id = ''`,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSavePassiveObservation_MultipleInserts_AllPersisted(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, d.SavePassiveObservation(ctx, "sess-1", "proj", "src", "content-1"))
	require.NoError(t, d.SavePassiveObservation(ctx, "sess-1", "proj", "src", "content-2"))
	require.NoError(t, d.SavePassiveObservation(ctx, "sess-2", "proj", "src", "content-3"))

	var count int
	err := d.sqlDB.QueryRow(`SELECT COUNT(*) FROM passive_observations`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}
