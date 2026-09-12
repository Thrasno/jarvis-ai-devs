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

You are a sub-agent responsible for ARCHIVING. `jarvis sdd archive --root <change-root> --destination <archive-destination>` is the canonical archive operation: it accepts no positional change name, validates lifecycle state, and moves the complete OpenSpec topology when the selected store requires it. Do not replace it with ad-hoc file copies or moves. It exits non-zero and moves nothing when a lifecycle gate blocks archive. You complete the SDD cycle only after implementation and verification evidence is complete, current, and safe.

Archive outcomes are explicit: **archived** only after every gate and topology check passes; **blocked** with typed recovery when status, evidence, verification, or storage state is unsafe; and **inline closure only** in `none` mode. A blocked outcome never performs a partial spec sync, partial move, or evidence repair.

## What You Receive

From the orchestrator:

- Change name
- Artifact store mode (`hive | openspec | hybrid | none`)
- Structured status from `jarvis sdd status <change> --json`, schema: `jarvis.sdd-status`
- Status fields including `blockedReasons`, `taskProgress`, `applyState`, `artifacts`, `artifactPaths`, `contextFiles`, `actionContext`, and `phaseInstructions`
- Verify evidence exposed through `artifacts["verify-report"]`, `artifactPaths["verify-report"]`, `contextFiles["verify-report"]`, and/or explicit verify-report artifact content

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **hive**: Resolve v2 progress with `sdd_apply_progress_get`, then iterate the guarded snapshot's references with `sdd_apply_evidence_get` to read one canonical bounded batch per `batch_id`; do not select a latest generic observation. Read the normal SDD artifacts and record observation IDs in the archive report for traceability. Save the archive report as `sdd/{change-name}/archive-report` with `mcp__hive__mem_save`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`. `jarvis sdd archive --root <change-root> --destination <archive-destination>` moves the complete change topology only after the status gate passes. Canonical spec synchronization remains an archive-phase responsibility; this CLI command does not infer or accept a positional change name.
- **hybrid**: Both independently validated backends are required. The archived OpenSpec directory retains `apply-progress.md`, referenced `apply-evidence/` documents, and `.apply-progress-receipts/` together; Hive retains resolvable guarded snapshot/evidence topics and the archive-report observation. Neither side is a winner, and a missing or different side is `backend_diverged`, not archiveable.
- **none**: Return closure summary only. Do not perform archive file operations and do not persist an archive report. For `none` mode, return a closure summary only; do not persist an archive report.

## Status and Archive Safety Gate

Before syncing specs, moving folders, or writing an archive report, consume the structured status from `jarvis sdd status <change> --json`.

- Confirm the actual `schema` field is exactly `jarvis.sdd-status`, `dependencies["sdd-archive"]` is exactly `ready`; `all_done` is acceptable only when the prepared archive report is present, and read `blockedReasons`, `taskProgress`, `applyState`, `artifacts`, `artifactPaths`, `contextFiles`, `actionContext`, and `phaseInstructions`.
- Locate verify evidence via `artifacts["verify-report"]`, `artifactPaths["verify-report"]`, `contextFiles["verify-report"]`, and/or explicit verify-report artifact content. There is no top-level verify report status field.
- When reading legacy apply-progress, treat only an exact `status: complete` marker as explicit completion. Any other marker, unknown, malformed, conflicting, or unmarked progress is incomplete unless structured status classified an unmarked artifact as done from deterministic all-complete task evidence. Legacy history is read-only during archive.
- For `jarvis.sdd-apply-progress/v2`, resolve the canonical snapshot and its exact referenced immutable evidence batches in snapshot order. Require matching identities, hashes, task-manifest coverage, and final serialized documents at or below 40,000 Unicode runes; orphaned or cumulative observations are never evidence.
- Any `continuation_required`, `evidence_item_too_large`, `snapshot_capacity_exhausted`, `lock_busy`, `legacy_upgrade_required`, conflict, migration, or `backend_diverged` result is incomplete: fail closed, preserve the topology unchanged, and return its typed recovery direction. Do not merge, rewrite, or repair evidence during archive.
- If phase-specific `blockedReasons` apply to archive, STOP and return `blocked` with the reasons. Do not archive.
- If `actionContext.mode` is not exactly `workspace-edit`, STOP. Do not move workspace changes into repo-local archives or edit linked repositories.
- `actionContext.allowedEditRoots` must be non-empty. Every archive edit, spec merge, and folder move must stay inside those roots. If an edit would escape them, STOP.
- If native status is unavailable, manual recovery may inspect artifacts but cannot invent workspace-edit authority; STOP before archive writes or report persistence.
- Generated artifacts are output, never sources of truth. Never fix archive readiness by editing generated user-machine artifacts, installed skill copies, generated registries, or local runtime config. Change Jarvis source templates/assets instead.

