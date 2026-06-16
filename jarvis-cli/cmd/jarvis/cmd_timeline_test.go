package main

import (
	"context"
	"strings"
	"testing"
)

func TestTimelineCmdRequiresProject(t *testing.T) {
	originalProject := timelineProject
	timelineProject = ""
	t.Cleanup(func() { timelineProject = originalProject })

	called := false
	originalRun := runTimelineTUI
	runTimelineTUI = func(context.Context, string, string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runTimelineTUI = originalRun })

	err := timelineCmd.RunE(timelineCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--project is required") {
		t.Fatalf("timelineCmd missing project error = %v, want --project requirement", err)
	}
	if called {
		t.Fatal("timeline TUI must not launch when --project is missing")
	}
}

func TestTimelineCmdForwardsProjectAndResolvedDaemonURL(t *testing.T) {
	for _, tt := range []struct {
		name        string
		daemonURL   string
		daemonPort  string
		wantBaseURL string
	}{
		{name: "explicit daemon URL", daemonURL: " http://127.0.0.1:9001 ", wantBaseURL: "http://127.0.0.1:9001"},
		{name: "daemon port", daemonPort: "9444", wantBaseURL: "http://127.0.0.1:9444"},
		{name: "default", wantBaseURL: hiveDaemonDefaultURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HIVE_DAEMON_URL", tt.daemonURL)
			t.Setenv("HIVE_HTTP_PORT", tt.daemonPort)

			originalProject := timelineProject
			timelineProject = "jarvis-ai-devs"
			t.Cleanup(func() { timelineProject = originalProject })

			var gotBaseURL, gotProject string
			var gotContext context.Context
			originalRun := runTimelineTUI
			runTimelineTUI = func(ctx context.Context, baseURL, project string) error {
				gotContext = ctx
				gotBaseURL = baseURL
				gotProject = project
				return nil
			}
			t.Cleanup(func() { runTimelineTUI = originalRun })

			if err := timelineCmd.RunE(timelineCmd, nil); err != nil {
				t.Fatalf("timelineCmd.RunE returned error: %v", err)
			}
			if gotContext != timelineCmd.Context() {
				t.Fatal("timeline command did not forward command context")
			}
			if gotBaseURL != tt.wantBaseURL {
				t.Fatalf("baseURL = %q, want %q", gotBaseURL, tt.wantBaseURL)
			}
			if gotProject != "jarvis-ai-devs" {
				t.Fatalf("project = %q, want jarvis-ai-devs", gotProject)
			}
		})
	}
}
