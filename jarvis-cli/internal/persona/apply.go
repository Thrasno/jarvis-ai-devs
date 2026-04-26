package persona

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

// PresetAgent defines the minimal contract required by ApplyPresetPipeline.
// agent.Agent satisfies this interface without introducing package cycles.
type PresetAgent interface {
	Name() string
	WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error
	SupportsOutputStyles() bool
	WriteOutputStyle(preset *Preset) error
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

// ApplyPresetPipeline applies a resolved preset with clean replacement semantics.
// It rewrites Layer2 instructions, clears previous output-style references/files,
// writes the new output-style, and optionally persists canonical preset identity.
func ApplyPresetPipeline(agents []PresetAgent, resolved *ResolvedPreset, opts ApplyOptions) error {
	if resolved == nil || resolved.Preset == nil {
		return fmt.Errorf("resolved preset is required")
	}

	resolvedSlug := NormalizeSlug(resolved.Slug)
	if resolvedSlug == "" {
		return fmt.Errorf("resolved preset slug cannot be empty")
	}

	layer2 := RenderLayer2(resolved.Preset)
	for _, a := range agents {
		if err := a.WriteInstructions(opts.Layer1, layer2, opts.Skills); err != nil {
			return fmt.Errorf("apply preset to %s instructions: %w", a.Name(), err)
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

		if err := a.WriteOutputStyle(resolved.Preset); err != nil {
			return fmt.Errorf("write output-style for %s: %w", a.Name(), err)
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
