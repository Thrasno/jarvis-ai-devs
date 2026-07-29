## Exploration: replace-healthy-daemons-with-syncing-users

### Current State

The admin Overview currently exposes `daemon_health`, backed by `SyncAttemptRepository.DaemonHealth`. It counts distinct non-empty `daemon_id` values with attempts in a 30-day window and counts a daemon as healthy when its latest 24-hour attempt is successful. `sync_attempt_logs` now has canonical nullable `portal_user_id` identity from migration 015; ingestion resolves authenticated users for member requests and resolves an admin's `dev_id` to a registered user. Sync attempts are retained for 90 days by best-effort cleanup during ingestion.

The existing project-health implementation already joins canonical active users and applies deterministic latest-attempt ordering per `(project, portal_user_id)`. This strongly supports deriving the new user metric and per-user context from canonical sync-attempt records rather than audit events. Audit remains appropriate for event-level investigation, not for the aggregate identity projection.

Admin user management currently returns the complete `User` list through `AdminService.ListUsers` and `/admin/users`, preserving inactive users and admin-only authorization. The response has no sync fields. The dashboard renders the response in `src/views/Users.ts`; Overview maps `operations.daemon_health` to the `Healthy Daemons` card through `src/main.ts` and `src/domain/dashboard.ts`.

### Affected Areas

- `hive-api/internal/repository/sync_attempt.go` and `postgres_sync_attempt.go` — add one set-oriented aggregate/query boundary for active-user 24-hour activity and per-user latest sync context; avoid per-user queries.
- `hive-api/internal/service/overview.go` — replace daemon-health aggregation with an explicit UTC cutoff and active-user denominator, preserving the admin capability boundary.
- `hive-api/internal/service/admin.go` and `hive-api/internal/repository/user.go` — compose complete users with one bulk sync-context query, or introduce a dedicated admin projection service boundary without N+1 behavior.
- `hive-api/internal/model/overview.go`, `response.go`, and sync-attempt models — define stable API DTOs for `syncing_users` and user `last_sync`/status; avoid exposing internal records.
- `hive-api/internal/handler/overview.go`, `admin.go`, router wiring, and mocks — preserve `RequireAuth` + `RequireAdmin` and update contract tests.
- `hive-api/migrations/015_sync_attempt_portal_users.sql` and a follow-up migration if needed — verify/support indexes for `(portal_user_id, activity timestamp)` and active-user joins; no durable lifetime column is currently present.
- `hive-dashboard/src/api/client.ts` — update response contracts using strict, named types.
- `hive-dashboard/src/main.ts`, `src/domain/dashboard.ts`, `src/views/Overview.ts` — map and render exactly `SYNCING USERS · 24H`, including honest `0 / 0` handling.
- `hive-dashboard/src/views/Users.ts` and related fixtures/tests — render `Last sync` and `Last 24h`/`Inactive`/`Never` for every returned user while retaining the full list.
- Existing Go and dashboard tests — cover aggregation, boundaries, authorization, wire contracts, loading/error/empty states, and all user statuses.

### Approaches

1. **Canonical sync-attempt projection, any qualifying success in window** — query active users joined to `sync_attempt_logs` by `portal_user_id`, count distinct users with a qualifying successful attempt in `[now-24h, now]`, and separately return each user's latest successful sync timestamp (or latest attempt timestamp if product chooses that meaning).
   - Pros: uses the identity established by #467; excludes device inference; one set-based query can serve Overview and User Management; naturally handles deduplication and active-account filtering; keeps Audit Log event-level.
   - Cons: 90-day retention means literal lifetime `Never` cannot be proven for users with no remaining rows; a query/API boundary must be designed carefully to prevent N+1.
   - Effort: Medium.

2. **Audit-event aggregation** — derive activity from authenticated audit actions/outcomes and join `actor_user_id` to users.
   - Pros: audit already has actor identity and explicit event semantics.
   - Cons: duplicates the canonical sync-attempt identity path introduced by #467; risks missing successful sync attempts or counting unrelated audit events; creates disagreement between operational sync data and the user-facing KPI.
   - Effort: Medium, with higher semantic risk.

3. **Durable per-user sync summary** — maintain `users.last_successful_sync_at` (and possibly `last_sync_attempt_at`) transactionally or through ingestion, then use direct user reads for both surfaces.
   - Pros: efficient reads and true lifetime `Never` semantics; avoids scanning retained attempts.
   - Cons: migration/backfill and dual-write correctness; historical users cannot be backfilled beyond retained rows; summary semantics become another source of truth; unnecessary for this single-PR scope unless product requires lifetime guarantees.
   - Effort: High.

### Semantic Comparison

