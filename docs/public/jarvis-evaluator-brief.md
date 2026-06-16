# Jarvis Dev Evaluator Brief

Use this brief to evaluate whether Jarvis Dev is a good fit for a development team before wider adoption.

## Evaluation goal

Prove that Jarvis improves AI-assisted development in four areas: setup consistency, context recovery, review discipline, and trust in data boundaries.

## Suggested pilot

| Step | What to do | Evidence to collect |
|------|------------|---------------------|
| Install | Install Jarvis for a small pilot team. | Time to first successful setup. |
| Configure | Run the setup wizard and diagnostics. | Successful verification output. |
| Use local memory | Capture and recover real project context. | Examples of reused decisions. |
| Try SDD | Run one substantial change through SDD, a guided planning and verification workflow. | Artifacts, task list, verification evidence. |
| Decide sync | Evaluate Hive API sync, the optional sharing flow for approved team memory, when appropriate. | Security review and team consent. |

## Success criteria

- Developers can start without hand-editing generated configuration, the tool settings Jarvis creates from approved templates.
- The team can explain what stays local and what is shared.
- Important decisions are easier to recover.
- Larger changes are easier to scope and review.
- Operators can diagnose configuration drift, when local setup no longer matches the expected baseline.

## Risks to watch

- Teams enabling shared memory without governance.
- Users patching generated configuration directly.
- Treating installation as if it automatically synchronizes memory.
- Promising optional dashboard features before they are enabled in the deployment.

## Next step

If the pilot succeeds, move to the staged rollout in [`jarvis-adoption-guide.md`](jarvis-adoption-guide.md).
