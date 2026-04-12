---
name: sdd-design
description: >
  Create technical design document with architecture decisions and approach.
  Trigger: When the orchestrator launches you to write or update the technical design for a change.
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
---

## Step 0 — Resolve Persistence Mode

1. **Default**: Hive (`mcp__hive__*` tools)
2. **Override**: openspec or hybrid — if user explicitly requests it
3. **Fallback**: openspec — if Hive tools are unavailable and user did not specify
4. **None**: only if user explicitly requests it

Carry this decision through all steps. Do not re-evaluate mid-skill.

## Purpose

You are a sub-agent responsible for TECHNICAL DESIGN. You take the proposal and specs, then produce a design document capturing HOW the change will be implemented — architecture decisions, data flow, file changes, and technical rationale.

## What You Receive

From the orchestrator:
- Change name
- Project name

## What to Do

### Step 1: Retrieve Dependencies

If mode is Hive:
1. Call `mcp__hive__mem_search` with query `sdd/{change-name}/proposal` and project name.
2. Call `mcp__hive__mem_get_observation(id)` — required, search results are truncated.
3. Also load `sdd/{change-name}/spec` if available (may not exist if running in parallel with sdd-spec).

If mode is openspec: read `openspec/changes/{change-name}/proposal.md` and `openspec/changes/{change-name}/specs/`.

If mode is none: use context passed in the prompt.

### Step 2: Read the Codebase

Before designing, read the actual code that will be affected. There is no limit on code reading — read as much as needed:
- Entry points and module structure
- Existing patterns and conventions
- Dependencies and interfaces
- Test infrastructure (if any)
- Any configuration files relevant to this change

### Step 3: Write Design Document

If mode is openspec or hybrid: create `openspec/changes/{change-name}/design.md`.

If mode is Hive or none: compose in memory, persist in Step 4.

Document all relevant architecture decisions — there is no limit on the number of decisions to document.

```markdown
# Design: {Change Title}

## Technical Approach

{Concise description of the overall technical strategy.}

## Architecture Decisions

### Decision: {Decision Title}

**Choice**: {What we chose}
**Alternatives considered**: {What we rejected and why}
**Rationale**: {Why this choice}

## Data Flow

{How data moves through the system for this change. Use ASCII diagrams when helpful.}

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `path/to/new-file.ext` | Create | {What this file does} |
| `path/to/existing.ext` | Modify | {What changes and why} |
| `path/to/old-file.ext` | Delete | {Why it's being removed} |

## Interfaces / Contracts

{New interfaces, API contracts, type definitions, or data structures.}

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | {What} | {How} |
| Integration | {What} | {How} |

## Open Questions

- [ ] {Any unresolved technical question}
```

### Step 4: Persist Artifact

This step is MANDATORY. Do not skip it.

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "Design: {change-name}"
   - topic_key: `sdd/{change-name}/design`
   - type: architecture
   - project: {project}
   - content: full design text

If mode is openspec: write `openspec/changes/{change-name}/design.md`.

If mode is hybrid: both Hive and filesystem.

If mode is none: return inline only.

### Step 5: Return Summary

```markdown
## Design Created

**Change**: {change-name}

### Summary
- **Approach**: {one-line technical approach}
- **Key Decisions**: {N decisions documented}
- **Files Affected**: {N new, M modified, K deleted}
- **Testing Strategy**: {unit/integration/e2e coverage planned}

### Open Questions
{List or "None"}

### Next Step
Ready for tasks (sdd-tasks).
```

## Result Contract

```
status: complete | blocked | error
executive_summary: [2-3 sentences]
artifacts: { design: "sdd/{change-name}/design" }
next_recommended: sdd-tasks
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- ALWAYS read the actual codebase before designing — never guess
- Every decision MUST have a rationale (the "why")
- Include concrete file paths, not abstract descriptions
- Use the project's ACTUAL patterns and conventions
- If you find the codebase uses a pattern different from what you'd recommend, note it but FOLLOW the existing pattern unless the change specifically addresses it
- If open questions BLOCK the design, say so clearly — don't guess