| Decision | Option A | Option B | Exploration recommendation |
|---|---|---|---|
| Numerator | Any qualifying success in the rolling window | Latest attempt wins | Any-success is the literal “associated with qualifying sync activity” interpretation and is resilient to a later failure; latest-wins is appropriate for health, not recent participation. Confirm with product. |
| `Last sync` | Latest successful sync | Latest attempt, including failure | Prefer latest successful sync for user-facing “sync” context, while optionally exposing a separate last-attempt outcome if reconciliation needs it. Confirm explicitly. |
| Active account | Active users only | All users | Active users form the denominator and numerator; inactive users remain visible in User Management with `Inactive`. |
| `Never` | No retained successful attempt | No lifetime record ever existed | With 90-day deletion, “never” can only mean no retained qualifying success unless durable summary persistence is added. Product must choose whether that limitation is acceptable. |
| Window | `started_at >= cutoff` | `COALESCE(ended_at, started_at) >= cutoff` | Use one documented activity timestamp consistently; UTC cutoff should be computed once per service call and use an inclusive lower bound with a clear upper-bound policy. |

### Recommendation

Use a canonical, set-based sync-attempt projection keyed only by `portal_user_id`, joined to `users.is_active`, with one captured `now.UTC()` and a rolling 24-hour cutoff. Implement the Overview numerator as distinct active users with at least one qualifying successful attempt in the window; retain the denominator as all active registered users. Prefer `Last sync` to mean the latest successful sync timestamp, and represent absent values as `null`/`Never`. Keep inactive users in the complete `/admin/users` response and label them `Inactive` independently of sync history.

The repository should expose a bulk projection that can return active-user aggregate counts and all-user latest sync context in bounded query work. The service should map that projection to API DTOs; the handler should remain a thin authorized adapter. If one query cannot cleanly serve both use cases, use a small fixed number of set-based queries, never one query per user. Do not add durable summary columns or a migration solely to manufacture lifetime `Never` during this change; document the 90-day retention limitation and revisit persistence if product requires a literal lifetime guarantee.

The frontend should replace the existing daemon-health field throughout the API/ViewModel/fixture/test chain, render `SYNCING USERS · 24H`, and distinguish `0 / 0` from a percentage (for example, display `0 / 0` without a percentage or “healthy” implication). User Management should add explicit columns/accessible labels for `Last sync` and `Sync status` while leaving role/status controls and all-user rendering unchanged.

### Risks

- A “last successful” choice can show an older success after a recent failure; a “last attempt” choice can make a failed sync appear as recent activity. The API must name the semantics and tests must pin them.
- Retention cleanup deletes rows older than 90 days, so `Never` is not literal lifetime history without durable user-level state.
- Boundary errors are likely if different queries use local time, `ended_at` versus `started_at`, or inconsistent inclusive/exclusive comparisons.
- Joining by nullable `portal_user_id` excludes legacy/unresolved attempts by design; falling back to `source_dev_id`, email, daemon ID, or audit actor would violate canonical identity and can misattribute users.
- Reusing `User` directly risks exposing internal fields and makes optional sync data harder to evolve; use explicit response DTOs.
- A dashboard contract change must update fixtures, loading/error states, and admin capability tests together; stale `healthyDaemons` references can preserve the old metric invisibly.
- Existing `GetForLevel` invokes multiple aggregate methods; adding per-user calls from the handler or service could create N+1 latency. Keep aggregation set-oriented and authorize before querying admin data.
- No new migration is justified yet beyond verifying the existing portal-user index; adding one should be limited to an evidenced query plan/index gap.

### Explicit Product Questions for Interactive Proposal Round

1. Should `SYNCING USERS · 24H` count a user when **any successful canonical sync attempt** occurred in the window, even if a later attempt failed, or should the user’s latest attempt determine eligibility?
2. Should `Last sync` display the latest **successful** attempt, or the latest attempt with a visible success/failure outcome?
3. What exact timestamp is the activity timestamp: `started_at`, `ended_at` when present, or `COALESCE(ended_at, started_at)`? Should the 24-hour lower boundary be inclusive (`>=`) and should a future-skewed timestamp be excluded with an upper bound (`<= now`)?
4. Is `Never` acceptable as “no retained successful attempt in the 90-day log,” or is literal lifetime history required, justifying durable user-level summary state and migration?
5. For `0 / 0`, should the UI show plain `0 / 0`, `No active users`, or another exact copy, and must it suppress percentage styling?
6. Should failed attempts be ignored for the numerator but still drive `Last sync`/status when `Last sync` is defined as latest attempt?
7. Is a fixed small set of bulk repository queries acceptable for User Management, or is a single combined projection required?

### Review Budget Forecast

- Expected authored change: approximately 700–1,200 lines across backend contracts/queries/tests, dashboard contracts/views/tests, and migration/index verification.
- Delivery strategy: single PR is plausible but requires disciplined scope and no durable-summary redesign.
- `1600-line budget risk`: Medium; confirm after design/task decomposition because broad contract tests or fixture rewrites could exceed the requested budget.
- `Decision needed before apply`: No for the delivery strategy; Yes for the unresolved product semantics above.
- `Chained PRs recommended`: No, unless the proposal chooses durable lifetime persistence or expands the metric/navigation scope.

### Ready for Proposal

Yes, after the interactive proposal round answers the seven product questions above. The proposal should lock the canonical sync-attempt source, timestamp/boundary semantics, `Last sync` meaning, retention interpretation of `Never`, exact `0 / 0` copy, and the fixed set-based repository contract before design and tasks.
