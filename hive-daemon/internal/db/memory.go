package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

// SaveMemory persists a memory to the database.
// When topic_key is set, it upserts (UPDATE on conflict with same project+topic_key).
// When topic_key is nil, it always inserts a new row.
// Returns the row's id.
func (d *DB) SaveMemory(mem *models.Memory) (int64, error) {
	if err := mem.Validate(); err != nil {
		return 0, fmt.Errorf("invalid memory: %w", err)
	}

	tagsJSON, err := marshalStringSlice(mem.Tags)
	if err != nil {
		return 0, fmt.Errorf("marshal tags: %w", err)
	}
	filesJSON, err := marshalStringSlice(mem.FilesAffected)
	if err != nil {
		return 0, fmt.Errorf("marshal files_affected: %w", err)
	}

	syncID := uuid.New().String()
	createdBy := detectUsername()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	const q = `
INSERT INTO memories
    (sync_id, project, topic_key, category, title, content, tags, files_affected,
     created_by, created_at, confidence, impact_score)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project, topic_key) WHERE topic_key IS NOT NULL
DO UPDATE SET
    title          = excluded.title,
    content        = excluded.content,
    category       = excluded.category,
    tags           = excluded.tags,
    files_affected = excluded.files_affected,
    confidence     = excluded.confidence,
    impact_score   = excluded.impact_score
RETURNING id`

	var id int64
	err = d.sqlDB.QueryRow(q,
		syncID, mem.Project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, tagsJSON, filesJSON,
		createdBy, now, mem.Confidence, mem.ImpactScore,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save memory: %w", err)
	}
	return id, nil
}

// GetMemory retrieves a memory by its id.
// Returns an error if not found.
func (d *DB) GetMemory(id int64) (*models.Memory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
       created_by, created_at, confidence, impact_score
FROM memories WHERE id = ?`

	row := d.sqlDB.QueryRow(q, id)
	mem, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("memory not found: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	return mem, nil
}

// ListMemories returns memories for a project, ordered by created_at DESC.
func (d *DB) ListMemories(project string, limit int) ([]*models.Memory, error) {
	const q = `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
       created_by, created_at, confidence, impact_score
FROM memories
WHERE project = ?
ORDER BY created_at DESC, id DESC
LIMIT ?`

	rows, err := d.sqlDB.Query(q, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan memory row: %w", err)
		}
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanMemory(s scanner) (*models.Memory, error) {
	var (
		mem          models.Memory
		topicKey     sql.NullString
		tagsJSON     string
		filesJSON    string
		createdAtStr string
	)

	err := s.Scan(
		&mem.ID, &mem.SyncID, &mem.Project, &topicKey,
		&mem.Category, &mem.Title, &mem.Content,
		&tagsJSON, &filesJSON,
		&mem.CreatedBy, &createdAtStr,
		&mem.Confidence, &mem.ImpactScore,
	)
	if err != nil {
		return nil, err
	}

	if topicKey.Valid {
		mem.TopicKey = &topicKey.String
	}

	if err := json.Unmarshal([]byte(tagsJSON), &mem.Tags); err != nil {
		mem.Tags = nil
	}
	if err := json.Unmarshal([]byte(filesJSON), &mem.FilesAffected); err != nil {
		mem.FilesAffected = nil
	}

	mem.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
	if err != nil {
		// Try RFC3339 fallback (some SQLite versions)
		mem.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}

	return &mem, nil
}

func marshalStringSlice(s []string) (string, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