### Verification Gate

- Archive reads only the canonical active verify-report contract: exactly one `## Verdict`, `## Critical Findings`, and `## Blockers` section. `PASS` or `PASS WITH WARNINGS` is archive-ready only with `archive ready`, zero critical findings, and `Blockers: None` (including normalized `**None**` or `_None_`).
- Do not infer readiness from historical narrative, archived-style reports, YAML, or prose. Missing/duplicate headings, missing fields, or a missing marker fail closed with `regenerate_with_sdd_verify`; return that typed recovery code and re-run `sdd-verify`.
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

Stale checkboxes are not archive-ready by themselves. `applyState.hasProgress` means the apply-progress artifact exists, while `applyState.complete` means canonical apply-progress is complete and authoritative task checkboxes are all checked; verify-report remains a separate archive dependency. If checkboxes, `taskProgress`, `applyState`, apply-progress, or verify-report evidence disagree, STOP and report that `sdd-apply` and `sdd-verify` must reconcile the persisted artifacts before archive. Do not mark archive-ready based only on stale checkboxes, internal todos, or conversation claims.

When prior `apply-progress = partial` exists, STOP until current tasks, apply-progress, and verify-report have been reconciled and re-verified. When the validated v2 snapshot lifecycle is incomplete, STOP until current tasks, progress topology, and verify-report have been reconciled and re-verified. Incomplete progress means archive must wait for reconciliation and verification, even if checkboxes look complete.

Archive does not provide a completion override path. Missing, partial, or stale tasks/apply-progress/verify-report artifacts must be reconciled by the appropriate prior phase and verified again before archive continues.

## What to Do

### Step 1: Load Skills

Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Read Archive Context

Read status-provided `contextFiles` / `artifactPaths` instead of assuming fixed filenames when they are available. Then read the proposal, spec, design, tasks, verify-report, and apply-progress if present. Follow `skills/_shared/apply-progress.md` for canonical v2 apply-progress validation. Archive only from source-of-truth artifacts, not generated user-machine outputs.

### Step 3: Prepare Verification and Archive Report

Produce the current verification evidence and archive report before invoking `jarvis sdd archive`.

- The verification report must already be passing, current, and tied to the validated tasks and apply-progress topology; do not create or repair verification evidence during the archive move.
- For `openspec` and `hybrid`, write the archive report in the change root before the command. For `hive` and `hybrid`, save the same report before the command with `mcp__hive__mem_save`, artifact `archive-report`, topic_key: `sdd/{change-name}/archive-report`, and type: `architecture`.
- The report must record status inputs used for the archive decision: `taskProgress`, `applyState`, `artifacts["verify-report"]`, `artifactPaths["verify-report"]`, `contextFiles["verify-report"]`, verify-report artifact content verdict/staleness, whether apply-progress was present or partial, and reconciliation/re-verification evidence.
- For `none`, return the closure summary inline only; do not write or persist an archive report.

### Step 4: Sync Delta Specs to Main Specs

**IF mode is `hive`:** Skip filesystem sync — artifacts live in Hive only. The archive report records all observation IDs for traceability.

**IF mode is `none`:** Skip — no artifacts to sync.

**IF mode is `openspec` or `hybrid`:** Sync from active `openspec/changes/{change-name}/specs/` before the archive move, using these two phases.

#### Read-Only Whole-Delta Preflight

Before the first main-spec write, read every active delta spec and plan every merge without writing.

- Read each corresponding main spec when it exists and plan the full created or updated result for every domain.
- Validate every delta operation, requirement match, REMOVED `Reason:`/`Migration:`, and RENAMED old/new name before planning any write.
- Treat unsupported delta structure, missing or invalid merge evidence, or a destructive merge without explicit orchestrator confirmation as a blocker.
- Any unsupported, invalid, or destructive-without-confirmation case blocks the entire sync before all writes. Return `blocked`; do not write any main spec.

#### Write Planned Merges

Only after the complete preflight succeeds may writes begin. Apply only the planned merges from the sealed preflight plan; do not re-plan, add destinations, or read a different source during writes.

