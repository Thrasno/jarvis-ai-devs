<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/_shared/persistence-contract.md -->
<!-- Upstream commit: 660917927b4821f5e540dc8fa501d6bee723222c; adapted for Jarvis/Hive runtime wording. -->
# Persistence Contract — Jarvis SDD Artifact Stores

Artifact store mode (`hive | openspec | hybrid | none`) controls where SDD phase artifacts and supported state/reference artifacts are read from and written to. Resolve it once before a phase starts, carry it through the whole phase, and do not re-resolve mid-skill.

This contract is about assistant/prompt SDD persistence only. Do not treat Jarvis product Hive, Hive API, or Hive ↔ Hive API synchronization as SDD artifact persistence.

## Mode Resolution

The orchestrator passes `artifact_store.mode` with one of: `hive | openspec | hybrid | none`.

The orchestrator asks the user which mode to use when a new SDD change starts or when an existing change is continued for the first time in a session. The choice is cached for the session.

Default behavior when the user does not specify a mode:

1. Use `hive` when Hive MCP tools are available.
2. Use `none` when Hive MCP tools are unavailable and no file-backed mode was explicitly requested.

Never force `openspec/` creation unless the orchestrator explicitly passed `openspec` or `hybrid`.

## Mode Roles

- **`hive`**: SDD working memory for recovery across sessions and compaction. It stores artifacts through Hive MCP tools and does not create project files.
- **`openspec`**: File-backed source of truth. It stores artifacts under `openspec/changes/{change-name}/` so teams can review, commit, and audit changes with git.
- **`hybrid`**: Both stores. It writes Hive artifacts for recovery and OpenSpec files for team review. It costs more tokens/tool calls and both writes must complete for the phase artifact to be complete.
- **`none`**: Ephemeral. It returns results inline only; artifacts are lost when the conversation context is lost.

Topic keys group related artifact saves; they are not artifact identity, recency, or version guarantees. If Hive search returns multiple artifacts for the same topic and no explicit recency/version metadata, treat the result as ambiguous. Ask the orchestrator/user or use a provided artifact reference before proceeding. This is recovery storage, not a substitute for git history or an OpenSpec archive trail.

### Mode Comparison

| Capability | `hive` | `openspec` | `hybrid` | `none` |
|------------|--------|------------|----------|--------|
| Cross-session recovery | Yes | Only if files are still present | Yes | No |
| Compaction survival | Yes | Only if files are still present | Yes | No |
| Team-shareable review artifact | No project file by default | Yes | Yes | No |
| Git-backed audit trail | No | Yes | Yes for files | No |
| Project files created | Never | Yes | Yes | Never |
| Token/tool cost | Medium | Low after file access | Highest | Lowest |

## Behavior Per Mode

| Mode | Read from | Write to | Project files |
|------|-----------|----------|---------------|
| `hive` | Hive MCP search, then full observation retrieval | Hive MCP save | Never |
| `openspec` | Filesystem paths from `openspec-convention.md` | Filesystem paths from `openspec-convention.md` | Yes |
| `hybrid` | Hive first, filesystem fallback | Hive and filesystem in the same phase | Yes |
| `none` | Orchestrator prompt context only | Nowhere | Never |

### Hive Mode

- Use `mcp__hive__mem_search` and then `mcp__hive__mem_get_observation` for artifact reads. Search results are previews, not source material.
- If the orchestrator passes a concrete Hive observation ID or artifact reference, retrieve that exact artifact instead of relying on search ordering.
- If search returns multiple candidate artifacts for the same topic and the tool/backend does not expose explicit recency or version metadata, stop and ask for the intended artifact reference.
- Use `mcp__hive__mem_save` for artifact writes.
- Include `project` in every Hive save call.
- Use topic keys in the format `{domain}/{identifier}` or `{domain}/{change}/{phase}`. SDD phase artifacts use `sdd/{change-name}/{artifact-type}`.
- SDD/config artifact saves must pass `capture_prompt: false` when supported because they are automated pipeline outputs, not human memories.
- Do not create `openspec/` directories or files.

### OpenSpec Mode

- Read and write only the paths defined in `openspec-convention.md`.
- Do not call Hive tools for artifact saves.
- Do not use OpenSpec paths as generated user-machine artifacts; they are project files.

### Hybrid Mode

- Persist every artifact to both Hive and OpenSpec in the same phase.
- Read priority: Hive first; fall back to filesystem if Hive returns no matching artifact.
- Write behavior: the phase artifact is complete only after the Hive save and filesystem write both succeed.
- State clearly in the return envelope if either store failed.

## Hive/Hybrid Degraded Mode

If Hive MCP tools cannot be exposed to SDD phase sub-agents, `hive` and `hybrid` modes are degraded and MUST fail clearly. Do not silently downgrade `hive` or `hybrid` to inline-only context.

Required Hive MCP tools: `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary`.

OpenCode permission entries use the corresponding `hive_mem_search`, `hive_mem_get_observation`, `hive_mem_save`, `hive_mem_context`, and `hive_mem_session_summary` tool names.

The blocked response MUST name the missing Hive MCP capability and recommend `jarvis init` or supported reconfiguration. Regeneration must preserve user-owned configuration through merge/no-clobber behavior.

This generated-artifact drift check is independent from the selected SDD artifact store mode; non-Hive modes do not require Hive persistence, but generated stale agents may still be reported as advisory drift when present.

### None Mode

- Call no persistence tools for SDD artifacts.
- Create or modify no project files.
- Return results inline and state that no artifacts were saved.
- Warn that phase recovery is not possible after context loss.

## State Persistence Across Phases

