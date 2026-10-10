<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/_shared/sdd-phase-common.md (tag v1.26.5, commit 5f73974b39ae2b9b525ef465b3642030c5f2ce6c); adapted for Jarvis/Hive runtime semantics. -->

# SDD Phase — Common Protocol

Boilerplate identical across all SDD phase skills. Sub-agents MUST load this alongside their phase-specific SKILL.md.

Executor boundary: every SDD phase agent is an EXECUTOR, not an orchestrator. Do the phase work yourself. Do NOT launch sub-agents, do NOT call `delegate`/`task`, and do NOT bounce work back unless the phase skill explicitly says to stop and report a blocker.

## A. Skill Loading

1. Check if the orchestrator injected a `## Skills to load before work` block in your launch prompt. If yes, read those exact `SKILL.md` files before task-specific work.
2. If no injected skills block was provided, check for `SKILL: Load` instructions. If present, load those exact skill files.
3. If neither was provided, search for the skill registry as a fallback:
   a. `mcp__hive__mem_search(query: "skill-registry", project: "{project}")` — if found, `mcp__hive__mem_get_observation(id)` for full content
   b. Fallback: read `.jarvis/skill-registry.md` from the project root if it exists; `.atl/skill-registry.md` is a legacy read fallback only
   c. From the registry's skill index, match triggers to your current task and read the exact listed `SKILL.md` paths.
4. If no registry exists, proceed with your phase skill only.

NOTE: the preferred path is (1) — exact skill paths selected by the orchestrator. Paths (2) and (3) are fallbacks for backwards compatibility. Searching the registry is SKILL LOADING, not delegation. If `## Skills to load before work` is present, IGNORE redundant `SKILL: Load` instructions.

## B. Artifact Retrieval (Hive Mode)

When the launch prompt provides artifact observation IDs or OpenSpec paths, read those directly: one `mcp__hive__mem_get_observation(id)` per ID or one file read per path. Skip `mcp__hive__mem_search` for them; search only for artifacts the prompt did not provide.

**CRITICAL**: `mcp__hive__mem_search` returns 300-char PREVIEWS, not full content. You MUST call `mcp__hive__mem_get_observation(id)` for EVERY artifact. **Skipping this produces wrong output.**

Search results are previews, not source material. If Hive search returns multiple candidate artifacts for the same topic and no explicit artifact reference is available, treat the result as ambiguous. Ask the orchestrator/user for the intended observation ID or use a provided artifact reference before proceeding.

**Run all searches in parallel** — do NOT search sequentially.

```
mcp__hive__mem_search(query: "sdd/{change-name}/{artifact-type}", project: "{project}") → save ID
```

Then **run all retrievals in parallel**:

```
mcp__hive__mem_get_observation(id: {saved_id}) → full content (REQUIRED)
```

Do NOT use search previews as source material.

## C. Artifact Persistence

Every phase that produces an artifact MUST persist it. Skipping this BREAKS the pipeline — downstream phases will not find your output.

Supported artifact store modes for Jarvis skill/runtime instructions: `hive | openspec | hybrid | none`.

### Bound Store Resolution

A persisted `hive`, `openspec`, or `hybrid` binding is immutable authority for its `project/change`; `none` is never persisted. Persisted binding wins even when `JARVIS_SDD_STORE_MODE` changes or is invalid. Resolve or adopt the binding before selecting a backend for status, continue, progress (`advance`, `checkpoint`, `upgrade-continuation`), or archive; planning artifacts cannot select authority.

Hive binding lives in SQLite, OpenSpec binding lives in `openspec/changes/{change-name}/state.yaml`, and hybrid requires matching copies. Outage or protocol failure is not absence. Blank, malformed, unsupported, noncanonical, blocked, or divergent binding/protected progress fails closed; hybrid never chooses a copy arbitrarily. Legacy adoption is protected by canonical `apply-progress` only.

### Hive mode

```
mcp__hive__mem_save(
  title: "sdd/{change-name}/{artifact-type}",
  topic_key: "sdd/{change-name}/{artifact-type}",
  type: "architecture",
  project: "{project}",
  capture_prompt: false,
  content: "{your full artifact markdown}"
)
```

`topic_key` groups related artifact saves; saving again creates a distinct observation. Do not assume search ordering identifies the current artifact; use an explicit artifact reference when one is provided, and treat multiple candidates as ambiguous when no explicit reference is available.
`capture_prompt: false` is mandatory for SDD artifacts because they are automated pipeline outputs, not human/proactive memory saves. Set it when the Hive tool schema supports it; if an older schema rejects or does not expose the field, omit it rather than failing.

### OpenSpec mode

File was already written during the phase's main step. No additional action needed.

### Hybrid mode

Do BOTH: write the file to the filesystem AND call `mcp__hive__mem_save` as above.

### None mode

Return result inline only. Do not write any files or call `mcp__hive__mem_save`.

## D. Return Envelope

Every phase MUST return a structured envelope to the orchestrator:

