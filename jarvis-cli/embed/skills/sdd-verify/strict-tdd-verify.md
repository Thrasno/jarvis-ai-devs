<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-verify/strict-tdd-verify.md -->
<!-- Upstream commit: 660917927b4821f5e540dc8fa501d6bee723222c. -->
<!-- Provenance: Upstream-derived from Gentle AI v1.40.2 and adapted for Jarvis/Hive runtime wording plus Jarvis-specific verification policy additions beyond runtime wording. -->
<!-- Maintenance guard: Future parity runs MUST NOT overwrite Jarvis-specific verification policy without maintainer approval. -->
# Strict TDD Module — Verify Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a test runner is available.**
> If you are reading this, the orchestrator already verified both conditions. Follow every instruction.

## TDD Verification Philosophy

When Strict TDD Mode is active, verification goes beyond "does the code work?" to "was the code built correctly?" The apply phase reports TDD evidence; your job is to validate that evidence against the repository, the changed tests, and current command output.

Strict TDD verification has two responsibilities:

1. Confirm the RED → GREEN → REFACTOR cycle was followed with executed evidence.
2. Confirm the resulting tests protect behavior instead of creating a weak TDD pass.

## Step 5a: TDD Compliance Check

Read the `apply-progress` artifact and verify that TDD was actually followed:

```text
Read apply-progress artifact:
├── Find the "TDD Cycle Evidence" table
├── FOR EACH task row:
│   ├── RED column:
│   │   ├── Must say the test was written before implementation
│   │   ├── Test file must exist in the codebase
│   │   ├── RED evidence must include an executed focused failing command
│   │   ├── Evidence must include the failing assertion, compile error, or behavior mismatch output
│   │   └── Flag CRITICAL when RED is hypothetical, missing, or not tied to a real command
│   │
│   ├── GREEN column:
│   │   ├── Must say the focused test passed after minimal implementation
│   │   ├── GREEN evidence must be re-run during verify
│   │   ├── The referenced test command must pass now
│   │   └── Flag CRITICAL if the test fails now or no executable command is recorded
│   │
│   ├── TRIANGULATE column:
│   │   ├── If "✅ N cases" → verify N meaningful cases exist in the test file
│   │   ├── A single spec scenario is not enough by itself to pass triangulation
│   │   ├── Accept a skipped triangulation rationale only for structural one-output work
│   │   └── Flag WARNING when behavior has too few varied test cases
│   │
│   ├── SAFETY NET column:
│   │   ├── If "✅ N/N" → existing tests were run before modification
│   │   ├── If "N/A (new)" → verify the referenced files were actually new
│   │   └── Flag WARNING if a modified file is reported as new or has no baseline run
│   │
│   └── REFACTOR column:
│       ├── REFACTOR evidence must include a post-refactor passing command or an explicit no-refactor rationale
│       └── Flag WARNING if refactor evidence is claimed but not supported by command output
│
├── If NO "TDD Cycle Evidence" table found:
│   └── Flag CRITICAL — Strict TDD was enabled but apply did not report evidence
│
└── Summary: "{N}/{total} tasks have complete TDD evidence"
```

## Step 5b: Test Execution Cross-Check

Run the focused test commands referenced in `apply-progress`, then run the configured test runner for the verification scope.

Go-friendly examples:

```bash
# Focused package/test cross-check
go test ./internal/skills -run TestName

# Full configured test runner when required by the phase
go test ./...
```

If a referenced command cannot be run because tooling or infrastructure is unavailable, report the exact blocker under skipped dimensions. Missing execution evidence cannot be upgraded to PASS.

## Step 5c: Test Layer Validation

### Test Layer Distribution

Classify all test files related to this change as unit, integration, E2E, or unknown. Cross-reference the distribution with cached testing capabilities.

```text
Scan test files created/modified by this change:
├── Classify each test file:
│   ├── Unit test: tests a single function, parser, command helper, or package boundary
│   ├── Integration test: tests component/package interaction, filesystem behavior, CLI flows, HTTP, or TUI state transitions
│   ├── E2E test: tests a full system path through real process/browser/service boundaries
│   └── Unknown: cannot classify → report as-is
│
├── Report distribution:
│   ├── Unit: {N} tests across {N} files
│   ├── Integration: {N} tests across {N} files
│   ├── E2E: {N} tests across {N} files
│   └── Total: {N} tests
│
├── Cross-reference with capabilities:
│   ├── If integration tests exist but tools were not detected, explain the tool source
│   ├── If E2E tests exist but tools were not detected, explain the tool source
│   └── Flag WARNING if tests depend on unavailable tooling
│
└── For each spec scenario, note which test layer covers it
    └── Flag SUGGESTION when critical behavior only has unit coverage and higher layers are available
```

Layer distribution does not excuse missing behavior coverage. A large number of low-level tests is not a substitute for scenario coverage.

### Coverage Allocation Audit

After classifying test layers, verify that coverage is allocated to the cheapest deterministic layer that can prove each behavior:

