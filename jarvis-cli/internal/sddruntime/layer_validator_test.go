package sddruntime

import "testing"

func TestValidateLayerSeparation_ClassifiesClosedTaxonomy(t *testing.T) {
	tests := []struct {
		name    string
		source  PromptSource
		wantErr bool
	}{
		{
			name: "layer1 behavior accepted",
			source: PromptSource{ID: "layer1.behavior", Layer: Layer1Behavior, ContentRole: RoleBehavioralInstruction},
			wantErr: false,
		},
		{
			name: "layer2 persona accepted",
			source: PromptSource{ID: "layer2.persona", Layer: Layer2Persona, ContentRole: RoleStylePersona},
			wantErr: false,
		},
		{
			name: "skill rule accepted",
			source: PromptSource{ID: "skill.sdd-apply", Layer: LayerSkillRule, ContentRole: RoleBehavioralInstruction},
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
