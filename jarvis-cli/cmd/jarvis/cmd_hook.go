package main

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hook"
)

// hookCmd is the parent for Claude Code hook handler subcommands.
// It is hidden from normal help output — these commands are called by Claude Code
// hooks, not by users directly.
var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Claude Code hook handlers (called by Claude Code hooks, not by users directly)",
	Hidden: true,
}

// hookSessionStartCmd handles the SessionStart hook event.
var hookSessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "Handle Claude Code SessionStart event",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		baseURL := resolveHiveBaseURL()
		hook.RunSessionStart(ctx, os.Stdin, cmd.OutOrStdout(), baseURL)
		return nil
	},
}

// hookSessionCompactCmd handles the SessionStart hook event with matcher "compact".
var hookSessionCompactCmd = &cobra.Command{
	Use:   "session-compact",
	Short: "Handle Claude Code SessionStart compact event",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hook.RunSessionCompact(ctx, os.Stdin, cmd.OutOrStdout())
		return nil
	},
}

// hookPromptSubmitCmd handles the UserPromptSubmit hook event.
var hookPromptSubmitCmd = &cobra.Command{
	Use:   "prompt-submit",
	Short: "Handle Claude Code UserPromptSubmit event",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 3s budget: RunPromptSubmit runs PostPrompt (1500ms client timeout) then
		// LatestSaveAt (1s client timeout) sequentially = up to 2.5s. A 2s parent
		// deadline would starve LatestSaveAt and misclassify a slow-but-reachable
		// daemon as Unreachable, suppressing the reminder under load. 3s covers the
		// worst case plus margin and stays well within Claude Code's 8s hook timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		baseURL := resolveHiveBaseURL()
		hook.RunPromptSubmit(ctx, os.Stdin, cmd.OutOrStdout(), baseURL)
		return nil
	},
}

// hookSubagentStopCmd handles the SubagentStop hook event.
var hookSubagentStopCmd = &cobra.Command{
	Use:   "subagent-stop",
	Short: "Handle Claude Code SubagentStop event",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		baseURL := resolveHiveBaseURL()
		hook.RunSubagentStop(ctx, os.Stdin, cmd.OutOrStdout(), baseURL)
		return nil
	},
}

// hookSessionStopCmd handles the Stop hook event.
var hookSessionStopCmd = &cobra.Command{
	Use:   "session-stop",
	Short: "Handle Claude Code Stop event",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		baseURL := resolveHiveBaseURL()
		hook.RunSessionStop(ctx, os.Stdin, cmd.OutOrStdout(), baseURL)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(
		hookSessionStartCmd,
		hookSessionCompactCmd,
		hookPromptSubmitCmd,
		hookSubagentStopCmd,
		hookSessionStopCmd,
	)
}

// resolveHiveBaseURL returns the base URL for the local hive-daemon.
// It respects the HIVE_HTTP_PORT environment variable for test overrides.
func resolveHiveBaseURL() string {
	port := os.Getenv("HIVE_HTTP_PORT")
	if port == "" {
		port = "7438"
	}
	return "http://127.0.0.1:" + port
}
