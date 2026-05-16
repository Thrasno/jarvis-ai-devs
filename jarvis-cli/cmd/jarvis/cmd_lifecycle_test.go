package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/lifecycle"
	"github.com/spf13/cobra"
)

func TestLifecycleCommands_AreWiredInRoot(t *testing.T) {
	t.Helper()

	for _, name := range []string{"verify", "doctor", "reconcile", "backup", "restore", "uninstall"} {
		if cmd, _, err := rootCmd.Find([]string{name}); err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("expected command %q to be wired in root (cmd=%v err=%v)", name, cmd, err)
		}
	}
}

func TestRunReconcileDryRun_RendersDoctorDerivedPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll .claude: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("provider", "claude", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("yes", false, "")
	if err := cmd.Flags().Set("provider", "claude"); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runReconcile(cmd, nil)
	})
	if runErr != nil {
		t.Fatalf("runReconcile returned error: %v", runErr)
	}

	for _, want := range []string{
		"dry-run: reconcile plan generated (no mutations)",
		"provider=claude",
		"read_only=true",
		"check_key=artifact.instructions.present",
		"reason_code=managed_artifact_missing",
		"class=owned",
		"safety_class=auto-safe",
		"safe_to_auto_apply=true",
		"next_action=restore managed artifact from Jarvis managed runtime state",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDoctor_RendersAdditiveStructuredDiagnosis(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll .claude: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("provider", "claude", "")
	if err := cmd.Flags().Set("provider", "claude"); err != nil {
		t.Fatalf("set provider: %v", err)
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})
	if runErr != nil {
		t.Fatalf("runDoctor returned error: %v", runErr)
	}

	for _, want := range []string{
		"doctor: read-only diagnosis",
		"provider=claude",
		"read_only=true",
		"reason_code=managed_artifact_missing",
		"next_action=restore managed artifact from Jarvis managed runtime state",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestLifecycleCommandValidation(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll .claude: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "verify rejects unknown provider", args: []string{"verify", "--provider", "invalid"}, wantErr: true},
		{name: "restore requires snapshot", args: []string{"restore", "--provider", "claude"}, wantErr: true},
		{name: "restore requires confirmation", args: []string{"restore", "--provider", "claude", "--snapshot", "snap-1"}, wantErr: true},
		{name: "reconcile requires confirmation", args: []string{"reconcile", "--provider", "claude"}, wantErr: true},
		{name: "reconcile dry-run bypasses confirmation", args: []string{"reconcile", "--provider", "claude", "--dry-run"}, wantErr: false},
		{name: "uninstall rejects soft mode", args: []string{"uninstall", "--provider", "claude", "--yes", "--soft"}, wantErr: true},
		{name: "uninstall rejects purge mode", args: []string{"uninstall", "--provider", "claude", "--yes", "--purge"}, wantErr: true},
		{name: "uninstall dry-run bypasses confirmation", args: []string{"uninstall", "--provider", "claude", "--dry-run"}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code := runJarvis(t, home, tt.args...)
			hasErr := code != 0
			if hasErr != tt.wantErr {
				t.Fatalf("runJarvis() code=%d, wantErr=%v out=%s", code, tt.wantErr, out)
			}
		})
	}
}

func TestRunPerProvider_AllFansOutDeterministically(t *testing.T) {
	var called []string
	err := runPerProvider(&cobra.Command{}, "all", func(_ *lifecycle.Engine, provider string) error {
		called = append(called, provider)
		return nil
	})
	if err != nil {
		t.Fatalf("runPerProvider returned error: %v", err)
	}
	want := []string{"claude", "opencode"}
	if !reflect.DeepEqual(called, want) {
		t.Fatalf("providers called mismatch: got %v want %v", called, want)
	}
}

func TestRunUninstall_AllProviderMutationUsesAtomicLifecyclePath(t *testing.T) {
	cmd := lifecycleCommandForTest(t, "all", true, false)

	var calls []string
	originalUninstall := uninstallLifecycle
	uninstallLifecycle = func(_ *lifecycle.Engine, provider, mode string) (lifecycle.UninstallResult, error) {
		calls = append(calls, fmt.Sprintf("%s/%s", provider, mode))
		return lifecycle.UninstallResult{}, nil
	}
	t.Cleanup(func() { uninstallLifecycle = originalUninstall })

	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("runUninstall returned error: %v", err)
	}

	want := []string{"all/all"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("uninstall calls mismatch: got %v want %v", calls, want)
	}
}

