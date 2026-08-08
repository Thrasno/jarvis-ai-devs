package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
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

func TestPromptSessionSemantics(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePromptForSession(ctx, "jarvis-dev", "sess-001", "older same session")
	if err != nil {
		t.Fatalf("SavePromptForSession older: %v", err)
	}
	_, err = d.SavePromptForSession(ctx, "jarvis-dev", "sess-002", "other session")
	if err != nil {
		t.Fatalf("SavePromptForSession other session: %v", err)
	}
	_, err = d.SavePromptForSession(ctx, "other", "sess-001", "other project")
	if err != nil {
		t.Fatalf("SavePromptForSession other project: %v", err)
	}
	_, err = d.SavePromptForSession(ctx, "jarvis-dev", "sess-001", "latest same session")
	if err != nil {
		t.Fatalf("SavePromptForSession latest: %v", err)
	}

	if p.SessionID != "sess-001" {
		t.Errorf("Prompt.SessionID = %q, want %q", p.SessionID, "sess-001")
	}
	var storedSessionID string
	if err := d.RawDB().QueryRowContext(ctx, "SELECT session_id FROM user_prompts WHERE id = ?", p.ID).Scan(&storedSessionID); err != nil {
		t.Fatalf("query session_id: %v", err)
	}
	if storedSessionID != "sess-001" {
		t.Errorf("stored session_id = %q, want %q", storedSessionID, "sess-001")
	}

	prompt, err := d.LatestPromptForSession(ctx, "jarvis-dev", "sess-001")
	if err != nil {
		t.Fatalf("LatestPromptForSession() unexpected error: %v", err)
	}
	if prompt == nil {
		t.Fatal("LatestPromptForSession() returned nil")
	}
	if prompt.Content != "latest same session" {
		t.Errorf("Content = %q, want %q", prompt.Content, "latest same session")
	}
	if prompt.Project != "jarvis-dev" || prompt.SessionID != "sess-001" {
		t.Errorf("prompt scope = (%q, %q), want (jarvis-dev, sess-001)", prompt.Project, prompt.SessionID)
	}

	prompt, err = d.LatestPromptForSession(ctx, "jarvis-dev", "missing-session")
	if err != nil {
		t.Fatalf("LatestPromptForSession() unexpected error: %v", err)
	}
	if prompt != nil {
		t.Fatalf("LatestPromptForSession() = %+v, want nil", prompt)
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
		if p.Project != "proj-x" {
			t.Errorf("expected project 'proj-x', got %q", p.Project)
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

// ─── GetUnsyncedPrompts tests ─────────────────────────────────────────────

func TestGetUnsyncedPrompts_ReturnsOnlyUnsyncedForProject(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()

	// Save 3 prompts for proj-A, 1 for proj-B
	p1, err := d.SavePrompt(ctx, "proj-A", "first prompt")
	if err != nil {
		t.Fatalf("SavePrompt 1: %v", err)
	}
	_, err = d.SavePrompt(ctx, "proj-A", "second prompt")
	if err != nil {
		t.Fatalf("SavePrompt 2: %v", err)
	}
	_, err = d.SavePrompt(ctx, "proj-B", "other project prompt")
	if err != nil {
		t.Fatalf("SavePrompt proj-B: %v", err)
	}

	// Mark p1 as synced
	sqlDB := d.RawDB()
	now := time.Now().UTC().Truncate(time.Second)
	_, err = sqlDB.ExecContext(ctx, "UPDATE user_prompts SET synced_at=? WHERE id=?", now.Format("2006-01-02 15:04:05"), p1.ID)
	if err != nil {
		t.Fatalf("UPDATE synced_at: %v", err)
	}

	result, err := d.GetUnsyncedPrompts(ctx, "proj-A")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts: %v", err)
	}
	// Only 1 unsynced for proj-A (p1 is synced, proj-B is excluded)
	if len(result) != 1 {
		t.Errorf("expected 1 unsynced prompt for proj-A, got %d", len(result))
	}
	if result[0].SyncedAt != nil {
		t.Error("expected SyncedAt == nil for unsynced prompt")
	}
	if result[0].Project != "proj-a" {
		t.Errorf("expected project 'proj-a', got %q", result[0].Project)
	}
}

func TestGetUnsyncedPrompts_AllSynced_ReturnsEmpty(t *testing.T) {
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

	sqlDB := d.RawDB()
	now := time.Now().UTC().Truncate(time.Second)
	_, err = sqlDB.ExecContext(ctx, "UPDATE user_prompts SET synced_at=? WHERE id=?", now.Format("2006-01-02 15:04:05"), p.ID)
	if err != nil {
		t.Fatalf("UPDATE synced_at: %v", err)
	}

	result, err := d.GetUnsyncedPrompts(ctx, "proj")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 unsynced prompts (all synced), got %d", len(result))
	}
}

func TestGetUnsyncedPrompts_OrderedByCreatedAtAsc(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	sqlDB := d.RawDB()

	// Insert rows with explicitly distinct created_at timestamps
	base := time.Now().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Second).Format("2006-01-02 15:04:05")
		_, err := sqlDB.ExecContext(ctx, `INSERT INTO user_prompts (sync_id, project, content, created_at) VALUES (?, 'proj', 'prompt', ?)`,
			uuid.NewString(), ts)
		if err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	result, err := d.GetUnsyncedPrompts(ctx, "proj")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 unsynced prompts, got %d", len(result))
	}

	// Verify ascending order (oldest first)
	for i := 1; i < len(result); i++ {
		if result[i].CreatedAt.Before(result[i-1].CreatedAt) {
			t.Errorf("prompts not in ascending order: index %d (%v) is before index %d (%v)",
				i, result[i].CreatedAt, i-1, result[i-1].CreatedAt)
		}
	}
}

