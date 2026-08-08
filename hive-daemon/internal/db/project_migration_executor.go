package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

var (
	ErrProjectMigrationPlanUnsafe  = errors.New("project migration plan is not executable")
	ErrProjectMigrationPlanStale   = errors.New("project migration plan changed before execution")
	ErrProjectMigrationUnsupported = errors.New("project migration contains unsupported state")
	ErrProjectMigrationConflict    = errors.New("project migration contains an unmergeable composite row")
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

// ExecuteProjectMigration rekeys the lossless SQLite subset in one transaction.
// Cursor and governance composites coalesce before scalar project columns move.
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
	if err := rekeyCompositeCursors(ctx, tx); err != nil {
		return err
	}
	if err := rekeyGovernanceComposites(ctx, tx); err != nil {
		return err
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
			if isCompositeMigrationState(record.Table) {
				continue
			}
			return ErrProjectMigrationPlanUnsafe
		}
	}
	return tx.Commit()
}

func requireSupportedProjectMigration(records []ProjectStateRecord) error {
	for _, record := range records {
		if projectidentity.Canonical(record.Project).String() != record.Project {
			if _, ok := rekeyableProjectColumns[record.Table]; ok || isCompositeMigrationState(record.Table) {
				continue
			}
			return fmt.Errorf("%w: %s", ErrProjectMigrationUnsupported, record.Table)
		}
	}
	return nil
}

func isCompositeMigrationState(state ProjectState) bool {
	switch state {
	case ProjectStateAliases, ProjectStateBlocks, ProjectStateQuarantineArchives, ProjectStateGovernance, ProjectStateImportAliases, ProjectStateMemoryPromptLinks:
		return true
	default:
		return false
	}
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
	ProjectStateMutationCursors:     "project",
	ProjectStatePullCursors:         "project",
}

type compositeMigrationSpec struct {
	table, columns string
	projectColumns []int
	key            func([]sql.NullString) (string, error)
	precedence     int
}

func rekeyGovernanceComposites(ctx context.Context, tx *sql.Tx) error {
	specs := []compositeMigrationSpec{
		{"project_aliases", "source_project,target_project,scope,reason,created_at,created_by,synced_at", []int{0, 1}, func(v []sql.NullString) (string, error) { return v[0].String, nil }, -1},
		{"project_blocks", "canonical_project_key,project,command_id,ack_token,reason,action,generation,blocked,blocked_at,ack_pending,ack_status,ack_warning,ack_applied_at,created_at,updated_at", []int{0, 1}, func(v []sql.NullString) (string, error) {
			if v[0].String != v[1].String {
				return "", ErrProjectMigrationConflict
			}
			return v[0].String, nil
		}, 6},
		{"project_quarantine_archives", "canonical_project_key,project,command_id,created_at", []int{0, 1}, func(v []sql.NullString) (string, error) {
			if v[0].String != v[1].String {
				return "", ErrProjectMigrationConflict
			}
			return v[0].String, nil
		}, -1},
		{"hive_project_governance", "project,archived_at,archived_by,archive_reason,merge_target,merged_at,merged_by,merge_reason", []int{0, 4}, func(v []sql.NullString) (string, error) { return v[0].String, nil }, -1},
		{"import_source_aliases", "source_system,source_table,source_id,source_project,hive_table,hive_pk,hive_sync_id,content_hash,run_id,created_at", []int{3}, func(v []sql.NullString) (string, error) {
			return strings.Join([]string{v[0].String, v[1].String, v[2].String, v[3].String}, "\x00"), nil
		}, -1},
	}
	for _, spec := range specs {
		if err := rekeyCompositeTable(ctx, tx, spec); err != nil {
			return err
		}
	}
	return validateMigrationLinks(ctx, tx)
}

