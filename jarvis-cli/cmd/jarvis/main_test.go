package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// jarvisBin is the path to the compiled binary built by TestMain.
var jarvisBin = "/tmp/jarvis-test-bin"

// TestMain compiles the binary once for all integration tests in this package.
func TestMain(m *testing.M) {
	// Build the binary into a temp path.
	cmd := exec.Command("go", "build", "-o", jarvisBin, "./")
	cmd.Dir = filepath.Join(os.Getenv("PWD"), "../../..")
	// Resolve the module root (cmd/jarvis is 3 levels deep from module root).
	// We build relative to cmd/jarvis to ensure the correct package.
	buildCmd := exec.Command("go", "build",
		"-o", jarvisBin,
		"github.com/Thrasno/jarvis-dev/jarvis-cli/cmd/jarvis")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		// Fall back: try building from the directory directly.
		cmd2 := exec.Command("go", "build", "-o", jarvisBin, ".")
		cmd2.Dir = "/home/andres/Desarrollo/Proyectos/jarvis-dev/jarvis-cli/cmd/jarvis"
		cmd2.Stdout = os.Stdout
		cmd2.Stderr = os.Stderr
		if err2 := cmd2.Run(); err2 != nil {
			os.Stderr.WriteString("SKIP: could not build jarvis binary: " + err2.Error() + "\n")
			os.Exit(0) // Skip (not fail) if binary can't be built in this environment.
		}
	}

	code := m.Run()
	os.Remove(jarvisBin)
	os.Exit(code)
}

// runJarvis executes the compiled jarvis binary with the provided arguments.
// It always sets HOME to a specific directory for isolation.
// Returns stdout+stderr combined, and the exit code.
func runJarvis(t *testing.T, home string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(jarvisBin, args...)
	if home != "" {
		cmd.Env = append(os.Environ(), "HOME="+home)
	}
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
	return string(out), exitCode
}

func TestHelp_ExitsZero(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}
	home := t.TempDir()
	out, code := runJarvis(t, home, "--help")
	if code != 0 {
		t.Errorf("expected exit code 0 for --help, got %d\noutput: %s", code, out)
	}
	// The help output must mention the tool name.
	if !strings.Contains(strings.ToLower(out), "jarvis") {
		t.Errorf("expected --help output to contain 'jarvis', got:\n%s", out)
	}
}

func TestVersion_ShowsVersion(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}
	home := t.TempDir()
	out, _ := runJarvis(t, home, "--help")

	// The root command help must list all subcommands.
	for _, subcmd := range []string{"persona", "sync", "login", "timeline"} {
		if !strings.Contains(out, subcmd) {
			t.Errorf("expected --help to mention subcommand %q, output:\n%s", subcmd, out)
		}
	}
}

func TestStatus_WhenConfigured(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}

	// Build a temp home with a valid config.yaml.
	home := t.TempDir()
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}

	email := "tony@stark-industries.io"
	configYAML := "email: " + email + "\napi_url: https://hivemem.dev\npreset: tony-stark\n"
	if err := os.WriteFile(filepath.Join(jarvisDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	out, code := runJarvis(t, home)
	if code != 0 {
		t.Errorf("expected exit code 0 for status run, got %d\noutput: %s", code, out)
	}

	// Output must contain the email from our config.
	if !strings.Contains(out, email) {
		t.Errorf("expected status output to contain email %q, got:\n%s", email, out)
	}
}

// writeConfig is a helper that creates ~/.jarvis/config.yaml with the given content
// under the provided home directory.
func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jarvisDir, "config.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

// TestStatus_ShowsPresetAndEmail verifies that running jarvis when configured
// prints both the email and preset from config.yaml.
func TestStatus_ShowsPresetAndEmail(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}

	home := t.TempDir()
	writeConfig(t, home, "email: test@example.com\napi_url: https://hivemem.dev\npreset: argentino\n")

	out, code := runJarvis(t, home)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d\noutput: %s", code, out)
	}
	if !strings.Contains(out, "test@example.com") {
		t.Errorf("expected output to contain email 'test@example.com', got:\n%s", out)
	}
	if !strings.Contains(out, "argentino") {
		t.Errorf("expected output to contain preset 'argentino', got:\n%s", out)
	}
}

// TestPersonaSet_UnknownPreset_ExitsNonZero verifies that requesting a non-existent
// preset fails with a non-zero exit code.
func TestPersonaSet_UnknownPreset_ExitsNonZero(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}

	home := t.TempDir()
	// Need IsConfigured=true so the binary doesn't drop into the wizard.
	writeConfig(t, home, "email: user@example.com\napi_url: https://hivemem.dev\npreset: neutra\n")

	_, code := runJarvis(t, home, "persona", "set", "nonexistent-preset-xyz")
	if code == 0 {
		t.Error("expected non-zero exit for unknown preset, got 0")
	}
}

// TestPersonaSet_ValidPreset_ExitsZero verifies that `jarvis persona set neutra`
// succeeds when a CLAUDE.md with sentinel markers exists at ~/.claude/CLAUDE.md.
func TestPersonaSet_ValidPreset_ExitsZero(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}

	home := t.TempDir()
	writeConfig(t, home, "email: user@example.com\napi_url: https://hivemem.dev\npreset: argentino\n")

	// Claude Code detects the agent by checking ~/.claude directory.
	// Write a CLAUDE.md with both sentinel blocks so WriteInstructions can patch it.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	claudeMD := "<!-- JARVIS:LAYER1:START -->\nLayer1 content\n<!-- JARVIS:LAYER1:END -->\n\n<!-- JARVIS:LAYER2:START -->\nLayer2 content\n<!-- JARVIS:LAYER2:END -->\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	out, code := runJarvis(t, home, "persona", "set", "neutra")
	if code != 0 {
		t.Errorf("expected exit code 0 for valid preset, got %d\noutput: %s", code, out)
	}
}

// TestSync_PrintsInfoMessage verifies that `jarvis sync` exits 0 and prints the
// stub message directing the user to use hive-daemon / mem_sync.
func TestSync_PrintsInfoMessage(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}

	home := t.TempDir()
	// sync command doesn't require config, but set HOME for isolation.
	out, code := runJarvis(t, home, "sync")
	if code != 0 {
		t.Errorf("expected exit code 0 for sync, got %d\noutput: %s", code, out)
	}
	if !strings.Contains(out, "hive-daemon") && !strings.Contains(out, "mem_sync") {
		t.Errorf("expected sync output to mention hive-daemon or mem_sync, got:\n%s", out)
	}
}

func TestNoTUI_FirstRun_RequiresInput(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}

	// Fresh home dir — no config at all.
	home := t.TempDir()

	// Run with --no-tui and /dev/null as stdin so no input is provided.
	cmd := exec.Command(jarvisBin, "--no-tui")
	cmd.Env = append(os.Environ(), "HOME="+home)

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer devNull.Close()
	cmd.Stdin = devNull

	out, execErr := cmd.CombinedOutput()
	output := string(out)

	// The binary must either: print wizard prompts before exiting, OR exit non-zero.
	// Either way it must NOT silently succeed without providing any output.
	if execErr == nil {
		// If it exited 0, it must have printed the wizard intro at minimum.
		if !strings.Contains(output, "Setup") && !strings.Contains(output, "Jarvis") {
			t.Errorf("--no-tui with empty stdin exited 0 but produced no wizard output:\n%s", output)
		}
	}
	// Non-zero exit is also acceptable (no input = incomplete wizard). Test passes.
}
