package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/importui"
)

func TestHiveCmd_UseAndShortAreSet(t *testing.T) {
	if hiveCmd.Use != "hive" {
		t.Fatalf("hiveCmd.Use = %q, want %q", hiveCmd.Use, "hive")
	}
	if hiveCmd.Short == "" {
		t.Fatal("hiveCmd.Short is empty, want a non-empty description")
	}
}

func TestHiveCmd_DaemonURLFlag(t *testing.T) {
	f := hiveCmd.Flags().Lookup("daemon-url")
	if f == nil {
		t.Fatal("hiveCmd missing --daemon-url flag")
	}
	if f.DefValue != "" {
		t.Fatalf("--daemon-url default = %q, want empty string", f.DefValue)
	}
}

func TestResolveHiveDaemonURLHonorsManagedRuntimeOverrides(t *testing.T) {
	t.Setenv("HIVE_DAEMON_URL", "http://127.0.0.1:7444")
	t.Setenv("HIVE_HTTP_PORT", "7555")
	if got := resolveHiveDaemonURL(); got != "http://127.0.0.1:7444" {
		t.Fatalf("resolveHiveDaemonURL() = %q, want HIVE_DAEMON_URL override", got)
	}
	t.Setenv("HIVE_DAEMON_URL", "")
	if got := resolveHiveDaemonURL(); got != "http://127.0.0.1:7555" {
		t.Fatalf("resolveHiveDaemonURL() = %q, want loopback port override", got)
	}
}

func TestHiveCmd_RegisteredOnRootCmd(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hive" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("hiveCmd is not registered on rootCmd")
	}
}

func TestHiveCmd_ImportEngramSubcommandRegistered(t *testing.T) {
	cmd, _, err := hiveCmd.Find([]string{"import-engram"})
	if err != nil || cmd == nil || cmd.Name() != "import-engram" {
		t.Fatalf("hive import-engram command = %v, err=%v; want registered subcommand", cmd, err)
	}
}

func TestHiveImportEngramRejectsPositionalArgs(t *testing.T) {
	fake := &fakeHiveImportClient{previewStart: donePreview()}

	_, err := executeHiveImportEngramForTest(t, fake, "", "unexpected")
	if err == nil || !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("positional arg error = %v, want predictable no-args failure", err)
	}
	if fake.previewCalled || fake.executeCalled {
		t.Fatalf("client called after positional arg failure: preview=%v execute=%v", fake.previewCalled, fake.executeCalled)
	}
}

func TestHiveImportEngramRedirectedOutputDoesNotUseTTYCarriageReturns(t *testing.T) {
	originalTerminal := terminalIsTTY
	terminalIsTTY = func() bool { return true }
	t.Cleanup(func() { terminalIsTTY = originalTerminal })
	fake := &fakeHiveImportClient{previewStart: donePreview()}

	out, err := executeHiveImportEngramForTest(t, fake, "", "--dry-run")
	if err != nil {
		t.Fatalf("execute import-engram dry-run: %v", err)
	}
	if strings.Contains(out, "\r") {
		t.Fatalf("redirected output contains carriage return:\n%q", out)
	}
}

func TestHiveImportEngramUsesHiveDaemonURLOverride(t *testing.T) {
	originalDaemonURL := hiveDaemonURL
	hiveDaemonURL = "http://127.0.0.1:9999"
	t.Cleanup(func() { hiveDaemonURL = originalDaemonURL })
	fake := &fakeHiveImportClient{previewStart: donePreview()}

	_, err := executeHiveImportEngramForTest(t, fake, "", "--dry-run")
	if err != nil {
		t.Fatalf("execute import-engram dry-run: %v", err)
	}
	if fake.baseURL != "http://127.0.0.1:9999" {
		t.Fatalf("daemon base URL = %q, want override", fake.baseURL)
	}
}

