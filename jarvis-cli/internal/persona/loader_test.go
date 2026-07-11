package persona

import (
	"strings"
	"testing"
	"testing/fstest"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

func TestLoadPreset(t *testing.T) {
	presetNames := []string{
		"argentino",
		"tony-stark",
		"neutra",
		"yoda",
		"sargento",
		"asturiano",
		"galleguinho",
	}

	for _, name := range presetNames {
		t.Run("load_"+name, func(t *testing.T) {
			preset, err := LoadPreset(jarvis.PersonaFS, name)
			if err != nil {
				t.Fatalf("LoadPreset(%q) failed: %v", name, err)
			}
			if preset.Name == "" {
				t.Error("preset.Name is empty")
			}
			if preset.DisplayName == "" {
				t.Error("preset.DisplayName is empty")
			}
		})
	}
}

func TestLoadPreset_Unknown(t *testing.T) {
	_, err := LoadPreset(jarvis.PersonaFS, "unknown-preset-xyz")
	if err == nil {
		t.Fatal("expected error for unknown preset, got nil")
	}
}

func TestListPresets(t *testing.T) {
	presets, err := ListPresets(jarvis.PersonaFS)
	if err != nil {
		t.Fatalf("ListPresets() failed: %v", err)
	}

	if len(presets) < 7 {
		t.Errorf("expected at least 7 presets, got %d", len(presets))
	}

	for _, p := range presets {
		if p.Name == "" {
			t.Error("preset with empty name found")
		}
	}
}

func TestListPresetNames_HardensTopLevelAndSlugContract(t *testing.T) {
	fsys := fstest.MapFS{
		"embed/personas/neutra.yaml":             &fstest.MapFile{Data: []byte("name: neutra")},
		"embed/personas/tony-stark.yaml":         &fstest.MapFile{Data: []byte("name: tony-stark")},
		"embed/personas/ignore_me.yaml":          &fstest.MapFile{Data: []byte("name: ignore-me")},
		"embed/personas/nested/should-skip.yaml": &fstest.MapFile{Data: []byte("name: should-skip")},
		"embed/personas/template.yaml.tmpl":      &fstest.MapFile{Data: []byte("template")},
		"embed/personas/UPPERCASE.yaml":          &fstest.MapFile{Data: []byte("name: upper")},
		"embed/personas/contains.dot.yaml":       &fstest.MapFile{Data: []byte("name: dot")},
		"embed/personas/spaces name.yaml":        &fstest.MapFile{Data: []byte("name: spaced")},
	}

	names := listPresetNames(fsys)
	want := []string{"neutra", "tony-stark"}

	if len(names) != len(want) {
		t.Fatalf("listPresetNames len = %d, want %d (%v)", len(names), len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("listPresetNames[%d] = %q, want %q (all=%v)", i, names[i], want[i], names)
		}
	}
}

func TestResolvePreset_RejectsInvalidSlugPathSeparators(t *testing.T) {
	_, err := ResolvePreset(jarvis.PersonaFS, "../neutra")
	if err == nil {
		t.Fatal("ResolvePreset expected invalid slug error, got nil")
	}
	if got := err.Error(); !contains(got, "path separators are not allowed") {
		t.Fatalf("ResolvePreset error = %q, want path separator validation", got)
	}
}

func TestValidateCustom(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid custom preset",
			content: `
name: my-persona
display_name: "My Persona"
description: "Test persona"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hello"]
  confirmations: ["Done"]
  transitions: ["Note:"]
  sign_offs: ["The end."]
`,
		},
		{
			name: "missing tone.formality",
			content: `
name: bad-persona
display_name: "Bad"
description: "Missing tone formality"
tone:
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hi"]
  confirmations: ["OK"]
`,
			wantErr: true,
		},
		{
			name: "contains protected Layer1 field",
			content: `
name: bad-persona
display_name: "Bad"
description: "Has Layer1 field"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hi"]
  confirmations: ["OK"]
expertise: "This should not be here"
`,
			wantErr: true,
		},
		{
			name: "invalid language value",
			content: `
name: bad-lang
display_name: "Bad"
description: "Invalid language"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: klingon
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hi"]
  confirmations: ["OK"]
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCustom([]byte(tt.content))
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRenderLayer2(t *testing.T) {
	preset, err := LoadPreset(jarvis.PersonaFS, "argentino")
	if err != nil {
		t.Fatalf("LoadPreset failed: %v", err)
	}

	rendered := RenderLayer2(preset)
	if len(rendered) == 0 {
		t.Error("RenderLayer2 produced empty string")
	}
	if rendered == "" {
		t.Error("RenderLayer2 returned empty content")
	}
}

func TestRenderLayer2_IgnoresStructuredSectionsInsideStrippedPersonaScopeBlock(t *testing.T) {
	preset := &Preset{
		Name:        "custom-stale-scope",
		DisplayName: "Custom Stale Scope",
		Description: "Custom persona with stale scoped notes.",
		Tone: Tone{
			Formality:  "balanced",
			Directness: "high",
			Humor:      "warm",
			Language:   "en-us",
		},
		CommunicationStyle: CommunicationStyle{Verbosity: "concise"},
		CharacteristicPhrases: CharacteristicPhrases{
			Greetings:     []string{"Hello"},
			Confirmations: []string{"Done"},
		},
		Notes: "<!-- gentle-ai:persona-scope -->\n## Tone\n\nStale tone override.\n\n## Communication Style\n\nStale communication override.\n\n## Characteristic Phrases\n\nStale phrases override.\n<!-- /gentle-ai:persona-scope -->",
	}

	rendered := RenderLayer2(preset)
	for _, required := range []string{
		"### Tone",
		"- **Formality**: balanced",
		"### Communication Style",
		"- Verbosity: concise",
		"### Characteristic Phrases",
		"**Greetings**: Hello",
		"**Confirmations**: Done",
	} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("RenderLayer2() missing canonical structured output %q\n%s", required, rendered)
		}
	}
	for _, stale := range []string{"Stale tone override.", "Stale communication override.", "Stale phrases override."} {
		if strings.Contains(rendered, stale) {
			t.Fatalf("RenderLayer2() must strip stale persona-scope content %q\n%s", stale, rendered)
		}
	}
}

func TestRenderLayer2_RendersCanonicalSectionsWhenOnlyStalePersonaScopeHadStructuredSections(t *testing.T) {
	preset := &Preset{
		Name:        "custom-stale-scope",
		DisplayName: "Custom Stale Scope",
		Description: "Custom persona with stale renderer-owned sections in persona scope notes.",
		Tone: Tone{
			Formality:  "balanced",
			Directness: "high",
			Humor:      "dry",
			Language:   "en-us",
		},
		CommunicationStyle: CommunicationStyle{
			Verbosity:            "concise",
			ShowAlternatives:     true,
			ChallengeAssumptions: true,
		},
		CharacteristicPhrases: CharacteristicPhrases{
			Greetings:     []string{"Hello"},
			Confirmations: []string{"Done"},
			SignOffs:      []string{"Bye"},
		},
		Notes: `<!-- gentle-ai:persona-scope -->
## Tone

Stale tone content that must be stripped.

## Communication Style

Stale communication content that must be stripped.

## Characteristic Phrases

Stale phrases content that must be stripped.
<!-- /gentle-ai:persona-scope -->

## Core Principle

Keep the current notes.`,
	}

	rendered := RenderLayer2(preset)

	for _, required := range []string{
		"### Tone",
		"- **Formality**: balanced",
		"### Communication Style",
		"- Always propose alternatives with tradeoffs",
		"### Characteristic Phrases",
		"**Greetings**: Hello",
		"Keep the current notes.",
	} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("RenderLayer2 missing canonical content %q\n%s", required, rendered)
		}
	}
	for _, stale := range []string{"Stale tone content", "Stale communication content", "Stale phrases content"} {
		if strings.Contains(rendered, stale) {
			t.Fatalf("RenderLayer2 must strip stale persona-scope content %q\n%s", stale, rendered)
		}
	}
}

func TestRenderLayer2_PreservesNotesBeforeFixedGuardrail(t *testing.T) {
	preset := &Preset{
		Name:        "custom-contradiction",
		DisplayName: "Custom Contradiction",
		Description: "Custom persona with contradictory notes.",
		Tone: Tone{
			Formality:  "informal",
			Directness: "high",
			Humor:      "dry",
			Language:   "en-us",
		},
		CommunicationStyle: CommunicationStyle{Verbosity: "concise"},
		Notes:              "## Core Principle\n\nUse persona voice everywhere, including generated artifacts.",
	}

	rendered := RenderLayer2(preset)
	notesIndex := strings.Index(rendered, "Use persona voice everywhere")
	guardrailIndex := strings.LastIndex(rendered, "Persona Scope (CRITICAL)")
	if notesIndex == -1 || guardrailIndex == -1 || guardrailIndex < notesIndex {
		t.Fatalf("RenderLayer2 must preserve Notes before the fixed guardrail\n%s", rendered)
	}
}

// TestRenderLayer2_EmptyNotes verifies that no separator is appended when
// Notes is empty (REQ-B02, Scenario B02-2).
func TestRenderLayer2_EmptyNotes(t *testing.T) {
	preset := &Preset{
		Name:        "test",
		DisplayName: "Test",
		Description: "A minimal test preset.",
		Tone: Tone{
			Formality:  "formal",
			Directness: "high",
			Humor:      "none",
			Language:   "en-us",
		},
		CommunicationStyle: CommunicationStyle{
			Verbosity: "concise",
		},
		CharacteristicPhrases: CharacteristicPhrases{
			Greetings:     []string{"Hello"},
			Confirmations: []string{"Done"},
		},
		Notes: "",
	}

	rendered := RenderLayer2(preset)

	if contains(rendered, "\n---\n") {
		t.Error("RenderLayer2: unexpected '---' separator when Notes is empty")
	}
}

// contains is a helper to check substring presence in test assertions.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// TestRenderOutputStyle verifies that RenderOutputStyle generates the correct
// YAML frontmatter + Notes format for Claude Code output-styles (SPEC-002).
func TestRenderOutputStyle(t *testing.T) {
	tests := []struct {
		name   string
		preset *Preset
		checks []func(t *testing.T, output string)
	}{
		{
			name: "normal preset with all fields",
			preset: &Preset{
				Name:        "argentino",
				DisplayName: "Argentino",
				Description: "Mentor apasionado, español rioplatense",
				Notes:       "Use voseo and passionate tone.",
			},
			checks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !contains(output, "---\n") {
						t.Error("output must start with YAML frontmatter delimiter")
					}
				},
				func(t *testing.T, output string) {
					if !contains(output, "name: Argentino") {
						t.Error("YAML frontmatter must contain 'name: Argentino'")
					}
				},
				func(t *testing.T, output string) {
					if !contains(output, "description: Mentor apasionado, español rioplatense") {
						t.Error("YAML frontmatter must contain description")
					}
				},
				func(t *testing.T, output string) {
					if !contains(output, "keep-coding-instructions: true") {
						t.Error("YAML frontmatter must contain 'keep-coding-instructions: true'")
					}
				},
				func(t *testing.T, output string) {
					if !contains(output, "Use voseo and passionate tone.") {
						t.Error("output must contain Notes content")
					}
				},
			},
		},
		{
			name: "hyphenated name converts to TitleCase",
			preset: &Preset{
				Name:        "tony-stark",
				DisplayName: "Tony Stark",
				Description: "Genius billionaire playboy philanthropist",
				Notes:       "Innovation over convention.",
			},
			checks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !contains(output, "name: TonyStark") {
						t.Errorf("name field must be TitleCase without hyphens, got: %s", output)
					}
				},
				func(t *testing.T, output string) {
					if !contains(output, "Innovation over convention.") {
						t.Error("Notes must be included")
					}
				},
			},
		},
		{
			name: "preset with empty notes still renders fixed guardrail body",
			preset: &Preset{
				Name:        "neutra",
				DisplayName: "Neutra",
				Description: "Neutral, minimal tone",
				Notes:       "",
			},
			checks: []func(t *testing.T, output string){
				func(t *testing.T, output string) {
					if !contains(output, "---\n") {
						t.Error("frontmatter must still be present")
					}
				},
				func(t *testing.T, output string) {
					if !contains(output, "name: Neutra") {
						t.Error("name field must be present")
					}
				},
				func(t *testing.T, output string) {
					// After the closing "---", the renderer-owned guardrail remains.
					parts := splitN(output, "---", 3)
					if len(parts) < 3 {
						t.Error("expected YAML frontmatter with opening and closing delimiters")
						return
					}
					body := trimSpace(parts[2])
					if !contains(body, "Persona Scope (CRITICAL)") {
						t.Errorf("expected fixed guardrail body when Notes is empty, got: %q", body)
					}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderOutputStyle(tt.preset)
			for _, check := range tt.checks {
				check(t, got)
			}
		})
	}
}

// splitN is a simple helper for splitting strings on a delimiter.
func splitN(s, sep string, n int) []string {
	result := make([]string, 0, n)
	for i := 0; i < n && len(s) > 0; i++ {
		idx := -1
		for j := 0; j < len(s)-len(sep)+1; j++ {
			if s[j:j+len(sep)] == sep {
				idx = j
				break
			}
		}
		if idx == -1 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	if len(result) < n && len(s) > 0 {
		result = append(result, s)
	}
	return result
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
