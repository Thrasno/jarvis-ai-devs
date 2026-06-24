---
name: sdd-archive
display_name: "SDD Archive"
description: "Merge delta specs to main specs and close the SDD change cycle. Trigger: When archiving changes"
disable-model-invocation: true
user-invocable: false
license: MIT
scope: core
metadata:
  author: gentleman-programming
  version: "2.0"
  delegate_only: true
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-archive/SKILL.md (tag v1.40.2, commit 660917927b4821f5e540dc8fa501d6bee723222c); adapted for Jarvis/Hive runtime semantics. -->

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-archive` sub-agent using your platform's delegation primitive.
> This skill is for EXECUTORS only.

## Executor Override

If you ARE the `sdd-archive` sub-agent (NOT the orchestrator), the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.

## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If Spanish technical artifacts are explicitly requested, use neutral/professional Spanish unless the user explicitly asks for a regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; Spanish comments default to neutral/professional Spanish unless the user or target context clearly calls for regional tone.

## Purpose

You are a sub-agent responsible for ARCHIVING. You merge delta specs into the main specs (source of truth), then move the change folder to the archive. You complete the SDD cycle only after implementation and verification evidence is complete, current, and safe.

## What You Receive

From the orchestrator:
- Change name
- Artifact store mode (`hive | openspec | hybrid | none`)
- Structured status from `jarvis sdd status <change> --json`, schema: `jarvis.sdd-status`
- Status fields including `blockedReasons`, `taskProgress`, `applyState`, `artifacts`, `artifactPaths`, `contextFiles`, `actionContext`, and `phaseInstructions`
- Verify evidence exposed through `artifacts["verify-report"]`, `artifactPaths["verify-report"]`, `contextFiles["verify-report"]`, and/or explicit verify-report artifact content

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **hive**: Read `sdd/{change-name}/proposal`, `sdd/{change-name}/spec`, `sdd/{change-name}/design`, `sdd/{change-name}/tasks`, `sdd/{change-name}/verify-report`, and any `sdd/{change-name}/apply-progress` (all required when present in status). Record all observation IDs in the archive report for traceability. Save as `sdd/{change-name}/archive-report` with `mcp__hive__mem_save`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. Perform merge and archive folder moves only after the status gate passes.
- **hybrid**: Follow BOTH conventions — persist archive report to Hive (with observation IDs) AND perform filesystem merge + archive folder moves.
- **none**: Return closure summary only. Do not perform archive file operations and do not persist an archive report. For `none` mode, return a closure summary only; do not persist an archive report.

## Status and Archive Safety Gate

Before syncing specs, moving folders, or writing an archive report, consume the structured status from `jarvis sdd status <change> --json`.

- Confirm `schema: jarvis.sdd-status` and read `blockedReasons`, `taskProgress`, `applyState`, `artifacts`, `artifactPaths`, `contextFiles`, `actionContext`, and `phaseInstructions`.
- Locate verify evidence via `artifacts["verify-report"]`, `artifactPaths["verify-report"]`, `contextFiles["verify-report"]`, and/or explicit verify-report artifact content. There is no top-level verify report status field.
- If `blockedReasons` is non-empty, STOP and return `blocked` with the reasons. Do not archive.
- If `actionContext.mode` is `workspace-planning`, STOP. Do not move workspace changes into repo-local archives or edit linked repositories.
- If `actionContext.allowedEditRoots` is present, every archive edit, spec merge, and folder move must stay inside those roots. If an edit would escape them, STOP.
- Generated artifacts are output, never sources of truth. Never fix archive readiness by editing generated user-machine artifacts, installed skill copies, generated registries, or local runtime config. Change Jarvis source templates/assets instead.

### Verification Gate

- If verify-report evidence is missing, failing, stale, or does not cover the current artifacts, STOP and return `blocked`; archive cannot proceed when verify-report evidence is missing/failing/stale.
- Unresolved CRITICAL verification findings always block archive. Do not accept overrides for unresolved CRITICAL findings.
- If verify evidence references older artifact IDs, an older task state, or a different apply-progress state, treat the report as stale and STOP.
- Partial, missing, or stale artifacts block archive until they are reconciled and re-verified.

### Task Completion Gate

`sdd-apply` is responsible for marking completed tasks in the persisted tasks artifact. `sdd-archive` validates that the persisted artifact reflects the final state before closing the cycle.

Before syncing specs or moving any archive folder, inspect the tasks artifact:

- **hive**: read the full `sdd/{change-name}/tasks` observation.
- **openspec/hybrid**: read `openspec/changes/{change-name}/tasks.md`.
- **status**: compare the artifact with `taskProgress` from `jarvis.sdd-status`.

Any incomplete task checkbox or `taskProgress` entry blocks archive. STOP and return `blocked`; do not sync specs, move the change folder, or claim the SDD cycle is complete.

Stale checkboxes are not archive-ready by themselves. If checkboxes, `taskProgress`, `applyState`, apply-progress, or verify-report evidence disagree, STOP and report that `sdd-apply` and `sdd-verify` must reconcile the persisted artifacts before archive. Do not mark archive-ready based only on stale checkboxes, internal todos, or conversation claims.

When prior `apply-progress = partial` exists, STOP until current tasks, apply-progress, and verify-report have been reconciled and re-verified. Partial apply-progress means archive must wait for reconciliation and verification, even if checkboxes look complete.

Archive does not provide a completion override path. Missing, partial, or stale tasks/apply-progress/verify-report artifacts must be reconciled by the appropriate prior phase and verified again before archive continues.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Read Archive Context

Read status-provided `contextFiles` / `artifactPaths` instead of assuming fixed filenames when they are available. Then read the proposal, spec, design, tasks, verify-report, and apply-progress if present. Archive only from source-of-truth artifacts, not generated user-machine outputs.

### Step 3: Sync Delta Specs to Main Specs

Do not start this step until the **Status and Archive Safety Gate**, **Verification Gate**, and **Task Completion Gate** pass.

**IF mode is `hive`:** Skip filesystem sync — artifacts live in Hive only. The archive report (Step 6) records all observation IDs for traceability.

**IF mode is `none`:** Skip — no artifacts to sync.

**IF mode is `openspec` or `hybrid`:** For each delta spec in `openspec/changes/{change-name}/specs/`:

#### If Main Spec Exists (`openspec/specs/{domain}/spec.md`)

Read the existing main spec and apply the delta:

```
FOR EACH SECTION in delta spec:
├── ADDED Requirements → Append to main spec's Requirements section
├── MODIFIED Requirements → Replace the matching requirement in main spec
├── REMOVED Requirements → Delete the matching requirement in main spec after the removal guard passes
└── RENAMED Requirements → Rename the matching requirement in main spec using the explicit old/new names
```

**Merge carefully:**
- Match requirements by name (e.g., "### Requirement: Session Expiration")
- Preserve all OTHER requirements that aren't in the delta
- Before deleting any REMOVED requirement, confirm the delta includes both `Reason:` and `Migration:` with non-empty, non-placeholder evidence
- `Migration: None` is valid only when it includes a justification
- If Reason or Migration is empty, placeholder text, or unjustified `None`, STOP before deleting it
- If a RENAMED requirement omits Old name or New name, STOP before renaming it
- For RENAMED requirements, preserve the requirement body and scenarios unless the delta also modifies them
- Maintain proper Markdown formatting and heading hierarchy

#### If Main Spec Does NOT Exist

The delta spec IS a full spec (not a delta). Copy it directly:

```bash
# Copy new spec to main specs
openspec/changes/{change-name}/specs/{domain}/spec.md
  → openspec/specs/{domain}/spec.md
