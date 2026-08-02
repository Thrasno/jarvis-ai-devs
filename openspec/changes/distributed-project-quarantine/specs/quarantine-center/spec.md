# Quarantine Center

## ADDED Requirements

### Requirement: Admin-only dashboard access
The Quarantine Center MUST require administrator authentication for list, detail, release, and progress operations. Unauthorized users MUST receive no project or quarantine metadata.

#### Scenario: Admin opens center with per-user progress
- GIVEN an authenticated administrator
- WHEN the center loads
- THEN quarantined projects and generation-safe progress are displayed
- AND progress includes active-account rows with username, current-generation ACK/convergence state, and necessary timestamps or outcome
- AND aggregate totals remain available from the same consistent snapshot

#### Scenario: Unauthorized access
- GIVEN an unauthenticated or non-admin user
- WHEN the center is requested
- THEN access is denied without project metadata, aggregate counts, usernames, or account membership disclosure

### Requirement: Safe list, detail, and release
The dashboard MUST show truthful quarantine state, generation, audit timestamp, and aggregate progress. Release MUST require explicit administrator action, MUST be reversible/non-destructive, and MUST not wait for all accounts.

#### Scenario: Release project
- GIVEN an admin views a quarantined project
- WHEN the admin confirms release
- THEN UNBLOCK is submitted, cloud access releases immediately, and convergence status remains visible

#### Scenario: Release failure
- GIVEN release authorization or validation fails
- WHEN the admin confirms release
- THEN the UI reports the failure and preserves the quarantined state

### Requirement: Generation-safe polling and refresh
Polling and manual refresh MUST preserve the selected project, filters, and scroll context. Responses for older generations MUST be discarded, and polling MUST stop or back off on authorization failure.

#### Scenario: New generation during polling
- GIVEN the UI is polling generation 14
- WHEN generation 15 is returned before an older response
- THEN the UI adopts generation 15 and ignores the late generation-14 response

#### Scenario: State preservation
- GIVEN an admin has selected a project and applied a filter
- WHEN refresh or polling completes
- THEN the selection and filter remain unchanged

### Requirement: Rollback-safe surface
The dashboard and its endpoints MUST be independently disableable without deleting audit rows, commands, archives, or already committed cloud releases. Re-enabling MUST read retained state safely.

#### Scenario: Surface rollback
- GIVEN the new center is disabled after a release
- WHEN legacy administration is used
- THEN retained audit and quarantine data remain intact and no destructive rollback occurs
