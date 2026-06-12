package agent

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// openCodeConfigDoc mirrors only the fields from opencode.json that the
// verifier cares about. Unknown keys are silently ignored.
type openCodeConfigDoc struct {
	Share        string                          `json:"share"`
	DefaultAgent string                          `json:"default_agent"`
	Permission   openCodeGlobalPermission        `json:"permission"`
	Agent        map[string]openCodeAgentEntry   `json:"agent"`
	MCP          map[string]openCodeMCPEntry     `json:"mcp"`
}

type openCodeGlobalPermission struct {
	Bash map[string]string `json:"bash"`
	Read map[string]string `json:"read"`
}

type openCodeAgentEntry struct {
	Mode       string                    `json:"mode"`
	Hidden     bool                      `json:"hidden"`
	Model      string                    `json:"model"`
	Prompt     string                    `json:"prompt"`
	Permission *openCodeAgentPermission  `json:"permission,omitempty"`
}

type openCodeAgentPermission struct {
	Task map[string]string `json:"task"`
}

type openCodeMCPEntry struct {
	Type    string          `json:"type"`
	URL     string          `json:"url"`
	Command json.RawMessage `json:"command"`
}

// parseOpenCodeConfig reads and parses opencode.json at path, returning an
// ObservedOpenCodeConfig. On any error (file not found, empty, malformed JSON),
// it returns a zero-value struct with StructureValid==false. Errors are never
// propagated — callers treat parse failure as incomplete observation only.
func parseOpenCodeConfig(path string) sddruntime.ObservedOpenCodeConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return sddruntime.ObservedOpenCodeConfig{}
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return sddruntime.ObservedOpenCodeConfig{}
	}

	var doc openCodeConfigDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return sddruntime.ObservedOpenCodeConfig{}
	}

	cfg := sddruntime.ObservedOpenCodeConfig{
		StructureValid: true,
		ShareMode:      doc.Share,
		DefaultAgent:   doc.DefaultAgent,
	}

	// Parse orchestrator entry.
	if orch, ok := doc.Agent["sdd-orchestrator"]; ok {
		cfg.OrchestratorMode   = orch.Mode
		cfg.OrchestratorModel  = orch.Model
		cfg.OrchestratorPrompt = orch.Prompt

		if orch.Permission != nil {
			cfg.TaskWildcardDeny = orch.Permission.Task["*"] == "deny"
			allows := make([]string, 0)
			for k, v := range orch.Permission.Task {
				if k != "*" && v == "allow" {
					allows = append(allows, k)
				}
			}
			sort.Strings(allows)
			cfg.TaskAllows = allows
		}
	}

	// Collect all agent names and hidden subagents.
	agentNames := make([]string, 0, len(doc.Agent))
	hiddenSubagents := make([]string, 0)
	for name, entry := range doc.Agent {
		agentNames = append(agentNames, name)
		if entry.Hidden && entry.Mode == "subagent" {
			hiddenSubagents = append(hiddenSubagents, name)
		}
	}
	sort.Strings(agentNames)
	sort.Strings(hiddenSubagents)
	cfg.AgentNames      = agentNames
	cfg.HiddenSubagents = hiddenSubagents

	// Parse top-level bash permission.
	if doc.Permission.Bash != nil {
		cfg.BashWildcardAllow = doc.Permission.Bash["*"] == "allow"
	}

	// Parse top-level read permission for secret deny patterns.
	if doc.Permission.Read != nil {
		cfg.ReadSecretDenies = hasSecretDenyPatterns(doc.Permission.Read)
	}

	// Parse MCP entries.
	if hive, ok := doc.MCP["hive"]; ok && hive.Type == "local" {
		cfg.MCPHivePresent = isMCPCommandNonEmpty(hive.Command)
	}
	if ctx7, ok := doc.MCP["context7"]; ok && ctx7.Type == "remote" {
		cfg.MCPContext7Present = true
	}

	return cfg
}

// hasSecretDenyPatterns returns true when the read permission map contains at
// least one deny entry matching a secrets/credentials/env pattern.
func hasSecretDenyPatterns(read map[string]string) bool {
	secretKeywords := []string{".env", "secret", "token", "credential", "key"}
	for pattern, action := range read {
		if action != "deny" {
			continue
		}
		lower := strings.ToLower(pattern)
		for _, kw := range secretKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// isMCPCommandNonEmpty returns true when the raw JSON for the command field
// decodes to a non-empty array of strings.
func isMCPCommandNonEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		// Could be a plain string command — treat as non-empty.
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			return strings.TrimSpace(s) != ""
		}
		return false
	}
	return len(arr) > 0
}
