package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
	ExecuteGuard(context.Context, hiveclient.GuardRequest) (hiveclient.GuardResult, error)
	ArchiveProject(context.Context, hiveclient.ProjectArchiveRequest) (hiveclient.ProjectArchiveResult, error)
	MergeProject(context.Context, hiveclient.ProjectMergeRequest) (hiveclient.ProjectMergeResult, error)
	MigrationIdentityStatus(context.Context) (hiveclient.MigrationIdentityStatus, error)
	RestoreMigrationBackup(context.Context, string, string) error
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
	cmd.AddCommand(statusCommand(client), projectsCommand(client), projectCommand(client), memoriesCommand(client), memoryCommand(client), warningsCommand(client), backupsCommand(client))
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
			fmt.Fprintf(cmd.OutOrStdout(), "id=%d project=%s category=%s title=%q deleted=%t created_by=%s created_at=%s", m.ID, m.Project, m.Category, m.Title, m.Deleted, m.CreatedBy, formatTime(m.CreatedAt))
			if includeDeleted && m.Deleted {
				fmt.Fprintf(cmd.OutOrStdout(), " deleted_at=%s deleted_by=%s delete_reason=%q", formatOptionalTime(m.DeletedAt), emptyDash(m.DeletedBy), m.DeleteReason)
			}
			fmt.Fprintln(cmd.OutOrStdout())
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

func memoryCommand(client governanceClient) *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Run guarded local memory operations"}
	cmd.AddCommand(memoryGuardCommand(client, "delete"), memoryGuardCommand(client, "restore"))
	return cmd
}

func projectCommand(client governanceClient) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Run guarded local project operations"}
	cmd.AddCommand(projectArchiveCommand(client), projectMergeCommand(client), projectIdentityCommand(client))
	return cmd
}

func projectIdentityCommand(client governanceClient) *cobra.Command {
	cmd := &cobra.Command{Use: "identity", Short: "Recover canonical project identity migration"}
	cmd.AddCommand(projectIdentityStatusCommand(client), projectIdentityResolveCommand(client), projectIdentityRetryCommand(), projectIdentityRollbackCommand(client))
	return cmd
}

func projectIdentityStatusCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show canonical identity migration recovery state", RunE: func(cmd *cobra.Command, _ []string) error {
		status, err := client.MigrationIdentityStatus(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "state=%s reason=%s backup=%s\n", emptyDash(status.State), emptyDash(status.Reason), emptyDash(status.BackupID))
		for _, conflict := range status.Conflicts {
			fmt.Fprintf(cmd.OutOrStdout(), "conflict=%s\n", conflict)
		}
		for _, variant := range status.Variants {
			fmt.Fprintf(cmd.OutOrStdout(), "variant=%s\n", variant)
		}
		if status.State == "migration-blocked" {
			fmt.Fprintln(cmd.OutOrStdout(), "continuation=hive project identity status")
			fmt.Fprintln(cmd.OutOrStdout(), "Choose explicit --source and --target before a concrete resolve command can exist.")
		}
		return nil
	}}
}

func projectIdentityResolveCommand(client governanceClient) *cobra.Command {
	var source, target, backupID, confirmation string
	cmd := &cobra.Command{Use: "resolve", Short: "Apply an explicit identity conflict choice", RunE: func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
			return fmt.Errorf("--source and --target are required; Hive never chooses an identity resolution")
		}
		if strings.TrimSpace(backupID) == "" {
			return fmt.Errorf("--backup-id is required for hive project identity resolve")
		}
		expected := "MERGE project " + source + " INTO " + target
		if confirmation != expected {
			return fmt.Errorf("confirmation must match exactly: %s", expected)
		}
		if _, err := client.MergeProject(cmd.Context(), hiveclient.ProjectMergeRequest{SourceProject: source, TargetProject: target, BackupID: backupID, Confirmation: confirmation}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Resolution recorded. Run: hive project identity retry")
		return nil
	}}
	cmd.Flags().StringVar(&source, "source", "", "explicit variant to merge")
	cmd.Flags().StringVar(&target, "target", "", "explicit surviving project spelling")
	cmd.Flags().StringVar(&backupID, "backup-id", "", "retained migration backup id")
	cmd.Flags().StringVar(&confirmation, "confirmation", "", "exact merge confirmation")
	return cmd
}

func projectIdentityRetryCommand() *cobra.Command {
	return &cobra.Command{Use: "retry", Short: "Print the full migration retry continuation", RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "Migration retry is pending an operator-managed daemon restart. Stop the running daemon with the same process manager that started it; Hive has no daemon lifecycle command.")
		fmt.Fprintln(cmd.OutOrStdout(), "Check: hive project identity status")
		return nil
	}}
}

func projectIdentityRollbackCommand(client governanceClient) *cobra.Command {
	var backupID, confirmation string
	cmd := &cobra.Command{Use: "rollback", Short: "Restore the retained migration backup", RunE: func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(backupID) == "" {
			return fmt.Errorf("--backup-id is required for hive project identity rollback")
		}
		expected := "RESTORE " + backupID
		if confirmation != expected {
			return fmt.Errorf("confirmation must match exactly: %s", expected)
		}
		if err := client.RestoreMigrationBackup(cmd.Context(), backupID, confirmation); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Backup restore is validated for safe daemon coordination. Run: hive project identity retry")
		return nil
	}}
	cmd.Flags().StringVar(&backupID, "backup-id", "", "retained migration backup id")
	cmd.Flags().StringVar(&confirmation, "confirmation", "", "exact restore confirmation")
	return cmd
}

