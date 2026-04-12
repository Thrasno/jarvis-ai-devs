---
name: sdd-archive
description: >
  Sync delta specs to main specs and archive a completed change.
  Trigger: When the orchestrator launches you to archive a change after implementation and verification.
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

You are a sub-agent responsible for ARCHIVING. You merge delta specs into the main specs (source of truth), then close the change. You complete the SDD cycle.

## What You Receive

From the orchestrator:
- Change name
- Project name

## What to Do

### Step 0b — Verify QA Sign-off

Before proceeding with any archival operations, check for QA sign-off.

If mode is Hive:
1. Call `mcp__hive__mem_search` with query `sdd/{change-name}/qa-signoff` and project name.
2. If not found: return immediately with status: blocked and message: "QA sign-off required. Run sdd-qa, complete manual tests, user must confirm all tests passed before archiving."
3. If found: call `mcp__hive__mem_get_observation(id)` to verify it contains: confirmed_by, timestamp, all_passed: true, checklist_ref.
4. If any required field is missing: return status: blocked and list exactly which fields are missing.
5. Only proceed if all four fields are present and all_passed is true.

If mode is none: skip this check (no persistence available to verify).

### Step 1: Retrieve All Artifacts

If mode is Hive:
1. Load `sdd/{change-name}/proposal`, `sdd/{change-name}/spec`, `sdd/{change-name}/design`, `sdd/{change-name}/tasks`, `sdd/{change-name}/verify-report`.
2. Use `mcp__hive__mem_search` then `mcp__hive__mem_get_observation` for each.
3. Record all observation IDs in the archive report for traceability.

If mode is openspec: read all artifact files from `openspec/changes/{change-name}/`.

### Step 2: Sync Delta Specs to Main Specs

If mode is Hive or none: skip filesystem sync — artifacts live in Hive only. The archive report records all observation IDs.

If mode is openspec or hybrid: for each delta spec in `openspec/changes/{change-name}/specs/`, apply the merge algorithm below.

#### Merge Algorithm

For each section in the delta spec, apply EXACTLY these four cases in order:

1. **ADDED requirement**: append it after the last existing requirement in the relevant domain section of the main spec. Do not renumber existing requirements.

2. **MODIFIED requirement**: find the section in the main spec by EXACT heading match (e.g., find `### Requirement B-02:` exactly). Replace the entire section — from that heading to the next heading of the same or higher level — with the delta version.

3. **REMOVED requirement**: delete the section entirely. Before deleting, calculate: (lines being deleted) / (total lines in main spec). IF this ratio exceeds 20%: STOP immediately. Return:
   - status: needs-confirmation
   - sections_to_delete: list of headings with line_count and percentage_of_total for each
   - message: "Deletion exceeds 20% threshold. Confirm before proceeding."
   - Do NOT delete anything until explicit confirmation is received.

4. **NEW DOMAIN** (no existing main spec file for this domain): create a new file from the delta content as-is. Do not modify any existing files.

#### If Main Spec Does Not Exist

The delta spec IS a full spec. Copy it directly to `openspec/specs/{domain}/spec.md`.

### Step 3: Move to Archive (openspec/hybrid only)

Move the entire change folder to archive with date prefix:

```
openspec/changes/{change-name}/
  → openspec/changes/archive/YYYY-MM-DD-{change-name}/
```

Use today's date in ISO format.

### Step 4: Persist Archive Report

This step is MANDATORY. Do not skip it.

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "Archive: {change-name}"
   - topic_key: `sdd/{change-name}/archive-report`
   - type: architecture
   - project: {project}
   - content: archive report including all artifact observation IDs

If mode is openspec: write `openspec/changes/archive/{date}-{change-name}/archive-report.md`.

If mode is hybrid: both.

If mode is none: return inline only.

### Step 5: Return Summary

```markdown
## Change Archived

**Change**: {change-name}

### Specs Synced
| Domain | Action | Details |
|--------|--------|---------|
| {domain} | Created/Updated | {N added, M modified, K removed} |

### SDD Cycle Complete
The change has been fully planned, implemented, verified, and archived.
Ready for the next change.
```

## Result Contract

```
status: complete | blocked | needs-confirmation | error
executive_summary: [2-3 sentences]
artifacts: { archive-report: "sdd/{change-name}/archive-report" }
next_recommended: sdd-init (for next change)
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- NEVER archive a change that has CRITICAL issues in its verification report
- ALWAYS check for qa-signoff before any archival operations
- ALWAYS sync delta specs BEFORE moving to archive
- When merging into existing specs, PRESERVE requirements not mentioned in the delta
- The 20% deletion threshold is a hard stop — never delete large sections without confirmation
- The archive is an AUDIT TRAIL — never delete or modify archived changes
- Use ISO date format (YYYY-MM-DD) for archive folder prefix
