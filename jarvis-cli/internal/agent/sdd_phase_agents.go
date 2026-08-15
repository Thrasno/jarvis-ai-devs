package agent

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

type SDDPhaseAgentDefinition struct {
	Name               string
	SkillID            string
	Description        string
	ModelKey           string
	OpenCodePermission string
	ClaudeTools        []string
}

type HiveMCPToolRequirement struct {
	LogicalName  string
	ClaudeTool   string
	OpenCodeTool string
}

type claudeSDDAgentTemplateData struct {
	Name        string
	Description string
	Tools       string
	Model       string
	Effort      string
	SkillID     string
	SkillPath   string
}

func RequiredHiveMCPToolRequirements() []HiveMCPToolRequirement {
	return []HiveMCPToolRequirement{
		{LogicalName: "mem_search", ClaudeTool: "mcp__hive__mem_search", OpenCodeTool: "hive_mem_search"},
		{LogicalName: "mem_get_observation", ClaudeTool: "mcp__hive__mem_get_observation", OpenCodeTool: "hive_mem_get_observation"},
		{LogicalName: "mem_save", ClaudeTool: "mcp__hive__mem_save", OpenCodeTool: "hive_mem_save"},
		{LogicalName: "mem_context", ClaudeTool: "mcp__hive__mem_context", OpenCodeTool: "hive_mem_context"},
		{LogicalName: "mem_session_summary", ClaudeTool: "mcp__hive__mem_session_summary", OpenCodeTool: "hive_mem_session_summary"},
	}
}

func RequiredClaudeHiveMCPTools() []string {
	requirements := RequiredHiveMCPToolRequirements()
	tools := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		tools = append(tools, requirement.ClaudeTool)
	}
	return tools
}

func RequiredOpenCodeHiveMCPTools() []string {
	requirements := RequiredHiveMCPToolRequirements()
	tools := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		tools = append(tools, requirement.OpenCodeTool)
	}
	return tools
}

func SDDPhaseAgentDefinitions() []SDDPhaseAgentDefinition {
	readTools := withClaudeHiveMCPTools("Read", "Grep", "Glob", "Bash")
	writeTools := withClaudeHiveMCPTools("Read", "Grep", "Glob", "Bash", "Edit", "MultiEdit", "Write")

	return []SDDPhaseAgentDefinition{
		{
			Name:               "sdd-init",
			SkillID:            "sdd-init",
			Description:        "Initialize SDD context, testing capabilities, registry, and persistence.",
			ModelKey:           "sdd-init",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":{"*":"ask","go test *":"allow"}}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-explore",
			SkillID:            "sdd-explore",
			Description:        "Explore SDD ideas and codebase context before committing to a change.",
			ModelKey:           "sdd-explore",
			OpenCodePermission: `{"task":"deny","edit":"deny","bash":"ask"}`,
			ClaudeTools:        readTools,
		},
		{
			Name:               "sdd-propose",
			SkillID:            "sdd-propose",
			Description:        "Create SDD change proposals with intent, scope, and approach.",
			ModelKey:           "sdd-propose",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":"ask"}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-spec",
			SkillID:            "sdd-spec",
			Description:        "Write SDD delta specifications with requirements and scenarios.",
			ModelKey:           "sdd-spec",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":"ask"}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-design",
			SkillID:            "sdd-design",
			Description:        "Create technical designs and architecture approaches for SDD changes.",
			ModelKey:           "sdd-design",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":"ask"}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-tasks",
			SkillID:            "sdd-tasks",
			Description:        "Break SDD changes into implementation tasks and reviewable PR slices.",
			ModelKey:           "sdd-tasks",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":"ask"}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-apply",
			SkillID:            "sdd-apply",
			Description:        "Implement assigned SDD tasks using the active testing contract.",
			ModelKey:           "sdd-apply",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":{"*":"ask","go test *":"allow"}}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-verify",
			SkillID:            "sdd-verify",
			Description:        "Verify implementation against SDD specs, design, tasks, and tests.",
			ModelKey:           "sdd-verify",
			OpenCodePermission: `{"task":"deny","edit":"deny","bash":{"*":"ask","go test *":"allow","go vet *":"allow"}}`,
			ClaudeTools:        readTools,
		},
		{
			Name:               "sdd-archive",
			SkillID:            "sdd-archive",
			Description:        "Archive completed SDD changes by syncing delta specs and closing artifacts.",
			ModelKey:           "sdd-archive",
			OpenCodePermission: `{"task":"deny","edit":"allow","bash":"ask"}`,
			ClaudeTools:        writeTools,
		},
		{
			Name:               "sdd-onboard",
			SkillID:            "sdd-onboard",
			Description:        "Walk users through the SDD workflow on the real codebase.",
			ModelKey:           "sdd-onboard",
			OpenCodePermission: `{"task":"deny","edit":"deny","bash":"ask"}`,
			ClaudeTools:        readTools,
		},
	}
}

func withClaudeHiveMCPTools(tools ...string) []string {
	return append(append([]string{}, tools...), RequiredClaudeHiveMCPTools()...)
}

func RenderClaudeSDDPhaseAgents(templatesFS fs.FS, models state.PhaseModels) (map[string][]byte, error) {
	templateBytes, err := fs.ReadFile(templatesFS, "embed/templates/claude-sdd-agent.md.tmpl")
	if err != nil {
		templateBytes, err = fs.ReadFile(jarvis.TemplatesFS, "embed/templates/claude-sdd-agent.md.tmpl")
		if err != nil {
			return nil, fmt.Errorf("read Claude SDD agent template: %w", err)
		}
	}
	routes, err := sddruntime.ResolvePhaseRoutesForPlatform(sddruntime.PlatformClaude, models)
	if err != nil {
		return nil, fmt.Errorf("resolve Claude phase routes: %w", err)
	}
	tmpl, err := template.New("claude-sdd-agent.md.tmpl").Parse(string(templateBytes))
	if err != nil {
		return nil, fmt.Errorf("parse Claude SDD agent template: %w", err)
	}

	defs := SDDPhaseAgentDefinitions()
	files := make(map[string][]byte, len(defs))
	for _, def := range defs {
		route := phaseRouteForDefinition(routes, def)
		data := claudeSDDAgentTemplateData{
			Name:        def.Name,
			Description: def.Description,
			Tools:       strings.Join(def.ClaudeTools, ", "),
			Model:       route.Model,
			Effort:      route.Effort,
			SkillID:     def.SkillID,
			SkillPath:   ".jarvis/skills/" + def.SkillID + "/SKILL.md",
		}
		var out bytes.Buffer
		if err := tmpl.Execute(&out, data); err != nil {
			return nil, fmt.Errorf("render Claude SDD agent %s: %w", def.Name, err)
		}
		files[def.Name+".md"] = out.Bytes()
	}
	return files, nil
}

func phaseRouteForDefinition(routes map[string]sddruntime.PhaseRoute, def SDDPhaseAgentDefinition) sddruntime.PhaseRoute {
	if route, ok := routes[def.ModelKey]; ok && strings.TrimSpace(route.Model) != "" {
		return route
	}
	if route, ok := routes[def.Name]; ok && strings.TrimSpace(route.Model) != "" {
		return route
	}
	return routes["default"]
}
