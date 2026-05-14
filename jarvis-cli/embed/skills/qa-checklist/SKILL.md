---
name: qa-checklist
description: "Trigger: batería de pruebas, checklist de pruebas, qué pruebas debería hacer, QA checklist, test checklist. Plan on-demand QA."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill only when the user explicitly asks for a QA checklist, test checklist, batería de pruebas, checklist de pruebas, or what tests should be performed. This is optional planning support and does not replace `sdd-verify`.

## Hard Rules

- Do not introduce a new SDD phase or approval gate.
- Do not claim tests were run unless this session actually ran them.
- If tests were not run, state `not executed` and `not verification evidence`.
- Separate manual checks from automated test recommendations.
- Ask or state assumptions when acceptance criteria, platform, or risk context is missing.

## Decision Gates

| Context | Response |
|---|---|
| Feature behavior is clear | Produce concrete manual and automated checks |
| Acceptance criteria are missing | List assumptions and ask one clarifying question if needed |
| Automation is not applicable | Say why and focus on manual QA |
| SDD change needs evidence | Point to `sdd-verify` for verification execution |

## Execution Steps

1. Identify the feature, user paths, data states, and risk areas.
2. Map each acceptance criterion to at least one manual check.
3. Recommend automated tests only where they add useful regression coverage.
4. Call out edge cases, failure modes, and environment assumptions.
5. Mark any unrun checks as `not executed`.

## Output Contract

Return these sections:

1. **Manual QA checklist** — user-visible checks with expected results.
2. **Automated test recommendations** — unit, integration, contract, or E2E suggestions when applicable.
3. **Risks and edge cases** — likely regressions, data boundaries, and platform concerns.
4. **Assumptions and questions** — missing context and any single blocking question.
5. **Execution note** — what was run, or `not executed — not verification evidence`; defer evidence to `sdd-verify`.
