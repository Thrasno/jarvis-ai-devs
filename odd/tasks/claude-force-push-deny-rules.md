# Claude force-push deny rules

Objective: stop emitting invalid Claude Bash deny patterns and replace obsolete Jarvis-emitted entries on configuration replay without removing user-owned settings.

Scope: Claude settings generation and focused regression tests. No release, issue, or PR. User explicitly requested a direct commit and push to master. Other worktree's unrelated changes are out of scope.

TDD: repository AGENTS.md expects strict TDD for SDD only; this is ODD. Runner: `go test ./internal/agent` from `jarvis-cli`, then `go test ./...` and `go vet ./...` if feasible. Build prohibited.

Delivery strategy: exception-ok (explicit direct master request); forecast <150 authored diff lines. Review: RDD global on; native risk assessment at work-unit boundary.

- [x] T1: Correct generated force-push deny rules and migrate only the four obsolete exact entries; prove behavior and idempotency in tests. Route: delegated writer (two nontrivial files). Check: `cd jarvis-cli && go test ./internal/agent` passed (worker); `git diff --check` passed. Commit: `b65db70d29e95bc5b5e55d5d2e5d118b897f63a7`.
- [x] T2: Verify suite and diff, assess/review as required, then push master without force. Route: delegated verifier for command execution. Checks: `cd jarvis-cli && go test ./...`, `cd jarvis-cli && go vet ./...`, `git diff --check` passed. Native high-risk review approved and acknowledged (`review-36bae2adf10cf1b6`); `master` pushed fast-forward to `ae00b9f4d2aca02eaf409cd218b61692ef1a66d8` and confirmed on remote.

Progress: implementation delivered directly to `public/master`; no issue, PR, or release. This final documentation update is passive and does not change the reviewed implementation.
