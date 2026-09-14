# Lazy Session Materialization Specification

## Purpose

Hive MUST reliably attribute session-bound local writes and lifecycle events when an explicit start notification is missing, repeated, delayed, or follows a prior end, without crossing canonical-project boundaries.

## Requirements

### Requirement: Canonical Project-Bound Session Identity

For a nonempty regular session ID, the system MUST use canonical project equality as the sole session compatibility and identity rule. Directory (including compatible checkout and worktree variants) MUST be retained only as provenance; `dev_id` and `client` MUST remain attributes and MUST NOT be conflict keys. The system MUST preserve existing developer-ID fallback/healing and sentinel handling.

#### Scenario: Compatible variants reuse one session

- GIVEN a session ID already bound to canonical project `alpha`
- WHEN an operation supplies the same ID with a directory, worktree, `dev_id`, or `client` variant that resolves to canonical project `alpha`
- THEN the operation MUST treat the session as compatible
- AND MUST NOT create a second session or report a conflict

#### Scenario: Different canonical project is rejected

- GIVEN a session ID already bound to canonical project `alpha`
- WHEN an operation supplies that ID with validated canonical project `beta`
- THEN it MUST return the typed `project_session_mismatch` error
- AND MUST NOT rewrite the binding or commit an associated write or lifecycle mutation

### Requirement: Caller-Declared Client Attribution

When materialization creates an absent regular session, it MUST persist the caller-declared `client` attribute without using it as an identity key. HTTP explicit session registration MUST retain its declared-client input contract. HTTP `POST /sessions/{id}/end`, `POST /prompts`, and `POST /observations/passive` MUST accept an optional `client` and use it for materialization, defaulting a missing value to `unknown`. MCP-attributed materialization MUST declare `client` as `mcp`; hook and OpenCode callers MUST declare `hook` and `opencode`, respectively. No materialization path MAY invent `dev_id`.

#### Scenario: HTTP capture defaults an omitted client

- GIVEN HTTP `POST /prompts` or `POST /observations/passive` materializes an absent session from a request without `client`
- WHEN the request commits successfully
- THEN the new session MUST have `client` set to `unknown`

#### Scenario: MCP capture declares its caller

- GIVEN an MCP `mem_session_start`, `mem_session_end`, `mem_save`, `mem_save_prompt`, or `mem_session_summary` operation materializes an absent explicit session
- WHEN the operation commits successfully
- THEN the new session MUST have `client` set to `mcp`
- AND MUST NOT receive an invented `dev_id`

#### Scenario: HTTP capture or end preserves a declared client

- GIVEN HTTP `POST /prompts`, `POST /observations/passive`, or `POST /sessions/{id}/end` materializes an absent session with `client` set to `hook` or `opencode`
- WHEN the request commits successfully
- THEN the new session MUST retain that declared client attribute

#### Scenario: Explicit registration retains declared client

- GIVEN HTTP `POST /sessions` materializes an absent session with a declared client under its existing input contract
- WHEN the request commits successfully
- THEN the new session MUST retain that declared client attribute

### Requirement: Transactional Regular-Session Materialization

For a nonempty session ID and validated canonical project evidence, the local store MUST provide one transactional materialization contract. It MUST create an absent session once, accept an active compatible session idempotently, and reopen a compatible ended session by preserving its `started_at` and session/sync identity, clearing `ended_at`, and resetting local `synced_at`. An active compatible retry MUST NOT restart the session clock.

#### Scenario: First compatible write materializes a session

- GIVEN a permitted operation carries a nonempty session ID and validated canonical project evidence
- WHEN no matching session exists
- THEN the store MUST create exactly one attributed session before completing the operation

#### Scenario: A compatible ended session is reused

- GIVEN a session is ended under the operation's canonical project
- WHEN a start or capture operation reuses its ID with compatible evidence
- THEN the store MUST reopen the existing row
- AND MUST preserve its original start and sync identities
- AND MUST clear `ended_at` and local `synced_at`

### Requirement: Safety Gates Precede Materialization

Materialization MUST NOT bypass existing project derivation/validation, identity registration, writable-project, quarantine, migration, or stale-session protections. An operation MUST resolve and validate its effective canonical project from issue-approved inputs before it mutates a session. A session ID alone MUST NOT authorize inferred or invented project attribution.

#### Scenario: A gate blocks a session-attributed operation

- GIVEN a session-attributed operation is migration-blocked, invalid, unresolved, quarantined, or otherwise blocked by an existing project gate
- WHEN the operation is processed
- THEN it MUST fail according to that gate
- AND MUST NOT create, reopen, or mutate the session or its dependent record

### Requirement: Atomic Session-Attributed Writes

For a nonempty session ID, session materialization or reopening and its dependent prompt, passive observation, memory, summary-memory, or lifecycle-end write MUST commit in one transaction. Failure of ensure, dependent write, or end MUST roll back all materialization effects, including a newly created row, `ended_at` changes, and `synced_at` reset.

#### Scenario: Ensure or dependent capture failure rolls back materialization

