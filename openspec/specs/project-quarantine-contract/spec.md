# Project Quarantine Contract

## ADDED Requirements

### Requirement: Quarantine-only contract
The API MUST represent project protection as reversible quarantine and MUST NOT expose new behavior implying deletion, backup, export, device state, or offline state. New writes MUST accept only supported quarantine actions.

#### Scenario: Accept quarantine action
- GIVEN an authenticated administrator submits BLOCK or UNBLOCK
- WHEN the request is validated
- THEN the API records the quarantine action and its generation

#### Scenario: Reject unsupported purge intent
- GIVEN a client submits a new request containing `purge_intent`
- WHEN validation runs
- THEN the API returns a client error and MUST NOT mutate project, audit, or generation data

### Requirement: Mixed-version and historical compatibility
Readers MUST tolerate absent legacy fields and MUST preserve readable historical `export_marker` and `purge_intent` values. Writers MUST NOT create new unsupported legacy intent values, and compatibility behavior MUST work during rolling upgrades.

#### Scenario: Read historical action
- GIVEN an old audit row contains `export_marker` or `purge_intent`
- WHEN a current reader requests the history
- THEN the row is returned without rewriting or interpreting it as a new command

#### Scenario: Read mixed-version payload
- GIVEN a legacy client omits newly introduced fields
- WHEN its compatible request is read
- THEN defaults are applied without rejecting or mutating the payload solely because fields are absent

### Requirement: Truthful audit semantics
Every accepted transition MUST expose action, project identity, generation, actor, and timestamp. The system MUST NOT label quarantine as deletion or claim completion for accounts not acknowledged.

#### Scenario: Audit transition
- GIVEN a valid BLOCK or UNBLOCK transition
- WHEN it is committed
- THEN an immutable audit record contains the transition facts and no deletion/export claim
