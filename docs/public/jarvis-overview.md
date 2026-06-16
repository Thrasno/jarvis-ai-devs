# Jarvis Dev Overview

Jarvis Dev helps development teams use AI coding tools with more consistency, memory, and control. It combines a setup CLI, local-first project memory, optional shared team memory, and a structured workflow for larger changes.

## The answer

Jarvis is not another chatbot. It is an operating layer around AI-assisted development: it helps teams configure tools consistently, preserve project knowledge, and reduce repeated decisions across sessions and machines.

## What it provides

| Capability | Business value |
|------------|----------------|
| Guided setup | Developers start from a consistent baseline. |
| Local memory | Project context survives between AI sessions. |
| Shared memory when enabled | Team decisions can be reused across machines. |
| Structured SDD workflow | SDD is a guided way to plan, implement, and verify larger changes with traceability. |
| Diagnostics and recovery | Teams can detect and repair configuration drift, when local setup no longer matches the expected baseline. |
| Optional dashboard | Operators can inspect shared memory data when deployed. |

## How teams use it

1. Install Jarvis for a pilot group.
2. Run the setup wizard on developer machines.
3. Use local memory to recover decisions and context.
4. Add shared Hive API sync, the optional flow that shares approved memory through the team service, when the team is ready.
5. Use SDD for higher-risk or multi-step changes.

## Trust model

Jarvis keeps local work local by default. Shared memory requires configuration. Generated tool configuration, the tool settings Jarvis creates from approved templates, is reproducible through Jarvis flows, so teams do not depend on manual patching of developer machines.

## Next step

Use [`jarvis-adoption-guide.md`](jarvis-adoption-guide.md) for rollout planning or [`jarvis-evaluator-brief.md`](jarvis-evaluator-brief.md) for a pilot evaluation.
