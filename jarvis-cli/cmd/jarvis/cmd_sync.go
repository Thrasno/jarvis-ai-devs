package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync memories with Hive Cloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Sync is managed by hive-daemon (MCP server).")
		fmt.Println("To sync manually, use the mem_sync tool within Claude Code or OpenCode.")
		fmt.Println("The hive-daemon runs as an MCP server and handles sync automatically.")
		return nil
	},
}
