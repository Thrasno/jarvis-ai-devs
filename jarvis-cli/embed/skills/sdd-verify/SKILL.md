---
name: sdd-verify
display_name: "SDD Verify"
description: "Verify implementation against specs with structural and behavioral checks. Trigger: When verifying implementation"
disable-model-invocation: true
user-invocable: false
license: MIT
scope: core
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

Run when the orchestrator launches verification for an SDD change. You are the quality gate: prove completion with source inspection plus real execution evidence when a test runner exists, or with an explicit static review when none exists.

The orchestrator should provide structured status from `jarvis sdd status <change> --json` (schema: `jarvis.sdd-status`). Use its actual `schema`, `planningHome`, `changeRoot`, `artifactPaths`, `contextFiles`, `blockedReasons`, `dependencies`, task progress, `phaseInstructions`, and `actionContext` before judging artifacts.

## Hard Rules

- Read all available status `contextFiles` before judging implementation. Full spec-driven verification reads proposal, specs, design, tasks, and apply-progress; partial artifact sets degrade as described below.
- Follow `../_shared/apply-progress.md` for canonical v2 apply-progress validation.
- Treat `artifactPaths` as the source of artifact locations. Do not assume fixed filenames when structured status provides paths.
- Confirm the status field `schema` is exactly `jarvis.sdd-status` and `dependencies["sdd-verify"]` is exactly `ready`; otherwise STOP and return the phase-specific `blockedReasons`.
- When reading apply-progress, treat only an exact `status: complete` marker as explicit completion. Treat `status: partial`, unknown, malformed, conflicting, or unmarked progress as incomplete unless structured status classified an unmarked artifact as done from deterministic all-complete task evidence.
- If `actionContext.mode` is not exactly `workspace-edit`, STOP. Verification of unedited linked workspaces is planning-only and cannot prove implementation readiness.
- `actionContext.allowedEditRoots` must be non-empty. Inspect only paths under those roots. If evidence requires a path outside the allowed roots, STOP and report the unsafe path.
- If native status is unavailable, manual recovery may inspect artifacts but cannot invent workspace-edit authority; STOP before verification or report persistence.
- Stop early: if any non-operator implementation task is unchecked or apply-progress is not complete, return `blocked` before running any command (no suite, no coverage), listing the pending tasks.
- When a runnable test command exists, execute it; static analysis alone is never verification.
- With a runnable test command, a spec scenario is compliant only when a covering test passed at runtime.
- Static review applies only when no runnable test command exists and never counts as a test-backed PASS.
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
| Orchestrator says `STRICT TDD MODE IS ACTIVE` | Strict TDD verify; treat as authoritative and load `strict-tdd-verify.md`. |
| Orchestrator says `TDD MODE: standard` | Standard verify; never load `strict-tdd-verify.md`, even if cached capabilities suggest strict. Still run available project test commands. |
| Nothing forwarded, cached `strict_tdd_suggestion: strict`, and a runner exists | Legacy only: Strict TDD verify; load module. Use legacy `strict_tdd: true` only when the suggestion is absent. |
| Nothing forwarded and cached suggestion is `standard` | Standard verify; skip TDD-cycle checks, but still run available project test commands. |
| No runnable test command can be determined | Static verify (`Verification mode: static`): no execution; scenarios are `static-reviewed`; the maximum verdict is `PASS WITH WARNINGS`, which stays archive-ready. Never load `strict-tdd-verify.md`. |
| `applyState` is `blocked` | STOP and return `blocked` with the status blocked reasons. |
| `actionContext.mode: workspace-planning` | STOP; full workspace implementation verification is not supported in this mode. |
| Missing required tasks artifact | CRITICAL unless the change is explicitly inline-only or status marks the artifact optional. |
| Missing proposal/spec/design | Continue only for available dimensions and report skipped checks. |
| Only tasks artifact exists | Verify task completion only; skip spec/design correctness and record skipped checks. |
| Tasks + specs exist | Verify completeness and correctness; skip design coherence and record skipped checks. |
| Proposal/specs/design/tasks exist | Verify all dimensions. |
| apply-progress missing or partial while implementation tasks are checked | CRITICAL; return `blocked` before running any command and route back to `sdd-apply` for reconciliation. |
| Unchecked implementation/core task | CRITICAL; return `blocked` before running any command, list the pending tasks, and route to `sdd-apply`. |
| Unchecked cleanup or explicitly deferred task | WARNING; continue verification. |
| Unchecked `[operator]` task | Report `pending-operator`, not CRITICAL. It still blocks archive readiness; route to `sdd-apply`, which pauses for the developer's acknowledgement. |
| Test command exits non-zero | CRITICAL. |
| Runtime mode: spec scenario has no passing covering test | CRITICAL `UNTESTED` or `FAILING`. |
| Static mode: changed code contradicts or omits a spec scenario | CRITICAL. |
| Design deviation exists | WARNING unless it breaks a spec. |
| Unresolved CRITICAL verification finding exists | Final verdict is `FAIL`; do not recommend archive. |

