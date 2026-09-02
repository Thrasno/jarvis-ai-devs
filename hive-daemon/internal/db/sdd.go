package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SDDArtifact is the winning active revision for one effective SDD topic.
type SDDArtifact struct {
	ID        int64
	Topic     string
	Content   string
	CreatedAt time.Time
}

// FetchSDDArtifacts returns at most one winning revision for each allowed
// artifact topic. Projection is complete and intentionally has no row limit.
func (d *DB) FetchSDDArtifacts(project, change string, artifacts []string) ([]SDDArtifact, error) {
	project = canonicalProjectKey(project)
	if project == "" {
		return nil, errors.New("project is required")
	}
	if change == "" {
		return nil, errors.New("change is required")
	}
	if len(artifacts) == 0 {
		return []SDDArtifact{}, nil
	}
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("fetch SDD artifacts block check: %w", err)
	}
	if blocked {
		return []SDDArtifact{}, nil
	}

	prefix := "sdd/" + change
	candidates, args, err := sddCandidateQuery(project, prefix, artifacts)
	if err != nil {
		return nil, err
	}
	topics := make([]string, len(artifacts))
	for i, artifact := range artifacts {
		topics[i] = prefix + "/" + artifact
	}
	query := `WITH candidates AS (` + candidates + `),
		eligible AS (SELECT * FROM candidates WHERE logical_topic IN (` + placeholders(len(topics)) + `)),
		ranked AS (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY logical_topic ORDER BY created_at DESC, id DESC
			) AS revision_rank
			FROM eligible
		)
		SELECT id, logical_topic, content, created_at
		FROM ranked WHERE revision_rank = 1
		ORDER BY logical_topic ASC`
	for _, topic := range topics {
		args = append(args, topic)
	}
	rows, err := d.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch SDD artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]SDDArtifact, 0, len(artifacts))
	for rows.Next() {
		var artifact SDDArtifact
		var createdAt string
		if err := rows.Scan(&artifact.ID, &artifact.Topic, &artifact.Content, &createdAt); err != nil {
			return nil, fmt.Errorf("scan SDD artifact: %w", err)
		}
		artifact.CreatedAt, err = parseTimeStr(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse SDD artifact created_at: %w", err)
		}
		result = append(result, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SDD artifacts: %w", err)
	}
	return result, nil
}

// ListSDDChanges returns projected change names after an exclusive keyset.
func (d *DB) ListSDDChanges(project string, artifacts []string, after string, limit int) ([]string, error) {
	project = canonicalProjectKey(project)
	if project == "" {
		return nil, errors.New("project is required")
	}
	if limit <= 0 {
		return nil, errors.New("limit must be positive")
	}
	if len(artifacts) == 0 {
		return []string{}, nil
	}
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("list SDD changes block check: %w", err)
	}
	if blocked {
		return []string{}, nil
	}
	candidates, args, err := sddCandidateQuery(project, "sdd", artifacts)
	if err != nil {
		return nil, err
	}
	query := `WITH candidates AS (` + candidates + `),
		parsed AS (
			SELECT *, substr(logical_topic, 5) AS topic_rest,
			       instr(substr(logical_topic, 5), '/') AS slash_at
			FROM candidates
		),
		eligible AS (
			SELECT *, substr(topic_rest, 1, slash_at - 1) AS change_name
			FROM parsed
			WHERE slash_at > 1
			  AND substr(topic_rest, slash_at + 1) IN (` + placeholders(len(artifacts)) + `)
		),
		ranked AS (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY logical_topic ORDER BY created_at DESC, id DESC
			) AS revision_rank
			FROM eligible
		),
		changes AS (SELECT DISTINCT change_name FROM ranked WHERE revision_rank = 1)
		SELECT change_name FROM changes
		WHERE change_name > ?
		ORDER BY change_name ASC
		LIMIT ?`
	for _, artifact := range artifacts {
		args = append(args, artifact)
	}
	args = append(args, after, limit)
	rows, err := d.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list SDD changes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var changes []string
	for rows.Next() {
		var change string
		if err := rows.Scan(&change); err != nil {
			return nil, fmt.Errorf("scan SDD change: %w", err)
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

func sddCandidateQuery(project, prefix string, artifacts []string) (string, []any, error) {
	selection, err := newSQLiteTopicSelection(TopicSelector{TopicPrefix: prefix})
	if err != nil {
		return "", nil, err
	}
	canonicalQuery, args := selection.queryAndArgs(project)
	titleTrim := logicalTrimSQL("title")
	legacyQuery := `
		SELECT ` + strings.Replace(topicCandidateColumns, `trim(topic_key) AS logical_topic`, titleTrim+` AS logical_topic`, 1) + `
		FROM memories
		WHERE project = ? AND deleted_at IS NULL
		  AND (topic_key IS NULL OR ` + logicalTopicKeySQL + ` = '')
		  AND (` + titleTrim + ` = ? OR (` + titleTrim + ` >= ? AND ` + titleTrim + ` < ?))`
	start := prefix + "/"
	end := prefix + "0"
	args = append(args, project, prefix, start, end)
	return canonicalQuery + " UNION ALL " + legacyQuery, args, nil
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
