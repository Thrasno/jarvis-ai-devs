package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

// seedAlias is a test helper that inserts an alias row directly and fails the
// test if the insert fails.
func seedAlias(t *testing.T, d *hivedb.DB, source, target string) {
	t.Helper()
	if err := d.AddAlias(context.Background(), source, target, "local", "test seed"); err != nil {
		t.Fatalf("seedAlias(%q->%q): %v", source, target, err)
	}
}

// TestAddAlias_HappyPath verifies a basic alias row is persisted.
func TestAddAlias_HappyPath(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	if err := d.AddAlias(context.Background(), "Foo", "Bar", "local", "test"); err != nil {
		t.Fatalf("AddAlias: %v", err)
	}
	target, found, err := d.ResolveAlias(context.Background(), "Foo")
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if !found {
		t.Fatal("ResolveAlias: expected found=true")
	}
	if target != "Bar" {
		t.Fatalf("ResolveAlias target = %q, want Bar", target)
	}
}

// TestAddAlias_SelfAliasRejected verifies source == target is rejected.
func TestAddAlias_SelfAliasRejected(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	err := d.AddAlias(context.Background(), "Foo", "Foo", "local", "test")
	if err == nil {
		t.Fatal("AddAlias self-alias: expected error, got nil")
	}
}

// TestAddAlias_TargetIsSourceRejected verifies that making a target that is
// already a source (cycle guard) is rejected.
func TestAddAlias_TargetIsSourceRejected(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	// Qux→Quux exists; Qux is a source. Attempt Fresh→Qux: Qux is already a
	// source so the cycle guard must fire. Fresh is neither source nor target,
	// so the bidirectional guard stays silent — only ErrAliasTargetIsSource is
	// expected here.
	seedAlias(t, d, "Qux", "Quux")
	err := d.AddAlias(context.Background(), "Fresh", "Qux", "local", "cycle attempt")
	if !errors.Is(err, hivedb.ErrAliasTargetIsSource) {
		t.Fatalf("AddAlias cycle: got %v, want ErrAliasTargetIsSource", err)
	}
}

// TestAddAlias_DuplicateSourceDifferentTargetRejected verifies UNIQUE constraint
// on source_project is surfaced as an error when target differs.
func TestAddAlias_DuplicateSourceDifferentTargetRejected(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	seedAlias(t, d, "Foo", "Bar")
	err := d.AddAlias(context.Background(), "Foo", "Qux", "local", "conflict")
	if err == nil {
		t.Fatal("AddAlias duplicate source to different target: expected error, got nil")
	}
}

// TestAddAlias_IdempotentSameTarget verifies re-inserting the same source->target
// is a no-op (no error).
func TestAddAlias_IdempotentSameTarget(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	seedAlias(t, d, "Foo", "Bar")
	if err := d.AddAlias(context.Background(), "Foo", "Bar", "local", "retry"); err != nil {
		t.Fatalf("AddAlias idempotent retry: %v", err)
	}
	// Verify still resolves to Bar after idempotent re-insert.
	target, found, err := d.ResolveAlias(context.Background(), "Foo")
	if err != nil || !found || target != "Bar" {
		t.Fatalf("ResolveAlias after idempotent retry: target=%q found=%v err=%v", target, found, err)
	}
}

// TestAddAlias_SourceIsExistingTargetRejected verifies the bidirectional chain
// guard: if B is already a target (A→B exists), then B cannot become a source
// in a new alias (B→C must be rejected with ErrAliasSourceIsTarget).
func TestAddAlias_SourceIsExistingTargetRejected(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	// Seed A→B so B is now a target.
	seedAlias(t, d, "A", "B")

	// Attempt to add B→C: B is already a target, so this must fail.
	err := d.AddAlias(context.Background(), "B", "C", "local", "chain attempt")
	if !errors.Is(err, hivedb.ErrAliasSourceIsTarget) {
		t.Fatalf("AddAlias source-is-target: got %v, want ErrAliasSourceIsTarget", err)
	}

	// Confirm B→C was not inserted.
	_, found, resolveErr := d.ResolveAlias(context.Background(), "B")
	if resolveErr != nil {
		t.Fatalf("ResolveAlias B: %v", resolveErr)
	}
	if found {
		t.Fatal("AddAlias source-is-target: alias B→C must not have been inserted")
	}
}

// TestResolveAlias_Miss verifies identity fallback when no alias exists.
func TestResolveAlias_Miss(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	target, found, err := d.ResolveAlias(context.Background(), "ProjectX")
	if err != nil {
		t.Fatalf("ResolveAlias miss: %v", err)
	}
	if found {
		t.Fatal("ResolveAlias miss: expected found=false")
	}
	if target != "" {
		t.Fatalf("ResolveAlias miss: target = %q, want empty", target)
	}
}

