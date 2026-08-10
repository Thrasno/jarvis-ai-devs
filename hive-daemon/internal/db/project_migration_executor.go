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
	"github.com/google/uuid"
	"modernc.org/sqlite"
)

var (
	ErrProjectMigrationPlanUnsafe     = errors.New("project migration plan is not executable")
	ErrProjectMigrationPlanStale      = errors.New("project migration plan changed before execution")
	ErrProjectMigrationUnsupported    = errors.New("project migration contains unsupported state")
	ErrProjectMigrationConflict       = errors.New("project migration contains an unmergeable composite row")
	ErrProjectMigrationInProgress     = errors.New("project migration is already executing")
	ErrProjectIdentityResolutionStale = errors.New("project identity resolution is stale or unrelated")
)

// projectMigrationActorID attributes a reproject to the daemon itself. Every
// other insertMemoryMutation call site names a human (detectUsername) or the
// importer (importActorID); no user asked for this move, so the audit trail
// names the actor that did make it, using the same spelling the daemon already
// records for its own governance actions.
const projectMigrationActorID = "hive-daemon"

// sqliteBusyCode is the primary result code SQLITE_BUSY. Extended result codes
// are enabled on these connections, so every busy variant is matched by masking
// the extended bits off.
const sqliteBusyCode = 5

// IsProjectMigrationContention reports a migration failure caused by another
// writer rather than by the state of this database. hive-daemon runs one process
// per MCP client session, so a second daemon can meet the first one's startup
// migration: it either waits out the lock and finds the plan already applied, or
// exhausts its busy timeout. Neither outcome means this database is broken, so
// callers may retry instead of gating the session off permanently.
func IsProjectMigrationContention(err error) bool {
	if errors.Is(err, ErrProjectMigrationPlanStale) || errors.Is(err, ErrProjectMigrationInProgress) {
		return true
	}
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqliteBusyCode
}

// syncStateMergeRow carries the sync_state columns that must survive when two
// canonically-equivalent rows collapse into one. Every column is addressed by
// name so a schema change cannot silently apply one column's policy to another.
type syncStateMergeRow struct {
	LastSyncAt          sql.NullString
	JWTToken            sql.NullString
	JWTExpiresAt        sql.NullString
	LastAttemptAt       sql.NullString
	LastSuccessAt       sql.NullString
	LastFailureAt       sql.NullString
	ConsecutiveFailures sql.NullString
	BackoffUntil        sql.NullString
	LastError           sql.NullString
	LastDrainState      sql.NullString
	LastDrainReason     sql.NullString
	LastDrainRemaining  sql.NullString
}

type syncStateMergeColumn struct {
	name  string
	value *sql.NullString
}

// columns binds every merged field to its sync_state column. It is the single
// source of truth for the SELECT, for the UPDATE ... SET, and for the merge
// policy: an unlisted column is never read nor written, and
// TestSyncStateMergeColumnsMatchSchema fails when this list drifts from either
// the sync_state schema or the struct.
func (row *syncStateMergeRow) columns() []syncStateMergeColumn {
	return []syncStateMergeColumn{
		{"last_sync_at", &row.LastSyncAt},
		{"jwt_token", &row.JWTToken},
		{"jwt_expires_at", &row.JWTExpiresAt},
		{"last_attempt_at", &row.LastAttemptAt},
		{"last_success_at", &row.LastSuccessAt},
		{"last_failure_at", &row.LastFailureAt},
		{"consecutive_failures", &row.ConsecutiveFailures},
		{"backoff_until", &row.BackoffUntil},
		{"last_error", &row.LastError},
		{"last_drain_state", &row.LastDrainState},
		{"last_drain_reason", &row.LastDrainReason},
		{"last_drain_remaining", &row.LastDrainRemaining},
	}
}

// ReadProjectMigrationPlan inventories every known daemon-local project-bearing
// state before passing its observations to the deterministic planner.
func ReadProjectMigrationPlan(ctx context.Context, database *DB) (ProjectMigrationPlan, error) {
	records, err := readProjectMigrationRecords(ctx, database.sqlDB)
	if err != nil {
		return ProjectMigrationPlan{}, err
	}
	return BuildProjectMigrationPlan(records), nil
}

