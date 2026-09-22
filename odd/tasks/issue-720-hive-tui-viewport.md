# Issue 720 — Hive TUI Vertical Viewport

## Objective
Make every Hive TUI screen usable within the current terminal height while keeping selectable rows visible and preserving existing navigation, input, and guarded-operation semantics.

## Problem
The TUI records terminal width but discards height. Views render complete unbounded documents, so content and logical selections can move outside the visible terminal area.

## Why
Projects, memories, timeline, warnings, backups, API health, purge, batch merge, and normalization can become partially inaccessible in normal terminals. Resize and multiline output have no shared vertical contract.

## Scope
- Fixed header and help/status footer when the terminal can accommodate them.
- Vertically bounded central content viewport.
- Automatic cursor-follow for selectable collections.
- Line, page, and boundary navigation under the approved key contract.
- Deterministic overflow/range feedback.
- Safe `height == 0`, tiny-terminal, resize, and offset-clamping behavior.
- P0 exhaustive coverage and P1 shared bounded-rendering coverage from the approved issue comment.

## Constraints
- Preserve text-input ownership, confirmations, shortcuts, empty states, back navigation, and guarded semantics.
- Mouse-wheel support is out of scope.
- Prefer a small project-owned helper unless a focused spike proves `bubbles/viewport` integrates cleanly.
- Do not add a dependency without concrete value.
- Technical artifacts remain in English.

## Delivery
- Strategy: Feature Branch Chain, selected by the user.
- Tracker branch: `fix/issue-720-viewport-tracker`.
- Forecast: 1,000–1,600 authored changed lines across three reviewable work units.
- Each child branch targets its immediate parent; master receives the complete behavior through the tracker.

## TDD and verification
- TDD mode: ordinary ODD verification; strict TDD is scoped to SDD by `AGENTS.md`.
- Test pattern: direct `Model.Update()` state-transition tests and table-driven layout/rendering cases.
- Runner: focused `go test` in `jarvis-cli`, then `go test ./...` and `go vet ./...` as applicable.

## Tasks

### 720-01 — Build the shared vertical foundation
- [x] Add behavior tests for height capture, available-area calculation, bounded offsets, resize clamping, tiny terminals, and overflow indicators.
- [x] Store terminal height and implement the project-owned vertical range/viewport foundation.
- [x] Preserve pre-resize rendering and prevent negative dimensions or panics.
- [x] Run focused tests and applicable module verification.
- [ ] Record the work-unit commit and native assessment/review outcome.

Route: delegated writer. Trigger: multi-file write across Hive TUI/layout implementation and tests.

### 720-02 — Migrate primary P0 views
- [ ] Add behavior tests for long Projects, project memories, memory detail, Timeline, Warnings, Backups, and API health views.
- [ ] Keep selected rows visible under line, wraparound, page, and boundary navigation.
- [ ] Render deterministic visible-range and overflow feedback.
- [ ] Preserve empty states, shortcuts, and back navigation.
- [ ] Run focused tests and applicable module verification.
- [ ] Record the work-unit commit and native assessment/review outcome.

Route: delegated writer. Trigger: multi-file write and broad P0 view migration.

### 720-03 — Cover complex workflows and P1 screens
- [ ] Add regressions for purge, batch merge, normalization, forms, confirmations, and multiline result/error states.
- [ ] Apply the shared height contract to remaining P0/P1 screens.
- [ ] Prove text input and guarded operations retain key ownership and semantics.
- [ ] Run focused tests, complete module tests, and vet.
- [ ] Record the work-unit commit and native assessment/review outcome.

Route: delegated writer. Trigger: multi-file write across complex workflows and regression suites.

## Acceptance criteria
- All Hive TUI screens remain usable inside the known terminal height.
- Header/footer remain fixed whenever possible and central content is bounded.
- Selectable rows remain visible after line, wraparound, page, and boundary navigation.
- Multiline non-selectable content is fully reachable.
- Resize preserves logical state and clamps offsets.
- Tiny terminals produce a safe deterministic frame or explicit `Terminal too small` state.
- Overflow is visible through range and `more` indicators.
- Existing inputs, confirmations, guarded operations, shortcuts, empty states, and back navigation remain unchanged.

## Progress
- Issue approved on GitHub with normative implementation clarifications.
- Implementation and parallel isolated worktrees authorized by the user.
- Feature Branch Chain selected.
- Tracking document created before source writes.
- Work unit 720-01 implemented on `fix/issue-720-01-vertical-foundation` with a project-owned viewport/range helper and no new dependency.
- Independent verification found and then confirmed correction of the unknown-height versus known exhausted-height ambiguity.

## Verification evidence
- Writer: `cd jarvis-cli && go test ./internal/hiveui ./internal/terminalui` — PASS.
- Writer: `cd jarvis-cli && go test ./...` — PASS.
- Writer: `cd jarvis-cli && go vet ./...` — PASS.
- Writer: `git diff --check` — PASS.
- Independent verifier after correction: all four commands PASS; prior medium finding resolved.
- Parent spot check: `cd jarvis-cli && go test ./internal/hiveui ./internal/terminalui` — PASS (cached).
- Native assessment: unavailable/empty output; treated as high risk and independently verified.

## Next step
Create the 720-01 work-unit commit, record its hash, and branch 720-02 from it.
