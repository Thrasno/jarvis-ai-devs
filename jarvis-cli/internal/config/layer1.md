### PROJECT CONTEXT (run at session start)

Detect the active project name at the START of EVERY session:
1. Run: `git remote get-url origin` → extract repo name (last path segment, strip `.git`)
2. Fallback: basename of the current working directory
3. Fallback: "default"

Use the resolved project name as the `project` field in ALL `mem_save` calls.
NEVER save a memory without a `project` field.

---

## Hive Persistent Memory — Protocol

You have access to Hive, a persistent memory system via MCP tools.
This protocol is MANDATORY and ALWAYS ACTIVE — not something you activate on demand.

### PROACTIVE SAVE TRIGGERS (mandatory — do NOT wait for user to ask)

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

### Format for mem_save

- **title**: Verb + what — short, searchable (e.g. "Fixed N+1 query in UserList")
- **type**: bugfix | decision | architecture | discovery | pattern | config | preference
- **scope**: `project` (default) | `personal`
- **topic_key** (recommended for evolving topics): stable key like `architecture/auth-model`
- **content**:
  - **What**: One sentence — what was done
  - **Why**: What motivated it (user request, bug, performance, etc.)
  - **Where**: Files or paths affected
  - **Learned**: Gotchas, edge cases, things that surprised you (omit if none)

### Topic Update Rules

- Different topics MUST NOT overwrite each other
- Same topic evolving → use same `topic_key` (upsert)
- Unsure about key → call `mem_suggest_topic_key` first
- Know exact ID to fix → use `mem_update`

### WHEN TO SEARCH MEMORY

On any variation of "remember", "recall", "what did we do", "how did we solve":
1. Call `mem_context` — checks recent session history (fast, cheap)
2. If not found, call `mem_search` with relevant keywords
3. If found, use `mem_get_observation` for full untruncated content

Also search PROACTIVELY when:
- Starting work on something that might have been done before
- User mentions a topic you have no context on
- User's FIRST message references the project, a feature, or a problem — call `mem_search` with keywords from their message to check for prior work before responding

### SESSION CLOSE PROTOCOL (mandatory)

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

### AFTER COMPACTION

If you see a compaction message or "FIRST ACTION REQUIRED":
1. IMMEDIATELY call `mem_session_summary` with the compacted summary content — this persists what was done before compaction
2. Call `mem_context` to recover additional context from previous sessions
3. Only THEN continue working

Do not skip step 1. Without it, everything done before compaction is lost from memory.

### Hive-specific features

- The `project` field is MANDATORY in ALL `mem_save` calls — NEVER save without it
- Call `mem_sync` after significant session work to trigger bidirectional cloud sync
- Team memory: memories with `project` scope are shared across all team members via hive-api

### SDD Orchestrator (model assignments)

| Phase | Model |
|-------|-------|
| orchestrator | opus |
| sdd-explore | sonnet |
| sdd-propose | opus |
| sdd-spec | sonnet |
| sdd-design | opus |
| sdd-tasks | sonnet |
| sdd-apply | sonnet |
| sdd-verify | sonnet |
| sdd-archive | haiku |

Sub-agent launch pattern: delegate reads of 4+ files, multi-file writes, and test runs to sub-agents. Resolve skills from the registry ONCE per session, cache compact rules, inject into sub-agent prompts as `## Project Standards (auto-resolved)`. Sub-agents do NOT read SKILL.md files — rules arrive pre-digested.

SDD DAG with sdd-qa: `proposal → specs → tasks → apply → sdd-qa → verify → archive`
(sdd-qa = manual behavior acceptance between apply and verify)

## Skills (Auto-load based on context)

When you detect any of these contexts, IMMEDIATELY read the corresponding skill file BEFORE writing any code.

| Context | Read this file |
| ------- | -------------- |
| Go tests, Bubbletea TUI testing | `~/.claude/skills/go-testing/SKILL.md` |
| Creating new AI skills | `~/.claude/skills/skill-creator/SKILL.md` |
| Zoho Deluge scripts | `~/.claude/skills/zoho-deluge/SKILL.md` |
| Laravel projects | `~/.claude/skills/laravel-architecture/SKILL.md` |
| PHP / Laravel tests | `~/.claude/skills/phpunit-testing/SKILL.md` |

Read skills BEFORE writing code. Apply ALL patterns. Multiple skills can apply simultaneously.
