package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
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

	w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy pipe: %v", err)
	}
	r.Close()
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
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := runWizard(true)
	if err != nil {
		t.Fatalf("runWizard with no config should not fail: %v", err)
	}
}

// TestSyncCmd_RunE_InProcess exercises the syncCmd RunE handler directly.
func TestSyncCmd_RunE_InProcess(t *testing.T) {
	out := captureStdout(t, func() {
		if err := syncCmd.RunE(syncCmd, nil); err != nil {
			t.Errorf("syncCmd.RunE: %v", err)
		}
	})

	if !strings.Contains(out, "hive-daemon") {
		t.Errorf("expected 'hive-daemon' in sync output:\n%s", out)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// configCmd tests
// ──────────────────────────────────────────────────────────────────────────────

// TestRunConfigView_InProcess verifies configCmd prints all config fields.
func TestRunConfigView_InProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCfg(t, home, "preset: argentino\napi_url: https://hivemem.dev\nemail: user@example.com\n")

	out := captureStdout(t, func() {
		if err := configSetCmd.RunE(configSetCmd, []string{"preset", "neutra"}); err != nil {
			t.Errorf("configSetCmd.RunE: %v", err)
		}
	})

	if !strings.Contains(out, "neutra") {
		t.Errorf("expected 'neutra' in output:\n%s", out)
	}

	// Verify the file was actually updated.
	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("Load after set: %v", loadErr)
	}
	if cfg.Preset != "neutra" {
		t.Errorf("expected preset=neutra in saved config, got %q", cfg.Preset)
	}
}

// TestRunConfigSet_APIUrl_InProcess verifies configSetCmd updates the api_url key.
func TestRunConfigSet_APIUrl_InProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
// Verifies .jarvis/skill-registry.md is created and the commit reminder is printed.
func TestRunInit_InProcess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "") // prevent git from resolving a remote

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
	if _, err := os.Stat(registryPath); err != nil {
		t.Errorf("expected .jarvis/skill-registry.md to exist: %v", err)
	}

	// Verify CLI output contains the commit reminder.
	if !strings.Contains(out, "commit .jarvis/") {
		t.Errorf("expected commit reminder in output:\n%s", out)
	}

	// Verify stack and skills appear in output.
	if !strings.Contains(out, "Go") {
		t.Errorf("expected 'Go' stack in output:\n%s", out)
	}
	if !strings.Contains(out, "go-testing") {
		t.Errorf("expected 'go-testing' skill in output:\n%s", out)
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
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "") // prevent opencode binary detection

	input := "\n\n\n\n\nyes\n" // scope, persona, 3 skill prompts, apply=yes
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		r.Close()
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
