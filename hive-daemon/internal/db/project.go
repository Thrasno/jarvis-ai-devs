package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

var (
	ErrGovernanceProjectRequired      = errors.New("project is required")
	ErrGovernanceProjectNotFound      = errors.New("governance project not found")
	ErrGovernanceProjectArchived      = errors.New("governance project is archived")
	ErrGovernanceProjectNotArchived   = errors.New("governance project is not archived")
	ErrGovernanceProjectMergeInvalid  = errors.New("governance project merge source and target must differ")
	ErrGovernanceProjectMergeConflict = errors.New("governance project already merged into another target")
	ErrGovernanceMemoryNotFound       = errors.New("governance memory not found")
	ErrGovernanceMemoryFilterConflict = errors.New("include_deleted and deleted_only cannot be combined")
)

type GovernanceProject struct {
	Key                string     `json:"key"`
	Name               string     `json:"name"`
	Directory          string     `json:"directory"`
	ActiveMemoryCount  int        `json:"active_memory_count"`
	DeletedMemoryCount int        `json:"deleted_memory_count"`
	SessionCount       int        `json:"session_count"`
	PromptCount        int        `json:"prompt_count"`
	LastActivityAt     time.Time  `json:"last_activity_at"`
	UnsyncedCount      int        `json:"unsynced_count"`
	Archived           bool       `json:"archived"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	ArchivedBy         string     `json:"archived_by,omitempty"`
	ArchiveReason      string     `json:"archive_reason,omitempty"`
	Merged             bool       `json:"merged"`
	MergeTarget        string     `json:"merge_target,omitempty"`
	MergedAt           *time.Time `json:"merged_at,omitempty"`
	MergedBy           string     `json:"merged_by,omitempty"`
	MergeReason        string     `json:"merge_reason,omitempty"`
}

type GovernanceMemory struct {
	ID           int64      `json:"id"`
	SyncID       string     `json:"sync_id"`
	Project      string     `json:"project"`
	TopicKey     *string    `json:"topic_key,omitempty"`
	Category     string     `json:"category"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	SessionID    string     `json:"session_id,omitempty"`
	Deleted      bool       `json:"deleted"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	DeletedBy    string     `json:"deleted_by,omitempty"`
	DeleteReason string     `json:"delete_reason,omitempty"`
}

type GovernanceMemoryFilter struct {
	Project        string
	ID             int64
	TopicKey       *string
	TopicPrefix    *string
	IncludeDeleted bool
	DeletedOnly    bool
	Limit          int
	Categories     []string // empty = no category filter (all types returned)
	OrderAsc       bool     // false = DESC (default); true = ASC
}

func (d *DB) KnownProjects(ctx context.Context) ([]project.KnownProject, error) {
	rows, err := d.sqlDB.QueryContext(ctx, `
		WITH known AS (
			SELECT project, MAX(directory) AS directory FROM sessions GROUP BY project
			UNION
			SELECT project, '' AS directory FROM memories GROUP BY project
			UNION
			SELECT project, '' AS directory FROM user_prompts GROUP BY project
		)
		SELECT COALESCE(NULLIF(i.remote_spelling, ''), i.first_spelling, known.project), MAX(known.directory) AS directory
		FROM known
		LEFT JOIN project_identities i ON i.project_key = known.project
		WHERE known.project != ''
		  AND known.project NOT IN (SELECT source_project FROM project_aliases)
		  AND known.project NOT IN (SELECT project FROM hive_project_governance WHERE archived_at IS NOT NULL)
		GROUP BY known.project, i.remote_spelling, i.first_spelling
		ORDER BY known.project`)
	if err != nil {
		return nil, fmt.Errorf("known projects: %w", err)
	}
	defer rows.Close()

	var projects []project.KnownProject
	for rows.Next() {
		var p project.KnownProject
		if err := rows.Scan(&p.Name, &p.Directory); err != nil {
			return nil, fmt.Errorf("scan known project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate known projects: %w", err)
	}
	return projects, nil
}

func (d *DB) ContextProjectCounts(ctx context.Context) (known, allowed int, err error) {
	err = d.sqlDB.QueryRowContext(ctx, `
WITH known AS (
	SELECT project FROM sessions UNION SELECT project FROM memories UNION SELECT project FROM user_prompts
)
SELECT COUNT(*), COUNT(CASE WHEN NOT EXISTS (
	SELECT 1 FROM project_blocks b WHERE b.canonical_project_key = known.project AND b.blocked = 1
) THEN 1 END)
FROM known WHERE project != ''`).Scan(&known, &allowed)
	if err != nil {
		return 0, 0, fmt.Errorf("count context projects: %w", err)
	}
	return known, allowed, nil
}

func (d *DB) SessionProject(ctx context.Context, sessionID string) (string, error) {
	var projectName string
	err := d.sqlDB.QueryRowContext(ctx, `SELECT project FROM sessions WHERE id = ?`, sessionID).Scan(&projectName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", project.ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("session project: %w", err)
	}
	return projectName, nil
}

func (d *DB) ListGovernanceProjects(ctx context.Context) ([]GovernanceProject, error) {
	rows, err := d.sqlDB.QueryContext(ctx, governanceProjectsQuery+` ORDER BY project_names.project`)
	if err != nil {
		return nil, fmt.Errorf("list governance projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projects []GovernanceProject
	for rows.Next() {
		project, err := scanGovernanceProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate governance projects: %w", err)
	}
	return projects, nil
}

func (d *DB) GetGovernanceProject(ctx context.Context, name string) (GovernanceProject, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return GovernanceProject{}, ErrGovernanceProjectRequired
	}
	key := canonicalProjectKey(name)
	project, err := scanGovernanceProject(d.sqlDB.QueryRowContext(ctx, governanceProjectsQuery+` WHERE project_names.project = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return GovernanceProject{}, fmt.Errorf("%w: %s", ErrGovernanceProjectNotFound, name)
	}
	if err != nil {
		return GovernanceProject{}, err
	}
	return project, nil
}

