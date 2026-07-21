package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ─── T-13: RunSessionStart derives canonical locally + pins via additionalContext ──

// TestRunSessionStart_WithGitDirectory_DerivesCanonicalAndPins verifies that
// when the payload contains a directory pointing to a git repo, RunSessionStart:
//  1. POSTs /sessions with the derived canonical name (not empty)
//  2. Returns additionalContext that includes the canonical name pin line (T-13).
func TestRunSessionStart_WithGitDirectory_DerivesCanonicalAndPins(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "session-derive-test")
	initHookGitRepo(t, dir, "https://github.com/org/jarvis-ai-devs.git")

	var capturedProject string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sessions" {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				capturedProject = body["project"]
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := `{"session_id":"session-derive-test","directory":"` + jsonEscape(dir) + `"}`
	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(payload), &out, srv.URL)

	// Verify canonical name was derived and posted to /sessions.
	if capturedProject != "jarvis-ai-devs" {
		t.Errorf("POST /sessions project = %q, want %q", capturedProject, "jarvis-ai-devs")
	}

	// Verify additionalContext contains the canonical pin line.
	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %q", err, out.String())
	}
	ctx := resp["additionalContext"]
	wantLine := "Active project: jarvis-ai-devs — use this exact name as the project argument in all mem_* calls."
	if !strings.Contains(ctx, wantLine) {
		t.Errorf("additionalContext should contain canonical pin line %q\ngot: %q", wantLine, ctx)
	}
}

// TestRunSessionStart_NoDirectory_AlwaysOutputsProtocol verifies that when no
// directory is in the payload, RunSessionStart still returns valid additionalContext
// containing the base Hive Memory Protocol text (T-13 back-compat).
func TestRunSessionStart_NoDirectory_AlwaysOutputsProtocol(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "no-dir-session")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(`{"session_id":"no-dir-session"}`), &out, srv.URL)

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(resp["additionalContext"], "Hive Memory Protocol") {
		t.Errorf("additionalContext should contain base protocol; got: %q", resp["additionalContext"])
	}
}

// TestRunSessionStart_UnresolvableDirectory_PostsEmptyAndOmitsPin verifies that
// when the payload directory cannot be resolved (non-existent path), derivation
// yields "" — RunSessionStart POSTs project="" to /sessions and the protocol
// text omits the "Active project:" pin line (no basename guessing, no leaked
// ambient repo name).
func TestRunSessionStart_UnresolvableDirectory_PostsEmptyAndOmitsPin(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "session-unresolvable-test")

	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")

	var captured map[string]string
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sessions" {
			posted = true
			_ = json.NewDecoder(r.Body).Decode(&captured)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := `{"session_id":"session-unresolvable-test","directory":"` + jsonEscape(nonExistent) + `"}`
	var out bytes.Buffer
	RunSessionStart(context.Background(), strings.NewReader(payload), &out, srv.URL)

	if !posted {
		t.Fatal("expected a POST /sessions call")
	}
	if captured["project"] != "" {
		t.Errorf("POST /sessions project = %q, want empty string", captured["project"])
	}

	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %q", err, out.String())
	}
	if strings.Contains(resp["additionalContext"], "Active project:") {
		t.Errorf("additionalContext should omit the 'Active project:' pin for an unresolvable directory\ngot: %q", resp["additionalContext"])
	}
}

// ─── T-14: RunPromptSubmit derives canonical locally ─────────────────────────

// TestRunPromptSubmit_WithGitDirectory_PostsCanonicalProject verifies that
// RunPromptSubmit derives the canonical project name from the directory and
// posts it to /prompts (T-14).
func TestRunPromptSubmit_WithGitDirectory_PostsCanonicalProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "prompt-derive-session")
	_ = DeleteMarker("prompt-derive-session")
	initHookGitRepo(t, dir, "https://github.com/org/canonical-project.git")

	var capturedProject string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prompts" {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				capturedProject = body["project"]
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := `{"prompt":"hello","session_id":"prompt-derive-session","directory":"` + jsonEscape(dir) + `","project":""}`
	var out bytes.Buffer
	RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, srv.URL)

	if capturedProject != "canonical-project" {
		t.Errorf("POST /prompts project = %q, want %q", capturedProject, "canonical-project")
	}
}

