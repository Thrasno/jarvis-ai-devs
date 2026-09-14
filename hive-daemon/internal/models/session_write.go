package models

// SessionInput carries the write-time attribution for a regular session.
type SessionInput struct {
	ID        string
	Project   string
	Directory string
	DevID     string
	Client    string
}