func TestRunUninstall_ProviderSpecificMutationKeepsProviderPath(t *testing.T) {
	for _, provider := range []string{"claude", "opencode"} {
		t.Run(provider, func(t *testing.T) {
			cmd := lifecycleCommandForTest(t, provider, true, false)

			var calls []string
			originalUninstall := uninstallLifecycle
			uninstallLifecycle = func(_ *lifecycle.Engine, provider, mode string) (lifecycle.UninstallResult, error) {
				calls = append(calls, fmt.Sprintf("%s/%s", provider, mode))
				return lifecycle.UninstallResult{}, nil
			}
			t.Cleanup(func() { uninstallLifecycle = originalUninstall })

			if err := runUninstall(cmd, nil); err != nil {
				t.Fatalf("runUninstall returned error: %v", err)
			}

			want := []string{provider + "/provider"}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("uninstall calls mismatch: got %v want %v", calls, want)
			}
		})
	}
}

func TestRunUninstall_DryRunDoesNotInvokeLifecycle(t *testing.T) {
	cmd := lifecycleCommandForTest(t, "all", false, true)

	originalUninstall := uninstallLifecycle
	uninstallLifecycle = func(_ *lifecycle.Engine, provider, mode string) (lifecycle.UninstallResult, error) {
		t.Fatalf("dry-run invoked uninstall provider=%s mode=%s", provider, mode)
		return lifecycle.UninstallResult{}, nil
	}
	t.Cleanup(func() { uninstallLifecycle = originalUninstall })

	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("runUninstall returned error: %v", err)
	}
}

func TestRunLifecycleMutatingCommands_ValidateBeforeEngineInvocation(t *testing.T) {
	tests := []struct {
		name string
		run  func(cmd *cobra.Command) error
		cmd  *cobra.Command
	}{
		{
			name: "reconcile requires yes when not dry-run",
			run:  func(cmd *cobra.Command) error { return runReconcile(cmd, nil) },
			cmd: func() *cobra.Command {
				c := &cobra.Command{}
				c.Flags().String("provider", "claude", "")
				c.Flags().Bool("dry-run", false, "")
				c.Flags().Bool("yes", false, "")
				_ = c.Flags().Set("provider", "claude")
				return c
			}(),
		},
		{
			name: "restore requires snapshot",
			run:  func(cmd *cobra.Command) error { return runRestore(cmd, nil) },
			cmd: func() *cobra.Command {
				c := &cobra.Command{}
				c.Flags().String("provider", "claude", "")
				c.Flags().String("snapshot", "", "")
				c.Flags().Bool("yes", true, "")
				_ = c.Flags().Set("provider", "claude")
				_ = c.Flags().Set("yes", "true")
				return c
			}(),
		},
		{
			name: "uninstall rejects unsupported mode",
			run:  func(cmd *cobra.Command) error { return runUninstall(cmd, nil) },
			cmd: func() *cobra.Command {
				c := &cobra.Command{}
				c.Flags().String("provider", "claude", "")
				c.Flags().Bool("soft", false, "")
				c.Flags().Bool("purge", false, "")
				c.Flags().Bool("dry-run", false, "")
				c.Flags().Bool("yes", true, "")
				_ = c.Flags().Set("provider", "claude")
				_ = c.Flags().Set("soft", "true")
				_ = c.Flags().Set("yes", "true")
				return c
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(tt.cmd); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func lifecycleCommandForTest(t *testing.T, provider string, yes, dryRun bool) *cobra.Command {
	t.Helper()
	c := &cobra.Command{}
	c.Flags().String("provider", "all", "")
	c.Flags().Bool("soft", false, "")
	c.Flags().Bool("purge", false, "")
	c.Flags().Bool("dry-run", false, "")
	c.Flags().Bool("yes", false, "")
	if err := c.Flags().Set("provider", provider); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if yes {
		if err := c.Flags().Set("yes", "true"); err != nil {
			t.Fatalf("set yes: %v", err)
		}
	}
	if dryRun {
		if err := c.Flags().Set("dry-run", "true"); err != nil {
			t.Fatalf("set dry-run: %v", err)
		}
	}
	return c
}
