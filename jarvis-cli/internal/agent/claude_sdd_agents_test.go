package agent

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

func TestRenderClaudeSDDPhaseAgents_GeneratesAllPhaseWrappersWithContracts(t *testing.T) {
	models := defaultRuntimePhaseModels()
	models.Claude = map[string]state.ClaudeModelAssignment{
		"sdd-design": {Model: "opus", Effort: "high"},
	}

	files, err := RenderClaudeSDDPhaseAgents(testTemplatesFS, models)
	if err != nil {
		t.Fatalf("RenderClaudeSDDPhaseAgents: %v", err)
	}

	wantNames := []string{
		"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	}
	if len(files) != len(wantNames) {
		t.Fatalf("rendered files = %d, want %d: %#v", len(files), len(wantNames), files)
	}
	for _, name := range wantNames {
		content, ok := files[name+".md"]
		if !ok {
			t.Fatalf("missing generated Claude SDD agent %q", name+".md")
		}
		text := string(content)
		for _, required := range []string{
			"name: " + name,
			".jarvis/skills/" + name + "/SKILL.md",
			"Generated technical artifacts default to English",
			"Result Contract",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing %q:\n%s", name, required, text)
			}
		}
		if strings.Contains(text, "## TDD Philosophy") || strings.Contains(text, "## Artifact Retrieval") {
			t.Fatalf("%s duplicated full skill/shared procedure body:\n%s", name, text)
		}
	}

	design := string(files["sdd-design.md"])
	if !strings.Contains(design, "model: opus") || !strings.Contains(design, "effort: high") {
		t.Fatalf("sdd-design did not use configured Claude route:\n%s", design)
	}

	embeddedFiles, err := RenderClaudeSDDPhaseAgents(jarvis.TemplatesFS, models)
	if err != nil {
		t.Fatalf("RenderClaudeSDDPhaseAgents with embedded template: %v", err)
	}
	embeddedDesign := string(embeddedFiles["sdd-design.md"])
	if !strings.Contains(embeddedDesign, ".jarvis/skills/sdd-design/SKILL.md") || !strings.Contains(embeddedDesign, "Result Contract") {
		t.Fatalf("embedded template missing Claude SDD wrapper contract:\n%s", embeddedDesign)
	}
}

func TestRenderClaudeSDDPhaseAgents_UsesPhaseSpecificToolBoundaries(t *testing.T) {
	files, err := RenderClaudeSDDPhaseAgents(testTemplatesFS, defaultRuntimePhaseModels())
	if err != nil {
		t.Fatalf("RenderClaudeSDDPhaseAgents: %v", err)
	}

	for _, name := range []string{"sdd-explore", "sdd-verify", "sdd-onboard"} {
		content := string(files[name+".md"])
		tools := frontmatterValue(content, "tools")
		assertClaudeSDDAgentHasHiveTools(t, name, tools)
		if strings.Contains(tools, "Edit") || strings.Contains(tools, "MultiEdit") || strings.Contains(tools, "Write") {
			t.Fatalf("%s tools include write capability, want read-only boundary: %q", name, tools)
		}
	}

	for _, name := range []string{"sdd-init", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-archive"} {
		content := string(files[name+".md"])
		tools := frontmatterValue(content, "tools")
		assertClaudeSDDAgentHasHiveTools(t, name, tools)
		for _, tool := range []string{"Edit", "MultiEdit", "Write"} {
			if !strings.Contains(tools, tool) {
				t.Fatalf("%s tools = %q, want %s capability", name, tools, tool)
			}
		}
	}
}

