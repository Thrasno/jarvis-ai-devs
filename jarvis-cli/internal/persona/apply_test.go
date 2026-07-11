package persona

import (
	"errors"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

type pipelineAgentStub struct {
	name            string
	outputSupported bool
	layer2          string
	settings        map[string]string
	outputFiles     map[string]string
	clearCalls      []string
	instructionsErr error
	clearErr        error
	outputErr       error
}

func newPipelineAgentStub(name string, outputSupported bool) *pipelineAgentStub {
	return &pipelineAgentStub{
		name:            name,
		outputSupported: outputSupported,
		settings:        map[string]string{},
		outputFiles:     map[string]string{},
	}
}

func (a *pipelineAgentStub) Name() string { return a.name }

func (a *pipelineAgentStub) WriteInstructions(_ string, layer2 string, _ []config.SkillInfo) error {
	if a.instructionsErr != nil {
		return a.instructionsErr
	}
	a.layer2 = layer2
	return nil
}

func (a *pipelineAgentStub) SupportsOutputStyles() bool { return a.outputSupported }

func (a *pipelineAgentStub) WriteOutputStyle(preset *Profile) error {
	if !a.outputSupported {
		return nil
	}
	if a.outputErr != nil {
		return a.outputErr
	}
	styleName := testTitleCase(preset.Name)
	a.settings["outputStyle"] = styleName
	a.outputFiles[styleName+".md"] = RenderOutputStyle(preset)
	return nil
}

func (a *pipelineAgentStub) ClearOutputStyle(name string) error {
	if !a.outputSupported {
		return nil
	}
	a.clearCalls = append(a.clearCalls, name)
	if a.clearErr != nil {
		return a.clearErr
	}
	delete(a.outputFiles, name+".md")
	delete(a.settings, "outputStyle")
	return nil
}

func newResolvedProfile(slug string) *ResolvedProfile {
	return &ResolvedProfile{
		Slug:   slug,
		Source: PresetSourceBuiltin,
		Preset: &Profile{
			SchemaVersion: 2,
			Name:          slug,
			DisplayName:   testTitleCase(slug),
			Presentation: Presentation{
				Language: "en-us", Register: "friendly-professional", Vocabulary: "plain-technical", Cadence: "measured",
				Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured",
				TeachingMetaphors: "construction", Examples: "practical", AddressPack: "peer", PhrasePack: "plain", AntiCaricature: "grounded",
			},
		},
	}
}

func TestApplyProfileReplacesOutputStyleAndPersistsCanonicalIdentity(t *testing.T) {
	isolateTestHome(t)

	if err := config.Save(&config.AppConfig{PersonaPreset: "argentino", PersonaPresetSource: "builtin", Preset: "argentino"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	agent := newPipelineAgentStub("claude", true)
	agent.outputFiles["Argentino.md"] = "legacy output style"
	resolved := newResolvedProfile("custom-mentor")
	resolved.Slug = "Custom Mentor"
	resolved.Source = PresetSource(" USER ")

	if err := ApplyProfile([]ProfileAgent{agent}, resolved, ApplyOptions{
		Layer1:             "layer1",
		PreviousPresetSlug: "argentino",
		PersistConfig:      true,
	}); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	if !strings.Contains(agent.layer2, "### Presentation") || strings.Contains(agent.layer2, "Technical Behavior") {
		t.Fatalf("schema-v2 layer2 must render presentation without policy: %q", agent.layer2)
	}
	if got := agent.settings["outputStyle"]; got != "CustomMentor" {
		t.Fatalf("settings.outputStyle = %q, want CustomMentor", got)
	}
	if _, exists := agent.outputFiles["Argentino.md"]; exists {
		t.Fatalf("previous output-style file residue detected: %v", keys(agent.outputFiles))
	}
	outputStyle, exists := agent.outputFiles["CustomMentor.md"]
	if !exists {
		t.Fatalf("new schema-v2 output-style file was not written: %v", keys(agent.outputFiles))
	}
	for _, forbidden := range []string{"Technical Behavior", "Persona Scope (CRITICAL)"} {
		if strings.Contains(outputStyle, forbidden) {
			t.Fatalf("schema-v2 output style contains policy %q:\n%s", forbidden, outputStyle)
		}
	}
	if !strings.Contains(outputStyle, "keep-coding-instructions: true") {
		t.Fatalf("schema-v2 output style must retain coding instructions:\n%s", outputStyle)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if cfg.PersonaPreset != "custom-mentor" || cfg.Preset != "custom-mentor" || cfg.PersonaPresetSource != "user" {
		t.Fatalf("persisted config = %+v, want canonical schema-v2 user identity", cfg)
	}
}

func TestApplyProfileDoesNotClearOutputStyleWhenNormalizedSlugsMatch(t *testing.T) {
	agent := newPipelineAgentStub("claude", true)
	agent.settings["outputStyle"] = "Argentino"
	agent.outputFiles["Argentino.md"] = "active output style"
	agent.clearErr = errors.New("active output style must not be cleared")

	if err := ApplyProfile([]ProfileAgent{agent}, newResolvedProfile("Argentino"), ApplyOptions{
		Layer1:             "layer1",
		PreviousPresetSlug: "  argentino  ",
	}); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	if len(agent.clearCalls) != 0 {
		t.Fatalf("ClearOutputStyle calls = %v, want none for the active canonical slug", agent.clearCalls)
	}
	if got := agent.settings["outputStyle"]; got != "Argentino" {
		t.Fatalf("settings.outputStyle = %q, want Argentino", got)
	}
	if _, exists := agent.outputFiles["Argentino.md"]; !exists {
		t.Fatalf("active output-style file was removed: %v", keys(agent.outputFiles))
	}
}

func TestApplyProfileErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		agents    []ProfileAgent
		resolved  *ResolvedProfile
		opts      ApplyOptions
		wantError string
	}{
		{
			name:      "nil resolved preset",
			wantError: "resolved schema v2 preset is required",
		},
		{
			name:      "resolved preset without payload",
			resolved:  &ResolvedProfile{Slug: "neutra", Source: PresetSourceBuiltin},
			wantError: "resolved schema v2 preset is required",
		},
		{
			name:      "empty resolved slug",
			resolved:  newResolvedProfile(""),
			wantError: "resolved schema v2 preset slug cannot be empty",
		},
		{
			name:      "write instructions failure",
			agents:    []ProfileAgent{&pipelineAgentStub{name: "claude", instructionsErr: errors.New("boom")}},
			resolved:  newResolvedProfile("neutra"),
			wantError: "apply schema v2 preset to claude instructions",
		},
		{
			name:     "clear output style failure",
			agents:   []ProfileAgent{&pipelineAgentStub{name: "claude", outputSupported: true, clearErr: errors.New("cleanup failed"), settings: map[string]string{}, outputFiles: map[string]string{}}},
			resolved: newResolvedProfile("tony-stark"),
			opts: ApplyOptions{
				PreviousPresetSlug: "argentino",
			},
			wantError: "cleanup previous output-style for claude",
		},
		{
			name:      "write output style failure",
			agents:    []ProfileAgent{&pipelineAgentStub{name: "claude", outputSupported: true, outputErr: errors.New("write failed"), settings: map[string]string{}, outputFiles: map[string]string{}}},
			resolved:  newResolvedProfile("tony-stark"),
			wantError: "write schema v2 output-style for claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyProfile(tt.agents, tt.resolved, tt.opts)
			if err == nil {
				t.Fatalf("ApplyProfile expected error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ApplyProfile error = %q, want contains %q", err.Error(), tt.wantError)
			}
		})
	}
}

func keys(m map[string]string) []string {
	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	return res
}

func testTitleCase(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}
