<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/sdd-verify/strict-tdd-verify.md -->
<!-- Upstream commit: 5f73974b39ae2b9b525ef465b3642030c5f2ce6c; adapted for Jarvis/Hive runtime wording. -->
# Strict TDD Module — Verify Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a test runner is available.**
> If you are reading this, the orchestrator already verified both conditions. Follow every instruction.

## TDD Verification Philosophy

When Strict TDD Mode is active, verification goes beyond "does the code work?" to "was the code built correctly?" — meaning: was TDD actually followed? The apply phase reports TDD evidence; your job is to validate that evidence against reality.

## Step 5a: TDD Compliance Check (includes Assertion Quality Audit)

Read the `apply-progress` artifact and verify that TDD was actually followed:

```
Read apply-progress artifact:
├── Find the "TDD Cycle Evidence" table
├── FOR EACH task row:
│   ├── RED column: must say "✅ Written" and the test file must exist
│   ├── GREEN column: must say "✅ Passed" and the referenced tests must pass now
│   ├── TRIANGULATE column: verify adequate cases or a valid single-scenario reason
│   ├── SAFETY NET column: modified files need a safety-net run before edits
│   └── REFACTOR column: subjective quality; skip strict verification
├── If NO "TDD Cycle Evidence" table found:
│   └── Flag: CRITICAL — Strict TDD was enabled but apply did not report evidence
└── Summary: "{N}/{total} tasks have complete TDD evidence"
```

## Step 5 Expanded: Test Layer Validation

Classify all test files related to this change as unit, integration, E2E, or unknown. Cross-reference the distribution with cached testing capabilities and note which layer covers each spec scenario.

## Step 5d Expanded: Changed File Coverage

When coverage tooling is available, run the configured coverage command, filter the report to files created or modified in this change, and report line coverage, branch coverage when available, uncovered ranges, and an aggregate changed-file average. If coverage is unavailable, report that cleanly; missing coverage tooling is not a failure.

## Step 5e: Quality Metrics (if tools available)

Run linter/type-checker only when detected in testing capabilities. Report changed-file findings as warnings or suggestions; do not fail verification only because optional tooling is absent.

## Report Template Extension

When Strict TDD Mode is active, include TDD compliance, test layer distribution, changed-file coverage, assertion quality, and quality metrics sections in the verification report.

## Step 5f: Assertion Quality Audit (MANDATORY)

Scan all test files created or modified by this change for trivial assertions, empty checks without non-empty companions, type-only assertions without value assertions, assertions that never call production code, ghost loops, smoke-test-only checks, implementation-detail coupling, and mock-heavy tests.

If zero issues are found, report: **Assertion quality**: ✅ All assertions verify real behavior.

## Rules (Strict TDD Verify specific)

- ALWAYS check the TDD Cycle Evidence table from apply-progress — it is the primary artifact.
- ALWAYS cross-reference reported test files against actual execution — do not trust the report blindly.
- ALWAYS run the Assertion Quality Audit — trivial tests are worse than missing tests.
- If apply-progress has no TDD evidence table, flag CRITICAL.
- Coverage and quality metrics are informational, not blocking.
- DO NOT fix issues — only report. The orchestrator decides.
