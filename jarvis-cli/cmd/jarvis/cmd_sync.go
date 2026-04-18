package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync memories with Hive Cloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("jarvis sync is a no-op: sync is handled through hive-daemon MCP tools.")
		fmt.Println("By default, run mem_sync in Claude Code/OpenCode when you want a manual cloud sync.")
		fmt.Println("Automatic background sync only runs when auto_sync is explicitly enabled.")
		return nil
	},
}
