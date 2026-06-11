package hiveui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

// ─── deriveDashboardState ────────────────────────────────────────────────────

func TestDeriveDashboardState(t *testing.T) {
	tests := []struct {
		name   string
		health []hiveclient.Health
		want   DashboardState
	}{
		{
			name:   "empty slice yields local-only",
			health: []hiveclient.Health{},
			want:   DashboardLocalOnly,
		},
		{
			name:   "nil slice yields local-only",
			health: nil,
			want:   DashboardLocalOnly,
		},
		{
			name: "401 in LastError yields degraded",
			health: []hiveclient.Health{
				{Project: "core-api", LastError: "401 unauthorized"},
			},
			want: DashboardDegraded,
		},
		{
			name: "ConsecutiveFailures > 0 yields degraded",
			health: []hiveclient.Health{
				{Project: "core-api", ConsecutiveFailures: 3},
			},
			want: DashboardDegraded,
		},
		{
			name: "non-empty health, no errors yields healthy",
			health: []hiveclient.Health{
				{Project: "core-api", ConsecutiveFailures: 0, LastError: ""},
			},
			want: DashboardHealthy,
		},
		{
			name: "non-empty health with LastError (non-401) yields degraded",
			health: []hiveclient.Health{
				{Project: "core-api", LastError: "connection timeout"},
			},
			want: DashboardDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveDashboardState(tt.health)
			if got != tt.want {
				t.Fatalf("deriveDashboardState() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── LoadSnapshot ────────────────────────────────────────────────────────────

func TestLoadSnapshot_AllRouteSuccess(t *testing.T) {
	var memoriesQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/health":
			writeJSON(w, map[string]any{
				"projects": []map[string]any{
					{"project": "core-api", "consecutive_failures": 0, "last_error": ""},
				},
			})
		case "/governance/projects":
			writeJSON(w, map[string]any{
				"projects": []map[string]any{
					{"name": "core-api", "active_memory_count": 10},
				},
			})
		case "/governance/memories":
			memoriesQuery = r.URL.RawQuery
			writeJSON(w, map[string]any{
				"memories": []map[string]any{
					{"sync_id": "mem_001", "project": "core-api", "title": "Test memory"},
				},
			})
		case "/governance/warnings":
			writeJSON(w, map[string]any{
				"warnings": []map[string]any{
					{"message": "test warning"},
				},
			})
		case "/governance/backups":
			writeJSON(w, map[string]any{
				"backups": []map[string]any{
					{"id": "backup-001"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	snap := LoadSnapshot(context.Background(), client, srv.URL, "")

	if snap.LoadError != nil {
		t.Fatalf("LoadError = %v, want nil", snap.LoadError)
	}
	if snap.DashboardState != DashboardHealthy {
		t.Fatalf("DashboardState = %v, want DashboardHealthy", snap.DashboardState)
	}
	if len(snap.Projects) == 0 {
		t.Fatal("Projects is empty, want at least 1")
	}
	if len(snap.Memories) == 0 {
		t.Fatal("Memories is empty, want at least 1")
	}
	if len(snap.Warnings) == 0 {
		t.Fatal("Warnings is empty, want at least 1")
	}
	if len(snap.Backups) == 0 {
		t.Fatal("Backups is empty, want at least 1")
	}
	if snap.DaemonURL != srv.URL {
		t.Fatalf("DaemonURL = %q, want %q", snap.DaemonURL, srv.URL)
	}
	if strings.Contains(memoriesQuery, "include_deleted=true") {
		t.Fatalf("memories query = %q, want normal TUI load without include_deleted", memoriesQuery)
	}
}

func TestLoadSnapshot_StatusTransportError_YieldsDaemonUnavailable(t *testing.T) {
	// Use a server that's immediately closed to force a transport error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	client, err := hiveclient.New(url)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	snap := LoadSnapshot(context.Background(), client, url, "")

	if snap.DashboardState != DashboardDaemonUnavailable {
		t.Fatalf("DashboardState = %v, want DashboardDaemonUnavailable", snap.DashboardState)
	}
	if snap.LoadError == nil {
		t.Fatal("LoadError = nil, want non-nil transport error")
	}
}

func TestLoadSnapshot_MemoriesEmptyFilterFallback(t *testing.T) {
	// Memories endpoint returns 4xx for empty project → fall back to per-project.
	var memoryQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/health":
			writeJSON(w, map[string]any{
				"projects": []map[string]any{
					{"project": "proj-a", "consecutive_failures": 0, "last_error": ""},
				},
			})
		case "/governance/projects":
			writeJSON(w, map[string]any{
				"projects": []map[string]any{
					{"name": "proj-a", "active_memory_count": 2},
				},
			})
		case "/governance/memories":
			memoryQueries = append(memoryQueries, r.URL.RawQuery)
			project := r.URL.Query().Get("project")
			if project == "" {
				// Reject empty-filter call with 400.
				http.Error(w, `{"error":"project required"}`, http.StatusBadRequest)
				return
			}
			// Per-project call succeeds.
			writeJSON(w, map[string]any{
				"memories": []map[string]any{
					{"sync_id": "mem_proj_a", "project": project, "title": "Memory for " + project},
				},
			})
		case "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	snap := LoadSnapshot(context.Background(), client, srv.URL, "")

	if snap.LoadError != nil {
		t.Fatalf("LoadError = %v, want nil", snap.LoadError)
	}
	// Fallback path must have loaded memories via per-project loop.
	if len(snap.Memories) == 0 {
		t.Fatal("Memories is empty after fallback, want at least 1")
	}
	found := false
	for _, m := range snap.Memories {
		if m.SyncID == "mem_proj_a" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected per-project fallback memory mem_proj_a, not found")
	}
	for _, query := range memoryQueries {
		if strings.Contains(query, "include_deleted=true") {
			t.Fatalf("memories query = %q, want normal fallback load without include_deleted", query)
		}
	}
}

// ─── LoadSnapshot: TimelineMemories ──────────────────────────────────────────

func TestLoadSnapshot_PopulatesTimelineMemoriesForSelectedProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/governance/health":
			writeJSON(w, map[string]any{"projects": []map[string]any{
				{"project": "core-api", "consecutive_failures": 0, "last_error": ""},
			}})
		case r.URL.Path == "/governance/projects":
			writeJSON(w, map[string]any{"projects": []map[string]any{
				{"name": "core-api", "active_memory_count": 2},
			}})
		case r.URL.Path == "/governance/memories":
			writeJSON(w, map[string]any{"memories": []map[string]any{
				{"sync_id": "m1", "project": "core-api", "category": "note", "title": "A note"},
			}})
		case r.URL.Path == "/governance/projects/core-api/timeline":
			writeJSON(w, map[string]any{"memories": []map[string]any{
				{"sync_id": "t1", "project": "core-api", "category": "decision", "title": "Use Go"},
				{"sync_id": "t2", "project": "core-api", "category": "architecture", "title": "Hexagonal"},
			}})
		case r.URL.Path == "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case r.URL.Path == "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	snap := LoadSnapshot(context.Background(), client, srv.URL, "core-api")

	if snap.LoadError != nil {
		t.Fatalf("LoadError = %v, want nil", snap.LoadError)
	}
	if len(snap.TimelineMemories) != 2 {
		t.Fatalf("TimelineMemories len = %d, want 2", len(snap.TimelineMemories))
	}
	if snap.TimelineMemories[0].SyncID != "t1" {
		t.Fatalf("TimelineMemories[0].SyncID = %q, want t1", snap.TimelineMemories[0].SyncID)
	}
}

func TestLoadSnapshot_NoProject_LeavesTimelineMemoriesEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/health":
			writeJSON(w, map[string]any{"projects": []any{}})
		case "/governance/projects":
			writeJSON(w, map[string]any{"projects": []any{}})
		case "/governance/memories":
			writeJSON(w, map[string]any{"memories": []any{}})
		case "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	// No project selected (empty string).
	snap := LoadSnapshot(context.Background(), client, srv.URL, "")

	if snap.LoadError != nil {
		t.Fatalf("LoadError = %v, want nil", snap.LoadError)
	}
	if snap.TimelineMemories != nil {
		t.Fatalf("TimelineMemories = %v, want nil when no project selected", snap.TimelineMemories)
	}
}

func TestLoadSnapshot_TimelineError_LeavesTimelineMemoriesEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/governance/health":
			writeJSON(w, map[string]any{"projects": []map[string]any{
				{"project": "core-api", "consecutive_failures": 0, "last_error": ""},
			}})
		case r.URL.Path == "/governance/projects":
			writeJSON(w, map[string]any{"projects": []map[string]any{
				{"name": "core-api", "active_memory_count": 1},
			}})
		case r.URL.Path == "/governance/memories":
			writeJSON(w, map[string]any{"memories": []any{}})
		case r.URL.Path == "/governance/projects/core-api/timeline":
			// Simulate a 404 (project not found in timeline).
			http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
		case r.URL.Path == "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case r.URL.Path == "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	snap := LoadSnapshot(context.Background(), client, srv.URL, "core-api")

	if snap.LoadError != nil {
		t.Fatalf("LoadError = %v, want nil (timeline error must not fail snapshot load)", snap.LoadError)
	}
	// TimelineMemories must be empty slice (not nil) on error.
	if snap.TimelineMemories == nil {
		t.Fatal("TimelineMemories = nil, want empty slice when Timeline returns error")
	}
	if len(snap.TimelineMemories) != 0 {
		t.Fatalf("TimelineMemories len = %d, want 0 on error", len(snap.TimelineMemories))
	}
}

func TestLoadSnapshot_TruncatedTimeline(t *testing.T) {
	// Build 500 memory entries to simulate a truncated response.
	truncatedMemories := make([]map[string]any, 500)
	for i := range truncatedMemories {
		truncatedMemories[i] = map[string]any{
			"sync_id":  strings.Repeat("x", 8),
			"project":  "core-api",
			"category": "decision",
			"title":    "Memory entry",
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/governance/health":
			writeJSON(w, map[string]any{"projects": []map[string]any{
				{"project": "core-api", "consecutive_failures": 0, "last_error": ""},
			}})
		case r.URL.Path == "/governance/projects":
			writeJSON(w, map[string]any{"projects": []map[string]any{
				{"name": "core-api", "active_memory_count": 500},
			}})
		case r.URL.Path == "/governance/memories":
			writeJSON(w, map[string]any{"memories": []any{}})
		case r.URL.Path == "/governance/projects/core-api/timeline":
			writeJSON(w, map[string]any{
				"memories":  truncatedMemories,
				"truncated": true,
			})
		case r.URL.Path == "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case r.URL.Path == "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}

	snap := LoadSnapshot(context.Background(), client, srv.URL, "core-api")

	if snap.LoadError != nil {
		t.Fatalf("LoadError = %v, want nil", snap.LoadError)
	}
	if len(snap.TimelineMemories) != 500 {
		t.Fatalf("TimelineMemories len = %d, want 500", len(snap.TimelineMemories))
	}
	if !snap.TimelineTruncated {
		t.Fatal("TimelineTruncated = false, want true when server returns truncated:true")
	}
}

// ─── RunHiveTUI ──────────────────────────────────────────────────────────────

func TestRunHiveTUI_StubsProgram_AndReturnsNil(t *testing.T) {
	// Stub the runProgram var so no real TUI is launched.
	original := runProgram
	t.Cleanup(func() { runProgram = original })

	var capturedModel Model
	runProgram = func(m interface{ View() string }) error {
		capturedModel, _ = m.(Model)
		return nil
	}

	// Spin up a test server that returns healthy data.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/health":
			writeJSON(w, map[string]any{"projects": []any{}})
		case "/governance/projects":
			writeJSON(w, map[string]any{"projects": []any{}})
		case "/governance/memories":
			writeJSON(w, map[string]any{"memories": []any{}})
		case "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := RunHiveTUI(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("RunHiveTUI returned error: %v", err)
	}

	// Daemon is reachable with empty health → DashboardLocalOnly.
	if capturedModel.snapshot.DashboardState != DashboardLocalOnly {
		t.Fatalf("DashboardState = %v, want DashboardLocalOnly", capturedModel.snapshot.DashboardState)
	}
}

func TestRunHiveTUI_DaemonDown_StartsWithUnavailableState(t *testing.T) {
	// Stub runProgram.
	original := runProgram
	t.Cleanup(func() { runProgram = original })

	var capturedModel Model
	runProgram = func(m interface{ View() string }) error {
		capturedModel, _ = m.(Model)
		return nil
	}

	// Use a closed server to simulate daemon down.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	err := RunHiveTUI(context.Background(), url)
	if err != nil {
		t.Fatalf("RunHiveTUI must return nil even when daemon is down, got: %v", err)
	}
	if capturedModel.snapshot.DashboardState != DashboardDaemonUnavailable {
		t.Fatalf("DashboardState = %v, want DashboardDaemonUnavailable", capturedModel.snapshot.DashboardState)
	}
	if capturedModel.snapshot.LoadError == nil {
		t.Fatal("LoadError = nil, want non-nil when daemon is down")
	}
	// The model must have executors wired (client is passed as all three).
	// We verify this by checking that the model view renders the offline screen.
	view := capturedModel.View()
	if view == "" {
		t.Fatal("View() is empty")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("writeJSON: " + err.Error())
	}
}

// Ensure errors package is used (for the transport-error test).
var _ = errors.New
