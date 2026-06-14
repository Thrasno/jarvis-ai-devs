<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/_shared/openspec-convention.md -->
<!-- Upstream commit: 5f73974b39ae2b9b525ef465b3642030c5f2ce6c; adapted for Jarvis/Hive runtime wording and Jarvis OpenSpec delta-safety conventions. -->
# OpenSpec File Convention (shared across all SDD skills)

## Directory Structure

```
openspec/
├── config.yaml              <- Project-specific SDD config
├── specs/                   <- Source of truth (main specs)
│   └── {domain}/
│       └── spec.md
└── changes/                 <- Active changes
    ├── archive/             <- Completed changes (YYYY-MM-DD-{change-name}/)
    └── {change-name}/       <- Active change folder
        ├── state.yaml       <- DAG state (survives compaction)
        ├── explore.md       <- (optional) from sdd-explore
        ├── proposal.md      <- from sdd-propose
        ├── specs/           <- from sdd-spec
        │   └── {domain}/
        │       └── spec.md  <- Delta spec
        ├── design.md        <- from sdd-design
        ├── tasks.md         <- from sdd-tasks (updated by sdd-apply)
        └── verify-report.md <- from sdd-verify
```

## Artifact File Paths

| Skill | Creates / Reads | Path |
|-------|----------------|------|
| orchestrator | Creates/Updates | `openspec/changes/{change-name}/state.yaml` |
| sdd-init | Creates | `openspec/config.yaml`, `openspec/specs/`, `openspec/changes/`, `openspec/changes/archive/` |
| sdd-explore | Creates (optional) | `openspec/changes/{change-name}/explore.md` |
| sdd-propose | Creates | `openspec/changes/{change-name}/proposal.md` |
| sdd-spec | Creates | `openspec/changes/{change-name}/specs/{domain}/spec.md` |
| sdd-design | Creates | `openspec/changes/{change-name}/design.md` |
| sdd-tasks | Creates | `openspec/changes/{change-name}/tasks.md` |
| sdd-apply | Updates | `openspec/changes/{change-name}/tasks.md` (marks `[x]`) |
| sdd-verify | Creates | `openspec/changes/{change-name}/verify-report.md` |
| sdd-archive | Moves | `openspec/changes/{change-name}/` → `openspec/changes/archive/YYYY-MM-DD-{change-name}/` |
| sdd-archive | Updates | `openspec/specs/{domain}/spec.md` (merges deltas into main specs) |

## Reading Artifacts

```
Proposal:   openspec/changes/{change-name}/proposal.md
Explore:    openspec/changes/{change-name}/explore.md
Specs:      openspec/changes/{change-name}/specs/  (all domain subdirectories)
Design:     openspec/changes/{change-name}/design.md
Tasks:      openspec/changes/{change-name}/tasks.md
Verify:     openspec/changes/{change-name}/verify-report.md
Config:     openspec/config.yaml
Main specs: openspec/specs/{domain}/spec.md
```

## Writing Rules

- Always create the change directory before writing artifacts.
- If a file already exists, read it first and update it; do not overwrite blindly.
- If the change directory already exists with artifacts, the change is being continued.
- Use `openspec/config.yaml` `rules` section for project-specific constraints per phase.

## Delta Spec Sections

Delta specs describe requirement-level changes with one or more of these sections:

## ADDED Requirements

Use this section for new requirements that do not replace or rename existing behavior.

## MODIFIED Requirements

Use this section for requirements whose behavior changes. Include the full updated requirement block because archive replaces the existing requirement with the delta block.

## REMOVED Requirements

Use this section for requirements intentionally removed from the main spec. REMOVED requirements MUST include non-empty, non-placeholder Reason and Migration evidence. `Migration: None` is allowed only when the delta explicitly justifies why no replacement or user/operator migration is needed.

```markdown
### Requirement: {Requirement Being Removed}

(Reason: {why this requirement is being removed})
(Migration: {what replaces this behavior, or `None` with justification})
```

## RENAMED Requirements

Use this section when the requirement identity/name changes but the behavior is otherwise preserved. RENAMED requirements MUST include explicit old and new requirement names.

```markdown
### Requirement: {New Requirement Name}

(Old name: {Existing Requirement Name})
(New name: {New Requirement Name})
(Reason: {why the requirement is being renamed})
```

## Config File Reference

```yaml
schema: spec-driven

context: |
  Tech stack: {detected}
  Architecture: {detected}
  Testing: {detected}
  Style: {detected}

rules:
  proposal:
    - Include rollback plan for risky changes
  specs:
    - Use Given/When/Then for scenarios
    - Use RFC 2119 keywords (MUST, SHALL, SHOULD, MAY)
  design:
    - Include sequence diagrams for complex flows
    - Document architecture decisions with rationale
  tasks:
    - Group by phase, use hierarchical numbering
    - Keep tasks completable in one session
  apply:
    - Follow existing code patterns
    tdd: false
    test_command: ""
  verify:
    test_command: ""
    build_command: ""
    coverage_threshold: 0
  archive:
    - Warn before merging destructive deltas
```

## Archive Structure

When archiving, the change folder moves to:

```
openspec/changes/archive/YYYY-MM-DD-{change-name}/
```

Use today's date in ISO format. The archive is an audit trail — never delete or modify archived changes.
