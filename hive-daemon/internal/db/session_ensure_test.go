package db

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSession_CreatesAbsentAttributedSession(t *testing.T) {
	d := openTestDB(t)

	session, err := d.EnsureSession(context.Background(), models.SessionInput{
		ID:        "ensure-absent",
		Project:   "Alpha Project",
		Directory: t.TempDir(),
		DevID:     "dev-1",
		Client:    "hook",
	})
	require.NoError(t, err)

	assert.Equal(t, "ensure-absent", session.ID)
	assert.Equal(t, "alpha-project", session.Project)
	assert.Equal(t, "dev-1", session.DevID)
	assert.Equal(t, "hook", session.Client)
	assert.Nil(t, session.EndedAt)
	assert.Nil(t, session.SyncedAt)

	unknownClient, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "ensure-unknown-client", Project: "Alpha Project"})
	require.NoError(t, err)
	assert.Equal(t, "unknown", unknownClient.Client)
}

func TestEnsureSession_ReusesAndReopensCompatibleSessions(t *testing.T) {
	for _, tt := range []struct {
		name  string
		ended bool
	}{
		{name: "active session"},
		{name: "ended session", ended: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			require.NoError(t, d.CreateSession("ensure-existing", "Alpha", "/original", "first-dev", "first-client"))
			before, err := d.GetSession("ensure-existing")
			require.NoError(t, err)
			if tt.ended {
				require.NoError(t, d.EndSession("ensure-existing", "first summary"))
				require.NoError(t, d.MarkSessionSynced("ensure-existing", time.Now().UTC()))
			}

			after, err := d.EnsureSession(context.Background(), models.SessionInput{
				ID: "ensure-existing", Project: "alpha", Directory: "/variant", DevID: "second-dev", Client: "second-client",
			})
			require.NoError(t, err)
			assert.Equal(t, before.SyncID, after.SyncID)
			assert.True(t, before.StartedAt.Equal(after.StartedAt))
			assert.Equal(t, "/original", after.Directory)
			assert.Equal(t, "first-dev", after.DevID)
			assert.Equal(t, "first-client", after.Client)
			assert.Nil(t, after.EndedAt)
			if tt.ended {
				assert.Nil(t, after.SyncedAt)
			}
		})
	}
}

func TestEnsureSession_RejectsCanonicalProjectMismatch(t *testing.T) {
	d := openTestDB(t)
	require.NoError(t, d.CreateSession("ensure-mismatch", "alpha", "", "dev", "client"))

	_, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "ensure-mismatch", Project: "beta"})
	var validationErr *project.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, project.CodeProjectSessionMismatch, validationErr.Code)

	session, getErr := d.GetSession("ensure-mismatch")
	require.NoError(t, getErr)
	assert.Equal(t, "alpha", session.Project)
}

func TestEnsureSession_HealsEmptyDeveloperID(t *testing.T) {
	d := openTestDB(t)
	require.NoError(t, d.CreateSession("ensure-heal-dev", "alpha", "", "dev", "client"))
	require.NoError(t, d.MarkSessionSynced("ensure-heal-dev", time.Now().UTC()))
	_, err := d.sqlDB.Exec(`UPDATE sessions SET dev_id = '   ' WHERE id = 'ensure-heal-dev'`)
	require.NoError(t, err)

	session, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "ensure-heal-dev", Project: "alpha"})
	require.NoError(t, err)
	assert.Equal(t, resolveDevID(), session.DevID)
	assert.Nil(t, session.SyncedAt)
}

func TestEnsureSession_RollbackKeepsPriorState(t *testing.T) {
	t.Run("failed insert leaves no session", func(t *testing.T) {
		d := openTestDB(t)
		_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_ensure_insert BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'insert blocked'); END`)
		require.NoError(t, err)
		_, err = d.EnsureSession(context.Background(), models.SessionInput{ID: "ensure-insert-rollback", Project: "alpha"})
		require.Error(t, err)
		_, err = d.GetSession("ensure-insert-rollback")
		require.ErrorIs(t, err, ErrSessionNotFound)
	})
	t.Run("failed reopen keeps ended synced session", func(t *testing.T) {
		d := openTestDB(t)
		require.NoError(t, d.CreateSession("ensure-reopen-rollback", "alpha", "", "dev", "client"))
		require.NoError(t, d.EndSession("ensure-reopen-rollback", "summary"))
		require.NoError(t, d.MarkSessionSynced("ensure-reopen-rollback", time.Now().UTC()))
		_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_ensure_reopen BEFORE UPDATE OF ended_at ON sessions BEGIN SELECT RAISE(ABORT, 'reopen blocked'); END`)
		require.NoError(t, err)
		_, err = d.EnsureSession(context.Background(), models.SessionInput{ID: "ensure-reopen-rollback", Project: "alpha"})
		require.Error(t, err)
		session, err := d.GetSession("ensure-reopen-rollback")
		require.NoError(t, err)
		assert.NotNil(t, session.EndedAt)
		assert.NotNil(t, session.SyncedAt)
	})
}

func TestEnsureSession_RespectsTargetAndExistingProjectBlocks(t *testing.T) {
	block := func(t *testing.T, d *DB, name string) {
		t.Helper()
		_, err := d.sqlDB.Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, canonicalProjectKey(name), name, "block-"+name)
		require.NoError(t, err)
	}

	t.Run("target block prevents materialization", func(t *testing.T) {
		d := openTestDB(t)
		block(t, d, "alpha")
		_, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "blocked-target", Project: "alpha"})
		require.ErrorIs(t, err, ErrProjectBlocked)
		_, err = d.GetSession("blocked-target")
		require.ErrorIs(t, err, ErrSessionNotFound)
	})
	t.Run("existing blocked project wins before mismatch", func(t *testing.T) {
		d := openTestDB(t)
		require.NoError(t, d.CreateSession("blocked-existing", "alpha", "", "dev", "client"))
		block(t, d, "alpha")
		_, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "blocked-existing", Project: "beta"})
		require.True(t, errors.Is(err, ErrProjectBlocked))
	})
}

func TestEnsureSession_ConcurrentFirstWritesConverge(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "converge.db")
	first, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, d := range []*DB{first, second} {
		wg.Add(1)
		go func(d *DB) {
			defer wg.Done()
			<-start
			_, err := d.EnsureSession(context.Background(), models.SessionInput{ID: "concurrent-ensure", Project: "alpha", Client: "hook"})
			errs <- err
		}(d)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var count int
	require.NoError(t, first.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'concurrent-ensure'`).Scan(&count))
	assert.Equal(t, 1, count)
}
