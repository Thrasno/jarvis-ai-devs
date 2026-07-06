package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

func (d *DB) SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error) {
	return d.SavePromptForSession(ctx, project, "", content)
}

func (d *DB) SavePromptForSession(ctx context.Context, project, sessionID, content string) (*models.Prompt, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("content is required")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("project is required")
	}
	if err := d.ensureProjectWritable(ctx, project); err != nil {
		return nil, err
	}

	syncID := uuid.NewString()

	const q = `
INSERT INTO user_prompts (sync_id, project, session_id, content)
VALUES (?, ?, ?, ?)
RETURNING id, created_at`

	var (
		id           int64
		createdAtStr string
	)
	err := d.sqlDB.QueryRowContext(ctx, q, syncID, project, sessionID, content).Scan(&id, &createdAtStr)
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
		SessionID: sessionID,
		Content:   content,
		CreatedAt: createdAt,
		SyncedAt:  nil,
	}, nil
}

func (d *DB) LatestPromptForSession(ctx context.Context, project, sessionID string) (*models.Prompt, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	blocked, err := d.IsProjectBlocked(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("latest prompt block check: %w", err)
	}
	if blocked {
		return nil, nil
	}

	const q = `
SELECT id, sync_id, project, session_id, content, created_at, synced_at
FROM user_prompts
WHERE project = ? AND session_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1`

	prompt, err := scanPromptRow(d.sqlDB.QueryRowContext(ctx, q, project, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest prompt for session: %w", err)
	}
	return prompt, nil
}

// ListRecentPrompts returns the most recent prompts for a project, ordered by
// created_at DESC. Returns nil when project is empty or limit is <= 0.
func (d *DB) ListRecentPrompts(ctx context.Context, project string, limit int) ([]*models.Prompt, error) {
	if project == "" || limit <= 0 {
		return nil, nil
	}
	blocked, err := d.IsProjectBlocked(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list recent prompts block check: %w", err)
	}
	if blocked {
		return []*models.Prompt{}, nil
	}
	if limit > 100 {
		limit = 100
	}

	const q = `
SELECT id, sync_id, project, session_id, content, created_at, synced_at
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
		prompt, err := scanPromptRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan prompt row: %w", err)
		}
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt rows: %w", err)
	}

	return prompts, nil
}

// GetUnsyncedPrompts returns prompts for the project where synced_at IS NULL,
// ordered by created_at ASC (oldest first — sync in order of capture).
// Rows with sync_id = "" are excluded — they predate UUID generation and would
// be rejected by the server with 400 (UUID validation failure).
func (d *DB) GetUnsyncedPrompts(ctx context.Context, project string) ([]*models.Prompt, error) {
	if project == "" {
		return nil, nil
	}
	blocked, err := d.IsProjectBlocked(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get unsynced prompts block check: %w", err)
	}
	if blocked {
		return []*models.Prompt{}, nil
	}
	const q = `
SELECT id, sync_id, project, session_id, content, created_at, synced_at
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
		prompt, err := scanPromptRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced prompt row: %w", err)
		}
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unsynced prompt rows: %w", err)
	}

	return prompts, nil
}

// GetUnsyncedPromptsPage returns at most `limit` unsynced prompts for the
// project, ordered by created_at ASC (oldest first) with id ASC as a
// secondary tiebreaker. This is the paged counterpart to GetUnsyncedPrompts
// (PR 1b-iii, hive-sync-batched-drain): syncBatchStep uses this instead of
// the unpaged getter so a single push batch never exceeds syncPageSize,
// while the ORDER BY keeps paging stable across repeated calls as earlier
// rows get marked synced. created_at has only second-level granularity, and
// this table is also served by idx_user_prompts_project_created (project,
// created_at DESC), so without the secondary id ASC key a created_at tie
// can be returned in descending id order instead of oldest-first, letting
// rows straddling a page boundary be skipped or duplicated as the
// synced_at IS NULL filter shifts between fetches. GetUnsyncedPrompts
// itself is left untouched for any other existing callers. Same
// sync_id = "" exclusion as GetUnsyncedPrompts (rows that predate UUID
// generation are never queued for sync).
func (d *DB) GetUnsyncedPromptsPage(ctx context.Context, project string, limit int) ([]*models.Prompt, error) {
	if project == "" {
		return nil, nil
	}
	blocked, err := d.IsProjectBlocked(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get unsynced prompts page block check: %w", err)
	}
	if blocked {
		return []*models.Prompt{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	const q = `
SELECT id, sync_id, project, session_id, content, created_at, synced_at
FROM user_prompts
WHERE project = ? AND synced_at IS NULL AND sync_id != ''
ORDER BY created_at ASC, id ASC LIMIT ?`

	rows, err := d.sqlDB.QueryContext(ctx, q, project, limit)
	if err != nil {
		return nil, fmt.Errorf("get unsynced prompts page: %w", err)
	}
	defer rows.Close()

	prompts := make([]*models.Prompt, 0)
	for rows.Next() {
		prompt, err := scanPromptRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced prompt page row: %w", err)
		}
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unsynced prompt page rows: %w", err)
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

func scanPromptRow(s scanner) (*models.Prompt, error) {
	var (
		id           int64
		syncID       string
		project      string
		sessionID    string
		content      string
		createdAtStr string
		syncedAtStr  *string
	)
	if err := s.Scan(&id, &syncID, &project, &sessionID, &content, &createdAtStr, &syncedAtStr); err != nil {
		return nil, err
	}

	createdAt, ok := parseDBTimestamp("created_at", createdAtStr)
	if !ok {
		return nil, fmt.Errorf("invalid created_at %q", createdAtStr)
	}

	var syncedAt *time.Time
	if syncedAtStr != nil {
		t, ok := parseDBTimestamp("synced_at", *syncedAtStr)
		if ok {
			syncedAt = &t
		} else {
			logger.Log.Printf("warn: scan prompt row: cannot parse synced_at %q for sync_id %s — treating as unsynced", *syncedAtStr, syncID)
		}
	}

	return &models.Prompt{
		ID:        id,
		SyncID:    syncID,
		Project:   project,
		SessionID: sessionID,
		Content:   content,
		CreatedAt: createdAt,
		SyncedAt:  syncedAt,
	}, nil
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
