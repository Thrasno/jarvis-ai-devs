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
	Mode       string          `json:"mode"`
	Hidden     bool            `json:"hidden"`
	Model      string          `json:"model"`
	Prompt     string          `json:"prompt"`
	Permission json.RawMessage `json:"permission,omitempty"`
}

// openCodeAgentPermission is the orchestrator-specific permission shape where
// "task" is a map (wildcard deny + named allows). Subagents use a different
// shape ("task":"deny" as a string), so we unmarshal the orchestrator permission
// separately rather than embedding this type in openCodeAgentEntry.
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
// it returns a zero-value struct with ParseSucceeded==false. Errors are never
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
		ParseSucceeded: true,
		ShareMode:      doc.Share,
		DefaultAgent:   doc.DefaultAgent,
	}

	// Parse orchestrator entry.
	if orch, ok := doc.Agent["sdd-orchestrator"]; ok {
		cfg.OrchestratorMode   = orch.Mode
		cfg.OrchestratorModel  = orch.Model
		cfg.OrchestratorPrompt = orch.Prompt

		// The orchestrator's permission.task is a map (wildcard deny + named allows).
		// Subagents use task as a plain string — unmarshal the orchestrator permission
		// separately to avoid type conflicts.
		if len(orch.Permission) > 0 {
			var orchPerm openCodeAgentPermission
			if err := json.Unmarshal(orch.Permission, &orchPerm); err == nil && orchPerm.Task != nil {
				cfg.TaskWildcardDeny = orchPerm.Task["*"] == "deny"
				allows := make([]string, 0)
				for k, v := range orchPerm.Task {
					if k != "*" && v == "allow" {
						allows = append(allows, k)
					}
				}
				sort.Strings(allows)
				cfg.TaskAllows = allows
			}
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
// decodes to a non-empty array, non-empty/non-whitespace string, or non-empty
// object. Returns false for null, empty array, empty/whitespace-only string,
// empty object, or any other value.
func isMCPCommandNonEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	// Try array of strings first (most common format).
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr) > 0
	}
	// Try plain string command.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s) != ""
	}
	// Try object format (e.g. {"bin": "hive-daemon", "args": []}).
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return len(obj) > 0
	}
	return false
}
