package db

import (
	"context"
	"database/sql"
	"testing"
)

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

// TestMergeSyncStateRowsKeepsEarlierPullWatermark locks the one column whose
// merge direction is inverted: last_sync_at is the pull watermark handed to the
// server as `since`, so keeping the later value would permanently skip every
// remote row in [target, source).
func TestMergeSyncStateRowsKeepsEarlierPullWatermark(t *testing.T) {
	for _, tt := range []struct {
		name           string
		source, target sql.NullString
		want           sql.NullString
	}{
		{
			name:   "earlier of two present watermarks",
			source: nullString("2026-02-01 00:00:00"),
			target: nullString("2026-03-01 00:00:00"),
			want:   nullString("2026-02-01 00:00:00"),
		},
		{
			name:   "earlier of two present watermarks reversed",
			source: nullString("2026-03-01 00:00:00"),
			target: nullString("2026-02-01 00:00:00"),
			want:   nullString("2026-02-01 00:00:00"),
		},
		{
			name:   "null source resets the watermark",
			source: sql.NullString{},
			target: nullString("2026-03-01 00:00:00"),
			want:   sql.NullString{},
		},
		{
			name:   "null target keeps the reset",
			source: nullString("2026-03-01 00:00:00"),
			target: sql.NullString{},
			want:   sql.NullString{},
		},
		{
			name:   "unparseable watermark resets instead of winning",
			source: nullString("not-a-timestamp"),
			target: nullString("2026-03-01 00:00:00"),
			want:   sql.NullString{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			merged := mergeSyncStateRows(syncStateMergeRow{LastSyncAt: tt.source}, syncStateMergeRow{LastSyncAt: tt.target})
			if merged.LastSyncAt != tt.want {
				t.Fatalf("merged last_sync_at = %#v, want %#v", merged.LastSyncAt, tt.want)
			}
			// The invariant behind every row above: a merge may lower the
			// watermark or clear it, never raise it.
			if merged.LastSyncAt.Valid {
				if !tt.target.Valid {
					t.Fatalf("merged watermark %q is later than a NULL target watermark", merged.LastSyncAt.String)
				}
				mergedAt, err := parseTimeStr(merged.LastSyncAt.String)
				if err != nil {
					t.Fatalf("merged watermark %q does not parse: %v", merged.LastSyncAt.String, err)
				}
				targetAt, err := parseTimeStr(tt.target.String)
				if err != nil {
					t.Fatalf("target watermark %q does not parse: %v", tt.target.String, err)
				}
				if mergedAt.After(targetAt) {
					t.Fatalf("merged watermark %q is later than target %q", merged.LastSyncAt.String, tt.target.String)
				}
			}
		})
	}
}

// TestMergeSyncStateRowsComparesTimestampsByInstant proves the merge orders
// timestamps by real instant. timePtr writes "2006-01-02 15:04:05" while
// parseTimeStr also accepts RFC3339, so both layouts coexist in one column, and
// a raw TEXT compare lets a same-day RFC3339 value win on the 'T' > ' ' byte.
func TestMergeSyncStateRowsComparesTimestampsByInstant(t *testing.T) {
	for _, tt := range []struct {
		name           string
		source, target sql.NullString
		wantAttempt    sql.NullString
		wantSync       sql.NullString
	}{
		{
			name:        "spaced target is later than RFC3339 source",
			source:      nullString("2026-01-01T09:00:00Z"),
			target:      nullString("2026-01-01 10:00:00"),
			wantAttempt: nullString("2026-01-01 10:00:00"),
			wantSync:    nullString("2026-01-01T09:00:00Z"),
		},
		{
			name:        "spaced source is later than RFC3339 target",
			source:      nullString("2026-01-01 10:00:00"),
			target:      nullString("2026-01-01T09:00:00Z"),
			wantAttempt: nullString("2026-01-01 10:00:00"),
			wantSync:    nullString("2026-01-01T09:00:00Z"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			merged := mergeSyncStateRows(
				syncStateMergeRow{LastAttemptAt: tt.source, LastSyncAt: tt.source},
				syncStateMergeRow{LastAttemptAt: tt.target, LastSyncAt: tt.target},
			)
			if merged.LastAttemptAt != tt.wantAttempt {
				t.Fatalf("merged last_attempt_at = %#v, want %#v", merged.LastAttemptAt, tt.wantAttempt)
			}
			if merged.LastSyncAt != tt.wantSync {
				t.Fatalf("merged last_sync_at = %#v, want %#v", merged.LastSyncAt, tt.wantSync)
			}
		})
	}
}

