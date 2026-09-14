package db

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureAndEndSession_LifecycleAndDuplicateSemantics(t *testing.T) {
	for _, tt := range []struct {
		name                string
		setup               func(t *testing.T, d *DB)
		rejectAlreadyEnded  bool
		project             string
		wantErr             error
		wantValidationError bool
		wantSummary         string
	}{
		{name: "missing session is created and ended", wantSummary: "new summary"},
		{
			name: "active session is ended and made eligible for sync",
			setup: func(t *testing.T, d *DB) {
				require.NoError(t, d.CreateSession("end-session", "alpha", "/original", "dev", "first-client"))
				require.NoError(t, d.MarkSessionSynced("end-session", time.Now()))
			},
			wantSummary: "new summary",
		},
		{
			name: "already ended session rejects without changing summary",
			setup: func(t *testing.T, d *DB) {
				require.NoError(t, d.CreateSession("end-session", "alpha", "", "dev", "client"))
				require.NoError(t, d.EndSession("end-session", "prior summary"))
			},
			rejectAlreadyEnded: true,
			wantErr:            ErrSessionAlreadyEnded,
			wantSummary:        "prior summary",
		},
		{
			name: "already ended session supports HTTP style no-op",
			setup: func(t *testing.T, d *DB) {
				require.NoError(t, d.CreateSession("end-session", "alpha", "", "dev", "client"))
				require.NoError(t, d.EndSession("end-session", "prior summary"))
			},
			wantSummary: "prior summary",
		},
		{
			name: "canonical project mismatch is typed",
			setup: func(t *testing.T, d *DB) {
				require.NoError(t, d.CreateSession("end-session", "alpha", "", "dev", "client"))
			},
			project:             "beta",
			wantValidationError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			if tt.setup != nil {
				tt.setup(t, d)
			}
			projectName := tt.project
			if projectName == "" {
				projectName = "alpha"
			}

			session, err := d.EnsureAndEndSession(context.Background(), models.SessionEndInput{
				Session:            models.SessionInput{ID: "end-session", Project: projectName, Directory: "/variant", Client: "hook"},
				Summary:            "new summary",
				RejectAlreadyEnded: tt.rejectAlreadyEnded,
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else if tt.wantValidationError {
				var validationErr *project.ValidationError
				require.ErrorAs(t, err, &validationErr)
				assert.Equal(t, project.CodeProjectSessionMismatch, validationErr.Code)
			} else {
				require.NoError(t, err)
				require.NotNil(t, session)
			}

			stored, getErr := d.GetSession("end-session")
			if tt.wantValidationError {
				require.NoError(t, getErr)
				assert.Equal(t, "alpha", stored.Project)
				assert.Nil(t, stored.EndedAt)
				return
			}
			require.NoError(t, getErr)
			assert.NotNil(t, stored.EndedAt)
			assert.Equal(t, tt.wantSummary, stored.Summary)
			assert.Nil(t, stored.SyncedAt)
		})
	}
}

func TestEnsureAndEndSession_RollsBackLifecycleOnEndFailure(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, d *DB)
	}{
		{name: "missing session leaves no row"},
		{
			name: "active synced session stays active and synced",
			setup: func(t *testing.T, d *DB) {
				require.NoError(t, d.CreateSession("rollback-end", "alpha", "", "dev", "client"))
				require.NoError(t, d.MarkSessionSynced("rollback-end", time.Now()))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			if tt.setup != nil {
				tt.setup(t, d)
			}
			require.NoError(t, mustExec(d, `CREATE TRIGGER abort_end BEFORE UPDATE OF ended_at ON sessions BEGIN SELECT RAISE(ABORT, 'end blocked'); END`))

			_, err := d.EnsureAndEndSession(context.Background(), models.SessionEndInput{
				Session: models.SessionInput{ID: "rollback-end", Project: "alpha"},
			})
			require.Error(t, err)
			stored, getErr := d.GetSession("rollback-end")
			if tt.setup == nil {
				require.ErrorIs(t, getErr, ErrSessionNotFound)
				return
			}
			require.NoError(t, getErr)
			assert.Nil(t, stored.EndedAt)
			assert.NotNil(t, stored.SyncedAt)
		})
	}
}

func TestEnsureAndEndSession_RespectsProjectGate(t *testing.T) {
	d := openTestDB(t)
	_, err := d.sqlDB.Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES ('alpha', 'alpha', 'blocked-end', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	_, err = d.EnsureAndEndSession(context.Background(), models.SessionEndInput{
		Session: models.SessionInput{ID: "blocked-end", Project: "alpha"},
	})
	require.ErrorIs(t, err, ErrProjectBlocked)
	_, err = d.GetSession("blocked-end")
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestEnsureAndEndSession_ConcurrentEndsSerialize(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "end-converge.db")
	first, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, d := range []*DB{first, second} {
		wg.Add(1)
		go func(d *DB) {
			defer wg.Done()
			<-start
			_, err := d.EnsureAndEndSession(context.Background(), models.SessionEndInput{
				Session: models.SessionInput{ID: "concurrent-end", Project: "alpha"},
			})
			errs <- err
		}(d)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		assert.NoError(t, err)
	}
	var count int
	require.NoError(t, first.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'concurrent-end'`).Scan(&count))
	assert.Equal(t, 1, count)
	stored, err := first.GetSession("concurrent-end")
	require.NoError(t, err)
	assert.NotNil(t, stored.EndedAt)
}

func mustExec(d *DB, query string) error {
	_, err := d.sqlDB.Exec(query)
	return err
}
