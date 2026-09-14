# Proposal: Lazy session materialization

## Authority and intent

Issue #648 is authoritative through the confirmed pre-proposal handoff and `exploration.md`. Execution is SDD auto mode; research is explicitly unselected. Delivery is `auto-chain`, `stacked-to-main`, with a 400 changed-line (additions + deletions) budget per PR. These choices supersede generic defaults; this phase writes only this OpenSpec proposal, not production code or generated configuration.

Make Hive session attribution reliable when lifecycle notifications are missing, repeated, or arrive after a session was ended. Developers should be able to capture prompts, observations, memories, and summaries without first successfully delivering an explicit start event. Local writes must self-heal session state without crossing project boundaries, and the shared Hive API must receive and apply the reopen transition.

## Current-state gap and evidence

The exploration was read directly. Targeted current-source reads confirm:

- `hive-daemon/internal/db/session.go`: regular creation is a plain insert after canonicalization and writable-project checks; manual sessions alone have an idempotent helper. Unsynced session paging selects `synced_at IS NULL`.
- `hive-daemon/internal/db/memory.go`: memory persistence already owns a transaction and a transactional manual-session preparation seam; explicit-session materialization must join that transaction.
- `hive-daemon/internal/db/prompt.go` and `passive_observation.go`: capture inserts do not ensure a regular session alongside the write.
- `hive-daemon/internal/httpapi/server.go`: session creation suppresses duplicate-key errors, while prompt and passive-observation routes delegate to separate store operations.
- `hive-daemon/internal/mcp/tools.go`: start calls plain creation; explicit summary rejects missing/ended sessions; save and prompt paths distinguish explicit IDs from manual fallback.
- `hive-api/internal/repository/postgres_session.go`: regular conflict updates preserve `ended_at`, so resetting local sync eligibility alone cannot propagate reopening. The pull watermark is server `synced_at`.
- `jarvis-cli/embed/hooks/opencode/hive.ts`: the installed source template captures `chat.message` with loopback fetch, a one-second timeout, and fail-open handling, but has no lifecycle event handling.

The exploration's adapter-focused slice is not the full write-surface scope: transactional capture and lifecycle-end paths listed below are mandatory. The authoritative issue #648 outcome table, reaffirmed by the gatekeeper correction, supersedes the exploration's suggestion to preserve missing-session end failures. Repository standards were read from the supplied `AGENTS.md` contract and `openspec/config.yaml`. CodeGraph CLI/MCP and shell execution are unavailable here; a read of `.codegraph/config.json` failed and does not establish whether the index exists. Evidence therefore uses constrained source reads and targeted searches, not a fresh graph analysis or live GitHub issue retrieval.

## Scope and business rules

### One regular-session materialization contract

For a supplied nonempty session ID, canonical project equality is the sole compatibility/identity rule for that ID. This does not merge different session IDs within a project.

- Absent: create exactly one session with valid attribution and existing defaults.
- Existing and compatible: materialization succeeds idempotently; the enclosing operation retains its deliberate duplicate-end semantics where applicable. Directory is provenance; `dev_id` and `client` are attributes, never conflict keys. Directory or attribute variation must not reject an otherwise compatible observation.
- Existing, compatible, and ended: for start/capture reuse, reopen the same row, preserve `started_at` and existing session/sync identity, clear `ended_at`, and reset local `synced_at` so normal sync resends it. End operations ensure compatibility first but must not use reopening to defeat deliberate duplicate-end or summary-preservation semantics.
- Existing under another canonical project: return typed `project_session_mismatch`, without rewriting the binding or committing the associated write.
- Preserve project derivation/validation, identity registration, quarantine and migration gates, developer-ID fallback/healing, sentinel rules, and existing stale-session policy. Materialization is not authorization to bypass any gate.

### Transactional HTTP/MCP coverage

