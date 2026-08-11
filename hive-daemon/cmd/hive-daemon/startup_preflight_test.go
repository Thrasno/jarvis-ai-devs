package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

// openStartupTestDB opens a database at a stable path so backups and preflights
// see the same file the daemon would.
func openStartupTestDB(t *testing.T) (string, *db.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return path, store
}

// databaseDigest checkpoints the WAL into the main database file and hashes it,
// so "did startup mutate anything?" can be answered on bytes rather than on a
// hand-picked set of rows.
func databaseDigest(t *testing.T, store *db.DB, path string) string {
	t.Helper()
	if _, err := store.RawDB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint wal: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read database: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// seedDuplicateProjectSpellings writes the same project under two spellings that
// fold to one canonical key. There is no conflict here — the executor could do
// this work unattended — which is exactly why it must not: folding two names the
// operator chose is a decision, not maintenance.
func seedDuplicateProjectSpellings(t *testing.T, store *db.DB) {
	t.Helper()
	for i, spelling := range []string{"Foo.Bar", "foo-bar"} {
		if _, err := store.RawDB().Exec(
			`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES (?, ?, ?, 'dev', 'test')`,
			"session-"+spelling, "sync-"+spelling, spelling); err != nil {
			t.Fatalf("seed spelling %d: %v", i, err)
		}
	}
}

func assertNothingWasMutated(t *testing.T, store *db.DB, path, before string) {
	t.Helper()
	if after := databaseDigest(t, store, path); after != before {
		t.Fatalf("database digest changed from %s to %s; the preflight must not write", before, after)
	}
	if created, err := governance.NewSQLiteBackupStore(path, "", store.RawDB()).List(context.Background()); err != nil || len(created) != 0 {
		t.Fatalf("backups = %v, %v; want none for a migration that never mutated", created, err)
	}
	warnings, err := store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: "active"})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings = %v, %v; want none — persisting one would itself be a mutation", warnings, err)
	}
}

func assertPendingOperatorStatus(t *testing.T, status project.MigrationStatus) {
	t.Helper()
	if status.State != project.MigrationStatePendingOperatorReview {
		t.Fatalf("state = %q, want %q", status.State, project.MigrationStatePendingOperatorReview)
	}
	if status.PlanFingerprint == "" {
		t.Fatal("plan fingerprint = empty, want the plan the operator will be shown")
	}
	if status.BackupID != "" {
		t.Fatalf("backup id = %q, want empty; nothing was backed up", status.BackupID)
	}
	if status.Continuation != project.MigrationPendingOperatorContinuation {
		t.Fatalf("continuation = %q, want %q", status.Continuation, project.MigrationPendingOperatorContinuation)
	}
	if status.Reason == "" {
		t.Fatal("reason = empty, want an explanation of what is being waited on")
	}
}

// TestStartupWithDuplicateSpellingsWaitsForTheOperatorWithoutTouchingTheDatabase
// is the core of this change: duplicate spellings used to be folded by startup on
// its own authority. Now startup only looks.
func TestStartupWithDuplicateSpellingsWaitsForTheOperatorWithoutTouchingTheDatabase(t *testing.T) {
	path, store := openStartupTestDB(t)
	seedDuplicateProjectSpellings(t, store)
	before := databaseDigest(t, store, path)

	status := runStartupMigration(context.Background(), store, path).Status()
	assertPendingOperatorStatus(t, status)
	assertNothingWasMutated(t, store, path, before)

	var spellings int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE project IN ('Foo.Bar', 'foo-bar')`).Scan(&spellings); err != nil || spellings != 2 {
		t.Fatalf("session spellings = %d, %v; want both left exactly as the operator wrote them", spellings, err)
	}
}

// TestStartupWithAConflictingPlanWaitsForTheOperatorWithoutTouchingTheDatabase
// keeps the pre-existing preflight-conflict invariant — unmutated database, no
// backup — while moving it onto the new pending state.
func TestStartupWithAConflictingPlanWaitsForTheOperatorWithoutTouchingTheDatabase(t *testing.T) {
	path, store := openStartupTestDB(t)
	seedPreflightProjectConflict(t, store)
	before := databaseDigest(t, store, path)

	assertPendingOperatorStatus(t, runStartupMigration(context.Background(), store, path).Status())
	assertNothingWasMutated(t, store, path, before)
}

// TestStartupRepeatedWithAPendingPlanStaysIdempotent covers the daemon's real
// shape: one process per client session, so this preflight runs again on every
// start. A second look must still change nothing and must describe the same plan,
// or the wizard's resolution guard would reject the fingerprint it was shown.
func TestStartupRepeatedWithAPendingPlanStaysIdempotent(t *testing.T) {
	path, store := openStartupTestDB(t)
	seedDuplicateProjectSpellings(t, store)

	first := runStartupMigration(context.Background(), store, path).Status()
	assertPendingOperatorStatus(t, first)
	before := databaseDigest(t, store, path)

	second := runStartupMigration(context.Background(), store, path).Status()
	assertPendingOperatorStatus(t, second)
	if second.PlanFingerprint != first.PlanFingerprint {
		t.Fatalf("restart fingerprint = %q, want the same plan %q", second.PlanFingerprint, first.PlanFingerprint)
	}
	assertNothingWasMutated(t, store, path, before)
}

// TestStartupNeedingOnlyTheIdentityRegistryRunsUnattended is the other half of
// the rule. Registry population is an idempotent INSERT ... ON CONFLICT DO
// NOTHING that folds no identity and creates no ambiguity, so holding it behind
// an operator confirmation would block Hive completely on a routine upgrade.
//
// The fixture registers the identity through pull_cursors: it is the only
// project-bearing table with no foreign key onto project_identities, so a
// canonical project can be missing from the registry after the ownership rebuild
// has already run.
func TestStartupNeedingOnlyTheIdentityRegistryRunsUnattended(t *testing.T) {
	path, store := openStartupTestDB(t)
	// First start settles the schema-ownership rebuild.
	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatalf("first startup = %v, want ready", err)
	}
	if _, err := store.RawDB().Exec(
		`INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id, updated_at) VALUES ('c', 'unregistered', 'memories', '2026-01-01', 'sid', '2026-01-01')`); err != nil {
		t.Fatalf("seed unregistered canonical project: %v", err)
	}

	gate := runStartupMigration(context.Background(), store, path)
	if err := gate.Check(); err != nil {
		t.Fatalf("gate = %v, want ready; registry population is maintenance, not a decision", err)
	}
	if state := gate.Status().State; state != project.MigrationStateReady {
		t.Fatalf("state = %q, want %q", state, project.MigrationStateReady)
	}
	var registered int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM project_identities WHERE project_key = 'unregistered'`).Scan(&registered); err != nil || registered != 1 {
		t.Fatalf("registry rows = %d, %v; want the canonical key populated automatically", registered, err)
	}
	if _, err := store.SaveHiveWarning(db.HiveWarningInput{Severity: "warning", Source: "test", Message: "hive is usable"}); err != nil {
		t.Fatalf("write after a ready gate: %v", err)
	}
}