func projectArchiveCommand(client governanceClient) *cobra.Command {
	var backupID, confirmation, actorID, reason string
	cmd := &cobra.Command{Use: "archive <project>", Short: "Run a guarded local project archive", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		projectName := strings.TrimSpace(args[0])
		if projectName == "" {
			return fmt.Errorf("project is required")
		}
		if !cmd.Flags().Changed("backup-id") {
			return fmt.Errorf("--backup-id is required for hive project archive")
		}
		if strings.TrimSpace(backupID) == "" {
			return fmt.Errorf("--backup-id is required for hive project archive")
		}
		if !cmd.Flags().Changed("confirmation") {
			return fmt.Errorf("--confirmation is required for hive project archive")
		}
		expectedConfirmation := "ARCHIVE project " + projectName
		if confirmation != expectedConfirmation {
			return fmt.Errorf("confirmation must match exactly: %s", expectedConfirmation)
		}
		result, err := client.ArchiveProject(cmd.Context(), hiveclient.ProjectArchiveRequest{Project: projectName, BackupID: backupID, Confirmation: confirmation, ActorID: actorID, Reason: reason})
		if err != nil {
			return err
		}
		status := "already archived"
		if result.Mutated {
			status = "archive completed"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "project %s %s with backup %s\n", result.Project, status, result.BackupID)
		fmt.Fprintf(cmd.OutOrStdout(), "Cloud handoff: %s\n", result.CloudHandoffNote)
		return nil
	}}
	cmd.Flags().StringVar(&backupID, "backup-id", "", "fresh Hive backup id for the destructive operation")
	cmd.Flags().StringVar(&confirmation, "confirmation", "", "exact confirmation required by the daemon guard")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "human or operator id recorded for the local mutation")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded for the local project archive")
	return cmd
}

func projectMergeCommand(client governanceClient) *cobra.Command {
	var backupID, confirmation, actorID, reason string
	cmd := &cobra.Command{Use: "merge <source> <target>", Short: "Run a guarded local project merge", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		sourceProject := strings.TrimSpace(args[0])
		targetProject := strings.TrimSpace(args[1])
		if sourceProject == "" || targetProject == "" {
			return fmt.Errorf("source and target projects are required")
		}
		if sourceProject == targetProject {
			return fmt.Errorf("source and target projects must differ")
		}
		if !cmd.Flags().Changed("backup-id") {
			return fmt.Errorf("--backup-id is required for hive project merge")
		}
		if strings.TrimSpace(backupID) == "" {
			return fmt.Errorf("--backup-id is required for hive project merge")
		}
		if !cmd.Flags().Changed("confirmation") {
			return fmt.Errorf("--confirmation is required for hive project merge")
		}
		expectedConfirmation := "MERGE project " + sourceProject + " INTO " + targetProject
		if confirmation != expectedConfirmation {
			return fmt.Errorf("confirmation must match exactly: %s", expectedConfirmation)
		}
		result, err := client.MergeProject(cmd.Context(), hiveclient.ProjectMergeRequest{SourceProject: sourceProject, TargetProject: targetProject, BackupID: backupID, Confirmation: confirmation, ActorID: actorID, Reason: reason})
		if err != nil {
			return err
		}
		status := "already merged"
		if result.Mutated {
			status = "merge into " + result.TargetProject + " completed"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "project %s %s with backup %s\n", result.SourceProject, status, result.BackupID)
		fmt.Fprintf(cmd.OutOrStdout(), "Cloud handoff: %s\n", result.CloudHandoffNote)
		return nil
	}}
	cmd.Flags().StringVar(&backupID, "backup-id", "", "fresh Hive backup id for the destructive operation")
	cmd.Flags().StringVar(&confirmation, "confirmation", "", "exact confirmation required by the daemon guard")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "human or operator id recorded for the local mutation")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded for the local project merge")
	return cmd
}

func memoryGuardCommand(client governanceClient, operation string) *cobra.Command {
	var backupID, confirmation, actorID, reason string
	cmd := &cobra.Command{Use: operation + " <id>", Short: "Run a guarded local memory " + operation, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("memory id must be a positive integer")
		}
		if !cmd.Flags().Changed("backup-id") {
			return fmt.Errorf("--backup-id is required for hive memory %s", operation)
		}
		if !cmd.Flags().Changed("confirmation") {
			return fmt.Errorf("--confirmation is required for hive memory %s", operation)
		}
		if operation == "delete" {
			reason = strings.TrimSpace(reason)
			if !cmd.Flags().Changed("reason") || reason == "" {
				return fmt.Errorf("--reason is required for hive memory delete")
			}
		}
		result, err := client.ExecuteGuard(cmd.Context(), hiveclient.GuardRequest{Operation: operation, TargetType: "memory", TargetID: id, BackupID: backupID, Confirmation: confirmation, ActorID: actorID, Reason: reason})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "memory %d %s completed with backup %s\n", result.TargetID, result.Operation, result.BackupID)
		fmt.Fprintln(cmd.OutOrStdout(), "Cloud handoff: local mutation uses the normal sync pipeline; no direct cloud mutation was attempted.")
		return nil
	}}
	cmd.Flags().StringVar(&backupID, "backup-id", "", "fresh Hive backup id for the destructive operation")
	cmd.Flags().StringVar(&confirmation, "confirmation", "", "exact confirmation required by the daemon guard")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "human or operator id recorded for the local mutation")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded for delete operations")
	return cmd
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return formatTime(*t)
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
