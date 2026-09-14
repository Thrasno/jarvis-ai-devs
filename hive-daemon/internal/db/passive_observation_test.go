package db

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
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

func TestSavePassiveObservationWithSession_MaterializesAndPreservesIdentity(t *testing.T) {
	for _, ended := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "ended"}[ended], func(t *testing.T) {
			d := openTestDB(t)
			var before *models.Session
			if ended {
				require.NoError(t, d.CreateSession("passive-session", "alpha", "/original", "first-dev", "first-client"))
				require.NoError(t, d.EndSession("passive-session", "ended"))
				_, err := d.sqlDB.Exec(`UPDATE sessions SET sync_from_project = 'source' WHERE id = 'passive-session'`)
				require.NoError(t, err)
				before, err = d.GetSession("passive-session")
				require.NoError(t, err)
			}
			err := d.SavePassiveObservationWithSession(context.Background(), models.PassiveObservationWrite{
				Session: models.SessionInput{ID: "passive-session", Project: "Alpha", Directory: "/new", DevID: "new-dev", Client: "http"},
				Source:  "subagent", Content: "capture",
			})
			require.NoError(t, err)
			session, err := d.GetSession("passive-session")
			require.NoError(t, err)
			require.Nil(t, session.EndedAt)
			if ended {
				require.Equal(t, before.SyncID, session.SyncID)
				require.True(t, before.StartedAt.Equal(session.StartedAt))
				require.Equal(t, "/original", session.Directory)
				require.Equal(t, "first-dev", session.DevID)
				require.Equal(t, "first-client", session.Client)
				var syncFromProject string
				require.NoError(t, d.sqlDB.QueryRow(`SELECT sync_from_project FROM sessions WHERE id = 'passive-session'`).Scan(&syncFromProject))
				require.Equal(t, "source", syncFromProject)
			} else {
				require.Equal(t, "http", session.Client)
			}
			var projectName, source, content string
			require.NoError(t, d.sqlDB.QueryRow(`SELECT project, source, content FROM passive_observations`).Scan(&projectName, &source, &content))
			require.Equal(t, "alpha", projectName)
			require.Equal(t, "subagent", source)
			require.Equal(t, "capture", content)
		})
	}
}

func TestSavePassiveObservationWithSession_RollsBackValidationAndTriggers(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*DB)
		input models.SessionInput
		check func(*testing.T, error)
	}{
		{
			name: "session trigger",
			setup: func(d *DB) {
				_, _ = d.sqlDB.Exec(`CREATE TRIGGER abort_session BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'session'); END`)
			},
			input: models.SessionInput{ID: "passive-session", Project: "alpha"},
			check: func(t *testing.T, err error) { require.Error(t, err) },
		},
		{
			name: "observation trigger",
			setup: func(d *DB) {
				_, _ = d.sqlDB.Exec(`CREATE TRIGGER abort_passive BEFORE INSERT ON passive_observations BEGIN SELECT RAISE(ABORT, 'observation'); END`)
			},
			input: models.SessionInput{ID: "passive-session", Project: "alpha"},
			check: func(t *testing.T, err error) { require.Error(t, err) },
		},
		{
			name: "mismatch",
			setup: func(d *DB) {
				require.NoError(t, d.CreateSession("passive-session", "alpha", "", "dev", "client"))
			},
			input: models.SessionInput{ID: "passive-session", Project: "beta"},
			check: func(t *testing.T, err error) {
				var validation *project.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, project.CodeProjectSessionMismatch, validation.Code)
			},
		},
		{
			name: "blocked",
			setup: func(d *DB) {
				require.NoError(t, d.CreateSession("passive-session", "alpha", "", "dev", "client"))
				_, _ = d.sqlDB.Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES ('alpha', 'alpha', 'block', CURRENT_TIMESTAMP)`)
			},
			input: models.SessionInput{ID: "passive-session", Project: "alpha"},
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrProjectBlocked) },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			tt.setup(d)
			err := d.SavePassiveObservationWithSession(context.Background(), models.PassiveObservationWrite{Session: tt.input, Content: "capture"})
			tt.check(t, err)
			var observations int
			require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM passive_observations`).Scan(&observations))
			require.Zero(t, observations)
			if tt.name == "session trigger" || tt.name == "observation trigger" {
				var sessions int
				require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'passive-session'`).Scan(&sessions))
				require.Zero(t, sessions)
			}
		})
	}
}

func TestSavePassiveObservationWithSession_RollsBackReopenWhenObservationFails(t *testing.T) {
	d := openTestDB(t)
	require.NoError(t, d.CreateSession("passive-session", "alpha", "/original", "dev", "client"))
	require.NoError(t, d.EndSession("passive-session", "ended"))
	_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_passive_reopen BEFORE INSERT ON passive_observations BEGIN SELECT RAISE(ABORT, 'observation'); END`)
	require.NoError(t, err)

	err = d.SavePassiveObservationWithSession(context.Background(), models.PassiveObservationWrite{Session: models.SessionInput{ID: "passive-session", Project: "alpha"}, Content: "capture"})
	require.Error(t, err)
	session, getErr := d.GetSession("passive-session")
	require.NoError(t, getErr)
	require.NotNil(t, session.EndedAt)
	require.Equal(t, "ended", session.Summary)
	require.Equal(t, "/original", session.Directory)
	var observations int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM passive_observations`).Scan(&observations))
	require.Zero(t, observations)
}

func TestSavePassiveObservationWithSession_ConcurrentCapturesRemainIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passive.db")
	first, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	start, errs := make(chan struct{}), make(chan error, 2)
	var wg sync.WaitGroup
	for _, d := range []*DB{first, second} {
		wg.Add(1)
		go func(d *DB) {
			defer wg.Done()
			<-start
			errs <- d.SavePassiveObservationWithSession(context.Background(), models.PassiveObservationWrite{Session: models.SessionInput{ID: "passive-session", Project: "alpha", Client: "http"}, Content: "capture"})
		}(d)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var observations int
	require.NoError(t, first.sqlDB.QueryRow(`SELECT COUNT(*) FROM passive_observations`).Scan(&observations))
	require.Equal(t, 2, observations)
}
