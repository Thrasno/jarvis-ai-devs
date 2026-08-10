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
	// SyncFromProject names the project literal the server still holds for this
	// session after a local identity migration renamed it here. It is a pending
	// write-side precondition sent as from_project, not history: empty means no
	// relocation is pending, and the ack clears it.
	SyncFromProject string
}
