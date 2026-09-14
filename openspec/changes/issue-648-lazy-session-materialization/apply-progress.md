# Apply Progress: Lazy session materialization

## Slice 1 — API regular-session lifecycle acceptance

**Status:** implementation and broad verification complete; awaiting native review and PR checkpoint.
**Boundary:** PR/slice 1 only (`main <- 📍1 <- 2`). No local lifecycle producer, schema migration, push, or PR was created.

### Completed implementation tasks

- [x] RED — added repository lifecycle acceptance coverage and a service forwarding test.
- [x] GREEN — split `CreateSession`'s correction-only conflict clause from regular sync lifecycle acceptance. Regular sync conflicts now apply nullable `ended_at`, summary, and a new server watermark while preserving identity/provenance and the relocation predicate. `sync.go` already forwarded nullable lifecycle fields unchanged; its behavior is now pinned by a test.
- [x] TRIANGULATE/REFACTOR — covered NULL reopen versus ended update, protected sentinel branches, and an explicitly skippable PostgreSQL watermark-pull integration case; renamed clause/comments and ran gofmt.
- [ ] Checkpoint — production+test diff is 299 additions+deletions against `11e6a824`, below the 399-line slice budget; PR attachment remains pending.

Persisted task checkboxes were updated in `tasks.md` only for slice 1.

### Files changed

| File | Change |
| --- | --- |
| `hive-api/internal/repository/postgres_session.go` | Split create correction and regular sync lifecycle conflict clauses; regular sessions now accept incoming `ended_at` (including NULL), summary, and watermark. |
| `hive-api/internal/repository/postgres_session_lifecycle_test.go` | Added fake-querier unit tests plus a `testing.Short()`-explicit PostgreSQL pull integration test. |
| `hive-api/internal/service/sync_test.go` | Pinned forwarding of nullable regular-session lifecycle fields. |
| `openspec/changes/issue-648-lazy-session-materialization/tasks.md` | Marked the four slice-1 implementation tasks complete. |
| `openspec/changes/issue-648-lazy-session-materialization/apply-progress.md` | This cumulative progress record. |

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 RED/GREEN | `hive-api/internal/repository/postgres_session_lifecycle_test.go` | Unit | `go test ./internal/repository ./internal/service` timed out after 120s before edits (PostgreSQL integration environment; no test failure output) | `TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen` failed: SQL lacked `ended_at = EXCLUDED.ended_at` and `summary = EXCLUDED.summary` | Passed after the lifecycle-only clause was added | Added ended-state, identity/provenance, sentinel-branch, and pull cases | Clause names/comments clarified; gofmt then focused tests passed |
| 1 service forwarding | `hive-api/internal/service/sync_test.go` | Unit | Same timeout | Added during triangulation; pre-existing forwarding behavior passed because `sync.go` already maps `EndedAt` and `Summary` directly | No production change required in `sync.go` | Nullable `EndedAt=nil` plus summary asserted at repository ingress | gofmt and focused tests passed |

### Commands and results

| Command | Result |
| --- | --- |
| `cd hive-api && go test ./internal/repository ./internal/service` (pre-edit safety net) | Timed out after 120s; no failing assertion output. |
| `cd hive-api && go test ./internal/repository -run '^TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen$' -count=1` | RED: failed on missing nullable lifecycle/summary SQL assignments. |
| `cd hive-api && gofmt -w internal/repository/postgres_session.go internal/repository/postgres_session_lifecycle_test.go && go test ./internal/repository -run '^TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen$' -count=1` | GREEN: passed. |
| `cd hive-api && gofmt -w internal/repository/postgres_session_lifecycle_test.go internal/service/sync_test.go && go test -short ./internal/repository ./internal/service -run '^(TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen|TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePreservesIdentityAndSentinelBranches|TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePullsReopenedRow|TestSyncService_Push_ForwardsNullableRegularSessionLifecycle)$' -count=1 -v` | Passed: 3 tests; PostgreSQL pull integration explicitly skipped under `testing.Short()`. |
| `cd hive-api && go test ./...` | Timed out after 180s after `cmd/server`, `internal/config`, `internal/handler`, `internal/middleware`, and `internal/model` passed; no package failure was reported. Repository/service PostgreSQL integration did not complete in this environment. |
| `cd hive-api && go vet ./...` | Passed (exit 0). |
| `cd hive-api && test -z "$(gofmt -l internal/repository/postgres_session.go internal/repository/postgres_session_lifecycle_test.go internal/service/sync_test.go)"` | Passed (`gofmt clean`). |

### Diff / workload checkpoint

- Production + test diff against `11e6a824`: **210** additions+deletions (47 tracked additions + 11 tracked deletions + 152-line untracked lifecycle test).
- This is below the slice threshold of 399 changed lines; no `size:exception` is needed.
- The task-prescribed `git merge-base origin/main HEAD` could not be calculated because this worktree has no `origin/main` ref. The explicit user-supplied baseline `11e6a824` was used instead.
- Rollback: revert the API-only regular lifecycle conflict clause and its tests. Keep it deployed once a reopen producer lands.

### Deviations and risks

- No deviation from the API-only design boundary. `sync.go` required no production edit because it already passed through nullable lifecycle data; the new service test prevents regression.
- PostgreSQL integration is explicit and skipped only under `testing.Short()`. The non-short repository/module suites timed out in this environment, so the integration pull case remains unexecuted here.
- The parent owns review/PR lifecycle. Deferred parent action: review the bounded API diff and its PostgreSQL pull/watermark evidence before merge. Do not start slice 2 from this worktree.

### Remaining tasks

All remaining unchecked implementation tasks are intentionally outside slice 1 (slices 2–10). Parent-owned review/lifecycle checkboxes remain unchanged.

### Structured status consumed

- `changeName`: `issue-648-lazy-session-materialization`
- `artifactStore`: `openspec`
- `applyState`: `ready` (parent supplied)
- `actionContext.mode`: `repo-local`
- `actionContext.workspaceRoot`: `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`
- `actionContext` warning: none supplied.

## Slice 1 — correction-only verifier remediation

**Status:** completed; verifier finding closed. This correction remains within slice 1 (`main <- 📍1 <- 2`) and changes no later slice, schema, producer, generated file, commit, push, or PR.

### Finding and correction

