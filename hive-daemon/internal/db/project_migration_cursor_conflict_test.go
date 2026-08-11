package db

import (
	"context"
	"errors"
	"testing"
)

// The cursor inventory identity must carry only the coordinates the executor
// requires to be unique, and the value must carry what the executor allows to
// disagree. If the identity ever embeds its own value again, same-identity
// implies same-value and the planner becomes structurally unable to report a
// collision the executor still aborts on. These tests pin that split from both
// sides: the identity shape itself, and the preflight/executor agreement it buys.

func cursorRecordsFor(t *testing.T, database *DB, table ProjectState) []ProjectStateRecord {
	t.Helper()
	all, err := readProjectMigrationRecords(context.Background(), database.sqlDB)
	if err != nil {
		t.Fatalf("readProjectMigrationRecords() error = %v", err)
	}
	var records []ProjectStateRecord
	for _, record := range all {
		if record.Table == table {
			records = append(records, record)
		}
	}
	return records
}

func TestMutationCursorInventoryIdentityExcludesTheEventItCanDisagreeOn(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for _, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', ?, 7, ?)`, project, "event-"+project); err != nil {
			t.Fatal(err)
		}
	}
	records := cursorRecordsFor(t, database, ProjectStateMutationCursors)
	if len(records) != 2 {
		t.Fatalf("mutation cursor records = %d, want 2", len(records))
	}
	if records[0].Identity != records[1].Identity {
		t.Fatalf("identities = %q and %q; two rows that differ only in event_id must share one identity, otherwise the planner can never see the collision", records[0].Identity, records[1].Identity)
	}
	if records[0].Value == records[1].Value {
		t.Fatalf("values = %q and %q; the divergent event_id must survive as the value", records[0].Value, records[1].Value)
	}
}

func TestPullCursorInventoryIdentityExcludesTheSyncItCanDisagreeOn(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for _, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', ?, 'memories', '2026-01-01T00:00:00Z', ?)`, project, "pull-"+project); err != nil {
			t.Fatal(err)
		}
	}
	records := cursorRecordsFor(t, database, ProjectStatePullCursors)
	if len(records) != 2 {
		t.Fatalf("pull cursor records = %d, want 2", len(records))
	}
	if records[0].Identity != records[1].Identity {
		t.Fatalf("identities = %q and %q; two rows that differ only in sync_id must share one identity", records[0].Identity, records[1].Identity)
	}
	if records[0].Value == records[1].Value {
		t.Fatalf("values = %q and %q; the divergent sync_id must survive as the value", records[0].Value, records[1].Value)
	}
}

func TestProjectMigrationPlanReportsMutationCursorCollisionOnTheSameSequence(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for _, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', ?, 7, ?)`, project, "event-"+project); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable {
		t.Fatalf("plan = %#v, want a non-executable plan", plan)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].Kind != ConflictNonMonotonicCursorProtocol || plan.Conflicts[0].Table != ProjectStateMutationCursors {
		t.Fatalf("conflicts = %#v, want exactly one non-monotonic-cursor-protocol conflict on mutation_cursors", plan.Conflicts)
	}
}

// A strictly higher sequence is not a contradiction: readMutationCursors keeps
// the higher one, so the preflight must report coalescible work, not a conflict.
func TestProjectMigrationPlanAllowsMutationCursorsOnDifferentSequences(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for index, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', ?, ?, ?)`, project, 7+index, "event-"+project); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Executable || len(plan.Conflicts) != 0 {
		t.Fatalf("plan = %#v, want an executable plan without conflicts", plan)
	}
}

func TestProjectMigrationPlanReportsPullCursorCollisionOnTheSameSyncedAt(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for _, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', ?, 'memories', '2026-01-01T00:00:00Z', ?)`, project, "pull-"+project); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable {
		t.Fatalf("plan = %#v, want a non-executable plan", plan)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].Kind != ConflictNonMonotonicCursorProtocol || plan.Conflicts[0].Table != ProjectStatePullCursors {
		t.Fatalf("conflicts = %#v, want exactly one non-monotonic-cursor-protocol conflict on pull_cursors", plan.Conflicts)
	}
}

// A strictly later synced_at is resolved by readPullCursors keeping the later
// row, so it must not surface as a conflict.
func TestProjectMigrationPlanAllowsPullCursorsOnDifferentSyncedAt(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for index, project := range []string{"Foo", "foo"} {
		syncedAt := []string{"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"}[index]
		if _, err := database.sqlDB.Exec(`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', ?, 'memories', ?, ?)`, project, syncedAt, "pull-"+project); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Executable || len(plan.Conflicts) != 0 {
		t.Fatalf("plan = %#v, want an executable plan without conflicts", plan)
	}
}

// The executor's own cursor checks stay as defence in depth. They are no longer
// reachable through a plan the preflight approved, so they are exercised
// directly against the same transaction the executor would use.
func TestCursorReadersStillRejectCollisionsAsDefenceInDepth(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		statements []string
		read       func(context.Context, *DB) error
	}{
		{
			name: "mutation_cursors",
			statements: []string{
				`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', 'Foo', 7, 'event-left')`,
				`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', 'foo', 7, 'event-right')`,
			},
			read: func(ctx context.Context, database *DB) error {
				tx, err := database.sqlDB.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()
				_, err = readMutationCursors(ctx, tx)
				return err
			},
		},
		{
			name: "pull_cursors",
			statements: []string{
				`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', 'Foo', 'memories', '2026-01-01T00:00:00Z', 'pull-left')`,
				`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', 'foo', 'memories', '2026-01-01T00:00:00Z', 'pull-right')`,
			},
			read: func(ctx context.Context, database *DB) error {
				tx, err := database.sqlDB.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()
				_, err = readPullCursors(ctx, tx)
				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			database := newMigrationExecutorDB(t)
			seedMigrationProject(t, database, "Foo")
			for _, statement := range testCase.statements {
				if _, err := database.sqlDB.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if err := testCase.read(context.Background(), database); !errors.Is(err, ErrProjectMigrationConflict) {
				t.Fatalf("reader error = %v, want a migration conflict", err)
			}
		})
	}
}
