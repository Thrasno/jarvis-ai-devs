package hiveui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

// runProgram is the injectable entry point for the Bubble Tea runtime.
// Tests replace this var to avoid launching a real TUI.
var runProgram = func(m interface{ View() string }) error {
	p := tea.NewProgram(m.(tea.Model), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// deriveDashboardState maps a slice of Health records to a DashboardState.
//
//   - empty slice → DashboardLocalOnly (no sync configured)
//   - any entry with a 401 in LastError → DashboardDegraded
//   - any entry with ConsecutiveFailures > 0 or a non-empty LastError → DashboardDegraded
//   - otherwise → DashboardHealthy
func deriveDashboardState(health []hiveclient.Health) DashboardState {
	if len(health) == 0 {
		return DashboardLocalOnly
	}
	for _, h := range health {
		if strings.Contains(h.LastError, "401") {
			return DashboardDegraded
		}
		if h.ConsecutiveFailures > 0 || strings.TrimSpace(h.LastError) != "" {
			return DashboardDegraded
		}
	}
	return DashboardHealthy
}

// LoadSnapshot loads all Snapshot fields via c.
// selectedProject, when non-empty, triggers a Timeline fetch that populates
// snap.TimelineMemories. Timeline errors are silently swallowed (empty slice).
//
// If the Status probe fails with a transport error, it returns a Snapshot with
// DashboardDaemonUnavailable and LoadError set — no other fields are populated.
// Subsequent field errors after a successful Status call are silently omitted
// (the model views handle empty slices gracefully).
func LoadSnapshot(ctx context.Context, c *hiveclient.Client, baseURL string, selectedProject string) Snapshot {
	snap := Snapshot{DaemonURL: baseURL}
	if snap.DaemonURL == "" {
		snap.DaemonURL = "http://127.0.0.1:7438"
	}

	health, err := c.Status(ctx)
	if err != nil {
		snap.DashboardState = DashboardDaemonUnavailable
		snap.LoadError = err
		return snap
	}

	snap.Health = health
	snap.DashboardState = deriveDashboardState(health)

	// Projects.
	projects, err := c.Projects(ctx)
	if err == nil {
		snap.Projects = projects
	}

	// Memories: try bulk empty-filter first; fall back to per-project on *APIError.
	memories, err := c.Memories(ctx, hiveclient.MemoryFilter{})
	if err != nil {
		var apiErr *hiveclient.APIError
		if isAPIError(err, &apiErr) {
			// Fall back: load memories per project.
			for _, p := range snap.Projects {
				pm, perr := c.Memories(ctx, hiveclient.MemoryFilter{Project: p.Name})
				if perr == nil {
					memories = append(memories, pm...)
				}
			}
		}
		// Non-API errors (transport) are silently dropped; snap.Memories stays nil.
	}
	snap.Memories = memories

	// Timeline memories — only when a project is selected.
	if selectedProject != "" {
		tr, terr := c.Timeline(ctx, selectedProject)
		if terr != nil {
			snap.TimelineMemories = []hiveclient.Memory{}
		} else {
			snap.TimelineMemories = tr.Memories
			snap.TimelineTruncated = tr.Truncated
		}
	}

	// Warnings.
	warnings, err := c.Warnings(ctx)
	if err == nil {
		snap.Warnings = warnings
	}

	// Backups.
	backups, err := c.Backups(ctx)
	if err == nil {
		snap.Backups = backups
	}

	return snap
}

// isAPIError checks whether err is of type *hiveclient.APIError and sets target.
func isAPIError(err error, target **hiveclient.APIError) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*hiveclient.APIError)
	if ok {
		*target = apiErr
	}
	return ok
}

// RunHiveTUI creates a live hiveclient.Client, loads a Snapshot, wires all
// three executor interfaces to the same client, and starts the Bubble Tea TUI.
// If the daemon is unreachable, the TUI starts with DashboardDaemonUnavailable
// and RunHiveTUI still returns nil (the user sees the offline screen).
func RunHiveTUI(ctx context.Context, baseURL string) error {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:7438"
	}

	client, err := hiveclient.New(baseURL)
	if err != nil {
		return err
	}

	snap := LoadSnapshot(ctx, client, baseURL, "")
	m := NewModelWithConfig(snap, client, client, client, client, client, client, client)

	return runProgram(m)
}

// RunTimelineTUI starts the Hive TUI with ScreenTimeline as the initial screen
// for the given project. It loads TimelineMemories from the daemon before the
// TUI renders. If the daemon is unreachable, the TUI shows the offline screen.
func RunTimelineTUI(ctx context.Context, baseURL string, project string) error {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:7438"
	}

	client, err := hiveclient.New(baseURL)
	if err != nil {
		return err
	}

	snap := LoadSnapshot(ctx, client, baseURL, project)

	// Locate the project index so the selected project is pre-wired.
	projectIndex := 0
	for i, p := range snap.Projects {
		if p.Name == project {
			projectIndex = i
			break
		}
	}

	m := NewModelWithConfig(snap, client, client, client, client, client, client, client)
	m.screen = ScreenTimeline
	m.projectIndex = projectIndex

	return runProgram(m)
}
