package sddruntime

import (
	"fmt"
	"strings"
)

// verifyOpenCodeConfigInvariants checks opencode.json structural invariants.
// All checks are gated on agent == "opencode" — this function must only be
// called from Verify after the agent gate is confirmed.
//
// If oc.ParseSucceeded is false, only the structure-valid guard check is
// emitted and the function returns immediately, preventing nil/zero-value
// noise from invalid state.
func verifyOpenCodeConfigInvariants(oc ObservedOpenCodeConfig, storeMode string) []CheckResult {
	var results []CheckResult

	// Guard: if JSON parsing failed, emit one error and short-circuit.
	if !oc.ParseSucceeded {
		results = append(results, CheckResult{
			Key:        "invariant.opencode.structure_valid",
			Status:     StatusFail,
			DriftClass: DriftOwned,
			Expected:   "opencode.json present, non-empty, and valid JSON",
			Observed:   "parse failed or file absent",
			Message:    "opencode.json could not be parsed; re-run jarvis init to regenerate",
		})
		return results
	}

	// --- R1: Share Mode ---
	shareStatus := StatusPass
	shareMsg := "share mode is disabled"
	shareObserved := oc.ShareMode
	if oc.ShareMode != "disabled" {
		shareStatus = StatusFail
		shareMsg = `share must be "disabled"; telemetry/sharing is left enabled`
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.share_disabled",
		Status:     shareStatus,
		DriftClass: driftClassFromStatus(shareStatus),
		Expected:   "disabled",
		Observed:   shareObserved,
		Message:    shareMsg,
	})

	// --- R2: Default Agent ---
	daStatus := StatusPass
	daMsg := "default_agent is sdd-orchestrator"
	daObserved := oc.DefaultAgent
	if oc.DefaultAgent != "sdd-orchestrator" {
		daStatus = StatusFail
		daMsg = `default_agent must be "sdd-orchestrator"`
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.default_agent",
		Status:     daStatus,
		DriftClass: driftClassFromStatus(daStatus),
		Expected:   "sdd-orchestrator",
		Observed:   daObserved,
		Message:    daMsg,
	})

	// --- R3 + R4: Orchestrator Primary Entry (mode, model, prompt ref) ---
	orchStatus := StatusPass
	orchMsg := "sdd-orchestrator entry is primary with non-empty model and prompt reference"
	orchObserved := fmt.Sprintf("mode=%s model_len=%d prompt_contains_ref=%v",
		oc.OrchestratorMode,
		len(oc.OrchestratorModel),
		strings.Contains(oc.OrchestratorPrompt, "sdd-orchestrator.md"),
	)
	if oc.OrchestratorMode != "primary" || oc.OrchestratorModel == "" || !strings.Contains(oc.OrchestratorPrompt, "sdd-orchestrator.md") {
		orchStatus = StatusFail
		orchMsg = "sdd-orchestrator missing or mode!=primary / model empty / prompt ref missing"
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.orchestrator_primary",
		Status:     orchStatus,
		DriftClass: driftClassFromStatus(orchStatus),
		Expected:   "mode=primary, non-empty model, prompt references sdd-orchestrator.md",
		Observed:   orchObserved,
		Message:    orchMsg,
	})

	// --- R5: Required Subagents Present ---
	// 10 SDD + 3 Judgment Day + 4 Review required hidden subagents.
	missingSubagents, unexpectedSubagents := diffRequiredOpenCodeSubagents(oc.HiddenSubagents)
	subStatus := StatusPass
	subMsg := fmt.Sprintf("all required subagents present (hidden=true, mode=subagent): found %d", len(oc.HiddenSubagents))
	subObserved := fmt.Sprintf("%d hidden subagents", len(oc.HiddenSubagents))
	if len(missingSubagents) > 0 {
		subStatus = StatusFail
		subObserved = formatOpenCodeSubagentDiff(missingSubagents, unexpectedSubagents)
		subMsg = "required subagents missing/not hidden/not mode=subagent: " + subObserved
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.subagents_present",
		Status:     subStatus,
		DriftClass: driftClassFromStatus(subStatus),
		Expected:   strings.Join(requiredOpenCodeSubagents(), ","),
		Observed:   subObserved,
		Message:    subMsg,
	})

	// --- R6: Task Allowlist ---
	// 10 SDD + 3 Judgment Day + 4 Review named allows required.
	missingTaskAllows, unexpectedTaskAllows := diffRequiredOpenCodeSubagents(oc.TaskAllows)
	taskStatus := StatusPass
	taskMsg := fmt.Sprintf("orchestrator task allowlist complete: wildcard deny=true, %d named allows", len(oc.TaskAllows))
	taskObserved := fmt.Sprintf("wildcard_deny=%v, allows=%d", oc.TaskWildcardDeny, len(oc.TaskAllows))
	if !oc.TaskWildcardDeny || len(missingTaskAllows) > 0 {
		taskStatus = StatusFail
		taskObserved = fmt.Sprintf("wildcard_deny=%v, %s", oc.TaskWildcardDeny, formatOpenCodeSubagentDiff(missingTaskAllows, unexpectedTaskAllows))
		taskMsg = fmt.Sprintf(`orchestrator task allowlist drift: "*" deny=%v, %s`, oc.TaskWildcardDeny, formatOpenCodeSubagentDiff(missingTaskAllows, unexpectedTaskAllows))
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.task_allowlist",
		Status:     taskStatus,
		DriftClass: driftClassFromStatus(taskStatus),
		Expected:   `task["*"]="deny" and named allows: ` + strings.Join(requiredOpenCodeSubagents(), ","),
		Observed:   taskObserved,
		Message:    taskMsg,
	})

	// --- R7: Bash Wildcard Allow ---
	bashStatus := StatusPass
	bashMsg := `bash wildcard allow ("*"="allow") is present`
	bashObserved := fmt.Sprintf("bash_wildcard_allow=%v", oc.BashWildcardAllow)
	if !oc.BashWildcardAllow {
		bashStatus = StatusFail
		bashMsg = `bash "*" not allow; orchestrator may be unable to run shell commands`
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.permission_bash",
		Status:     bashStatus,
		DriftClass: driftClassFromStatus(bashStatus),
		Expected:   `bash["*"]="allow"`,
		Observed:   bashObserved,
		Message:    bashMsg,
	})

	// --- R8: Read Secret Deny Patterns ---
	readStatus := StatusPass
	readMsg := "read permission contains secret/credential deny patterns"
	readObserved := fmt.Sprintf("read_secret_denies=%v", oc.ReadSecretDenies)
	if !oc.ReadSecretDenies {
		readStatus = StatusFail
		readMsg = "read secret-deny patterns missing (.env/secrets/tokens/keys)"
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.permission_read_secrets",
		Status:     readStatus,
		DriftClass: driftClassFromStatus(readStatus),
		Expected:   "deny patterns for .env, secrets, tokens, keys",
		Observed:   readObserved,
		Message:    readMsg,
	})

	// --- R11: Plugin hive.ts (error) ---
	pluginStatus := StatusPass
	pluginMsg := "plugins/hive.ts exists and is non-empty"
	pluginObserved := fmt.Sprintf("plugin_hive_exists=%v", oc.PluginHiveExists)
	if !oc.PluginHiveExists {
		pluginStatus = StatusFail
		pluginMsg = "plugins/hive.ts missing or empty; re-run jarvis init to install the prompt hook"
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.plugin_hive",
		Status:     pluginStatus,
		DriftClass: driftClassFromStatus(pluginStatus),
		Expected:   "plugins/hive.ts present and non-empty",
		Observed:   pluginObserved,
		Message:    pluginMsg,
	})

	// --- R12: SDD subagent Hive MCP grants ---
	hiveGrantStatus := StatusPass
	hiveGrantMsg := "all generated SDD subagents include Hive MCP tool grants"
	hiveGrantDrift := DriftNone
	missingHiveGrants := missingOpenCodeSDDSubagentHiveGrants(oc.SDDSubagentHiveGrantEvidence)
	if len(missingHiveGrants) > 0 {
		hiveGrantStatus = hiveToolDriftStatus(storeMode)
		hiveGrantDrift = DriftOwned
		if openCodeHiveGrantsBlockedByStrictWildcard(oc.SDDSubagentHiveGrantEvidence) {
			hiveGrantDrift = DriftNonOwned
			if hiveGrantStatus == StatusFail {
				hiveGrantMsg = "strict user-owned OpenCode Hive wildcard guardrail blocks generated Hive MCP access for Hive/hybrid mode; manually adjust or remove the hive_mem_* / hive_* guardrail, or add exact tool allows where appropriate"
			} else {
				hiveGrantMsg = "strict user-owned OpenCode Hive wildcard guardrail blocks generated Hive MCP access; advisory only because this SDD store mode does not require Hive persistence"
			}
		} else if hiveGrantStatus == StatusFail {
			hiveGrantMsg = "generated OpenCode SDD subagent Hive MCP grants are missing for Hive/hybrid mode; re-run jarvis init or supported reconfiguration to regenerate opencode.json"
		} else {
			hiveGrantMsg = "generated OpenCode SDD subagent Hive MCP grants are missing; advisory generated-artifact drift only because this SDD store mode does not require Hive persistence"
		}
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.sdd_hive_grants",
		Status:     hiveGrantStatus,
		DriftClass: hiveGrantDrift,
		Expected:   strings.Join(requiredOpenCodeHiveMCPTools(), ","),
		Observed:   strings.Join(missingHiveGrants, ","),
		Message:    hiveGrantMsg,
	})

	// --- R9: MCP Hive ---
	mcpHiveStatus := StatusPass
	mcpHiveMsg := "mcp.hive entry present (type=local, non-empty command)"
	mcpHiveObserved := fmt.Sprintf("mcp_hive_present=%v", oc.MCPHivePresent)
	mcpHiveDrift := DriftNone
	if !oc.MCPHivePresent {
		mcpHiveStatus = openCodeMCPHiveDriftStatus(storeMode)
		mcpHiveDrift = DriftOwned
		if mcpHiveStatus == StatusFail {
			mcpHiveMsg = "Hive/hybrid mode requires top-level mcp.hive because SDD subagents cannot reach Hive tools without the MCP server; re-run jarvis init or supported reconfiguration"
		} else {
			mcpHiveMsg = "mcp.hive missing/not local/empty command (daemon may be absent on clean install); Hive artifact persistence is not required for openspec or none modes"
			mcpHiveDrift = DriftUnknown
		}
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.mcp_hive",
		Status:     mcpHiveStatus,
		DriftClass: mcpHiveDrift,
		Expected:   `mcp.hive with type="local" and non-empty command`,
		Observed:   mcpHiveObserved,
		Message:    mcpHiveMsg,
	})

	// --- R10: MCP Context7 (warning only) ---
	ctx7Status := StatusPass
	ctx7Msg := "mcp.context7 entry present (type=remote)"
	ctx7Observed := fmt.Sprintf("mcp_context7_present=%v", oc.MCPContext7Present)
	ctx7Drift := DriftNone
	if !oc.MCPContext7Present {
		ctx7Status = StatusWarn
		ctx7Msg = "mcp.context7 missing/not remote/non-canonical url"
		ctx7Drift = DriftUnknown
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.mcp_context7",
		Status:     ctx7Status,
		DriftClass: ctx7Drift,
		Expected:   `mcp.context7 with type="remote"`,
		Observed:   ctx7Observed,
		Message:    ctx7Msg,
	})

	return results
}

