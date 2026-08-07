package db

import (
	"reflect"
	"testing"
	"time"
)

func TestProjectMigrationPlanInventoryAndDisplayPrecedence(t *testing.T) {
	wantTables := []ProjectState{
		ProjectStateMemories, ProjectStateSessions, ProjectStateSyncState,
		ProjectStateMemoryMutations, ProjectStateMutationReceipts, ProjectStateMutationCursors,
		ProjectStatePullCursors, ProjectStatePrompts, ProjectStateAliases,
		ProjectStateBlocks, ProjectStateQuarantineArchives, ProjectStateGovernance,
		ProjectStateImportAliases, ProjectStatePassiveObservations, ProjectStateSyncAttempts,
		ProjectStateRecoveryTokens, ProjectStateMemoryPromptLinks,
	}
	if got := ProjectKeyedStates(); !reflect.DeepEqual(got, wantTables) {
		t.Fatalf("ProjectKeyedStates() = %v, want %v", got, wantTables)
	}

	oldest := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	plan := BuildProjectMigrationPlan([]ProjectStateRecord{
		{Table: ProjectStateMemories, Project: " Foo.Bar ", Identity: "memory-1", Value: "same", RegisteredAt: oldest.Add(time.Hour), StableID: "2"},
		{Table: ProjectStateMemories, Project: "foo.bar", Identity: "memory-1", Value: "same", RegisteredAt: oldest, StableID: "1"},
		{Table: ProjectStateSessions, Project: "FOO_BAR", Identity: "session-1", Value: "same", RegisteredAt: oldest, StableID: "3", RemoteDisplay: "Foo_Bar"},
	})
	if !plan.Executable || len(plan.Groups) != 2 {
		t.Fatalf("plan = %#v, want two executable groups", plan)
	}
	if got := plan.Groups[0]; got.Key != "foo.bar" || got.Display != "foo.bar" || got.DisplaySource != DisplaySourceOldestRegistration || got.Coalesced != 1 {
		t.Fatalf("first group = %#v", got)
	}
	if got := plan.Groups[1]; got.Key != "foo_bar" || got.Display != "Foo_Bar" || got.DisplaySource != DisplaySourceRemote {
		t.Fatalf("second group = %#v", got)
	}
}

func TestProjectMigrationPlanConflictIsWholePlanAndDeterministic(t *testing.T) {
	input := []ProjectStateRecord{
		{Table: ProjectStateMemories, Project: "FOO", Identity: "memory-1", Value: "left", StableID: "2"},
		{Table: ProjectStateMemories, Project: "foo", Identity: "memory-1", Value: "right", StableID: "1"},
		{Table: ProjectStateSessions, Project: "Bar", Identity: "session-1", Value: "same", StableID: "3"},
	}
	plan := BuildProjectMigrationPlan(input)
	if plan.Executable || len(plan.Conflicts) != 1 || plan.Conflicts[0].Kind != ConflictDivergentGlobalEntity {
		t.Fatalf("plan = %#v, want a non-executable divergent-entity conflict", plan)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no partial mutation selection", plan.Actions)
	}
	if again := BuildProjectMigrationPlan([]ProjectStateRecord{input[2], input[1], input[0]}); !reflect.DeepEqual(plan, again) {
		t.Fatalf("planning is not deterministic:\nfirst=%#v\nagain=%#v", plan, again)
	}
}

func TestProjectMigrationPlanClassifiesSafetyConflicts(t *testing.T) {
	for _, tt := range []struct {
		table ProjectState
		want  MigrationConflictKind
	}{
		{ProjectStateSessions, ConflictIncompatibleSessionSentinel},
		{ProjectStateAliases, ConflictContradictoryAlias},
		{ProjectStateMutationCursors, ConflictNonMonotonicCursorProtocol},
		{ProjectStateGovernance, ConflictContradictoryGovernanceHead},
		{ProjectStateMemoryPromptLinks, ConflictBrokenReference},
	} {
		t.Run(string(tt.table), func(t *testing.T) {
			plan := BuildProjectMigrationPlan([]ProjectStateRecord{
				{Table: tt.table, Project: "Foo", Identity: "same", Value: "left"},
				{Table: tt.table, Project: "foo", Identity: "same", Value: "right"},
			})
			if len(plan.Conflicts) != 1 || plan.Conflicts[0].Kind != tt.want {
				t.Fatalf("conflicts = %#v, want %s", plan.Conflicts, tt.want)
			}
		})
	}
}
