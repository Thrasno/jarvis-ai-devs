package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- RunSessionStart ---

func TestRunSessionStart_HappyPath_InjectsProtocol(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "start-test-session")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	payload := `{"session_id":"start-test-session"}`
	RunSessionStart(context.Background(), strings.NewReader(payload), &out, srv.URL)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %q", err, out.String())
	}
	ctx := resp["additionalContext"]
	if !strings.Contains(ctx, "Hive Memory Protocol") {
		t.Errorf("additionalContext should contain protocol text, got: %q", ctx)
	}
	// Marker should have been created
	if !MarkerExists("start-test-session") {
		t.Error("marker should exist after session-start")
	}
}

func TestRunSessionStart_MarkerAlreadyPresent_StillOutputsProtocol(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "repeated-session")

	// Pre-create marker
	if err := CreateMarker("repeated-session"); err != nil {
		t.Fatalf("pre-create marker: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader("{}"), &out, srv.URL)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["additionalContext"] == "" {
		t.Error("additionalContext should be present even when marker already exists")
	}
}

func TestRunSessionStart_DaemonDown_StillOutputsProtocol(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "no-daemon-session")

	var out bytes.Buffer
	// Use a URL that will be refused
	RunSessionStart(context.Background(), strings.NewReader("{}"), &out, "http://127.0.0.1:19876")

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["additionalContext"] == "" {
		t.Error("additionalContext should be present even when daemon is down")
	}
}

func TestRunSessionStart_MalformedInput_OutputsProtocol(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "malformed-start")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader("{not valid json"), &out, srv.URL)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if resp["additionalContext"] == "" {
		t.Error("should still output protocol even on malformed input")
	}
}

// --- RunSessionCompact ---

func TestRunSessionCompact_OutputsCompactProtocol(t *testing.T) {
	var out bytes.Buffer
	RunSessionCompact(context.Background(), strings.NewReader("{}"), &out)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	ctx := resp["additionalContext"]
	if !strings.Contains(ctx, "Hive Memory Protocol") {
		t.Errorf("compact should contain protocol text, got: %q", ctx)
	}
	if !strings.Contains(ctx, "Post-Compaction Recovery") {
		t.Errorf("compact should contain recovery instructions, got: %q", ctx)
	}
}

func TestRunSessionCompact_DoesNotCreateMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "compact-session-abc")

	// Delete marker if any
	_ = DeleteMarker("compact-session-abc")

	var out bytes.Buffer
	RunSessionCompact(context.Background(), strings.NewReader(`{"session_id":"compact-session-abc"}`), &out)

	if MarkerExists("compact-session-abc") {
		t.Error("session-compact must NOT create a marker file")
	}
}

// --- RunPromptSubmit ---

func TestRunPromptSubmit_FirstPrompt_CreatesMarkerAndReturnsSystemMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "prompt-first-session")

	// Ensure marker does not exist
	_ = DeleteMarker("prompt-first-session")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	payload := `{"prompt":"test","session_id":"prompt-first-session","directory":"/dir","project":"proj"}`
	RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, srv.URL)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["systemMessage"] == "" {
		t.Error("first prompt should return systemMessage")
	}
	if !MarkerExists("prompt-first-session") {
		t.Error("marker should be created on first prompt")
	}
}

func TestRunPromptSubmit_SubsequentPrompt_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "prompt-second-session")

	// Pre-create marker (simulates second+ prompt)
	if err := CreateMarker("prompt-second-session"); err != nil {
		t.Fatalf("create marker: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	payload := `{"prompt":"second","session_id":"prompt-second-session"}`
	RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, srv.URL)

	out_s := out.String()
	if out_s != "{}" {
		t.Errorf("subsequent prompt should return {}, got: %q", out_s)
	}
}

