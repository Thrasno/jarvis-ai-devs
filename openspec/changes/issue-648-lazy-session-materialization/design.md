# Technical Design: Lazy session materialization

## Decision and authority

Implement issue #648 at the local transactional store boundary, with thin HTTP/MCP adapters, snapshot-conditional sync acknowledgement, Hive API regular-session lifecycle acceptance, and bounded OpenCode notifications. This repository's explicitly approved scope is `hive-daemon`, `hive-api`, and `jarvis-cli`, not `packages/coding-agent`.

Inputs read directly: `exploration.md`, `proposal.md`, and all three delta specs (`lazy-session-materialization`, `session-registration-self-heal`, `opencode-lifecycle-notifications`). The proposal/specs supersede exploration's obsolete missing-session-end rejection. Active artifact backend is OpenSpec despite the repository's generic hybrid default. The maintainer explicitly chose MORE SMALL DELIVERIES, not size:exception. Delivery is ten cohesive stacked-to-main slices, each forecast strictly below 400 additions plus deletions including its tests/docs. There is no seven-slice cap and no size exception is authorized. Proposal/spec product contracts remain authoritative; this revision supersedes only the previous delivery boundaries and budget-blocker language.

This phase changes only this design. No build, test, branch, generated configuration, or production change was performed. Shell and CodeGraph tools are unavailable, so the supplied root could not be independently Git-resolved/index-checked; evidence is constrained exact-source reads and targeted searches, not fresh graph analysis. Four injected skills were loaded; the additional expected phase skill at `gentle-pi/skills/sdd-design/SKILL.md` was absent. Skill resolution: `paths-injected` for supplied skills, with unavailable phase-skill fallback explicitly recorded. No child agents were launched.

## Current source findings

- `hive-daemon/internal/db/session.go`: `CreateSession` registers identity then inserts; `EndSession` separately reads and updates, overwrites duplicate summaries, and does not dirty an already-synced row. `GetSession` hides blocked rows, so it cannot be the transactional identity lookup. `MarkSessionSynced` unconditionally acknowledges by ID and clears relocation provenance.
- `db/memory.go`: `saveMemory(mem, prepareTx)` already puts registration, writable checks, memory insertion, prompt linking, and mutation journaling in one transaction. This is the observation/summary extension seam; do not duplicate it.
- `db/prompt.go`: prompt registration/check/insert currently occur without a transaction. `db/passive_observation.go` performs an unguarded raw insert and intentionally accepts empty attribution.
- `db/db.go`: each DB has one connection and a DSN-configured 15-second SQLite busy timeout. Several MCP processes may share the database. `registerProjectIdentity` performs an INSERT even on conflict; placing it first inside the transaction obtains SQLite's writer reservation before session reads.
- `project/validator.go` already has `ValidationError`, `CodeProjectSessionMismatch`, alias resolution, directory corroboration, and missing-session-tolerant validation. Its recovery-token path deliberately retains exact selected-project checking and consumption semantics.
- HTTP start suppresses SQLite duplicate errors. HTTP end accepts no evidence body today and succeeds on repeat. MCP end rejects repeat after a racy read; summary rejects absent/ended explicit IDs. MCP start currently requires dev/client input.
- `sync/syncer.go` snapshots sessions, pushes outside any DB transaction, then acknowledges by ID. Sessions defer memories/mutations until drained. Existing prompt batching is independent and is not redesigned here.
- `hive-api/internal/repository/postgres_session.go` shares `sessionCorrectionConflict` between CreateSession and UpsertSession; it changes only project/watermark behind the existing relocation-source predicate. `ListSessionsSince` already uses watermark/keyset paging. The sync service already processes sessions before captures and wraps Sync in a transaction with project locks.
- `jarvis-cli/embed/hooks/opencode/hive.ts` has prompt PID fallback, project/directory resolvers, advisory migration status, and one-second fetch timeout. Lifecycle resolution must not reuse its PID fallback.
- `jarvis-cli/internal/hook/client.go` declares `hook` only on start; prompt/passive omit it, and end sends only an empty summary. These source callers need small accompanying contract changes, not edits to installed files.

## 1. Store contracts and transaction ownership

Introduce small command inputs in `hive-daemon/internal/models/session.go` (or a cohesive new `session_write.go` in that package). They are internal command models, not persisted columns or sync JSON fields:

```go
type SessionInput struct {
    ID, Project, Directory, DevID, Client string
}
type SessionEndInput struct {
    Session SessionInput
    Summary string
    RejectAlreadyEnded bool
}
type PromptWrite struct {
    Project, SessionID, Directory, Client, Content string
}
type PassiveObservationWrite struct {
    SessionID, Project, Directory, Client, Source, Content string
}
```

