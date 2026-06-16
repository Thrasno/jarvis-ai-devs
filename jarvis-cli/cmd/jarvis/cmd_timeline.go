package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveui"
)

var timelineProject string

var runTimelineTUI = func(ctx context.Context, baseURL string, project string) error {
	return hiveui.RunTimelineTUI(ctx, baseURL, project)
}

var timelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Browse project memory timeline in TUI",
	Long:  "Opens the Hive TUI on the timeline screen for the specified project.\n\nRequires --project.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if timelineProject == "" {
			return errors.New("--project is required: specify the project name to view its timeline (e.g. jarvis timeline --project my-project)")
		}
		return runTimelineTUI(cmd.Context(), resolveHiveDaemonURL(), timelineProject)
	},
}

func init() {
	timelineCmd.Flags().StringVar(&timelineProject, "project", "", "project name whose timeline to browse (required)")
}
