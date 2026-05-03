package db_test

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
)

func TestSavePrompt_HappyPath(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	p, err := d.SavePrompt(ctx, "What is the capital of France?")
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
	p, err := d.SavePrompt(ctx, "")
	if err == nil {
		t.Error("expected error for empty content, got nil")
	}
	if p != nil {
		t.Errorf("expected nil prompt on error, got %+v", p)
	}
	if err != nil && !containsString(err.Error(), "content") {
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
	p, err := d.SavePrompt(ctx, "   \t\n  ")
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
	_, err = d.SavePrompt(ctx, "deploy kubernetes cluster")
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
	p, err := d.SavePrompt(ctx, "remove this prompt from fts")
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
	p1, err := d.SavePrompt(ctx, "first prompt")
	if err != nil {
		t.Fatalf("first SavePrompt() unexpected error: %v", err)
	}
	p2, err := d.SavePrompt(ctx, "first prompt")
	if err != nil {
		t.Fatalf("second SavePrompt() unexpected error: %v", err)
	}
	if p1.ID == p2.ID {
		t.Errorf("expected distinct IDs, got %d and %d", p1.ID, p2.ID)
	}
}

// containsString is a helper to avoid importing strings in tests.
func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
