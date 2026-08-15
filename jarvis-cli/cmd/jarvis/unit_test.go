package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// captureStdout redirects os.Stdout to a pipe, runs fn, then restores stdout
// and returns the captured output as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer pipe: %v", err)
	}
	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader pipe: %v", err)
	}
	return buf.String()
}

// writeCfg writes a minimal config.yaml under home/.jarvis/.
func writeCfg(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

// TestRunWizard_InProcess verifies root flow uses wizard semantics.
func TestRunWizard_InProcess(t *testing.T) {
	home := isolateTestHome(t)

	writeCfg(t, home, "email: inprocess@example.com\napi_url: https://hivemem.dev\npreset: neutra\n")

	out := captureStdout(t, func() {
		if err := runWizard(true); err != nil {
			t.Errorf("runWizard returned error: %v", err)
		}
	})

	if !strings.Contains(out, "Setup") {
		t.Errorf("expected wizard setup output, got:\n%s", out)
	}
}

// TestRunWizard_ConfiguredAgents_InProcess verifies wizard still runs for reruns.
func TestRunWizard_ConfiguredAgents_InProcess(t *testing.T) {
	home := isolateTestHome(t)

	writeCfg(t, home, "email: a@b.com\napi_url: https://hivemem.dev\npreset: tony-stark\nconfigured_agents:\n  - claude-code\n  - opencode\n")

	out := captureStdout(t, func() {
		if err := runWizard(true); err != nil {
			t.Errorf("runWizard: %v", err)
		}
	})

	if !strings.Contains(out, "Setup") {
		t.Errorf("expected setup wizard output:\n%s", out)
	}
}

// TestRunWizard_NoConfig_InProcess verifies runWizard works from fresh state.
func TestRunWizard_NoConfig_InProcess(t *testing.T) {
	isolateTestHome(t)
	err := runWizard(true)
	if err != nil {
		t.Fatalf("runWizard with no config should not fail: %v", err)
	}
}

// TestSyncCmd_RunE_MutatesNothingWithoutADesiredState replaces the old no-op
// message assertion: preflight must fail before anything is planned or written,
// and must not fall back to exiting 0 with advice about Hive's memory sync.
func TestSyncCmd_RunE_MutatesNothingWithoutADesiredState(t *testing.T) {
	home := isolateTestHome(t)
	var err error
	out := captureStdout(t, func() { err = syncCmd.RunE(syncCmd, nil) })

	if err == nil {
		t.Fatal("expected sync to fail with no desired-state manifest")
	}
	if strings.Contains(out, "hive-daemon") || strings.Contains(out, "no-op") {
		t.Errorf("sync still prints the retired no-op message:\n%s", out)
	}
	if entries, readErr := os.ReadDir(filepath.Join(home, ".claude")); readErr == nil && len(entries) > 0 {
		t.Errorf("preflight failure must mutate nothing, found %d entries under .claude", len(entries))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// configCmd tests
// ──────────────────────────────────────────────────────────────────────────────

// TestRunConfigView_InProcess verifies configCmd prints all config fields.
func TestRunConfigView_InProcess(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "preset: tony-stark\napi_url: https://hivemem.dev\nemail: user@example.com\nconfigured_agents:\n  - claude\nversion: 1.0.0\n")

	out := captureStdout(t, func() {
		if err := configCmd.RunE(configCmd, nil); err != nil {
			t.Errorf("configCmd.RunE: %v", err)
		}
	})

	for _, want := range []string{"tony-stark", "hivemem.dev", "user@example.com", "claude", "1.0.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

// TestRunConfigView_NoConfig_InProcess verifies configCmd works even without a config file.
func TestRunConfigView_NoConfig_InProcess(t *testing.T) {
	isolateTestHome(t)
	// No config file — should show defaults without error.
	out := captureStdout(t, func() {
		if err := configCmd.RunE(configCmd, nil); err != nil {
			t.Errorf("configCmd.RunE with no config: %v", err)
		}
	})
	if !strings.Contains(out, "Current configuration") {
		t.Errorf("expected header in output:\n%s", out)
	}
}

// TestRunConfigSet_Preset_InProcess verifies configSetCmd updates the preset key.
func TestRunConfigSet_Preset_InProcess(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "preset: argentino\napi_url: https://hivemem.dev\nemail: user@example.com\n")

	out := captureStdout(t, func() {
		if err := configSetCmd.RunE(configSetCmd, []string{"preset", "neutra"}); err != nil {
			t.Errorf("configSetCmd.RunE: %v", err)
		}
	})

	if !strings.Contains(out, "neutra") {
		t.Errorf("expected 'neutra' in output:\n%s", out)
	}

	// ~/.jarvis/state.yaml owns the persona, so that is where the new value has
	// to land.
	manifest, loadErr := state.Load()
	if loadErr != nil {
		t.Fatalf("load manifest after set: %v", loadErr)
	}
	if manifest.Persona != "neutra" {
		t.Errorf("expected persona=neutra in the manifest, got %q", manifest.Persona)
	}
}

func TestRunConfigSet_Preset_Valid_DoesNotApplyAgentsOrMutateArtifacts(t *testing.T) {
	home := isolateTestHome(t)

	writeCfg(t, home, "persona_preset: neutra\npersona_preset_source: builtin\npreset: neutra\napi_url: https://hivemem.dev\n")

	agentsPath := filepath.Join(home, ".claude", "CLAUDE.md")
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	outputStylePath := filepath.Join(home, ".claude", "output-styles", "Neutra.md")

	if err := os.MkdirAll(filepath.Dir(outputStylePath), 0o755); err != nil {
		t.Fatalf("mkdir output-styles dir: %v", err)
	}

	const beforeInstructions = "# Existing instructions\n"
	const beforeSettings = `{"outputStyle":"Neutra"}`
	const beforeOutputStyle = "# Existing style\n"

	if err := os.WriteFile(agentsPath, []byte(beforeInstructions), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(beforeSettings), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	if err := os.WriteFile(outputStylePath, []byte(beforeOutputStyle), 0o644); err != nil {
		t.Fatalf("write output-style file: %v", err)
	}

	if err := configSetCmd.RunE(configSetCmd, []string{"preset", "argentino"}); err != nil {
		t.Fatalf("configSetCmd.RunE valid preset: %v", err)
	}

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "CLAUDE instructions", path: agentsPath, want: beforeInstructions},
		{name: "settings", path: settingsPath, want: beforeSettings},
		{name: "output-style", path: outputStylePath, want: beforeOutputStyle},
	} {
		content, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read %s: %v", tc.name, err)
		}
		if string(content) != tc.want {
			t.Fatalf("%s was mutated by config set preset; got %q want %q", tc.name, string(content), tc.want)
		}
	}

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest after valid preset set: %v", err)
	}
	if manifest.Persona != "argentino" {
		t.Fatalf("expected canonical persona argentino, got %q", manifest.Persona)
	}
	if manifest.PersonaSource != state.PersonaSourceBuiltin {
		t.Fatalf("expected builtin source, got %q", manifest.PersonaSource)
	}
}

