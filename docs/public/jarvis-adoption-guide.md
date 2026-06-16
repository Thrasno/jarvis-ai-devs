# Jarvis Dev Adoption Guide

Adopt Jarvis Dev in stages: start with consistent setup and local memory, then add shared team memory and structured change workflows when the pilot team is ready.

## Recommended rollout

| Stage | What happens | Success signal |
|-------|--------------|----------------|
| 1. Pilot | A small team installs Jarvis and runs setup. | Setup is repeatable and support questions are low. |
| 2. Local memory | Developers use local project memory during real work. | Context is easier to recover between sessions. |
| 3. Shared memory | Hive API sync, the optional sharing flow for approved team memory, is enabled for approved projects. | Team decisions are reusable across machines. |
| 4. Structured delivery | SDD, a guided planning and verification workflow, is used for substantial changes. | Work is easier to review and verify. |
| 5. Operations | Diagnostics and dashboard are used when enabled. | Configuration drift, when local setup no longer matches the expected baseline, is easier to explain. |

## Adoption checklist

- [ ] Choose one pilot team and one repository.
- [ ] Decide whether the pilot uses production or beta release channel.
- [ ] Define what project knowledge may be shared.
- [ ] Assign an operator for Hive API if shared sync is enabled.
- [ ] Review security and privacy expectations before expanding.

## What to communicate to teams

- Jarvis does not replace engineers; it gives AI-assisted work better structure.
- Local memory can work without shared sync.
- Shared memory is optional and should be governed by the team.
- Generated configuration, the tool settings Jarvis creates from approved templates, should be regenerated through Jarvis, not manually patched.

## Next step

Use [`jarvis-security-and-privacy.md`](jarvis-security-and-privacy.md) with stakeholders before enabling shared memory.
