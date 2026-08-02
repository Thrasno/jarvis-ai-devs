# Distributed Quarantine Lifecycle

## ADDED Requirements

### Requirement: Monotonic audited generations
The system MUST allocate a strictly increasing generation per canonical project under serialized transition control. Each BLOCK or UNBLOCK MUST create an auditable command and MUST preserve prior commands.

#### Scenario: Sequential transitions
- GIVEN a quarantined project at generation 4
- WHEN an administrator releases it
- THEN UNBLOCK is assigned generation 5 and the prior BLOCK remains auditable

#### Scenario: Concurrent transitions
- GIVEN two administrators transition one project concurrently
- WHEN both requests commit
- THEN generations are unique and strictly ordered, with no lost transition

### Requirement: Account-authenticated inbox delivery
The daemon MUST poll quarantine commands through an account-authenticated inbox independent of project synchronization. Delivery MUST be scoped to the authenticated account and MUST preserve HTTP 423 for quarantined project operations.

#### Scenario: Authorized delivery
- GIVEN an account has a valid authenticated inbox session and an undelivered command
- WHEN it polls the inbox
- THEN only that account’s command is delivered and the command remains traceable to its generation

#### Scenario: Unauthorized delivery
- GIVEN a caller lacks the account authorization for an inbox
- WHEN it requests commands
- THEN the request is rejected without revealing command or project data

### Requirement: Idempotent generation application
A daemon MUST apply commands without physical deletion, MUST ignore stale generations, and MUST treat duplicate commands as already applied. Local quarantine archives MUST be reversible and preserve project data.

#### Scenario: Stale command
- GIVEN local generation 8 is applied and command generation 7 arrives
- WHEN the daemon processes it
- THEN local state remains at generation 8 and an ACK reports stale handling

#### Scenario: Release archive
- GIVEN a local project is archived by BLOCK
- WHEN generation 9 UNBLOCK is applied
- THEN the archive is reversibly restored without deleting local or cloud data

### Requirement: Reliable acknowledgements and immediate cloud release
ACK delivery MUST be retried safely until accepted or durably classified. Cloud UNBLOCK MUST take effect immediately after its authorized transition and MUST NOT wait for every account.

#### Scenario: ACK retry
- GIVEN command application succeeded but the ACK request times out
- WHEN the daemon retries the same ACK
- THEN the server deduplicates it and reports one final acknowledgment

#### Scenario: Immediate release
- GIVEN an administrator authorizes UNBLOCK
- WHEN the cloud transition commits
- THEN cloud access is released immediately while account convergence continues asynchronously
