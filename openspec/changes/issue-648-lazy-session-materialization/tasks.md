# Implementation Tasks: Lazy Session Materialization

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 2,480–3,470 total native-accounting lines; 120–390 per slice |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 → PR 5 → PR 6 → PR 7 → PR 8 → PR 9 → PR 10 → PR 11 |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

## Execution Rules

- The maintainer reset former slice 2 after native accounting measured **463 total lines** (**387 product/tests + SDD evidence**) over the 399-line limit. Its valid uncommitted implementation is now two delivery units, not discarded work.
- Every clean slice must remain **below 399 total native-accounting lines**, including product code, tests, docs/interface churn, `tasks.md` applicability changes, and apply evidence. Measure before delivery; do not compress tests, evidence, comments, or behavior.
- Strict TDD is RED → GREEN → TRIANGULATE → REFACTOR. Checkmarks record implementation evidence only; they do not claim a commit, review, merge, push, or PR checkpoint.
- Each slice is a stacked-to-main work unit. It starts at its immediate predecessor, retains tests with the behavior, and has the listed rollback boundary. Parent delivery gates remain separately unchecked until review and native accounting are complete.
- Run commands only from the named module; do not run builds or a root workspace command. Use `gofmt` for edited Go files and report PostgreSQL/Node skips explicitly.

## Implementation Work Units

### 1. API regular-session lifecycle acceptance — 180–270 native lines

**Dependency / finish:** `main` → regular Hive API upsert accepts ended-to-open lifecycle state and exposes it through the existing pull watermark; no local producer activates. **Paths:** `hive-api/internal/repository/postgres_session.go`, `hive-api/internal/repository/postgres_session_lifecycle_test.go`, `hive-api/internal/service/sync_test.go`. **Rollback:** revert the API clause and its tests; retain it once reopen producers land.

- [x] **RED:** Add repository/service tests for nullable `ended_at`, summary/watermark, identity/provenance preservation, sentinel/relocation protection, and an explicitly skippable PostgreSQL pull case; record the failing lifecycle assertion. <!-- sdd-owner: implementation -->
- [x] **GREEN:** Separate regular sync lifecycle acceptance from correction-only create handling in `hive-api/internal/repository/postgres_session.go`, applying nullable lifecycle fields without changing manual/legacy branches. <!-- sdd-owner: implementation -->
- [x] **TRIANGULATE/REFACTOR:** Cover repeated-open and watermark-pull variants; clarify the lifecycle clause/comment, format changed Go files, and record focused/module test and vet evidence. <!-- sdd-owner: implementation -->

### 2. EnsureSession create/replay/reopen foundation — 240–300 native lines

**Dependency / finish:** slice 1 → additive `SessionInput` and transactional `EnsureSession` create, active replay, and ended reopen foundation with no end API or adapter activation. **Paths:** `hive-daemon/internal/models/session_write.go`, `hive-daemon/internal/db/session.go`, `hive-daemon/internal/db/session_ensure_test.go`. **Excluded:** `SessionEndInput`, `EnsureAndEndSession`, end state mutation/rollback, and end concurrency. **Rollback:** revert only this unused regular foundation and its tests before consumers land.

- [x] **RED:** Add table-driven SQLite tests for absent/active/ended regular ensure, canonical mismatch, directory/dev/client variants, default client and developer-ID healing, immutable start/sync identity, sentinels, target/existing project gates, and compatible first-write convergence; record the missing `SessionInput`/`EnsureSession` failure. <!-- sdd-owner: implementation -->
- [x] **GREEN:** Add `SessionInput`, transaction-owned `EnsureSession`, and the private transaction-scoped reader/materializer in `hive-daemon/internal/db/session.go`; register identity first, enforce writable/quarantine checks, return typed canonical mismatch, and create/replay/reopen without changing legacy methods. <!-- sdd-owner: implementation -->
- [x] **TRIANGULATE/REFACTOR:** Prove retry clock/provenance preservation, reopen sync reset, blocked/mismatch rollback, and one-/two-handle convergence without sleeps; retain only cohesive private helpers, format, and record DB/race/module/vet evidence. <!-- sdd-owner: implementation -->