## Runtime Evidence Policy

- Resolve runnable commands from forwarded status, cached testing capabilities, config, or project files. A runnable test command selects `Verification mode: runtime`; none selects `Verification mode: static`.

### Runtime mode

- Run the project's test command once for the change and each available quality command (vet, lint, type-check) once. Strict TDD changes the depth of evidence review; it does not make runtime evidence optional for non-strict verification.
- Do not re-run each apply GREEN command individually; a passing suite that includes the covering tests confirms GREEN evidence.
- Coverage, the assertion audit, and the coverage allocation audit are optional and warn-only: run them only when cheap or requested; they never block the verdict.
- Source inspection alone does not prove spec scenario compliance.

### Static mode

- Run no commands. Read the changed code against each spec scenario and against the loaded project skills' rules (for example `zoho-deluge` conventions).
- Report each reviewed scenario as `static-reviewed`, never `COMPLIANT` or `PASS` by test. Scenarios that depend on the tenant follow the `[operator]` rules below.
- Add the WARNING `no test runner: static review only`. Missing runtime evidence that cannot exist is not CRITICAL and does not block archive.

### Evidence rules

- Strict TDD evidence (RED, GREEN, completion coverage) is validated by reading apply-progress, never by re-executing apply commands; read the safety-net baseline once from the apply return or the first task's red summary.
- A documented manual verification path is not evidence by itself.
- Manual or runtime verification counts as `PASS` only when it was executed and the report records the command or manual action, result, timestamp or session, and operator/evidence source.
- Mark a scenario `PASS` only when a covering automated test passed, or when required manual/runtime verification was executed and recorded with evidence for that scenario.
- A spec scenario that depends on an `[operator]` task is reported `operator-attested`, never `PASS` backed by a test: the developer's acknowledgement is attestation, not runtime evidence. It is neither CRITICAL nor `UNTESTED`. Never ask for screenshots, IDs, or logs, and never use Zoho MCP servers to check it; they point at our own account, never the client tenant.
- If tests fail to execute because of infrastructure, missing dependencies, or absent runner configuration, record runtime evidence as `skipped`, explain why, and classify behavior that depends on execution as `UNTESTED` instead of `PASS`.

## Skipped Dimensions

- Report every missing or unavailable verification dimension with the missing artifact/evidence and its consequence.
- Missing specs means spec correctness is skipped; do not infer requirements from tasks alone.
- Missing design means design coherence is skipped; do not claim architecture conformance.
- Tasks-only verification may confirm objective checkbox completion, but if runtime evidence is unavailable the final verdict is at most `PASS WITH WARNINGS` for task completion only.
- Unchecked implementation tasks remain CRITICAL even when other dimensions are skipped.

## Final Verdict Constraints

- `PASS`: all required tasks are complete, no CRITICAL findings exist, required spec scenarios are covered by passing runtime evidence, and no required verification dimension is skipped. Static mode never yields `PASS`.
- `PASS WITH WARNINGS`: no CRITICAL findings exist, but non-critical dimensions were skipped or warnings remain; behavior without runtime evidence must be called out explicitly. It is the maximum verdict in static mode, with the WARNING `no test runner: static review only`, and it is archive-ready as `**PASS WITH WARNINGS — archive ready.**`.
- `FAIL`: any CRITICAL finding remains, including partial/missing apply-progress for checked implementation tasks, failing test or quality commands, required spec scenarios without passing runtime evidence in runtime mode, or scenarios the static review finds contradicted or missing.

## Execution Steps