Public store methods (exact intended responsibilities):

```go
EnsureSession(ctx context.Context, in models.SessionInput) (*models.Session, error)
EnsureAndEndSession(ctx context.Context, in models.SessionEndInput) (*models.Session, error)
SaveMemoryWithSession(mem *models.Memory, in models.SessionInput) (int64, error)
SavePromptWithSession(ctx context.Context, in models.PromptWrite) (*models.Prompt, error)
SavePassiveObservationWithSession(ctx context.Context, in models.PassiveObservationWrite) error
AckSessionSnapshot(ctx context.Context, sent *models.Session, at time.Time) (bool, error)
```

Do not expose SQL transactions to transports. `EnsureSession` and `EnsureAndEndSession` own BeginTx/deferred Rollback/Commit. Capture methods own their entire write transaction. Private `ensureSessionInTx(ctx, tx, in, mode)` joins an existing transaction and never commits. Use an unexported two-value mode (`reopenForWrite`, `preserveForEnd`), not adapter-side ensure-then-end calls. A private transaction-scoped row reader uses `tx.QueryRowContext`, not `d.GetSession`.

`SaveMemoryWithSession` uses the existing `saveMemory` preparation callback. Require `in.ID == mem.SessionID` and equal canonical project keys before persistence; adapters construct both from the same validated input. The callback ensures the explicit session before memory insertion/link/journal. Summary is this same method with category `session_summary`; it does not update `sessions.summary` or end the session. Keep `SaveMemoryWithManualSession` unchanged. Legacy `SaveMemory` remains for internal non-materializing/import callers; listed MCP explicit writes must use the new method.

`SavePromptWithSession` validates content/project, opens a transaction, registers/checks the project, ensures a nonempty regular ID, inserts and scans the prompt, parses its timestamp, then commits. Preserve existing wrapper signatures for non-converted callers; wrappers delegate using `Client: unknown` and empty directory where appropriate. Empty HTTP prompt ID stays empty; MCP's absent-ID manual fallback remains its existing manual path, not a regular session.

`SavePassiveObservationWithSession` uses the same transaction sequence when ID is nonempty. Empty ID delegates to the existing raw empty-attribution behavior, including empty project and NULL sync identity; do not impose new project requirements on that branch. It remains a local passive record, not a new sync entity.

Retain low-level `CreateSession`/`EndSession` while unconverted/internal callers need them; do not change their semantics globally to activate an incomplete stack. The required public adapters explicitly switch to the new contracts. No nested DB calls while holding the only pooled connection.

### Lifecycle state table

| Existing row | Start / observation / prompt / passive / summary | End |
| --- | --- | --- |
| Absent, valid evidence | Insert one open row | Insert then close in same transaction |
| Active, compatible | Return same row; preserve start identity | Close, store supplied summary, reset synced_at |
| Ended, compatible | Clear ended_at and synced_at; preserve started_at, sync_id and summary | MCP rejects; HTTP returns success without mutation |
| Different canonical project | Typed mismatch; rollback | Typed mismatch; rollback |

For HTTP duplicates, preserve its existing successful outcome but protect the original ended timestamp and summary rather than repeating its destructive update. MCP sets `RejectAlreadyEnded: true`, producing `ErrSessionAlreadyEnded` and unchanged state. The mode never reopens a row for an end operation. Returned lifecycle timestamps come from the committed row, not `time.Now()` in adapters.

On existing compatible rows retain original directory/client and nonempty dev_id: incoming variants do not overwrite provenance or conflict. Heal only empty/whitespace developer attribution through the existing `resolveDevID` fallback, and dirty the row if healing changes it. New rows persist declared client; capture/end omission defaults to `unknown`. Adapters never fabricate developer identity; the store alone applies existing HIVE_DEV_ID/unknown fallback. Preserve manual-save and legacy sentinel dispatch; do not route a manual fallback through regular reopening or alter stale-session closure policy.

## 2. Identity, error mapping, and safety ordering

Reuse `*project.ValidationError{Code: project.CodeProjectSessionMismatch, ...}` for store-time mismatch (the DB already depends on project contracts). This gives `errors.As` across wrapped errors without a second string-matched error family. Return the same actionable message/canonical candidates as validator mismatch. Never classify SQLite UNIQUE failures as compatibility; only the row/project comparison establishes it.

