---
name: sdd-tasks
display_name: "SDD Tasks"
description: "Break down a change into a concrete, ordered implementation checklist. Trigger: When creating task lists"
disable-model-invocation: true
user-invocable: false
license: MIT
scope: core
metadata:
  author: gentleman-programming
  version: "2.0"
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/sdd-tasks/SKILL.md (tag v1.26.5, commit 5f73974b39ae2b9b525ef465b3642030c5f2ce6c); adapted for Jarvis/Hive runtime semantics. -->

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-tasks` executor using your platform's delegation primitive.
> This skill is for EXECUTORS only.

## Executor Override

If you ARE the `sdd-tasks` executor, the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.

## Purpose

You are a sub-agent responsible for creating the TASK BREAKDOWN. You take the proposal, specs, and design, then produce a `tasks.md` with concrete, actionable implementation steps organized by phase.

## What You Receive

From the orchestrator:

- Change name
- Artifact store mode (`hive | openspec | hybrid | none`)
- Preflight decisions for the feature (the `## SDD Decisions` block): delivery strategy (`auto-chain | exception-ok`), `review_budget_lines` (budget N, absent under `size:exception`), chain strategy (`stacked-to-main | feature-branch-chain | size:exception`), and TDD mode (`strict | standard`)

These decisions are final for the feature. Consume them; never ask the user for a chain strategy, a size exception, or a budget.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **hive**: Read `sdd/{change-name}/proposal` (required), `sdd/{change-name}/spec` (required), `sdd/{change-name}/design` (required). Save as `sdd/{change-name}/tasks`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`.
- **hybrid**: Follow BOTH conventions — persist to Hive AND write `tasks.md` to filesystem. Retrieve dependencies from Hive (primary) with filesystem fallback.
- **none**: Return result only. Never create or modify project files.

## What to Do

### Step 1: Load Skills

Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Analyze the Design

From the design document, identify:

- All files that need to be created/modified/deleted
- The dependency order (what must come first)
- Testing requirements per component

### Step 3: Write tasks.md

**IF mode is `openspec` or `hybrid`:** Create the task file:

```
openspec/changes/{change-name}/
├── proposal.md
├── specs/
├── design.md
└── tasks.md               ← You create this
```

**IF mode is `hive` or `none`:** Do NOT create any `openspec/` directories or files. Compose the tasks content in memory — you will persist it in Step 4.

#### Task File Format

```markdown
# Tasks: {Change Title}

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | <rough estimate or range> |
| Review budget | <forwarded N changed lines> |
| Budget risk | Low / Medium / High |
| Chained PRs recommended | Yes / No |
| Suggested split | <single PR or PR 1 → PR 2 → PR 3> |
| Delivery strategy | <auto-chain / exception-ok> |
| Chain strategy | <stacked-to-main / feature-branch-chain / size:exception / pending> |

Decision needed before apply: <one of: Yes, No>
Chained PRs recommended: <one of: Yes, No>
Chain strategy: <one of: stacked-to-main, feature-branch-chain, size:exception, pending>
Budget risk: <one of: Low, Medium, High>

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | <standalone deliverable> | PR 1 | <base branch; tests/docs included> |
| 2 | <standalone deliverable> | PR 2 | <immediate parent/base branch boundary; depends on PR 1 or independent> |

## Phase 1: {Phase Name} (e.g., Infrastructure / Foundation)

- [ ] 1.1 {Concrete action — what file, what change}
- [ ] 1.2 {Concrete action}
- [ ] 1.3 {Concrete action}

## Phase 2: {Phase Name} (e.g., Core Implementation)

- [ ] 2.1 {Concrete action}
- [ ] 2.2 {Concrete action}
- [ ] 2.3 {Concrete action}
- [ ] 2.4 {Concrete action}

## Phase 3: {Phase Name} (e.g., Testing / Verification)

- [ ] 3.1 {Write tests for ...}
- [ ] 3.2 {Write tests for ...}
- [ ] 3.3 {Verify integration between ...}

## Phase 4: {Phase Name} (e.g., Cleanup / Documentation)

- [ ] 4.1 {Update docs/comments}
- [ ] 4.2 {Remove temporary code}
```

### Task Writing Rules

Each task MUST be:

| Criteria | Example ✅ | Anti-example ❌ |
| ---------- | ----------- | ---------------- |
| **Specific** | "Create `internal/auth/middleware.go` with JWT validation" | "Add auth" |
| **Actionable** | "Add `ValidateToken()` method to `AuthService`" | "Handle tokens" |
| **Verifiable** | "Test: `POST /login` returns 401 without token" | "Make sure it works" |
| **Small** | One file or one logical unit of work | "Implement the feature" |

### Review Workload Forecast Rules

Apply the forwarded size policy. Never ask the user; the preflight already decided.

**`size:exception`** (`exception-ok`): skip the line forecast entirely. Do not estimate lines, rate budget risk, or split into work units. Write only these guard lines:

```text
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size:exception
```

**Budget N** (`auto-chain` with `review_budget_lines: N`): estimate whether implementation is likely to exceed the forwarded budget of N changed lines (`additions + deletions`). Never substitute a hard-coded budget. This is a planning guard, not an exact diff count. Use available signals: number of files, phases, integration points, tests, docs, generated artifacts, migrations, and how many concerns the change crosses.

- If the estimate is **High** or likely above N lines:
  1. Mark `Chained PRs recommended` as `Yes`.
  2. Split tasks into **work units** for the forwarded chain strategy (`stacked-to-main` or `feature-branch-chain`).
  3. Each suggested PR must have a clear start, clear finish, verification, and autonomous scope.
  4. Write `Decision needed before apply: No` and `Chain strategy: <forwarded value>`.
- If the estimate is within N lines: write `Chained PRs recommended: No`, `Decision needed before apply: No`, and `Chain strategy: <forwarded value>` (`stacked-to-main` when none was forwarded for a single in-budget PR).

**Decisions not forwarded** (legacy or headless launch with no `## SDD Decisions`): never guess. Write `Decision needed before apply: Yes` and `Chain strategy: pending` so the native gate blocks apply until the orchestrator runs the preflight.

