# Tasks: Issue 438 Memory Delete/Restore Revalidation

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 80–140 SDD evidence/report lines; 0 production/test lines attributable to this change; existing implementation is reviewed, not counted as authored change |
| 400-line budget risk | Low (single-PR budget: 2,300 lines) |
| Chained PRs recommended | No |
| Suggested split | Single PR: validation evidence and closure artifacts only |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Immutable baseline, authority, validation, and closure evidence | PR 1 | Issue-438 focused commands; then touched suites | Native `jarvis.sdd-status`/review dispatcher; no harness modification | Remove only successor evidence artifacts; never revert product/tests or predecessor |

## Phase 1: Immutable Baseline and Authority

- [x] 1.1 Capture repository root/HEAD, NUL-safe staged/unstaged/untracked status, production and `*_test.go` byte hashes, and predecessor artifact hashes; reject mismatched root, incomplete state classes, or predecessor drift.
- [x] 1.2 Fresh successor-bound authority is satisfied by approved lineage `review-4e558b116694ecfe`, whose post-apply gate allowed this SDD change.
- [x] 1.3 Prohibit all production, `*_test.go`, predecessor, review-receipt, and historical-report edits; stop immediately on any attributable defect or mutation. Do not generate, replay, repair, or modify code/tests.

## Phase 2: Independent Validation

- [x] 2.1 RED evidence: exercise repository-root mismatch and manifest-completeness assertions; prove they reject unsafe collection before any validation proceeds, without adding tests or changing code.
- [x] 2.2 Independently run predecessor-derived issue-438 focused delete/restore, acknowledgements, deleted-only lookup, terminal rejection, TUI, MCP least-privilege, managed-path, daemon, and API checks; classify any product/test defect as stop/fail.
- [x] 2.3 Run touched package suites, available module-wide `go test ./...` from `hive-daemon`, `hive-api`, and `jarvis-cli`, and `go vet ./...` in all modules; report Windows symlink/persona and rootless-Docker failures as not green and unrelated only when evidenced.

## Phase 3: Final Apply Evidence

- [x] 3.1 Final manifests match the baseline exactly and approved lineage `review-4e558b116694ecfe` supplied the native receipt/post-apply allow condition. Production/test manifest: `7eb428eb0ee4cd004c5dfddbce2f5afc40b152ff139c9aa6d56075a2aa21f9c1`; predecessor manifest: `a4a6e2cd7ea1d43a55d427b519590751a77d039ce72b8a51f123e0498379f4f9`.

## Post-Apply Phase Gates

Native phase ownership prevents these later-phase actions from being apply checkboxes:

- **Verify phase** owns creation of `verify-report.md`. It has not run and no report has been written.
- **Archive phase** owns archival after native verify succeeds. It has not run.
- This reconciliation changes the successor artifact scope. Before verify, revalidate the existing receipt or obtain fresh successor-bound authority; do not rely on `review-4e558b116694ecfe` without that native revalidation.
