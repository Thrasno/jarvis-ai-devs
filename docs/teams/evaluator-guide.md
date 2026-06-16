# Evaluator Guide

Evaluate Jarvis Dev by measuring whether it improves setup consistency, context recovery, review quality, and team trust without weakening data boundaries.

## Quick path

1. Choose one real repository and a small team.
2. Install Jarvis from the intended release channel.
3. Run setup, verification, and a local Hive memory workflow.
4. Run one substantial change through SDD when appropriate.
5. Decide whether to enable shared Hive API sync.
6. Record findings and adoption risks.

## Evaluation checklist

| Area | Question | Evidence |
|------|----------|----------|
| Setup | Can a new user install and run `jarvis` without manual file patching? | Installer output, `jarvis verify`. |
| Local memory | Can users recover project context locally? | Hive TUI/timeline behavior. |
| Sync boundary | Do users understand install is separate from Hive API sync? | Team feedback and config review. |
| SDD | Are substantial changes better scoped and verified? | SDD artifacts and verification report. |
| Security | Are secrets and generated files handled correctly? | Config review and absence of committed secrets. |
| Operations | Can drift be diagnosed safely? | `jarvis doctor --provider all` output. |

## Suggested pilot tasks

- Install Jarvis and run reconfiguration once.
- Browse local memory with `jarvis hive`.
- Browse a project timeline with `jarvis timeline --project <project>`.
- Run `jarvis verify --provider all` and `jarvis doctor --provider all`.
- Use SDD for one non-trivial documentation, CLI, or workflow change.

## Decision criteria

Proceed when the pilot team can explain:

- where local data lives,
- when shared sync is enabled,
- how generated agent files are regenerated,
- how to diagnose setup drift,
- and when SDD should be used.

## Next step

For non-technical stakeholder material, use [`../public/jarvis-evaluator-brief.md`](../public/jarvis-evaluator-brief.md).
