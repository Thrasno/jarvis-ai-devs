package models

// SessionInput carries the write-time attribution for a regular session.
type SessionInput struct {
	ID        string
	Project   string
	Directory string
	DevID     string
	Client    string
}

// SessionEndInput carries the attribution and end semantics for a regular session.
type SessionEndInput struct {
	Session            SessionInput
	Summary            string
	RejectAlreadyEnded bool
}
