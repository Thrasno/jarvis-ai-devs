# Design: Hive MCP Tools for SDD Subagents

## Technical Approach

Add one shared SDD phase metadata source for required Hive memory tools (`mem_search`, `mem_get_observation`, `mem_save`, `mem_context`, `mem_session_summary`) and render platform-specific grants from it. Claude Code receives explicit `mcp__hive__...` entries in each SDD phase agent `tools:` frontmatter. OpenCode receives per-subagent `permission` grants for Hive MCP tools, then parser/verification/doctor checks prove generated artifacts are current. Hive/hybrid phase instructions fail clearly when those tools are unavailable instead of silently relying on inline context.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Shared Hive tool metadata | Define canonical Hive tool requirements in `jarvis-cli/internal/agent/sdd_phase_agents.go`. | Duplicate constants in templates/tests. | Keeps Claude/OpenCode generation and tests aligned from one source. |
| Claude grants | Append `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary` to every SDD phase agent allowlist. | Grant only writer phases. | Even read-only phases retrieve artifacts/context; Hive memory access must not alter filesystem edit permissions. |
| OpenCode grants | Use per-agent `permission` keys for exact `hive_mem_search`, `hive_mem_get_observation`, `hive_mem_save`, `hive_mem_context`, and `hive_mem_session_summary`; parser also accepts `hive_mem_*`/`hive_*` evidence. | Deprecated `agent.tools`; top-level-only MCP presence. | Current OpenCode docs say `tools` is deprecated and MCP tool names match permission wildcard patterns like `mymcp_*`; per-agent permission is explicit and testable. |
| Existing installs | Doctor reports owned drift and recommends `jarvis init`/supported reconfiguration; it does not rewrite config in doctor mode. | Auto-mutate during doctor. | Preserves user-owned config and matches current lifecycle read-only doctor behavior. |

## Data Flow

    SDDPhaseAgentDefinitions
      ├─ Claude renderer ──→ ~/.claude/agents/sdd-*.md tools: mcp__hive__...
      └─ OpenCode renderer ─→ opencode.json agent.sdd-*.permission hive_mem_*
                               │
                    runtime observers/parsers
                               │
                    sddruntime.Verify + lifecycle.Doctor
                               │
                    regeneration/degraded-mode guidance

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `jarvis-cli/internal/agent/sdd_phase_agents.go` | Modify | Add shared Hive MCP tool metadata and helpers for Claude/OpenCode names. |
| `jarvis-cli/internal/agent/opencode.go` | Modify | Inject per-SDD-subagent Hive MCP permission grants while preserving existing task/edit/bash permissions. |
| `jarvis-cli/embed/templates/claude-sdd-agent.md.tmpl` | Modify | Keep rendering explicit tool allowlists; update only if template data naming changes. |
| `jarvis-cli/embed/templates/opencode.json.tmpl` | Modify | Render generated agent permission JSON containing Hive grants. |
| `jarvis-cli/internal/agent/opencode_parse.go` | Modify | Parse per-SDD-subagent permission evidence for exact and wildcard Hive MCP grants. |
| `jarvis-cli/internal/sddruntime/observed_opencode.go` | Modify | Store parsed OpenCode grant evidence. |
| `jarvis-cli/internal/agent/runtime.go` | Modify | Observe Claude SDD agent tool frontmatter and OpenCode parsed grants. |
| `jarvis-cli/internal/sddruntime/verify.go` / `verify_opencode.go` | Modify | Add owned-drift checks for missing SDD Hive grants. |
| `jarvis-cli/internal/lifecycle/engine.go` | Modify | Map grant drift to read-only regeneration guidance. |
| `jarvis-cli/embed/skills/_shared/sdd-phase-common.md` / `persistence-contract.md` | Modify | Add fail-clear Hive/hybrid degraded behavior. |
| `docs/sdd-user-guide.md` | Modify | Document existing-install regeneration guidance. |
| `*_test.go` in affected packages | Modify | Add generation, parser, verifier, doctor, and prompt-asset regression tests. |

## Interfaces / Contracts

```go
type HiveMCPToolRequirement struct {
    LogicalName  string // mem_search
    ClaudeTool   string // mcp__hive__mem_search
    OpenCodeTool string // hive_mem_search
}
```

Verification contract: every generated SDD phase agent MUST expose all required Hive MCP tools; missing grants are owned generated-artifact drift. In Hive/hybrid mode, unavailable Hive MCP tools are a blocking degraded state with remediation text.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Claude rendered `tools:` include Hive tools without write-permission expansion. | Update `claude_sdd_agents_test.go`. |
| Unit | OpenCode generated subagents include per-agent Hive permissions and preserve user keys. | Update `opencode_test.go`. |
| Unit | Parser recognizes exact and wildcard Hive grants. | Update `runtime_opencode_parse_test.go`. |
| Integration | Verify/doctor report old generated artifacts as drift with `jarvis init` guidance. | Update `verify_opencode_checks_test.go` and lifecycle tests. |
| Contract | Prompt assets contain no silent Hive/hybrid fallback. | Update `catalog_contract_test.go`. |

## Migration / Rollout

No data migration required. Existing installations must rerun `jarvis init` or the supported reconfiguration flow to regenerate Claude agents and OpenCode config. Doctor remains read-only and only reports drift/remediation.

## Open Questions

None. OpenCode MCP naming risk is resolved to the documented `permission` wildcard pattern model, with regression tests guarding version drift.

## SDD Phase Result Envelope

- status: success
- executive_summary: Design content preserved and the required phase result envelope persisted to both artifact backends.
- artifacts:
  - Engram `sdd/hive-mcp-subagent-tools/design`
  - `openspec/changes/hive-mcp-subagent-tools/design.md`
- next_recommended: sdd-tasks
- risks: No unresolved CRITICAL risks.
- skill_resolution: paths-injected
