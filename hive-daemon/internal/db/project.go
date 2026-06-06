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
	ErrGovernanceProjectRequired = errors.New("project is required")
	ErrGovernanceProjectNotFound = errors.New("governance project not found")
)

type GovernanceProject struct {
	Name               string    `json:"name"`
	Directory          string    `json:"directory"`
	ActiveMemoryCount  int       `json:"active_memory_count"`
	DeletedMemoryCount int       `json:"deleted_memory_count"`
	SessionCount       int       `json:"session_count"`
	PromptCount        int       `json:"prompt_count"`
	LastActivityAt     time.Time `json:"last_activity_at"`
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
       COALESCE(activity.last_activity_at, '')
FROM project_names
LEFT JOIN directories ON directories.project = project_names.project
LEFT JOIN memory_counts ON memory_counts.project = project_names.project
LEFT JOIN session_counts ON session_counts.project = project_names.project
LEFT JOIN prompt_counts ON prompt_counts.project = project_names.project
LEFT JOIN activity ON activity.project = project_names.project`

func scanGovernanceProject(scanner interface{ Scan(...any) error }) (GovernanceProject, error) {
	var project GovernanceProject
	var lastActivity string
	if err := scanner.Scan(&project.Name, &project.Directory, &project.ActiveMemoryCount, &project.DeletedMemoryCount, &project.SessionCount, &project.PromptCount, &lastActivity); err != nil {
		return GovernanceProject{}, err
	}
	if lastActivity != "" {
		project.LastActivityAt, _ = parseTimeStr(lastActivity)
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