func TestRunConfigSet_Preset_InvalidSlug_DoesNotMutateState(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "persona_preset: neutra\npersona_preset_source: builtin\npreset: neutra\napi_url: https://hivemem.dev\n")

	err := configSetCmd.RunE(configSetCmd, []string{"preset", "preset-inexistente"})
	if err == nil {
		t.Fatal("expected error for invalid preset slug")
	}

	// The rejection happens before anything is written, so the manifest must not
	// exist yet and config.yaml must still carry the original persona.
	if _, loadErr := state.Load(); loadErr == nil {
		t.Fatal("a rejected preset must not create a manifest")
	}
	raw, readErr := os.ReadFile(filepath.Join(home, ".jarvis", "config.yaml"))
	if readErr != nil {
		t.Fatalf("read config.yaml after failed set: %v", readErr)
	}
	if !strings.Contains(string(raw), "persona_preset: neutra") {
		t.Fatalf("persona mutated on failure:\n%s", raw)
	}
}

func TestRunConfigSet_Preset_RejectsLegacyCustomProfileWithMigrationGuidance(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "persona_preset: neutra\npersona_preset_source: builtin\npreset: neutra\napi_url: https://hivemem.dev\n")
	legacyPath := filepath.Join(home, ".jarvis", "personas", "legacy-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy preset dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n"), 0o644); err != nil {
		t.Fatalf("write legacy preset: %v", err)
	}

	err := runConfigSet("preset", "legacy-custom")
	if err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("runConfigSet() error = %v, want actionable migration guidance", err)
	}

	if _, err := state.Load(); err == nil {
		t.Fatal("a rejected legacy custom profile must not create a manifest")
	}
	raw, readErr := os.ReadFile(filepath.Join(home, ".jarvis", "config.yaml"))
	if readErr != nil {
		t.Fatalf("read config.yaml after failed preset set: %v", readErr)
	}
	if !strings.Contains(string(raw), "persona_preset: neutra") {
		t.Fatalf("legacy custom rejection mutated the recorded persona:\n%s", raw)
	}
}

