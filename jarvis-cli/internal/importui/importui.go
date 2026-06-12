package importui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

const defaultPollInterval = 500 * time.Millisecond

type Client interface {
	StartEngramImportPreview(context.Context, hiveclient.EngramImportRequest) (hiveclient.EngramImportJob, error)
	StartEngramImportExecute(context.Context, hiveclient.EngramImportRequest) (hiveclient.EngramImportJob, error)
	GetEngramImportJob(context.Context, string) (hiveclient.EngramImportJob, error)
}

type Options struct {
	Source       string
	PreviewID    string
	Out          io.Writer
	IsTTY        bool
	PollInterval time.Duration
	Sleep        func(context.Context, time.Duration) error
}

func RunPreview(ctx context.Context, client Client, opts Options) (hiveclient.EngramImportJob, error) {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	fprintf(opts.Out, opts.IsTTY, "Engram import dry-run\n")
	job, err := client.StartEngramImportPreview(ctx, hiveclient.EngramImportRequest{Source: opts.Source})
	if err != nil {
		return hiveclient.EngramImportJob{}, err
	}
	return poll(ctx, client, job, opts)
}

func RunExecute(ctx context.Context, client Client, opts Options) (hiveclient.EngramImportJob, error) {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	fprintf(opts.Out, opts.IsTTY, "Engram import execute\n")
	job, err := client.StartEngramImportExecute(ctx, hiveclient.EngramImportRequest{Source: opts.Source, PreviewID: opts.PreviewID})
	if err != nil {
		return hiveclient.EngramImportJob{}, err
	}
	return poll(ctx, client, job, opts)
}

func poll(ctx context.Context, client Client, job hiveclient.EngramImportJob, opts Options) (hiveclient.EngramImportJob, error) {
	renderProgress(opts.Out, opts.IsTTY, job)
	for !job.Done {
		if err := sleep(ctx, opts); err != nil {
			return hiveclient.EngramImportJob{}, err
		}
		next, err := client.GetEngramImportJob(ctx, job.ID)
		if err != nil {
			return hiveclient.EngramImportJob{}, err
		}
		job = next
		renderProgress(opts.Out, opts.IsTTY, job)
	}
	if job.Error != "" || job.Phase == hiveclient.EngramImportPhaseFailed {
		reason := strings.TrimSpace(job.Error)
		if reason == "" {
			reason = strings.TrimSpace(job.Message)
		}
		fmt.Fprintf(opts.Out, "Failed: %s\n", reason)
		return job, fmt.Errorf("engram import failed: %s", reason)
	}
	renderReport(opts.Out, job)
	return job, nil
}

func sleep(ctx context.Context, opts Options) error {
	interval := opts.PollInterval
	if interval == 0 {
		interval = defaultPollInterval
	}
	if opts.Sleep != nil {
		return opts.Sleep(ctx, interval)
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func renderProgress(out io.Writer, isTTY bool, job hiveclient.EngramImportJob) {
	line := strings.TrimSpace(strings.Join([]string{string(job.Phase), job.Message, progressText(job)}, " | "))
	line = strings.Trim(line, " |")
	fprintf(out, isTTY, "%s\n", line)
}

func progressText(job hiveclient.EngramImportJob) string {
	if job.Total > 0 {
		return fmt.Sprintf("%d/%d (%d%%)", job.Processed, job.Total, job.Percent)
	}
	if job.Percent > 0 {
		return fmt.Sprintf("%d%%", job.Percent)
	}
	return ""
}

func renderReport(out io.Writer, job hiveclient.EngramImportJob) {
	if job.Report == nil {
		fmt.Fprintln(out, "Completed.")
		return
	}
	report := job.Report
	if job.Kind == hiveclient.EngramImportJobKindPreview {
		fmt.Fprintln(out, "Preview report")
		if report.PreviewID != "" {
			fmt.Fprintf(out, "Preview ID: %s\n", report.PreviewID)
		}
	} else {
		fmt.Fprintln(out, "Import report")
		if report.BackupID != "" {
			fmt.Fprintf(out, "Backup: %s\n", report.BackupID)
		}
		fmt.Fprintf(out, "Imported: %d\n", report.Imported.Imported)
		fmt.Fprintf(out, "Reused: %d\n", report.Imported.Reused)
		fmt.Fprintf(out, "Ambiguous: %d\n", report.Imported.Ambiguous)
		if len(report.AmbiguousDuplicates) > 0 {
			fmt.Fprintln(out, "Ambiguous duplicates:")
			for _, duplicate := range report.AmbiguousDuplicates {
				fmt.Fprintf(out, "- source_id=%s project=%s title=%s reason=%s\n", duplicate.SourceID, duplicate.Project, duplicate.Title, duplicate.Reason)
			}
		}
	}
	if report.SourcePath != "" {
		fmt.Fprintf(out, "Source: %s\n", report.SourcePath)
	}
	if len(report.Projects) > 0 {
		fmt.Fprintf(out, "Projects: %s\n", strings.Join(report.Projects, ", "))
	}
	fmt.Fprintf(out, "Sessions: %d\n", report.Projected.Sessions)
	fmt.Fprintf(out, "Prompts: %d\n", report.Projected.Prompts)
	fmt.Fprintf(out, "Observations: %d\n", report.Projected.Observations)
	if len(report.ProjectedByProject) > 0 {
		fmt.Fprintln(out, "Project impact:")
		for _, impact := range report.ProjectedByProject {
			fmt.Fprintf(out, "- %s: sessions=%d prompts=%d observations=%d\n", impact.Project, impact.Projected.Sessions, impact.Projected.Prompts, impact.Projected.Observations)
		}
	}
	fmt.Fprintf(out, "Skipped relations: %d\n", report.SkippedRelations)
	fmt.Fprintf(out, "Invalid rows: %d\n", len(report.InvalidRows))
}

func fprintf(out io.Writer, isTTY bool, format string, args ...any) {
	if isTTY {
		format = "\r" + format
	}
	fmt.Fprintf(out, format, args...)
}
