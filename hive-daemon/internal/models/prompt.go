package models

import "time"

// Prompt represents a captured user prompt stored in Hive.
type Prompt struct {
	ID        int64      `json:"id"`
	Content   string     `json:"content"`
	CreatedAt time.Time  `json:"created_at"`
	SyncedAt  *time.Time `json:"synced_at"`
}
