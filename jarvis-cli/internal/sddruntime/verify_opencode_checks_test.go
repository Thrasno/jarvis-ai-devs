package sddruntime

import (
	"strings"
	"testing"
)

// compliantOpenCodeRuntime returns a fully-passing ObservedRuntime for the
// opencode agent. The OpenCode field is populated via compliantOpenCodeObserved
// (defined in verify_test.go).
func compliantOpenCodeRuntime(t *testing.T) ObservedRuntime {
	t.Helper()
	// compliantObservedRuntime already sets OpenCode via compliantOpenCodeObserved.
	return compliantObservedRuntime(t)
}

// ---------------------------------------------------------------------------
// Phase 6: Structure Valid Guard
// ---------------------------------------------------------------------------

// TestVerifyOpenCode_ParseFailure_EmitsStructureValidOnlyAndNoOtherOpenCodeChecks
// asserts that when ParseSucceeded==false, only invariant.opencode.structure_valid
// is emitted and no other invariant.opencode.* checks appear.
func TestVerifyOpenCode_ParseFailure_EmitsStructureValidOnlyAndNoOtherOpenCodeChecks(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode = ObservedOpenCodeConfig{ParseSucceeded: false} // all other fields zero

	report := Verify("opencode", observed)

	structCheck := findCheckByKey(report.Checks, "invariant.opencode.structure_valid")
	if structCheck == nil {
		t.Fatal("expected invariant.opencode.structure_valid check when ParseSucceeded==false")
	}
	if structCheck.Status != StatusFail {
		t.Fatalf("expected StatusFail for structure_valid, got %q", structCheck.Status)
	}

	// No other invariant.opencode.* key should appear.
	for _, c := range report.Checks {
		if c.Key != "invariant.opencode.structure_valid" && strings.HasPrefix(c.Key, "invariant.opencode.") {
			t.Errorf("unexpected opencode check %q when ParseSucceeded==false", c.Key)
		}
	}
}

// TestVerifyOpenCode_ParseSuccess_DoesNotEmitStructureValidFail verifies that
// when ParseSucceeded==true the structure guard does not emit a fail check.
func TestVerifyOpenCode_ParseSuccess_DoesNotEmitStructureValidFail(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.structure_valid")
	// Either not present at all, or present with pass status.
	if check != nil && check.Status == StatusFail {
		t.Fatalf("structure_valid must not fail when ParseSucceeded==true, got %q", check.Status)
	}
}

