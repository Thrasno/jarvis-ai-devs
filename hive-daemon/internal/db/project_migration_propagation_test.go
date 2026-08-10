package db

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
)

// TestProjectMigrationEnqueuesServerPropagation pins the half of the identity
// migration the server can see. Rewriting the local rows is not enough: the push
// only ever selects rows with synced_at IS NULL, so a row the server already
// holds under the old spelling would keep that spelling forever and the same
// memory would live under two project names.
//
// Rows the server has never seen (synced_at IS NULL) must stay untouched — they
// push under the new name on their own, and stamping a from_project on them
// would assert a server-side precondition that was never true.
func TestProjectMigrationEnqueuesServerPropagation(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedPropagationProject(t, database, "Foo.Bar")

	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if !plan.Executable {
		t.Fatalf("plan = %#v, want executable", plan)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}

	assertRelocationPending(t, database, `SELECT synced_at, sync_from_project FROM sessions WHERE id = ?`, "synced-session", "Foo.Bar")
	assertRelocationAbsent(t, database, `SELECT synced_at, sync_from_project FROM sessions WHERE id = ?`, "unsynced-session")
	assertRelocationPending(t, database, `SELECT synced_at, sync_from_project FROM user_prompts WHERE sync_id = ?`, "synced-prompt", "Foo.Bar")
	assertRelocationAbsent(t, database, `SELECT synced_at, sync_from_project FROM user_prompts WHERE sync_id = ?`, "unsynced-prompt")

	mutations, err := database.GetPendingMutations("foo-bar", 100)
	if err != nil {
		t.Fatalf("GetPendingMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("pending mutations = %d, want exactly one reproject", len(mutations))
	}
	mutation := mutations[0]
	if mutation.Op != MutationOpReproject {
		t.Fatalf("mutation op = %q, want %q", mutation.Op, MutationOpReproject)
	}
	if mutation.EntitySyncID != "synced-memory" {
		t.Fatalf("mutation entity_sync_id = %q, want the memory the server already holds", mutation.EntitySyncID)
	}
	if mutation.Project != "foo-bar" {
		t.Fatalf("mutation project = %q, want the canonical target", mutation.Project)
	}
	if mutation.Reproject == nil {
		t.Fatal("mutation carries no reproject payload")
	}
	if mutation.Reproject.FromProject != "Foo.Bar" || mutation.Reproject.ToProject != "foo-bar" {
		t.Fatalf("reproject payload = %#v, want Foo.Bar -> foo-bar", *mutation.Reproject)
	}
	if mutation.Memory != nil || mutation.Tombstone != nil {
		t.Fatal("reproject mutation must not carry a memory or tombstone payload")
	}
	if mutation.EventID == "" {
		t.Fatal("reproject mutation has no event_id")
	}

	// The payload must round-trip through the journal without a memory or
	// tombstone key: hive-api rejects a reproject that carries either.
	var payloadJSON string
	if err := database.sqlDB.QueryRow(`SELECT payload_json FROM memory_mutations WHERE event_id = ?`, mutation.EventID).Scan(&payloadJSON); err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("payload_json is not an object: %v", err)
	}
	if _, ok := payload["memory"]; ok {
		t.Fatalf("payload_json = %s, want no memory key", payloadJSON)
	}
	if _, ok := payload["tombstone"]; ok {
		t.Fatalf("payload_json = %s, want no tombstone key", payloadJSON)
	}
}

// TestProjectMigrationLeavesUnseenRowsAlone covers the migration that renames
// nothing the server has seen: no relocation may be enqueued at all, because
// every pending row already carries the corrected spelling.
func TestProjectMigrationLeavesUnseenRowsAlone(t *testing.T) {
	database := newMigrationExecutorDB(t)
	if _, err := database.sqlDB.Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session-sync', 'Foo.Bar', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.sqlDB.Exec(`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES ('memory-sync', 'Foo.Bar', 'title', 'content', 's')`); err != nil {
		t.Fatal(err)
	}

	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}

	mutations, err := database.GetPendingMutations("foo-bar", 100)
	if err != nil {
		t.Fatalf("GetPendingMutations() error = %v", err)
	}
	if len(mutations) != 0 {
		t.Fatalf("pending mutations = %d, want none for rows the server never saw", len(mutations))
	}
	assertRelocationAbsent(t, database, `SELECT synced_at, sync_from_project FROM sessions WHERE id = ?`, "s")
}

// TestProjectMigrationReprojectNamesTheDaemonAsActor pins the audit trail. No
// user asked for this move, so the reproject must still name the actor that did
// it; every other insertMemoryMutation call site names a human or the importer,
// and an empty actor_id makes "who moved this memory" unanswerable.
func TestProjectMigrationReprojectNamesTheDaemonAsActor(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedPropagationProject(t, database, "Foo.Bar")
	runPropagationMigration(t, database)

	var actorID string
	if err := database.sqlDB.QueryRow(`SELECT actor_id FROM memory_mutations WHERE op = ?`, string(MutationOpReproject)).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if actorID != projectMigrationActorID {
		t.Fatalf("reproject actor_id = %q, want %q", actorID, projectMigrationActorID)
	}
}

