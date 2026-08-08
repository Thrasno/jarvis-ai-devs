package db

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
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

func TestProjectMigrationPlanRejectsDivergentCanonicalSyncStateRowsBeforeExecution(t *testing.T) {
	database := newMigrationExecutorDB(t)
	for _, statement := range []string{
		`INSERT INTO sync_state (project, jwt_token, consecutive_failures, last_error) VALUES ('Foo', 'left-token', 1, 'left-error')`,
		`INSERT INTO sync_state (project, jwt_token, consecutive_failures, last_error) VALUES ('foo', 'right-token', 2, 'right-error')`,
	} {
		if _, err := database.sqlDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if plan.Executable || len(plan.Conflicts) != 1 || plan.Conflicts[0].Kind != ConflictDivergentSyncState {
		t.Fatalf("plan = %#v, want divergent sync-state conflict", plan)
	}
	backedUp := false
	err = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error {
		backedUp = true
		return nil
	}, nil)
	if !errors.Is(err, ErrProjectMigrationPlanUnsafe) || backedUp {
		t.Fatalf("ExecuteProjectMigration() error = %v, backedUp = %v", err, backedUp)
	}
	var rows int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project IN ('Foo', 'foo')`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("sync-state rows after preflight = %d, want 2", rows)
	}
}

func TestProjectMigrationExecutorCoalescesEquivalentCanonicalSyncStateRows(t *testing.T) {
	database := newMigrationExecutorDB(t)
	for _, project := range []string{"Foo", "foo"} {
		if _, err := database.sqlDB.Exec(`INSERT INTO sync_state (project, last_sync_at, jwt_token, jwt_expires_at, last_attempt_at, last_success_at, last_failure_at, consecutive_failures, backoff_until, last_error, last_drain_state, last_drain_reason, last_drain_remaining) VALUES (?, '2026-01-01', 'token', '2026-02-01', '2026-01-02', '2026-01-03', '2026-01-04', 3, '2026-01-05', 'error', 'partial', 'remaining', 4)`, project); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.sqlDB.Exec(`INSERT INTO sync_state (project, jwt_token) VALUES ('__auth__', 'auth-token')`); err != nil {
		t.Fatal(err)
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if !plan.Executable || len(plan.Groups) != 1 || plan.Groups[0].Coalesced != 1 {
		t.Fatalf("plan = %#v, want one equivalent coalescence excluding __auth__", plan)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	var rows int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = 'foo' AND jwt_token = 'token' AND consecutive_failures = 3 AND last_error = 'error'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("canonical equivalent rows = %d, want 1", rows)
	}
	var authToken string
	if err := database.sqlDB.QueryRow(`SELECT jwt_token FROM sync_state WHERE project = '__auth__'`).Scan(&authToken); err != nil || authToken != "auth-token" {
		t.Fatalf("auth sentinel token = %q, error = %v", authToken, err)
	}
}

func TestProjectMigrationExecutorRetriesAfterExplicitSyncStateResolution(t *testing.T) {
	database := newMigrationExecutorDB(t)
	for _, statement := range []string{
		`INSERT INTO sync_state (project, jwt_token) VALUES ('Foo', 'loser')`,
		`INSERT INTO sync_state (project, jwt_token) VALUES ('foo', 'winner')`,
	} {
		if _, err := database.sqlDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	blocked, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil || blocked.Executable {
		t.Fatalf("blocked plan = %#v, error = %v", blocked, err)
	}
	if err := database.ResolveProjectIdentityConflict(context.Background(), "Foo", "foo"); err != nil {
		t.Fatalf("ResolveProjectIdentityConflict() error = %v", err)
	}
	resolved, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil || !resolved.Executable {
		t.Fatalf("resolved plan = %#v, error = %v", resolved, err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, resolved, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("retry ExecuteProjectMigration() error = %v", err)
	}
	var token string
	if err := database.sqlDB.QueryRow(`SELECT jwt_token FROM sync_state WHERE project = 'foo'`).Scan(&token); err != nil || token != "winner" {
		t.Fatalf("resolved token = %q, error = %v", token, err)
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
	var registered int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM project_identities WHERE project_key = 'foo-bar'`).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	if registered != 0 {
		t.Fatalf("registry rows after rollback = %d, want 0", registered)
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

func TestProjectMigrationExecutorRejectsConcurrentExecution(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var first error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		first = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error {
			close(entered)
			<-release
			return nil
		}, nil)
	}()
	<-entered
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); !errors.Is(err, ErrProjectMigrationInProgress) {
		t.Fatalf("concurrent ExecuteProjectMigration() error = %v, want in-progress error", err)
	}
	close(release)
	wait.Wait()
	if first != nil {
		t.Fatalf("first ExecuteProjectMigration() error = %v", first)
	}
}