func hiveToolDriftStatus(storeMode string) IntegrityStatus {
	mode, err := ResolveStoreMode(storeMode)
	if err != nil {
		return StatusFail
	}
	if mode == StoreModeOpenSpec || mode == StoreModeNone {
		return StatusWarn
	}
	return StatusFail
}

func openCodeMCPHiveDriftStatus(storeMode string) IntegrityStatus {
	mode, err := ResolveStoreMode(storeMode)
	if err != nil {
		return StatusFail
	}
	if mode == StoreModeOpenSpec || mode == StoreModeNone {
		return StatusWarn
	}
	return StatusFail
}

func openCodeHiveGrantsBlockedByStrictWildcard(evidence map[string][]OpenCodePermissionEvidence) bool {
	for _, agentName := range requiredOpenCodeSDDSubagents() {
		agentEvidence := evidence[agentName]
		for _, tool := range requiredOpenCodeHiveMCPTools() {
			if openCodeHiveGrantEvidenceAllows(agentEvidence, tool) {
				continue
			}
			for _, entry := range agentEvidence {
				if (entry.Key == "hive_mem_*" || entry.Key == "hive_*") && openCodeHivePermissionSpecificity(entry.Key, tool) >= 0 && isOpenCodeStricterPermissionAction(entry.Action) {
					return true
				}
			}
		}
	}
	return false
}

