# Design: Final Validation for Issue 438 Memory Delete and Restore

## Technical Approach

Use one evidence-only apply sequence over the unchanged issue-438 implementation. Apply captures baseline manifests, runs existing focused/touched and broad checks, records `go vet`, classifies failures, and captures final manifests. After every apply checkbox is complete, exactly one successor-bound post-apply review/binding attests that closed evidence revision. Verify then evaluates only that bound evidence and creates `verify-report.md` once. Archive remains a later native gate, never an apply task.

## Architecture Decisions

| Option | Tradeoff | Decision |
|---|---|---|
| Reuse or repair historical evidence | Fewer artifacts, but corrupts audit history | Reject. Both earlier issue-438 lineages and all production/tests are immutable. |
| Put verify/archive in `tasks.md` | Appears unified, but native verify would depend on its own unchecked task | Reject. Checkboxes cover apply work only; verify/archive are phase gates. |
| Bind before apply finishes or rebind after report creation | Either binds incomplete evidence or recreates the report-authority cycle | Reject. Perform exactly one binding after apply, covering the closed apply evidence revision; the phase-owned report is the sole permitted later write. |
| Re-run existing evidence without remediation | Cannot turn unrelated failures green, but preserves causality | Adopt. Exact results and output hashes are recorded; unknown or issue-438 failures stop before binding. |

## Data Flow

    baseline manifests → apply checks → final manifests → all apply tasks complete
                                                        ↓
    native verify-ready status → one post-apply review/binding
                                                        ↓
    verify reads bound evidence → write report once → native archive gate

No post-report review, rebinding, correction, or report rewrite is allowed. Stale evidence requires a new successor.

## File Changes

| File | Action | Description |
|---|---|---|
| `openspec/changes/issue-438-memory-delete-restore-final-validation/design.md` | Create | This design. |
| `openspec/changes/issue-438-memory-delete-restore-final-validation/tasks.md` | Create later | Apply-only evidence checkboxes; no verify/archive checkbox. |
| `openspec/changes/issue-438-memory-delete-restore-final-validation/apply-progress.md` | Create later | Baseline/final manifests, command receipts, classifications, and completed apply state. |
| `openspec/changes/issue-438-memory-delete-restore-final-validation/verify-report.md` | Create once later | Final 5/5, 7/7 verdict over the bound evidence revision. |
| `openspec/changes/issue-438-memory-delete-restore*/` (other lineages), production Go, and `*_test.go` paths | Read only | Immutable validation inputs. |

## Interfaces / Contracts

Each manifest records repository root, HEAD, classified path set, staged/unstaged patch hashes, untracked file hashes, and per-file SHA-256. Separate manifests cover production/tests and both historical OpenSpec lineages. Baseline and final values must match exactly.

Each command receipt records command, module cwd, exit code, output hash, and classification. The binding receipt identifies the successor, completed apply evidence digest, review lineage, and post-apply allow/deny result. The verify report references that digest and records exactly `requirements: 5/5` and `scenarios: 7/7`; creation is atomic and fails if the path already exists.

Native `jarvis.sdd-status` JSON is authoritative: verify runs only when `dependencies.sdd-verify` is ready and `nextRecommended` authorizes it; archive uses the equivalent archive gate.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Focused/touched | Guarded delete/restore, v2 acknowledgement/rejection, deleted-only lookup, TUI, MCP least privilege, managed CLI path | Run predecessor-derived existing tests in `hive-daemon/internal/{db,governance,httpapi,sync,mcp}`, `hive-api/internal/{service,handler}`, and `jarvis-cli/internal/{hiveclient,hiveui}`, plus managed `cmd/jarvis`/agent tests. Add no tests. |
| Broad | Module safety nets | Run `go test ./... -count=1` and `go vet ./...` in `hive-daemon`, `hive-api`, and `jarvis-cli`; no build. |
| Evidence | Scope and immutability | Compare baseline/final manifests before the single binding. |

Known Windows symlink/persona failures, rootless-Docker limitations, missing daemon E2E executable, and issue #441 remain **not green** only when current output matches established unrelated evidence. Any changed signature is unknown and blocking. No remediation occurs.

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no executable-file classification | SDD paths are evidence only. | None. |
| Git repository selection | Applicable | Use recorded absolute root; root/HEAD mismatch stops before collection. | Apply probe supplies a mismatched root and proves rejection; no test file added. |
| Commit state | Applicable | Include staged, unstaged, untracked, and empty-index states; omission stops. | Apply completeness probes exercise each state class; no test file added. |
| Push state | N/A — no push | No remote mutation. | None. |
| PR commands | N/A — no PR automation | No PR command composition. | None. |

## Migration / Rollout

No migration required. Before verify, successor planning/apply artifacts may be withdrawn. After report creation, nothing is rewritten; failure or staleness requires a new successor.

## Open Questions

None.