| Surface | Required outcome |
| --- | --- |
| HTTP `POST /sessions` | Use the shared create/repeat/reopen contract, not duplicate-error suppression. |
| HTTP `POST /sessions/{id}/end` | Ensure the session first under the canonical project contract, then end it in the same transaction; a missing session is materialized and committed as ended. Preserve deliberate compatible duplicate-end/summary semantics where applicable. |
| HTTP `POST /prompts` | For a nonempty session ID, ensure and persist the prompt atomically. |
| HTTP `POST /observations/passive` | For a nonempty session ID, ensure and persist the observation atomically after applicable project resolution and safety checks. |
| MCP `mem_session_start` | Use the same create/repeat/reopen contract. |
| MCP `mem_session_end` | Ensure the session first under the canonical project contract, then end it atomically, including when initially missing. Retain deliberate already-ended rejection and prior-summary protection; do not reopen to bypass that check. |
| MCP `mem_save` | Ensure an explicit session in the memory-write transaction. |
| MCP `mem_save_prompt` | Ensure an explicit session in the prompt-write transaction. |
| MCP `mem_session_summary` | Ensure an explicit session in the summary-memory transaction instead of rejecting an absent or compatible ended session. |

Every session-attributed operation in this table ensures its session first. A failed dependent capture or end write must roll back creation or reopening, including the sync reset. Missing-session ensure-and-end must never leave a newly created open row after failure; deliberate duplicate-end rejection must leave the preexisting ended row and summary unchanged. Concurrent compatible first observations must converge to one session, while legitimate individual capture writes retain their existing semantics; this is not payload deduplication. Transport adapters expose the typed mismatch consistently and actionably rather than converting it into success or an unexplained 500.

Empty-session behavior remains unchanged on every surface: retain current manual-save fallbacks, empty/NULL attribution, and required-ID validation where applicable. Do not invent a project or attach unresolved data to another project's session. Both HTTP end and MCP `mem_session_end` are mandatory lazy-materialization surfaces, while their committed outcome remains closure rather than an open session. Resolve and validate the effective canonical project using the issue-approved inputs and existing project-resolution rules before any mutation; an ID alone must not justify inventing project attribution for a missing row. Apply typed `project_session_mismatch` and all safety gates to end operations too. Preserve deliberate compatible duplicate-end/summary semantics where applicable, not the old missing-session failure.

### Required shared-state propagation

The existing daemon sync flow must send reopened rows and acknowledge them only after successful push. Hive API's regular-session upsert must apply incoming `ended_at = NULL`, preserve original start identity, and advance its server sync watermark so existing session pulls expose the reopened state. Retain session-before-memory ordering, project relocation preconditions, quarantine protection at both relocation ends, sentinel conflict branches, and cursor/paging semantics.

### OpenCode lifecycle source template

Extend only `jarvis-cli/embed/hooks/opencode/hive.ts` and its source/installer tests to handle `session.created` and `session.deleted`. Creation registers through `/sessions`; deletion signals `/sessions/{id}/end`. Resolve actual lifecycle payload identity defensively with the existing project/directory conventions. Notifications remain loopback-only, bounded by a finite timeout, fire-and-forget, and fail-open for network failures and rejected responses. Missing usable lifecycle identity must not target an unrelated session. Preserve prompt capture and migration advisories; do not edit the installed plugin on this machine.

## Affected areas and ecosystem boundaries

- Local Hive: `hive-daemon/internal/db/{session,memory,prompt,passive_observation}.go` and focused persistence, concurrency, and rollback tests.
- HTTP/MCP: `hive-daemon/internal/httpapi/server.go`, `hive-daemon/internal/mcp/{tools,server}.go`, store interfaces, transport tests, and existing migration/project-gate regressions. Include both end adapters and transactional ensure-and-end persistence in `hive-daemon/internal/db/session.go`, with explicit missing-session, conflict, duplicate-end, summary-preservation, and rollback tests.
- Shared sync: `hive-daemon/internal/sync/syncer.go`, `hive-api/internal/service/sync.go`, `hive-api/internal/repository/postgres_session.go`, and their tests; change orchestration code only where evidence requires it.
- CLI distribution: embedded OpenCode template and `jarvis-cli/internal/agent` source-asset/installation tests, with hook tests where appropriate.
- No intended changes to SDD prompt injection, persona, configuration replay, doctor/reconfiguration behavior, or Todoist integration. Installation continues to distribute the source template. Hive memory sync is distinct from `jarvis sync`, which replays machine configuration, and from SDD artifact storage.