- **Finding:** regular `UpsertSession` calls with no `FromProject` passed `relocationSource(s.Project, s.FromProject)` as `$10`. That returns `""`, so `WHERE sessions.project = $10` never matched an ordinary same-project row and lifecycle UPDATE was unreachable.
- **Correction:** regular calls now bind `$10` to `s.Project` when `FromProject` is absent. Explicit relocation keeps its source project through `relocationSource`; explicit self-relocation keeps the existing empty no-op predicate. Sentinel dispatch, correction-only `CreateSession`, quarantine checks, immutable identity/provenance fields, and schema remain unchanged.

### Persisted task state

- Re-read `tasks.md`: all four slice-1 implementation rows remain visibly `- [x]`.
- No later-slice or parent-owned checkbox was modified. The existing slice-1 task completion remains the truthful persisted completion record for this correction-only work.

### TDD Cycle Evidence

| Task | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- |
| Ordinary same-project predicate | Added an assertion that argument `$10` is `"alpha"` when `FromProject` is absent. Focused repository test failed before production edit: expected `"alpha"`, actual `""`. | Bound `$10` to the destination project only when no source is supplied; focused predicate and relocation tests passed. | Added source-preservation and self-relocation no-op assertions; `gofmt` clean. |
| Repeated open / effective lifecycle state | Extended the PostgreSQL integration to issue two ordinary open updates, retain the original start/provenance, retain open state, use the second summary, and expose the row through the watermark pull. The short run explicitly skipped this container test. | Non-short PostgreSQL/Testcontainers run passed after the predicate correction. | The first container run exposed a test-only timezone-location comparison problem; changed it to instant equality (`Time.Equal`) and reran green. |

### Verification

| Command | Result |
| --- | --- |
| `cd hive-api && go test ./internal/repository -run '^(TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen|TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePreservesRelocationSource)$' -count=1` | **RED:** failed before the production correction (`$10`: expected `alpha`, actual empty). |
| Same focused predicate command after production edit | **GREEN:** passed. |
| `cd hive-api && go test -short ./internal/repository ./internal/service -run '^(TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen|TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePreservesRelocationSource|TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePreservesIdentityAndSentinelBranches|TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePullsReopenedRow|TestSyncService_Push_ForwardsNullableRegularSessionLifecycle)$' -count=1 -v` | Passed: repository predicate/relocation/sentinel tests and service forwarding test. PostgreSQL lifecycle integration **skipped explicitly** under `-short`. |
| `cd hive-api && go test ./internal/repository -run '^TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePullsReopenedRow$' -count=1 -v` | First container run failed only on `time.Time` location representation; after test-only instant-equality refactor, second PostgreSQL/Testcontainers run **passed**. |
| `cd hive-api && go test ./internal/repository ./internal/service` | Timed out at 180 seconds with no output; do not treat this bounded package suite as passed. |
| `cd hive-api && go vet ./...` | Passed. |
| `gofmt -l` for the three changed Go files; `git diff --check` | Passed (no output). |

### Diff / boundary

- Production + test diff against `11e6a824`: **228 additions + 12 deletions = 240 changed lines**, below the 399-line slice budget.
- The calculation includes tracked production/service changes and the untracked lifecycle test via `git diff --no-index /dev/null`; it excludes OpenSpec artifact bookkeeping.
- Rollback remains: revert the API lifecycle clause and its lifecycle/service tests as the slice-1 unit. Do not alter relocation or sentinel branches independently.

### Remaining / risks

- The verifier blocker is closed: ordinary same-project lifecycle updates can reach the existing guarded conflict clause; repeated opens and watermark pull are proven in PostgreSQL.
- Later implementation tasks (slices 2–10) and all parent lifecycle tasks remain intentionally unchecked and deferred to the parent.

## Slice 1 — relocation-regression remediation and final verification

The first broad repository run exposed a real regression in `TestUpsertSession_RepushUnderCorrectedProjectMovesOnlyTheProject`: true project correction cleared the stored `ended_at` and summary. The final SQL applies incoming lifecycle fields only when the stored and incoming projects are equal; true relocation still updates project/watermark while preserving prior lifecycle state. The adjacent source comment now documents ordinary, relocation, and self-relocation behavior accurately.

- **RED:** broad repository/service run failed because relocation produced nil `ended_at` and summary; a subsequent broad run caught a stale SQL-fragment assertion after the production correction.
- **GREEN:** focused ordinary reopen, relocation, sentinel, and live PostgreSQL watermark tests passed after the conditional lifecycle update and assertion correction.
- **Broad verification:** `cd hive-api && go test ./internal/repository ./internal/service` passed (`repository` 265.312s; `service` cached).
- **Module verification:** `cd hive-api && go test ./... && go vet ./...` passed; changed Go files are gofmt-clean and `git diff --check` passed.
- **Final size:** 299 production+test changed lines against `11e6a824`. SDD bookkeeping is committed separately from the code/test candidate so the review slice remains below 399 without deleting or compressing evidence.
    - **Pending:** native review, local code/test commit, and eventual PR checkpoint/attachment. No push or PR was performed.

## Slice 2 — Transactional lifecycle-store foundation

**Status:** implementation complete; PR checkpoint and parent lifecycle remain pending.
**Boundary:** `main <- 1 <- 📍2 <- 3`. This changes additive DB primitives only: no HTTP/MCP adapter, hook, sync acknowledgement, schema, commit, push, or PR.

### Completed implementation tasks

- [x] RED — added table-driven lifecycle tests before the store API existed. The focused command failed to compile because `models.SessionInput` and `(*DB).EnsureSession` were undefined; end variants then failed because `EnsureAndEndSession` and `ErrSessionAlreadyEnded` were undefined.
- [x] GREEN — added `SessionInput`/`SessionEndInput`, transactional `EnsureSession` and `EnsureAndEndSession`, and a private transaction-scoped materialization/read path. It registers the canonical identity first, checks target and stored-project writability, returns `*project.ValidationError{Code: project.CodeProjectSessionMismatch}` for incompatible canonical bindings, preserves regular provenance/identity, reopens only write paths, and rolls back failed lifecycle writes.
- [x] TRIANGULATE/REFACTOR — covered absent/active/ended paths, compatible variants, mismatch, default client, developer-ID healing, target/existing blocks, insert/reopen/end trigger rollbacks, duplicate-end semantics, and two-handle concurrent first writes. Extracted only the mode and transaction-reader helpers; legacy `CreateSession`/`EndSession` retain their existing behavior.

Persisted `tasks.md` slice-2 RED, GREEN, and TRIANGULATE/REFACTOR rows are visibly marked `- [x]`.

### Files changed

