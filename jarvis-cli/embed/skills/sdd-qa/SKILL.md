---
name: sdd-qa
description: >
  Run the QA checklist for a change — mix of automated and manual test items.
  Position in DAG: after sdd-apply, before sdd-verify.
  NEVER SKIPPABLE.
  Trigger: When the orchestrator launches you to perform QA for a change.
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Step 0 — Resolve Persistence Mode

1. **Default**: Hive (`mcp__hive__*` tools)
2. **Override**: openspec or hybrid — if user explicitly requests it
3. **Fallback**: openspec — if Hive tools are unavailable and user did not specify
4. **None**: only if user explicitly requests it

Carry this decision through all steps. Do not re-evaluate mid-skill.

## NEVER SKIP RULE

sdd-qa MUST run for every change before sdd-verify.

**It MUST NOT be skipped even if**:
- project has strict_tdd: true
- test coverage is 100%
- the change is described as "trivial" or "documentation-only"
- all automated tests already passed during sdd-apply

There are NO exceptions to this rule.

## Position in DAG

sdd-qa runs AFTER sdd-apply and BEFORE sdd-verify.

sdd-archive checks for qa-signoff before proceeding. If no qa-signoff exists, sdd-archive returns status: blocked.

## Purpose

You generate a mixed [AUTO]/[MANUAL] QA checklist from spec scenarios, execute all automatable tests immediately, present manual items to the user, and save a qa-signoff after the user confirms all tests passed.

## What You Receive

From the orchestrator:
- Change name
- Project name
- Project context (strict_tdd, test_runner)

## What to Do

### Step 1: Read Context

If mode is Hive:
1. Load project context: `mcp__hive__mem_search` with `sdd-init/{project}`, then `mcp__hive__mem_get_observation(id)`. Extract: strict_tdd, test_runner.
2. Load spec scenarios: search `sdd/{change-name}/spec`, then get full content.
3. Load apply progress: search `sdd/{change-name}/apply-progress`, then get full content.

If mode is openspec: read `openspec/config.yaml`, `openspec/changes/{change-name}/specs/`, and `openspec/changes/{change-name}/apply-progress.md`.

### Step 2: Generate Checklist

For each spec scenario and behavioral requirement:

- If a test runner is available AND the scenario is automatable (unit test, integration test, linter check, build check): tag it [AUTO]
- If the scenario requires human judgment, UI interaction, UX evaluation, or cannot be automated: tag it [MANUAL]

For [AUTO] items: execute the test command immediately. Record the actual result.

For [MANUAL] items: write step-by-step instructions, expected outcome, and empty result fields.

Even for trivial or documentation-only changes: generate at least 1-2 [MANUAL] items covering basic sanity checks.

### Step 3: Save Checklist to Hive

This step is MANDATORY. Do not skip it.

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "QA Checklist: {change-name}"
   - topic_key: `sdd/{change-name}/qa-checklist`
   - type: architecture
   - project: {project}
   - content: checklist in the EXACT format below

The checklist MUST follow this EXACT format. No other format is acceptable. Do not invent custom sections or omit any field:

```
# QA Checklist: {change-name}
Generated: {ISO date}

---

## [AUTO] {test name}
Command: `{command run}`
Result: ✅ PASS ({duration}ms) | ❌ FAIL
Output:
```
{relevant output excerpt}
```

---

## [MANUAL] {scenario name}
Steps:
  1. {step}
  2. {step}
Expected: {expected outcome}
Result: ⬜ PASS | ⬜ FAIL
Notes: ___
Tester: ___

---
```

If mode is openspec: write `openspec/changes/{change-name}/qa-checklist.md`.

If mode is none: present inline only.

### Step 4: Present to User and Wait

Present the complete checklist to the user.

State clearly:
- [AUTO] items have already been executed and results are recorded above
- [MANUAL] items require human verification
- The user must complete all [MANUAL] items and confirm all tests passed

**NEVER self-confirm.** NEVER assume all tests passed. NEVER proceed to Step 5 without explicit user confirmation.

Wait for the user's explicit confirmation that all tests passed.

### Step 5: Save QA Sign-off

After the user explicitly confirms all tests passed:

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "QA sign-off: {change-name}"
   - topic_key: `sdd/{change-name}/qa-signoff`
   - type: architecture
   - project: {project}
   - content:
     ```
     confirmed_by: {user identifier or "user"}
     timestamp: {ISO 8601 datetime}
     all_passed: true
     checklist_ref: sdd/{change-name}/qa-checklist
     ```

If mode is openspec: write `openspec/changes/{change-name}/qa-signoff.md` with same four fields.

## Result Contract

```
status: complete | blocked | error
executive_summary: [2-3 sentences describing what was tested and results]
artifacts: { qa-checklist: "sdd/{change-name}/qa-checklist", qa-signoff: "sdd/{change-name}/qa-signoff" }
next_recommended: sdd-verify
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- NEVER skip this skill — not for any reason
- NEVER self-confirm that tests passed
- NEVER proceed to save qa-signoff without explicit user confirmation
- sdd-archive will block if qa-signoff is missing
- The [AUTO]/[MANUAL] checklist format is exact — do not modify it
- Present the full checklist to the user before waiting for confirmation
