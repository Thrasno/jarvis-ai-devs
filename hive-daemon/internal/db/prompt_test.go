package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
)

// ─── Schema tests ──────────────────────────────────────────────────────────

func TestSchema_UserPrompts_HasProjectColumn(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	sqlDB := d.RawDB()
	rows, err := sqlDB.Query("PRAGMA table_info(user_prompts)")
	if err != nil {
		t.Fatalf("PRAGMA table_info failed: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if name == "project" {
			found = true
			break
		}
	}
	if !found {
		t.Error("user_prompts table is missing 'project' column")
	}
}

func TestSchema_UserPrompts_HasSyncIDColumn(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	sqlDB := d.RawDB()
	rows, err := sqlDB.Query("PRAGMA table_info(user_prompts)")
	if err != nil {
		t.Fatalf("PRAGMA table_info failed: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if name == "sync_id" {
			found = true
			break
		}
	}
	if !found {
		t.Error("user_prompts table is missing 'sync_id' column")
	}
}

func TestSchema_UserPrompts_HasProjectIndex(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	sqlDB := d.RawDB()
	var name string
	err = sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_user_prompts_project_created'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("index idx_user_prompts_project_created not found: %v", err)
	}
	if name != "idx_user_prompts_project_created" {
		t.Errorf("expected index name %q, got %q", "idx_user_prompts_project_created", name)
	}
}

// ─── SavePrompt tests ──────────────────────────────────────────────────────

func TestSavePrompt_HappyPath(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "jarvis-dev", "What is the capital of France?")
	if err != nil {
		t.Fatalf("SavePrompt() unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("SavePrompt() returned nil prompt")
	}
	if p.ID <= 0 {
		t.Errorf("expected ID > 0, got %d", p.ID)
	}
	if p.Content != "What is the capital of France?" {
		t.Errorf("expected Content %q, got %q", "What is the capital of France?", p.Content)
	}
	if p.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if p.SyncedAt != nil {
		t.Errorf("expected SyncedAt == nil, got %v", p.SyncedAt)
	}
}

func TestSavePrompt_EmptyContentReturnsError(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "proj", "")
	if err == nil {
		t.Error("expected error for empty content, got nil")
	}
	if p != nil {
		t.Errorf("expected nil prompt on error, got %+v", p)
	}
	if err != nil && !strings.Contains(err.Error(), "content") {
		t.Errorf("expected error message to contain %q, got %q", "content", err.Error())
	}
}

func TestSavePrompt_WhitespaceOnlyReturnsError(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "proj", "   \t\n  ")
	if err == nil {
		t.Error("expected error for whitespace-only content, got nil")
	}
	if p != nil {
		t.Errorf("expected nil prompt on error, got %+v", p)
	}
}

func TestSavePrompt_FTSRowCreatedOnInsert(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	_, err = d.SavePrompt(ctx, "proj", "deploy kubernetes cluster")
	if err != nil {
		t.Fatalf("SavePrompt() unexpected error: %v", err)
	}

	sqlDB := d.RawDB()
	var count int
	err = sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_prompts_fts WHERE user_prompts_fts MATCH '"deploy"'`).Scan(&count)
	if err != nil {
		t.Fatalf("FTS query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected FTS count == 1, got %d", count)
	}
}

func TestSavePrompt_FTSRowRemovedOnDelete(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "proj", "remove this prompt from fts")
	if err != nil {
		t.Fatalf("SavePrompt() unexpected error: %v", err)
	}

	sqlDB := d.RawDB()
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM user_prompts WHERE id = ?", p.ID)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}

	var count int
	err = sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_prompts_fts WHERE user_prompts_fts MATCH '"remove"'`).Scan(&count)
	if err != nil {
		t.Fatalf("FTS query after delete failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected FTS count == 0 after delete, got %d", count)
	}
}

func TestValidateSchema_PassesAfterOpen(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(':memory:') failed: %v", err)
	}
	defer func() { _ = d.Close() }()
}

func TestSavePrompt_TwoCalls_ProduceDistinctIDs(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p1, err := d.SavePrompt(ctx, "proj", "first prompt")
	if err != nil {
		t.Fatalf("first SavePrompt() unexpected error: %v", err)
	}
	p2, err := d.SavePrompt(ctx, "proj", "first prompt")
	if err != nil {
		t.Fatalf("second SavePrompt() unexpected error: %v", err)
	}
	if p1.ID == p2.ID {
		t.Errorf("expected distinct IDs, got %d and %d", p1.ID, p2.ID)
	}
}

func TestSavePrompt_WhitespaceOnlyProjectReturnsError(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "   \t\n  ", "some content")
	if err == nil {
		t.Error("expected error for whitespace-only project, got nil")
	}
	if p != nil {
		t.Errorf("expected nil prompt on error, got %+v", p)
	}
	if err != nil && !strings.Contains(err.Error(), "project") {
		t.Errorf("expected error message to contain %q, got %q", "project", err.Error())
	}
}

func TestSavePrompt_EmptyProjectReturnsError(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "", "some content")
	if err == nil {
		t.Error("expected error for empty project, got nil")
	}
	if p != nil {
		t.Errorf("expected nil prompt on error, got %+v", p)
	}
	if err != nil && !strings.Contains(err.Error(), "project") {
		t.Errorf("expected error message to contain %q, got %q", "project", err.Error())
	}
}