| File | Change |
| --- | --- |
| `hive-daemon/internal/models/session_write.go` | Adds additive write command inputs. |
| `hive-daemon/internal/db/session.go` | Adds typed, transaction-owned ensure/reopen/end primitives and preserves legacy methods. |
| `hive-daemon/internal/db/session_lifecycle_test.go` | Adds SQLite/t.TempDir lifecycle, rollback, gate, and two-handle convergence coverage. |
| `openspec/changes/issue-648-lazy-session-materialization/tasks.md` | Marks only the three completed slice-2 implementation rows. |
| `openspec/changes/issue-648-lazy-session-materialization/apply-progress.md` | Cumulative slice-2 evidence. |

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Slice 2 lifecycle primitives | `hive-daemon/internal/db/session_lifecycle_test.go` | SQLite DB | `cd hive-daemon && go test ./internal/db` passed before edits | Focused absent-session test failed: missing `SessionInput` and `EnsureSession` | Passed after minimal input plus transactional absent-session creation | End/mismatch tests then failed for missing `EnsureAndEndSession`/`ErrSessionAlreadyEnded`; final table-driven lifecycle, rollback, gate, healing, and concurrency variants pass | Private mode and transaction reader extracted; focused tests remained green |

### Verification

| Command | Result |
| --- | --- |
| `cd hive-daemon && go test ./internal/db` (pre-edit) | Passed. |
| `cd hive-daemon && go test ./internal/db -run '^TestEnsureSession_CreatesAbsentAttributedSession$' -count=1` | RED: failed to compile (`EnsureSession`/`SessionInput` undefined). |
| `cd hive-daemon && go test ./internal/db -run '^(TestEnsureSession_ReusesAndReopensCompatibleSessions|TestEnsureSession_RejectsCanonicalProjectMismatch|TestEnsureAndEndSession_LifecycleAndRollback)$' -count=1` | RED: failed to compile (`EnsureAndEndSession`/`ErrSessionAlreadyEnded` undefined). |
| Focused lifecycle command after implementation | GREEN and triangulation: passed. |
| `cd hive-daemon && go test ./internal/db` | Passed. |
| `cd hive-daemon && go test -race ./internal/db` | Passed (87.608s) before the final test-only healing addition. |
| `cd hive-daemon && go test -race ./internal/db -run '^(TestEnsureSession_(ConcurrentFirstWritesConverge|HealsEmptyDeveloperID)|TestEnsureAndEndSession_LifecycleAndRollback)$' -count=1` | Passed (1.920s) after final changes. |
| `cd hive-daemon && go test ./...` | Passed. |
| `cd hive-daemon && go vet ./...` | Passed (no output, exit 0). |
| `git diff --check`; targeted `gofmt -l` | Passed clean. |

### Workload / PR boundary

- Product + test diff against the required local predecessor `afe6bbd5`: **387 additions + 0 deletions = 387 changed lines**, below the 399-line cap.
- `origin/main` is unavailable in this worktree, so the task-prescribed merge-base command cannot be evaluated. The user-supplied local predecessor was used instead.
- Rollback: revert the unused primitives and their tests together before adapter consumers land; do not modify already materialized rows.
- Excluded: all adapters, capture transactions, sync acknowledgement, API changes, and later slices.

### Deviations, risks, and remaining tasks

- No design deviation. New public methods are intentionally unused until later adapter slices.
- The two-handle test proves DB-level convergence with a barrier channel and no sleeps; it does not add the later adapter-local coalescer.
- Remaining implementation-owned slice-2 task (left unchecked):
  - `- [ ] **Checkpoint:** Confirm PR 2 is below 400 changed lines using the mandated merge-base calculation; record actual size, DB/race evidence, exclusions (all adapters and sync acknowledgement), rollback, and \`main <- 1 <- 📍2 <- 3\` before starting slice 3. <!-- sdd-owner: implementation -->`
- Later slices and all parent-owned lifecycle rows remain unchanged and deferred. Parent must review the bounded DB primitive work and own the PR/merge checkpoint. No commit, push, or PR was performed.

### Structured status consumed

- `changeName`: `issue-648-lazy-session-materialization`
- `artifactStore`: `openspec`
- `applyState`: `ready` (parent supplied)
- `actionContext.mode`: `repo-local`
- `actionContext.workspaceRoot`: `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`
- `actionContext` warning: operate only in the supplied worktree and allowed edit surfaces; satisfied.

## Slice 2 — delivery-boundary reset

**Status:** retained and revalidated. The authorized reset split the former combined lifecycle candidate: this worktree now contains slice 2 only (`main <- 1 <- 📍2 <- 3`); its PR checkpoint is open.

- Retained `SessionInput`, transaction-owned `EnsureSession`, regular create/replay/reopen, canonical mismatch, provenance/default-client/developer-ID healing, sync reset, project writable/quarantine gates, rollback, and compatible two-handle convergence.
- Removed all slice-3 code and tests: `SessionEndInput`, `ErrSessionAlreadyEnded`, `EnsureAndEndSession`, the end mode, end mutation/rollback, duplicate-end behavior, and end concurrency coverage. Legacy `CreateSession` and `EndSession` are unchanged.
- Renamed the cohesive regular-session test file to `hive-daemon/internal/db/session_ensure_test.go`; `session_lifecycle_test.go` is absent.
- Reset the three slice-3 implementation checkboxes to `- [ ]`; slice-2 implementation rows remain visibly `- [x]`. Parent-owned rows remain byte-for-byte unchanged.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Slice-2 delivery-boundary refactor | `hive-daemon/internal/db/session_ensure_test.go` | SQLite DB | Focused seven-scenario EnsureSession suite passed before removal | Pre-recorded RED/GREEN retained from the earlier slice-2 cycle; no new product behavior was added | Focused suite passed after removing slice 3 | Absent, active, ended-reopen, mismatch, healing, gate/rollback, and two-handle cases passed | Removed end-only API/mode/tests and renamed the cohesive test file; no behavior change to slice 2 |

### Verification

| Command | Result |
| --- | --- |
| `cd hive-daemon && go test ./internal/db -run '^(TestEnsureSession_...)$' -count=1` | Passed before and after the delivery-boundary refactor. |
| `cd hive-daemon && go test -race ./internal/db -run '^(TestEnsureSession_...)$' -count=1` | Passed. |
| `cd hive-daemon && go test ./...` | Passed. |
| `cd hive-daemon && go vet ./...` | Passed. |
| `gofmt -l internal/models/session_write.go internal/db/session.go internal/db/session_ensure_test.go`; `git diff --check` | Passed clean. |
| Slice-3 symbol/file check | Passed: no `SessionEndInput`, `EnsureAndEndSession`, `ErrSessionAlreadyEnded`, or `preserveForEnd`; no `session_lifecycle_test.go`. |

