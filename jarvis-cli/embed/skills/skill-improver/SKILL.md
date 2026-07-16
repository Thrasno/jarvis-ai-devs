---
name: skill-improver
display_name: "Skill Improver"
description: "Trigger: improve skills, audit skills, refactor skills, skill quality. Audit and upgrade existing LLM-first skills safely."
license: Apache-2.0
scope: optional
metadata:
  author: gentleman-programming
  version: "1.0"
---

Packaged for Jarvis skill registry and path-injected loading.
<!-- gentle-ai v2.1.5 selective sync: minimal .jarvis canonical / .atl read-fallback adaptation. -->

## Activation Contract

Use this skill when the user asks to improve skills, audit skills, refactor skills, or evaluate skill quality. Prefer project-discovered skill paths from `.jarvis/skill-registry.md`, especially `.jarvis/skills/<skill>/SKILL.md` entries, with `.atl/skill-registry.md` as legacy read fallback.

## Hard Safety Rules

- Default to audit-only mode: inspect, explain findings, and recommend changes first.
- Require explicit user approval before modifying any skill file.
- Never change user-authored or project-local skills as an implied side effect of analysis.
- Do not add commands, background jobs, or rewrite engines for skill mutation.
- Keep generated skill content in English unless the target skill or user request requires another language.

## Style Guide Checks

Audit existing skills against the style guide before proposing edits:

| Check | Standard |
|---|---|
| Trigger fit | Frontmatter description starts with concrete trigger phrases. |
| LLM-first shape | The skill gives operational instructions, not long tutorial prose. |
| Safety boundaries | The skill states when to stop, ask, or avoid mutation. |
| Local references | References point to local bundled files or registry-discovered paths. |
| Output contract | The skill says what result to return to the user or orchestrator. |

## Execution Steps

1. Read the target skill and any referenced local files.
2. Identify activation, safety, structure, reference, and output-contract gaps.
3. Return a concise audit with severity, evidence, and proposed edits.
4. Ask for explicit approval before applying edits.
5. If approved, make the smallest safe change and report files changed plus checks performed.

## Output Contract

Return audit findings, style-guide checks performed, whether approval was requested or granted, files changed if any, and remaining risks.
