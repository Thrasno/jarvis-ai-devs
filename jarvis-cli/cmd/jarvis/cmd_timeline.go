package main

import (
	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/tui"
)

var timelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Browse memory timeline (MVP 2)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.RunTimeline()
	},
}
