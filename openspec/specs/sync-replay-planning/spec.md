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

The system MUST resolve each skill's lifecycle action from two memberships —
presence in the manifest's `skills` list and presence in the embedded
catalog — plus, for a manifest skill the catalog lacks, whether this build
explicitly retired it, per this table:

| In manifest | In catalog | Interactive? / Retired? | Action |
| --- | --- | --- | --- |
| Yes | Yes | — | Update |
| Yes | No | Retired by this build | Delete |
| Yes | No | Not retired | Retain |
| No | Yes | Non-interactive | Install |
| No | Yes | Interactive | Do not install |
| No | No | — | Never touch |

#### Scenario: Manifest and catalog both list the skill

- GIVEN a skill present in both the manifest and the catalog
- WHEN planning resolves its action
- THEN the plan updates the skill

#### Scenario: Manifest lists a skill this build retired

- GIVEN a skill present in the manifest, absent from the catalog, and retired
  by this build
- WHEN planning resolves its action
- THEN the plan deletes the skill

#### Scenario: Manifest lists a skill unknown to this build

- GIVEN a skill present in the manifest, absent from the catalog, and not
  retired by this build
- WHEN planning resolves its action
- THEN the plan neither renders nor deletes the skill
- AND the skill remains in the manifest's `skills` list

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

### Requirement: Resolved Skill State Is Persisted Only After Convergence

The manifest's original `skills` list is retained as the deletion authority
through planning, snapshotting, application, and verification. Only after the
replay converges and verifies successfully, under the state lock, the system
MUST persist the lifecycle-resolved list: catalog-only non-interactive
additions are included, explicitly retired managed skills are removed, and
retained skills unknown to this build stay listed. The sync path MUST NOT
call `config.Save`.

#### Scenario: A retired skill remains listed until verified deletion

- GIVEN a manifest listing a skill this build retired and no longer offers
- WHEN `jarvis sync` plans and applies that deletion
- THEN the original manifest membership authorizes only that managed skill tree
- AND the skill remains in durable state until deletion and verification succeed
- AND the post-verification locked write removes it from the manifest

### Requirement: Desired Absence Is Planned and Protected

An explicitly retired manifest skill MUST be represented as explicit desired
absence in the plan. Its managed directory MUST be recursively snapshotted
before deletion and replay MUST reject malformed IDs, traversal, separators,
unsafe roots, and symlinked trees. It MUST NOT use an unconstrained recursive
remove operation.

#### Scenario: Retired managed skill tree is removed safely

- GIVEN a manifest-owned skill this build retired and an existing managed
  skill directory
- WHEN replay applies the plan
- THEN all regular files in that tree are included in the pre-apply snapshot
- AND replay removes only that managed tree
- AND verification passes only when the directory is absent
- AND `_shared` and unowned skill directories remain untouched

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
