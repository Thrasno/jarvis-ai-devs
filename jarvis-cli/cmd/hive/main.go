package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/spf13/cobra"
)

type governanceClient interface {
	Status(context.Context) ([]hiveclient.Health, error)
	Projects(context.Context) ([]hiveclient.Project, error)
	Memories(context.Context, hiveclient.MemoryFilter) ([]hiveclient.Memory, error)
	Warnings(context.Context) ([]hiveclient.Warning, error)
	Backups(context.Context) ([]hiveclient.Backup, error)
}

func main() {
	client, err := hiveclient.NewFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := NewRootCommand(client).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func NewRootCommand(client governanceClient) *cobra.Command {
	cmd := &cobra.Command{Use: "hive", Short: "Local Hive governance CLI", SilenceErrors: true, SilenceUsage: true}
	cmd.AddCommand(statusCommand(client), projectsCommand(client), memoriesCommand(client), warningsCommand(client), backupsCommand(client))
	return cmd
}

func statusCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"health"}, Short: "Show daemon governance health", RunE: func(cmd *cobra.Command, _ []string) error {
		health, err := client.Status(cmd.Context())
		if err != nil {
			return err
		}
		if len(health) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Hive governance status: no sync health records")
			return nil
		}
		for _, h := range health {
			fmt.Fprintf(cmd.OutOrStdout(), "project=%s failures=%d last_error=%s last_success=%s backoff_until=%s\n", h.Project, h.ConsecutiveFailures, emptyDash(h.LastError), formatTime(h.LastSuccessAt), formatTime(h.BackoffUntil))
		}
		return nil
	}}
}

func projectsCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "projects", Short: "List local Hive projects", RunE: func(cmd *cobra.Command, _ []string) error {
		projects, err := client.Projects(cmd.Context())
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No local Hive projects found")
			return nil
		}
		for _, p := range projects {
			fmt.Fprintf(cmd.OutOrStdout(), "project=%s active=%d deleted=%d sessions=%d prompts=%d last_activity=%s\n", p.Name, p.ActiveMemoryCount, p.DeletedMemoryCount, p.SessionCount, p.PromptCount, formatTime(p.LastActivityAt))
		}
		return nil
	}}
}

func memoriesCommand(client governanceClient) *cobra.Command {
	var project string
	var includeDeleted bool
	var limit int
	cmd := &cobra.Command{Use: "memories", Short: "List local Hive memories for a project", RunE: func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(project) == "" {
			return fmt.Errorf("--project is required for hive memories")
		}
		memories, err := client.Memories(cmd.Context(), hiveclient.MemoryFilter{Project: project, IncludeDeleted: includeDeleted, Limit: limit})
		if err != nil {
			return err
		}
		if len(memories) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No local Hive memories found")
			return nil
		}
		for _, m := range memories {
			fmt.Fprintf(cmd.OutOrStdout(), "id=%d project=%s category=%s title=%q deleted=%t created_by=%s created_at=%s\n", m.ID, m.Project, m.Category, m.Title, m.Deleted, m.CreatedBy, formatTime(m.CreatedAt))
		}
		return nil
	}}
	cmd.Flags().StringVar(&project, "project", "", "project name to inspect")
	cmd.Flags().BoolVar(&includeDeleted, "include-deleted", false, "include deleted memories")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum memories to list")
	return cmd
}

func warningsCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "warnings", Short: "List local Hive warnings", RunE: func(cmd *cobra.Command, _ []string) error {
		warnings, err := client.Warnings(cmd.Context())
		if errors.Is(err, hiveclient.ErrNotAvailable) {
			fmt.Fprintln(cmd.OutOrStdout(), "Hive warnings are not available from this daemon yet")
			return nil
		}
		if err != nil {
			return err
		}
		if len(warnings) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No Hive warnings found")
			return nil
		}
		for _, w := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "id=%d severity=%s source=%s state=%s message=%q created_at=%s\n", w.ID, w.Severity, w.Source, w.ResolutionState, w.Message, formatTime(w.CreatedAt))
		}
		return nil
	}}
}

func backupsCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "backups", Short: "List local Hive database backups", RunE: func(cmd *cobra.Command, _ []string) error {
		backups, err := client.Backups(cmd.Context())
		if err != nil {
			return err
		}
		if len(backups) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No Hive backups found")
			return nil
		}
		for _, b := range backups {
			fmt.Fprintf(cmd.OutOrStdout(), "id=%s bytes=%d archive=%s created_at=%s\n", b.ID, b.SizeBytes, b.ArchivePath, formatTime(b.CreatedAt))
		}
		return nil
	}}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
