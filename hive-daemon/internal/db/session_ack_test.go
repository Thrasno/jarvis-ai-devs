package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAckSessionSnapshot_AcknowledgesExactNullableSnapshot(t *testing.T) {
	d := openTestDB(t)
	require.NoError(t, d.CreateSession("ack-exact", "alpha", "/work/alpha", "dev", "client"))

	sent, err := d.GetSession("ack-exact")
	require.NoError(t, err)
	_, err = d.sqlDB.Exec(`UPDATE sessions SET started_at = ?, sync_from_project = ? WHERE id = ?`, sent.StartedAt.UTC().Format(time.RFC3339), "previous-alpha", sent.ID)
	require.NoError(t, err)
	sent.SyncFromProject = "previous-alpha"

	acknowledged, err := d.AckSessionSnapshot(context.Background(), sent, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.True(t, acknowledged)

	var syncedAt sql.NullString
	var syncFromProject string
	var summary sql.NullString
	require.NoError(t, d.sqlDB.QueryRow(`SELECT synced_at, sync_from_project, summary FROM sessions WHERE id = ?`, sent.ID).Scan(&syncedAt, &syncFromProject, &summary))
	assert.True(t, syncedAt.Valid)
	assert.Empty(t, syncFromProject)
	assert.False(t, summary.Valid, "a NULL summary must match the empty summary decoded into the snapshot")
}

func TestAckSessionSnapshot_LeavesNewerDirtyStatePending(t *testing.T) {
	for _, tt := range []struct {
		name    string
		prepare func(t *testing.T, d *DB) *models.Session
		mutate  func(t *testing.T, d *DB)
	}{
		{
			name: "reopen after ended snapshot",
			prepare: func(t *testing.T, d *DB) *models.Session {
				require.NoError(t, d.CreateSession("ack-reopen", "alpha", "", "dev", "client"))
				require.NoError(t, d.EndSession("ack-reopen", "closed"))
				sent, err := d.GetSession("ack-reopen")
				require.NoError(t, err)
				return sent
			},
			mutate: func(t *testing.T, d *DB) {
				_, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "ack-reopen", Project: "alpha"})
				require.NoError(t, err)
			},
		},
		{
			name: "end after open snapshot",
			prepare: func(t *testing.T, d *DB) *models.Session {
				require.NoError(t, d.CreateSession("ack-end", "alpha", "", "dev", "client"))
				sent, err := d.GetSession("ack-end")
				require.NoError(t, err)
				return sent
			},
			mutate: func(t *testing.T, d *DB) {
				require.NoError(t, d.EndSession("ack-end", "new summary"))
			},
		},
		{
			name: "relocation source changes after snapshot",
			prepare: func(t *testing.T, d *DB) *models.Session {
				require.NoError(t, d.CreateSession("ack-relocation", "alpha", "", "dev", "client"))
				sent, err := d.GetSession("ack-relocation")
				require.NoError(t, err)
				return sent
			},
			mutate: func(t *testing.T, d *DB) {
				_, err := d.sqlDB.Exec(`UPDATE sessions SET sync_from_project = ? WHERE id = ?`, "previous-alpha", "ack-relocation")
				require.NoError(t, err)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			sent := tt.prepare(t, d)
			tt.mutate(t, d)

			acknowledged, err := d.AckSessionSnapshot(context.Background(), sent, time.Now().UTC())
			require.NoError(t, err)
			assert.False(t, acknowledged)

			var syncedAt sql.NullString
			require.NoError(t, d.sqlDB.QueryRow(`SELECT synced_at FROM sessions WHERE id = ?`, sent.ID).Scan(&syncedAt))
			assert.False(t, syncedAt.Valid, "a stale acknowledgement must leave the newer state dirty")
		})
	}
}

func TestAckSessionSnapshot_AcknowledgesEquivalentRestoredState(t *testing.T) {
	t.Run("ended timestamp stored as RFC3339", func(t *testing.T) {
		d := openTestDB(t)
		require.NoError(t, d.CreateSession("ack-ended", "alpha", "", "dev", "client"))
		require.NoError(t, d.EndSession("ack-ended", "closed"))
		sent, err := d.GetSession("ack-ended")
		require.NoError(t, err)
		require.NotNil(t, sent.EndedAt)
		_, err = d.sqlDB.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, sent.EndedAt.UTC().Format(time.RFC3339), sent.ID)
		require.NoError(t, err)

		acknowledged, err := d.AckSessionSnapshot(context.Background(), sent, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, acknowledged)
	})

	t.Run("state changed then exactly restored", func(t *testing.T) {
		d := openTestDB(t)
		require.NoError(t, d.CreateSession("ack-restored", "alpha", "", "dev", "client"))
		sent, err := d.GetSession("ack-restored")
		require.NoError(t, err)
		require.NoError(t, d.EndSession(sent.ID, "temporary summary"))
		_, err = d.sqlDB.Exec(`UPDATE sessions SET ended_at = NULL, summary = NULL, synced_at = NULL WHERE id = ?`, sent.ID)
		require.NoError(t, err)

		acknowledged, err := d.AckSessionSnapshot(context.Background(), sent, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, acknowledged)
	})
}

func TestAckSessionSnapshot_ReturnsFalseForMissingSession(t *testing.T) {
	acknowledged, err := openTestDB(t).AckSessionSnapshot(context.Background(), &models.Session{ID: "missing"}, time.Now().UTC())
	require.NoError(t, err)
	assert.False(t, acknowledged)
}
