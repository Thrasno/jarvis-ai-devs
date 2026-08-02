## Exploration: distributed-project-quarantine

### Current State

Issues #474–#477 form one ordered initiative: #474 corrects the existing write contract, #475 adds a reversible distributed lifecycle, #476 adds account-based operational read models, and #477 adds the dashboard governance surface. GitHub issue bodies were read live from `Thrasno/jarvis-ai-devs`; all four currently have no comments. RDD remains disabled as requested.

The current implementation is a one-way, project-sync-discovered block:

- Hive API accepts `quarantine` and `purge_intent`; `ProjectBlockRequest` requires arbitrary non-empty `export_marker`. `project_governance.go` persists the marker and includes it in audit metadata. The handler exposes admin block/status routes and an authenticated daemon ACK route. Status returns one globally latest ACK and redacts command secrets for non-admin reads.
- PostgreSQL migration `012_project_blocks.sql` stores `export_marker`, a single blocked row per canonical project, and ACKs. Migration `013_project_block_ack_subjects.sql` changes ACK uniqueness to command/project/auth-subject, which is the foundation for account-level aggregation but does not yet model generations or lifecycle transitions.
- The daemon discovers a block through normal project sync/HTTP 423, persists it in SQLite, archives/hides local data without deleting rows/files, queues and retries an ACK, and cannot use the blocked project sync path to discover release. Local `project_blocks` has one current command and ACK state, not an inbox, generation history, or reversible archive state.
- The dashboard exposes block controls inside operational Projects. `src/api/client.ts`, `Projects.ts`, `main.ts`, and their tests send/require `export_marker` and expose `purge_intent`; there is no durable Quarantine Center route or release flow. `/projects` itself excludes blocked projects.
- Existing governance project read models in the daemon are local archive/memory views and are not a substitute for the proposed Hive API quarantine read models.

### Affected Areas

- `hive-api/internal/model/project_block.go` — split new-write contracts from historical audit/read contracts; remove marker and purge action from new requests; add explicit lifecycle/action/generation/ACK semantics.
- `hive-api/internal/service/project_governance.go` — preserve transactional canonical-project locking and audit guarantees while implementing BLOCK/UNBLOCK transitions, immediate cloud release, and separate local convergence.
- `hive-api/internal/handler/project_governance.go`, `hive-api/internal/handler/router.go`, auth middleware — add admin-only inbox and quarantine list/detail/release endpoints; retain authenticated daemon delivery/ACK routes and HTTP 423 fallback.
- `hive-api/internal/repository/postgres_project_block.go`, repository interfaces/mocks, user repositories, and `hive-api/migrations/012_project_blocks.sql`, `013_project_block_ack_subjects.sql` — retain historical rows, allocate monotonic generations transactionally, query latest ACK per active account, and add indexes based on query plans without breaking rolling deployments.
- `hive-api/internal/model/audit.go` and audit persistence — remove `export_marker` from new audit payloads while keeping historical metadata readable and preserving actor, timestamp, action, reason, canonical project, and resulting state.
- `hive-daemon/internal/sync/syncer.go`, `hive-daemon/internal/sync/client.go` — add authenticated general governance-inbox polling before sync/drain at startup and after reconnect; apply BLOCK/UNBLOCK idempotently by generation; ACK stale/duplicate/current commands deterministically; retain 423 defense-in-depth and retry pending ACKs.
- `hive-daemon/internal/db/project_block.go` and archive/persistence code — evolve from one current block to durable command/generation/convergence state, reversible archive/unarchive, and retained SQLite rows/repositories/files. Delayed BLOCK data must reconcile normally after release.
- `hive-dashboard/src/api/client.ts`, `src/main.ts`, `src/views/Projects.ts`, `src/domain/dashboard.ts`, route/navigation components, fixtures, and tests — remove marker/purge terminology from new contracts and add admin-only `/dashboard/quarantine` list/detail/release flows with generation-aware polling and route/filter preservation.
- `hive-api/internal/handler/project.go` and existing `/projects` behavior — keep operational active-project semantics separate from persistent quarantine records; do not make the governance view depend on `/projects`.
- Protocol/API documentation and mixed-version tests — document old-client behavior, unsupported `purge_intent` 4xx responses, tolerated legacy marker fields, old daemon safe-blocked behavior, bounded compatibility windows, and rollback.

