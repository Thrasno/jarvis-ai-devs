package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/spf13/cobra"
)

func TestHiveStatusCommandUsesDaemonClient(t *testing.T) {
	client := &fakeHiveClient{health: []hiveclient.Health{{Project: "alpha", ConsecutiveFailures: 2, LastError: "rate limited"}}}
	out, err := executeHiveCommand(t, NewRootCommand(client), "status")
	if err != nil {
		t.Fatalf("status command: %v", err)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "failures=2") || !strings.Contains(out, "rate limited") {
		t.Fatalf("status output = %q, want project health from fake daemon client", out)
	}
}

func TestHiveReadOnlyCommandsUseDaemonClient(t *testing.T) {
	deletedAt := time.Date(2026, 6, 6, 21, 0, 0, 0, time.UTC)
	client := &fakeHiveClient{
		projects: []hiveclient.Project{{Name: "alpha", ActiveMemoryCount: 3, DeletedMemoryCount: 1}},
		memories: []hiveclient.Memory{{ID: 7, Project: "alpha", Category: "decision", Title: "Governance boundary", Deleted: true, DeletedAt: &deletedAt, DeletedBy: "tester", DeleteReason: "manual cleanup"}},
		backups:  []hiveclient.Backup{{ID: "backup-1", ArchivePath: "/tmp/hive-backups/backup-1/memory.db", SizeBytes: 42}},
	}

	projectOut, err := executeHiveCommand(t, NewRootCommand(client), "projects")
	if err != nil {
		t.Fatalf("projects command: %v", err)
	}
	if !strings.Contains(projectOut, "alpha") || !strings.Contains(projectOut, "active=3") {
		t.Fatalf("projects output = %q, want daemon project summary", projectOut)
	}

	memoryOut, err := executeHiveCommand(t, NewRootCommand(client), "memories", "--project", "alpha", "--include-deleted", "--limit", "2")
	if err != nil {
		t.Fatalf("memories command: %v", err)
	}
	if client.memoryFilter.Project != "alpha" || !client.memoryFilter.IncludeDeleted || client.memoryFilter.Limit != 2 {
		t.Fatalf("memory filter = %+v, want CLI flags forwarded", client.memoryFilter)
	}
	if !strings.Contains(memoryOut, "Governance boundary") || !strings.Contains(memoryOut, "deleted=true") || !strings.Contains(memoryOut, "deleted_at=2026-06-06T21:00:00Z") || !strings.Contains(memoryOut, "deleted_by=tester") || !strings.Contains(memoryOut, `delete_reason="manual cleanup"`) {
		t.Fatalf("memories output = %q, want daemon memory rows with delete audit metadata", memoryOut)
	}

	backupOut, err := executeHiveCommand(t, NewRootCommand(client), "backups")
	if err != nil {
		t.Fatalf("backups command: %v", err)
	}
	if !strings.Contains(backupOut, "backup-1") || !strings.Contains(backupOut, "bytes=42") {
		t.Fatalf("backups output = %q, want daemon backup manifests", backupOut)
	}
}

func TestHiveWarningsCommandPropagatesDaemonErrors(t *testing.T) {
	client := &fakeHiveClient{warningsErr: hiveclient.ErrNotAvailable}
	out, err := executeHiveCommand(t, NewRootCommand(client), "warnings")
	if err == nil {
		t.Fatal("warnings command error = nil, want daemon error")
	}
	if out != "" {
		t.Fatalf("warnings output = %q, want no fallback unavailable message", out)
	}
}

func TestHiveWarningsCommandListsWarningsWhenEndpointExists(t *testing.T) {
	client := &fakeHiveClient{warnings: []hiveclient.Warning{{ID: 5, Severity: "warning", Source: "startup", Message: "degraded config", ResolutionState: "active"}}}
	out, err := executeHiveCommand(t, NewRootCommand(client), "warnings")
	if err != nil {
		t.Fatalf("warnings command: %v", err)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "degraded config") || !strings.Contains(out, "active") {
		t.Fatalf("warnings output = %q, want warning inbox row", out)
	}
}

func TestHiveMemoriesCommandRequiresProjectLocally(t *testing.T) {
	client := &fakeHiveClient{}
	out, err := executeHiveCommand(t, NewRootCommand(client), "memories")
	if err == nil {
		t.Fatal("memories command error = nil, want local --project validation error")
	}
	if !strings.Contains(err.Error(), "--project is required") {
		t.Fatalf("memories command error = %v, want clear --project requirement", err)
	}
	if out != "" {
		t.Fatalf("memories command output = %q, want no noisy usage output", out)
	}
	if client.memoriesCalled {
		t.Fatal("memories command called daemon client without required --project")
	}
}

