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
- Preflight decisions for the feature: delivery strategy (`auto-chain | exception-ok`), review budget, chain strategy (or `size:exception`), assigned PR slice when chained, and TDD mode (`strict | standard`)

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`, using Jarvis/Hive terminology.

- **hive**: Read the required artifacts and resolve progress through the dedicated guarded progress API. Search results with multiple candidates are ambiguous; use the status-provided snapshot reference rather than selecting a newest result. `mcp__hive__mem_save` remains for non-progress SDD artifacts only; it is not an apply-progress writer.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. Update `tasks.md` with `[x]` marks and use the dedicated progress command for the bounded snapshot/batch topology.
- **hybrid**: Use the dedicated progress command to publish the same immutable evidence batches and guarded snapshot advance to both sides. Update filesystem task checkboxes; do not use a general memory save to bridge divergent progress.
- **none**: Return progress only. Do not update project artifacts.

Before selecting a progress backend, resolve or adopt the immutable `project/change` binding from status: `hive`, `openspec`, or `hybrid`; `none` is unpersisted. A persisted binding wins over `JARVIS_SDD_STORE_MODE`, including an invalid value. Hive binding is in SQLite, OpenSpec binding is in `openspec/changes/{change-name}/state.yaml`, and hybrid requires matching copies. Unavailable is not absent; malformed, unsupported, divergent, or protected-progress state blocks fail closed. Planning artifacts cannot choose authority.

## Status and Workspace Guard

Before reading implementation files or writing code, consume the structured status provided by the orchestrator. If status is not provided but `jarvis sdd status <change> --json` is available, run it. If `jarvis sdd status <change> --json` is unavailable, STOP before editing. Manual recovery may inspect artifacts, but cannot invent workspace-edit authority or authorize an edit; report missing status dimensions: schema, blockers, dependencies, workspace-planning, artifact context, and allowed edit roots.

- Confirm the status field `schema` is exactly `jarvis.sdd-status`.
- Confirm `dependencies["sdd-apply"]` is exactly `ready`; otherwise STOP and return the phase-specific `blockedReasons`.
- Read context from `contextFiles` and `artifactPaths` before reading implementation files. Do not assume fixed artifact filenames when status provides paths or Hive topics.
- If status includes `blockedReasons`, review them first. If any blocker prevents apply, STOP and return `blocked` with those reasons.
- Use dependency states to decide whether `sdd-apply` is blocked, ready, or already satisfied. If the `sdd-apply` dependency is blocked, STOP and return `blocked`.
- Use `applyState.hasProgress` and `applyState.complete` to understand whether prior apply work exists and is complete. `hasProgress` means an apply-progress artifact exists. `complete` means canonical apply-progress is complete and authoritative task checkboxes are all checked; verify-report remains a separate dependency. Do not rename, remove, or invent extra `applyState` values beyond the Jarvis status contract.
- If all assigned implementation is complete and the validated v2 snapshot coverage agrees with the frozen task manifest and persisted checkboxes, do not edit. Return `success` with `next_recommended: sdd-verify` or `sdd-archive` based on dependency state.
- If the `sdd-apply` dependency is ready, proceed only on the assigned pending tasks.
- If `actionContext.mode` is not exactly `workspace-edit`, treat linked repos and folders as read-only planning context. STOP before editing and return `blocked`.
- Treat `actionContext.allowedEditRoots` from valid native status as the authoritative edit-root guard. If `actionContext.allowedEditRoots` is missing or empty, STOP before editing. Manual recovery or maintainer approval cannot substitute for this authority.
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

- `Budget risk: High`
- `Chained PRs recommended: Yes`
- `Decision needed before apply: Yes`

Then you MUST confirm the preflight decisions forwarded by the orchestrator resolve the delivery path:

1. **`auto-chain` with a chain strategy**: implement only the assigned work-unit slice, keep scope autonomous, and report the intended PR boundary. Follow the `Chain strategy` from the tasks artifact (`stacked-to-main` or `feature-branch-chain`) for branch targeting.
2. **`exception-ok` / `size:exception`**: implement the feature as one PR with no further size checks; do not forecast lines or ask about size.

Also check for `Chain strategy` in the tasks artifact. If present and not `pending`, follow it consistently:

- `stacked-to-main`: each PR targets the previous PR's branch (or `main` after the previous merges).
- `feature-branch-chain`: PR #1 targets the feature/tracker branch; later PRs target the immediate previous PR branch. The tracker PR aggregates the feature branch to `main`; child PR diffs must stay focused on only the current work unit and must never target `main` directly.

If neither delivery decision nor chain strategy is present, STOP before writing code and return `blocked` with: `Preflight decisions missing before apply: the tasks artifact has no resolved size policy or chain strategy. The orchestrator must run the SDD Session Preflight and record the ## SDD Decisions block.`

#### Step 2b: Read Previous Apply-Progress (if exists)

