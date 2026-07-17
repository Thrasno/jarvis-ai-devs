# Apply Progress: Issue 438 Memory Delete/Restore Revalidation

## Status

All apply checkboxes are complete. Available broad module suites remain recorded as not green where the Windows environment or known base-only defects prevent completion. No production or test file was written or changed by this successor. Verify and archive remain native later-phase gates and have not run.

## Baseline and Final Pre-Evidence Manifest

| Field | Baseline | Final before successor evidence write | Result |
|---|---|---|---|
| Repository root | `C:/@Sources/jarvis-dev` | `C:/@Sources/jarvis-dev` | Equal |
| HEAD | `ecf0aff626c087dcba8fe930e7f41c6ab312d8e1` | `ecf0aff626c087dcba8fe930e7f41c6ab312d8e1` | Equal |
| Git status SHA-256 | `fd7d38aec278aa4396e78a6b2b5318000b3ae4b16d8a7dead3dd22b4d025b4b5` | same | Equal |
| Staged binary diff SHA-256 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | same | Equal |
| Unstaged binary diff SHA-256 | `f0e5645ed742b0120c7cfa9606218d3c9bde8f73d1d2b84bbda7db9f86eac305` | same | Equal |
| Go manifest SHA-256 | `7eb428eb0ee4cd004c5dfddbce2f5afc40b152ff139c9aa6d56075a2aa21f9c1` | same | Equal |
| Go paths | 463 (238 production, 225 tests; one untracked test) | same | Equal |
| Predecessor manifest SHA-256 | `a4a6e2cd7ea1d43a55d427b519590751a77d039ce72b8a51f123e0498379f4f9` | same | Equal |
| Predecessor artifact paths | 7 | 7 | Equal |

The predecessor paths were `apply-progress.md`, `design.md`, `exploration.md`, `proposal.md`, `tasks.md`, `verify-report.md`, and `specs/memory-delete-restore/spec.md`. Their aggregate and per-file hashes remained unchanged.

The pre-evidence status listed only the five pre-existing successor OpenSpec artifacts. This batch adds only this successor's `apply-progress.md` and task-checkbox evidence, plus the Engram apply-progress topic. It adds no production/test path and does not write predecessor or review-authority files.

## Native Authority

- Gentle AI version: `2.1.6`.
- Fresh lineage `review-4e558b116694ecfe` is approved, bound to this SDD change, and its post-apply gate allowed the candidate. This satisfies tasks 1.2 and 3.1.
- The persisted final manifest evidence was rechecked against its baseline before this artifact-only reconciliation: production/test `7eb428eb0ee4cd004c5dfddbce2f5afc40b152ff139c9aa6d56075a2aa21f9c1`; predecessor `a4a6e2cd7ea1d43a55d427b519590751a77d039ce72b8a51f123e0498379f4f9`; both equal.
- This reconciliation changes the successor artifact scope. Revalidate `review-4e558b116694ecfe` or obtain fresh successor-bound authority before native verify; no authority mutation is performed here.

## TDD Context

Strict TDD is active for implementation work, but this is an evidence-only successor over pre-existing code. No RED/GREEN/REFACTOR cycle is claimed or fabricated. Task 2.1 is a manual collector safety rejection, not a production-test RED cycle.

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 2.1 | `ROOT_MISMATCH_REJECTED=1`; `MANIFEST_INCOMPLETE_REJECTED=1; MISSING=untracked` | N/A — no production implementation | N/A — no code authored |

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command | Focused issue-438 suite passed: daemon DB/governance/http/sync/MCP; Hive API sync service/handler; CLI hiveclient/hiveui; managed path. All commands used `-count=1`; 12 packages passed. |
| Runtime harness | Existing SQLite, `httptest`, direct TUI workflow, and managed-path test harnesses passed; no harness was modified. |
| Rollback boundary | Remove only this successor's `apply-progress.md` and completed evidence checkboxes; do not revert production/tests, predecessor artifacts, or review state. |

## Command Evidence

| Scope | Command | Exit | Result |
|---|---|---:|---|
| Collector safety | Manual root mismatch and missing `untracked` state assertions | 0 | PASS; both unsafe cases rejected |
| Focused issue-438 | Focused daemon/API/CLI commands listed above | 0 | PASS |
| Daemon touched suites | `(hive-daemon) go test ./internal/db ./internal/governance ./internal/httpapi ./internal/sync ./internal/mcp -count=1` | 0 | PASS; output SHA-256 `d83c4afb4f4d29942881a927dfaa9ce23c6a241a725b6f31ae9398a60401a234` |
| Hive API touched suites | `(hive-api) go test ./internal/service ./internal/handler -count=1` | 1 | STOP: Docker limitation plus unknown dashboard asset assertion failure |
| CLI touched suites | `(jarvis-cli) go test ./cmd/jarvis ./internal/agent ./internal/hiveclient ./internal/hiveui -count=1` | 1 | NOT GREEN: Windows symlink privilege failures in `internal/agent`; issue-438 CLI packages passed |
| Daemon vet | `(hive-daemon) go vet ./...` | 0 | PASS |
| Hive API vet | `(hive-api) go vet ./...` | 0 | PASS |
| CLI vet | `(jarvis-cli) go vet ./...` | 0 | PASS |