func (d *DB) ListGovernanceMemories(ctx context.Context, filter GovernanceMemoryFilter) ([]GovernanceMemory, error) {
	project := strings.TrimSpace(filter.Project)
	if project == "" {
		return nil, ErrGovernanceProjectRequired
	}
	project = canonicalProjectKey(project)
	if project == "" {
		return nil, ErrGovernanceProjectRequired
	}
	var topicSelection *sqliteTopicSelection
	if filter.TopicKey != nil || filter.TopicPrefix != nil {
		if filter.TopicKey != nil && filter.TopicPrefix != nil {
			return nil, ErrTopicSelectorExclusive
		}
		if filter.TopicKey != nil && strings.TrimSpace(*filter.TopicKey) == "" ||
			filter.TopicPrefix != nil && strings.TrimSpace(*filter.TopicPrefix) == "" {
			return nil, ErrTopicSelectorBlank
		}
		selector := TopicSelector{}
		if filter.TopicKey != nil {
			selector.TopicKey = *filter.TopicKey
		}
		if filter.TopicPrefix != nil {
			selector.TopicPrefix = *filter.TopicPrefix
		}
		selection, err := newSQLiteTopicSelection(selector)
		if err != nil {
			return nil, err
		}
		topicSelection = &selection
	}
	blocked, err := d.isCanonicalProjectBlocked(ctx, project)
	if err != nil {
		return nil, err
	}
	if blocked {
		return []GovernanceMemory{}, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	if filter.IncludeDeleted && filter.DeletedOnly {
		return nil, ErrGovernanceMemoryFilterConflict
	}
	q := `
SELECT id, sync_id, project, topic_key, category, title, content, created_by, created_at, session_id,
       deleted_at, deleted_by, delete_reason
FROM memories
WHERE project = ?`
	args := []any{project}
	if filter.ID > 0 {
		q += ` AND id = ?`
		args = append(args, filter.ID)
	}
	if topicSelection != nil {
		predicate, predicateArgs := topicSelection.literalPredicate()
		q += ` AND ` + predicate
		args = append(args, predicateArgs...)
	}
	if filter.DeletedOnly {
		q += ` AND deleted_at IS NOT NULL`
	} else if !filter.IncludeDeleted {
		q += ` AND deleted_at IS NULL`
	}
	if len(filter.Categories) > 0 {
		placeholders := make([]string, len(filter.Categories))
		for i, c := range filter.Categories {
			placeholders[i] = "?"
			args = append(args, c)
		}
		q += ` AND category IN (` + strings.Join(placeholders, ",") + `)`
	}
	order := "DESC"
	if filter.OrderAsc {
		order = "ASC"
	}
	q += ` ORDER BY created_at ` + order + `, id ` + order + ` LIMIT ?`
	args = append(args, limit)

	rows, err := d.sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list governance memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var memories []GovernanceMemory
	for rows.Next() {
		memory, err := scanGovernanceMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, memory)
	}
	return memories, rows.Err()
}

