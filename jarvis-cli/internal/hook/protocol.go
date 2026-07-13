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
)

// Marker file base names. markerPath composes "<name>-<safeSessionID>.done".
const (
	markerFirstPrompt    = "first-prompt"
	markerMemoryReminder = "memory-reminder"
)

// MemoryReminderSystemMessage is the systemMessage emitted mid-session when a
// session has run a while without saving to Hive. It nudges the agent to call
// mem_save so decisions, discoveries, and completed work are not lost.
const MemoryReminderSystemMessage = `Memory checkpoint: this session has gone a while without saving to Hive. If you made any decisions, discoveries, or completed work, call mem_save now so team knowledge isn't lost.`

// HiveProtocolText is the Hive Memory Protocol instruction block injected into
// Claude Code at session start via additionalContext.
//
// Update this constant directly when the protocol changes.
const HiveProtocolText = `## Hive Memory Protocol — ACTIVE

MANDATORY FIRST ACTION: call mem_context to recover memory from previous sessions before responding to the user.

If mem_context is not available, call ToolSearch with query "mem_context" to load it first.

Do not respond to the user until memory context has been loaded.`

// HiveCompactProtocolText is the protocol text injected at session-compact time.
// It includes the standard protocol header plus a 4-step post-compaction recovery
// sequence that guides the model through recovering context after a compaction event.
const HiveCompactProtocolText = `## Hive Memory Protocol — ACTIVE

MANDATORY FIRST ACTION: call mem_context to recover memory from previous sessions before responding to the user.

If mem_context is not available, call ToolSearch with query "mem_context" to load it first.

Do not respond to the user until memory context has been loaded.

## Post-Compaction Recovery — REQUIRED

A context compaction just occurred. Follow these steps before resuming work:

1. Call mem_session_summary with the compacted summary to persist what was done before compaction.
2. Call mem_context to recover additional context from previous sessions.
3. Review any open tasks or in-progress work from the recovered context.
4. Only then continue working — do NOT skip step 1, or everything before compaction is lost.`

// FirstPromptSystemMessage is the systemMessage returned by the prompt-submit hook
// on the first prompt of a session (when the marker file does not yet exist).
const FirstPromptSystemMessage = `Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user.`

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