// TestRunPromptSubmit_NonGitDirectory_UsesBasename verifies that when the
// directory is a real but non-git directory, the canonical project is the
// basename (T-14 derivation path with basename fallback).
func TestRunPromptSubmit_NonGitDirectory_UsesBasename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "prompt-nongit-session")
	_ = DeleteMarker("prompt-nongit-session")

	projectDir := t.TempDir() // real dir, not a git repo
	want := filepath.Base(projectDir)

	var capturedProject string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prompts" {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				capturedProject = body["project"]
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := `{"prompt":"hello","session_id":"prompt-nongit-session","directory":"` + jsonEscape(projectDir) + `","project":"should-be-overridden"}`
	var out bytes.Buffer
	RunPromptSubmit(context.Background(), strings.NewReader(payload), &out, srv.URL)

	if capturedProject != want {
		t.Errorf("POST /prompts project = %q, want basename %q", capturedProject, want)
	}
}

// ─── FIX 2: RunSubagentStop derives canonical project name ───────────────────

// TestRunSubagentStop_DerivesCanonicalProject verifies that RunSubagentStop
// computes canonical := project.DetectProject(directory) and POSTs the derived
// canonical name — NOT payload.Project — to /observations/passive (FIX 2).
func TestRunSubagentStop_DerivesCanonicalProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "subagent-canonical-session")
	initHookGitRepo(t, dir, "https://github.com/org/derived-subagent-repo.git")

	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/observations/passive" {
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	// payload.Project is set to a raw/wrong value; directory points to git repo.
	// After the fix, capturedBody["project"] must equal the derived canonical.
	payload := `{"session_id":"subagent-canonical-session","project":"raw-payload-project","directory":"` + jsonEscape(dir) + `","stdout":"output"}`
	var out bytes.Buffer
	RunSubagentStop(context.Background(), strings.NewReader(payload), &out, srv.URL)

	if capturedBody["project"] != "derived-subagent-repo" {
		t.Errorf("POST /observations/passive project = %q, want derived canonical %q", capturedBody["project"], "derived-subagent-repo")
	}
}

// ─── T-15: RunSubagentStop POSTs cwd as directory field ──────────────────────

// TestRunSubagentStop_PostsDirectoryField verifies that RunSubagentStop includes
// the directory field in the POST body (T-15).
func TestRunSubagentStop_PostsDirectoryField(t *testing.T) {
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "subagent-dir-session")

	var capturedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/observations/passive" {
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	var out bytes.Buffer
	payload := `{"session_id":"subagent-dir-session","cwd":"/work/myproject","stdout":"tool output"}`
	RunSubagentStop(context.Background(), strings.NewReader(payload), &out, srv.URL)

	if out.String() != "{}" {
		t.Errorf("subagent-stop should output {}, got: %q", out.String())
	}
	if capturedBody["directory"] != "/work/myproject" {
		t.Errorf("POST /observations/passive directory = %q, want %q", capturedBody["directory"], "/work/myproject")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// initHookGitRepo initialises a git repo in dir with the given remote URL.
func initHookGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	cfgEmail := exec.Command("git", "config", "user.email", "test@test.com")
	cfgEmail.Dir = dir
	_ = cfgEmail.Run()
	cfgName := exec.Command("git", "config", "user.name", "Test")
	cfgName.Dir = dir
	_ = cfgName.Run()
	run("remote", "add", "origin", remoteURL)

	f := filepath.Join(dir, "README.md")
	if err := os.WriteFile(f, []byte("test"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", ".")

	commitCmd := exec.Command("git", "commit", "-m", "init")
	commitCmd.Dir = dir
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// jsonEscape escapes a string for embedding in a JSON string literal.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	// b includes the surrounding quotes; strip them.
	return string(b[1 : len(b)-1])
}
