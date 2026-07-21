package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// captureHookLog redirects the package stderr logger to a buffer for the
// duration of the test and restores it afterwards. Hook stdout carries the JSON
// protocol response, so all diagnostics go to stderr and must never leak into
// the hook contract.
func captureHookLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := logger.Writer()
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(prev) })
	return &buf
}

// TestRunSessionStart_PostFailure_LoggedNeverSwallowed verifies that when the
// daemon session-start registration POST fails, the error is captured and
// logged to stderr with a reason — never silently swallowed — while the hook
// still emits valid JSON and never aborts (spec: Registration Failures Are
// Logged, Never Swallowed).
func TestRunSessionStart_PostFailure_LoggedNeverSwallowed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "loud-fail-session")

	logBuf := captureHookLog(t)

	var out bytes.Buffer
	// Point at a refused address so PostSessionStart returns an error.
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"loud-fail-session"}`), &out, "http://127.0.0.1:19876")

	// Hook contract: still valid JSON, still injects protocol, never aborts.
	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("registration failure must still yield valid JSON: %v (%q)", err, out.String())
	}
	if !strings.Contains(resp["additionalContext"], "Hive Memory Protocol") {
		t.Errorf("expected protocol text even on registration failure, got: %q", resp["additionalContext"])
	}

	// Observability: the failure must be logged with the session id and a reason.
	logged := logBuf.String()
	if !strings.Contains(logged, "session-start registration failed") {
		t.Errorf("expected registration failure log line, got: %q", logged)
	}
	if !strings.Contains(logged, "loud-fail-session") {
		t.Errorf("log must identify the session, got: %q", logged)
	}
}

// TestRunSessionStart_PostSucceeds_NoFailureLog verifies that a successful
// registration POST does NOT emit a failure log line — the log is reserved for
// real failures so it stays diagnostic, not noisy.
func TestRunSessionStart_PostSucceeds_NoFailureLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "quiet-ok-session")

	logBuf := captureHookLog(t)

	srv := lastSaveServer(t, 200, "")

	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"quiet-ok-session"}`), &out, srv.URL)

	if strings.Contains(logBuf.String(), "session-start registration failed") {
		t.Errorf("no failure log expected on success, got: %q", logBuf.String())
	}
}