// ResolveProjectIdentityConflict records the explicit winner for a singleton
// sync-state collision before the complete migration is replanned.
func (d *DB) ResolveProjectIdentityConflict(ctx context.Context, source, target string) error {
	source, target = strings.TrimSpace(source), strings.TrimSpace(target)
	if source == "" || target == "" || source == target || canonicalProjectKey(source) != canonicalProjectKey(target) {
		return ErrProjectIdentityResolutionStale
	}
	var sourceCount, targetCount int
	if err := d.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_state WHERE project = ?`, source).Scan(&sourceCount); err != nil {
		return err
	}
	if err := d.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_state WHERE project = ?`, target).Scan(&targetCount); err != nil {
		return err
	}
	if sourceCount != 1 || targetCount != 1 {
		return ErrProjectIdentityResolutionStale
	}
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := mergeSyncStateInto(ctx, tx, source, target); err != nil {
		return err
	}
	key, err := registerProjectIdentity(ctx, tx, target)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE project_identities SET remote_spelling = ? WHERE project_key = ?`, target, key); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeSyncStateInto(ctx context.Context, tx *sql.Tx, source, target string) error {
	sourceRow, err := readSyncStateRow(ctx, tx, source)
	if err != nil {
		return err
	}
	targetRow, err := readSyncStateRow(ctx, tx, target)
	if err != nil {
		return err
	}
	merged := mergeSyncStateRows(sourceRow, targetRow)
	columns := merged.columns()
	assignments := make([]string, len(columns))
	args := make([]any, len(columns)+1)
	for i, column := range columns {
		assignments[i] = column.name + " = ?"
		if column.value.Valid {
			args[i] = column.value.String
		}
	}
	args[len(columns)] = target
	if _, err := tx.ExecContext(ctx, `UPDATE sync_state SET `+strings.Join(assignments, ",")+` WHERE project = ?`, args...); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM sync_state WHERE project = ?`, source)
	return err
}

func readSyncStateRow(ctx context.Context, tx *sql.Tx, project string) (syncStateMergeRow, error) {
	var row syncStateMergeRow
	columns := row.columns()
	reads := make([]string, len(columns))
	scan := make([]any, len(columns))
	for i, column := range columns {
		// CAST to TEXT: DATETIME columns would otherwise round-trip through the driver's time layout.
		reads[i] = `CAST(` + column.name + ` AS TEXT)`
		scan[i] = column.value
	}
	if err := tx.QueryRowContext(ctx, `SELECT `+strings.Join(reads, ",")+` FROM sync_state WHERE project = ?`, project).Scan(scan...); err != nil {
		return syncStateMergeRow{}, err
	}
	return row, nil
}

// mergeSyncStateRows keeps the more valuable value per column: cursor timestamps only
// advance, credentials move as a token/expiry pair, and drain telemetry follows the
// surviving cursor. The failure triple resets, because a stale backoff_until inherited
// from a spelling that no longer exists would wedge the identity just repaired.
func mergeSyncStateRows(source, target syncStateMergeRow) syncStateMergeRow {
	merged := target
	sourceOwnsCursor := advancesSyncValue(source.LastSyncAt, target.LastSyncAt)
	keepLaterSyncValue(&merged.LastSyncAt, source.LastSyncAt)
	keepLaterSyncValue(&merged.LastAttemptAt, source.LastAttemptAt)
	keepLaterSyncValue(&merged.LastSuccessAt, source.LastSuccessAt)
	keepLaterSyncValue(&merged.LastFailureAt, source.LastFailureAt)
	// Token and expiry move together: a token must never inherit a foreign expiry.
	if target.JWTToken.String == "" {
		merged.JWTToken, merged.JWTExpiresAt = source.JWTToken, source.JWTExpiresAt
	}
	merged.ConsecutiveFailures = sql.NullString{String: "0", Valid: true}
	merged.BackoffUntil = sql.NullString{}
	merged.LastError = sql.NullString{Valid: true}
	// Drain telemetry is only meaningful next to the cursor that produced it.
	if sourceOwnsCursor {
		merged.LastDrainState, merged.LastDrainReason, merged.LastDrainRemaining = source.LastDrainState, source.LastDrainReason, source.LastDrainRemaining
	}
	return merged
}

// keepLaterSyncValue advances a merged timestamp only when the candidate is
// later; any real value beats NULL.
func keepLaterSyncValue(current *sql.NullString, candidate sql.NullString) {
	if advancesSyncValue(candidate, *current) {
		*current = candidate
	}
}

