package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
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
				mem1 := createTestMemory("project-a")
				mem2 := createTestMemory("project-a")
				mem3 := createTestMemory("project-b")
				_, err := d.SaveMemory(mem1)
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
				mem1 := createTestMemory("project-a")
				mem2 := createTestMemory("project-b")
				_, err := d.SaveMemory(mem1)
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

			tt.setupData(db)

			mem := tt.memory()
			err := db.SaveFromRemote(mem)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// createTestMemory is a helper to create a test Memory struct.
// Note: SyncID is left empty and will be auto-generated by SaveMemory.
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
	}
}