## Broad Module Evidence (Merged Continuation)

| Module | Command | Exit | Output SHA-256 | Classification |
|---|---|---:|---|---|
| `hive-daemon` | `go test ./... -count=1` | 1 | `a8f90c17e4d29abf46af2cef4b37d3134f1993d3469bde0b8cbd71f8f2dbd768` | NOT GREEN: `cmd/hive-daemon` E2E tests require a temp `hive-daemon-test-*` executable that is absent; no build was run by instruction. Issue-438 packages passed. |
| `hive-api` | `go test ./... -count=1` | 1 | `b3dc789c2ba06c72b0ce5bcf30853a5a44872539deb5011873d3de7de58755b1` | NOT GREEN: rootless Docker is unsupported on Windows. The dashboard asset failure is separately tracked base-only issue [#441](https://github.com/Thrasno/jarvis-ai-devs/issues/441). |
| `jarvis-cli` | `go test ./... -count=1` | 1 | `bb63f4b4f3d3af2fe7753c7542169d6a87eeee1dd4b333d67a030316b21c592d` | NOT GREEN: known Windows symlink privilege and persona isolation failures. Issue-438 CLI packages passed. |
| `hive-daemon` | `go vet ./...` | 0 | empty-output SHA-256 | PASS |
| `hive-api` | `go vet ./...` | 0 | empty-output SHA-256 | PASS |
| `jarvis-cli` | `go vet ./...` | 0 | empty-output SHA-256 | PASS |

`TestRouter_DashboardServesConfiguredAssets/asset_route_returns_static_asset` deterministically returns dashboard HTML instead of the configured JavaScript asset on Windows. Its tracked issue [#441](https://github.com/Thrasno/jarvis-ai-devs/issues/441) records that `filepath.Clean` changes URL path separators before the router's URL-prefix check. Issue-438 does not touch `hive-api/internal/handler/router.go` or `router_test.go`; this is therefore recorded as a pre-existing, base-only failure and is not claimed green.

## Final Manifest Capture (Task 3.1 Evidence)

After all available broad commands, the final production/test and predecessor manifests exactly match the baseline:

| Field | Baseline | Final | Result |
|---|---|---|---|
| HEAD | `ecf0aff626c087dcba8fe930e7f41c6ab312d8e1` | same | Equal |
| Go manifest SHA-256 | `7eb428eb0ee4cd004c5dfddbce2f5afc40b152ff139c9aa6d56075a2aa21f9c1` | same | Equal |
| Go path count | 463 | 463 | Equal |
| Predecessor manifest SHA-256 | `a4a6e2cd7ea1d43a55d427b519590751a77d039ce72b8a51f123e0498379f4f9` | same | Equal |
| Predecessor path count | 7 | 7 | Equal |
| Staged binary diff SHA-256 | empty hash | same | Equal |
| Unstaged binary diff SHA-256 | `f0e5645ed742b0120c7cfa9606218d3c9bde8f73d1d2b84bbda7db9f86eac305` | same | Equal |

Task 3.1 is complete: the manifests match and fresh successor-bound lineage `review-4e558b116694ecfe` supplied the approved post-apply allow. This reconciliation itself changes the candidate artifact scope, so that receipt must be revalidated or replaced before verify.

## Completed Tasks

- [x] 1.1 Immutable baseline and predecessor hash capture.
- [x] 1.3 Zero-edit enforcement for production/tests/predecessor/review history.
- [x] 2.1 Unsafe collector rejection evidence.
- [x] 2.2 Focused issue-438 validation.
- [x] 2.3 Touched suites, all available broad module suites, and all module vet commands reported.
- [x] 1.2 Fresh successor-bound authority (`review-4e558b116694ecfe`).
- [x] 3.1 Final manifests and native receipt/post-apply allow evidence.

## Post-Apply Phase Gates

- **Verify** owns `verify-report.md`; no verify report has been written and verify has not run.
- **Archive** owns archival after native verify; archive has not run.
- Revalidate the receipt or obtain fresh successor-bound authority after this artifact-only scope reconciliation, before attempting verify.