func (d *DB) GetGovernanceMemoryByID(ctx context.Context, id int64) (GovernanceMemory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content, created_by, created_at, session_id,
       deleted_at, deleted_by, delete_reason
FROM memories WHERE id = ? AND deleted_at IS NULL`
	memory, err := scanGovernanceMemory(d.sqlDB.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return GovernanceMemory{}, fmt.Errorf("%w: id=%d", ErrGovernanceMemoryNotFound, id)
	}
	if err != nil {
		return GovernanceMemory{}, fmt.Errorf("get governance memory: %w", err)
	}
	blocked, err := d.IsProjectBlocked(ctx, memory.Project)
	if err != nil {
		return GovernanceMemory{}, err
	}
	if blocked {
		return GovernanceMemory{}, fmt.Errorf("%w: id=%d", ErrGovernanceMemoryNotFound, id)
	}
	return memory, nil
}

func (d *DB) ArchiveGovernanceProject(ctx context.Context, name, actorID, reason string, archivedAt time.Time) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, ErrGovernanceProjectRequired
	}
	name = canonicalProjectKey(name)
	release := AcquireProjectLifecycleWrite(name)
	defer release()

	// Check governance record first — merged projects may have no rows after
	// physical migration, so we must detect them via the governance table.
	var govMergedAt, govArchivedAt sql.NullString
	err := d.sqlDB.QueryRowContext(ctx, `
SELECT COALESCE(merged_at, ''), COALESCE(archived_at, '')
FROM hive_project_governance
WHERE project = ?`, name).Scan(&govMergedAt, &govArchivedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read governance for archive: %w", err)
	}
	if govMergedAt.Valid && govMergedAt.String != "" {
		return false, ErrGovernanceProjectMergeConflict
	}
	if govArchivedAt.Valid && govArchivedAt.String != "" {
		return false, nil // already archived — idempotent no-op
	}

	// Project must exist via rows (not just governance record) to be archivable.
	project, err := d.GetGovernanceProject(ctx, name)
	if err != nil {
		return false, err
	}
	if project.Archived {
		return false, nil
	}
	if project.Merged {
		return false, ErrGovernanceProjectMergeConflict
	}
	if actorID == "" {
		actorID = detectUsername()
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}
	result, err := d.sqlDB.ExecContext(ctx, `
INSERT INTO hive_project_governance (project, archived_at, archived_by, archive_reason)
VALUES (?, ?, ?, ?)
ON CONFLICT(project) DO UPDATE SET
    archived_at = excluded.archived_at,
    archived_by = excluded.archived_by,
    archive_reason = excluded.archive_reason
WHERE hive_project_governance.archived_at IS NULL
  AND hive_project_governance.merged_at IS NULL`,
		name, archivedAt.UTC().Format("2006-01-02 15:04:05"), actorID, reason)
	if err != nil {
		return false, fmt.Errorf("archive governance project: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("archive governance project rows affected: %w", err)
	}
	if rowsAffected == 0 {
		current, err := d.GetGovernanceProject(ctx, name)
		if err != nil {
			return false, err
		}
		if current.Archived {
			return false, nil
		}
		if current.Merged {
			return false, ErrGovernanceProjectMergeConflict
		}
	}
	return rowsAffected > 0, nil
}

func (d *DB) MergeGovernanceProject(ctx context.Context, source, target, actorID, reason string, mergedAt time.Time) (bool, error) {
	source = canonicalProjectKey(strings.TrimSpace(source))
	targetSpelling := strings.TrimSpace(target)
	target = canonicalProjectKey(targetSpelling)
	if source == "" || target == "" {
		return false, ErrGovernanceProjectRequired
	}
	if source == target {
		return false, ErrGovernanceProjectMergeInvalid
	}

	release := AcquireProjectLifecycleWrite(source, target)
	defer release()
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin merge governance project: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	changed, err := d.MergeGovernanceProjectTx(ctx, tx, source, target, targetSpelling, actorID, reason, mergedAt, false, false)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit merge governance project: %w", err)
	}
	return changed, nil
}

// MergeGovernanceProjectTx is the single core local-state merge contract. The
// workspace promotion path adds binding revalidation around this transaction;
// governance calls use the same migration, redirect, and lifecycle checks.
func (d *DB) MergeGovernanceProjectTx(ctx context.Context, tx *sql.Tx, source, target, targetSpelling, actorID, reason string, mergedAt time.Time, allowBoundSource, workspacePromotion bool) (bool, error) {
	var srcMergeTarget, srcMergedAt, srcArchivedAt sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT COALESCE(merge_target, ''), COALESCE(merged_at, ''), COALESCE(archived_at, '')
FROM hive_project_governance
WHERE project = ?`, source).Scan(&srcMergeTarget, &srcMergedAt, &srcArchivedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read source governance: %w", err)
	}
	if srcMergedAt.Valid && srcMergedAt.String != "" {
		if srcMergeTarget.String == target {
			return false, nil
		}
		return false, ErrGovernanceProjectMergeConflict
	}
	if srcArchivedAt.Valid && srcArchivedAt.String != "" {
		return false, ErrGovernanceProjectArchived
	}

	var srcExists bool
	err = tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM sessions WHERE project = ?
    UNION ALL SELECT 1 FROM memories WHERE project = ?
    UNION ALL SELECT 1 FROM user_prompts WHERE project = ?
    LIMIT 1
)`, source, source, source).Scan(&srcExists)
	if err != nil {
		return false, fmt.Errorf("check source exists: %w", err)
	}
	if !srcExists && !allowBoundSource {
		return false, fmt.Errorf("%w: %s", ErrGovernanceProjectNotFound, source)
	}

	var tgtMergedAt, tgtArchivedAt sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT COALESCE(merged_at, ''), COALESCE(archived_at, '')
FROM hive_project_governance
WHERE project = ?`, target).Scan(&tgtMergedAt, &tgtArchivedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read target governance: %w", err)
	}
	if tgtArchivedAt.Valid && tgtArchivedAt.String != "" {
		return false, ErrGovernanceProjectArchived
	}
	if tgtMergedAt.Valid && tgtMergedAt.String != "" {
		return false, ErrGovernanceProjectMergeConflict
	}
	guarded, err := governanceMergeHasImmutableApplyProgress(tx, source, target)
	if err != nil {
		return false, fmt.Errorf("check immutable apply progress merge guard: %w", err)
	}
	if guarded {
		if workspacePromotion {
			return false, &WorkspacePromotionProtectedError{Source: source, Target: target}
		}
		return false, fmt.Errorf("%w: immutable apply progress topology", ErrGovernanceProjectMergeConflict)
	}
	if err := reconcileSDDStoreBindingsTx(ctx, tx, source, target); err != nil {
		return false, fmt.Errorf("reconcile SDD store bindings: %w", err)
	}
	if err := validatePromotionStateCollisions(ctx, tx, source, target); err != nil {
		return false, err
	}
	if _, err := registerProjectIdentity(ctx, tx, targetSpelling); err != nil {
		return false, err
	}

	if actorID == "" {
		actorID = detectUsername()
	}
	if mergedAt.IsZero() {
		mergedAt = time.Now().UTC()
	}
	mergedAtStr := mergedAt.UTC().Format("2006-01-02 15:04:05")

	// Preserve the exact relocation protocol used by ExecuteProjectMigration:
	// only rows the server already holds carry source provenance, while unsynced
	// rows simply move and will first arrive under target.
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET synced_at = NULL, sync_from_project = ? WHERE project = ? AND synced_at IS NOT NULL`, source, source); err != nil {
		return false, fmt.Errorf("queue session relocation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_prompts SET synced_at = NULL, sync_from_project = ? WHERE project = ? AND synced_at IS NOT NULL AND sync_id != ''`, source, source); err != nil {
		return false, fmt.Errorf("queue prompt relocation: %w", err)
	}
	if _, err := enqueuePromotionMemoryReprojections(ctx, tx, source, target, mergedAtStr); err != nil {
		return false, fmt.Errorf("queue memory reprojection: %w", err)
	}
	if err := rekeyPendingMutationPayloads(ctx, tx, source, target); err != nil {
		return false, err
	}

	for _, migration := range []struct {
		query string
		label string
	}{
		{`UPDATE memories SET project = ? WHERE project = ?`, "migrate memories"},
		{`UPDATE user_prompts SET project = ? WHERE project = ?`, "migrate user_prompts"},
		{`UPDATE sessions SET project = ? WHERE project = ?`, "migrate sessions"},
		{`UPDATE memory_mutations SET project = ? WHERE project = ? AND synced_at IS NULL`, "migrate pending memory mutations"},
		{`UPDATE mutation_receipts SET project = ? WHERE project = ?`, "migrate mutation receipts"},
		// Sync positions are remote namespace coordinates, not local state to
		// transplant. Dropping A makes the next B sync start a safe full pull.
		{`DELETE FROM mutation_cursors WHERE project = ?`, "reset source mutation cursors"},
		{`DELETE FROM pull_cursors WHERE project = ?`, "reset source pull cursors"},
		{`DELETE FROM sync_state WHERE project = ? AND project != '__auth__'`, "reset source sync state"},
		{`UPDATE passive_observations SET project = ? WHERE project = ?`, "migrate passive observations"},
		{`UPDATE sync_attempt_logs SET project = ? WHERE project = ?`, "migrate sync attempt logs"},
		{`UPDATE recovery_tokens SET requested_project = ? WHERE requested_project = ?`, "migrate recovery tokens"},
		{`UPDATE import_source_aliases SET source_project = ? WHERE source_project = ?`, "migrate import aliases"},
		{`UPDATE project_blocks SET canonical_project_key = ?, project = ? WHERE canonical_project_key = ? AND project = ?`, "migrate project blocks"},
		{`UPDATE project_quarantine_archives SET canonical_project_key = ?, project = ? WHERE canonical_project_key = ? AND project = ?`, "migrate quarantine archives"},
		{`DELETE FROM hive_warnings WHERE source = ?`, "delete hive warnings"},
	} {
		args := []any{target, source}
		switch migration.label {
		case "migrate project blocks", "migrate quarantine archives":
			args = []any{target, target, source, source}
		case "delete hive warnings", "reset source mutation cursors", "reset source pull cursors", "reset source sync state":
			args = []any{source}
		}
		if _, err := tx.ExecContext(ctx, migration.query, args...); err != nil {
			return false, fmt.Errorf("%s: %w", migration.label, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO hive_project_governance (project, merge_target, merged_at, merged_by, merge_reason)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(project) DO UPDATE SET
    merge_target = excluded.merge_target,
    merged_at    = excluded.merged_at,
    merged_by    = excluded.merged_by,
    merge_reason = excluded.merge_reason
WHERE hive_project_governance.archived_at IS NULL
  AND hive_project_governance.merged_at IS NULL`, source, target, mergedAtStr, actorID, reason); err != nil {
		return false, fmt.Errorf("upsert governance merge record: %w", err)
	}
	if err := addAliasTx(ctx, tx, source, target, "local", reason); err != nil {
		return false, fmt.Errorf("merge governance project alias: %w", err)
	}
	return true, nil
}

// validatePromotionStateCollisions keeps a promotion atomic and retryable.
// These composite identities cannot be merged without inventing an idempotency
// alias or remote block history. A collision therefore fails before
// the first rekey; callers can reconcile the contradictory target explicitly
// and retry the unchanged source→target promotion.
func validatePromotionStateCollisions(ctx context.Context, tx *sql.Tx, source, target string) error {
	checks := []struct {
		state ProjectState
		query string
	}{
		{ProjectStateImportAliases, `SELECT EXISTS(SELECT 1 FROM import_source_aliases s JOIN import_source_aliases t ON t.source_system = s.source_system AND t.source_table = s.source_table AND t.source_id = s.source_id AND t.source_project = ? WHERE s.source_project = ?)`},
		{ProjectStateBlocks, `SELECT EXISTS(SELECT 1 FROM project_blocks s JOIN project_blocks t ON t.command_id = s.command_id AND t.canonical_project_key = ? WHERE s.canonical_project_key = ?)`},
		{ProjectStateQuarantineArchives, `SELECT EXISTS(SELECT 1 FROM project_quarantine_archives s JOIN project_quarantine_archives t ON t.command_id = s.command_id AND t.canonical_project_key = ? WHERE s.canonical_project_key = ?)`},
	}
	for _, check := range checks {
		var conflict bool
		if err := tx.QueryRowContext(ctx, check.query, target, source).Scan(&conflict); err != nil {
			return fmt.Errorf("check %s promotion collision: %w", check.state, err)
		}
		if conflict {
			return fmt.Errorf("%w: %s source %q target %q", ErrProjectMigrationConflict, check.state, source, target)
		}
	}
	return nil
}

// rekeyPendingMutationPayloads keeps the journal's authoritative project
// coordinates consistent with its indexed project column. Typed decoding avoids
// unsafe string replacement and makes a retry naturally idempotent: values that
// already name target are written unchanged.
func rekeyPendingMutationPayloads(ctx context.Context, tx *sql.Tx, source, target string) error {
	rows, err := tx.QueryContext(ctx, `SELECT event_id, payload_json FROM memory_mutations WHERE project = ? AND synced_at IS NULL`, source)
	if err != nil {
		return fmt.Errorf("read pending mutation payloads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type pendingPayload struct {
		eventID string
		payload mutationPayload
	}
	var pending []pendingPayload
	for rows.Next() {
		var eventID, raw string
		if err := rows.Scan(&eventID, &raw); err != nil {
			return fmt.Errorf("scan pending mutation payload: %w", err)
		}
		var payload mutationPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return fmt.Errorf("decode pending mutation payload %s: %w", eventID, err)
		}
		if payload.Memory != nil {
			payload.Memory.Project = rekeyPayloadProject(payload.Memory.Project, source, target)
		}
		if payload.Reproject != nil {
			payload.Reproject.FromProject = rekeyPayloadProject(payload.Reproject.FromProject, source, target)
			payload.Reproject.ToProject = rekeyPayloadProject(payload.Reproject.ToProject, source, target)
		}
		pending = append(pending, pendingPayload{eventID: eventID, payload: payload})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pending mutation payloads: %w", err)
	}
	for _, mutation := range pending {
		raw, err := json.Marshal(mutation.payload)
		if err != nil {
			return fmt.Errorf("encode pending mutation payload %s: %w", mutation.eventID, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memory_mutations SET payload_json = ? WHERE event_id = ? AND synced_at IS NULL`, string(raw), mutation.eventID); err != nil {
			return fmt.Errorf("rewrite pending mutation payload %s: %w", mutation.eventID, err)
		}
	}
	return nil
}