HTTP maps both validator and transactional mismatch through `writeProjectValidationError`: **400**, existing JSON `error_code: project_session_mismatch`, `error`, and candidates. A new 409 convention is unnecessary and would make preflight/store conflicts inconsistent. MCP uses `toolValidationError` for store errors too: `IsError=true` and the same structured JSON. Other DB errors remain internal failures; project-blocked mapping remains HTTP 423/current MCP blocked outcome. Duplicate-end rejection is a separate lifecycle error, not project mismatch.

Ordering for all nonempty-ID surfaces:

1. Existing migration route/tool wrapper admits the operation; blocked requests never reach validation/materialization.
2. Decode bounded input; enforce IDs, content limits, and sanitize capture before the transactional write.
3. `ValidateWriteProject` resolves explicit project/directory/aliases and checks session compatibility using `SessionID` on start/end as well as captures. Preserve recovery fields on existing surfaces. Missing rows already pass this check. ID alone cannot supply a project, even for an existing row.
4. Begin transaction; `registerProjectIdentity(ctx, tx, resolvedProject)` is the first database operation (writer reservation). Its new identity row rolls back on later failure. Then `ensureProjectWritableInTx` checks the canonical target.
5. Read the session within the same transaction; if present, check its stored project's writable status before mutation and compare canonical project keys. Do not hide a quarantined existing row as absent. Aliases are resolved by the established validator; canonicalization itself is not alias resolution or authority to reproject.
6. Materialize/reopen, execute the dependent write, commit. Record activity and queue existing autosync only after success.

Do not bypass summary `project_unknown` self-heal/directory corroboration by copying the session's project. Existing recovery-token bookkeeping can commit before capture, as today; this design's atomic guarantee covers session/capture/lifecycle changes, not a redesign of recovery-token consumption.

## 3. Concurrency without additional dependencies

Approved issue #648 outcome 4 requires **process-local in-flight initialization sharing as well as store idempotence**. SQLite serialization does not satisfy coalescing: it serializes separate transactions but still executes each initialization. Use two deliberately separate mechanisms, without a new dependency.

### Standalone registration/start: share one in-flight result

Add a small non-generic `sessioninit.Group` in `hive-daemon/internal/sessioninit/group.go`, backed by `sync.Mutex` and `map[Key]*flight`; `Key` is a struct containing exact session ID and validated canonical project. Each HTTP Server and MCP server registration owns one group for its lifetime and store instance, never one group per request or a global group shared across unrelated stores. Its `Do(ctx, key, initialize)` returns `(*models.Session, error)` and is used **only** around standalone `EnsureSession` for HTTP POST /sessions and MCP mem_session_start, after every caller independently passes migration, input and project validation. Equal canonical aliases join; different canonical projects and different IDs cannot join. Directory/dev/client variants are not key fields: the first admitted caller supplies creation attributes under the existing first-provenance policy.

Under the mutex, join an existing flight or install `{done chan struct{}, session, err}` as leader; release the mutex before initialization. The leader executes EnsureSession once with its request context, publishes a detached immutable session snapshot and the original error, removes that exact entry and closes done under the mutex on completion. Followers wait for done and receive the same committed snapshot/error (copy pointer-valued timestamps when returning, so callers cannot mutate shared state). No successes or failures remain cached; the next request after completion performs a new initialization, including retry after failure or reopening after an intervening end. Cleanup is deferred on every exit; an unexpected initializer panic publishes a safe internal failure and wakes followers before normal panic handling, never leaving an entry stuck. Do not hold the group mutex across SQL or wait operations.

All non-cancelled joined callers receive the same initialization outcome, including typed mismatch and store/commit errors. A follower whose own context is cancelled may leave with its context error without cancelling/removing the shared flight. Leader cancellation cancels that initialization and its resulting error is shared; no automatic retry or detached unbounded worker is introduced. Each successful caller may perform its own existing response/activity bookkeeping after Do; only initialization is shared. End operations do not join start flights or share closure results: they keep atomic ensure-and-end and deliberate duplicate-summary semantics.

OpenCode mirrors this with one module-local Map of in-flight created-notification promises, scoped to this plugin process/loopback endpoint (see section 6). It coalesces registration requests, not prompts or deletion. These adapter-local groups do not provide cross-process consistency; the store remains authoritative even when HTTP, MCP and OpenCode have separate flights.

### Transactional captures: never share or detach the dependent write

Observation/memory, prompt, passive and summary captures must each execute their own `ensureSessionInTx` plus dependent write in their own transaction. They do not call Group.Do, await a standalone initialization as proof of attribution, or pre-ensure outside their transaction. No shared future may return another capture's ID or suppress its payload. Even if a start flight exists, the capture's transactional check is required: start may fail or state may change. This distinction fulfills shared standalone initialization without breaking rollback of capture-driven creation/reopen. There is no standalone initialization sub-operation to detach from a capture transaction.

