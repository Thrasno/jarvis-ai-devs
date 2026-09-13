package db

import (
	"context"
	"errors"
	"strings"
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
func TestProjectMigrationRefusesRekeyWhenImmutableApplyProgressHeadExists(t *testing.T) {
	database := openTestDB(t)
	_, err := database.RawDB().Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES ('Foo.Bar', 'change', 1, 1, 1, 'digest')`)
	if err != nil {
		t.Fatal(err)
	}
	err = refuseApplyProgressRekey(database.RawDB())
	if !errors.Is(err, ErrProjectMigrationApplyProgress) {
		t.Fatalf("refuseApplyProgressRekey() error = %v, want fail-closed refusal", err)
	}
	message := err.Error()
	for _, value := range []string{"Foo.Bar", "sdd/change/apply-progress/v2", "digest"} {
		if !strings.Contains(message, value) {
			t.Fatalf("refusal = %q, want sanitized remediation value %q", message, value)
		}
	}
}

func TestProjectMigrationIgnoresOtherProjectImmutableRekeyRefusal(t *testing.T) {
	database := openTestDB(t)
	seedMigrationProject(t, database, "Foo.Bar")
	if _, err := database.RawDB().Exec(`INSERT INTO memories (sync_id, project, topic_key, category, title, content, tags, files_affected, created_by, session_id) VALUES ('other-immutable', 'Other.Project', 'sdd/change/apply-progress/v2', 'architecture', 'immutable', 'sensitive-content', '[]', '[]', 'test', 's')`); err != nil {
		t.Fatal(err)
	}
	plan, err := ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExecuteProjectMigration(context.Background(), database, plan, func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("ExecuteProjectMigration() error = %v, want only the target project considered", err)
	}
	var project string
	if err := database.RawDB().QueryRow(`SELECT project FROM sessions WHERE id = 's'`).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if project != "foo-bar" {
		t.Fatalf("target project = %q, want foo-bar", project)
	}
}

func TestProjectMigrationRefusesRekeyWhenImmutableEvidenceExistsWithoutHead(t *testing.T) {
	database := openTestDB(t)
	if _, err := database.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('immutable-session', 'immutable-session-sync', 'Foo.Bar', 'test', 'test')`); err != nil {
		t.Fatal(err)
	}
	const content = "sensitive immutable content"
	if _, err := database.RawDB().Exec(`INSERT INTO memories (sync_id, project, topic_key, category, title, content, tags, files_affected, created_by, session_id) VALUES ('immutable-evidence', 'Foo.Bar', 'sdd/change/apply-evidence/apb-00000000000000000000000000000001', 'architecture', 'immutable', ?, '[]', '[]', 'test', 'immutable-session')`, content); err != nil {
		t.Fatal(err)
	}
	err := refuseApplyProgressRekey(database.RawDB(), "foo-bar")
	var refusal *ApplyProgressRekeyRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("refuseApplyProgressRekey() error = %v, want sanitized evidence-only refusal", err)
	}
	if len(refusal.Offenders) != 1 {
		t.Fatalf("offenders = %#v, want exactly one target-project row", refusal.Offenders)
	}
	offender := refusal.Offenders[0]
	if offender.Topic != "sdd/change/apply-evidence/apb-00000000000000000000000000000001" || offender.Digest == "" || offender.DetectedSpelling != "Foo.Bar" {
		t.Fatalf("offender = %#v, want topic, digest, and detected spelling only", offender)
	}
	if strings.Contains(err.Error(), content) {
		t.Fatalf("refusal leaked immutable content: %q", err)
	}
}

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
