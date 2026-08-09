package db

import (
	"testing"
	"time"
)

// TestApplyRemoteReprojectMovesTheNamedRow covers the pull side of a project
// move: a teammate's daemon folded a spelling variant on a shared project, and
// this daemon has to follow the row rather than keep a second copy of the same
// memory under the old name.
func TestApplyRemoteReprojectMovesTheNamedRow(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedRemoteReprojectMemory(t, database, "old-name")

	applied, err := database.ApplyRemoteMutation(MutationEnvelope{
		EventID:      "11111111-1111-4111-8111-111111111111",
		EntityType:   "memory",
		EntitySyncID: "shared-memory",
		Project:      "new-name",
		Op:           MutationOpReproject,
		OccurredAt:   time.Now().UTC(),
		Reproject:    &MutationReprojectPayload{FromProject: "old-name", ToProject: "new-name"},
	})
	if err != nil {
		t.Fatalf("ApplyRemoteMutation() error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyRemoteMutation() applied = false, want the row moved")
	}
	if got := memoryProject(t, database, "shared-memory"); got != "new-name" {
		t.Fatalf("memory project = %q, want %q", got, "new-name")
	}
}

// TestApplyRemoteReprojectRequiresTheStoredProject is the property that makes
// the op safe to accept at all: from_project is a precondition, so a stale or
// invented source moves nothing instead of dragging some other row out of some
// other project. Matching nothing is the documented idempotent path — a replay
// after the row already moved must not be an error.
func TestApplyRemoteReprojectRequiresTheStoredProject(t *testing.T) {
	for _, tc := range []struct {
		name        string
		storedAs    string
		fromProject string
	}{
		// A replay, or a daemon that already ran its own identity migration.
		{name: "already at the target", storedAs: "new-name", fromProject: "old-name"},
		{name: "a third project", storedAs: "old-name", fromProject: "some-other-name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := newMigrationExecutorDB(t)
			seedRemoteReprojectMemory(t, database, tc.storedAs)

			applied, err := database.ApplyRemoteMutation(MutationEnvelope{
				EventID:      "22222222-2222-4222-8222-222222222222",
				EntityType:   "memory",
				EntitySyncID: "shared-memory",
				Project:      "new-name",
				Op:           MutationOpReproject,
				OccurredAt:   time.Now().UTC(),
				Reproject:    &MutationReprojectPayload{FromProject: tc.fromProject, ToProject: "new-name"},
			})
			if err != nil {
				t.Fatalf("ApplyRemoteMutation() error = %v, want a no-op rather than a failure", err)
			}
			if applied {
				t.Fatal("ApplyRemoteMutation() applied = true, want no row moved")
			}
			if got := memoryProject(t, database, "shared-memory"); got != tc.storedAs {
				t.Fatalf("memory project = %q, want the row left alone at %q", got, tc.storedAs)
			}
		})
	}
}

// TestApplyRemoteReprojectRejectsMalformedInstructions covers the instructions
// that cannot be carried out under any state of the database, as opposed to a
// precondition that simply did not hold.
func TestApplyRemoteReprojectRejectsMalformedInstructions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event MutationEnvelope
	}{
		{name: "no payload", event: MutationEnvelope{}},
		{name: "no source", event: MutationEnvelope{Reproject: &MutationReprojectPayload{ToProject: "new-name"}}},
		{name: "target disagrees with the envelope", event: MutationEnvelope{Reproject: &MutationReprojectPayload{FromProject: "old-name", ToProject: "other-name"}}},
		{name: "source equals target", event: MutationEnvelope{Reproject: &MutationReprojectPayload{FromProject: "new-name", ToProject: "new-name"}}},
		{name: "carries a memory payload", event: MutationEnvelope{
			Reproject: &MutationReprojectPayload{FromProject: "old-name", ToProject: "new-name"},
			Memory:    &MutationMemoryPayload{SyncID: "shared-memory"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := newMigrationExecutorDB(t)
			seedRemoteReprojectMemory(t, database, "old-name")

			event := tc.event
			event.EventID = "33333333-3333-4333-8333-333333333333"
			event.EntityType = "memory"
			event.EntitySyncID = "shared-memory"
			event.Project = "new-name"
			event.Op = MutationOpReproject
			event.OccurredAt = time.Now().UTC()

			if _, err := database.ApplyRemoteMutation(event); err == nil {
				t.Fatal("ApplyRemoteMutation() error = nil, want a malformed-instruction error")
			}
			if got := memoryProject(t, database, "shared-memory"); got != "old-name" {
				t.Fatalf("memory project = %q, want the row left alone at %q", got, "old-name")
			}
		})
	}
}

// TestApplyRemoteMutationSkipsUnknownOps pins the blast radius of an op this
// build does not know. Returning an error aborts the whole batch in syncer.go
// before the mutation cursor advances and before the mutations this daemon just
// pushed are acked, so one undeliverable event would permanently stop this
// daemon from receiving its teammates' work. Skipping costs only that event.
func TestApplyRemoteMutationSkipsUnknownOps(t *testing.T) {
	database := newMigrationExecutorDB(t)
	seedRemoteReprojectMemory(t, database, "old-name")

	applied, err := database.ApplyRemoteMutation(MutationEnvelope{
		EventID:      "44444444-4444-4444-8444-444444444444",
		EntityType:   "memory",
		EntitySyncID: "shared-memory",
		Project:      "old-name",
		Op:           MutationOp("some-future-op"),
		OccurredAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ApplyRemoteMutation() error = %v, want the event skipped so the batch survives", err)
	}
	if applied {
		t.Fatal("ApplyRemoteMutation() applied = true, want the unknown op reported as not applied")
	}
	// Nothing is journaled: this build did not apply it, so it must not claim it
	// did, and it must never push the event back as its own.
	var journaled int
	if err := database.sqlDB.QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, "44444444-4444-4444-8444-444444444444").Scan(&journaled); err != nil {
		t.Fatal(err)
	}
	if journaled != 0 {
		t.Fatalf("journaled rows = %d, want the skipped event unrecorded", journaled)
	}
}

func seedRemoteReprojectMemory(t *testing.T, database *DB, project string) {
	t.Helper()
	if _, err := database.sqlDB.Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session-sync', ?, 'dev', 'test')`, project); err != nil {
		t.Fatal(err)
	}
	if _, err := database.sqlDB.Exec(`INSERT INTO memories (sync_id, project, title, content, session_id, synced_at) VALUES ('shared-memory', ?, 'title', 'content', 's', '2026-01-01 00:00:00')`, project); err != nil {
		t.Fatal(err)
	}
}

func memoryProject(t *testing.T, database *DB, syncID string) string {
	t.Helper()
	var project string
	if err := database.sqlDB.QueryRow(`SELECT project FROM memories WHERE sync_id = ?`, syncID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	return project
}
