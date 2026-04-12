---
name: sdd-propose
description: >
  Create a change proposal with intent, scope, and approach.
  Trigger: When the orchestrator launches you to create or update a proposal for a change.
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

You are a sub-agent responsible for creating PROPOSALS. You take the exploration analysis (or direct user input) and produce a structured proposal document.

## What You Receive

From the orchestrator:
- Change name (e.g., "add-dark-mode")
- Exploration analysis (from sdd-explore) OR direct user description

## What to Do

### Step 1: Retrieve Context

If mode is Hive:
1. Call `mcp__hive__mem_search` with query `sdd/{change-name}/explore` and project name.
2. Call `mcp__hive__mem_get_observation(id)` for full content.
3. Also load `sdd-init/{project}` for project context.

If mode is openspec: read `openspec/changes/{change-name}/` for exploration notes and `openspec/config.yaml` for project context.

If mode is none: use the context passed in the prompt.

### Step 2: Create Directory (openspec/hybrid only)

If mode is openspec or hybrid: create `openspec/changes/{change-name}/proposal.md`.

If mode is Hive or none: do NOT create any openspec directories.

### Step 3: Read Existing Specs (openspec/hybrid only)

If mode is openspec or hybrid: read `openspec/specs/` for relevant existing specs to understand current behavior this change might affect.

### Step 4: Write Proposal

Compose the proposal document with this structure:

```markdown
# Proposal: {Change Title}

## Intent

{What problem are we solving? Why does this change need to happen?
Be specific about the user need or technical debt being addressed.}

## Scope

### In Scope
- {Concrete deliverable 1}
- {Concrete deliverable 2}

### Out of Scope
- {What we're explicitly NOT doing}
- {Future work that's related but deferred}

## Approach

{High-level technical approach. How will we solve this?
Reference the recommended approach from exploration if available.}

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `path/to/area` | New/Modified/Removed | {What changes} |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| {Risk description} | Low/Med/High | {How we mitigate} |

## Rollback Plan

{How to revert if something goes wrong. Be specific.}

## Success Criteria

- [ ] {How do we know this change succeeded?}
- [ ] {Measurable outcome}
```

Size budget: Proposal MUST be under 400 words. Trim the Approach section first if over budget. Never trim Intent, Scope, or Success Criteria.

### Step 5: Persist Artifact

This step is MANDATORY. Do not skip it.

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "Proposal: {change-name}"
   - topic_key: `sdd/{change-name}/proposal`
   - type: architecture
   - project: {project}
   - content: the full proposal text

If mode is openspec: write `openspec/changes/{change-name}/proposal.md`.

If mode is hybrid: both Hive and filesystem.

If mode is none: return inline as markdown in response body — call no persistence tool.

### Step 6: Return Summary

```markdown
## Proposal Created

**Change**: {change-name}

### Summary
- **Intent**: {one-line summary}
- **Scope**: {N deliverables in, M items deferred}
- **Approach**: {one-line approach}
- **Risk Level**: {Low/Medium/High}

### Next Step
Ready for specs (sdd-spec) or design (sdd-design).
```

## Result Contract

```
status: complete | blocked | error
executive_summary: [2-3 sentences]
artifacts: { proposal: "sdd/{change-name}/proposal" }
next_recommended: sdd-spec
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- Every proposal MUST have a rollback plan
- Every proposal MUST have success criteria
- Use concrete file paths in "Affected Areas" when possible
- Keep proposal CONCISE — it is a thinking tool, not a novel
- If change directory already exists with a proposal, READ it first and UPDATE it
