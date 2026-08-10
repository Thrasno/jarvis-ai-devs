package models

import "time"

// Prompt represents a captured user prompt stored in Hive.
type Prompt struct {
	ID             int64      `json:"id"`
	SyncID         string     `json:"sync_id"`
	Project        string     `json:"project"`
	DisplayProject string     `json:"-"`
	SessionID      string     `json:"session_id,omitempty"`
	Content        string     `json:"content"`
	CreatedAt      time.Time  `json:"created_at"`
	SyncedAt       *time.Time `json:"synced_at"`
	// SyncFromProject is the same pending relocation precondition
	// Session.SyncFromProject carries; see it for the full rationale. Not
	// serialized: it is a sync-internal concern, never part of a prompt's
	// public shape.
	SyncFromProject string `json:"-"`
}