// TestMergeSyncStateRowsAdvancesTelemetryResetsFailuresAndKeepsTargetDrain
// pins the rest of the policy so the watermark inversion cannot leak into it.
func TestMergeSyncStateRowsAdvancesTelemetryResetsFailuresAndKeepsTargetDrain(t *testing.T) {
	source := syncStateMergeRow{
		LastSyncAt:          nullString("2026-01-01 00:00:00"),
		JWTToken:            nullString("source-token"),
		JWTExpiresAt:        nullString("2026-06-01 00:00:00"),
		LastAttemptAt:       nullString("2026-05-01 00:00:00"),
		LastSuccessAt:       nullString("2026-04-01 00:00:00"),
		LastFailureAt:       nullString("2026-03-01 00:00:00"),
		ConsecutiveFailures: nullString("7"),
		BackoffUntil:        nullString("2026-09-01 00:00:00"),
		LastError:           nullString("source failure"),
		LastDrainState:      nullString("source-state"),
		LastDrainReason:     nullString("source-reason"),
		LastDrainRemaining:  nullString("11"),
	}
	target := syncStateMergeRow{
		LastSyncAt:          nullString("2026-02-01 00:00:00"),
		JWTToken:            sql.NullString{},
		JWTExpiresAt:        sql.NullString{},
		LastAttemptAt:       nullString("2026-01-15 00:00:00"),
		LastSuccessAt:       nullString("2026-04-15 00:00:00"),
		LastFailureAt:       sql.NullString{},
		ConsecutiveFailures: nullString("2"),
		BackoffUntil:        nullString("2026-08-01 00:00:00"),
		LastError:           nullString("target failure"),
		LastDrainState:      nullString("target-state"),
		LastDrainReason:     nullString("target-reason"),
		LastDrainRemaining:  nullString("3"),
	}
	merged := mergeSyncStateRows(source, target)
	for _, tt := range []struct {
		column string
		got    sql.NullString
		want   sql.NullString
	}{
		{"last_sync_at", merged.LastSyncAt, nullString("2026-01-01 00:00:00")},
		{"last_attempt_at", merged.LastAttemptAt, nullString("2026-05-01 00:00:00")},
		{"last_success_at", merged.LastSuccessAt, nullString("2026-04-15 00:00:00")},
		{"last_failure_at", merged.LastFailureAt, nullString("2026-03-01 00:00:00")},
		{"jwt_token", merged.JWTToken, nullString("source-token")},
		{"jwt_expires_at", merged.JWTExpiresAt, nullString("2026-06-01 00:00:00")},
		{"consecutive_failures", merged.ConsecutiveFailures, nullString("0")},
		{"backoff_until", merged.BackoffUntil, sql.NullString{}},
		{"last_error", merged.LastError, sql.NullString{Valid: true}},
		{"last_drain_state", merged.LastDrainState, nullString("target-state")},
		{"last_drain_reason", merged.LastDrainReason, nullString("target-reason")},
		{"last_drain_remaining", merged.LastDrainRemaining, nullString("3")},
	} {
		if tt.got != tt.want {
			t.Errorf("merged %s = %#v, want %#v", tt.column, tt.got, tt.want)
		}
	}
}

// TestCoalesceProjectSyncStateMergesDivergentSpellings drives the executor step
// directly: divergent rows must fold into one canonical row instead of aborting
// the whole migration.
func TestCoalesceProjectSyncStateMergesDivergentSpellings(t *testing.T) {
	t.Run("two divergent spellings", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		seedSyncStateRow(t, database, "Foo", "2026-03-01 00:00:00", "later-attempt", "loser-token")
		seedSyncStateRow(t, database, "foo", "2026-02-01 00:00:00", "2026-01-01 00:00:00", "")
		coalesceMigrationSyncState(t, database)
		rows := syncStateProjects(t, database)
		if len(rows) != 1 || rows[0] != "foo" {
			t.Fatalf("sync_state projects = %v, want only the canonical spelling", rows)
		}
		if got := syncStateColumn(t, database, "foo", "last_sync_at"); got != "2026-02-01 00:00:00" {
			t.Fatalf("merged last_sync_at = %q, want the earlier watermark", got)
		}
		if got := syncStateColumn(t, database, "foo", "jwt_token"); got != "loser-token" {
			t.Fatalf("merged jwt_token = %q, want the only present token", got)
		}
		if got := syncStateColumn(t, database, "foo", "consecutive_failures"); got != "0" {
			t.Fatalf("merged consecutive_failures = %q, want a reset", got)
		}
	})

	t.Run("three divergent spellings", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		seedSyncStateRow(t, database, "Foo", "2026-03-01 00:00:00", "2026-03-02 00:00:00", "")
		seedSyncStateRow(t, database, "FOO", "2026-02-01 00:00:00", "2026-02-02 00:00:00", "")
		seedSyncStateRow(t, database, "foo", "2026-04-01 00:00:00", "2026-04-02 00:00:00", "kept-token")
		coalesceMigrationSyncState(t, database)
		rows := syncStateProjects(t, database)
		if len(rows) != 1 || rows[0] != "foo" {
			t.Fatalf("sync_state projects = %v, want only the canonical spelling", rows)
		}
		if got := syncStateColumn(t, database, "foo", "last_sync_at"); got != "2026-02-01 00:00:00" {
			t.Fatalf("merged last_sync_at = %q, want the earliest watermark", got)
		}
		if got := syncStateColumn(t, database, "foo", "last_attempt_at"); got != "2026-04-02 00:00:00" {
			t.Fatalf("merged last_attempt_at = %q, want the latest attempt", got)
		}
		if got := syncStateColumn(t, database, "foo", "jwt_token"); got != "kept-token" {
			t.Fatalf("merged jwt_token = %q, want the only present token", got)
		}
	})

	t.Run("byte-identical spellings fold without merging", func(t *testing.T) {
		database := newMigrationExecutorDB(t)
		for _, project := range []string{"Foo", "foo"} {
			if _, err := database.sqlDB.Exec(`INSERT INTO sync_state (project, last_sync_at, jwt_token, consecutive_failures, last_error) VALUES (?, '2026-01-01 00:00:00', 'token', 3, 'error')`, project); err != nil {
				t.Fatal(err)
			}
		}
		coalesceMigrationSyncState(t, database)
		rows := syncStateProjects(t, database)
		if len(rows) != 1 || rows[0] != "foo" {
			t.Fatalf("sync_state projects = %v, want only the canonical spelling", rows)
		}
		if got := syncStateColumn(t, database, "foo", "consecutive_failures"); got != "3" {
			t.Fatalf("consecutive_failures = %q, want the untouched original", got)
		}
		if got := syncStateColumn(t, database, "foo", "last_error"); got != "error" {
			t.Fatalf("last_error = %q, want the untouched original", got)
		}
		if got := syncStateColumn(t, database, "foo", "last_sync_at"); got != "2026-01-01 00:00:00" {
			t.Fatalf("last_sync_at = %q, want the untouched original", got)
		}
	})
}

