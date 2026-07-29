# Exploration: Replace Open Conflicts with Degraded Projects

## Current State

GitHub issue [#467](https://github.com/Thrasno/jarvis-ai-devs/issues/467) is open and proposes replacing the misleading administrative Overview metric `OPEN CONFLICTS` with `DEGRADED PROJECTS`. The live issue describes the current value as a 30-day count of `audit_logs` rows with `action = sync_conflict`; those rows have no open/resolved lifecycle and may aggregate multiple conflicts.

The repository confirms that behavior:

- `hive-api/internal/service/overview.go` calls `auditRepo.CountSyncConflicts(...)` and serializes the result as `OverviewConflicts.Open`.
- `hive-api/internal/model/overview.go` exposes `conflicts: { open: number }` in both admin overview response shapes, while `ProjectSyncHealth.Status` already supports `healthy | degraded | unknown`.
- The same overview service obtains per-project health from `syncRepo.SyncHealthByProject(...)`, but currently defaults every non-success latest outcome to `degraded`; it does not expose an explicit no-telemetry/freshness policy.
- `hive-dashboard/src/main.ts` maps `operations.conflicts.open` to the `Open Conflicts` metric, and `hive-dashboard/src/domain/dashboard.ts` stores it as `openConflicts`.
- `hive-dashboard/src/views/Overview.ts` renders metric tiles as non-interactive `article` elements. The existing Projects view renders all API projects and already displays normalized `healthy`, `degraded`, or `unknown` health, but it has no URL-backed health filter.
- Historical conflict events remain represented by `hive-api/internal/model/audit.go` (`sync_conflict`) and counted by `hive-api/internal/repository/postgres_audit.go`; this data should remain available as event history, not as an open-work queue.

The requested artifact store is OpenSpec, but `openspec/config.yaml` is currently absent. Existing change artifacts still establish the expected `openspec/changes/{change}/exploration.md` path; downstream phases should resolve or document the missing project configuration before relying on phase-specific rules.

## Affected Areas

- `hive-api/internal/service/overview.go` — replace the audit-row KPI source with a project-health aggregation and define the qualifying/latest telemetry policy.
- `hive-api/internal/model/overview.go` — evolve the admin overview contract from `conflicts.open` to degraded-project totals/status metadata without exposing conflict lifecycle semantics that do not exist.
- `hive-api/internal/repository/postgres_audit.go` and `hive-api/internal/repository/audit.go` — preserve historical `sync_conflict` queries; only change consumers or labels if Audit Log currently presents them as open conflicts.
- `hive-api/internal/repository/*sync*` and overview repository tests — verify latest outcome selection, freshness boundaries, multiple attempts, blocked/excluded projects, and projects with no telemetry.
- `hive-dashboard/src/api/client.ts` — update the typed overview response and project response/filter contract.
- `hive-dashboard/src/main.ts` — map the new aggregate into the view model and pass the route query when loading Projects.
- `hive-dashboard/src/domain/dashboard.ts` — rename the metric view-model field and filter project data by an explicit `health=degraded` route state.
- `hive-dashboard/src/views/Overview.ts` — render the exact `DEGRADED PROJECTS` label and, in the follow-up navigation work, provide the contextual link without nesting interactive controls.
- `hive-dashboard/src/views/Projects.ts` and dashboard routing/query helpers — restore the degraded filter on direct load, refresh, and browser navigation; provide inspectable filtered results and an empty state.
- `hive-dashboard/src/fixtures/hive-dashboard/overview.ts`, `hive-dashboard/src/app.test.ts`, `hive-dashboard/src/views/Overview.test.ts`, `hive-dashboard/src/views/Projects.test.ts`, and domain/client tests — update fixtures and contract, accessibility, routing, and empty-state coverage.

## Dependencies and Integration

### Issue #469 — compact Sync Health table

Live issue #469 is open. It explicitly depends on #467's degraded-health semantics for status badges, ordering, and footer navigation. It is primarily a frontend Overview presentation change and states that no backend changes are expected. Therefore #467 should establish the canonical status and count contract first; #469 should consume it rather than infer degraded status independently. Shared status primitives and the five-project summary must not silently redefine numerator, denominator, unknown handling, or freshness.

### Issue #468 — contextual metric navigation

Live issue #468 is open and depends on #466 and #467. It expects the resulting metric to be named `DEGRADED PROJECTS` and to navigate to `/dashboard/projects?health=degraded`. Its implementation should follow or fold in after #467 establishes the API/view-model contract and Projects filter. This creates a frontend integration dependency, but it does not require #467 to implement the full semantic-card navigation scope.

### Issue #466 — upstream metric contract

Although not requested as a primary dependency investigation, #468 identifies #466 as a prerequisite for the complete set of Overview metrics. #467 must avoid coupling degraded-project semantics to the syncing-user metric; the two aggregates can share the admin overview response but should remain independently defined and authorized.

## Approaches

1. **Derive degraded totals from the existing per-project sync-health query**
   - Pros: reuses the existing `SyncHealthByProject` repository/service path and status vocabulary; minimizes API surface change; naturally supports #469's table.
   - Cons: requires extending the query contract to return the tracked-project denominator and explicit unknown/freshness information; current non-success fallback is too coarse.
   - Effort: Medium

2. **Add a dedicated degraded-project aggregate repository query**
   - Pros: can optimize numerator/denominator computation in SQL and make the KPI contract explicit.
   - Cons: risks duplicating latest-outcome and freshness logic used by the per-project rows; #469 could show a different result from the card; more backend contract and test surface.
   - Effort: Medium-High

3. **Compute the metric entirely in the dashboard from the project list**
   - Pros: small backend change if project health is already complete.
   - Cons: the existing `/projects` response is a different projection, may not expose the same tracked set or freshness evidence, and would allow Overview and backend consumers to disagree; not suitable for an authoritative admin KPI.
   - Effort: Low initially, High risk

## Recommendation

Use the existing server-side per-project sync-health aggregation, strengthened into one canonical projection that returns the tracked-project denominator, each project's latest qualifying outcome, status, and telemetry freshness/unknown state. Derive `degraded_count` from that same projection and expose a clear numerator/denominator contract to the dashboard. Keep historical `sync_conflict` audit rows unchanged and rename only their presentation to event-oriented wording where necessary.

The contract must decide before implementation:

1. The exact freshness window and whether a stale latest attempt becomes `unknown` or `degraded`.
2. Whether projects with no telemetry are included in the denominator and excluded from the degraded numerator (the issue requests explicit behavior; silently treating them as healthy is unsafe).
3. Whether blocked/quarantined projects are tracked, excluded, or represented as unknown, consistent with the existing `SyncHealthByProject` repository behavior.
4. Whether the API preserves a compatibility alias for `conflicts.open` during rollout or makes the response rename atomically with dashboard consumers.

Implement #467's backend/API contract first, then integrate the Projects `health=degraded` filter and Overview presentation. Coordinate #469 and #468 against that contract; do not let either frontend issue independently infer health semantics.

## Risks

- **Semantic disagreement:** A separate card calculation, table calculation, and Projects filter could classify the same project differently unless they consume one canonical contract.
- **Unknown telemetry ambiguity:** Missing or stale attempts can make a denominator look healthier than the system can prove; this requires explicit API and copy semantics.
- **Backward compatibility:** Renaming `conflicts.open` affects Go models, JSON fixtures, TypeScript types, mapping, and any external dashboard client.
- **Historical terminology:** Audit Log may still use `open` wording even though the underlying records are immutable events; changing the KPI must not delete or reinterpret history.
- **Issue sequencing:** #468 depends on #467 and #466, while #469 consumes #467's status semantics; implementing frontend navigation or table styling against provisional fields will cause rework.
- **Authorization consistency:** The degraded-project projection and filtered Projects route must preserve existing admin/member capability boundaries.
- **Review budget:** The requested `single-pr` strategy has an 800-line review budget, but the combined backend contract, dashboard route/filter, accessibility, and tests may exceed the default 400-line guard. The task phase should forecast exact authored lines and split delivery if needed despite the provided strategy.

## Ready for Proposal

Yes, with the four contract decisions above recorded in the proposal/spec. No implementation or build was performed in this exploration.
