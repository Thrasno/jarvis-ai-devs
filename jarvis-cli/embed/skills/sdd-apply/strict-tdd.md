# Strict TDD Module — Apply Phase

> **This module is loaded ONLY when Strict TDD Mode is enabled AND a test runner is available.**
> If you are reading this, the orchestrator already verified both conditions. Follow every instruction.
> If the assigned files cannot run under any test runner, the `strict-tdd-unrunnable` gate in `SKILL.md` applies: stop before the first task. There is no silent fallback to Standard Mode.
> Strict TDD never applies to `[operator]` tasks: the developer executes them, so they get no RED/GREEN cycle, no safety net, and no `strict-tdd-unrunnable` block. Follow "Operator Handoff Tasks" in `SKILL.md` for them.

## TDD Philosophy

TDD is not testing. TDD is **software design driven by tests**. You write a test that describes what the code SHOULD do, then write the minimum code to make it real. The tests design the API, the contracts, the behavior. Code is a side effect of tests.

### The Three Laws

1. **Do NOT write production code** until you have a failing test
2. **Do NOT write more test** than is necessary to fail
3. **Do NOT write more code** than is necessary to pass the test

## TDD Implementation Cycle

```text
BATCH START:
└── 0. SAFETY NET — once per affected package at batch start, not per task
    ├── Run the existing tests of every package the batch will modify
    ├── Capture the baseline: "{command} → {N} tests passing"
    └── If any FAIL → STOP, report as "pre-existing failure"
        (do NOT fix pre-existing failures — report to orchestrator)

FOR EACH TASK:
├── 1. UNDERSTAND
│   ├── Read the task, its spec scenarios (acceptance criteria), and design decisions
│   ├── Read existing code and test patterns (match the style)
│   └── Determine test layer (see "Choosing Test Layer" below)
│
├── 2. RED — Write a failing test FIRST
│   ├── The test describes the expected behavior from the spec
│   ├── It references production behavior that does NOT exist yet
│   │   (a compile failure still requires executing the focused test command)
│   ├── EXECUTE the focused test command and capture the failing output
│   │   ├── ✅ Failed for the expected reason → proceed to GREEN
│   │   ├── ❌ Passed → STOP and strengthen the test before implementing
│   │   └── ⚠️ Infrastructure blocked execution → STOP and report the blocker; do NOT proceed
│   └── GATE: Do NOT proceed to GREEN until RED is confirmed by execution
│
├── 3. GREEN — Write the MINIMUM code to pass
│   ├── Implement ONLY what the failing test needs; Fake It is valid here
│   ├── EXECUTE tests → must PASS; on failure fix the implementation, NOT the test
│   └── GATE: Do NOT proceed until GREEN is confirmed by execution
│
├── 4. TRIANGULATE (MANDATORY for most tasks)
│   ├── DEFAULT: triangulation is REQUIRED. You need a compelling reason to skip it.
│   ├── Add a case with DIFFERENT inputs/expected outputs until every spec scenario
│   │   of the task is covered; generalize any Fake It the new case breaks
│   ├── MINIMUM: at least 2 test cases per behavior (happy path + one edge case)
│   │   ├── One test with data that produces a NON-EMPTY/NON-TRIVIAL result
│   │   └── One test with data that exercises a DIFFERENT code path
│   ├── The new case may run inside GREEN's command; record it as the `triangulate` step
│   ├── A real GREEN means production code RAN and produced the expected output;
│   │   a pass because nothing rendered or a loop iterates 0 times is NOT GREEN
│   ├── Skip triangulation ONLY when ALL of these are true:
│   │   ├── The task is purely structural (config file, constant definition, type export)
│   │   ├── There is literally ONE possible output (no branching, no logic)
│   │   └── Record `triangulate: {skip_reason: "Triangulation skipped: {reason}"}`
│   ├── A single spec scenario is NOT a triangulation skip reason; only structural one-output work may skip triangulation
│   └── GATE: All spec scenarios for this task must have tests before REFACTOR
│
├── 5. REFACTOR — Improve without changing behavior
│   ├── Extract constants and functions, improve naming, remove duplication
│   ├── Push toward pure functions; leave code cleaner than you found it
│   ├── Make one run after refactoring, not after every micro-step → must STILL PASS
│   │   └── ❌ Failed → revert the refactoring and retry in smaller steps
│   └── Skip the step when nothing was refactored
│
└── 6. RECORD — ONE `task_records` item per task for the batch checkpoint
    ├── Only steps that actually ran, each with its real command, exit code, and summary
    └── Note any deviations or issues discovered
```

Task checkboxes change only through the batch checkpoint in `SKILL.md` Steps 5–6; this module never marks `[x]`.

## Choosing Test Layer

Use the HIGHEST layer the cached testing capabilities (`sdd/{project}/testing-capabilities`) support for what the task does:

- Pure logic, utility, calculation, data transformation → unit test.
- Component rendering, interaction, state changes, multi-component or API flows → integration test when available, otherwise unit test with a small fake.
- Critical business flow or full user journey → E2E when available, otherwise integration, otherwise unit.

NEVER skip a task because a layer is unavailable — degrade to the next available layer.

## Test Execution

Read the test command from cached capabilities (`test_runner.command`), then `openspec/config.yaml` `rules.apply.test_command`, then the project manifests. Run ONLY the relevant test file or package during the cycle (for example `go test ./{package}/... -run {TestName}` or `pnpm vitest run {test-file}`); the full suite runs in sdd-verify.

