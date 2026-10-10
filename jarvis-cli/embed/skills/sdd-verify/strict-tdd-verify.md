<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-verify/strict-tdd-verify.md -->
<!-- Upstream commit: 660917927b4821f5e540dc8fa501d6bee723222c. -->
<!-- Provenance: Upstream-derived from Gentle AI v1.40.2 and adapted for Jarvis/Hive runtime wording plus Jarvis-specific verification policy additions beyond runtime wording. -->
<!-- Maintenance guard: Future parity runs MUST NOT overwrite Jarvis-specific verification policy without maintainer approval. -->
# Strict TDD Module — Verify Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a runnable test command exists.**
> Without a test runner, verify runs in static mode and never loads this module.

## TDD Verification Philosophy

When Strict TDD Mode is active, verification goes beyond "does the code work?" to "was the code built correctly?" The apply phase records TDD evidence; your job is to validate that evidence structurally and confirm it with the single verify suite run.

Strict TDD verification has two responsibilities:

1. Confirm the RED → GREEN → REFACTOR cycle was followed with executed evidence.
2. Confirm the resulting tests protect behavior instead of creating a weak TDD pass.

## Step 5a: TDD Compliance Check

Read the canonical v2 snapshot and exactly its referenced immutable evidence batches. In Hive use `sdd_apply_progress_get`, then iterate ordered `snapshot.batches` with `sdd_apply_evidence_get` for each `batch_id`; in OpenSpec read `apply-progress.md` and only snapshot-referenced `apply-evidence/<batch-id>.json`. Validate evidence by reading it; never re-execute apply commands.

```text
Resolve canonical v2 progress:
├── Validate snapshot identity, hashes, task-manifest digest, coverage, and ordered batch references
├── FOR EACH EvidenceEntry in snapshot batch order:
│   ├── Require entry_id, task_ids, completes_task_ids, kind, summary, command, exit_code, outcome, and files
│   ├── IMPORTED (`kind=imported`): preserve it as migration provenance only; it NEVER satisfies RED, GREEN, TRIANGULATE, or REFACTOR quality for Strict TDD
│   ├── OPERATOR (`kind=operator`): developer attestation only; it NEVER satisfies RED, GREEN, TRIANGULATE, or REFACTOR. It completes only its `[operator]` task, which needs no TDD evidence; report dependent scenarios `operator-attested`
│   ├── RED (`kind=red`): must record an executed focused failing command, non-zero exit code, failure summary, and real test file
│   ├── GREEN (`kind=green`): must record an executable passing command and zero exit code; the single verify suite run confirms it
│   ├── TRIANGULATE (`kind=triangulate`): verify varied meaningful cases; accept `outcome=not_run` only with a structural one-output reason
│   ├── REFACTOR (`kind=refactor`): require a post-refactor passing command or an explicit no-refactor rationale
│   └── Flag CRITICAL for malformed, hypothetical, missing, unreferenced, or non-executable RED/GREEN evidence
├── Verify each completes_task_ids value is covered by a matching validated entry and current frozen task manifest
├── If no referenced v2 EvidenceEntry values exist, flag CRITICAL — Strict TDD was enabled but apply did not persist evidence
└── Summary: "{N}/{total} tasks have complete structured TDD evidence"
```

Safety net: read the baseline once from the apply return or the first task's red summary; do not require per-modified-file baseline entries. A missing baseline is a WARNING.

## Step 5b: Suite Cross-Check

Run the project's test command once for the change. Do not run each referenced GREEN command individually: a passing suite that includes the covering tests confirms GREEN evidence. Map each GREEN entry's test files to the suite result; flag CRITICAL when a covering test fails or is absent from the suite.

If the suite cannot run because tooling or infrastructure is unavailable, report the exact blocker under skipped dimensions. Missing execution evidence cannot be upgraded to PASS.

## Step 5c: Optional Audits (warn-only)

Run these only when cheap or requested. Findings are WARNING or SUGGESTION and do not block the verdict, except a found test that cannot fail (see Banned Assertion Patterns), which is CRITICAL. No audit compensates for missing RED/GREEN evidence. When an audit is not run, record it as skipped.

### Test Layer Distribution

Classify changed test files as unit, integration, E2E, or unknown, cross-referenced with cached testing capabilities. For each spec scenario, note which test layer covers it. Flag WARNING when tests depend on unavailable tooling. Layer distribution does not excuse missing behavior coverage.

### Coverage Allocation Audit

Coverage belongs at the cheapest deterministic layer that proves the behavior:

- Pure logic, parsing, mapping, validation, command construction, artifact rendering → deterministic unit coverage.
- Package wiring, filesystem effects, CLI behavior, API boundaries → deterministic integration coverage.
- Full user journeys across real process/browser/service boundaries → E2E, only for the journey risk.