1. Load relevant skills via shared SDD Section A.
2. Read structured status first when provided. Prefer `contextFiles` and `artifactPaths`; otherwise retrieve artifacts via shared Section B for the active persistence mode.
3. Confirm native status authority: `schema` is `jarvis.sdd-status`, `dependencies["sdd-verify"]` is `ready`, `blockedReasons` do not block verify, `actionContext.mode` is `workspace-edit`, and `allowedEditRoots` is non-empty. Do not use manual recovery to bypass any missing authority.
4. Resolve TDD mode: the forwarded TDD mode first (`STRICT TDD MODE IS ACTIVE` → strict; `TDD MODE: standard` → standard); only when nothing was forwarded, use the cached `strict_tdd_suggestion` (legacy `strict_tdd` when the suggestion is absent). Resolve runnable test commands from forwarded status, cached capabilities, config, or project files either way; that selects `Verification mode: runtime` or `Verification mode: static`.
5. Count completed and incomplete tasks. Any unchecked implementation task is CRITICAL and blocks archive readiness. An unchecked `[operator]` task is reported `pending-operator` instead, not CRITICAL; it still blocks archive readiness until `sdd-apply` records the developer's acknowledgement.
6. Stop early: if any non-operator implementation task is unchecked or apply-progress is not complete, return `blocked` now, before running any command, with the pending task IDs and `next_recommended: sdd-apply`.
7. Read apply-progress. In Strict TDD, validate the v2 evidence structure by reading it (`strict-tdd-verify.md`); do not re-execute it.
8. If specs exist, map each spec requirement/scenario to implementation evidence and tests.
9. If design exists, check design decisions against changed code. If design is missing, skip design coherence and record why.
10. Runtime mode: Run the project's test command once for the change and each available quality command (vet, lint, type-check) once; add coverage or audits only when cheap or requested, as warnings. Static mode: run nothing and review the changed code against each scenario and the loaded project skills' rules.
11. Build the compliance matrix from the test results (runtime) or the static review (static) when specs/scenarios exist.
12. Persist and return the verification report, including the verification mode and skipped dimensions for missing artifacts.

## Canonical Active Verify Report

Persist one active report with these headings exactly once and exactly as written:

```markdown
## Verdict

**PASS — archive ready.**

## Critical Findings

0

## Blockers

None
```

`PASS WITH WARNINGS` is also archive-ready only in this exact verdict form:
`**PASS WITH WARNINGS — archive ready.**`.

Emit the phrase `archive ready` only when `Critical Findings` is `0` and `Blockers` is `None`. `None`, `**None**`, and `_None_` are equivalent blocker values after Markdown normalization. For `FAIL`, a nonzero critical count, or actual blockers, do not emit `archive ready`.

The runtime consumes only these three active sections. Each must occur once; do not use a historical, archived-style, YAML, or prose report as a fallback. Any missing or duplicate active heading, missing field, or missing archive-ready marker must be regenerated by `sdd-verify` with code `regenerate_with_sdd_verify`.

## Output Contract

Return `## Verification Report` with change, TDD mode, verification mode (`runtime` or `static`), artifact/status source, completeness table, build/tests/coverage evidence, spec compliance matrix, correctness table, design coherence table, skipped dimensions, issues grouped as CRITICAL/WARNING/SUGGESTION, and final verdict `PASS`, `PASS WITH WARNINGS`, or `FAIL`. End the persisted report with the canonical active sections above.

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
- `../_shared/apply-progress.md` — canonical v2 apply-progress validation.
- `../_shared/sdd-phase-common.md` — skill loading, retrieval, persistence, and return envelope.

<!-- section:model-capable -->
## Capable Model Execution Strategy

- Perform full artifact reconciliation: status JSON, proposal, specs, design, tasks, apply-progress, changed files, and generated-output boundaries.
- Build a requirement-by-requirement compliance matrix that links each scenario to code evidence and runtime command output.
- Run the project's test command and quality commands once each; do not re-run individual apply commands.
- Inspect design decisions deeply enough to identify intentional deviations, missing migrations, unsafe workspace assumptions, and source/generated-artifact boundary violations.
- Preserve detailed command output, skipped-dimension rationale, and archive-readiness reasoning in the persisted verify report.
<!-- /section:model-capable -->

<!-- section:model-small -->
## Small Model Execution Strategy

- You are a VERIFY sub-agent. Your job: check implemented changes match spec acceptance criteria. Do NOT delegate.
- Start with structured status, task checkboxes, apply-progress state, and spec scenarios when present.
- Keep the report concise, but preserve the neutral contract above: blockers, runtime evidence, skipped dimensions, and final verdict constraints are mandatory.
- Stop with `blocked` before any command when a non-operator implementation task is unchecked or apply-progress is not complete.
- Run the explicit test command from status/config/cached capabilities once when available. If no command can be determined, review statically: scenarios are `static-reviewed`, the status is at most `pass_with_warnings`, and the warning is `no test runner: static review only`.
- Prefer a compact checklist over prose when reporting checks.

## Return Minimal Report

```json
{
  "status": "pass|pass_with_warnings|fail|blocked",
  "verification_mode": "runtime|static",
  "checks": [{"criterion": "text", "result": "pass|fail|static-reviewed|skipped", "evidence": "one-line"}],
  "runtime_evidence": {"result": "passed|failed|skipped", "command": "text-or-empty", "reason": "text-or-empty"},
  "blocked_by": ["unchecked-task|pending-operator|missing-artifact|partial-apply-progress|workspace-planning|critical-finding"],
  "next": "ready-for-archive|sdd-apply|missing-evidence-required"
}
```
<!-- /section:model-small -->