func TestRunPromptSubmit_DaemonDown_StillHandlesMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "prompt-no-daemon")

	_ = DeleteMarker("prompt-no-daemon")

	var out bytes.Buffer
	payload := `{"prompt":"hello","session_id":"prompt-no-daemon"}`
	RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, "http://127.0.0.1:19876")

	// Should still apply first-prompt logic despite daemon being down
	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["systemMessage"] == "" {
		t.Error("should return systemMessage on first prompt even when daemon is down")
	}
}

// --- RunSubagentStop ---

func TestRunSubagentStop_PostsObservation(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv("HIVE_CLAUDE_SESSION_ID", "subagent-session")

	var out bytes.Buffer
	payload := `{"session_id":"subagent-session","cwd":"/work","stdout":"tool output here"}`
	RunSubagentStop(context.Background(), strings.NewReader(payload), &out, srv.URL)

	out_s := out.String()
	if out_s != "{}" {
		t.Errorf("subagent-stop should output {}, got: %q", out_s)
	}
	if receivedPath != "/observations/passive" {
		t.Errorf("should POST to /observations/passive, got: %q", receivedPath)
	}
}

func TestRunSubagentStop_DaemonDown_OutputsEmpty(t *testing.T) {
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "subagent-no-daemon")

	var out bytes.Buffer
	payload := `{"stdout":"output"}`
	RunSubagentStop(context.Background(), strings.NewReader(payload), &out, "http://127.0.0.1:19876")

	if out.String() != "{}" {
		t.Errorf("should output {} when daemon down, got: %q", out.String())
	}
}

// --- RunSessionStop ---

func TestRunSessionStop_PreservesMarkerAndPostsEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "stop-session-1")

	// Pre-create marker
	if err := CreateMarker("stop-session-1"); err != nil {
		t.Fatalf("create marker: %v", err)
	}

	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStop(context.Background(), strings.NewReader(`{"session_id":"stop-session-1"}`), &out, srv.URL)

	if out.String() != "{}" {
		t.Errorf("session-stop should output {}, got: %q", out.String())
	}
	// Marker must NOT be deleted — Stop fires after every agent turn in interactive
	// mode, so deleting it would cause FirstPromptSystemMessage on every prompt.
	if !MarkerExists("stop-session-1") {
		t.Error("marker must be preserved after session-stop; Stop fires after every turn")
	}
	if receivedPath != "/sessions/stop-session-1/end" {
		t.Errorf("should POST to /sessions/{id}/end, got: %q", receivedPath)
	}
}

func TestRunSessionStop_MarkerAbsent_StillPostsEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "stop-no-marker")

	_ = DeleteMarker("stop-no-marker")

	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStop(context.Background(), strings.NewReader("{}"), &out, srv.URL)

	if out.String() != "{}" {
		t.Errorf("session-stop should output {}, got: %q", out.String())
	}
	if !strings.Contains(receivedPath, "/end") {
		t.Errorf("should still POST /sessions/.../end, got: %q", receivedPath)
	}
}

func TestRunSessionStop_404_FromDaemon_NonFatal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "stop-404")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStop(context.Background(), strings.NewReader("{}"), &out, srv.URL)

	if out.String() != "{}" {
		t.Errorf("session-stop with 404 should still output {}, got: %q", out.String())
	}
}

func TestRunSessionStop_DaemonDown_OutputsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "stop-down-daemon")

	var out bytes.Buffer
	RunSessionStop(context.Background(), strings.NewReader("{}"), &out, "http://127.0.0.1:19876")

	if out.String() != "{}" {
		t.Errorf("should output {} when daemon down, got: %q", out.String())
	}
}

// --- Timeout budget tests ---

func TestRunPromptSubmit_CompletesWithinTimeout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "timeout-test-prompt")

	// Slow server: responds after 100ms (well within 1500ms budget)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_ = DeleteMarker("timeout-test-prompt")

	start := time.Now()
	var out bytes.Buffer
	RunPromptSubmit(context.Background(), strings.NewReader(`{"prompt":"test"}`), &out, srv.URL)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("RunPromptSubmit took too long: %v", elapsed)
	}
}