- `status`: `success`, `partial`, or `blocked`
- `executive_summary`: 1-3 sentence summary of what was done
- `detailed_report`: (optional) full phase output, or omit if already inline
- `artifacts`: list of artifact keys/paths written; in `hive` or `hybrid` mode include each saved observation ID so the orchestrator can forward it
- `next_recommended`: the next SDD phase to run, or `none`
- `risks`: risks discovered, or `None`
- `skill_resolution`: how skills were loaded — `paths-injected` (received exact `SKILL.md` paths from orchestrator), `fallback-registry` (self-loaded paths from registry), `fallback-path` (loaded via `SKILL: Load` path), or `none` (no skills loaded)

The literal contract line is: `status`: `success`, `partial`, or `blocked`.

Example:

```markdown
**Status**: success
**Summary**: Proposal created for `{change-name}`. Defined scope, approach, and rollback plan.
**Artifacts**: Hive `sdd/{change-name}/proposal` | `openspec/changes/{change-name}/proposal.md`
**Next**: sdd-spec or sdd-design
**Risks**: None
**Skill Resolution**: paths-injected — 3 skills (react-19, typescript, tailwind-4)
(other values: `fallback-registry`, `fallback-path`, or `none — no registry found`)
```

## E. Review Workload Guard

SDD must protect reviewer cognitive load, not only generate tasks.

- The size policy comes from the SDD Session Preflight and is recorded in the proposal's `## SDD Decisions` block: a review budget of N changed lines (`additions + deletions`; 400 is the recommended option) with `delivery_strategy: auto-chain`, or `size:exception` with `delivery_strategy: exception-ok`. There is no hidden default budget.
- The orchestrator MUST pass `delivery_strategy`, `review_budget_lines`, and `chain_strategy` to `sdd-tasks` and `sdd-apply`. No phase asks the user for a chain strategy, size exception, or budget again.
- `size:exception` is sticky for the feature: no line forecast, no size question, and no further size checks for that change.
- Under a budget, `sdd-tasks` MUST forecast whether the planned work may exceed the forwarded N.
- The forecast MUST include exact plain-text guard lines, one chosen value per line: `Decision needed before apply: <Yes or No>`, `Chained PRs recommended: <Yes or No>`, `Chain strategy: <value>`, and `Budget risk: <Low, Medium, or High>` (omitted under `size:exception`).
- If the forecast exceeds N, `sdd-tasks` MUST split the work into deliverable work units for the forwarded chain strategy.
- When decisions were not forwarded, `sdd-tasks` writes `Decision needed before apply: Yes` and `Chain strategy: pending` so apply stays blocked until the preflight runs.
- `sdd-apply` MUST NOT start oversized work unless the recorded decisions resolve to chained/stacked PR slices or `size:exception`.
- Each chained PR slice must have a clear start, clear finish, autonomous scope, verification, and reasonable rollback.
- In a Feature Branch Chain, PR #1 targets the feature/tracker branch and later child PRs target the immediate previous PR branch; if GitHub shows previous slices in a child diff, retarget/rebase until the diff is clean.

This guard exists to reduce reviewer burnout and keep implementation delivery safe. Do not treat it as optional process noise.

## F. Hive/Hybrid Degraded Mode

If artifact store mode is `hive` or `hybrid` and required Hive MCP tools are unavailable to the phase sub-agent, STOP and return `blocked`. Do not silently fall back to inline artifact context as the happy path.

Required Hive MCP tools: `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary`.

OpenCode permission entries use the corresponding `hive_mem_search`, `hive_mem_get_observation`, `hive_mem_save`, `hive_mem_context`, and `hive_mem_session_summary` tool names.

The blocked response MUST name the missing Hive MCP capability and include remediation guidance. Remediation: run `jarvis init` or the supported reconfiguration flow to regenerate agent artifacts without clobbering user-owned configuration.

## G. Native Status Gate

Mutating phases (`sdd-apply`, `sdd-verify`, `sdd-archive`) apply this gate to the `jarvis.sdd-status` JSON before any edit, command, or report write. `<phase>` is the executor's own phase name.

1. `schema` is exactly `jarvis.sdd-status`.
2. `dependencies["<phase>"]` is exactly `ready` (the phase skill names any narrower exception).
3. `actionContext.mode` is exactly `workspace-edit`; `workspace-planning` is read-only planning context.
4. `actionContext.allowedEditRoots` is non-empty.

- Write only inside `actionContext.allowedEditRoots`; if a needed path is outside every root, STOP and report the unsafe path.
- Review `blockedReasons` first; a reason that blocks this phase is a gate failure.
- Fail closed: blocked, missing, unavailable, or invalid status is a gate failure. STOP and return `blocked` with the failing check and the phase-specific `blockedReasons`.
- Manual recovery may inspect artifacts but cannot invent workspace-edit authority; maintainer approval cannot substitute for it.
- Generated artifacts are outputs, never sources of truth: never edit installed agent configuration, generated registries, or runtime copies to pass a phase; change the source assets/templates instead.

The orchestrator runs `jarvis sdd status <change> --json` once per phase transition and forwards that JSON verbatim. Executors run it themselves only when no status was forwarded.