Before starting work, read `skills/_shared/apply-progress.md` and use the mode-specific canonical reader: OpenSpec reads the change's canonical `apply-progress.md` snapshot and exactly its referenced `apply-evidence/<batch-id>.json` documents; Hive calls `sdd_apply_progress_get` with project and change, then iterates `snapshot.batches` with one `sdd_apply_evidence_get` request per `batch_id`. Validate the snapshot against those batches before using it. Hybrid resolves and independently validates both. Read the current generation, revision, digest, receipt, coverage, and next unpersisted entry; never infer completion from a cumulative observation or a checkbox alone.

Continue only from validated canonical bounded state. Do not merge or rewrite historical evidence. Reconcile current task state against validated coverage, then do not jump to `sdd-verify` until apply progress and task checkboxes agree. The guarded snapshot is the authoritative progress state.

### Step 3: Resolve TDD Mode and Test Command

The TDD mode forwarded by the orchestrator is the preflight choice for this feature and always wins over cached capabilities. Cached capabilities decide the mode only for legacy launches that forwarded nothing; otherwise they are read only to find the test command.

```
Resolve mode (first match wins):
├── Prompt contains `STRICT TDD MODE IS ACTIVE` → STRICT TDD MODE
│   └── Load and follow strict-tdd.md (read the file: skills/sdd-apply/strict-tdd.md)
├── Prompt contains `TDD MODE: standard` → STANDARD MODE
│   └── Use Step 4 below; never load strict-tdd.md
└── Nothing forwarded (legacy launch only)
    ├── Read cached testing capabilities:
    │   ├── hive: mcp__hive__mem_search("sdd/{project}/testing-capabilities") → mcp__hive__mem_get_observation(id)
    │   ├── openspec: openspec/config.yaml → strict_tdd_suggestion + detection_reason + testing section
    │   └── Fallback: check project files directly for a real test command (package.json test script, *_test.go, etc.)
    ├── Use `strict_tdd_suggestion`; fall back to legacy `strict_tdd` only when the suggestion is absent
    ├── Suggestion is strict AND a real test command exists → STRICT TDD MODE
    └── Otherwise → STANDARD MODE

Cache the resolved mode for the return summary.
```

#### Runnable Test Command Gate (Strict TDD Only)

If Strict TDD Mode is active but no runnable test command exists for the files the assigned tasks touch (for example Deluge code, or a project without a test runner), STOP before the first task and return `blocked` with reason `strict-tdd-unrunnable` and this sentence for the user: `Strict TDD was chosen but these files cannot run under a test runner; choose standard for this feature or provide a test command.` Do not loop writing tests that cannot run, and do not switch to Standard Mode yourself.

**Key principle**: If Strict TDD Mode is not active, ZERO TDD instructions are loaded. The `strict-tdd.md` module is never read, never processed, never consumes tokens.

#### Hard Gate (Strict TDD Only)

If Strict TDD Mode is active (from the forwarded TDD mode or, for legacy launches only, self-discovery):

