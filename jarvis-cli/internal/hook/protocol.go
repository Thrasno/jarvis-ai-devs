package hook

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
// The canonical name is inserted verbatim — no normalization, no lowercasing —
// so the assistant reproduces the exact string the daemon registered.
func BuildHiveProtocolText(canonicalProject string) string {
	if canonicalProject == "" {
		return HiveProtocolText
	}
	return HiveProtocolText + "\n\nActive project: " + canonicalProject + " — use this exact name as the project argument in all mem_* calls."
}
