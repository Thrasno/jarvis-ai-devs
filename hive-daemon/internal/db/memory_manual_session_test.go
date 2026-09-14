package db

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/require"
)

func TestSaveMemoryWithManualSession_RollsBackEveryWriteOnFailure(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *DB, *models.Memory)
	}{
		{
			name: "memory insert",
			prepare: func(t *testing.T, d *DB, _ *models.Memory) {
				_, err := d.sqlDB.Exec(`CREATE TRIGGER fail_memory_insert BEFORE INSERT ON memories BEGIN SELECT RAISE(FAIL, 'memory insert failed'); END`)
				require.NoError(t, err)
			},
		},
		{
			name: "prompt link",
			prepare: func(_ *testing.T, _ *DB, mem *models.Memory) {
				mem.PromptID = 999999
			},
		},
		{
			name: "mutation journal",
			prepare: func(t *testing.T, d *DB, _ *models.Memory) {
				_, err := d.sqlDB.Exec(`CREATE TRIGGER fail_mutation_insert BEFORE INSERT ON memory_mutations BEGIN SELECT RAISE(FAIL, 'mutation insert failed'); END`)
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			mem := &models.Memory{Project: "atomic-project", Title: "Atomic save", Content: "content"}
			tt.prepare(t, d, mem)

			_, err := d.SaveMemoryWithManualSession(mem)
			require.Error(t, err)
			require.Equal(t, "manual-save-atomic-project", mem.SessionID)
			requireTableCount(t, d, "sessions", 0)
			requireTableCount(t, d, "memories", 0)
			requireTableCount(t, d, "memory_prompt_links", 0)
			requireTableCount(t, d, "memory_mutations", 0)
		})
	}
}

func TestSaveMemoryWithManualSession_PreservesPreexistingSessionOnFailure(t *testing.T) {
	d := openTestDB(t)
	_, err := d.EnsureManualSaveSession("existing-project")
	require.NoError(t, err)
	_, err = d.sqlDB.Exec(`CREATE TRIGGER fail_memory_insert BEFORE INSERT ON memories BEGIN SELECT RAISE(FAIL, 'memory insert failed'); END`)
	require.NoError(t, err)

	_, err = d.SaveMemoryWithManualSession(&models.Memory{Project: "existing-project", Title: "Atomic save", Content: "content"})
	require.Error(t, err)
	requireTableCount(t, d, "sessions", 1)
	requireTableCount(t, d, "memories", 0)
}

func TestSaveMemoryWithManualSession_ConcurrentFirstSaves(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, title := range []string{"first", "second"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			<-start
			_, err := d.SaveMemoryWithManualSession(&models.Memory{Project: "concurrent", Title: title, Content: "content"})
			errs <- err
		}(title)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	requireTableCount(t, d, "sessions", 1)
	requireTableCount(t, d, "memories", 2)
	requireTableCount(t, d, "memory_mutations", 2)
}

func TestSaveMemoryWithManualSession_BlockedProjectWritesNothing(t *testing.T) {
	d := openTestDB(t)
	_, err := d.RecordProjectBlock(context.Background(), ProjectBlockCommand{
		CommandID: "block-atomic", AckToken: "ack-atomic", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: time.Now(),
	})
	require.NoError(t, err)

	_, err = d.SaveMemoryWithManualSession(&models.Memory{Project: "blocked", Title: "Atomic save", Content: "content"})
	require.ErrorIs(t, err, ErrProjectBlocked)
	requireTableCount(t, d, "sessions", 0)
	requireTableCount(t, d, "memories", 0)
	requireTableCount(t, d, "memory_mutations", 0)
}

func TestSaveMemoryWithSession_MaterializesReopensAndRollsBack(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, *DB)
	}{
		{name: "absent"},
		{name: "ended", setup: func(t *testing.T, d *DB) {
			require.NoError(t, d.CreateSession("capture", "alpha", "/original", "first", "first-client"))
			require.NoError(t, d.EndSession("capture", "preserved summary"))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			if tt.setup != nil {
				tt.setup(t, d)
			}
			_, err := d.SaveMemoryWithSession(context.Background(), &models.Memory{Project: "alpha", Title: "capture", Content: "content"}, models.SessionInput{ID: "capture", Project: "alpha", Directory: "/capture", DevID: "later", Client: "mcp"})
			require.NoError(t, err)
			session, err := d.GetSession("capture")
			require.NoError(t, err)
			require.Nil(t, session.EndedAt)
			if tt.name == "absent" {
				require.Equal(t, "mcp", session.Client)
			}
			requireTableCount(t, d, "memories", 1)
			requireTableCount(t, d, "memory_mutations", 1)
		})
	}
}

