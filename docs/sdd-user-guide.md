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
| ------- | --------- | ----------------- |
| Init | Detect stack, testing commands, and workflow context. | Project SDD/testing capability context. |
| Explore | Clarify the problem and constraints. | Exploration notes. |
| Proposal | Decide intent, scope, and approach. | Proposal artifact. |
| Spec | Define observable behavior and scenarios. | Requirements and acceptance scenarios. |
| Design | Define technical approach and boundaries. | Design artifact. |
| Tasks | Slice implementation into reviewable units. | Task checklist and review workload forecast. |
| Apply | Implement assigned tasks. | Code/docs changes plus apply progress. |
| Verify | Independently prove the work matches spec/design/tasks. | Verification report with evidence. |
| Archive | Consolidate completed change knowledge. | Archived SDD record. |

The nine canonical lifecycle phases are `sdd-init`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, and `sdd-archive`; Jarvis installs them as core workflow capability. `sdd-onboard` remains automatic and non-interactive guidance, but is not a lifecycle phase or core requirement.

## Activation and preflight policy

SDD activation is deliberate. Mutating, planning, apply, verify, and archive SDD work should pass a session preflight before execution. The preflight gathers the choices that affect safe execution, such as artifact storage, execution mode, and delivery strategy. Read-only status checks may be allowed without the full preflight.

This policy exists to prevent accidental implementation, unclear artifact storage, and oversized review batches. It should be understood as a user-facing safety gate, not as an implementation detail of a prompt.

## Artifact storage boundary

Jarvis SDD artifacts belong to the Jarvis product workflow. Do not confuse them with assistant memory systems used by an external agent during development. In this repository, Hive is the default SDD artifact store unless a workflow explicitly chooses an OpenSpec/file-based mode.

## Store binding and diagnostics

Each `project/change` has at most one immutable persisted binding: `hive`, `openspec`, or `hybrid`. `none` is inline-only and never persists. A persisted binding takes precedence over `JARVIS_SDD_STORE_MODE`; the environment variable only selects an unbound change, including when a later value is invalid.

Hive owns bindings in SQLite. OpenSpec owns them in `openspec/changes/<change>/state.yaml`. Hybrid requires matching copies and equivalent protected progress; unavailable, malformed, unsupported, blank, noncanonical, or divergent state blocks rather than selecting one side. Planning artifacts never select authority, and legacy adoption is permitted only through protected canonical `apply-progress`.

Use `jarvis sdd status <change>` to show the binding and provenance in `jarvis sdd status`. Status may adopt an unbound change before it selects a backend, so it is not purely read-only recovery. For a problem report, provide the exact project and change, inspect both copies for hybrid, and distinguish absence from unavailable. `jarvis doctor` cannot determine an effective per-change binding because it does not receive those coordinates.

Progress and archive resolve/adopt the binding first. Hybrid recovery preserves its existing receipt, request ID, payload, identity, and authority. A partial recovery may replay or revalidate the exact request against both backends through their idempotent contracts before accepting acknowledgements. A complete receipt returns without replay. Never change the payload, identity, or authority, and never rewrite confirmed progress. Archive requires a complete executor-written `archive-report`; hybrid requires it to be byte-identical and non-blank on both sides, then revalidates it and protected progress under a local lock. This is not distributed atomicity: Hive writes after the final fetch are outside that lock. There is no typed closure API, closure state, or archive receipt. Archive requires completed (`done`) progress in Hive, OpenSpec, and hybrid; it never accepts a superseded predecessor, including on revalidation under the archive lock. Historical completed v2 changes remain eligible.

To replace a bound partial change, run `jarvis sdd supersede --change <predecessor> --successor <successor> --actor <actor> --reason <reason>` (optionally `--project <project>` and `--root <predecessor-change-directory>`). A new seal requires `--actor` and `--reason`. Strict predecessor and successor preflight runs before consent; signed credited tasks require typing the exact successor name, while zero credit skips the prompt only after preflight. Retry with the same successor and omit attribution flags or supply their exact signed values. The predecessor retains its original task manifest and immutable history; successor progress begins at generation 1/revision 1 without transferred credit. Hybrid publication is not distributed-atomic: retry the exact signed intent after interruption, never select one backend as winner. Never archive the superseded predecessor; only completed (`done`) changes can be archived. Live daemon-plus-OpenSpec end-to-end verification remains a separate ODD-724-06 check.

## Existing install regeneration

Existing installations created by older Jarvis versions must regenerate generated agent artifacts before Hive or hybrid SDD subagents can rely on Hive MCP tools. Run `jarvis init` or the supported reconfiguration flow for your installed provider.

Doctor is read-only: it reports outdated generated artifacts and does not silently mutate configuration. Regeneration preserves user-owned configuration through merge/no-clobber behavior.

Hive/hybrid SDD phases fail clearly instead of silently falling back to inline artifact context when required Hive MCP tools are unavailable.

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
