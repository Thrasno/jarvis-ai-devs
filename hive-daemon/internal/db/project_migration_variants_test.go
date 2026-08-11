package db

import (
	"strconv"
	"testing"
)

func variantSpellings(variants []ProjectMigrationVariant) []string {
	spellings := make([]string, 0, len(variants))
	for _, variant := range variants {
		spellings = append(spellings, variant.Spelling)
	}
	return spellings
}

func canonicalVariants(variants []ProjectMigrationVariant) []string {
	var canonical []string
	for _, variant := range variants {
		if variant.Canonical {
			canonical = append(canonical, variant.Spelling)
		}
	}
	return canonical
}

func sessionRecord(project, identity string) ProjectStateRecord {
	return ProjectStateRecord{Table: ProjectStateSessions, Project: project, Identity: identity, StableID: identity}
}

// TestMigrationGroupCarriesTheDistinctSpellingsThatCollide is what makes the
// overview screen mean anything. "2 records" tells an operator nothing; the raw
// spellings are their own data, which is the only thing they can recognize and
// therefore the only thing they can approve.
func TestMigrationGroupCarriesTheDistinctSpellingsThatCollide(t *testing.T) {
	t.Run("two spellings", func(t *testing.T) {
		plan := BuildProjectMigrationPlan([]ProjectStateRecord{
			sessionRecord("Jarvis-Dev", "a"),
			sessionRecord("jarvis-dev", "b"),
		})
		if len(plan.Groups) != 1 {
			t.Fatalf("groups = %#v, want one", plan.Groups)
		}
		got := variantSpellings(plan.Groups[0].Variants)
		if len(got) != 2 || got[0] != "Jarvis-Dev" || got[1] != "jarvis-dev" {
			t.Fatalf("variants = %v, want both spellings sorted", got)
		}
	})

	t.Run("three spellings", func(t *testing.T) {
		plan := BuildProjectMigrationPlan([]ProjectStateRecord{
			sessionRecord("jarvis-dev", "c"),
			sessionRecord("JARVIS.DEV", "a"),
			sessionRecord("Jarvis-Dev", "b"),
		})
		got := variantSpellings(plan.Groups[0].Variants)
		want := []string{"JARVIS.DEV", "Jarvis-Dev", "jarvis-dev"}
		if len(got) != len(want) {
			t.Fatalf("variants = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("variants = %v, want %v", got, want)
			}
		}
	})

	t.Run("many records collapse to few spellings", func(t *testing.T) {
		var records []ProjectStateRecord
		for i := range 74 {
			records = append(records,
				sessionRecord("Jarvis-Dev", "upper-"+strconv.Itoa(i)),
				sessionRecord("jarvis-dev", "lower-"+strconv.Itoa(i)))
		}
		group := BuildProjectMigrationPlan(records).Groups[0]
		if group.Records != 148 {
			t.Fatalf("records = %d, want 148", group.Records)
		}
		if got := variantSpellings(group.Variants); len(got) != 2 {
			t.Fatalf("variants = %v, want the 2 distinct spellings rather than one per row", got)
		}
	})
}

// TestMigrationGroupMarksTheCanonicalVariant lets the wizard say "these become
// that one" without re-deriving canonicalization in the client. The canonical
// spelling is the one nothing rewrites, and a TUI that had to recognize it by
// comparing strings against the key would be reimplementing the rule that decides
// the fold.
func TestMigrationGroupMarksTheCanonicalVariant(t *testing.T) {
	t.Run("one variant is already canonical", func(t *testing.T) {
		group := BuildProjectMigrationPlan([]ProjectStateRecord{
			sessionRecord("Jarvis-Dev", "a"),
			sessionRecord("jarvis-dev", "b"),
		}).Groups[0]
		canonical := canonicalVariants(group.Variants)
		if len(canonical) != 1 || canonical[0] != group.Key {
			t.Fatalf("canonical variants = %v, want exactly the group key %q", canonical, group.Key)
		}
		for _, variant := range group.Variants {
			if variant.Spelling == "Jarvis-Dev" && variant.Canonical {
				t.Fatal("Jarvis-Dev is marked canonical; it is the spelling being rewritten")
			}
		}
	})

	t.Run("no variant is canonical when every spelling is rewritten", func(t *testing.T) {
		group := BuildProjectMigrationPlan([]ProjectStateRecord{
			sessionRecord("Jarvis-Dev", "a"),
			sessionRecord("JARVIS.DEV", "b"),
		}).Groups[0]
		if canonical := canonicalVariants(group.Variants); len(canonical) != 0 {
			t.Fatalf("canonical variants = %v, want none; the canonical key %q is not one of the stored spellings", canonical, group.Key)
		}
	})
}

// TestPlanIsIdenticalWhateverOrderTheRecordsArriveIn guards the fingerprint, and
// through it the operator's approval. The inventory reader's row order is not a
// contract, so a plan whose variants followed it would produce a different
// fingerprint on every read and every approval would come back stale.
func TestPlanIsIdenticalWhateverOrderTheRecordsArriveIn(t *testing.T) {
	forward := BuildProjectMigrationPlan([]ProjectStateRecord{
		sessionRecord("JARVIS.DEV", "a"),
		sessionRecord("Jarvis-Dev", "b"),
		sessionRecord("jarvis-dev", "c"),
		{Table: ProjectStateMemories, Project: "Other.One", Identity: "d", StableID: "d"},
		{Table: ProjectStateMemories, Project: "other-one", Identity: "e", StableID: "e"},
	})
	reversed := BuildProjectMigrationPlan([]ProjectStateRecord{
		{Table: ProjectStateMemories, Project: "other-one", Identity: "e", StableID: "e"},
		{Table: ProjectStateMemories, Project: "Other.One", Identity: "d", StableID: "d"},
		sessionRecord("jarvis-dev", "c"),
		sessionRecord("Jarvis-Dev", "b"),
		sessionRecord("JARVIS.DEV", "a"),
	})
	if forward.Fingerprint != reversed.Fingerprint {
		t.Fatalf("fingerprint = %q for one input order and %q for another; the approval guard must not depend on row order",
			forward.Fingerprint, reversed.Fingerprint)
	}
	if len(forward.Groups) != len(reversed.Groups) {
		t.Fatalf("groups = %d and %d", len(forward.Groups), len(reversed.Groups))
	}
	for i := range forward.Groups {
		left, right := variantSpellings(forward.Groups[i].Variants), variantSpellings(reversed.Groups[i].Variants)
		if len(left) != len(right) {
			t.Fatalf("group %d variants = %v and %v", i, left, right)
		}
		for j := range left {
			if left[j] != right[j] {
				t.Fatalf("group %d variants = %v and %v", i, left, right)
			}
		}
	}
	// Rebuilding the same input twice must also agree, or nothing above proves the
	// plan is a function of the records rather than of the run.
	if again := BuildProjectMigrationPlan([]ProjectStateRecord{
		sessionRecord("JARVIS.DEV", "a"),
		sessionRecord("Jarvis-Dev", "b"),
		sessionRecord("jarvis-dev", "c"),
		{Table: ProjectStateMemories, Project: "Other.One", Identity: "d", StableID: "d"},
		{Table: ProjectStateMemories, Project: "other-one", Identity: "e", StableID: "e"},
	}); again.Fingerprint != forward.Fingerprint {
		t.Fatalf("fingerprint = %q on a rebuild, want %q", again.Fingerprint, forward.Fingerprint)
	}
}