func TestSaveMemoryWithSession_RollsBackSessionMemoryLinkAndJournal(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, *DB, *models.Memory)
	}{
		{name: "session", setup: func(t *testing.T, d *DB, _ *models.Memory) {
			_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_session BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'session'); END`)
			require.NoError(t, err)
		}},
		{name: "memory", setup: func(t *testing.T, d *DB, _ *models.Memory) {
			_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_memory BEFORE INSERT ON memories BEGIN SELECT RAISE(ABORT, 'memory'); END`)
			require.NoError(t, err)
		}},
		{name: "link", setup: func(_ *testing.T, _ *DB, mem *models.Memory) { mem.PromptID = 99999 }},
		{name: "journal", setup: func(t *testing.T, d *DB, _ *models.Memory) {
			_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_journal BEFORE INSERT ON memory_mutations BEGIN SELECT RAISE(ABORT, 'journal'); END`)
			require.NoError(t, err)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			mem := &models.Memory{Project: "alpha", Title: "capture", Content: "content"}
			tt.setup(t, d, mem)
			_, err := d.SaveMemoryWithSession(context.Background(), mem, models.SessionInput{ID: "capture", Project: "alpha", Client: "mcp"})
			require.Error(t, err)
			requireTableCount(t, d, "sessions", 0)
			requireTableCount(t, d, "memories", 0)
			requireTableCount(t, d, "memory_prompt_links", 0)
			requireTableCount(t, d, "memory_mutations", 0)
		})
	}
}

func TestSaveMemoryWithSession_EndedSessionReopenRollsBackDownstreamFailure(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *DB, *models.Memory)
	}{
		{
			name: "memory insert trigger",
			prepare: func(t *testing.T, d *DB, _ *models.Memory) {
				_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_memory BEFORE INSERT ON memories BEGIN SELECT RAISE(ABORT, 'memory'); END`)
				require.NoError(t, err)
			},
		},
		{
			name: "prompt link foreign key",
			prepare: func(_ *testing.T, _ *DB, mem *models.Memory) {
				mem.PromptID = 999999
			},
		},
		{
			name: "mutation journal trigger",
			prepare: func(t *testing.T, d *DB, _ *models.Memory) {
				_, err := d.sqlDB.Exec(`CREATE TRIGGER abort_journal BEFORE INSERT ON memory_mutations BEGIN SELECT RAISE(ABORT, 'journal'); END`)
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			require.NoError(t, d.CreateSession("summary", "alpha", "/original", "first", "first-client"))
			require.NoError(t, d.EndSession("summary", "prior end"))
			before, err := d.GetSession("summary")
			require.NoError(t, err)
			mem := &models.Memory{Project: "alpha", Title: "summary", Content: "content", Category: "session_summary"}
			tt.prepare(t, d, mem)

			_, err = d.SaveMemoryWithSession(context.Background(), mem, models.SessionInput{ID: "summary", Project: "alpha", Directory: "/later", DevID: "later", Client: "mcp"})
			require.Error(t, err)
			after, err := d.GetSession("summary")
			require.NoError(t, err)
			require.NotNil(t, after.EndedAt)
			require.True(t, after.EndedAt.Equal(*before.EndedAt))
			require.Equal(t, before.Summary, after.Summary)
			require.Equal(t, before.SyncID, after.SyncID)
			require.Equal(t, before.Project, after.Project)
			require.Equal(t, before.Directory, after.Directory)
			require.Equal(t, before.DevID, after.DevID)
			require.Equal(t, before.Client, after.Client)
			require.True(t, after.StartedAt.Equal(before.StartedAt))
			requireTableCount(t, d, "memories", 0)
			requireTableCount(t, d, "memory_prompt_links", 0)
			requireTableCount(t, d, "memory_mutations", 0)
		})
	}
}

func TestSaveMemoryWithSession_RejectsMismatchAndBlockedProject(t *testing.T) {
	d := openTestDB(t)
	require.NoError(t, d.CreateSession("capture", "alpha", "", "dev", "mcp"))
	_, err := d.SaveMemoryWithSession(context.Background(), &models.Memory{Project: "beta", Title: "capture", Content: "content"}, models.SessionInput{ID: "capture", Project: "beta", Client: "mcp"})
	var validation *project.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, project.CodeProjectSessionMismatch, validation.Code)
	requireTableCount(t, d, "memories", 0)
	_, err = d.RecordProjectBlock(context.Background(), ProjectBlockCommand{CommandID: "block", AckToken: "ack", Project: "alpha", CanonicalProjectKey: "alpha", BlockedAt: time.Now()})
	require.NoError(t, err)
	_, err = d.SaveMemoryWithSession(context.Background(), &models.Memory{Project: "alpha", Title: "capture", Content: "content"}, models.SessionInput{ID: "capture", Project: "alpha", Client: "mcp"})
	require.ErrorIs(t, err, ErrProjectBlocked)
	requireTableCount(t, d, "memories", 0)
}

func requireTableCount(t *testing.T, d *DB, table string, want int) {
	t.Helper()
	var got int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&got))
	require.Equal(t, want, got, table)
}
