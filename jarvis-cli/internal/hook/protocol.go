package hook

import (
	"strings"
	"time"
)

// Mid-session memory-reminder thresholds. These are intentionally hardcoded Go
// constants — there is NO settings, environment, or CLI-flag override path.
const (
	// SessionAgeGate is the minimum session age (now − first-prompt marker
	// timestamp) before the memory reminder may fire.
	SessionAgeGate = 5 * time.Minute
	// MemoryReminderThreshold is the minimum time since the last save before the
	// memory reminder may fire.
	MemoryReminderThreshold = 15 * time.Minute
	// MemoryReminderCooldown is the minimum interval between consecutive
	// reminders (aligns with engram's 900s save cadence).
	MemoryReminderCooldown = 15 * time.Minute
	MigrationStatusCommand = "hive project identity status"
)

// Marker file base names. markerPath composes "<name>-<safeSessionID>.done".
const (
	// markerSessionStart is the dedicated baseline marker written by
	// RunSessionStart. It is intentionally distinct from markerFirstPrompt so
	// SessionStart never pre-populates the first-prompt marker; otherwise the
	// exclusive create in RunPromptSubmit would always observe created=false and
	// the FIRST ACTION nudge would never fire (issue #452).
	markerSessionStart   = "session-start"
	markerFirstPrompt    = "first-prompt"
	markerMemoryReminder = "memory-reminder"
)

// MemoryReminderSystemMessage is the systemMessage emitted mid-session when a
// session has run a while without saving to Hive. It nudges the agent to call
// mem_save so decisions, discoveries, and completed work are not lost.
const MemoryReminderSystemMessage = `Memory checkpoint: this session has gone a while without saving to Hive. If you made any decisions, discoveries, or completed work, call mcp__hive__mem_save now so team knowledge isn't lost.`

// hiveMemToolNames is the canonical, ordered list of Hive memory MCP tool names.
// It is the SINGLE SOURCE OF TRUTH for the ToolSearch load directive embedded in
// every protocol message below. Adding or renaming a Hive memory tool requires
// updating ONLY this slice — the three messages derive from it and stay in sync.
//
// The `mcp__hive__` prefix is fixed: jarvis registers the memory MCP server under
// the name `hive` (see internal/agent Claude MCP registration).
var hiveMemToolNames = []string{
	"mcp__hive__mem_context",
	"mcp__hive__mem_save",
	"mcp__hive__mem_search",
	"mcp__hive__mem_get_observation",
	"mcp__hive__mem_session_summary",
}

// hiveToolSearchQuery returns the ToolSearch "select:" query that loads every
// Hive memory tool in a single call, e.g. "select:mcp__hive__mem_context,...".
func hiveToolSearchQuery() string {
	return "select:" + strings.Join(hiveMemToolNames, ",")
}

// hiveProtocolHeader is the shared "load tools then recover memory" directive
// used by both the session-start and session-compact protocol blocks.
var hiveProtocolHeader = "## Hive Memory Protocol — ACTIVE\n\n" +
	"MANDATORY FIRST ACTION: call ToolSearch with query \"" + hiveToolSearchQuery() + "\" to load the Hive memory tools, then call mcp__hive__mem_context to recover memory from previous sessions before responding to the user.\n\n" +
	"Do not respond to the user until memory context has been loaded."

// HiveProtocolText is the Hive Memory Protocol instruction block injected into
// Claude Code at session start via additionalContext. It force-loads the Hive
// memory tools via ToolSearch, then recovers session memory.
//
// The directive is derived from hiveMemToolNames — update that slice, not this.
var HiveProtocolText = hiveProtocolHeader

// HiveCompactProtocolText is the protocol text injected at session-compact time.
// It includes the standard protocol header plus a 4-step post-compaction recovery
// sequence that guides the model through recovering context after a compaction event.
var HiveCompactProtocolText = hiveProtocolHeader + "\n\n" +
	"## Post-Compaction Recovery — REQUIRED\n\n" +
	"A context compaction just occurred. Follow these steps before resuming work:\n\n" +
	"1. Call mcp__hive__mem_session_summary with the compacted summary to persist what was done before compaction.\n" +
	"2. Call mcp__hive__mem_context to recover additional context from previous sessions.\n" +
	"3. Review any open tasks or in-progress work from the recovered context.\n" +
	"4. Only then continue working — do NOT skip step 1, or everything before compaction is lost."

// FirstPromptSystemMessage is the systemMessage returned by the prompt-submit hook
// on the first prompt of a session (when the marker file does not yet exist). Like
// the session-start block, it force-loads the Hive memory tools via ToolSearch
// before recovering session memory. Derived from hiveMemToolNames.
var FirstPromptSystemMessage = "Memory protocol is active. FIRST ACTION before responding to the user: call ToolSearch with query \"" + hiveToolSearchQuery() + "\" to load the Hive memory tools, then call mcp__hive__mem_context to recover session memory."

// BuildHiveProtocolText returns the Hive Memory Protocol text to inject into
// Claude Code's additionalContext at session start.
//
// When canonicalProject is non-empty, a canonical name pin line is appended so
// the assistant uses the exact derived project name in all subsequent mem_* calls:
//
//	Active project: <canonicalProject> — use this exact name as the project argument in all mem_* calls.
//
// When canonicalProject is empty, HiveProtocolText is returned unchanged
// (back-compat: no pin line injected).
//
// Defensive sanitization: \r and \n are stripped from canonicalProject before
// interpolation to prevent prompt-injection via crafted git remote URLs.
func BuildHiveProtocolText(canonicalProject string) string {
	if canonicalProject == "" {
		return HiveProtocolText
	}
	// Strip \r and \n defensively — the derivation source already sanitizes via
	// extractRepoName, but we guard at the injection point as well.
	safe := strings.NewReplacer("\r", "", "\n", "").Replace(canonicalProject)
	return HiveProtocolText + "\n\nActive project: " + safe + " — use this exact name as the project argument in all mem_* calls."
}

func BuildMigrationBlockedProtocol(reason, backupID string) string {
	return "## Hive Migration Blocked\n\n" +
		"State: migration-blocked\n" +
		"Reason: " + strings.TrimSpace(reason) + "\n" +
		"Backup: " + strings.TrimSpace(backupID) + "\n" +
		"Continue with: " + MigrationStatusCommand
}