## Non-goals

No resident Core, daemon spawning/autostart, background lifecycle retry service, idempotency keys, payload deduplication, schema expansion or FK migration. No generated user-machine edits, assistant-memory integration, broad architecture rewrite, stale-session policy redesign, or new lifecycle conflict-resolution protocol. Reopen propagation to Hive API is required, not a deferred enhancement.

## Success criteria and verification

1. Every listed nonempty-ID surface ensures its session first. Start/capture surfaces succeed on first observation, compatible repeat, and compatible ended-session reuse; one session row remains. HTTP `POST /sessions/{id}/end` and MCP `mem_session_end` both atomically materialize and end a missing session under the canonical project contract; active compatible sessions end normally, and already-ended sessions retain each surface's deliberate duplicate-end/summary semantics.
2. Canonical aliases and directory/dev/client variations do not create false conflicts. Another canonical project produces typed `project_session_mismatch` and no committed capture or lifecycle mutation.
3. Reopening preserves `started_at` and session/sync identity, clears `ended_at`, and resets `synced_at`. Active compatible retries do not restart the session clock.
4. Failure injection proves capture and ensure/reopen roll back together, and proves missing-session ensure-and-end rolls back completely on both end surfaces. Test each end surface for missing, active compatible, already-ended compatible, canonical mismatch, unresolved/invalid project, blocked project, and empty-ID cases; verify prior summaries and lifecycle state remain unchanged on deliberate duplicate-end rejection. Concurrent first observations/end requests cannot create duplicate sessions or cross-project attribution.
5. Invalid, migration-blocked, and quarantined writes retain their protections; empty IDs, sentinels, developer-ID fallback, and end-operation semantics retain regression coverage.
6. A previously synced ended row becomes push-eligible after reopen; failed push leaves it eligible. Successful push updates Hive API to open and makes it discoverable through watermark-based session pull without breaking paging or relocation protections.
7. OpenCode source and installation tests cover both lifecycle events, payload identity, finite timeout, non-OK/network failure handling, and unchanged prompt capture. No local generated files are modified.

Implementation must use strict RED → GREEN → REFACTOR with tests beside each behavior. Run narrow package tests first, then relevant `go test ./...` and `go vet ./...` from each affected Go module; there is no root Go workspace. Do not run builds unless explicitly requested. This proposal phase runs no tests and claims no implementation verification.

## Delivery plan

**400-line budget risk: High. Chained PRs recommended: Yes; already approved.** Expanded capture coverage makes the exploration's four-slice estimate optimistic. One preliminary cohesive slicing pass follows; each slice targets at most 400 changed lines, including its tests/docs, rather than claiming measured totals.

```text
main <- PR 1: shared API reopen acceptance and pull-watermark tests
└── PR 2: transactional ensure/ensure-and-end primitives, DB/rollback/sync-reset tests
                └── PR 3: HTTP/MCP start and end adapters, conflict/duplicate-end tests
                      └── PR 4: atomic prompt capture, HTTP/MCP coverage
                            └── PR 5: atomic passive observation capture
                                  └── PR 6: atomic memory/summary capture, MCP coverage
                                        └── PR 7: OpenCode lifecycle source/install tests
```

