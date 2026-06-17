---
name: skill-creator
description: "Trigger: new skills, agent instructions, documenting AI usage patterns. Create LLM-first skills with valid frontmatter."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Create or update a skill when a reusable AI behavior, workflow, convention, or decision tree needs runtime guidance. Do not create a skill for one-off instructions or human-only documentation.

## Hard Rules

- A skill is an LLM instruction contract, not a tutorial.
- Use valid YAML frontmatter with `name`, one-line quoted `description`, `license`, `metadata.author`, and `metadata.version`.
- Put trigger words first in `description`; preserve the exact phrases users or agents will say.
- Do not add a `Keywords` section.
- Keep `SKILL.md` concise; move long examples, evals, schemas, and templates into local `references/` or `assets/`.
- References must point to local files.

## Decision Gates

| Need | Action |
|---|---|
| Reusable behavior or workflow | Create/update `SKILL.md` |
| Templates, schemas, fixtures | Put under `assets/` |
| Detailed examples or quality loops | Put under `references/` |
| Project skill discovery | Register in the project skill registry |

## Execution Steps

1. Check whether the skill already exists and whether the pattern is reusable.
2. Draft the trigger phrase before writing the body.
3. Write compact sections: Activation Contract, Hard Rules, Decision Gates, Execution Steps, Output Contract, References.
4. Add focused trigger/output checks; use [references/quality-loop.md](references/quality-loop.md) when deeper iteration is needed.
5. Register or update discovery metadata when the project has a skill catalog.

## Output Contract

Return files created or modified, style source used, registry changes, supporting `assets/` or `references/`, and trigger/output checks performed.

## References

- [references/quality-loop.md](references/quality-loop.md) — optional trigger and output quality loop.
