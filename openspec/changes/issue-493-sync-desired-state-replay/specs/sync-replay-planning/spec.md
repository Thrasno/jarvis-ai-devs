# Sync Replay Planning Specification

## Purpose

Defines the read-only planning stage of `jarvis sync`: fail-closed loading of
`~/.jarvis/state.yaml`, rendering of desired targets from the currently
installed binary's embedded assets, identity-based ownership classification,
and skill lifecycle rules. Planning MUST NOT mutate any file; it produces the
plan that the application stage consumes.

## Requirements

### Requirement: Fail-Closed State Load

The system MUST fail closed when loading `~/.jarvis/state.yaml`. A missing
manifest on a fresh machine MUST be treated as acceptable (not an error). A
read error, a corrupt file, an incompatible schema version, a whitespace-only
value, or an unrecognized value MUST all abort planning before any mutation.

#### Scenario: Missing manifest on a fresh machine is acceptable

- GIVEN no `~/.jarvis/state.yaml` exists
- WHEN `jarvis sync` runs on a machine with no prior installation
- THEN planning treats this as the fresh-machine case, not an error

#### Scenario: Corrupt or incompatible manifest aborts before mutation

- GIVEN `~/.jarvis/state.yaml` is unreadable, corrupt, at an incompatible
  schema version, contains a whitespace-only value, or contains an
  unrecognized value
- WHEN `jarvis sync` runs
- THEN the command aborts with an actionable message
- AND no file is written

### Requirement: Target Rendering from Embedded Assets

The system MUST render desired targets exclusively from the assets embedded
in the currently installed binary, never from a remote source or a cached
prior render.

#### Scenario: Targets reflect the installed version

- GIVEN a binary at version N with embedded assets for version N
- WHEN planning renders targets
- THEN every rendered target matches version N's embedded content

### Requirement: Identity-Based Ownership Classification

The system MUST classify ownership of on-disk artifacts by identity —
membership in the embedded catalog and membership in the manifest's `skills`
list — never by provenance markers where none exist, by file path, or by
naming convention (for example, an `sdd-` prefix or a `scope:` frontmatter
value).

#### Scenario: Frontmatter scope does not decide ownership

- GIVEN a skill whose frontmatter declares `scope: optional`
- WHEN ownership is classified
- THEN the classification depends only on catalog and manifest membership,
  never on the frontmatter value

### Requirement: Skill Lifecycle Rules

The system MUST resolve each skill's lifecycle action from exactly two
memberships — presence in the manifest's `skills` list and presence in the
embedded catalog — per this table:

| In manifest | In catalog | Interactive? | Action |
|---|---|---|---|
| Yes | Yes | — | Update |
| Yes | No | — | Delete |
| No | Yes | Non-interactive | Install |
| No | Yes | Interactive | Do not install |
| No | No | — | Never touch |

#### Scenario: Manifest and catalog both list the skill

- GIVEN a skill present in both the manifest and the catalog
- WHEN planning resolves its action
- THEN the plan updates the skill

#### Scenario: Manifest lists a skill the catalog dropped

- GIVEN a skill present in the manifest but absent from the catalog
- WHEN planning resolves its action
- THEN the plan deletes the skill

#### Scenario: Catalog offers a non-interactive skill not yet in the manifest

- GIVEN a skill absent from the manifest, present in the catalog, and not a
  member of `interactiveSkillIDs`
- WHEN planning resolves its action
- THEN the plan installs the skill

#### Scenario: Catalog offers an interactive skill not yet in the manifest

- GIVEN a skill absent from the manifest, present in the catalog, and a
  member of `interactiveSkillIDs`
- WHEN planning resolves its action
- THEN the plan does not install the skill

#### Scenario: Skill in neither list is untouchable

- GIVEN a skill absent from both the manifest and the catalog
- WHEN planning resolves its action
- THEN the plan does not touch that skill

### Requirement: Manifest Skills List Is Never Filtered on Write

The system MUST NOT filter the manifest's `skills` list against the current
catalog when writing the manifest. The unfiltered list is the only proof
that authorizes deleting a skill a later version dropped from the catalog.

#### Scenario: A catalog-dropped skill remains listed until deleted

- GIVEN a manifest listing a skill no longer in the current catalog
- WHEN the manifest is written during this sync run
- THEN the skill remains listed in the manifest until its deletion is
  applied, and the write does not silently drop it from the list

### Requirement: No Filesystem Redetection

The system MUST NOT redetect configured agents by scanning the filesystem. A
manifest with no configured agents MUST block with an actionable message
naming the recovery command `jarvis`.

#### Scenario: Agent-less manifest blocks with the recovery command

- GIVEN a manifest with no configured agents
- WHEN `jarvis sync` runs
- THEN the command blocks with an actionable message naming `jarvis` as the
  recovery command
- AND no file is written
