# Hive Persistent Memory — Protocol

You have access to Hive, a persistent memory system that survives across sessions and compactions.
This protocol is MANDATORY and ALWAYS ACTIVE — not something you activate on demand.

## SESSION START (mandatory — do this FIRST, before anything else)

Call `mem_context` at the start of every session before responding to the user.

This is required because `mem_context` registers the current project with Hive. Without it,
any subsequent `mem_save` call will fail with a `project_unknown` error.

If `mem_context` is not yet available as a tool, call `ToolSearch` with query `"mem_context"` to load it first.

## PROACTIVE SAVE TRIGGERS (mandatory — do NOT wait for user to ask)

Call `mem_save` IMMEDIATELY and WITHOUT BEING ASKED after any of these:
- Architecture or design decision made
- Team convention documented or established
- Workflow change agreed upon
- Tool or library choice made with tradeoffs
- Bug fix completed (include root cause)
- Feature implemented with non-obvious approach
- Notion/Jira/GitHub artifact created or updated with significant content
- Configuration change or environment setup done
- Non-obvious discovery about the codebase
- Gotcha, edge case, or unexpected behavior found
- Pattern established (naming, structure, convention)
- User preference or constraint learned

Self-check after EVERY task: "Did I make a decision, fix a bug, learn something non-obvious, or establish a convention? If yes, call mem_save NOW."

## FORMAT FOR mem_save

- **title**: Verb + what — short, searchable (e.g. "Fixed N+1 query in UserList")
- **type**: bugfix | decision | architecture | discovery | pattern | config | preference
- **scope**: `project` (default) | `personal`
- **topic_key** (recommended for evolving topics): grouping/context key like `architecture/auth-model`
- **capture_prompt** (optional): defaults to `true` for human/proactive saves. Set `false` for automated artifacts such as SDD outputs, generated config, registry updates, and machine-produced reports.
- **content**:
  - **What**: One sentence — what was done
  - **Why**: What motivated it (user request, bug, performance, etc.)
  - **Where**: Files or paths affected
  - **Learned**: Gotchas, edge cases, things that surprised you (omit if none)

Topic update rules:
- Different topics MUST NOT overwrite each other
- Same topic evolving → reuse the same `topic_key` to group related observations.
- If search returns multiple candidates for the same `topic_key`, treat retrieval as ambiguous unless an explicit observation ID or artifact reference is available; do not infer latest or authoritative meaning from ordering.
- Unsure about key → call `mem_suggest_topic_key` first
- Observations saved WITHOUT a `topic_key` are append-only — never update them by ID; save a new observation instead

## WHEN TO SEARCH MEMORY

On any variation of "remember", "recall", "what did we do", "how did we solve", "recordar", "qué hicimos", or references to past work:
1. Call `mem_context` — checks recent session history (fast, cheap)
2. If not found, call `mem_search` with relevant keywords
3. If found, use `mem_get_observation` for full untruncated content

Also search PROACTIVELY when:
- Starting work on something that might have been done before
- User mentions a topic you have no context on
- User's FIRST message references the project, a feature, or a problem — call `mem_search` with keywords from their message to check for prior work before responding

## SDD ARTIFACT BOUNDARY (MVP CONTRACT)

- Reserved namespace: `sdd/{change}/{artifact}` is for automated SDD artifacts only.
- Automated SDD/config artifact saves MUST pass `capture_prompt:false`; they are pipeline outputs, not human memories.
- General/conversational memory MUST use non-`sdd/` topics.
- General memory entries MUST NOT reuse or pollute reserved SDD artifact topic keys.
- In `hybrid` mode, artifact payload semantics must remain equivalent across both destinations.

## SESSION CLOSE PROTOCOL (mandatory)

Before ending a session or saying "done" / "listo" / "that's it", call `mem_session_summary`:

## Goal
[What we were working on this session]

## Instructions
[User preferences or constraints discovered — skip if none]

## Discoveries
- [Technical findings, gotchas, non-obvious learnings]

## Accomplished
- [Completed items with key details]

## Next Steps
- [What remains to be done — for the next session]

## Relevant Files
- path/to/file — [what it does or what changed]

This is NOT optional. If you skip this, the next session starts blind.

## AFTER COMPACTION

If you see a compaction message or "FIRST ACTION REQUIRED":
1. IMMEDIATELY call `mem_session_summary` with the compacted summary content — this persists what was done before compaction
2. Call `mem_context` to recover additional context from previous sessions
3. Only THEN continue working

Do not skip step 1. Without it, everything done before compaction is lost from memory.

## AUTO-REMINDER SYSTEM

Hive uses an automatic MCP nudge system instead of an agent-side timer:
- **Trigger**: Every 5 tool calls (`mem_search`, `mem_context`, `mem_get_observation`) without a `mem_save`
- **No timer**: Do not create a separate 15-minute reminder loop. There is intentionally no agent-side 15-minute timer because noisy timers interrupt fast coding sessions and duplicate the MCP nudge.
- **Message**: Appears inline in tool responses with semantic pattern hints (e.g., "look for: agreement patterns like 'let's do', 'yes, go ahead'")
- **Reset**: Calling `mem_save` or `mem_session_summary` resets the counter

## AUTO-SYNC

Cloud sync is NOT always automatic.

- Default mode: manual sync using `mem_sync` when needed.
- Optional mode: background auto-sync only when `auto_sync` (or `HIVE_AUTO_SYNC`) is explicitly enabled.
- `jarvis sync` does not perform transport sync; use MCP tools inside your agent.