func advancesSyncValue(candidate, current sql.NullString) bool {
	return candidate.Valid && (!current.Valid || candidate.String > current.String)
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
	database.setProjectMigrationSummary(ProjectMigrationSummary{})
	if !projectMigrationNeeded(records) && !registryNeeded && !ownershipNeeded {
		return nil
	}
	summary := ProjectMigrationSummary{Ran: true}
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
	if err := coalesceEquivalentSyncState(ctx, tx, records); err != nil {
		return err
	}
	// Propagation is enqueued BEFORE the rekey, while each row still carries the
	// raw spelling that identifies which old literal the server holds for it.
	// After the rekey two folded spellings ("Foo.Bar" and "foo.bar") are
	// indistinguishable, and from_project would be a guess.
	if err := enqueueProjectRelocationPropagation(ctx, tx, records, &summary); err != nil {
		return err
	}
	// Re-key each raw spelling separately: SQLite deliberately does not derive keys.
	for _, rekey := range planProjectRekeys(records) {
		result, err := tx.ExecContext(ctx, `UPDATE `+string(rekey.Table)+` SET `+rekey.Column+` = ? WHERE `+rekey.Column+` = ?`, rekey.To, rekey.From)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err == nil {
			summary.RowsRekeyed += affected
		}
	}
	if ownershipNeeded {
		if err := rebuildStandaloneProjectOwnershipTables(ctx, tx); err != nil {
			return err
		}
		if err := rebuildContentProjectOwnershipTables(ctx, tx); err != nil {
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
	if err := tx.Commit(); err != nil {
		return err
	}
	// Only after the commit: a summary describing a rolled-back transaction
	// would report work that did not survive.
	database.setProjectMigrationSummary(summary)
	return nil
}

// enqueueProjectRelocationPropagation tells the server about the rename the
// local rekey is about to perform.
//
// The rekey rewrites this daemon's own rows, but every push selects only rows
// with synced_at IS NULL. Without this step each row the server already holds
// would keep the old spelling there forever and the same memory would live
// under two project names — the exact split the migration exists to end.
//
// Two kinds of rows, two mechanisms:
//
//   - Sessions and prompts re-push. Clearing synced_at puts them back into the
//     push selection; sync_from_project carries the literal the server still
//     holds, so the server relocates that exact row and nothing else.
//   - Memories do not re-push — an ordinary upsert cannot move the project
//     column. They get a reproject mutation, the only op that can.
//
// Rows with synced_at IS NULL are deliberately left alone: the server has never
// seen them, they push under the new name on their own, and stamping a
// from_project on them would assert a server-side precondition that was never
// true.
//
// This runs inside the migration's single transaction on purpose. Enqueuing
// after the commit would need its own durable pending marker, its own
// idempotency and its own crash recovery — everything the transaction already
// guarantees. These statements take no new lock, do no I/O outside the
// transaction, and add no abort path the rekey did not already have.
//
// records holds one entry per ROW, so each (table, spelling) pair is collapsed
// first: the rekey UPDATE is idempotent under repetition, but inserting the same
// reproject mutation once per memory row is not.
func enqueueProjectRelocationPropagation(ctx context.Context, tx *sql.Tx, records []ProjectStateRecord, summary *ProjectMigrationSummary) error {
	type relocation struct {
		table   ProjectState
		project string
	}
	seen := map[relocation]bool{}
	occurredAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, record := range records {
		target := projectidentity.Canonical(record.Project).String()
		if target == record.Project {
			continue
		}
		key := relocation{table: record.Table, project: record.Project}
		if seen[key] {
			continue
		}
		seen[key] = true
		switch record.Table {
		case ProjectStateSessions, ProjectStatePrompts:
			// The predicate must mirror each table's own push selection, or
			// clearing synced_at hands the row to a pusher that will never
			// pick it up. GetUnsyncedPromptsPage requires a non-empty sync_id,
			// so a legacy prompt that predates sync_id assignment would end up
			// permanently pending locally AND still under the old project name
			// on the server; leaving it synced keeps it exactly as the server
			// already has it. ListUnsyncedSessionsPage has no such guard and
			// MarkSessionSynced acks by id, so sessions relocate unconditionally.
			pushable := ""
			if record.Table == ProjectStatePrompts {
				pushable = ` AND sync_id != ''`
			}
			result, err := tx.ExecContext(ctx, `UPDATE `+string(record.Table)+`
SET synced_at = NULL, sync_from_project = ?
WHERE project = ? AND synced_at IS NOT NULL`+pushable, record.Project, record.Project)
			if err != nil {
				return err
			}
			if affected, err := result.RowsAffected(); err == nil {
				if record.Table == ProjectStateSessions {
					summary.SessionsRequeued += affected
				} else {
					summary.PromptsRequeued += affected
				}
			}
		case ProjectStateMemories:
			enqueued, err := enqueueMemoryReprojections(ctx, tx, record.Project, target, occurredAt)
			if err != nil {
				return err
			}
			summary.ReprojectsEnqueued += enqueued
		}
	}
	return nil
}

// enqueueMemoryReprojections journals one reproject mutation per memory the
// server already holds under the old spelling.
//
// The sync_ids are collected before the first insert because the read cursor and
// the writes share one SQLite connection, and the reproject carries no memory or
// tombstone payload: hive-api rejects a reproject that does, since such a
// payload would reach every puller with the weight of a create while never being
// written to any row.
func enqueueMemoryReprojections(ctx context.Context, tx *sql.Tx, fromProject, toProject, occurredAt string) (int64, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT sync_id FROM memories WHERE project = ? AND synced_at IS NOT NULL AND sync_id != ''`, fromProject)
	if err != nil {
		return 0, err
	}
	var syncIDs []string
	for rows.Next() {
		var syncID string
		if err := rows.Scan(&syncID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		syncIDs = append(syncIDs, syncID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, syncID := range syncIDs {
		if err := insertMemoryMutation(tx, memoryMutationRecord{
			EventID:      uuid.NewString(),
			EntitySyncID: syncID,
			Project:      toProject,
			Op:           MutationOpReproject,
			OccurredAt:   occurredAt,
			ActorID:      projectMigrationActorID,
			Payload:      mutationPayload{Reproject: &MutationReprojectPayload{FromProject: fromProject, ToProject: toProject}},
		}); err != nil {
			return 0, err
		}
	}
	return int64(len(syncIDs)), nil
}

func projectMigrationNeeded(records []ProjectStateRecord) bool {
	for _, record := range records {
		if projectidentity.Canonical(record.Project).String() != record.Project {
			return true
		}
	}
	return false
}

func coalesceEquivalentSyncState(ctx context.Context, tx *sql.Tx, records []ProjectStateRecord) error {
	groups := make(map[string][]ProjectStateRecord)
	for _, record := range records {
		if record.Table == ProjectStateSyncState {
			key := projectidentity.Canonical(record.Project).String()
			groups[key] = append(groups[key], record)
		}
	}
	for key, group := range groups {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].Project < group[j].Project })
		for _, record := range group[1:] {
			if record.Value != group[0].Value {
				return fmt.Errorf("%w: sync_state %s", ErrProjectMigrationConflict, key)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM sync_state WHERE project = ?`, record.Project); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sync_state SET project = ? WHERE project = ?`, key, group[0].Project); err != nil {
			return err
		}
	}
	return nil
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
	for _, table := range []string{"sync_state", "memory_mutations", "mutation_receipts", "sync_attempt_logs", "sessions", "memories", "user_prompts"} {
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

func rebuildContentProjectOwnershipTables(ctx context.Context, tx *sql.Tx) error {
	for _, trigger := range []string{"memories_ai", "memories_au", "memories_ad", "user_prompts_ai", "user_prompts_au", "user_prompts_ad"} {
		var name string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'trigger' AND name = ?`, trigger).Scan(&name); err != nil {
			return fmt.Errorf("%w: missing schema trigger %s", ErrProjectMigrationConflict, trigger)
		}
	}
	for _, statement := range []string{
		`CREATE TABLE sessions_new (id TEXT PRIMARY KEY, sync_id TEXT NOT NULL UNIQUE, project TEXT NOT NULL REFERENCES project_identities(project_key), directory TEXT NOT NULL DEFAULT '', dev_id TEXT NOT NULL, client TEXT NOT NULL, started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, ended_at DATETIME, summary TEXT, synced_at DATETIME, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, sync_from_project TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE memories_new (id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT NOT NULL, project TEXT NOT NULL REFERENCES project_identities(project_key), topic_key TEXT, category TEXT NOT NULL DEFAULT '', title TEXT NOT NULL, content TEXT NOT NULL, tags TEXT NOT NULL DEFAULT '[]', files_affected TEXT NOT NULL DEFAULT '[]', created_by TEXT NOT NULL DEFAULT 'unknown', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME, deleted_at DATETIME, deleted_by TEXT, delete_reason TEXT, restored_at DATETIME, confidence TEXT NOT NULL DEFAULT '', impact_score INTEGER NOT NULL DEFAULT 0, session_id TEXT NOT NULL REFERENCES sessions_new(id))`,
		`CREATE TABLE user_prompts_new (id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT NOT NULL DEFAULT '', project TEXT NOT NULL DEFAULT '', project_key TEXT GENERATED ALWAYS AS (NULLIF(project, '')) STORED REFERENCES project_identities(project_key), session_id TEXT NOT NULL DEFAULT '', content TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME, sync_from_project TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE memory_prompt_links_new (memory_id INTEGER NOT NULL REFERENCES memories_new(id) ON DELETE CASCADE, prompt_id INTEGER NOT NULL REFERENCES user_prompts_new(id) ON DELETE CASCADE, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (memory_id, prompt_id))`,
		// Column lists are explicit on both relocation-bearing tables: a bare
		// SELECT * matches by position, so the next column added to either
		// table would silently land in the wrong slot or abort the rebuild.
		`INSERT INTO sessions_new (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary, synced_at, created_at, updated_at, sync_from_project) SELECT id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary, synced_at, created_at, updated_at, sync_from_project FROM sessions`,
		`INSERT INTO memories_new SELECT * FROM memories`,
		`INSERT INTO user_prompts_new (id, sync_id, project, session_id, content, created_at, synced_at, sync_from_project) SELECT id, sync_id, project, session_id, content, created_at, synced_at, sync_from_project FROM user_prompts`,
		`INSERT INTO memory_prompt_links_new SELECT * FROM memory_prompt_links`,
		`DROP TABLE memory_prompt_links`,
		`DROP TRIGGER memories_ai`, `DROP TRIGGER memories_au`, `DROP TRIGGER memories_ad`, `DROP TABLE memories_fts`,
		`DROP TRIGGER user_prompts_ai`, `DROP TRIGGER user_prompts_au`, `DROP TRIGGER user_prompts_ad`, `DROP TABLE user_prompts_fts`,
		`DROP TABLE memories`, `DROP TABLE sessions`, `DROP TABLE user_prompts`,
		`ALTER TABLE sessions_new RENAME TO sessions`, `ALTER TABLE memories_new RENAME TO memories`, `ALTER TABLE user_prompts_new RENAME TO user_prompts`, `ALTER TABLE memory_prompt_links_new RENAME TO memory_prompt_links`,
		`CREATE INDEX idx_sessions_project ON sessions(project)`, `CREATE INDEX idx_sessions_started_at ON sessions(started_at DESC)`, `CREATE INDEX idx_sessions_dev_id ON sessions(dev_id)`, `CREATE UNIQUE INDEX idx_sessions_sync_id ON sessions(sync_id)`,
		`CREATE INDEX idx_memories_topic_key ON memories(project, topic_key) WHERE topic_key IS NOT NULL`, `CREATE INDEX idx_memories_project ON memories(project)`, `CREATE INDEX idx_memories_created_at ON memories(created_at DESC)`, `CREATE INDEX idx_memories_project_active ON memories(project, created_at DESC) WHERE deleted_at IS NULL`, `CREATE INDEX idx_memories_session ON memories(session_id)`, `CREATE UNIQUE INDEX idx_memories_sync_id ON memories(sync_id) WHERE sync_id != ''`,
		`CREATE INDEX idx_user_prompts_project_created ON user_prompts(project, created_at DESC)`, `CREATE INDEX idx_user_prompts_project_session_created ON user_prompts(project, session_id, created_at DESC, id DESC)`, `CREATE INDEX idx_memory_prompt_links_prompt_id ON memory_prompt_links(prompt_id)`,
		`CREATE VIRTUAL TABLE memories_fts USING fts5(title, content, tags, content='memories', content_rowid='id', tokenize='unicode61')`,
		`CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN INSERT INTO memories_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags); END`,
		`CREATE TRIGGER memories_au AFTER UPDATE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, title, content, tags) VALUES ('delete', old.id, old.title, old.content, old.tags); INSERT INTO memories_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags); END`,
		`CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, title, content, tags) VALUES ('delete', old.id, old.title, old.content, old.tags); END`,
		`CREATE VIRTUAL TABLE user_prompts_fts USING fts5(content, content='user_prompts', content_rowid='id', tokenize='unicode61')`,
		`CREATE TRIGGER user_prompts_ai AFTER INSERT ON user_prompts BEGIN INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END`,
		`CREATE TRIGGER user_prompts_au AFTER UPDATE ON user_prompts BEGIN INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content) VALUES ('delete', old.id, old.content); INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END`,
		`CREATE TRIGGER user_prompts_ad AFTER DELETE ON user_prompts BEGIN INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content) VALUES ('delete', old.id, old.content); END`,
		`INSERT INTO memories_fts(memories_fts) VALUES ('rebuild')`, `INSERT INTO user_prompts_fts(user_prompts_fts) VALUES ('rebuild')`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild content project ownership: %w", err)
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

// ProjectMigrationSummary is what one migration actually did. It exists so a
// successful startup migration can say so: this runs before the MCP transport is
// served, so an operator staring at a client that has not come up otherwise has
// no way to tell a hung daemon from a working one.
//
// Ran distinguishes "did nothing because there was nothing to do" — every daemon
// start after the first — from a migration that moved rows. Only the latter is
// worth a line.
type ProjectMigrationSummary struct {
	Ran                bool
	RowsRekeyed        int64
	ReprojectsEnqueued int64
	SessionsRequeued   int64
	PromptsRequeued    int64
}

// LastProjectMigrationSummary reports what the most recent ExecuteProjectMigration
// on this handle did.
//
// It is carried on the handle rather than returned because the executor's error
// return is load-bearing at ~60 call sites, and this is observability: a caller
// that ignores it loses a log line, not correctness.
func (d *DB) LastProjectMigrationSummary() ProjectMigrationSummary {
	d.migrationSummaryMu.Lock()
	defer d.migrationSummaryMu.Unlock()
	return d.migrationSummary
}

func (d *DB) setProjectMigrationSummary(summary ProjectMigrationSummary) {
	d.migrationSummaryMu.Lock()
	defer d.migrationSummaryMu.Unlock()
	d.migrationSummary = summary
}

// projectRekey is one scalar-column UPDATE the migration owes: move every row of
// Table whose Column reads From to the canonical spelling To.
type projectRekey struct {
	Table  ProjectState
	Column string
	From   string
	To     string
}

// planProjectRekeys reduces the per-ROW inventory to the per-SPELLING work the
// rekey actually owes.
//
// records holds one entry per row, and the loop this replaces executed one UPDATE
// per entry. Two things made that quadratic rather than merely redundant:
//
//   - A spelling shared by N rows produced N identical statements. Only the first
//     matched anything; the rest scanned the table to move nothing.
//   - A spelling that was ALREADY canonical produced N statements that each
//     matched all N rows and rewrote them — every write firing the FTS
//     delete/insert triggers. A first startup on an already-canonical database
//     (the normal case for anyone whose project names never needed folding, who
//     still runs this migration because the schema ownership rebuild is owed)
//     therefore cost O(rows²): measured at 3.1s for 250 memories, 12.6s for 500,
//     51.5s for 1000 and 3m31s for 2000, i.e. roughly 22 minutes at 5000.
//
// Both are dropped here rather than in SQL because both are pure no-ops: setting
// a column to the value it already holds changes no row, and the FTS content is
// rebuilt wholesale by rebuildProjectMigrationState afterwards either way.
//
// The result is ordered (table, then spelling) so the statement sequence is
// deterministic and a failure is reproducible.
func planProjectRekeys(records []ProjectStateRecord) []projectRekey {
	seen := make(map[projectRekey]bool, len(records))
	rekeys := make([]projectRekey, 0, len(records))
	for _, record := range records {
		column, ok := rekeyableProjectColumns[record.Table]
		if !ok {
			continue
		}
		canonical := projectidentity.Canonical(record.Project).String()
		if canonical == record.Project {
			continue
		}
		rekey := projectRekey{Table: record.Table, Column: column, From: record.Project, To: canonical}
		if seen[rekey] {
			continue
		}
		seen[rekey] = true
		rekeys = append(rekeys, rekey)
	}
	sort.Slice(rekeys, func(i, j int) bool {
		if rekeys[i].Table != rekeys[j].Table {
			return rekeys[i].Table < rekeys[j].Table
		}
		return rekeys[i].From < rekeys[j].From
	})
	return rekeys
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
		{ProjectStateSyncState, `SELECT project, 'canonical-project', json_array(last_sync_at, jwt_token, jwt_expires_at, last_attempt_at, last_success_at, last_failure_at, consecutive_failures, backoff_until, last_error, last_drain_state, last_drain_reason, last_drain_remaining), '' FROM sync_state WHERE project != '__auth__'`},
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
