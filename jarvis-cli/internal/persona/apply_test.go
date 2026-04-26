package persona

import (
	"errors"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
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

func TestApplyPresetPipeline_NoResidueAcrossPresetSwitch(t *testing.T) {
	agents := []PresetAgent{newPipelineAgentStub("claude", true)}

	presetA := &Preset{
		Name:                  "argentino",
		DisplayName:           "Argentino",
		Description:           "Preset A",
		Tone:                  Tone{Formality: "casual", Directness: "high", Humor: "warm", Language: "es-ar"},
		CommunicationStyle:    CommunicationStyle{Verbosity: "high", ShowAlternatives: true, ChallengeAssumptions: true},
		CharacteristicPhrases: CharacteristicPhrases{Greetings: []string{"Che"}, Confirmations: []string{"Dale"}},
		Notes:                 "# A\n\nSolo A.",
	}
	presetB := &Preset{
		Name:                  "tony-stark",
		DisplayName:           "Tony Stark",
		Description:           "Preset B",
		Tone:                  Tone{Formality: "balanced", Directness: "high", Humor: "sarcastic", Language: "en-us"},
		CommunicationStyle:    CommunicationStyle{Verbosity: "moderate", ShowAlternatives: true, ChallengeAssumptions: true},
		CharacteristicPhrases: CharacteristicPhrases{Greetings: []string{"Hey"}, Confirmations: []string{"Done"}},
		Notes:                 "# B\n\nSolo B.",
	}

	for _, resolved := range []*ResolvedPreset{
		{Slug: "argentino", Source: PresetSourceBuiltin, Preset: presetA},
		{Slug: "tony-stark", Source: PresetSourceBuiltin, Preset: presetB},
	} {
		err := ApplyPresetPipeline(agents, resolved, ApplyOptions{
			Layer1:               "layer1",
			PreviousPresetSlug:   "argentino",
			PreviousPresetSource: PresetSourceBuiltin,
			PersistConfig:        false,
		})
		if err != nil {
			t.Fatalf("ApplyPresetPipeline(%s) returned error: %v", resolved.Slug, err)
		}
	}

	agent := agents[0].(*pipelineAgentStub)
	if !strings.Contains(agent.layer2, "Tony Stark") {
		t.Fatalf("layer2 should include new preset content, got: %q", agent.layer2)
	}
	if strings.Contains(agent.layer2, "Argentino") {
		t.Fatalf("layer2 contains previous preset residue: %q", agent.layer2)
	}
	if got := agent.settings["outputStyle"]; got != "TonyStark" {
		t.Fatalf("settings.outputStyle = %q, want %q", got, "TonyStark")
	}
	if _, exists := agent.outputFiles["Argentino.md"]; exists {
		t.Fatalf("previous output-style file residue detected: %v", keys(agent.outputFiles))
	}
	if _, exists := agent.outputFiles["TonyStark.md"]; !exists {
		t.Fatalf("new output-style file was not written")
	}
}

func TestApplyPresetPipeline_ErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		agents    []PresetAgent
		resolved  *ResolvedPreset
		opts      ApplyOptions
		wantError string
	}{
		{
			name:      "nil resolved preset",
			resolved:  nil,
			wantError: "resolved preset is required",
		},
		{
			name:      "resolved preset without payload",
			resolved:  &ResolvedPreset{Slug: "neutra", Source: PresetSourceBuiltin},
			wantError: "resolved preset is required",
		},
		{
			name:      "empty resolved slug",
			resolved:  newResolvedPreset(""),
			wantError: "resolved preset slug cannot be empty",
		},
		{
			name:      "write instructions failure",
			agents:    []PresetAgent{&pipelineAgentStub{name: "claude", instructionsErr: errors.New("boom")}},
			resolved:  newResolvedPreset("neutra"),
			wantError: "apply preset to claude instructions",
		},
		{
			name:     "clear output style failure",
			agents:   []PresetAgent{&pipelineAgentStub{name: "claude", outputSupported: true, clearErr: errors.New("cleanup failed"), settings: map[string]string{}, outputFiles: map[string]string{}}},
			resolved: newResolvedPreset("tony-stark"),
			opts: ApplyOptions{
				PreviousPresetSlug: "argentino",
			},
			wantError: "cleanup previous output-style for claude",
		},
		{
			name:      "write output style failure",
			agents:    []PresetAgent{&pipelineAgentStub{name: "claude", outputSupported: true, outputErr: errors.New("write failed"), settings: map[string]string{}, outputFiles: map[string]string{}}},
			resolved:  newResolvedPreset("tony-stark"),
			wantError: "write output-style for claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyPresetPipeline(tt.agents, tt.resolved, tt.opts)
			if err == nil {
				t.Fatalf("ApplyPresetPipeline expected error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ApplyPresetPipeline error = %q, want contains %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestApplyPresetPipeline_PersistConfigAndSourceNormalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seed := &config.AppConfig{
		Preset:              "argentino",
		PersonaPreset:       "argentino",
		PersonaPresetSource: "builtin",
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	resolved := newResolvedPreset("Custom Mentor")
	resolved.Source = PresetSource(" USER ")

	if err := ApplyPresetPipeline(nil, resolved, ApplyOptions{PersistConfig: true}); err != nil {
		t.Fatalf("ApplyPresetPipeline persist config: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PersonaPreset != "custom-mentor" {
		t.Fatalf("persona preset = %q, want custom-mentor", cfg.PersonaPreset)
	}
	if cfg.Preset != "custom-mentor" {
		t.Fatalf("preset = %q, want custom-mentor", cfg.Preset)
	}
	if cfg.PersonaPresetSource != "user" {
		t.Fatalf("persona_preset_source = %q, want user", cfg.PersonaPresetSource)
	}
}

func TestApplyPresetPipeline_DoesNotClearWhenPreviousEqualsResolved(t *testing.T) {
	agent := newPipelineAgentStub("claude", true)

	err := ApplyPresetPipeline([]PresetAgent{agent}, newResolvedPreset("neutra"), ApplyOptions{
		PreviousPresetSlug: "NEUTRA",
	})
	if err != nil {
		t.Fatalf("ApplyPresetPipeline: %v", err)
	}
	if len(agent.clearCalls) != 0 {
		t.Fatalf("expected no cleanup call when previous equals resolved, got %v", agent.clearCalls)
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
