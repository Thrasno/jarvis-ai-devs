# Claude force-push deny rules

Objective: stop emitting invalid Claude Bash deny patterns and replace obsolete Jarvis-emitted entries on configuration replay without removing user-owned settings.

Scope: Claude settings generation and focused regression tests. No release, issue, or PR. User explicitly requested a direct commit and push to master. Other worktree's unrelated changes are out of scope.

TDD: repository AGENTS.md expects strict TDD for SDD only; this is ODD. Runner: `go test ./internal/agent` from `jarvis-cli`, then `go test ./...` and `go vet ./...` if feasible. Build prohibited.

Delivery strategy: exception-ok (explicit direct master request); forecast <150 authored diff lines. Review: RDD global on; native risk assessment at work-unit boundary.

- [x] T1: Correct generated force-push deny rules and migrate only the four obsolete exact entries; prove behavior and idempotency in tests. Route: delegated writer (two nontrivial files). Check: `cd jarvis-cli && go test ./internal/agent` passed (worker); `git diff --check` passed. Commit: pending identity.
- [ ] T2: Verify suite and diff, assess/review as required, then push master without force. Route: delegated verifier for command execution. Check: full Go tests, vet, remote fast-forward. Commit evidence: pending.

Progress: writer updated generator and migration with order-preserving replay; focused tests passed. Next: commit T1, then T2.
