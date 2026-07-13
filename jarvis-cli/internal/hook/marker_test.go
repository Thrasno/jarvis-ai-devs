package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSafeSessionID_ReplacesSlash(t *testing.T) {
	got := safeSessionID("abc/def")
	if strings.Contains(got, "/") {
		t.Errorf("safeSessionID should replace /: got %q", got)
	}
}

func TestSafeSessionID_ReplacesBackslash(t *testing.T) {
	got := safeSessionID(`abc\def`)
	if strings.Contains(got, `\`) {
		t.Errorf("safeSessionID should replace backslash: got %q", got)
	}
}

func TestSafeSessionID_ReplacesAllForbiddenChars(t *testing.T) {
	input := `abc/\:*?"<>| def`
	got := safeSessionID(input)
	forbidden := []string{"/", `\`, ":", "*", "?", `"`, "<", ">", "|", " "}
	for _, ch := range forbidden {
		if strings.Contains(got, ch) {
			t.Errorf("safeSessionID should replace %q: got %q", ch, got)
		}
	}
}

func TestSafeSessionID_PreservesAlphanumericAndDash(t *testing.T) {
	input := "abc-123_foo"
	got := safeSessionID(input)
	if got != input {
		t.Errorf("safeSessionID should preserve %q, got %q", input, got)
	}
}

func TestSafeSessionID_EmptyInput(t *testing.T) {
	got := safeSessionID("")
	if got != "" {
		t.Errorf("safeSessionID of empty should be empty, got %q", got)
	}
}

func TestMarkerPath_ContainsSessionID(t *testing.T) {
	p := markerPath("mysession", markerFirstPrompt)
	if !strings.Contains(p, "mysession") {
		t.Errorf("markerPath should contain session ID: got %q", p)
	}
}

func TestMarkerPath_ContainsJarvisHiveSubpath(t *testing.T) {
	p := markerPath("s1", markerFirstPrompt)
	if !strings.Contains(p, "jarvis-hive") {
		t.Errorf("markerPath should contain jarvis-hive: got %q", p)
	}
	if !strings.Contains(p, "claude-hooks") {
		t.Errorf("markerPath should contain claude-hooks: got %q", p)
	}
	if !strings.Contains(p, "first-prompt-") {
		t.Errorf("markerPath should contain first-prompt-: got %q", p)
	}
	if !strings.HasSuffix(p, ".done") {
		t.Errorf("markerPath should end with .done: got %q", p)
	}
}

func TestMarkerPath_UsesXDGRuntimeDir_WhenSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	// Clear lower-priority vars
	t.Setenv("TMPDIR", "")
	t.Setenv("TEMP", "")
	t.Setenv("TMP", "")

	p := markerPath("s1", markerFirstPrompt)
	if !strings.HasPrefix(p, dir) {
		t.Errorf("expected path to start with XDG_RUNTIME_DIR %q, got %q", dir, p)
	}
}

func TestMarkerPath_UsesTMPDIR_WhenXDGAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", dir)
	t.Setenv("TEMP", "")
	t.Setenv("TMP", "")

	p := markerPath("s1", markerFirstPrompt)
	if !strings.HasPrefix(p, dir) {
		t.Errorf("expected path to start with TMPDIR %q, got %q", dir, p)
	}
}

func TestMarkerPath_UsesNameAsPrefix(t *testing.T) {
	p := markerPath("s1", markerMemoryReminder)
	if !strings.Contains(p, "memory-reminder-s1.done") {
		t.Errorf("markerPath should use the name as prefix: got %q", p)
	}
}

func TestReadMarkerTime_ParsesStoredTimestamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "read-marker-ok"
	if err := RewriteMarker(sessionID, markerMemoryReminder); err != nil {
		t.Fatalf("RewriteMarker: %v", err)
	}

	ts, err := ReadMarkerTime(sessionID, markerMemoryReminder)
	if err != nil {
		t.Fatalf("ReadMarkerTime: %v", err)
	}
	if ts.IsZero() {
		t.Error("ReadMarkerTime should return a non-zero timestamp")
	}
	if time.Since(ts) > time.Minute {
		t.Errorf("timestamp should be recent, got %v", ts)
	}
}

func TestReadMarkerTime_MissingFile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	if _, err := ReadMarkerTime("no-such-session", markerMemoryReminder); err == nil {
		t.Error("ReadMarkerTime should error when the marker file is missing")
	}
}

func TestReadMarkerTime_CorruptContent_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "corrupt-marker"
	p := markerPath(sessionID, markerMemoryReminder)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("not-a-timestamp\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := ReadMarkerTime(sessionID, markerMemoryReminder); err == nil {
		t.Error("ReadMarkerTime should error on corrupt (unparseable) content")
	}
}