Do not bury this in prose. Put the forecast near the top of the tasks artifact so the user sees it before implementation starts.

The forecast MUST include these plain-text lines so downstream guards can match them literally. Replace each `<one of: ...>` placeholder with exactly ONE chosen value; never copy the placeholder or an option list:

```text
Decision needed before apply: <one of: Yes, No>
Chained PRs recommended: <one of: Yes, No>
Chain strategy: <one of: stacked-to-main, feature-branch-chain, size:exception, pending>
Budget risk: <one of: Low, Medium, High>
```

Guard contract rules:

- Write exactly one chosen value per line, for example `Chain strategy: stacked-to-main`. Never write the option list (`stacked-to-main|feature-branch-chain|...`) or several values on one line.
- Write `Chain strategy: pending` only when no decisions were forwarded. `pending` never unblocks apply.
- An unfilled `Decision needed before apply:` placeholder keeps apply blocked; always replace it with one value.
- Under `size:exception`, omit the `Budget risk` line; there is no forecast.
- When `Decision needed before apply: Yes`, apply stays blocked until the artifact contains a whole line `Chain strategy: stacked-to-main`, `Chain strategy: feature-branch-chain`, or `Chain strategy: size:exception`, or a whole line `Decision needed before apply: No`.
- Mentions in tables, prose, bullets, or a bare `size:exception` token do not count as a decision.

You may keep the table for readability, but only the plain-text lines are the guard contract.

For `feature-branch-chain`, suggested work units SHOULD name the intended base boundary: PR #1 base = feature/tracker branch; PR #2 base = PR #1 branch; PR #3 base = PR #2 branch. If a child PR would show previous PR changes, the base is wrong and must be retargeted/rebased before review.

### Phase Organization Guidelines

```
Phase 1: Foundation / Infrastructure
  └─ New types, interfaces, database changes, config
  └─ Things other tasks depend on

Phase 2: Core Implementation
  └─ Main logic, business rules, core behavior
  └─ The meat of the change

Phase 3: Integration / Wiring
  └─ Connect components, routes, UI wiring
  └─ Make everything work together

Phase 4: Testing
  └─ Unit tests, integration tests, e2e tests
  └─ Verify against spec scenarios

Phase 5: Cleanup (if needed)
  └─ Documentation, remove dead code, polish
```

### Step 4: Persist Artifact

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.

- artifact: `tasks`
- topic_key: `sdd/{change-name}/tasks`
- type: `architecture`

### Step 5: Return Summary

Return to the orchestrator:

```markdown
## Tasks Created

**Change**: {change-name}
**Location**: `openspec/changes/{change-name}/tasks.md` (openspec/hybrid) | Hive `sdd/{change-name}/tasks` (hive) | inline (none)

### Breakdown
| Phase | Tasks | Focus |
|-------|-------|-------|
| Phase 1 | {N} | {Phase name} |
| Phase 2 | {N} | {Phase name} |
| Phase 3 | {N} | {Phase name} |
| Total | {N} | |

### Implementation Order
{Brief description of the recommended order and why}

### Review Workload Forecast
- Estimated changed lines: {estimate or range}
- Budget risk: {Low | Medium | High | not forecast (size:exception)}
- Chained PRs recommended: {Yes | No}
- Delivery strategy: {auto-chain | exception-ok}
- Chain strategy: {stacked-to-main | feature-branch-chain | size:exception | pending}
- Decision needed before apply: {Yes | No}
- Suggested work-unit PR split: {brief list or "Not needed"}

### Next Step
{Ready for implementation (sdd-apply) OR preflight decisions missing — the orchestrator must run the preflight before sdd-apply.}
```

## Rules

- ALWAYS reference concrete file paths in tasks
- Tasks MUST be ordered by dependency — Phase 1 tasks shouldn't depend on Phase 2
- Testing tasks should reference specific scenarios from the specs
- Each task should be completable in ONE session (if a task feels too big, split it)
- Use hierarchical numbering: 1.1, 1.2, 2.1, 2.2, etc.
- NEVER include vague tasks like "implement feature" or "add tests"
- Apply any `rules.tasks` from `openspec/config.yaml`
- If the forwarded TDD mode is `strict`, integrate test-first tasks: RED task (write failing test) → GREEN task (make it pass) → REFACTOR task (clean up)
- **Size budget**: Tasks artifact MUST be under 530 words. Each task: 1-2 lines max. Use checklist format, not paragraphs.
- **Review workload guard**: ALWAYS include the Review Workload Forecast guard lines. Under a budget, forecast against the forwarded N and split above it for the forwarded chain strategy; under `size:exception`, skip the forecast. Never ask the user for a chain strategy or size exception.
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
