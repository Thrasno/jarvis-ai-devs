package persona

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

// PresetV2Agent is the optional adapter contract for dormant schema-v2
// presentation profiles. It deliberately remains separate from PresetAgent so
// the active V1 pipeline stays source and behavior compatible until activation.
type PresetV2Agent interface {
	Name() string
	WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error
	SupportsOutputStyles() bool
	WriteOutputStyleV2(preset *PresetV2) error
	ClearOutputStyle(name string) error
}

// ApplyOptions controls how preset apply is executed.
type ApplyOptions struct {
	Layer1               string
	Skills               []config.SkillInfo
	PreviousPresetSlug   string
	PreviousPresetSource PresetSource
	PersistConfig        bool
}

// ApplyPresetV2Pipeline applies a previously validated schema-v2 presentation
// profile through V2-capable adapters. It is intentionally not called by the
// default resolver or normal V1 CLI/TUI selection paths.
func ApplyPresetV2Pipeline(agents []PresetV2Agent, resolved *ResolvedPresetV2, opts ApplyOptions) error {
	if resolved == nil || resolved.Preset == nil {
		return fmt.Errorf("resolved schema v2 preset is required")
	}

	resolvedSlug := NormalizeSlug(resolved.Slug)
	if resolvedSlug == "" {
		return fmt.Errorf("resolved schema v2 preset slug cannot be empty")
	}

	layer2 := RenderLayer2V2(resolved.Preset)
	for _, a := range agents {
		if err := a.WriteInstructions(opts.Layer1, layer2, opts.Skills); err != nil {
			return fmt.Errorf("apply schema v2 preset to %s instructions: %w", a.Name(), err)
		}

		if !a.SupportsOutputStyles() {
			continue
		}

		previousSlug := NormalizeSlug(opts.PreviousPresetSlug)
		if previousSlug != "" && previousSlug != resolvedSlug {
			if err := a.ClearOutputStyle(toTitleCase(previousSlug)); err != nil {
				return fmt.Errorf("cleanup previous output-style for %s: %w", a.Name(), err)
			}
		}

		if err := a.WriteOutputStyleV2(resolved.Preset); err != nil {
			return fmt.Errorf("write schema v2 output-style for %s: %w", a.Name(), err)
		}
	}

	if !opts.PersistConfig {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.PersonaPreset = resolvedSlug
	cfg.Preset = resolvedSlug
	cfg.PersonaPresetSource = normalizePresetSourceForConfig(resolved.Source)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}

func normalizePresetSourceForConfig(source PresetSource) string {
	trimmed := strings.ToLower(strings.TrimSpace(string(source)))
	if trimmed == "user" {
		return "user"
	}
	return "builtin"
}
