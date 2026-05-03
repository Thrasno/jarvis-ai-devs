package sddruntime

import "fmt"

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
		return RuntimePlan{}, fmt.Errorf("unsupported agent %q", agent)
	}

	return plan, nil
}
