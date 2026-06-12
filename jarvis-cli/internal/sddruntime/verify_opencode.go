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
func verifyOpenCodeConfigInvariants(oc ObservedOpenCodeConfig) []CheckResult {
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

	// --- R5: All 13 Subagents Present ---
	subStatus := StatusPass
	subMsg := fmt.Sprintf("all 13 required subagents present (hidden=true, mode=subagent): found %d", len(oc.HiddenSubagents))
	subObserved := fmt.Sprintf("%d hidden subagents", len(oc.HiddenSubagents))
	if len(oc.HiddenSubagents) != 13 {
		subStatus = StatusFail
		subMsg = fmt.Sprintf("required 13 subagents missing/not hidden/not mode=subagent; found %d", len(oc.HiddenSubagents))
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.subagents_present",
		Status:     subStatus,
		DriftClass: driftClassFromStatus(subStatus),
		Expected:   "13 hidden subagents with mode=subagent",
		Observed:   subObserved,
		Message:    subMsg,
	})

	// --- R6: Task Allowlist ---
	taskStatus := StatusPass
	taskMsg := fmt.Sprintf("orchestrator task allowlist complete: wildcard deny=true, %d named allows", len(oc.TaskAllows))
	taskObserved := fmt.Sprintf("wildcard_deny=%v, allows=%d", oc.TaskWildcardDeny, len(oc.TaskAllows))
	if !oc.TaskWildcardDeny || len(oc.TaskAllows) != 13 {
		taskStatus = StatusFail
		taskMsg = fmt.Sprintf(`orchestrator task allowlist drift: "*" deny=%v, named allows=%d (want 13)`, oc.TaskWildcardDeny, len(oc.TaskAllows))
	}
	results = append(results, CheckResult{
		Key:        "invariant.opencode.task_allowlist",
		Status:     taskStatus,
		DriftClass: driftClassFromStatus(taskStatus),
		Expected:   `task["*"]="deny" and 13 named allows`,
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

	// --- R9: MCP Hive (warning only) ---
	mcpHiveStatus := StatusPass
	mcpHiveMsg := "mcp.hive entry present (type=local, non-empty command)"
	mcpHiveObserved := fmt.Sprintf("mcp_hive_present=%v", oc.MCPHivePresent)
	mcpHiveDrift := DriftNone
	if !oc.MCPHivePresent {
		mcpHiveStatus = StatusWarn
		mcpHiveMsg = "mcp.hive missing/not local/empty command (daemon may be absent on clean install)"
		mcpHiveDrift = DriftUnknown
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
