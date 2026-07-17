# Design: Guarded Memory Delete and Restore

## Technical Approach

Extend the existing loopback governance boundary rather than bypassing it: `jarvis hive` orchestrates preview → backup → revalidation → guard through `hiveclient`; the daemon atomically owns identity checks, soft mutation, audit journal, idempotency, and sync state. MCP remains read/create-only.

## Architecture Decisions

| Decision | Choice | Rejected / rationale |
|---|---|---|
| Mutation API | Enrich existing backup and guard routes; add narrow capability and mutation-receipt reads | A composite workflow duplicates established boundaries; current composition is safe when identity is rechecked client-side and transactionally daemon-side. Existing reads cannot negotiate safety or reconcile response loss. |
| State model | Receipt exposes `local=committed` and shared `pending/completed/failed_retryable/legacy_unsupported`, keyed by request ID, event ID, project, local ID, entity `sync_id`, and operation | Aggregate health or `memories.synced_at` cannot identify a particular mutation. |
| Causality | Journal events stay ordered by sequence. A never-attempted pending create followed by delete is retained for audit but suppressed from dispatch; shared status is completed (`not_required`). If delivery may have started, v2 applies ordered idempotent events in one server transaction, exposing no intermediate create | Deleting journal history loses audit; legacy row-state cannot safely propagate tombstones. |
| Compatibility | `GET /governance/capabilities`; missing endpoint or incomplete guard contract disables actions. Persist observed API compatibility per project; legacy sync reports `legacy_unsupported` without claiming completion | Version-string guessing and silent fallback weaken safety. |
| Deleted data | Add mutually exclusive `deleted_only` filtering and deleted-aware detail lookup; keep separate TUI slices/screens | Partitioning one mixed UI slice risks tombstone leakage. |

## Data Flow

    TUI → capabilities → target preview → POST backup → target re-read
     │                                             │
     └→ POST guard(expected identity, request_id, reason)
                          ↓
             DB transaction: identity CAS + soft mutation + journal/receipt
                          ↓
              sync-v2 retry/ack → receipt refresh → TUI

On backup expiry the TUI creates one replacement, re-reads project/local ID/`sync_id`, and proceeds only on exact equality. A lost POST response triggers receipt lookup, never a new request ID. Repeating the same request ID returns the stored receipt; reusing it with different input conflicts. Restore requires a trimmed reason and stores actor/reason in the journal payload.

## File Changes

| File | Action | Description |
|---|---|---|
| `jarvis-cli/internal/hiveclient/client.go` | Modify | Backup create, capabilities, deleted-only/detail, enriched guard, receipt DTOs. |
| `jarvis-cli/internal/hiveui/{runner.go,model.go}` | Modify | Separate Active/Recently Deleted state, automatic backup/revalidation, duplicate-submit lock, reconciliation/status UX. |
| `jarvis-cli/cmd/jarvis/cmd_hive.go`, `jarvis-cli/internal/agent/startscript.go` | Verify/tests | Prove managed `jarvis hive` and installer-first daemon resolution. |
| `hive-daemon/internal/{governance,httpapi}` | Modify | Capability/receipt reads and strict enriched guard validation; preserve loopback routing. |
| `hive-daemon/internal/db/{db.go,memory.go,project.go,sync.go}` | Modify | Additive journal attempt/idempotency fields, atomic expected-identity mutation, deleted-only/status reads, restore audit. |
| `hive-daemon/internal/sync/{syncer.go,syncwrite.go}`, `hive-api/internal/{model,service,repository}` | Modify | Suppression, attempt/failure recording, v2 reason/ack correlation, stale-event protection; legacy truth. |
| Existing `*_test.go` files in these packages and `hive-daemon/internal/mcp/server_test.go` | Modify | RED-first behavioral coverage and destructive-tool exclusion. |

## Interfaces / Contracts

`GuardRequest` gains `expected_project`, `expected_sync_id`, and a standard-library-generated opaque `request_id`; `GuardResult` returns a `MutationReceipt`. `GET /governance/mutations/{request_id}` verifies target identity before returning it. Journal rows gain nullable `request_id` (unique), `attempted_at`, `attempt_count`, `last_error`, and `suppressed_at`; existing `synced_at` is the acknowledgement. Errors are sanitized and reasons/content are never logged or returned by capability/status endpoints.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | CAS identity, reason, expiry renewal, state derivation, suppression/idempotency | Table-driven Go tests; direct Bubble Tea `Model.Update`. |
| Integration | Atomic mutation+journal, response loss, duplicate request/event, v2 ack/failure/retry, stale resurrection, legacy mode | SQLite/API/HTTP tests with injected failures and `t.TempDir()`. |
| Acceptance | Installed `jarvis hive`, installer-first path, separate screens, MCP exclusion | Stub daemon/TUI plus managed-path fixtures; no build. |

## Threat Matrix

| Boundary | Applicability | Reason |
|---|---|---|
| Documentation-like paths | N/A | No file execution/classification change. |
| Git repository selection | N/A | No VCS command. |
| Commit state | N/A | No VCS mutation. |
| Push state | N/A | No VCS mutation. |
| PR commands | N/A | No PR automation. |

HTTP route additions remain loopback-only and receive loopback/method/body-limit regression tests.

## Migration / Rollout

Use additive SQLite migrations and nullable defaults; old rows derive status from `synced_at` and project compatibility. Roll out daemon/API before CLI capability enablement. Observe sanitized per-event attempts, ack latency, suppression count, and reconciliation outcomes. Rollback disables UI capabilities/routes while retaining tombstones and journal columns; no down-migration or data deletion.

## Open Questions

None.