// ─── T-DB-2: new SavePrompt tests ─────────────────────────────────────────

func TestSavePrompt_WithProject_StoresProjectAndSyncID(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "jarvis-dev", "explain goroutines")
	if err != nil {
		t.Fatalf("SavePrompt() unexpected error: %v", err)
	}
	if p.Project != "jarvis-dev" {
		t.Errorf("Project = %q, want 'jarvis-dev'", p.Project)
	}
	if p.SyncID == "" {
		t.Error("SyncID should not be empty")
	}
}

func TestSavePrompt_SyncIDIsValidUUID(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "proj", "test content")
	if err != nil {
		t.Fatalf("SavePrompt() unexpected error: %v", err)
	}
	if _, uuidErr := uuid.Parse(p.SyncID); uuidErr != nil {
		t.Errorf("SyncID %q is not a valid UUID: %v", p.SyncID, uuidErr)
	}
}

func TestSavePrompt_TwoCalls_ProduceDistinctSyncIDs(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p1, err := d.SavePrompt(ctx, "proj", "first content")
	if err != nil {
		t.Fatalf("first SavePrompt() unexpected error: %v", err)
	}
	p2, err := d.SavePrompt(ctx, "proj", "second content")
	if err != nil {
		t.Fatalf("second SavePrompt() unexpected error: %v", err)
	}
	if p1.SyncID == p2.SyncID {
		t.Errorf("expected distinct SyncIDs, got both %q", p1.SyncID)
	}
}

// ─── T-DB-3: ListRecentPrompts tests ──────────────────────────────────────

func TestListRecentPrompts_EmptyProject_ReturnsNil(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	result, err := d.ListRecentPrompts(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty project, got %v", result)
	}
}

func TestListRecentPrompts_NoRowsForProject_ReturnsEmpty(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	result, err := d.ListRecentPrompts(context.Background(), "nonexistent-proj", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestListRecentPrompts_ReturnsPromptsSavedForProject(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()

	// Save 3 for proj-X, 1 for proj-Y
	for i := 0; i < 3; i++ {
		_, err = d.SavePrompt(ctx, "proj-X", "prompt for X")
		if err != nil {
			t.Fatalf("SavePrompt proj-X: %v", err)
		}
	}
	_, err = d.SavePrompt(ctx, "proj-Y", "prompt for Y")
	if err != nil {
		t.Fatalf("SavePrompt proj-Y: %v", err)
	}

	result, err := d.ListRecentPrompts(ctx, "proj-X", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 prompts for proj-X, got %d", len(result))
	}
	for _, p := range result {
		if p.Project != "proj-X" {
			t.Errorf("expected project 'proj-X', got %q", p.Project)
		}
	}
}

func TestListRecentPrompts_OrderedByCreatedAtDesc(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	sqlDB := d.RawDB()

	// Insert rows with explicitly distinct created_at: SQLite CURRENT_TIMESTAMP has 1-second resolution
	base := time.Now().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Second).Format("2006-01-02 15:04:05")
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO user_prompts (sync_id, project, content, created_at) VALUES (?, 'proj', 'prompt', ?)`,
			uuid.NewString(), ts)
		if err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	result, err := d.ListRecentPrompts(ctx, "proj", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 prompts, got %d — insert failed?", len(result))
	}

	// Verify descending order
	for i := 1; i < len(result); i++ {
		if result[i].CreatedAt.After(result[i-1].CreatedAt) {
			t.Errorf("prompts not in descending order: index %d (%v) is after index %d (%v)",
				i, result[i].CreatedAt, i-1, result[i-1].CreatedAt)
		}
	}
}

func TestListRecentPrompts_LimitIsRespected(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()

	// Save 5 prompts
	for i := 0; i < 5; i++ {
		_, err = d.SavePrompt(ctx, "proj", "prompt")
		if err != nil {
			t.Fatalf("SavePrompt: %v", err)
		}
	}

	result, err := d.ListRecentPrompts(ctx, "proj", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 prompts (limit), got %d", len(result))
	}
}

func TestListRecentPrompts_LimitCappedAt100(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	for i := 0; i < 105; i++ {
		_, err = d.SavePrompt(ctx, "proj", "prompt")
		if err != nil {
			t.Fatalf("SavePrompt: %v", err)
		}
	}

	result, err := d.ListRecentPrompts(ctx, "proj", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 100 {
		t.Errorf("expected 100 prompts (cap), got %d", len(result))
	}
}

func TestListRecentPrompts_SyncedAtIsScanned(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "proj", "content")
	if err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}

	// Manually set synced_at on the row
	now := time.Now().UTC().Truncate(time.Second)
	sqlDB := d.RawDB()
	_, err = sqlDB.ExecContext(ctx, "UPDATE user_prompts SET synced_at=? WHERE id=?", now.Format("2006-01-02 15:04:05"), p.ID)
	if err != nil {
		t.Fatalf("UPDATE synced_at: %v", err)
	}

	result, err := d.ListRecentPrompts(ctx, "proj", 10)
	if err != nil {
		t.Fatalf("ListRecentPrompts: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].SyncedAt == nil {
		t.Error("expected SyncedAt to be populated, got nil")
	}
}