func missingOpenCodeSDDSubagentHiveGrants(evidence map[string][]OpenCodePermissionEvidence) []string {
	missing := make([]string, 0)
	for _, agentName := range requiredOpenCodeSDDSubagents() {
		agentEvidence := evidence[agentName]
		for _, tool := range requiredOpenCodeHiveMCPTools() {
			if !openCodeHiveGrantEvidenceAllows(agentEvidence, tool) {
				missing = append(missing, agentName+":"+tool)
			}
		}
	}
	return missing
}

func openCodeHiveGrantEvidenceAllows(evidence []OpenCodePermissionEvidence, tool string) bool {
	lastAction := ""
	for _, entry := range evidence {
		if openCodeHivePermissionSpecificity(entry.Key, tool) < 0 {
			continue
		}
		lastAction = entry.Action
	}
	return lastAction == "allow"
}

func openCodeHivePermissionSpecificity(pattern, tool string) int {
	switch pattern {
	case tool:
		return 3
	case "hive_mem_*":
		if strings.HasPrefix(tool, "hive_mem_") {
			return 2
		}
	case "hive_*":
		if strings.HasPrefix(tool, "hive_") {
			return 1
		}
	}
	return -1
}

func isOpenCodeStricterPermissionAction(action string) bool {
	return action == "ask" || action == "deny"
}

