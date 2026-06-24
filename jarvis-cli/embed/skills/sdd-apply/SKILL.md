---
name: sdd-apply
display_name: "SDD Apply"
description: "Implement tasks following specs and design; supports Strict TDD mode. Trigger: When implementing tasks"
disable-model-invocation: true
user-invocable: false
license: MIT
scope: core
metadata:
  author: gentleman-programming
  version: "3.0"
  delegate_only: true
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-apply/SKILL.md (tag v1.40.2, commit 660917927b4821f5e540dc8fa501d6bee723222c); adapted for Jarvis/Hive runtime semantics. -->

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-apply` executor using your platform's delegation primitive.
> This skill is for EXECUTORS only.

## Executor Override

If you ARE the `sdd-apply` executor, the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.

## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If Spanish technical artifacts are explicitly requested, use neutral/professional Spanish unless the user explicitly asks for a regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; Spanish comments default to neutral/professional Spanish unless the user or target context clearly calls for regional tone.

## Purpose

You are an implementation executor. You receive specific tasks and implement them by writing actual code. Follow the specs, design, assigned task boundary, and workspace safety contract strictly.

## What You Receive

From the orchestrator:
- Change name
- The specific task(s) to implement (for example, "Phase 1, tasks 1.1-1.3")
- Artifact store mode (`hive | openspec | hybrid | none`)
- Structured status from `jarvis sdd status <change> --json` (schema: `jarvis.sdd-status`): `schemaName`/`schema`, `planningHome`, `changeRoot`, `artifactPaths`, `contextFiles`, `blockedReasons`, `applyState`, task progress, dependency states, `relationships`, `phaseInstructions`, and `actionContext`
- Delivery strategy and resolved workload decision (`ask-on-risk | auto-chain | single-pr | exception-ok`, plus PR slice or `size:exception` when applicable)

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`, using Jarvis/Hive terminology.

- **hive**: Read `sdd/{change-name}/proposal`, `sdd/{change-name}/spec`, `sdd/{change-name}/design`, and `sdd/{change-name}/tasks` (all required). Mark tasks complete by saving the updated tasks artifact with `mcp__hive__mem_save(topic_key: "sdd/{change-name}/tasks", capture_prompt: false, content: "...")` — re-saving an SDD artifact under the same topic_key creates another candidate artifact; if Hive search returns multiple candidates and no explicit observation ID or artifact reference is available, treat retrieval as ambiguous and ask before proceeding. Save progress as `sdd/{change-name}/apply-progress` with `capture_prompt:false`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. Update `tasks.md` with `[x]` marks.
- **hybrid**: Follow BOTH conventions — persist progress to Hive (`mcp__hive__mem_save` with topic_key grouping and `capture_prompt:false`) AND update `tasks.md` with `[x]` marks on filesystem.
- **none**: Return progress only. Do not update project artifacts.

## Status and Workspace Guard

Before reading implementation files or writing code, consume the structured status provided by the orchestrator. If status is not provided but `jarvis sdd status <change> --json` is available, run it. If `jarvis sdd status <change> --json` is unavailable, STOP before editing unless the maintainer explicitly approves manual recovery mode in the current conversation. Manual recovery mode does not make missing status safe by default; report missing status dimensions: blockers, dependencies, workspace-planning, artifact context, and allowed edit roots.

- Confirm the status uses schema: `jarvis.sdd-status`.
- Read context from `contextFiles` and `artifactPaths` before reading implementation files. Do not assume fixed artifact filenames when status provides paths or Hive topics.
- If status includes `blockedReasons`, review them first. If any blocker prevents apply, STOP and return `blocked` with those reasons.
- Use dependency states to decide whether `sdd-apply` is blocked, ready, or already satisfied. If the `sdd-apply` dependency is blocked, STOP and return `blocked`.
- Use `applyState.hasProgress` and `applyState.complete` to understand whether prior apply work exists and whether downstream verification has already happened. Do not invent extra `applyState` values beyond the Jarvis status contract.
- If all assigned implementation is complete and no `apply-progress = partial` reconciliation is needed, do not edit. Return `success` with `next_recommended: sdd-verify` or `sdd-archive` based on dependency state.
- If the `sdd-apply` dependency is ready, proceed only on the assigned pending tasks.
- If `actionContext.mode` is `workspace-planning`, treat linked repos and folders as read-only planning context. STOP before editing and return `blocked` unless the orchestrator provides a safe workspace-edit status.
- Treat `actionContext.allowedEditRoots` from valid status as the authoritative edit-root guard. If `actionContext.allowedEditRoots` is missing or empty, STOP before editing.
- If `actionContext.allowedEditRoots` is present, write only inside those roots. If a needed edit is outside every `actionContext.allowedEditRoots` entry, STOP and report the unsafe path.
- Use `phaseInstructions` to report the next phase command when returning; do not invent phase routing.
- Generated artifacts are output, never sources of truth. Do not edit installed user-machine agent configuration, generated registries, or runtime copies to make apply or verification pass; change the source assets/templates instead.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Read Context

