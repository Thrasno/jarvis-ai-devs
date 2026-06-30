package mcp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

// ─── T-04: mem_save schema — Directory field parsed ──────────────────────────

// TestMemSave_DirectoryFieldParsed verifies that the directory field is
// accepted by the MCP mem_save handler (T-04).
func TestMemSave_DirectoryFieldParsed(t *testing.T) {
	t.Parallel()

	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "proj"}}, nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	// directory field must be accepted — not cause a parse error.
	res := callTool(t, session, "mem_save", map[string]any{
		"title":     "test",
		"content":   "test content",
		"type":      "architecture",
		"project":   "proj",
		"directory": "/some/dir",
	})
	if res.IsError {
		t.Fatalf("mem_save with directory field should succeed, got error: %s", textContent(t, res))
	}
}

// ─── T-05: mem_save ordering fix — derive-then-validate ──────────────────────

// TestMemSave_UnknownProjectReturnsStructuredErrorWithoutGhostWrite_Regression
// verifies that the existing no-ghost-session guarantee is preserved (T-05a,
// tools_test.go:860 regression guard).
// This mirrors the existing test but with directory to ensure derivation does
// not create ghost sessions for a non-empty assistant-supplied unknown project.
func TestMemSave_AssistantSuppliedUnknownProject_WithDirectory_StillRejectsWithGuardrail(t *testing.T) {
	t.Parallel()

	var saveCalled bool
	var ensureCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "jarvis-dev"}}, nil
		},
		ensureManualSaveSessionFn: func(project string) (string, error) {
			ensureCalled = true
			return "manual-save-" + project, nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			saveCalled = true
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	// Non-empty assistant-supplied project + directory: guardrail must fire,
	// provenance escape is unreachable for assistant-supplied names.
	res := callTool(t, session, "mem_save", map[string]any{
		"title":     "Ghost",
		"content":   "should not persist",
		"type":      "architecture",
		"project":   "my-fake-project",
		"directory": "/some/real/looking/path",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true for assistant-supplied unknown project with directory")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
	}
	if ensureCalled {
		t.Fatal("EnsureManualSaveSession must not create a ghost session after validation failure")
	}
	if saveCalled {
		t.Fatal("SaveMemory must not be called after validation failure")
	}
}

// TestMemSave_EmptyProject_WithGitDirectory_DerivesAndSucceeds verifies the
// first-write-success case when project is empty and directory points to a
// git repo — the derived name bypasses project_unknown (T-05c).
func TestMemSave_EmptyProject_WithGitDirectory_DerivesAndSucceeds(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/new-repo.git")

	var savedProject string
	store := &mockStore{
		// KnownProjects returns empty — new repo never registered before.
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{}, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedProject = m.Project
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":     "First write",
		"content":   "content for new repo",
		"type":      "architecture",
		"project":   "",
		"directory": dir,
	})

	if res.IsError {
		t.Fatalf("first-write with derived project should succeed, got error: %s", textContent(t, res))
	}
	if savedProject != "new-repo" {
		t.Errorf("saved project = %q, want %q", savedProject, "new-repo")
	}
}

// TestMemSave_EmptyProject_WithGitDirectory_EnsureManualSaveSessionCalledAfterValidation
// verifies that EnsureManualSaveSession is called AFTER the derivation+validation
// block — not before — preserving the no-ghost-session guarantee (T-05d).
func TestMemSave_EmptyProject_WithGitDirectory_EnsureManualSaveSessionCalledAfterValidation(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/ordered-repo.git")

	var orderLog []string
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			orderLog = append(orderLog, "KnownProjects")
			return []project.KnownProject{}, nil
		},
		ensureManualSaveSessionFn: func(proj string) (string, error) {
			orderLog = append(orderLog, "EnsureManualSaveSession")
			return "manual-save-" + proj, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			orderLog = append(orderLog, "SaveMemory")
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":     "Order test",
		"content":   "content",
		"type":      "architecture",
		"project":   "",
		"directory": dir,
	})

	if res.IsError {
		t.Fatalf("first-write should succeed, got error: %s", textContent(t, res))
	}

	// Ensure KnownProjects (validation) comes before EnsureManualSaveSession.
	knownIdx, ensureIdx := -1, -1
	for i, step := range orderLog {
		switch step {
		case "KnownProjects":
			if knownIdx == -1 {
				knownIdx = i
			}
		case "EnsureManualSaveSession":
			ensureIdx = i
		}
	}
	if ensureIdx != -1 && knownIdx != -1 && ensureIdx < knownIdx {
		t.Errorf("ordering violation: EnsureManualSaveSession (step %d) called before KnownProjects (step %d)\norderLog: %v",
			ensureIdx, knownIdx, orderLog)
	}
}

// ─── T-09: mem_session_start MCP handler ─────────────────────────────────────

// TestMemSessionStart_EmptyProjectWithDirectory_DerivesProject verifies that
// when project is empty and directory is provided, the derived canonical name
// is used in CreateSession (T-09).
func TestMemSessionStart_EmptyProjectWithDirectory_DerivesProject(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/derived-mcp.git")

	var capturedProject string
	store := &mockStore{
		createSessionFn: func(id, proj, directory, devID, client string) error {
			capturedProject = proj
			return nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_start", map[string]any{
		"id":        "s1",
		"project":   "",
		"directory": dir,
		"dev_id":    "d",
		"client":    "c",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if capturedProject != "derived-mcp" {
		t.Errorf("CreateSession called with project=%q, want %q", capturedProject, "derived-mcp")
	}
}

// ─── T-10: KnownProjects integration tests ───────────────────────────────────

// TestMemSave_FirstWrite_DerivedName_RegistrationRoundtrip is a smoke test
// verifying the full stack: empty project + directory → derived name →
// SaveMemory called with derived name (no stubs on validation path) (T-10).
func TestMemSave_FirstWrite_DerivedName_RegistrationRoundtrip(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/roundtrip-repo.git")

	var savedMem *models.Memory
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{}, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedMem = m
			return 42, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":     "Roundtrip",
		"content":   "integration content",
		"type":      "decision",
		"project":   "",
		"directory": dir,
	})

	if res.IsError {
		t.Fatalf("roundtrip save should succeed, got error: %s", textContent(t, res))
	}
	if savedMem == nil {
		t.Fatal("SaveMemory was not called")
	}
	if savedMem.Project != "roundtrip-repo" {
		t.Errorf("saved project = %q, want %q", savedMem.Project, "roundtrip-repo")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// initGitRepoMCP initialises a git repo in dir with the given remote URL.
// Used in the mcp_test package for MCP handler tests.
func initGitRepoMCP(t *testing.T, dir, remoteURL string) {
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