For store consistency, begin a normal transaction and perform identity registration before any session read. This avoids read-to-write snapshot upgrade races across separate DB handles/processes; the one-connection pool also serializes local transactions. Keep the existing driver, busy timeout, context cancellation and normal error propagation. The small mutex protects only the process-local flight map; no distributed lock, retry service, singleflight package or new dependency is needed.

After acquiring the writer reservation, absent sessions are inserted with the existing primary key; a compatible concurrent operation observes the committed row. Never use INSERT OR REPLACE (it destroys identities). If a genuine database busy/commit failure remains, roll back and report failure rather than claiming success. Only successful captures must all persist independently; this is not payload deduplication.

Concurrent end/end serializes: first closes; second returns HTTP no-op success or MCP already-ended error. Capture after end reopens; end after capture closes. Final state follows transaction order, not event occurrence time. Delayed notifications may therefore reopen or close a row; lifecycle versioning/event ordering is explicitly not introduced.

## 4. Local sync acknowledgement and Hive API propagation

Reopen and successful end set `synced_at = NULL`; reopen preserves `sync_from_project`, start, ID, sync ID, and prior summary. Failed transaction restores the previous dirty/ended state. Failed push makes no acknowledgement.

Replace the syncer's by-ID acknowledgement with `AckSessionSnapshot`. Its conditional UPDATE succeeds only when the current row still matches the exact sent semantic state: ID, sync ID, canonical project literal, relocation source, directory, dev_id, client, started_at, nullable ended_at and summary, and `synced_at IS NULL`. Clear relocation source only in that successful UPDATE. Compare nullable fields null-safely, and normalize NULL/empty summaries consistently with `scanSession`; timestamps must use existing storage formats or SQL-normalized equality, with tests for supported SQLite/RFC3339 representations. Fail closed (leave dirty) on uncertain comparison.

Return `(false, nil)` for stale/absent snapshots, `(true, nil)` only for an actual mark, and errors for database failures. In `syncBatchStep`, increment `recordsMarkedSynced` only on true. A stale snapshot remains eligible for the next normal bounded drain; retain existing no-progress limits. Never hold a SQLite transaction across network I/O. Keep legacy `MarkSessionSynced` for other callers/tests until intentionally migrated; the production push loop must not use it.

This is state-based compare-and-swap without a schema version: snapshot closed -> local reopen -> old success cannot clear the new dirty state. Also cover relocation between fetch and acknowledgement and end after an open snapshot. If state changes away and back to exactly the acknowledged payload, acknowledgement is safe for row-state convergence (there is no new event-history guarantee). Concurrent stale remote pushes remain governed by existing arrival ordering; no new distributed conflict protocol is promised.

Hive API `UpsertSession` regular conflict clause must set `ended_at = EXCLUDED.ended_at`, `summary = EXCLUDED.summary`, project and `synced_at = now()`, preserving original started_at, ID, sync ID, directory/dev/client. NULL must be assigned directly, not coalesced with existing ended_at. Preserve `WHERE sessions.project = $10` and `rejectRelocationEnds`; `relocationSource` supplies the same-project predicate for ordinary pushes. Keep manual/legacy branches unchanged. Split out the regular sync lifecycle clause from the CreateSession correction clause, so non-sync API creation does not inadvertently become a lifecycle writer. Update misleading first-write-wins comments.

`ListSessionsSince` already exposes the advanced watermark and open row; retain >= watermark and `(synced_at, sync_id)` paging. The service needs regression tests, not an orchestration rewrite. Local `SaveSessionFromRemote` currently ignores an existing ID; peer-side overwriting/conflict reconciliation is not required by these specs and remains unchanged. Verify the returned API pull contains NULL ended_at; do not claim every existing peer row is overwritten.

## 5. Transport and caller contracts

