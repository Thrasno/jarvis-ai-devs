# Desired State Manifest Specification

## Purpose

Defines `~/.jarvis/state.yaml`: a new, independently versioned store holding
the fields required to replay a machine-scoped installation deterministically
— configured agents, selected skills, per-phase model assignments, persona,
setup scope, and statusline consent. These fields move out of `config.yaml`
(advancing it to `schema_version: 3`) rather than being duplicated, so
`config.yaml` and `state.yaml` never hold the same field and can never
disagree.

## Requirements

### Requirement: State Store Schema and Versioning

The system MUST persist replay-relevant fields (configured agents, selected
skills, per-phase model assignments, persona, setup scope, statusline
consent) in `~/.jarvis/state.yaml` under its own schema version, independent
of `config.yaml`'s schema version.

#### Scenario: State store holds all replay fields

- GIVEN a machine with a completed wizard run
- WHEN `~/.jarvis/state.yaml` is inspected
- THEN it contains configured agents, selected skills, per-phase model
  assignments, persona, setup scope, and statusline consent

### Requirement: Statusline Tri-State Consent

The system MUST persist statusline consent as a tri-state: not-decided,
decided-disabled, and decided-enabled. "Not decided" and "decided-disabled"
are distinct facts that both resolve to leaving the statusline untouched;
only "decided-enabled" authorizes installing or maintaining it.

#### Scenario: Not-decided leaves statusline untouched

- GIVEN a manifest where statusline consent is not-decided
- WHEN replay runs
- THEN the statusline is neither installed nor modified

#### Scenario: Decided-disabled leaves statusline untouched

- GIVEN a manifest where statusline consent is decided-disabled
- WHEN replay runs
- THEN the statusline is neither installed nor modified

#### Scenario: Decided-enabled authorizes the statusline

- GIVEN a manifest where statusline consent is decided-enabled
- WHEN replay runs
- THEN the statusline is eligible for installation or maintenance

### Requirement: One-Way Field Migration

The system MUST migrate replay fields out of `config.yaml` into
`~/.jarvis/state.yaml` exactly once, advancing `config.yaml` to
`schema_version: 3`. Migration MUST run before any early return in the sync
command, including a return triggered by later validation failure. Migration
MUST report success only after a durable write; it MUST NOT report before
the write completes, and it MUST NOT leave fields duplicated in both stores.

#### Scenario: Migration precedes validation blocking

- GIVEN a `config.yaml` at `schema_version: 2` whose replay fields (for
  example, configured agents) are unpopulated
- WHEN `jarvis sync` runs
- THEN migration executes and writes `state.yaml` and advances
  `config.yaml` to `schema_version: 3`
- AND validation then blocks afterward on the missing agents list

#### Scenario: Fields are moved, never copied

- GIVEN a successful migration
- WHEN both stores are inspected afterward
- THEN no replay field is present in `config.yaml`
- AND the migrated value is present only in `state.yaml`

#### Scenario: Notice withheld until the write is durable

- GIVEN a migration attempt that fails before the write completes
- WHEN the command exits
- THEN no migration-success notice is reported
- AND `config.yaml` remains at its pre-migration schema version

### Requirement: Store Disjointness

`config.yaml` and `state.yaml` MUST NOT share any field after migration. No
value MUST be readable from both stores at once, so no tie-breaking rule
between the two stores exists or is needed.

#### Scenario: No field exists in both stores

- GIVEN a post-migration installation
- WHEN the schemas of `config.yaml` and `state.yaml` are compared
- THEN their field sets are disjoint
