# Hive Dashboard Endpoint Inventory

This checklist is the T03.1 source of truth for the dashboard API contracts that should feed T03.2 typed API client work. It maps the existing `hive-dashboard/` app, the PDF-derived dashboard screen registry, and current fixtures to Hive API endpoint needs.

## Scope

- Current SPA implementation: `hive-dashboard/src/main.ts` renders `Overview`, `Users`, `Memories`, and `Audit & sync` through explicit DOM views.
- PDF-derived target screens: `hive-dashboard/src/domain/dashboard.ts` defines 13 dashboard screens grouped as Explore, Team, Insights, and Governance.
- Fixture source: `hive-dashboard/src/fixtures/hive-dashboard/` provides target view models for all 13 screens.
- Current Hive API source: `hive-api/internal/handler/router.go` exposes auth, memory, sync, admin user, stats, and audit routes.

## Existing Endpoint Baseline

| Endpoint | Current auth | Current query/body support | Ready for T03.2 |
|----------|--------------|----------------------------|-----------------|
| `POST /auth/login` | Public | `{ email, password }` | Yes, already used. |
| `GET /auth/me` | Authenticated | None | Yes, already used. |
| `GET /health` | Public | None | Yes, already used. |
| `GET /admin/stats` | Admin | None | Partial, covers overview and basic analytics only. |
| `GET /admin/users` | Admin | None | Partial, lacks filtering, pagination, admin seat summary, and project memberships. |
| `POST /admin/users/:username/level` | Admin | `{ level }` | Yes, typed in T03.2; UI wiring remains future work. |
| `POST /admin/users/:username/grant-admin` | Admin | None | Yes, typed in T03.2; overlaps with level update semantics and needs deliberate UI usage. |
| `POST /admin/users/:username/deactivate` | Admin | None | Yes, typed in T03.2; UI wiring remains future work. |
| `GET /memories` | Authenticated | `project`, `category`, `limit`, `offset` | Partial, lacks tags, author, date, sort, and aggregate facets. |
| `GET /memories/search` | Authenticated | `query`, `project`, `limit`, `offset` | Partial, lacks highlights, score, category, author, date, and pagination total by query. |
| `GET /memories/:id` | Authenticated | Path id | Partial, useful for detail drilldowns only. |
| `POST /memories` | Authenticated | Memory create payload | Out of scope for read-first dashboard screens. |
| `POST /sync` | Authenticated | Daemon sync payload | Out of scope for dashboard UI; dashboard should observe sync state, not run daemon sync. |
| `GET /admin/audit-logs` | Admin | `project`, `actor_user_id`, `action`, `outcome`, `since`, `until`, `limit`, `offset` | Partial, supports audit/sync log but not conflict diff or dashboard event taxonomy. |

## Shared Shell Contracts

| Area | UI purpose | Fixture/view model source | Required endpoint(s) | Query/filter/pagination | Role/permission | Loading/empty/error needs | Backend gaps | T03.2 readiness |
|------|------------|---------------------------|----------------------|--------------------------|-----------------|---------------------------|--------------|-----------------|
| Login | Authenticate into same-host Hive API dashboard. | Existing `main.ts`, `auth/session.ts`; target profile in `shared.ts`. | `POST /auth/login`, `GET /auth/me`. | None. | Public for login, authenticated for session restore. | Login pending, invalid credentials, expired token restore failure. | None for current UI. Target profile needs role mapping from `User.level` to `ContributorRole`. | Ready. |
| Navigation and profile | Show current identity, role, logout, and target screen groups. | `CurrentProfileViewModel`, `NavigationGroupViewModel`, `dashboardNavigationGroups`. | `GET /auth/me`; local navigation registry. | None. | Authenticated. Governance links should be admin-only. | Shell loading, unauthorized admin message, missing profile fallback. | Need frontend mapping from current user to target profile initials/name fields. | Ready with mapper. |
| Notifications | Show unread and recent memory notifications. | `NotificationSummaryViewModel`, `NotificationViewModel`, `dashboardNotifications`. | Proposed `GET /dashboard/notifications`. | `limit`, `unread`, optional `project`, optional `since`. | Authenticated; admin can see organization-wide, member/viewer should be scoped. | Loading badge, zero unread, empty list, fetch failure. | No current endpoint provides notification summaries or unread state. | Blocked by new contract. |