Before writing ANY code:
1. Consume the structured status and confirm the `sdd-apply` dependency is ready for the assigned work.
2. Enforce `actionContext.mode` and `allowedEditRoots`; stop on read-only planning mode, missing roots, or unsafe paths.
3. Read every applicable artifact path/topic from `contextFiles` and `artifactPaths`.
4. Read the specs — understand WHAT the code must do.
5. Read the design — understand HOW to structure the code.
6. Read existing code in affected files — understand current patterns.
7. Check the project's coding conventions from `config.yaml` when available.

#### Step 2a: Enforce Review Workload Decision

Before implementing, inspect the tasks artifact for `Review Workload Forecast`.

If the forecast says any of the following:

- `400-line budget risk: High`
- `Chained PRs recommended: Yes`
- `Decision needed before apply: Yes`

Then you MUST confirm the orchestrator/user provided a resolved delivery path:

1. **`auto-chain` or chosen chained/stacked PR mode**: implement only the assigned work-unit slice, keep scope autonomous, and report the intended PR boundary. Follow the `Chain strategy` from the tasks artifact (`stacked-to-main` or `feature-branch-chain`) for branch targeting.
2. **`exception-ok` or single PR with exception**: continue only if the prompt explicitly says the maintainer accepts `size:exception`.
3. **`single-pr` above budget**: continue only after the prompt explicitly records `size:exception`.

Also check for `Chain strategy` in the tasks artifact. If present and not `pending`, follow it consistently:
- `stacked-to-main`: each PR targets the previous PR's branch (or `main` after the previous merges).
- `feature-branch-chain`: PR #1 targets the feature/tracker branch; later PRs target the immediate previous PR branch. The tracker PR aggregates the feature branch to `main`; child PR diffs must stay focused on only the current work unit and must never target `main` directly.

If neither delivery decision nor chain strategy is present, STOP before writing code and return `blocked` with: `Workload decision required before apply: estimated work may exceed 400 changed lines. Ask the user which chain strategy to use (stacked-to-main, feature-branch-chain, or size-exception).`

#### Step 2b: Read Previous Apply-Progress (if exists)

Before starting work, check for existing apply-progress:

1. `mcp__hive__mem_search(query: "sdd/{change-name}/apply-progress", project: "{project}")`
2. If found: `mcp__hive__mem_get_observation(id)` → read the full content
3. Parse which tasks are already marked complete, partial, or blocked
4. Skip tasks that are truly complete and start from the first assigned incomplete or partial task
5. When saving your apply-progress in Step 6, MERGE: include all previously completed tasks PLUS your newly completed tasks in a single combined artifact

**CRITICAL**: If the orchestrator told you previous progress exists, you MUST read it. If you overwrite without reading, completed work from prior batches is permanently lost.

When prior `apply-progress = partial` exists, merge/reconcile it with current task state, continue apply from the unfinished work, and do not jump to `sdd-verify` until apply progress and task checkboxes agree.

### Step 3: Read Testing Capabilities and Resolve Mode

Read the cached testing capabilities to determine implementation mode:

```
Read testing capabilities from:
├── hive: mcp__hive__mem_search("sdd/{project}/testing-capabilities") → mcp__hive__mem_get_observation(id)
├── openspec: openspec/config.yaml → strict_tdd + testing section
└── Fallback: check project files directly (package.json, pyproject.toml, go.mod, etc.)

Resolve mode:
├── IF strict_tdd: true AND test runner exists
│   └── STRICT TDD MODE → Load and follow strict-tdd.md module
│       (read the file: skills/sdd-apply/strict-tdd.md)
│
├── IF strict_tdd: false OR no test runner
│   └── STANDARD MODE → use Step 4 below (no TDD module loaded)
│
└── Cache the resolved mode for the return summary
```

**Key principle**: If Strict TDD Mode is not active, ZERO TDD instructions are loaded. The `strict-tdd.md` module is never read, never processed, never consumes tokens.

#### Hard Gate (Strict TDD Only)

If Strict TDD Mode is active (either from orchestrator injection or self-discovery):
- You MUST produce a **TDD Cycle Evidence** table in your apply-progress artifact
- Each task row MUST have: RED (test written first) → GREEN (implementation passes) → REFACTOR columns
- If you complete a task WITHOUT writing tests first, mark it as FAILED in the evidence table
- If partial apply progress already has evidence, preserve and merge that evidence with the new cycle evidence
- The verify phase WILL reject your work if the TDD Evidence table is missing or incomplete

