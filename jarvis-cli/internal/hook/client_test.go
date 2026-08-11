package hook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewDaemonClient_DefaultPort(t *testing.T) {
	t.Setenv("HIVE_HTTP_PORT", "")
	c := NewDaemonClient()
	if c.BaseURL != "http://127.0.0.1:7438" {
		t.Errorf("expected default base URL, got %q", c.BaseURL)
	}
}

func TestNewDaemonClient_CustomPort(t *testing.T) {
	t.Setenv("HIVE_HTTP_PORT", "9999")
	c := NewDaemonClient()
	if c.BaseURL != "http://127.0.0.1:9999" {
		t.Errorf("expected custom port, got %q", c.BaseURL)
	}
}

func TestDaemonClient_PostPrompt_HappyPath(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prompts" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	err := c.PostPrompt(context.Background(), "sid1", "/work/dir", "my-project", "hello prompt")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if received["content"] != "hello prompt" {
		t.Errorf("content: got %q", received["content"])
	}
	if received["session_id"] != "sid1" {
		t.Errorf("session_id: got %q", received["session_id"])
	}
	if received["directory"] != "/work/dir" {
		t.Errorf("directory: got %q", received["directory"])
	}
	if received["project"] != "my-project" {
		t.Errorf("project: got %q", received["project"])
	}
}

func TestDaemonClient_PostPrompt_ServerDown_ReturnsNil(t *testing.T) {
	c := &DaemonClient{BaseURL: "http://127.0.0.1:19999", Timeout: 200 * time.Millisecond}
	err := c.PostPrompt(context.Background(), "sid", "/dir", "proj", "hello")
	// Non-fatal: must return nil even when server is unreachable
	if err != nil {
		t.Errorf("expected nil (non-fatal), got: %v", err)
	}
}

func TestDaemonClient_PostSessionStart_HappyPath(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	err := c.PostSessionStart(context.Background(), "sid2", "my-project", "/work/dir")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if received["id"] != "sid2" {
		t.Errorf("id: got %q", received["id"])
	}
	if received["project"] != "my-project" {
		t.Errorf("project: got %q", received["project"])
	}
	if received["directory"] != "/work/dir" {
		t.Errorf("directory: got %q", received["directory"])
	}
	if received["client"] != "hook" {
		t.Errorf("client: got %q", received["client"])
	}
}

// TestDaemonClient_PostSessionStart_ServerDown_ReturnsError verifies that
// session registration surfaces a real error when the daemon is unreachable.
// Unlike the other fire-and-forget notifications, PostSessionStart reports its
// failure so the caller can log it — a silent registration failure would leave
// a session with no project, undiagnosable (spec: Registration Failures Are
// Logged, Never Swallowed). The caller still treats the error as non-fatal.
func TestDaemonClient_PostSessionStart_ServerDown_ReturnsError(t *testing.T) {
	c := &DaemonClient{BaseURL: "http://127.0.0.1:19999", Timeout: 200 * time.Millisecond}
	err := c.PostSessionStart(context.Background(), "sid", "proj", "/dir")
	if err == nil {
		t.Error("expected a non-nil error when the daemon is unreachable so it can be logged")
	}
}

func TestDaemonClient_PostSessionEnd_HappyPath(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	err := c.PostSessionEnd(context.Background(), "session-abc")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if receivedPath != "/sessions/session-abc/end" {
		t.Errorf("path: got %q, want %q", receivedPath, "/sessions/session-abc/end")
	}
}

func TestDaemonClient_PostSessionEnd_404_ReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	err := c.PostSessionEnd(context.Background(), "ghost-session")
	// 404 must be treated as non-fatal
	if err != nil {
		t.Errorf("expected nil for 404 (non-fatal), got: %v", err)
	}
}

func TestDaemonClient_PostSessionEnd_ServerDown_ReturnsNil(t *testing.T) {
	c := &DaemonClient{BaseURL: "http://127.0.0.1:19999", Timeout: 200 * time.Millisecond}
	err := c.PostSessionEnd(context.Background(), "sid")
	if err != nil {
		t.Errorf("expected nil (non-fatal), got: %v", err)
	}
}

func TestDaemonClient_PostPassiveObservation_HappyPath(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/observations/passive" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	err := c.PostPassiveObservation(context.Background(), "sid3", "proj", "subagent", "some output", "/work/dir")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if received["session_id"] != "sid3" {
		t.Errorf("session_id: got %q", received["session_id"])
	}
	if received["project"] != "proj" {
		t.Errorf("project: got %q", received["project"])
	}
	if received["source"] != "subagent" {
		t.Errorf("source: got %q", received["source"])
	}
	if received["content"] != "some output" {
		t.Errorf("content: got %q", received["content"])
	}
	if received["directory"] != "/work/dir" {
		t.Errorf("directory: got %q", received["directory"])
	}
}

