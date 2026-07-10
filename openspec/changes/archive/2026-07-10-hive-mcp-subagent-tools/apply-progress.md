# Apply Progress: Hive MCP Tools for SDD Subagents

## Mode
Strict TDD

## Delivery Boundary
- Strategy: auto-chain
- Chain strategy: stacked-to-main
- Current assigned slice: corrective pre-PR critical review fixes round 3
- Boundary: Starts after round-3 verifier review found Claude stale Hive tool checks still failed non-Hive store modes; ends with focused `sddruntime`, `go test ./...`, and `go vet ./...` passing. Git/PR workflow remains blocked until pre-PR review is rerun.
- Prior completed slice: PR 1 — shared metadata and Claude SDD agent Hive MCP grants/tests
- Prior completed slice: PR 2 — OpenCode grants, parser, observer, verifier tests
- Prior completed corrective slice: PR 2 corrective fix — OpenCode parser/verifier permission action precedence and lifecycle fake fixture alignment
- Prior completed slice: PR 3 — doctor guidance, degraded-mode assets, docs

## Completed Tasks
- [x] 1.1 RED: Updated `jarvis-cli/internal/agent/claude_sdd_agents_test.go` to require all Claude SDD agents include required Hive MCP tools while preserving read-only filesystem boundaries.
- [x] 1.2 Added `HiveMCPToolRequirement` metadata/helpers to `jarvis-cli/internal/agent/sdd_phase_agents.go` for Claude and OpenCode Hive MCP tool names.
- [x] 1.3 Confirmed existing Claude renderer/template path already consumes `def.ClaudeTools`; no `claude.go` or `claude-sdd-agent.md.tmpl` changes were needed.
- [x] 2.1 RED: Updated `jarvis-cli/internal/agent/opencode_test.go` to require every generated OpenCode SDD subagent to include exact Hive MCP permission grants while preserving user-owned keys.
- [x] 2.2 Generated OpenCode SDD subagent Hive MCP grants from shared metadata via `withOpenCodeHiveMCPPermissions`; the existing template already renders the merged permission object.
- [x] 2.3 RED: Updated `jarvis-cli/internal/agent/runtime_opencode_parse_test.go` to require exact Hive grants plus `hive_mem_*` and `hive_*` wildcard evidence.
- [x] 2.4 Extended OpenCode parsing and observed runtime state to retain per-SDD-subagent Hive grant evidence.
- [x] 2.5 RED/GREEN: Updated OpenCode verifier tests and code to report missing SDD Hive grants as owned generated-artifact drift with `jarvis init` regeneration guidance.
- [x] C2.1 RED/GREEN: Retained OpenCode Hive permission action evidence and made exact `deny`/`ask` entries override wildcard Hive allow evidence conservatively during verification.
- [x] C2.2 RED/GREEN: Aligned lifecycle fake OpenCode compliant fixture with verifier-required `SDDSubagentHiveGrantEvidence`, restoring the uninstall owned-boundaries safety net before PR 3.
- [x] 3.1 RED/GREEN: Updated lifecycle doctor mapping so OpenCode generated-artifact drift remains read-only/manual and recommends `jarvis init` or supported reconfiguration with no-clobber wording.
- [x] 3.2 RED/GREEN: Updated shared SDD phase and persistence contract assets to fail clearly in Hive/hybrid degraded mode when required Hive MCP tools are unavailable to subagents.
- [x] 3.3 RED/GREEN: Updated `docs/sdd-user-guide.md` and added a docs contract test for existing-install regeneration and no-clobber guidance.
- [x] C3.1 RED/GREEN: Aligned shared degraded-mode assets and contract tests with all five required Hive MCP tools: `mcp__hive__mem_search`, `mcp__hive__mem_get_observation`, `mcp__hive__mem_save`, `mcp__hive__mem_context`, and `mcp__hive__mem_session_summary`.
- [x] C3.2 RED/GREEN: Clarified the OpenCode `mcp.hive` warning so generated-artifact drift diagnostics do not imply `openspec` or `none` SDD modes require Hive artifact persistence.
- [x] C3.3 RED/GREEN: Made missing OpenCode SDD subagent Hive grants blocking only in Hive/hybrid modes and advisory in OpenSpec/none modes; updated stale planning docs to list all five required Hive MCP tools.
- [x] C3.4 RED/GREEN: Made missing top-level OpenCode `mcp.hive` blocking in Hive/hybrid modes and advisory in OpenSpec/none modes so SDD subagents cannot appear Hive-ready when the Hive MCP server is absent.
- [x] C3.5 RED/GREEN: Treated empty or whitespace-only OpenCode top-level `mcp.hive.command` array entries as unusable so Hive/hybrid modes fail clearly instead of accepting an unusable MCP command.
- [x] C4.1 RED/GREEN: Aligned the TUI compliant OpenCode runtime verification fixture with verifier-required `SDDSubagentHiveGrantEvidence` using canonical SDD subagent definitions and required OpenCode Hive MCP tool metadata.
- [x] C5.1 RED/GREEN: Preserved user-owned OpenCode Hive wildcard `ask`/`deny` guardrails during generated permission merges and made verifier treat strict matching Hive policies as effective non-allow in Hive/hybrid mode.
- [x] C5.2 RED/GREEN: Treated OpenCode `mcp.hive.enabled=false` and malformed object command values as unusable for Hive/hybrid readiness while preserving enabled true/omitted and valid array/string commands.
- [x] C5.3 RED/GREEN: Made `jarvis sdd` `none` store mode use an inline/no-persistence artifact source without connecting to Hive.
- [x] C5.4 RED/GREEN: Added Claude SDD stale-agent Hive MCP tool verifier/doctor coverage with read-only regeneration guidance.
- [x] C5.5 RED/GREEN: Clarified degraded-mode assets with the OpenCode `hive_mem_*` tool-name mapping.
- [x] C6.1 RED/GREEN: Removed stale generated exact OpenCode Hive `allow` permissions when user-owned `hive_mem_*` or `hive_*` wildcard `ask`/`deny` guardrails exist, while preserving user-owned exact `ask`/`deny` entries and normal generation without strict wildcards.
- [x] C6.2 RED/GREEN: Forwarded provider-specific OpenCode parsed config and Claude SDD Hive tool evidence through the real lifecycle adapter so doctor/verify diagnoses malformed OpenCode config and stale Claude SDD agents through the real adapter path.
- [x] C7.1 RED/GREEN: Made stale or missing Claude SDD Hive MCP tools blocking only in Hive/hybrid modes and advisory in OpenSpec/none modes, matching OpenCode and the non-Hive artifact-store contract.

