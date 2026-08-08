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

func TestProjectMigrationExecutorCoalescesCompositeCursors(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	for _, statement := range []string{
		`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', ' Foo.Bar ', 4, 'event-4')`,
		`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', 'foo/bar', 7, 'event-7')`,
		`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', ' Foo.Bar ', 'memories', '2026-01-01T00:00:00Z', 'pull-1')`,
		`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id) VALUES ('daemon', 'foo/bar', 'memories', '2026-01-02T00:00:00Z', 'pull-2')`,
	} {
		if _, err := database.sqlDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM mutation_cursors WHERE project = 'foo-bar' AND sequence = 7 AND event_id = 'event-7'`,
		`SELECT COUNT(*) FROM pull_cursors WHERE project = 'foo-bar' AND sync_id = 'pull-2'`,
	} {
		var count int
		if err := database.sqlDB.QueryRow(query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s count = %d, want 1", query, count)
		}
	}
}

func TestProjectMigrationExecutorRejectsAmbiguousCompositeCursor(t *testing.T) {
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
	err = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil)
	if !errors.Is(err, ErrProjectMigrationConflict) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want composite conflict", err)
	}
	var count int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM mutation_cursors WHERE project IN ('Foo', 'foo')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("cursor rows after conflict = %d, want 2", count)
	}
}

func TestProjectMigrationExecutorCoalescesGovernanceComposites(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	for _, statement := range []string{
		`INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by) VALUES (' Foo.Bar ', 'Other.Project', 'global', 'merge', '2026-01-01', 'user')`,
		`INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by) VALUES ('foo/bar', 'other/project', 'global', 'merge', '2026-01-01', 'user')`,
		`INSERT INTO project_blocks (canonical_project_key, project, command_id, ack_token, generation, blocked, blocked_at, ack_pending) VALUES (' Foo.Bar ', ' Foo.Bar ', 'command-1', 'ack-1', 1, 1, '2026-01-01', 1)`,
		`INSERT INTO project_blocks (canonical_project_key, project, command_id, ack_token, generation, blocked, blocked_at, ack_pending) VALUES ('foo/bar', 'foo/bar', 'command-2', 'ack-2', 2, 1, '2026-01-02', 0)`,
		`INSERT INTO project_quarantine_archives (canonical_project_key, project, command_id) VALUES (' Foo.Bar ', ' Foo.Bar ', 'quarantine')`,
		`INSERT INTO project_quarantine_archives (canonical_project_key, project, command_id) VALUES ('foo/bar', 'foo/bar', 'quarantine')`,
		`INSERT INTO hive_project_governance (project, archived_at, archived_by, archive_reason) VALUES (' Foo.Bar ', '2026-01-01', 'user', 'reason')`,
		`INSERT INTO hive_project_governance (project, archived_at, archived_by, archive_reason) VALUES ('foo/bar', '2026-01-01', 'user', 'reason')`,
		`INSERT INTO import_runs (id, source_system) VALUES ('run', 'engram')`,
		`INSERT INTO import_source_aliases (source_system, source_table, source_id, source_project, hive_table, hive_pk, hive_sync_id, run_id) VALUES ('engram', 'observations', 'source', ' Foo.Bar ', 'memories', '1', 'memory-sync', 'run')`,
		`INSERT INTO import_source_aliases (source_system, source_table, source_id, source_project, hive_table, hive_pk, hive_sync_id, run_id) VALUES ('engram', 'observations', 'source', 'foo/bar', 'memories', '1', 'memory-sync', 'run')`,
		`INSERT INTO user_prompts (sync_id, project, content) VALUES ('prompt', ' Foo.Bar ', 'content')`,
		`INSERT INTO memory_prompt_links (memory_id, prompt_id) VALUES (1, 1)`,
	} {
		if _, err := database.sqlDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM project_aliases WHERE source_project = 'foo-bar' AND target_project = 'other-project'`,
		`SELECT COUNT(*) FROM project_blocks WHERE canonical_project_key = 'foo-bar' AND command_id = 'command-2' AND ack_token = 'ack-2' AND generation = 2`,
		`SELECT COUNT(*) FROM project_quarantine_archives WHERE canonical_project_key = 'foo-bar' AND command_id = 'quarantine'`,
		`SELECT COUNT(*) FROM hive_project_governance WHERE project = 'foo-bar' AND archived_by = 'user'`,
		`SELECT COUNT(*) FROM import_source_aliases WHERE source_project = 'foo-bar' AND hive_sync_id = 'memory-sync'`,
		`SELECT COUNT(*) FROM memory_prompt_links`,
	} {
		var count int
		if err := database.sqlDB.QueryRow(query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s count = %d, want 1", query, count)
		}
	}
}

func TestProjectMigrationExecutorRejectsDivergentEqualGenerationBlock(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, "Foo")
	for _, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, ack_token, generation, blocked, blocked_at, ack_pending) VALUES (?, ?, ?, 'ack', 2, 1, '2026-01-01', 1)`, project, project, "command-"+project); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	err = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil)
	if !errors.Is(err, ErrProjectMigrationConflict) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want composite conflict", err)
	}
	var blocks, sessions int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM project_blocks WHERE project IN ('Foo', 'foo')`).Scan(&blocks); err != nil {
		t.Fatal(err)
	}
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE project = 'Foo'`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if blocks != 2 || sessions != 1 {
		t.Fatalf("rows after conflict = blocks:%d sessions:%d, want 2 and 1", blocks, sessions)
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