| Surface | Adapter/model change | Store call |
| --- | --- | --- |
| HTTP POST /sessions | Preserve existing id/project/directory/dev_id/client fields; include ID in project validation; remove duplicate suppression | EnsureSession |
| HTTP POST /sessions/{id}/end | Bounded JSON body: project/directory and optional client; empty body treated as absent evidence then rejected by project validation; do not infer project from ID; HTTP still supplies empty summary | EnsureAndEndSession, reject=false |
| HTTP POST /prompts | Add optional client (unknown default); preserve recovery fields and sanitization | SavePromptWithSession |
| HTTP POST /observations/passive | Add optional client (unknown default); full shared validation only for explicit ID, preserve empty-ID branch | SavePassiveObservationWithSession |
| MCP mem_session_start | Retain required ID/dev input and existing compatibility fields; declare stored client=mcp regardless of legacy client argument; document legacy client as ignored/deprecated | EnsureSession |
| MCP mem_session_end | Add project/directory plus existing summary to schema/decoder; allow either evidence field, validate at runtime; stored client=mcp | EnsureAndEndSession, reject=true |
| MCP mem_save / mem_session_summary | Remove explicit summary existence/open precheck; preserve limits/manual path; construct SessionInput with client=mcp | SaveMemoryWithSession for explicit ID |
| MCP mem_save_prompt | Explicit ID no longer goes through an existence-dependent resolver; client=mcp; retain existing required project/manual fallback | SavePromptWithSession |

Keep HTTP success statuses/bodies (start/end 200, prompt 201, passive 202). Update `httpapi.SessionStore`, both PromptStore interfaces, MCP MemoryStore and all implementing mocks with their owning slice. Use compile-time interface assertions where existing patterns permit. No session Client field is added to stored prompt/memory rows or their sync payloads: it is write-command attribution only.

Native hooks: add `client: hook` to PostPrompt/PostPassiveObservation. Extend PostSessionEnd and its event caller to carry already-resolved project/directory and client=hook, encode the ID path, retaining bounded fail-open behavior. This is required caller compatibility with end validation, not permission to redesign hook project derivation. OpenCode prompt capture adds client=opencode so missed lifecycle delivery still records the correct caller; preserve content extraction, session fallback, evidence fields, sanitization expectations and response diagnostics otherwise. Exact existing prompt tests should expect only this additive attribution field.

## 6. OpenCode embedded source design

Use OpenCode's plugin `event` callback and dispatch on `event.type` for `session.created` / `session.deleted`. The event envelope's `properties.info.id` is the primary actual lifecycle session ID. A single `resolveLifecycleSessionId(event)` accepts nonempty string identity from that documented session-info payload, defensive session_id/sessionId/sessionID spellings on lifecycle properties, then explicitly configured HIVE_OPENCODE_SESSION_ID/OPENCODE_SESSION_ID/SESSION_ID. Never read an arbitrary envelope event ID as a session ID; never call the prompt resolver's ppid fallback. Actual payload ID takes precedence over ambient environment. Unknown event types no-op.

Both branches use the same lifecycle evidence resolver built on existing project/directory conventions (env first, supplied info/properties, existing cwd fallback for directory). Require usable session ID and at least one nonempty project/directory. No synthetic project/default or dev_id. Resolution, encoding, JSON serialization, timeout construction and fetch all occur inside the caught async notification function.

For `session.created` only, `registerLifecycleOnce` returns the same pending Promise to concurrent callers from a module-local Map. Use an unambiguous JSON tuple key `[id, "canonical", canonicalProject]` only when canonical identity is actually known; the plugin must not invent canonicalization/alias resolution. Normally it has unvalidated evidence, so use `[id, "evidence", project, directory]` with the exact resolved nonempty/empty values. Distinct unknown evidence tuples must not collide, even if the server later resolves them to one project; the daemon's validated canonical-key group supplies that convergence. Never key solely by ID or concatenate ambiguous delimiters.

Install the promise before its asynchronous fetch begins (for example via Promise.resolve().then), using the first caller's resolved payload. Resolve it to one shared outcome for OK, rejected status, timeout or thrown error; fail-open diagnostics stay content-free. Delete the exact Map entry in finally on success or failure, without retaining a settled promise or producing an unhandled rejected finally chain. The existing <=1000ms timeout bounds each flight; a later event retries naturally after failure, but no retry is scheduled. The event callback remains fire-and-forget while the helper is awaitable in tests. Deleted notifications and every chat.message retain independent requests; neither joins the created map.

- Created: POST `http://127.0.0.1:${HIVE_PORT}/sessions`, JSON `{id, client: "opencode", ...nonemptyEvidence}`.
- Deleted: POST `http://127.0.0.1:${HIVE_PORT}/sessions/${encodeURIComponent(id)}/end`, JSON `{client: "opencode", ...nonemptyEvidence}`.
- Both: `Content-Type: application/json`, `AbortSignal.timeout(1000)`; event callback uses `void notifyLifecycle(event)` and returns without awaiting network. The helper catches all failures and does not rethrow. Non-OK only emits optional content-free endpoint/status diagnostic. No retry, process spawning, daemon launch, or permanent resource.

