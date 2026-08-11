package main

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
)

// A successful startup migration used to be completely silent: the ready gate
// carried no log and main.go logged only on gate.Check() error. Nothing reported
// what the migration did, or how long it took.
//
// That silence is what makes a slow migration undiagnosable. This runs before
// the MCP transport is served, so an operator staring at a client that has not
// come up has no way to tell a hung daemon from a working one. One line turns
// "did it hang?" into "it is working".
//
// It must go to logger.Log: this is an MCP stdio server and stdout carries
// JSON-RPC.
//
// The work startup still performs unattended is maintenance — canonical identity
// registration and the schema-ownership rebuild — so that is what this line has
// to report. The relocation counters belong to the fold, which startup no longer
// executes; TestProjectMigrationSummaryReportsEveryRelocationCounter below keeps
// them covered on the path that does.
func TestSuccessfulStartupMigrationLogsWhatItDid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedCanonicalProject(t, store)

	var logged bytes.Buffer
	previous := logger.Log.Writer()
	logger.Log.SetOutput(&logged)
	t.Cleanup(func() { logger.Log.SetOutput(previous) })

	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatalf("startup migration gate = %v", err)
	}

	line := migrationSummaryLine(t, logged.String())
	if !regexp.MustCompile(`in \d`).MatchString(line) {
		t.Errorf("summary line reports no elapsed time:\n%s", line)
	}
	if !strings.Contains(line, "rows rekeyed=0") {
		t.Errorf("summary line claims rekeyed rows for a maintenance-only run:\n%s", line)
	}
}

// TestProjectMigrationSummaryReportsEveryRelocationCounter keeps the fold's
// counters covered now that startup no longer runs the fold. The migration that
// relocates a project is executed after an operator approves it, so this drives
// the executor directly and asserts the summary that a caller then logs.
func TestProjectMigrationSummaryReportsEveryRelocationCounter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedRelocatableProject(t, store)

	plan, err := db.ReadProjectMigrationPlan(context.Background(), store)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	if err := governance.ExecuteProjectMigrationWithBackup(context.Background(), store, plan, backups); err != nil {
		t.Fatalf("execute approved migration: %v", err)
	}

	var logged bytes.Buffer
	previous := logger.Log.Writer()
	logger.Log.SetOutput(&logged)
	t.Cleanup(func() { logger.Log.SetOutput(previous) })
	logProjectMigrationSummary(store.LastProjectMigrationSummary(), time.Second)

	line := migrationSummaryLine(t, logged.String())
	for _, want := range []string{
		"rows rekeyed=4",
		"reprojects enqueued=2",
		"sessions re-queued=1",
		"prompts re-queued=1",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("summary line missing %q:\n%s", want, line)
		}
	}
}

// A migration with nothing to do must stay quiet. Every daemon start after the
// first one is this case, and one line per start about zero work is noise that
// would train an operator to ignore the line that matters.
func TestStartupMigrationWithNothingToDoStaysSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedCanonicalProject(t, store)
	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatal(err)
	}

	var logged bytes.Buffer
	previous := logger.Log.Writer()
	logger.Log.SetOutput(&logged)
	t.Cleanup(func() { logger.Log.SetOutput(previous) })

	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatalf("repeated startup migration = %v", err)
	}

	if strings.Contains(logged.String(), "project identity migration:") {
		t.Fatalf("a no-op migration logged a summary:\n%s", logged.String())
	}
}

// seedCanonicalProject writes rows whose project spelling is already canonical,
// so the startup migration owes only its unattended maintenance work and never
// needs an operator decision.
func seedCanonicalProject(t *testing.T, store *db.DB) {
	t.Helper()
	if _, err := store.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s1', 'sync-s1', 'foobar', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
}

// Three rows carrying a non-canonical spelling, two of them already known to the
// server (so they owe a reproject / a re-push) and one never synced.
func seedRelocatableProject(t *testing.T, store *db.DB) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO sessions (id, sync_id, project, dev_id, client, synced_at) VALUES ('s1', 'sync-s1', ' Foo.Bar ', 'dev', 'test', CURRENT_TIMESTAMP)`,
		`INSERT INTO memories (sync_id, project, title, content, session_id, synced_at) VALUES ('sync-m1', ' Foo.Bar ', 't', 'c', 's1', CURRENT_TIMESTAMP)`,
		`INSERT INTO memories (sync_id, project, title, content, session_id, synced_at) VALUES ('sync-m2', ' Foo.Bar ', 't', 'c', 's1', CURRENT_TIMESTAMP)`,
		`INSERT INTO user_prompts (sync_id, project, session_id, content, synced_at) VALUES ('sync-p1', ' Foo.Bar ', 's1', 'p', CURRENT_TIMESTAMP)`,
	} {
		if _, err := store.RawDB().Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func migrationSummaryLine(t *testing.T, logged string) string {
	t.Helper()
	for _, line := range strings.Split(logged, "\n") {
		if strings.Contains(line, "project identity migration:") {
			return line
		}
	}
	t.Fatalf("no migration summary line in:\n%s", logged)
	return ""
}
