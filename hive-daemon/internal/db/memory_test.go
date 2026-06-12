package db

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
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

// TestSaveMemory_TopicKeyAlwaysInserts asserts that saving twice with the same
// topic_key creates two distinct rows (Issue #119: topic_key is a grouping key,
// not an upsert/identity key).
func TestSaveMemory_TopicKeyAlwaysInserts(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	key := "arch/auth"

	id1, err := d.SaveMemory(&models.Memory{
		Project:   "proj",
		Title:     "First save",
		Content:   "First content",
		TopicKey:  &key,
		SessionID: "manual-save-proj",
	})
	require.NoError(t, err)

	id2, err := d.SaveMemory(&models.Memory{
		Project:   "proj",
		Title:     "Second save",
		Content:   "Second content",
		TopicKey:  &key,
		SessionID: "manual-save-proj",
	})
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2, "two saves with same topic_key must produce two distinct row ids")

	var count int
	require.NoError(t, d.sqlDB.QueryRow(
		"SELECT COUNT(*) FROM memories WHERE project=? AND topic_key=?", "proj", key,
	).Scan(&count))
	assert.Equal(t, 2, count, "expected 2 rows after two saves with same topic_key")

	// Both mutations must be 'create'.
	var ops []string
	rows, err := d.sqlDB.Query(`SELECT op FROM memory_mutations ORDER BY sequence ASC`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var op string
		require.NoError(t, rows.Scan(&op))
		ops = append(ops, op)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"create", "create"}, ops)
}

// TestSaveMemory_AlwaysCreatesNewSyncID asserts that each SaveMemory call
// produces a fresh sync_id, even when using the same topic_key.
func TestSaveMemory_AlwaysCreatesNewSyncID(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj-sync")

	key := "arch/sync-test"

	id1, err := saveTestMemory(t, d, &models.Memory{
		Project:   "proj-sync",
		Title:     "First",
		Content:   "Content",
		TopicKey:  &key,
		SessionID: "manual-save-proj-sync",
	})
	require.NoError(t, err)

	id2, err := d.SaveMemory(&models.Memory{
		Project:   "proj-sync",
		Title:     "Second",
		Content:   "Content",
		TopicKey:  &key,
		SessionID: "manual-save-proj-sync",
	})
	require.NoError(t, err)

	var syncID1, syncID2 string
	require.NoError(t, d.sqlDB.QueryRow("SELECT sync_id FROM memories WHERE id=?", id1).Scan(&syncID1))
	require.NoError(t, d.sqlDB.QueryRow("SELECT sync_id FROM memories WHERE id=?", id2).Scan(&syncID2))

	assert.NotEmpty(t, syncID1)
	assert.NotEmpty(t, syncID2)
	assert.NotEqual(t, syncID1, syncID2, "second save with same topic_key must generate a fresh sync_id")

	// Both mutations must be 'create'.
	rows, err := d.sqlDB.Query(`SELECT op FROM memory_mutations WHERE project='proj-sync' ORDER BY sequence ASC`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var ops []string
	for rows.Next() {
		var op string
		require.NoError(t, rows.Scan(&op))
		ops = append(ops, op)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"create", "create"}, ops)
}

// TestSaveMemory_DeletedTopicKeyDoesNotBlock asserts that a soft-deleted row
// does not prevent a new SaveMemory with the same topic_key. The new save
// creates a fresh active row; the deleted tombstone is preserved untouched.
func TestSaveMemory_DeletedTopicKeyDoesNotBlock(t *testing.T) {
	d := openTestDB(t)

	key := "deleted/topic"
	id1, err := saveTestMemory(t, d, &models.Memory{
		Project:   "del-test",
		Title:     "Original",
		Content:   "Content",
		TopicKey:  &key,
		SessionID: "manual-save-del-test",
	})
	require.NoError(t, err)

	require.NoError(t, d.DeleteMemory(id1, "tester", "obsolete"))

	// Saving again with the same topic_key must succeed.
	id2, err := d.SaveMemory(&models.Memory{
		Project:   "del-test",
		Title:     "New version",
		Content:   "New content",
		TopicKey:  &key,
		SessionID: "manual-save-del-test",
	})
	require.NoError(t, err, "SaveMemory must not return an error when prior row is deleted")
	assert.NotEqual(t, id1, id2)

	// New row must be active (deleted_at IS NULL).
	var deletedAt sql.NullString
	require.NoError(t, d.sqlDB.QueryRow("SELECT deleted_at FROM memories WHERE id=?", id2).Scan(&deletedAt))
	assert.False(t, deletedAt.Valid, "new row must not be tombstoned")

	// Original deleted row must still be tombstoned.
	var origDeletedAt sql.NullString
	require.NoError(t, d.sqlDB.QueryRow("SELECT deleted_at FROM memories WHERE id=?", id1).Scan(&origDeletedAt))
	assert.True(t, origDeletedAt.Valid, "original tombstoned row must remain deleted")
}

