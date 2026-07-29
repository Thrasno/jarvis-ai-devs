# Design: Replace Open Conflicts with Degraded Projects

## Technical Approach

Persist the authenticated portal-user association on each sync-attempt audit row, then build one unbounded PostgreSQL projection that selects every active user's latest attempt per project and aggregates project health. Overview totals, overview rows, and Projects filtering consume that projection. Replace the API and Dashboard contract in one release; retain conflict audit storage but remove open-work wording.

## Architecture Decisions

| Decision | Alternatives | Rationale |
|---|---|---|
| Add nullable `portal_user_id` plus `portal_user_source` to `sync_attempt_logs` | Treat `source_dev_id` or device IDs as user identity | JWT `sub` is the stable portal identity. For member ingestion, record the authenticated subject with source `auth_subject`; for admin cross-user ingestion, resolve the exact `dev_id` email to a portal user with source `admin_dev_id`; unresolved admin submissions remain audit-only. Backfill exact email matches as `legacy_email`. The `(project, portal_user_id)` rows establish telemetry-derived participation, not authorization or membership. |
| Select latest with `COALESCE(ended_at, started_at) DESC, ingested_at DESC, attempt_id DESC, id DESC` | Timestamp alone; daemon/device tie-break | This is a total, stable ordering over persisted records and never exposes or depends on device identity. |
| Return a single `ProjectSyncHealthProjection` containing rows and totals | Separate KPI query; Dashboard counting | One repository result prevents numerator/denominator drift. It has no 30-day cutoff: all recorded attempts participate. |
| Add server-side `GET /projects?health=degraded` | Client-only filtering | The API preserves access boundaries and makes shared URLs inspectable. Absent `health` keeps the normal list; `degraded` returns only canonical degraded participants; unsupported values return `400`. |

## Data Flow

```text
JWT subject + attempt payload -> identity resolution -> sync_attempt_logs
                                                    |
users(is_active) + project_blocks -> ranked attempts -> health projection
                                             |             |
                                      Overview API     Projects API
                                             |             |
                                      N / total      ?health=degraded
```

The SQL joins `users` and filters `is_active = true` at read time, so deactivation is immediate. It excludes rows without `portal_user_id` and projects matching an active canonical project block. After ranking per `(project, portal_user_id)`, any latest failure makes the project degraded; otherwise it is healthy. Projects without qualifying attempts have no KPI classification and are absent from both totals.

## File Changes

| File | Action | Description |
|---|---|---|
| `hive-api/migrations/015_sync_attempt_portal_users.sql`, `hive-api/migrations/migrations.go`, `hive-api/cmd/server/main.go` | Create/modify | Add identity columns, constrained provenance, exact-email backfill, and projection index; register migration. |
| `hive-api/internal/model/{sync_attempt,overview}.go` | Modify | Carry portal identity/provenance; replace `conflicts` with degraded totals and projection types. |
| `hive-api/internal/{handler,service}/sync_attempt.go` | Modify | Pass authenticated actor and resolve per-attempt portal users without trusting device metadata. |
| `hive-api/internal/repository/{sync_attempt,postgres_sync_attempt,project,postgres_project}.go` | Modify | Expose the canonical projection and merge it into project summaries/filtering. |
| `hive-api/internal/{service,handler}/project.go`, `hive-api/internal/service/overview.go` | Modify | Consume one projection and validate the health filter. |
| `hive-dashboard/src/api/{client,urlFilters}.ts`, `src/{main,domain/dashboard}.ts` | Modify | Atomically rename types/models, serialize `health`, reload Projects when query state changes, and map canonical results. |
| `hive-dashboard/src/views/{Overview,Projects}.ts`, `src/fixtures/hive-dashboard/overview.ts` | Modify | Render `DEGRADED PROJECTS`, active filter state, filtered rows, and explicit degraded empty state; retain event wording. |
| `hive-api/internal/repository/postgres_sync_attempt_overview_test.go`, `hive-api/internal/{service,handler}/{sync_attempt,overview,project}_test.go` | Modify | Add identity, projection, and JSON/filter contract coverage. |
| `hive-dashboard/src/{api,domain,views}/*.test.ts`, `hive-dashboard/src/app.test.ts` | Modify | Add mapping, routing, accessibility, and fixture coverage. |

## Interfaces / Contracts

```json
"degraded_projects": { "degraded": 2, "total": 5 }
```

This replaces `conflicts` in both admin overview responses with no alias. `sync_health_by_project` is emitted from the same projection. Project summaries use nullable/omitted health for nonparticipants; they are never counted as `unknown` in this KPI.

## Testing Strategy

| Layer | Planned RED coverage |
|---|---|
| Repository integration | Member/admin/legacy/unresolved provenance; multiple active users; disabled users; blocked projects; no attempts; older failure/newer success; equal timestamps with different outcomes; rows equal totals. |
| Go unit/handler | Table-driven identity resolution and invalid filter tests; exact JSON contract contains `degraded_projects` and omits `conflicts`; both overview shapes agree. |
| Dashboard unit/integration | API query serialization; `N / total`; direct/refreshed/shared degraded URL; visible All/Degraded links with `aria-current`; popstate back/forward reload; unauthorized omission; accessible rows and empty state; no nested controls. |

## Threat Matrix

| Boundary | Applicability | Reason |
|---|---|---|
| Documentation-like paths | N/A | No executable-file classification. |
| Git repository selection | N/A | No VCS execution. |
| Commit state | N/A | No commit automation. |
| Push state | N/A | No push automation. |
| PR commands | N/A | No PR automation. |

Browser query routing is covered by the Dashboard RED tests above; it does not cross a shell/process boundary.

## Migration / Rollout

Run the additive migration first, then deploy the coordinated API/Dashboard contract in the same release. Unresolved legacy rows remain queryable but nonparticipating. Roll back code together; columns and audit data may remain safely.

## Open Questions

None.
