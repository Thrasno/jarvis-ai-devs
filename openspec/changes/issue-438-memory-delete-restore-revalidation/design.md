# Design: Revalidate Issue 438 Memory Delete and Restore

## Technical Approach

Use an evidence-only pipeline around the already approved current-tree implementation. Before review or verification, capture a reproducible baseline of every staged, unstaged, and untracked production/test path plus byte hashes of the predecessor artifacts. Then follow Gentle AI 2.1.6 native status, bind valid review authority to this successor, independently rerun focused and available broad checks, write only this successor's `verify-report.md`, and compare the final production/test and predecessor manifests with the baseline.

## Architecture Decisions

| Decision | Choice | Alternatives / rationale |
|---|---|---|
| Verification subject | Treat the current issue-438 implementation and tests as immutable, pre-existing input | Reapply or remediate was rejected because it would destroy validation-only causality. |
| Diff proof | Record repository root/HEAD, NUL-safe status, tracked staged/unstaged binary diff hashes, and byte hashes for all changed/untracked production/test files; repeat the identical collector after verification | `git diff` alone omits untracked files and cannot prove zero attributable change. |
| Authority | Read native `jarvis.sdd-status` JSON before each action; route only through `nextRecommended` and `dependencies`. Reference `review-issue438-fixtures-20260715`, but obtain and bind fresh authority to this successor/evidence revision when native status requires it | Reusing prose or an old receipt without native validation is not authority under Gentle AI 2.1.6. |
| Failure classification | Separate issue-438 failures, unknown failures, and established unrelated environment failures | Treating every red command as equivalent would either hide defects or overstate unrelated Windows/Docker limitations. |

## Data Flow

    current tree + predecessor hashes → baseline manifest
                  ↓
    native status → review authority/binding → independent checks
                  ↓
    successor verify-report → final manifest → exact comparison

Any unauthorized transition, invalid binding, predecessor drift, or production/test manifest difference stops the pipeline before closure.

## File Changes

| File | Action | Description |
|---|---|---|
| `openspec/changes/issue-438-memory-delete-restore-revalidation/design.md` | Create | This evidence-pipeline design. |
| `openspec/changes/issue-438-memory-delete-restore-revalidation/verify-report.md` | Create later | New receipts, command results/hashes, limitation classification, and zero-diff proof. |
| `openspec/changes/issue-438-memory-delete-restore/**` | Read only | Immutable semantics, lineage, and historical FAIL evidence. |
| Production and `*_test.go` paths | Read only | Approved implementation and test subject; no edits permitted. |

## Interfaces / Contracts

The baseline/final evidence record contains: repository root and HEAD; timestamp; classified path set; staged and unstaged patch hashes; per-file byte hashes including untracked files; predecessor artifact hashes; Gentle AI version/status JSON; review lineage, receipt, binding, and evidence revision; and command, cwd, exit code, output hash, and classification for every check. Success requires exact baseline/final equality for production/tests and predecessor hashes; successor artifacts are the only allowed delta.

## Testing Strategy

| Layer | What to verify | Approach |
|---|---|---|
| Focused | Delete/restore, explicit v2 acknowledgements, deleted-only restore lookup, terminal rejection, TUI workflow, MCP least privilege | Independently rerun the focused daemon, Hive API, CLI, managed-path, and MCP commands identified by predecessor evidence; do not reuse historical outcomes. |
| Broad | Available module safety nets | Run `go test ./...` and `go vet ./...` independently from `hive-daemon`, `hive-api`, and `jarvis-cli`; no build. |
| Evidence | No attributable product/test or predecessor mutation | Compare final manifests and hashes byte-for-byte with baseline. |

Known Windows symlink/persona failures and rootless-Docker limitations remain **not green** and are reported with command evidence, scope, and established unrelated causality. Unknown or issue-438-attributable failures fail verification. No remediation is proposed or performed.

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior | Planned RED evidence |
|---|---|---|---|
| Documentation-like paths | N/A | No executable classification. | None. |
| Git repository selection | Applicable | Commands use the recorded repository root; root/HEAD mismatch stops. | A mismatched-root preflight must be rejected before evidence collection. |
| Commit state | Applicable | Collector includes staged, unstaged, untracked, and empty-index states; omission stops. | Manifest completeness assertion fails when any present state class is absent. |
| Push state | N/A | No push. | None. |
| PR commands | N/A | No PR automation. | None. |

## Migration / Rollout

No migration required. Rollback removes only successor validation artifacts; predecessor and product/test files remain untouched.

## Open Questions

None.
