package db

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper para abrir DB en test y limpiar al final
func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(':memory:') failed: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// ensureManualSaveSessions guarantees a manual-save sentinel session exists for
// each project so direct SaveMemory calls satisfy the FK introduced in CRIT-5.
// Idempotent — INSERT OR IGNORE.
func ensureManualSaveSessions(t *testing.T, d *DB, projects ...string) {
	t.Helper()
	for _, p := range projects {
		if _, err := d.EnsureManualSaveSession(p); err != nil {
			t.Fatalf("ensureManualSaveSessions(%q): %v", p, err)
		}
	}
}

// saveTestMemory wraps SaveMemory after ensuring the manual-save session for
// mem.Project exists. Tests that bypass the MCP handler (which normally does
// this via EnsureManualSaveSession) use this to satisfy the FK.
func saveTestMemory(t *testing.T, d *DB, mem *models.Memory) (int64, error) {
	t.Helper()
	if mem.SessionID == "" {
		mem.SessionID = "manual-save-" + mem.Project
	}
	if _, err := d.EnsureManualSaveSession(mem.Project); err != nil {
		t.Fatalf("ensure manual-save session for %q: %v", mem.Project, err)
	}
	return d.SaveMemory(mem)
}

// helper para construir una Memory mínima válida.
// SessionID is pre-set to the manual-save sentinel id so direct SaveMemory
// calls satisfy the FK NOT NULL contract enforced post-Slice 4 (CRIT-5). The
// MCP handler resolves this via EnsureManualSaveSession; tests bypassing the
// handler must provide it explicitly.
func newMemory(project, title, content string) *models.Memory {
	return &models.Memory{
		Project:   project,
		Title:     title,
		Content:   content,
		SessionID: "manual-save-" + project,
	}
}

// ─── 3.1 SaveMemory ────────────────────────────────────────────────────────

func TestSaveMemory_CreatesNewRow(t *testing.T) {
	d := openTestDB(t)

	id, err := saveTestMemory(t, d, newMemory("proj", "Title", "Content"))
	if err != nil {
		t.Fatalf("SaveMemory() failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("SaveMemory() id = %d, want > 0", id)
	}
}

func TestSaveMemory_PopulatesSyncIDAndCreatedBy(t *testing.T) {
	d := openTestDB(t)

	id, err := saveTestMemory(t, d, newMemory("proj", "Title", "Content"))
	if err != nil {
		t.Fatal(err)
	}

	var syncID, createdBy string
	err = d.sqlDB.QueryRow(
		"SELECT sync_id, created_by FROM memories WHERE id=?", id,
	).Scan(&syncID, &createdBy)
	if err != nil {
		t.Fatalf("query after save: %v", err)
	}
	if syncID == "" {
		t.Error("sync_id should not be empty")
	}
	if createdBy == "" {
		t.Error("created_by should not be empty")
	}
}

func TestSaveMemory_TopicKeyUpsert(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	key := "arch/auth"
	mem := &models.Memory{
		Project:   "proj",
		Title:     "Original",
		Content:   "Original content",
		TopicKey:  &key,
		SessionID: "manual-save-proj",
	}

	id1, err := d.SaveMemory(mem)
	if err != nil {
		t.Fatalf("first SaveMemory() failed: %v", err)
	}

	mem.Title = "Updated"
	mem.Content = "Updated content"
	id2, err := d.SaveMemory(mem)
	if err != nil {
		t.Fatalf("second SaveMemory() failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("upsert should return same id: id1=%d id2=%d", id1, id2)
	}

	// Verify only 1 row exists
	var count int
	if err := d.sqlDB.QueryRow("SELECT COUNT(*) FROM memories WHERE project='proj'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after upsert, got %d", count)
	}

	// Verify content was updated
	var title string
	if err := d.sqlDB.QueryRow("SELECT title FROM memories WHERE id=?", id1).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Updated" {
		t.Errorf("title after upsert = %q, want 'Updated'", title)
	}
}

func TestSaveMemory_NullTopicKeyAlwaysInserts(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	mem := newMemory("proj", "Title", "Content") // no topic_key → nil

	id1, err := d.SaveMemory(mem)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := d.SaveMemory(mem)
	if err != nil {
		t.Fatal(err)
	}

	if id1 == id2 {
		t.Error("NULL topic_key should always INSERT new row, got same ID")
	}

	var count int
	if err := d.sqlDB.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows for NULL topic_key, got %d", count)
	}
}

func TestSaveMemory_ValidationError(t *testing.T) {
	d := openTestDB(t)

	_, err := d.SaveMemory(&models.Memory{}) // missing project, title, content
	if err == nil {
		t.Error("SaveMemory() should return error for invalid memory")
	}
}