API acceptance lands before producers start emitting reopen transitions. Each slice starts from its predecessor, ends with the named tested capability, and excludes later surfaces. PR 2 should introduce the reusable primitives, including atomic missing-session ensure-and-end, without silently activating unconverted write callers. PR 3 must include both end surfaces with project resolution, rollback, and summary-preservation coverage; these are not optional follow-up work. Reforecast this lifecycle slice against the 400-line budget with its added end coverage. PRs 3–6 depend on PR 2; PR 7 follows backend readiness. This is stacked-to-main, not a feature-branch tracker: merge in order, then rebase/retarget the next PR to keep its diff limited to its own work unit.

Current boundary is this proposal only; no branch or PR is created. Design/tasks must forecast actual additions plus deletions, preserve tests/docs with each unit, and document independent verification and rollback. Each eventual PR includes chain context and a dependency diagram marking itself with `📍`. If one honest implementation slicing pass cannot fit a cohesive unit within 400 lines, stop and request explicit `size:exception`; do not compress code/tests or infer consent.

## Risks and rollback

| Risk | Mitigation |
| --- | --- |
| Validation rejects absent sessions before the ensure path, or ensure bypasses project gates | Exercise every transport through its real validation and store transaction; distinguish missing-session healing from canonical mismatch. |
| Separate ensure/write transactions leave orphan sessions or unwanted reopen on failure | Use the caller-owned write transaction and failure-injection tests, including ensure-and-end on both transports. |
| End materialization invents a project or reopens an ended row to bypass duplicate-end protection | Validate canonical project attribution before mutation; test missing-session closure, typed mismatch, already-ended behavior, and prior-summary preservation atomically. |
| Local reopen is lost remotely or an acknowledgement races a newer local change | Test reset, failed/successful push, watermark visibility, and acknowledgement interleavings; preserve unsynced newer state without introducing a new protocol. |
| API conflict changes weaken relocation/quarantine or sentinel protection | Keep regular lifecycle updates bounded by existing project safeguards; retain sentinel branches and relocation regression tests. |
| Delayed notifications or stale sync payloads affect lifecycle ordering | Preserve approved compatible-reopen and existing closure semantics; test relevant orderings rather than inventing versioning. |
| OpenCode payload shape/fallback points at the wrong session | Verify real event envelope handling in template tests; no-op when lifecycle identity is unusable. |
| Partial stack deployment exposes incomplete healing | Land API support first, capture adapters next, plugin last; do not declare completion until all required surfaces pass. |

Rollback is code-only and slice-scoped: revert the plugin slice first if event delivery is faulty, then affected capture and lifecycle adapters, including HTTP `POST /sessions/{id}/end` and MCP `mem_session_end`, and local activation in reverse dependency order. Revert end-adapter activation together with its dependent ensure-and-end contract rather than leaving a split ensure/closure path. Rollback can restore missing-session end failures but must not delete sessions already materialized and ended, overwrite their summaries, or undo their closure. Retain compatible API acceptance while local reopen producers remain deployed. No schema downgrade or data deletion is required. Reverting code does not undo already persisted captures or reconstruct previous `ended_at` values; do not mass-close sessions or manually rewrite local configuration as rollback. Existing sessions and pending sync data remain durable, although old behavior can restore the original missing-session failure mode.

## Proposal question round

The approved identity, reopening, coverage, non-goals, and delivery choices are settled; no new approval gate is imposed in auto mode. If the parent offers an optional product question round, these questions improve the PRD by checking implications and edge cases, not harness mechanics. The user may answer, skip, correct framing, or request another round:

1. Is there any user-facing explanation needed when a delayed observation reopens a session that was deliberately closed? Assumption: automatic reopening remains authoritative; no new prompt or control is added.
2. Should provenance/attribute history be exposed to users when compatible observations differ? Assumption: differences never conflict; no new history UI or attribution policy is introduced in this slice.
3. Is any support guidance needed for lifecycle notifications lost while Hive is offline? Assumption: best-effort hooks plus the next normal transactional write provide recovery, with no guaranteed event delivery or new retry service.

These are optional communication/refinement questions, not permission to relax the approved contract. Any answers should be summarized for correction or a second question round before changing product scope.