### Approaches

1. **Incremental additive protocol with historical compatibility** — first remove misleading new-write fields while preserving historical reads, then add generation-based governance inbox and reversible lifecycle, then add active-user read models, and finally ship the dashboard against feature-detectable endpoints.
   - Pros: matches the issue dependency order; supports rolling mixed versions; preserves audit readability; gives each phase a reversible migration and verification boundary; keeps `/projects` and existing 423 behavior stable.
   - Cons: temporarily requires dual contracts and compatibility branches; more migrations and integration tests; old daemons cannot release until upgraded.
   - Effort: High

2. **Replace project-block protocol in one coordinated cutover** — replace the current block table/API/daemon flow with the final lifecycle and dashboard contract at once.
   - Pros: fewer long-lived compatibility branches and a single conceptual model.
   - Cons: unsafe for rolling deployments and offline daemons; risks unreadable historical `purge_intent` rows, broken 423 clients, and cloud release deadlock; large review and rollback surface.
   - Effort: Very High

### Recommendation

Use Approach 1, with #474 as a hard contract-cleanup/migration prerequisite and #475→#476→#477 as additive slices. Keep historical `export_marker` and `purge_intent` values readable but exclude them from new request/audit contracts; reject new `purge_intent` explicitly rather than coercing it. Model authoritative cloud lifecycle separately from per-account convergence. Allocate and compare a monotonic generation under canonical-project transactional locking, make daemon application idempotent and non-destructive, and deliver commands through an account-authenticated inbox independent of project sync. Compute #476’s denominator from `users.is_active=true` at query time and synthesize `pending` only for missing current-generation ACKs. The dashboard should consume only admin-authorized list/detail/release APIs and describe pending as “No ACK received,” never offline.

Operational rollout should be: schema compatibility and #474 writes/read preservation; API final lifecycle/inbox with old 423 and safe old-daemon behavior; read models; dashboard feature detection. Rollback must preserve old columns/history, stop new writes safely, and avoid reverting an already audited cloud release into a block merely because local convergence is incomplete.

### Risks

- A migration that drops `export_marker` or constrains actions immediately can break rolling binaries or historical audit reads.
- Treating `purge_intent` as an alias would silently mutate semantics; old clients need an explicit unsupported-action 4xx.
- Generation allocation/comparison races can let delayed BLOCK or UNBLOCK override newer authoritative state; use canonical-project locks and transactionally allocated revisions.
- Release cannot depend on every daemon being online; old daemons remain safely blocked and must be surfaced as pending convergence, not cloud failure.
- Archive reversal may expose local state created by delayed commands; retain rows/files and reconcile through normal sync with explicit generation checks.
- ACK aggregation must use active accounts, stable latest-row tie-breakers, consistent snapshots, and no auth subjects/tokens in admin responses or logs.
- Polling and stale dashboard responses can overwrite newer lifecycle generations; bound polling and apply generation-aware response guards.
- The current code has limited direct coverage around `ProjectGovernanceService`, handler wiring, `handleProjectBlocked`, and release; tests must span repository, migration, authorization, startup/reconnect ordering, races, mixed versions, and the full BLOCK→progress→UNBLOCK flow.
- `openspec/config.yaml` is absent in the current checkout; later SDD phases should apply repository Go/TypeScript conventions and explicit phase rules without inventing a conflicting project config.

### Ready for Proposal

Yes. The initiative is sufficiently understood for proposal/spec/design. The proposal should preserve the strict order #474→#475→#476→#477, define the mixed-version and rollback windows explicitly, and treat historical audit readability as a non-negotiable compatibility invariant. No production implementation was performed in this phase.
