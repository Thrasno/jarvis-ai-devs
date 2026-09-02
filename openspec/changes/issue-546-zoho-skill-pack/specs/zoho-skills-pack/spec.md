# Zoho Skills Pack Specification

## Purpose

Provide one durable, catalog-derived lifecycle for selecting and synchronizing embedded Zoho skills while preserving existing managed-file safety and desired-state ownership semantics.

## Requirements

### Requirement: One Pack-Level Zoho Choice

The system MUST present exactly one choice named **Zoho Skills Pack** in both TUI and non-TUI setup. The system MUST NOT present, expose, or independently control individual Zoho skills in either setup flow. The same pack answer MUST produce the same Zoho desired-state outcome in both flows.

#### Scenario: User selects the pack in the TUI

- GIVEN a catalog containing current Zoho members
- WHEN the user selects **Zoho Skills Pack** in TUI setup
- THEN the setup result MUST select every current pack member
- AND it MUST NOT require an individual Zoho-skill answer.

#### Scenario: User selects the pack in non-TUI setup

- GIVEN the same catalog and the affirmative non-TUI answer for **Zoho Skills Pack**
- WHEN non-TUI setup completes
- THEN its selected Zoho IDs MUST equal the TUI selected Zoho IDs for the same answer
- AND no individual Zoho control MUST be presented.

### Requirement: Catalog-Constrained Pack Membership

The system MUST define Zoho Skills Pack membership as every embedded catalog ID with the `zoho-` prefix. The system MUST exclude every catalog ID without that prefix from the pack. Membership MUST be derived from the current catalog so a later embedded `zoho-*` ID is a pack member without a separate membership change.

#### Scenario: Catalog contains Zoho and non-Zoho skills

- GIVEN an embedded catalog containing `zoho-crm`, a later `zoho-expense`, and non-Zoho IDs
- WHEN the system determines Zoho Skills Pack membership
- THEN the membership MUST contain `zoho-crm` and `zoho-expense`
- AND it MUST NOT contain any non-Zoho ID.

### Requirement: Deterministic V0 Desired State

For the V0 catalog, the system MUST treat exactly these IDs as Zoho Skills Pack members: `zoho-analytics`, `zoho-books`, `zoho-creator`, `zoho-crm`, `zoho-deluge`, `zoho-people`, and `zoho-projects`. Whenever selected Zoho IDs are persisted, the system MUST persist them in lexicographic ID order.

#### Scenario: Fresh selected setup persists V0 membership

- GIVEN a fresh desired state and the V0 catalog
- WHEN setup completes with **Zoho Skills Pack** selected
- THEN desired state MUST contain all seven V0 Zoho IDs in this order: `zoho-analytics`, `zoho-books`, `zoho-creator`, `zoho-crm`, `zoho-deluge`, `zoho-people`, `zoho-projects`.

### Requirement: Fresh Unselected and Deselected Pack State

When the pack is unselected in fresh setup, the system MUST add no Zoho ID to desired state. When a previously selected pack is deselected, the system MUST remove all recorded Zoho IDs from desired state while preserving unrelated desired-state IDs. Deselection MUST stop later pack management but MUST NOT uninstall already-installed Zoho files or directories.

#### Scenario: Fresh unselected setup

- GIVEN a fresh desired state with unrelated selected skills
- WHEN setup completes with **Zoho Skills Pack** unselected
- THEN desired state MUST contain no Zoho ID
- AND the unrelated selected skills MUST remain selected.

#### Scenario: User deselects an existing pack

- GIVEN desired state containing Zoho and unrelated IDs and existing installed Zoho files
- WHEN setup persists a deselected **Zoho Skills Pack**
- THEN desired state MUST contain no Zoho ID
- AND unrelated desired-state IDs and installed Zoho files MUST remain unchanged
- AND a later sync MUST NOT manage the deselected pack.

### Requirement: Eligible In-Memory Sync Expansion

The system MUST recognize `zoho-deluge` as the only released legacy selection anchor. A sync MUST expand an eligible legacy selection or an already selected pack to all current catalog pack members in memory before creating its replay plan. The system MUST NOT infer pack enrollment from any other isolated or orphaned `zoho-*` ID. The expanded in-memory selection MUST be used for the complete sync convergence lifecycle.

#### Scenario: Released legacy anchor expands before planning

- GIVEN durable desired state that contains `zoho-deluge` and lacks other current pack members
- WHEN flagless `jarvis sync` prepares replay
- THEN its replay plan MUST include every current catalog pack member before application begins
- AND durable desired state MUST remain unchanged until successful completion requirements are met.

#### Scenario: Arbitrary Zoho ID is not a legacy anchor

- GIVEN durable desired state containing an isolated `zoho-*` ID other than `zoho-deluge` without a current selected-pack state
- WHEN `jarvis sync` prepares replay
- THEN the system MUST NOT expand that state solely because of that ID.

### Requirement: Future Pack Convergence

When desired state represents a selected pack and the embedded catalog later contains an additional `zoho-*` ID, the next eligible sync MUST include that ID in its in-memory convergence set before planning. The system MUST durably add the future member only after the successful-convergence requirements are met.

#### Scenario: A selected pack encounters a future catalog member

