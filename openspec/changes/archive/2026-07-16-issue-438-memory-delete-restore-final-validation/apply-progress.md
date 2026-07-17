# Apply Progress: Issue 438 Memory Delete/Restore Final Validation

## Status

All eight apply-only evidence tasks are complete. This apply run wrote no production, test, or historical artifact path. Review/binding, verify, and archive have not run and remain post-apply native gates.

## Exact Specification and Task Boundaries

- The successor delta spec contains exactly **5** `Requirement` headings and **7** `Scenario` headings.
- `tasks.md` has exactly eight APPLY checkboxes. Its post-apply section has no review, verify, or archive checkbox task.
- Strict TDD is active, but no production behavior or test was authored. No RED/GREEN/REFACTOR cycle is claimed or fabricated.

## Baseline and Final Immutable Manifests

The final capture occurred immediately before writing this successor evidence. Its immutable inputs match baseline exactly.

| Field | Baseline | Final | Result |
|---|---|---|---|
| Repository root | `C:/@Sources/jarvis-dev` | `C:/@Sources/jarvis-dev` | Equal |
| HEAD | `ecf0aff626c087dcba8fe930e7f41c6ab312d8e1` | same | Equal |
| Git status SHA-256 | `475d6f752c9eb01162d783e75561a953893181ab408b37ecf5c214b2cb11783d` | same | Equal before successor evidence write |
| Staged binary diff SHA-256 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | same | Equal; empty index captured |
| Unstaged binary diff SHA-256 | `7d438a6fa583e3610c283e4225bf79431d2e18ffc1bfd4df1fb2fda365856d21` | same | Equal |
| Go per-file SHA-256 manifest | `38e33130608ea4a6df3c4e08dc564525f238894334a7ab0a49ac130f8326b47a` | same | Equal |
| Go paths | 463 total (238 production, 225 tests) | same | Equal |
| Historical per-file SHA-256 manifest | `d453b0fd6fbfeca92abb68245a49c93c37d1082dc24d22a0b3dcf423975735c2` | same | Equal |
| Historical paths | 14 across both issue-438 lineages | same | Equal |
| Untracked per-file SHA-256 manifest | `1c42223c33e020c388c5d11b8e8341f031aeca54d53b74512e9494234748ab3b` | same | Equal |
| Untracked paths | 21 | same | Equal before successor evidence write |

Each aggregate is the SHA-256 of the sorted `path<TAB>file-SHA-256` mapping. The immutable historical path set is the original `issue-438-memory-delete-restore` lineage and the `issue-438-memory-delete-restore-revalidation` lineage. The pre-existing dirty tree was captured as input; this apply run did not alter any of those paths.

## Collector Completeness Probes

| Probe | Exact result | Classification |
|---|---|---|
| Mismatched repository root | `REJECTED` | PASS |
| Mismatched HEAD | `REJECTED` | PASS |
| Staged state | `empty-index-captured` | PASS |
| Unstaged state | `captured` | PASS |
| Untracked state | `captured` | PASS |
| Missing state class | `0` | PASS |

## Focused / Touched Evidence

| Module | Command | Exit | Output SHA-256 | Classification |
|---|---|---:|---|---|
| `hive-daemon` | `go test ./internal/db ./internal/governance ./internal/httpapi ./internal/sync ./internal/mcp -count=1` | 0 | `8959fe3257dc5a5d53b943832d7ffdc47974a272b0c444bc21452081905028b5` | PASS; 5 packages |
| `hive-api` | `go test ./internal/service ./internal/handler -count=1` | 1 | `71a8d0e85d9bb6104d4e352f615f5b9628e40a8804f386299aa2ffb361ea57f5` | NOT GREEN; known Windows rootless-Docker, symlink privilege, and #441 signatures only |
| `jarvis-cli` | `go test ./cmd/jarvis ./internal/agent ./internal/hiveclient ./internal/hiveui -count=1` | 1 | `d98aa7a857f76a27e3c03c4e88b19fa91edcb59e9e7aec9bd6ce6616d6500287` | NOT GREEN; known Windows symlink privilege signature only; issue-438 CLI packages passed |

