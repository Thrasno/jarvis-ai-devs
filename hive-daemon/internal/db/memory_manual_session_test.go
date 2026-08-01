package db

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
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

func requireTableCount(t *testing.T, d *DB, table string, want int) {
	t.Helper()
	var got int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&got))
	require.Equal(t, want, got, table)
}
