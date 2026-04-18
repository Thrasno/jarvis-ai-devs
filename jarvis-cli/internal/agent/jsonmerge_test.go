package agent

import (
	"encoding/json"
	"testing"
)

func TestMergeJSON(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		patch   string
		check   func(t *testing.T, result map[string]any)
		wantErr bool
	}{
		{
			name:  "empty base gets patch applied",
			base:  `{}`,
			patch: `{"mcpServers": {"hive": {"command": "/bin/bash", "args": []}}}`,
			check: func(t *testing.T, result map[string]any) {
				mcp, ok := result["mcpServers"].(map[string]any)
				if !ok {
					t.Fatal("expected mcpServers to be an object")
				}
				if _, ok := mcp["hive"]; !ok {
					t.Fatal("expected hive key in mcpServers")
				}
			},
		},
		{
			name: "add hive MCP to existing config preserving other keys",
			base: `{
				"outputStyle": "stark",
				"mcpServers": {
					"engram": {"command": "/home/user/go/bin/engram", "args": ["mcp"]}
				}
			}`,
			patch: `{"mcpServers": {"hive": {"command": "/bin/bash", "args": ["~/.jarvis/start.sh"]}}}`,
			check: func(t *testing.T, result map[string]any) {
				// outputStyle must be preserved
				if result["outputStyle"] != "stark" {
					t.Errorf("expected outputStyle=stark, got %v", result["outputStyle"])
				}
				mcp, ok := result["mcpServers"].(map[string]any)
				if !ok {
					t.Fatal("expected mcpServers")
				}
				// engram must still exist
				if _, ok := mcp["engram"]; !ok {
					t.Fatal("engram entry was lost after merge")
				}
				// hive must be added
				if _, ok := mcp["hive"]; !ok {
					t.Fatal("hive entry was not added")
				}
			},
		},
		{
			name:  "hive key always overwritten by patch (Jarvis-owned)",
			base:  `{"mcpServers": {"hive": {"command": "OLD", "args": []}}}`,
			patch: `{"mcpServers": {"hive": {"command": "NEW", "args": ["updated"]}}}`,
			check: func(t *testing.T, result map[string]any) {
				mcp := result["mcpServers"].(map[string]any)
				hive := mcp["hive"].(map[string]any)
				if hive["command"] != "NEW" {
					t.Errorf("expected hive.command=NEW, got %v", hive["command"])
				}
			},
		},
		{
			name: "array merge preserves existing agents and appends new ones",
			base: `{
				"mcp": {
					"engram": {"command": ["/go/bin/engram"], "type": "local"}
				},
				"agent": [{"name": "gentleman", "mode": "primary"}]
			}`,
			patch: `{
				"mcp": {"hive": {"command": ["/go/bin/hive-daemon"], "type": "local"}},
				"agent": [{"name": "new-agent", "mode": "subagent"}]
			}`,
			check: func(t *testing.T, result map[string]any) {
				mcp := result["mcp"].(map[string]any)
				if _, ok := mcp["engram"]; !ok {
					t.Fatal("engram was lost")
				}
				if _, ok := mcp["hive"]; !ok {
					t.Fatal("hive was not added")
				}
				agents := result["agent"].([]any)
				if len(agents) < 2 {
					t.Fatalf("expected >= 2 agents, got %d", len(agents))
				}
			},
		},
		{
			name:  "Claude format (command string + args array) preserved",
			base:  `{"mcpServers": {"existing": {"command": "/bin/tool", "args": ["--flag"], "type": "stdio"}}}`,
			patch: `{"mcpServers": {"hive": {"command": "/bin/bash", "args": ["~/.jarvis/start.sh"], "type": "stdio"}}}`,
			check: func(t *testing.T, result map[string]any) {
				mcp := result["mcpServers"].(map[string]any)
				hive := mcp["hive"].(map[string]any)
				// command must be string for Claude format
				if _, ok := hive["command"].(string); !ok {
					t.Errorf("Claude format: expected command to be string, got %T", hive["command"])
				}
				// args must be array
				if _, ok := hive["args"].([]any); !ok {
					t.Errorf("Claude format: expected args to be array, got %T", hive["args"])
				}
			},
		},
		{
			name:  "OpenCode format (command array) preserved",
			base:  `{"mcp": {"existing": {"command": ["/bin/tool", "--flag"], "type": "local"}}}`,
			patch: `{"mcp": {"hive": {"command": ["/go/bin/hive-daemon"], "type": "local", "env": {"KEY": "val"}}}}`,
			check: func(t *testing.T, result map[string]any) {
				mcp := result["mcp"].(map[string]any)
				hive := mcp["hive"].(map[string]any)
				// command must be array for OpenCode format
				if _, ok := hive["command"].([]any); !ok {
					t.Errorf("OpenCode format: expected command to be array, got %T", hive["command"])
				}
			},
		},
		{
			name:    "corrupt base JSON returns error",
			base:    `{invalid json`,
			patch:   `{}`,
			wantErr: true,
		},
		{
			name:    "corrupt patch JSON returns error",
			base:    `{}`,
			patch:   `{invalid`,
			wantErr: true,
		},
		{
			name:  "empty base (nil) treated as empty object",
			base:  ``,
			patch: `{"key": "value"}`,
			check: func(t *testing.T, result map[string]any) {
				if result["key"] != "value" {
					t.Errorf("expected key=value, got %v", result["key"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := MergeJSON([]byte(tt.base), []byte(tt.patch))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var result map[string]any
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatalf("result is not valid JSON: %v", err)
			}

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// TestMergeJSON_Context7JarvisOwned verifies Context7 overwrites user config (jarvis-owned semantics).
// Spec: R4 — Context7 is always owned by Jarvis, like Hive.
// This test ensures that user's custom context7 config is COMPLETELY replaced, not deep-merged.
func TestMergeJSON_Context7JarvisOwned(t *testing.T) {
	base := `{"mcpServers": {"context7": {"command": "OLD_USER_COMMAND", "args": [], "customKey": "should-be-removed"}}}`
	patch := `{"mcpServers": {"context7": {"transport": "http", "url": "https://mcp.context7.com/mcp"}}}`

	out, err := MergeJSON([]byte(base), []byte(patch))
	if err != nil {
		t.Fatalf("MergeJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	mcp := result["mcpServers"].(map[string]any)
	context7 := mcp["context7"].(map[string]any)

	// Context7 must be FULLY overwritten (Jarvis-owned) — patch wins, no deep merge
	if context7["transport"] != "http" {
		t.Errorf("expected context7.transport=http (Jarvis-owned), got %v", context7["transport"])
	}

	if context7["url"] != "https://mcp.context7.com/mcp" {
		t.Errorf("expected context7.url=https://mcp.context7.com/mcp, got %v", context7["url"])
	}

	// CRITICAL: customKey from user config MUST be removed (full overwrite, not merge)
	if _, exists := context7["customKey"]; exists {
		t.Errorf("context7.customKey should NOT exist (Jarvis owns context7 completely), but found: %v", context7["customKey"])
	}
}

// TestMergeJSON_Context7JarvisOwned_OpenCodeFormat verifies Context7 ownership with OpenCode remote format.
// Triangulation: different format (remote URL) with different config structure.
func TestMergeJSON_Context7JarvisOwned_OpenCodeFormat(t *testing.T) {
	base := `{"mcp": {"context7": {"type": "local", "url": "http://old-endpoint.com", "enabled": false}}}`
	patch := `{"mcp": {"context7": {"type": "remote", "url": "https://mcp.context7.com/mcp", "enabled": true}}}`

	out, err := MergeJSON([]byte(base), []byte(patch))
	if err != nil {
		t.Fatalf("MergeJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	mcp := result["mcp"].(map[string]any)
	context7 := mcp["context7"].(map[string]any)

	// Context7 must be FULLY overwritten — new values from patch
	if context7["type"] != "remote" {
		t.Errorf("expected context7.type=remote, got %v", context7["type"])
	}

	if context7["url"] != "https://mcp.context7.com/mcp" {
		t.Errorf("expected context7.url=https://mcp.context7.com/mcp, got %v", context7["url"])
	}

	if context7["enabled"] != true {
		t.Errorf("expected context7.enabled=true, got %v", context7["enabled"])
	}
}
