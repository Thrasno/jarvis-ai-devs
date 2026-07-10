# Tasks: Hive MCP Tools for SDD Subagents

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 650-850 |
| 800-line budget risk | Medium |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 Claude grants/tests → PR 2 OpenCode grants/parser/verifier/tests → PR 3 doctor/docs/degraded-mode |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
800-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Shared metadata and Claude SDD agent grants | PR 1 | Include Claude generation tests. |
| 2 | OpenCode grants, parser, observer, verifier | PR 2 | Depends on PR 1 metadata. |
| 3 | Doctor guidance, degraded-mode assets, docs | PR 3 | Depends on verifier drift results. |

## Phase 1: Shared Metadata and Claude Grants

- [x] 1.1 RED: Update `jarvis-cli/internal/agent/claude_sdd_agents_test.go` to require all SDD agents include `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary` without broader file permissions.
- [x] 1.2 Add `HiveMCPToolRequirement` metadata/helpers to `jarvis-cli/internal/agent/sdd_phase_agents.go` for Claude and OpenCode tool names.
- [x] 1.3 Wire Claude rendering through `jarvis-cli/internal/agent/claude.go` and `jarvis-cli/embed/templates/claude-sdd-agent.md.tmpl` if template data needs the new grants.

## Phase 2: OpenCode Grants, Parsing, and Verification

- [x] 2.1 RED: Update `jarvis-cli/internal/agent/opencode_test.go` to require per-SDD-agent `permission` grants for `hive_mem_search`, `hive_mem_get_observation`, `hive_mem_save`, `hive_mem_context`, and `hive_mem_session_summary` while preserving user-owned keys.
- [x] 2.2 Generate OpenCode grants in `jarvis-cli/internal/agent/opencode.go` and `jarvis-cli/embed/templates/opencode.json.tmpl` using shared metadata.
- [x] 2.3 RED: Update `jarvis-cli/internal/agent/runtime_opencode_parse_test.go` for exact grants plus `hive_mem_*`/`hive_*` wildcard evidence.
- [x] 2.4 Extend `jarvis-cli/internal/agent/opencode_parse.go`, `runtime.go`, and `jarvis-cli/internal/sddruntime/observed_opencode.go` to retain per-subagent grant evidence.
- [x] 2.5 RED/GREEN: Update `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go`, `verify.go`, and `verify_opencode.go` to report missing grants as generated-artifact drift.

## Corrective Continuation: PR 2 Resilience Review

- [x] C2.1 RED/GREEN: Retain OpenCode Hive permission actions (`allow`, `ask`, `deny`) and fail verification when an exact `ask`/`deny` entry overrides wildcard Hive allow evidence for a required SDD subagent tool.
- [x] C2.2 RED/GREEN: Align lifecycle fake OpenCode compliant fixture with verifier-required `SDDSubagentHiveGrantEvidence` so uninstall safety nets pass before PR 3 work.

## Phase 3: Doctor, Degraded Mode, and Documentation

- [x] 3.1 RED/GREEN: Update `jarvis-cli/internal/lifecycle/engine_verify_test.go` and `engine.go` so doctor remains read-only and recommends `jarvis init` or supported reconfiguration.
- [x] 3.2 RED/GREEN: Update `jarvis-cli/internal/skills/catalog_contract_test.go`, `jarvis-cli/embed/skills/_shared/sdd-phase-common.md`, and `persistence-contract.md` to fail clearly in Hive/hybrid degraded mode.
- [x] 3.3 Update `docs/sdd-user-guide.md` with existing-install regeneration guidance and no-clobber wording.

## Corrective Continuation: PR 3 Degraded-Mode Contract Alignment

- [x] C3.1 RED/GREEN: Align shared degraded-mode assets and contract tests with all five required Hive MCP tools: `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary`.
- [x] C3.2 RED/GREEN: Clarify the OpenCode `mcp.hive` warning so generated-artifact drift diagnostics do not imply `openspec` or `none` SDD modes require Hive artifact persistence.
- [x] C3.3 RED/GREEN: Make missing OpenCode SDD subagent Hive grants blocking only in Hive/hybrid modes and advisory in OpenSpec/none modes; update stale planning docs to list all five required Hive MCP tools.
- [x] C3.4 RED/GREEN: Make missing top-level OpenCode `mcp.hive` blocking in Hive/hybrid modes and advisory in OpenSpec/none modes while preserving non-Hive warning behavior.
- [x] C3.5 RED/GREEN: Treat empty or whitespace-only OpenCode top-level `mcp.hive.command` array entries as unusable so Hive/hybrid modes fail clearly instead of accepting an unusable MCP command.

## Corrective Continuation: Verify Failure Fix

- [x] C4.1 RED/GREEN: Align the TUI compliant OpenCode runtime verification fixture with verifier-required `SDDSubagentHiveGrantEvidence` so pass-report and pending-config setup paths remain green in Hive/hybrid mode.

## Corrective Continuation: Pre-PR Critical Review Fixes

- [x] C5.1 RED/GREEN: Preserve user-owned OpenCode Hive wildcard `ask`/`deny` guardrails during generated permission merges and make verifier treat strict matching Hive policies as effective non-allow in Hive/hybrid mode.
- [x] C5.2 RED/GREEN: Treat OpenCode `mcp.hive.enabled=false` and malformed object command values as unusable for Hive/hybrid readiness while preserving enabled true/omitted and valid array/string commands.
- [x] C5.3 RED/GREEN: Make `jarvis sdd` `none` store mode use an inline/no-persistence artifact source without connecting to Hive.
- [x] C5.4 RED/GREEN: Add Claude SDD stale-agent Hive MCP tool verifier/doctor coverage with read-only regeneration guidance.
- [x] C5.5 RED/GREEN: Clarify degraded-mode assets with the OpenCode `hive_mem_*` tool-name mapping.

## Corrective Continuation: Pre-PR Critical Review Fixes Round 2

- [x] C6.1 RED/GREEN: Remove stale generated exact OpenCode Hive `allow` permissions when user-owned `hive_mem_*` or `hive_*` wildcard `ask`/`deny` guardrails exist, while preserving user-owned exact `ask`/`deny` entries and normal exact allow generation without strict wildcards.
- [x] C6.2 RED/GREEN: Forward provider-specific OpenCode parsed config and Claude SDD Hive tool evidence through the real lifecycle adapter so doctor/verify can diagnose malformed OpenCode config and stale Claude SDD agents outside fake lifecycle tests.

## Corrective Continuation: Pre-PR Critical Review Fixes Round 3

- [x] C7.1 RED/GREEN: Make stale or missing Claude SDD Hive MCP tools blocking only in Hive/hybrid modes and advisory in OpenSpec/none modes, matching the non-Hive artifact-store contract and OpenCode behavior.

## Phase 4: Verification

- [x] 4.1 Run focused package tests for `jarvis-cli/internal/agent`, `jarvis-cli/internal/sddruntime`, `jarvis-cli/internal/lifecycle`, `jarvis-cli/internal/tui`, `jarvis-cli/internal/skills`, and `jarvis-cli/cmd/jarvis`.
- [x] 4.2 Run `go test ./...` and `go vet ./...`; ensure all spec scenarios pass.
