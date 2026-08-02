# Design: Distributed Project Quarantine

## Technical Approach

Deliver additive slices **#474 → #475 → #476 → #477**. PostgreSQL owns cloud state and generations; daemons apply reversible local state. Quarantine Center adds an admin-only, generation-pinned read model whose public projection identifies users only by username.

## Architecture Decisions

| Decision | Alternatives / tradeoff | Choice and rationale |
|---|---|---|
| Lifecycle | Persist inferred phases | Persist `cloud_blocked` plus immutable BLOCK/UNBLOCK commands; derive `QUARANTINING/QUARANTINED/RELEASING/ACTIVE` from current-generation ACKs so release remains immediate. |
| Progress identity | Aggregate-only or account/device detail | Return username rows only to admins. Username is the maintainer-approved minimum identity; email, IDs, auth subjects, devices, tokens, and presence inference are forbidden. |
| Snapshot | Cached counters or independent queries | Compute totals and rows in one read-only `REPEATABLE READ` transaction pinned to canonical project and generation. |
| Local reversal | Delete archive records | Reverse only quarantine-owned metadata; preserve data and pre-existing/manual archives. |

## Data Model and Transactions

`hive-api/migrations/017_quarantine_contract.sql` preserves historical fields, accepts only quarantine writes, and rejects new `purge_intent` before mutation. `hive-api/migrations/018_distributed_quarantine.sql` adds locked heads, immutable generation commands, generation/action ACK fields, uniqueness, and lookup indexes. Backfill blocked rows as generation 1 without rewriting history.

`TxManager.WithinTx` serializes transitions by canonical-key advisory lock plus head `FOR UPDATE`; overflow, unchanged targets, and failures do not mutate state. SQLite stores generation/action, durable ACK retry, and quarantine-owned archive metadata. BLOCK/UNBLOCK never deletes project data.

## API, Projection, and Authorization

`GET /admin/quarantines` returns bounded summaries. `GET /admin/quarantines/:canonical?generation=<g>&limit=<n>&after=<opaque>` returns this closed projection:

```text
{ project, canonical_project_key, generation, action, state, transitioned_at,
  totals:{active,acknowledged,pending},
  progress:[{username,state,acknowledged_at?}], next_cursor? }
```

`state` is `pending|applied|failed|skipped`; `pending` means only “No ACK received.” `acknowledged_at` is the accepted current-generation ACK time and is absent for pending. No free-form warning is exposed. List summaries omit `progress`; detail preserves aggregate totals.

Routes run `RequireAuth` then `RequireAdmin` before service/repository access. A dedicated response DTO—not `model.User`, ACK, or delivery structs—is the only serializer. Internal joins may use user ID/auth subject but projection drops them. `401/403` use generic existing errors and no body fields, counts, existence signal, usernames, or membership; authorization failure never executes the read query. Logs/metrics also omit usernames and forbidden fields.

## Repository Snapshot and Ordering

Extend `ProjectBlockRepository` in `hive-api/internal/repository/project_block.go` with quarantine summary/detail queries in `hive-api/internal/repository/postgres_project_block.go`; extend `TxManager` in `hive-api/internal/repository/tx.go` and `hive-api/internal/repository/postgres_tx.go` with explicit read-only `REPEATABLE READ` execution. The detail transaction:

1. Reads the canonical head and pins requested/current generation.
2. Selects `users.is_active=true`; left-joins one deduplicated ACK for that project command/generation.
3. Derives each row and totals from that same relation; old-generation and inactive-user ACKs cannot count.
4. Orders rows by `lower(username) ASC, username ASC, users.id ASC` (ID is cursor-only, never projected).

The opaque cursor binds canonical key, generation, and final sort tuple. Later pages remain generation-pinned; malformed or cross-project cursors fail without data. Duplicate ACKs collapse deterministically to the latest accepted result by `applied_at DESC, updated_at DESC`, with stable outcome precedence as final tie-breaker. Reactivation enters totals immediately and reuses only an ACK for the pinned command/generation.

## Flow and Dashboard

Account-authenticated daemons flush ACKs, drain an inbox ordered by `(canonical_project_key,generation)`, then sync. Stale commands ACK `skipped`; replay returns the durable result; ACK retries are idempotent. HTTP 423 and mixed-version compatibility remain.

The detail renders aggregate cards then a semantic table: **Username**, **Current generation**, **Outcome**, **Acknowledged at**. Pending shows “No ACK received,” never offline/unreachable. Rows use server order; filtering is local and must not change totals. Polling is route-scoped (15 seconds), hidden-paused, abortable, and discards lower generations/request sequences while preserving selection, filters, cursor, and scroll. `401` ends session; `403` denies; absent capability or `404/405/501` is terminal unsupported with no legacy fallback.

## Files and Tests

