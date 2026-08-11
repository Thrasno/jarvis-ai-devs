package db

import "context"

// ProjectMigrationPreflight is one read-only look at the database, carrying both
// answers a startup decision needs: the plan itself, and whether executing it
// would fold two project spellings into one.
//
// The two are separate because ExecuteProjectMigration does three separable kinds
// of work behind one entry point. Only the identity fold is a decision about the
// operator's own project names; registry population and the schema-ownership
// rebuild are idempotent maintenance that create no ambiguity. A caller that can
// only see "the migration has something to do" cannot tell those apart, and would
// have to hold a routine no-op upgrade behind a human confirmation.
type ProjectMigrationPreflight struct {
	Plan ProjectMigrationPlan
	// FoldsIdentities reports at least one inventoried row whose project
	// spelling is not its own canonical form, i.e. a rekey that collapses names
	// the operator chose.
	FoldsIdentities bool
}

// NeedsOperatorReview reports a preflight that must not be executed unattended:
// either it would fold identities, or the planner refused the plan outright.
func (p ProjectMigrationPreflight) NeedsOperatorReview() bool {
	return p.FoldsIdentities || !p.Plan.Executable || len(p.Plan.Conflicts) != 0
}

// ReadProjectMigrationPreflight inventories the database once and classifies it
// without writing. It opens no transaction and issues no statement that mutates,
// so a caller may run it on every start and still promise an untouched database.
func ReadProjectMigrationPreflight(ctx context.Context, database *DB) (ProjectMigrationPreflight, error) {
	records, err := readProjectMigrationRecords(ctx, database.sqlDB)
	if err != nil {
		return ProjectMigrationPreflight{}, err
	}
	return ProjectMigrationPreflight{
		Plan:            BuildProjectMigrationPlan(records),
		FoldsIdentities: projectMigrationNeeded(records),
	}, nil
}
