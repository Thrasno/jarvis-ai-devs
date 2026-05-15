package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncDB_GetLastSync tests retrieving the last sync timestamp for a project.
func TestSyncDB_GetLastSync(t *testing.T) {
	tests := []struct {
		name       string
		project    string
		setupData  func(*DB)
		wantTime   time.Time
		wantIsZero bool
	}{
		{
			name:       "no sync state exists",
			project:    "test-project",
			setupData:  func(d *DB) {},
			wantIsZero: true,
		},
		{
			name:    "sync state exists",
			project: "test-project",
			setupData: func(d *DB) {
				ts := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
				err := d.SetLastSync("test-project", ts)
				require.NoError(t, err)
			},
			wantTime:   time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
			wantIsZero: false,
		},
		{
			name:    "different project returns zero",
			project: "other-project",
			setupData: func(d *DB) {
				ts := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
				err := d.SetLastSync("test-project", ts)
				require.NoError(t, err)
			},
			wantIsZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			tt.setupData(db)

			got, err := db.GetLastSync(tt.project)
			require.NoError(t, err)

			if tt.wantIsZero {
				assert.True(t, got.IsZero(), "expected zero time, got %v", got)
			} else {
				assert.WithinDuration(t, tt.wantTime, got, time.Second)
			}
		})
	}
}

// TestSyncDB_SetLastSync tests saving the last sync timestamp.
func TestSyncDB_SetLastSync(t *testing.T) {
	tests := []struct {
		name    string
		project string
		time1   time.Time
		time2   time.Time
	}{
		{
			name:    "insert new sync state",
			project: "new-project",
			time1:   time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:    "update existing sync state",
			project: "existing-project",
			time1:   time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
			time2:   time.Date(2026, 4, 15, 14, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			// First save
			err := db.SetLastSync(tt.project, tt.time1)
			require.NoError(t, err)

			got, err := db.GetLastSync(tt.project)
			require.NoError(t, err)
			assert.WithinDuration(t, tt.time1, got, time.Second)

			// Second save (if specified) — should update
			if !tt.time2.IsZero() {
				err = db.SetLastSync(tt.project, tt.time2)
				require.NoError(t, err)

				got, err = db.GetLastSync(tt.project)
				require.NoError(t, err)
				assert.WithinDuration(t, tt.time2, got, time.Second)
			}
		})
	}
}

// TestSyncDB_JWT tests JWT storage and retrieval.
func TestSyncDB_JWT(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		expiresAt  time.Time
		checkAfter time.Duration
		wantToken  string
	}{
		{
			name:       "store and retrieve valid JWT",
			token:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
			expiresAt:  time.Now().Add(2 * time.Hour),
			checkAfter: 0,
			wantToken:  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
		},
		{
			name:       "expired JWT returns empty string",
			token:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.expired",
			expiresAt:  time.Now().Add(-2 * time.Hour), // expired
			checkAfter: 0,
			wantToken:  "", // should return empty for expired
		},
		{
			name:       "JWT expiring within 1 hour returns empty",
			token:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.soon",
			expiresAt:  time.Now().Add(30 * time.Minute), // expires in 30 min
			checkAfter: 0,
			wantToken:  "", // should return empty (< 1 hour margin)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			err := db.SetJWT(tt.token, tt.expiresAt)
			require.NoError(t, err)

			if tt.checkAfter > 0 {
				time.Sleep(tt.checkAfter)
			}

			got := db.GetJWT()
			assert.Equal(t, tt.wantToken, got)
		})
	}
}

// TestSyncDB_JWT_UpdateExisting tests that SetJWT updates existing JWT.
func TestSyncDB_JWT_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// First JWT
	token1 := "token1"
	expires1 := time.Now().Add(2 * time.Hour)
	err := db.SetJWT(token1, expires1)
	require.NoError(t, err)

	got := db.GetJWT()
	assert.Equal(t, token1, got)

	// Update with new JWT
	token2 := "token2"
	expires2 := time.Now().Add(3 * time.Hour)
	err = db.SetJWT(token2, expires2)
	require.NoError(t, err)

	got = db.GetJWT()
	assert.Equal(t, token2, got)
}

func TestSyncDB_GetSyncHealth(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		setupData func(t *testing.T, d *DB)
		assertion func(t *testing.T, got SyncHealth)
	}{
		{
			name:    "missing project returns zero health",
			project: "missing-project",
			setupData: func(t *testing.T, d *DB) {
				t.Helper()
			},
			assertion: func(t *testing.T, got SyncHealth) {
				assert.Equal(t, "missing-project", got.Project)
				assert.True(t, got.LastAttemptAt.IsZero())
				assert.True(t, got.LastSuccessAt.IsZero())
				assert.True(t, got.LastFailureAt.IsZero())
				assert.True(t, got.BackoffUntil.IsZero())
				assert.Zero(t, got.ConsecutiveFailures)
				assert.Empty(t, got.LastError)
			},
		},
		{
			name:    "reads persisted failure health",
			project: "project-a",
			setupData: func(t *testing.T, d *DB) {
				t.Helper()
				attemptAt := time.Date(2026, 5, 8, 11, 0, 0, 0, time.UTC)
				backoffUntil := attemptAt.Add(2 * time.Minute)
				err := d.RecordSyncFailure("project-a", attemptAt, 3, backoffUntil, assert.AnError)
				require.NoError(t, err)
			},
			assertion: func(t *testing.T, got SyncHealth) {
				assert.Equal(t, "project-a", got.Project)
				assert.Equal(t, time.Date(2026, 5, 8, 11, 0, 0, 0, time.UTC), got.LastAttemptAt)
				assert.Equal(t, time.Date(2026, 5, 8, 11, 0, 0, 0, time.UTC), got.LastFailureAt)
				assert.Equal(t, time.Date(2026, 5, 8, 11, 2, 0, 0, time.UTC), got.BackoffUntil)
				assert.Equal(t, 3, got.ConsecutiveFailures)
				assert.Equal(t, assert.AnError.Error(), got.LastError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			tt.setupData(t, db)

			got, err := db.GetSyncHealth(tt.project)
			require.NoError(t, err)
			tt.assertion(t, got)
		})
	}
}

