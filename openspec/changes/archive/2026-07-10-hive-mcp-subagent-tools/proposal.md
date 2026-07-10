# Proposal: Hive MCP Tools for SDD Subagents

## Intent

Complete issue #322 by making Hive MCP tool availability explicit, testable, and diagnosable for SDD subagents in Claude Code and OpenCode. Hive/hybrid SDD flows must not silently fall back to inline context when subagents cannot access required Hive memory tools.

## Scope

### In Scope
- Grant required Hive MCP tools to generated Claude Code SDD phase agents.
- Explicitly grant/prove Hive MCP access for generated OpenCode SDD subagents.
- Extend runtime verification/doctor to detect outdated generated agents/config and recommend `jarvis init`/reconfigure regeneration.
- Document degraded behavior: fail clearly with actionable guidance in Hive/hybrid mode when a runtime cannot expose Hive MCP tools to SDD subagents.
- Add regression tests for generation, parsing, verification/doctor output, and degraded-mode messaging.

### Out of Scope
- Silent mutation of existing user config outside normal init/reconfigure flows.
- Replacing Hive MCP transport or changing Hive API sync semantics.
- Expanding SDD phase behavior beyond memory-tool availability and diagnostics.

## Capabilities

### New Capabilities
- `sdd-subagent-hive-tools`: Defines required Hive MCP grants, verification, and degraded-mode behavior for SDD subagents.

### Modified Capabilities
- None; no existing OpenSpec specs are present.

## Approach

Add shared SDD phase metadata for Hive memory tool requirements, then render platform-specific grants from that source. Claude Code should include explicit `mcp__hive__...` tools in each generated SDD phase agent. OpenCode should render explicit per-subagent Hive tool access using verified OpenCode tool naming/patterns, parse those fields, and fail verification when generated SDD subagents lack the grant. Doctor remains read-only: it reports drift/outdated generated artifacts and points users to regeneration.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/internal/agent/sdd_phase_agents.go` | Modified | Shared Hive tool capability metadata. |
| `jarvis-cli/embed/templates/claude-sdd-agent.md.tmpl` | Modified | Claude tool allowlist rendering. |
| `jarvis-cli/internal/agent/opencode.go` | Modified | Generated OpenCode subagent grant data. |
| `jarvis-cli/embed/templates/opencode.json.tmpl` | Modified | Per-subagent Hive tool access fields. |
| `jarvis-cli/internal/agent/opencode_parse.go` | Modified | Parse generated subagent Hive grants. |
| `jarvis-cli/internal/sddruntime/*opencode*.go` | Modified | Verify/doctor invariant and messages. |
| `docs/` | Modified | Fallback/degraded-mode documentation. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| OpenCode Hive tool pattern mismatch | Med | Validate naming in tests and docs before implementation. |
| User config clobbering | Low | Preserve deep-merge behavior; doctor recommends regeneration only. |
| Confusing memory writes with filesystem writes | Med | Tests/docs distinguish Hive artifact persistence from file edit permissions. |

## Rollback Plan

Revert generated-agent metadata/template/parser/verifier/doc changes. Users can rerun `jarvis init` or reconfigure to restore the previous generated artifacts.

## Dependencies

- Current Claude Code MCP tool allowlist naming.
- Current OpenCode MCP tool naming and per-agent tool/permission semantics.

## Success Criteria

- [ ] Claude SDD agents include required Hive MCP tools.
- [ ] OpenCode SDD subagents explicitly expose Hive tools and verifier detects drift.
- [ ] Doctor gives actionable regeneration guidance without unsafe mutation.
- [ ] Hive/hybrid degraded mode fails clearly instead of silently using inline fallback.
