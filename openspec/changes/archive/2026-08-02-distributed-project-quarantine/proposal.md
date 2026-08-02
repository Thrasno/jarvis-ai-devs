# Proposal: Distributed Project Quarantine

## Intent

Replace the misleading one-way block with reversible, auditable distributed quarantine. Administrators need safe cloud release, deterministic convergence, account progress, and durable governance without implying backup, deletion, devices, or offline status.

## Scope

### In Scope
- Deliver live issues in order: #474 contract cleanup, #475 lifecycle/inbox, #476 progress models, then #477 Quarantine Center.
- Preserve mixed-version behavior and historical `export_marker`/`purge_intent` readability while rejecting new `purge_intent` without mutation.
- Preserve monotonic generations, account convergence, admin-only governance, and reversible non-destructive archives.

### Out of Scope
- Physical deletion, backup/export, or historical record rewriting.
- Device inventory, online/offline inference, or waiting for all accounts before cloud release.
- Issues outside #474–#477 or a future destructive purge contract.

## Capabilities

### New Capabilities
- `project-quarantine-contract`: Quarantine-only writes, legacy-field tolerance, and readable historical actions.
- `distributed-quarantine-lifecycle`: Audited BLOCK/UNBLOCK generations, account inbox, idempotent application, ACK retry, and immediate cloud release.
- `quarantine-convergence-read-model`: Admin progress from current-generation ACKs across active accounts.
- `quarantine-center`: Admin dashboard list, detail, release, refresh, and generation-safe polling.

### Modified Capabilities
- None; no relevant baseline capability exists.

## Approach

Ship additive slices in issue order. Separate cloud lifecycle from local convergence; allocate generations transactionally under canonical-project locking. Poll an account-authenticated inbox outside project sync, retain HTTP 423, and apply commands idempotently without deletion. Derive progress from `users.is_active=true`; `pending` means “No ACK received.”

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `hive-api/` | Modified | Contracts, lifecycle, migration, audit, inbox, aggregation, admin APIs |
| `hive-daemon/` | Modified | Inbox ordering, generations, archive reversal, ACK retry |
| `hive-dashboard/` | Modified | Contract cleanup and admin Quarantine Center |
| Protocol/tests | Modified | Compatibility, authorization, race, migration, end-to-end coverage |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Rolling versions break history or release | High | Additive migrations; bounded compatibility; preserve legacy reads and 423 |
| Delayed commands override newer state | High | Transactional monotonic generations and stale-command ACKs |
| Progress leaks identity or misstates status | Medium | Admin authorization, safe projections, consistent snapshots |

## Rollback Plan

Retain legacy columns, rows, endpoints, and audit values. Disable new inbox/read/UI surfaces independently; stop new transitions without reverting an already audited cloud release or deleting local/cloud data.

## Dependencies

- Mandatory order: #474 → #475 → #476 → #477.
- RDD is disabled; no production implementation belongs to this phase.

## Success Criteria

- [ ] New contracts are truthful while historical audit data remains readable across mixed versions.
- [ ] BLOCK/UNBLOCK converges monotonically and non-destructively per active account.
- [ ] Admin APIs/UI report generation-correct progress and safely release quarantined projects.
