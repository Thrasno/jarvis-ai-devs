# SDD Workflow — Spec-Driven Development

## Overview

SDD is the structured planning layer for non-trivial changes. Never skip phases for changes that affect multiple files, introduce new architecture, or change existing behavior.

## Dependency Graph

```
proposal → specs ──→ tasks → apply → verify → archive
                ↑
              design
```

## Phases

| Phase | Command | Output | Required Input |
|-------|---------|--------|----------------|
| Explore | `/sdd-explore <topic>` | exploration notes | — |
| Propose | part of `/sdd-new` | proposal | exploration (optional) |
| Spec | `/sdd-spec` | requirements + scenarios | proposal |
| Design | `/sdd-design` | architecture decisions | proposal |
| Tasks | `/sdd-tasks` | implementation checklist | spec + design |
| Apply | `/sdd-apply` | code + tests | tasks + spec + design |
| Verify | `/sdd-verify` | pass/fail report | spec + tasks |
| Archive | `/sdd-archive` | closed artifact | all phases |

## Key Rules

- **Never skip SDD** for changes affecting 3+ files or introducing new architecture
- **Spec first**: requirements and scenarios BEFORE any code
- **Design = decisions**: record WHY, not just WHAT
- **Tasks = checklist**: each task is independently implementable and testable
- **Verify against spec**: verify reads the original spec, not the code
- **Archive always**: close the loop, update main spec if delta spec was used

## When NOT to use SDD

- Single-file cosmetic changes
- Typo fixes
- Updating a comment or README

## Artifact Store

Artifacts live in Hive memory with topic keys:
- `sdd/{change-name}/proposal`
- `sdd/{change-name}/spec`
- `sdd/{change-name}/design`
- `sdd/{change-name}/tasks`
- `sdd/{change-name}/apply-progress`
- `sdd/{change-name}/verify-report`
