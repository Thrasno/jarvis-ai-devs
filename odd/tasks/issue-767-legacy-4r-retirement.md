# Issue #767 — Retire legacy 4R and consented configuration reset

## Objective and scope
Remove Jarvis-issued legacy 4R agents and fresh-review prompts from newly installed Claude/OpenCode configurations; report existing residue without deleting it. Add an explicitly consented, backed-up wizard reset that replaces only contract-declared configuration surfaces. Keep Council #647, Judgment Day, Hive memory, `jarvis sync`, `state.yaml`, and `config.yaml` out of scope. Source issue: https://github.com/Thrasno/jarvis-ai-devs/issues/767 (`status:approved`).

## Constraints and accepted design
- Worktree: `../jarvis-dev-issue-767`, branch `feat/issue-767-retire-legacy-4r`, starting at `public/master` (`5ddf342f`). Do not modify the original documentation worktree.
- Contract owns platform, JSON paths, historical command-token and permission-literal matchers; wizard display and reset derive from one inventory. A hook reset filters nested commands, preserving mixed groups, matcher, and unrelated fields. Preserve user settings semantically in JSON and unchanged file/block content bytewise.
- Snapshot every affected path before any write, including entire Claude `agents/`; rollback explicitly restores previous bytes and absent paths/directories on any failure. On restore failure, report snapshot ID and unrecovered paths. Residue diagnosis is informational, never auto-safe; refusing reset leaves residue validly.
- Generated assets and source are edited, never real-home generated agent configuration. No builds. Go tests and vet under `jarvis-cli/`.
- Effective ODD TDD: not established by session or repository for non-SDD implementation (AGENTS.md strict TDD applies to SDD); use tests-first where feasible and focused tests then `go test ./...` and `go vet ./...`. Reassess if a configured ODD TDD mode is found.
- Forecast: 900–1400 authored diff lines across two slices. Delivery strategy `ask-on-risk`; user selected `stacked-to-main` for the issue's 3–4 PR slices. PR creation is not authorized; record slice boundaries before opening PRs.

## Tasks (stable IDs)
- [ ] T1 Retire four embedded Claude 4R agents and OpenCode generation/allowlist; negative generation tests. Route: delegated writer (multi-file). Checks: focused `internal/agent` tests and source grep. Commit: pending.
- [ ] T2 Remove 4R review/audit directives from embedded orchestrator, retaining delegation and review-budget safeguards. Route: delegated writer or direct if one-file and understood. Checks: full prompt readback, scoped grep/content test. Commit: pending.
- [ ] T3 Stop requiring 4R in verifier; diagnose partial OpenCode/Claude residue informationally in doctor/reconcile without automatic cleanup. Route: delegated writer (multi-file). Checks: table tests incl partial residue. Commit: pending.
- [ ] T4 Update impacted fixtures and assertions, negative end-to-end rendered outputs, retirement/generated-artifact docs and parity when touched. Route: delegated writer (multi-file). Checks: `go test ./...`, `go vet ./...`. Commit: pending.
- [ ] T5 Define platform-specific, content-identifiable reset ownership in runtime contract; derive wizard inventory and mutation specification from it. Route: delegated writer (multi-file). Checks: exact display golden, matcher tests incl user commands. Commit: pending.
- [ ] T6 Implement backed-up, atomic, all-or-restore installer reset; nested mixed-hook filtering; absent-path rollback and restore-error evidence. Route: delegated writer (multi-file). Checks: filesystem tests for all user-specified scenarios incl restore failure. Commit: pending.
- [ ] T7 Add explicit default-no reset step before scope, detected inventory, apply-plan and final summary; declining preserves prior behavior. Route: delegated writer (multi-file). Checks: model/teatest for one/two agents, navigation, consent. Commit: pending.
- [ ] T8 Finish installation/onboarding/release docs and reset integration regression/coverage. Route: delegated writer (multi-file). Checks: `go test -cover` touched packages, `go test ./...`, `go vet ./...`; no builds. Commit: pending.

## Progress and evidence
- Approved issue read; isolated clean worktree created. Clarifications appended bilingually to issue #767 and verified remotely. No implementation yet.
- T1 delegated writer completed source deletion/generation and focused checks: `go test ./internal/agent -run 'TestBuildGeneratedAgents|Test.*ReviewAgents|Test.*Legacy4R' -count=1` passed; `go vet ./internal/agent` passed; `git diff --check` passed. Assessment initially unassessable because ODD task document was untracked; stage explicitly for reassessment. Full suite pending T3/T4. Commit and independent verification pending.
- Next: finish T1 assessment/verification and commit, then proceed to T2; preserve unrelated documentation worktree.
