package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// seedFirstPromptMarker writes the first-prompt marker for sessionID with a
// specific timestamp so tests can control session age.
func seedFirstPromptMarker(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	p := markerPath(sessionID, markerFirstPrompt)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir marker dir: %v", err)
	}
	if err := os.WriteFile(p, []byte(at.UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		t.Fatalf("write first-prompt marker: %v", err)
	}
}

// lastSaveServer returns an httptest.Server that answers the last-save GET with
// the given JSON body/status and any other request with 200.
func lastSaveServer(t *testing.T, status int, lastSaveAt string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "last-save") {
			w.WriteHeader(status)
			if status == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]string{"last_save_at": lastSaveAt})
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// promptSubmitResult is the decoded prompt-submit hook output. The prompt-submit
// hook now emits nested hookSpecificOutput (model-visible) plus top-level
// systemMessage (user-visible), so we decode both plus the raw bytes.
type promptSubmitResult struct {
	SystemMessage      string `json:"systemMessage"`
	AdditionalContext  string `json:"additionalContext"`
	HookSpecificOutput *struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
	Raw string `json:"-"`
}

func runPromptSubmitResp(t *testing.T, sessionID, baseURL string) promptSubmitResult {
	t.Helper()
	var out bytes.Buffer
	payload := `{"session_id":"` + sessionID + `","prompt":"hi"}`
	RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, baseURL)
	var resp promptSubmitResult
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v (%q)", err, out.String())
	}
	resp.Raw = out.String()
	return resp
}

func TestRunPromptSubmit_Reminder_FiresWhenFoundStale(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-found-stale"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	srv := lastSaveServer(t, http.StatusOK, time.Now().Add(-20*time.Minute).UTC().Format(time.RFC3339))

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	assertReminderFired(t, resp)
}

// assertReminderFired verifies the reminder reached BOTH the model (nested
// hookSpecificOutput.additionalContext with hookEventName "UserPromptSubmit")
// and the user (top-level systemMessage).
func assertReminderFired(t *testing.T, resp promptSubmitResult) {
	t.Helper()
	if resp.SystemMessage != MemoryReminderSystemMessage {
		t.Errorf("systemMessage: got %q, want reminder message", resp.SystemMessage)
	}
	if resp.HookSpecificOutput == nil {
		t.Fatalf("hookSpecificOutput missing; raw: %q", resp.Raw)
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName: got %q, want UserPromptSubmit", resp.HookSpecificOutput.HookEventName)
	}
	if resp.HookSpecificOutput.AdditionalContext != MemoryReminderSystemMessage {
		t.Errorf("nested additionalContext: got %q, want reminder message", resp.HookSpecificOutput.AdditionalContext)
	}
}

func TestRunPromptSubmit_Reminder_FiresWhenEmpty(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-empty"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	srv := lastSaveServer(t, http.StatusOK, "") // reachable, never saved

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	assertReminderFired(t, resp)
}

func TestRunPromptSubmit_Reminder_NoFireWhenSessionYoung(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-young"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-2*time.Minute))
	srv := lastSaveServer(t, http.StatusOK, "")

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	if resp.Raw != "{}" {
		t.Errorf("expected empty {} for young session, got: %q", resp.Raw)
	}
}

func TestRunPromptSubmit_Reminder_NoFireWhenRecentSave(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-recent-save"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	srv := lastSaveServer(t, http.StatusOK, time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339))

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	if resp.Raw != "{}" {
		t.Errorf("expected empty {} for recent save, got: %q", resp.Raw)
	}
}

func TestRunPromptSubmit_Reminder_NoFireWhenCooldownActive(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-cooldown"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	// A reminder fired recently — within the cooldown window.
	p := markerPath(sessionID, markerMemoryReminder)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(time.Now().Add(-1*time.Minute).UTC().Format(time.RFC3339)+"\n"), 0o644)
	srv := lastSaveServer(t, http.StatusOK, "")

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	if resp.Raw != "{}" {
		t.Errorf("expected empty {} during cooldown, got: %q", resp.Raw)
	}
}

func TestRunPromptSubmit_Reminder_NoFireWhenDaemonUnreachable(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-daemon-down"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))

	resp := runPromptSubmitResp(t, sessionID, "http://127.0.0.1:19876")
	if resp.Raw != "{}" {
		t.Errorf("expected empty {} when daemon unreachable (fail-safe), got: %q", resp.Raw)
	}
}