func TestRewriteMarker_OverwritesTimestamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "rewrite-marker"
	// Seed with an old timestamp.
	p := markerPath(sessionID, markerMemoryReminder)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(p, []byte(old+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := RewriteMarker(sessionID, markerMemoryReminder); err != nil {
		t.Fatalf("RewriteMarker: %v", err)
	}
	ts, err := ReadMarkerTime(sessionID, markerMemoryReminder)
	if err != nil {
		t.Fatalf("ReadMarkerTime: %v", err)
	}
	if time.Since(ts) > time.Minute {
		t.Errorf("RewriteMarker should overwrite with a fresh timestamp, got %v", ts)
	}
}

func TestCreateMarker_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "test-create-marker"
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("CreateMarker returned error: %v", err)
	}

	p := markerPath(sessionID, markerFirstPrompt)
	if _, err := os.Stat(p); err != nil {
		t.Errorf("marker file not created at %q: %v", p, err)
	}
}

func TestCreateMarker_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "test-parent-dirs"
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("CreateMarker returned error: %v", err)
	}

	// Verify the parent directory was created
	p := markerPath(sessionID, markerFirstPrompt)
	parent := filepath.Dir(p)
	if _, err := os.Stat(parent); err != nil {
		t.Errorf("parent dir not created at %q: %v", parent, err)
	}
}

func TestCreateMarker_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "test-idempotent"
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("first CreateMarker: %v", err)
	}
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("second CreateMarker (idempotent): %v", err)
	}
}

// TestCreateMarker_PreservesTimestampWhenExists proves CreateMarker honors its
// documented idempotent contract: a second call on an existing marker must NOT
// rewrite the stored timestamp. This is the session-age baseline used by the
// mid-session memory reminder; a resume (which re-runs SessionStart →
// CreateMarker) must not reset that baseline.
func TestCreateMarker_PreservesTimestampWhenExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "test-preserve-timestamp"

	// Seed the marker with an old timestamp to simulate an existing session.
	p := markerPath(sessionID, markerFirstPrompt)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	if err := os.WriteFile(p, []byte(old.Format(time.RFC3339)+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// A second CreateMarker (e.g. on resume) must leave the timestamp untouched.
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("CreateMarker on existing marker: %v", err)
	}

	got, err := ReadMarkerTime(sessionID, markerFirstPrompt)
	if err != nil {
		t.Fatalf("ReadMarkerTime: %v", err)
	}
	if !got.Equal(old) {
		t.Errorf("CreateMarker must preserve existing timestamp: want %v, got %v", old, got)
	}
}

func TestDeleteMarker_DeletesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "test-delete"
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("CreateMarker: %v", err)
	}

	if err := DeleteMarker(sessionID); err != nil {
		t.Fatalf("DeleteMarker: %v", err)
	}

	p := markerPath(sessionID, markerFirstPrompt)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("marker file should be deleted, got stat err: %v", err)
	}
}

func TestDeleteMarker_NonFatal_WhenNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	// Delete without creating first — must not error
	if err := DeleteMarker("nonexistent-session"); err != nil {
		t.Errorf("DeleteMarker should be non-fatal when file absent, got: %v", err)
	}
}

func TestMarkerExists_ReturnsTrueWhenPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "test-exists-true"
	if err := CreateMarker(sessionID); err != nil {
		t.Fatalf("CreateMarker: %v", err)
	}

	if !MarkerExists(sessionID) {
		t.Error("MarkerExists should return true after CreateMarker")
	}
}

func TestMarkerExists_ReturnsFalseWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	if MarkerExists("nonexistent-session-xyz") {
		t.Error("MarkerExists should return false for nonexistent marker")
	}
}

func TestCreateMarkerExclusive_FirstCall_ReturnsCreatedTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	created, err := CreateMarkerExclusive("exclusive-session-1")
	if err != nil {
		t.Fatalf("CreateMarkerExclusive returned unexpected error: %v", err)
	}
	if !created {
		t.Error("first call to CreateMarkerExclusive should return created=true")
	}
	// Marker file must exist after creation.
	if !MarkerExists("exclusive-session-1") {
		t.Error("marker file should exist after CreateMarkerExclusive created=true")
	}
}

func TestCreateMarkerExclusive_SecondCall_ReturnsCreatedFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	// First call creates the marker.
	created1, err := CreateMarkerExclusive("exclusive-session-2")
	if err != nil {
		t.Fatalf("first CreateMarkerExclusive: %v", err)
	}
	if !created1 {
		t.Error("first call should return created=true")
	}

	// Second call on the same session must not create again.
	created2, err := CreateMarkerExclusive("exclusive-session-2")
	if err != nil {
		t.Fatalf("second CreateMarkerExclusive returned unexpected error: %v", err)
	}
	if created2 {
		t.Error("second call to CreateMarkerExclusive should return created=false")
	}
}

func TestCreateMarkerExclusive_OnlyOneSystemMessage(t *testing.T) {
	// Simulates two concurrent-style calls; only the winner gets created=true
	// and should emit systemMessage.
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	sessionID := "exclusive-race-session"

	systemMessages := 0
	for i := 0; i < 2; i++ {
		created, err := CreateMarkerExclusive(sessionID)
		if err != nil {
			t.Fatalf("CreateMarkerExclusive call %d: %v", i, err)
		}
		if created {
			systemMessages++
		}
	}

	if systemMessages != 1 {
		t.Errorf("expected exactly 1 systemMessage emission, got %d", systemMessages)
	}
}
