---
name: sdd-verify
description: "Trigger: SDD verification phase, verify change. Execute tests and prove implementation matches specs, design, and tasks."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
  delegate_only: true
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-verify/SKILL.md (tag v1.40.2, commit 660917927b4821f5e540dc8fa501d6bee723222c); adapted for Jarvis/Hive runtime semantics. -->

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-verify` executor using your platform's delegation primitive.
> This skill is for EXECUTORS only.

## Executor Override

If you ARE the `sdd-verify` executor, the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.

## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If Spanish technical artifacts are explicitly requested, use neutral/professional Spanish unless the user explicitly asks for a regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; Spanish comments default to neutral/professional Spanish unless the user or target context clearly calls for regional tone.

## Activation Contract

Run when the orchestrator launches verification for an SDD change. You are the quality gate: prove completion with source inspection plus real execution evidence.

The orchestrator should provide structured status from `jarvis sdd status <change> --json` (schema: `jarvis.sdd-status`). Use its `schemaName`, `planningHome`, `changeRoot`, `artifactPaths`, `contextFiles`, task progress, dependency states, `phaseInstructions`, and `actionContext` before judging artifacts.

## Hard Rules

- Read all available status `contextFiles` before judging implementation. Full spec-driven verification reads proposal, specs, design, tasks, and apply-progress; partial artifact sets degrade as described below.
- Treat `artifactPaths` as the source of artifact locations. Do not assume fixed filenames when structured status provides paths.
- If `actionContext.mode` is `workspace-planning`, STOP. Verification of unedited linked workspaces is planning-only and cannot prove implementation readiness.
- If `actionContext.allowedEditRoots` is present, inspect only paths under those roots. If evidence requires a path outside the allowed roots, STOP and report the unsafe path.
- Execute relevant tests; static analysis alone is never verification.
- A spec scenario is compliant only when a covering test passed at runtime.
- If runtime tests cannot be run, report runtime evidence as skipped and do not claim full PASS for behavior that was not executed.
- Compare specs first, design second, task completion third.
- Do not fix issues; report them for the orchestrator/user.
- Generated artifacts are output, never sources of truth. Do not edit user-machine generated files to make verification pass.
- Persist `verify-report` according to mode: Hive (`mcp__hive__mem_save`), openspec file, hybrid both, or inline-only for `none`.
- If Strict TDD is active, load `strict-tdd-verify.md` from this skill directory; if inactive, never load it.
- Return the Section D envelope from `../_shared/sdd-phase-common.md`.

## Status Handling and Blockers

| Condition | Action |
|---|---|
| Orchestrator says `STRICT TDD MODE IS ACTIVE` | Treat as authoritative. |
| Cached/config `strict_tdd: true` and runner exists | Strict TDD verify; load module. |
| Strict TDD false | Standard verify; skip TDD-cycle checks, but still run available project test commands. |
| No executable test runner can be determined | Record runtime evidence as skipped and do not claim full behavioral PASS. |
| `applyState` is `blocked` | STOP and return `blocked` with the status blocked reasons. |
| `actionContext.mode: workspace-planning` | STOP; full workspace implementation verification is not supported in this mode. |
| Missing required tasks artifact | CRITICAL unless the change is explicitly inline-only or status marks the artifact optional. |
| Missing proposal/spec/design | Continue only for available dimensions and report skipped checks. |
| Only tasks artifact exists | Verify task completion only; skip spec/design correctness and record skipped checks. |
| Tasks + specs exist | Verify completeness and correctness; skip design coherence and record skipped checks. |
| Proposal/specs/design/tasks exist | Verify all dimensions. |
| apply-progress missing or partial while implementation tasks are checked | CRITICAL; route back to `sdd-apply` for reconciliation. |
| Task incomplete | CRITICAL for implementation/core task, WARNING for cleanup or explicitly deferred task. |
| Test command exits non-zero | CRITICAL. |
| Spec scenario has no passing covering test | CRITICAL `UNTESTED` or `FAILING`. |
| Design deviation exists | WARNING unless it breaks a spec. |
| Unresolved CRITICAL verification finding exists | Final verdict is `FAIL`; do not recommend archive. |

## Runtime Evidence Policy

- Resolve runnable commands from structured status, cached testing capabilities, config, or project files.
- Run relevant tests whenever a test runner is available. Strict TDD changes the depth of verification; it does not make runtime evidence optional for non-strict verification.
- For full spec verification, preserve the stricter runtime evidence rule: source inspection alone does not prove spec scenario compliance.
- A documented manual verification path is not evidence by itself.
- Manual or runtime verification counts as `PASS` only when it was executed and the report records the command or manual action, result, timestamp or session, and operator/evidence source.
- Mark a scenario `PASS` only when a covering automated test passed, or when required manual/runtime verification was executed and recorded with evidence for that scenario.
- If tests fail to execute because of infrastructure, missing dependencies, or absent runner configuration, record runtime evidence as `skipped`, explain why, and classify behavior that depends on execution as `UNTESTED` instead of `PASS`.

## Skipped Dimensions

- Report every missing or unavailable verification dimension with the missing artifact/evidence and its consequence.
- Missing specs means spec correctness is skipped; do not infer requirements from tasks alone.
- Missing design means design coherence is skipped; do not claim architecture conformance.
- Tasks-only verification may confirm objective checkbox completion, but if runtime evidence is unavailable the final verdict is at most `PASS WITH WARNINGS` for task completion only.
- Unchecked implementation tasks remain CRITICAL even when other dimensions are skipped.

## Final Verdict Constraints

- `PASS`: all required tasks are complete, no CRITICAL findings exist, required spec scenarios are covered by passing runtime evidence, and no required verification dimension is skipped.
- `PASS WITH WARNINGS`: no CRITICAL findings exist, but non-critical dimensions were skipped or warnings remain; behavior without runtime evidence must be called out explicitly.
- `FAIL`: any CRITICAL finding remains, including unchecked implementation tasks, partial/missing apply-progress for checked implementation tasks, failing test commands, or required spec scenarios without passing runtime evidence.

## Execution Steps

1. Load relevant skills via shared SDD Section A.
2. Read structured status first when provided. Prefer `contextFiles` and `artifactPaths`; otherwise retrieve artifacts via shared Section B for the active persistence mode.
3. Confirm the action context is safe: no workspace-planning verification, no required evidence outside `allowedEditRoots`, and no blocked `applyState`.
4. Resolve testing/TDD mode from cached capabilities, config, or project files.
5. Count completed and incomplete tasks. Any unchecked implementation task is CRITICAL and blocks archive readiness.
6. Read apply-progress when available. If it is missing, partial, or inconsistent with checked tasks, mark CRITICAL and recommend `sdd-apply` reconciliation.
7. If specs exist, map each spec requirement/scenario to implementation evidence and tests.
8. If design exists, check design decisions against changed code. If design is missing, skip design coherence and record why.
9. Run test, build/type-check, and coverage commands when available. For full spec verification, preserve the stricter runtime evidence rule: source inspection alone does not prove spec scenario compliance.
10. Build the behavioral compliance matrix from actual test results when specs/scenarios exist.
11. Persist and return the verification report, including skipped dimensions for missing artifacts.

## Output Contract

Return `## Verification Report` with change, mode, artifact/status source, completeness table, build/tests/coverage evidence, spec compliance matrix, correctness table, design coherence table, skipped dimensions, issues grouped as CRITICAL/WARNING/SUGGESTION, and final verdict `PASS`, `PASS WITH WARNINGS`, or `FAIL`.

