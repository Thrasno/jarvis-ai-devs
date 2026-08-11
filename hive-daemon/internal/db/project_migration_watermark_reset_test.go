package db

import (
	"context"
	"testing"
)

// TestCoalesceProjectSyncStateReportsResetWatermarks makes the one deliberately
// destructive-looking merge legible.
//
// keepEarlierSyncWatermark clears last_sync_at whenever the two rows cannot be
// ordered or either side is NULL, which makes that project re-pull its full
// window on its next sync. That is safe and idempotent, but from the outside it
// looks exactly like a fold that lost the sync position — so the summary has to
// say which canonical projects it happened to, by name, or the operator has only
// a suspicion to act on.
func TestCoalesceProjectSyncStateReportsResetWatermarks(t *testing.T) {
	t.Run("null watermark on one side is reported", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		seedNullWatermarkSyncStateRow(t, database, "Foo", "2026-03-02 00:00:00")
		seedSyncStateRow(t, database, "foo", "2026-02-01 00:00:00", "2026-02-02 00:00:00", "")
		summary := coalesceMigrationSyncStateWithSummary(t, database)
		if got := syncStateColumn(t, database, "foo", "last_sync_at"); got != "" {
			t.Fatalf("last_sync_at = %q, want the reset this test is about", got)
		}
		if len(summary.SyncPositionsReset) != 1 || summary.SyncPositionsReset[0] != "foo" {
			t.Fatalf("sync positions reset = %v, want [foo]", summary.SyncPositionsReset)
		}
	})

	t.Run("unorderable watermark is reported", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		seedSyncStateRow(t, database, "Foo", "not-a-timestamp", "2026-03-02 00:00:00", "left-token")
		seedSyncStateRow(t, database, "foo", "2026-02-01 00:00:00", "2026-02-02 00:00:00", "")
		summary := coalesceMigrationSyncStateWithSummary(t, database)
		if len(summary.SyncPositionsReset) != 1 || summary.SyncPositionsReset[0] != "foo" {
			t.Fatalf("sync positions reset = %v, want [foo]", summary.SyncPositionsReset)
		}
	})

	t.Run("an orderable merge resets nothing", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		seedSyncStateRow(t, database, "Foo", "2026-03-01 00:00:00", "2026-03-02 00:00:00", "left-token")
		seedSyncStateRow(t, database, "foo", "2026-02-01 00:00:00", "2026-02-02 00:00:00", "")
		summary := coalesceMigrationSyncStateWithSummary(t, database)
		if len(summary.SyncPositionsReset) != 0 {
			t.Fatalf("sync positions reset = %v, want none; both watermarks were orderable", summary.SyncPositionsReset)
		}
	})

	t.Run("two projects with nothing to lose report nothing", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		seedNullWatermarkSyncStateRow(t, database, "Foo", "2026-03-02 00:00:00")
		seedNullWatermarkSyncStateRow(t, database, "foo", "2026-02-02 00:00:00")
		summary := coalesceMigrationSyncStateWithSummary(t, database)
		if len(summary.SyncPositionsReset) != 0 {
			t.Fatalf("sync positions reset = %v, want none; neither row had a watermark to reset", summary.SyncPositionsReset)
		}
	})
}

// TestProjectMigrationSummaryReportsResetWatermarksEndToEnd proves the counter
// reaches the summary a caller actually reads, not just the internal step.
func TestProjectMigrationSummaryReportsResetWatermarksEndToEnd(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedNullWatermarkSyncStateRow(t, database, "Foo", "2026-03-02 00:00:00")
	seedSyncStateRow(t, database, "foo", "2026-02-01 00:00:00", "2026-02-02 00:00:00", "")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	summary := database.LastProjectMigrationSummary()
	if len(summary.SyncPositionsReset) != 1 || summary.SyncPositionsReset[0] != "foo" {
		t.Fatalf("summary sync positions reset = %v, want [foo]", summary.SyncPositionsReset)
	}
}

// seedNullWatermarkSyncStateRow writes a sync_state row whose pull watermark is
// genuinely NULL rather than an empty string, because NULL is the case
// keepEarlierSyncWatermark treats as "cannot be ordered".
func seedNullWatermarkSyncStateRow(t *testing.T, database *DB, project, lastAttemptAt string) {
	t.Helper()
	if _, err := database.sqlDB.Exec(
		`INSERT INTO sync_state (project, last_sync_at, last_attempt_at, jwt_token, consecutive_failures, last_error) VALUES (?, NULL, ?, '', 4, 'stale failure')`,
		project, lastAttemptAt); err != nil {
		t.Fatal(err)
	}
}

func coalesceMigrationSyncStateWithSummary(t *testing.T, database *DB) ProjectMigrationSummary {
	t.Helper()
	ctx := context.Background()
	tx, err := database.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	records, err := readProjectMigrationRecords(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	var summary ProjectMigrationSummary
	if err := coalesceProjectSyncState(ctx, tx, records, &summary); err != nil {
		t.Fatalf("coalesceProjectSyncState() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return summary
}