Leave `reportMigrationStatus` advisory behavior and installer sourcing intact. Tests exercise the exported plugin/event callback with fake fetch, not only grep for event names. Reuse the existing Go-driven Node test approach in `claude_embedded_hooks_test.go`: execute extracted/source-derived JS with narrowly asserted TypeScript annotation removal (fail if extraction changes), no copied behavioral implementation or new npm dependencies. Node-dependent tests skip explicitly when unavailable/short; required source/install assertions still run. Validate actual supported OpenCode envelope in the manual checklist before claiming runtime compatibility.

Manual checklist (created/prompt subset with slice 9; full checklist with slice 10): use a temporary test OpenCode configuration generated from the source installer and a local loopback receiver; create/delete a session and inspect exact path/body/client/evidence, test IDs needing encoding, send a prompt and confirm existing content/session capture plus client, return 400/423/500 and delay beyond one second, then stop receiver. Confirm OpenCode remains usable and no retries/unhandled failures/process launch occur. Do not hand-edit the developer's installed plugin; record actual runtime/version and skipped steps.

## 7. Strict TDD seams and acceptance coverage

Every slice records RED command/failure, minimal GREEN change, triangulation where needed, then refactor with tests green. No retrospective test-first claims. Table-driven Go tests use t.TempDir and controlled environment; no real home/config or secrets.

- DB lifecycle: absent/active/ended; canonical spelling aliases through validator plus direct canonical-key equality; directory/dev/client variants; default/healed dev; immutable start/sync identities; mismatched project; both blocked projects; sentinels. End matrix includes missing/active/duplicate/mismatch/invalid evidence and rollback. Test HTTP duplicate success leaves original summary intact and MCP duplicate rejects.
- Atomic captures: use SQLite test-only aborting triggers for session insert/update, user_prompts/passive_observations insert, memories insert, memory_prompt_links and mutation journal insert. For absent and ended sessions assert no orphan, no accidental reopen/sync reset, and no dependent write on failure. Reuse existing memory_manual_session rollback fixture patterns rather than production fault-injection callbacks.
- Process-local coalescing (separate from SQLite tests): block an injected initializer behind channels, admit N callers to the same canonical key, and prove its invocation count is exactly one before release; then assert every live caller receives the identical committed snapshot or typed failure. Explicitly prove followers joined before releasing, not merely that N calls eventually succeeded. After failed-flight cleanup, a new call invokes the initializer again and succeeds; after successful cleanup, a later call also invokes it again. Cover independent keys, same ID/different projects, aliases sharing a canonical key, leader/follower cancellation, panic cleanup and returned-snapshot isolation. Exercise HTTP/MCP real adapter wiring with a blocking fake store, not only the helper. Migration/project-invalid callers must not join a valid flight.
- Store concurrency: channel/barrier-started goroutines with one DB and two independently opened handles on the same temporary file; wait with deadlines, no timing sleeps. Assert one session plus all successful independent captures, mismatch isolation, and serialized end outcomes. With a start flight held, concurrent captures still each persist their payload through their own transaction; a failing capture rolls back only its own changes, without a separately committed pre-ensure. Coalescer invocation counts and DB convergence are different assertions.
- Transport: real httptest/MCP server with temporary DB for transactional guarantees; small spies for caller attribution, mapping and post-commit activity. Exercise all nine required surfaces and migration wrappers, required/empty IDs, no evidence, project_unknown, directory corroboration, aliases, recovery and quarantine. Add failure-trigger coverage through both end adapters.
- Sync: block a fake server response after snapshot, commit reopen/end/relocation, release success, assert stale ack=false and row dirty; failed push stays dirty; unchanged successful snapshot clears dirty/relocation state and increments progress. Check failed compare does not inflate drain progress and normal next batch sends changed state.
- Hive API: existing pgxQuerier seam asserts regular clause and argument NULL; repository integration test proves ended -> NULL, start unchanged, summary update and pull watermark visibility. Keep sentinel/relocation/quarantine-at-both-ends/paging regressions. Service tests prove incoming NULL is forwarded without rewriting ordering; SQL string checks alone do not prove propagation.
- CLI: native hook receiver tests prove client/evidence; source-derived executable OpenCode tests prove exact methods/URLs/headers/bodies, encoded ID, actual-info ID, env fallback, no PID fallback, no-op, timeout<=1000, thrown fetch/non-OK/timeout, immediate callback completion while fetch is pending, and unchanged prompt behavior apart from declared client. Hold fake fetch pending, call the created helper N times and assert one fetch, the same pending promise/shared outcome for all, cleanup after success/rejection/timeout, and a fresh fetch on the next event after failure. Distinct project/evidence keys cannot join; prompts and deletions are not swallowed by a created flight. Installer tests compare embedded bytes in t.TempDir.