// TestRunConfigSet_APIUrl_InProcess verifies configSetCmd updates the api_url key.
func TestRunConfigSet_APIUrl_InProcess(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "preset: neutra\napi_url: https://hivemem.dev\nemail: user@example.com\n")

	out := captureStdout(t, func() {
		if err := configSetCmd.RunE(configSetCmd, []string{"api_url", "https://custom.hive.dev"}); err != nil {
			t.Errorf("configSetCmd.RunE: %v", err)
		}
	})
	if !strings.Contains(out, "custom.hive.dev") {
		t.Errorf("expected updated URL in output:\n%s", out)
	}
}

// TestRunConfigSet_Email_InProcess verifies configSetCmd updates the email key.
func TestRunConfigSet_Email_InProcess(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "preset: neutra\napi_url: https://hivemem.dev\nemail: old@example.com\n")

	captureStdout(t, func() {
		if err := configSetCmd.RunE(configSetCmd, []string{"email", "new@example.com"}); err != nil {
			t.Errorf("configSetCmd.RunE: %v", err)
		}
	})

	cfg, _ := config.Load()
	if cfg.Email != "new@example.com" {
		t.Errorf("expected email=new@example.com, got %q", cfg.Email)
	}
}

// TestRunConfigSet_InvalidKey_InProcess verifies configSetCmd returns error for unknown keys.
func TestRunConfigSet_InvalidKey_InProcess(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, "preset: neutra\napi_url: https://hivemem.dev\nemail: user@example.com\n")

	err := configSetCmd.RunE(configSetCmd, []string{"version", "2.0.0"})
	if err == nil {
		t.Fatal("expected error for read-only key 'version'")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("expected 'unknown key' in error, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// initCmd tests
// ──────────────────────────────────────────────────────────────────────────────

// TestRunInit_InProcess calls runInit() directly with a temp project dir.
// Verifies .jarvis/skill-registry.md is created and local cache guidance is printed.
func TestRunInit_InProcess(t *testing.T) {
	dir := initCommandGitWorktree(t)

	// Create a go.mod so the Go stack is detected.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runInit(dir); err != nil {
			t.Errorf("runInit: %v", err)
		}
	})

	// Verify the registry file was created.
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	registryData, err := os.ReadFile(registryPath)
	if err != nil {
		t.Errorf("expected .jarvis/skill-registry.md to exist: %v", err)
	}
	registry := string(registryData)
	// Verify structural elements of the registry file format.
	for _, want := range []string{
		"## Installed Skills",
		"| Trigger | Skill | Scope | Path |",
		"## Project Conventions",
		"Canonical registry path: `.jarvis/skill-registry.md`",
		// go-testing SKILL.md is scanned from disk; verify its row appears.
		"go-testing",
	} {
		if !strings.Contains(registry, want) {
			t.Fatalf("expected rich registry content %q, got:\n%s", want, registry)
		}
	}
	for _, forbidden := range []string{
		"**Stack**",
		"## Suggested Skills",
		"## Compact Rules",
		"| Skill | Trigger / Description | Scope | Path |",
		"| Skill | Trigger | Path | Type |",
		"| Go Testing | When writing Go tests, using teatest, or adding test coverage | `go-testing/SKILL.md` | optional |",
		"| Go Testing | When writing Go tests, using teatest, or adding test coverage — Go testing patterns including Bubbletea TUI testing | optional | `go-testing/SKILL.md` |",
	} {
		if strings.Contains(registry, forbidden) {
			t.Fatalf("registry must not use legacy compact-rule-primary schema %q, got:\n%s", forbidden, registry)
		}
	}

	for _, path := range []string{
		filepath.Join(dir, ".jarvis", "skills", "go-testing", "SKILL.md"),
		filepath.Join(dir, ".jarvis", "skills", "sdd-apply", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated registry path to be loadable at %s: %v", path, err)
		}
	}
	skillImproverPath := filepath.Join(dir, ".jarvis", "skills", "skill-improver", "SKILL.md")
	skillImproverData, err := os.ReadFile(skillImproverPath)
	if err != nil {
		t.Fatalf("expected generated registry path to be loadable at %s: %v", skillImproverPath, err)
	}
	if strings.TrimSpace(string(skillImproverData)) == "" {
		t.Fatalf("expected copied skill-improver skill at %s to be non-empty", skillImproverPath)
	}
	embeddedSkillImprover, err := jarvis.SkillsFS.ReadFile("embed/skills/skill-improver/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded skill-improver skill: %v", err)
	}
	if !bytes.Equal(skillImproverData, embeddedSkillImprover) {
		t.Fatalf("expected copied skill-improver skill to match embedded content")
	}

	// Verify CLI output describes the generated local cache behavior.
	if !strings.Contains(out, ".jarvis/ generated cache is gitignored by default") {
		t.Errorf("expected generated cache guidance in output:\n%s", out)
	}
	if strings.Contains(out, "commit .jarvis/") {
		t.Errorf("init output must not tell users to commit generated .jarvis cache:\n%s", out)
	}

	// Verify init success messages appear in output.
	for _, want := range []string{"Skill registry created", "Skills:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in init output:\n%s", want, out)
		}
	}
}