## TDD Cycle Evidence
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `jarvis-cli/internal/agent/claude_sdd_agents_test.go` | Unit | ✅ `go test ./internal/agent` passing before edits | ✅ Added Hive MCP assertions before production changes | ✅ `go test ./internal/agent -run TestRenderClaudeSDDPhaseAgents_UsesPhaseSpecificToolBoundaries` passed | ✅ Covered read-only and write-capable agents | ✅ `gofmt`; package tests passed |
| 1.2 | `jarvis-cli/internal/agent/claude_sdd_agents_test.go` | Unit | ✅ Same package safety net | ✅ Metadata contract driven by failing Claude grant test | ✅ Shared requirement helpers passed focused/package tests | ✅ Added platform-specific names test | ✅ Pure helper shape kept |
| 1.3 | `jarvis-cli/internal/agent/claude_sdd_agents_test.go` | Unit | ✅ Same package safety net | ✅ Existing renderer test exercised template data flow | ✅ Package tests proved rendered frontmatter includes Hive tools | ➖ Triangulation skipped: conditional wiring already consumed tool list | ✅ No template changes needed |
| 2.1 | `jarvis-cli/internal/agent/opencode_test.go` | Unit | ✅ `go test ./internal/agent ./internal/sddruntime` baseline | ✅ Added exact `hive_mem_*` permission assertions | ✅ Focused OpenCode generation test passed | ✅ Asserted all generated SDD subagents and all required tools | ✅ Permission merge isolated |
| 2.2 | `jarvis-cli/internal/agent/opencode_test.go` | Unit | ✅ Same package safety net | ✅ 2.1 RED drove generation | ✅ Added `withOpenCodeHiveMCPPermissions`; template already rendered `.Permission` | ✅ Required five OpenCode Hive tools | ✅ Deterministic permission generation |
| 2.3 | `jarvis-cli/internal/agent/runtime_opencode_parse_test.go` | Unit | ✅ `go test ./internal/agent ./internal/sddruntime` baseline | ✅ Parser test referenced missing `SDDSubagentHiveGrantEvidence` | ✅ Focused parser tests passed | ✅ Exact, `hive_mem_*`, and `hive_*` evidence covered | ✅ Parser helpers extracted |
| 2.4 | `jarvis-cli/internal/agent/runtime_opencode_parse_test.go` | Unit | ✅ Same package safety net | ✅ 2.3 RED drove observed model extension | ✅ Parsed config now fills evidence map | ✅ Evidence retained per subagent and sorted | ✅ Existing observer assignment carried parsed config |
| 2.5 | `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/agent ./internal/sddruntime` baseline | ✅ Verifier tests for missing grants/wildcard evidence failed before check existed | ✅ Focused verifier tests passed | ✅ Missing exact grant fails; wildcard passes; compliant config passes | ✅ Focused verifier helpers added |
| C2.1 | `jarvis-cli/internal/agent/runtime_opencode_parse_test.go`, `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/agent ./internal/sddruntime` passing before corrective edits | ✅ Tests required retaining permission actions and strict action precedence | ✅ Package tests passed after action evidence and precedence logic | ✅ Exact `deny`/`ask` over wildcard allow plus valid wildcard allow covered | ✅ Pure verifier helpers isolated |
| C2.2 | `jarvis-cli/internal/lifecycle/engine_uninstall_test.go` | Unit | ❌ Baseline reproduced uninstall safety-net failure before edits | ✅ Added fake fixture contract test for SDD Hive grant evidence | ✅ Fixture test and uninstall safety net passed | ✅ Covered all 10 SDD subagents and all five tools | ✅ Fixture grant helper extracted |
| 3.1 | `jarvis-cli/internal/lifecycle/engine_verify_test.go` | Unit | ✅ `go test ./internal/lifecycle -run TestEngineUninstall_UsesOwnedBoundariesOnly` passed before edits | ✅ Doctor regeneration guidance test failed before mapping | ✅ Focused regeneration guidance tests passed after mapping OpenCode generated-artifact drift | ✅ Added second case for missing generated `plugins/hive.ts` guidance | ✅ Mapping helpers extracted; `gofmt`; package tests passed |
| 3.2 | `jarvis-cli/internal/skills/catalog_contract_test.go` | Unit/contract | ✅ Existing asset contract tests read before edits; focused RED command established missing contract snippets | ✅ Degraded-mode contract test failed on missing sections | ✅ Focused asset contract test passed after asset updates | ✅ Covered both shared phase common and persistence contract assets | ✅ Wording kept small and explicit |
| 3.3 | `jarvis-cli/internal/skills/catalog_contract_test.go` | Unit/contract | ✅ Degraded-mode contract test passed before docs task | ✅ User guide contract test failed on missing guide section | ✅ Focused docs contract test passed after guide update | ✅ Required regeneration, read-only doctor, no-clobber, and no silent inline fallback wording | ✅ Documentation inserted as a short progressive-disclosure section |
| C3.1 | `jarvis-cli/internal/skills/catalog_contract_test.go` | Unit/contract | ✅ `go test ./internal/skills -run TestCatalogContract_SharedSDDProtocolsFailClearlyWhenHiveSubagentToolsAreUnavailable` passed before edits | ✅ Contract test failed while shared assets listed only three required Hive MCP tools | ✅ Focused contract test passed after both shared assets listed all five tools | ✅ Required five-tool wording in both phase common and persistence contract assets | ✅ Wording kept as one explicit canonical list |
| C3.2 | `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/sddruntime -run TestVerifyOpenCode_MCPHive_EmitsWarnWhenNotPresent` passed before edits | ✅ Warning test failed until message scoped non-Hive modes precisely | ✅ Focused warning test passed after message update | ✅ Runtime message distinguishes generated-artifact warning from `openspec`/`none` artifact-store requirements | ✅ Minimal message-only production change |
| C3.3 | `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/sddruntime -run 'TestVerifyOpenCode_SDDSubagentHiveGrants|TestVerifyOpenCode_MCPHive'` passed before edits | ✅ OpenSpec/none-mode missing-grant tests failed while verifier always blocked on missing grants | ✅ Focused missing-grant/store contract tests and `go test ./internal/sddruntime` passed after store-mode-aware severity | ✅ Covered OpenSpec/none advisory behavior and Hive blocking behavior | ✅ Store-mode severity helper extracted; `none` store contract support added; `gofmt` applied |
| C3.4 | `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/sddruntime -run 'TestVerifyOpenCode_MCPHive|TestVerifyOpenCode_SDDSubagentHiveGrants'` passed before edits | ✅ Missing `mcp.hive` tests failed while Hive/hybrid still returned `StatusWarn` | ✅ Focused MCP Hive tests and `go test ./internal/sddruntime` passed after store-mode-aware top-level MCP severity | ✅ Covered Hive/hybrid blocking and OpenSpec/none advisory behavior | ✅ Reused store-mode resolution helper shape; `gofmt` applied |
| C3.5 | `jarvis-cli/internal/agent/runtime_opencode_parse_test.go`, `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/agent -run 'TestIsMCPCommandNonEmpty|TestParseOpenCodeConfig_HiveMCPWithEmptyCommand'` and `go test ./internal/sddruntime -run 'TestVerifyOpenCode_MCPHive'` passed before edits | ✅ Parser tests failed for `command: [""]` and `["   "]` while verifier runtime test documented fail-clear behavior for unusable parsed commands | ✅ Focused parser/runtime tests and `go test ./internal/agent ./internal/sddruntime` passed | ✅ Covered empty array, empty string array, whitespace-only string array, and valid `command: ["hive-daemon"]` behavior | ✅ Minimal parser change; `gofmt` applied |
| C4.1 | `jarvis-cli/internal/tui/agent_setup_test.go` | Unit | ❌ Focused TUI runtime verification tests reproduced the verify failure before edits | ✅ Existing failing pass-report and pending-config tests served as RED for missing `SDDSubagentHiveGrantEvidence` in the compliant fixture | ✅ Focused TUI tests, `go test ./internal/tui`, and `go test ./internal/agent ./internal/sddruntime` passed after fixture update | ✅ Covered direct pass-report setup and pending OpenCode default config verification paths | ✅ Reused canonical agent definitions and required OpenCode Hive tool metadata in local TUI helper |
| C5.1 | `jarvis-cli/internal/agent/opencode_test.go`, `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Unit | ✅ `go test ./internal/agent ./internal/sddruntime ./internal/lifecycle ./cmd/jarvis` passed before edits | ✅ Tests failed because generated exact Hive allows weakened wildcard `ask`/`deny` and verifier accepted exact allows over strict wildcard policies | ✅ Focused package tests passed after merge and verifier changes | ✅ Covered `hive_mem_*` deny, `hive_*` ask, and strict wildcard plus exact allow evidence | ✅ Small helper extraction in merge logic and verifier precedence |
| C5.2 | `jarvis-cli/internal/agent/runtime_opencode_parse_test.go` | Unit | ✅ Same focused baseline | ✅ Tests failed for `enabled:false` and object command semantics | ✅ Parser tests passed after enabled handling and command validation | ✅ Covered enabled omitted/true/false, object command unusable, valid array/string commands | ✅ Removed object-command acceptance path |
| C5.3 | `jarvis-cli/cmd/jarvis/cmd_sdd_test.go` | Unit | ✅ Same focused baseline | ✅ Test reproduced `none` mode falling through to Hive connection | ✅ `cmd/jarvis` tests passed after inline none source | ✅ Covered `ListChanges` and `FetchArtifacts` empty inline behavior | ✅ Minimal private source implementation |
| C5.4 | `jarvis-cli/internal/sddruntime/verify_test.go`, `jarvis-cli/internal/lifecycle/engine_verify_test.go`, `jarvis-cli/internal/agent/runtime_integration_test.go` | Unit/integration | ✅ Same focused baseline | ✅ Tests initially failed to compile because Claude SDD Hive tool evidence was not modeled | ✅ Focused packages passed after observed model, runtime parser, verifier, and doctor mapping | ✅ Covered stale Claude SDD agent missing tools, compliant agents, and doctor regeneration guidance | ✅ Agent artifact inspection isolated to Claude SDD files |
| C5.5 | `jarvis-cli/internal/skills/catalog_contract_test.go` | Unit/contract | ✅ `go test ./internal/skills` passed before wording update | ✅ Contract required OpenCode-visible `hive_mem_*` mapping | ✅ Skills contract passed after asset update | ✅ Covered both shared phase common and persistence contract assets | ✅ Wording kept as explicit mapping |
| C6.1 | `jarvis-cli/internal/agent/opencode_test.go` | Unit | ✅ Focused affected package baseline from prior corrective slice | ✅ Test failed because stale exact `hive_mem_*` allow keys survived strict wildcard `ask`/`deny` guardrails | ✅ Focused agent test passed after permission-map pruning | ✅ Covered `hive_mem_*` deny, `hive_*` ask, stale exact allows, and preserved user-owned exact `ask`/`deny` | ✅ Reused exact Hive permission key helper for pruning |
| C6.2 | `jarvis-cli/internal/agent/lifecycle_adapter_test.go` | Unit/integration | ✅ Focused affected package baseline from prior corrective slice | ✅ Adapter test failed because `Observe` dropped OpenCode and Claude SDD Hive evidence | ✅ Focused adapter tests passed after forwarding provider-specific evidence | ✅ Covered raw adapter forwarding plus real doctor diagnosis for stale Claude agents and malformed OpenCode config | ✅ Minimal field-forwarding change |
| C7.1 | `jarvis-cli/internal/sddruntime/verify_test.go` | Unit | ✅ `go test ./internal/sddruntime -run 'TestVerifyClaude|TestResolveStore'` passed before edits | ✅ OpenSpec/none Claude stale-tool tests failed while verifier returned `StatusFail` | ✅ Focused `TestVerifyClaude`/`TestResolveStore`, package `go test ./internal/sddruntime`, broad `go test ./...`, and `go vet ./...` passed | ✅ Covered both `openspec` and `none` advisory behavior while preserving existing Hive fail/pass coverage | ✅ Reused shared Hive tool drift severity helper with OpenCode to avoid split semantics |

## Test Summary
- Total tests written/updated through corrective pre-PR fix: OpenCode merge/parser/verifier tests, `jarvis sdd` none-mode test, Claude stale-agent verifier/doctor/runtime-observation tests, and degraded-mode asset contract tests, in addition to prior corrective contract/parser/runtime cases.
- Total tests passing in this corrective pre-PR fix: focused package set, `go test ./...`, and `go vet ./...`.
- Round-2 focused tests passing: stale OpenCode Hive wildcard merge, lifecycle adapter evidence forwarding, real Claude stale-agent doctor path, and real malformed OpenCode doctor path.
- Round-3 focused tests passing: Claude stale/missing Hive tools fail in Hive/hybrid, warn in OpenSpec/none, and pass when compliant.
- Layers used: Unit/contract.
- Approval tests: None — behavior/docs contract changes, not refactoring-only work.
- Pure functions/helpers created or updated in this corrective fix: 2 local TUI test helpers.

## Files Changed In This Corrective Verify-Failure Slice
| File | Action | What Was Done |
|------|--------|---------------|
| `jarvis-cli/internal/skills/catalog_contract_test.go` | Modified | Required all five Hive MCP tools in shared degraded-mode contract assets and clarified artifact-store-mode scope. |
| `jarvis-cli/embed/skills/_shared/sdd-phase-common.md` | Modified | Updated degraded-mode required tools from three to five. |
| `jarvis-cli/embed/skills/_shared/persistence-contract.md` | Modified | Updated degraded-mode required tools from three to five and clarified non-Hive artifact-store behavior. |
| `jarvis-cli/internal/sddruntime/verify_opencode_checks_test.go` | Modified | Added RED/GREEN coverage for OpenSpec/none advisory behavior and Hive/hybrid blocking behavior for missing subagent grants and missing top-level `mcp.hive`. |
| `jarvis-cli/internal/agent/runtime_opencode_parse_test.go` | Modified | Added RED/GREEN parser coverage for unusable OpenCode `mcp.hive.command` arrays containing empty or whitespace-only command strings while preserving valid command arrays. |
| `jarvis-cli/internal/agent/opencode_parse.go` | Modified | Requires the first command-array entry to trim non-empty before top-level OpenCode `mcp.hive` is considered present. |
| `jarvis-cli/internal/sddruntime/store_contract.go` | Modified | Added `none` as a valid non-Hive store mode with no persistence targets. |
| `jarvis-cli/internal/sddruntime/store_contract_test.go` | Modified | Added RED/GREEN coverage for the `none` store contract. |
| `jarvis-cli/internal/sddruntime/verify.go` | Modified | Passes observed store mode into OpenCode-specific invariant checks. |
| `jarvis-cli/internal/sddruntime/verify_opencode.go` | Modified | Clarified `mcp.hive` warning, made missing SDD Hive grant severity store-mode-aware for OpenSpec/none, and made missing top-level `mcp.hive` fail in Hive/hybrid modes. |
| `openspec/changes/hive-mcp-subagent-tools/design.md` | Modified | Updated stale planning text from three required Hive MCP tools to all five required tools. |
| `openspec/changes/hive-mcp-subagent-tools/tasks.md` | Modified | Updated stale planning tasks to all five required tools and marked C3.3/C3.5 complete. |
| `openspec/changes/hive-mcp-subagent-tools/apply-progress.md` | Modified | Persisted merged corrective PR 3 apply progress for both artifact-store mode. |
| `jarvis-cli/internal/tui/agent_setup_test.go` | Modified | Populated the compliant OpenCode TUI runtime fixture with SDD subagent Hive grant evidence generated from canonical SDD phase definitions and required OpenCode Hive MCP tools. |
| `jarvis-cli/internal/agent/jsonmerge.go` | Modified | Preserved strict user-owned Hive wildcard permission guardrails when generated exact Hive allows are merged. |
| `jarvis-cli/internal/agent/opencode_parse.go` | Modified | Treats `enabled:false` and object command values as unusable for top-level OpenCode Hive MCP readiness. |
| `jarvis-cli/internal/agent/runtime.go` | Modified | Observes generated Claude SDD agent tool frontmatter for Hive MCP stale-artifact verification. |
| `jarvis-cli/cmd/jarvis/cmd_sdd.go` | Modified | Resolves `none` store mode to an inline/no-persistence artifact source without Hive connection. |
| `jarvis-cli/internal/sddruntime/verify.go` | Modified | Verifies observed Claude SDD agent Hive MCP tool allowlists and reports stale generated artifacts with regeneration guidance. |
| `jarvis-cli/internal/lifecycle/engine.go` | Modified | Maps Claude generated SDD Hive tool drift to read-only doctor regeneration guidance. |
| `jarvis-cli/internal/agent/jsonmerge.go` | Modified | Removes stale generated exact OpenCode Hive allow keys when strict user-owned Hive wildcard ask/deny policies exist. |
| `jarvis-cli/internal/agent/opencode_test.go` | Modified | Added stale exact allow regression coverage plus preserved user-owned exact ask/deny assertions. |
| `jarvis-cli/internal/agent/lifecycle_adapter.go` | Modified | Forwards provider-specific OpenCode and Claude SDD Hive evidence into lifecycle observed state. |
| `jarvis-cli/internal/agent/lifecycle_adapter_test.go` | Modified | Added adapter and real doctor-path regression coverage for provider-specific verifier evidence. |
| `jarvis-cli/internal/sddruntime/verify.go` | Modified | Makes Claude SDD Hive tool drift severity store-mode-aware. |
| `jarvis-cli/internal/sddruntime/verify_opencode.go` | Modified | Renamed the Hive tool drift severity helper for shared Claude/OpenCode use. |
| `jarvis-cli/internal/sddruntime/verify_test.go` | Modified | Added RED/GREEN coverage for Claude stale/missing Hive tools in OpenSpec and none modes. |

## Deviations from Design
None — implementation matches the corrective review feedback and preserves the design distinction between generated-artifact drift and SDD artifact-store mode requirements.

## Issues Found
- CodeGraph fallback note: `.codegraph` exists in the workspace listing, but CodeGraph MCP reported no usable index for this project, so targeted file reads/searches were used after that failure.
- The shared degraded-mode contract had drifted behind runtime metadata: runtime required five Hive MCP tools, while assets required only three.
- The OpenCode `mcp.hive` warning is intentionally generated-artifact/runtime-capability drift independent of artifact-store mode; its message now states that `openspec` and `none` modes do not require Hive artifact persistence.
- The OpenCode SDD subagent grant verifier also needed the observed store mode, otherwise OpenSpec/none modes inherited Hive/hybrid blocking semantics.
- Final resilience review found that top-level `mcp.hive` absence cannot remain warning-only in Hive/hybrid modes because generated SDD subagent grants are unusable without the MCP server itself.
- Final resilience review found that `command: [""]` or whitespace-only command-array values were incorrectly treated as usable top-level `mcp.hive` commands.
- Verify failure found that the TUI compliant OpenCode runtime fixture had drifted behind the verifier contract and did not populate `SDDSubagentHiveGrantEvidence`, causing valid Hive/hybrid setup paths to fail runtime verification.
- Pre-PR critical review found generated OpenCode exact Hive allows could weaken broader user-owned wildcard `ask`/`deny` policies unless merge and verifier logic preserve strict wildcard semantics.
- Pre-PR critical review found `mcp.hive.enabled=false`, malformed object commands, `none` store mode source resolution, and Claude stale-agent runtime inspection needed explicit tests and fixes.
- Round-2 review found stale exact OpenCode Hive allow permissions needed active pruning, not only avoiding new generation, when strict wildcard ask/deny policies already exist.
- Round-2 review found the real lifecycle adapter must carry provider-specific verifier evidence; fake lifecycle tests alone were not enough to prove doctor behavior.
- Round-3 review found Claude SDD Hive tool drift was still using unconditional Hive/hybrid blocking semantics and needed the same store-mode severity downgrade already used by OpenCode.

## Remaining Tasks
- [x] 4.1 Run focused package tests for `jarvis-cli/internal/agent`, `jarvis-cli/internal/sddruntime`, `jarvis-cli/internal/lifecycle`, `jarvis-cli/internal/tui`, and `jarvis-cli/internal/skills`.
- [x] 4.2 Run `go test ./...` and `go vet ./...`; ensure all spec scenarios pass.

## Workload / PR Boundary
- Mode: chained PR slice / stacked-to-main
- Current work unit: corrective pre-PR critical review fixes round 3
- Boundary: Starts after round-3 verifier review found Claude stale Hive tool checks still failed non-Hive store modes; ends with focused `sddruntime`, `go test ./...`, and `go vet ./...` passing. Git/PR workflow remains blocked until pre-PR review is rerun.
- Estimated review budget impact: Medium corrective slice; rerun review before PR.

## Status
29/29 implementation/corrective tasks complete. Corrective pre-PR round-3 verification passed with focused `sddruntime`, `go test ./...`, and `go vet ./...`; rerun the mandatory pre-PR review before git/PR workflow.
