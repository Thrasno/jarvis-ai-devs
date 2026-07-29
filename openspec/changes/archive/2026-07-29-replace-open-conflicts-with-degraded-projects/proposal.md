# Proposal: Replace Open Conflicts with Degraded Projects

## Intent

Replace a misleading historical-event count with an actionable admin KPI. `OPEN CONFLICTS` implies lifecycle state that `sync_conflict` events do not have; `DEGRADED PROJECTS` will use canonical attempt telemetry.

## Scope

### In Scope
- Atomically replace `conflicts.open` across API and Dashboard with degraded and participating-project totals; no compatibility alias.
- Classify each project from active users' latest attempts across devices: any failure means degraded; all successes means healthy.
- Exclude disabled users immediately, blocked/quarantined projects, and projects with no recorded attempts from classification and KPI totals.
- Render `DEGRADED PROJECTS` as `N / total`; support the URL-backed degraded filter, inspectable results, browser restoration, and an empty state.
- Preserve `sync_conflict` audit history and use event-oriented presentation wording.

### Out of Scope
- Conflict open/resolved lifecycle or incident management.
- Empty/content lifecycle classification or an unknown bucket for this KPI.
- Broader metric-card navigation, owned by issue #468.

## Capabilities

### New Capabilities
- `project-sync-health`: Per-project health and KPI semantics from active users' latest attempts.
- `degraded-project-discovery`: KPI presentation and URL-backed degraded-project filtering.

### Modified Capabilities
None.

## Approach

Use one server-side sync-health projection for project rows and the KPI. Select the newest attempt by timestamp for each project and active portal user across devices, then aggregate latest outcomes. Replace the admin API field and Dashboard model together. Keep audit storage and queries intact; change only misleading consumer wording.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `hive-api/internal/{repository,service,model}` | Modified | Aggregation and overview contract |
| `hive-dashboard/src/{api,domain,views,main.ts}` | Modified | KPI, filter, results, and copy |
| API/Dashboard tests and fixtures | Modified | Contract, routing, and accessibility coverage |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Views disagree | Medium | Consume one projection and contract-test totals |
| Attempt timestamps tie | Medium | Specify deterministic ordering in design/spec |
| Atomic rename breaks stale clients | Medium | Coordinate API and Dashboard deployment |
| Single PR exceeds 800 lines | Medium | Forecast in tasks; keep #468/#469 separate |

## Rollback Plan

Revert API and Dashboard contract changes together without deleting sync attempts or audit events.

## Dependencies

- Issue #469 must consume these health semantics; issue #468 owns broader contextual metric navigation.

## Success Criteria

- [ ] API and Dashboard expose no `conflicts.open` compatibility field or `openConflicts` KPI model.
- [ ] KPI and project rows follow the confirmed active-user, latest-attempt, exclusion, and health rules.
- [ ] Direct, refreshed, shared, and back/forward degraded-filter URLs remain consistent and accessible.
- [ ] Historical conflict events remain queryable and are never presented as open conflicts.
