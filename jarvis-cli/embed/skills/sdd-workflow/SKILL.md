---
name: sdd-workflow
description: >
  SDD Workflow — Spec-Driven Development lifecycle and rules.
  Trigger: When user asks about the SDD process, phases, or workflow.
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
---

## Overview

SDD is the structured planning layer for non-trivial changes. Never skip phases for changes that affect multiple files, introduce new architecture, or change existing behavior.

## Dependency Graph

```
proposal → specs ──→ tasks → apply → sdd-qa → verify → archive
                ↑
              design
```

sdd-qa runs AFTER apply and BEFORE verify. It is never skippable.

## Phases

| Phase | Command | Output | Required Input |
|-------|---------|--------|----------------|
| Explore | `/sdd-explore <topic>` | exploration notes | — |
| Propose | part of `/sdd-new` | proposal | exploration (optional) |
| Spec | `/sdd-spec` | requirements + scenarios | proposal |
| Design | `/sdd-design` | architecture decisions | proposal |
| Tasks | `/sdd-tasks` | implementation checklist | spec + design |
| Apply | `/sdd-apply` | code + tests | tasks + spec + design |
| QA | `/sdd-qa` | [AUTO]/[MANUAL] checklist | spec + apply-progress |
| Verify | `/sdd-verify` | pass/fail report | spec + tasks |
| Archive | `/sdd-archive` | closed artifact | all phases + qa-signoff |

## Step 0 — Resolve Persistence Mode

All four persistence modes are supported. Resolve mode at session start:

| Mode | When | Backend |
|------|------|---------|
| hive | Default — always unless overridden | `mcp__hive__*` calls |
| openspec | User says "use openspec" or "save to files" | Files under `openspec/changes/{change-name}/` |
| hybrid | User says "both" or "hybrid" | Both Hive and openspec |
| none | User says "inline only" or "no persistence" | Return results only, no saves |

### Hive topic keys

- `sdd-init/{project}` — project context
- `sdd/{change-name}/proposal`
- `sdd/{change-name}/spec`
- `sdd/{change-name}/design`
- `sdd/{change-name}/tasks`
- `sdd/{change-name}/apply-progress`
- `sdd/{change-name}/qa-checklist`
- `sdd/{change-name}/qa-signoff`
- `sdd/{change-name}/verify-report`
- `sdd/{change-name}/archive-report`

## Key Rules

- **Never skip SDD** for changes affecting 3+ files or introducing new architecture
- **Spec first**: requirements and scenarios BEFORE any code
- **Design = decisions**: record WHY, not just WHAT
- **Tasks = checklist**: each task is independently implementable and testable
- **sdd-qa is MANDATORY**: runs after apply, before verify. Never skip for any reason
- **Archive requires qa-signoff**: sdd-archive will block if sdd-qa has not been completed
- **Verify against spec**: sdd-verify reads the original spec, not the code
- **Archive always**: close the loop, update main spec if delta spec was used

## When NOT to use SDD

- Single-file cosmetic changes
- Typo fixes
- Updating a comment or README

## Phase Dependencies

| Phase | Reads | Writes |
|-------|-------|--------|
| sdd-explore | sdd-init/{project} (optional) | sdd/{change}/explore |
| sdd-propose | explore (optional) | sdd/{change}/proposal |
| sdd-spec | proposal (required) | sdd/{change}/spec |
| sdd-design | proposal (required), spec (optional) | sdd/{change}/design |
| sdd-tasks | spec + design (required) | sdd/{change}/tasks |
| sdd-apply | tasks + spec + design | sdd/{change}/apply-progress |
| sdd-qa | spec + apply-progress | sdd/{change}/qa-checklist, qa-signoff |
| sdd-verify | spec + tasks | sdd/{change}/verify-report |
| sdd-archive | all artifacts + qa-signoff | sdd/{change}/archive-report |