// TestProjectMigrationPlanCoalescesDivergentSyncState proves the planner no
// longer blocks the migration on a sync_state value divergence.
func TestProjectMigrationPlanCoalescesDivergentSyncState(t *testing.T) {
	plan := BuildProjectMigrationPlan([]ProjectStateRecord{
		{Table: ProjectStateSyncState, Project: "Foo", Identity: "canonical-project", Value: "left", StableID: "1"},
		{Table: ProjectStateSyncState, Project: "foo", Identity: "canonical-project", Value: "right", StableID: "2"},
	})
	if !plan.Executable {
		t.Fatalf("plan = %#v, want an executable plan", plan)
	}
	if len(plan.Conflicts) != 0 {
		t.Fatalf("conflicts = %#v, want none", plan.Conflicts)
	}
	if len(plan.Groups) != 1 || plan.Groups[0].Coalesced == 0 {
		t.Fatalf("groups = %#v, want one group with coalescible work", plan.Groups)
	}
}

// TestProjectMigrationExecutorMergesDivergentSyncStateEndToEnd runs the whole
// migration over the database shape that used to be unrecoverable: two
// spellings, both carrying their own sync_state row.
func TestProjectMigrationExecutorMergesDivergentSyncStateEndToEnd(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedSyncStateRow(t, database, "Foo", "2026-03-01 00:00:00", "2026-03-02 00:00:00", "left-token")
	seedSyncStateRow(t, database, "foo", "2026-02-01 00:00:00", "2026-02-02 00:00:00", "")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if !plan.Executable || len(plan.Conflicts) != 0 {
		t.Fatalf("plan = %#v, want an executable plan", plan)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	rows := syncStateProjects(t, database)
	if len(rows) != 1 || rows[0] != "foo" {
		t.Fatalf("sync_state projects = %v, want exactly the canonical spelling", rows)
	}
	if got := syncStateColumn(t, database, "foo", "last_sync_at"); got != "2026-02-01 00:00:00" {
		t.Fatalf("migrated last_sync_at = %q, want the earlier input watermark", got)
	}
	if got := syncStateColumn(t, database, "foo", "jwt_token"); got != "left-token" {
		t.Fatalf("migrated jwt_token = %q, want the only present token", got)
	}
}

func seedSyncStateRow(t *testing.T, database *DB, project, lastSyncAt, lastAttemptAt, jwtToken string) {
	t.Helper()
	if _, err := database.sqlDB.Exec(
		`INSERT INTO sync_state (project, last_sync_at, last_attempt_at, jwt_token, consecutive_failures, last_error) VALUES (?, ?, ?, ?, 4, 'stale failure')`,
		project, lastSyncAt, lastAttemptAt, jwtToken); err != nil {
		t.Fatal(err)
	}
}

func coalesceMigrationSyncState(t *testing.T, database *DB) {
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
	if err := coalesceProjectSyncState(ctx, tx, records, nil); err != nil {
		t.Fatalf("coalesceProjectSyncState() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func syncStateProjects(t *testing.T, database *DB) []string {
	t.Helper()
	rows, err := database.sqlDB.Query(`SELECT project FROM sync_state WHERE project != '__auth__' ORDER BY project`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var projects []string
	for rows.Next() {
		var project string
		if err := rows.Scan(&project); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return projects
}

func syncStateColumn(t *testing.T, database *DB, project, column string) string {
	t.Helper()
	var value sql.NullString
	if err := database.sqlDB.QueryRow(`SELECT CAST(`+column+` AS TEXT) FROM sync_state WHERE project = ?`, project).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value.String
}