func TestRunPromptSubmit_Reminder_NoFireWhenDaemon500(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-daemon-500"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	srv := lastSaveServer(t, http.StatusInternalServerError, "")

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	if resp.Raw != "{}" {
		t.Errorf("expected empty {} on non-200 (fail-safe), got: %q", resp.Raw)
	}
}

func TestRunPromptSubmit_Reminder_NoFireWhenFirstMarkerCorrupt(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-corrupt-marker"
	// Corrupt first-prompt marker → session age unknown → fail-safe.
	p := markerPath(sessionID, markerFirstPrompt)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte("garbage\n"), 0o644)
	srv := lastSaveServer(t, http.StatusOK, "")

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	if resp.Raw != "{}" {
		t.Errorf("expected empty {} for corrupt first-prompt marker, got: %q", resp.Raw)
	}
}

func TestRunPromptSubmit_Reminder_RewritesCooldownMarkerOnFire(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-rewrites"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	srv := lastSaveServer(t, http.StatusOK, "")

	resp := runPromptSubmitResp(t, sessionID, srv.URL)
	assertReminderFired(t, resp)
	// Cooldown marker must now exist with a fresh timestamp.
	ts, err := ReadMarkerTime(sessionID, markerMemoryReminder)
	if err != nil {
		t.Fatalf("cooldown marker should exist after firing: %v", err)
	}
	if time.Since(ts) > time.Minute {
		t.Errorf("cooldown marker should be freshly written, got %v", ts)
	}
}

func TestRunPromptSubmit_Reminder_ConcurrentInvocationsValidJSON(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sessionID := "reminder-concurrent"
	seedFirstPromptMarker(t, sessionID, time.Now().Add(-30*time.Minute))
	srv := lastSaveServer(t, http.StatusOK, "")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out bytes.Buffer
			payload := `{"session_id":"` + sessionID + `","prompt":"hi"}`
			RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, srv.URL)
			var resp promptSubmitResult
			if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
				t.Errorf("invalid JSON from concurrent invocation: %v (%q)", err, out.String())
			}
		}()
	}
	wg.Wait()
}

// --- RunSessionStart ---

func TestRunSessionStart_MigrationBlocked_SurfacesRecovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/governance/project-identity/status" {
			_ = json.NewEncoder(w).Encode(MigrationStatus{State: "migration-blocked", Reason: "canonical conflict", BackupID: "backup-42"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"blocked"}`), &out, srv.URL)
	var response struct {
		AdditionalContext string `json:"additionalContext"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"migration-blocked", "canonical conflict", "backup-42", MigrationStatusCommand} {
		if !strings.Contains(response.AdditionalContext, want) {
			t.Fatalf("additionalContext = %q, missing %q", response.AdditionalContext, want)
		}
	}
	if got, want := strings.Fields(MigrationStatusCommand), []string{"hive", "project", "identity", "status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("continuation tokens = %q, want %q", got, want)
	}
}

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
	// The dedicated SessionStart marker should have been created; the first-prompt
	// marker must remain untouched (owned exclusively by RunPromptSubmit).
	if !sessionStartMarkerExists("start-test-session") {
		t.Error("session-start marker should exist after session-start")
	}
	if MarkerExists("start-test-session") {
		t.Error("session-start must NOT create the first-prompt marker")
	}
}

