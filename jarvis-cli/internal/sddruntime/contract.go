package sddruntime

import "github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"

const (
	DefaultContractVersion = "2026.05"
	DefaultRegistryPath    = ".jarvis/skill-registry.md"
)

type Contract struct {
	Version               string
	JarvisVersion         string
	ContractVersion       string
	ProviderSchemaVersion string
	RegistryPath          string
	Phases                []string
	PlatformCatalogs      map[Platform][]string
	DefaultPhaseModels    map[string]config.PhaseModelSelection
	// ModelAssignments is retained as legacy compatibility data.
	// Production fallback/verification behavior must derive defaults from DefaultPhaseModels.
	ModelAssignments map[string]string
	ManagedArtifacts []ManagedArtifact
}

type Platform string

const (
	PlatformOpenCode Platform = "opencode"
	PlatformClaude   Platform = "claude"
)

type ManagedArtifact struct {
	ID             string
	RelativePath   string
	Required       bool
	Scope          OwnershipScope
	Markers        [2]string
	ExpectedSHA256 string
}

type OwnershipScope string

const (
	OwnershipFile     OwnershipScope = "file"
	OwnershipBlock    OwnershipScope = "block"
	OwnershipJSONPath OwnershipScope = "json_path"
	OwnershipLogical  OwnershipScope = "logical"
)

func DefaultContract() Contract {
	return Contract{
		Version:               DefaultContractVersion,
		JarvisVersion:         "dev",
		ContractVersion:       DefaultContractVersion,
		ProviderSchemaVersion: "v1",
		RegistryPath:          DefaultRegistryPath,
		Phases: []string{
			"default",
			"orchestrator",
			"sdd-explore",
			"sdd-propose",
			"sdd-spec",
			"sdd-design",
			"sdd-tasks",
			"sdd-apply",
			"sdd-verify",
			"sdd-archive",
		},
		PlatformCatalogs: map[Platform][]string{
			PlatformOpenCode: []string{"opus", "sonnet", "haiku"},
			PlatformClaude:   []string{"opus", "sonnet", "haiku"},
		},
		DefaultPhaseModels: map[string]config.PhaseModelSelection{
			"orchestrator": {OpenCode: "opus", Claude: "opus"},
			"sdd-explore":  {OpenCode: "sonnet", Claude: "sonnet"},
			"sdd-propose":  {OpenCode: "opus", Claude: "opus"},
			"sdd-spec":     {OpenCode: "sonnet", Claude: "sonnet"},
			"sdd-design":   {OpenCode: "opus", Claude: "opus"},
			"sdd-tasks":    {OpenCode: "sonnet", Claude: "sonnet"},
			"sdd-apply":    {OpenCode: "sonnet", Claude: "sonnet"},
			"sdd-verify":   {OpenCode: "sonnet", Claude: "sonnet"},
			"sdd-archive":  {OpenCode: "haiku", Claude: "haiku"},
			"default":      {OpenCode: "sonnet", Claude: "sonnet"},
		},
		ModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-explore":  "sonnet",
			"sdd-propose":  "opus",
			"sdd-spec":     "sonnet",
			"sdd-design":   "opus",
			"sdd-tasks":    "sonnet",
			"sdd-apply":    "sonnet",
			"sdd-verify":   "sonnet",
			"sdd-archive":  "haiku",
			"default":      "sonnet",
		},
		ManagedArtifacts: []ManagedArtifact{
			{ID: "instructions", Required: true, Scope: OwnershipBlock, Markers: [2]string{"<!-- jarvis:layer1:start -->", "<!-- jarvis:layer1:end -->"}},
			{ID: "orchestrator", Required: true, RelativePath: "sdd-orchestrator.md", Scope: OwnershipFile},
			{ID: "skills", Required: true, RelativePath: "skills/", Scope: OwnershipFile},
			{ID: "settings", RelativePath: "settings.json", Scope: OwnershipJSONPath},
			{ID: "output_style_settings", RelativePath: "settings.json", Scope: OwnershipJSONPath},
			{ID: "output_style", RelativePath: "output-styles/", Scope: OwnershipFile},
			{ID: "prompt_hook", RelativePath: "hive-hooks/", Scope: OwnershipLogical},
		},
	}
}
