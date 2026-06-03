package agent

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
)

// TestAgentInterfaceExtension verifies that the Agent interface includes
// the output-style methods required by SPEC-001.
func TestAgentInterfaceExtension(t *testing.T) {
	// This test ensures the interface has been extended correctly.
	// It will fail to compile if the methods don't exist on the interface.

	var _ interface {
		SupportsOutputStyles() bool
		WriteOutputStyle(*persona.Preset) error
		ClearOutputStyle(string) error
	} = (Agent)(nil)

	// If we reach here, the interface has the required methods
	t.Log("Agent interface has SupportsOutputStyles and WriteOutputStyle methods")
}

func TestRenderOutputStyle_IncludesFixedStructuralGuardrails(t *testing.T) {
	preset := &persona.Preset{
		Name:        "custom-example",
		DisplayName: "Custom Example",
		Description: "Custom output style",
		Notes:       "<!-- gentle-ai:persona-scope -->\n## Persona Scope (CRITICAL)\n\nOld output-style scope text.\n\n## Response Length Contract\n\nUse long answers by default.\n\n## Language Rules\n\nUse persona language in generated artifacts.\n<!-- /gentle-ai:persona-scope -->\n\n## Core Principle\n\nKeep the active persona's voice in direct replies. Use persona voice everywhere, including generated artifacts.\n\n## Persona Scope (CRITICAL)\n\nSecond old output-style scope text.",
	}

	output := persona.RenderOutputStyle(preset)

	for _, required := range []string{
		"keep-coding-instructions: true",
		"Persona Scope (CRITICAL)",
		"Response Length Contract",
		"Language Rules",
		"When Asking Questions",
		"STOP",
		"code, identifiers, variable names, function names, comments",
		"UI labels, UI copy, error messages",
		"documentation, README files, commit messages, PR descriptions",
		"string literals",
		"Keep the active persona's voice in direct replies.",
	} {
		if !strings.Contains(output, required) {
			t.Fatalf("RenderOutputStyle missing %q\n%s", required, output)
		}
	}

	if got := strings.Count(output, "Persona Scope (CRITICAL)"); got != 1 {
		t.Fatalf("RenderOutputStyle Persona Scope marker count = %d, want 1\n%s", got, output)
	}
	if got := strings.Count(output, "<!-- gentle-ai:persona-scope -->"); got != 1 {
		t.Fatalf("RenderOutputStyle persona-scope start marker count = %d, want 1\n%s", got, output)
	}
	if got := strings.Count(output, "<!-- /gentle-ai:persona-scope -->"); got != 1 {
		t.Fatalf("RenderOutputStyle persona-scope end marker count = %d, want 1\n%s", got, output)
	}
	for _, stale := range []string{"Old output-style scope text", "Second old output-style scope text", "Use long answers by default.", "Use persona language in generated artifacts."} {
		if strings.Contains(output, stale) {
			t.Fatalf("RenderOutputStyle must strip stale persona scope content %q\n%s", stale, output)
		}
	}

	notesIndex := strings.Index(output, "Use persona voice everywhere")
	guardrailIndex := strings.LastIndex(output, "Persona Scope (CRITICAL)")
	if notesIndex == -1 || guardrailIndex == -1 {
		t.Fatalf("RenderOutputStyle missing notes or guardrail\n%s", output)
	}
	if guardrailIndex < notesIndex {
		t.Fatalf("RenderOutputStyle guardrail must follow preset notes so fixed scope rules have precedence\n%s", output)
	}
}