```text
FOR EACH changed behavior or spec scenario:
├── Identify the behavior type:
│   ├── Pure logic, parsing, mapping, validation, command construction, artifact rendering
│   │   └── Should have deterministic unit coverage
│   ├── Package/component wiring, filesystem effects, CLI command behavior, API boundaries
│   │   └── Should have deterministic integration coverage
│   └── Full user journey across real process/browser/service boundaries
│       └── May need E2E coverage, but only for the full journey risk
├── Cross-check actual coverage allocation:
│   ├── Flag WARNING when behavior is covered only by E2E or broad integration tests but deterministic unit or lower-layer integration tests should cover it
│   ├── Flag WARNING when coverage is E2E-heavy and lower-layer deterministic tests are missing for isolated behavior
│   └── Flag WARNING when tests are over-integrated: expensive, broad, flaky, or dependent on unrelated wiring for behavior that has a smaller deterministic boundary
└── Report the missing cheaper layer and the behavior it should cover
```

Do not accept expensive E2E coverage as a substitute for cheaper deterministic coverage of pure logic, parsing, mapping, validation, command construction, or artifact rendering.

This audit does not ban E2E tests. It flags over-integrated or E2E-heavy coverage when deterministic lower-layer tests should cover the behavior first, with E2E reserved for true end-to-end journey risk.

## Step 5d: Changed File Coverage

When coverage tooling is available, report coverage for changed files specifically:

```text
IF coverage tool is available from cached capabilities or project convention:
├── Run the configured coverage command
├── Parse the coverage report
├── Filter to ONLY files created or modified in this change
│   (get file list from apply-progress "Files Changed" table and current git diff)
├── Report per file:
│   ├── File path
│   ├── Line coverage %
│   ├── Branch coverage % when available
│   ├── Uncovered line ranges
│   └── Rating: ✅ Excellent (≥95%), ⚠️ Acceptable (≥80%), or ⚠️ Low (<80%)
├── Report aggregate changed-file average
└── Flag WARNING for changed files below the configured threshold, or below 80% when no threshold exists

IF coverage tool is NOT available:
└── Report: "Coverage analysis skipped — no coverage tool detected"
```

Go coverage example: `go test ./... -coverprofile=/tmp/opencode/jarvis-sdd-verify.coverprofile`

Coverage is supporting evidence. It must not hide missing RED/GREEN/REFACTOR evidence, missing behavior assertions, or untested spec scenarios.

## Step 5e: Quality Metrics

Run quality checks only when tools are available:

```text
IF linter is available:
├── Run linter on changed files when supported, otherwise whole project
├── Report errors and warnings that affect changed files
└── Flag WARNING for errors, SUGGESTION for warnings

IF type checker or static analyzer is available:
├── Run the configured command
├── Filter output to changed files when possible
└── Flag WARNING for findings in changed files

IF no quality tools are available:
└── Report: "Quality metrics skipped — no tools detected"
```

For Go projects, `go vet ./...` is the default static check when the phase requires static verification.

## Step 5f: Assertion Quality Audit (MANDATORY)

Scan all test files created or modified by this change and check whether assertions prove real behavior.

```text
FOR EACH test file related to the change:
├── Read the file content
├── Scan for banned or weak assertion patterns:
│   ├── Tautologies: expect(true).toBe(true), assert True, assert 1 == 1, if got != got
│   ├── Orphan empty checks: empty result assertions without a companion non-empty case
│   ├── Type-only assertions used alone: defined/non-nil/type checks with no concrete value assertion
│   ├── Assertions with no production-code execution: no function call, render, request, command, or package boundary
│   ├── Ghost loops: assertions inside loops over possibly empty query/filter results
│   ├── Incomplete TDD cycle: setup prevents the code path under test from running
│   ├── Smoke-test-only: render/startup/existence check without asserting produced behavior
│   ├── Implementation-detail coupling: CSS classes, internal state, incidental mock call counts
│   └── Mock/assertion ratio: mocks greatly outnumber behavior assertions
│
├── For each violation found:
│   ├── Record file, line number, assertion, issue, and severity
│   └── Severity guide:
│       ├── CRITICAL: tautology or assertion without production execution
│       ├── CRITICAL: ghost loop whose body can execute zero times
│       ├── CRITICAL: test setup prevents the target behavior from running
│       ├── WARNING: empty result without companion non-empty case
│       ├── WARNING: type-only check used as the only proof
│       ├── WARNING: smoke-test-only
│       ├── WARNING: implementation-detail assertion
│       └── WARNING: mock-heavy test at the wrong layer
│
├── Check triangulation quality:
│   ├── Count distinct test cases per behavior
│   ├── Flag WARNING when behavior with multiple paths has only one meaningful case
│   ├── Flag WARNING when all cases assert the same trivial shape, such as only empty collections
│   └── Confirm a well-triangulated behavior asserts different expected values or code paths
│
└── Summary: "{N} weak assertions found across {N} files"
```

### Behavior coverage vs implementation-only tests

