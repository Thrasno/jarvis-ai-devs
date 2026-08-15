# Sync Replay Application Specification

## Purpose

Defines the mutation stage of `jarvis sync`: applying the plan produced by
sync-replay-planning to disk in a fixed, tested component order, replaying
instructions, Jarvis-managed MCPs, skills, persona, models, and the
statusline, and reporting outcomes honestly — including when one agent
converges and another fails.

## Requirements

### Requirement: Component Application Order Contract

The system MUST apply components in a fixed order, with persona applied
LAST, after every content-injecting component (instructions, skills). Jarvis
inverts the `gentle-ai` reference order because a single writer
(`WriteInstructions`) owns the whole instruction file, rebuilding or
patching it and re-injecting the Hive protocol and orchestrator import
itself; persona must layer on top of that fully-assembled file, not under
it. The order MUST be locked by a test that fails if the order changes.

#### Scenario: Persona runs after content injectors

- GIVEN a replay that applies instructions, skills, and persona
- WHEN the component order is inspected
- THEN persona applies after every component that injects content into
  shared files, and persona is the last component applied

#### Scenario: A test fails on reordering

- GIVEN the component order test
- WHEN the application order is changed without updating the contract
- THEN the test fails

### Requirement: Machine-Scoped Artifact Replay

The system MUST replay instructions, Jarvis-managed MCPs, skills, persona,
model assignments, and the statusline against the manifest and the currently
installed version's embedded assets. Jarvis-managed MCPs MUST be replaced
unconditionally, since they are derived from embedded assets and are never a
persisted user choice. Byte-for-byte preservation applies only per the
Managed Instruction File Ownership Scope requirement below; it does not cover
the whole path of every shared file unconditionally.

#### Scenario: Replay brings artifacts to the installed version

- GIVEN a manifest from an older installed version and a newer installed
  binary
- WHEN `jarvis sync` runs
- THEN instructions, skills, persona, models, and eligible statusline state
  match the newer version's embedded assets, with no interactive prompts

### Requirement: Managed Instruction File Ownership Scope

For a `CLAUDE.md`/`AGENTS.md` belonging to a manifest-listed agent, the
system MUST treat the whole path as Jarvis-owned. When the file carries
Jarvis sentinels, the system MUST patch only the managed sections in place
and MUST preserve content outside those sections byte-for-byte. When the
file carries no Jarvis sentinels, the system MUST render it fresh and
discard the previous content, matching installer behavior
(`WriteInstructions`); the pre-apply backup snapshot is the only recovery
path for that discarded content. For a file at a path Jarvis does not own,
the system MUST NOT touch it.

#### Scenario: Sentinel-bearing file preserves content outside managed sections

- GIVEN a manifest-listed agent's instruction file that carries Jarvis
  sentinels and contains user-authored content outside the managed sections
- WHEN replay applies the instructions component
- THEN the managed sections are patched in place
- AND the content outside the managed sections is preserved byte-for-byte

#### Scenario: No-sentinel managed file is rendered fresh

- GIVEN a manifest-listed agent's instruction file with no Jarvis sentinels
  and pre-existing content
- WHEN replay applies the instructions component
- THEN the file is rendered fresh from embedded assets
- AND the previous content is discarded
- AND that content is recoverable only from the pre-apply backup snapshot

#### Scenario: A file at an unowned path is never touched

- GIVEN a file at a path that does not belong to a manifest-listed agent's
  instruction file
- WHEN `jarvis sync` runs
- THEN the file is not read, modified, or replaced

### Requirement: Statusline Reinstallation on Drift

When the manifest records statusline consent as decided-enabled but the
statusline script is absent from disk, the system MUST treat this as drift,
not revocation, and MUST reinstall the script. The manifest is the sole
authority for statusline intent.

#### Scenario: Deleted script is reinstalled

- GIVEN a manifest with statusline consent decided-enabled and the statusline
  script deleted from disk
- WHEN `jarvis sync` runs
- THEN the statusline script is reinstalled
- AND the manifest's decided-enabled state is unchanged

### Requirement: Partial Failure Reporting Across Agents

When replay succeeds for one configured agent and fails for another, the
system MUST NOT roll back the successful agent's changes. It MUST report
which agents converged and which did not, including the failure cause for
each failed agent, MUST exit with a non-zero status, and MUST NOT report
global convergence.

#### Scenario: One agent succeeds, another fails

- GIVEN two configured agents where replay completes for the first and fails
  midway for the second
- WHEN `jarvis sync` finishes
- THEN the first agent's applied changes remain in place
- AND the report names the first agent as converged and the second as failed
  with its cause
- AND the process exits with a non-zero status
- AND the report does not claim global convergence
- AND the pre-apply backup snapshot remains available for manual restore
