package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

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
	client := &fakeHiveClient{
		projects: []hiveclient.Project{{Name: "alpha", ActiveMemoryCount: 3, DeletedMemoryCount: 1}},
		memories: []hiveclient.Memory{{ID: 7, Project: "alpha", Category: "decision", Title: "Governance boundary", Deleted: true}},
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
	if !strings.Contains(memoryOut, "Governance boundary") || !strings.Contains(memoryOut, "deleted=true") {
		t.Fatalf("memories output = %q, want daemon memory rows", memoryOut)
	}

	backupOut, err := executeHiveCommand(t, NewRootCommand(client), "backups")
	if err != nil {
		t.Fatalf("backups command: %v", err)
	}
	if !strings.Contains(backupOut, "backup-1") || !strings.Contains(backupOut, "bytes=42") {
		t.Fatalf("backups output = %q, want daemon backup manifests", backupOut)
	}
}

func TestHiveWarningsCommandReportsNotAvailableClearly(t *testing.T) {
	client := &fakeHiveClient{warningsErr: hiveclient.ErrNotAvailable}
	out, err := executeHiveCommand(t, NewRootCommand(client), "warnings")
	if err != nil {
		t.Fatalf("warnings command should not fail when endpoint is unavailable: %v", err)
	}
	if !strings.Contains(out, "warnings are not available") {
		t.Fatalf("warnings output = %q, want explicit not-available message", out)
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
	memoriesCalled bool
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
