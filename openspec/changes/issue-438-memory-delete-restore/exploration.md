## Exploration: issue-438-memory-delete-restore

### Current State

Issue #438's central diagnosis is confirmed: the daemon already owns a loopback governance boundary for backup listing/creation, memory listing/detail, and guarded delete/restore. `governance.Service.ExecuteGuard` requires a positive memory ID, operation-specific confirmation, a fresh backup (10-minute freshness), and a non-empty delete reason; the DB performs soft tombstone/restore updates and journals each mutation transactionally by `sync_id`. Normal governance reads exclude tombstones unless `include_deleted=true`.

The installed `jarvis hive` path is incomplete. `cmd/jarvis` resolves the daemon URL and starts `hiveui.RunHiveTUI`; the runner loads projects, active-only memories, health, warnings, backups, and aggregate sync summary. `hiveclient` has `Backups` and `ExecuteGuard`, but no backup-create method. The TUI's existing memory guard asks for a pasted backup ID, then reason/confirmation, and has no first-class deleted-memory collection or Recently Deleted screen. The standalone `hive` command can request `include_deleted=true` and dispatch guards, but also requires a pre-existing backup and is not the canonical installed surface.

Modern sync-v2 carries mutation envelopes, cursors, delete tombstones, restore operations, and acknowledgements. The syncer distinguishes `mutation-sync-v2` from `legacy-row-state`; legacy mode handles row-state synchronization and cannot represent equivalent tombstone propagation. Local mutation state exposed to the client is currently insufficient for per-memory pending/acknowledged/unsupported presentation: `Memory` has `sync_id` but no `synced_at`, mutation status, or compatibility mode. The daemon's aggregate health/summary is not a per-target propagation receipt.

The daemon HTTP layer already exposes `POST /governance/backups`, so a new daemon endpoint is not presently required merely to create backups. Governance is loopback-only, while MCP registration tests explicitly ban delete names and the guarded destructive request shape. Installer/runtime resolution is source-backed and resolves the managed daemon path before PATH and legacy Go locations; `jarvis hive` itself defaults to the configured loopback URL and does not independently manage daemon lifecycle.

### Affected Areas
- `jarvis-cli/internal/hiveclient/client.go` — add the missing client operation and likely explicit target/sync/propagation response models; preserve API error behavior.
- `jarvis-cli/internal/hiveui/runner.go` — load active and tombstoned data intentionally, without mixing deleted rows into active collections; refresh after mutation.
- `jarvis-cli/internal/hiveui/model.go` and tests — extend the Bubble Tea state machine, identity preview, automatic backup step, Recently Deleted navigation, restore, duplicate-submit prevention, and result/error stages.
- `jarvis-cli/cmd/jarvis/cmd_hive.go` — verify the installed canonical entry point and daemon URL/runtime assumptions; no standalone CLI canonization is warranted by current evidence.
- `hive-daemon/internal/governance/service.go` — existing guard and freshness contract is reusable; only response/status enrichment should be considered if the current boundary cannot expose required evidence.
- `hive-daemon/internal/httpapi/server.go` — existing backup POST and guard routes are sufficient foundations; compatibility/error mapping must remain actionable.
- `hive-daemon/internal/db/memory.go`, `db/sync.go`, and mutation tests — preserve exact local-ID/project/sync-ID checks, transactional tombstones, and causal journal ordering.
- `hive-daemon/internal/sync/syncer.go`, `syncwrite.go`, and tests — define unsynced-create-then-delete ordering, v2 acknowledgement/retry, remote apply, and explicit legacy limitation.
- `hive-daemon/internal/mcp/server_test.go` and MCP registration — retain the human-only least-privilege boundary; no destructive MCP route is indicated.
- Installer/runtime tests under `jarvis-cli/internal/agent` — verify acceptance through the managed `jarvis hive` path rather than only a standalone binary.

### Confirmed, Corrected, and Uncertain Claims from #438
- **Confirmed:** daemon backup create/list and guarded memory delete/restore exist; fresh-backup and exact-confirmation enforcement is daemon-side; delete reason is trimmed/required; mutations are soft, transactional, and journaled; normal reads hide tombstones; v2 and legacy sync modes differ; `hiveclient` lacks backup creation; installed TUI loads active-only memories and requires a pasted backup ID; standalone `hive` is non-canonical and has the same backup gap; MCP destructive tools are excluded and tested; installer-managed daemon resolution exists.
- **Corrected/qualified:** issue wording that “completion output” already exposes complete shared propagation semantics is not true of the current client/TUI model. The current implementation exposes aggregate health and sync summary, not a target-specific mutation receipt. Also, the HTTP governance route is already capable of backup creation, so the gap is client/UI orchestration, not necessarily daemon API creation.
- **Uncertain:** exact per-memory sync-state semantics; whether `synced_at` alone is enough or journal/compatibility data must be added; whether unsynced create plus delete emits both events or may compact safely; behavior when backup succeeds but target changes before guard; behavior after local commit when the daemon/client loses connection; how older daemons should be detected; whether restore should preserve historical delete reason in user-visible audit data; and the desired retry/idempotency contract for repeated submissions.

### Approaches
1. **Compose existing backup and guard boundaries in `jarvis hive`** — create/validate a backup through the existing governance route, then execute the existing guard after revalidating the target identity.
   - Pros: smallest architectural change; preserves daemon ownership, audit, transactions, MCP boundary, and compatibility behavior.
   - Cons: backup-to-guard is a race window; accurate propagation status may require response or read-model enrichment.
   - Effort: Medium/High

2. **Add a daemon composite backup-plus-guard operation** — make backup creation, target revalidation, and mutation one daemon-coordinated workflow.
   - Pros: narrows stale-preview and backup-expiry races; can return a correlated local mutation receipt.
   - Cons: expands governance API and compatibility surface; does not by itself solve sync-v2/legacy status semantics or TUI modeling.
   - Effort: High

### Recommendation
Start proposal/design from Approach 1, but make the race analysis an explicit gate. Reuse the existing `POST /governance/backups` and guard boundary unless evidence shows that a safe target revalidation plus fresh-backup policy cannot satisfy the invariants. Treat per-memory propagation state and unsynced-create-then-delete ordering as product/spec decisions before implementation, not as incidental UI fields. Keep active and deleted collections separate and preserve MCP least privilege.

### Risks
- A stale snapshot or duplicate submit could target a changed/deleted row unless project, local ID, and `sync_id` are revalidated at execution.
- Local tombstone success can be incorrectly presented as shared success, especially after transport loss or under legacy sync.
- Backup creation can succeed while guard execution fails or expires; backup retention and retry messaging need clear semantics.
- Loading `include_deleted` into active collections could expose tombstones or offer the wrong action.
- Unsynced create/delete ordering can resurrect a stale record if mutation causality is not preserved.
- Older daemons may lack required capabilities; unsafe degradation must be prohibited.
- Installer acceptance can pass for a built standalone binary while the installed `jarvis hive` runtime remains broken.

### Ready for Proposal
Yes. The architecture and main gaps are verified. The proposal should first resolve the per-memory sync-state vocabulary, unsynced-create-then-delete causal contract, backup-to-guard race policy, older-daemon behavior, and restore audit-display semantics before committing to an endpoint shape.