func TestHiveMemoryGuardCommandsUseDaemonClient(t *testing.T) {
	client := &fakeHiveClient{}
	deleteOut, err := executeHiveCommand(t, NewRootCommand(client), "memory", "delete", "7", "--backup-id", "backup-1", "--confirmation", "DELETE memory 7", "--actor-id", "tester", "--reason", "cleanup")
	if err != nil {
		t.Fatalf("memory delete command: %v", err)
	}
	if client.guardRequest.Operation != "delete" || client.guardRequest.TargetType != "memory" || client.guardRequest.TargetID != 7 || client.guardRequest.BackupID != "backup-1" || client.guardRequest.Confirmation != "DELETE memory 7" || client.guardRequest.ActorID != "tester" || client.guardRequest.Reason != "cleanup" {
		t.Fatalf("guard request = %+v, want delete request from CLI flags", client.guardRequest)
	}
	if !strings.Contains(deleteOut, "memory 7 delete completed") || !strings.Contains(deleteOut, "no direct cloud mutation") {
		t.Fatalf("delete output = %q, want local result and cloud handoff note", deleteOut)
	}

	restoreOut, err := executeHiveCommand(t, NewRootCommand(client), "memory", "restore", "7", "--backup-id", "backup-2", "--confirmation", "RESTORE memory 7")
	if err != nil {
		t.Fatalf("memory restore command: %v", err)
	}
	if client.guardRequest.Operation != "restore" || client.guardRequest.TargetID != 7 || client.guardRequest.BackupID != "backup-2" || client.guardRequest.Confirmation != "RESTORE memory 7" {
		t.Fatalf("guard request = %+v, want restore request from CLI flags", client.guardRequest)
	}
	if !strings.Contains(restoreOut, "memory 7 restore completed") {
		t.Fatalf("restore output = %q, want local restore result", restoreOut)
	}
}

func TestHiveMemoryGuardCommandsRequireBackupIDAndConfirmationLocally(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "delete missing backup id", args: []string{"memory", "delete", "7", "--confirmation", "DELETE memory 7"}, wantErr: "--backup-id is required"},
		{name: "delete missing confirmation", args: []string{"memory", "delete", "7", "--backup-id", "backup-1"}, wantErr: "--confirmation is required"},
		{name: "restore missing backup id", args: []string{"memory", "restore", "7", "--confirmation", "RESTORE memory 7"}, wantErr: "--backup-id is required"},
		{name: "restore missing confirmation", args: []string{"memory", "restore", "7", "--backup-id", "backup-1"}, wantErr: "--confirmation is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeHiveClient{}
			out, err := executeHiveCommand(t, NewRootCommand(client), tt.args...)
			if err == nil {
				t.Fatal("memory guard command error = nil, want local required flag error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("memory guard command error = %v, want %q", err, tt.wantErr)
			}
			if out != "" {
				t.Fatalf("memory guard command output = %q, want no noisy usage output", out)
			}
			if client.guardCalled {
				t.Fatal("memory guard command called daemon client without required local flags")
			}
		})
	}
}

func TestHiveMemoryDeleteCommandRequiresReasonLocally(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing reason", args: []string{"memory", "delete", "7", "--backup-id", "backup-1", "--confirmation", "DELETE memory 7"}},
		{name: "blank reason", args: []string{"memory", "delete", "7", "--backup-id", "backup-1", "--confirmation", "DELETE memory 7", "--reason", "  \t  "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeHiveClient{}
			out, err := executeHiveCommand(t, NewRootCommand(client), tt.args...)
			if err == nil {
				t.Fatal("memory delete command error = nil, want local --reason validation error")
			}
			if !strings.Contains(err.Error(), "--reason is required for hive memory delete") {
				t.Fatalf("memory delete command error = %v, want --reason requirement", err)
			}
			if out != "" {
				t.Fatalf("memory delete command output = %q, want no noisy usage output", out)
			}
			if client.guardCalled {
				t.Fatal("memory delete command called daemon client without required reason")
			}
		})
	}
}

func TestHiveMemoryGuardCommandPreservesConfirmationWhitespace(t *testing.T) {
	client := &fakeHiveClient{}
	confirmation := "  DELETE memory 7  "
	_, err := executeHiveCommand(t, NewRootCommand(client), "memory", "delete", "7", "--backup-id", "backup-1", "--confirmation", confirmation, "--reason", "manual cleanup")
	if err != nil {
		t.Fatalf("memory delete command: %v", err)
	}
	if !client.guardCalled {
		t.Fatal("memory delete command did not call daemon client")
	}
	if client.guardRequest.Confirmation != confirmation {
		t.Fatalf("confirmation = %q, want exact value %q", client.guardRequest.Confirmation, confirmation)
	}
	if client.guardRequest.Reason != "manual cleanup" {
		t.Fatalf("reason = %q, want manual cleanup", client.guardRequest.Reason)
	}
}