- You MUST record TDD proof as `task_records` in the batch checkpoint, not a Markdown TDD table; the CLI expands each record into structured v2 `EvidenceEntry` values.
- Each task gets ONE record whose `red` step carries the executed focused failing command and failure summary, whose `green` step carries the passing command and exit code, and whose `triangulate` step is either an executed case or an explicit structural `skip_reason`.
- Completion coverage may name a task only when its record's evidence actually establishes completion. Do not infer coverage from a narrative table.
- Record new TDD evidence in the new immutable batch; do not merge or rewrite prior apply-progress evidence. The verify phase rejects missing or incomplete v2 evidence.

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
├── Add the task's `task_records` item (files plus a `verification` step with the command you ran, or only a summary when no command applies); leave the persisted task checkbox unchecked
└── Note any issues or deviations
```

### Step 5: Persist Progress

**This step is MANDATORY — do NOT skip it.**

Checkpoint ONCE per apply batch, not per task. After every assigned task in the batch is implemented and its evidence is captured, run `jarvis sdd progress checkpoint --root <change-root> --request <request.json>` a single time. It accepts no positional arguments, plans at most one whole-entry prefix, and commits immutable evidence batches through the guarded writer; every serialized snapshot and batch stays at most 40,000 Unicode runes. Do not use `mcp__hive__mem_save` to write, replace, or recover apply-progress.

##### Canonical v2 checkpoint request

Freeze `tasks` and the `task_records` (the ordered complete entries stream they expand to) before the checkpoint; a continuation reuses them unchanged through a stable base snapshot and cursor. Always send `base` and the expected coordinates from the progress read in Step 2b: they are the optimistic-concurrency guard and are never optional.

| Field | Required value |
| --- | --- |
| `project`, `change` | Canonical project and change identifiers. Each protocol identifier is 1–64 characters matching `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`. |
| `tasks` | The ordered task list parsed from the same frozen `tasks.md`: each item is `{id, text, path}`. `id` and normalized text determine `task_manifest_sha256`; do not manufacture, reorder, or edit tasks. |
| `base` | `null` for the initial generation; otherwise the exact resolved v2 snapshot from the progress read. |
| `expected_generation`, `expected_revision`, `expected_digest` | Coordinates of `base`; use zero values and an empty digest only for an initial snapshot. |
| `task_records` | One record per task this batch covers: `task_id`, `files`, optional `completes` (default `true`), and only the steps that actually ran — `red`, `green`, `triangulate`, `refactor`, `verification` — each `{command, exit_code, summary}`; `triangulate` may instead be `{skip_reason}`. Completion rides on the last step that ran (a `skip_reason` triangulation did not run), and that step must pass. |
| `entry_index`, `entry_id` | `0` with `entry_id` omitted for a new stream; `entry_index: 0` fills the first expanded entry ID. A continuation uses exactly the pair `next.instruction` names. |

Omit `request_id`, `batch_id`, and `stream_sha256`: the checkpoint derives them from the request. Explicit `entries` (whole `EvidenceEntry` values with `entry_id`, `task_ids`, `completes_task_ids`, `kind`, `summary`, `command`, `exit_code`, `outcome`, and `files`) remain accepted but are mutually exclusive with `task_records`. `imported` is internal legacy provenance only; callers MUST NOT supply it.

Never invent coverage: a record names only a task this batch implemented and carries only commands that actually ran. `tasks.md` stays the human-visible task state, never a substitute for v2 evidence; a later task edit changes the manifest digest and requires explicit reconciliation, not a continuation.

The checkpoint `outcome` is one of `committed`, `continuation_required`, `evidence_item_too_large`, `snapshot_capacity_exhausted`, `stream_preflight_required`, and `checkpoint_consolidation_required`, or a typed conflict, invalid, blocked, or recovery response. Every response carries `next: {action, instruction}`. After the command, follow `next.action` and `next.instruction` only: `continue_stream` is yours to rerun now with the returned cursor (the orchestrator does not relaunch for it), `done` and `continue_tasks` end this batch's checkpointing, and every `stop_*` action ends the phase with `code` and `recovery` reported verbatim. Fail closed: if the response has no `next`, or its action is not one named here or in Recovery, STOP and return `blocked` with the raw `outcome`, `code`, and `recovery`. Do not emit free-form lifecycle markers: the validated v2 snapshot status and referenced batches are authoritative.

#### Recovery

Rare paths still follow `next`; these notes only name what it covers.

- `run_upgrade_continuation` (`legacy_upgrade_required`): a historical v2 snapshot serialized an explicit all-zero continuation group. Run `jarvis sdd progress upgrade-continuation --root <change-root> --request <request.json>` with the same request, then rerun the checkpoint from the upgraded snapshot. A legacy Markdown artifact needs no step: its first checkpoint imports it and keeps the legacy source authoritative until that guarded commit succeeds.
- `stop_consolidate` (`stream_preflight_required`, `checkpoint_consolidation_required`): combine pending evidence into fewer, larger records before binding a new stream. Never modify records for an active frozen continuation.
- `stop_new_change` (`snapshot_capacity_exhausted`): checkpoint frequency cannot repair a full reference set; stop with apply partial and carry the uncovered task IDs into a new SDD change. A committed `warning` is an early capacity signal: do not start new tiny streams.
- `retry_identical` (transport loss, `lock_busy`, partial hybrid recovery): retry at most twice; if it still does not commit, STOP and report `code` and `recovery`. Preserve the existing hybrid receipt and rerun the identical request, so the same request ID, batch ID, base, cursor, and byte-identical payload replay. A partial recovery may replay or revalidate the exact request against both backends through their idempotent contracts before accepting acknowledgements. A complete receipt returns without replay. Never change the payload, identity, or authority, and never rewrite confirmed progress.
- The low-level `advance` remains a recovery/compatibility primitive for an already planned batch and snapshot. Do not ask `advance`, HTTP, or MCP to select prefixes or plan capacity.

### Step 6: Mark Tasks Complete

Once the batch checkpoint (including any `continue_stream` reruns) has returned `committed`, update the persisted tasks artifact in one edit: change `- [ ]` to `- [x]` for exactly the tasks the committed snapshot coverage names, and nothing else. Never mark a task `[x]` without committed coverage.

```markdown
## Phase 1: Foundation

- [x] 1.1 Create `internal/auth/middleware.go` with JWT validation
- [x] 1.2 Add `AuthConfig` struct to `internal/config/config.go`
- [ ] 1.3 Add auth routes to `internal/server/server.go`  ← still pending
```

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

{IF Strict TDD Mode → include the safety-net baseline and each task's RED/GREEN/TRIANGULATE/REFACTOR outcomes from strict-tdd.md}

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
- In `openspec` mode, mark tasks complete in `tasks.md` only after the guarded checkpoint commits matching task coverage
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
