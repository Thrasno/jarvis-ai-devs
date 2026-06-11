package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

var (
	ErrGovernanceProjectRequired        = errors.New("project is required")
	ErrGovernanceProjectNotFound        = errors.New("governance project not found")
	ErrGovernanceProjectArchived        = errors.New("governance project is archived")
	ErrGovernanceProjectNotArchived     = errors.New("governance project is not archived")
	ErrGovernanceProjectMergeInvalid    = errors.New("governance project merge source and target must differ")
	ErrGovernanceProjectMergeConflict   = errors.New("governance project already merged into another target")
	ErrGovernanceMemoryNotFound         = errors.New("governance memory not found")
)

type GovernanceProject struct {
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
	IncludeDeleted bool
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
		SELECT project, MAX(directory) AS directory
		FROM known
		WHERE project != ''
		  AND project NOT IN (SELECT source_project FROM project_aliases)
		  AND project NOT IN (SELECT project FROM hive_project_governance WHERE archived_at IS NOT NULL)
		GROUP BY project
		ORDER BY project`)
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
	project, err := scanGovernanceProject(d.sqlDB.QueryRowContext(ctx, governanceProjectsQuery+` WHERE project_names.project = ?`, name))
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
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	q := `
SELECT id, sync_id, project, topic_key, category, title, content, created_by, created_at, session_id,
       deleted_at, deleted_by, delete_reason
FROM memories
WHERE project = ?`
	args := []any{project}
	if !filter.IncludeDeleted {
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
	return memory, nil
}

func (d *DB) ArchiveGovernanceProject(ctx context.Context, name, actorID, reason string, archivedAt time.Time) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, ErrGovernanceProjectRequired
	}

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
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == "" || target == "" {
		return false, ErrGovernanceProjectRequired
	}
	if source == target {
		return false, ErrGovernanceProjectMergeInvalid
	}

	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin merge governance project: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Read governance record for source first. This handles idempotency and
	// conflict detection without relying on row existence (rows may have already
	// been moved on a prior partial run or idempotent re-call).
	var srcMergeTarget, srcMergedAt, srcArchivedAt sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT COALESCE(merge_target, ''), COALESCE(merged_at, ''), COALESCE(archived_at, '')
FROM hive_project_governance
WHERE project = ?`, source).Scan(&srcMergeTarget, &srcMergedAt, &srcArchivedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read source governance: %w", err)
	}
	if srcMergedAt.Valid && srcMergedAt.String != "" {
		// Idempotency: already merged into the same target is a no-op.
		if srcMergeTarget.String == target {
			return false, nil
		}
		return false, ErrGovernanceProjectMergeConflict
	}
	if srcArchivedAt.Valid && srcArchivedAt.String != "" {
		return false, ErrGovernanceProjectArchived
	}

	// Source must exist: check for rows in at least one write table.
	// This guard ensures we don't silently merge a typo project name.
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
	if !srcExists {
		return false, fmt.Errorf("%w: %s", ErrGovernanceProjectNotFound, source)
	}

	// Target existence guard removed: physical migration creates the target
	// implicitly. Only check target lifecycle via governance record.
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

	if actorID == "" {
		actorID = detectUsername()
	}
	if mergedAt.IsZero() {
		mergedAt = time.Now().UTC()
	}

	mergedAtStr := mergedAt.UTC().Format("2006-01-02 15:04:05")

	// Step (a): migrate memories.
	if _, err := tx.ExecContext(ctx,
		`UPDATE memories SET project = ? WHERE project = ?`, target, source); err != nil {
		return false, fmt.Errorf("migrate memories: %w", err)
	}
	// Step (b): migrate user_prompts.
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_prompts SET project = ? WHERE project = ?`, target, source); err != nil {
		return false, fmt.Errorf("migrate user_prompts: %w", err)
	}
	// Step (c): migrate sessions.
	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET project = ? WHERE project = ?`, target, source); err != nil {
		return false, fmt.Errorf("migrate sessions: %w", err)
	}
	// Step (d): migrate pending memory_mutations only (synced ones are cloud-historical).
	if _, err := tx.ExecContext(ctx,
		`UPDATE memory_mutations SET project = ? WHERE project = ? AND synced_at IS NULL`,
		target, source); err != nil {
		return false, fmt.Errorf("migrate memory_mutations: %w", err)
	}
	// Step (e): delete hive_warnings for the source project.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM hive_warnings WHERE source = ?`, source); err != nil {
		return false, fmt.Errorf("delete hive_warnings: %w", err)
	}
	// Step (f): delete sync_state for the source project; never touch __auth__.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sync_state WHERE project = ? AND project != '__auth__'`, source); err != nil {
		return false, fmt.Errorf("delete sync_state: %w", err)
	}
	// Step (g): governance record upsert — mark source as merged.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO hive_project_governance (project, merge_target, merged_at, merged_by, merge_reason)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(project) DO UPDATE SET
    merge_target = excluded.merge_target,
    merged_at    = excluded.merged_at,
    merged_by    = excluded.merged_by,
    merge_reason = excluded.merge_reason