// TestProjectMigrationLeavesPromptsWithoutSyncIDSynced pins the guard the
// memories half already has. GetUnsyncedPromptsPage selects only prompts with a
// non-empty sync_id, so clearing synced_at on a legacy prompt that predates
// sync_id assignment would strand it: permanently pending locally and still
// under the old project name on the server. Sessions have no such guard in
// ListUnsyncedSessionsPage, so they must keep relocating.
func TestProjectMigrationLeavesPromptsWithoutSyncIDSynced(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedPropagationProject(t, database, "Foo.Bar")
	if _, err := database.sqlDB.Exec(`INSERT INTO user_prompts (sync_id, project, session_id, content, synced_at) VALUES ('', 'Foo.Bar', 'synced-session', 'legacy', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	runPropagationMigration(t, database)

	var syncedAt *string
	var fromProject string
	if err := database.sqlDB.QueryRow(`SELECT synced_at, sync_from_project FROM user_prompts WHERE sync_id = '' AND content = 'legacy'`).Scan(&syncedAt, &fromProject); err != nil {
		t.Fatal(err)
	}
	if syncedAt == nil {
		t.Fatal("legacy prompt without a sync_id was flipped to unsynced; GetUnsyncedPromptsPage can never push it again")
	}
	if fromProject != "" {
		t.Fatalf("legacy prompt sync_from_project = %q, want empty for a row that can never push", fromProject)
	}
	// The rows that can push must still relocate.
	assertRelocationPending(t, database, `SELECT synced_at, sync_from_project FROM user_prompts WHERE sync_id = ?`, "synced-prompt", "Foo.Bar")
	assertRelocationPending(t, database, `SELECT synced_at, sync_from_project FROM sessions WHERE id = ?`, "synced-session", "Foo.Bar")
}

// TestRejectedReprojectIsReported pins the operator signal. A reproject carries
// no request_id, so MarkMutationsRejected finds no mutation_receipts row to
// stamp: without an explicit log a server-side rejection would mark the event
// done and print nothing, leaving the memory split under the old project name
// with the one-shot migration never running again.
func TestRejectedReprojectIsReported(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedPropagationProject(t, database, "Foo.Bar")
	runPropagationMigration(t, database)

	mutations, err := database.GetPendingMutations("foo-bar", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(mutations) != 1 {
		t.Fatalf("pending mutations = %d, want exactly one reproject", len(mutations))
	}

	var buf bytes.Buffer
	logger.Log.SetOutput(&buf)
	defer logger.Log.SetOutput(os.Stderr)

	if err := database.MarkMutationsRejected([]string{mutations[0].EventID}, time.Now()); err != nil {
		t.Fatalf("MarkMutationsRejected() error = %v", err)
	}

	logged := buf.String()
	for _, want := range []string{mutations[0].EventID, "synced-memory", "Foo.Bar", "foo-bar", string(MutationOpReproject)} {
		if !strings.Contains(logged, want) {
			t.Fatalf("rejected reproject log = %q, want it to mention %q", logged, want)
		}
	}
}

func runPropagationMigration(t *testing.T, database *DB) {
	t.Helper()
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatalf("ReadProjectMigrationPlan() error = %v", err)
	}
	if !plan.Executable {
		t.Fatalf("plan = %#v, want executable", plan)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v", err)
	}
}

// seedPropagationProject seeds one already-pushed and one never-pushed row of
// every relocatable kind under the given raw project spelling.
func seedPropagationProject(t *testing.T, database *DB, project string) {
	t.Helper()
	const synced = "2026-01-01 00:00:00"
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO sessions (id, sync_id, project, dev_id, client, synced_at) VALUES ('synced-session', 'synced-session-sync', ?, 'dev', 'test', ?)`, []any{project, synced}},
		{`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('unsynced-session', 'unsynced-session-sync', ?, 'dev', 'test')`, []any{project}},
		{`INSERT INTO memories (sync_id, project, title, content, session_id, synced_at) VALUES ('synced-memory', ?, 'title', 'content', 'synced-session', ?)`, []any{project, synced}},
		{`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES ('unsynced-memory', ?, 'title', 'content', 'synced-session')`, []any{project}},
		{`INSERT INTO user_prompts (sync_id, project, session_id, content, synced_at) VALUES ('synced-prompt', ?, 'synced-session', 'content', ?)`, []any{project, synced}},
		{`INSERT INTO user_prompts (sync_id, project, session_id, content) VALUES ('unsynced-prompt', ?, 'synced-session', 'content')`, []any{project}},
	}
	for _, statement := range statements {
		if _, err := database.sqlDB.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed %s: %v", statement.query, err)
		}
	}
}

func assertRelocationPending(t *testing.T, database *DB, query, key, wantFrom string) {
	t.Helper()
	var syncedAt *string
	var fromProject string
	if err := database.sqlDB.QueryRow(query, key).Scan(&syncedAt, &fromProject); err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	if syncedAt != nil {
		t.Fatalf("%s synced_at = %q, want NULL so the row re-pushes", key, *syncedAt)
	}
	if fromProject != wantFrom {
		t.Fatalf("%s sync_from_project = %q, want %q", key, fromProject, wantFrom)
	}
}

func assertRelocationAbsent(t *testing.T, database *DB, query, key string) {
	t.Helper()
	var syncedAt *string
	var fromProject string
	if err := database.sqlDB.QueryRow(query, key).Scan(&syncedAt, &fromProject); err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	if fromProject != "" {
		t.Fatalf("%s sync_from_project = %q, want empty for a row the server never saw", key, fromProject)
	}
}
