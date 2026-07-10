package sddruntime

// ObservedOpenCodeConfig holds parsed state from opencode.json.
// It is nested on ObservedRuntime and populated only when the active agent is
// "opencode". The Claude adapter leaves this at its zero value, which is safe:
// ParseSucceeded==false and all slices nil — no invariant checks will fire.
type ObservedOpenCodeConfig struct {
	// ParseSucceeded is true only when opencode.json was found, non-empty,
	// and unmarshalled without error. It does NOT mean the config is semantically
	// valid — individual fields may still be zero. All other fields are meaningful
	// only when this is true.
	ParseSucceeded bool

	// ShareMode is the value of the top-level "share" key. Expected: "disabled".
	ShareMode string

	// DefaultAgent is the value of "default_agent". Expected: "sdd-orchestrator".
	DefaultAgent string

	// OrchestratorMode is the "mode" field of the sdd-orchestrator agent entry.
	// Expected: "primary".
	OrchestratorMode string

	// OrchestratorModel is the "model" field of the sdd-orchestrator agent entry.
	// Must be non-empty.
	OrchestratorModel string

	// OrchestratorPrompt is the "prompt" field of the sdd-orchestrator agent entry.
	// Must reference "sdd-orchestrator.md".
	OrchestratorPrompt string

	// AgentNames contains all keys found under the top-level "agent" map.
	AgentNames []string

	// HiddenSubagents contains the names of agent entries that have both
	// hidden==true and mode=="subagent".
	HiddenSubagents []string

	// TaskAllows contains the keys in orchestrator.permission.task whose value
	// is "allow". Expected to include every generated subagent that the
	// orchestrator may delegate to.
	TaskAllows []string

	// TaskWildcardDeny is true when orchestrator.permission.task["*"] == "deny".
	TaskWildcardDeny bool

	// BashWildcardAllow is true when orchestrator.permission.bash["*"] == "allow".
	BashWildcardAllow bool

	// ReadSecretDenies is true when permission.read contains at least one deny
	// pattern covering secrets/credentials paths (.env, tokens, keys, secrets).
	ReadSecretDenies bool

	// MCPHivePresent is true when mcp.hive.type == "local" and command is non-empty.
	MCPHivePresent bool

	// MCPContext7Present is true when mcp.context7.type == "remote".
	MCPContext7Present bool

	// PluginHiveExists is true when plugins/hive.ts exists and is non-empty.
	// This field is populated from the prompt_hook artifact observation, not from JSON.
	PluginHiveExists bool

	// SDDSubagentHiveGrantEvidence maps each generated SDD subagent name to the
	// exact permission entries or wildcard permission patterns that affect Hive
	// MCP tool access, for example hive_mem_search, hive_mem_*, or hive_*.
	// Both allow and stricter ask/deny entries are retained so verification can
	// conservatively evaluate effective access after user-owned merge overrides.
	SDDSubagentHiveGrantEvidence map[string][]OpenCodePermissionEvidence
}

// OpenCodePermissionEvidence records one permission entry that can affect Hive
// MCP access for a generated OpenCode SDD subagent.
type OpenCodePermissionEvidence struct {
	Key    string
	Action string
}
