package sddruntime

import "fmt"

var (
	ErrMissingRequiredSource = fmt.Errorf("missing required prompt source")
	ErrInvalidLayer          = fmt.Errorf("invalid prompt layer")
	ErrLayerRoleLeakage      = fmt.Errorf("layer role leakage")
	ErrUnsupportedAgent      = fmt.Errorf("unsupported agent")
)

func ValidateLayerSource(source PromptSource) error {
	switch source.Layer {
	case Layer1Behavior:
		if source.ContentRole != RoleBehavioralInstruction {
			return fmt.Errorf("%w: %s cannot hold %s", ErrLayerRoleLeakage, Layer1Behavior, source.ContentRole)
		}
	case Layer2Persona:
		if source.ContentRole != RoleStylePersona {
			return fmt.Errorf("%w: %s cannot hold %s", ErrLayerRoleLeakage, Layer2Persona, source.ContentRole)
		}
	case LayerSkillRule, LayerRegistry, LayerProtocol:
		if source.ContentRole != RoleBehavioralInstruction {
			return fmt.Errorf("%w: %s must be behavioral", ErrLayerRoleLeakage, source.Layer)
		}
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLayer, source.Layer)
	}
	return nil
}
