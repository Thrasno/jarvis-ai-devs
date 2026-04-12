---
name: sdd-init
description: >
  Initialize Spec-Driven Development context in any project. Detects stack, conventions, testing capabilities, and bootstraps the active persistence backend.
  Trigger: When user wants to initialize SDD in a project, or says "sdd init", "iniciar sdd", "openspec init".
license: MIT
metadata:
  author: gentleman-programming
  version: "4.0"
---

## Step 0 — Resolve Persistence Mode

1. **Default**: Hive (`mcp__hive__*` tools)
2. **Override**: openspec or hybrid — if user explicitly requests it
3. **Fallback**: openspec — if Hive tools are unavailable and user did not specify
4. **None**: only if user explicitly requests it

Carry this decision through all steps. Do not re-evaluate mid-skill.

## Role

You are an **EXECUTOR** with ONE interactive pause. You run all detection steps autonomously. The ONLY pause is Step 3 (Strict TDD choice). For Step 3: include the question in your return summary. The orchestrator presents it to the user and passes the answer back. Do NOT ask the user directly during execution.

## Purpose

You detect the project stack, conventions, and testing capabilities, then save the project context to the active persistence backend. You do NOT launch sub-agents, do NOT coordinate other phases, and do NOT hand execution back unless you hit a real blocker.

## What to Do

### Step 1: Detect Project Context

Read the project to understand:
- Tech stack (check package.json, go.mod, pyproject.toml, composer.json, Cargo.toml, etc.)
- Existing conventions (linters, test frameworks, CI configuration)
- Architecture patterns in use (directory structure, naming conventions)

### Step 2 — Detect Testing Capabilities

2a. Check for test runner:
- `go.mod` present → runner: `go test` (built-in)
- `composer.json` has `phpunit/phpunit` in require-dev → runner: `phpunit`
- `package.json` has `jest` in scripts or devDependencies → runner: `jest`
- `package.json` has `vitest` in scripts or devDependencies → runner: `vitest`
- `package.json` has `mocha` in devDependencies → runner: `mocha`
- `package.json` has a `test` script → runner: `npm test`
- `pyproject.toml` or `requirements.txt` contains `pytest` → runner: `pytest`
- `Cargo.toml` present → runner: `cargo test` (built-in)
- `Makefile` with a `test` target → runner: `make test`
- None of the above → runner: none

2b. Check for coverage tool:
- JS/TS: `vitest --coverage`, `jest --coverage`, `c8`, `istanbul/nyc` → coverage_available: true
- Python: `coverage.py` or `pytest-cov` → coverage_available: true
- Go: `go test -cover` (built-in) → coverage_available: true
- None found → coverage_available: false

2c. Set strict_tdd: false — Step 3 determines the final value via orchestrator interaction.

### Step 3: Ask STRICT TDD MODE (Interactive Pause)

This is the ONE interactive pause. Do not continue execution. Include this question in your return summary for the orchestrator to present to the user.

Only offer Strict TDD Mode if a test runner was found in Step 2a.

If test runner detected: ask — "Enable Strict TDD Mode? (RED → GREEN → REFACTOR for all implementation tasks). Takes longer, produces better code. [YES / NO]"

If no test runner: set strict_tdd: false, note "Strict TDD Mode unavailable — no test runner detected."

After receiving the user's answer (passed back by orchestrator): set strict_tdd: true or false accordingly.

### Step 4: Initialize Persistence Backend

If mode is openspec or hybrid: create this directory structure:

```
.jarvis/
└── skill-registry.md   ← Built in Step 7
openspec/
├── config.yaml
├── specs/
└── changes/
    └── archive/
```

If mode is Hive or none: do NOT create `openspec/` directories.

### Step 5: Generate Config (openspec/hybrid only)

Create `openspec/config.yaml` with detected context:

```yaml
schema: spec-driven
context: |
  Tech stack: {detected stack}
  Architecture: {detected patterns}
  Testing: {detected test framework}
  Style: {detected linting/formatting}
strict_tdd: {true/false}
```

### Step 6: Persist Testing Capabilities

This step is MANDATORY. Do not skip it.

If mode is Hive or hybrid:
1. Call `mcp__hive__mem_save` with:
   - title: "Testing capabilities: {project}"
   - topic_key: `sdd/{project}/testing-capabilities`
   - type: config
   - project: {project}
   - content: testing capabilities in this format:

```markdown
## Testing Capabilities

**Strict TDD Mode**: {enabled/disabled}
**Detected**: {date}

### Test Runner
- Command: `{command}`
- Framework: {name}

### Coverage
- Available: yes/no
- Command: `{command or —}`
```

### Step 7: Build Skill Registry

Scan for skills and write `.jarvis/skill-registry.md` in the project root. This file is mode-independent — it is infrastructure, not an SDD artifact.

1. Scan `~/.claude/skills/` for `*/SKILL.md` files. Also check `~/.config/opencode/skills/` if opencode is installed. Skip `_shared/` directories.
2. Scan for project conventions: check for `CLAUDE.md`, `AGENTS.md`, `.cursorrules`, `GEMINI.md` in the project root.
3. Write `.jarvis/skill-registry.md` with the registry of found skills and their compact rules.

If mode is Hive or hybrid: also save the registry to Hive:
1. Call `mcp__hive__mem_save` with topic_key `skill-registry`, type: config, project: {project}.

### Step 8: Persist Project Context

This step is MANDATORY. Do not skip it.

If mode is Hive or hybrid:
1. Call `mcp__hive__mem_save` with:
   - title: "sdd-init/{project}"
   - topic_key: `sdd-init/{project}`
   - type: architecture
   - project: {project}
   - content: full detected context including: project name, stack, test_runner, strict_tdd, test_framework, coverage_available

### Step 9: Return Summary

Include:
- Detected project context
- Testing capabilities table
- Strict TDD question (if test runner detected and user has not yet answered)
- Confirmation of what was persisted

```markdown
## SDD Initialized

**Project**: {project name}
**Stack**: {detected stack}
**Strict TDD Mode**: {enabled / disabled / awaiting user input}

### Testing Capabilities
| Capability | Status |
|------------|--------|
| Test Runner | {tool} / not found |
| Coverage | yes / no |

### Context Saved
- topic_key: sdd-init/{project}
- Capabilities: sdd/{project}/testing-capabilities

### Next Steps
Ready for /sdd-explore <topic> or /sdd-new <change-name>.
```

## Result Contract

```
status: complete | awaiting-strict-tdd | blocked | error
executive_summary: [2-3 sentences]
artifacts: { init: "sdd-init/{project}", capabilities: "sdd/{project}/testing-capabilities" }
next_recommended: sdd-explore or sdd-new
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- NEVER create placeholder spec files — specs are created via sdd-spec during a change
- ALWAYS detect the real tech stack — do not guess
- ALWAYS detect testing capabilities — this is not optional
- ALWAYS persist testing capabilities as a separate observation — downstream phases depend on it
- Topic key `sdd-init/{project}` enables upserts — re-running init updates existing context, not duplicates
- If the project already has an `openspec/` directory, report what exists — do not silently overwrite