Verification during apply: narrow `go test ./internal/db`, `./internal/httpapi`, `./internal/mcp`, `./internal/sync` in hive-daemon as relevant; repository/service packages in hive-api; agent/hook packages in jarvis-cli. Then `go test ./...` and `go vet ./...` in each affected module, plus focused `go test -race` concurrency packages where supported. Run integration tests against the configured test PostgreSQL and report skips explicitly. No root workspace command, build, release or generated-home modification. This design phase executed none of these commands.

## 8. Ten cohesive stacked-to-main slices

This is one honest revised slicing pass under the maintainer's MORE SMALL DELIVERIES decision. Figures include production code, tests, docs, and interface/mock churn; they are forecasts, not measured diffs or guaranteed caps. The hard per-slice gate remains **<400 additions + deletions** and about **≤60 minutes** of focused review. No tests, comments, formatting or behavior are removed to meet it. Splitting lifecycle foundation from acknowledgement, registration from end, and OpenCode creation from deletion creates deliverable capabilities, not implementation/test separation.

| Slice | Starts / ends (included files and tests) | Forecast | Verification / rollback |
| --- | --- | ---: | --- |
| 1 API lifecycle acceptance | Current API -> sync-only regular conflict applies NULL/summary and watermark; hive-api repository postgres_session.go + repository/service integration and regression tests and clause docs. No local activation. | 180–270 | API repository/service then module suite; retain while producers exist |
| 2 Atomic lifecycle store foundation | Slice 1 -> SessionInput/SessionEndInput, db/session.go transactional ensure/ensure-and-end, typed mismatch and writer ordering. Include state/default/provenance/sentinel/gate matrices, end rollback triggers and one-/two-handle concurrency tests. Additive primitives; public adapters and acknowledgement unchanged. | 250–350 | DB suite and focused race tests; remove primitives only after dependent adapters, preserve rows |
| 3 Snapshot-safe sync acknowledgement | Slice 2 -> AckSessionSnapshot, syncer interface/loop/mocks and stale reopen/end/relocation interleaving, timestamp/NULL, failed-push and progress tests. Public producers still unchanged; production sync now protects newer dirty state. No lifecycle adapter work. | 170–260 | DB/sync suites; revert loop and contract together, never clear pending data |
| 4 Coalesced standalone registration | Slice 3 -> reusable sessioninit/group.go plus its complete channel-controlled outcome/cancellation/panic/cleanup/isolation tests; HTTP/MCP start schema/handlers/interfaces/mocks and canonical-key wiring tests with blocking stores. Include validation/migration exclusion, attribution, repeat/reopen and typed mapping tests. Starts now self-heal and share initialization; end and captures remain unchanged. | 300–390 | Sessioninit/HTTP/MCP suites and focused race tests; revert start activation and group together |
| 5 Atomic lifecycle end and shipped callers | Slice 4 -> HTTP/MCP end schemas/handlers/interfaces/mocks, validated evidence and duplicate semantics; native hook PostSessionEnd and event caller carry evidence/client and encoded ID. Include both adapters' missing/active/duplicate/mismatch/invalid/blocked/empty-ID and failure-trigger tests, hook receiver tests, and evidence compatibility help/docs. No capture activation. | 250–350 | HTTP/MCP/hook suites; revert end callers/adapters together without data undo |
| 6 Atomic prompts | Slice 5 -> PromptWrite, db/prompt.go transactional command/wrappers, HTTP/MCP prompt interfaces/handlers/tests and hook PostPrompt client test. Include insert/ensure rollback, independent concurrent captures (including held start flight), manual/empty-ID and gate regressions. No passive/memory change. | 280–375 | DB/HTTP/MCP/hook suites; revert prompt activation only |
| 7 Atomic passive observations | Slice 6 -> PassiveObservationWrite, db/passive_observation.go, HTTP explicit-ID validation/model/tests, hook passive client/tests. Include absent/ended rollback and independent-write tests, preserving empty attribution. No memory change. | 220–330 | DB/HTTP/hook suites; revert explicit-ID passive behavior only |
| 8 Atomic observations and summaries | Slice 7 -> memory preparation wrapper, MCP explicit save/summary conversion and mocks, insert/link/journal rollback and independent-capture tests, manual/project_unknown/recovery regressions. Backend required surfaces now complete; no plugin changes. | 280–385 | DB/MCP suites; revert explicit capture activation, leave stored memories |
| 9 OpenCode coalesced creation and prompt attribution | Slice 8 -> embedded hive.ts shared safe lifecycle resolver/evidence helper and caught notifier, created event dispatch and created-flight Map, plus additive prompt client. Include source-derived executable harness, complete created request/identity/no-op/failure/timeout/immediate-return/shared-promise/cleanup/retry/isolation tests, prompt independence/regressions, temporary-home installer assertions and creation/prompt manual checklist. Deletion remains a no-op until slice 10; no untested created coalescing is deferred. | 280–380 | Agent suite, Node contracts and creation/prompt manual checks; revert embedded created feature via normal installer regeneration |
| 10 OpenCode deletion delivery | Slice 9 -> deleted event dispatch through the existing resolver/notifier with encoded end path and exact evidence/client body. Include executable deletion-specific request/identity/no-op/fail-open/timeout/immediate-return tests, deletion independence during a pending created flight, installed-byte assertions for both handlers, and full manual checklist. No resolver rewrite, new harness, retry or installer algorithm. | 120–200 | Agent suite, Node contracts and full runtime checklist; revert deletion independently, then creation if necessary |

