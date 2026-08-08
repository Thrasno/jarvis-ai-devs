package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

var (
	ErrProjectMigrationPlanUnsafe  = errors.New("project migration plan is not executable")
	ErrProjectMigrationPlanStale   = errors.New("project migration plan changed before execution")
	ErrProjectMigrationUnsupported = errors.New("project migration contains unsupported state")
	ErrProjectMigrationConflict    = errors.New("project migration contains an unmergeable composite row")
	ErrProjectMigrationInProgress  = errors.New("project migration is already executing")
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
	if !database.migrationMu.TryLock() {
		return ErrProjectMigrationInProgress
	}
	defer database.migrationMu.Unlock()
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
	registryNeeded, err := projectIdentityRegistryNeeded(ctx, database.sqlDB, records)
	if err != nil {
		return err
	}
	ownershipNeeded, err := projectSchemaOwnershipNeeded(ctx, database.sqlDB)
	if err != nil {
		return err
	}
	if !projectMigrationNeeded(records) && !registryNeeded && !ownershipNeeded {
		return nil
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
	if err := populateProjectIdentityRegistry(ctx, tx, records); err != nil {
		return err
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
	if ownershipNeeded {
		if err := rebuildStandaloneProjectOwnershipTables(ctx, tx); err != nil {
			return err
		}
	}
	if failpoint != nil {
		if err := failpoint(); err != nil {
			return err
		}
	}
	if err := rebuildProjectMigrationState(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func projectMigrationNeeded(records []ProjectStateRecord) bool {
	for _, record := range records {
		if projectidentity.Canonical(record.Project).String() != record.Project {
			return true
		}
	}
	return false
}

type projectIdentityQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func projectIdentityRegistryNeeded(ctx context.Context, queryer projectIdentityQuerier, records []ProjectStateRecord) (bool, error) {
	keys := canonicalProjectRegistrations(records)
	for key := range keys {
		rows, err := queryer.QueryContext(ctx, `SELECT project_key FROM project_identities WHERE project_key = ?`, key)
		if err != nil {
			return false, err
		}
		present := rows.Next()
		if err := rows.Close(); err != nil {
			return false, err
		}
		if !present {
			return true, nil
		}
	}
	return false, nil
}

func populateProjectIdentityRegistry(ctx context.Context, tx *sql.Tx, records []ProjectStateRecord) error {
	for key, record := range canonicalProjectRegistrations(records) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_identities (project_key, first_spelling, first_seen_at, first_source) VALUES (?, ?, ?, 'migration') ON CONFLICT(project_key) DO NOTHING`, key, record.Project, record.RegisteredAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("register canonical project identity: %w", err)
		}
	}
	return nil
}

func canonicalProjectRegistrations(records []ProjectStateRecord) map[string]ProjectStateRecord {
	registrations := make(map[string]ProjectStateRecord)
	for _, record := range records {
		key := projectidentity.Canonical(record.Project).String()
		prior, exists := registrations[key]
		if !exists || record.RegisteredAt.Before(prior.RegisteredAt) || (record.RegisteredAt.Equal(prior.RegisteredAt) && record.StableID < prior.StableID) {
			registrations[key] = record
		}
	}
	return registrations
}

func projectIdentityDisplay(ctx context.Context, database *sql.DB, key string) (string, error) {
	var remote, first string
	if err := database.QueryRowContext(ctx, `SELECT remote_spelling, first_spelling FROM project_identities WHERE project_key = ?`, key).Scan(&remote, &first); err != nil {
		return "", err
	}
	if remote != "" {
		return remote, nil
	}
	return first, nil
}

func rebuildProjectMigrationState(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		`REINDEX`,
		`INSERT INTO memories_fts(memories_fts) VALUES ('rebuild')`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild project migration state: %w", err)
		}
	}
	if err := validateMigrationLinks(ctx, tx); err != nil {
		return err
	}
	for _, trigger := range []string{"memories_ai", "memories_au", "memories_ad", "user_prompts_ai", "user_prompts_au", "user_prompts_ad"} {
		var name string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'trigger' AND name = ?`, trigger).Scan(&name); err != nil {
			return fmt.Errorf("%w: missing schema trigger %s", ErrProjectMigrationConflict, trigger)
		}
	}
	var foreignKeyViolations int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations); err != nil {
		return err
	}
	if foreignKeyViolations != 0 {
		return fmt.Errorf("%w: foreign key violations", ErrProjectMigrationConflict)
	}
	var integrity string
	if err := tx.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("%w: SQLite integrity %s", ErrProjectMigrationConflict, integrity)
	}
	after, err := readProjectMigrationRecords(ctx, tx)
	if err != nil {
		return err
	}
	if needed, err := projectIdentityRegistryNeeded(ctx, tx, after); err != nil {
		return err
	} else if needed {
		return fmt.Errorf("%w: missing project identity registry entry", ErrProjectMigrationConflict)
	}
	if projectMigrationNeeded(after) {
		return ErrProjectMigrationPlanUnsafe
	}
	if needed, err := projectSchemaOwnershipNeeded(ctx, tx); err != nil {
		return err
	} else if needed {
		return fmt.Errorf("%w: missing project identity ownership constraint", ErrProjectMigrationConflict)
	}
	return nil
}