Phase agents persist their own phase artifact according to the resolved mode. The orchestrator may pass state or artifact references to phase agents, but this contract does not require per-transition DAG-state persistence unless runtime status explicitly implements it.

If a runtime/orchestrator state artifact is explicitly available, use this conventional state topic key: `sdd/{change-name}/state`. Phase agents must not assume this state artifact exists; they should rely on the structured status, `contextFiles`, `artifactPaths`, and provided artifact references passed by the orchestrator.

| Mode | Persist state | Recover state |
|------|---------------|---------------|
| `hive` | `mcp__hive__mem_save(topic_key: "sdd/{change-name}/state", project: "{project}", capture_prompt: false, ...)` | `mcp__hive__mem_search("sdd/{change-name}/state", project: "{project}")` → `mcp__hive__mem_get_observation(id)` |
| `openspec` | Write `openspec/changes/{change-name}/state.yaml` | Read `openspec/changes/{change-name}/state.yaml` |
| `hybrid` | Save to Hive and write `state.yaml` | Hive first, filesystem fallback |
| `none` | Not possible | Not possible |

Phase artifacts follow the same mode:

- Explore: `sdd/{change-name}/explore` or `openspec/changes/{change-name}/explore.md`
- Proposal: `sdd/{change-name}/proposal` or `openspec/changes/{change-name}/proposal.md`
- Spec: `sdd/{change-name}/spec` or `openspec/changes/{change-name}/specs/`
- Design: `sdd/{change-name}/design` or `openspec/changes/{change-name}/design.md`
- Tasks: `sdd/{change-name}/tasks` or `openspec/changes/{change-name}/tasks.md`
- Apply progress: `sdd/{change-name}/apply-progress` and task checkbox updates as required by mode
- Verify report: `sdd/{change-name}/verify-report` or `openspec/changes/{change-name}/verify-report.md`
- Archive report: `sdd/{change-name}/archive-report`; OpenSpec archive moves the full change folder under `openspec/changes/archive/`

Explore artifact uses `explore` for both the Hive topic key and the OpenSpec file path.

For any automated state or artifact save, set `capture_prompt: false` when the Hive tool schema supports it. If an older schema rejects or does not expose the field, omit it rather than failing.

## Common Rules

- `hive` means no project files for SDD artifacts.
- `openspec` means files only under OpenSpec paths.
- `hybrid` means both stores must be updated deliberately; do not assume product sync will do it.
- `none` means inline-only and no recovery guarantees.
- If the resolved mode is unsafe or unavailable, stop and report the blocker instead of silently changing modes.
- Generated user-machine artifacts such as installed agent configs, generated registries, and installed `.jarvis/skills/**` copies are outputs, never SDD artifact stores.

## Sub-Agent Context Rules

Sub-agents start with fresh context. The orchestrator must pass enough status, mode, artifact paths/topic keys, and allowed edit roots for the phase to work safely.

Who reads and writes:

- Non-SDD task: the orchestrator searches Hive if needed, passes a concise summary, and the sub-agent saves important discoveries through Hive when applicable.
- SDD phase with dependencies: the sub-agent reads required artifacts directly from the resolved store and saves its phase artifact.
- SDD phase without dependencies, such as exploration: the sub-agent may start without artifact reads, then saves its phase artifact according to mode.

Why this split:

- The orchestrator knows what non-SDD context is relevant.
- SDD artifacts can be large; phase sub-agents should retrieve exact artifacts rather than receiving lossy summaries.
- The phase sub-agent has the most complete detail for the artifact it produces.

## Orchestrator Prompt Instructions for Sub-Agents

For SDD phases with dependencies:

```markdown
Artifact store mode: {hive|openspec|hybrid|none}
Read these artifacts before starting:
- Hive: search `sdd/{change-name}/{artifact-type}`, then get the full observation by ID.
- OpenSpec: read the paths listed in `contextFiles` or `artifactPaths`.

PERSISTENCE (MANDATORY):
After completing your work, save `{artifact-type}` according to the resolved mode.
For Hive saves, use:
  mcp__hive__mem_save(
    title: "sdd/{change-name}/{artifact-type}",
    topic_key: "sdd/{change-name}/{artifact-type}",
    type: "architecture",
    project: "{project}",
    capture_prompt: false,
    content: "{full artifact markdown}"
  )
```

For SDD phases without dependencies:

```markdown
Artifact store mode: {hive|openspec|hybrid|none}

PERSISTENCE (MANDATORY):
After completing your work, save `{artifact-type}` according to the resolved mode. If mode is `none`, return inline and state that no artifact was saved.
```

## Sub-Agent Response Ordering

When a sub-agent persists artifacts through Hive or file writes, the persistence action MUST happen before the final text response. The sub-agent's final output MUST be text, never only a tool result. Put another way: the final output MUST be text, never only a tool result.

Why: parent agents consume the sub-agent's final text. If the last action is only a tool call, the parent may receive the tool result instead of the phase analysis and return envelope.

Sub-agents must not call session-summary persistence tools. Session summaries are reserved for the top-level agent managing the conversation.

## Skill Registry Handoff

The orchestrator pre-resolves skill paths from the skill registry and injects them as `## Skills to load before work` in the phase prompt.

Sub-agent skill loading:

1. If a `## Skills to load before work` block is present, read those exact `SKILL.md` files before task-specific work.
2. If no block is present, check for explicit `SKILL: Load` instructions.
3. If neither exists, proceed without extra skills and report `skill_resolution: none`.

To generate or refresh the registry, run the `skill-registry` skill or run `sdd-init` from the orchestrator context.

## Detail Level

The orchestrator may pass `detail_level`: `concise | standard | deep`.

Detail level controls response verbosity only. It does not change artifact completeness: every persisted SDD artifact must include the full required content for downstream phases.
