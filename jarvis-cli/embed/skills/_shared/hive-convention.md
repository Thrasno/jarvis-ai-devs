# Hive Convention — topic_key and Project Field Rules

<!-- gentle-ai v2.1.5 selective sync: Hive naming and automated-artifact persistence conventions. -->

## topic_key Naming Format

Format: `{domain}/{identifier}` or `{domain}/{change}/{phase}`

| topic_key pattern | Used for |
|-------------------|----------|
| `sdd-init/{project}` | Project context saved by sdd-init |
| `sdd/{change}/explore` | Exploration analysis |
| `sdd/{change}/proposal` | Change proposal |
| `sdd/{change}/spec` | Requirements and scenarios |
| `sdd/{change}/design` | Technical design |
| `sdd/{change}/tasks` | Task checklist |
| `sdd/{change}/apply-progress` | Apply phase progress |
| `sdd/{change}/verify-report` | Verification report |
| `sdd/{change}/archive-report` | Archive report |
| `sdd/{project}/testing-capabilities` | Detected test runner and tools |
| `skill-registry` | Project skill registry |
| `architecture/{topic}` | Architecture decisions |
| `bugfix/{component}/{issue}` | Bug fixes |
| `sessions/summary` | Session summaries |

## The project Field is MANDATORY

Every `mcp__hive__mem_save` call MUST include the `project` field.

NEVER save a memory without a `project` field. Without it, the memory cannot be found by project-scoped searches and may pollute other projects' results.

Examples:
- Correct: `mcp__hive__mem_save(project: "jarvis-dev", ...)`
- Wrong: `mcp__hive__mem_save(title: "...", content: "...")` — missing project

## Grouping Semantics

Saving with the same `topic_key` adds a NEW grouped observation; the previous one is kept. Search results are previews, not source material. If Hive search returns multiple candidate artifacts for the same topic and no explicit artifact reference is available, treat the result as ambiguous.

Rule: **Different topics MUST NOT share the same topic_key.** Saving with the same topic_key adds a new grouped observation; the previous one is kept. Use an explicit observation ID or orchestrator-provided artifact reference when a specific saved artifact is required.

If you are unsure whether your topic_key is correct or conflicts with an existing key: call `mcp__hive__mem_suggest_topic_key` first. It will suggest a key based on existing conventions and warn about conflicts.

## Two-Step Retrieval

`mcp__hive__mem_search` returns truncated results. ALWAYS follow with `mcp__hive__mem_get_observation(id)` to get full content.

```
Step 1: mcp__hive__mem_search(query: "{topic_key}", project: "{project}")
         → returns list with IDs
Step 2: mcp__hive__mem_get_observation(id: {id from step 1})
         → returns FULL untruncated content
```

Never assume you have the full content from a search result alone.