// ─── MarkPromptSynced tests ───────────────────────────────────────────────

func TestMarkPromptSynced_SetsSyncedAt(t *testing.T) {
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
	if p.SyncedAt != nil {
		t.Fatal("expected SyncedAt == nil before marking synced")
	}

	now := time.Now().UTC().Truncate(time.Second)
	err = d.MarkPromptSynced(ctx, p.SyncID, now)
	if err != nil {
		t.Fatalf("MarkPromptSynced: %v", err)
	}

	// Verify via GetUnsyncedPrompts: after marking, the prompt should not appear
	remaining, err := d.GetUnsyncedPrompts(ctx, "proj")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts after mark: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 unsynced after MarkPromptSynced, got %d", len(remaining))
	}

	// Also verify via DB that synced_at is non-NULL
	sqlDB := d.RawDB()
	var syncedAtStr *string
	err = sqlDB.QueryRowContext(ctx, "SELECT synced_at FROM user_prompts WHERE sync_id=?", p.SyncID).Scan(&syncedAtStr)
	if err != nil {
		t.Fatalf("query synced_at: %v", err)
	}
	if syncedAtStr == nil {
		t.Fatal("expected synced_at to be non-NULL after MarkPromptSynced")
	}
}

func TestMarkPromptSynced_RemovesFromUnsynced(t *testing.T) {
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

	// Before marking: 1 unsynced
	before, err := d.GetUnsyncedPrompts(ctx, "proj")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts before: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected 1 unsynced before marking, got %d", len(before))
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := d.MarkPromptSynced(ctx, p.SyncID, now); err != nil {
		t.Fatalf("MarkPromptSynced: %v", err)
	}

	// After marking: 0 unsynced
	after, err := d.GetUnsyncedPrompts(ctx, "proj")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts after: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("expected 0 unsynced after marking, got %d", len(after))
	}

	// ctx used only in setup above
	_ = ctx
}

// FIX-2: rows with sync_id="" must be excluded from GetUnsyncedPrompts.
// Old rows created before UUID generation have sync_id="". The server rejects
// them with 400 (UUID validation), so they must never reach the sync pipeline.
func TestGetUnsyncedPrompts_ExcludesEmptySyncID(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	sqlDB := d.RawDB()

	// Insert a legacy row with sync_id = '' directly (bypasses SavePrompt which always generates a UUID).
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO user_prompts (sync_id, project, content, created_at) VALUES (?, 'proj', 'legacy prompt', CURRENT_TIMESTAMP)`,
		"",
	)
	if err != nil {
		t.Fatalf("insert legacy row with empty sync_id: %v", err)
	}

	// Insert a valid row with a proper UUID for contrast.
	_, err = d.SavePrompt(ctx, "proj", "valid prompt")
	if err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}

	result, err := d.GetUnsyncedPrompts(ctx, "proj")
	if err != nil {
		t.Fatalf("GetUnsyncedPrompts: %v", err)
	}

	// Only the valid-UUID row should be returned; the empty-sync_id row must be excluded.
	if len(result) != 1 {
		t.Errorf("expected 1 unsynced prompt (empty sync_id excluded), got %d", len(result))
	}
	if len(result) == 1 && result[0].SyncID == "" {
		t.Error("returned row has empty sync_id — exclusion filter is not working")
	}
}

// FIX-5: MarkPromptSynced with a non-existent syncID must return nil (non-fatal).
func TestMarkPromptSynced_NonExistentSyncID_ReturnsNil(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()

	// Call MarkPromptSynced with a UUID that doesn't exist in the DB.
	err = d.MarkPromptSynced(ctx, "non-existent-uuid-1234-5678-abcd-ef0123456789", time.Now())
	if err != nil {
		t.Errorf("MarkPromptSynced on non-existent syncID should return nil, got: %v", err)
	}
}
