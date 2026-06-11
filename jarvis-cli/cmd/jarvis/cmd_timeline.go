package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/tui"
)

var timelineProject string

var timelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Browse project memory timeline in TUI",
	Long:  "Opens the Hive TUI on the timeline screen for the specified project.\n\nRequires --project.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if timelineProject == "" {
			return errors.New("--project is required: specify the project name to view its timeline (e.g. jarvis timeline --project my-project)")
		}
		return tui.RunTimeline(timelineProject)
	},
}

func init() {
	timelineCmd.Flags().StringVar(&timelineProject, "project", "", "project name whose timeline to browse (required)")
}