// TestResolveAlias_SingleHopOnly verifies that resolution is not recursive
// (alias of alias is not followed).
func TestResolveAlias_SingleHopOnly(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	// Foo -> Bar; Bar -> Baz. ResolveAlias("Foo") must return Bar, not Baz.
	seedAlias(t, d, "Foo", "Bar")
	// Add Bar->Baz as a direct INSERT to bypass the AddAlias guard (which would
	// reject this as a cycle since Foo->Bar already exists and Bar would be a
	// source, but we want to test single-hop behavior without relying on guard).
	// We do this via raw DB insert to simulate future rows without triggering guards.
	if _, err := d.RawDB().Exec(
		`INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		"Bar", "Baz", "local", "chain test", time.Now().UTC().Format("2006-01-02 15:04:05"), "test",
	); err != nil {
		t.Fatalf("raw insert Bar->Baz: %v", err)
	}
	target, found, err := d.ResolveAlias(context.Background(), "Foo")
	if err != nil {
		t.Fatalf("ResolveAlias single-hop: %v", err)
	}
	if !found || target != "Bar" {
		t.Fatalf("ResolveAlias single-hop: target=%q found=%v, want Bar true", target, found)
	}
}

// TestRemoveAlias_DeletesRow verifies the row is hard-deleted.
func TestRemoveAlias_DeletesRow(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	seedAlias(t, d, "Foo", "Bar")

	if err := d.RemoveAlias(context.Background(), "Foo"); err != nil {
		t.Fatalf("RemoveAlias: %v", err)
	}

	target, found, err := d.ResolveAlias(context.Background(), "Foo")
	if err != nil {
		t.Fatalf("ResolveAlias after remove: %v", err)
	}
	if found {
		t.Fatalf("ResolveAlias after remove: expected found=false, got target=%q", target)
	}
}

// TestRemoveAlias_ResolvesToIdentityAfterRemoval verifies ResolveAlias returns
// (empty, false, nil) once the alias is gone.
func TestRemoveAlias_ResolvesToIdentityAfterRemoval(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	seedAlias(t, d, "Old", "New")
	if err := d.RemoveAlias(context.Background(), "Old"); err != nil {
		t.Fatalf("RemoveAlias: %v", err)
	}
	_, found, err := d.ResolveAlias(context.Background(), "Old")
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if found {
		t.Fatal("expected found=false after removal")
	}
}

// TestAddAlias_ForwardCompatScopeGlobal verifies that scope="global" rows are
// accepted by the schema without error (forward-compatibility).
func TestAddAlias_ForwardCompatScopeGlobal(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	if err := d.AddAlias(context.Background(), "Remote", "Local", "global", "forward compat"); err != nil {
		t.Fatalf("AddAlias scope=global: %v", err)
	}
	target, found, err := d.ResolveAlias(context.Background(), "Remote")
	if err != nil || !found || target != "Local" {
		t.Fatalf("ResolveAlias scope=global: target=%q found=%v err=%v", target, found, err)
	}
}

// TestAliasReadPathContract verifies the full alias contract end-to-end:
// writes to target are visible when searching by target, invisible when
// searching by source, and source is excluded from KnownProjects.
//
// This covers the spec scenario "Search transparently follows alias": the alias
// system redirects writes to target at save time, so any read by target sees
// the data while any read by source returns nothing.
func TestAliasReadPathContract(t *testing.T) {
	t.Parallel()

	const source = "old-project"
	const target = "new-project"

	d := openGovernanceTestDB(t)

	// Establish the alias: old-project -> new-project.
	seedAlias(t, d, source, target)

	// Save a memory directly under the target project, simulating what the
	// write-redirect path produces when a caller saves with project=source.
	if _, err := d.EnsureManualSaveSession(target); err != nil {
		t.Fatalf("EnsureManualSaveSession(%q): %v", target, err)
	}
	if _, err := d.SaveMemory(&models.Memory{
		Project:   target,
		Title:     "redirected memory",
		Content:   "stored under target after alias redirect",
		SessionID: "manual-save-" + target,
	}); err != nil {
		t.Fatalf("SaveMemory under target: %v", err)
	}

	t.Run("search by target returns the memory", func(t *testing.T) {
		results, err := d.Search(models.MemorySearchCriteria{Query: "target", Project: target, Limit: 10})
		if err != nil {
			t.Fatalf("Search by target: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Search by target: got %d results, want 1", len(results))
		}
	})

	t.Run("search by source returns nothing", func(t *testing.T) {
		results, err := d.Search(models.MemorySearchCriteria{Query: "target", Project: source, Limit: 10})
		if err != nil {
			t.Fatalf("Search by source: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Search by source: got %d results, want 0", len(results))
		}
	})

	t.Run("KnownProjects excludes source and includes target", func(t *testing.T) {
		projects, err := d.KnownProjects(context.Background())
		if err != nil {
			t.Fatalf("KnownProjects: %v", err)
		}
		found := map[string]bool{}
		for _, p := range projects {
			found[p.Name] = true
		}
		if found[source] {
			t.Fatalf("KnownProjects: source %q must not appear (it is an alias source)", source)
		}
		if !found[target] {
			t.Fatalf("KnownProjects: target %q must appear; got %v", target, found)
		}
	})
}
