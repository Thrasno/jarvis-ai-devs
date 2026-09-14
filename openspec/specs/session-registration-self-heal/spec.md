# Session Registration Self-Heal Specification

## Purpose

`mem_session_summary` self-heals `project_unknown` failures by deriving a project name from a caller-supplied directory and idempotently registering it, instead of failing the whole session summary.

## Requirements

### Requirement: Directory Parameter on mem_session_summary

The `mem_session_summary` tool MUST accept an optional `directory` parameter.

#### Scenario: Directory parameter accepted

- GIVEN a caller invokes `mem_session_summary` with a `directory` field set
- WHEN the tool processes the request
- THEN the `directory` value is available to the registration self-heal path

### Requirement: Self-Heal on project_unknown

When `mem_session_summary` receives `project_unknown` and a non-empty `directory` was supplied, the system MUST derive a canonical project name from that directory, register it idempotently, and proceed with the write instead of failing.

#### Scenario: Self-heal succeeds and session summary proceeds

- GIVEN `mem_session_summary` is called with a project that resolves to `project_unknown` and `directory` set to a valid, derivable path
- WHEN the handler processes the request
- THEN it derives a project name from `directory`
- AND registers that project idempotently
- AND completes the session summary write successfully

#### Scenario: No directory supplied, unknown project still fails

- GIVEN `mem_session_summary` is called with a project that resolves to `project_unknown` and no `directory`
- WHEN the handler processes the request
- THEN the call fails with `project_unknown` (no self-heal is attempted)

### Requirement: Idempotent Registration

Registering a derived project via the self-heal path MUST be idempotent: repeated calls for the same directory MUST NOT create duplicate registrations or errors.

#### Scenario: Repeated self-heal for the same directory

- GIVEN two consecutive `mem_session_summary` calls with the same `directory` that previously self-healed
- WHEN the second call is processed
- THEN the project is recognized as already registered
- AND no duplicate registration error occurs

### Requirement: Filesystem-Derived Name Wins on Conflict

When the directory-derived project name differs from the caller-supplied project name, the filesystem-derived name MUST take precedence for registration and the write.

#### Scenario: Derived name overrides caller-supplied name

- GIVEN `mem_session_summary` is called with project `"stale-name"` and `directory` pointing to a project that derives to `"actual-repo"`, and provenance indicates the derived name should apply
- WHEN the handler processes the request
- THEN the session summary is registered/written under `"actual-repo"`, not `"stale-name"`

### Requirement: Never Register "default"

The self-heal path MUST NOT register a project literally named `"default"`.

#### Scenario: Derivation would yield default is refused

- GIVEN derivation from `directory` would otherwise resolve to the sentinel value `"default"` (e.g. `ErrNoDerivableName` or an empty/degenerate basename)
- WHEN the self-heal path evaluates the derived name
- THEN it refuses to register `"default"`
- AND the call fails with a typed error rather than silently registering `"default"`

### Requirement: mem_save Escape Behavior Unchanged

The existing `mem_save` provenance-gated escape (bypassing validation when `derived && project != "default"`) MUST remain unchanged by this feature.

#### Scenario: mem_save parity scenario

- GIVEN `mem_save` is called with a project that resolves to `project_unknown`, `derived=true`, and a non-`"default"` derived name
- WHEN the handler processes the request
- THEN it bypasses validation and writes as before this change
- AND its behavior is identical to pre-change behavior

### Requirement: Explicit Session Summary Materialization

When `mem_session_summary` receives a nonempty explicit session ID and validated canonical project evidence, it MUST ensure that regular session in the same transaction as its summary-memory write. It MUST materialize an absent compatible session with caller `client` set to `mcp`, and reopen a compatible ended session under the regular-session contract; it MUST NOT bypass the existing `project_unknown` self-heal, derivation, registration, writable-project, quarantine, or migration gates or invent `dev_id`.

#### Scenario: Explicit summary heals a missing session

- GIVEN `mem_session_summary` receives a nonempty explicit session ID and valid canonical project evidence
- WHEN no compatible regular session exists
- THEN it MUST atomically materialize the session with `client` set to `mcp`
- AND MUST persist the summary-memory write

#### Scenario: Summary does not cross a project boundary

- GIVEN the explicit session ID is already bound to another canonical project
- WHEN `mem_session_summary` receives it with validated canonical project evidence
- THEN it MUST return `project_session_mismatch`
- AND MUST NOT commit the summary-memory write or mutate the existing session

#### Scenario: Empty explicit session retains existing behavior

- GIVEN `mem_session_summary` receives no session ID
- WHEN it is processed
- THEN its existing manual-session and `project_unknown` self-heal behavior MUST remain unchanged
