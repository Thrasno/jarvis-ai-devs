package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

func (d *DB) SavePrompt(ctx context.Context, content string) (*models.Prompt, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("content is required")
	}

	const q = `
INSERT INTO user_prompts (content)
VALUES (?)
RETURNING id, created_at`

	var (
		id           int64
		createdAtStr string
	)
	err := d.sqlDB.QueryRowContext(ctx, q, content).Scan(&id, &createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("save prompt: %w", err)
	}

	createdAt, perr := time.Parse("2006-01-02 15:04:05", createdAtStr)
	if perr != nil {
		createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}

	return &models.Prompt{
		ID:        id,
		Content:   content,
		CreatedAt: createdAt,
		SyncedAt:  nil,
	}, nil
}