func rekeyCompositeTable(ctx context.Context, tx *sql.Tx, spec compositeMigrationSpec) error {
	columns := strings.Split(spec.columns, ",")
	rows, err := tx.QueryContext(ctx, "SELECT "+spec.columns+" FROM "+spec.table)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	chosen := map[string][]sql.NullString{}
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		scan := make([]any, len(values))
		for i := range values {
			scan[i] = &values[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return err
		}
		for _, index := range spec.projectColumns {
			if values[index].Valid {
				values[index].String = projectidentity.Canonical(values[index].String).String()
			}
		}
		key, err := spec.key(values)
		if err != nil {
			return fmt.Errorf("%w: %s", err, spec.table)
		}
		if spec.table == "project_aliases" && values[0].String == values[1].String {
			return fmt.Errorf("%w: %s %s", ErrProjectMigrationConflict, spec.table, key)
		}
		prior, exists := chosen[key]
		if !exists {
			chosen[key] = values
			continue
		}
		if spec.precedence >= 0 {
			current, err := strconv.ParseInt(values[spec.precedence].String, 10, 64)
			if err != nil {
				return err
			}
			previous, err := strconv.ParseInt(prior[spec.precedence].String, 10, 64)
			if err != nil {
				return err
			}
			if current > previous {
				chosen[key] = values
				continue
			}
			if current < previous {
				continue
			}
		}
		if compositeValuesKey(values) != compositeValuesKey(prior) {
			return fmt.Errorf("%w: %s %s", ErrProjectMigrationConflict, spec.table, key)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+spec.table); err != nil {
		return err
	}
	keys := make([]string, 0, len(chosen))
	for key := range chosen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	marks := strings.TrimRight(strings.Repeat("?,", len(columns)), ",")
	for _, key := range keys {
		values := chosen[key]
		args := make([]any, len(values))
		for i, value := range values {
			if value.Valid {
				args[i] = value.String
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+spec.table+" ("+spec.columns+") VALUES ("+marks+")", args...); err != nil {
			return err
		}
	}
	return nil
}

func compositeValuesKey(values []sql.NullString) string {
	parts := make([]string, len(values))
	for i, value := range values {
		if value.Valid {
			parts[i] = "1" + value.String
		}
	}
	return strings.Join(parts, "\x00")
}

func validateMigrationLinks(ctx context.Context, tx *sql.Tx) error {
	var broken int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_prompt_links l LEFT JOIN memories m ON m.id = l.memory_id LEFT JOIN user_prompts p ON p.id = l.prompt_id WHERE m.id IS NULL OR p.id IS NULL`).Scan(&broken)
	if err != nil {
		return err
	}
	if broken != 0 {
		return fmt.Errorf("%w: memory_prompt_links", ErrProjectMigrationConflict)
	}
	return nil
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
		{ProjectStateMutationCursors, `SELECT project, consumer || ':' || CAST(sequence AS TEXT) || ':' || event_id, event_id FROM mutation_cursors`},
		{ProjectStatePullCursors, `SELECT project, consumer || ':' || channel || ':' || synced_at || ':' || sync_id, sync_id FROM pull_cursors`},
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

type migrationMutationCursor struct {
	consumer, project, eventID, updatedAt string
	sequence                              int64
}

type migrationPullCursor struct {
	consumer, project, channel, syncedAt, syncID, updatedAt string
}

func rekeyCompositeCursors(ctx context.Context, tx *sql.Tx) error {
	mutation, err := readMutationCursors(ctx, tx)
	if err != nil {
		return err
	}
	pull, err := readPullCursors(ctx, tx)
	if err != nil {
		return err
	}
	if err := replaceMutationCursors(ctx, tx, mutation); err != nil {
		return err
	}
	return replacePullCursors(ctx, tx, pull)
}

func readMutationCursors(ctx context.Context, tx *sql.Tx) (map[string]migrationMutationCursor, error) {
	rows, err := tx.QueryContext(ctx, `SELECT consumer, project, sequence, event_id, updated_at FROM mutation_cursors`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := map[string]migrationMutationCursor{}
	for rows.Next() {
		var cursor migrationMutationCursor
		if err := rows.Scan(&cursor.consumer, &cursor.project, &cursor.sequence, &cursor.eventID, &cursor.updatedAt); err != nil {
			return nil, err
		}
		cursor.project = projectidentity.Canonical(cursor.project).String()
		key := cursor.consumer + "\x00" + cursor.project
		if prior, ok := result[key]; ok {
			if prior.sequence == cursor.sequence && prior.eventID != cursor.eventID {
				return nil, fmt.Errorf("%w: mutation_cursors %s", ErrProjectMigrationConflict, key)
			}
			if prior.sequence >= cursor.sequence {
				continue
			}
		}
		result[key] = cursor
	}
	return result, rows.Err()
}

func replaceMutationCursors(ctx context.Context, tx *sql.Tx, cursors map[string]migrationMutationCursor) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM mutation_cursors`); err != nil {
		return err
	}
	for _, cursor := range cursors {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mutation_cursors (consumer, project, sequence, event_id, updated_at) VALUES (?, ?, ?, ?, ?)`, cursor.consumer, cursor.project, cursor.sequence, cursor.eventID, cursor.updatedAt); err != nil {
			return err
		}
	}
	return nil
}

func readPullCursors(ctx context.Context, tx *sql.Tx) (map[string]migrationPullCursor, error) {
	rows, err := tx.QueryContext(ctx, `SELECT consumer, project, channel, synced_at, sync_id, updated_at FROM pull_cursors`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := map[string]migrationPullCursor{}
	for rows.Next() {
		var cursor migrationPullCursor
		if err := rows.Scan(&cursor.consumer, &cursor.project, &cursor.channel, &cursor.syncedAt, &cursor.syncID, &cursor.updatedAt); err != nil {
			return nil, err
		}
		cursor.project = projectidentity.Canonical(cursor.project).String()
		key := cursor.consumer + "\x00" + cursor.project + "\x00" + cursor.channel
		if prior, ok := result[key]; ok {
			if prior.syncedAt == cursor.syncedAt && prior.syncID != cursor.syncID {
				return nil, fmt.Errorf("%w: pull_cursors %s", ErrProjectMigrationConflict, key)
			}
			if prior.syncedAt >= cursor.syncedAt {
				continue
			}
		}
		result[key] = cursor
	}
	return result, rows.Err()
}

func replacePullCursors(ctx context.Context, tx *sql.Tx, cursors map[string]migrationPullCursor) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM pull_cursors`); err != nil {
		return err
	}
	for _, cursor := range cursors {
		if _, err := tx.ExecContext(ctx, `INSERT INTO pull_cursors (consumer, project, channel, synced_at, sync_id, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, cursor.consumer, cursor.project, cursor.channel, cursor.syncedAt, cursor.syncID, cursor.updatedAt); err != nil {
			return err
		}
	}
	return nil
}
