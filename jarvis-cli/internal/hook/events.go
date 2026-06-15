package hook

import (
	"context"
	"io"
	"time"
)

// RunSessionStart handles the Claude Code SessionStart hook event.
//
// It:
//  1. Parses the stdin payload
//  2. Resolves the session ID
//  3. Creates the first-prompt marker (idempotent)
//  4. POSTs to /sessions to notify the daemon
//  5. Returns additionalContext containing the Hive Memory Protocol text
//
// Always writes valid JSON to w and never errors the caller.
func RunSessionStart(ctx context.Context, r io.Reader, w io.Writer, baseURL string) {
	payload, _ := ParsePayload(r)
	sessionID := ResolveSessionID(payload)

	// Create marker — idempotent; ignore error (non-fatal)
	_ = CreateMarker(sessionID)

	// Notify daemon — non-fatal
	client := &DaemonClient{BaseURL: baseURL, Timeout: 4 * time.Second}
	_ = client.PostSessionStart(ctx, sessionID, payload.Project, coalesce(payload.Directory, payload.CWD))

	WriteResponse(w, HookResponse{AdditionalContext: HiveProtocolText})
}

// RunSessionCompact handles the Claude Code SessionStart hook event when the
// matcher is "compact" (i.e. a context compaction occurred).
//
// It outputs additionalContext with protocol text + post-compaction recovery
// instructions. It does NOT create or reference any marker file.
func RunSessionCompact(ctx context.Context, r io.Reader, w io.Writer) {
	// Parse payload so resolveSessionID works, but we don't use it here.
	_, _ = ParsePayload(r)
	WriteResponse(w, HookResponse{AdditionalContext: HiveCompactProtocolText})
}

// RunPromptSubmit handles the Claude Code UserPromptSubmit hook event.
//
// It:
//  1. Parses the stdin payload
//  2. Resolves session ID, project, directory
//  3. POSTs the prompt to the daemon's /prompts endpoint (fire-and-forget, 1500ms budget)
//  4. Checks the first-prompt marker:
//     - If marker is absent: creates it and returns systemMessage
//     - If marker is present: returns {}
//
// Always writes valid JSON to w and never errors the caller.
func RunPromptSubmit(ctx context.Context, r io.Reader, w io.Writer, baseURL string) {
	payload, _ := ParsePayload(r)
	sessionID := ResolveSessionID(payload)
	directory := coalesce(payload.Directory, payload.CWD)
	project := payload.Project

	// POST to daemon — fire-and-forget, short timeout
	client := &DaemonClient{BaseURL: baseURL, Timeout: 1500 * time.Millisecond}
	_ = client.PostPrompt(ctx, sessionID, directory, project, payload.Prompt)

	// First-prompt logic: O_CREATE|O_EXCL makes the check-and-create atomic,
	// preventing a TOCTOU race when concurrent hook invocations both observe the
	// marker as absent and both inject systemMessage.
	created, _ := CreateMarkerExclusive(sessionID)
	if created {
		WriteResponse(w, HookResponse{SystemMessage: FirstPromptSystemMessage})
		return
	}
	WriteEmpty(w)
}

// RunSubagentStop handles the Claude Code SubagentStop hook event.
//
// It extracts session_id, cwd, and stdout from the payload and POSTs a passive
// observation to the daemon. Always outputs {} and never errors.
func RunSubagentStop(ctx context.Context, r io.Reader, w io.Writer, baseURL string) {
	payload, _ := ParsePayload(r)
	sessionID := ResolveSessionID(payload)
	directory := coalesce(payload.Directory, payload.CWD)

	client := &DaemonClient{BaseURL: baseURL, Timeout: 8 * time.Second}
	_ = client.PostPassiveObservation(ctx, sessionID, payload.Project, "subagent", coalesce(payload.Stdout, ""))

	_ = directory // captured for future use in observation context
	WriteEmpty(w)
}

// RunSessionStop handles the Claude Code Stop hook event.
//
// It:
//  1. Resolves the session ID
//  2. Deletes the marker file (non-fatal if absent)
//  3. POSTs to /sessions/{id}/end (404 is non-fatal)
//  4. Outputs {}
func RunSessionStop(ctx context.Context, r io.Reader, w io.Writer, baseURL string) {
	payload, _ := ParsePayload(r)
	sessionID := ResolveSessionID(payload)

	// Delete marker — non-fatal
	_ = DeleteMarker(sessionID)

	// Notify daemon — non-fatal
	client := &DaemonClient{BaseURL: baseURL, Timeout: 2 * time.Second}
	_ = client.PostSessionEnd(ctx, sessionID)

	WriteEmpty(w)
}

// coalesce returns the first non-empty string from the provided values.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
