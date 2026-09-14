## Exploration: issue-648-lazy-session-materialization

### Scope and authority

Issue #648 is an approved implementation. Its requirements, as supplied for this phase, are authoritative: provide lazy, idempotent, conflict-aware and self-healing Hive session materialization through the local DB/store, HTTP, MCP, and required Hive API sync paths; reopen ended sessions; preserve existing project-validation and migration gates; add bounded fail-open OpenCode `session.created`/`session.deleted` registration in the embedded source template; and use strict TDD. Session identity is canonical project equality only: directory is provenance, and `dev_id`/`client` are attributes. The same session ID bound to a different canonical project MUST return typed `project_session_mismatch`; directory variants within the same canonical project are compatible. Reopen MUST clear `ended_at` and reset local `synced_at` so Hive API receives the transition. No external research lane is selected.

Non-goals are equally binding: no generated user-machine edits, no resident Core/autostart behavior, no idempotency-key scheme, and no FK/schema expansion.

### Current state

- `hive-daemon/internal/db/session.go` is the local persistence chokepoint. `CreateSession` registers the project identity, rejects non-writable/quarantined projects, fills an empty `dev_id`, then performs a plain `INSERT`. A repeated ID is therefore a duplicate error rather than materialization. `EndSession` sets `ended_at` and summary but has no reopen path.
- `EnsureManualSaveSession` already demonstrates the desired lazy/idempotent pattern for its sentinel only: it passes project registration and writable-project checks before `INSERT OR IGNORE`. It does not apply to regular OpenCode/MCP session IDs.
- `hive-daemon/internal/httpapi/server.go` exposes `POST /sessions` and `POST /sessions/{id}/end`. The create handler validates the effective project through `project.ValidateWriteProject` (including directory derivation) and currently maps a duplicate-key error to HTTP 200. That preserves retry success but cannot repair/reopen an existing ended row or distinguish a semantic conflict. The end route delegates to `EndSession`.
- `hive-daemon/internal/mcp/tools.go` exposes `mem_session_start` and `mem_session_end`. Start uses the same write-project validation but directly calls `CreateSession`; end explicitly rejects an already-ended session before calling `EndSession`. Both are migration-gated by the normal MCP registration wrapper. Save/summary paths already use the manual-save fallback when no session ID is supplied, while explicit sessions retain lifecycle validation.
- Migration and project safety are already defended at several layers. Local `CreateSession` and `EnsureManualSaveSession` register identity and call `ensureProjectWritable`; HTTP and MCP create paths call `ValidateWriteProject`; migration-route tests cover write gating. The new materialization path must reuse rather than bypass these checks.
- The existing sync path makes Hive API an implementation surface. `hive-daemon/internal/sync/syncer.go` sends `ListUnsyncedSessionsPage` rows, then marks them synced after a successful batch; resetting local `synced_at` on reopen therefore requeues the session. `hive-api/internal/service/sync.go` forwards each session to `Session.UpsertSession` and returns session pulls before memories. However, `hive-api/internal/repository/postgres_session.go` currently leaves `ended_at` unchanged on a regular-session `sync_id` conflict, so an incoming reopened row (`ended_at=NULL`) would not heal the remote session. The existing regular-session sync/upsert and pull paths must be extended and tested to accept the reopen transition without a schema or FK expansion.
- `jarvis-cli/embed/hooks/opencode/hive.ts` is the source template installed by `OpenCodeAgent.InstallPromptHook` into `~/.config/opencode/plugins/hive.ts`. It currently registers only `chat.message` prompt capture, with loopback `fetch`, a 1-second `AbortSignal.timeout`, and fail-open error handling. It does not yet handle OpenCode lifecycle events. This embedded asset—not a generated user file—is the correct edit surface.

### Affected areas

1. `hive-daemon/internal/db/session.go` and `session_test.go`
   - Introduce/reuse one regular-session materialization operation that creates absent rows, accepts same-canonical-project repeats (including directory variants), reopens an ended row, clears its `ended_at`, and resets its local `synced_at` to requeue it for sync.
   - Reject a same-ID row bound to a different canonical project with typed `project_session_mismatch`; directory is provenance and `dev_id`/`client` remain non-identity attributes.
   - Preserve `registerProjectIdentity`, `ensureProjectWritable`, empty-dev-ID healing, sentinel behavior, and existing stale-session policy.
2. `hive-daemon/internal/httpapi/server.go`, `sessions_test.go`, and existing gate/autoregistration tests
   - Route `POST /sessions` through the materialization contract after current validation; maintain bounded bodies and correct HTTP mapping for validation, conflict, and store failure.
   - Keep `/sessions/{id}/end` semantics and migration gating intact.
3. `hive-daemon/internal/mcp/tools.go`, `mcp/server.go`, and `tools_test.go`
   - Route `mem_session_start` through the same materialization contract; ensure repeats reopen ended sessions rather than erroring.
   - Keep `mem_session_end`'s protection against duplicate end events and retain MCP migration gating.
4. `jarvis-cli/embed/hooks/opencode/hive.ts` plus source-asset/installer tests under `jarvis-cli/internal/agent` (and, where appropriate, `internal/hook`)
   - Add only bounded, loopback, fire-and-forget session-created/session-deleted notifications using the existing resolved session/project/directory inputs and HTTP session endpoints.
   - Verify the shipped source asset and installation path; do not edit `~/.config/opencode/plugins/hive.ts`.
5. `hive-daemon/internal/sync/syncer.go` and sync tests plus `hive-api/internal/service/sync.go`, `internal/repository/postgres_session.go`, and repository/service tests
   - Reopen is a required lifecycle transition: local materialization must reset `synced_at` so the existing unsynced-session batch sends it, and Hive API's regular-session upsert must accept `ended_at=NULL` and make the healed state discoverable through the existing session pull path.
   - Preserve session-before-memory ordering, project relocation/quarantine protections, sentinel behavior, and current cursor/paging semantics; do not add foreign keys, schema columns, or an idempotency-key protocol.