```

### Step 4: Move to Archive

**IF mode is `hive`:** Skip — there are no `openspec/` directories to move. The archive report in Hive serves as the audit trail.

**IF mode is `none`:** Skip — no filesystem operations.

**IF mode is `openspec` or `hybrid`:** Move the entire change folder to archive with date prefix:

```
openspec/changes/{change-name}/
  → openspec/changes/archive/YYYY-MM-DD-{change-name}/
```

Use today's date in ISO format (e.g., `2026-02-16`).

### Step 5: Verify Archive

**IF mode is `openspec` or `hybrid`:** Confirm:
- [ ] Main specs updated correctly
- [ ] Change folder moved to archive
- [ ] Archive contains all artifacts (proposal, specs, design, tasks, verify-report)
- [ ] Archived `tasks.md` has no unchecked implementation tasks
- [ ] No partial apply-progress remains unreconciled
- [ ] Active changes directory no longer has this change

**IF mode is `hive`:** Confirm all artifact observation IDs are recorded in the archive report, the tasks observation has no unchecked implementation tasks, `taskProgress` is complete, apply-progress is reconciled, and verify-report is current and passing.

**IF mode is `none`:** Skip verification — no persisted artifacts.

### Step 6: Persist Archive Report

For `hive` and `hybrid`, this step is MANDATORY — do NOT skip it.

For `none`, skip this step entirely: return the closure summary inline only and do not write files or call `mcp__hive__mem_save`.

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `archive-report`
- topic_key: `sdd/{change-name}/archive-report`
- type: `architecture`

The report must record status inputs used for the archive decision: `taskProgress`, `applyState`, `artifacts["verify-report"]`, `artifactPaths["verify-report"]`, `contextFiles["verify-report"]`, verify-report artifact content verdict/staleness, whether apply-progress was present or partial, and reconciliation/re-verification evidence.

### Step 7: Return Summary

Return to the orchestrator:

```markdown
## Change Archived

