package db

import (
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
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

// helper para construir una Memory mínima válida
func newMemory(project, title, content string) *models.Memory {
	return &models.Memory{
		Project: project,
		Title:   title,
		Content: content,
	}
}

// ─── 3.1 SaveMemory ────────────────────────────────────────────────────────

func TestSaveMemory_CreatesNewRow(t *testing.T) {
	d := openTestDB(t)

	id, err := d.SaveMemory(newMemory("proj", "Title", "Content"))
	if err != nil {
		t.Fatalf("SaveMemory() failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("SaveMemory() id = %d, want > 0", id)
	}
}

func TestSaveMemory_PopulatesSyncIDAndCreatedBy(t *testing.T) {
	d := openTestDB(t)

	id, err := d.SaveMemory(newMemory("proj", "Title", "Content"))
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

	key := "arch/auth"
	mem := &models.Memory{
		Project:  "proj",
		Title:    "Original",
		Content:  "Original content",
		TopicKey: &key,
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
		if _, err := d.SaveMemory(newMemory("foo", "foo mem", "c")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.SaveMemory(newMemory("bar", "bar mem", "c")); err != nil {
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
		if _, err := d.SaveMemory(newMemory("proj", "mem", "c")); err != nil {
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
		if _, err := d.SaveMemory(newMemory("proj", title, "c")); err != nil {
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
