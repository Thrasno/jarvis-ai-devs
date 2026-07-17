# Memory Delete and Restore Specification

## Purpose

Define the guarded, human-only delete and restore workflow exposed by the installed `jarvis hive` command, including local durability and shared propagation truth.

## Requirements

### Requirement: Canonical guarded workflow

The installed `jarvis hive` path MUST be the supported human workflow. It MUST verify project, local ID, and `sync_id` from the current target before either mutation, and MUST NOT expose hard delete or bulk mutation.

#### Scenario: Installer-path acceptance
- GIVEN a managed installation and a supported daemon
- WHEN the user starts `jarvis hive` and selects one memory
- THEN delete and restore are available only through that installed path and target identity is shown before confirmation.

#### Scenario: Identity drift
- GIVEN the previewed project, local ID, or `sync_id` differs at execution
- WHEN the user confirms
- THEN the operation MUST abort without mutation and report revalidation failure.

### Requirement: Fresh backup and confirmation

Delete and restore MUST automatically obtain a fresh backup. If it expires or is unavailable, the operation MUST renew or fail clearly; it MUST then revalidate project, local ID, and `sync_id`. Each operation MUST require a non-empty trimmed reason and exact operation-specific confirmation.

#### Scenario: Safe delete
- GIVEN a verified target and a stale backup
- WHEN the user submits a reason and exact delete confirmation
- THEN the system renews the backup, revalidates identity, and executes only if all checks pass.

#### Scenario: Invalid input
- GIVEN an empty reason or inexact confirmation
- WHEN the user submits
- THEN no mutation occurs and the missing or incorrect guard is reported.

### Requirement: Atomic local outcome

The system MUST report local success only after the tombstone or restore and its mutation journal commit atomically. Local success MUST be independent from shared propagation status.

#### Scenario: Local delete commit
- GIVEN all guards pass
- WHEN delete commits
- THEN the target is excluded from active results, the journal records the causal `sync_id`, and local status is successful.

### Requirement: Isolated Recently Deleted and restore

Active and Recently Deleted collections MUST remain separate. Deleted memories MUST NOT appear active, and restore MUST require the same backup, identity, reason, and exact confirmation contract.

#### Scenario: Restore
- GIVEN a tombstoned target in Recently Deleted
- WHEN the user provides a restore reason and exact restore confirmation
- THEN it returns to active results only after atomic local restore success.

### Requirement: Target-level propagation truth

Each mutation MUST expose separate target-level shared status: pending, completed, failed/retryable, or legacy-unsupported. Local success MUST NOT imply shared completion. v2 acknowledgements MUST be correlated to the mutation `sync_id`, retried idempotently, and reconciled after response loss.

#### Scenario: Response loss
- GIVEN local commit succeeded but the client lost the response
- WHEN status is refreshed
- THEN the client MUST reconcile by target identity and mutation `sync_id`, without submitting a second mutation.

#### Scenario: Legacy daemon
- GIVEN the daemon lacks the complete safe capability contract or v2 propagation
- WHEN the user requests delete or restore
- THEN the operation MUST be disabled or report legacy-unsupported, never silently degrade safety.

### Requirement: Causal unsynced deletion

An item created locally and deleted before synchronization MUST NOT appear remotely. Sync MUST preserve causal ordering and MUST treat duplicate, stale, and retried mutations idempotently; stale events MUST NOT resurrect data.

#### Scenario: Unsynced create then delete
- GIVEN a local item has no acknowledged remote create
- WHEN it is deleted
- THEN no remote active record MUST be created or exposed.

### Requirement: Least privilege and observability

MCP and agents MUST remain unable to invoke delete, restore, or guard execution. The workflow MUST provide actionable audit/status evidence without exposing memory content, reasons, tokens, or credentials beyond authorized local presentation.

#### Scenario: MCP inspection
- GIVEN MCP tools are registered
- WHEN destructive names or requests are queried
- THEN no destructive capability is available, while human UI status remains observable.