**Change**: {change-name}
**Archived to**: `openspec/changes/archive/{YYYY-MM-DD}-{change-name}/` (openspec/hybrid) | Hive archive report (hive) | inline (none)

### Archive Safety Status
| Check | Result | Evidence |
|-------|--------|----------|
| taskProgress | Complete | {status/artifact reference} |
| tasks artifact | Complete | {N}/{N} tasks complete |
| applyState | Reconciled | {state} |
| verify-report evidence | Passing and current | {artifact state/id/path/content reference} |
| CRITICAL findings | None unresolved | {summary} |

### Specs Synced
| Domain | Action | Details |
|--------|--------|---------|
| {domain} | Created/Updated | {N added, M modified, K removed, R renamed requirements} |

### Archive Contents
- proposal.md ✅
- specs/ ✅
- design.md ✅
- tasks.md ✅ ({N}/{N} tasks complete)
- verify-report.md ✅ (passing/current)

### Source of Truth Updated
The following specs now reflect the new behavior:
- `openspec/specs/{domain}/spec.md`

### SDD Cycle Complete
The change has been fully planned, implemented, verified, and archived.
Ready for the next change.
```

## Rules

- ALWAYS consume structured `jarvis.sdd-status` before archive; do not infer archive readiness from conversation alone.
- NEVER archive a change that has unresolved CRITICAL issues in its verification report.
- NEVER archive when the verify report is missing, failing, stale, or does not cover current artifacts.
- NEVER archive when implementation tasks are incomplete in task checkboxes or `taskProgress`.
- NEVER treat stale checkboxes, internal todos, or conversation claims as archive-ready evidence.
- NEVER archive when apply-progress is partial until tasks, apply-progress, and verify-report are reconciled and re-verified.
- NEVER fix archive readiness by editing generated artifacts; update source-of-truth Jarvis assets/templates or persisted SDD artifacts only.
- NEVER use user/orchestrator approval to archive with partial, missing, stale, or unreconciled artifacts; block and require reconciliation plus re-verification instead.
- In `none` mode, return closure status inline only; do not persist an archive report.
- ALWAYS sync delta specs BEFORE moving to archive.
- When merging into existing specs, PRESERVE requirements not mentioned in the delta.
- Use ISO date format (YYYY-MM-DD) for archive folder prefix.
- If the merge would be destructive (removing large sections), WARN the orchestrator and ask for confirmation.
- The archive is an AUDIT TRAIL — never delete or modify archived changes.
- If `openspec/changes/archive/` doesn't exist, create it.
- Apply any `rules.archive` from `openspec/config.yaml`.
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
