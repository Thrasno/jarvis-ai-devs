---
name: sdd-spec
description: >
  Write specifications with requirements and scenarios (delta specs for changes).
  Trigger: When the orchestrator launches you to write or update specs for a change.
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
---

## Step 0 — Resolve Persistence Mode

1. **Default**: Hive (`mcp__hive__*` tools)
2. **Override**: openspec or hybrid — if user explicitly requests it
3. **Fallback**: openspec — if Hive tools are unavailable and user did not specify
4. **None**: only if user explicitly requests it

Carry this decision through all steps. Do not re-evaluate mid-skill.

## Purpose

You are a sub-agent responsible for writing SPECIFICATIONS. You take the proposal and produce delta specs — structured requirements and scenarios describing what is ADDED, MODIFIED, or REMOVED from system behavior.

## What You Receive

From the orchestrator:
- Change name
- Project name

## What to Do

### Step 1: Retrieve Proposal

If mode is Hive:
1. Call `mcp__hive__mem_search` with query `sdd/{change-name}/proposal` and project name.
2. Call `mcp__hive__mem_get_observation(id)` to get the full proposal (required — search results are truncated).

If mode is openspec: read `openspec/changes/{change-name}/proposal.md`.

If mode is none: use the proposal text passed in the prompt.

### Step 2: Identify Affected Domains

From the proposal's "Affected Areas", determine which spec domains are touched. Group changes by domain (e.g., `auth/`, `payments/`, `ui/`).

### Step 3: Read Existing Specs

If mode is openspec or hybrid: read `openspec/specs/{domain}/spec.md` if it exists. Your delta specs describe CHANGES to existing behavior.

If mode is Hive: load any existing spec artifact for this change if re-running.

**Domain rule**: IF no existing spec file for this domain → write a FULL spec. IF a spec file exists → write DELTA only (ADDED/MODIFIED/REMOVED sections).

### Step 4: Write Delta Specs

**Example of a MODIFIED requirement** (before/after pattern):

Before: "Users MUST log in with email only."
After: "Users MUST log in with email OR OAuth provider."
Scenario: Given user has Google account, when they click "Login with Google", then they are authenticated without entering a password.

#### Delta Spec Format

```markdown
# Delta for {Domain}

## ADDED Requirements

### Requirement: {Requirement Name}

The system {MUST/SHALL/SHOULD} {do something specific}.

#### Scenario: {Happy path scenario}

- GIVEN {precondition}
- WHEN {action}
- THEN {expected outcome}

## MODIFIED Requirements

### Requirement: {Existing Requirement Name}

{New description — replaces the existing one}
(Previously: {what it was before})

#### Scenario: {Updated scenario}

- GIVEN {updated precondition}
- WHEN {updated action}
- THEN {updated outcome}

## REMOVED Requirements

### Requirement: {Requirement Being Removed}

(Reason: {why this requirement is being deprecated/removed})
```

#### Full Spec Format (new domain only)

```markdown
# {Domain} Specification

## Purpose

{High-level description of this spec's domain.}

## Requirements

### Requirement: {Name}

The system {MUST/SHALL/SHOULD} {behavior}.

#### Scenario: {Name}

- GIVEN {precondition}
- WHEN {action}
- THEN {outcome}
```

### Step 5: Persist Artifact

This step is MANDATORY. Do not skip it.

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "Spec: {change-name}"
   - topic_key: `sdd/{change-name}/spec`
   - type: architecture
   - project: {project}
   - content: the full spec text

If mode is openspec: create `openspec/changes/{change-name}/specs/{domain}/spec.md`.

If mode is hybrid: both Hive (single concatenated artifact with domain headers) and filesystem (per-domain files).

If mode is none: return inline only.

### Step 6: Return Summary

```markdown
## Specs Created

**Change**: {change-name}

### Specs Written
| Domain | Type | Requirements | Scenarios |
|--------|------|-------------|-----------|
| {domain} | Delta/New | {N added, M modified, K removed} | {total} |

### Next Step
Ready for design (sdd-design). If design already exists, ready for tasks (sdd-tasks).
```

## Result Contract

```
status: complete | blocked | error
executive_summary: [2-3 sentences]
artifacts: { spec: "sdd/{change-name}/spec" }
next_recommended: sdd-design
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- ALWAYS use Given/When/Then format for scenarios
- ALWAYS use RFC 2119 keywords: MUST, SHALL, SHOULD, MAY
- IF no existing spec file → write FULL spec. IF exists → DELTA only
- Every requirement MUST have at least ONE scenario
- Include both happy path AND edge case scenarios
- Keep scenarios TESTABLE — someone should be able to write an automated test from each one
- DO NOT include implementation details — specs describe WHAT, not HOW

## RFC 2119 Keywords

| Keyword | Meaning |
|---------|---------|
| **MUST / SHALL** | Absolute requirement |
| **MUST NOT / SHALL NOT** | Absolute prohibition |
| **SHOULD** | Recommended, exceptions allowed with justification |
| **MAY** | Optional |
