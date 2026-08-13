package main

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
)

// TestSyncCommand_RejectsEveryFlagWithoutRunning locks the CLI boundary of this
// version: `jarvis sync` takes no flags at all, --dry-run included, and a
// rejected invocation must never reach the run seam, so it can mutate nothing.
//
// The inherited-flag case is the one that matters: cobra rejects an unknown
// flag by itself, but a persistent flag declared on the root command parses
// happily on every subcommand, so an assumed rejection would be wrong.
func TestSyncCommand_RejectsEveryFlagWithoutRunning(t *testing.T) {
	for _, tc := range []struct{ name, flag string }{
		{"dry run", "--dry-run"},
		{"unknown long flag", "--force"},
		{"unknown short flag", "-f"},
		{"inherited global flag", "--no-tui"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := 0
			root := newSyncTestRoot(func() error { runs++; return nil })
			root.SetArgs([]string{"sync", tc.flag})

			if err := root.Execute(); err == nil {
				t.Fatalf("expected %q to be a usage error", tc.flag)
			}
			if runs != 0 {
				t.Fatalf("expected zero runs after a usage error, got %d", runs)
			}
		})
	}
}

// TestSyncCommand_RunsWhenInvokedWithNoFlags is the other half of the boundary:
// the guard must refuse flags, not the command.
func TestSyncCommand_RunsWhenInvokedWithNoFlags(t *testing.T) {
	runs := 0
	root := newSyncTestRoot(func() error { runs++; return nil })
	root.SetArgs([]string{"sync"})

	if err := root.Execute(); err != nil {
		t.Fatalf("expected a flagless sync to run, got %v", err)
	}
	if runs != 1 {
		t.Fatalf("expected exactly one run, got %d", runs)
	}
}

// newSyncTestRoot mirrors the production wiring: a root command carrying the
// same persistent flag, with sync mounted underneath it.
func newSyncTestRoot(run func() error) *cobra.Command {
	root := &cobra.Command{Use: "jarvis", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("no-tui", false, "disable TUI, use readline prompts")
	root.AddCommand(newSyncCommand(run))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}
