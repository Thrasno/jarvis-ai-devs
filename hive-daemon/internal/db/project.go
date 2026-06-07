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
	ErrGovernanceProjectRequired      = errors.New("project is required")
	ErrGovernanceProjectNotFound      = errors.New("governance project not found")
	ErrGovernanceProjectArchived      = errors.New("governance project is archived")
	ErrGovernanceProjectMergeInvalid  = errors.New("governance project merge source and target must differ")
	ErrGovernanceProjectMergeConflict = errors.New("governance project already merged into another target")
)

type GovernanceProject struct {
	Name               string     `json:"name"`
	Directory          string     `json:"directory"`
	ActiveMemoryCount  int        `json:"active_memory_count"`
	DeletedMemoryCount int        `json:"deleted_memory_count"`
	SessionCount       int        `json:"session_count"`
	PromptCount        int        `json:"prompt_count"`
	LastActivityAt     time.Time  `json:"last_activity_at"`
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
SELECT id, sync_id, project, topic_key, category, title, created_by, created_at, session_id,
       deleted_at, deleted_by, delete_reason
FROM memories
WHERE project = ?`
	args := []any{project}
	if !filter.IncludeDeleted {
		q += ` AND deleted_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
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

func (d *DB) ArchiveGovernanceProject(ctx context.Context, name, actorID, reason string, archivedAt time.Time) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, ErrGovernanceProjectRequired
	}
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

	sourceProject, err := getGovernanceProjectTx(ctx, tx, source)
	if err != nil {
		return false, err
	}
	if sourceProject.Merged {
		if sourceProject.MergeTarget == target {
			return false, nil
		}
		return false, ErrGovernanceProjectMergeConflict
	}
	targetProject, err := getGovernanceProjectTx(ctx, tx, target)
	if err != nil {
		return false, err
	}
	if sourceProject.Archived || targetProject.Archived {
		return false, ErrGovernanceProjectArchived
	}
	if targetProject.Merged {
		return false, ErrGovernanceProjectMergeConflict
	}
	if actorID == "" {
		actorID = detectUsername()
	}
	if mergedAt.IsZero() {
		mergedAt = time.Now().UTC()
	}

	var existingTarget, existingMergedAt string
	err = tx.QueryRowContext(ctx, `
SELECT merge_target, COALESCE(merged_at, '')
FROM hive_project_governance
WHERE project = ?`, source).Scan(&existingTarget, &existingMergedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read merge governance project: %w", err)
	}
	if existingMergedAt != "" {
		if existingTarget == target {
			return false, nil
		}
		return false, ErrGovernanceProjectMergeConflict
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO hive_project_governance (project, merge_target, merged_at, merged_by, merge_reason)
SELECT ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM sessions WHERE project = ? UNION SELECT 1 FROM memories WHERE project = ? UNION SELECT 1 FROM user_prompts WHERE project = ?)
  AND EXISTS (SELECT 1 FROM sessions WHERE project = ? UNION SELECT 1 FROM memories WHERE project = ? UNION SELECT 1 FROM user_prompts WHERE project = ?)
  AND NOT EXISTS (SELECT 1 FROM hive_project_governance WHERE project = ? AND archived_at IS NOT NULL)
  AND NOT EXISTS (SELECT 1 FROM hive_project_governance WHERE project = ? AND (archived_at IS NOT NULL OR merged_at IS NOT NULL))
ON CONFLICT(project) DO UPDATE SET
    merge_target = excluded.merge_target,
    merged_at = excluded.merged_at,
    merged_by = excluded.merged_by,
    merge_reason = excluded.merge_reason
WHERE hive_project_governance.archived_at IS NULL
  AND hive_project_governance.merged_at IS NULL
  AND EXISTS (SELECT 1 FROM sessions WHERE project = ? UNION SELECT 1 FROM memories WHERE project = ? UNION SELECT 1 FROM user_prompts WHERE project = ?)
  AND NOT EXISTS (SELECT 1 FROM hive_project_governance WHERE project = ? AND (archived_at IS NOT NULL OR merged_at IS NOT NULL))`,
		source, target, mergedAt.UTC().Format("2006-01-02 15:04:05"), actorID, reason,
		source, source, source, target, target, target, source, target,
		target, target, target, target)
	if err != nil {
		return false, fmt.Errorf("merge governance project: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("merge governance project rows affected: %w", err)
	}
	if rowsAffected == 0 {
		currentSource, err := getGovernanceProjectTx(ctx, tx, source)
		if err != nil {
			return false, err
		}
		if currentSource.Merged && currentSource.MergeTarget == target {
			return false, nil
		}
		if currentSource.Merged {
			return false, ErrGovernanceProjectMergeConflict
		}
		if currentSource.Archived {
			return false, ErrGovernanceProjectArchived
		}
		currentTarget, err := getGovernanceProjectTx(ctx, tx, target)
		if err != nil {
			return false, err
		}
		if currentTarget.Archived {
			return false, ErrGovernanceProjectArchived
		}
		if currentTarget.Merged {
			return false, ErrGovernanceProjectMergeConflict
		}
		return false, ErrGovernanceProjectMergeConflict
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit merge governance project: %w", err)
	}
	return rowsAffected > 0, nil
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
       COALESCE(project_governance.merge_reason, '')
FROM project_names
LEFT JOIN directories ON directories.project = project_names.project
LEFT JOIN memory_counts ON memory_counts.project = project_names.project
LEFT JOIN session_counts ON session_counts.project = project_names.project
LEFT JOIN prompt_counts ON prompt_counts.project = project_names.project
LEFT JOIN activity ON activity.project = project_names.project
LEFT JOIN hive_project_governance AS project_governance ON project_governance.project = project_names.project`

func scanGovernanceProject(scanner interface{ Scan(...any) error }) (GovernanceProject, error) {
	var project GovernanceProject
	var lastActivity string
	var archivedAt, mergedAt sql.NullString
	if err := scanner.Scan(&project.Name, &project.Directory, &project.ActiveMemoryCount, &project.DeletedMemoryCount, &project.SessionCount, &project.PromptCount, &lastActivity, &archivedAt, &project.ArchivedBy, &project.ArchiveReason, &project.MergeTarget, &mergedAt, &project.MergedBy, &project.MergeReason); err != nil {
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
	if err := scanner.Scan(&memory.ID, &memory.SyncID, &memory.Project, &topicKey, &memory.Category, &memory.Title, &memory.CreatedBy, &createdAt, &memory.SessionID, &deletedAt, &deletedBy, &deleteReason); err != nil {
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