func TestRunInitRejectsSymlinkedJarvisBeforeWritingSkillCopies(t *testing.T) {
	dir := initCommandGitWorktree(t)

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/symlink\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	externalJarvis := filepath.Join(t.TempDir(), "external-jarvis")
	if err := os.Mkdir(externalJarvis, 0755); err != nil {
		t.Fatalf("mkdir external jarvis: %v", err)
	}
	if err := os.Symlink(externalJarvis, filepath.Join(dir, ".jarvis")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := runInit(dir)
	if err == nil {
		t.Fatal("expected runInit to reject symlinked .jarvis")
	}
	if !strings.Contains(err.Error(), "install project skill copies") || !strings.Contains(err.Error(), "refusing to follow symlink") {
		t.Fatalf("expected symlink rejection while installing skill copies, got: %v", err)
	}

	externalSkills := filepath.Join(externalJarvis, "skills")
	if _, err := os.Stat(externalSkills); !os.IsNotExist(err) {
		t.Fatalf("expected no skill copies written outside project through symlink, stat err: %v", err)
	}
}

func TestInitCmdRunEUsesCurrentWorkingDirectory(t *testing.T) {
	dir := initCommandGitWorktree(t)

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/initcmd\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to temp project: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	out := captureStdout(t, func() {
		if err := initCmd.RunE(initCmd, nil); err != nil {
			t.Errorf("initCmd.RunE: %v", err)
		}
	})

	registryData, err := os.ReadFile(filepath.Join(dir, ".jarvis", "skill-registry.md"))
	if err != nil {
		t.Fatalf("expected init command to write registry in current directory: %v", err)
	}
	registry := string(registryData)
	if !strings.Contains(registry, "Canonical registry path: `.jarvis/skill-registry.md`") {
		t.Fatalf("expected canonical registry metadata from init command, got:\n%s", registry)
	}
	if !strings.Contains(out, "✓ Skill registry created: .jarvis/skill-registry.md") {
		t.Fatalf("expected init command success output, got:\n%s", out)
	}
}

// TestRunWizard_NoTUI_SkipsAuth exercises runWizard via stdin injection.
//
// In test environments stdin is NOT a TTY, so runWizard always calls RunNoTUI
// (regardless of the noTUI flag). We inject 6 readline responses:
//
//  1. HiveCloud: email → empty (skip cloud auth)
//  2. Persona:   choice → empty (default preset 0)
//     3-6. 4 optional skills → empty (default N — decline install)
//
// No agents are detected because HOME is a fresh tmpdir with no .claude or opencode dirs.
func TestRunWizard_NoTUI_SkipsAuth(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "") // prevent opencode binary detection

	input := "\n\n\n\n\nyes\n" // scope, persona, 3 skill prompts, apply=yes
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer pipe: %v", err)
	}

	origStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		if err := r.Close(); err != nil {
			t.Fatalf("close reader pipe: %v", err)
		}
	}()

	out := captureStdout(t, func() {
		if err := runWizard(false); err != nil {
			t.Errorf("runWizard returned error: %v", err)
		}
	})

	if !strings.Contains(out, "Configuration applied successfully") {
		t.Errorf("expected apply confirmation in output, got:\n%s", out)
	}
}
