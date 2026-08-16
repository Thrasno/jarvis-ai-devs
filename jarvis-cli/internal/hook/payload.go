package hook

import (
	"encoding/json"
	"io"
)

// HookPayload holds the JSON payload received from Claude Code on stdin.
// Field names cover both snake_case and camelCase variants that different
// Claude Code versions may emit.
type HookPayload struct {
	SessionID      string `json:"session_id"`
	SessionId      string `json:"sessionId"` // alternate casing
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	Prompt         string `json:"prompt"` // UserPromptSubmit only
	Stdout         string `json:"stdout"` // SubagentStop only
	Project        string `json:"project"`
	Directory      string `json:"directory"`
}

// ParsePayload reads JSON from r and returns a HookPayload.
// On invalid or empty JSON it returns a zero-value struct.
// It never panics and never propagates errors to the caller — a hook
// must always continue regardless of payload issues.
func ParsePayload(r io.Reader) (HookPayload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return HookPayload{}, err
	}
	if len(data) == 0 {
		return HookPayload{}, nil
	}
	var p HookPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return HookPayload{}, err
	}
	return p, nil
}
