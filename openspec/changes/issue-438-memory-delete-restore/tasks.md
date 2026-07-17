# Tasks: Guarded Memory Delete and Restore

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated authored changed lines | 900–1,150 |
| 400-line budget risk | High |
| 1200-line session-budget risk | Medium |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 daemon/API contract; PR 2 sync and persistence; PR 3 CLI/TUI and docs |
| Delivery strategy | single-pr-default |
| Chain strategy | none unless maintainer approves a split |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High
1200-line session-budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Guard/capability/receipt contracts and atomic daemon mutation | PR 1 | `go test ./hive-daemon/internal/{governance,httpapi,db}` | Loopback HTTP tests with stub daemon | daemon governance/httpapi/db files |
| 2 | Causal v2/legacy propagation and API acknowledgement | PR 2 | `go test ./hive-daemon/internal/sync ./hive-api/...` | Injected response-loss/retry sync harness | sync/API files and mutation columns |
| 3 | Installed `jarvis hive` workflow, TUI, MCP guard, docs | PR 3 | `go test ./jarvis-cli/...` | Managed installer-path fixture; no build | CLI/TUI/startscript/docs changes |

## Phase 1: Daemon Safety Contract (RED → GREEN)

- [x] 1.1 RED: add loopback/body-limit/method, incomplete-capability, identity-drift, invalid-reason/confirmation, and response-loss tests in `hive-daemon/internal/{governance,httpapi}`; then implement capabilities, enriched guard, receipt lookup, and sanitized status.
- [x] 1.2 RED: add `t.TempDir` SQLite tests for atomic CAS tombstone/restore, request-id idempotency/conflict, nullable journal fields, deleted-only reads, and audit reason/actor; then update `internal/db/{db.go,memory.go,project.go,sync.go}`.
- [x] 1.3 Verify local success is independent of shared status and old rows derive safely from `synced_at`/compatibility; run focused daemon tests.

## Phase 2: Propagation and API (RED → GREEN)

- [x] 2.1 RED: test unsynced create-then-delete suppression, ordered duplicate/stale events, stale resurrection prevention, v2 ack/failure/retry, and response reconciliation; then update `hive-daemon/internal/sync`.
- [x] 2.2 RED: test Hive API mutation receipt correlation and legacy-row-state `legacy_unsupported`; then update `hive-api/internal/{model,service,repository}` and migrations additively.
- [x] 2.3 Verify sync-v2 and legacy paths separately, including no false completion and retryable failure evidence.

## Phase 3: CLI, TUI, and Least Privilege (RED → GREEN)

- [x] 3.1 RED: add client contract tests for capabilities, backup renewal, deleted-only/detail, identity re-read, request-id reuse, and receipt DTOs; then update `jarvis-cli/internal/hiveclient/client.go`.
- [x] 3.2 RED: add direct `Model.Update` tests for separate Active/Recently Deleted slices, exact confirmations, duplicate-submit lock, reconciliation states, and legacy disablement; then update `jarvis-cli/internal/hiveui/{runner.go,model.go}`.
- [x] 3.3 Verify installed/managed `jarvis hive` resolution through `cmd/jarvis/cmd_hive.go` and `internal/agent/startscript.go`; add MCP tests proving no delete/restore/guard tool or request exists.

## Phase 4: Documentation and Final Evidence

- [x] 4.1 After terminology stabilizes, update user-facing workflow/status documentation; confirm no hard-delete, bulk, agent, or MCP mutation claims.
- [x] 4.2 Run `go test ./...` and `go vet ./...`; record package results, focused acceptance evidence, and any intentionally skipped external harnesses. Do not build. **Maintainer-approved verified exception**: focused/touched suites and `jarvis-cli` vet are green; unrelated Windows/environment broad-suite blockers are recorded in apply-progress.