// TestStartupNeedingOnlyTheOwnershipRebuildRunsUnattended locks down the third
// kind of work in the executor: rebuilding the tables that own project identity.
// The identity is pre-registered so the registry has nothing to do, isolating the
// rebuild.
func TestStartupNeedingOnlyTheOwnershipRebuildRunsUnattended(t *testing.T) {
	path, store := openStartupTestDB(t)
	if _, err := store.RawDB().Exec(
		`INSERT INTO project_identities (project_key, first_spelling, first_seen_at, first_source) VALUES ('foo', 'foo', ?, 'test')`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("pre-register identity: %v", err)
	}
	if _, err := store.RawDB().Exec(
		`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'sync', 'foo', 'dev', 'test')`); err != nil {
		t.Fatalf("seed canonical session: %v", err)
	}

	gate := runStartupMigration(context.Background(), store, path)
	if err := gate.Check(); err != nil {
		t.Fatalf("gate = %v, want ready; the ownership rebuild is maintenance", err)
	}
	if _, err := store.RawDB().Exec(`SELECT "table" FROM pragma_foreign_key_list('sessions') WHERE "table" = 'project_identities'`); err != nil {
		t.Fatalf("inspect ownership: %v", err)
	}
	var owned int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_list('sessions') WHERE "table" = 'project_identities'`).Scan(&owned); err != nil || owned == 0 {
		t.Fatalf("sessions ownership foreign keys = %d, %v; want the rebuild applied", owned, err)
	}
	if created, err := governance.NewSQLiteBackupStore(path, "", store.RawDB()).List(context.Background()); err != nil || len(created) != 1 {
		t.Fatalf("backups = %v, %v; want the maintenance run's mandatory pre-mutation backup", created, err)
	}
}

// TestStartupOnAFreshDatabaseIsReadyAndBlocksNothing keeps the everyday case
// honest: a brand-new install has no ambiguity to review.
func TestStartupOnAFreshDatabaseIsReadyAndBlocksNothing(t *testing.T) {
	path, store := openStartupTestDB(t)
	gate := runStartupMigration(context.Background(), store, path)
	if err := gate.Check(); err != nil {
		t.Fatalf("fresh startup gate = %v, want ready", err)
	}
	status := gate.Status()
	if status.State != project.MigrationStateReady || status.Reason != "" || status.Continuation != "" {
		t.Fatalf("fresh status = %+v, want a clean ready status", status)
	}
	warnings, err := store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: "active"})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings = %v, %v; want none on a fresh database", warnings, err)
	}
}

// TestStartupPendingReviewIsNotTheFailureBlock guards the distinction the HTTP
// and MCP surfaces branch on: a plan waiting for a decision must never be
// reported with the failure state or its CLI continuation.
func TestStartupPendingReviewIsNotTheFailureBlock(t *testing.T) {
	path, store := openStartupTestDB(t)
	seedDuplicateProjectSpellings(t, store)
	status := runStartupMigration(context.Background(), store, path).Status()
	if status.State == project.MigrationStateBlocked {
		t.Fatal("state = migration-blocked, want the pending state instead")
	}
	if strings.Contains(status.Continuation, "hive project identity status") {
		t.Fatalf("continuation = %q, want the TUI entry point, not the CLI status command", status.Continuation)
	}
}
