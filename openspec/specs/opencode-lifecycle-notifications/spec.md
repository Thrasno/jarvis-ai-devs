# OpenCode Lifecycle Notifications Specification

## Purpose

The shipped OpenCode Hive source template MUST provide bounded, best-effort lifecycle notifications that supply Hive with session and project evidence while preserving prompt capture and user-machine source-of-truth boundaries.

## Requirements

### Requirement: Source-Template Lifecycle Coverage

Only the embedded source template `jarvis-cli/embed/hooks/opencode/hive.ts` and its source/installer tests MUST add handlers for OpenCode `session.created` and `session.deleted`. The created handler MUST register through `POST /sessions`; the deleted handler MUST signal `POST /sessions/{id}/end`. Installation MUST continue to distribute that embedded source template, and the change MUST NOT edit an installed `~/.config/opencode/plugins/hive.ts` file.

#### Scenario: Installed template contains both lifecycle handlers

- GIVEN the OpenCode agent installs its Hive plugin
- WHEN the installed plugin is read from a temporary test home
- THEN it MUST derive from the embedded source template
- AND MUST contain the created and deleted lifecycle handling alongside unchanged prompt capture

### Requirement: Single Safe Lifecycle Identity Resolver

Both lifecycle handlers MUST use one shared lifecycle session resolver and the existing project/directory resolution conventions. The lifecycle resolver MUST accept only a nonempty actual session identity supplied by the lifecycle payload or explicitly configured session environment; it MUST NOT synthesize a lifecycle target from a process ID or other ambient fallback. A handler MUST no-op when it lacks a usable session ID or at least one nonempty project/directory evidence value.

#### Scenario: Unusable lifecycle identity is ignored

- GIVEN a `session.created` or `session.deleted` event lacks an actual session ID or project/directory evidence
- WHEN the lifecycle handler runs
- THEN it MUST make no lifecycle HTTP request
- AND MUST NOT target an unrelated session

### Requirement: Exact Lifecycle Request Contract

For a resolvable lifecycle event, the template MUST use loopback `http://127.0.0.1:${HIVE_PORT}` and issue these exact requests:

- `session.created`: `POST /sessions` with `Content-Type: application/json` and a JSON body containing `id` set to the resolved session ID, `client` set exactly to `opencode`, plus only nonempty `project` and `directory` evidence fields.
- `session.deleted`: `POST /sessions/{encodeURIComponent(id)}/end` with `Content-Type: application/json` and a JSON body containing `client` set exactly to `opencode`, plus only nonempty `project` and `directory` evidence fields.

Lifecycle requests MUST NOT add invented project values or `dev_id`. The `client: "opencode"` declaration and project/directory evidence in the deleted request MUST allow the daemon to validate canonical project attribution and correctly attribute a missing session when it is materialized and ended.

#### Scenario: Created payload is exact

- GIVEN a created event resolves session ID `s-1`, project `alpha`, and directory `/work/alpha`
- WHEN the handler sends the registration notification
- THEN the request MUST be `POST /sessions`
- AND its JSON body MUST equal `{ "id": "s-1", "client": "opencode", "project": "alpha", "directory": "/work/alpha" }`
- AND it MUST contain no `session_id`, `dev_id`, or unrelated field

#### Scenario: Deleted payload carries attribution evidence

- GIVEN a deleted event resolves session ID `s-1` and directory `/work/alpha` without a project name
- WHEN the handler sends the end notification
- THEN the request path MUST be `/sessions/s-1/end`
- AND its JSON body MUST equal `{ "client": "opencode", "directory": "/work/alpha" }`

### Requirement: Bounded Fail-Open Lifecycle Delivery

Each lifecycle request MUST be fire-and-forget from the user's workflow, use a finite timeout no greater than the existing one-second hook timeout, and fail open on network failure, timeout, thrown error, or non-OK response. A rejected response MAY produce a content-free diagnostic, but MUST NOT abort OpenCode, start a daemon, create a persistent process, or schedule retries.

#### Scenario: Lifecycle delivery failure does not block OpenCode

- GIVEN a lifecycle request times out, throws, or receives a non-OK response
- WHEN the handler completes
- THEN the OpenCode lifecycle flow MUST continue without an unhandled failure
- AND no retry or daemon-start action MUST occur

### Requirement: Contract Verification and Manual OpenCode Checklist

Source-asset and installer tests MUST execute or otherwise assert the exact created and deleted request method, URL/path, headers, JSON payload including `client: "opencode"`, absence of invented `dev_id`, encoded session path, resolver fallback/no-op behavior, finite timeout, non-OK handling, network-failure handling, and unchanged `chat.message` prompt payload behavior. The change MUST include a manual OpenCode checklist that verifies, against a local loopback receiver, creation registration, deletion closure with carried `client: "opencode"` and project/directory evidence, prompt capture continuity, and that daemon absence or rejection remains non-blocking.

#### Scenario: Manual checklist validates fail-open operation

- GIVEN a developer follows the OpenCode lifecycle checklist with the daemon unavailable or a loopback receiver returning a failure status
- WHEN they create or delete a session and send a prompt
- THEN OpenCode MUST remain usable
- AND the checklist MUST record that lifecycle and prompt failure handling was non-blocking

## Non-Goals

This capability MUST NOT add generated user-machine edits, a resident Core, daemon spawning or autostart, a lifecycle retry service, idempotency keys, payload deduplication, a new event schema beyond defensively reading supplied identity, or changes to migration advisories and existing prompt-capture semantics.
