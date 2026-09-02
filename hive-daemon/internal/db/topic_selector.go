package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

var (
	ErrTopicSelectorRequired  = errors.New("topic selector is required")
	ErrTopicSelectorBlank     = errors.New("topic selector is blank")
	ErrTopicSelectorExclusive = errors.New("topic_key and topic_prefix are mutually exclusive")
)

// TopicSelector selects one logical topic or one slash-delimited topic subtree.
// TopicKey and TopicPrefix are mutually exclusive.
type TopicSelector struct {
	TopicKey    string
	TopicPrefix string
}

type sqliteTopicSelection struct {
	candidateQuery string
	exact          bool
	values         []string
}

const topicCandidateColumns = `
	id, sync_id, project, topic_key, category, title, content, tags, files_affected,
	created_by, created_at, session_id, trim(topic_key) AS logical_topic`

// SQLite's one-argument trim only removes ASCII spaces. This character set
// mirrors Go's strings.TrimSpace for historical rows written before normalization.
const logicalWhitespaceSQL = `char(9) || char(10) || char(11) || char(12) || char(13) || char(32) || ` +
	`char(133) || char(160) || char(5760) || char(8192) || char(8193) || char(8194) || ` +
	`char(8195) || char(8196) || char(8197) || char(8198) || char(8199) || char(8200) || ` +
	`char(8201) || char(8202) || char(8232) || char(8233) || char(8239) || char(8287) || char(12288)`

const logicalTopicKeySQL = `trim(topic_key, ` + logicalWhitespaceSQL + `)`

func logicalTrimSQL(column string) string {
	return `trim(` + column + `, ` + logicalWhitespaceSQL + `)`
}

func withLogicalTopicTrim(query string) string {
	return strings.ReplaceAll(query, "trim(topic_key)", logicalTopicKeySQL)
}

func newSQLiteTopicSelection(selector TopicSelector) (sqliteTopicSelection, error) {
	hasExact := selector.TopicKey != ""
	hasPrefix := selector.TopicPrefix != ""
	if hasExact && hasPrefix {
		return sqliteTopicSelection{}, ErrTopicSelectorExclusive
	}
	if !hasExact && !hasPrefix {
		return sqliteTopicSelection{}, ErrTopicSelectorRequired
	}

	value := selector.TopicKey
	if hasPrefix {
		value = selector.TopicPrefix
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return sqliteTopicSelection{}, ErrTopicSelectorBlank
	}

	if hasExact {
		return sqliteTopicSelection{
			candidateQuery: buildTopicCandidateQuery(true, true),
			exact:          true,
			values:         []string{value, value},
		}, nil
	}

	descendantStart := value + "/"
	// Under the column's default BINARY collation, every value beginning with
	// value+"/" is below value+"0" because '/' immediately precedes '0'.
	descendantEnd := value + "0"
	return sqliteTopicSelection{
		candidateQuery: buildTopicCandidateQuery(false, true),
		values:         []string{value, descendantStart, descendantEnd, value, descendantStart, descendantEnd},
	}, nil
}

func buildTopicCandidateQuery(exact, scoped bool) string {
	project := ""
	if scoped {
		project = "project = ? AND "
	}
	if exact {
		return withLogicalTopicTrim(`
			SELECT ` + topicCandidateColumns + ` FROM memories
			WHERE ` + project + `topic_key = ? AND deleted_at IS NULL
			UNION ALL
			SELECT ` + topicCandidateColumns + ` FROM memories
			WHERE ` + project + `topic_key IS NOT NULL
			  AND topic_key <> trim(topic_key) AND trim(topic_key) = ?
			  AND deleted_at IS NULL`)
	}
	return withLogicalTopicTrim(`
		SELECT ` + topicCandidateColumns + ` FROM memories
		WHERE ` + project + `topic_key = ? AND deleted_at IS NULL
		UNION ALL
		SELECT ` + topicCandidateColumns + ` FROM memories
		WHERE ` + project + `topic_key >= ? AND topic_key < ?
		  AND topic_key = trim(topic_key) AND deleted_at IS NULL
		UNION ALL
		SELECT ` + topicCandidateColumns + ` FROM memories
		WHERE ` + project + `topic_key IS NOT NULL
		  AND topic_key <> trim(topic_key)
		  AND (trim(topic_key) = ? OR (trim(topic_key) >= ? AND trim(topic_key) < ?))
		  AND deleted_at IS NULL`)
}

func (s sqliteTopicSelection) args(project string) []any {
	if len(s.values) == 2 {
		return []any{project, s.values[0], project, s.values[1]}
	}
	return []any{
		project, s.values[0],
		project, s.values[1], s.values[2],
		project, s.values[3], s.values[4], s.values[5],
	}
}

func (s sqliteTopicSelection) queryAndArgs(project string) (string, []any) {
	if project != "" {
		return s.candidateQuery, s.args(project)
	}
	args := make([]any, len(s.values))
	for i, value := range s.values {
		args[i] = value
	}
	return buildTopicCandidateQuery(s.exact, false), args
}

func (s sqliteTopicSelection) literalPredicate() (string, []any) {
	if s.exact {
		return logicalTopicKeySQL + ` = ?`, []any{s.values[0]}
	}
	return `(` + logicalTopicKeySQL + ` = ? OR (` + logicalTopicKeySQL + ` >= ? AND ` + logicalTopicKeySQL + ` < ?))`,
		[]any{s.values[0], s.values[1], s.values[2]}
}

// SelectLatestTopicMemories returns one latest active revision per selected
// logical topic. Ranking happens before limit so revision-heavy topics cannot
// starve other topics from the response.
func (d *DB) SelectLatestTopicMemories(project string, selector TopicSelector, limit int) ([]*models.Memory, error) {
	project = canonicalProjectKey(project)
	if project == "" {
		return nil, errors.New("project is required")
	}
	if limit <= 0 {
		return nil, errors.New("limit must be positive")
	}
	selection, err := newSQLiteTopicSelection(selector)
	if err != nil {
		return nil, err
	}
	blocked, err := d.IsProjectBlocked(context.Background(), project)
	if err != nil {
		return nil, fmt.Errorf("select topic memories block check: %w", err)
	}
	if blocked {
		return []*models.Memory{}, nil
	}

	query := `WITH candidates AS (` + selection.candidateQuery + `),
		ranked AS (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY logical_topic ORDER BY created_at DESC, id DESC
			) AS revision_rank
			FROM candidates
		)
		SELECT id, sync_id, project, logical_topic, category, title, content, tags, files_affected,
		       created_by, created_at, session_id
		FROM ranked
		WHERE revision_rank = 1
		ORDER BY created_at DESC, id DESC
		LIMIT ?`
	args := append(selection.args(project), limit)
	rows, err := d.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("select latest topic memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var memories []*models.Memory
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan selected topic memory: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate selected topic memories: %w", err)
	}
	return memories, nil
}