### Remaining work and boundary

- Slice 2 has no end API, end-mode, end rollback, duplicate-end, or end concurrency behavior. Those three slice-3 implementation rows are intentionally unchecked for a later worktree/candidate.
- Deferred parent lifecycle action: review the slice-2 native-accounting receipt, DB/race evidence, exclusions, rollback, and `main <- 1 <- 📍2 <- 3` context before any commit/PR checkpoint.
- Structured status consumed: `changeName=issue-648-lazy-session-materialization`, `applyState=ready`, `artifactStore=openspec`, `actionContext.mode=repo-local`, workspace `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`; no action-context warnings.

## Slice 3 — EnsureAndEndSession end rollback + concurrency extension

**Status:** implementation complete; native review/checkpoint remains pending.
**Boundary:** `main <- 1 <- 2 <- 📍3 <- 4`, starting at user-supplied `795b62de`. Only the approved DB/models/tests and SDD artifacts changed; no adapter, sync, later slice, commit, push, or PR was created.

### Completed implementation tasks

- [x] RED — new `session_end_test.go` failed to compile before production work: `SessionEndInput`, `EnsureAndEndSession`, and `ErrSessionAlreadyEnded` were undefined.
- [x] GREEN — added additive `SessionEndInput` and `ErrSessionAlreadyEnded`; the new transaction-owned end method materializes missing rows, closes active rows with `synced_at = NULL`, and uses private `preserveForEnd` mode so duplicate ends never reopen or overwrite state.
- [x] TRIANGULATE/REFACTOR — exercised missing/active/already-ended/mismatch/gate paths, rejection versus no-op duplicates, summary preservation, abort-trigger rollback for missing and active rows, and two-handle serialized ends. The private mode keeps legacy `EnsureSession`, `CreateSession`, and `EndSession` unchanged.
- Re-read `tasks.md`: all three slice-3 implementation rows are visibly `- [x]`. Parent-owned rows are byte-for-byte unchanged.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Slice 3 end primitive | `hive-daemon/internal/db/session_end_test.go` | SQLite DB | Slice-2 DB/module/race/vet evidence was green at the supplied baseline | `go test ./internal/db -run '^TestEnsureAndEndSession' -count=1` failed with the three missing symbols | Same focused command passed after the minimal transaction/mode implementation | Added duplicate/no-op, typed mismatch, gates, rollback, and two-handle cases; focused and race runs passed | Kept one private two-value mode and formatted; no behavioral refactor needed |

### Verification

| Command | Result |
| --- | --- |
| `cd hive-daemon && go test ./internal/db -run '^TestEnsureAndEndSession' -count=1` | RED compile failure, then GREEN and triangulation pass. |
| `cd hive-daemon && go test -race ./internal/db -run '^TestEnsureAndEndSession' -count=1` | Passed. |
| `cd hive-daemon && go test ./...` | Passed. |
| `cd hive-daemon && go vet ./...` | Passed. |
| `gofmt -l` on the three changed Go files; `git diff --check` | Passed clean. |

### Workload, remaining work, and risks

- Native-accounted diff from `795b62de`: **252 product/test lines** (58 tracked DB/models additions+deletions plus 194 new test lines, counted as additions) + **6 tasks bookkeeping lines** + **35 apply-progress lines** = **293 total changed lines**, below 399.
- Deferred parent lifecycle action (unchanged): `- [ ] Review slice 3's EnsureAndEndSession extension; confirm end rollback/concurrency receipts, separate native-accounting result, rollback, and \`main <- 1 <- 2 <- 📍3 <- 4\` context before merge. <!-- sdd-owner: parent -->`
- Remaining implementation work is deliberately outside slice 3, beginning with slice 4. Risk: transport-level end evidence, HTTP/MCP duplicate mapping, and snapshot sync acknowledgement remain unactivated and unverified by this DB-only unit.
- Structured status consumed: `changeName=issue-648-lazy-session-materialization`; `applyState=ready`; `artifactStore=openspec`; `actionContext.mode=repo-local`; allowed root `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`; warnings none. Skill resolution: `paths-injected`.

## Slice 4 — Snapshot-safe local sync acknowledgement

**Status:** implementation complete; native review/checkpoint pending. **Boundary:** `main <- 1 <- 2 <- 3 <- 📍4 <- 5`; no adapters, later slices, commit, push, or PR.

- [x] RED: new DB tests failed because `AckSessionSnapshot` did not exist; sync stale-ack test failed because the old loop marked progress by ID.
- [x] GREEN: `AckSessionSnapshot` conditionally marks only the exact dirty snapshot, normalizing NULL/empty summaries and SQLite/RFC3339 timestamp representations; the push loop now counts only `true` after network I/O. `MarkSessionSynced` remains unchanged.
- [x] TRIANGULATE/REFACTOR: covered reopen/end/relocation stale snapshots, nullable and non-null timestamp/summary cases, state restoration, blocked in-flight push interleaving, failed push, and false/error progress. No production refactor was needed.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Slice 4 acknowledgement | `internal/db/session_ack_test.go`, `internal/sync/syncer_test.go` | DB + sync integration | `go test ./internal/db`; focused sync passed | Missing DB API and stale-progress assertions failed | Focused DB/sync passed | Snapshot/end/reopen/relocation/restored/failed-push cases passed | gofmt; no extra refactor |

### Verification and workload

- Passed: focused DB/sync tests; `go test -race ./internal/db ./internal/sync`; `cd hive-daemon && go test ./...`; `go vet ./...`; `gofmt -l`; `git diff --check`.
- Persisted tasks: slice-4 RED, GREEN, and TRIANGULATE/REFACTOR rows are visibly `- [x]`; parent-owned rows remain unchanged.
- Native accounting is pending final receipt; current code/tests/tasks are 320 additions+deletions before this evidence, so this bounded slice remains below 399 with this minimal record. Parent must review stale/failed-push evidence and exact final count before lifecycle checkpoint.
- No design deviation. Residual risk: unknown timestamp encodings fail closed and stay dirty for retry; no SQLite transaction spans network I/O.
    - Structured status: `changeName=issue-648-lazy-session-materialization`, `applyState=ready`, `artifactStore=openspec`, `actionContext.mode=repo-local`, workspace `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`, warnings none; skill resolution `paths-injected`.