func rekeyPayloadProject(project, source, target string) string {
	if canonicalProjectKey(project) == source {
		return target
	}
	return project
}

func governanceMergeHasImmutableApplyProgress(tx *sql.Tx, source, target string) (bool, error) {
	var exists bool
	err := tx.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM sdd_apply_heads WHERE project IN (?, ?)
		UNION ALL
		SELECT 1 FROM sdd_apply_receipts WHERE project IN (?, ?)
		UNION ALL
		SELECT 1 FROM memories
		WHERE project IN (?, ?)
		  AND (topic_key LIKE 'sdd/%/apply-progress/v2' OR topic_key LIKE 'sdd/%/apply-evidence/%')
		LIMIT 1
	)`, source, target, source, target, source, target).Scan(&exists)
	return exists, err
}

// retiredPredecessorClosureTx returns the canonical project and every local
// predecessor that redirects into it. A local alias is authoritative only when
// its governance merge record confirms the same redirect; legacy governance
// records are also followed directly. It walks those redirects backwards only:
// an outbound reference from a retired identity never
// makes its target purgeable. The fixed-point traversal is deterministic and
// cycle-safe so malformed legacy redirect cycles cannot escape the purge.
func retiredPredecessorClosureTx(ctx context.Context, tx *sql.Tx, root string) ([]string, error) {
	closure := map[string]struct{}{root: {}}
	for {
		keys := make([]string, 0, len(closure))
		for key := range closure {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		added := false
		for start := 0; start < len(keys); start += 500 {
			end := start + 500
			if end > len(keys) {
				end = len(keys)
			}
			placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
			args := make([]any, end-start)
			for i, key := range keys[start:end] {
				args[i] = key
			}
			for _, query := range []string{
				`SELECT a.source_project FROM project_aliases a
