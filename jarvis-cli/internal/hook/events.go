package hook

import (
	"context"
	"io"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
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
	directory := coalesce(payload.Directory, payload.CWD)

	// Derive the canonical project name locally using the original DetectProject
	// so the pinned name equals the registered name (same function, same input).
	canonical := project.DetectProject(directory)

	// Create marker — idempotent; ignore error (non-fatal)
	_ = CreateMarker(sessionID)

	// Notify daemon with the derived canonical name — non-fatal.
	// This registers the canonical project before any mem_save call.
	client := &DaemonClient{BaseURL: baseURL, Timeout: 4 * time.Second}
	_ = client.PostSessionStart(ctx, sessionID, canonical, directory)

	WriteResponse(w, HookResponse{AdditionalContext: BuildHiveProtocolText(canonical)})
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

	// Derive canonical project locally so prompts attach to the canonical project.
	canonical := project.DetectProject(directory)

	// POST to daemon — fire-and-forget, short timeout
	client := &DaemonClient{BaseURL: baseURL, Timeout: 1500 * time.Millisecond}
	_ = client.PostPrompt(ctx, sessionID, directory, canonical, payload.Prompt)

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
//
// The canonical project name is derived locally via DetectProject (same as
// RunSessionStart/RunPromptSubmit) so the observation is attributed to the
// canonical project, not the raw payload.Project value.
func RunSubagentStop(ctx context.Context, r io.Reader, w io.Writer, baseURL string) {
	payload, _ := ParsePayload(r)
	sessionID := ResolveSessionID(payload)
	directory := coalesce(payload.Directory, payload.CWD)

	// Derive canonical project from the filesystem — consistent with RunSessionStart
	// and RunPromptSubmit so all events use the same canonical name.
	canonical := project.DetectProject(directory)

	client := &DaemonClient{BaseURL: baseURL, Timeout: 8 * time.Second}
	_ = client.PostPassiveObservation(ctx, sessionID, canonical, "subagent", coalesce(payload.Stdout, ""), directory)

	WriteEmpty(w)
}

// RunSessionStop handles the Claude Code Stop hook event.
//
// It:
//  1. Resolves the session ID
//  2. POSTs to /sessions/{id}/end (404 is non-fatal)
//  3. Outputs {}
//
// Note: the first-prompt marker is intentionally NOT deleted here.
// Claude Code fires Stop after every agent turn in interactive mode, not only
// when the session window closes. Deleting the marker on Stop would cause
// FirstPromptSystemMessage to fire on every subsequent prompt in the session.
// Markers use UUID-based names so old markers from finished sessions never
// collide with new ones; the OS temp dir eventually reclaims them.
func RunSessionStop(ctx context.Context, r io.Reader, w io.Writer, baseURL string) {
	payload, _ := ParsePayload(r)
	sessionID := ResolveSessionID(payload)

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