### Key invariants and test matrix

Strict TDD should begin with focused failing tests, then minimum implementation, then refactor. Tests should prove behavior at the smallest boundary first and retain existing safety regressions.

| Boundary | RED scenarios to add | Required preserved behavior |
| --- | --- | --- |
| Local DB/store | first observation creates one regular session; same canonical-project repeat leaves one row and succeeds; same-project directory variants succeed; `dev_id`/`client` variants do not conflict; repeat after `EndSession` clears `ended_at` and resets `synced_at`; different canonical project returns typed `project_session_mismatch` without mutation; blocked project and migration/project-registration paths remain enforced | manual-save sentinel idempotence; `dev_id` fallback; no schema/FK change |
| HTTP | valid repeated `POST /sessions` is successful through materialization; ended session is reopened; canonical-project mismatch maps to an actionable non-500 response; empty/derived/mismatched project validation remains unchanged | bounded body, migration gate |
| MCP | repeated `mem_session_start` succeeds and reopens; canonical-project mismatch is a typed tool error with no mutation; end remains non-idempotent/does not overwrite prior summary; gate remains applied | required ID/dev/client, project validation, activity tracking |
| OpenCode template | source contains handlers for `session.created` and `session.deleted`; created sends derived registration fields to `/sessions`; deleted calls the end endpoint; each uses a finite timeout and catches all failures; no daemon spawning/persistent process behavior | existing prompt capture and migration advisory behavior; installed plugin comes only from embedded asset |
| Local sync + Hive API | reopened local row is returned by the unsynced-session page and marked synced only after successful push; regular-session server upsert clears remote `ended_at` from the incoming reopen and updates its server sync watermark so pull returns it | session-before-memory ordering, project relocation, sentinel, cursor/paging, and block-quarantine rules |

### Recommended design direction

Use a single local store-level materialization operation as the authoritative lifecycle transition, called only after existing HTTP/MCP project validation. It must inspect an existing ID instead of relying on transport-level duplicate suppression: create when absent; accept any same-canonical-project observation, including directory variants; reopen an ended same-project row by clearing `ended_at` and resetting `synced_at`; and return typed `project_session_mismatch` when its canonical project differs. Directory is provenance, while `dev_id` and client are attributes, not conflict keys. The mapping must be explicit rather than inferred from SQLite unique-error strings.

HTTP and MCP should be thin adapters over that operation. This eliminates divergent duplicate behavior: HTTP currently suppresses duplicate errors while MCP exposes them. The existing project identity, quarantine, and migration checks remain before any creation/reopen side effect. The existing daemon sync batch then carries the requeued row, and Hive API's regular-session upsert must apply the incoming `ended_at=NULL` state and advance its server-side sync watermark so peers can pull the self-healed session.

The OpenCode plugin should use the existing direct loopback pattern only: lifecycle events trigger a best-effort fetch with an explicit bounded timeout, a non-OK warning where useful, and catch-all failure handling. It must neither launch a daemon nor introduce a resident service, retries, or idempotency keys. Deletion should signal the established `POST /sessions/{id}/end` route; creation should signal `POST /sessions` with the existing session, project, and directory derivation.

### Delivery plan

Delivery is preselected as `auto-chain` with `stacked-to-main` and a 400 changed-line review budget. One honest preliminary slice is:

```text
PR 1  local materialization, canonical-project mismatch, sync reset,  <= 400 lines
      and DB/sync tests
PR 2  HTTP and MCP adapters/tests, stacked on PR 1                     <= 400 lines
PR 3  Hive API regular-session reopen upsert/pull tests, stacked on PR 2 <= 400 lines
PR 4  embedded OpenCode lifecycle template + source/install tests,     <= 400 lines
      stacked on PR 3
```

Each PR keeps its tests with its implementation and should run its narrow package tests before the relevant module suite. The Hive API reopen propagation is required and has its own cohesive slice; it is not optional follow-up work. The implementation phase must reforecast actual additions plus deletions and request a human `size:exception` only if a cohesive slice cannot fit.

### Risks

- **Canonical comparison:** the local store must compare canonical project identity, not raw spelling, while retaining the provided directory as provenance. A raw-string comparison could wrongly reject aliases; a project-changing update could violate the required typed `project_session_mismatch` boundary.
- **Required reopen propagation:** local reset of `synced_at` must make the row visible to the existing unsynced-session batch, and the Hive API regular-session upsert/pull path must not retain a stale remote `ended_at`.
- **Event payload shape:** the embedded plugin's existing resolver supports multiple OpenCode input/output spellings for session ID and directory. Lifecycle handlers should use those same resolvers and defensively no-op if the required session ID cannot be resolved, rather than inventing a new event schema.
- **Failure semantics:** lifecycle registration is explicitly fail-open and bounded; it cannot guarantee delivery when the daemon is down. The next normal observation/start must self-heal locally without adding a resident retry service.
- **Safety regression risk:** implementing materialization beneath or outside validation could create/reopen rows in quarantined or migration-blocked projects. Tests must prove all existing gates still win.

### Exploration method

`.codegraph/` exists in the project. The CodeGraph MCP/CLI is not available in this executor tool surface, so after the required index check this exploration used constrained source reads and targeted searches as the fallback. No production code, generated user-machine artifact, build, or test command was run.

### Ready for proposal

Yes. The approved identity rule and required reopen propagation are now explicit, alongside the source-of-truth boundary, test seams, strict-TDD plan, and stacked delivery strategy. The proposal can translate these settled requirements into acceptance criteria without reopening them.