// TestApplyRemoteMutation_UnaffectedByTopicKeyChange verifies that the sync
// path (ApplyRemoteMutation, keyed by sync_id) is not broken when two rows
// share the same topic_key.
func TestApplyRemoteMutation_UnaffectedByTopicKeyChange(t *testing.T) {
	d := openTestDB(t)

	key := "sdd/shared-topic"

	id1, err := saveTestMemory(t, d, &models.Memory{
		Project:   "sync-proj",
		Title:     "Row 1",
		Content:   "Content 1",
		TopicKey:  &key,
		SessionID: "manual-save-sync-proj",
	})
	require.NoError(t, err)

	id2, err := d.SaveMemory(&models.Memory{
		Project:   "sync-proj",
		Title:     "Row 2",
		Content:   "Content 2",
		TopicKey:  &key,
		SessionID: "manual-save-sync-proj",
	})
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2)

	// Get sync_id of first row.
	var syncID1 string
	require.NoError(t, d.sqlDB.QueryRow("SELECT sync_id FROM memories WHERE id=?", id1).Scan(&syncID1))

	// ApplyRemoteMutation on the first row's sync_id should succeed.
	updatedTitle := "Row 1 updated by remote"
	applied, err := d.ApplyRemoteMutation(MutationEnvelope{
		EventID:      "evt-remote-test-001",
		EntityType:   "memory",
		Op:           MutationOpUpdate,
		EntitySyncID: syncID1,
		Project:      "sync-proj",
		OccurredAt:   time.Now().UTC(),
		Memory: &MutationMemoryPayload{
			SyncID:    syncID1,
			Title:     updatedTitle,
			Content:   "Content 1 updated",
			Category:  "architecture",
			UpdatedAt: time.Now().UTC(),
			SessionID: "manual-save-sync-proj",
		},
	})
	require.NoError(t, err)
	assert.True(t, applied, "ApplyRemoteMutation should return applied=true")

	// First row should be updated.
	got1, err := d.GetMemory(id1)
	require.NoError(t, err)
	assert.Equal(t, updatedTitle, got1.Title)

	// Second row must be unchanged.
	got2, err := d.GetMemory(id2)
	require.NoError(t, err)
	assert.Equal(t, "Row 2", got2.Title, "second row with same topic_key must be unaffected")
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

