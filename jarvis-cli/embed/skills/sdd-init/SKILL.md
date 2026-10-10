---
name: sdd-init
display_name: "SDD Init"
description: "Detect project stack, testing capabilities, and initialize SDD context. Trigger: When initializing SDD, iniciar sdd, openspec init, /sdd-init"
disable-model-invocation: true
user-invocable: false
license: MIT
scope: core
metadata:
  author: gentleman-programming
  version: "3.0"
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/sdd-init/SKILL.md (tag v1.26.5, commit 5f73974b39ae2b9b525ef465b3642030c5f2ce6c); adapted for Jarvis/Hive runtime semantics. -->

## Activation Contract

Run this phase when the orchestrator/user asks to initialize SDD in a project. You are the phase executor: do the work yourself, do not delegate, and do not behave like the orchestrator.

## Hard Rules

- Detect the real stack, conventions, architecture, testing tools, and persistence mode; never guess.
- In `hive` mode, do **not** create `openspec/`.
- In `openspec` mode, follow `../_shared/openspec-convention.md` and write file artifacts.
- In `hybrid` mode, write both openspec files and Hive observations.
- Always persist testing capabilities separately as `sdd/{project}/testing-capabilities` or `openspec/config.yaml` `testing:`.
- A test runner counts as detected only when a real test command exists; a config or manifest file alone is not enough. Use the Testing Capability Checklist in `references/init-details.md`.
- Detection produces a suggestion, not a decision: cache `strict_tdd_suggestion: strict|standard` plus a one-line `detection_reason`; the preflight `TDD mode` decides per feature.
- Keep caching `strict_tdd` for backward compatibility; it mirrors the suggestion only (`true` when the suggestion is `strict`) and never activates Strict TDD by itself.
- Always build `.jarvis/skill-registry.md`; also save `skill-registry` to Hive when available.
- Use `capture_prompt: false` for automated SDD/config saves when supported; omit it if the tool schema lacks it.
- If `openspec/` already exists, report what exists and ask before updating it.
- Artifact store modes supported by Jarvis skills: `hive | openspec | hybrid | none`.

## Decision Gates

| Input | Action |
|---|---|
| `mode=hive` | Save context and capabilities to Hive only. |
| `mode=openspec` | Create/update openspec bootstrap files only. |
| `mode=hybrid` | Do both Hive and openspec persistence. |
| `mode=none` | Return detected context only; write no SDD artifacts except registry if required. |
| strict TDD marker/config found | Use that value as `strict_tdd_suggestion`. |
| no marker/config and project code is mainly Deluge | Suggest `standard`: Deluge code cannot run under a local test runner. |
| no marker/config and a real test command exists | Suggest `strict`. |
| no real test command | Suggest `standard` and record why in `detection_reason`. |

## Execution Steps

1. Inspect project files (`package.json`, `go.mod`, `pyproject.toml`, CI, lint/test config) and summarize stack/conventions.
2. Detect real test commands, test layers, coverage, linter, type checker, and formatter.
3. Resolve the Strict TDD suggestion in gate order: agent marker or `openspec/config.yaml` `strict_tdd:`, then Deluge, then real test command, then no test command. Record the one-line `detection_reason`.
4. Initialize persistence for the resolved mode.
5. Build `.jarvis/skill-registry.md` using the skill-registry scan rules.
6. Persist testing capabilities and project context.
7. Return the structured initialization envelope.

## Output Contract

Return `status`, `executive_summary`, `artifacts`, `next_recommended`, and `risks`. Include project, stack, persistence mode, Strict TDD suggestion with its detection reason, testing capability table, saved observation IDs/paths, registry path, and next `/sdd-explore` or `/sdd-new` step.

## References

- [references/init-details.md](references/init-details.md) — detection checklist, Hive payloads, config skeleton, and output templates.
- `../_shared/hive-convention.md` — Hive artifact naming.
- `../_shared/openspec-convention.md` — openspec layout and rules.
