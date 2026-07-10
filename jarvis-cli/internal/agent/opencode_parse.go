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
	Share        string                        `json:"share"`
	DefaultAgent string                        `json:"default_agent"`
	Permission   openCodeGlobalPermission      `json:"permission"`
	Agent        map[string]openCodeAgentEntry `json:"agent"`
	MCP          map[string]openCodeMCPEntry   `json:"mcp"`
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
	Enabled *bool           `json:"enabled"`
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
		cfg.OrchestratorMode = orch.Mode
		cfg.OrchestratorModel = orch.Model
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
	hiveGrantEvidence := make(map[string][]sddruntime.OpenCodePermissionEvidence)
	sddSubagents := make(map[string]struct{})
	for _, name := range openCodeSDDSubagents() {
		sddSubagents[name] = struct{}{}
	}
	for name, entry := range doc.Agent {
		agentNames = append(agentNames, name)
		if entry.Hidden && entry.Mode == "subagent" {
			hiddenSubagents = append(hiddenSubagents, name)
		}
		if _, ok := sddSubagents[name]; ok {
			evidence := parseOpenCodeHiveGrantEvidence(entry.Permission)
			if len(evidence) > 0 {
				hiveGrantEvidence[name] = evidence
			}
		}
	}
	sort.Strings(agentNames)
	sort.Strings(hiddenSubagents)
	cfg.AgentNames = agentNames
	cfg.HiddenSubagents = hiddenSubagents
	cfg.SDDSubagentHiveGrantEvidence = hiveGrantEvidence

	// Parse top-level bash permission.
	if doc.Permission.Bash != nil {
		cfg.BashWildcardAllow = doc.Permission.Bash["*"] == "allow"
	}

	// Parse top-level read permission for secret deny patterns.
	if doc.Permission.Read != nil {
		cfg.ReadSecretDenies = hasSecretDenyPatterns(doc.Permission.Read)
	}

	// Parse MCP entries.
	if hive, ok := doc.MCP["hive"]; ok && hive.Type == "local" && isOpenCodeMCPEnabled(hive.Enabled) {
		cfg.MCPHivePresent = isMCPCommandNonEmpty(hive.Command)
	}
	if ctx7, ok := doc.MCP["context7"]; ok && ctx7.Type == "remote" && isOpenCodeMCPEnabled(ctx7.Enabled) {
		cfg.MCPContext7Present = true
	}

	return cfg
}

func isOpenCodeMCPEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func parseOpenCodeHiveGrantEvidence(raw json.RawMessage) []sddruntime.OpenCodePermissionEvidence {
	if len(raw) == 0 {
		return nil
	}
	var permission map[string]any
	if err := json.Unmarshal(raw, &permission); err != nil {
		return nil
	}
	evidence := make([]sddruntime.OpenCodePermissionEvidence, 0)
	for key, value := range permission {
		action, ok := value.(string)
		if !ok || !isOpenCodePermissionActionEvidence(action) {
			continue
		}
		if isOpenCodeHiveGrantEvidence(key) {
			evidence = append(evidence, sddruntime.OpenCodePermissionEvidence{Key: key, Action: action})
		}
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].Key == evidence[j].Key {
			return evidence[i].Action < evidence[j].Action
		}
		return evidence[i].Key < evidence[j].Key
	})
	return evidence
}

func isOpenCodePermissionActionEvidence(action string) bool {
	return action == "allow" || action == "ask" || action == "deny"
}

func isOpenCodeHiveGrantEvidence(key string) bool {
	if key == "hive_*" || key == "hive_mem_*" {
		return true
	}
	for _, tool := range RequiredOpenCodeHiveMCPTools() {
		if key == tool {
			return true
		}
	}
	return false
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
// decodes to an array whose first command entry is non-empty/non-whitespace or a
// non-empty/non-whitespace string. Object command values are unusable for the
// local MCP command semantics Jarvis verifies.
func isMCPCommandNonEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	// Try array of strings first (most common format).
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr) > 0 && strings.TrimSpace(arr[0]) != ""
	}
	// Try plain string command.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s) != ""
	}
	return false
}
