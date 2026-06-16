# SDD User Guide

Spec-Driven Development (SDD) in Jarvis is a guided workflow for moving from intent to verified implementation without relying on chat history as the source of truth.

## Quick path

1. Start with project calibration when needed.
2. Explore the change before committing to implementation.
3. Create proposal, spec, and design artifacts.
4. Break the work into reviewable tasks.
5. Apply tasks with verification evidence.
6. Archive completed changes into the durable project knowledge base.

## User-level flow

| Phase | Purpose | Expected output |
|-------|---------|-----------------|
| Init | Detect stack, testing commands, and workflow context. | Project SDD/testing capability context. |
| Explore | Clarify the problem and constraints. | Exploration notes. |
| Proposal | Decide intent, scope, and approach. | Proposal artifact. |
| Spec | Define observable behavior and scenarios. | Requirements and acceptance scenarios. |
| Design | Define technical approach and boundaries. | Design artifact. |
| Tasks | Slice implementation into reviewable units. | Task checklist and review workload forecast. |
| Apply | Implement assigned tasks. | Code/docs changes plus apply progress. |
| Verify | Independently prove the work matches spec/design/tasks. | Verification report with evidence. |
| Archive | Consolidate completed change knowledge. | Archived SDD record. |

## Activation and preflight policy

SDD activation is deliberate. Mutating, planning, apply, verify, and archive SDD work should pass a session preflight before execution. The preflight gathers the choices that affect safe execution, such as artifact storage, execution mode, and delivery strategy. Read-only status checks may be allowed without the full preflight.

This policy exists to prevent accidental implementation, unclear artifact storage, and oversized review batches. It should be understood as a user-facing safety gate, not as an implementation detail of a prompt.

## Artifact storage boundary

Jarvis SDD artifacts belong to the Jarvis product workflow. Do not confuse them with assistant memory systems used by an external agent during development. In this repository, Hive is the default SDD artifact store unless a workflow explicitly chooses an OpenSpec/file-based mode.

## Review workload rule

When a change is likely to exceed roughly 400 changed lines or touches multiple critical areas, split it into reviewable work units or record an explicit size exception. The goal is to keep human review effective, not to maximize one-shot output.

## Checklist

- [ ] The current phase has the required previous artifacts.
- [ ] Artifact storage choice is explicit.
- [ ] Tasks are small enough for review or have an accepted exception.
- [ ] Verification evidence is captured before closure.
- [ ] Completed knowledge is archived for future sessions.

## Next step

Read existing SDD reference material in [`sdd-workflows/`](sdd-workflows/) and the architecture summary in [`reference/architecture.md`](reference/architecture.md).
