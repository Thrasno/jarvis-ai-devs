package models

import "time"

// Session represents a lifecycle session in Hive.
type Session struct {
	ID        string
	SyncID    string
	Project   string
	Directory string
	DevID     string
	Client    string
	StartedAt time.Time
	EndedAt   *time.Time
	Summary   string
	SyncedAt  *time.Time
}