func TestRequiredHiveMCPToolRequirements_ExposePlatformSpecificNames(t *testing.T) {
	want := []HiveMCPToolRequirement{
		{LogicalName: "mem_search", ClaudeTool: "mcp__hive__mem_search", OpenCodeTool: "hive_mem_search"},
		{LogicalName: "mem_get_observation", ClaudeTool: "mcp__hive__mem_get_observation", OpenCodeTool: "hive_mem_get_observation"},
		{LogicalName: "mem_save", ClaudeTool: "mcp__hive__mem_save", OpenCodeTool: "hive_mem_save"},
		{LogicalName: "mem_context", ClaudeTool: "mcp__hive__mem_context", OpenCodeTool: "hive_mem_context"},
		{LogicalName: "mem_session_summary", ClaudeTool: "mcp__hive__mem_session_summary", OpenCodeTool: "hive_mem_session_summary"},
	}

	got := RequiredHiveMCPToolRequirements()
	if len(got) != len(want) {
		t.Fatalf("RequiredHiveMCPToolRequirements() returned %d tools, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RequiredHiveMCPToolRequirements()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func assertClaudeSDDAgentHasHiveTools(t *testing.T, name, tools string) {
	t.Helper()
	for _, tool := range []string{
		"mcp__hive__mem_search",
		"mcp__hive__mem_get_observation",
		"mcp__hive__mem_save",
		"mcp__hive__mem_context",
		"mcp__hive__mem_session_summary",
	} {
		if !strings.Contains(tools, tool) {
			t.Fatalf("%s tools = %q, want Hive MCP tool %s", name, tools, tool)
		}
	}
}

func TestClaudeAgent_InstallSDDPhaseAgents_OverwritesManagedNamesAndPreservesOthers(t *testing.T) {
	home := isolateTestHome(t)
	agent := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	agentsDir := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	managed := filepath.Join(agentsDir, "sdd-design.md")
	foreign := filepath.Join(agentsDir, "sdd-custom.md")
	if err := os.WriteFile(managed, []byte("old managed"), 0644); err != nil {
		t.Fatalf("write old managed agent: %v", err)
	}
	if err := os.WriteFile(foreign, []byte("user custom"), 0644); err != nil {
		t.Fatalf("write custom agent: %v", err)
	}

	models := defaultRuntimePhaseModels()
	models.Claude = map[string]state.ClaudeModelAssignment{
		"sdd-design": {Model: "sonnet", Effort: "max"},
	}
	if err := agent.InstallSDDPhaseAgents(models); err != nil {
		t.Fatalf("InstallSDDPhaseAgents: %v", err)
	}

	updated, err := os.ReadFile(managed)
	if err != nil {
		t.Fatalf("read managed agent: %v", err)
	}
	if !strings.Contains(string(updated), "name: sdd-design") || !strings.Contains(string(updated), "effort: max") {
		t.Fatalf("managed SDD agent was not overwritten with rendered content:\n%s", updated)
	}
	preserved, err := os.ReadFile(foreign)
	if err != nil {
		t.Fatalf("read custom agent: %v", err)
	}
	if string(preserved) != "user custom" {
		t.Fatalf("custom sdd file was modified: %q", preserved)
	}
}

func TestClaudeStaticAndOpenCodeSDDAgentsRemainAvailable(t *testing.T) {
	agentsFS, err := fs.Sub(jarvis.AgentsFS, "embed/agents/claude")
	if err != nil {
		t.Fatalf("sub AgentsFS: %v", err)
	}
	for _, name := range []string{
		"jd-judge-a.md", "jd-judge-b.md", "jd-fix-agent.md",
		"review-risk.md", "review-readability.md", "review-reliability.md", "review-resilience.md",
	} {
		if _, err := fs.Stat(agentsFS, name); err != nil {
			t.Fatalf("static Claude agent %q missing: %v", name, err)
		}
	}

	agents := buildOpenCodeGeneratedAgents(nil, nil)
	byName := make(map[string]opencodeGeneratedAgent, len(agents))
	for _, agent := range agents {
		byName[agent.Name] = agent
	}
	for _, name := range openCodeSDDSubagents() {
		got, ok := byName[name]
		if !ok {
			t.Fatalf("OpenCode SDD agent %q missing", name)
		}
		if got.Mode != "subagent" || !got.Hidden || !strings.Contains(got.Prompt, ".jarvis/skills/"+name+"/SKILL.md") {
			t.Fatalf("OpenCode SDD agent %q regressed: %#v", name, got)
		}
	}
}

func frontmatterValue(content, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
