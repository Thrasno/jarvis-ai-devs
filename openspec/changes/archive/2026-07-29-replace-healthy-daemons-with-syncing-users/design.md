# Design: Replace Healthy Daemons with Syncing Users

## Technical Approach

Replace the device aggregate with one canonical user projection over retained `sync_attempt_logs.portal_user_id`. Both Overview and User Management consume the same repository facts and the same service mapping rules. Each request captures `now := time.Now().UTC()` once and passes it through; activity chronology uses `ended_at` only.

## Architecture Decisions

| Decision | Alternatives | Rationale |
|---|---|---|
| One `UserSyncProjection(ctx, now)` for both surfaces | Separate KPI and user-context SQL queries | One set-based query returns every user ID, account activity, latest completed attempt, and latest successful completion. Overview aggregates rows; Admin merges them with `UserRepository.List`. This keeps one semantic source while remaining bounded: one query for Overview, two for Admin, regardless of user count. A projection containing full users was rejected because it would cross repository boundaries and risk exposing persistence fields. |
| Map statuses in a shared service helper | Encode UI statuses in SQL or duplicate mapping | Repository rows remain facts. `userSyncStatus(row, now)` applies precedence: inactive account → `inactive`; active with no retained success → `never`; otherwise latest completed success in inclusive `[now-24h, now]` → `last_24h`; all other cases → `inactive`. `last_sync_at` independently uses latest successful `ended_at`. Projection failure is operational `unknown`, rendered `Unavailable`, not a fourth successful-projection status. |
| Add a projection index migration | Reuse migration 015 index | Migration 015 starts with `project`, but this projection partitions globally by `portal_user_id`; PostgreSQL cannot efficiently use that leading key. Migration 016 adds a partial `(portal_user_id, ended_at DESC, ingested_at DESC, attempt_id DESC, id DESC) INCLUDE (outcome)` index for completed canonical attempts. No data migration or summary column is added. |
| Stable surface-specific DTOs | Serialize `model.User` directly | `AdminUserResponse` preserves current user fields and adds only `sync_status` and nullable `last_sync_at`; auth DTOs remain unchanged. Overview replaces `daemon_health` atomically with `syncing_users: { syncing, total }`. |

## Data Flow

```text
GET /overview (admin) ─┐
                      ├─ captured UTC now → SyncAttemptRepository.UserSyncProjection
GET /admin/users ─────┘                         │
        └─ UserRepository.List ── merge by ID ──┘ → explicit DTOs → dashboard
```

The SQL ranks only rows with non-null canonical identity and `ended_at`. Latest ordering is `ended_at DESC, ingested_at DESC, attempt_id DESC, id DESC`; tie-breakers are deterministic but never substitute `started_at`. A grouped `MAX(ended_at) FILTER (WHERE outcome='success')` supplies latest success, and a left join from `users` retains users without attempts.

## File Changes

| Files | Action | Description |
|---|---|---|
| `hive-api/internal/model/{sync_attempt,overview,response}.go` | Modify | Add projection facts, status constants/admin DTO; replace `daemon_health` DTOs. |
| `hive-api/internal/repository/{sync_attempt,postgres_sync_attempt,mock_sync_attempt}.go` | Modify | Add one projection method; remove `DaemonHealth`. |
| `hive-api/internal/service/{user_sync,overview,admin}.go` | Create/Modify | Shared mapping, KPI aggregation, complete user merge, captured clock. |
| `hive-api/internal/handler/{admin,router}.go`, `hive-api/cmd/server/main.go` | Modify | Return admin DTO envelope, update interfaces/wiring; preserve middleware. |
| `hive-api/migrations/{016_sync_attempt_user_projection.sql,migrations.go}` | Create/Modify | Add and embed the evidenced index. |
| `hive-dashboard/src/{api/client,domain/dashboard,main,views/Overview,views/Users}.ts`, `hive-dashboard/src/styles.css` | Modify | Const-derived sync enum, contract mapping, `SYNCING USERS · 24H`, account/sync/Last sync columns, and global styling; null renders `Never`. |
| Corresponding Go repository/service/handler/router tests and dashboard `*.test.ts`/fixtures | Modify | Replace daemon assertions and add required scenarios. |

## Interfaces / Contracts

```go
type UserSyncProjectionRow struct {
    PortalUserID string
    IsActive bool
    LatestEndedAt *time.Time
    LatestOutcome *SyncAttemptOutcome
    LatestSuccessEndedAt *time.Time
}
UserSyncProjection(ctx context.Context, now time.Time) ([]UserSyncProjectionRow, error)
```

Wire contracts: `operations.syncing_users` and admin stats `syncing_users` are `{ "syncing": number, "total": number }`; `/admin/users` remains `{ "users": [...] }`, with each row adding `sync_status` (`last_24h|inactive|never`, or operational `unknown` rendered `Unavailable` on projection failure) and `last_sync_at` (`RFC3339|null`). `daemon_health` is removed, not aliased.

## Testing Strategy

Strict RED-GREEN-REFACTOR: first add table-driven service tests for shared-now boundaries, latest-failure, future/incomplete attempts, inactive precedence, and retained-success mapping. PostgreSQL integration tests prove canonical-only identity, deterministic ties, all-user left join, empty projection, and one SQL call; service mocks prove Admin uses exactly two repository calls and no N+1. Handler/router tests prove DTO shape, daemon-field removal, admin denial, and member Overview omission. Vitest contract/mapping/view tests prove exact KPI copy, `0 / 0` without percentage, enums, `Never`, and separate account/sync columns. Run narrow packages first, then `go test ./...`, `go vet ./...`, and dashboard tests/lint.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable classification, or process-integration boundary changes; existing routes and middleware remain.

## Migration / Rollout

Deploy migration 016 before/with the API, then dashboard assets atomically because `daemon_health` is replaced. Roll back API/dashboard together; the additive index may remain or be dropped later. No data rollback is required.

## Open Questions

None.
