### PROJECT CONTEXT (run at session start)

Detect the active project name at the START of EVERY session:
1. Run: `git remote get-url origin` → extract repo name (last path segment, strip `.git`)
2. Fallback: basename of the current working directory
3. Fallback: "default"

Use the resolved project name as the `project` field in ALL Hive memory saves.
NEVER save a memory without a `project` field.

---

## Hive Protocol Source Boundary

The canonical Hive protocol source is `protocol.hive` at `jarvis-cli/embed/hive-protocol.md`.
Layer1 MUST NOT duplicate the Hive protocol body. Generated runtimes receive the protocol through the dedicated protocol injection source, so update `embed/hive-protocol.md` when the protocol itself changes.

## Contextual Skill Loading Self-Check

Before every response, check whether the request matches an installed skill.
If a matching skill exists, load that skill before task-specific work, code changes, generated artifacts, or user-facing guidance.
Multiple skills can apply simultaneously; load all relevant skills before proceeding.

## Persona Scope and Artifact Language

Persona voice applies only to direct user replies.
Persona voice MUST NOT alter code, identifiers, comments, UI copy, documentation, configuration, prompts, SDD artifacts, or other generated technical artifacts.
Generated technical artifacts default to English unless the user explicitly requests another artifact language or an existing project convention requires otherwise.
Preserve product names exactly, including Hive, jarvis CLI, `.jarvis/skill-registry.md`, and `.jarvis/skills/<skill>/SKILL.md`.

### Hive-specific features

- The `project` field is MANDATORY in ALL Hive memory saves — NEVER save without it.
- Use Hive MCP sync tools when a workflow explicitly requires cloud synchronization.
- Team memory: memories with `project` scope are shared across all team members via hive-api.

### SDD Runtime Contract Notes

Model assignments and runtime ownership invariants are contract-owned and validated by `internal/sddruntime`.
Do not duplicate phase→model tables in this file; defer to the canonical runtime contract used by setup/runtime verification.

Sub-agent launch pattern: delegate reads of 4+ files, multi-file writes, and test runs to sub-agents. The orchestrator resolves skills from the registry and passes exact `SKILL.md` paths under:

```markdown
## Skills to load before work
- /absolute/or/repo-resolved/path/to/skill/SKILL.md
```

Sub-agents read those exact files before task-specific work and report `skill_resolution: paths-injected` when loading succeeds. Compact summaries may exist only as transitional metadata; they are not the primary skill contract.

SDD DAG: `proposal → specs → tasks → apply → verify → archive`
Apply-progress continuity is mandatory across chained apply batches: read existing `sdd/{change-name}/apply-progress`, merge it, and persist the combined artifact.

### SDD Activation Policy (Runtime Entry)

CANONICAL SOURCE: `jarvis-cli/embed/orchestrator/sdd-orchestrator.md` section "Runtime Activation Policy (Explicit Override First)"

This file DEFERS to the orchestrator for:
- Normalization pipeline specification (deterministic steps, accent map, punctuation scope)
- Override vocabulary (bilingual phrases, exact strings)
- Decision order contract (explicit first, heuristics second)
- Reconfirmation detection rules (same normalization as explicit detection)

Layer1 summary (for quick reference only — orchestrator is authoritative):
- `complexity_check` is recommendation-only guidance, never an enforcement switch.
- Explicit user command takes precedence over heuristic recommendations.
- Decision order is mandatory and deterministic: explicit override parsing first, complexity heuristics second.
- Warning-only pushback is allowed for risk/overhead framing, but direct user command execution is never blocked.
- Normalization rules and vocabulary must match canonical source exactly (see orchestrator section for deterministic normalization steps).

## Layer Boundary Rule (MVP)

Layer1 is behavior/instruction policy only.
Do NOT place persona, tone, teaching style, or biography content in this file.
That belongs to Layer2 exclusively.