// TestRunSessionStart_OutputShapeUnchanged is a regression guard: SessionStart
// must keep emitting top-level additionalContext and MUST NOT adopt the nested
// hookSpecificOutput wrapper used by the prompt-submit hook.
func TestRunSessionStart_OutputShapeUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "start-shape-session")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"start-shape-session"}`), &out, srv.URL)

	var got map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := got["additionalContext"]; !ok {
		t.Error("SessionStart must emit top-level additionalContext")
	}
	if _, ok := got["hookSpecificOutput"]; ok {
		t.Error("SessionStart must NOT emit hookSpecificOutput wrapper")
	}
	if _, ok := got["systemMessage"]; ok {
		t.Error("SessionStart must NOT emit systemMessage")
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

// TestRunSessionStart_DerivationError_FailsSafe verifies that when the supplied
// directory cannot be derived (empty directory → hivederive.ErrEmptyDir, mapped
// to an empty canonical name), the SessionStart hook still emits valid JSON,
// injects the unpinned protocol text, and never crashes. The pin line is
// absent because there is no canonical name to pin.
func TestRunSessionStart_DerivationError_FailsSafe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "derive-error-start")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	// No directory/cwd in payload → DetectProject("") returns "" (ErrEmptyDir).
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"derive-error-start"}`), &out, srv.URL)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("derivation error must still yield valid JSON: %v (%q)", err, out.String())
	}
	if !strings.Contains(resp["additionalContext"], "Hive Memory Protocol") {
		t.Errorf("expected protocol text even on derivation error, got: %q", resp["additionalContext"])
	}
	if strings.Contains(resp["additionalContext"], "Active project:") {
		t.Errorf("no pin line expected when derivation fails, got: %q", resp["additionalContext"])
	}
}

// TestRunPromptSubmit_DerivationError_FailsSafe verifies the prompt-submit hook
// degrades safely when derivation fails: valid JSON, no crash, no non-zero exit.
func TestRunPromptSubmit_DerivationError_FailsSafe(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	// No directory → derivation error path; first prompt still returns a message.
	RunPromptSubmit(context.Background(), strings.NewReader(`{"session_id":"derive-error-prompt","prompt":"hi"}`), &out, srv.URL)

	var resp map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("derivation error must still yield valid JSON: %v (%q)", err, out.String())
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

	var resp promptSubmitResult
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// First prompt must reach BOTH the user (systemMessage) and the model
	// (nested hookSpecificOutput.additionalContext with hookEventName).
	if resp.SystemMessage != FirstPromptSystemMessage {
		t.Errorf("systemMessage: got %q, want FirstPromptSystemMessage", resp.SystemMessage)
	}
	if resp.HookSpecificOutput == nil {
		t.Fatalf("hookSpecificOutput missing; raw: %q", out.String())
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName: got %q, want UserPromptSubmit", resp.HookSpecificOutput.HookEventName)
	}
	if resp.HookSpecificOutput.AdditionalContext != FirstPromptSystemMessage {
		t.Errorf("nested additionalContext: got %q, want FirstPromptSystemMessage", resp.HookSpecificOutput.AdditionalContext)
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
	var resp promptSubmitResult
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.SystemMessage == "" {
		t.Error("should return systemMessage on first prompt even when daemon is down")
	}
	if resp.HookSpecificOutput == nil || resp.HookSpecificOutput.AdditionalContext == "" {
		t.Error("should return nested additionalContext on first prompt even when daemon is down")
	}
}

func TestRunPromptSubmit_PostsPromptWithExactContent(t *testing.T) {
	markerDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", markerDir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "prompt-fidelity-session")

	_ = DeleteMarker("prompt-fidelity-session")

	// Use a real temp directory (not a git repo) so DetectProject returns the basename.
	projectDir := t.TempDir()
	wantContent := "quoted \"value\" with backslash \\ and literal \\n plus real\nnewline"
	requestPayload, err := json.Marshal(map[string]string{
		"prompt":     wantContent,
		"session_id": "prompt-fidelity-session",
		"directory":  projectDir,
		"project":    "should-be-overridden-by-derivation",
	})
	if err != nil {
		t.Fatalf("marshal hook payload: %v", err)
	}

	calls := 0
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/prompts" {
			t.Errorf("path = %q, want /prompts", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunPromptSubmit(context.Background(), bytes.NewReader(requestPayload), &out, srv.URL)

	if calls != 1 {
		t.Fatalf("POST calls = %d, want 1", calls)
	}
	if got["content"] != wantContent {
		t.Fatalf("content mismatch\ngot:  %q\nwant: %q", got["content"], wantContent)
	}
	if got["session_id"] != "prompt-fidelity-session" {
		t.Errorf("session_id = %q, want prompt-fidelity-session", got["session_id"])
	}
	if got["directory"] != projectDir {
		t.Errorf("directory = %q, want %q", got["directory"], projectDir)
	}
	// Project is now derived from the directory (T-14) — basename of temp dir.
	wantProject := filepath.Base(projectDir)
	if got["project"] != wantProject {
		t.Errorf("project = %q, want %q (derived basename)", got["project"], wantProject)
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
