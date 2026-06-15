package hook

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveSessionID resolves the session ID using the canonical priority chain,
// reading environment variables from os.Getenv.
// It never returns an empty string.
func ResolveSessionID(payload HookPayload) string {
	return resolveSessionID(os.Getenv, payload)
}

// resolveSessionID resolves the session ID using the canonical priority chain.
// envGetter is injected so the function can be unit-tested without modifying
// real environment variables.
//
// Priority (first non-empty wins):
//  1. HIVE_CLAUDE_SESSION_ID env var
//  2. CLAUDE_SESSION_ID env var
//  3. SESSION_ID env var
//  4. payload.session_id field
//  5. payload.sessionId field (alternate casing)
//  6. basename of payload.transcript_path
//  7. "ppid-{os.Getppid()}" fallback
func resolveSessionID(envGetter func(string) string, payload HookPayload) string {
	for _, name := range []string{"HIVE_CLAUDE_SESSION_ID", "CLAUDE_SESSION_ID", "SESSION_ID"} {
		if v := envGetter(name); v != "" {
			return v
		}
	}
	if payload.SessionID != "" {
		return payload.SessionID
	}
	if payload.SessionId != "" {
		return payload.SessionId
	}
	if payload.TranscriptPath != "" {
		return filepath.Base(payload.TranscriptPath)
	}
	return fmt.Sprintf("ppid-%d", os.Getppid())
}
