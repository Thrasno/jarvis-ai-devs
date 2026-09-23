# Issue 761: Windows OpenCode lifecycle test flake

Objective: Make the source-template lifecycle test diagnose early Node failure and reliably distinguish it from a missing HTTP request or slow startup, without weakening lifecycle assertions.

Problem/evidence: Windows CI runs 35910821790 and 35910871843 timed out after 10 seconds waiting for an initial session start; run 35907345176 failed on attempt 1 and passed on attempt 2 at the same SHA. The root cause is unproven. Issue #761 was approved with `status:approved` on 2026-09-23.

Scope: `jarvis-cli/internal/agent/opencode_lifecycle_test.go` and, only if evidence requires it, `.github/workflows/ci.yml`; preserve the source template and product behavior unless a proven defect requires a separate decision. No generated user configuration, build, push, PR, or release.

Acceptance: early child exit reports its status/stderr promptly; missing requests retain a bounded deadline and useful diagnostics; coalescing, distinct evidence, prompt independence and retry ordering assertions remain; focused Go tests and Go module suite pass. Windows CI is the final platform confirmation after publication, not locally claimable.

Testing: strict TDD enabled from `openspec/config.yaml` (`testing.strict_tdd: true`); runner `cd jarvis-cli && go test ./internal/agent -run 'TestOpenCodeHiveTemplate_' -count=1`, then `cd jarvis-cli && go test ./...`; `cd jarvis-cli && go vet ./...` as static check. RED must precede production test-harness change. Node-dependent cases skip when Node is unavailable.

Delivery: ask-on-risk; forecast 60–120 authored changed lines, with possible 150–250 if the investigation finds deeper timing problems. Actual implementation: 176 authored changed lines (155 additions, 21 deletions). Reviewed boundary: `12b933c0` from `d4936b54`. Work-unit commit is Conventional Commit; no push or PR. Native high-risk review `review-0623b68d8a9f0c5d` approved and acknowledged; two advisory non-blocking warnings at the helper are separate later work.

## Tasks

- [x] T1 (delegated writer): Add early child exit diagnosis, bounded child-aware waits, fixture synchronization and tests; preserve product assertions. RED/GREEN observed, focused test/race, CLI suite, vet and independent review passed. Code commit: `12b933c0`.
- [ ] T2 (blocked, external): Verify the committed change in Windows CI after publication; if initial timeout persists, use captured exit/stderr diagnostics to identify and address the actual cause. No publication requested in this task.

## Evidence and next step

Approval: GitHub issue #761 now carries `status:approved` instead of `status:needs-review`. Baseline worktree clean on branch `fix/issue-761-windows-opencode-lifecycle-flake`. First writer pass: early-exit diagnostic RED was a compile failure; independent verification exposed nil `cmd.Process` and unbounded cleanup. Writer observed behavioral RED (nil-process panic), then used `CommandContext` and bounded cleanup. Follow-up found unbounded final waits and fixture timing; writer observed RED for a completed child, added bounded final waits and fixture startup handshake. A final read-only review identified an unnecessary 3-second final wait; changed it mechanically to the existing 10-second request deadline. Final independent checks all passed: `cd jarvis-cli && go test ./internal/agent -run 'TestOpenCodeHiveTemplate_' -count=1`, focused `-race`, `cd jarvis-cli && go test ./...`, `cd jarvis-cli && go vet ./...`, and `git diff --check`. Parent spot check passed before the last two-line correction. No actionable medium/high findings remain in the independent check. Committed `12b933c0` and native high-risk review approved/acknowledged. Windows CI remains unconfirmed and the original flake cause is still unproven; T2 is blocked on publication.