**There is no silent fallback.** If you resolved Strict TDD as active, you follow it or you report failure. You do NOT quietly switch to Standard Mode.

### Step 4: Implement Tasks (Standard Workflow)

This step is used when Strict TDD Mode is NOT active:

```
FOR EACH TASK:
├── Read the task description
├── Read relevant spec scenarios (these are your acceptance criteria)
├── Read the design decisions (these constrain your approach)
├── Read existing code patterns (match the project's style)
├── Confirm every target path is under allowedEditRoots
├── Write the code
├── Mark task as complete [x] in the persisted tasks artifact immediately
└── Note any issues or deviations
```

### Step 5: Mark Tasks Complete

Update the persisted tasks artifact — change `- [ ]` to `- [x]` for completed tasks:

```markdown
## Phase 1: Foundation

- [x] 1.1 Create `internal/auth/middleware.go` with JWT validation
- [x] 1.2 Add `AuthConfig` struct to `internal/config/config.go`
- [ ] 1.3 Add auth routes to `internal/server/server.go`  ← still pending
```

### Step 6: Persist Progress

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `apply-progress`
- topic_key: `sdd/{change-name}/apply-progress`
- type: `architecture`
- Also update the tasks artifact with `[x]` marks via `mcp__hive__mem_save` with `topic_key: "sdd/{change-name}/tasks"` (hive), file edit (openspec), or both (hybrid)

#### Merge Protocol

When saving apply-progress:
1. If you read previous progress in Step 2b, your artifact MUST include ALL previously completed tasks and evidence PLUS your new completions
2. If previous progress was partial, explicitly record what was reconciled, what remains, and why apply should continue or finish
3. The final artifact should show the cumulative state of ALL assigned tasks across ALL batches
4. Format: keep the same structure but ensure no completed task is lost from prior batches

### Step 7: Return Summary

Before returning, re-read the persisted tasks artifact and confirm every task you report as completed is marked `[x]` there. If the artifact still shows a completed task as `- [ ]`, fix the checkbox before returning. Do not report `Ready for verify` while completed work is only reflected in internal todos or apply-progress.

Return to the orchestrator:

```markdown
## Implementation Progress

**Change**: {change-name}
**Mode**: {Strict TDD | Standard}

### Completed Tasks
- [x] {task 1.1 description}
- [x] {task 1.2 description}

### Files Changed
| File | Action | What Was Done |
|------|--------|---------------|
| `path/to/file.ext` | Created | {brief description} |
| `path/to/other.ext` | Modified | {brief description} |

{IF Strict TDD Mode → include TDD Cycle Evidence table from strict-tdd.md}

### Deviations from Design
{List any places where the implementation deviated from design.md and why. If none, say "None — implementation matches design."}

### Issues Found
{List any problems discovered during implementation. If none, say "None."}

### Remaining Tasks
- [ ] {next task}
- [ ] {next task}

### Workload / PR Boundary
- Mode: {single PR | chained PR slice | stacked PR slice | size:exception}
- Current work unit: {unit name or "N/A"}
- Boundary: {what this apply batch starts from and ends with}
- Estimated review budget impact: {brief note}

### Status
{N}/{total} tasks complete. {Ready for next batch / Ready for verify / Blocked by X}
```

## Rules

- ALWAYS read specs before implementing — specs are your acceptance criteria
- ALWAYS follow the design decisions — do not freelance a different approach
- ALWAYS match existing code patterns and conventions in the project
- ALWAYS consume or produce structured status before implementation; do not infer readiness from conversation alone
- STOP on blocked `sdd-apply` dependency, unsafe `actionContext`, missing edit roots, or edits outside `allowedEditRoots`
- In `openspec` mode, mark tasks complete in `tasks.md` AS you go, not at the end
- Before returning, re-read the persisted tasks artifact and ensure completed tasks are visibly marked `[x]`; internal todos are not completion evidence
- If you discover the design is wrong or incomplete, NOTE IT in your return summary — do not silently deviate
- If a task is blocked by something unexpected, STOP and report back
- If workload forecast requires a decision and none was provided, STOP before writing code
- When applying a chained/stacked PR slice, keep the batch autonomous: one deliverable scope, verification included, and clear rollback boundary
- When applying `size:exception`, state it explicitly in apply-progress and the return summary
- NEVER implement tasks that were not assigned to you
- Skill loading is handled in Step 1 — follow any loaded skills strictly when writing code
- Apply any `rules.apply` from `openspec/config.yaml`
- If Strict TDD Mode is active (Step 3), load `strict-tdd.md` and follow its cycle INSTEAD of Step 4
- When Strict TDD is active, the `strict-tdd.md` module's rules OVERRIDE Step 4 entirely
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
