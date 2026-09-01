package models

import (
	"errors"
	"time"
)

// Memory represents a single memory observation stored in Hive.
type Memory struct {
	ID            int64      `json:"id"`
	SyncID        string     `json:"sync_id"`
	Project       string     `json:"project"`
	TopicKey      *string    `json:"topic_key"`
	Category      string     `json:"category"`
	Title         string     `json:"title"`
	Content       string     `json:"content"`
	Tags          []string   `json:"tags"`
	FilesAffected []string   `json:"files_affected"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	SyncedAt      *time.Time `json:"synced_at"` // nil = pendiente de sync
	// SessionID links this memory to a lifecycle session. Set by the MCP handler
	// (either explicit from the caller or resolved via EnsureManualSaveSession).
	SessionID string `json:"session_id,omitempty"`
	// PromptID optionally links this memory to a captured user prompt in local Hive.
	// It is local-only in PR1 and is persisted through memory_prompt_links.
	PromptID int64 `json:"prompt_id,omitempty"`
}

// MemorySearchCriteria describes full-text and structured topic filters for a
// local memory search. TopicKey and TopicPrefix preserve presence so callers
// can distinguish an omitted selector from an explicitly blank one.
type MemorySearchCriteria struct {
	Query       string
	Project     string
	Category    string
	TopicKey    *string
	TopicPrefix *string
	Limit       int
}

// Validate checks that all required fields are present.
func (m *Memory) Validate() error {
	if m.Project == "" {
		return errors.New("project is required")
	}
	if m.Title == "" {
		return errors.New("title is required")
	}
	if m.Content == "" {
		return errors.New("content is required")
	}
	return nil
}