// TestVerifyOpenCode_AgentGate_NoOpenCodeChecksForClaudeAgent confirms that
// none of the invariant.opencode.* checks appear when agent is "claude".
func TestVerifyOpenCode_AgentGate_NoOpenCodeChecksForClaudeAgent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("claude", observed)

	for _, c := range report.Checks {
		if strings.HasPrefix(c.Key, "invariant.opencode.") {
			t.Errorf("unexpected opencode check %q for claude agent", c.Key)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 7: Error-severity check keys
// ---------------------------------------------------------------------------

// --- invariant.opencode.share_disabled ---

func TestVerifyOpenCode_ShareDisabled_FailsWhenShareModeNotDisabled(t *testing.T) {
	tests := []struct {
		name      string
		shareMode string
	}{
		{"empty", ""},
		{"enabled", "enabled"},
		{"team", "team"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantOpenCodeRuntime(t)
			observed.OpenCode.ShareMode = tt.shareMode

			report := Verify("opencode", observed)

			check := findCheckByKey(report.Checks, "invariant.opencode.share_disabled")
			if check == nil {
				t.Fatal("expected invariant.opencode.share_disabled check")
			}
			if check.Status != StatusFail {
				t.Fatalf("expected StatusFail, got %q", check.Status)
			}
		})
	}
}

func TestVerifyOpenCode_ShareDisabled_PassesWhenShareModeIsDisabled(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.ShareMode = "disabled"

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.share_disabled")
	if check == nil {
		t.Fatal("expected invariant.opencode.share_disabled check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

func TestVerifyOpenCode_ShareDisabled_NotEmittedForNonOpenCodeAgent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.ShareMode = "enabled" // would fail if checked

	report := Verify("claude", observed)

	if c := findCheckByKey(report.Checks, "invariant.opencode.share_disabled"); c != nil {
		t.Errorf("share_disabled must not be emitted for claude agent")
	}
}

// --- invariant.opencode.default_agent ---

func TestVerifyOpenCode_DefaultAgent_FailsWhenNotSddOrchestrator(t *testing.T) {
	tests := []struct {
		name         string
		defaultAgent string
	}{
		{"empty", ""},
		{"wrong", "claude"},
		{"partial", "sdd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantOpenCodeRuntime(t)
			observed.OpenCode.DefaultAgent = tt.defaultAgent

			report := Verify("opencode", observed)

			check := findCheckByKey(report.Checks, "invariant.opencode.default_agent")
			if check == nil {
				t.Fatal("expected invariant.opencode.default_agent check")
			}
			if check.Status != StatusFail {
				t.Fatalf("expected StatusFail, got %q", check.Status)
			}
		})
	}
}

func TestVerifyOpenCode_DefaultAgent_PassesWhenCorrect(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.DefaultAgent = "sdd-orchestrator"

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.default_agent")
	if check == nil {
		t.Fatal("expected invariant.opencode.default_agent check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// --- invariant.opencode.orchestrator_primary ---

func TestVerifyOpenCode_OrchestratorPrimary_FailsWhenModeNotPrimary(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.OrchestratorMode = "subagent"

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.orchestrator_primary")
	if check == nil {
		t.Fatal("expected invariant.opencode.orchestrator_primary check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_OrchestratorPrimary_FailsWhenModelEmpty(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.OrchestratorModel = ""

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.orchestrator_primary")
	if check == nil {
		t.Fatal("expected invariant.opencode.orchestrator_primary check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_OrchestratorPrimary_FailsWhenPromptMissingReference(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.OrchestratorPrompt = "some-other-prompt.md"

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.orchestrator_primary")
	if check == nil {
		t.Fatal("expected invariant.opencode.orchestrator_primary check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_OrchestratorPrimary_PassesWhenAllFieldsCorrect(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.orchestrator_primary")
	if check == nil {
		t.Fatal("expected invariant.opencode.orchestrator_primary check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// --- invariant.opencode.subagents_present ---

func TestVerifyOpenCode_SubagentsPresent_FailsWhenCountBelow17(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	// Remove one hidden subagent.
	observed.OpenCode.HiddenSubagents = observed.OpenCode.HiddenSubagents[:16]

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.subagents_present")
	if check == nil {
		t.Fatal("expected invariant.opencode.subagents_present check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_SubagentsPresent_FailsWhenEmpty(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.HiddenSubagents = nil

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.subagents_present")
	if check == nil {
		t.Fatal("expected invariant.opencode.subagents_present check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_SubagentsPresent_PassesWhenExactly17(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	// compliantOpenCodeObserved sets exactly 17.

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.subagents_present")
	if check == nil {
		t.Fatal("expected invariant.opencode.subagents_present check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

func TestVerifyOpenCode_SubagentsPresent_FailsWhenCountAbove17(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.HiddenSubagents = append(observed.OpenCode.HiddenSubagents, "extra-agent")

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.subagents_present")
	if check == nil {
		t.Fatal("expected invariant.opencode.subagents_present check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail for 18 subagents, got %q", check.Status)
	}
}

// --- invariant.opencode.task_allowlist ---

func TestVerifyOpenCode_TaskAllowlist_FailsWhenWildcardDenyMissing(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.TaskWildcardDeny = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.task_allowlist")
	if check == nil {
		t.Fatal("expected invariant.opencode.task_allowlist check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_TaskAllowlist_FailsWhenAllowsBelow17(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.TaskAllows = observed.OpenCode.TaskAllows[:16]

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.task_allowlist")
	if check == nil {
		t.Fatal("expected invariant.opencode.task_allowlist check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_TaskAllowlist_PassesWhenDenyAndExactly17Allows(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.task_allowlist")
	if check == nil {
		t.Fatal("expected invariant.opencode.task_allowlist check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

func TestVerifyOpenCode_TaskAllowlist_FailsWhenAllowsAbove17(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.TaskAllows = append(observed.OpenCode.TaskAllows, "extra-task")

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.task_allowlist")
	if check == nil {
		t.Fatal("expected invariant.opencode.task_allowlist check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail for 18 task allows, got %q", check.Status)
	}
}

// --- invariant.opencode.permission_bash ---

func TestVerifyOpenCode_PermissionBash_FailsWhenBashWildcardAllowFalse(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.BashWildcardAllow = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.permission_bash")
	if check == nil {
		t.Fatal("expected invariant.opencode.permission_bash check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_PermissionBash_PassesWhenBashWildcardAllowTrue(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.permission_bash")
	if check == nil {
		t.Fatal("expected invariant.opencode.permission_bash check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// --- invariant.opencode.permission_read_secrets ---

func TestVerifyOpenCode_PermissionReadSecrets_FailsWhenReadSecretDeniesFalse(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.ReadSecretDenies = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.permission_read_secrets")
	if check == nil {
		t.Fatal("expected invariant.opencode.permission_read_secrets check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_PermissionReadSecrets_PassesWhenReadSecretDeniesTrue(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.permission_read_secrets")
	if check == nil {
		t.Fatal("expected invariant.opencode.permission_read_secrets check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// --- invariant.opencode.plugin_hive ---

func TestVerifyOpenCode_PluginHive_FailsWhenPluginHiveExistsFalse(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.PluginHiveExists = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.plugin_hive")
	if check == nil {
		t.Fatal("expected invariant.opencode.plugin_hive check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
}

func TestVerifyOpenCode_PluginHive_PassesWhenPluginHiveExistsTrue(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.plugin_hive")
	if check == nil {
		t.Fatal("expected invariant.opencode.plugin_hive check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// ---------------------------------------------------------------------------
// Phase 8: Warning-severity check keys
// ---------------------------------------------------------------------------

// --- invariant.opencode.mcp_hive ---

func TestVerifyOpenCode_MCPHive_EmitsWarnWhenNotPresent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.MCPHivePresent = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.mcp_hive")
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_hive check")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected StatusWarn (not fail), got %q", check.Status)
	}
}

// TestVerifyOpenCode_MCPHive_WarnDoesNotCauseOverallFail asserts that a missing
// mcp.hive entry (warn-only) does not push the report status to StatusFail, and
// that the warn check is actually present in the report with StatusWarn.
func TestVerifyOpenCode_MCPHive_WarnDoesNotCauseOverallFail(t *testing.T) {
	const checkKey = "invariant.opencode.mcp_hive"
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.MCPHivePresent = false

	report := Verify("opencode", observed)

	if report.Status == StatusFail {
		t.Fatalf("mcp_hive warning must not cause overall StatusFail, got %q", report.Status)
	}
	if report.Status != StatusWarn {
		t.Fatalf("expected overall StatusWarn when only %s warns, got %q", checkKey, report.Status)
	}

	check := findCheckByKey(report.Checks, checkKey)
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_hive check to be present in report")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected invariant.opencode.mcp_hive StatusWarn, got %q", check.Status)
	}
}

func TestVerifyOpenCode_MCPHive_PassesWhenPresent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.mcp_hive")
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_hive check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// --- invariant.opencode.mcp_context7 ---

func TestVerifyOpenCode_MCPContext7_EmitsWarnWhenNotPresent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.MCPContext7Present = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.mcp_context7")
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_context7 check")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected StatusWarn (not fail), got %q", check.Status)
	}
}

func TestVerifyOpenCode_MCPContext7_WarnDoesNotCauseOverallFail(t *testing.T) {
	const checkKey = "invariant.opencode.mcp_context7"
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.MCPContext7Present = false

	report := Verify("opencode", observed)

	if report.Status == StatusFail {
		t.Fatalf("mcp_context7 warning must not cause overall StatusFail, got %q", report.Status)
	}
	if report.Status != StatusWarn {
		t.Fatalf("expected overall StatusWarn when only %s warns, got %q", checkKey, report.Status)
	}

	check := findCheckByKey(report.Checks, checkKey)
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_context7 check to be present in report")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected invariant.opencode.mcp_context7 StatusWarn, got %q", check.Status)
	}
}

func TestVerifyOpenCode_MCPContext7_PassesWhenPresent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.mcp_context7")
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_context7 check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q", check.Status)
	}
}

// ---------------------------------------------------------------------------
// Phase 9: Agent-gate + Claude regression
// ---------------------------------------------------------------------------

// TestVerifyOpenCode_AgentGate_AllChecksGatedOnOpenCodeAgent confirms that
// every invariant.opencode.* check is absent when agent is "claude".
func TestVerifyOpenCode_AgentGate_AllOpenCodeChecksAbsentForClaudeAgent(t *testing.T) {
	// Populate an observed with all OpenCode fields in a FAILING state.
	// If the gate works, none of the checks should appear.
	observed := compliantObservedRuntime(t)
	observed.OpenCode = ObservedOpenCodeConfig{
		ParseSucceeded:     false,
		ShareMode:          "enabled",
		DefaultAgent:       "wrong",
		OrchestratorMode:   "subagent",
		OrchestratorModel:  "",
		OrchestratorPrompt: "",
		HiddenSubagents:    nil,
		TaskAllows:         nil,
		TaskWildcardDeny:   false,
		BashWildcardAllow:  false,
		ReadSecretDenies:   false,
		MCPHivePresent:     false,
		MCPContext7Present: false,
		PluginHiveExists:   false,
	}

	report := Verify("claude", observed)

	for _, c := range report.Checks {
		if strings.HasPrefix(c.Key, "invariant.opencode.") {
			t.Errorf("check %q must not appear for claude agent", c.Key)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 10: Integration — all checks pass together on compliant config
// ---------------------------------------------------------------------------

// TestVerifyOpenCode_AllChecksPassForCompliantConfig asserts that a fully
// compliant opencode config produces pass status for every invariant.opencode.*
// check key (none fail, none warn).
func TestVerifyOpenCode_AllChecksPassForCompliantConfig(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)

	report := Verify("opencode", observed)

	for _, c := range report.Checks {
		if !strings.HasPrefix(c.Key, "invariant.opencode.") {
			continue
		}
		if c.Status != StatusPass {
			t.Errorf("check %q expected StatusPass, got %q (message: %s)", c.Key, c.Status, c.Message)
		}
	}
}