WHERE hive_project_governance.archived_at IS NULL
  AND hive_project_governance.merged_at IS NULL`,
		source, target, mergedAtStr, actorID, reason); err != nil {
		return false, fmt.Errorf("upsert governance merge record: %w", err)
	}
	// Step (h): alias safety net — write-redirect for stray cloud writes.
	if err := addAliasTx(ctx, tx, source, target, "local", reason); err != nil {
		return false, fmt.Errorf("merge governance project alias: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit merge governance project: %w", err)
	}
	return true, nil
}

// DeleteGovernanceProject irreversibly purges all local data for an archived project
// in a single self-contained transaction. The project must already be archived
// (archived_at IS NOT NULL in hive_project_governance); if not, ErrGovernanceProjectNotArchived
// is returned. Deletion order: memory_mutations → project_aliases → memories
// (FTS5 maintained by the memories_ad trigger) → user_prompts → sessions →
// sync_state (excluding __auth__) → hive_warnings → hive_project_governance.
// Returns the total count of rows deleted across memory_mutations, project_aliases,
// memories, user_prompts, sessions, hive_warnings, and the governance row.
// sync_state rows are deleted but not counted (the __auth__ row is intentionally
// excluded from deletion and the per-project row count is not meaningful to callers).
// If the project is not found at all (already purged), returns (0, nil) for idempotency.
func (d *DB) DeleteGovernanceProject(ctx context.Context, name, actorID, reason string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, ErrGovernanceProjectRequired
	}

	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delete governance project: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Check governance record: project must be archived (not merged, not live).
	var mergedAt, archivedAt sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(merged_at, ''), COALESCE(archived_at, '') FROM hive_project_governance WHERE project = ?`, name,
	).Scan(&mergedAt, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// No governance row: check whether the project has any data rows at all.
		// If it does, it exists but was never archived. If not, it was already purged.
		var exists bool
		existErr := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM sessions WHERE project = ?
    UNION ALL SELECT 1 FROM memories WHERE project = ?
    UNION ALL SELECT 1 FROM user_prompts WHERE project = ?
    LIMIT 1
)`, name, name, name).Scan(&exists)
		if existErr != nil {
			return 0, fmt.Errorf("check project existence for delete: %w", existErr)
		}
		if exists {
			// Project has rows but no governance row — it is not archived.
			return 0, ErrGovernanceProjectNotArchived
		}
		// No rows and no governance row: already purged, idempotent.
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read governance for delete: %w", err)
	}
	if mergedAt.Valid && mergedAt.String != "" {
		// Merged projects must return the merge-conflict sentinel, not not-archived.
		// Mirrors the guard in ArchiveGovernanceProject.
		return 0, ErrGovernanceProjectMergeConflict
	}
	if !archivedAt.Valid || archivedAt.String == "" {
		return 0, ErrGovernanceProjectNotArchived
	}

	var total int

	// Step 1: delete memory_mutations.
	res, err := tx.ExecContext(ctx, `DELETE FROM memory_mutations WHERE project = ?`, name)
	if err != nil {
		return 0, fmt.Errorf("delete memory_mutations: %w", err)
	}
	n, _ := res.RowsAffected()
	total += int(n)

	// Step 2: delete project_aliases (both directions).
	res, err = tx.ExecContext(ctx,
		`DELETE FROM project_aliases WHERE source_project = ? OR target_project = ?`, name, name)
	if err != nil {
		return 0, fmt.Errorf("delete project_aliases: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	// Step 3: delete memories (memories_ad AFTER DELETE trigger maintains FTS5).
	res, err = tx.ExecContext(ctx, `DELETE FROM memories WHERE project = ?`, name)
	if err != nil {
		return 0, fmt.Errorf("delete memories: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	// Step 4: delete user_prompts.
	res, err = tx.ExecContext(ctx, `DELETE FROM user_prompts WHERE project = ?`, name)
	if err != nil {
		return 0, fmt.Errorf("delete user_prompts: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	// Step 5: delete sessions.
	res, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE project = ?`, name)
	if err != nil {
		return 0, fmt.Errorf("delete sessions: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	// Step 6: delete sync_state (never touch __auth__).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sync_state WHERE project = ? AND project != '__auth__'`, name); err != nil {
		return 0, fmt.Errorf("delete sync_state: %w", err)
	}

	// Step 7: delete hive_warnings.
	res, err = tx.ExecContext(ctx, `DELETE FROM hive_warnings WHERE source = ?`, name)
	if err != nil {
		return 0, fmt.Errorf("delete hive_warnings: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	// Step 8: delete governance row.
	res, err = tx.ExecContext(ctx, `DELETE FROM hive_project_governance WHERE project = ?`, name)
	if err != nil {
		return 0, fmt.Errorf("delete hive_project_governance: %w", err)
	}
	n, _ = res.RowsAffected()
	total += int(n)

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit delete governance project: %w", err)
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

func getGovernanceProjectTx(ctx context.Context, tx *sql.Tx, name string) (GovernanceProject, error) {
	project, err := scanGovernanceProject(tx.QueryRowContext(ctx, governanceProjectsQuery+` WHERE project_names.project = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return GovernanceProject{}, fmt.Errorf("%w: %s", ErrGovernanceProjectNotFound, name)
	}
	if err != nil {
		return GovernanceProject{}, err
	}
	return project, nil
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
       COALESCE(directories.directory, ''),
       COALESCE(memory_counts.active_count, 0),
       COALESCE(memory_counts.deleted_count, 0),
       COALESCE(session_counts.session_count, 0),
       COALESCE(prompt_counts.prompt_count, 0),
       COALESCE(activity.last_activity_at, ''),
       project_governance.archived_at,
       COALESCE(project_governance.archived_by, ''),
       COALESCE(project_governance.archive_reason, ''),
       COALESCE(project_governance.merge_target, ''),
       project_governance.merged_at,
       COALESCE(project_governance.merged_by, ''),
       COALESCE(project_governance.merge_reason, ''),
       COALESCE(unsynced_counts.unsynced_count, 0)
FROM project_names
LEFT JOIN directories ON directories.project = project_names.project
LEFT JOIN memory_counts ON memory_counts.project = project_names.project
LEFT JOIN session_counts ON session_counts.project = project_names.project
LEFT JOIN prompt_counts ON prompt_counts.project = project_names.project
LEFT JOIN activity ON activity.project = project_names.project
LEFT JOIN hive_project_governance AS project_governance ON project_governance.project = project_names.project
LEFT JOIN unsynced_counts ON unsynced_counts.project = project_names.project`

func scanGovernanceProject(scanner interface{ Scan(...any) error }) (GovernanceProject, error) {
	var project GovernanceProject
	var lastActivity string
	var archivedAt, mergedAt sql.NullString
	if err := scanner.Scan(&project.Name, &project.Directory, &project.ActiveMemoryCount, &project.DeletedMemoryCount, &project.SessionCount, &project.PromptCount, &lastActivity, &archivedAt, &project.ArchivedBy, &project.ArchiveReason, &project.MergeTarget, &mergedAt, &project.MergedBy, &project.MergeReason, &project.UnsyncedCount); err != nil {
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