func TestSchema_MemoryPromptLinksExists(t *testing.T) {
	d := openTestDB(t)

	var name string
	err := d.sqlDB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='memory_prompt_links'`).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "memory_prompt_links", name)
}

func TestSaveMemory_WithPromptIDCreatesPromptLink(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	prompt, err := d.SavePromptForSession(t.Context(), "proj", "manual-save-proj", "Please save this decision")
	require.NoError(t, err)

	mem := newMemory("proj", "Decision", "Captured from current prompt")
	mem.PromptID = prompt.ID
	id, err := d.SaveMemory(mem)
	require.NoError(t, err)

	var linkedPromptID int64
	require.NoError(t, d.sqlDB.QueryRow(
		`SELECT prompt_id FROM memory_prompt_links WHERE memory_id = ?`, id,
	).Scan(&linkedPromptID))
	assert.Equal(t, prompt.ID, linkedPromptID)
}

func TestSaveMemory_WithoutPromptIDCreatesNoPromptLink(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	_, err := d.SavePromptForSession(t.Context(), "proj", "manual-save-proj", "Prompt exists but capture is disabled")
	require.NoError(t, err)

	id, err := d.SaveMemory(newMemory("proj", "Automated artifact", "capture_prompt:false behavior"))
	require.NoError(t, err)

	var links int
	require.NoError(t, d.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM memory_prompt_links WHERE memory_id = ?`, id,
	).Scan(&links))
	assert.Equal(t, 0, links)
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
			name: "new save with same topic_key after delete creates fresh row, tombstone preserved",
			assert: func(t *testing.T, d *DB, id int64) {
				key := "soft/delete/topic"
				require.NoError(t, d.DeleteMemory(id, "tester", "obsolete"))

				// Re-saving with the same topic_key must now SUCCEED (no upsert guard).
				newID, err := d.SaveMemory(&models.Memory{
					Project:   "soft-delete",
					TopicKey:  &key,
					Title:     "New version",
					Content:   "New content",
					SessionID: "manual-save-soft-delete",
				})
				require.NoError(t, err, "SaveMemory must not fail when prior row is deleted")
				assert.NotEqual(t, id, newID, "new save must create a distinct row")

				// New row must be active.
				got, err := d.GetMemory(newID)
				require.NoError(t, err)
				assert.Equal(t, "New version", got.Title)

				// Original tombstoned row is still present and deleted.
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

func TestMemorySoftDeleteNormalReadsHideTombstones(t *testing.T) {
	d := openTestDB(t)
	activeID, err := saveTestMemory(t, d, newMemory("soft-delete-normal", "Active auth note", "shared auth search term"))
	require.NoError(t, err)
	deletedID, err := saveTestMemory(t, d, newMemory("soft-delete-normal", "Deleted auth note", "shared auth search term"))
	require.NoError(t, err)

	require.NoError(t, d.DeleteMemory(deletedID, "tester", "duplicate entry"))

	_, err = d.GetMemory(deletedID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory not found")

	listed, err := d.ListMemories("soft-delete-normal", 10)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, activeID, listed[0].ID)

	searched, err := d.Search("auth", "soft-delete-normal", "", 10)
	require.NoError(t, err)
	require.Len(t, searched, 1)
	assert.Equal(t, activeID, searched[0].ID)

	deleted, err := d.GetDeletedMemory(deletedID)
	require.NoError(t, err)
	assert.Equal(t, "tester", deleted.DeletedBy)
	assert.Equal(t, "duplicate entry", deleted.DeleteReason)
	assert.False(t, deleted.DeletedAt.IsZero())

	mutations, err := d.GetPendingMutations("soft-delete-normal", 10)
	require.NoError(t, err)
	require.NotEmpty(t, mutations)
	deleteMutation := mutations[len(mutations)-1]
	require.Equal(t, MutationOpDelete, deleteMutation.Op)
	require.NotNil(t, deleteMutation.Tombstone)
	assert.Equal(t, "tester", deleteMutation.Tombstone.DeletedBy)
	assert.Equal(t, "duplicate entry", deleteMutation.Tombstone.Reason)
}

func TestMemoryMutationsJournaledTransactionally(t *testing.T) {
	d := openTestDB(t)
	key := "journal/topic"

	// First save — creates row id1.
	id1, err := saveTestMemory(t, d, &models.Memory{
		Project:   "journal-project",
		TopicKey:  &key,
		Title:     "Created",
		Content:   "Content",
		SessionID: "manual-save-journal-project",
	})
	require.NoError(t, err)

	// Second save with same topic_key — must create a DISTINCT row id2.
	id2, err := d.SaveMemory(&models.Memory{
		Project:   "journal-project",
		TopicKey:  &key,
		Title:     "Second",
		Content:   "Second content",
		SessionID: "manual-save-journal-project",
	})
	require.NoError(t, err)
	require.NotEqual(t, id1, id2, "same topic_key must now create a distinct row")

	// Delete + Restore the second row to complete a coherent entity lifecycle.
	require.NoError(t, d.DeleteMemory(id2, "tester", "done"))
	require.NoError(t, d.RestoreMemory(id2, "tester"))

	rows, err := d.sqlDB.Query(`SELECT op, entity_sync_id, project, event_id FROM memory_mutations ORDER BY sequence ASC`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var ops []string
	var syncIDs []string
	for rows.Next() {
		var op, entitySyncID, project, eventID string
		require.NoError(t, rows.Scan(&op, &entitySyncID, &project, &eventID))
		ops = append(ops, op)
		syncIDs = append(syncIDs, entitySyncID)
		assert.Equal(t, "journal-project", project)
		assert.NotEmpty(t, entitySyncID)
		assert.NotEmpty(t, eventID)
	}
	require.NoError(t, rows.Err())

	// Two creates, then delete and restore on the second entity.
	assert.Equal(t, []string{"create", "create", "delete", "restore"}, ops)

	// The two creates must carry distinct sync_ids.
	assert.NotEqual(t, syncIDs[0], syncIDs[1], "two creates carry distinct sync_ids")
	// Delete and restore must reference the second row's sync_id.
	assert.Equal(t, syncIDs[1], syncIDs[2], "delete references the second row's sync_id")
	assert.Equal(t, syncIDs[2], syncIDs[3], "restore references the same row")

	// Failed delete must not add a journal row.
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