func TestSyncDB_RecordSyncHealthLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		record    func(t *testing.T, d *DB, at time.Time)
		assertion func(t *testing.T, d *DB, at time.Time)
	}{
		{
			name: "record attempt keeps prior failure state",
			record: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				require.NoError(t, d.RecordSyncFailure("project-a", at.Add(-time.Minute), 2, at.Add(3*time.Minute), assert.AnError))
				require.NoError(t, d.RecordSyncAttempt("project-a", at))
			},
			assertion: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				got, err := d.GetSyncHealth("project-a")
				require.NoError(t, err)
				assert.Equal(t, at, got.LastAttemptAt)
				assert.Equal(t, at.Add(-time.Minute), got.LastFailureAt)
				assert.Equal(t, at.Add(3*time.Minute), got.BackoffUntil)
				assert.Equal(t, 2, got.ConsecutiveFailures)
			},
		},
		{
			name: "record success updates last sync and clears failures",
			record: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				require.NoError(t, d.RecordSyncFailure("project-a", at.Add(-time.Minute), 4, at.Add(5*time.Minute), assert.AnError))
				require.NoError(t, d.RecordSyncSuccess("project-a", at))
			},
			assertion: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				got, err := d.GetSyncHealth("project-a")
				require.NoError(t, err)
				assert.Equal(t, at, got.LastAttemptAt)
				assert.Equal(t, at, got.LastSuccessAt)
				assert.True(t, got.LastFailureAt.IsZero())
				assert.True(t, got.BackoffUntil.IsZero())
				assert.Zero(t, got.ConsecutiveFailures)
				assert.Empty(t, got.LastError)

				lastSync, err := d.GetLastSync("project-a")
				require.NoError(t, err)
				assert.Equal(t, at, lastSync)
			},
		},
		{
			name: "record failure sanitizes and caps stored error",
			record: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				message := "  boom\n\x00" + string([]rune{'世'}) + string([]rune{'界'})
				for i := 0; i < 600; i++ {
					message += "x"
				}
				require.NoError(t, d.RecordSyncFailure("project-a", at, 5, at.Add(15*time.Minute), assert.AnError))
				require.NoError(t, d.RecordSyncFailure("project-a", at, 5, at.Add(15*time.Minute), wrappedError(message)))
			},
			assertion: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				got, err := d.GetSyncHealth("project-a")
				require.NoError(t, err)
				assert.Equal(t, at, got.LastAttemptAt)
				assert.Equal(t, at, got.LastFailureAt)
				assert.Equal(t, at.Add(15*time.Minute), got.BackoffUntil)
				assert.Equal(t, 5, got.ConsecutiveFailures)
				assert.NotContains(t, got.LastError, "\x00")
				assert.NotContains(t, got.LastError, "\n")
				assert.LessOrEqual(t, len([]rune(got.LastError)), 500)
				assert.Contains(t, got.LastError, "boom")
			},
		},
		{
			name: "record failure strips raw server body payload",
			record: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				require.NoError(t, d.RecordSyncFailure("project-a", at, 2, at.Add(70*time.Second), wrappedError("sync failed (500): upstream exploded\nsecond line")))
			},
			assertion: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				got, err := d.GetSyncHealth("project-a")
				require.NoError(t, err)
				assert.Equal(t, "sync failed (500)", got.LastError)
				assert.NotContains(t, got.LastError, "upstream exploded")
				assert.NotContains(t, got.LastError, "second line")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			at := time.Date(2026, 5, 8, 11, 30, 0, 0, time.UTC)
			tt.record(t, db, at)
			tt.assertion(t, db, at)
		})
	}
}