Forecast total: **2,330–3,290 changed lines across ten PRs**. These estimates deliberately allow more aggregate work than the former seven-slice forecast: separate tested boundaries incur fixture/interface and documentation overhead. Slice 4 has the tightest headroom (390); slice 9 includes all created/coalescer tests and the executable harness cost (380), rather than hiding that cost in deletion. Existing fixtures may be reused, but not copied behavioral implementations or omitted assertions. Tests for each behavior land with that behavior; section 7 remains the complete acceptance matrix. Shared test helpers land with their first tested consumer, with any subsequent extension charged to its owning slice.

**Review-budget gate: clear at design/forecast level.** The authorized ten-slice plan has no forecast at or above 400 and needs no size:exception. The old seven-slice blocker is resolved, not waived. Tasks must retain these boundaries and explicitly list RED/GREEN helper and both Go start-wiring tests, both end rollback matrices, OpenCode created shared-promise/retry tests, deletion independence tests, and transactional independent-capture regressions. Actual additions + deletions must still be measured per clean PR diff during apply; if a cohesive unit exceeds the gate, stop and report the measured overage for a maintainer delivery decision. Do not infer an exception, shrink code/tests, or repeatedly reshuffle forecasts to claim compliance. This revision creates no tasks.md.

Dependency diagram: `main <- 1 <- 2 <- 3 <- 4 <- 5 <- 6 <- 7 <- 8 <- 9 <- 10`. Each PR starts at its immediate predecessor and excludes later rows' work. Logical prerequisites: 3–8 use store primitives from 2; all public reopen activation follows safe acknowledgement in 3; 9 follows backend completion; 10 reuses the fully tested creation slice's resolver/notifier/harness. API is deployed first and plugin last. Each partial state is safe but not the completed feature; slice 9 is useful tested creation recovery with unchanged deletion behavior, not a claim of complete OpenCode coverage.

Each eventual PR includes start/end, predecessor, follow-up, exclusions, actual additions+deletions, verification, rollback and a diagram marking itself with 📍. Merge in order and rebase/retarget the next PR to main after its predecessor lands; no feature tracker PR and no mixed chain strategy. Current boundary is design only, not a created PR.

## Rollout, non-goals and residual risks

No schema/FK changes, new libraries, idempotency keys, capture deduplication, event-version protocol, Core/autostart, lifecycle retries, stale-session-policy redesign or peer conflict rewrite. SDD injection, personas, config replay (`jarvis sync`), doctor/reconfiguration, Hive memory sync concepts and Todoist remain separate. Installation distributes the embedded asset; no config merge logic or generated local file is edited.

Partial stack deployment is supported but incomplete until every required surface passes. Old evidence-free end callers now fail safely instead of closing by ID; update shipped native caller with the adapter slice and document evidence requirements. External clients must send project/directory. MCP legacy client input no longer controls attribution; describe mcp attribution in schema/help and retain accepted compatibility input rather than silently trusting it.

Rollback reverses plugin/caller activation and then affected adapters/captures; keep API acceptance until no reopen producers remain. Do not delete materialized sessions, overwrite summaries, reconstruct ended_at, mass-close rows, or clear dirty data. Code rollback cannot undo committed lifecycle events.

Main residual risks: forecast-to-actual review-budget growth (especially slices 4/9); unverified live OpenCode envelope; optional Node/PostgreSQL test environment; stale remote payload arrival without versioning; time-format compare causing conservative extra sync; deliberate API-only pull visibility rather than local peer overwrite. Required implementation gates are measured review budgets and passing transaction/interleaving tests, not additional product authorization invented by this phase.
