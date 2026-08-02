# Quarantine Convergence Read Model

## ADDED Requirements

### Requirement: Active-account aggregation
Admin progress MUST aggregate only users where `is_active=true`, scoped to the canonical project and requested generation. The read model MUST expose totals for active accounts, acknowledged accounts, and pending accounts.

#### Scenario: Current-generation progress
- GIVEN five active accounts and three ACKs for generation 12
- WHEN an administrator reads progress
- THEN totals are 5 active, 3 acknowledged, and 2 pending

#### Scenario: Inactive account excluded
- GIVEN an inactive account has an ACK and an active account has none
- WHEN progress is calculated
- THEN the inactive account is excluded and the active account is pending

### Requirement: Generation-correct status
The read model MUST count an ACK only when its project and generation match the requested current generation. Pending MUST mean “No ACK received,” not device offline or unreachable.

#### Scenario: Older ACK does not complete current work
- GIVEN generation 11 is acknowledged but current generation is 12
- WHEN generation-12 progress is requested
- THEN the account remains pending

#### Scenario: Duplicate ACK
- GIVEN an account submits the same generation ACK twice
- WHEN progress is read
- THEN it counts once and remains deterministic

### Requirement: Privacy and administrative authorization
Progress endpoints MUST require administrator authorization and MUST return aggregate totals plus least-privilege per-user progress rows. Each admin-only row MUST contain the username, current-generation ACK/convergence state, and only timestamps or outcome fields necessary to explain that state. Rows MUST NOT contain devices, tokens, auth subjects, email, online/offline inference, or unrelated project data unless separately justified by an approved requirement. Non-admin responses MUST NOT reveal identity, counts, or account membership.

#### Scenario: Authorized aggregate and per-user progress
- GIVEN an authorized administrator requests a project summary
- WHEN the read completes
- THEN aggregate totals and per-user rows are returned for active accounts, with username, current-generation state, and necessary timestamps or outcome
- AND rows contain no device, token, auth-subject, email, or online/offline fields

#### Scenario: Non-admin request
- GIVEN a non-administrator requests progress
- WHEN authorization runs
- THEN access is denied without leaking counts, usernames, or account membership

### Requirement: Consistent snapshots
A progress response MUST identify the project and generation used for calculation so dashboard consumers can discard stale responses. Aggregate totals and per-user rows MUST come from the same snapshot.

#### Scenario: Snapshot race
- GIVEN generation 13 is created while generation-12 progress is being read
- WHEN the response is returned
- THEN it is labeled generation 12 and MUST NOT claim to describe generation 13
- AND totals and rows consistently describe generation 12