func TestSyncDB_HealthWritesKeepJWTAndLastSyncCompatible(t *testing.T) {
	tests := []struct {
		name      string
		setupData func(t *testing.T, d *DB)
		record    func(t *testing.T, d *DB, at time.Time)
		wantJWT   string
		assertion func(t *testing.T, health SyncHealth)
	}{
		{
			name: "failure write preserves jwt auth row",
			setupData: func(t *testing.T, d *DB) {
				t.Helper()
				require.NoError(t, d.SetJWT("jwt-token", time.Now().Add(2*time.Hour)))
			},
			record: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				require.NoError(t, d.RecordSyncFailure("project-a", at, 2, at.Add(time.Minute), assert.AnError))
			},
			wantJWT: "jwt-token",
			assertion: func(t *testing.T, health SyncHealth) {
				assert.Equal(t, 2, health.ConsecutiveFailures)
				assert.Equal(t, assert.AnError.Error(), health.LastError)
			},
		},
		{
			name: "last sync write preserves project health row",
			setupData: func(t *testing.T, d *DB) {
				t.Helper()
				require.NoError(t, d.RecordSyncFailure("project-a", time.Date(2026, 5, 8, 11, 0, 0, 0, time.UTC), 3, time.Date(2026, 5, 8, 11, 5, 0, 0, time.UTC), assert.AnError))
			},
			record: func(t *testing.T, d *DB, at time.Time) {
				t.Helper()
				require.NoError(t, d.SetLastSync("project-a", at))
			},
			assertion: func(t *testing.T, health SyncHealth) {
				assert.Equal(t, 3, health.ConsecutiveFailures)
				assert.Equal(t, time.Date(2026, 5, 8, 11, 5, 0, 0, time.UTC), health.BackoffUntil)
				assert.Equal(t, assert.AnError.Error(), health.LastError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			tt.setupData(t, db)
			at := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
			tt.record(t, db, at)

			assert.Equal(t, tt.wantJWT, db.GetJWT())

			health, err := db.GetSyncHealth("project-a")
			require.NoError(t, err)
			if tt.assertion != nil {
				tt.assertion(t, health)
			}
		})
	}
}

func TestSyncDB_GetSyncHealth_RestartSafeBackoffRead(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restart-safe.db")

	db, err := Open(dbPath)
	require.NoError(t, err)

	at := time.Date(2026, 5, 8, 13, 0, 0, 0, time.UTC)
	require.NoError(t, db.RecordSyncFailure("project-a", at, 4, at.Add(10*time.Minute), assert.AnError))
	require.NoError(t, db.Close())

	reopened, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, reopened.Close())
	})

	got, err := reopened.GetSyncHealth("project-a")
	require.NoError(t, err)
	assert.Equal(t, at.Add(10*time.Minute), got.BackoffUntil)
	assert.Equal(t, 4, got.ConsecutiveFailures)
	assert.Equal(t, assert.AnError.Error(), got.LastError)
}

// setupTestDB creates a temporary SQLite database for testing.
func setupTestDB(t *testing.T) *DB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	require.NoError(t, err, "failed to open test database")

	return db
}

// TestSyncDB_NoJWT tests GetJWT when no JWT exists.
func TestSyncDB_NoJWT(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	got := db.GetJWT()
	assert.Empty(t, got, "expected empty JWT when none stored")
}

// TestSyncDB_GetUnsynced tests retrieving unsynced memories.
func TestSyncDB_GetUnsynced(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		setupData func(*DB) (expectedSyncIDs []string)
		wantCount int
	}{
		{
			name:      "no memories in database",
			project:   "test-project",
			setupData: func(d *DB) []string { return nil },
			wantCount: 0,
		},
		{
			name:    "all memories already synced",
			project: "test-project",
			setupData: func(d *DB) []string {
				_, err := d.EnsureManualSaveSession("test-project")
				require.NoError(t, err)
				mem := createTestMemory("test-project")
				id, err := d.SaveMemory(mem)
				require.NoError(t, err)
				// Get the auto-generated sync_id
				saved, err := d.GetMemory(id)
				require.NoError(t, err)
				// Mark as synced
				err = d.MarkSynced(saved.SyncID, time.Now())
				require.NoError(t, err)
				return nil // none should be unsynced
			},
			wantCount: 0,
		},
		{
			name:    "one unsynced memory",
			project: "test-project",
			setupData: func(d *DB) []string {
				_, err := d.EnsureManualSaveSession("test-project")
				require.NoError(t, err)
				mem := createTestMemory("test-project")
				id, err := d.SaveMemory(mem)
				require.NoError(t, err)
				saved, err := d.GetMemory(id)
				require.NoError(t, err)
				return []string{saved.SyncID}
			},
			wantCount: 1,
		},
		{
			name:    "multiple unsynced memories for project",
			project: "project-a",
			setupData: func(d *DB) []string {
				_, err := d.EnsureManualSaveSession("project-a")
				require.NoError(t, err)
				_, err = d.EnsureManualSaveSession("project-b")
				require.NoError(t, err)
				mem1 := createTestMemory("project-a")
				mem2 := createTestMemory("project-a")
				mem3 := createTestMemory("project-b")
				_, err = d.SaveMemory(mem1)
				require.NoError(t, err)
				_, err = d.SaveMemory(mem2)
				require.NoError(t, err)
				_, err = d.SaveMemory(mem3)
				require.NoError(t, err)
				return nil
			},
			wantCount: 2, // only project-a
		},
		{
			name:    "empty project filter returns all unsynced",
			project: "",
			setupData: func(d *DB) []string {
				_, err := d.EnsureManualSaveSession("project-a")
				require.NoError(t, err)
				_, err = d.EnsureManualSaveSession("project-b")
				require.NoError(t, err)
				mem1 := createTestMemory("project-a")
				mem2 := createTestMemory("project-b")
				_, err = d.SaveMemory(mem1)
				require.NoError(t, err)
				_, err = d.SaveMemory(mem2)
				require.NoError(t, err)
				return nil
			},
			wantCount: 2, // all projects
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			expectedSyncIDs := tt.setupData(db)

			got, err := db.GetUnsynced(tt.project)
			require.NoError(t, err)
			assert.Len(t, got, tt.wantCount)

			// Verify sync_ids if expected
			if len(expectedSyncIDs) > 0 && len(got) > 0 {
				assert.Equal(t, expectedSyncIDs[0], got[0].SyncID)
			}
		})
	}
}