func TestProjectMigrationExecutorRebuildsSQLiteStateAfterRekey(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	var integrity string
	if err := database.sqlDB.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("SQLite integrity = %q, want ok", integrity)
	}
	var foreignKeyViolations int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations); err != nil {
		t.Fatal(err)
	}
	if foreignKeyViolations != 0 {
		t.Fatalf("foreign key violations = %d, want 0", foreignKeyViolations)
	}
}

func TestProjectMigrationExecutorRollsBackWhenSchemaInvariantFails(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if _, err := database.sqlDB.Exec(`DROP TRIGGER memories_ai`); err != nil {
		t.Fatal(err)
	}
	err = ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil)
	if !errors.Is(err, ErrProjectMigrationConflict) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want schema invariant conflict", err)
	}
	if got := migrationProjectValues(t, database); got[0] != " Foo.Bar " || got[1] != " Foo.Bar " {
		t.Fatalf("schema invariant rollback projects = %q, want original spelling", got)
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

func TestProjectMigrationExecutorRegistersCanonicalIdentityAndPreservesRemoteDisplay(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	if _, err := database.sqlDB.Exec(`INSERT INTO project_identities (project_key, first_spelling, first_seen_at, first_source, remote_spelling, remote_seen_at, remote_source) VALUES ('foo-bar', 'oldest-name', '2025-01-01T00:00:00Z', 'migration', 'Remote-Name', '2025-02-01T00:00:00Z', 'git-remote')`); err != nil {
		t.Fatal(err)
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	var key, first, source, remote, remoteSource string
	if err := database.sqlDB.QueryRow(`SELECT project_key, first_spelling, first_source, remote_spelling, remote_source FROM project_identities WHERE project_key = 'foo-bar'`).Scan(&key, &first, &source, &remote, &remoteSource); err != nil {
		t.Fatal(err)
	}
	if key != "foo-bar" || first != "oldest-name" || source != "migration" || remote != "Remote-Name" || remoteSource != "git-remote" {
		t.Fatalf("registry = %q %q %q %q %q", key, first, source, remote, remoteSource)
	}
}

func TestProjectIdentityRegistryMigratesExistingDatabaseAndReopensIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedMigrationProject(t, database, " Foo.Bar ")
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = Open(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer func() { _ = database.Close() }()
	var count int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM project_identities WHERE project_key = 'foo-bar' AND first_spelling = ' Foo.Bar ' AND first_source = 'migration'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canonical registry rows = %d, want 1", count)
	}
}

func TestProjectIdentityRegistryDisplayPrefersRemoteThenOldestRegistration(t *testing.T) {
	database := newMigrationExecutorDB(t)
	if _, err := database.sqlDB.Exec(`INSERT INTO project_identities (project_key, first_spelling, first_seen_at, first_source) VALUES ('foo-bar', 'Oldest Name', '2025-01-01T00:00:00Z', 'migration')`); err != nil {
		t.Fatal(err)
	}
	if got, err := projectIdentityDisplay(context.Background(), database.sqlDB, "foo-bar"); err != nil || got != "Oldest Name" {
		t.Fatalf("projectIdentityDisplay() = %q, %v; want oldest registration", got, err)
	}
	if _, err := database.sqlDB.Exec(`UPDATE project_identities SET remote_spelling = 'Remote Name', remote_seen_at = '2025-02-01T00:00:00Z', remote_source = 'git-remote' WHERE project_key = 'foo-bar'`); err != nil {
		t.Fatal(err)
	}
	if got, err := projectIdentityDisplay(context.Background(), database.sqlDB, "foo-bar"); err != nil || got != "Remote Name" {
		t.Fatalf("projectIdentityDisplay() = %q, %v; want remote display", got, err)
	}
}

func TestProjectMigrationExecutorRebuildsStandaloneProjectOwnershipTables(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	for _, statement := range []string{
		`INSERT INTO sync_state (project, jwt_token) VALUES (' Foo.Bar ', 'project-token')`,
		`INSERT INTO sync_state (project, jwt_token) VALUES ('__auth__', 'auth-token')`,
		`INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES ('event', 'memory-sync', ' Foo.Bar ', 'save')`,
		`INSERT INTO mutation_receipts (request_id, operation, target_id, project, entity_sync_id, event_id, local_status, shared_status) VALUES ('request', 'save', 1, ' Foo.Bar ', 'memory-sync', 'event', 'done', 'done')`,
		`INSERT INTO sync_attempt_logs (attempt_id, project, started_at, ended_at, outcome) VALUES ('attempt', ' Foo.Bar ', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'success')`,
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
	for _, table := range []string{"sync_state", "memory_mutations", "mutation_receipts", "sync_attempt_logs"} {
		var foreignKeys int
		if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_list('` + table + `') WHERE "table" = 'project_identities'`).Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 {
			t.Fatalf("%s project identity foreign keys = %d, want 1", table, foreignKeys)
		}
	}
	var projectToken, authToken string
	if err := database.sqlDB.QueryRow(`SELECT jwt_token FROM sync_state WHERE project = 'foo-bar'`).Scan(&projectToken); err != nil {
		t.Fatal(err)
	}
	if err := database.sqlDB.QueryRow(`SELECT jwt_token FROM sync_state WHERE project = '__auth__'`).Scan(&authToken); err != nil {
		t.Fatal(err)
	}
	if projectToken != "project-token" || authToken != "auth-token" {
		t.Fatalf("sync tokens = %q, %q; want project and auth tokens preserved", projectToken, authToken)
	}
	for _, index := range []string{"idx_memory_mutations_project_unsynced", "idx_memory_mutations_entity", "idx_sync_attempt_logs_pending", "idx_sync_attempt_logs_retention"} {
		var count int
		if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("index %s was not recreated", index)
		}
	}
	var foreignKeyViolations int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations); err != nil {
		t.Fatal(err)
	}
	if foreignKeyViolations != 0 {
		t.Fatalf("foreign key violations = %d, want 0", foreignKeyViolations)
	}
	plan, err = ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	backups := 0
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error {
		backups++
		return nil
	}, nil); err != nil {
		t.Fatalf("idempotent ExecuteProjectMigration() error = %v", err)
	}
	if backups != 0 {
		t.Fatalf("idempotent migration created %d backups, want 0", backups)
	}
}

func TestProjectMigrationExecutorRebuildsContentProjectOwnership(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedMigrationProject(t, database, " Foo.Bar ")
	for _, statement := range []string{
		`INSERT INTO user_prompts (sync_id, project, session_id, content) VALUES ('prompt-sync', ' Foo.Bar ', 's', 'findable prompt')`,
		`INSERT INTO user_prompts (sync_id, project, content) VALUES ('legacy-prompt-sync', '', 'unscoped legacy prompt')`,
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
	fail := errors.New("content rebuild failpoint")
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, func() error { return fail }); !errors.Is(err, fail) {
		t.Fatalf("ExecuteProjectMigration() error = %v, want failpoint", err)
	}
	if got := migrationProjectValues(t, database); got != [2]string{" Foo.Bar ", " Foo.Bar "} {
		t.Fatalf("rollback projects = %q, want original spellings", got)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
	for _, table := range []string{"sessions", "memories", "user_prompts"} {
		var foreignKeys int
		if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_list('` + table + `') WHERE "table" = 'project_identities'`).Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 {
			t.Fatalf("%s project identity foreign keys = %d, want 1", table, foreignKeys)
		}
	}
	var sessionForeignKeys int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_list('memories') WHERE "table" = 'sessions'`).Scan(&sessionForeignKeys); err != nil {
		t.Fatal(err)
	}
	if sessionForeignKeys != 1 {
		t.Fatalf("memories session foreign keys = %d, want 1", sessionForeignKeys)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM memories WHERE sync_id = 'memory-sync' AND project = 'foo-bar' AND session_id = 's'`,
		`SELECT COUNT(*) FROM user_prompts WHERE sync_id = 'prompt-sync' AND project = 'foo-bar' AND session_id = 's'`,
		`SELECT COUNT(*) FROM user_prompts WHERE sync_id = 'legacy-prompt-sync' AND project = '' AND session_id = ''`,
		`SELECT COUNT(*) FROM memory_prompt_links WHERE memory_id = 1 AND prompt_id = 1`,
		`SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH 'title'`,
		`SELECT COUNT(*) FROM user_prompts_fts WHERE user_prompts_fts MATCH 'findable OR unscoped'`,
	} {
		var count int
		if err := database.sqlDB.QueryRow(query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		want := 1
		if query == `SELECT COUNT(*) FROM user_prompts_fts WHERE user_prompts_fts MATCH 'findable OR unscoped'` {
			want = 2
		}
		if count != want {
			t.Fatalf("%s count = %d, want %d", query, count, want)
		}
	}
	for _, name := range []string{"idx_memories_sync_id", "idx_sessions_sync_id", "idx_user_prompts_project_created", "memories_ai", "user_prompts_ai"} {
		var count int
		if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("schema object %s was not rebuilt", name)
		}
	}
	var foreignKeyViolations int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations); err != nil {
		t.Fatal(err)
	}
	if foreignKeyViolations != 0 {
		t.Fatalf("foreign key violations = %d, want 0", foreignKeyViolations)
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

func TestResolveProjectIdentityConflictMergesSyncStateIntoTarget(t *testing.T) {
	const columns = `(project, last_sync_at, jwt_token, jwt_expires_at, last_attempt_at, last_success_at, last_failure_at, consecutive_failures, backoff_until, last_error, last_drain_state, last_drain_reason, last_drain_remaining)`
	const source = `('Foo', '2026-01-10', 'source-token', '2026-03-01', '2026-01-11', '2026-01-10', '2026-01-30', 4, '2099-01-01', 'boom', 'partial', 'budget', 7)`
	for _, testCase := range []struct {
		name, target string
		want         string
	}{
		{
			name:   "never synced target adopts the losing cursor and credentials",
			target: `('foo', NULL, '', NULL, NULL, NULL, NULL, 0, NULL, '', NULL, NULL, NULL)`,
			want:   "2026-01-10|source-token|2026-03-01|2026-01-11|2026-01-10|2026-01-30|0|||partial|budget|7",
		},
		{
			name:   "already synced target keeps its own advanced cursor and token",
			target: `('foo', '2026-02-20', 'target-token', '2026-04-01', '2026-02-21', '2026-02-20', NULL, 1, NULL, '', 'complete', 'drained', 0)`,
			want:   "2026-02-20|target-token|2026-04-01|2026-02-21|2026-02-20|2026-01-30|0|||complete|drained|0",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			database := newMigrationExecutorDB(t)
			if _, err := database.sqlDB.Exec(`INSERT INTO sync_state ` + columns + ` VALUES ` + source + `,` + testCase.target); err != nil {
				t.Fatal(err)
			}
			if err := database.ResolveProjectIdentityConflict(context.Background(), "Foo", "foo"); err != nil {
				t.Fatalf("ResolveProjectIdentityConflict() error = %v", err)
			}
			if got := syncStateRowValues(t, database, "foo"); got != testCase.want {
				t.Fatalf("merged sync_state = %q, want %q", got, testCase.want)
			}
			var rows int
			if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sync_state`).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != 1 {
				t.Fatalf("sync_state rows = %d, want 1", rows)
			}
		})
	}
}

func syncStateRowValues(t *testing.T, database *DB, project string) string {
	t.Helper()
	columns := syncStateMergeColumnNames()
	values := make([]string, len(columns))
	scan := make([]any, len(values))
	for i := range values {
		scan[i] = &values[i]
	}
	query := `SELECT IFNULL(` + strings.Join(columns, `,''), IFNULL(`) + `,'') FROM sync_state WHERE project = ?`
	if err := database.sqlDB.QueryRow(query, project).Scan(scan...); err != nil {
		t.Fatal(err)
	}
	return strings.Join(values, "|")
}

func syncStateMergeColumnNames() []string {
	var row syncStateMergeRow
	columns := row.columns()
	names := make([]string, len(columns))
	for i, column := range columns {
		names[i] = column.name
	}
	return names
}

// TestSyncStateMergeColumnsMatchSchema keeps the merge column list the single
// source of truth: a sync_state column added without a merge policy, or a merged
// field left unbound to a column, fails here instead of silently mis-merging.
func TestSyncStateMergeColumnsMatchSchema(t *testing.T) {
	var row syncStateMergeRow
	if got, want := len(row.columns()), reflect.TypeOf(row).NumField(); got != want {
		t.Fatalf("columns() binds %d columns for %d struct fields; every merged field must be bound", got, want)
	}
	database := newMigrationExecutorDB(t)
	rows, err := database.sqlDB.Query(`SELECT name FROM pragma_table_info('sync_state')`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	// project identifies the row and project_key is generated from it, so neither is merged.
	identity := map[string]bool{"project": true, "project_key": true}
	var schema []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if !identity[name] {
			schema = append(schema, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	merged := syncStateMergeColumnNames()
	sort.Strings(schema)
	sort.Strings(merged)
	if got, want := strings.Join(merged, ","), strings.Join(schema, ","); got != want {
		t.Fatalf("merged sync_state columns = %q, want %q", got, want)
	}
}
