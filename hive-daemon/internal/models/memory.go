package models

import (
	"errors"
	"time"
)

// Memory represents a single memory observation stored in Hive.
type Memory struct {
	ID            int64     `json:"id"`
	SyncID        string    `json:"sync_id"`
	Project       string    `json:"project"`
	TopicKey      *string   `json:"topic_key"`
	Category      string    `json:"category"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Tags          []string  `json:"tags"`
	FilesAffected []string  `json:"files_affected"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	Confidence    string    `json:"confidence"`
	ImpactScore   int       `json:"impact_score"`
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