// TestSyncDB_MarkSynced tests marking a memory as synced.
func TestSyncDB_MarkSynced(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	_, err := db.EnsureManualSaveSession("test-project")
	require.NoError(t, err)

	// Create unsynced memory
	mem := createTestMemory("test-project")
	id, err := db.SaveMemory(mem)
	require.NoError(t, err)

	// Get the auto-generated sync_id
	saved, err := db.GetMemory(id)
	require.NoError(t, err)

	// Verify it's unsynced
	unsynced, err := db.GetUnsynced("test-project")
	require.NoError(t, err)
	assert.Len(t, unsynced, 1)
	assert.Equal(t, saved.SyncID, unsynced[0].SyncID)

	// Mark as synced
	syncTime := time.Now().UTC()
	err = db.MarkSynced(saved.SyncID, syncTime)
	require.NoError(t, err)

	// Verify it's no longer unsynced
	unsynced, err = db.GetUnsynced("test-project")
	require.NoError(t, err)
	assert.Len(t, unsynced, 0)
}

// TestSyncDB_SaveFromRemote tests saving a memory received from the server.
func TestSyncDB_SaveFromRemote(t *testing.T) {
	tests := []struct {
		name      string
		setupData func(*DB)
		memory    func() *models.Memory
		wantErr   bool
	}{
		{
			name:      "save new memory from remote",
			setupData: func(d *DB) {},
			memory: func() *models.Memory {
				mem := createTestMemory("remote-project")
				mem.SyncID = "remote-sync-1" // Set explicit sync_id for remote memory
				return mem
			},
			wantErr: false,
		},
		{
			name: "duplicate sync_id is ignored (INSERT OR IGNORE)",
			setupData: func(d *DB) {
				mem := createTestMemory("remote-project")
				mem.SyncID = "duplicate-sync"
				err := d.SaveFromRemote(mem)
				require.NoError(t, err)
			},
			memory: func() *models.Memory {
				// Same sync_id, different content
				mem := createTestMemory("remote-project")
				mem.SyncID = "duplicate-sync"
				mem.Content = "This should be ignored"
				return mem
			},
			wantErr: false,
		},
		{
			name:      "memory with nil tags and files",
			setupData: func(d *DB) {},
			memory: func() *models.Memory {
				mem := createTestMemory("remote-project")
				mem.SyncID = "nil-fields"
				mem.Tags = nil
				mem.FilesAffected = nil
				return mem
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			// Ensure FK target exists for the manual-save sentinel id used by createTestMemory.
			_, err := db.EnsureManualSaveSession("remote-project")
			require.NoError(t, err)

			tt.setupData(db)

			mem := tt.memory()
			err = db.SaveFromRemote(mem)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSyncDB_PendingMutationsLifecycle(t *testing.T) {
	tests := []struct {
		name    string
		project string
		wantOps []MutationOp
	}{
		{
			name:    "filters unsynced mutations by project in deterministic order",
			project: "mut-project-a",
			wantOps: []MutationOp{MutationOpCreate, MutationOpDelete},
		},
		{
			name:    "empty project returns all unsynced mutations",
			project: "",
			wantOps: []MutationOp{MutationOpCreate, MutationOpDelete, MutationOpCreate},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			require.NoError(t, db.CreateSession("manual-save-mut-project-a", "mut-project-a", "", "test", "test"))
			require.NoError(t, db.CreateSession("manual-save-mut-project-b", "mut-project-b", "", "test", "test"))

			idA, err := db.SaveMemory(createTestMemory("mut-project-a"))
			require.NoError(t, err)
			require.NoError(t, db.DeleteMemory(idA, "tester", "cleanup"))
			_, err = db.SaveMemory(createTestMemory("mut-project-b"))
			require.NoError(t, err)

			got, err := db.GetPendingMutations(tt.project, 10)
			require.NoError(t, err)
			require.Len(t, got, len(tt.wantOps))
			for i, wantOp := range tt.wantOps {
				assert.Equal(t, wantOp, got[i].Op)
				assert.NotZero(t, got[i].Sequence)
				assert.NotEmpty(t, got[i].EventID)
				if got[i].Memory != nil {
					assert.Equal(t, got[i].EntitySyncID, got[i].Memory.SyncID)
					assert.Equal(t, got[i].Project, got[i].Memory.Project)
					assert.False(t, got[i].Memory.CreatedAt.IsZero())
					assert.False(t, got[i].Memory.UpdatedAt.IsZero())
				}
			}

			require.NoError(t, db.MarkMutationsSynced([]string{got[0].EventID}, time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)))
			after, err := db.GetPendingMutations(tt.project, 10)
			require.NoError(t, err)
			require.Len(t, after, len(tt.wantOps)-1)
			assert.NotEqual(t, got[0].EventID, after[0].EventID)
		})
	}
}

func TestSyncDB_MutationCursorHelpers(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, db *DB)
		consumer  string
		project   string
		want      MutationCursor
		assertion func(t *testing.T, db *DB)
	}{
		{
			name:     "missing cursor returns zero value",
			setup:    func(t *testing.T, db *DB) {},
			consumer: "hive-api",
			project:  "cursor-project-a",
			want:     MutationCursor{},
		},
		{
			name: "insert cursor persists sequence and event id",
			setup: func(t *testing.T, db *DB) {
				require.NoError(t, db.SetMutationCursor("hive-api", "cursor-project-a", MutationCursor{Sequence: 12, EventID: "evt-12"}, time.Date(2026, 5, 11, 17, 0, 0, 0, time.UTC)))
			},
			consumer: "hive-api",
			project:  "cursor-project-a",
			want:     MutationCursor{Sequence: 12, EventID: "evt-12"},
		},
		{
			name: "update cursor replaces sequence and event id",
			setup: func(t *testing.T, db *DB) {
				require.NoError(t, db.SetMutationCursor("hive-api", "cursor-project-a", MutationCursor{Sequence: 12, EventID: "evt-12"}, time.Date(2026, 5, 11, 17, 0, 0, 0, time.UTC)))
				require.NoError(t, db.SetMutationCursor("hive-api", "cursor-project-a", MutationCursor{Sequence: 13, EventID: "evt-13"}, time.Date(2026, 5, 11, 17, 5, 0, 0, time.UTC)))
			},
			consumer: "hive-api",
			project:  "cursor-project-a",
			want:     MutationCursor{Sequence: 13, EventID: "evt-13"},
		},
		{
			name: "cursor is isolated by project",
			setup: func(t *testing.T, db *DB) {
				require.NoError(t, db.SetMutationCursor("hive-api", "cursor-project-a", MutationCursor{Sequence: 21, EventID: "evt-a-21"}, time.Date(2026, 5, 11, 17, 10, 0, 0, time.UTC)))
				require.NoError(t, db.SetMutationCursor("hive-api", "cursor-project-b", MutationCursor{Sequence: 22, EventID: "evt-b-22"}, time.Date(2026, 5, 11, 17, 10, 0, 0, time.UTC)))
			},
			consumer: "hive-api",
			project:  "cursor-project-b",
			want:     MutationCursor{Sequence: 22, EventID: "evt-b-22"},
			assertion: func(t *testing.T, db *DB) {
				got, err := db.GetMutationCursor("hive-api", "cursor-project-a")
				require.NoError(t, err)
				assert.Equal(t, MutationCursor{Sequence: 21, EventID: "evt-a-21"}, got)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() { require.NoError(t, db.Close()) })

			tt.setup(t, db)

			got, err := db.GetMutationCursor(tt.consumer, tt.project)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			if tt.assertion != nil {
				tt.assertion(t, db)
			}
		})
	}
}