## Slice 5a — standalone session-init group foundation

**Status:** complete for the assigned foundation only. Boundary: `main <- 1 <- 2 <- 3 <- 4 <- 📍5a <- 5b`; no HTTP/MCP wiring, commit, push, or PR.

- [x] RED/GREEN/TRIANGULATE: `group_test.go` first failed to compile because `Key`/`NewGroup` were absent, then proved one canonical-project/exact-ID invocation, detached snapshots/errors, cleanup/retry, key isolation, alias-key sharing, waiter cancellation, and panic recovery.
- [x] Persisted tasks: only the three new **Slice 5a** implementation rows at `tasks.md:71-73` are checked. Original slice-5 adapter rows remain unchecked.

### TDD Cycle Evidence

| Task | Test file | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| 5a group | `hive-daemon/internal/sessioninit/group_test.go` | `go test` failed: `Key`/`NewGroup` undefined | focused group test passed | controlled same-key/error/panic/retry/key-isolation/cancellation cases passed under `-race -count=10`; gofmt clean |

### Verification and boundary

- Passed: focused `go test ./internal/sessioninit -run '^TestGroupDo' -count=1 -v`; `go test -race ./internal/sessioninit -run '^TestGroupDo' -count=10`; `cd hive-daemon && go test ./... && go vet ./...`; `gofmt -l` and `git diff --check`.
- Files: `hive-daemon/internal/sessioninit/{group.go,group_test.go}` plus this evidence and task record. The mutex covers only the flight map; SQL/initializer work and waiter selects occur unlocked. Returned session timestamps are copied.
- Native accounting: 286 Go + 6 task + 27 evidence additions/deletions = **319 lines**, below 399. Rollback: remove the standalone package and its three 5a task rows; adapters are untouched.
- Deviation: slice 5 was split by parent instruction; canonical aliases are represented by the same already-validated `Key`, not adapter validation. No design deviation within 5a.
- Remaining adapter work (intentionally unchecked):
  - `- [ ] **RED:** Add channel-controlled group tests for one invocation, shared detached snapshots/errors, cleanup/retry, key isolation, alias sharing, cancellation, panic cleanup, and snapshot isolation; add real HTTP/MCP blocking-store wiring tests proving invalid or blocked callers never join. <!-- sdd-owner: implementation -->`
  - `- [ ] **GREEN:** Implement the mutex/map flight group without holding its lock during SQL/waiting, give each HTTP/MCP server lifetime-owned state, and route only independently validated start calls through \`Group.Do\` and \`EnsureSession\`. <!-- sdd-owner: implementation -->`
  - `- [ ] **TRIANGULATE/REFACTOR:** Cover absent/active/ended starts, typed mismatch mapping, first-writer provenance, migration exclusion, and capture non-use; format and run sessioninit/HTTP/MCP/race/module/vet checks. <!-- sdd-owner: implementation -->`
- Structured status consumed: `changeName=issue-648-lazy-session-materialization`; `applyState=ready` (parent); `artifactStore=openspec`; `actionContext.mode=repo-local`; workspace `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`; allowed roots supplied; warnings none. Skill resolution: `paths-injected`.

## Slice 5b — HTTP/MCP standalone-start activation

**Status:** complete for the assigned adapter-only slice. Boundary: `main <- 1 <- 2 <- 3 <- 4 <- 5a <- 📍5b`; no end, prompt, passive, memory/summary, OpenCode, commit, push, or PR work.

- [x] RED: blocking HTTP and MCP start tests failed because both adapters bypassed `EnsureSession`.
- [x] GREEN: each server now owns one `sessioninit.Group`; independently gate- and project-validated starts use the validated canonical project plus exact ID key and invoke `EnsureSession` once. MCP persists `client: mcp`; HTTP retains its supplied client.
- [x] TRIANGULATE/REFACTOR: the real adapter tests hold a valid leader, prove the concurrent valid follower shares it, and prove empty-ID and migration-blocked callers neither enter nor block the flight. Existing start suites retain ID, resolution, mismatch, and transport-contract coverage.

### TDD Cycle Evidence

| Task | Test file | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| HTTP/MCP group wiring | `internal/httpapi/sessions_test.go`, `internal/mcp/migration_gate_test.go` | both focused tests failed: start bypassed `EnsureSession` | passed after lifetime group plus `EnsureSession` wiring | focused start suites and race (`-count=5`) passed; gofmt clean |

### Verification

- Safety net before edits: `go test ./internal/httpapi ./internal/mcp` passed.
- Passed: focused RED/GREEN HTTP and MCP commands; `go test ./internal/httpapi -run '^TestPostSessions_' -count=1`; `go test ./internal/mcp -run '^TestMemSessionStart_' -count=1 -v`; `go test -race ./internal/httpapi ./internal/mcp -run '^(TestPostSessions_ConcurrentValidStartsShareOneEnsureAndExcludeInvalidOrBlocked|TestMemSessionStart_ConcurrentValidCallsShareOneEnsureAndExcludeInvalidOrBlocked)$' -count=5`; `go test ./...`; `go vet ./...`; `gofmt -l`; `git diff --check`.

### Files / accounting / remaining

- Changed: `hive-daemon/internal/{httpapi/server.go,httpapi/sessions_test.go,mcp/server.go,mcp/tools.go,mcp/server_test.go,mcp/migration_gate_test.go,mcp/tools_test.go}`, plus this record and `tasks.md`.
- Persisted tasks: the three original slice-5 implementation rows are visibly `- [x]`; 5a rows remain complete. Parent review rows are unchanged.
- Native accounting from base `342ce781`: **265** additions+deletions (231 Go/test + 28 apply-progress + 6 tasks); hard cap `<399` satisfied.
- Remaining: all slice-6+ implementation rows and all parent-owned review/lifecycle rows. Deferred parent action: review slice 5 start coalescing, validation exclusion, race evidence, and native accounting before lifecycle checkpoint.
- Risks: adapter-local flights are deliberately process/server scoped; cross-process convergence remains the transactional store's responsibility. `Group` never covers excluded capture/end paths.
- Status consumed: parent `issue-648-lazy-session-materialization`, `apply=ready`, repo-local allowed root `/home/andres/Desarrollo/Proyectos/jarvis-dev-issue-648`, no action-context warnings; skill resolution `paths-injected`.

## Slice 6a — HTTP atomic end adapter

