# Proposal: Replace Healthy Daemons with Syncing Users

## Intent

Replace device-oriented health reporting with user-oriented synchronization visibility. Administrators need a 24-hour KPI and per-user context based on canonical identity, without hiding inactive accounts or conflating account and sync status.

## Scope

### In Scope
- Replace `Healthy Daemons` with `SYNCING USERS · 24H`, shown as qualifying active users over all active users.
- Use each user's latest completed attempt (`ended_at`) to determine 24-hour eligibility; a later failure overrides an earlier success.
- Add User Management sync context: latest successful `Last sync` and a sync status distinct from account status, while retaining every user.
- Render only `0 / 0` for a zero denominator.

### Out of Scope
- Lifetime sync history, durable user summary columns, and identity inference from daemon, device, email, or audit actor.
- Changes to account activation, roles, Audit Log semantics, or project-health classification.
- KPI-to-User-Management navigation; GitHub issue #468 remains a coordinated follow-up using this change's filters and status semantics.

## Capabilities

### New Capabilities
- `syncing-user-visibility`: Defines the Overview KPI and complete User Management sync projection, including identity, time-window, retention, empty-state, and account-status rules.

### Modified Capabilities
- None.

## Approach

Query retained `sync_attempt_logs` only by canonical `portal_user_id`. Use UTC `now`, `ended_at`, deterministic latest-attempt ordering, and fixed set-based queries. Count active users whose latest completed attempt within 24 hours succeeded; denominator is all active users. Separately project each user's latest retained success, displaying `Never` when none exists in 90-day history. Map explicit API DTOs through both surfaces; never issue per-user queries.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `hive-api/internal/{repository,service,model,handler}` | Modified | Replace daemon aggregation and add bulk user sync context. |
| `hive-dashboard/src/{api,domain,views}` | Modified | Update contracts, KPI, and user presentation. |
| Backend/dashboard tests and fixtures | Modified | Pin semantics, authorization, boundaries, and empty states. |

## Risks

- Retention makes `Never` a 90-day statement; document and test it.
- Contract drift or N+1 queries could produce inconsistent surfaces; share explicit semantics and enforce bounded query counts.

## Rollback Plan

Revert API, repository, dashboard, and test changes together to restore `daemon_health`; no data rollback is required because no new persistent state is introduced.

## Dependencies

- Canonical `sync_attempt_logs.portal_user_id` population and indexing established by #467/migration 015.
- Existing 90-day sync-attempt retention.
- Issue #468 for coordinated KPI navigation after this contract lands.

## Success Criteria

- [ ] KPI and user rows match the approved latest-attempt, `ended_at`, latest-success, retention, and zero-denominator rules.
- [ ] Inactive users remain visible and account status remains independent.
- [ ] Authorization is preserved and repository access uses a fixed number of set-based queries.
