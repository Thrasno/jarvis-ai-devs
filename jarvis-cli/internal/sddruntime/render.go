package sddruntime

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

type RuntimePaths struct {
	Instructions string
	Settings     string
	Orchestrator string
	Registry     string
}

type RuntimePlan struct {
	Agent    string
	Contract Contract
	Paths    RuntimePaths
}

func Build(agent string) (RuntimePlan, error) {
	contract := DefaultContract()
	plan := RuntimePlan{Agent: agent, Contract: contract}

	switch agent {
	case "claude":
		plan.Paths = RuntimePaths{
			Instructions: ".claude/CLAUDE.md",
			Settings:     ".claude/settings.json",
			Orchestrator: ".claude/sdd-orchestrator.md",
			Registry:     contract.RegistryPath,
		}
	case "opencode":
		plan.Paths = RuntimePaths{
			Instructions: ".config/opencode/AGENTS.md",
			Settings:     ".config/opencode/opencode.json",
			Orchestrator: ".config/opencode/sdd-orchestrator.md",
			Registry:     contract.RegistryPath,
		}
	default:
		return RuntimePlan{}, fmt.Errorf("%w %q", ErrUnsupportedAgent, agent)
	}

	return plan, nil
}

type modelAssignmentRow struct {
	Phase  string
	Model  string
	Effort string
	Reason string
}

type orchestratorTemplateData struct {
	ModelRows []modelAssignmentRow
}

// RenderOrchestrator renders the orchestrator template using the resolved
// cross-platform phase map, selecting the active platform column by agent.
func RenderOrchestrator(agent string, cfg *config.AppConfig, templateContent string) (string, error) {
	platform, err := platformForAgent(agent)
	if err != nil {
		return "", err
	}

	assignments, err := ResolveAssignmentsForPlatform(platform, cfg)
	if err != nil {
		return "", err
	}
	contract := DefaultContract()
	rows := make([]modelAssignmentRow, 0, len(contract.Phases))
	for _, phase := range contract.Phases {
		rows = append(rows, modelAssignmentRow{Phase: phase, Model: assignments[phase], Effort: assignmentEffort(platform, cfg, phase), Reason: phaseReason(phase)})
	}
	selectedTemplate, err := RenderModelSections(templateContent, ModelSectionClassForModel(assignments["orchestrator"]))
	if err != nil {
		return "", fmt.Errorf("render orchestrator model sections: %w", err)
	}

	tmpl, err := template.New("sdd-orchestrator").Parse(selectedTemplate)
	if err != nil {
		return "", fmt.Errorf("parse orchestrator template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, orchestratorTemplateData{ModelRows: rows}); err != nil {
		return "", fmt.Errorf("execute orchestrator template: %w", err)
	}

	return out.String(), nil
}

func assignmentEffort(platform Platform, cfg *config.AppConfig, phase string) string {
	if platform != PlatformOpenCode || cfg == nil || cfg.SDD.OpenCodePhaseModels == nil {
		return "-"
	}
	assignment := cfg.SDD.OpenCodePhaseModels[phase]
	if strings.TrimSpace(assignment.ProviderID) == "" || strings.TrimSpace(assignment.ModelID) == "" {
		return "-"
	}
	effort := strings.TrimSpace(assignment.Effort)
	if effort == "" {
		return "-"
	}
	return effort
}

func platformForAgent(agent string) (Platform, error) {
	switch agent {
	case "opencode":
		return PlatformOpenCode, nil
	case "claude":
		return PlatformClaude, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnsupportedAgent, agent)
	}
}

func phaseReason(phase string) string {
	switch phase {
	case "orchestrator":
		return "Coordinates, makes decisions"
	case "sdd-explore":
		return "Reads code, structural - not architectural"
	case "sdd-propose", "sdd-design":
		return "Architectural decisions"
	case "sdd-spec":
		return "Structured writing"
	case "sdd-tasks":
		return "Mechanical breakdown"
	case "sdd-apply":
		return "Implementation"
	case "sdd-verify":
		return "Validation against spec"
	case "sdd-archive":
		return "Copy and close"
	default:
		return "Non-SDD general delegation"
	}
}