**Status:** complete for HTTP only. The former slice 6 is now three delivery boundaries: 6a HTTP adapter (this record), 6b MCP adapter, and 6c native hook caller. No MCP, hook, prompt, passive, memory, OpenCode, commit, push, or PR work occurred.

- **RED:** `TestPostSessionsEnd_MissingSessionMaterializesAtomically` expected 200 but the legacy endpoint returned 404 for the absent session.
- **GREEN:** the HTTP handler decodes bounded project evidence, independently runs `ValidateWriteProject`, then calls `EnsureAndEndSession` with `RejectAlreadyEnded: false`. Missing and active sessions end atomically; duplicate ends preserve the stored summary/lifecycle.
- **TRIANGULATE/REFACTOR:** temporary SQLite tests cover missing, active, duplicate, typed mismatch/identity/unknown evidence errors, empty ID, migration and project-block HTTP mappings, and abort-trigger rollback. `gofmt` retained one handler path and focused/race HTTP tests passed.
- **Verification:** `cd hive-daemon && go test ./internal/httpapi -run '^TestPostSessionsEnd_' -count=1`; `go test -race ./internal/httpapi -run '^TestPostSessionsEnd_' -count=1`; `go test ./...`; `go vet ./...`; targeted `gofmt -l`; and `git diff --check` all passed.
- **Accounting / rollback:** **280 additions+deletions** across the five permitted files, including 153 new temporary-DB test lines and 32 task/evidence changes; below the 399-line cap. Parent review remains required. Revert only the HTTP handler/interface/mock/tests and this 6a task/evidence record; do not modify persisted ended rows. 6b and 6c remain intentionally unchecked.

## Slice 6b — MCP atomic end adapter

**Status:** implementation complete for MCP only. No HTTP, hook, prompt, passive observation, memory/summary, OpenCode, commit, push, or PR work occurred.

- **RED:** `go test ./internal/mcp -run '^TestMemSessionEnd_(MissingSessionMaterializesAtomically|UsesAtomicStoreAndRejectsDuplicate|PreservesProjectValidationAndRollback)$' -count=1` failed to compile because the MCP mock and store interface had no `EnsureAndEndSession` adapter seam.
- **GREEN:** `mem_session_end` now rejects blank IDs and absent project evidence, canonically resolves project/directory evidence before mutation, and atomically calls `EnsureAndEndSession` with MCP attribution and `RejectAlreadyEnded: true`. Duplicate ends remain MCP tool errors wrapping `ErrSessionAlreadyEnded`; successful missing/active ends clear activity only after the transaction succeeds.
- **TRIANGULATE/REFACTOR:** temporary SQLite coverage proves missing and active materialization, duplicate-summary preservation, abort-trigger rollback, and canonical directory-evidence mismatch without materialization. Mock coverage pins the exact atomic input, duplicate rejection, no-project and unresolved-directory validation rejection, unknown-project rejection, and typed store-mismatch mapping. The existing migration-gate suite retains blocked MCP mapping coverage. `gofmt` kept one handler path.
- **Verification:** focused MCP end/migration test command and its `-race` variant passed; `cd hive-daemon && go test ./...`; `go vet ./...`; targeted `gofmt -l`; and `git diff --check` passed.
- **Accounting / rollback:** against `e6808b71`, **356 additions+deletions** (149 tracked MCP code/test + 195 new MCP end-materialization test + 12 task/evidence) is below 399. Revert only the MCP interface, schema/handler, MCP mocks/tests, and this 6b task/evidence record. Do not modify persisted ended rows. Slice 6c remains unchecked.

## Slice 6c — Native hook end caller compatibility

**Status:** implementation complete for the native hook caller only. No daemon HTTP/MCP, prompt, passive observation, memory, OpenCode, commit, push, or PR work occurred.

- **RED:** focused hook tests failed to compile because `PostSessionEnd` accepted only a session ID while the new receiver contract supplied canonical project and directory evidence.
- **GREEN:** `RunSessionStop` now derives `directory` from `directory`/`cwd`, derives the canonical project with `project.DetectProject`, and forwards both with the session ID. `PostSessionEnd` path-escapes the ID and sends the exact summary/project/directory/hook-client body.
- **TRIANGULATE/REFACTOR:** receiver coverage pins escaped IDs, POST/content-type, exact JSON, 404 and transport fail-open behavior; event coverage pins canonical directory forwarding and the `cwd` fallback. `end-evidence-compatibility.md` records the internal signature and legacy-receiver implications.
- **Verification:** focused hook end tests and their `-race` variant passed; `cd jarvis-cli && go test ./... && go vet ./...` passed; targeted `gofmt -l` and `git diff --check` passed.
- **Accounting / rollback:** **143 additions+deletions** against `7d30847f` (114 Go/test + 4 task + 12 evidence + 13 compatibility-document lines), below the 399-line cap. Revert only the hook caller/client/tests and compatibility/task/evidence records; do not alter daemon adapters or persisted session state.

All 6a, 6b, and 6c implementation rows are now complete. The parent-owned slice-6 review/checkpoint remains unchecked.

## Slice 7a — DB prompt foundation

**Status:** DB-only foundation retained after splitting the 394-line slice-7 candidate. HTTP/MCP/hook source and mocks were restored to HEAD, and their untracked prompt-materialization tests were removed. No transport activation is claimed by this slice.

- **Foundation:** `PromptWrite` and `SavePromptWithSession` share one SQLite transaction with session materialization/reopen and prompt insertion. `SavePrompt` and `SavePromptForSession` remain unchanged.
- **DB evidence:** `prompt_write_test.go` covers absent and ended materialization, prompt-insert failure after new-session creation and after an ended-session reopen, persisted mismatch/gate rollback state, manual/empty preservation, two-handle captures, and existing-session start, sync identity, provenance, and first-writer attribution preservation.
- **Observed verification:** focused and race `TestSavePromptWithSession` runs, the full hive-daemon suite, `go vet ./...`, targeted formatting, and `git diff --check` passed after the deferred 7b tests were removed.
- **Accounting:** 97 tracked additions/deletions plus the 253-line DB test = **350 native lines**, below 399. Do not treat the former 394-line candidate or its transport evidence as slice-7a evidence.

## Slice 7b — HTTP/MCP/hook prompt activation

**Status:** implementation complete; aggregate slice 7 is complete. Parent review/checkpoint remains pending.

