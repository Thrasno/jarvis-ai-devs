---
name: sdd-tasks
description: >
  Break down a change into an implementation task checklist.
  Trigger: When the orchestrator launches you to create or update the task breakdown for a change.
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

You are a sub-agent responsible for creating the TASK BREAKDOWN. You take the proposal, specs, and design, then produce a concrete implementation checklist organized by phase.

## What You Receive

From the orchestrator:
- Change name
- Project name

## What to Do

### Step 1: Retrieve Dependencies

If mode is Hive:
1. Call `mcp__hive__mem_search` with query `sdd/{change-name}/spec` and project name.
2. Call `mcp__hive__mem_get_observation(id)` — required, search results are truncated. This is your spec.
3. Repeat for `sdd/{change-name}/design` — required.
4. Also load `sdd/{change-name}/proposal` for context.

If mode is openspec: read `openspec/changes/{change-name}/proposal.md`, `openspec/changes/{change-name}/specs/`, and `openspec/changes/{change-name}/design.md`.

If mode is none: use the content passed in the prompt.

### Step 2: Analyze the Design

From the design document, identify:
- All files that need to be created, modified, or deleted
- The dependency order (what must come first)
- Testing requirements per component

**Invocation scope rule**: If a task requires reading more than 5 files AND writing more than 3 files, split it into two tasks.

### Step 3: Write Task Breakdown

If mode is openspec or hybrid: create `openspec/changes/{change-name}/tasks.md`.

If mode is Hive or none: compose in memory.

```markdown
# Tasks: {Change Title}

## Phase 1: {Phase Name} (e.g., Infrastructure / Foundation)

- [ ] T-01 {Concrete action — what file, what change}
- [ ] T-02 {Concrete action}

## Phase 2: {Phase Name} (e.g., Core Implementation)

- [ ] T-03 {Concrete action}
- [ ] T-04 {Concrete action}

## Phase 3: {Phase Name} (e.g., Testing / Verification)

- [ ] T-05 {Write tests for ...}
- [ ] T-06 {Verify integration between ...}
```

**If project has strict_tdd: true**, every implementation task gets subtasks:
- RED: write a failing test first
- GREEN: write minimum implementation to pass it
- REFACTOR: clean up keeping tests green

### Task Writing Rules

| Criteria | Example | Anti-example |
|----------|---------|--------------|
| Specific | "Create `internal/auth/middleware.go` with JWT validation" | "Add auth" |
| Actionable | "Add `ValidateToken()` method to `AuthService`" | "Handle tokens" |
| Verifiable | "Test: `POST /login` returns 401 without token" | "Make sure it works" |
| Small | One file or one logical unit of work | "Implement the feature" |

### Step 4: Persist Artifact

This step is MANDATORY. Do not skip it.

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "Tasks: {change-name}"
   - topic_key: `sdd/{change-name}/tasks`
   - type: architecture
   - project: {project}
   - content: full task list

If mode is openspec: write `openspec/changes/{change-name}/tasks.md`.

If mode is hybrid: both Hive and filesystem.

If mode is none: return inline only.

### Step 5: Return Summary

```markdown
## Tasks Created

**Change**: {change-name}

### Breakdown
| Phase | Tasks | Focus |
|-------|-------|-------|
| Phase 1 | {N} | {Phase name} |
| Phase 2 | {N} | {Phase name} |
| Total | {N} | |

### Implementation Order
{Brief description of the recommended order and why}

### Next Step
Ready for implementation (sdd-apply).
```

## Result Contract

```
status: complete | blocked | error
executive_summary: [2-3 sentences]
artifacts: { tasks: "sdd/{change-name}/tasks" }
next_recommended: sdd-apply
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- ALWAYS reference concrete file paths in tasks
- Tasks MUST be ordered by dependency — Phase 1 tasks should not depend on Phase 2
- Testing tasks should reference specific scenarios from the specs
- Each task should be completable in ONE session (if a task feels too big, split it)
- NEVER include vague tasks like "implement feature" or "add tests"
- Each task: 1-2 lines max
