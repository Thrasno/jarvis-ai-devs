package main

import (
	"context"
	"fmt"
	"io"
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
	ResolveMigrationIdentity(context.Context, hiveclient.IdentityResolutionRequest) error
	RequestMigrationRetry(context.Context) error
	RestoreMigrationBackup(context.Context, string, string) (hiveclient.RestoreResult, error)
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
			// retain_until is load-bearing, not decoration: a migration backup
			// is the ONLY rollback artifact for that migration and the daemon
			// reclaims it after 24h, so an operator who cannot see the deadline
			// discovers it by finding the backup gone.
			fmt.Fprintf(cmd.OutOrStdout(), "id=%s bytes=%d archive=%s created_at=%s retain_until=%s\n", b.ID, b.SizeBytes, b.ArchivePath, formatTime(b.CreatedAt), formatTime(b.RetainUntil))
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
	cmd.AddCommand(projectIdentityStatusCommand(client), projectIdentityResolveCommand(client), projectIdentityRetryCommand(client), projectIdentityRollbackCommand(client))
	return cmd
}

// The daemon's two non-serving migration states, mirrored from
// hive-daemon/internal/project.MigrationState*. Both block Hive, so both need the
// recovery guidance, but only one of them ever touched the database.
const (
	migrationStateBlocked               = "migration-blocked"
	migrationStatePendingOperatorReview = "migration-pending-operator-review"
)

// The next step printed for each blocking state. These are local literals, and
// status.Continuation is deliberately not printed even though the daemon sends
// it: this line invites the operator to run a command, and commit 9af78aa9
// ("fix(hive): secure global context hooks") settled that the daemon's
// continuation is untrusted text that must not be rendered as one. Routing on
// State instead is safe because it is a small closed set validated above.
const (
	migrationStatusCommand        = "hive project identity status"
	migrationNormalizationCommand = "jarvis hive → Project normalization"
)

func projectIdentityStatusCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show canonical identity migration recovery state", RunE: func(cmd *cobra.Command, _ []string) error {
		status, err := client.MigrationIdentityStatus(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "state=%s reason=%s backup=%s plan=%s\n", emptyDash(status.State), emptyDash(status.Reason), emptyDash(status.BackupID), emptyDash(status.PlanFingerprint))
		if status.State != migrationStateBlocked && status.State != migrationStatePendingOperatorReview {
			return nil
		}
		// The state selects the next step: the failure state is recovered from this
		// CLI, while an ambiguous identity can only be resolved in the TUI wizard.
		// A fixed command for both was wrong; the daemon's own text is not the fix.
		continuation := migrationStatusCommand
		if status.State == migrationStatePendingOperatorReview {
			continuation = migrationNormalizationCommand
		}
		fmt.Fprintf(cmd.OutOrStdout(), "continuation=%s\n", continuation)
		// Everything below is specific to a migration that ran and failed. A
		// pending operator review attempted nothing, so it took no archive and
		// there is nothing to roll back or to aim a resolve command at; saying so
		// there would describe a failure that never happened.
		if status.State != migrationStateBlocked {
			return nil
		}
		if status.BackupID == "" {
			// An empty backup id covers three outcomes the daemon cannot tell
			// apart on the wire: none was created, the archive passed its
			// retention, or it failed its checksum. Naming only the first
			// would contradict the daemon's own corruption log.
			fmt.Fprintln(cmd.OutOrStdout(), "No migration backup is available for this block (none was created, its retention expired, or its archive failed checksum validation); rollback is unavailable.")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Choose explicit --source and --target before a concrete resolve command can exist.")
		return nil
	}}
}

func projectIdentityResolveCommand(client governanceClient) *cobra.Command {
	var source, target, planFingerprint, confirmation string
	cmd := &cobra.Command{Use: "resolve", Short: "Apply an explicit identity conflict choice", RunE: func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
			return fmt.Errorf("--source and --target are required; Hive never chooses an identity resolution")
		}
		// A blocked preflight never mutated the database and never created a
		// backup, so the plan the operator was shown is the only honest guard.
		if strings.TrimSpace(planFingerprint) == "" {
			return fmt.Errorf("--plan-fingerprint is required for hive project identity resolve; read plan= from hive project identity status")
		}
		expected := "RESOLVE project identity " + source + " INTO " + target
		if confirmation != expected {
			return fmt.Errorf("confirmation must match exactly: %s", expected)
		}
		if err := client.ResolveMigrationIdentity(cmd.Context(), hiveclient.IdentityResolutionRequest{SourceProject: source, TargetProject: target, PlanFingerprint: planFingerprint, Confirmation: confirmation}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Resolution recorded. Run: hive project identity retry")
		return nil
	}}
	cmd.Flags().StringVar(&source, "source", "", "explicit variant to merge")
	cmd.Flags().StringVar(&target, "target", "", "explicit surviving project spelling")
	cmd.Flags().StringVar(&planFingerprint, "plan-fingerprint", "", "migration plan fingerprint reported by hive project identity status")
	cmd.Flags().StringVar(&confirmation, "confirmation", "", "exact merge confirmation")
	return cmd
}

func projectIdentityRetryCommand(client governanceClient) *cobra.Command {
	return &cobra.Command{Use: "retry", Short: "Print the full migration retry continuation", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := client.RequestMigrationRetry(cmd.Context()); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Migration retry requested; the daemon will stop cleanly and its MCP lifecycle owner will start one fresh daemon.")
		fmt.Fprintln(cmd.OutOrStdout(), "Check after restart: hive project identity status")
		return nil
	}}
}

// printRestoreOutcome reports what the daemon actually did, which is not always
// what this command used to claim.
//
// The daemon has two genuinely different branches. With the migration gate
// BLOCKED it schedules the restore and stops itself, so the managed restart
// message is true. With the gate READY it takes the other branch entirely:
// RestoreBackup is PlanRestore, which validates the archive and answers
// coordination_required — nothing is scheduled and the live database is
// untouched. Printing the restart message there tells an operator the rollback
// is handled when in fact they still have to stop the daemon themselves.
func printRestoreOutcome(out io.Writer, result hiveclient.RestoreResult) {
	if result.Status == hiveclient.RestoreStatusRestartRequested {
		fmt.Fprintln(out, "Backup restore was scheduled. The managed daemon will restart, restore it before reopening SQLite, and re-run migration planning.")
		return
	}
	fmt.Fprintf(out, "Backup restore was validated but not applied (status: %s).\n", emptyDash(result.Status))
	if message := strings.TrimSpace(result.Message); message != "" {
		fmt.Fprintln(out, message)
	}
	fmt.Fprintln(out, "The daemon is serving normally, so it did not schedule the restore. Stop hive-daemon yourself before the live database can be replaced.")
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
		result, err := client.RestoreMigrationBackup(cmd.Context(), backupID, confirmation)
		if err != nil {
			return err
		}
		printRestoreOutcome(cmd.OutOrStdout(), result)
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
