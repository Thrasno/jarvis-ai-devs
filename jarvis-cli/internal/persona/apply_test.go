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

func (a *pipelineAgentStub) WriteOutputStyle(preset *Preset) error {
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

func (a *pipelineAgentStub) WriteOutputStyleV2(preset *PresetV2) error {
	if !a.outputSupported {
		return nil
	}
	if a.outputErr != nil {
		return a.outputErr
	}
	styleName := testTitleCase(preset.Name)
	a.settings["outputStyle"] = styleName
	a.outputFiles[styleName+".md"] = RenderOutputStyleV2(preset)
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

func newResolvedPreset(slug string) *ResolvedPreset {
	return &ResolvedPreset{
		Slug:   slug,
		Source: PresetSourceBuiltin,
		Preset: &Preset{
			Name:        slug,
			DisplayName: testTitleCase(slug),
			Description: "test preset",
			Tone: Tone{
				Formality:  "neutral",
				Directness: "direct",
				Humor:      "none",
				Language:   "en-us",
			},
			CommunicationStyle: CommunicationStyle{
				Verbosity:            "concise",
				ShowAlternatives:     true,
				ChallengeAssumptions: true,
			},
			CharacteristicPhrases: CharacteristicPhrases{
				Greetings:     []string{"Hi"},
				Confirmations: []string{"OK"},
			},
			Notes: "# Notes\n\nBody.",
		},
	}
}

func newResolvedPresetV2(slug string) *ResolvedPresetV2 {
	return &ResolvedPresetV2{
		Slug:   slug,
		Source: PresetSourceBuiltin,
		Preset: &PresetV2{
			SchemaVersion: 2,
			Name:          slug,
			DisplayName:   testTitleCase(slug),
			Presentation: PresentationV2{
				Language: "en-us", Register: "friendly-professional", Vocabulary: "plain-technical", Cadence: "measured",
				Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured",
				TeachingMetaphors: "construction", Examples: "practical", AddressPack: "peer", PhrasePack: "plain", AntiCaricature: "grounded",
			},
		},
	}
}

func TestApplyPresetV2PipelineReplacesOutputStyleAndPersistsCanonicalIdentity(t *testing.T) {
	isolateTestHome(t)

	if err := config.Save(&config.AppConfig{PersonaPreset: "argentino", PersonaPresetSource: "builtin", Preset: "argentino"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	agent := newPipelineAgentStub("claude", true)
	agent.outputFiles["Argentino.md"] = "legacy output style"
	resolved := newResolvedPresetV2("custom-mentor")
	resolved.Slug = "Custom Mentor"
	resolved.Source = PresetSource(" USER ")

	if err := ApplyPresetV2Pipeline([]PresetV2Agent{agent}, resolved, ApplyOptions{
		Layer1:             "layer1",
		PreviousPresetSlug: "argentino",
		PersistConfig:      true,
	}); err != nil {
		t.Fatalf("ApplyPresetV2Pipeline: %v", err)
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

func TestApplyPresetV2PipelineDoesNotClearOutputStyleWhenNormalizedSlugsMatch(t *testing.T) {
	agent := newPipelineAgentStub("claude", true)
	agent.settings["outputStyle"] = "Argentino"
	agent.outputFiles["Argentino.md"] = "active output style"
	agent.clearErr = errors.New("active output style must not be cleared")

	if err := ApplyPresetV2Pipeline([]PresetV2Agent{agent}, newResolvedPresetV2("Argentino"), ApplyOptions{
		Layer1:             "layer1",
		PreviousPresetSlug: "  argentino  ",
	}); err != nil {
		t.Fatalf("ApplyPresetV2Pipeline: %v", err)
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

func TestApplyPresetV2PipelineErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		agents    []PresetV2Agent
		resolved  *ResolvedPresetV2
		opts      ApplyOptions
		wantError string
	}{
		{
			name:      "nil resolved preset",
			wantError: "resolved schema v2 preset is required",
		},
		{
			name:      "resolved preset without payload",
			resolved:  &ResolvedPresetV2{Slug: "neutra", Source: PresetSourceBuiltin},
			wantError: "resolved schema v2 preset is required",
		},
		{
			name:      "empty resolved slug",
			resolved:  newResolvedPresetV2(""),
			wantError: "resolved schema v2 preset slug cannot be empty",
		},
		{
			name:      "write instructions failure",
			agents:    []PresetV2Agent{&pipelineAgentStub{name: "claude", instructionsErr: errors.New("boom")}},
			resolved:  newResolvedPresetV2("neutra"),
			wantError: "apply schema v2 preset to claude instructions",
		},
		{
			name:     "clear output style failure",
			agents:   []PresetV2Agent{&pipelineAgentStub{name: "claude", outputSupported: true, clearErr: errors.New("cleanup failed"), settings: map[string]string{}, outputFiles: map[string]string{}}},
			resolved: newResolvedPresetV2("tony-stark"),
			opts: ApplyOptions{
				PreviousPresetSlug: "argentino",
			},
			wantError: "cleanup previous output-style for claude",
		},
		{
			name:      "write output style failure",
			agents:    []PresetV2Agent{&pipelineAgentStub{name: "claude", outputSupported: true, outputErr: errors.New("write failed"), settings: map[string]string{}, outputFiles: map[string]string{}}},
			resolved:  newResolvedPresetV2("tony-stark"),
			wantError: "write schema v2 output-style for claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyPresetV2Pipeline(tt.agents, tt.resolved, tt.opts)
			if err == nil {
				t.Fatalf("ApplyPresetV2Pipeline expected error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ApplyPresetV2Pipeline error = %q, want contains %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestApplyPresetSelectionPipelineUsesV2PresentationAndOutputStyle(t *testing.T) {
	agent := newPipelineAgentStub("claude", true)
	v2 := newResolvedPresetV2("custom-mentor")

	if err := ApplyPresetSelectionPipeline([]PresetAgent{agent}, PresetSelection{V2: v2}, ApplyOptions{PreviousPresetSlug: "neutra"}); err != nil {
		t.Fatalf("ApplyPresetSelectionPipeline(V2) error = %v", err)
	}
	if !strings.Contains(agent.layer2, "### Presentation") || strings.Contains(agent.layer2, "Legacy Notes") {
		t.Fatalf("V2 selection did not render only presentation data: %q", agent.layer2)
	}
	if _, ok := agent.outputFiles["CustomMentor.md"]; !ok {
		t.Fatalf("V2 output style was not written: %v", keys(agent.outputFiles))
	}
}

func TestApplyPresetSelectionPipelineRejectsAmbiguousSelections(t *testing.T) {
	err := ApplyPresetSelectionPipeline(nil, PresetSelection{
		V1: newResolvedPreset("neutra"),
		V2: &ResolvedPresetV2{Slug: "custom-mentor", Preset: &PresetV2{Name: "custom-mentor"}},
	}, ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "exactly one preset version") {
		t.Fatalf("ApplyPresetSelectionPipeline() error = %v, want version-selection guidance", err)
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
