package db

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

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

// Search performs full-text search across memory title, content, and tags.
// Results are ranked by BM25 relevance with title matches weighted 2× over content.
//
// When query is empty, returns all memories for project ordered by created_at DESC.
// When project is empty, searches across all projects.
// When category is non-empty, only observations with that category are returned.
func (d *DB) Search(query, project, category string, limit int) ([]*models.Memory, error) {
	ftsQuery := buildFTS5Query(query)

	if ftsQuery == "" {
		return d.searchAllForProject(project, category, limit)
	}

	const q = `
SELECT m.id, m.sync_id, m.project, m.topic_key, m.category, m.title, m.content,
       m.tags, m.files_affected, m.created_by, m.created_at, m.confidence, m.impact_score
FROM memories m
JOIN memories_fts f ON m.id = f.rowid
WHERE f.memories_fts MATCH ?
  AND (? = '' OR m.project = ?)
  AND (? = '' OR m.category = ?)
ORDER BY bm25(memories_fts, 10, 5, 1)
LIMIT ?`

	rows, err := d.sqlDB.Query(q, ftsQuery, project, project, category, category, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}

// searchAllForProject returns all memories for a project (or all projects if empty),
// filtered by category when non-empty, ordered by created_at DESC.
// Used when the search query is empty.
func (d *DB) searchAllForProject(project, category string, limit int) ([]*models.Memory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content,
       tags, files_affected, created_by, created_at, confidence, impact_score
FROM memories
WHERE (? = '' OR project = ?)
  AND (? = '' OR category = ?)
ORDER BY created_at DESC, id DESC
LIMIT ?`

	rows, err := d.sqlDB.Query(q, project, project, category, category, limit)
	if err != nil {
		return nil, fmt.Errorf("list all for project: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}
