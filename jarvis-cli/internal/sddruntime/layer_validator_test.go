package sddruntime

import "testing"

func TestValidateLayerSeparation_ClassifiesClosedTaxonomy(t *testing.T) {
	tests := []struct {
		name    string
		source  PromptSource
		wantErr bool
	}{
		{
			name:    "layer1 behavior accepted",
			source:  PromptSource{ID: "layer1.behavior", Layer: Layer1Behavior, ContentRole: RoleBehavioralInstruction},
			wantErr: false,
		},
		{
			name:    "layer2 persona accepted",
			source:  PromptSource{ID: "layer2.persona", Layer: Layer2Persona, ContentRole: RoleStylePersona},
			wantErr: false,
		},
		{
			name:    "skill rule accepted",
			source:  PromptSource{ID: "skill.sdd-apply", Layer: LayerSkillRule, ContentRole: RoleBehavioralInstruction},
			wantErr: false,
		},
		{
			name:    "registry skill index accepted",
			source:  PromptSource{ID: "registry.skill-index", Layer: LayerRegistry, ContentRole: RoleBehavioralInstruction},
			wantErr: false,
		},
		{
			name:    "Hive protocol accepted",
			source:  PromptSource{ID: "protocol.hive", Layer: LayerProtocol, ContentRole: RoleBehavioralInstruction},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLayerSource(tt.source)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateLayerSource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLayerSeparation_DefaultPromptContractSourcesRemainCompatible(t *testing.T) {
	for _, tc := range []struct {
		agent    string
		phase    string
		wantByID map[string]SourceRole
	}{
		{
			agent: "claude",
			phase: "orchestrator",
			wantByID: map[string]SourceRole{
				"layer1.behavior":          RoleBehavioralInstruction,
				"layer2.persona":           RoleStylePersona,
				"skill.sdd-orchestrator":   RoleBehavioralInstruction,
				RegistrySkillIndexSourceID: RoleBehavioralInstruction,
				"protocol.hive":            RoleBehavioralInstruction,
			},
		},
		{
			agent: "opencode",
			phase: "sdd-apply",
			wantByID: map[string]SourceRole{
				"layer1.behavior":          RoleBehavioralInstruction,
				"layer2.persona":           RoleStylePersona,
				"skill.sdd-apply":          RoleBehavioralInstruction,
				RegistrySkillIndexSourceID: RoleBehavioralInstruction,
				"protocol.hive":            RoleBehavioralInstruction,
			},
		},
	} {
		t.Run(tc.agent+"_"+tc.phase, func(t *testing.T) {
			sources, err := DefaultPromptContract(tc.agent, tc.phase).OrderedRequiredSources()
			if err != nil {
				t.Fatalf("OrderedRequiredSources() error = %v", err)
			}
			for _, source := range sources {
				if err := ValidateLayerSource(source); err != nil {
					t.Fatalf("Default source %s no longer validates: %v", source.ID, err)
				}
				wantRole, ok := tc.wantByID[source.ID]
				if !ok {
					continue
				}
				if source.ContentRole != wantRole {
					t.Fatalf("Default source %s role = %s, want %s", source.ID, source.ContentRole, wantRole)
				}
			}
			byID := make(map[string]PromptSource, len(sources))
			for _, source := range sources {
				byID[source.ID] = source
			}
			for id, wantRole := range tc.wantByID {
				source, ok := byID[id]
				if !ok {
					t.Fatalf("required source %s missing from default contract", id)
				}
				if source.ContentRole != wantRole {
					t.Fatalf("Default source %s role = %s, want %s", id, source.ContentRole, wantRole)
				}
			}
		})
	}
}

func TestValidateLayerSeparation_RejectsCrossRoleLeakage(t *testing.T) {
	tests := []struct {
		name   string
		source PromptSource
	}{
		{
			name:   "layer1 rejects persona style",
			source: PromptSource{ID: "layer1.behavior", Layer: Layer1Behavior, ContentRole: RoleStylePersona},
		},
		{
			name:   "layer2 rejects behavioral instruction",
			source: PromptSource{ID: "layer2.persona", Layer: Layer2Persona, ContentRole: RoleBehavioralInstruction},
		},
		{
			name:   "unknown layer rejected",
			source: PromptSource{ID: "x", Layer: PromptLayer("unknown"), ContentRole: RoleBehavioralInstruction},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLayerSource(tt.source)
			if err == nil {
				t.Fatal("expected layer validation error")
			}
		})
	}
}