## Explore Screens

| Screen | UI purpose | Fixture/view model source | Required endpoint(s) | Query/filter/pagination | Role/permission | Loading/empty/error needs | Backend gaps | T03.2 readiness |
|--------|------------|---------------------------|----------------------|--------------------------|-----------------|---------------------------|--------------|-----------------|
| Overview | Show organization health, total memories, active projects, daemon health, growth, project sync health, live activity, and most active projects. | Existing `Overview.ts`; target `OverviewFixtureViewModel`, `overview.ts`, `shared.ts`. | Current `GET /health`, `GET /admin/stats`; proposed `GET /dashboard/overview`. | Optional `range`, `project`; no pagination. | Current SPA requires admin. Target can be authenticated with scoped data, admin for org-wide. | Global dashboard loading, no memories, degraded health, partial stats failure. | `admin/stats` lacks active projects, daemon health, growth series, live activity, and sync health by project. | Partial. T03.2 can type current endpoints plus add a target overview DTO placeholder only if backend contract is accepted. |
| Projects | List projects with region, sync status, memory count, contributor count, and last sync. | `ProjectListFixtureViewModel`, `ProjectPrimitiveViewModel`, `projectsFixture`. | Proposed `GET /dashboard/projects`. | `status`, `region`, `search`, `limit`, `offset`, `sort`. | Authenticated; admin sees all, member/viewer scoped by membership. | Loading list, no projects, filtered empty, endpoint failure. | No project listing endpoint exists; current memory stats by project are aggregate-only. | Blocked by new contract. |
| Knowledge Browser | Browse memories by category with export count and paginated memory cards. | Existing `Memories.ts`; target `KnowledgeBrowserFixtureViewModel`, `CategoryFilterViewModel`. | Current `GET /memories`; proposed `GET /dashboard/memory-facets` or extend list response with facets. | `project`, `category`, `tag`, `author`, `from`, `to`, `limit`, `offset`, `sort`; export should reuse same filters. | Authenticated; scoped by accessible projects. | Loading page, empty category, empty filtered result, invalid filter, export unavailable. | Current list lacks category counts/facets, tags/date/author filters, sort, and export count semantics. | Partial. Type current list now; facets need backend contract. |
| Global Search | Search memories across accessible projects with highlights, score, sharing, clearing, and export. | `GlobalSearchFixtureViewModel`, `SearchResultViewModel`. | Current `GET /memories/search`; proposed richer `GET /dashboard/search`. | Required `query`; optional `project`, `category`, `tag`, `author`, `from`, `to`, `limit`, `offset`; stable scoring/sort. | Authenticated; scoped by accessible projects. | Initial empty query, loading, no matches, invalid query, search failure. | Current search lacks highlights array, score contract, category/date filters, and offset in response. | Partial. Current search is typable but not enough for target result cards. |
| Knowledge Graph | Visualize relationships between projects, contributors, memories, and categories. | `KnowledgeGraphFixtureViewModel`. | Proposed `GET /dashboard/knowledge-graph`. | `project`, `category`, `depth`, `limit`, optional `focus_id`. | Authenticated; scoped by accessible projects. | Loading graph, graph too sparse, no nodes, layout error, endpoint failure. | No relationship/graph endpoint exists; backend must define node/link derivation and limits. | Blocked by new contract. |
| Activity Feed | Show grouped recent memory activity with live polling. | `ActivityFeedFixtureViewModel`, `ActivityEntryViewModel`. | Proposed `GET /dashboard/activity`. | `project`, `category`, `actor`, `since`, `until`, `limit`, `cursor`; polling interval client-side. | Authenticated; scoped by accessible projects. | Initial loading, empty activity, stale poll error, reconnect/backoff. | No activity endpoint exists. Audit logs are admin-only and do not match memory activity entries. | Blocked by new contract. |

## Team Screens