### 3. EnsureAndEndSession end rollback + concurrency extension — 160–230 native lines

**Dependency / finish:** slice 2 → additive `SessionEndInput`, `ErrSessionAlreadyEnded`, and atomic ensure-and-end behavior; no HTTP/MCP/hook adapter activation. **Paths:** `hive-daemon/internal/models/session_write.go`, `hive-daemon/internal/db/session.go`, `hive-daemon/internal/db/session_lifecycle_test.go`. **Excluded:** transport semantics and any standalone coalescer. **Rollback:** revert this extension and its end tests while retaining slice-2 regular ensure behavior.

- [ ] **RED:** Extend lifecycle tests with missing/active/already-ended/mismatch end cases, duplicate-summary preservation, aborting-trigger failure, and serialized one-/two-handle end outcomes; record the missing `EnsureAndEndSession`/`ErrSessionAlreadyEnded` failure. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add `SessionEndInput`, `ErrSessionAlreadyEnded`, and `EnsureAndEndSession` using the transaction-owned preserve-for-end path: materialize then close missing/active compatible rows, never reopen a duplicate end, and roll back every lifecycle mutation on failure. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Prove a failed missing-session end leaves neither open nor ended row, MCP-style duplicate rejection preserves summary/state, HTTP-style duplicate no-op remains representable, and concurrent ends serialize; format and record DB/race/module/vet evidence. <!-- sdd-owner: implementation -->

### 4. Snapshot-safe local sync acknowledgement — 170–260 native lines

**Dependency / finish:** slice 3 → successful session pushes acknowledge only their exact local snapshot; public lifecycle producers remain unchanged. **Paths:** `hive-daemon/internal/db/session.go`, focused acknowledgement tests in `hive-daemon/internal/db/`, `hive-daemon/internal/sync/syncer.go`, and sync mocks/tests in `hive-daemon/internal/sync/`. **Rollback:** revert acknowledgement contract and loop migration together; do not clear pending rows.

- [ ] **RED:** Add DB/sync interleaving tests holding a push after its snapshot, then committing reopen, end, and relocation; assert stale acknowledgement is false, dirty state remains, progress is unchanged, failed push stays dirty, and nullable timestamps/summaries compare safely. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Implement `AckSessionSnapshot(ctx, sent, at)` as a null-safe conditional update and migrate only the production session push loop to count progress on a true acknowledgement after network I/O. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Cover end-after-open snapshot and changed-then-restored state, keep comparisons fail-closed, then format and run focused DB/sync, race, module, and vet checks. <!-- sdd-owner: implementation -->

### 5. Coalesced standalone registration — 300–390 native lines

**Dependency / finish:** slice 4 → validated HTTP/MCP starts share one in-flight `EnsureSession` result per canonical project/exact ID; captures and ends do not join the group. **Paths:** `hive-daemon/internal/sessioninit/group.go`, `group_test.go`, `hive-daemon/internal/httpapi/server.go` plus start interfaces/mocks/tests, and `hive-daemon/internal/mcp/{server.go,tools.go}` plus start interfaces/mocks/tests. **Rollback:** revert start activation and group together.

- [ ] **RED:** Add channel-controlled group tests for one invocation, shared detached snapshots/errors, cleanup/retry, key isolation, alias sharing, cancellation, panic cleanup, and snapshot isolation; add real HTTP/MCP blocking-store wiring tests proving invalid or blocked callers never join. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Implement the mutex/map flight group without holding its lock during SQL/waiting, give each HTTP/MCP server lifetime-owned state, and route only independently validated start calls through `Group.Do` and `EnsureSession`. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Cover absent/active/ended starts, typed mismatch mapping, first-writer provenance, migration exclusion, and capture non-use; format and run sessioninit/HTTP/MCP/race/module/vet checks. <!-- sdd-owner: implementation -->

### 6. Atomic lifecycle end and shipped callers — 250–350 native lines

