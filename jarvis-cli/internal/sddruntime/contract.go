package sddruntime

const (
	DefaultContractVersion = "2026.05"
	DefaultRegistryPath    = ".jarvis/skill-registry.md"
)

type Contract struct {
	Version          string
	RegistryPath     string
	ModelAssignments map[string]string
	ManagedArtifacts []ManagedArtifact
}

type ManagedArtifact struct {
	ID            string
	RelativePath  string
	Scope         OwnershipScope
	Markers       [2]string
	ExpectedSHA256 string
}

type OwnershipScope string

const (
	OwnershipFile  OwnershipScope = "file"
	OwnershipBlock OwnershipScope = "block"
)

func DefaultContract() Contract {
	return Contract{
		Version:      DefaultContractVersion,
		RegistryPath: DefaultRegistryPath,
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
			{ID: "instructions", Scope: OwnershipBlock, Markers: [2]string{"<!-- jarvis:layer1:start -->", "<!-- jarvis:layer1:end -->"}},
			{ID: "orchestrator", RelativePath: "sdd-orchestrator.md", Scope: OwnershipFile},
			{ID: "skills", RelativePath: "skills/", Scope: OwnershipFile},
		},
	}
}
