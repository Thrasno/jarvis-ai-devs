package sddruntime

import "testing"

// TestObservedOpenCodeConfig_ZeroValueIsSafe asserts that the zero value of
// ObservedOpenCodeConfig has StructureValid == false and all slice fields nil/empty.
// This guards the Claude adapter regression: when OpenCode field is not populated
// (zero value), no false-positive checks fire.
func TestObservedOpenCodeConfig_ZeroValueIsSafe(t *testing.T) {
	var cfg ObservedOpenCodeConfig

	if cfg.StructureValid {
		t.Error("zero value StructureValid must be false")
	}
	if cfg.ShareMode != "" {
		t.Errorf("zero value ShareMode must be empty, got %q", cfg.ShareMode)
	}
	if cfg.DefaultAgent != "" {
		t.Errorf("zero value DefaultAgent must be empty, got %q", cfg.DefaultAgent)
	}
	if cfg.OrchestratorMode != "" {
		t.Errorf("zero value OrchestratorMode must be empty, got %q", cfg.OrchestratorMode)
	}
	if cfg.OrchestratorModel != "" {
		t.Errorf("zero value OrchestratorModel must be empty, got %q", cfg.OrchestratorModel)
	}
	if cfg.OrchestratorPrompt != "" {
		t.Errorf("zero value OrchestratorPrompt must be empty, got %q", cfg.OrchestratorPrompt)
	}
	if len(cfg.AgentNames) != 0 {
		t.Errorf("zero value AgentNames must be empty, got %v", cfg.AgentNames)
	}
	if len(cfg.HiddenSubagents) != 0 {
		t.Errorf("zero value HiddenSubagents must be empty, got %v", cfg.HiddenSubagents)
	}
	if len(cfg.TaskAllows) != 0 {
		t.Errorf("zero value TaskAllows must be empty, got %v", cfg.TaskAllows)
	}
	if cfg.TaskWildcardDeny {
		t.Error("zero value TaskWildcardDeny must be false")
	}
	if cfg.BashWildcardAllow {
		t.Error("zero value BashWildcardAllow must be false")
	}
	if cfg.ReadSecretDenies {
		t.Error("zero value ReadSecretDenies must be false")
	}
	if cfg.MCPHivePresent {
		t.Error("zero value MCPHivePresent must be false")
	}
	if cfg.MCPContext7Present {
		t.Error("zero value MCPContext7Present must be false")
	}
	if cfg.PluginHiveExists {
		t.Error("zero value PluginHiveExists must be false")
	}
}

// TestObservedRuntime_HasOpenCodeField asserts that ObservedRuntime carries an
// OpenCode field of type ObservedOpenCodeConfig and that its zero value is safe.
func TestObservedRuntime_HasOpenCodeField(t *testing.T) {
	var r ObservedRuntime

	// The field must exist and be addressable.
	cfg := r.OpenCode
	if cfg.StructureValid {
		t.Error("ObservedRuntime.OpenCode zero value must have StructureValid == false")
	}

	// Assigning a populated struct must work.
	r.OpenCode = ObservedOpenCodeConfig{
		StructureValid: true,
		ShareMode:      "disabled",
		DefaultAgent:   "sdd-orchestrator",
	}
	if !r.OpenCode.StructureValid {
		t.Error("assigned ObservedOpenCodeConfig not reflected on ObservedRuntime")
	}
	if r.OpenCode.ShareMode != "disabled" {
		t.Errorf("ShareMode = %q, want %q", r.OpenCode.ShareMode, "disabled")
	}
}