**Dependency / finish:** slice 5 → HTTP/MCP ends validate evidence and atomically materialize/close missing sessions; native hook end calls carry encoded ID, evidence, and `client: hook`. **Paths:** HTTP/MCP end handlers/interfaces/mocks/tests under `hive-daemon/internal/{httpapi,mcp}/`, `jarvis-cli/internal/hook/client.go` and tests, `openspec/changes/issue-648-lazy-session-materialization/end-evidence-compatibility.md`. **Rollback:** revert adapters and hook caller together, never undo persisted closures/summaries.

- [ ] **RED:** Add temporary-DB HTTP/MCP tests for missing, active, duplicate, mismatch, invalid/unresolved/no evidence, blocked, and empty-ID outcomes plus trigger rollback through both adapters; add hook receiver assertions for encoded path and exact evidence/client. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add bounded end evidence decoding/validation, call `EnsureAndEndSession` with HTTP duplicate no-op and MCP duplicate rejection, preserve typed error mapping/statuses, and update the native hook caller plus compatibility document. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Cover aliases, directory corroboration, recovery/migration, and concurrent end serialization; format and run hive-daemon HTTP/MCP plus jarvis-cli hook/module/vet checks. <!-- sdd-owner: implementation -->

### 7. Atomic prompts — 280–375 native lines

**Dependency / finish:** slice 6 → explicit HTTP/MCP prompt capture ensures/reopens in its own transaction while manual/empty behavior stays unchanged. **Paths:** `hive-daemon/internal/models/session_write.go`, `hive-daemon/internal/db/prompt.go` and prompt tests, HTTP/MCP prompt paths/tests, `jarvis-cli/internal/hook/client.go` and tests. **Rollback:** revert explicit prompt activation only.

- [ ] **RED:** Add absent/ended/mismatch, insert/update/prompt failure rollback, caller attribution, manual/empty, gate, and independent concurrent prompt tests. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add `PromptWrite` and transactional `SavePromptWithSession`; activate it only for explicit HTTP/MCP requests and add hook client attribution while retaining wrappers/fallbacks. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Cover canonical/provenance variants and failed capture after reopen; format and run focused DB/HTTP/MCP/hook, race, module, and vet checks. <!-- sdd-owner: implementation -->

### 8. Atomic passive observations — 220–330 native lines

**Dependency / finish:** slice 7 → explicit passive observations materialize/reopen atomically while empty/NULL attribution stays unchanged. **Paths:** `hive-daemon/internal/models/session_write.go`, `hive-daemon/internal/db/passive_observation.go` and tests, HTTP passive paths/tests, `jarvis-cli/internal/hook/client.go` and tests. **Rollback:** revert explicit-ID passive behavior only.

- [ ] **RED:** Add explicit-ID absent/ended/mismatch/gate/rollback/concurrency tests and regressions proving empty IDs retain raw empty/NULL attribution without a regular session. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add `PassiveObservationWrite` and `SavePassiveObservationWithSession`; route only explicit HTTP IDs through it with unknown default client and hook attribution. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Cover directory/alias and identity preservation variants; format and run focused DB/HTTP/hook, race, module, and vet checks. <!-- sdd-owner: implementation -->

### 9. Atomic observations and summaries — 280–385 native lines

**Dependency / finish:** slice 8 → explicit MCP save/summary materializes in the existing memory transaction; manual and project-unknown recovery stay intact. **Paths:** `hive-daemon/internal/db/memory.go` and tests, MCP memory interfaces/schema/mocks/tests under `hive-daemon/internal/mcp/`, reuse of `hive-daemon/internal/models/session_write.go` only if needed. **Rollback:** revert explicit MCP activation only.

- [ ] **RED:** Extend memory transaction fixtures for session/memory/link/journal abort triggers; test explicit save/summary absent/ended/mismatch rollback, independent capture while a start flight is held, and manual/project_unknown/recovery/migration/quarantine regressions. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add `SaveMemoryWithSession` through the existing preparation callback and convert only explicit MCP save/summary, retaining validation, limits, manual fallback, and non-lifecycle summary-memory behavior. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Cover capture-after-ended reopen and failed-summary preservation; format and run focused DB/MCP, race, module, and vet checks. <!-- sdd-owner: implementation -->