- GIVEN a session-attributed capture would materialize or reopen a compatible session
- WHEN materialization or persistence of the capture fails
- THEN no new session MUST remain
- AND an existing session's lifecycle and sync eligibility MUST remain unchanged
- AND no dependent capture MUST be committed

#### Scenario: Ensure-and-end failure rolls back completely

- GIVEN a missing compatible session is being materialized for an end operation
- WHEN the end write fails
- THEN no open or ended session created by that operation MUST remain committed

### Requirement: Required HTTP and MCP Surface Coverage

The following surfaces MUST ensure every nonempty session ID before their deliberate operation: HTTP `POST /sessions`, HTTP `POST /sessions/{id}/end`, HTTP `POST /prompts`, HTTP `POST /observations/passive`, MCP `mem_session_start`, MCP `mem_session_end`, MCP `mem_save`, MCP `mem_save_prompt`, and MCP `mem_session_summary`. HTTP and MCP adapters MUST map `project_session_mismatch` consistently to an actionable non-success result and MUST NOT reduce it to duplicate success or an unexplained server error.

#### Scenario: Capture surfaces self-heal a missing session

- GIVEN each listed prompt, passive-observation, memory, or summary surface receives a nonempty ID with validated canonical project evidence
- WHEN the session is absent
- THEN the surface MUST atomically materialize the session and persist its requested capture

#### Scenario: Start surfaces share lifecycle semantics

- GIVEN HTTP `POST /sessions` or MCP `mem_session_start` receives a compatible session ID
- WHEN the session is absent, active, or ended
- THEN the surface MUST respectively create, succeed idempotently, or reopen the single session under the shared contract

### Requirement: End Materialization Preserves End Semantics

HTTP `POST /sessions/{id}/end` and MCP `mem_session_end` MUST both lazily materialize a missing session only from validated canonical project evidence carried by that operation, then commit it as ended atomically. End adapters MUST NOT invent missing project evidence. For an existing compatible ended session, each surface MUST preserve its deliberate duplicate-end rejection and prior-summary protection; it MUST NOT reopen the row merely to make the second end succeed.

#### Scenario: Missing session is materialized and closed

- GIVEN either end surface receives a nonempty ID and validated canonical project evidence
- WHEN that session is absent
- THEN it MUST create and end the session in one transaction

#### Scenario: Active session ends normally

- GIVEN either end surface receives a nonempty ID and validated canonical project evidence for an active compatible session
- WHEN the end operation succeeds
- THEN it MUST close that session without changing its canonical project binding

#### Scenario: Duplicate end preserves prior state

- GIVEN an existing compatible session is already ended and has a prior summary where applicable
- WHEN either end surface receives another end request
- THEN it MUST retain that surface's deliberate already-ended outcome
- AND MUST leave the ended state and prior summary unchanged

### Requirement: Empty, Manual, and Passive Behavior Remains Unchanged

An empty or absent session ID MUST retain each surface's existing validation, manual-save fallback, and empty/NULL attribution behavior. Passive observations without a session ID MUST remain valid where they were valid. The system MUST NOT invent a project or attach unresolved data to another project's session.

#### Scenario: Empty session does not activate regular materialization

- GIVEN a save, prompt, passive-observation, summary, start, or end surface receives no session ID
- WHEN the surface is processed
- THEN it MUST retain its pre-existing required-ID or manual/empty-attribution behavior
- AND MUST NOT create or select a regular session solely as a fallback

### Requirement: Concurrent Compatible Operations Converge Safely

Concurrent compatible first observations or end requests for the same session ID and canonical project MUST converge to one session row and MUST NOT cross-project attribute data. This requirement MUST NOT deduplicate legitimate individual capture payloads.

#### Scenario: Concurrent first observations share one session

- GIVEN concurrent compatible capture operations use the same absent session ID and canonical project
- WHEN they commit successfully
- THEN exactly one session row MUST exist
- AND each capture MUST retain its existing independent write semantics

### Requirement: Reopen Propagates Through Hive Synchronization

A locally reopened session MUST become eligible for the existing session-before-memory sync flow and MUST be acknowledged only after a successful push. Hive API regular-session acceptance MUST apply an incoming `ended_at = NULL`, preserve original start identity, advance the server sync watermark, and expose the reopened state through existing watermark-based pulls. Existing paging, cursor, relocation, quarantine-at-both-ends, and sentinel protections MUST remain unchanged.

#### Scenario: Reopen reaches Hive API

- GIVEN a previously synced ended session is reopened locally
- WHEN its session sync push succeeds
- THEN Hive API MUST store it as open
- AND a subsequent watermark-based pull MUST include the reopened state

#### Scenario: Failed reopen push stays eligible

- GIVEN a reopened local session is pending sync
- WHEN its push fails
- THEN it MUST remain eligible for a later push
- AND MUST NOT be marked acknowledged

## Non-Goals

This capability MUST NOT add a schema or foreign-key migration, idempotency keys, payload deduplication, a new lifecycle conflict-resolution protocol, resident Core/daemon autostart, background lifecycle retry, stale-session-policy redesign, generated user-machine edits, or changes to SDD prompt injection, persona, `jarvis sync`, doctor/reconfiguration, or Todoist behavior.