func TestSyncDB_MutationCursorHelpersReturnDBErrors(t *testing.T) {
	tests := []struct {
		name    string
		call    func(*DB) error
		wantErr string
	}{
		{
			name: "get cursor on closed database returns wrapped db error",
			call: func(db *DB) error {
				_, err := db.GetMutationCursor("hive-api", "cursor-project")
				return err
			},
			wantErr: "get mutation cursor",
		},
		{
			name: "set cursor on closed database returns wrapped db error",
			call: func(db *DB) error {
				return db.SetMutationCursor("hive-api", "cursor-project", MutationCursor{Sequence: 44, EventID: "evt-44"}, time.Date(2026, 5, 11, 18, 0, 0, 0, time.UTC))
			},
			wantErr: "set mutation cursor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			require.NoError(t, db.Close())

			err := tt.call(db)

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.ErrorContains(t, err, "database is closed")
		})
	}
}

func TestSyncDB_ApplyRemoteMutationIdempotent(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	event := MutationEnvelope{
		EventID:      "remote-event-1",
		EntityType:   "memory",
		EntitySyncID: "remote-memory-1",
		Project:      "remote-mut-project",
		Op:           MutationOpCreate,
		Sequence:     7,
		OccurredAt:   time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
		Memory: &MutationMemoryPayload{
			Title:     "Remote title",
			Content:   "Remote content",
			Category:  "remote",
			CreatedBy: "remote-user",
			SessionID: "manual-save-remote-mut-project",
		},
	}

	applied, err := db.ApplyRemoteMutation(event)
	require.NoError(t, err)
	assert.True(t, applied)

	applied, err = db.ApplyRemoteMutation(event)
	require.NoError(t, err)
	assert.False(t, applied, "duplicate event_id must be idempotent")

	var memoryCount, mutationCount int
	require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, event.EntitySyncID).Scan(&memoryCount))
	require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, event.EventID).Scan(&mutationCount))
	assert.Equal(t, 1, memoryCount)
	assert.Equal(t, 1, mutationCount)
}

