package main

import (
	"os"
	"reflect"
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

func TestLifecycleCommandValidation(t *testing.T) {
	if _, err := os.Stat(jarvisBin); os.IsNotExist(err) {
		t.Skip("jarvis binary not available")
	}
	home := t.TempDir()

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