func requiredOpenCodeHiveMCPTools() []string {
	return []string{"hive_mem_search", "hive_mem_get_observation", "hive_mem_save", "hive_mem_context", "hive_mem_session_summary"}
}

func requiredOpenCodeSDDSubagents() []string {
	return []string{
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}
}

func requiredOpenCodeSubagents() []string {
	return []string{
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
		"jd-judge-a",
		"jd-judge-b",
		"jd-fix-agent",
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
	}
}

func diffRequiredOpenCodeSubagents(observed []string) ([]string, []string) {
	requiredSet := make(map[string]struct{})
	for _, required := range requiredOpenCodeSubagents() {
		requiredSet[required] = struct{}{}
	}
	present := make(map[string]struct{}, len(observed))
	for _, name := range observed {
		present[name] = struct{}{}
	}
	missing := make([]string, 0)
	for _, required := range requiredOpenCodeSubagents() {
		if _, ok := present[required]; !ok {
			missing = append(missing, required)
		}
	}
	unexpected := make([]string, 0)
	for _, name := range observed {
		if _, ok := requiredSet[name]; !ok {
			unexpected = append(unexpected, name)
		}
	}
	return missing, unexpected
}

func formatOpenCodeSubagentDiff(missing, unexpected []string) string {
	parts := make([]string, 0, 2)
	if len(missing) > 0 {
		parts = append(parts, "missing="+strings.Join(missing, ","))
	}
	if len(unexpected) > 0 {
		parts = append(parts, "unexpected="+strings.Join(unexpected, ","))
	}
	if len(parts) == 0 {
		return "missing= unexpected="
	}
	return strings.Join(parts, " ")
}