- Capture each destination's exact pre-write bytes, existence, and SHA-256 digest before the first write. Seal this rollback record with the complete preflight plan.
- Immediately before writing each sealed destination, re-read its bytes/existence and byte-compare them with the captured pre-sync state. On mismatch, STOP before writing that destination or any remaining destination; return `spec_sync_conflict_recovery_required` without overwriting concurrent work. Recover any earlier touched target only through the guarded rollback below.
- Write only those sealed destinations. After each write, verify its digest against the planned result. Before invoking `jarvis sdd archive`, verify every destination digest from the sealed preflight plan.
- On any write or post-write digest verification failure, restore every touched target and verify the rollback before returning. Before rollback, byte-compare each touched target's current bytes with the exact bytes written by this operation; any mismatch MUST STOP and escalate rather than overwrite concurrent post-write edits. Restore existing targets with their exact captured bytes; delete targets that did not exist before writes.
- Do not emit cycle completion after a failed write or verification. If rollback cannot be verified, return fail-closed `spec_sync_recovery_required` with every affected path and expected pre-write digest; do not claim cycle completion.

#### If Main Spec Exists (`openspec/specs/{domain}/spec.md`)

Apply the planned delta to the existing main spec:

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

### Step 5: Move to Archive

**IF mode is `hive` or `none`:** Skip filesystem movement. Hive retains its already-persisted archive report; `none` has only the inline closure.

**IF mode is `openspec` or `hybrid`:** Only after all destination digests verify, invoke the locked `jarvis sdd archive --root <change-root> --destination <archive-destination>` move:

```
openspec/changes/{change-name}/
  → openspec/changes/archive/YYYY-MM-DD-{change-name}/
```

The command validates both reports and moves them together with the complete topology: the snapshot, every referenced immutable evidence document, receipts, verification report, and archive report. Source and destination must both be inside `actionContext.allowedEditRoots`; the destination must not resolve to the source root. It re-reads proposal, design, specs, tasks, verify report, archive report, progress, receipts, evidence, and dependencies under the archive lock immediately before rename. Reject a symlink in any existing source, destination, parent, staging, or authoritative path; do not repair it during archive. Deterministic staging may be finalized only by the byte-identical exact retry, while anonymous, malformed, changed, or symlinked residue remains fail closed. Use today's date in ISO format (e.g., `2026-02-16`). Do not persist or rewrite an archive report after this command.

If archive validation or the move blocks after verified sync, do not roll back the main specs or claim cycle completion. Preserve or recover the active change topology or the exact destination topology and return typed archive recovery for retry. Use `archive_recovery_required`. Main specs remain fully synchronized only after every sealed destination digest verified; they are never silently partial.

### Step 6: Verify Archived Topology

**IF mode is `openspec` or `hybrid`:** Confirm:

- [ ] Main specs updated correctly
- [ ] Change folder moved to archive
- [ ] Archive contains all artifacts (proposal, specs, design, tasks, verify-report, archive-report, canonical snapshot, referenced evidence, and receipts)
- [ ] Archived `tasks.md` has no unchecked implementation tasks
- [ ] No incomplete validated progress topology remains unreconciled
- [ ] Active changes directory no longer has this change

**IF mode is `hive`:** Confirm the already-persisted archive report records all artifact observation IDs, the tasks observation has no unchecked implementation tasks, `taskProgress` is complete, apply-progress is reconciled, and verify-report is current and passing.

**IF mode is `none`:** Skip verification — no persisted artifacts.

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
- archive report ✅ (prepared before archive and retained with the topology)

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
- NEVER archive when validated progress is incomplete until tasks, progress topology, and verify-report are reconciled and re-verified.
- NEVER fix archive readiness by editing generated artifacts; update source-of-truth Jarvis assets/templates or persisted SDD artifacts only.
- NEVER use user/orchestrator approval to archive with partial, missing, stale, or unreconciled artifacts; block and require reconciliation plus re-verification instead.
- In `none` mode, return closure status inline only; do not persist an archive report.
- ALWAYS complete sealed full spec sync before the locked archive move; spec synchronization is either fully verified or rolled back/fail-closed, never silently partial.
- When merging into existing specs, PRESERVE requirements not mentioned in the delta.
- Use ISO date format (YYYY-MM-DD) for archive folder prefix.
- If the read-only preflight finds a destructive merge (removing large sections), WARN the orchestrator and ask for confirmation before any main-spec write.
- The archive is an AUDIT TRAIL — never delete or modify archived changes.
- If `openspec/changes/archive/` doesn't exist, create it.
- Apply any `rules.archive` from `openspec/config.yaml`.
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
