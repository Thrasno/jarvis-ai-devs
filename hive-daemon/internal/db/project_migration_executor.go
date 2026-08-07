package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

var (
	ErrProjectMigrationPlanUnsafe  = errors.New("project migration plan is not executable")
	ErrProjectMigrationPlanStale   = errors.New("project migration plan changed before execution")
	ErrProjectMigrationUnsupported = errors.New("project migration contains unsupported state")
)

// ReadProjectMigrationPlan inventories every known daemon-local project-bearing
// state before passing its observations to the deterministic planner.
func ReadProjectMigrationPlan(ctx context.Context, database *DB) (ProjectMigrationPlan, error) {
	records, err := readProjectMigrationRecords(ctx, database.sqlDB)
	if err != nil {
		return ProjectMigrationPlan{}, err
	}
	return BuildProjectMigrationPlan(records), nil
}

// ExecuteProjectMigration rekeys the lossless subset of SQLite project state in
// one transaction. States with composite identity or governance semantics remain
// deliberately unsupported until their deterministic coalescing rules exist.
func ExecuteProjectMigration(ctx context.Context, database *DB, plan ProjectMigrationPlan, backup func(context.Context) error, failpoint func() error) error {
	if !plan.Executable || len(plan.Conflicts) != 0 {
		return ErrProjectMigrationPlanUnsafe
	}
	if backup == nil {
		return errors.New("project migration backup is required")
	}
	records, err := readProjectMigrationRecords(ctx, database.sqlDB)
	if err != nil {
		return err
	}
	if BuildProjectMigrationPlan(records).Fingerprint != plan.Fingerprint {
		return ErrProjectMigrationPlanStale
	}
	if err := requireSupportedProjectMigration(records); err != nil {
		return err
	}
	if err := backup(ctx); err != nil {
		return fmt.Errorf("create pre-mutation backup: %w", err)
	}
	tx, err := database.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	records, err = readProjectMigrationRecords(ctx, tx)
	if err != nil {
		return err
	}
	if BuildProjectMigrationPlan(records).Fingerprint != plan.Fingerprint {
		return ErrProjectMigrationPlanStale
	}
	// Re-key each raw spelling separately: SQLite deliberately does not derive keys.
	for _, record := range records {
		column, ok := rekeyableProjectColumns[record.Table]
		if !ok {
			continue
		}
		key := projectidentity.Canonical(record.Project).String()
		if _, err := tx.ExecContext(ctx, `UPDATE `+string(record.Table)+` SET `+column+` = ? WHERE `+column+` = ?`, key, record.Project); err != nil {
			return err
		}
	}
	if failpoint != nil {
		if err := failpoint(); err != nil {
			return err
		}
	}
	after, err := readProjectMigrationRecords(ctx, tx)
	if err != nil {
		return err
	}
	for _, record := range after {
		if projectidentity.Canonical(record.Project).String() != record.Project {
			return ErrProjectMigrationPlanUnsafe
		}
	}
	return tx.Commit()
}

func requireSupportedProjectMigration(records []ProjectStateRecord) error {
	for _, record := range records {
		if projectidentity.Canonical(record.Project).String() != record.Project {
			if _, ok := rekeyableProjectColumns[record.Table]; ok {
				continue
			}
			return fmt.Errorf("%w: %s", ErrProjectMigrationUnsupported, record.Table)
		}
	}
	return nil
}

var rekeyableProjectColumns = map[ProjectState]string{
	ProjectStateMemories:            "project",
	ProjectStateSessions:            "project",
	ProjectStateSyncState:           "project",
	ProjectStateMemoryMutations:     "project",
	ProjectStateMutationReceipts:    "project",
	ProjectStatePrompts:             "project",
	ProjectStatePassiveObservations: "project",
	ProjectStateSyncAttempts:        "project",
	ProjectStateRecoveryTokens:      "requested_project",
}

type projectMigrationQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readProjectMigrationRecords(ctx context.Context, queryer projectMigrationQuerier) ([]ProjectStateRecord, error) {
	queries := []struct {
		state ProjectState
		query string
	}{
		{ProjectStateMemories, `SELECT project, sync_id, CAST(id AS TEXT) FROM memories`},
		{ProjectStateSessions, `SELECT project, id, sync_id FROM sessions`},
		{ProjectStateSyncState, `SELECT project, project, project FROM sync_state WHERE project != '__auth__'`},
		{ProjectStateMemoryMutations, `SELECT project, event_id, CAST(sequence AS TEXT) FROM memory_mutations`},
		{ProjectStateMutationReceipts, `SELECT project, request_id, event_id FROM mutation_receipts`},
		{ProjectStateMutationCursors, `SELECT project, consumer, event_id FROM mutation_cursors`},
		{ProjectStatePullCursors, `SELECT project, consumer || ':' || channel, sync_id FROM pull_cursors`},
		{ProjectStatePrompts, `SELECT project, sync_id, CAST(id AS TEXT) FROM user_prompts`},
		{ProjectStateAliases, `SELECT source_project, source_project, target_project FROM project_aliases`},
		{ProjectStateBlocks, `SELECT project, canonical_project_key, command_id FROM project_blocks`},
		{ProjectStateQuarantineArchives, `SELECT project, canonical_project_key, command_id FROM project_quarantine_archives`},
		{ProjectStateGovernance, `SELECT project, project, merge_target FROM hive_project_governance`},
		{ProjectStateImportAliases, `SELECT source_project, source_system || ':' || source_table || ':' || source_id, hive_sync_id FROM import_source_aliases`},
		{ProjectStatePassiveObservations, `SELECT project, CAST(id AS TEXT), COALESCE(sync_id, '') FROM passive_observations`},
		{ProjectStateSyncAttempts, `SELECT project, attempt_id, attempt_id FROM sync_attempt_logs`},
		{ProjectStateRecoveryTokens, `SELECT requested_project, token, context_hash FROM recovery_tokens`},
		{ProjectStateMemoryPromptLinks, `SELECT m.project, CAST(l.memory_id AS TEXT) || ':' || CAST(l.prompt_id AS TEXT), CAST(l.memory_id AS TEXT) FROM memory_prompt_links l JOIN memories m ON m.id = l.memory_id`},
	}
	var records []ProjectStateRecord
	for _, item := range queries {
		rows, err := queryer.QueryContext(ctx, item.query)
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", item.state, err)
		}
		for rows.Next() {
			var record ProjectStateRecord
			record.Table = item.state
			if err := rows.Scan(&record.Project, &record.Identity, &record.Value); err != nil {
				_ = rows.Close()
				return nil, err
			}
			record.StableID = record.Identity
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return records, nil
}