| Screen | UI purpose | Fixture/view model source | Required endpoint(s) | Query/filter/pagination | Role/permission | Loading/empty/error needs | Backend gaps | T03.2 readiness |
|--------|------------|---------------------------|----------------------|--------------------------|-----------------|---------------------------|--------------|-----------------|
| Contributors | Show contributor cards with roles, human/agent kind, sync status, memory count, and project assignments. | `ContributorsFixtureViewModel`, `ContributorPrimitiveViewModel`, `contributorsFixture`. | Current `GET /admin/users`; proposed `GET /dashboard/contributors`. | `role`, `kind`, `status`, `project`, `search`, `limit`, `offset`. | Admin for all contributors. Member/viewer support requires scoped read decision. | Loading cards, no contributors, filtered empty, permission denied. | Current users endpoint lacks agent contributors, memory count, sync status, project IDs, role summary, and pagination. | Partial for admin users only; target cards blocked by new contract. |
| Developer Timeline | Show one developer's sessions grouped by date with linked memories. | `DeveloperTimelineFixtureViewModel`, `TimelineGroupViewModel`. | Proposed `GET /dashboard/contributors/:id/timeline`. | `project`, `from`, `to`, `limit`, `cursor`; selected contributor path param. | Admin for all contributors. Contributor self-view could be authenticated-only if product wants it. | Loading timeline, no sessions, contributor not found, linked memory missing. | No session/timeline read endpoint exists in Hive API. Sync payload stores sessions, but dashboard read contract is absent. | Blocked by new contract. |
| Sync Status | Show daemon health summary and contributor sync rows. | Existing `AuditSync.ts` partially; target `SyncStatusFixtureViewModel`, `DaemonHealthSummaryViewModel`. | Proposed `GET /dashboard/sync-status`; current `GET /admin/audit-logs` can support audit trail only. | `project`, `status`, `kind`, `limit`, `offset`; optional `stale_after`. | Admin. Dashboard must not start/stop daemons. | Loading rows, no daemons, degraded/unknown states, stale data warning. | No daemon health endpoint exists; current audit logs do not expose current daemon status or rates. | Blocked by new contract. |

## Insights Screens

| Screen | UI purpose | Fixture/view model source | Required endpoint(s) | Query/filter/pagination | Role/permission | Loading/empty/error needs | Backend gaps | T03.2 readiness |
|--------|------------|---------------------------|----------------------|--------------------------|-----------------|---------------------------|--------------|-----------------|
| Analytics | Show KPIs, activity over time, category distribution, active projects, developer contribution, and sync success ratio. | `AnalyticsFixtureViewModel`, `insights.ts`. | Current `GET /admin/stats`; proposed `GET /dashboard/analytics`. | `range`, `bucket`, optional `project`, optional `category`. | Admin for organization-wide. Scoped analytics for non-admins require product decision. | Loading charts, no data, partial chart failure, invalid range. | Current stats lacks time buckets, developer contribution, sync success ratio, peak activity, and range filtering. | Partial for static counts; target charts blocked by new contract. |

## Governance Screens

| Screen | UI purpose | Fixture/view model source | Required endpoint(s) | Query/filter/pagination | Role/permission | Loading/empty/error needs | Backend gaps | T03.2 readiness |
|--------|------------|---------------------------|----------------------|--------------------------|-----------------|---------------------------|--------------|-----------------|
| User Management | Manage users, roles, active state, current user protections, and admin seat usage. | Existing `Users.ts`; target `UserManagementFixtureViewModel`, `ManagedUserViewModel`. | Current `GET /admin/users`, `POST /admin/users/:username/level`, `POST /admin/users/:username/grant-admin`, `POST /admin/users/:username/deactivate`; proposed `GET /dashboard/user-management`. | `role`, `status`, `search`, `limit`, `offset`; mutations need target username and level. | Admin only. Self-demotion remains forbidden by API. | Loading users, no users, mutation pending, mutation conflict, permission denied, max admin seats reached. | Current list lacks pagination/filtering, admin seat summary, current-user flag in response, and reactivation path. | Partial. T03.2 should add typed mutation methods and keep target management DTO separate. |
| Audit Log | Review governance and sync audit events with filters and pagination. | Existing `AuditSync.ts`; target `AuditLogFixtureViewModel`, `AuditEventViewModel`. | Current `GET /admin/audit-logs`; proposed mapping to dashboard audit taxonomy. | Current: `project`, `actor_user_id`, `action`, `outcome`, `since`, `until`, `limit`, `offset`; target also wants event type filters. | Admin only. | Loading rows, no events, filtered empty, invalid date/filter, endpoint failure. | Current actions are `sync_push`, `sync_conflict`, `user_level_change`, `user_deactivate`; fixture event kinds are `sync_reject`, `role_change`, `deactivation`, `project_merge`, `conflict`. Need taxonomy alignment. | Mostly ready if T03.2 types current API and maps taxonomy explicitly. |
| Conflict Viewer | Inspect open/resolved memory conflicts with winning/losing versions and diff segments. | `ConflictViewerFixtureViewModel`, `ConflictViewModel`. | Proposed `GET /dashboard/conflicts`; optional future `POST /dashboard/conflicts/:id/restore-losing-version`. | `status`, `project`, `topic_key`, `author`, `limit`, `offset`. | Admin for restore. Read access requires product decision; safest is admin-only initially. | Loading conflicts, no open conflicts, resolved-only state, diff load failure, restore mutation pending/error. | No conflict read endpoint exists; sync response only returns conflict counts and does not persist dashboard-readable conflict detail. | Blocked by new contract. |