### 10. OpenCode coalesced creation and prompt attribution — 280–380 native lines

**Dependency / finish:** slice 9 → embedded source sends fail-open coalesced `session.created` registration and prompt client attribution; deletion remains absent until slice 11. **Paths:** `jarvis-cli/embed/hooks/opencode/hive.ts`, source-derived/installer tests under `jarvis-cli/internal/agent/`, `openspec/changes/issue-648-lazy-session-materialization/manual-opencode-lifecycle-checklist.md`. **Rollback:** revert source template behavior through installer regeneration only.

- [ ] **RED:** Extend source-derived executable and installer tests for actual lifecycle ID/evidence, no PID fallback, exact request/body/header, shared pending created promise and cleanup/retry/isolation, timeout/fail-open/immediate return, and unchanged prompt behavior except client; explicitly skip Node execution when unavailable/short. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add the shared lifecycle resolver/evidence helper, caught notifier, created-flight map, fire-and-forget created dispatch, and prompt `client: "opencode"` in the embedded template only. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Add installer-byte and created/prompt independence tests plus the creation/prompt manual checklist subset; run agent/module/vet checks. <!-- sdd-owner: implementation -->

### 11. OpenCode deletion delivery — 120–200 native lines

**Dependency / finish:** slice 10 → the tested resolver/notifier sends independent fail-open `session.deleted` closure evidence and the manual checklist is complete. **Paths:** `jarvis-cli/embed/hooks/opencode/hive.ts`, the same `jarvis-cli/internal/agent/` source-derived/installer tests, `openspec/changes/issue-648-lazy-session-materialization/manual-opencode-lifecycle-checklist.md`. **Rollback:** revert deletion independently, then slice 10 if necessary; never edit installed plugin files.

- [ ] **RED:** Add executable/install tests for deleted identity/no-op, encoded end URL, exact evidence/client body, timeout/fail-open/immediate return, and independence from a pending created flight. <!-- sdd-owner: implementation -->
- [ ] **GREEN:** Add only deleted-event dispatch through the established resolver/notifier using `encodeURIComponent(id)` and the exact end endpoint/body. <!-- sdd-owner: implementation -->
- [ ] **TRIANGULATE/REFACTOR:** Complete the temporary installer/loopback manual checklist for created/deleted/prompt continuity and failure cases, record skips/runtime, and run agent/module/vet checks. <!-- sdd-owner: implementation -->

## Parent Review and Lifecycle Gates

- [ ] Review slice 1's API lifecycle-only diff, its native-accounting receipt, pull/watermark evidence, rollback, and `main <- 📍1 <- 2` context before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 2's regular EnsureSession foundation separately from end behavior; confirm its native-accounting receipt, DB/race evidence, exclusions, rollback, and `main <- 1 <- 📍2 <- 3` context before any commit/PR checkpoint. <!-- sdd-owner: parent -->
- [ ] Review slice 3's EnsureAndEndSession extension; confirm end rollback/concurrency receipts, separate native-accounting result, rollback, and `main <- 1 <- 2 <- 📍3 <- 4` context before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 4's snapshot acknowledgement, stale/failed-push evidence, no-network-held-transaction boundary, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 5's start coalescing, validation exclusion, race evidence, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 6's end adapters/native hook caller, evidence/duplicate/rollback receipts, compatibility document, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 7's prompt atomicity, manual-path preservation, independent-capture evidence, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 8's passive-observation atomicity, empty-attribution preservation, rollback evidence, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 9's memory/summary activation, trigger rollback and recovery regressions, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 10's OpenCode creation/prompt source-template change, source/install/shared-promise receipts, partial manual checklist, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
- [ ] Review slice 11's OpenCode deletion change, pending-created independence, full manual checklist/skips, source-of-truth compliance, and native-accounting receipt before merge. <!-- sdd-owner: parent -->