## Pure Function Preference

Prefer pure functions in GREEN and TRIANGULATE: same input, same output, no side effects, trivially testable. Do not force them where they do not fit, such as stateful UI components.

## Approval Testing (for refactoring existing code)

BEFORE touching production code in a refactoring task, write approval tests that call the code with known inputs and assert its CURRENT outputs, run them (they must pass), refactor, and run them again (they must still pass). When the spec changes behavior, update the approval test first so it fails (RED), then implement (GREEN).

## Failure Evidence Requirements

Strict TDD requires proof that RED happened before GREEN. RED evidence MUST include the executed focused failing test command and the failing assertion, compile error, or behavior mismatch output.

RED evidence must include one of these executed failure forms, captured from the executed focused failing test command:

1. A failing focused test command and the failing assertion or error output.
2. A Go compile-time failure from a missing symbol introduced by the new test.
3. A failing focused test command that reaches existing production code and proves the new behavior is not implemented yet.

If infrastructure blocks the focused RED command, STOP and report the blocker instead of implementing. Do not proceed to GREEN with hypothetical RED evidence.

Do not document RED as "would fail"; it is not RED until the focused command was executed and failed. If the focused RED command cannot execute, STOP and report the infrastructure blocker; do NOT implement or move to another task.

## Evidence Record

Persist TDD proof as `task_records` in the batch checkpoint; the CLI expands each record into structured v2 entries, one `EvidenceEntry` per step with `completes_task_ids` on the last step that ran, and the immutable v2 entries are authoritative. Do not use a Markdown cycle table as apply-progress evidence. When reading prior Hive proof, call `sdd_apply_progress_get`, then `sdd_apply_evidence_get` for each referenced `batch_id`.

```json
{
  "task_id": "1.1",
  "files": ["internal/skills/example.go", "internal/skills/example_test.go"],
  "red": {"command": "go test ./internal/skills -run TestExample", "exit_code": 1, "summary": "Safety net: go test ./internal/skills → 42 passing. RED: undefined: Example"},
  "green": {"command": "go test ./internal/skills -run TestExample", "exit_code": 0, "summary": "Happy path passes"},
  "triangulate": {"command": "go test ./internal/skills -run TestExample", "exit_code": 0, "summary": "Empty-input case drove real logic"},
  "refactor": {"command": "go test ./internal/skills", "exit_code": 0, "summary": "Extracted helper; package still green"}
}
```

Record each package's safety-net baseline once, in the `red` summary of the first task that touches that package, and in the return summary. A structural skip records a `triangulate` step with only `skip_reason`, which expands to `kind: "triangulate"` with `outcome: not_run`; one scenario is not a skip reason. The record completes its task only when its last executed step passed.

## Assertion Quality Rules (MANDATORY)

**Every assertion must verify REAL behavior.** A test that passes without exercising production logic is worse than no test because it gives false confidence. A REAL assertion calls production code, asserts a specific output or observable effect derived from the spec, and would FAIL if the production code were wrong.

### Banned Assertion Patterns (NEVER write these)

- **Tautologies**: `expect(true).toBe(true)`, `assert 1 == 1`, or any check that holds without production code.
- **Unexplained empty results**: `toEqual([])`, `toHaveLength(0)`, `len(result) == 0` are valid only when the setup should produce empty, production code ran to produce it, and a companion test reaches a NON-EMPTY result through the same path.
- **Type-only checks**: `toBeDefined()`, `not.toBeNull()`, `result != nil` alone; assert the actual value.
- **Ghost loops**: assertions inside a loop that iterates 0 times never run; assert the collection is non-empty FIRST, or set up data so it is.
- **Trivial GREEN**: a pass because the code path never ran (component not rendered, setup does not trigger it) is not GREEN; triangulate with a setup that exercises it.
- **Smoke Test Rule**: "Renders without crash" or initializing a command without asserting output is a smoke test; it does NOT count toward TDD coverage.
- **Mock/Fake Hygiene Rules**: If you need more mocks than assertions, you are testing at the wrong level. 4–6 mocks means consider extracting logic; 7+ means STOP and extract or move up a layer.
- **Extract-Before-Mock Rule**: extract mapping, filtering, or conditional logic to a pure function and test it directly with zero mocks. Fakes are valid only to make a real boundary (filesystem, clock, HTTP, command execution) deterministic, never to duplicate the logic under test.
- **Behavior-First Test Rule**: Tests must assert **behavior visible to the user or caller**, not internals: no CSS class or inline-style assertions, no mock call counts, no internal state. Verify visual styling with semantic outcomes or visual regression tools.

## Rules (Strict TDD specific)

- NEVER write production code before writing its test — this is the ONE rule that cannot be broken
- NEVER skip the RED or GREEN execution gate — you MUST run the tests and observe the result
- NEVER skip triangulation when the spec defines multiple scenarios — hardcoded Fake It must be forced out
- NEVER write trivial assertions (see Banned Assertion Patterns above) — they are WORSE than no test
- ALWAYS run the Safety Net once per affected package at batch start, before modifying existing files
- ALWAYS record exactly ONE `task_records` item per task with only the steps that ran — the verify phase checks them
- If a test runner execution fails for infrastructure reasons (not test failures), STOP and report the blocker; do not continue with implementation
- For refactoring tasks, ALWAYS write approval tests before touching code
- Run ONLY the relevant test file or package during the cycle, not the full suite