Tests should prove behavior visible to the user, caller, CLI operator, API consumer, or persisted artifact contract. Do not count tests that only prove an internal helper was called, a CSS class exists, a mock was invoked a certain number of times without behavioral meaning, or a struct field was initialized without exercising the behavior it supports.

For Go contract tests, prefer explicit `got`/`want` checks against source content, rendered output, command behavior, or durable side effects. A check such as `if got != want` is useful only when `got` comes from production code or a real artifact under test.

### Mock/Fake Hygiene

Mocks and fakes are valid when they isolate a boundary, but they must not replace the behavior being verified.

```text
Mock/fake hygiene checks:
├── Count mocks/fakes/stubs used by each test file
├── Count behavior assertions in the same file
├── Flag WARNING when mocks are more than 2× behavior assertions
├── Flag WARNING when a fake returns the exact expected answer without exercising production logic
├── Recommend extracting pure transformation logic before adding many mocks
└── Recommend moving to integration/E2E when the behavior depends on real wiring
```

## Report Template Extension

When Strict TDD Mode is active, the verification report MUST include these additional sections:

```markdown
### TDD Compliance
| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ / ❌ | {Found in apply-progress / Missing} |
| RED confirmed | ✅ / ❌ | {N}/{total} tasks have executed failing-command evidence |
| GREEN confirmed | ✅ / ❌ | {N}/{total} referenced tests pass now |
| REFACTOR confirmed | ✅ / ⚠️ / ➖ | {post-refactor command, no-refactor rationale, or missing evidence} |
| Triangulation adequate | ✅ / ⚠️ / ➖ | {N} tasks triangulated / {N} structural skips |
| Safety Net for modified files | ✅ / ⚠️ | {N}/{total} modified files had baseline evidence |

**TDD Compliance**: {N}/{total} checks passed

### Test Layer Distribution
| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | {N} | {N} | {tool} |
| Integration | {N} | {N} | {tool or "not installed"} |
| E2E | {N} | {N} | {tool or "not installed"} |
| Unknown | {N} | {N} | {reason} |
| **Total** | **{N}** | **{N}** | |

### Changed File Coverage
| File | Line % | Branch % | Uncovered Lines | Rating |
|------|--------|----------|-----------------|--------|
| `path/to/file.ext` | 95% | 90% | — | ✅ Excellent |
| `path/to/other.ext` | 82% | N/A | L45-48, L62 | ⚠️ Acceptable |

**Average changed file coverage**: {N}%
{or "Coverage analysis skipped — no coverage tool detected"}

### Assertion Quality
| File | Line | Assertion | Issue | Severity |
|------|------|-----------|-------|----------|
| `path/test.ts` | 15 | `expect(true).toBe(true)` | Tautology — proves nothing | CRITICAL |
| `path/test.go` | 23 | `if len(got) != 0` | Empty result without companion non-empty case | WARNING |
| `path/test.go` | 31 | `if got == nil` | Type-only proof with no value check | WARNING |

**Assertion quality**: {N} CRITICAL, {N} WARNING
{or "✅ All assertions verify real behavior"}

### Quality Metrics
**Linter/static analysis**: ✅ No errors / ⚠️ {N} warnings / ❌ {N} errors / ➖ Not available
**Type checker/compiler**: ✅ No errors / ❌ {N} errors / ➖ Not available

### Skipped Dimensions and Uncertainty
| Dimension | Status | Reason | Impact |
|-----------|--------|--------|--------|
| Coverage | skipped | no coverage tool detected | Cannot assess changed-file coverage |
| E2E | skipped | E2E capability unavailable | Scenario covered at lower layer only |
```

## Skipped Dimensions and Uncertainty

Report every verification dimension that was skipped, unavailable, or uncertain. Missing optional tooling is not a failure, but skipped TDD evidence is still a finding.

Use this rule of thumb:

- Missing optional coverage, linter, type-check, integration, or E2E tooling → report as skipped with impact.
- Missing RED, GREEN, triangulation, safety-net, or assertion-quality evidence → report as WARNING or CRITICAL according to the rules above.
- Do not upgrade a skipped dimension to PASS. Say exactly what was not verified.

## Rules (Strict TDD Verify specific)

- ALWAYS check the TDD Cycle Evidence table from apply-progress — it is the primary artifact.
- ALWAYS cross-reference reported test files against actual execution — do not trust the report blindly.
- ALWAYS run the Assertion Quality Audit — trivial tests are worse than missing tests.
- If apply-progress has no TDD evidence table, flag CRITICAL.
- If tautology assertions are found, flag CRITICAL.
- If RED evidence is hypothetical or lacks executed failing-command output, flag CRITICAL.
- If GREEN evidence cannot be reproduced, flag CRITICAL.
- If triangulation is skipped without a structural one-output rationale, flag WARNING.
- Coverage and quality metrics can produce warnings or suggestions, but they cannot compensate for missing TDD evidence.
- Test layer distribution is reportable context; use it to explain behavior coverage risk, not to waive missing scenarios.
- If coverage or quality tools are not available, say so cleanly and move on.
- DO NOT fix issues — only report. The orchestrator decides.
