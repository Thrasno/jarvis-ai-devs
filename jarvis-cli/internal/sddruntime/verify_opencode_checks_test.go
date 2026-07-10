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

func TestVerifyOpenCode_SubagentsPresent_AllowsExtraUserOwnedSubagents(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.HiddenSubagents = append(observed.OpenCode.HiddenSubagents, "extra-agent")

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.subagents_present")
	if check == nil {
		t.Fatal("expected invariant.opencode.subagents_present check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass with extra user-owned subagent, got %q", check.Status)
	}
}

func TestVerifyOpenCode_SubagentsPresent_FailsWhen17WrongNames(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.HiddenSubagents = []string{
		"wrong-01", "wrong-02", "wrong-03", "wrong-04", "wrong-05", "wrong-06",
		"wrong-07", "wrong-08", "wrong-09", "wrong-10", "wrong-11", "wrong-12",
		"wrong-13", "wrong-14", "wrong-15", "wrong-16", "wrong-17",
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.subagents_present")
	if check == nil {
		t.Fatal("expected invariant.opencode.subagents_present check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail for wrong subagent names, got %q", check.Status)
	}
	if !strings.Contains(check.Observed, "missing=sdd-init") || !strings.Contains(check.Observed, "unexpected=wrong-01") {
		t.Fatalf("expected useful missing/unexpected diagnostic, got observed=%q message=%q", check.Observed, check.Message)
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

func TestVerifyOpenCode_TaskAllowlist_AllowsExtraUserOwnedTaskAllows(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.TaskAllows = append(observed.OpenCode.TaskAllows, "extra-task")

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.task_allowlist")
	if check == nil {
		t.Fatal("expected invariant.opencode.task_allowlist check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass with extra user-owned task allow, got %q", check.Status)
	}
}

func TestVerifyOpenCode_TaskAllowlist_FailsWhen17WrongNames(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.TaskAllows = []string{
		"wrong-01", "wrong-02", "wrong-03", "wrong-04", "wrong-05", "wrong-06",
		"wrong-07", "wrong-08", "wrong-09", "wrong-10", "wrong-11", "wrong-12",
		"wrong-13", "wrong-14", "wrong-15", "wrong-16", "wrong-17",
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.task_allowlist")
	if check == nil {
		t.Fatal("expected invariant.opencode.task_allowlist check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail for wrong task allow names, got %q", check.Status)
	}
	if !strings.Contains(check.Observed, "missing=sdd-init") || !strings.Contains(check.Observed, "unexpected=wrong-01") {
		t.Fatalf("expected useful missing/unexpected diagnostic, got observed=%q message=%q", check.Observed, check.Message)
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

func TestVerifyOpenCode_SDDSubagentHiveGrants_FailsWhenGrantMissing(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_mem_search", Action: "allow"},
		{Key: "hive_mem_save", Action: "allow"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail, got %q", check.Status)
	}
	if check.DriftClass != DriftOwned {
		t.Fatalf("expected owned generated-artifact drift, got %q", check.DriftClass)
	}
	if !strings.Contains(check.Message, "jarvis init") {
		t.Fatalf("missing regeneration guidance in message: %s", check.Message)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_WarnsWhenGrantMissingInOpenSpecMode(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.StoreMode = "openspec"
	observed.StoreReadFrom = []string{"openspec"}
	observed.StoreWriteTo = []string{"openspec"}
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_mem_search", Action: "allow"},
		{Key: "hive_mem_save", Action: "allow"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected StatusWarn for non-Hive advisory drift, got %q", check.Status)
	}
	if check.DriftClass != DriftOwned {
		t.Fatalf("expected owned generated-artifact drift, got %q", check.DriftClass)
	}
	if report.Status == StatusFail {
		t.Fatalf("missing Hive grants must not fail openspec verification, got %q", report.Status)
	}
	if !strings.Contains(check.Message, "advisory") {
		t.Fatalf("expected advisory message for non-Hive mode, got %q", check.Message)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_WarnsWhenGrantMissingInNoneMode(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.StoreMode = "none"
	observed.StoreReadFrom = nil
	observed.StoreWriteTo = nil
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_mem_search", Action: "allow"},
		{Key: "hive_mem_save", Action: "allow"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected StatusWarn for none-mode advisory drift, got %q", check.Status)
	}
	if report.Status == StatusFail {
		t.Fatalf("missing Hive grants must not fail none-mode verification, got %q", report.Status)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_FailsWhenGrantMissingInHiveMode(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.StoreMode = "hive"
	observed.StoreReadFrom = []string{"hive"}
	observed.StoreWriteTo = []string{"hive"}
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_mem_search", Action: "allow"},
		{Key: "hive_mem_save", Action: "allow"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail for Hive mode, got %q", check.Status)
	}
	if report.Status != StatusFail {
		t.Fatalf("expected overall StatusFail for Hive mode, got %q", report.Status)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_PassesWithWildcardEvidence(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	for _, name := range observed.OpenCode.HiddenSubagents[:10] {
		observed.OpenCode.SDDSubagentHiveGrantEvidence[name] = []OpenCodePermissionEvidence{{Key: "hive_mem_*", Action: "allow"}}
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q (message: %s)", check.Status, check.Message)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_FailsWhenExactDenyOverridesWildcardAllow(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_mem_*", Action: "allow"},
		{Key: "hive_mem_save", Action: "deny"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail when exact deny overrides wildcard allow, got %q", check.Status)
	}
	if !strings.Contains(check.Observed, "sdd-apply:hive_mem_save") {
		t.Fatalf("expected observed drift for denied tool, got %q", check.Observed)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_FailsWhenExactAskOverridesWildcardAllow(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_*", Action: "allow"},
		{Key: "hive_mem_get_observation", Action: "ask"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail when exact ask overrides wildcard allow, got %q", check.Status)
	}
	if !strings.Contains(check.Observed, "sdd-apply:hive_mem_get_observation") {
		t.Fatalf("expected observed drift for ask-gated tool, got %q", check.Observed)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_PassesWhenExactAllowsFollowStrictWildcard(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
		{Key: "hive_mem_*", Action: "deny"},
		{Key: "hive_mem_search", Action: "allow"},
		{Key: "hive_mem_get_observation", Action: "allow"},
		{Key: "hive_mem_save", Action: "allow"},
		{Key: "hive_mem_context", Action: "allow"},
		{Key: "hive_mem_session_summary", Action: "allow"},
	}

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != StatusPass {
		t.Fatalf("expected StatusPass when exact allows follow strict wildcard, got %q (message: %s)", check.Status, check.Message)
	}
}

func TestVerifyOpenCode_SDDSubagentHiveGrants_FailsWhenStrictWildcardFollowsGeneratedExactAllows(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "deny", action: "deny"},
		{name: "ask", action: "ask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantOpenCodeRuntime(t)
			observed.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"] = []OpenCodePermissionEvidence{
				{Key: "hive_mem_search", Action: "allow"},
				{Key: "hive_mem_get_observation", Action: "allow"},
				{Key: "hive_mem_save", Action: "allow"},
				{Key: "hive_mem_context", Action: "allow"},
				{Key: "hive_mem_session_summary", Action: "allow"},
				{Key: "hive_mem_*", Action: tt.action},
			}

			report := Verify("opencode", observed)

			check := findCheckByKey(report.Checks, "invariant.opencode.sdd_hive_grants")
			if check == nil {
				t.Fatal("expected invariant.opencode.sdd_hive_grants check")
			}
			if check.Status != StatusFail {
				t.Fatalf("expected StatusFail when strict wildcard %s follows generated exact allows, got %q", tt.action, check.Status)
			}
			if check.DriftClass != DriftNonOwned {
				t.Fatalf("expected strict wildcard guardrail to be classified as non-owned drift, got %q", check.DriftClass)
			}
			if strings.Contains(check.Message, "jarvis init") || !strings.Contains(check.Message, "manually adjust") {
				t.Fatalf("expected manual guardrail remediation, got %q", check.Message)
			}
			if !strings.Contains(check.Observed, "sdd-apply:hive_mem_search") {
				t.Fatalf("expected observed drift for wildcard-%s tool, got %q", tt.action, check.Observed)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 8: Warning-severity check keys
// ---------------------------------------------------------------------------

// --- invariant.opencode.mcp_hive ---

func TestVerifyOpenCode_MCPHive_EmitsWarnWhenNotPresent(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.StoreMode = string(StoreModeOpenSpec)
	observed.StoreReadFrom = []string{"openspec"}
	observed.StoreWriteTo = []string{"openspec"}
	observed.OpenCode.MCPHivePresent = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.mcp_hive")
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_hive check")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected StatusWarn (not fail), got %q", check.Status)
	}
	if !strings.Contains(check.Message, "Hive artifact persistence is not required for openspec or none modes") {
		t.Fatalf("expected non-Hive mode scope in warning message, got %q", check.Message)
	}
}

func TestVerifyOpenCode_MCPHive_FailsWhenNotPresentInHiveBackedModes(t *testing.T) {
	tests := []struct {
		name      string
		storeMode string
		readFrom  []string
		writeTo   []string
	}{
		{
			name:      "hive",
			storeMode: string(StoreModeHive),
			readFrom:  []string{"hive"},
			writeTo:   []string{"hive"},
		},
		{
			name:      "hybrid",
			storeMode: string(StoreModeHybrid),
			readFrom:  []string{"hive", "openspec"},
			writeTo:   []string{"hive", "openspec"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantOpenCodeRuntime(t)
			observed.StoreMode = tt.storeMode
			observed.StoreReadFrom = tt.readFrom
			observed.StoreWriteTo = tt.writeTo
			observed.OpenCode.MCPHivePresent = false

			report := Verify("opencode", observed)

			check := findCheckByKey(report.Checks, "invariant.opencode.mcp_hive")
			if check == nil {
				t.Fatal("expected invariant.opencode.mcp_hive check")
			}
			if check.Status != StatusFail {
				t.Fatalf("expected StatusFail for missing mcp.hive in %s mode, got %q", tt.storeMode, check.Status)
			}
			if !strings.Contains(check.Message, "Hive/hybrid mode requires top-level mcp.hive") {
				t.Fatalf("expected Hive/hybrid blocking message, got %q", check.Message)
			}
			if report.Status != StatusFail {
				t.Fatalf("expected overall StatusFail for missing mcp.hive in %s mode, got %q", tt.storeMode, report.Status)
			}
		})
	}
}

func TestVerifyOpenCode_MCPHive_FailsClearlyWhenParsedCommandIsUnusable(t *testing.T) {
	observed := compliantOpenCodeRuntime(t)
	observed.StoreMode = string(StoreModeHive)
	observed.StoreReadFrom = []string{"hive"}
	observed.StoreWriteTo = []string{"hive"}
	observed.OpenCode.MCPHivePresent = false

	report := Verify("opencode", observed)

	check := findCheckByKey(report.Checks, "invariant.opencode.mcp_hive")
	if check == nil {
		t.Fatal("expected invariant.opencode.mcp_hive check")
	}
	if check.Status != StatusFail {
		t.Fatalf("expected StatusFail for unusable parsed mcp.hive command in Hive mode, got %q", check.Status)
	}
	if !strings.Contains(check.Message, "Hive/hybrid mode requires top-level mcp.hive") {
		t.Fatalf("expected clear Hive/hybrid remediation message, got %q", check.Message)
	}
	if !strings.Contains(check.Expected, `non-empty command`) {
		t.Fatalf("expected check to require a non-empty command, got %q", check.Expected)
	}
}

func TestVerifyOpenCode_MCPHive_WarnsWhenNotPresentInNonHiveModes(t *testing.T) {
	tests := []struct {
		name      string
		storeMode string
		readFrom  []string
		writeTo   []string
	}{
		{
			name:      "openspec",
			storeMode: string(StoreModeOpenSpec),
			readFrom:  []string{"openspec"},
			writeTo:   []string{"openspec"},
		},
		{
			name:      "none",
			storeMode: string(StoreModeNone),
			readFrom:  nil,
			writeTo:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantOpenCodeRuntime(t)
			observed.StoreMode = tt.storeMode
			observed.StoreReadFrom = tt.readFrom
			observed.StoreWriteTo = tt.writeTo
			observed.OpenCode.MCPHivePresent = false

			report := Verify("opencode", observed)

			check := findCheckByKey(report.Checks, "invariant.opencode.mcp_hive")
			if check == nil {
				t.Fatal("expected invariant.opencode.mcp_hive check")
			}
			if check.Status != StatusWarn {
				t.Fatalf("expected StatusWarn for missing mcp.hive in %s mode, got %q", tt.storeMode, check.Status)
			}
			if !strings.Contains(check.Message, "Hive artifact persistence is not required for openspec or none modes") {
				t.Fatalf("expected non-Hive advisory message, got %q", check.Message)
			}
			if report.Status == StatusFail {
				t.Fatalf("missing mcp.hive warning must not cause overall StatusFail in %s mode", tt.storeMode)
			}
		})
	}
}

// TestVerifyOpenCode_MCPHive_WarnDoesNotCauseOverallFail asserts that a missing
// mcp.hive entry (warn-only) does not push the report status to StatusFail, and
// that the warn check is actually present in the report with StatusWarn.
func TestVerifyOpenCode_MCPHive_WarnDoesNotCauseOverallFail(t *testing.T) {
	const checkKey = "invariant.opencode.mcp_hive"
	observed := compliantOpenCodeRuntime(t)
	observed.StoreMode = string(StoreModeOpenSpec)
	observed.StoreReadFrom = []string{"openspec"}
	observed.StoreWriteTo = []string{"openspec"}
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
