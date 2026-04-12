# SDD Phase Common — Shared Instructions for All SDD Phases

## Section A — Load Project Standards

The orchestrator has already resolved and injected project-specific coding standards as "Project Standards" in your prompt. Apply them throughout your work. Do not re-read skill files or the registry yourself — rules arrive pre-digested from the orchestrator.

## Section B — How to Load Context from Hive

Use this two-step process for any artifact you need to read:

1. Call `mcp__hive__mem_search(query: "{topic_key}", project: "{project}")` to find the observation ID.
2. Call `mcp__hive__mem_get_observation(id: {id})` to get the FULL content.

Search results are ALWAYS truncated. Step 2 is REQUIRED — never assume you have the full content from search alone.

## Section C — How to Save Artifacts to Hive

When saving an SDD artifact:

1. Call `mcp__hive__mem_save` with:
   - `title`: "Phase: {change-name}" (e.g., "Spec: user-auth-flow")
   - `topic_key`: per SDD convention (e.g., `sdd/{change-name}/spec`)
   - `type`: `architecture`
   - `project`: {project name} — REQUIRED, never omit
   - `content`: full artifact text

Same `topic_key` = upsert (safe to call multiple times, only the last version is kept).

## Section D — Result Contract (ALL phases MUST return this)

Every SDD phase MUST return this exact envelope to the orchestrator:

```
status: complete | partial | blocked | needs-confirmation | error
executive_summary: [2-3 sentences describing what was done]
artifacts: { {phase}: "{topic_key}" }
next_recommended: {name of next phase}
risks: [list of risks or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

- `status: blocked` — a prerequisite is missing and the phase cannot proceed
- `status: needs-confirmation` — the phase has paused and needs a user decision before continuing
- `skill_resolution`: report whether compact rules were injected (`injected`), came from a fallback source, or were absent (`none`)

## Phase Dependency Table

| Phase | Required Inputs | Writes |
|-------|----------------|--------|
| sdd-explore | sdd-init/{project} (optional) | sdd/{change}/explore |
| sdd-propose | explore (optional) | sdd/{change}/proposal |
| sdd-spec | proposal (required) | sdd/{change}/spec |
| sdd-design | proposal (required), spec (optional) | sdd/{change}/design |
| sdd-tasks | spec (required), design (required) | sdd/{change}/tasks |
| sdd-apply | tasks (required), spec, design | sdd/{change}/apply-progress |
| sdd-qa | spec, apply-progress | sdd/{change}/qa-checklist, qa-signoff |
| sdd-verify | spec (required), tasks | sdd/{change}/verify-report |
| sdd-archive | all artifacts + qa-signoff | sdd/{change}/archive-report |

If a required input is missing, return `status: blocked` with a message naming the missing artifact.