## T03.2 Typed Client Checklist

- [x] Keep existing typed methods for `login`, `currentUser`, `health`, `adminStats`, `adminUsers`, `memories`, `searchMemories`, and `auditLogs`.
- [x] Add typed admin mutation methods for `setUserLevel`, `grantAdmin`, and `deactivateUser` because endpoints already exist.
- [x] Add query parameter types that match current backend validation: memory list/search use `project`, `category`, `query`, `limit`, `offset`; audit logs use `project`, `actor_user_id`, `action`, `outcome`, `since`, `until`, `limit`, `offset`.
- [x] Add frontend mapper types from current API DTOs to the existing DOM views without pretending they satisfy all PDF-derived target view models.
- [x] Do not type proposed `/dashboard/*` endpoints as implemented until backend route contracts exist.
- [x] If T03.2 introduces mock clients for PDF-derived screens, mark those methods as fixture-backed or contract-proposed, not production-backed.

## T03.4 URL Filter Serialization Checklist

- [x] Add reusable dashboard URL filter utilities for browser/search/share flows without implementing later-epic pages.
- [x] Support current production query params: memories list/search (`project`, `category`, `query`, `limit`, `offset`) and audit logs (`project`, `actor_user_id`, `action`, `outcome`, `since`, `until`, `limit`, `offset`).
- [x] Support target dashboard filter keys that can round-trip through shared URLs: `query`, `project`, `category`, `developer`, `author`, `from`, `until`, repeated `tag`, `status`, `action`, `outcome`, `limit`, and `offset`.
- [x] Omit empty string, `null`, `undefined`, and invalid numeric values; parse invalid numbers by omission.
- [x] Use stable serialized ordering so browser URLs survive reload/share without incidental object key ordering.
- [x] Treat scalar repeated params as first-value-wins and repeated `tag` params as an explicit array.

## Backend Follow-up Checklist

- [ ] Define whether dashboard target contracts should be under `/dashboard/api/*` or general API resources such as `/projects`, `/contributors`, `/analytics`, and `/conflicts`.
- [ ] Add project list and contributor list read models before building Projects, Contributors, Knowledge Graph, or scoped analytics screens.
- [ ] Add current sync status read model that observes daemon state without starting, stopping, or configuring daemons.
- [ ] Add session/timeline read model if Developer Timeline remains part of the first production dashboard slice.
- [ ] Align audit event taxonomy between backend `AuditAction` values and governance fixture `AuditEventKind` values.
- [ ] Decide non-admin visibility rules for overview, projects, contributors, analytics, and conflict read screens.
- [ ] Define export semantics for memory browser/search before implementing export buttons.

## Acceptance Checklist

- [x] Every current SPA screen has a documented endpoint path.
- [x] Every PDF-derived dashboard screen in `dashboardScreenKeys` has a documented endpoint need.
- [x] Existing fixture/view model sources are referenced for each screen.
- [x] Query, filter, pagination, role, loading, empty, and error needs are captured.
- [x] Backend gaps are separated from T03.2 typed client work.
