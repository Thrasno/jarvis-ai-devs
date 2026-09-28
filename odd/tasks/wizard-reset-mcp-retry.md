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
- [x] ODD-RESET-01 — Repair managed MCP ordering for a consented reset, with a regression test that reads the final OpenCode JSON and verifies the Hive/hybrid runtime contract. Route: delegated writer (multi-file implementation). Checks: focused Go tests and preserved unrelated MCPs; record RED/GREEN evidence. Commit: pending.
- [x] ODD-RESET-01B — Complete the real-agent hybrid verification fixture: no unrelated missing-manifest/instruction errors may be accepted as success. Route: same delegated writer follow-up. Checks: focused Go tests and full successful runtime contract.
- [ ] ODD-RESET-02 — Make Apply failure recovery visible and operable (retry plus exit/return) with direct Bubbletea Update tests; do not change successful Apply navigation. Route: delegated writer (multi-file implementation). Checks: focused Go tests, full `go test ./...` and `go vet ./...`; record RED/GREEN evidence. Commit: pending.

## Progress
- Status: ODD-RESET-01 and ODD-RESET-01B completed. RED reproduced absent Hive; GREEN and strengthened fixture proved full hybrid verification success, a present manifest, preserved user MCP, and reset rollback when reconciliation fails.
- Check: `cd jarvis-cli && go test ./internal/tui -run 'Test.*(Reset|MCP|OpenCode).*' -count=1` passed; diff --check passed. Full suite and vet pending ODD-RESET-02.
- Running authored line count: 0.
- Native review boundary: branch point `dc9df688` (review only at work-unit commits if enabled).
- Next: implement ODD-RESET-02 with TUI error-state tests, then full suite and vet.
