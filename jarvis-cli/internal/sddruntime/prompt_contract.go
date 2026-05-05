package sddruntime

import (
	"fmt"
	"sort"
)

type PromptLayer string

const (
	Layer1Behavior PromptLayer = "layer1_behavior"
	Layer2Persona  PromptLayer = "layer2_persona"
	LayerSkillRule PromptLayer = "skill_rule"
	LayerRegistry  PromptLayer = "registry_rule"
	LayerProtocol  PromptLayer = "protocol_rule"
)

type SourceRole string

const (
	RoleBehavioralInstruction SourceRole = "behavioral_instruction"
	RoleStylePersona          SourceRole = "style_persona"
)

type PromptSource struct {
	ID          string
	Layer       PromptLayer
	Path        string
	Required    bool
	Order       int
	ContentRole SourceRole
}

type PromptContract struct {
	Agent   string
	Phase   string
	Sources []PromptSource
}

func DefaultPromptContract(agent, phase string) PromptContract {
	return PromptContract{
		Agent: agent,
		Phase: phase,
		Sources: []PromptSource{
			{ID: "layer1.behavior", Layer: Layer1Behavior, Path: "jarvis-cli/internal/config/layer1.md", Required: true, Order: 1, ContentRole: RoleBehavioralInstruction},
			{ID: "layer2.persona", Layer: Layer2Persona, Path: "persona", Required: true, Order: 2, ContentRole: RoleStylePersona},
			{ID: fmt.Sprintf("skill.%s", canonicalSkillPhase(phase)), Layer: LayerSkillRule, Path: "skills", Required: true, Order: 3, ContentRole: RoleBehavioralInstruction},
			{ID: "registry.compact-rules", Layer: LayerRegistry, Path: ".jarvis/skill-registry.md", Required: true, Order: 4, ContentRole: RoleBehavioralInstruction},
			{ID: "protocol.hive", Layer: LayerProtocol, Path: "jarvis-cli/embed/hive-protocol.md", Required: true, Order: 5, ContentRole: RoleBehavioralInstruction},
		},
	}
}

func (c PromptContract) OrderedRequiredSources() ([]PromptSource, error) {
	sources := make([]PromptSource, 0, len(c.Sources))
	for _, source := range c.Sources {
		if !source.Required {
			continue
		}
		if err := ValidateLayerSource(source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}

	skillID := "skill." + canonicalSkillPhase(c.Phase)
	if !hasRequiredSource(sources, skillID) {
		return nil, fmt.Errorf("%w: %s", ErrMissingRequiredSource, skillID)
	}

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Order < sources[j].Order
	})

	return sources, nil
}

func canonicalSkillPhase(phase string) string {
	if phase == "orchestrator" {
		return "sdd-orchestrator"
	}
	return phase
}

func hasRequiredSource(sources []PromptSource, id string) bool {
	for _, source := range sources {
		if source.ID == id {
			return true
		}
	}
	return false
}