func TestHiveProjectArchiveCommandUsesDaemonClient(t *testing.T) {
	client := &fakeHiveClient{}
	out, err := executeHiveCommand(t, NewRootCommand(client), "project", "archive", "alpha", "--backup-id", "backup-1", "--confirmation", "ARCHIVE project alpha", "--actor-id", "tester", "--reason", "cleanup")
	if err != nil {
		t.Fatalf("project archive command: %v", err)
	}
	if client.archiveRequest.Project != "alpha" || client.archiveRequest.BackupID != "backup-1" || client.archiveRequest.Confirmation != "ARCHIVE project alpha" || client.archiveRequest.ActorID != "tester" || client.archiveRequest.Reason != "cleanup" {
		t.Fatalf("archive request = %+v, want project archive request from CLI flags", client.archiveRequest)
	}
	if !strings.Contains(out, "project alpha archive completed") || !strings.Contains(out, "direct cloud mutation") {
		t.Fatalf("archive output = %q, want local archive result and cloud handoff note", out)
	}
}

func TestHiveProjectArchiveCommandRequiresBackupIDAndConfirmationLocally(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing backup id", args: []string{"project", "archive", "alpha", "--confirmation", "ARCHIVE project alpha"}, wantErr: "--backup-id is required"},
		{name: "empty backup id", args: []string{"project", "archive", "alpha", "--backup-id", "", "--confirmation", "ARCHIVE project alpha"}, wantErr: "--backup-id is required"},
		{name: "whitespace backup id", args: []string{"project", "archive", "alpha", "--backup-id", "  \t  ", "--confirmation", "ARCHIVE project alpha"}, wantErr: "--backup-id is required"},
		{name: "missing confirmation", args: []string{"project", "archive", "alpha", "--backup-id", "backup-1"}, wantErr: "--confirmation is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeHiveClient{}
			out, err := executeHiveCommand(t, NewRootCommand(client), tt.args...)
			if err == nil {
				t.Fatal("project archive command error = nil, want local required flag error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("project archive command error = %v, want %q", err, tt.wantErr)
			}
			if out != "" {
				t.Fatalf("project archive command output = %q, want no noisy usage output", out)
			}
			if client.archiveCalled {
				t.Fatal("project archive command called daemon client without required local flags")
			}
		})
	}
}

func TestHiveProjectArchiveCommandRejectsNonExactConfirmationBeforeDaemonCall(t *testing.T) {
	client := &fakeHiveClient{}
	confirmation := "  ARCHIVE project alpha  "
	out, err := executeHiveCommand(t, NewRootCommand(client), "project", "archive", "alpha", "--backup-id", "backup-1", "--confirmation", confirmation)
	if err == nil {
		t.Fatal("project archive command error = nil, want exact confirmation error")
	}
	if !strings.Contains(err.Error(), "confirmation must match exactly") {
		t.Fatalf("project archive command error = %v, want exact confirmation error", err)
	}
	if out != "" {
		t.Fatalf("project archive command output = %q, want no noisy usage output", out)
	}
	if client.archiveCalled {
		t.Fatal("project archive command called daemon client with non-exact confirmation")
	}
}

func TestHiveProjectMergeCommandUsesDaemonClient(t *testing.T) {
	client := &fakeHiveClient{}
	out, err := executeHiveCommand(t, NewRootCommand(client), "project", "merge", "alpha", "beta", "--backup-id", "backup-1", "--confirmation", "MERGE project alpha INTO beta", "--actor-id", "tester", "--reason", "dedupe")
	if err != nil {
		t.Fatalf("project merge command: %v", err)
	}
	if client.mergeRequest.SourceProject != "alpha" || client.mergeRequest.TargetProject != "beta" || client.mergeRequest.BackupID != "backup-1" || client.mergeRequest.Confirmation != "MERGE project alpha INTO beta" || client.mergeRequest.ActorID != "tester" || client.mergeRequest.Reason != "dedupe" {
		t.Fatalf("merge request = %+v, want project merge request from CLI flags", client.mergeRequest)
	}
	if !strings.Contains(out, "project alpha merge into beta completed") || !strings.Contains(out, "direct cloud mutation") {
		t.Fatalf("merge output = %q, want local merge result and cloud handoff note", out)
	}
}

