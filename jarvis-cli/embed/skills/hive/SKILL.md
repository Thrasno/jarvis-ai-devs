# Hive Memory — Complete Usage Protocol

## Section 1 — MCP Tool Reference

### mcp__hive__mem_save

**Purpose**: Save a memory observation to Hive persistent storage.

**Parameters**:
- `title` (required): Short searchable title. Format: "Verb + what" (e.g., "Fixed race condition in sync loop")
- `content` (required): Full memory content. Markdown OK.
- `type` (required): `bugfix | decision | architecture | discovery | pattern | config | preference | session_summary`
- `project` (required): Project identifier. NEVER omit. Example: `"jarvis-dev"`
- `topic_key` (recommended): Stable key for upsert. Same key overwrites previous observation. Example: `"sdd/my-change/spec"`
- `files_affected` (optional): Array of file paths affected
- `tags` (optional): Array of tags for categorization

**Example — saving an SDD artifact**:
```
mcp__hive__mem_save(
  title: "Spec: user-auth-flow",
  topic_key: "sdd/user-auth-flow/spec",
  type: "architecture",
  project: "my-project",
  content: "# Spec: user-auth-flow\n\n## Requirements\n..."
)
```

---

### mcp__hive__mem_search

**Purpose**: Search memories using full-text BM25 ranking.

**Parameters**:
- `query` (required): Search terms. Can be a topic key or keywords.
- `project` (optional): Filter by project. Use this to scope results.
- `limit` (optional): Max results. Default 10, max 50.

**Returns**: Truncated list with observation IDs. ALWAYS follow with `mcp__hive__mem_get_observation` to get full content.

**Example**:
```
mcp__hive__mem_search(query: "sdd/user-auth-flow/spec", project: "my-project")
→ returns list with IDs
→ then call mcp__hive__mem_get_observation(id: {id}) for full content
```

---

### mcp__hive__mem_get_observation

**Purpose**: Retrieve the full untruncated content of a specific memory by its numeric ID.

**Parameters**:
- `id` (required): Observation ID from mem_search results.

**When to use**: ALWAYS after `mcp__hive__mem_search` when you need to read the full content. Search results are truncated. Never assume you have the full content from search alone.

**Example**:
```
mcp__hive__mem_get_observation(id: 42)
→ returns full memory content
```

---

### mcp__hive__mem_update

**Purpose**: Update an existing observation by its numeric ID.

**Parameters**:
- `id` (required): Observation ID to update.
- `content` (optional): New content to replace existing.
- Other fields can be updated as needed.

**When to use**: When you need to update task checkboxes, correct a previous save, or append to an existing observation. Most commonly used in sdd-apply to mark tasks `[x]` as complete. Store the tasks observation ID at the start of sdd-apply as `{tasks-obs-id}` and call `mem_update(id: {tasks-obs-id})` after each task.

---

### mcp__hive__mem_context

**Purpose**: Get recent memory context for a project, ordered by recency. Fast and cheap — check this BEFORE mem_search for "what did we do" queries.

**Parameters**:
- `project` (optional): Filter by project.
- `limit` (optional): Max results. Default 20.

**When to use**: At the start of a session to recover recent context. Before mem_search when looking for recent work.

---

### mcp__hive__mem_session_summary

**Purpose**: Save a session summary. Call at session end or before compaction to preserve work.

**Parameters**: Same as mem_save. Use `type: "session_summary"`.

**When to use**: Before ending a session, before saying "done", or immediately when you see a compaction message.

---

### mcp__hive__mem_sync

**Purpose**: Sync local memories to the Hive API.

**Parameters**:
- `project` (required — always include): Project identifier.

**When to use**: After significant saves, at session end, or when working with a team that shares the same project memory. The `project` field is always required — never call mem_sync without it.

---

## Section 2 — When to Save (Proactive Triggers)

Call `mcp__hive__mem_save` IMMEDIATELY and WITHOUT BEING ASKED after any of these:

1. Architecture or design decision made
2. Team convention documented or established
3. Workflow change agreed upon
4. Tool or library choice made with tradeoffs
5. Bug fix completed (include root cause in content)
6. Feature implemented with non-obvious approach
7. Notion/Jira/GitHub artifact created or updated with significant content
8. Configuration change or environment setup done
9. Non-obvious discovery about the codebase
10. Gotcha, edge case, or unexpected behavior found
11. Pattern established (naming, structure, convention)
12. User preference or constraint learned

**Self-check after EVERY task**: "Did I make a decision, fix a bug, learn something non-obvious, or establish a convention? If yes → call mem_save NOW."

---

## Section 3 — When NOT to Save (Noise Reduction)

Do NOT save:
- Ephemeral data (temporary calculations, intermediate reasoning steps)
- Obvious facts already in code comments or README
- Failed attempts that produced no learning
- Content that duplicates what is already in a spec or design artifact
- Operational chatter (log output, intermediate build steps)
- Questions you asked but have not answered yet

---

## Section 4 — topic_key Conventions

Format: `{domain}/{identifier}` or `{domain}/{change}/{phase}`

| topic_key | What it stores |
|-----------|----------------|
| `sdd-init/{project}` | Project context from sdd-init |
| `sdd/{change}/proposal` | Change proposal |
| `sdd/{change}/spec` | Requirements and scenarios |
| `sdd/{change}/design` | Technical design and architecture decisions |
| `sdd/{change}/tasks` | Implementation task checklist |
| `sdd/{change}/apply-progress` | Apply phase progress |
| `sdd/{change}/qa-checklist` | QA checklist with [AUTO]/[MANUAL] items |
| `sdd/{change}/qa-signoff` | QA sign-off confirmation |
| `sdd/{change}/verify-report` | Verification report |
| `sdd/{change}/archive-report` | Archive report with artifact IDs |
| `sdd/{project}/testing-capabilities` | Detected test runner and coverage tools |
| `skill-registry` | Project skill registry (compact rules) |
| `architecture/{topic}` | Architectural decisions by topic |
| `bugfix/{component}/{issue}` | Bug fix with root cause |
| `pattern/{name}` | Established patterns and conventions |
| `sessions/summary` | Session summaries |

**Rules**:
- `project` field is MANDATORY in every `mem_save` call — NEVER omit it
- Same `topic_key` = upsert (overwrites previous observation for that topic)
- Different topics MUST use different topic keys — never overwrite a different topic
- When unsure about the right key: call `mcp__hive__mem_suggest_topic_key` first

---

## Section 5 — Session Close Protocol

Before ending a session or saying "done", call `mcp__hive__mem_session_summary` with this content:

```markdown
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
```

This is NOT optional. If you skip this, the next session starts blind.

---

## Section 6 — After-Compaction Recovery

If you see a compaction message or "FIRST ACTION REQUIRED":

1. IMMEDIATELY call `mcp__hive__mem_session_summary` with the compacted summary content to persist what was done before compaction.
2. Call `mcp__hive__mem_context` to recover additional context from previous sessions.
3. Only THEN continue working.

Do not skip step 1. Without it, everything done before compaction is lost from memory.

---

## Section 7 — scope: project vs personal

- `project` (default): shared across all agents and sessions working on this project. Visible to the whole team. Use for code decisions, bugs, and conventions.
- `personal`: private to the saving agent or user. Use for cross-project notes and personal preferences.

Rule: ALWAYS use `project` scope for code decisions, bugs, and conventions. Use `personal` only for agent self-configuration.

---

## Section 8 — Team Memory Semantics

Multiple agents and developers share project memories via Hive. Memories saved with `project: "jarvis-dev"` are visible to all agents initialized with the same project name.

Use `mcp__hive__mem_sync(project: "{project}")` to push local memories to the shared store after significant saves or at session end.

This means any decision or discovery made in one session is available to the next agent or session working on the same project — no re-detection or re-explanation needed.
