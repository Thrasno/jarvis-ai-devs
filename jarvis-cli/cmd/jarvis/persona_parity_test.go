package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/tui"
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
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatalf("create opencode dir for wizard flow: %v", err)
	}

	seed := &config.AppConfig{
		PersonaPreset:       targetPreset,
		PersonaPresetSource: "builtin",
		Preset:              targetPreset,
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("seed config for wizard flow: %v", err)
	}

	input := "\n\n\n\n\nyes\n"
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
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatalf("create opencode dir for persona set flow: %v", err)
	}

	seed := &config.AppConfig{
		PersonaPreset:       "argentino",
		PersonaPresetSource: "builtin",
		SelectedSkills:      append([]string(nil), selectedSkills...),
		Preset:              "argentino",
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("seed config for persona set flow: %v", err)
	}

	if err := personaSetCmd.RunE(personaSetCmd, []string{targetPreset}); err != nil {
		t.Fatalf("persona set %q: %v", targetPreset, err)
	}

	return readSnapshot(t, home)
}

func readSnapshot(t *testing.T, home string) personaFlowSnapshot {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	agentsBody, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read %s: %v", agentsPath, err)
	}

	return personaFlowSnapshot{
		personaPreset:       cfg.PersonaPreset,
		personaPresetSource: cfg.PersonaPresetSource,
		selectedSkills:      append([]string(nil), cfg.SelectedSkills...),
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