func TestSaveMemory_StoresTags(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	mem := newMemory("proj", "Title", "Content")
	mem.Tags = []string{"go", "sqlite", "mcp"}
	mem.FilesAffected = []string{"internal/db/db.go"}

	id, err := d.SaveMemory(mem)
	if err != nil {
		t.Fatal(err)
	}

	got, err := d.GetMemory(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 3 || got.Tags[0] != "go" {
		t.Errorf("Tags: got %v, want [go sqlite mcp]", got.Tags)
	}
	if len(got.FilesAffected) != 1 || got.FilesAffected[0] != "internal/db/db.go" {
		t.Errorf("FilesAffected: got %v", got.FilesAffected)
	}
}

// ─── 3.2 GetMemory ─────────────────────────────────────────────────────────

func TestGetMemory_ReturnsAllFields(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	key := "sdd/spec"
	mem := &models.Memory{
		Project:       "proj",
		Title:         "My Title",
		Content:       "My Content",
		TopicKey:      &key,
		Category:      "architecture",
		Tags:          []string{"go"},
		FilesAffected: []string{"main.go"},
		Confidence:    "high",
		ImpactScore:   90,
		SessionID:     "manual-save-proj",
	}

	id, err := d.SaveMemory(mem)
	if err != nil {
		t.Fatal(err)
	}

	got, err := d.GetMemory(id)
	if err != nil {
		t.Fatalf("GetMemory(%d) failed: %v", id, err)
	}

	if got.ID != id {
		t.Errorf("ID: got %d, want %d", got.ID, id)
	}
	if got.Title != "My Title" {
		t.Errorf("Title: got %q, want 'My Title'", got.Title)
	}
	if got.Content != "My Content" {
		t.Errorf("Content: got %q", got.Content)
	}
	if got.TopicKey == nil || *got.TopicKey != "sdd/spec" {
		t.Errorf("TopicKey: got %v, want sdd/spec", got.TopicKey)
	}
	if got.Category != "architecture" {
		t.Errorf("Category: got %q", got.Category)
	}
	if got.Confidence != "high" {
		t.Errorf("Confidence: got %q", got.Confidence)
	}
	if got.ImpactScore != 90 {
		t.Errorf("ImpactScore: got %d, want 90", got.ImpactScore)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if got.SyncID == "" {
		t.Error("SyncID should not be empty")
	}
}

func TestGetMemory_NotFound_ReturnsError(t *testing.T) {
	d := openTestDB(t)

	_, err := d.GetMemory(999)
	if err == nil {
		t.Error("GetMemory(999) should return error for non-existent id")
	}
}

// ─── 3.2 ListMemories ──────────────────────────────────────────────────────

func TestListMemories_ProjectFilter(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 3; i++ {
		if _, err := saveTestMemory(t, d, newMemory("foo", "foo mem", "c")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := saveTestMemory(t, d, newMemory("bar", "bar mem", "c")); err != nil {
		t.Fatal(err)
	}

	results, err := d.ListMemories("foo", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results for project 'foo', got %d", len(results))
	}
	for _, r := range results {
		if r.Project != "foo" {
			t.Errorf("expected project 'foo', got %q", r.Project)
		}
	}
}

func TestListMemories_RespectsLimit(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 5; i++ {
		if _, err := saveTestMemory(t, d, newMemory("proj", "mem", "c")); err != nil {
			t.Fatal(err)
		}
	}

	results, err := d.ListMemories("proj", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

func TestListMemories_OrderByCreatedAtDesc(t *testing.T) {
	d := openTestDB(t)

	titles := []string{"first", "second", "third"}
	for _, title := range titles {
		if _, err := saveTestMemory(t, d, newMemory("proj", title, "c")); err != nil {
			t.Fatal(err)
		}
	}

	results, err := d.ListMemories("proj", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Most recent first
	if results[0].Title != "third" {
		t.Errorf("first result should be 'third' (most recent), got %q", results[0].Title)
	}
}

func TestListMemories_EmptyProject_ReturnsEmpty(t *testing.T) {
	d := openTestDB(t)

	results, err := d.ListMemories("nonexistent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown project, got %d", len(results))
	}
}

// R2-WARN-1 — GetMemory and ListMemories must include session_id in their SELECT
// projections AND scan it into models.Memory. Otherwise local read paths (mcp tools
// `mem_get_observation`, `mem_context`) silently strip attribution.

func TestGetMemory_ReturnsSessionID(t *testing.T) {
	d := openTestDB(t)

	mem := newMemory("r2w1-proj", "Title", "Content")
	mem.SessionID = "manual-save-r2w1-proj"
	id, err := saveTestMemory(t, d, mem)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := d.GetMemory(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SessionID != "manual-save-r2w1-proj" {
		t.Errorf("GetMemory.SessionID = %q, want %q",
			got.SessionID, "manual-save-r2w1-proj")
	}
}

func TestListMemories_ReturnsSessionID(t *testing.T) {
	d := openTestDB(t)

	mem := newMemory("r2w1-list", "Title", "Content")
	mem.SessionID = "manual-save-r2w1-list"
	if _, err := saveTestMemory(t, d, mem); err != nil {
		t.Fatalf("save: %v", err)
	}

	results, err := d.ListMemories("r2w1-list", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].SessionID != "manual-save-r2w1-list" {
		t.Errorf("ListMemories[0].SessionID = %q, want %q",
			results[0].SessionID, "manual-save-r2w1-list")
	}
}

func TestMemorySoftDeleteBehavior(t *testing.T) {
	tests := []struct {
		name   string
		assert func(t *testing.T, d *DB, id int64)
	}{
		{
			name: "delete hides from default reads and exposes tombstone metadata",
			assert: func(t *testing.T, d *DB, id int64) {
				require.NoError(t, d.DeleteMemory(id, "tester", "duplicate"))

				_, err := d.GetMemory(id)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "memory not found")

				listed, err := d.ListMemories("soft-delete", 10)
				require.NoError(t, err)
				assert.Empty(t, listed)

				deleted, err := d.GetDeletedMemory(id)
				require.NoError(t, err)
				assert.Equal(t, "Title", deleted.Memory.Title)
				assert.Equal(t, "tester", deleted.DeletedBy)
				assert.Equal(t, "duplicate", deleted.DeleteReason)
				assert.False(t, deleted.DeletedAt.IsZero())
			},
		},
		{
			name: "normal topic update does not restore tombstone",
			assert: func(t *testing.T, d *DB, id int64) {
				key := "soft/delete/topic"
				require.NoError(t, d.DeleteMemory(id, "tester", "obsolete"))

				_, err := d.SaveMemory(&models.Memory{
					Project:   "soft-delete",
					TopicKey:  &key,
					Title:     "Updated",
					Content:   "Updated content",
					SessionID: "manual-save-soft-delete",
				})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "explicit restore")

				deleted, err := d.GetDeletedMemory(id)
				require.NoError(t, err)
				assert.Equal(t, "Title", deleted.Memory.Title)
				assert.False(t, deleted.DeletedAt.IsZero())
			},
		},
		{
			name: "restore reactivates and clears current tombstone fields",
			assert: func(t *testing.T, d *DB, id int64) {
				require.NoError(t, d.DeleteMemory(id, "tester", "obsolete"))
				require.NoError(t, d.RestoreMemory(id, "tester"))

				got, err := d.GetMemory(id)
				require.NoError(t, err)
				assert.Equal(t, "Title", got.Title)

				var deletedAt, deletedBy, deleteReason sql.NullString
				var restoredAt string
				err = d.sqlDB.QueryRow(
					`SELECT deleted_at, deleted_by, delete_reason, restored_at FROM memories WHERE id = ?`, id,
				).Scan(&deletedAt, &deletedBy, &deleteReason, &restoredAt)
				require.NoError(t, err)
				assert.False(t, deletedAt.Valid)
				assert.False(t, deletedBy.Valid)
				assert.False(t, deleteReason.Valid)
				assert.NotEmpty(t, restoredAt)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			key := "soft/delete/topic"
			id, err := saveTestMemory(t, d, &models.Memory{
				Project:   "soft-delete",
				TopicKey:  &key,
				Title:     "Title",
				Content:   "Content",
				SessionID: "manual-save-soft-delete",
			})
			require.NoError(t, err)

			tt.assert(t, d, id)
		})
	}
}

func TestMemoryMutationsJournaledTransactionally(t *testing.T) {
	d := openTestDB(t)
	key := "journal/topic"
	id, err := saveTestMemory(t, d, &models.Memory{
		Project:   "journal-project",
		TopicKey:  &key,
		Title:     "Created",
		Content:   "Content",
		SessionID: "manual-save-journal-project",
	})
	require.NoError(t, err)

	_, err = d.SaveMemory(&models.Memory{
		Project:   "journal-project",
		TopicKey:  &key,
		Title:     "Updated",
		Content:   "Updated content",
		SessionID: "manual-save-journal-project",
	})
	require.NoError(t, err)
	require.NoError(t, d.DeleteMemory(id, "tester", "done"))
	require.NoError(t, d.RestoreMemory(id, "tester"))

	rows, err := d.sqlDB.Query(`SELECT op, entity_sync_id, project, event_id FROM memory_mutations ORDER BY sequence ASC`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var ops []string
	var syncID string
	for rows.Next() {
		var op, entitySyncID, project, eventID string
		require.NoError(t, rows.Scan(&op, &entitySyncID, &project, &eventID))
		ops = append(ops, op)
		assert.Equal(t, "journal-project", project)
		assert.NotEmpty(t, entitySyncID)
		assert.NotEmpty(t, eventID)
		if syncID == "" {
			syncID = entitySyncID
		}
		assert.Equal(t, syncID, entitySyncID)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"create", "update", "delete", "restore"}, ops)

	err = d.DeleteMemory(999999, "tester", "missing")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "memory not found"))

	var deleteCount int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE op = 'delete'`).Scan(&deleteCount))
	assert.Equal(t, 1, deleteCount, "failed delete must not leave a committed journal row")
}

func TestMemoryRestoreRequiresDeletedRow(t *testing.T) {
	d := openTestDB(t)
	id, err := saveTestMemory(t, d, newMemory("restore-project", "Active", "Content"))
	require.NoError(t, err)

	err = d.RestoreMemory(id, "tester")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not deleted")

	var count int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE op = 'restore'`).Scan(&count))
	assert.Equal(t, 0, count)
}

func assertRecentTime(t *testing.T, got time.Time) {
	t.Helper()
	assert.WithinDuration(t, time.Now().UTC(), got, 5*time.Second)
}
