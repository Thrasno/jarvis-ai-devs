package importui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestRunPreviewShowsProgressAndFinalReport(t *testing.T) {
	client := &fakeImportClient{
		startPreview: job("preview-1", hiveclient.EngramImportJobKindPreview, hiveclient.EngramImportPhaseDiscovery, "scanning Engram projects", 1, 4, 25, false),
		jobs: []hiveclient.EngramImportJob{{ID: "preview-1", Kind: hiveclient.EngramImportJobKindPreview, Phase: hiveclient.EngramImportPhaseCompleted, Message: "preview complete", Processed: 4, Total: 4, Percent: 100, Done: true, Report: &hiveclient.EngramImportReport{
			PreviewID: "preview-1", SourcePath: "C:/tmp/engram.db", Projects: []string{"alpha", "beta"},
			Projected: hiveclient.EngramImportEntityCounts{Sessions: 2, Prompts: 3, Observations: 5}, SkippedRelations: 7,
			InvalidRows: []hiveclient.EngramImportInvalidRow{{Table: "observations", SourceID: "22", Reason: "missing session"}},
		}}},
	}
	var out bytes.Buffer

	got, err := RunPreview(context.Background(), client, Options{Source: "C:/tmp/engram.db", Out: &out, Sleep: noSleep})
	if err != nil {
		t.Fatalf("RunPreview: %v", err)
	}
	if client.previewSource != "C:/tmp/engram.db" || !got.Done || got.Report == nil {
		t.Fatalf("preview source=%q job=%+v, want completed preview", client.previewSource, got)
	}
	assertOutputContains(t, out.String(), "Engram import dry-run", "discovery", "1/4 (25%)", "Preview report", "Projects: alpha, beta", "Sessions: 2", "Prompts: 3", "Observations: 5", "Skipped relations: 7", "Invalid rows: 1")

	out.Reset()
	renderProgress(&out, true, client.startPreview)
	assertOutputContains(t, out.String(), "discovery", "1/4 (25%)")
}

func TestRunExecuteShowsBackupProgressAndImportReport(t *testing.T) {
	client := &fakeImportClient{
		startExecute: job("execute-1", hiveclient.EngramImportJobKindExecute, hiveclient.EngramImportPhaseBackup, "creating Hive backup", 0, 1, 0, false),
		jobs: []hiveclient.EngramImportJob{
			job("execute-1", hiveclient.EngramImportJobKindExecute, hiveclient.EngramImportPhaseImport, "importing rows", 3, 6, 50, false),
			{ID: "execute-1", Kind: hiveclient.EngramImportJobKindExecute, Phase: hiveclient.EngramImportPhaseCompleted, Message: "import complete", Processed: 6, Total: 6, Percent: 100, Done: true, Report: &hiveclient.EngramImportReport{BackupID: "backup-1", Imported: hiveclient.EngramImportCounts{Imported: 4, Reused: 2}}},
		},
	}
	var out bytes.Buffer

	_, err := RunExecute(context.Background(), client, Options{Source: "C:/tmp/engram.db", PreviewID: "preview-1", Out: &out, Sleep: noSleep})
	if err != nil {
		t.Fatalf("RunExecute: %v", err)
	}
	if client.executeSource != "C:/tmp/engram.db" || client.executePreviewID != "preview-1" {
		t.Fatalf("execute source=%q preview=%q, want forwarded request", client.executeSource, client.executePreviewID)
	}
	assertOutputContains(t, out.String(), "Engram import execute", "backup", "creating Hive backup", "import", "3/6 (50%)", "Import report", "Backup: backup-1", "Imported: 4", "Reused: 2")
}

func TestRunExecuteReportsFailedJobReason(t *testing.T) {
	client := &fakeImportClient{startExecute: job("execute-failed", hiveclient.EngramImportJobKindExecute, hiveclient.EngramImportPhaseBackup, "creating backup", 0, 0, 0, false), jobs: []hiveclient.EngramImportJob{{ID: "execute-failed", Kind: hiveclient.EngramImportJobKindExecute, Phase: hiveclient.EngramImportPhaseFailed, Message: "source missing", Done: true, Error: "source missing"}}}
	var out bytes.Buffer

	_, err := RunExecute(context.Background(), client, Options{PreviewID: "preview-1", Out: &out, Sleep: noSleep})
	if err == nil || !strings.Contains(err.Error(), "source missing") {
		t.Fatalf("RunExecute error = %v, want source missing", err)
	}
	assertOutputContains(t, out.String(), "Failed: source missing")
}

type fakeImportClient struct {
	startPreview, startExecute   hiveclient.EngramImportJob
	jobs                         []hiveclient.EngramImportJob
	previewSource, executeSource string
	executePreviewID             string
}

func (f *fakeImportClient) StartEngramImportPreview(_ context.Context, req hiveclient.EngramImportRequest) (hiveclient.EngramImportJob, error) {
	f.previewSource = req.Source
	return f.startPreview, nil
}
func (f *fakeImportClient) StartEngramImportExecute(_ context.Context, req hiveclient.EngramImportRequest) (hiveclient.EngramImportJob, error) {
	f.executeSource, f.executePreviewID = req.Source, req.PreviewID
	return f.startExecute, nil
}
func (f *fakeImportClient) GetEngramImportJob(context.Context, string) (hiveclient.EngramImportJob, error) {
	job := f.jobs[0]
	f.jobs = f.jobs[1:]
	return job, nil
}

func job(id string, kind hiveclient.EngramImportJobKind, phase hiveclient.EngramImportPhase, msg string, processed, total, percent int, done bool) hiveclient.EngramImportJob {
	return hiveclient.EngramImportJob{ID: id, Kind: kind, Phase: phase, Message: msg, Processed: processed, Total: total, Percent: percent, Done: done}
}
func noSleep(context.Context, time.Duration) error { return nil }
func assertOutputContains(t *testing.T, output string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
