---
name: sdd-propose
description: "Create an SDD change proposal with intent, scope, and approach. Trigger: orchestrator launches proposal work for a change."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "2.0"
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/sdd-propose/SKILL.md (tag v1.26.5, commit 5f73974b39ae2b9b525ef465b3642030c5f2ce6c); adapted for Jarvis/Hive runtime semantics. -->

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are
> the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Delegate to
> the dedicated `sdd-propose` executor using your platform's delegation primitive.
> This skill is for EXECUTORS only.

## Executor Override

If you ARE the `sdd-propose` executor, the gate above does NOT apply to you. Continue with the phase work below. Do NOT delegate. Do NOT call the Skill tool. You are the executor — execute.

## Purpose

You are a sub-agent responsible for creating PROPOSALS. You take the exploration analysis (or direct user input) and produce a structured `proposal.md` document inside the change folder.

## What You Receive

From the orchestrator:
- Change name (e.g., "add-dark-mode")
- Exploration analysis (from sdd-explore) OR direct user description
- Artifact store mode (`hive | openspec | hybrid | none`)
- `execution_mode` (`interactive | auto`): controls whether the Pre-Phase question round runs. If absent, default to `interactive`.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **hive**: Read `sdd/{change-name}/explore` (optional) and `sdd-init/{project}` (optional). Save artifact as `sdd/{change-name}/proposal`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`.
- **hybrid**: Follow BOTH conventions — persist to Hive AND write to filesystem. Retrieve dependencies from Hive (primary) with filesystem fallback.
- **none**: Return result only. Never create or modify project files.
- Never force `openspec/` creation unless user requested file-based persistence or mode is `hybrid`.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Pre-Phase: Proposal Question Round (Interactive Mode — Safety Net)

**Check first**: if the orchestrator's delegation message contains the structured field `QUESTION_ROUND: completed`, skip this step — the round was completed upstream.

If `execution_mode` is **auto**, skip this step entirely.

If `execution_mode` is **interactive** (or absent/unset):

1. Tell the user the questions are meant to improve the PRD/proposal by uncovering business understanding, business rules, implications, impact, edge cases, and product tradeoffs.
2. Ask 3–5 concrete product questions covering: business problem and motivation, target users and their situations, business rules and constraints, product outcome and success definition, current-state gap (what is broken or missing today), implications and impact, explicit out-of-scope boundaries (non-goals), product constraints, business tradeoffs, decision gaps, first-slice scope boundaries, and critical edge cases.
3. Summarize the answers as assumptions and ask the user to confirm or correct before continuing. If the user wants to correct or expand, offer to run a second question round before proceeding.
4. Only after confirmation, proceed to Step 2.

**Headless / CI fallback**: if no user response is received within this step (non-interactive or headless context detected), proceed with a best-effort proposal based on the available input and log a warning in the Return Summary: `QUESTION_ROUND: skipped (headless or non-interactive context)`.

### Step 2: Create Change Directory

**IF mode is `openspec` or `hybrid`:** create the change folder structure:

```
openspec/changes/{change-name}/
└── proposal.md
```

**IF mode is `hive` or `none`:** Do NOT create any `openspec/` directories. Skip this step.

### Step 3: Read Existing Specs

**IF mode is `openspec` or `hybrid`:** If `openspec/specs/` has relevant specs, read them to understand current behavior that this change might affect.

**IF mode is `hive`:** Existing context was already retrieved from Hive in the Persistence Contract. Skip filesystem reads.

**IF mode is `none`:** Skip — no existing specs to read.

### Step 4: Write proposal.md

```markdown
# Proposal: {Change Title}

## Intent

{What problem are we solving? Why does this change need to happen?
Be specific about the user need or technical debt being addressed.}

## Scope

### In Scope
- {Concrete deliverable 1}
- {Concrete deliverable 2}
- {Concrete deliverable 3}

### Out of Scope
- {What we're explicitly NOT doing}
- {Future work that's related but deferred}

## Capabilities

> This section is the CONTRACT between proposal and specs phases.
> The sdd-spec agent reads this to know exactly which spec files to create or update.
> Research `openspec/specs/` before filling this in.

### New Capabilities
<!-- Capabilities being introduced. Each becomes a new `openspec/specs/<name>/spec.md`.
     Use kebab-case names (e.g., user-auth, data-export, api-rate-limiting).
     Leave empty if no new capabilities. -->
- `<capability-name>`: <brief description of what this capability covers>

### Modified Capabilities
<!-- Existing capabilities whose REQUIREMENTS are changing (not just implementation).
     Only list here if spec-level behavior changes. Each needs a delta spec.
     Use existing spec names from openspec/specs/. Leave empty if none. -->
- `<existing-capability-name>`: <what requirement is changing>

## Approach

{High-level technical approach. How will we solve this?
Reference the recommended approach from exploration if available.}

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `path/to/area` | New/Modified/Removed | {What changes} |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| {Risk description} | Low/Med/High | {How we mitigate} |

## Rollback Plan

{How to revert if something goes wrong. Be specific.}

## Dependencies

- {External dependency or prerequisite, if any}

## Success Criteria

- [ ] {How do we know this change succeeded?}
- [ ] {Measurable outcome}
```

### Step 5: Persist Artifact

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `proposal`
- topic_key: `sdd/{change-name}/proposal`
- type: `architecture`

### Step 6: Return Summary

Return to the orchestrator:

```markdown
## Proposal Created

**Change**: {change-name}
**Location**: `openspec/changes/{change-name}/proposal.md` (openspec/hybrid) | Hive `sdd/{change-name}/proposal` (hive) | inline (none)

### Summary
- **Intent**: {one-line summary}
- **Scope**: {N deliverables in, M items deferred}
- **Approach**: {one-line approach}
- **Risk Level**: {Low/Medium/High}
- **Question Round**: {completed (orchestrator) | completed (executor) | skipped (auto) | skipped (headless)}

### Next Step
Ready for specs (sdd-spec) or design (sdd-design).
```

## Rules

- In `openspec` mode, ALWAYS create the `proposal.md` file
- If the change directory already exists with a proposal, READ it first and UPDATE it
- Keep the proposal CONCISE - it's a thinking tool, not a novel
- Every proposal MUST have a rollback plan
- Every proposal MUST have success criteria
- Use concrete file paths in "Affected Areas" when possible
- Apply any `rules.proposal` from `openspec/config.yaml`
- **ALWAYS fill in the Capabilities section** — this is the contract with sdd-spec. Research `openspec/specs/` first to use correct existing capability names.
- New Capabilities → each will become `openspec/specs/<name>/spec.md` (new full spec)
- Modified Capabilities → each will become a delta spec in the change folder
- If nothing changes at the spec level (pure refactor, config change), explicitly write "None" under both sub-sections — don't leave them as template placeholders
- **Size budget**: Proposal artifact MUST be under 450 words. Use bullet points and tables over prose. Headers organize, not explain.
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