func TestSyncDB_ApplyRemoteMutationNonCreateBranches(t *testing.T) {
	baseTime := time.Date(2026, 5, 11, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		seed      func(t *testing.T, db *DB) (memoryID int64, syncID string)
		event     func(syncID string) MutationEnvelope
		wantApply bool
		wantErr   string
		assertion func(t *testing.T, db *DB, memoryID int64, syncID string)
	}{
		{
			name: "delete tombstones active local memory and duplicate replay is idempotent",
			seed: seedRemoteApplyMemory("remote-delete-project"),
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-delete-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-delete-project",
					Op:           MutationOpDelete,
					OccurredAt:   baseTime,
					ActorID:      "remote-user",
					Tombstone: &MutationTombstonePayload{
						DeletedAt: baseTime.Add(-time.Minute),
						DeletedBy: "remote-user",
						Reason:    "remote cleanup",
					},
				}
			},
			wantApply: true,
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				_, err := db.GetMemory(memoryID)
				assert.Error(t, err)

				deleted, err := db.GetDeletedMemory(memoryID)
				require.NoError(t, err)
				assert.Equal(t, "remote-user", deleted.DeletedBy)
				assert.Equal(t, "remote cleanup", deleted.DeleteReason)
				assert.WithinDuration(t, baseTime.Add(-time.Minute), deleted.DeletedAt, time.Second)

				appliedAgain, err := db.ApplyRemoteMutation(MutationEnvelope{
					EventID:      "remote-delete-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-delete-project",
					Op:           MutationOpDelete,
					OccurredAt:   baseTime,
				})
				require.NoError(t, err)
				assert.False(t, appliedAgain)

				var mutationCount int
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, "remote-delete-event").Scan(&mutationCount))
				assert.Equal(t, 1, mutationCount)
			},
		},
		{
			name: "restore clears tombstone metadata and normal reads work",
			seed: func(t *testing.T, db *DB) (int64, string) {
				t.Helper()
				memoryID, syncID := seedRemoteApplyMemory("remote-restore-project")(t, db)
				require.NoError(t, db.DeleteMemory(memoryID, "local-user", "local cleanup"))
				return memoryID, syncID
			},
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-restore-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-restore-project",
					Op:           MutationOpRestore,
					OccurredAt:   baseTime.Add(time.Minute),
					ActorID:      "remote-user",
				}
			},
			wantApply: true,
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				active, err := db.GetMemory(memoryID)
				require.NoError(t, err)
				assert.Equal(t, syncID, active.SyncID)

				_, err = db.GetDeletedMemory(memoryID)
				assert.Error(t, err)

				var deletedAt, deletedBy, reason sql.NullString
				require.NoError(t, db.sqlDB.QueryRow(`SELECT deleted_at, deleted_by, delete_reason FROM memories WHERE sync_id = ?`, syncID).Scan(&deletedAt, &deletedBy, &reason))
				assert.False(t, deletedAt.Valid)
				assert.False(t, deletedBy.Valid)
				assert.False(t, reason.Valid)
			},
		},
		{
			name: "update active memory applies payload and records mutation",
			seed: seedRemoteApplyMemory("remote-update-project"),
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-update-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-update-project",
					Op:           MutationOpUpdate,
					OccurredAt:   baseTime.Add(90 * time.Second),
					Memory: &MutationMemoryPayload{
						Title:         "Remote updated title",
						Content:       "Remote updated content",
						Category:      "remote-update",
						Tags:          []string{"remote", "update"},
						FilesAffected: []string{"sync.go"},
						CreatedBy:     "remote-user",
						Confidence:    "medium",
						ImpactScore:   7,
						SessionID:     "manual-save-remote-update-project",
					},
				}
			},
			wantApply: true,
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				active, err := db.GetMemory(memoryID)
				require.NoError(t, err)
				assert.Equal(t, "Remote updated title", active.Title)
				assert.Equal(t, "Remote updated content", active.Content)
				assert.Equal(t, "remote-update", active.Category)
				assert.Equal(t, []string{"remote", "update"}, active.Tags)

				var mutationCount int
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ? AND op = ?`, "remote-update-event", string(MutationOpUpdate)).Scan(&mutationCount))
				assert.Equal(t, 1, mutationCount)
			},
		},
		{
			name: "update on tombstone is rejected and does not alter tombstone row",
			seed: func(t *testing.T, db *DB) (int64, string) {
				t.Helper()
				memoryID, syncID := seedRemoteApplyMemory("remote-update-tombstone-project")(t, db)
				require.NoError(t, db.DeleteMemory(memoryID, "local-user", "keep tombstone"))
				return memoryID, syncID
			},
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-update-tombstone-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-update-tombstone-project",
					Op:           MutationOpUpdate,
					OccurredAt:   baseTime.Add(2 * time.Minute),
					Memory: &MutationMemoryPayload{
						Title:     "Remote update should not apply",
						Content:   "remote content",
						Category:  "remote",
						CreatedBy: "remote-user",
						SessionID: "manual-save-remote-update-tombstone-project",
					},
				}
			},
			wantErr: "explicit restore required before update",
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				deleted, err := db.GetDeletedMemory(memoryID)
				require.NoError(t, err)
				assert.Equal(t, "Test Memory", deleted.Memory.Title)
				assert.Equal(t, "keep tombstone", deleted.DeleteReason)

				var mutationCount int
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, "remote-update-tombstone-event").Scan(&mutationCount))
				assert.Equal(t, 0, mutationCount)
			},
		},
		{
			name: "update missing row returns diagnosable error and records no mutation",
			seed: func(t *testing.T, db *DB) (int64, string) {
				t.Helper()
				return 0, "missing-update-sync-id"
			},
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-update-missing-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-update-missing-project",
					Op:           MutationOpUpdate,
					OccurredAt:   baseTime,
					Memory: &MutationMemoryPayload{
						Title:     "Remote update missing",
						Content:   "remote content",
						Category:  "remote",
						CreatedBy: "remote-user",
						SessionID: "manual-save-remote-update-missing-project",
					},
				}
			},
			wantErr: "memory not found",
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				var memoryCount, mutationCount int
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, syncID).Scan(&memoryCount))
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, "remote-update-missing-event").Scan(&mutationCount))
				assert.Equal(t, 0, memoryCount)
				assert.Equal(t, 0, mutationCount)
			},
		},
		{
			name: "delete missing row returns diagnosable error and records no mutation",
			seed: func(t *testing.T, db *DB) (int64, string) {
				t.Helper()
				return 0, "missing-delete-sync-id"
			},
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-delete-missing-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-delete-missing-project",
					Op:           MutationOpDelete,
					OccurredAt:   baseTime,
				}
			},
			wantErr: "memory not found",
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				var mutationCount int
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, "remote-delete-missing-event").Scan(&mutationCount))
				assert.Equal(t, 0, mutationCount)
			},
		},
		{
			name: "restore missing row returns diagnosable error and records no mutation",
			seed: func(t *testing.T, db *DB) (int64, string) {
				t.Helper()
				return 0, "missing-restore-sync-id"
			},
			event: func(syncID string) MutationEnvelope {
				return MutationEnvelope{
					EventID:      "remote-restore-missing-event",
					EntityType:   "memory",
					EntitySyncID: syncID,
					Project:      "remote-restore-missing-project",
					Op:           MutationOpRestore,
					OccurredAt:   baseTime,
				}
			},
			wantErr: "memory not deleted",
			assertion: func(t *testing.T, db *DB, memoryID int64, syncID string) {
				var mutationCount int
				require.NoError(t, db.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, "remote-restore-missing-event").Scan(&mutationCount))
				assert.Equal(t, 0, mutationCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() { require.NoError(t, db.Close()) })

			memoryID, syncID := tt.seed(t, db)
			applied, err := db.ApplyRemoteMutation(tt.event(syncID))

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.False(t, applied)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantApply, applied)
			}

			tt.assertion(t, db, memoryID, syncID)
		})
	}
}

func TestSyncDB_ApplyRemoteMutationValidationErrors(t *testing.T) {
	validMemory := &MutationMemoryPayload{
		Title:     "Remote title",
		Content:   "Remote content",
		Category:  "remote",
		CreatedBy: "remote-user",
		SessionID: "manual-save-validation-project",
	}

	tests := []struct {
		name    string
		event   MutationEnvelope
		wantErr string
	}{
		{
			name: "missing event id",
			event: MutationEnvelope{
				EntityType:   "memory",
				EntitySyncID: "validation-sync-id",
				Project:      "validation-project",
				Op:           MutationOpCreate,
				Memory:       validMemory,
			},
			wantErr: "event_id is required",
		},
		{
			name: "unsupported entity type",
			event: MutationEnvelope{
				EventID:      "validation-unsupported-entity",
				EntityType:   "note",
				EntitySyncID: "validation-sync-id",
				Project:      "validation-project",
				Op:           MutationOpCreate,
				Memory:       validMemory,
			},
			wantErr: "unsupported mutation entity_type",
		},
		{
			name: "missing entity sync id",
			event: MutationEnvelope{
				EventID:    "validation-missing-sync-id",
				EntityType: "memory",
				Project:    "validation-project",
				Op:         MutationOpCreate,
				Memory:     validMemory,
			},
			wantErr: "entity_sync_id is required",
		},
		{
			name: "missing project",
			event: MutationEnvelope{
				EventID:      "validation-missing-project",
				EntityType:   "memory",
				EntitySyncID: "validation-sync-id",
				Op:           MutationOpCreate,
				Memory:       validMemory,
			},
			wantErr: "project is required",
		},
		{
			name: "missing memory payload",
			event: MutationEnvelope{
				EventID:      "validation-missing-memory",
				EntityType:   "memory",
				EntitySyncID: "validation-sync-id",
				Project:      "validation-project",
				Op:           MutationOpUpdate,
			},
			wantErr: "memory payload required",
		},
		{
			name: "unsupported operation",
			event: MutationEnvelope{
				EventID:      "validation-unsupported-op",
				EntityType:   "memory",
				EntitySyncID: "validation-sync-id",
				Project:      "validation-project",
				Op:           MutationOp("archive"),
			},
			wantErr: "unsupported mutation op",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			t.Cleanup(func() { require.NoError(t, db.Close()) })

			applied, err := db.ApplyRemoteMutation(tt.event)

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.False(t, applied)
		})
	}
}

func seedRemoteApplyMemory(project string) func(t *testing.T, db *DB) (int64, string) {
	return func(t *testing.T, db *DB) (int64, string) {
		t.Helper()
		require.NoError(t, db.CreateSession("manual-save-"+project, project, "", "test", "test"))
		memoryID, err := db.SaveMemory(createTestMemory(project))
		require.NoError(t, err)

		var syncID string
		require.NoError(t, db.sqlDB.QueryRow(`SELECT sync_id FROM memories WHERE id = ?`, memoryID).Scan(&syncID))
		return memoryID, syncID
	}
}

// createTestMemory is a helper to create a test Memory struct.
// Note: SyncID is left empty and will be auto-generated by SaveMemory.
// SessionID is populated with the manual-save sentinel so direct SaveMemory
// calls satisfy memories.session_id NOT NULL post-Slice 4 (CRIT-5).
// For SaveFromRemote tests, set SyncID explicitly.
func createTestMemory(project string) *models.Memory {
	return &models.Memory{
		Project:       project,
		Category:      "test",
		Title:         "Test Memory",
		Content:       "Test content for " + project,
		Tags:          []string{"test"},
		FilesAffected: []string{},
		CreatedBy:     "test-user",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		Confidence:    "high",
		ImpactScore:   5,
		SessionID:     "manual-save-" + project,
	}
}

// ─── T-06c: SaveFromRemote non-stripping regression ──────────────────────────

// TestSyncDB_SaveFromRemote_ContentWithPrivateTag_StoredVerbatim ensures that
// SaveFromRemote never strips <private> tags. Stripping happens ONLY at the
// handler boundary (mem_save, mem_save_prompt, mem_session_summary, /prompts).
// Remote-pulled rows must be persisted exactly as received.
func TestSyncDB_SaveFromRemote_ContentWithPrivateTag_StoredVerbatim(t *testing.T) {
	rawContent := "secret: <private>tok123</private> end"
	rawTitle := "title <private>label</private> tail"

	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	_, err := db.EnsureManualSaveSession("remote-project")
	require.NoError(t, err)

	mem := createTestMemory("remote-project")
	mem.SyncID = "regression-no-strip"
	mem.Title = rawTitle
	mem.Content = rawContent

	require.NoError(t, db.SaveFromRemote(mem))

	// Read back the row directly by sync_id. SaveFromRemote marks rows synced
	// immediately, so GetUnsynced won't surface them — query the DB whitebox.
	var gotTitle, gotContent string
	err = db.sqlDB.QueryRow(
		`SELECT title, content FROM memories WHERE sync_id = ?`,
		mem.SyncID,
	).Scan(&gotTitle, &gotContent)
	require.NoError(t, err)

	if gotContent != rawContent {
		t.Errorf("content stored = %q, want verbatim %q", gotContent, rawContent)
	}
	if gotTitle != rawTitle {
		t.Errorf("title stored = %q, want verbatim %q", gotTitle, rawTitle)
	}
}

type wrappedError string

func (e wrappedError) Error() string {
	return string(e)
}

// CRIT-3 — GetUnsynced/SaveFromRemote must include session_id end-to-end.
// Without these, daemon→server pushes drop session_id and pulls never restore it.

func TestGetUnsynced_IncludesSessionID(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Seed a session and a memory pointing at it via mem_save resolver path.
	sessID, err := db.EnsureManualSaveSession("crit3-proj")
	require.NoError(t, err)

	mem := createTestMemory("crit3-proj")
	mem.SessionID = sessID
	_, err = db.SaveMemory(mem)
	require.NoError(t, err)

	got, err := db.GetUnsynced("crit3-proj")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, sessID, got[0].SessionID,
		"GetUnsynced must propagate session_id so the wire push carries attribution")
}

func TestSaveFromRemote_PersistsSessionID(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Pre-create the session row locally so the FK in memories(session_id) is valid.
	require.NoError(t, db.CreateSession("sess-remote-1", "crit3-pull", "", "dev", "claude"))

	mem := createTestMemory("crit3-pull")
	mem.SyncID = "remote-with-session"
	mem.SessionID = "sess-remote-1"

	require.NoError(t, db.SaveFromRemote(mem))

	var stored sql.NullString
	err := db.sqlDB.QueryRow(
		`SELECT session_id FROM memories WHERE sync_id = ?`, mem.SyncID,
	).Scan(&stored)
	require.NoError(t, err)
	require.True(t, stored.Valid, "session_id must be persisted, not NULL")
	assert.Equal(t, "sess-remote-1", stored.String)
}

// R2-CRIT-3 — SaveFromRemote with empty SessionID must lazy-create the manual-save
// session and persist with that id, NOT fail FK or silently drop the row via
// INSERT OR IGNORE. The whole point of NOT NULL was to make attribution mandatory.
func TestSaveFromRemote_EmptySessionID_LazyCreatesManualSave(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Caller did NOT pre-create any session. SaveFromRemote with empty SessionID
	// must resolve to manual-save-<project> server-defensively and insert.
	mem := createTestMemory("r2c3-fallback")
	mem.SyncID = "remote-without-session"
	mem.SessionID = ""

	require.NoError(t, db.SaveFromRemote(mem),
		"SaveFromRemote must not error when SessionID is empty — must lazy-create manual-save")

	var stored sql.NullString
	err := db.sqlDB.QueryRow(
		`SELECT session_id FROM memories WHERE sync_id = ?`, mem.SyncID,
	).Scan(&stored)
	require.NoError(t, err, "memory must be persisted, not silently dropped by INSERT OR IGNORE")
	require.True(t, stored.Valid)
	assert.Equal(t, "manual-save-r2c3-fallback", stored.String,
		"empty SessionID must resolve to the project's manual-save session")

	// And that session must actually exist in the sessions table.
	var exists int
	err = db.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE id = ?`, "manual-save-r2c3-fallback",
	).Scan(&exists)
	require.NoError(t, err)
	assert.Equal(t, 1, exists)
}
