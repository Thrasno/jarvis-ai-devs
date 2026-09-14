# Delta for Session Registration Self-Heal

## ADDED Requirements

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