func TestHiveImportEngramFlow(t *testing.T) {
	t.Run("dry-run uses source and prints preview", func(t *testing.T) {
		fake := &fakeHiveImportClient{previewStart: donePreview()}
		out, err := executeHiveImportEngramForTest(t, fake, "", "--dry-run", "--source", "C:/tmp/engram.db")
		if err != nil || fake.previewSource != "C:/tmp/engram.db" || fake.executeCalled {
			t.Fatalf("dry-run err=%v source=%q execute=%v", err, fake.previewSource, fake.executeCalled)
		}
		assertCmdOutputContains(t, out, "Engram import dry-run", "Preview report", "Preview ID: preview-1", "Projects: alpha", "Observations: 3")
	})
	t.Run("execute rejects missing preview", func(t *testing.T) {
		fake := &fakeHiveImportClient{}
		_, err := executeHiveImportEngramForTest(t, fake, "", "--yes")
		if err == nil || !strings.Contains(err.Error(), "--preview-id") || fake.previewCalled || fake.executeCalled {
			t.Fatalf("missing preview err=%v preview=%v execute=%v", err, fake.previewCalled, fake.executeCalled)
		}
	})
	t.Run("execute with yes forwards preview and prints backup", func(t *testing.T) {
		fake := &fakeHiveImportClient{executeStart: doneExecute()}
		out, err := executeHiveImportEngramForTest(t, fake, "", "--preview-id", "preview-1", "--yes", "--source", "C:/tmp/engram.db")
		if err != nil || fake.executeSource != "C:/tmp/engram.db" || fake.executePreviewID != "preview-1" {
			t.Fatalf("execute err=%v source=%q preview=%q", err, fake.executeSource, fake.executePreviewID)
		}
		assertCmdOutputContains(t, out, "Engram import execute", "Import report", "Backup: backup-1", "Imported: 4", "Reused: 2")
	})
	t.Run("execute requires typed confirmation without yes", func(t *testing.T) {
		fake := &fakeHiveImportClient{executeStart: doneExecute()}
		out, err := executeHiveImportEngramForTest(t, fake, "IMPORT\n", "--preview-id", "preview-1")
		if err != nil || !fake.executeCalled {
			t.Fatalf("confirmed execute err=%v called=%v", err, fake.executeCalled)
		}
		assertCmdOutputContains(t, out, "Type IMPORT to continue", "Import report")

		fake = &fakeHiveImportClient{}
		_, err = executeHiveImportEngramForTest(t, fake, "no\n", "--preview-id", "preview-1")
		if err == nil || !strings.Contains(err.Error(), "confirmation") || fake.executeCalled {
			t.Fatalf("wrong confirmation err=%v execute=%v", err, fake.executeCalled)
		}
	})
}

func executeHiveImportEngramForTest(t *testing.T, client *fakeHiveImportClient, input string, args ...string) (string, error) {
	t.Helper()
	originalFactory := newHiveImportClient
	newHiveImportClient = func(baseURL string) (importui.Client, error) {
		client.baseURL = baseURL
		return client, nil
	}
	t.Cleanup(func() { newHiveImportClient = originalFactory })

	cmd := newHiveImportEngramCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

type fakeHiveImportClient struct {
	baseURL      string
	previewStart hiveclient.EngramImportJob
	executeStart hiveclient.EngramImportJob
	jobs         []hiveclient.EngramImportJob

	previewCalled    bool
	previewSource    string
	executeCalled    bool
	executeSource    string
	executePreviewID string
}

func donePreview() hiveclient.EngramImportJob {
	return hiveclient.EngramImportJob{ID: "preview-1", Kind: hiveclient.EngramImportJobKindPreview, Phase: hiveclient.EngramImportPhaseCompleted, Message: "preview complete", Done: true, Report: &hiveclient.EngramImportReport{PreviewID: "preview-1", SourcePath: "C:/tmp/engram.db", Projects: []string{"alpha"}, Projected: hiveclient.EngramImportEntityCounts{Sessions: 1, Prompts: 2, Observations: 3}}}
}

func doneExecute() hiveclient.EngramImportJob {
	return hiveclient.EngramImportJob{ID: "execute-1", Kind: hiveclient.EngramImportJobKindExecute, Phase: hiveclient.EngramImportPhaseCompleted, Message: "import complete", Done: true, Report: &hiveclient.EngramImportReport{SourcePath: "C:/tmp/engram.db", BackupID: "backup-1", Imported: hiveclient.EngramImportCounts{Imported: 4, Reused: 2}}}
}

func (f *fakeHiveImportClient) StartEngramImportPreview(_ context.Context, req hiveclient.EngramImportRequest) (hiveclient.EngramImportJob, error) {
	f.previewCalled = true
	f.previewSource = req.Source
	return f.previewStart, nil
}

func (f *fakeHiveImportClient) StartEngramImportExecute(_ context.Context, req hiveclient.EngramImportRequest) (hiveclient.EngramImportJob, error) {
	f.executeCalled = true
	f.executeSource = req.Source
	f.executePreviewID = req.PreviewID
	return f.executeStart, nil
}

func (f *fakeHiveImportClient) GetEngramImportJob(_ context.Context, _ string) (hiveclient.EngramImportJob, error) {
	if len(f.jobs) == 0 {
		return f.executeStart, nil
	}
	job := f.jobs[0]
	f.jobs = f.jobs[1:]
	return job, nil
}

func assertCmdOutputContains(t *testing.T, output string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
