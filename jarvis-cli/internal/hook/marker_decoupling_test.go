package hook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sessionStartMarkerExists reports whether the dedicated SessionStart marker
// (markerSessionStart) exists for sessionID. It is intentionally distinct from
// MarkerExists, which reports on the first-prompt marker.
func sessionStartMarkerExists(sessionID string) bool {
	_, err := os.Stat(markerPath(sessionID, markerSessionStart))
	return err == nil
}

// okServer returns an httptest.Server answering every request with 200 OK.
func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- Requirement: Distinct SessionStart Marker ---

// TestRunSessionStart_WritesSessionStartMarkerOnly proves the decoupling: a fresh
// SessionStart writes its own markerSessionStart and MUST NOT create the
// first-prompt marker. Before this change, RunSessionStart pre-created the
// first-prompt marker, which suppressed the FIRST ACTION nudge on the first
// prompt (issue #452).
func TestRunSessionStart_WritesSessionStartMarkerOnly(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "decouple-session-start"
	srv := okServer(t)

	var out bytes.Buffer
	payload := `{"session_id":"` + sessionID + `"}`
	RunSessionStart(context.Background(), strings.NewReader(payload), &out, srv.URL)

	if !sessionStartMarkerExists(sessionID) {
		t.Error("SessionStart must create its own markerSessionStart marker")
	}
	if MarkerExists(sessionID) {
		t.Error("SessionStart must NOT create the first-prompt marker (owned by RunPromptSubmit)")
	}
}

// --- Requirement: FIRST ACTION Nudge Fires Once Per Real Session ---

// TestFirstActionNudge_FiresAfterSessionStart is the end-to-end regression for
// issue #452: after RunSessionStart runs, the first user prompt MUST still emit
// FirstPromptSystemMessage, and a subsequent prompt MUST NOT.
func TestFirstActionNudge_FiresAfterSessionStart(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "decouple-nudge-session"
	srv := okServer(t)

	// SessionStart runs first (writes only markerSessionStart).
	var startOut bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"`+sessionID+`"}`), &startOut, srv.URL)

	// First prompt: the nudge MUST fire because the first-prompt marker is absent.
	first := runPromptSubmitResp(t, sessionID, srv.URL)
	if first.SystemMessage != FirstPromptSystemMessage {
		t.Errorf("first prompt after SessionStart must emit FIRST ACTION nudge; got systemMessage=%q raw=%q", first.SystemMessage, first.Raw)
	}
	if !MarkerExists(sessionID) {
		t.Error("first prompt must create the first-prompt marker")
	}

	// Second prompt in the same session: no nudge.
	second := runPromptSubmitResp(t, sessionID, srv.URL)
	if second.SystemMessage == FirstPromptSystemMessage {
		t.Errorf("second prompt must NOT re-emit the FIRST ACTION nudge; raw=%q", second.Raw)
	}
	if second.Raw != "{}" {
		t.Errorf("second prompt (young session) should return {}, got %q", second.Raw)
	}
}

// --- Requirement: Compaction Path Unaffected ---

// TestCompaction_DoesNotRetriggerNudge verifies that a compaction event does not
// re-trigger the FIRST ACTION nudge: the first-prompt marker survives the
// compaction, RunSessionCompact touches no markers, and the next prompt is silent.
func TestCompaction_DoesNotRetriggerNudge(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "decouple-compaction-session"
	srv := okServer(t)

	// The session already had its first-prompt marker created before compaction.
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-1*time.Minute))

	// A compaction occurs — the compact hook must not create or delete any marker.
	var compactOut bytes.Buffer
	RunSessionCompact(context.Background(), strings.NewReader(`{"session_id":"`+sessionID+`"}`), &compactOut)

	if sessionStartMarkerExists(sessionID) {
		t.Error("compaction must NOT create a session-start marker")
	}
	if !MarkerExists(sessionID) {
		t.Error("first-prompt marker must survive a compaction event")
	}

	// A prompt after compaction must not re-fire the nudge.
	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	if resp.SystemMessage == FirstPromptSystemMessage {
		t.Errorf("compaction must not re-trigger the FIRST ACTION nudge; raw=%q", resp.Raw)
	}
}

// --- Requirement: Distinct SessionStart Marker (unit-level lifecycle) ---

// TestCreateSessionStartMarker_Idempotent_PreservesTimestamp proves the
// SessionStart marker honors the same idempotent, timestamp-preserving contract
// as the first-prompt marker, so re-running SessionStart (resume/clear) never
// rewrites the baseline.
func TestCreateSessionStartMarker_Idempotent_PreservesTimestamp(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "decouple-idempotent"

	p := markerPath(sessionID, markerSessionStart)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	if err := os.WriteFile(p, []byte(old.Format(time.RFC3339)+"\n"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	if err := CreateSessionStartMarker(sessionID); err != nil {
		t.Fatalf("CreateSessionStartMarker on existing marker: %v", err)
	}

	got, err := ReadMarkerTime(sessionID, markerSessionStart)
	if err != nil {
		t.Fatalf("ReadMarkerTime: %v", err)
	}
	if !got.Equal(old) {
		t.Errorf("CreateSessionStartMarker must preserve existing timestamp: want %v, got %v", old, got)
	}
}