- GIVEN durable desired state representing a selected pack and a catalog containing a newly added `zoho-*` ID
- WHEN a successful eligible sync runs
- THEN the sync MUST converge that newly added member
- AND the member MUST be durably recorded only after complete success and final verification.

### Requirement: Post-Convergence Durable State Commit

The system MUST durably add expanded Zoho IDs only after sync application has completely succeeded and final verification has succeeded. Planning failure, blocked application, partial application, and final-verification failure MUST retain the pre-run durable Zoho desired state and MUST NOT report uncommitted IDs as added. Pack expansion MUST NOT use `config.Save()`.

#### Scenario: An eligible sync fails before completion

- GIVEN an eligible expansion and a saved pre-run desired state
- WHEN planning fails, application is blocked, application is partial, or final verification fails
- THEN durable desired state MUST retain its pre-run Zoho IDs
- AND the command MUST NOT report any uncommitted Zoho ID as successfully added.

#### Scenario: Complete verified convergence commits additions

- GIVEN an eligible expansion with missing current catalog members
- WHEN application completes and final verification succeeds
- THEN the missing members MUST be added to durable desired state
- AND no expansion persistence MUST occur through `config.Save()`.

### Requirement: Concurrent Desired-State Safety

Before committing successful expansion, the system MUST reconcile against current durable desired state. The commit MUST preserve concurrent unrelated desired-state changes. If the pack is concurrently deselected, the system MUST NOT resurrect any Zoho ID through expansion persistence.

#### Scenario: Unrelated state changes during sync

- GIVEN an eligible expansion is in progress
- AND another actor adds an unrelated desired-state ID before expansion commit
- WHEN the eligible sync completes successfully
- THEN the unrelated ID MUST remain in durable desired state
- AND only eligible Zoho additions MAY be committed.

#### Scenario: Pack is deselected during sync

- GIVEN an eligible expansion is in progress
- AND another actor deselects the pack before expansion commit
- WHEN the sync otherwise completes successfully
- THEN the commit MUST NOT restore any Zoho ID removed by that deselection.

### Requirement: Deterministic Successful-Addition Reporting

After a successful durable expansion, the system MUST report every Zoho ID newly added to durable desired state, in lexicographic ID order. This requirement applies to both `zoho-deluge` legacy expansion and future catalog-member expansion. The system MUST NOT report IDs already durable before the run or IDs that did not become durable during the run.

#### Scenario: Legacy expansion reports all newly durable members

- GIVEN desired state containing only the released legacy anchor and a V0 catalog
- WHEN sync completely converges and commits the missing members
- THEN the command MUST report every newly durable Zoho ID in lexicographic ID order
- AND it MUST NOT report `zoho-deluge` as newly added.

#### Scenario: Second successful sync is idempotent

- GIVEN a prior successful sync has durably recorded every current pack member
- WHEN a second sync runs against the unchanged catalog
- THEN it MUST add no Zoho ID to durable desired state
- AND it MUST report no Zoho ID as newly added.

### Requirement: Selected Managed-File Safety

For pack members selected for sync, the system MUST retain established managed-file behavior: selected files MUST be included in planning, tracking, backup, application, mode enforcement, and final verification; byte-identical selected files MUST remain idempotent; changed managed selected files MUST be safely overwritten through the established replacement behavior; unrelated extra files MUST be preserved; symlink directories and ancestors MUST be refused; and a final-file symlink MUST be handled without following its target.

#### Scenario: Selected files coexist with user extras and a safe replacement target

- GIVEN a selected pack member has a managed file requiring replacement and an unrelated extra file in its installed directory
- WHEN a successful sync applies the selected member
- THEN the managed file MUST converge through the established safe overwrite behavior
- AND the unrelated extra file MUST remain preserved.

#### Scenario: Selected path encounters a symlink hazard

- GIVEN a selected pack member path includes a symlink directory, symlink ancestor, or symlink final file
- WHEN sync applies the plan
- THEN the system MUST enforce its established symlink-safety behavior
- AND it MUST NOT follow an unsafe symlink target.

### Requirement: Sync Interface Stability

`jarvis sync` MUST remain flagless and non-interactive while performing pack expansion. Pack expansion MUST NOT introduce a prompt or alter the existing rejection of supplied sync flags.

#### Scenario: Non-interactive sync invocation

- GIVEN a selected or legacy-eligible pack state
- WHEN `jarvis sync` is invoked without flags in a non-interactive environment
- THEN the command MUST perform its existing non-interactive sync behavior with any eligible in-memory expansion
- AND it MUST not request user input.

#### Scenario: Flagged sync invocation

- GIVEN any desired state
- WHEN `jarvis sync` is invoked with a flag
- THEN the command MUST retain its existing flag-rejection behavior.

### Requirement: Issue #547 End-to-End Boundary

This change MUST NOT add or claim completion of nested-reference behavior or Claude Code/OpenCode end-to-end parity owned by issue #547.

#### Scenario: Zoho pack verification scope

- GIVEN tests and verification for this change
- WHEN Zoho Skills Pack acceptance is evaluated
- THEN evidence MUST cover this specification's setup, state, sync, reporting, and file-safety contracts
- AND it MUST NOT claim issue #547 nested-reference or cross-agent end-to-end parity as implemented by this change.