Flag WARNING when behavior is covered only by E2E or broad integration tests but deterministic unit or lower-layer integration tests should cover it. Flag WARNING when coverage is E2E-heavy or over-integrated (expensive, broad, flaky, or dependent on unrelated wiring) and deterministic lower-layer tests are missing. Do not accept expensive E2E coverage as a substitute for cheaper deterministic coverage of pure logic, parsing, mapping, validation, command construction, or artifact rendering. This audit does not ban E2E tests.

### Changed File Coverage

When a coverage tool is available, run it once and report line coverage (branch when available) and uncovered ranges for files created or modified by this change (from EvidenceEntry `files` and the current git diff). Flag WARNING below the configured threshold, or below 80% when none exists.

Go coverage example: `go test ./... -coverprofile=/tmp/opencode/jarvis-sdd-verify.coverprofile`

Without a tool, report: "Coverage analysis skipped — no coverage tool detected".

### Banned Assertion Patterns

Running the assertion audit is optional. When it runs, scan changed test files and record file, line, assertion, and issue for each match. A test that cannot fail proves nothing, so these four findings are CRITICAL whenever the audit finds them:

- Tautologies: `expect(true).toBe(true)`, `assert True`, `if got != got`.
- Assertions with no production-code execution (no function call, render, request, command, or package boundary).
- Ghost loops: assertions inside loops whose body can execute zero times.
- Setup that prevents the target code path from running.

The remaining patterns are WARNING:

- Orphan empty checks without a companion non-empty case.
- Type-only or non-nil checks used as the only proof.
- Smoke-test-only: render/startup/existence with no asserted behavior.
- Implementation-detail coupling: CSS classes, internal state, incidental mock call counts.
- Mock/assertion ratio: mocks or fakes more than 2× behavior assertions, or a fake that returns the expected answer without exercising production logic.
- Triangulation: a multi-path behavior with only one meaningful case, or cases that all assert the same trivial shape.

### Behavior coverage vs implementation-only tests

Tests should prove behavior visible to the user, caller, CLI operator, API consumer, or persisted artifact contract. For Go contract tests, prefer explicit `got`/`want` checks where `got` comes from production code or a real artifact under test.

### Mock/Fake Hygiene

Mocks and fakes may isolate a boundary but must not replace the behavior being verified. Recommend extracting pure logic before adding many mocks, and moving to integration/E2E when behavior depends on real wiring.

## Report Template Extension

When Strict TDD Mode is active, the verification report MUST include:

```markdown
### TDD Compliance
| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ / ❌ | {Found in apply-progress / Missing} |
| RED confirmed | ✅ / ❌ | {N}/{total} tasks have executed failing-command evidence |
| GREEN confirmed | ✅ / ❌ | {N}/{total} covering tests pass in the verify suite run |
| REFACTOR confirmed | ✅ / ⚠️ / ➖ | {post-refactor command, no-refactor rationale, or missing evidence} |
| Triangulation adequate | ✅ / ⚠️ / ➖ | {N} tasks triangulated / {N} structural skips |
| Safety net baseline | ✅ / ⚠️ | {recorded once in apply return / first red summary, or missing} |

### Quality Metrics
**Linter/static analysis**: ✅ No errors / ⚠️ {N} warnings / ❌ {N} errors / ➖ Not available
**Type checker/compiler**: ✅ No errors / ❌ {N} errors / ➖ Not available
```

When the optional audits run, add `### Test Layer Distribution`, `### Changed File Coverage`, and `### Assertion Quality` sections; the assertion table uses `| File | Line | Assertion | Issue | Severity |`.

## Skipped Dimensions and Uncertainty

Report every verification dimension that was skipped, unavailable, or uncertain. Missing optional tooling is not a failure, but skipped TDD evidence is still a finding.

- Skipped coverage, audits, linter, type-check, integration, or E2E tooling → report as skipped with impact.
- Missing RED or GREEN evidence → CRITICAL; missing triangulation rationale or safety-net baseline → WARNING.
- Do not upgrade a skipped dimension to PASS. Say exactly what was not verified.

## Rules (Strict TDD Verify specific)

- ALWAYS check snapshot-referenced v2 EvidenceEntry batches — they are the primary artifact.
- ALWAYS cross-reference entry `files` against the single suite run — do not trust evidence blindly, and do not re-execute each entry's `command`.
- If canonical v2 progress has no referenced TDD evidence entries, flag CRITICAL.
- Imported legacy evidence never counts toward Strict-TDD quality; require fresh RED/GREEN evidence for the completed task.
- Operator evidence is developer attestation for an `[operator]` task only; never require RED/GREEN for that task, and flag CRITICAL when operator evidence completes a task not marked `[operator]`.
- If RED evidence is hypothetical or lacks executed failing-command output, flag CRITICAL.
- If a covering test fails in the verify suite run, flag CRITICAL.
- If triangulation is skipped without a structural one-output rationale, flag WARNING.
- Optional audits are warn-only, except that a found test that cannot fail (tautology, no production execution, ghost loop, or setup that skips the target path) is CRITICAL; audits can never compensate for missing TDD evidence.
- DO NOT fix issues — only report. The orchestrator decides.
