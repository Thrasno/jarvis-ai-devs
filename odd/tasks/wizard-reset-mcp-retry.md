# Wizard reset MCP and retry recovery

## Objective
Make a consented configuration reset regenerate managed OpenCode MCPs before runtime verification, and let users leave the Apply failure screen without repeating a persistent failure.

## Problem and evidence
The merged reset wizard reconciles managed MCPs in `internal/tui/steps.go` before `configureWizardAgents` applies the reset. `ApplyReset` removes `/mcp/hive` and `/mcp/context7`; generated config intentionally does not install MCPs. Hive/hybrid verification then fails, and runtime-verification failure does not roll the reset back. The failure screen offers only Enter to retry although Ctrl-C quits globally.

## Scope and constraints
- Work on `fix/wizard-reset-mcp-retry` from `public/master` in a clean isolated worktree; leave existing documentation worktree untouched.
- Preserve the explicit reset consent, backup/rollback semantics, user-owned OpenCode MCP entries and other config keys.
- Add a supported visible non-retry exit/return on the Apply failure screen. No shell recovery of user configuration and no builds.
- Technical artifacts in English; conventional commits without AI attribution.
- Effective TDD: on for this fix (chosen implementation discipline from project Go testing and regression-first plan); source: task plan, with `go test` focused commands; full `go test ./...` and `go vet ./...` at closure.
- Delivery: ask-on-risk; forecast approximately 250 authored changed lines, reviewable as two work-unit commits.

## Tasks
- [x] ODD-RESET-01 — Repair managed MCP ordering for a consented reset, with a regression test that reads the final OpenCode JSON and verifies the Hive/hybrid runtime contract. Route: delegated writer (multi-file implementation). Checks: focused Go tests and preserved unrelated MCPs; RED/GREEN observed. Commit: `4d9cf312`.
- [x] ODD-RESET-01B — Complete the real-agent hybrid verification fixture: no unrelated missing-manifest/instruction errors may be accepted as success. Route: same delegated writer follow-up. Checks: focused Go tests and full successful runtime contract.
- [x] ODD-RESET-02 — Failed Apply visibly offers `q` to quit as well as Enter to retry; Bubbletea Update tests cover no false success, retry, and Ctrl-C. Route: delegated writer (multi-file implementation). RED/GREEN observed; focused tests, full `go test ./...`, and `go vet ./...` passed. Commit: this work unit.

## Progress
- Status: all tasks completed. RED reproduced absent Hive and invisible Apply escape; GREEN proved full hybrid runtime success, preserved user MCP, rollback on MCP failure, and a visible non-retry quit action.
- Check: focused reset/MCP/OpenCode tests and Apply escape test passed. Full Go suite and vet passed under writer and independent verifier; branch diff --check clean.
- Running authored line count: 193 across two work-unit commits against `public/master` (includes task document).
- Native review boundary: branch point `dc9df688`; committed assessment attempted after first work unit but unavailable (`schema-incompatible`), treated as unassessable/high and requiring independent verification. No review receipt claimed.
- Independent verifier passed `go test ./internal/tui -count=1`, `go test ./...`, and `go vet ./...`; `git diff --check public/master` clean. Parent spot-check passed both critical tests. No interactive end-to-end or live MCP server test; no native review receipt.
- Next: inspect native review authority without claiming a receipt, then deliver the branch for review/release planning; no release or deployment performed.
