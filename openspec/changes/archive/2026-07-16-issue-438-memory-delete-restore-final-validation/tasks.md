# Tasks: Issue 438 Memory Delete/Restore Final Validation

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 40–80 artifact lines |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Capture and validate immutable evidence | PR 1 | Manifest comparison probes | N/A: evidence-only workflow | Remove successor apply evidence only |
| 2 | Run existing checks and close evidence | PR 1 | Listed focused packages, then `go test ./... -count=1` and `go vet ./...` | N/A: no new runtime behavior | Remove successor apply evidence only |

## Phase 1: Baseline and Scope Controls

- [x] 1.1 Capture baseline manifests for repository state, production/test paths, and both historical issue-438 lineages, including HEAD, patch state, untracked hashes, and per-file SHA-256.
- [x] 1.2 Exercise root/HEAD mismatch and staged, unstaged, untracked, and empty-index completeness probes; stop on omission or mismatch without adding test files.
- [x] 1.3 Record and enforce zero edits to production, `*_test.go`, predecessor artifacts, and prior reports; only successor evidence may change.

## Phase 2: Existing Evidence Checks

- [x] 2.1 Run existing focused/touched tests for `hive-daemon/internal/{db,governance,httpapi,sync,mcp}`, `hive-api/internal/{service,handler}`, `jarvis-cli/internal/{hiveclient,hiveui}`, and managed `cmd/jarvis`/agent paths; record receipts and hashes.
- [x] 2.2 Run `go test ./... -count=1` and `go vet ./...` in `hive-daemon`, `hive-api`, and `jarvis-cli`; do not build, remediate, or edit tests.
- [x] 2.3 Classify focused results, established Windows/rootless-Docker/missing-E2E limitations, broad failures, and issue #441 separately; treat changed signatures or unknown failures as blocking.

## Phase 3: Close Apply Evidence

- [x] 3.1 Capture final manifests and prove they exactly match baseline for all immutable production, test, and historical paths.
- [x] 3.2 Write successor `apply-progress.md` with command receipts, output hashes, classifications, evidence digest, and all apply checkboxes complete; leave review binding for the post-apply gate.

## Post-Apply Gates (not APPLY checkboxes)

After all checkboxes complete, native status authorizes exactly one successor-bound review/binding over the closed evidence revision. Then `sdd-verify` writes `verify-report.md` exactly once with `requirements: 5/5` and `scenarios: 7/7`; no report rewrite or rebinding is allowed. Archive runs only through the native archive gate.
