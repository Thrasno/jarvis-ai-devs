## Exploration: hive-mcp-subagent-tools

### Current State
Claude SDD phase agents are rendered from `SDDPhaseAgentDefinitions()` through `claude-sdd-agent.md.tmpl`. The template emits an explicit `tools:` frontmatter field, but the read/write tool sets only include built-in file/shell tools, so Hive MCP tools are not available to generated Claude SDD subagents unless added to the allowlist.

OpenCode installs a top-level `mcp.hive` entry and generated SDD subagents with per-agent `permission` blocks. The generated subagents do not define per-agent `tools` grants for Hive, and the OpenCode parser/verifier only checks top-level MCP registration plus subagent presence/task routing. OpenCode documentation confirms MCP tools can be globally or per-agent enabled with `tools` wildcard patterns such as `my-mcp*`, and permissions can match MCP/custom tool names with wildcard patterns such as `mymcp_*`.

### Affected Areas
- `jarvis-cli/internal/agent/sdd_phase_agents.go` — central SDD phase metadata; owns Claude tool allowlists and OpenCode subagent permission strings.
- `jarvis-cli/embed/templates/claude-sdd-agent.md.tmpl` — renders Claude `tools:` frontmatter from the phase definition.
- `jarvis-cli/internal/agent/claude_sdd_agents_test.go` — currently verifies phase-specific read/write boundaries but not Hive MCP availability.
- `jarvis-cli/internal/agent/opencode.go` — renders OpenCode generated subagents and can carry any per-agent tools/permissions fields derived from the shared phase definitions.
- `jarvis-cli/embed/templates/opencode.json.tmpl` — emits generated OpenCode agent JSON; may need a `tools` field and/or expanded permissions per SDD subagent.
- `jarvis-cli/internal/agent/opencode_parse.go` — parser currently ignores per-agent tools/permissions except orchestrator task routing.
- `jarvis-cli/internal/sddruntime/observed_opencode.go` and `jarvis-cli/internal/sddruntime/verify_opencode.go` — verification model/checks currently cannot prove SDD subagents have Hive MCP tool access.
- OpenCode tests (`opencode_test.go`, `runtime_opencode_parse_test.go`, `verify_opencode_checks_test.go`) — need regression coverage for generated Hive tool grants and verifier drift detection.
- SDD shared skill/orchestrator contracts — already require Engram/Hive artifact retrieval and persistence, so subagents must have the matching memory tools when running in Hive/hybrid modes.

### Approaches
1. **Shared phase capability metadata** — Add explicit Hive memory capability data to the SDD phase definitions and render platform-specific grants from it.
   - Pros: Single source of truth across Claude and OpenCode; keeps read/write file boundaries separate from memory access; easy to test all SDD phases consistently.
   - Cons: Requires small model/template/parser updates for OpenCode if `tools` fields are added.
   - Effort: Medium

2. **Patch platform outputs independently** — Append Hive MCP tool names directly to Claude tool lists and add hard-coded OpenCode `tools`/permission fragments in the renderer/template.
   - Pros: Fastest implementation; localized changes.
   - Cons: Higher drift risk between platforms; duplicates Hive tool naming; weaker architectural fit for future memory tool changes.
   - Effort: Low

3. **Verifier-only OpenCode change** — Keep generated OpenCode config unchanged and only strengthen diagnostics around top-level `mcp.hive`.
   - Pros: Minimal runtime risk.
   - Cons: Does not satisfy the parity goal because it still cannot explicitly grant/prove subagent Hive access; leaves default OpenCode behavior assumptions undocumented in generated config.
   - Effort: Low

### Recommendation
Use the shared phase capability metadata approach. Claude should receive the explicit tools `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary` in every generated SDD phase agent. OpenCode should explicitly grant Hive MCP tools per SDD subagent using documented `tools` wildcard/name patterns (likely `hive_*`, verified against generated tool names) and extend parsing/verification to fail when SDD subagents lack the grant. This fixes Claude’s confirmed allowlist bug and closes OpenCode’s parity/diagnostic gap without weakening file edit boundaries.

### Risks
- OpenCode exact tool naming must be validated against current OpenCode MCP naming (`hive_*` vs a more specific generated name), otherwise generated grants may not match runtime tools.
- Adding Hive memory tools to read-only SDD phases is intentional for artifact access but could be mistaken for write capability because `mem_save`/`mem_session_summary` persist memory; tests and docs should distinguish memory persistence from filesystem edits.
- Existing user-owned OpenCode config must remain preserved by deep merge; generated fields should not clobber unrelated custom agent configuration.
- Verification should avoid treating missing top-level Hive MCP as a hard error if current product semantics intentionally keep it warning-only for clean installs, while still detecting missing subagent grants when the generated config is present.

### Ready for Proposal
Yes — propose a focused bugfix SDD for GitHub issue #322 covering generated Claude SDD agent Hive tool allowlists, explicit OpenCode SDD subagent Hive tool grants, and regression/doctor verification for both platforms.