func projectSchemaOwnershipNeeded(ctx context.Context, queryer projectIdentityQuerier) (bool, error) {
	for _, table := range []string{"sync_state", "memory_mutations", "mutation_receipts", "sync_attempt_logs"} {
		rows, err := queryer.QueryContext(ctx, `SELECT "table" FROM pragma_foreign_key_list('`+table+`') WHERE "table" = 'project_identities'`)
		if err != nil {
			return false, err
		}
		owned := rows.Next()
		if err := rows.Close(); err != nil {
			return false, err
		}
		if !owned {
			return true, nil
		}
	}
	return false, nil
}

func rebuildStandaloneProjectOwnershipTables(ctx context.Context, tx *sql.Tx) error {
	tables := []struct {
		name, columns, create string
		indexes               []string
	}{
		{
			name:    "sync_state",
			columns: "project,last_sync_at,jwt_token,jwt_expires_at,last_attempt_at,last_success_at,last_failure_at,consecutive_failures,backoff_until,last_error,last_drain_state,last_drain_reason,last_drain_remaining",
			create:  `CREATE TABLE sync_state (project TEXT PRIMARY KEY, project_key TEXT GENERATED ALWAYS AS (CASE WHEN project = '__auth__' THEN NULL ELSE project END) STORED REFERENCES project_identities(project_key), last_sync_at DATETIME, jwt_token TEXT, jwt_expires_at DATETIME, last_attempt_at DATETIME, last_success_at DATETIME, last_failure_at DATETIME, consecutive_failures INTEGER NOT NULL DEFAULT 0, backoff_until DATETIME, last_error TEXT NOT NULL DEFAULT '', last_drain_state TEXT, last_drain_reason TEXT, last_drain_remaining INTEGER)`,
		},
		{
			name:    "memory_mutations",
			columns: "sequence,event_id,entity_type,entity_sync_id,project,op,occurred_at,actor_id,base_updated_at,payload_json,request_id,synced_at",
			create:  `CREATE TABLE memory_mutations (sequence INTEGER PRIMARY KEY AUTOINCREMENT, event_id TEXT NOT NULL UNIQUE, entity_type TEXT NOT NULL DEFAULT 'memory', entity_sync_id TEXT NOT NULL, project TEXT NOT NULL REFERENCES project_identities(project_key), op TEXT NOT NULL, occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, actor_id TEXT NOT NULL DEFAULT '', base_updated_at DATETIME, payload_json TEXT NOT NULL DEFAULT '{}', request_id TEXT, synced_at DATETIME)`,
			indexes: []string{
				`CREATE UNIQUE INDEX idx_memory_mutations_event_id ON memory_mutations(event_id)`,
				`CREATE INDEX idx_memory_mutations_project_unsynced ON memory_mutations(project, sequence) WHERE synced_at IS NULL`,
				`CREATE INDEX idx_memory_mutations_entity ON memory_mutations(entity_type, entity_sync_id, sequence)`,
				`CREATE UNIQUE INDEX idx_memory_mutations_request_id ON memory_mutations(request_id) WHERE request_id IS NOT NULL`,
			},
		},
		{
			name:    "mutation_receipts",
			columns: "request_id,operation,target_id,project,entity_sync_id,event_id,actor_id,reason,local_status,shared_status,created_at",
			create:  `CREATE TABLE mutation_receipts (request_id TEXT PRIMARY KEY, operation TEXT NOT NULL, target_id INTEGER NOT NULL, project TEXT NOT NULL REFERENCES project_identities(project_key), entity_sync_id TEXT NOT NULL, event_id TEXT NOT NULL, actor_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', local_status TEXT NOT NULL, shared_status TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		},
		{
			name:    "sync_attempt_logs",
			columns: "attempt_id,dev_id,project,client,daemon_id,started_at,ended_at,outcome,http_status,error_code,error_message,request_id,sync_counts_json,metadata_json,delivered_at,created_at",
			create:  `CREATE TABLE sync_attempt_logs (attempt_id TEXT PRIMARY KEY, dev_id TEXT NOT NULL DEFAULT '', project TEXT NOT NULL REFERENCES project_identities(project_key), client TEXT NOT NULL DEFAULT '', daemon_id TEXT NOT NULL DEFAULT '', started_at DATETIME NOT NULL, ended_at DATETIME NOT NULL, outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')), http_status INTEGER NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '', request_id TEXT NOT NULL DEFAULT '', sync_counts_json TEXT NOT NULL DEFAULT '{}', metadata_json TEXT NOT NULL DEFAULT '{}', delivered_at DATETIME, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
			indexes: []string{
				`CREATE INDEX idx_sync_attempt_logs_pending ON sync_attempt_logs(delivered_at, started_at) WHERE delivered_at IS NULL AND dev_id != ''`,
				`CREATE INDEX idx_sync_attempt_logs_retention ON sync_attempt_logs(ended_at)`,
			},
		},
	}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE `+table.name+` RENAME TO `+table.name+`_legacy`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, table.create); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+table.name+` (`+table.columns+`) SELECT `+table.columns+` FROM `+table.name+`_legacy`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE `+table.name+`_legacy`); err != nil {
			return err
		}
		for _, index := range table.indexes {
			if _, err := tx.ExecContext(ctx, index); err != nil {
				return err
			}
		}
	}
	return nil
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
		{ProjectStateMemories, `SELECT project, sync_id, CAST(id AS TEXT), created_at FROM memories`},
		{ProjectStateSessions, `SELECT project, id, sync_id, created_at FROM sessions`},
		{ProjectStateSyncState, `SELECT project, project, project, COALESCE(last_sync_at, '') FROM sync_state WHERE project != '__auth__'`},
		{ProjectStateMemoryMutations, `SELECT project, event_id, CAST(sequence AS TEXT), occurred_at FROM memory_mutations`},
		{ProjectStateMutationReceipts, `SELECT project, request_id, event_id, created_at FROM mutation_receipts`},
		{ProjectStateMutationCursors, `SELECT project, consumer || ':' || CAST(sequence AS TEXT) || ':' || event_id, event_id, updated_at FROM mutation_cursors`},
		{ProjectStatePullCursors, `SELECT project, consumer || ':' || channel || ':' || synced_at || ':' || sync_id, sync_id, updated_at FROM pull_cursors`},
		{ProjectStatePrompts, `SELECT project, sync_id, CAST(id AS TEXT), created_at FROM user_prompts`},
		{ProjectStateAliases, `SELECT source_project, source_project, target_project, created_at FROM project_aliases`},
		{ProjectStateBlocks, `SELECT project, canonical_project_key, command_id, created_at FROM project_blocks`},
		{ProjectStateQuarantineArchives, `SELECT project, canonical_project_key, command_id, created_at FROM project_quarantine_archives`},
		{ProjectStateGovernance, `SELECT project, project, merge_target, COALESCE(archived_at, merged_at, '') FROM hive_project_governance`},
		{ProjectStateImportAliases, `SELECT source_project, source_system || ':' || source_table || ':' || source_id, hive_sync_id, created_at FROM import_source_aliases`},
		{ProjectStatePassiveObservations, `SELECT project, CAST(id AS TEXT), COALESCE(sync_id, ''), created_at FROM passive_observations`},
		{ProjectStateSyncAttempts, `SELECT project, attempt_id, attempt_id, created_at FROM sync_attempt_logs`},
		{ProjectStateRecoveryTokens, `SELECT requested_project, token, context_hash, created_at FROM recovery_tokens`},
		{ProjectStateMemoryPromptLinks, `SELECT m.project, CAST(l.memory_id AS TEXT) || ':' || CAST(l.prompt_id AS TEXT), CAST(l.memory_id AS TEXT), l.created_at FROM memory_prompt_links l JOIN memories m ON m.id = l.memory_id`},
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
			var registeredAt string
			if err := rows.Scan(&record.Project, &record.Identity, &record.Value, &registeredAt); err != nil {
				_ = rows.Close()
				return nil, err
			}
			record.StableID = record.Identity
			record.RegisteredAt = parseMigrationRegisteredAt(registeredAt)
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

func parseMigrationRegisteredAt(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
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