func TestHiveProjectMergeCommandRequiresBackupIDAndConfirmationLocally(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing backup id", args: []string{"project", "merge", "alpha", "beta", "--confirmation", "MERGE project alpha INTO beta"}, wantErr: "--backup-id is required"},
		{name: "empty backup id", args: []string{"project", "merge", "alpha", "beta", "--backup-id", "", "--confirmation", "MERGE project alpha INTO beta"}, wantErr: "--backup-id is required"},
		{name: "missing confirmation", args: []string{"project", "merge", "alpha", "beta", "--backup-id", "backup-1"}, wantErr: "--confirmation is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeHiveClient{}
			out, err := executeHiveCommand(t, NewRootCommand(client), tt.args...)
			if err == nil {
				t.Fatal("project merge command error = nil, want local required flag error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("project merge command error = %v, want %q", err, tt.wantErr)
			}
			if out != "" {
				t.Fatalf("project merge command output = %q, want no noisy usage output", out)
			}
			if client.mergeCalled {
				t.Fatal("project merge command called daemon client without required local flags")
			}
		})
	}
}

func TestHiveProjectMergeCommandRejectsNonExactConfirmationBeforeDaemonCall(t *testing.T) {
	client := &fakeHiveClient{}
	out, err := executeHiveCommand(t, NewRootCommand(client), "project", "merge", "alpha", "beta", "--backup-id", "backup-1", "--confirmation", " MERGE project alpha INTO beta ")
	if err == nil {
		t.Fatal("project merge command error = nil, want exact confirmation error")
	}
	if !strings.Contains(err.Error(), "confirmation must match exactly") {
		t.Fatalf("project merge command error = %v, want exact confirmation error", err)
	}
	if out != "" {
		t.Fatalf("project merge command output = %q, want no noisy usage output", out)
	}
	if client.mergeCalled {
		t.Fatal("project merge command called daemon client with non-exact confirmation")
	}
}

func executeHiveCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

type fakeHiveClient struct {
	health         []hiveclient.Health
	projects       []hiveclient.Project
	memories       []hiveclient.Memory
	warnings       []hiveclient.Warning
	backups        []hiveclient.Backup
	warningsErr    error
	memoryFilter   hiveclient.MemoryFilter
	guardRequest   hiveclient.GuardRequest
	archiveRequest hiveclient.ProjectArchiveRequest
	mergeRequest   hiveclient.ProjectMergeRequest
	memoriesCalled bool
	guardCalled    bool
	archiveCalled  bool
	mergeCalled    bool
}

func (f *fakeHiveClient) Status(context.Context) ([]hiveclient.Health, error) { return f.health, nil }
func (f *fakeHiveClient) Projects(context.Context) ([]hiveclient.Project, error) {
	return f.projects, nil
}
func (f *fakeHiveClient) Memories(_ context.Context, filter hiveclient.MemoryFilter) ([]hiveclient.Memory, error) {
	f.memoriesCalled = true
	f.memoryFilter = filter
	return f.memories, nil
}
func (f *fakeHiveClient) Warnings(context.Context) ([]hiveclient.Warning, error) {
	if f.warningsErr != nil {
		return nil, f.warningsErr
	}
	return f.warnings, nil
}
func (f *fakeHiveClient) Backups(context.Context) ([]hiveclient.Backup, error) { return f.backups, nil }
func (f *fakeHiveClient) ExecuteGuard(_ context.Context, req hiveclient.GuardRequest) (hiveclient.GuardResult, error) {
	f.guardCalled = true
	f.guardRequest = req
	return hiveclient.GuardResult{Operation: req.Operation, TargetType: req.TargetType, TargetID: req.TargetID, BackupID: req.BackupID, Mutated: true}, nil
}
func (f *fakeHiveClient) ArchiveProject(_ context.Context, req hiveclient.ProjectArchiveRequest) (hiveclient.ProjectArchiveResult, error) {
	f.archiveCalled = true
	f.archiveRequest = req
	return hiveclient.ProjectArchiveResult{Operation: "archive", TargetType: "project", Project: req.Project, BackupID: req.BackupID, Mutated: true, CloudHandoffNote: "Local project archive completed. No direct cloud mutation was performed."}, nil
}
func (f *fakeHiveClient) MergeProject(_ context.Context, req hiveclient.ProjectMergeRequest) (hiveclient.ProjectMergeResult, error) {
	f.mergeCalled = true
	f.mergeRequest = req
	return hiveclient.ProjectMergeResult{Operation: "merge", TargetType: "project", SourceProject: req.SourceProject, TargetProject: req.TargetProject, BackupID: req.BackupID, Mutated: true, CloudHandoffNote: "Local project merge metadata recorded. No direct cloud mutation was performed."}, nil
}
