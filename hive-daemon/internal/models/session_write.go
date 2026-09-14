package models

// SessionInput carries the write-time attribution for a regular session.
type SessionInput struct {
	ID        string
	Project   string
	Directory string
	DevID     string
	Client    string
}

// PromptWrite carries the session attribution and content for an atomic prompt capture.
type PromptWrite struct {
	Session SessionInput
	Content string
}

// SessionEndInput carries the attribution and end semantics for a regular session.
type SessionEndInput struct {
	Session            SessionInput
	Summary            string
	RejectAlreadyEnded bool
}

// PassiveObservationWrite carries the session attribution and content for an
// atomic passive-observation capture.
type PassiveObservationWrite struct {
	Session SessionInput
	Source  string
	Content string
}
