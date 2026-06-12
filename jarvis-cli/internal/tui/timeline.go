package tui

import (
	"context"
	"os"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveui"
)

// RunTimeline starts the Hive TUI on ScreenTimeline for the given project.
// baseURL is resolved from env vars (same as the hive command) when empty.
func RunTimeline(project string) error {
	baseURL := resolveTimelineDaemonURL()
	return hiveui.RunTimelineTUI(context.Background(), baseURL, project)
}

func resolveTimelineDaemonURL() string {
	if u := strings.TrimSpace(os.Getenv("HIVE_DAEMON_URL")); u != "" {
		return u
	}
	if port := strings.TrimSpace(os.Getenv("HIVE_HTTP_PORT")); port != "" {
		return "http://127.0.0.1:" + port
	}
	return "http://127.0.0.1:7438"
}
