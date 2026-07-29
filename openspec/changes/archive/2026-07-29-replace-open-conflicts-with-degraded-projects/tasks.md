# Tasks: Replace Open Conflicts with Degraded Projects

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 950–1,250 authored lines |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 migration/identity; PR 2 projection/API; PR 3 Dashboard |
| Delivery strategy | single-pr |
| Chain strategy | pending maintainer decision |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Persist/resolve portal identity | PR 1 | `go test ./hive-api/internal/{service,handler,repository}` | `go test` integration DB | migration and sync-attempt identity files |
| 2 | Build canonical projection/API contract | PR 2 | `go test ./hive-api/internal/{repository,service,handler}` | `go test` HTTP contract suite | projection, overview, project API files |
| 3 | Ship Dashboard discovery UX | PR 3 | `npm test -- --runInBand` (dashboard) | `npm run test` browser-like harness | dashboard API/domain/view files |

## Phase 1: Identity Persistence (PR 1)

- [x] 1.1 RED: add repository/service tests for member `auth_subject`, admin exact-email `admin_dev_id`, unresolved admin, and `legacy_email` backfill; assert device metadata is never trusted.
- [x] 1.2 GREEN: add migration `hive-api/migrations/015_sync_attempt_portal_users.sql`, register it, and persist nullable `portal_user_id` plus constrained `portal_user_source` with exact-email backfill/index.
- [x] 1.3 GREEN: update `hive-api/internal/{model,service,handler}/sync_attempt.go` and repositories to resolve authenticated actors and write provenance; refactor only after tests pass.

## Phase 2: Canonical Projection and API (PR 2)

- [x] 2.1 RED: add PostgreSQL projection tests for latest-per-active-user, older failure/newer success, equal-timestamp deterministic ordering, multiple users, disabled users, blocked projects, no attempts, and rows/totals agreement.
- [x] 2.2 GREEN: implement `ProjectSyncHealthProjection` in `hive-api/internal/repository/{sync_attempt,postgres_sync_attempt,project,postgres_project}.go`, excluding nonqualifying rows and device identity.
- [x] 2.3 RED: add handler/service tests for `degraded_projects`, omission of `conflicts`, both overview shapes, `health=degraded`, unsupported health `400`, and authorization boundaries.
- [x] 2.4 GREEN: wire projection into `hive-api/internal/{service,handler}/overview.go` and `project.go`; update models and canonical project summaries/filtering.

## Phase 3: Dashboard Contract and Routing (PR 3)

- [x] 3.1 RED: add API/domain tests for contract mapping, `N / total`, nullable nonparticipant health, query serialization, direct/refresh/shared URLs, and popstate restoration.
- [x] 3.2 GREEN: rename types in `hive-dashboard/src/api/{client,urlFilters}.ts` and `src/{main,domain/dashboard}.ts`; reload Projects when URL health changes.
- [x] 3.3 RED: add view/app tests for exact `DEGRADED PROJECTS`, visible All/Degraded links with `aria-current`, accessible rows, unauthorized omission, no nested controls, and explicit empty state.
- [x] 3.4 GREEN: update `hive-dashboard/src/views/{Overview,Projects}.ts`, fixtures, and app wiring; preserve event wording as historical events.

## Phase 4: Verification

- [x] 4.1 Run focused Go and Dashboard suites, then `go test ./...` and `go vet ./...`; inspect API snapshots for no `conflicts.open`/`openConflicts` alias.
- [x] 4.2 Confirm migration-first rollout and coordinated API/Dashboard rollback; if single-PR delivery is retained, obtain explicit `size:exception` approval before apply.
  - Attempt 15 (`sha256:0cbc80f822e19961837030fd6056a662ce44ecaf4964bb0e39bf539863984526`): maintainer explicitly approved `size:exception`; historical RED commands/output were unavailable and are not reconstructed.