JOIN hive_project_governance g ON g.project = a.source_project AND g.merge_target = a.target_project
WHERE a.scope = 'local' AND a.target_project IN (` + placeholders + `)`,
				`SELECT project FROM hive_project_governance WHERE merge_target IN (` + placeholders + `)`,
			} {
				rows, err := tx.QueryContext(ctx, query, args...)
				if err != nil {
					return nil, fmt.Errorf("read retired predecessor closure: %w", err)
				}
				for rows.Next() {
					var predecessor string
					if err := rows.Scan(&predecessor); err != nil {
						_ = rows.Close()
						return nil, fmt.Errorf("scan retired predecessor closure: %w", err)
					}
					if predecessor != "" {
						if _, exists := closure[predecessor]; !exists {
							closure[predecessor] = struct{}{}
							added = true
						}
					}
				}
				if err := rows.Err(); err != nil {
					_ = rows.Close()
					return nil, fmt.Errorf("iterate retired predecessor closure: %w", err)
				}
				if err := rows.Close(); err != nil {
					return nil, fmt.Errorf("close retired predecessor closure: %w", err)
				}
			}
		}
		if !added {
			break
		}
	}
	keys := make([]string, 0, len(closure))
	for key := range closure {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// DeleteGovernanceProject irreversibly purges every local trace of an archived,
// non-merged project and its retired local predecessors. It holds the lifecycle write lease for the complete
// transaction, snapshots indirect sync/evidence coordinates before deleting their
// owning rows, and leaves Hive API data untouched. A zero result is reserved for a
// project with no remaining local trace at all.
func (d *DB) DeleteGovernanceProject(ctx context.Context, name, actorID, reason string) (int, error) {
	name = canonicalProjectKey(name)
	if name == "" {
		return 0, ErrGovernanceProjectRequired
	}
	release := AcquireProjectLifecycleWrite(name)
	defer release()

	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delete governance project: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var mergedAt, archivedAt sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(merged_at, ''), COALESCE(archived_at, '') FROM hive_project_governance WHERE project = ?`, name).Scan(&mergedAt, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		exists, err := projectHasLocalTraceTx(ctx, tx, name)
		if err != nil {
			return 0, fmt.Errorf("check project existence for delete: %w", err)
		}
		if exists {
			return 0, ErrGovernanceProjectNotArchived
		}
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read governance for delete: %w", err)
	}
	if mergedAt.Valid && mergedAt.String != "" {
		return 0, ErrGovernanceProjectMergeConflict
	}
	if !archivedAt.Valid || archivedAt.String == "" {
		return 0, ErrGovernanceProjectNotArchived
	}

	closure, err := retiredPredecessorClosureTx(ctx, tx, name)
	if err != nil {
		return 0, err
	}
	coordinates, err := snapshotPurgeCoordinatesTx(ctx, tx, closure)
	if err != nil {
		return 0, err
	}
	var total int
	deleteStep := func(label, query string, args ...any) error {
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("%s rows affected: %w", label, err)
		}
		total += int(rows)
		return nil
	}
	updateStep := func(label, query string, args ...any) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}
	deleteProjects := func(label string, copies int, query func(string) string) error {
		batchSize := 500 / copies
		for start := 0; start < len(closure); start += batchSize {
			end := start + batchSize
			if end > len(closure) {
				end = len(closure)
			}
			placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
			args := make([]any, 0, (end-start)*copies)
			for i := 0; i < copies; i++ {
				for _, project := range closure[start:end] {
					args = append(args, project)
				}
			}
			if err := deleteStep(label, query(placeholders), args...); err != nil {
				return err
			}
		}
		return nil
	}
	updateProjects := func(label string, query func(string) string) error {
		for start := 0; start < len(closure); start += 500 {
			end := start + 500
			if end > len(closure) {
				end = len(closure)
			}
			placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
			args := make([]any, 0, end-start)
			for _, project := range closure[start:end] {
				args = append(args, project)
			}
			if err := updateStep(label, query(placeholders), args...); err != nil {
				return err
			}
		}
		return nil
	}
	deleteCoordinates := func(label, table, column string, values map[string]struct{}) error {
		const batchSize = 500
		keys := make([]string, 0, len(values))
		for value := range values {
			keys = append(keys, value)
		}
		sort.Strings(keys)
		for start := 0; start < len(keys); start += batchSize {
			end := start + batchSize
			if end > len(keys) {
				end = len(keys)
			}
			args := make([]any, 0, end-start)
			placeholders := make([]string, 0, end-start)
			for _, value := range keys[start:end] {
				args = append(args, value)
				placeholders = append(placeholders, "?")
			}
			if err := deleteStep(label, `DELETE FROM `+table+` WHERE `+column+` IN (`+strings.Join(placeholders, ",")+`)`, args...); err != nil {
				return err
			}
		}
		return nil
	}

	// Indirect WU-04 state is keyed by memory sync IDs and mutation event IDs, so
	// remove it while the owners are still available to define this purge scope.
	if err := deleteCoordinates("delete memory entity dispatches", "memory_entity_dispatches", "entity_sync_id", coordinates.syncIDs); err != nil {
		return 0, err
	}
	if err := deleteCoordinates("delete memory remote presence", "memory_remote_presence", "entity_sync_id", coordinates.syncIDs); err != nil {
		return 0, err
	}
	if err := deleteCoordinates("delete memory local origins", "memory_local_origins", "entity_sync_id", coordinates.syncIDs); err != nil {
		return 0, err
	}
	if err := deleteCoordinates("delete mutation outcomes by event", "memory_mutation_outcomes", "event_id", coordinates.eventIDs); err != nil {
		return 0, err
	}
	if err := deleteCoordinates("delete mutation outcomes by entity", "memory_mutation_outcomes", "entity_sync_id", coordinates.syncIDs); err != nil {
		return 0, err
	}
	if err := deleteCoordinates("delete mutation dispatches", "memory_mutation_dispatches", "event_id", coordinates.eventIDs); err != nil {
		return 0, err
	}

	for _, step := range []struct {
		label    string
		copies   int
		newQuery func(string) string
	}{
		{"delete memory prompt links", 2, func(placeholders string) string {
			return `DELETE FROM memory_prompt_links WHERE memory_id IN (SELECT id FROM memories WHERE project IN (` + placeholders + `)) OR prompt_id IN (SELECT id FROM user_prompts WHERE project IN (` + placeholders + `))`
		}},
		{"delete memory mutations", 1, func(placeholders string) string {
			return `DELETE FROM memory_mutations WHERE project IN (` + placeholders + `)`
		}},
		{"delete mutation receipts", 1, func(placeholders string) string {
			return `DELETE FROM mutation_receipts WHERE project IN (` + placeholders + `)`
		}},
		{"delete mutation cursors", 1, func(placeholders string) string {
			return `DELETE FROM mutation_cursors WHERE project IN (` + placeholders + `)`
		}},
		{"delete pull cursors", 1, func(placeholders string) string {
			return `DELETE FROM pull_cursors WHERE project IN (` + placeholders + `)`
		}},
		{"delete sync state", 1, func(placeholders string) string {
			return `DELETE FROM sync_state WHERE project IN (` + placeholders + `) AND project != '__auth__'`
		}},
		{"delete sync attempts", 1, func(placeholders string) string {
			return `DELETE FROM sync_attempt_logs WHERE project IN (` + placeholders + `)`
		}},
		{"delete passive observations", 1, func(placeholders string) string {
			return `DELETE FROM passive_observations WHERE project IN (` + placeholders + `)`
		}},
		{"delete import aliases", 1, func(placeholders string) string {
			return `DELETE FROM import_source_aliases WHERE source_project IN (` + placeholders + `)`
		}},
		{"delete project blocks", 2, func(placeholders string) string {
			return `DELETE FROM project_blocks WHERE canonical_project_key IN (` + placeholders + `) OR project IN (` + placeholders + `)`
		}},
		{"delete project quarantine archives", 2, func(placeholders string) string {
			return `DELETE FROM project_quarantine_archives WHERE canonical_project_key IN (` + placeholders + `) OR project IN (` + placeholders + `)`
		}},
		{"delete hive warnings", 1, func(placeholders string) string {
			return `DELETE FROM hive_warnings WHERE source IN (` + placeholders + `)`
		}},
		{"delete workspace bindings", 1, func(placeholders string) string {
			return `DELETE FROM workspace_project_bindings WHERE project IN (` + placeholders + `)`
		}},
		{"delete aliases", 2, func(placeholders string) string {
			return `DELETE FROM project_aliases WHERE source_project IN (` + placeholders + `) OR target_project IN (` + placeholders + `)`
		}},
		{"delete SDD store bindings", 1, func(placeholders string) string {
			return `DELETE FROM sdd_store_bindings WHERE project IN (` + placeholders + `)`
		}},
		{"delete SDD apply heads", 1, func(placeholders string) string {
			return `DELETE FROM sdd_apply_heads WHERE project IN (` + placeholders + `)`
		}},
	} {
		if err := deleteProjects(step.label, step.copies, step.newQuery); err != nil {
			return 0, err
		}
	}
	if err := updateProjects("clear session relocation provenance", func(placeholders string) string {
		return `UPDATE sessions SET sync_from_project = '' WHERE sync_from_project IN (` + placeholders + `)`
	}); err != nil {
		return 0, err
	}
	if err := updateProjects("clear prompt relocation provenance", func(placeholders string) string {
		return `UPDATE user_prompts SET sync_from_project = '' WHERE sync_from_project IN (` + placeholders + `)`
	}); err != nil {
		return 0, err
	}
	tokens := make([]string, 0, len(coordinates.recoveryTokens))
	for token := range coordinates.recoveryTokens {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	for _, token := range tokens {
		if err := deleteStep("delete recovery token", `DELETE FROM recovery_tokens WHERE token = ?`, token); err != nil {
			return 0, err
		}
	}

	// Receipt deletion is the sole narrowly-scoped exception to its immutable
	// trigger. The helper restores the trigger before this transaction can commit.
	receiptRows, err := deleteApplyProgressReceiptsForGovernancePurge(ctx, tx, closure)
	if err != nil {
		return 0, fmt.Errorf("delete sdd_apply_receipts: %w", err)
	}
	total += receiptRows

	for _, step := range []struct {
		label    string
		copies   int
		newQuery func(string) string
	}{
		{"delete memories", 1, func(placeholders string) string {
			return `DELETE FROM memories WHERE project IN (` + placeholders + `)`
		}},
		{"delete prompts", 1, func(placeholders string) string {
			return `DELETE FROM user_prompts WHERE project IN (` + placeholders + `)`
		}},
		{"delete sessions", 1, func(placeholders string) string {
			return `DELETE FROM sessions WHERE project IN (` + placeholders + `)`
		}},
		// Reverse merge records are redirects to this identity and cannot survive
		// a local purge of their target.
		{"delete governance metadata", 2, func(placeholders string) string {
			return `DELETE FROM hive_project_governance WHERE project IN (` + placeholders + `) OR merge_target IN (` + placeholders + `)`
		}},
		{"delete project identity", 1, func(placeholders string) string {
			return `DELETE FROM project_identities WHERE project_key IN (` + placeholders + `)`
		}},
	} {
		if err := deleteProjects(step.label, step.copies, step.newQuery); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit delete governance project: %w", err)
	}
	return total, nil
}

