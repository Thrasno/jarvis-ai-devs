package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/tui"
)

type personaFlowSnapshot struct {
	personaPreset       string
	personaPresetSource string
	selectedSkills      []string
	agentsMD            string
}

func TestWizardAndPersonaSet_Parity_FinalConfigAndArtifacts(t *testing.T) {
	targetPreset := "neutra"

	wizardHome := t.TempDir()
	wizardSnapshot := runWizardNoTUIAndCaptureSnapshot(t, wizardHome, targetPreset)

	personaSetHome := t.TempDir()
	personaSetSnapshot := runPersonaSetAndCaptureSnapshot(t, personaSetHome, targetPreset, wizardSnapshot.selectedSkills)

	if wizardSnapshot.personaPreset != personaSetSnapshot.personaPreset {
		t.Fatalf("persona_preset mismatch: wizard=%q persona-set=%q", wizardSnapshot.personaPreset, personaSetSnapshot.personaPreset)
	}
	if wizardSnapshot.personaPresetSource != personaSetSnapshot.personaPresetSource {
		t.Fatalf("persona_preset_source mismatch: wizard=%q persona-set=%q", wizardSnapshot.personaPresetSource, personaSetSnapshot.personaPresetSource)
	}
	if wizardSnapshot.agentsMD != personaSetSnapshot.agentsMD {
		t.Fatalf("AGENTS.md mismatch between wizard and persona set")
	}
}

func runWizardNoTUIAndCaptureSnapshot(t *testing.T, home string, targetPreset string) personaFlowSnapshot {
	t.Helper()
	setTestHome(t, home)
	t.Setenv("PATH", "")

	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatalf("create opencode dir for wizard flow: %v", err)
	}

	seedPersonaManifest(t, targetPreset, nil)

	input := "\n\n\n\n\nyes\nI ACKNOWLEDGE\n"
	if err := runWithStdinString(input, func() error {
		return tui.RunNoTUI(tui.WizardConfig{
			PersonaFS:  jarvis.PersonaFS,
			SkillsFS:   jarvis.SkillsFS,
			TemplateFS: jarvis.TemplatesFS,
		})
	}); err != nil {
		t.Fatalf("RunNoTUI: %v", err)
	}

	return readSnapshot(t, home)
}

func runPersonaSetAndCaptureSnapshot(t *testing.T, home string, targetPreset string, selectedSkills []string) personaFlowSnapshot {
	t.Helper()
	setTestHome(t, home)
	t.Setenv("PATH", "")

	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatalf("create opencode dir for persona set flow: %v", err)
	}

	seedPersonaManifest(t, "argentino", selectedSkills)

	if err := personaSetCmd.RunE(personaSetCmd, []string{targetPreset}); err != nil {
		t.Fatalf("persona set %q: %v", targetPreset, err)
	}

	return readSnapshot(t, home)
}

// seedPersonaManifest records the starting persona and skills in
// ~/.jarvis/state.yaml, which owns both.
func seedPersonaManifest(t *testing.T, persona string, selectedSkills []string) {
	t.Helper()
	seed := state.New()
	seed.Persona = persona
	seed.PersonaSource = state.PersonaSourceBuiltin
	seed.Skills = append([]string(nil), selectedSkills...)
	if err := state.Save(seed); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
}

func readSnapshot(t *testing.T, home string) personaFlowSnapshot {
	t.Helper()

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	agentsBody, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read %s: %v", agentsPath, err)
	}

	persona, source := manifest.ResolvedPersona()
	return personaFlowSnapshot{
		personaPreset:       persona,
		personaPresetSource: string(source),
		selectedSkills:      append([]string(nil), manifest.Skills...),
		agentsMD:            string(agentsBody),
	}
}

func runWithStdinString(input string, fn func() error) error {
	tmp, err := os.CreateTemp("", "jarvis-stdin-*.txt")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	defer func() { _ = tmp.Close() }()

	if _, err := io.Copy(tmp, bytes.NewBufferString(input)); err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	original := os.Stdin
	os.Stdin = tmp
	defer func() { os.Stdin = original }()

	return fn()
}
