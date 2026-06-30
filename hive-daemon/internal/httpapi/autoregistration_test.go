package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── T-03: Schema addition: Directory field in handleObservationsPassive ─────

// TestPostObservationsPassive_DirectoryFieldAccepted verifies that the
// directory field is accepted in the request body without error (T-03).
func TestPostObservationsPassive_DirectoryFieldAccepted(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	body := `{"content":"agent output","directory":"/some/dir","project":""}`
	rr := postJSON(srv, "/observations/passive", body)

	// Must be accepted (202) — the field must not cause a parse error.
	assert.Equal(t, http.StatusAccepted, rr.Code)
}

// ─── T-06: Chokepoint wiring: handleSessionsCreate ───────────────────────────

// TestPostSessions_EmptyProjectWithDirectory_DerivesProject verifies that when
// project is empty but directory is provided, the derived name is used in
// CreateSession (T-06).
func TestPostSessions_EmptyProjectWithDirectory_DerivesProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoHTTP(t, dir, "https://github.com/org/derived-proj.git")

	var capturedProject string
	store := &mockSessionStore{
		createSessionFn: func(id, proj, directory, devID, client string) error {
			capturedProject = proj
			return nil
		},
	}
	srv := newServerWithSessions(store)

	reqBody := mustMarshal(t, map[string]string{
		"id":        "sess-derive",
		"project":   "",
		"directory": dir,
		"dev_id":    "",
		"client":    "hook",
	})
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "derived-proj", capturedProject,
		"CreateSession should be called with derived canonical name, not empty string")
}

// TestPostSessions_EmptyProjectWithDirectory_KnownProjectsIncludesDerived verifies
// that after a session create with directory derivation and a real DB,
// KnownProjects includes the derived name (T-10 contribution).
func TestPostSessions_EmptyProjectWithDirectory_KnownProjectsIncludesDerived(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoHTTP(t, dir, "https://github.com/org/new-proj.git")

	dbPath := filepath.Join(t.TempDir(), "test.db")
	realDB, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	srv := httpapi.NewServerWithSessions("127.0.0.1:0", &mockPromptStore{}, realDB)

	reqBody := mustMarshal(t, map[string]string{
		"id":        "sess-register",
		"project":   "",
		"directory": dir,
		"dev_id":    "",
		"client":    "hook",
	})
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// Verify that KnownProjects now includes the derived project.
	known, err := realDB.KnownProjects(context.Background())
	require.NoError(t, err)
	found := false
	for _, kp := range known {
		if kp.Name == "new-proj" {
			found = true
			break
		}
	}
	assert.True(t, found, "KnownProjects should include 'new-proj' after session create with directory derivation")
}

// ─── T-07: Chokepoint wiring: handlePrompts ──────────────────────────────────

// TestPostPrompts_EmptyProjectWithDirectory_DerivesProject verifies that when
// project is empty but directory is provided, the derived canonical name is
// used and returned in the response (T-07).
func TestPostPrompts_EmptyProjectWithDirectory_DerivesProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoHTTP(t, dir, "https://github.com/org/derived-prompt-proj.git")

	dbPath := filepath.Join(t.TempDir(), "test.db")
	realDB, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	// Register the project first via session so the validator allows it.
	require.NoError(t, realDB.CreateSession("pre-sess", "derived-prompt-proj", dir, "", "hook"))

	srv := httpapi.NewServerWithAll("127.0.0.1:0", realDB, realDB, nil, nil, nil, realDB)

	reqBody := mustMarshal(t, map[string]string{
		"content":   "hello world",
		"project":   "",
		"directory": dir,
	})
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "derived-prompt-proj", resp["project"],
		"response should carry derived canonical name")
}

// ─── T-08: Chokepoint wiring: handleObservationsPassive derivation ────────────

// TestPostObservationsPassive_EmptyProjectWithDirectory_DerivesProject verifies
// that when project is empty but directory is provided, the derived name is
// forwarded to SavePassiveObservation (T-08).
func TestPostObservationsPassive_EmptyProjectWithDirectory_DerivesProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoHTTP(t, dir, "https://github.com/org/passive-derived.git")

	var capturedProject string
	store := &mockSessionStore{
		savePassiveObservationFn: func(_ context.Context, _, proj, _, _ string) error {
			capturedProject = proj
			return nil
		},
	}
	srv := newServerWithSessions(store)

	reqBody := mustMarshal(t, map[string]string{
		"content":   "subagent output",
		"project":   "",
		"directory": dir,
	})
	req := httptest.NewRequest(http.MethodPost, "/observations/passive", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	assert.Equal(t, "passive-derived", capturedProject,
		"SavePassiveObservation should be called with derived canonical name")
}

// ─── T-11: Normalization consistency ─────────────────────────────────────────

// TestNormalizationParity verifies that case/separator variants of a derived
// project name normalize to the same key as the stored name (Decision-5 contract).
func TestNormalizationParity(t *testing.T) {
	t.Parallel()

	// Project store has "Jarvis-Dev" (mixed-case, as-derived).
	// Writing with "jarvis-dev" must match via normalizeName.
	projectStore := mockProjectStore{
		known: []project.KnownProject{{Name: "Jarvis-Dev"}},
	}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", &mockPromptStore{}, projectStore)

	reqBody := mustMarshal(t, map[string]string{
		"content":   "hello",
		"project":   "jarvis-dev",
		"directory": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// The validator normalizes both sides, so "jarvis-dev" matches "Jarvis-Dev".
	assert.Equal(t, http.StatusCreated, rr.Code,
		"'jarvis-dev' should match 'Jarvis-Dev' via normalization; body: %s", rr.Body.String())
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// initGitRepoHTTP initialises a git repo in dir with the given remote URL.
// Mirrors initGitRepo from the project package tests for use in httpapi_test.
func initGitRepoHTTP(t *testing.T, dir, remoteURL string) {
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
	require.NoError(t, os.WriteFile(f, []byte("test"), 0o644))
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
