package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newSyncCommand builds `jarvis sync`.
//
// It deliberately declares no flags. --dry-run in particular is not a missing
// feature: replay is the whole command, and a run that described changes
// without making them would need a second, divergent path through the applier.
//
// Cobra rejects an unknown flag on its own, but an inherited persistent flag
// such as the root command's --no-tui parses happily and would otherwise be
// accepted silently. The guard therefore refuses any flag that was set at all,
// and it runs in PreRunE, before the run seam and so before any mutation.
func newSyncCommand(run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync memories with Hive Cloud",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			supplied := make([]string, 0, 1)
			cmd.Flags().Visit(func(flag *pflag.Flag) { supplied = append(supplied, "--"+flag.Name) })
			if len(supplied) == 0 {
				return nil
			}
			return fmt.Errorf("jarvis sync accepts no flags, got %s", strings.Join(supplied, ", "))
		},
		RunE: func(*cobra.Command, []string) error { return run() },
	}
}

var syncCmd = newSyncCommand(func() error {
	fmt.Println("jarvis sync is a no-op: sync is handled through hive-daemon MCP tools.")
	fmt.Println("By default, run mem_sync in Claude Code/OpenCode when you want a manual cloud sync.")
	fmt.Println("Automatic background sync only runs when auto_sync is explicitly enabled.")
	return nil
})
