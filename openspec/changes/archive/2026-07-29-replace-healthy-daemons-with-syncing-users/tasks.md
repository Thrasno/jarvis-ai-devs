# Tasks: Replace Healthy Daemons with Syncing Users

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 650–900 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 backend projection; PR 2 API/dashboard contract and views; PR 3 cleanup and verification |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes — resolved by maintainer reset: `exception-ok`, same single PR, final unit hard 500 changed-line budget, cumulative 1600 changed-line limit.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Migration, projection, shared mapping | PR 1 | `go test ./hive-api/internal/{repository,service}` | N/A: set-based repository/service tests | migration 016 and projection/service files |
| 2 | API DTOs, wiring, auth, dashboard | PR 2 | `go test ./hive-api/internal/{handler,service} && npm test -- --run` | N/A: HTTP/DOM contract tests | API DTO/wiring and dashboard files |
| 3 | Remove daemon-health residue and full proof | PR 3 | `go test ./... && go vet ./...` | N/A: no new process boundary | stale-health cleanup and tests |

## Phase 1: Canonical Projection Foundation

- [x] 1.1 RED: add repository/service table tests for latest-failure, inclusive boundary, future/incomplete attempts, unresolved identity, inactive precedence, retained success, and no-success `Never`; assert one projection query and deterministic ties.
- [x] 1.2 GREEN: add `UserSyncProjectionRow` and `UserSyncProjection` to `hive-api/internal/model` and `repository/{sync_attempt,postgres_sync_attempt,mock_sync_attempt}.go`; add migration `016_sync_attempt_user_projection.sql` and embed it in `migrations.go`.
- [x] 1.3 RED→GREEN→REFACTOR: create `hive-api/internal/service/user_sync.go`; map shared captured UTC `now`, latest-success `last_sync_at`, and `last_24h|inactive|never` without per-user queries.

## Phase 2: API and Dashboard Contracts

- [x] 2.1 RED→GREEN: update `model/{overview,response}.go`, `service/{overview,admin}.go`, `handler/{admin,router}.go`, and `cmd/server/main.go`; replace `daemon_health` atomically, merge users with exactly two repository calls, preserve admin middleware, and add handler/router tests for DTO shape, member denial, and event omission.
- [x] 2.2 RED→GREEN→REFACTOR: update dashboard `src/{api/client,domain/dashboard,main,views/Overview,views/Users}.ts` and `src/styles.css`; use const-derived sync status, exact KPI copy and `0 / 0`, separate account/sync columns, and null `Never`; add contract, mapping, Overview, Users, and empty-state tests.

## Phase 3: Cleanup and Verification

- [x] 3.1 RED→GREEN: remove `DaemonHealth` interface/SQL/mock usage and stale daemon-health assertions; prove `daemon_health` is absent while account/role/Audit Log behavior remains unchanged.
- [x] 3.2 REFACTOR: review rollback boundary (API/dashboard together; additive index may remain), run focused tests, then `go test ./...`, `go vet ./...`, and dashboard tests/lint.