Existing runtime boundaries executed through SQLite/`httptest`/TUI and managed CLI test harnesses. No harness was changed.

## Broad Suites and Vet Evidence

| Module | Command | Exit | Output SHA-256 | Classification |
|---|---|---:|---|---|
| `hive-daemon` | `go test ./... -count=1` | 1 | `82ac990704bd8b8462168fe99bc85bc5edbeb2f6c8d6ea04ef2d0080bf5cf589` | NOT GREEN: exact absent `hive-daemon-test-*` E2E executable signature; no build was run |
| `hive-api` | `go test ./... -count=1` | 1 | `c2a5ac8e4762aebc3e2eeba4305a6f93334cbfa0acba33070700716c8cbd3358` | NOT GREEN: exact Windows rootless-Docker and symlink privilege signatures, plus #441 |
| `jarvis-cli` | `go test ./... -count=1` | 1 | `2607d1cd3581b037ef005d4337eb92ccfc3f30fb0c45bf52d0cdb25ef1a175a9` | NOT GREEN: exact Windows symlink/persona isolation signatures |
| `hive-daemon` | `go vet ./...` | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | PASS |
| `hive-api` | `go vet ./...` | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | PASS |
| `jarvis-cli` | `go vet ./...` | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | PASS |

## Failure Classification

Only previously verified unrelated signatures were observed:

- **Daemon E2E executable**: `exec: ... hive-daemon-test-...: executable file not found in %PATH%`.
- **Windows rootless Docker**: `rootless Docker is not supported on Windows` while starting PostgreSQL test containers.
- **Windows symlink/persona**: `A required privilege is not held by the client` from symlink-based test setup and the known persona-isolation boundary.
- **Dashboard #441**: `TestRouter_DashboardServesConfiguredAssets/asset_route_returns_static_asset` returned `<html>Hive Dashboard</html>` instead of `console.log('dashboard')`.

No additional or changed failure signature was observed. These failures are not green, are not attributed to issue 438, and received no remediation.

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 1.1–3.2 | N/A — evidence-only task; no test or production code authored | N/A — existing evidence commands recorded above | N/A — no code authored |

## Work Unit Evidence

| Work unit | Focused test command and exact result | Runtime harness command/scenario and exact result | Rollback boundary |
|---|---|---|---|
| 1 — immutable evidence | Manifest collector probes: exit 0; root/HEAD rejected, all four state classes captured | N/A — no new runtime boundary; existing code is immutable input | Remove only this successor `apply-progress.md` and checkbox marks |
| 2 — existing checks | Focused commands above: daemon exit 0; API/CLI exit 1 only for classified unrelated signatures | Existing SQLite, `httptest`, TUI, and managed CLI harnesses ran via focused packages; no harness changed | Remove only this successor `apply-progress.md` and checkbox marks |

## Evidence Digest

Canonical receipt digest (HEAD, immutable manifest hashes, and the six test-output hashes): `sha256:a9fd8b49b174661023d2828b22c999c833eb0ad98009767fac2a40bcd52c7cdc`.

## Completed Apply Tasks

- [x] 1.1 Capture baseline manifests.
- [x] 1.2 Exercise completeness probes.
- [x] 1.3 Enforce successor-only scope.
- [x] 2.1 Run focused/touched tests.
- [x] 2.2 Run broad suites and all vets.
- [x] 2.3 Classify all observed failures.
- [x] 3.1 Capture and compare final manifests.
- [x] 3.2 Persist closed apply evidence.

## Post-Apply Gates

One successor-bound review/binding is required before `sdd-verify`. `sdd-verify` alone may create the write-once `verify-report.md`; archive requires its later native authorization. Neither operation is an apply checkbox.