type purgeCoordinates struct {
	syncIDs        map[string]struct{}
	eventIDs       map[string]struct{}
	recoveryTokens map[string]struct{}
}

func snapshotPurgeCoordinatesTx(ctx context.Context, tx *sql.Tx, projects []string) (purgeCoordinates, error) {
	coordinates := purgeCoordinates{syncIDs: map[string]struct{}{}, eventIDs: map[string]struct{}{}, recoveryTokens: map[string]struct{}{}}
	collect := func(query string, destination map[string]struct{}, args ...any) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return err
			}
			if value != "" {
				destination[value] = struct{}{}
			}
		}
		return rows.Err()
	}
	for _, project := range projects {
		for _, source := range []struct {
			query       string
			destination map[string]struct{}
		}{
			{`SELECT sync_id FROM memories WHERE project = ?`, coordinates.syncIDs},
			{`SELECT entity_sync_id FROM memory_mutations WHERE project = ?`, coordinates.syncIDs},
			{`SELECT event_id FROM memory_mutations WHERE project = ?`, coordinates.eventIDs},
			{`SELECT entity_sync_id FROM mutation_receipts WHERE project = ?`, coordinates.syncIDs},
			{`SELECT event_id FROM mutation_receipts WHERE project = ?`, coordinates.eventIDs},
		} {
			if err := collect(source.query, source.destination, project); err != nil {
				return purgeCoordinates{}, fmt.Errorf("snapshot purge coordinates: %w", err)
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT token, requested_project, selected_project, candidates_json FROM recovery_tokens`)
	if err != nil {
		return purgeCoordinates{}, fmt.Errorf("read recovery tokens for purge: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var token, requested, selected, candidatesJSON string
		if err := rows.Scan(&token, &requested, &selected, &candidatesJSON); err != nil {
			return purgeCoordinates{}, fmt.Errorf("scan recovery token for purge: %w", err)
		}
		var candidates []struct {
			Project string `json:"project"`
		}
		if err := json.Unmarshal([]byte(candidatesJSON), &candidates); err != nil {
			return purgeCoordinates{}, fmt.Errorf("decode recovery token %q for purge: %w", token, err)
		}
		for _, project := range projects {
			if canonicalProjectKey(requested) == project || canonicalProjectKey(selected) == project {
				coordinates.recoveryTokens[token] = struct{}{}
				break
			}
			for _, candidate := range candidates {
				if canonicalProjectKey(candidate.Project) == project {
					coordinates.recoveryTokens[token] = struct{}{}
					break
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return purgeCoordinates{}, fmt.Errorf("iterate recovery tokens for purge: %w", err)
	}
	return coordinates, nil
}

func projectHasLocalTraceTx(ctx context.Context, tx *sql.Tx, project string) (bool, error) {
	// Indirect evidence has no project column. It is attributable only while an
	// owning memory, mutation, or receipt supplies its sync/event coordinate, so
	// snapshotPurgeCoordinatesTx removes it before owner deletion. Orphaned global
	// evidence cannot be attributed to any project and does not block idempotency.
	closure, err := retiredPredecessorClosureTx(ctx, tx, project)
	if err != nil {
		return false, err
	}
	coordinates, err := snapshotPurgeCoordinatesTx(ctx, tx, closure)
	if err != nil {
		return false, err
	}
	if len(coordinates.recoveryTokens) > 0 {
		return true, nil
	}
	for _, project := range closure {
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM project_identities WHERE project_key = ?
		UNION ALL SELECT 1 FROM sessions WHERE project = ? OR sync_from_project = ?
		UNION ALL SELECT 1 FROM memories WHERE project = ?
		UNION ALL SELECT 1 FROM user_prompts WHERE project = ? OR sync_from_project = ?
		UNION ALL SELECT 1 FROM memory_mutations WHERE project = ?
		UNION ALL SELECT 1 FROM mutation_receipts WHERE project = ?
		UNION ALL SELECT 1 FROM mutation_cursors WHERE project = ?
		UNION ALL SELECT 1 FROM pull_cursors WHERE project = ?
		UNION ALL SELECT 1 FROM sync_state WHERE project = ? AND project != '__auth__'
		UNION ALL SELECT 1 FROM sync_attempt_logs WHERE project = ?
		UNION ALL SELECT 1 FROM passive_observations WHERE project = ?
		UNION ALL SELECT 1 FROM import_source_aliases WHERE source_project = ?
		UNION ALL SELECT 1 FROM project_blocks WHERE canonical_project_key = ? OR project = ?
		UNION ALL SELECT 1 FROM project_quarantine_archives WHERE canonical_project_key = ? OR project = ?
		UNION ALL SELECT 1 FROM hive_warnings WHERE source = ?
		UNION ALL SELECT 1 FROM workspace_project_bindings WHERE project = ?
		UNION ALL SELECT 1 FROM project_aliases WHERE source_project = ? OR target_project = ?
		UNION ALL SELECT 1 FROM hive_project_governance WHERE project = ? OR merge_target = ?
		UNION ALL SELECT 1 FROM sdd_store_bindings WHERE project = ?
		UNION ALL SELECT 1 FROM sdd_apply_heads WHERE project = ?
		UNION ALL SELECT 1 FROM sdd_apply_receipts WHERE project = ?
		LIMIT 1
	)`, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project, project).Scan(&exists)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

// deleteApplyProgressReceiptsForGovernancePurge is the sole receipt-deletion
// exception. SQLite triggers cannot identify the calling Go method, so this
// archived-project transaction removes the delete trigger only for its exact
// delete statement and recreates it before commit. SQLite DDL is transactional:
// any failure rolls back both the receipt deletion and trigger removal.
func deleteApplyProgressReceiptsForGovernancePurge(ctx context.Context, tx *sql.Tx, projects []string) (int, error) {
	if _, err := tx.ExecContext(ctx, `DROP TRIGGER protect_sdd_apply_receipts_delete`); err != nil {
		return 0, fmt.Errorf("drop receipt protection trigger: %w", err)
	}
	var total int
	var deleteErr error
	for start := 0; start < len(projects); start += 500 {
		end := start + 500
		if end > len(projects) {
			end = len(projects)
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", end-start), ",")
		args := make([]any, 0, end-start)
		for _, project := range projects[start:end] {
			args = append(args, project)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM sdd_apply_receipts WHERE project IN (`+placeholders+`)`, args...)
		if err != nil {
			deleteErr = err
			break
		}
		rows, err := result.RowsAffected()
		if err != nil {
			deleteErr = fmt.Errorf("rows affected: %w", err)
			break
		}
		total += int(rows)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TRIGGER protect_sdd_apply_receipts_delete BEFORE DELETE ON sdd_apply_receipts BEGIN SELECT RAISE(ABORT, 'immutable apply progress receipt'); END`); err != nil {
		return 0, fmt.Errorf("restore receipt protection trigger: %w", err)
	}
	if deleteErr != nil {
		return 0, deleteErr
	}
	return total, nil
}

// ProjectMergeSyncEvidence reports whether any memory in the given projects has
// synced_at IS NOT NULL, indicating cloud-synchronized data is present.
// Used by the batch merge service to populate the cloud guardrail flag.
func (d *DB) ProjectMergeSyncEvidence(ctx context.Context, projects []string) (bool, error) {
	if len(projects) == 0 {
		return false, nil
	}
	// Build placeholders: (?, ?, ...)
	placeholders := make([]string, len(projects))
	args := make([]any, len(projects))
	for i, p := range projects {
		placeholders[i] = "?"
		args[i] = p
	}
	q := `SELECT EXISTS(SELECT 1 FROM memories WHERE project IN (` +
		strings.Join(placeholders, ",") +
		`) AND synced_at IS NOT NULL)`
	var exists int
	if err := d.sqlDB.QueryRowContext(ctx, q, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("project merge sync evidence: %w", err)
	}
	return exists == 1, nil
}

const governanceProjectsQuery = `
WITH project_names AS (
    SELECT project FROM sessions WHERE project != ''
    UNION
    SELECT project FROM memories WHERE project != ''
    UNION
    SELECT project FROM user_prompts WHERE project != ''
), directories AS (
    SELECT project, MAX(directory) AS directory
    FROM sessions
    WHERE directory != ''
    GROUP BY project
), memory_counts AS (
    SELECT project,
           SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END) AS active_count,
           SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END) AS deleted_count
    FROM memories
    GROUP BY project
), session_counts AS (
    SELECT project, COUNT(*) AS session_count FROM sessions GROUP BY project
), prompt_counts AS (
    SELECT project, COUNT(*) AS prompt_count FROM user_prompts GROUP BY project
), activity AS (
    SELECT project, MAX(activity_at) AS last_activity_at
    FROM (
        SELECT project, updated_at AS activity_at FROM memories
        UNION ALL SELECT project, started_at AS activity_at FROM sessions
        UNION ALL SELECT project, created_at AS activity_at FROM user_prompts
    )
    GROUP BY project
), unsynced_counts AS (
    SELECT project, COUNT(*) AS unsynced_count
    FROM memories
    WHERE synced_at IS NULL AND deleted_at IS NULL
    GROUP BY project
)
SELECT project_names.project,
       COALESCE(NULLIF(identity.remote_spelling, ''), NULLIF(identity.first_spelling, ''), project_names.project),
       COALESCE(directories.directory, ''),
       COALESCE(memory_counts.active_count, 0),
       COALESCE(memory_counts.deleted_count, 0),
       COALESCE(session_counts.session_count, 0),
       COALESCE(prompt_counts.prompt_count, 0),
       COALESCE(activity.last_activity_at, ''),
       project_governance.archived_at,
       COALESCE(project_governance.archived_by, ''),
       COALESCE(project_governance.archive_reason, ''),
       COALESCE(NULLIF(merge_identity.remote_spelling, ''), NULLIF(merge_identity.first_spelling, ''), project_governance.merge_target, ''),
       project_governance.merged_at,
       COALESCE(project_governance.merged_by, ''),
       COALESCE(project_governance.merge_reason, ''),
       COALESCE(unsynced_counts.unsynced_count, 0)
FROM project_names
LEFT JOIN project_identities AS identity ON identity.project_key = project_names.project
LEFT JOIN directories ON directories.project = project_names.project
LEFT JOIN memory_counts ON memory_counts.project = project_names.project
LEFT JOIN session_counts ON session_counts.project = project_names.project
LEFT JOIN prompt_counts ON prompt_counts.project = project_names.project
LEFT JOIN activity ON activity.project = project_names.project
LEFT JOIN hive_project_governance AS project_governance ON project_governance.project = project_names.project
LEFT JOIN project_identities AS merge_identity ON merge_identity.project_key = project_governance.merge_target
LEFT JOIN unsynced_counts ON unsynced_counts.project = project_names.project`

func scanGovernanceProject(scanner interface{ Scan(...any) error }) (GovernanceProject, error) {
	var project GovernanceProject
	var lastActivity string
	var archivedAt, mergedAt sql.NullString
	if err := scanner.Scan(&project.Key, &project.Name, &project.Directory, &project.ActiveMemoryCount, &project.DeletedMemoryCount, &project.SessionCount, &project.PromptCount, &lastActivity, &archivedAt, &project.ArchivedBy, &project.ArchiveReason, &project.MergeTarget, &mergedAt, &project.MergedBy, &project.MergeReason, &project.UnsyncedCount); err != nil {
		return GovernanceProject{}, err
	}
	if lastActivity != "" {
		project.LastActivityAt, _ = parseTimeStr(lastActivity)
	}
	if archivedAt.Valid && archivedAt.String != "" {
		project.Archived = true
		parsed, _ := parseTimeStr(archivedAt.String)
		project.ArchivedAt = &parsed
	}
	if mergedAt.Valid && mergedAt.String != "" {
		project.Merged = true
		parsed, _ := parseTimeStr(mergedAt.String)
		project.MergedAt = &parsed
	}
	return project, nil
}

func scanGovernanceMemory(scanner interface{ Scan(...any) error }) (GovernanceMemory, error) {
	var memory GovernanceMemory
	var topicKey, deletedAt, deletedBy, deleteReason sql.NullString
	var createdAt string
	if err := scanner.Scan(&memory.ID, &memory.SyncID, &memory.Project, &topicKey, &memory.Category, &memory.Title, &memory.Content, &memory.CreatedBy, &createdAt, &memory.SessionID, &deletedAt, &deletedBy, &deleteReason); err != nil {
		return GovernanceMemory{}, fmt.Errorf("scan governance memory: %w", err)
	}
	if topicKey.Valid {
		memory.TopicKey = &topicKey.String
	}
	memory.CreatedAt, _ = parseTimeStr(createdAt)
	if deletedAt.Valid && deletedAt.String != "" {
		memory.Deleted = true
		parsedDeletedAt, _ := parseTimeStr(deletedAt.String)
		memory.DeletedAt = &parsedDeletedAt
	}
	if deletedBy.Valid {
		memory.DeletedBy = deletedBy.String
	}
	if deleteReason.Valid {
		memory.DeleteReason = deleteReason.String
	}
	return memory, nil
}