- **RED:** HTTP/MCP explicit-session tests failed before activation: absent sessions were not materialized, ended sessions stayed closed, and atomic capture mocks were not called. Hook receiver test observed no client attribution.
- **GREEN:** Explicit non-empty HTTP/MCP captures now call `SavePromptWithSession` with validated canonical project/directory evidence; HTTP uses its supplied client or `http`, MCP uses `mcp`, and the hook sends `client: "hook"`. Empty/manual paths retain legacy wrappers.
- **TRIANGULATE/REFACTOR:** HTTP/MCP tests cover absent/ended materialization, store-returned typed validation mapping, validation-before-store, and independence from a held start flight. Focused and `-race` adapter/hook tests, both module suites, both vets, gofmt, and `git diff --check` passed.
- **Accounting / rollback:** 113 tracked additions/deletions plus 234 lines across the two untracked adapter tests = **347 native lines**, below 399. Revert only transport interfaces/handlers, hook payload attribution, adapter tests, and this 7b record; retain the 7a DB foundation. No commit, push, or PR was created.

## Slice 8 — Atomic passive observations

**Status:** implementation complete; parent review/checkpoint remains pending.

- **RED:** focused DB tests failed to compile because `PassiveObservationWrite` and `SavePassiveObservationWithSession` were absent; HTTP tests then proved explicit IDs still used the legacy raw writer.
- **GREEN:** attributed captures now materialize/reopen through one transaction; explicit HTTP IDs validate canonical evidence and map typed validation/gates, default to `unknown`, and hooks send `client: "hook"`. Empty/NULL IDs retain the raw writer.
- **TRIANGULATE/REFACTOR:** DB coverage proves absent/ended identity preservation, mismatch/gate, session-trigger rollback, observation-trigger rollback after both session creation and ended-session reopen, and two-handle independence; HTTP covers atomic routing/mappings and empty/NULL fallback. `gofmt` retained the focused paths.
- **Verification:** focused DB/HTTP and `-race` runs passed; `cd hive-daemon && go test ./... && go vet ./...` passed; focused/full hook tests and `cd jarvis-cli && go vet ./...` passed; `git diff --check` and targeted `gofmt -l` passed.
- **Accounting / rollback:** **384 native lines** (368 Go/test diff + 6 task checkbox lines + 10 evidence lines) against `609bce9d`, below 399. Revert the passive write/input, HTTP activation, hook attribution, tests, and this evidence together; raw empty/NULL behavior is retained. No commit, push, or PR was created.

## Slice 9 — Atomic MCP memory captures

**Status:** implementation plus independent-verification extension complete; parent review/checkpoint remains pending.

- **RED:** the original production RED is retained: the focused DB test failed to compile because `SaveMemoryWithSession` was undefined; old summary tests then failed because ended/absent sessions now materialize instead of rejecting. The independent extension is test-only against already-present behavior, so it has no honest new production RED.
- **GREEN:** explicit MCP `mem_save` and `mem_session_summary` use `SaveMemoryWithSession` with `client: "mcp"`; empty IDs retain manual fallback and summaries do not close sessions. The new downstream rollback and held-flight tests pass without a production change.
- **TRIANGULATE/REFACTOR:** the ended-session table independently proves that a failing memory insert trigger, real prompt-link foreign-key rejection, or failing mutation-journal trigger rolls back the attempted reopen and preserves the prior end timestamp, summary, sync identity, directory, dev ID, client, and start timestamp. It also confirms no memory, link, or journal remains. This deliberately does not call the link failure a trigger.
- **MCP flight coverage:** `TestMCPExplicitMemoryCaptureDoesNotJoinStartFlight` now has `mem_save` and `mem_session_summary` subtests. The exact focused and `-race` regex below names that test, so both new MCP cases are executed.
- **Verification:** `cd hive-daemon && go test ./internal/db ./internal/mcp -run '^(TestSaveMemoryWithSession_(MaterializesReopensAndRollsBack|RollsBackSessionMemoryLinkAndJournal|EndedSessionReopenRollsBackDownstreamFailure|RejectsMismatchAndBlockedProject)|TestMCP(ExplicitMemoryCapturesMaterializeAndReopen|ExplicitMemoryCaptureDoesNotJoinStartFlight))$' -count=1 -v` passed; the same exact selection with `-race -count=5 -v` passed. `go test ./...`, `go vet ./...`, targeted `gofmt -l`, and `git diff --check` passed.
- **Accounting / rollback:** against `8c1ed37d`, **295 Go/test lines** (202 tracked + 93-line untracked MCP test) plus **17 SDD lines** (11 evidence + 6 task replacement lines) = **312 native-accounting lines**. The tracked base diff is 189 additions + 30 deletions; adding the untracked test yields the same total. This is below 399. Revert explicit-session DB/store/handler wiring and tests together. No OpenCode, commit, push, or PR.

## Slice 10 — OpenCode created lifecycle and prompt attribution

**Status:** embedded-template implementation and automated evidence, including independent-verifier remediation, complete; deletion is absent. The manual lifecycle checklist now records the creation/prompt subset, its unrun live-runtime status, and rationale; the TRIANGULATE/REFACTOR checkbox is complete under the design's explicit skip/runtime-record allowance.

- **RED:** Original source-execution RED found no created lifecycle contract. Verifier remediation added multi-part text evidence, which failed with `content = "capture\nthis prompt"` rather than preserving per-part whitespace; the synchronous-fetch setup variant failed before the local catch with no observed start.
- **GREEN:** the template resolves only event/environment session evidence (no PID fallback), sends bounded fail-open created registration, catches synchronous fetch setup errors, and clears a process-local flight in `finally`. Prompt joining now preserves individual text-part whitespace and trims only the final content; prompt client attribution remains independent.
- **TRIANGULATE:** source-of-truth Node execution proves one immediate duplicate is absent during a negative wait, observes first-request cancellation before the second observed start, and asserts exactly two starts; no tight timing threshold is used. It also proves multipart prompt joining, prompt/start independence, and synchronous setup fail-open. Installer coverage pins byte-for-byte installation; Node skips under `-short` or when unavailable.
- **Verification:** focused/race/full CLI tests, vet, gofmt, diff check, and `node --experimental-strip-types --check embed/hooks/opencode/hive.ts` passed. `tsc` is unavailable and this module has no TypeScript project configuration, so no semantic TypeScript type check was available without adding tooling.
- **Rollback:** revert `embed/hooks/opencode/hive.ts` and the two agent test changes together, then regenerate through the existing installer; do not edit an installed user plugin.
- **Accounting / scope:** final permitted-surface diff is 384 additions+deletions, below 399. No deletion endpoint/event, resident core, autostart, idempotency key, inactivity close, commit, push, or PR was added.

