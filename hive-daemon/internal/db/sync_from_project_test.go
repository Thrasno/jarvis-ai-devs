package db

import (
	"context"
	"testing"
	"time"
)

// TestUnsyncedRowsCarryPendingRelocation pins the read half of the relocation:
// a pending sync_from_project must reach the push payload, or the server has no
// way to tell an ordinary re-push from a project move and refuses to relocate.
func TestUnsyncedRowsCarryPendingRelocation(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedPropagationProject(t, database, "Foo.Bar")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}

	sessions, err := database.ListUnsyncedSessionsPage("foo-bar", 100)
	if err != nil {
		t.Fatalf("ListUnsyncedSessionsPage() error = %v", err)
	}
	relocations := map[string]string{}
	for _, session := range sessions {
		relocations[session.ID] = session.SyncFromProject
	}
	if got := relocations["synced-session"]; got != "Foo.Bar" {
		t.Fatalf("relocated session SyncFromProject = %q, want %q", got, "Foo.Bar")
	}
	if got, ok := relocations["unsynced-session"]; !ok || got != "" {
		t.Fatalf("never-pushed session SyncFromProject = %q (present=%v), want empty", got, ok)
	}

	prompts, err := database.GetUnsyncedPromptsPage(context.Background(), "foo-bar", 100)
	if err != nil {
		t.Fatalf("GetUnsyncedPromptsPage() error = %v", err)
	}
	promptRelocations := map[string]string{}
	for _, prompt := range prompts {
		promptRelocations[prompt.SyncID] = prompt.SyncFromProject
	}
	if got := promptRelocations["synced-prompt"]; got != "Foo.Bar" {
		t.Fatalf("relocated prompt SyncFromProject = %q, want %q", got, "Foo.Bar")
	}
	if got, ok := promptRelocations["unsynced-prompt"]; !ok || got != "" {
		t.Fatalf("never-pushed prompt SyncFromProject = %q (present=%v), want empty", got, ok)
	}
}

// TestAckClearsPendingRelocation pins the lifetime of the precondition: once the
// server has acked the row under its new name the move is done, and re-asserting
// from_project on every later push would keep claiming a relocation that already
// happened.
func TestAckClearsPendingRelocation(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedPropagationProject(t, database, "Foo.Bar")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}

	now := time.Now().UTC()
	if err := database.MarkSessionSynced("synced-session", now); err != nil {
		t.Fatalf("MarkSessionSynced() error = %v", err)
	}
	if err := database.MarkPromptSynced(context.Background(), "synced-prompt", now); err != nil {
		t.Fatalf("MarkPromptSynced() error = %v", err)
	}

	assertRelocationCleared(t, database, `SELECT sync_from_project FROM sessions WHERE id = ?`, "synced-session")
	assertRelocationCleared(t, database, `SELECT sync_from_project FROM user_prompts WHERE sync_id = ?`, "synced-prompt")
}

func assertRelocationCleared(t *testing.T, database *DB, query, key string) {
	t.Helper()
	var fromProject string
	if err := database.sqlDB.QueryRow(query, key).Scan(&fromProject); err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	if fromProject != "" {
		t.Fatalf("%s sync_from_project = %q after ack, want cleared", key, fromProject)
	}
}
