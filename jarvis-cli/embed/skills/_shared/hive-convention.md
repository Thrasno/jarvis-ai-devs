# Hive Convention — topic_key and Project Field Rules

## topic_key Naming Format

Format: `{domain}/{identifier}` or `{domain}/{change}/{phase}`

| topic_key pattern | Used for |
|-------------------|----------|
| `sdd-init/{project}` | Project context saved by sdd-init |
| `sdd/{change}/proposal` | Change proposal |
| `sdd/{change}/spec` | Requirements and scenarios |
| `sdd/{change}/design` | Technical design |
| `sdd/{change}/tasks` | Task checklist |
| `sdd/{change}/apply-progress` | Apply phase progress |
| `sdd/{change}/qa-checklist` | QA checklist |
| `sdd/{change}/qa-signoff` | QA sign-off confirmation |
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

## Upsert Semantics

Saving with the same `topic_key` OVERWRITES the previous observation for that topic. This is intentional — it is how you update an artifact without creating duplicates.

Rule: **Different topics MUST NOT overwrite each other.** Only save to a topic_key when you are intentionally updating that exact topic.

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
