package db

import (
	"testing"
)

// The rekey step used to issue one UPDATE per inventoried ROW. records holds one
// entry per row, so a project with N memories produced N identical statements —
// and when the spelling was ALREADY canonical every one of them matched all N
// rows and rewrote them through the FTS triggers, making a first startup on an
// already-canonical database quadratic in row count (measured: 250 rows 3.1s,
// 500 rows 12.6s, 1000 rows 51.5s, 2000 rows 3m31s).
//
// planProjectRekeys is the decision that used to be implicit in that loop: which
// (table, spelling) pairs actually need an UPDATE. A spelling that already equals
// its canonical form needs none, and a spelling shared by many rows needs exactly
// one.
func TestProjectRekeyPlanSkipsSpellingsThatAreAlreadyCanonical(t *testing.T) {
	records := []ProjectStateRecord{
		{Table: ProjectStateMemories, Project: "foo-bar"},
		{Table: ProjectStateMemories, Project: "foo-bar"},
		{Table: ProjectStateMemories, Project: "foo-bar"},
		{Table: ProjectStateSessions, Project: "foo-bar"},
	}
	if rekeys := planProjectRekeys(records); len(rekeys) != 0 {
		t.Fatalf("rekeys for an already-canonical database = %+v, want none", rekeys)
	}
}

func TestProjectRekeyPlanIssuesOneRekeyPerRelocatedSpelling(t *testing.T) {
	records := []ProjectStateRecord{
		{Table: ProjectStateMemories, Project: " Foo.Bar "},
		{Table: ProjectStateMemories, Project: " Foo.Bar "},
		{Table: ProjectStateMemories, Project: " Foo.Bar "},
		{Table: ProjectStateMemories, Project: "FOO.BAR"},
		{Table: ProjectStateSessions, Project: " Foo.Bar "},
		// Already canonical alongside the relocated spellings: still no work.
		{Table: ProjectStateSessions, Project: "foo-bar"},
		// Not a scalar-column table: the rekey loop never touched it.
		{Table: ProjectStateBlocks, Project: " Foo.Bar "},
	}

	rekeys := planProjectRekeys(records)

	want := []projectRekey{
		{Table: ProjectStateMemories, Column: "project", From: " Foo.Bar ", To: "foo-bar"},
		{Table: ProjectStateMemories, Column: "project", From: "FOO.BAR", To: "foo-bar"},
		{Table: ProjectStateSessions, Column: "project", From: " Foo.Bar ", To: "foo-bar"},
	}
	if len(rekeys) != len(want) {
		t.Fatalf("rekeys = %+v, want %+v", rekeys, want)
	}
	for i, rekey := range rekeys {
		if rekey != want[i] {
			t.Fatalf("rekeys[%d] = %+v, want %+v", i, rekey, want[i])
		}
	}
}

// recovery_tokens rekeys a differently-named column; the plan must carry it.
func TestProjectRekeyPlanCarriesEachTablesOwnProjectColumn(t *testing.T) {
	rekeys := planProjectRekeys([]ProjectStateRecord{
		{Table: ProjectStateRecoveryTokens, Project: " Foo.Bar "},
	})
	if len(rekeys) != 1 || rekeys[0].Column != "requested_project" {
		t.Fatalf("rekeys = %+v, want the recovery_tokens requested_project column", rekeys)
	}
}
