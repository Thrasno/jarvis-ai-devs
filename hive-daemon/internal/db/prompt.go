package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

func (d *DB) SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("content is required")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("project is required")
	}

	syncID := uuid.NewString()

	const q = `
INSERT INTO user_prompts (sync_id, project, content)
VALUES (?, ?, ?)
RETURNING id, created_at`

	var (
		id           int64
		createdAtStr string
	)
	err := d.sqlDB.QueryRowContext(ctx, q, syncID, project, content).Scan(&id, &createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("save prompt: %w", err)
	}

	createdAt, ok := parseDBTimestamp("created_at", createdAtStr)
	if !ok {
		return nil, fmt.Errorf("save prompt: could not parse created_at %q", createdAtStr)
	}

	return &models.Prompt{
		ID:        id,
		SyncID:    syncID,
		Project:   project,
		Content:   content,
		CreatedAt: createdAt,
		SyncedAt:  nil,
	}, nil
}

// ListRecentPrompts returns the most recent prompts for a project, ordered by
// created_at DESC. Returns nil when project is empty or limit is <= 0.
func (d *DB) ListRecentPrompts(ctx context.Context, project string, limit int) ([]*models.Prompt, error) {
	if project == "" || limit <= 0 {
		return nil, nil
	}
	if limit > 100 {
		limit = 100
	}

	const q = `
SELECT id, sync_id, project, content, created_at, synced_at
FROM user_prompts
WHERE project = ?
ORDER BY created_at DESC
LIMIT ?`

	rows, err := d.sqlDB.QueryContext(ctx, q, project, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent prompts: %w", err)
	}
	defer rows.Close()

	prompts := make([]*models.Prompt, 0)
	for rows.Next() {
		var (
			id           int64
			syncID       string
			proj         string
			content      string
			createdAtStr string
			syncedAtStr  *string
		)
		if err := rows.Scan(&id, &syncID, &proj, &content, &createdAtStr, &syncedAtStr); err != nil {
			return nil, fmt.Errorf("scan prompt row: %w", err)
		}

		createdAt, ok := parseDBTimestamp("created_at", createdAtStr)
		if !ok {
			return nil, fmt.Errorf("scan prompt row: invalid created_at %q", createdAtStr)
		}

		var syncedAt *time.Time
		if syncedAtStr != nil {
			t, ok := parseDBTimestamp("synced_at", *syncedAtStr)
			if ok {
				syncedAt = &t
			}
		}

		prompts = append(prompts, &models.Prompt{
			ID:        id,
			SyncID:    syncID,
			Project:   proj,
			Content:   content,
			CreatedAt: createdAt,
			SyncedAt:  syncedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt rows: %w", err)
	}

	return prompts, nil
}

// GetUnsyncedPrompts returns prompts for the project where synced_at IS NULL,
// ordered by created_at ASC (oldest first — sync in order of capture).
// Rows with sync_id = '' are excluded — they predate UUID generation and would
// be rejected by the server with 400 (UUID validation failure).
func (d *DB) GetUnsyncedPrompts(ctx context.Context, project string) ([]*models.Prompt, error) {
	if project == "" {
		return nil, nil
	}
	const q = `
SELECT id, sync_id, project, content, created_at, synced_at
FROM user_prompts
WHERE project = ? AND synced_at IS NULL AND sync_id != ''
ORDER BY created_at ASC`

	rows, err := d.sqlDB.QueryContext(ctx, q, project)
	if err != nil {
		return nil, fmt.Errorf("get unsynced prompts: %w", err)
	}
	defer rows.Close()

	prompts := make([]*models.Prompt, 0)
	for rows.Next() {
		var (
			id           int64
			syncID       string
			proj         string
			content      string
			createdAtStr string
			syncedAtStr  *string
		)
		if err := rows.Scan(&id, &syncID, &proj, &content, &createdAtStr, &syncedAtStr); err != nil {
			return nil, fmt.Errorf("scan unsynced prompt row: %w", err)
		}

		createdAt, ok := parseDBTimestamp("created_at", createdAtStr)
		if !ok {
			return nil, fmt.Errorf("scan unsynced prompt row: invalid created_at %q", createdAtStr)
		}

		var syncedAt *time.Time
		if syncedAtStr != nil {
			t, ok := parseDBTimestamp("synced_at", *syncedAtStr)
			if ok {
				syncedAt = &t
			} else {
				logger.Log.Printf("warn: GetUnsyncedPrompts: cannot parse synced_at %q for sync_id %s — treating as unsynced", *syncedAtStr, syncID)
			}
		}

		prompts = append(prompts, &models.Prompt{
			ID:        id,
			SyncID:    syncID,
			Project:   proj,
			Content:   content,
			CreatedAt: createdAt,
			SyncedAt:  syncedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unsynced prompt rows: %w", err)
	}

	return prompts, nil
}

// MarkPromptSynced sets synced_at for the given sync_id.
// If syncID doesn't match any row, logs a warning and returns nil (non-fatal).
func (d *DB) MarkPromptSynced(ctx context.Context, syncID string, at time.Time) error {
	const q = `UPDATE user_prompts SET synced_at = ? WHERE sync_id = ?`
	result, err := d.sqlDB.ExecContext(ctx, q, at.UTC().Format("2006-01-02 15:04:05"), syncID)
	if err != nil {
		return fmt.Errorf("mark prompt synced: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		logger.Log.Printf("warn: MarkPromptSynced: no row found for sync_id %s", syncID)
	}
	return nil
}

// parseDBTimestamp tries SQLite's default datetime format then RFC3339.
// Returns the parsed time and true on success; zero time and false on failure.
func parseDBTimestamp(field, s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			logger.Log.Printf("warn: could not parse %s %q: %v", field, s, err)
			return t, false
		}
	}
	return t, true
}