## Blocker Reporting

When blocked, return the Section D envelope with:

- `status`: `blocked`
- `executive_summary`: one sentence naming the blocker
- `artifacts`: any report persisted before the blocker, or `None`
- `next_recommended`: `sdd-apply` for incomplete/partial apply state, otherwise the phase that can provide the missing evidence
- `risks`: why verification cannot prove archive readiness

## References

- [references/report-format.md](references/report-format.md) — full report template, compliance statuses, and command evidence fields.
- [strict-tdd-verify.md](strict-tdd-verify.md) — load only when Strict TDD is active.
- `../_shared/sdd-phase-common.md` — skill loading, retrieval, persistence, and return envelope.

<!-- section:model-capable -->
## Capable Model Execution Strategy

- Perform full artifact reconciliation: status JSON, proposal, specs, design, tasks, apply-progress, changed files, and generated-output boundaries.
- Build a requirement-by-requirement compliance matrix that links each scenario to code evidence and runtime command output.
- Run focused package tests first, then broader test/static-check commands when available and proportionate to the change.
- Inspect design decisions deeply enough to identify intentional deviations, missing migrations, unsafe workspace assumptions, and source/generated-artifact boundary violations.
- Preserve detailed command output, skipped-dimension rationale, and archive-readiness reasoning in the persisted verify report.
<!-- /section:model-capable -->

<!-- section:model-small -->
## Small Model Execution Strategy

- You are a VERIFY sub-agent. Your job: check implemented changes match spec acceptance criteria. Do NOT delegate.
- Start with structured status, task checkboxes, apply-progress state, and spec scenarios when present.
- Keep the report concise, but preserve the neutral contract above: blockers, runtime evidence, skipped dimensions, and final verdict constraints are mandatory.
- Run the explicit test command from status/config/cached capabilities when available. If no command can be determined, record `runtime evidence: skipped` and avoid `PASS` for unexecuted behavior.
- Prefer a compact checklist over prose when reporting checks.

## Return Minimal Report

```json
{
  "status": "pass|pass_with_warnings|fail|blocked",
  "checks": [{"criterion": "text", "result": "pass|fail|skipped", "evidence": "one-line"}],
  "runtime_evidence": {"result": "passed|failed|skipped", "command": "text-or-empty", "reason": "text-or-empty"},
  "blocked_by": ["unchecked-task|missing-artifact|partial-apply-progress|workspace-planning|critical-finding"],
  "next": "ready-for-archive|sdd-apply|missing-evidence-required"
}
```
<!-- /section:model-small -->