## Slice 11 — OpenCode deleted-session delivery

**Status:** implementation and automated evidence complete; the manual checklist now records the full procedure, automated observations, and unrun live-runtime status/rationale, so its TRIANGULATE/REFACTOR task is complete under the design's explicit skip/runtime-record allowance.

- **RED:** the new source-of-truth Node loopback test timed out waiting for `/sessions/{id}/end` before deletion dispatch existed.
- **GREEN:** generic `session.deleted` reads OpenCode's `properties.info`, requires an event ID, URL-escapes it, and POSTs optional summary plus canonical project/directory evidence and `client: "opencode"` with a one-second fail-open timeout.
- **Endpoint correction (STRICT TDD):** changed the source-derived created-request expectation to `POST /sessions`; it failed before the template correction by timing out waiting for the first created request. Changing only the template's obsolete created endpoint to `/sessions` made the same focused test pass and aligns the source with design §6.
- **TRIANGULATE:** loopback execution proves the nested event shape, encoded reserved ID, exact body, missing-ID no-op, synchronous-fetch failure, timeout/retry without permanent dedupe, and independence from a pending creation. Existing source/install and creation/prompt tests remain green; no inactivity close was added.
- **Verification:** focused and race agent tests, `go test ./...`, `go vet ./...`, Node syntax, gofmt, and diff check passed.
- **Accounting / rollback:** the final inclusive receipt below accounts for the endpoint correction as well as the pre-existing slice-11 work. Revert the template, lifecycle test, checklist, and slice-11 records together. Never edit an installed plugin.

## Slices 10/11 — OpenCode manual lifecycle checklist

**Status:** complete as an artifact; live OpenCode execution is explicitly **NOT RUN**. Design §6 and the slice-11 task allow a temporary installer/loopback procedure with actual runtime and skipped steps recorded. This work records that procedure and the current source-derived Node observations without invoking OpenCode, external models, authentication, or user configuration.

- **Automated evidence recorded:** created, deleted, and prompt requests; exact method/path/body/client/evidence; encoded deletion ID; created/deleted timeout cancellation beyond one second; synchronous fetch setup failures for created/deleted; missing deleted-ID no-op; created-flight coalescing; and deletion independence from a pending creation.
- **Not claimed:** live `opencode 1.18.29` envelope compatibility, 400/423/500 or generic non-OK tolerance, prompt timeout/synchronous failure, comprehensive retry absence, unhandled-failure absence, process/daemon absence, or post-failure OpenCode usability. The checklist gives the disposable procedure and rationale for every NOT RUN item.
- **Created endpoint compliance:** source/test now use created `POST /sessions`, matching design §6. The checklist records the focused RED→GREEN correction; no obsolete created-route claim remains.
- **Accounting:** final inclusive accounting is **235 changed lines**: template 30 additions + 1 deletion = 31; lifecycle test 94 + 4 = 98; apply-progress 21 + 1 = 22; tasks 4 + 4 = 8; and the 76-line untracked checklist. This is the exact `git diff --numstat` total plus `wc -l` for the untracked checklist, including the endpoint correction, and is below 399 (**164 lines headroom**).

## Final verifier remediation — HTTP prompt and MCP memory error mapping

- **RED:** `TestPostPrompts_ExplicitSessionDefaultsOmittedClientToUnknown` observed `http`; `TestMCPExplicitMemoryCaptureMapsTransactionalErrors` reached the transactional store, then observed non-JSON generic errors for `project_session_mismatch` in both `mem_save` and `mem_session_summary`.
- **GREEN/TRIANGULATE:** Explicit HTTP prompt capture now defaults only omitted clients to `unknown`. Both MCP handlers pass store errors through `toolValidationError`, preserving structured `project_session_mismatch`; table cases retain generic `context canceled` behavior. Existing explicit caller/MCP and manual-path focused cases remain covered.
- **Verification:** focused HTTP/MCP tests, focused `-race`, full `hive-daemon` suite, `go vet ./...`, `gofmt -l`, and `git diff --check` passed. No OpenCode changes, commit, push, PR, or review.

## Final verifier remediation — OpenCode lifecycle contract

- **RED:** tightening the generic event tests to `event.properties.info.id`, exact evidence-only bodies, distinct same-ID evidence, and prompt environment/PID regression behavior timed out because created used the wrong envelope and deleted missed the nested ID. The independent synchronous-fetch correction added resolved evidence plus numeric-text parsing; its focused run timed out waiting for the numeric prompt because text was narrowed to strings.
- **GREEN:** one lifecycle resolver now accepts documented nested IDs, defensive lifecycle property spellings, then explicit session environment fallback; it never accepts an arbitrary event ID or prompt PID fallback. Shared env-first evidence requires an ID plus project or directory. Created sends only ID/client/nonempty evidence; deleted sends only client/nonempty evidence. Prompt parsing now preserves public/master `filter`/`map`/`join` coercion: numeric text stringifies, and non-array parts throws into the advisory caught handler.
- **TRIANGULATE/REFACTOR:** created flights use exact JSON evidence tuples, install before their fetch microtask, return the shared pending promise, clean up their own entry, and retry only on a later event. The executable synchronous-failure case now resolves documented lifecycle evidence, keeps throwing `fetch` through that microtask, observes one attempted fetch, confirms immediate callback return, and asserts no unhandled rejection. Generic event delivery is fire-and-forget; deletion/prompt requests stay independent. Prompt resolution restores base environment precedence, PID/cwd fallback, and content extraction, adding only `client: "opencode"`.
- **Verification:** focused and `-race` Node-backed creation/deletion agent tests, `cd jarvis-cli && go test ./...`, `go vet ./...`, `node --experimental-strip-types --check embed/hooks/opencode/hive.ts`, targeted `gofmt -l`, `git diff --check`, and `git diff --check public/master` all passed. The mandated committed-only `git diff --check public/master..HEAD` still reports the pre-existing committed trailing whitespace, which this uncommitted repair removes from the worktree.
- **Accounting / residual gap:** current permitted-surface worktree accounting against `d310ac22` is **342 additions+deletions** (215 template, 100 lifecycle test, 10 apply-progress, 16 checklist, 1 task), leaving **57 lines** below 399. Live OpenCode runtime, non-OK lifecycle responses, and exhaustive unhandled-rejection/process assertions remain explicitly NOT RUN in the checklist; source-derived tests cover the documented callback and loopback contract only.
