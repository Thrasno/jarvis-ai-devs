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
