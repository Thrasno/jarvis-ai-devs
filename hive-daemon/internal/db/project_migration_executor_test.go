package db

import (
	"context"
	"errors"
	"testing"
)

func TestProjectMigrationExecutorRejectsUnsafePlansBeforeBackup(t *testing.T) {
	database := newMigrationExecutorDB(t)
	backedUp := false
	err := ExecuteProjectMigration(context.Background(), database, ProjectMigrationPlan{}, func(context.Context) error {
		backedUp = true
		return nil
	}, nil)
	if !errors.Is(err, ErrProjectMigrationPlanUnsafe) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want unsafe-plan error", err)
	}
	if backedUp {
		t.Fatal("backup ran for a non-executable plan")
	}
}

func TestProjectMigrationExecutorRollsBackFailpointAndRetries(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	backups := 0
	fail := errors.New("fail after sessions")
	err = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error {
		backups++
		return nil
	}, func() error { return fail })
	if !errors.Is(err, fail) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want failpoint", err)
	}
	if got := migrationProjectValues(t, database); got[0] != " Foo.Bar " || got[1] != " Foo.Bar " {
		t.Fatalf("rollback projects = %q, want original spelling", got)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error {
		backups++
		return nil
	}, nil); err != nil {
		t.Fatalf("retry ExecuteProjectMigration() error = %v", err)
	}
	if got := migrationProjectValues(t, database); got[0] != "foo-bar" || got[1] != "foo-bar" {
		t.Fatalf("retry projects = %q, want canonical spelling", got)
	}
	if backups != 2 {
		t.Fatalf("backups = %d, want 2 pre-mutation backups", backups)
	}
}

func TestProjectMigrationExecutorRejectsStaleFingerprint(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	err = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error {
		_, err := database.sqlDB.Exec(`UPDATE sessions SET project = 'Bar'`)
		return err
	}, nil)
	if !errors.Is(err, ErrProjectMigrationPlanStale) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want stale-plan error", err)
	}
}

func TestProjectMigrationExecutorRekeysSafeFullState(t *testing.T) {
	database := newMigrationExecutorDB(t)
	project := " Foo.Bar "
	seedMigrationProject(t, database, project)
	for _, statement := range []string{
		`INSERT INTO sync_state (project) VALUES (?)`,
		`INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES ('event', 'memory-sync', ?, 'save')`,
		`INSERT INTO mutation_receipts (request_id, operation, target_id, project, entity_sync_id, event_id, local_status, shared_status) VALUES ('request', 'save', 1, ?, 'memory-sync', 'event', 'done', 'done')`,
		`INSERT INTO user_prompts (sync_id, project, content) VALUES ('prompt', ?, 'content')`,
		`INSERT INTO passive_observations (project, content) VALUES (?, 'content')`,
		`INSERT INTO sync_attempt_logs (attempt_id, project, started_at, ended_at, outcome) VALUES ('attempt', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'success')`,
		`INSERT INTO recovery_tokens (token, reason, requested_project, candidates_json, context_hash, created_at, expires_at) VALUES ('token', 'reason', ?, '[]', 'hash', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	} {
		if _, err := database.sqlDB.Exec(statement, project); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	fail := errors.New("rollback all project state")
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, func() error { return fail }); !errors.Is(err, fail) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want failpoint", err)
	}
	for _, table := range []string{"sync_state", "memory_mutations", "mutation_receipts", "user_prompts", "passive_observations", "sync_attempt_logs"} {
		var count int
		if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE project = ?`, project).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s rollback rows = %d, want 1", table, count)
		}
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	for _, table := range []string{"sync_state", "memory_mutations", "mutation_receipts", "user_prompts", "passive_observations", "sync_attempt_logs"} {
		var count int
		if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE project = 'foo-bar'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s canonical rows = %d, want 1", table, count)
		}
	}
	var requestedProject string
	if err := database.sqlDB.QueryRow(`SELECT requested_project FROM recovery_tokens WHERE token = 'token'`).Scan(&requestedProject); err != nil {
		t.Fatal(err)
	}
	if requestedProject != "foo-bar" {
		t.Fatalf("recovery token project = %q, want canonical spelling", requestedProject)
	}
}

func newMigrationExecutorDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func seedMigrationProject(t *testing.T, database *DB, project string) {
	t.Helper()
	if _, err := database.sqlDB.Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session-sync', ?, 'dev', 'test')`, project); err != nil {
		t.Fatal(err)
	}
	if _, err := database.sqlDB.Exec(`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES ('memory-sync', ?, 'title', 'content', 's')`, project); err != nil {
		t.Fatal(err)
	}
}

func migrationProjectValues(t *testing.T, database *DB) [2]string {
	t.Helper()
	var values [2]string
	if err := database.sqlDB.QueryRow(`SELECT project FROM sessions WHERE id = 's'`).Scan(&values[0]); err != nil {
		t.Fatal(err)
	}
	if err := database.sqlDB.QueryRow(`SELECT project FROM memories WHERE sync_id = 'memory-sync'`).Scan(&values[1]); err != nil {
		t.Fatal(err)
	}
	return values
}