| Component | Existing | Proposed new |
|---|---|---|
| API | `hive-api/internal/model/project_block.go`<br>`hive-api/internal/repository/project_block.go`<br>`hive-api/internal/repository/postgres_project_block.go`<br>`hive-api/internal/repository/tx.go`<br>`hive-api/internal/repository/postgres_tx.go`<br>`hive-api/internal/service/project_governance.go`<br>`hive-api/internal/handler/project_governance.go`<br>`hive-api/internal/handler/router.go`<br>`hive-api/migrations/migrations.go`<br>`hive-api/internal/handler/project_governance_test.go`<br>`hive-api/internal/repository/postgres_project_block_test.go`<br>`hive-api/internal/repository/postgres_tx_test.go` | `hive-api/migrations/017_quarantine_contract.sql`<br>`hive-api/migrations/018_distributed_quarantine.sql`<br>`hive-api/migrations/distributed_quarantine_test.go` |
| Daemon | `hive-daemon/internal/db/db.go`<br>`hive-daemon/internal/db/project.go`<br>`hive-daemon/internal/db/project_block.go`<br>`hive-daemon/internal/sync/client.go`<br>`hive-daemon/internal/sync/syncer.go`<br>`hive-daemon/internal/db/project_block_test.go`<br>`hive-daemon/internal/sync/syncer_test.go` | None |
| Dashboard | `hive-dashboard/src/api/client.ts`<br>`hive-dashboard/src/domain/dashboard.ts`<br>`hive-dashboard/src/main.ts`<br>`hive-dashboard/src/components/Sidebar.ts`<br>`hive-dashboard/src/fixtures/hive-dashboard/governance.ts`<br>`hive-dashboard/src/fixtures/hive-dashboard/index.ts`<br>`hive-dashboard/src/styles.css`<br>`hive-dashboard/src/api/client.test.ts`<br>`hive-dashboard/src/app.test.ts`<br>`hive-dashboard/src/components/Sidebar.test.ts` | `hive-dashboard/src/views/Quarantine.ts`<br>`hive-dashboard/src/views/Quarantine.test.ts`<br>`hive-dashboard/src/views/QuarantineRoute.test.ts` |

Coverage: API auth/projection/redaction/order/snapshot/races, PostgreSQL migration/locking, SQLite stale/replay/archive/ACK retry, and Dashboard route/poll/render/accessibility/privacy plus BLOCK→progress→release→UNBLOCK history. Dashboard behavioral proof uses the repository-supported focused Vitest route/client/controller suite; `hive-dashboard/package.json` defines no E2E command. Use `t.TempDir()`; skip integrations in short mode.

## Bounded Verification Remediation

The remediation runtime evidence is intentionally layered: `memory_transaction_integration_test.go` drives two concurrent service transitions through PostgreSQL advisory locks and verifies immutable generations `1,2` plus the current head. `postgres_project_block_test.go` drives the real `ReadOnlyRepeatableRead` manager, reads the generation-pinned admin projection, commits a concurrent UNBLOCK through another connection, and proves the in-flight list/detail snapshot remains generation 1 while a later read observes generation 2. `project_governance_test.go` covers the authenticated admin list projection and anonymous/accountless inbox rejection without service access or disclosed command fields. `QuarantineRoute.test.ts` proves a disabled center can be re-enabled to load retained server state.

The list summary is a closed DTO. It exposes only project, canonical key, generation, action, the latest current-generation active-account outcome (or `pending` when none exists), and transition time. It never exposes raw account identifiers, delivery tokens, actor IDs, or historical ACK payloads.

## Threat Matrix

API/dashboard routing changes, but none of the execution/VCS boundaries apply.

| Boundary | Minimum adversarial cases | Applicability | Design response | Planned RED tests |
|---|---|---|---|---|
| Documentation-like paths | Executable-looking docs | N/A: no file classification/execution | None | None |
| Git repository selection | Relative/absolute selectors | N/A: no VCS operation | None | None |
| Commit state | Staged/unstaged/index | N/A: no commit operation | None | None |
| Push state | Tracking/refspec | N/A: no push operation | None | None |
| PR commands | Head/env/composition | N/A: no PR automation | None | None |

## Migration / Rollout

Roll out #474, #475 API/migrations before daemons, then #476 reads and #477 UI behind independent flags. Rollback disables writes/surfaces but retains schema, history, archives, and committed cloud releases.

## Delivery Authorization Reconciliation

The maintainer explicitly approved `size:exception` up to 3,000 changed code/test lines for this change. The verified code/test review surface is 2,607 lines. This supersedes the prior 2,000-line approval and resolves the authorization blocker recorded in the historical failed verification report; it does not change that report's `FAIL` verdict, which remains historical until a fresh `sdd-verify` run.

## Open Questions

None.
