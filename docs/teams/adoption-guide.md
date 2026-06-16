# Team Adoption Guide

Adopt Jarvis Dev as a workflow layer, not just a CLI install. A successful rollout gives developers local-first memory, predictable agent configuration, clear diagnostics, and an agreed path for shared team knowledge.

## Quick path

1. Pick a pilot team and one active repository.
2. Install the same Jarvis release channel for all pilot users.
3. Run `jarvis` on each machine to generate managed configuration.
4. Confirm `jarvis verify --provider all` passes.
5. Use local Hive memory first.
6. Enable Hive API sync only after the team agrees sharing boundaries.
7. Review outcomes and expand gradually.

## Adoption stages

| Stage | Goal | Exit signal |
|-------|------|-------------|
| Pilot | Validate setup and local memory on a real repo. | Developers can run Jarvis and recover context locally. |
| Team memory | Configure Hive API for shared memory when needed. | Shared decisions appear for team members without blocking local work. |
| SDD workflow | Use SDD for substantial changes. | Specs, tasks, implementation, and verification are traceable. |
| Governance | Add diagnostics, recovery, and dashboard operations. | Team can diagnose drift and explain data boundaries. |

## Team agreements

- Which repositories are in scope.
- Which memory categories are safe to share.
- Who operates Hive API and dashboard access.
- Which release channel is used: production or beta.
- How oversized changes are split for review.
- How Todoist-backed backlog workflow is used when configured.

## Checklist

- [ ] Everyone understands generated files are not manually patched.
- [ ] Secrets stay outside the repository.
- [ ] Local memory is useful before shared sync is required.
- [ ] Hive API is treated as shared team infrastructure.
- [ ] SDD is used for high-risk or cross-component changes.

## Next step

Use [`evaluator-guide.md`](evaluator-guide.md) to run a structured evaluation before wider rollout.
