package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

var ErrSearchCriterionRequired = errors.New("at least one nonblank search criterion is required")

// buildFTS5Query sanitizes a user query for safe use in FTS5 MATCH expressions.
// Each whitespace-separated term is wrapped in double quotes, preventing FTS5
// syntax errors from special characters (@, #, :, *, etc.).
// Internal double quotes within a term are escaped by doubling them.
// Returns "" for empty or whitespace-only queries.
func buildFTS5Query(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	terms := strings.Fields(query)
	quoted := make([]string, len(terms))
	for i, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		quoted[i] = `"` + term + `"`
	}
	return strings.Join(quoted, " ")
}

// Search performs BM25 full-text search and/or literal structured topic search.
func (d *DB) Search(criteria models.MemorySearchCriteria) ([]*models.Memory, error) {
	criteria.Project = canonicalProjectKey(criteria.Project)
	ftsQuery := buildFTS5Query(criteria.Query)
	var selection *sqliteTopicSelection
	if criteria.TopicKey != nil || criteria.TopicPrefix != nil {
		selector := TopicSelector{}
		if criteria.TopicKey != nil {
			selector.TopicKey = *criteria.TopicKey
		}
		if criteria.TopicPrefix != nil {
			selector.TopicPrefix = *criteria.TopicPrefix
		}
		selected, err := newSQLiteTopicSelection(selector)
		if err != nil {
			return nil, err
		}
		selection = &selected
	}
	if ftsQuery == "" && selection == nil {
		return nil, ErrSearchCriterionRequired
	}
	blockedKeys, err := d.blockedProjectKeys(context.Background())
	if err != nil {
		return nil, err
	}
	if criteria.Project != "" {
		if _, blocked := blockedKeys[criteria.Project]; blocked {
			return []*models.Memory{}, nil
		}
	}

	if selection == nil {
		const q = `
SELECT m.id, m.sync_id, m.project, m.topic_key, m.category, m.title, m.content,
	   m.tags, m.files_affected, m.created_by, m.created_at, m.session_id
FROM memories m
JOIN memories_fts f ON m.id = f.rowid
WHERE f.memories_fts MATCH ?
  AND m.deleted_at IS NULL
  AND (? = '' OR m.project = ?)
  AND (? = '' OR m.category = ?)
ORDER BY bm25(memories_fts, 10, 5, 1)
LIMIT ?`
		rows, err := d.sqlDB.Query(q, ftsQuery, criteria.Project, criteria.Project, criteria.Category, criteria.Category, criteria.Limit)
		if err != nil {
			return nil, fmt.Errorf("fts search: %w", err)
		}
		return scanSearchRows(rows, blockedKeys)
	}

	candidates, args := selection.queryAndArgs(criteria.Project)
	query := `WITH candidates AS (` + candidates + `)
		SELECT m.id, m.sync_id, m.project, m.logical_topic, m.category, m.title, m.content,
		       m.tags, m.files_affected, m.created_by, m.created_at, m.session_id
		FROM candidates m`
	if ftsQuery != "" {
		query += ` JOIN memories_fts f ON m.id = f.rowid
			WHERE f.memories_fts MATCH ? AND (? = '' OR m.category = ?)
			ORDER BY bm25(memories_fts, 10, 5, 1) LIMIT ?`
		args = append(args, ftsQuery, criteria.Category, criteria.Category, criteria.Limit)
	} else {
		query += ` WHERE (? = '' OR m.category = ?)
			ORDER BY m.created_at DESC, m.id DESC LIMIT ?`
		args = append(args, criteria.Category, criteria.Category, criteria.Limit)
	}
	rows, err := d.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("structured search: %w", err)
	}
	return scanSearchRows(rows, blockedKeys)
}

func scanSearchRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}, blockedKeys map[string]struct{}) ([]*models.Memory, error) {
	defer func() { _ = rows.Close() }()
	var results []*models.Memory
	for rows.Next() {
		mem, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		if _, blocked := blockedKeys[canonicalProjectKey(mem.Project)]; blocked {
			continue
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}
