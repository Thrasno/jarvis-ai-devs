# Sync Lifecycle Safety Specification

## Purpose

Defines the safety machinery wrapping `jarvis sync`: pre-apply backup sourced
from sync's own target list, snapshot/diff-measured idempotency, required
user-visible output, bookkeeping written under lock only on change, and the
domain and CLI boundaries sync MUST NOT cross.

## Requirements

### Requirement: Backup Precedes Mutation

The system MUST create a backup snapshot from sync's own computed target
list before the first mutation of any run. If backup creation fails, the
system MUST NOT perform any mutation.

#### Scenario: Backup runs before the first write

- GIVEN a sync run with a non-empty target list
- WHEN the run proceeds
- THEN a backup snapshot is created before any target is mutated

#### Scenario: Backup failure blocks all mutation

- GIVEN a sync run where backup creation fails
- WHEN the run proceeds
- THEN no target is mutated
- AND the failure is reported

### Requirement: Measured Idempotency

The system MUST measure idempotency by snapshot/diff over sync's own path
list, never by declaring success. A second consecutive run against an
unchanged manifest and unchanged installed version MUST report zero changed
files and perform zero writes.

#### Scenario: Second consecutive run is a true no-op

- GIVEN a completed `jarvis sync` run with no subsequent manifest or version
  change
- WHEN `jarvis sync` runs again immediately
- THEN the reported changed-file count is zero
- AND no file is written

### Requirement: Required Changed-Path Output

The system MUST list every changed path in its user-visible report as
required output, not an optional detail.

#### Scenario: Changed paths are listed

- GIVEN a sync run that changes at least one file
- WHEN the run completes
- THEN the report lists each changed path

### Requirement: Bookkeeping Under Lock

The system MUST write its own bookkeeping record under a lock, and only when
at least one target changed.

#### Scenario: No-op run writes no bookkeeping

- GIVEN a sync run with zero changed files
- WHEN the run completes
- THEN no bookkeeping record is written

### Requirement: Post-Apply Verification and Recovery Naming

The system MUST verify applied state after mutation and, on failure, name an
actionable recovery command. When the manifest has no configured agents, the
recovery command named MUST be `jarvis`.

#### Scenario: Recovery command names jarvis for an agent-less manifest

- GIVEN a manifest with no configured agents
- WHEN sync blocks
- THEN the reported recovery command is `jarvis`

### Requirement: Domain and CLI Boundary Exclusions

`jarvis sync` MUST NOT call or implement Hive memory synchronization, MUST
NOT write `~/.jarvis/sync.json`, and MUST accept no flags — passing any flag
MUST be a usage error. When scope includes `local+cloud` and `sync.json` is
missing or unparseable, the system MUST report `jarvis login` for that part
of the sync and MUST NOT abort the rest of the run.

#### Scenario: A flag is a usage error

- GIVEN `jarvis sync` invoked with any flag
- WHEN the command parses arguments
- THEN it reports a usage error and performs no mutation

#### Scenario: Missing sync.json reports login without aborting

- GIVEN a `local+cloud` scope and a missing or unparseable `~/.jarvis/sync.json`
- WHEN `jarvis sync` runs
- THEN the report names `jarvis login` for the cloud portion
- AND the local-scope portion of sync still completes

#### Scenario: Sync never touches Hive

- GIVEN any `jarvis sync` run
- WHEN its call graph is inspected
- THEN it contains no call into Hive memory synchronization