func TestDaemonClient_PostPassiveObservation_ServerDown_ReturnsNil(t *testing.T) {
	c := &DaemonClient{BaseURL: "http://127.0.0.1:19999", Timeout: 200 * time.Millisecond}
	err := c.PostPassiveObservation(context.Background(), "sid", "proj", "subagent", "output", "")
	if err != nil {
		t.Errorf("expected nil (non-fatal), got: %v", err)
	}
}

// --- LatestSaveAt (3-valued: Found / Empty / Unreachable) ---

func TestLatestSaveAt_Found(t *testing.T) {
	want := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"last_save_at": want.Format(time.RFC3339)})
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	ts, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveFound {
		t.Fatalf("status: got %v, want SaveFound", status)
	}
	if !ts.Equal(want) {
		t.Errorf("ts: got %v, want %v", ts, want)
	}
}

func TestLatestSaveAt_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"last_save_at": ""})
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	_, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveEmpty {
		t.Fatalf("status: got %v, want SaveEmpty", status)
	}
}

func TestLatestSaveAt_ServerDown_Unreachable(t *testing.T) {
	c := &DaemonClient{BaseURL: "http://127.0.0.1:19876", Timeout: 300 * time.Millisecond}
	_, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveUnreachable {
		t.Fatalf("status: got %v, want SaveUnreachable", status)
	}
}

func TestLatestSaveAt_Non200_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	_, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveUnreachable {
		t.Fatalf("status: got %v, want SaveUnreachable", status)
	}
}

func TestLatestSaveAt_MalformedJSON_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "not json{")
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	_, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveUnreachable {
		t.Fatalf("status: got %v, want SaveUnreachable", status)
	}
}

func TestLatestSaveAt_UnparseableTimestamp_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"last_save_at": "yesterday"})
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	_, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveUnreachable {
		t.Fatalf("status: got %v, want SaveUnreachable", status)
	}
}

func TestLatestSaveAt_Timeout_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"last_save_at": time.Now().UTC().Format(time.RFC3339)})
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: 50 * time.Millisecond}
	_, status := c.LatestSaveAt(context.Background(), "proj")
	if status != SaveUnreachable {
		t.Fatalf("status: got %v, want SaveUnreachable (timeout)", status)
	}
}

func TestLatestSaveAt_EscapesProjectPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"last_save_at": ""})
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	_, _ = c.LatestSaveAt(context.Background(), "org/team proj")
	want := "/projects/org%2Fteam%20proj/last-save"
	if gotPath != want {
		t.Errorf("escaped path: got %q, want %q", gotPath, want)
	}
}

// MigrationStatus must report every state that is not "ready", not only the
// failure state. A second blocking state exists (the read-only preflight that
// stopped and is waiting for an operator decision), and a state this build does
// not know about may be added later. Going silent about a Hive that is not
// serving is the worse failure, so only "ready" and an absent state are dropped.
func TestDaemonClientMigrationStatusReportsEveryNonReadyState(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantSeen bool
	}{
		{"failed migration", `{"state":"migration-blocked","reason":"canonical conflict"}`, true},
		{"pending operator review", `{"state":"migration-pending-operator-review","reason":"identities are ambiguous","continuation":"jarvis hive → Project normalization"}`, true},
		{"unknown future state", `{"state":"migration-quarantined","reason":"something new"}`, true},
		{"ready", `{"state":"ready"}`, false},
		{"empty state", `{}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/governance/project-identity/status" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
			got := c.MigrationStatus(context.Background())
			if tt.wantSeen != (got != nil) {
				t.Fatalf("MigrationStatus(%s) = %+v, want reported=%v", tt.body, got, tt.wantSeen)
			}
		})
	}
}

// The daemon's own state and reason are what the notice is built from. The
// continuation field is decoded only to keep the wire shape complete; nothing
// renders it — see BuildMigrationProtocol and
// TestMigrationProtocol_NeverRendersDaemonSuppliedContinuation.
func TestDaemonClientMigrationStatusDecodesTheDaemonsOwnState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":"migration-pending-operator-review","reason":"ambiguous","continuation":"attacker-controlled-continuation"}`))
	}))
	defer srv.Close()

	c := &DaemonClient{BaseURL: srv.URL, Timeout: time.Second}
	got := c.MigrationStatus(context.Background())
	if got == nil {
		t.Fatal("MigrationStatus = nil, want the pending status")
	}
	if got.State != MigrationStatePendingOperatorReview || got.Reason != "ambiguous" {
		t.Fatalf("status = %+v, want the daemon's own state and reason", got)
	}
}
