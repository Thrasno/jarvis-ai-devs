package hook

import (
	"strings"
	"time"
	"unicode/utf8"
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
	// MigrationStatusCommand is the local recovery surface for a migration that
	// ran and failed.
	MigrationStatusCommand = "hive project identity status"
	// MigrationNormalizationCommand is the local entry point for an ambiguous
	// project identity. The CLI status command can report the ambiguity but
	// cannot resolve it; the wizard is a screen inside the Hive TUI.
	//
	// It is defined HERE, not read from the daemon's status payload, for the same
	// reason MigrationStatusCommand is: see migrationProtocolContinuation.
	MigrationNormalizationCommand = "jarvis hive → Project normalization"
)

// The daemon's migration-gate state literals, mirrored from
// hive-daemon/internal/project.MigrationState*. They are wire values, so they
// are duplicated rather than imported: jarvis-cli does not depend on the daemon
// module, and the hook must keep working against an older or newer daemon.
//
// Only two of them are named here because only two change what the hook renders.
// Every other value — including one this build has never heard of — is treated as
// a failure, which is the cautious reading: this build cannot prove the database
// was left intact, so it must not claim it was.
const (
	// MigrationStateReady is the one state that means Hive is serving normally.
	MigrationStateReady = "ready"
	// MigrationStateBlocked means a migration was attempted and failed.
	MigrationStateBlocked = "migration-blocked"
	// MigrationStatePendingOperatorReview means a read-only preflight found
	// ambiguous project identities and stopped before writing anything. Nothing
	// failed and nothing was migrated; what is missing is a human decision.
	MigrationStatePendingOperatorReview = "migration-pending-operator-review"
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
// interpolation to prevent prompt-injection via crafted git remote URLs. The
// name itself is never shortened — see sanitizeProtocolIdentifier.
func BuildHiveProtocolText(canonicalProject string) string {
	if canonicalProject == "" {
		return HiveProtocolText
	}
	safe := sanitizeProtocolIdentifier(canonicalProject)
	return HiveProtocolText + "\n\nActive project: " + safe + " — use this exact name as the project argument in all mem_* calls."
}

// ProtocolValueTruncated marks a value this package shortened, so a reader can
// tell a bounded value from a complete one.
const ProtocolValueTruncated = " […truncated]"

// Injected-value bounds. A migration reason is a sentence or two in every real
// case; a backup id is a timestamp and a uuid. There is deliberately NO bound
// for the project pin — see sanitizeProtocolIdentifier.
// There is no continuation bound because the continuation is never
// daemon-supplied — it is one of two local constants.
const (
	maxProtocolReasonLength   = 500
	maxProtocolBackupIDLength = 128
	maxProtocolStateLength    = 128
)

// The two kinds of value this package interpolates into the session context are
// NOT interchangeable, and the difference is why there are two sanitizers:
//
//   - An IDENTIFIER (the project pin) is an instruction to the model to pass
//     that exact string as an argument. It must round-trip. Shortening it — with
//     or without a marker — makes the model address a project that does not
//     exist, because projectidentity.Canonical folds a shortened spelling into a
//     different key than the full name the hook registered with the daemon.
//   - PROSE (the migration reason, the backup id) is text for a human to read.
//     Nothing looks it up, so it can be cut at any length.
//
// Both kinds share exactly one requirement — no line breaks. Every value here
// occupies one labelled line, so a value carrying \r or \n could open a line of
// its own and impersonate the protocol ("## System", another "Continue with:",
// anything). That hazard is real for prose and for identifiers alike: a
// directory name may legally contain a newline.
//
// Do not merge these two functions back into one policy.

// sanitizeProtocolIdentifier flattens line breaks in a value the model is told
// to reproduce verbatim as a lookup key. It never shortens the value.
func sanitizeProtocolIdentifier(value string) string {
	return strings.TrimSpace(flattenLineBreaks(value))
}

// sanitizeProtocolProse flattens line breaks and bounds the length of a value
// shown to a human.
//
// The value reaching BuildMigrationBlockedProtocol is NOT locally authored: it
// is the daemon's migration error, which names the conflicting project key, and
// projectidentity.Canonical trims only the ENDS of a project name, so a
// teammate's memory whose project carries an interior newline survives
// canonicalization and reaches memories.project. From there a migration conflict
// names it, status.Reason becomes err.Error(), and this hook injects it into
// every Claude Code SessionStart on this machine. An error naming thousands of
// conflicting keys would otherwise be pasted into every session's context in
// full. Truncation is on a rune boundary so the result is never invalid UTF-8.
func sanitizeProtocolProse(value string, limit int) string {
	safe := strings.TrimSpace(flattenLineBreaks(value))
	if len(safe) <= limit {
		return safe
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(safe[cut]) {
		cut--
	}
	return strings.TrimSpace(safe[:cut]) + ProtocolValueTruncated
}

func flattenLineBreaks(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, value)
}

// BuildMigrationProtocol renders the notice appended to the session-start context
// while Hive is not serving. The state, reason, and backup id come from the daemon
// over HTTP and are prose a human reads, so all three go through
// sanitizeProtocolProse. The continuation does not come from the daemon at all.
//
// The state is no longer hardcoded because there is more than one non-serving
// state, and they need different words: a failed migration may have left the
// database in any shape, while a pending operator review never wrote to it at
// all. Naming the wrong one sends the operator to the wrong surface.
func BuildMigrationProtocol(status MigrationStatus) string {
	state := sanitizeProtocolProse(status.State, maxProtocolStateLength)
	notice := migrationProtocolHeading(state) + "\n\n" +
		"State: " + state + "\n" +
		"Reason: " + sanitizeProtocolProse(status.Reason, maxProtocolReasonLength) + "\n"
	// A pending review took no archive because it attempted no migration, so an
	// empty Backup line would only suggest a rollback artifact went missing.
	if state != MigrationStatePendingOperatorReview {
		notice += "Backup: " + sanitizeProtocolProse(status.BackupID, maxProtocolBackupIDLength) + "\n"
	}
	return notice + "Continue with: " + migrationProtocolContinuation(state)
}

// migrationProtocolHeading names the state in the operator's terms. Anything this
// build does not recognize gets the cautious heading, never the reassuring one.
func migrationProtocolHeading(state string) string {
	if state == MigrationStatePendingOperatorReview {
		return "## Hive Project Normalization — Waiting For Operator Decision"
	}
	return "## Hive Migration Blocked"
}

// migrationProtocolContinuation derives the next step from the state alone. The
// daemon's own status.Continuation is deliberately NOT used, and no amount of
// sanitizing would make it safe to use.
//
// "Continue with:" is a command the model, and through it the operator, may act
// on. Commit 9af78aa9 ("fix(hive): secure global context hooks") established that
// the daemon's continuation is untrusted text and must never be rendered into an
// agent's context — its test feeds "attacker-controlled-continuation" through the
// OpenCode plugin and asserts it never appears. This notice goes into the same
// kind of context, so it holds the same rule.
//
// The state is safe to route on where the continuation is not, because the state
// is a small closed set this package validates against its own constants: an
// unrecognized value cannot invent a new command, it only selects the cautious
// one.
func migrationProtocolContinuation(state string) string {
	if state == MigrationStatePendingOperatorReview {
		return MigrationNormalizationCommand
	}
	return MigrationStatusCommand
}
