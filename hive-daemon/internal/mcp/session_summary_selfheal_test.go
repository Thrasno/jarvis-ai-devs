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

// PR2 — Session registration self-heal for mem_session_summary.
//
// These tests exercise the provenance-gated escape mirrored from
// memSaveHandler (tools.go:285-311): when mem_session_summary would fail with
// project_unknown but a usable directory was supplied, the handler derives the
// canonical project name from the real filesystem, and — because that name came
// from derivation (derived=true) and is not the reserved "default" sentinel —
// bypasses the project_unknown gate so the very session-summary row registers
// the project. See specs/session-registration-self-heal/spec.md.

// ─── 2.1: Self-Heal on project_unknown ──────────────────────────────────────

// TestMemSessionSummary_ProjectUnknown_WithGitDirectory_SelfHeals verifies that
// an empty project plus a derivable directory self-heals: the derived name is
// used and the summary is written even though the project was never registered
// (KnownProjects empty). Also proves the `directory` field is accepted.
func TestMemSessionSummary_ProjectUnknown_WithGitDirectory_SelfHeals(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/heal-repo.git")

	var savedProject string
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{}, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedProject = m.Project
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":   "## Goal\nself-heal session summary",
		"project":   "",
		"directory": dir,
	})

	if res.IsError {
		t.Fatalf("self-heal session summary should succeed, got error: %s", textContent(t, res))
	}
	if savedProject != "heal-repo" {
		t.Errorf("saved project = %q, want %q", savedProject, "heal-repo")
	}
}

// TestMemSessionSummary_ProjectUnknown_NoDirectory_StillFails verifies that
// without a directory there is no self-heal: an unknown project still fails
// with project_unknown and nothing is written.
func TestMemSessionSummary_ProjectUnknown_NoDirectory_StillFails(t *testing.T) {
	t.Parallel()

	var saveCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "some-project"}}, nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			saveCalled = true
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": "## Goal\nunknown project, no directory",
		"project": "ghost-project",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true: unknown project without directory must not self-heal")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
	}
	if saveCalled {
		t.Fatal("SaveMemory must not be called when self-heal is unavailable")
	}
}

// ─── 2.2: Idempotent Registration ───────────────────────────────────────────

// TestMemSessionSummary_SelfHeal_Idempotent_SecondCallAlreadyKnown verifies that
// a second self-heal for the same directory recognises the project as already
// registered (present in KnownProjects) and completes without a duplicate
// registration error, writing under the same derived name.
func TestMemSessionSummary_SelfHeal_Idempotent_SecondCallAlreadyKnown(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/idem-repo.git")

	// registered flips to true after the first write lands, simulating the
	// implicit KnownProjects UNION registration.
	registered := false
	var savedProjects []string
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			if registered {
				return []project.KnownProject{{Name: "idem-repo"}}, nil
			}
			return []project.KnownProject{}, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedProjects = append(savedProjects, m.Project)
			registered = true
			return int64(len(savedProjects)), nil
		},
	}
	session := connectTestServer(t, store)

	args := map[string]any{
		"content":   "## Goal\nidempotent self-heal",
		"project":   "",
		"directory": dir,
	}

	first := callTool(t, session, "mem_session_summary", args)
	if first.IsError {
		t.Fatalf("first self-heal should succeed, got error: %s", textContent(t, first))
	}
	second := callTool(t, session, "mem_session_summary", args)
	if second.IsError {
		t.Fatalf("second self-heal should succeed (already registered), got error: %s", textContent(t, second))
	}

	if len(savedProjects) != 2 {
		t.Fatalf("expected 2 saves, got %d: %v", len(savedProjects), savedProjects)
	}
	for i, p := range savedProjects {
		if p != "idem-repo" {
			t.Errorf("save %d project = %q, want %q", i, p, "idem-repo")
		}
	}
}

// ─── 2.3: Filesystem-Derived Name Wins on Conflict ──────────────────────────

// TestMemSessionSummary_DerivedName_WinsOverStaleCallerProject verifies that a
// derivable directory overrides a stale caller-supplied project name: the write
// is registered under the filesystem-derived name, not the caller's string.
func TestMemSessionSummary_DerivedName_WinsOverStaleCallerProject(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initGitRepoMCP(t, dir, "https://github.com/org/actual-repo.git")

	var savedProject string
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{}, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedProject = m.Project
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":   "## Goal\nderived name wins",
		"project":   "stale-name",
		"directory": dir,
	})

	if res.IsError {
		t.Fatalf("conflict self-heal should succeed under derived name, got error: %s", textContent(t, res))
	}
	if savedProject != "actual-repo" {
		t.Errorf("saved project = %q, want %q (filesystem-derived name must win)", savedProject, "actual-repo")
	}
}

// ─── 2.4: Never Register "default" ──────────────────────────────────────────

// TestMemSessionSummary_UnderivableDirectory_DoesNotSelfHeal verifies that a
// directory that cannot be resolved (hivederive.Derive returns a typed error)
// does not enable self-heal: the unknown caller project still fails with
// project_unknown and nothing is registered.
func TestMemSessionSummary_UnderivableDirectory_DoesNotSelfHeal(t *testing.T) {
	t.Parallel()

	var saveCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "some-project"}}, nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			saveCalled = true
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":   "## Goal\nunderivable directory",
		"project":   "ghost-project",
		"directory": "/totally/nonexistent/path/that/does/not/exist/xyz789",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true: an underivable directory must not enable self-heal")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
	}
	if saveCalled {
		t.Fatal("SaveMemory must not be called for an underivable directory")
	}
}

// TestMemSessionSummary_DirectoryBasenameDefault_Refused documents a deliberate
// decision: even though hivederive.Derive returns the real basename "default"
// (not a typed error) for a directory literally named "default", the self-heal
// escape refuses it. "default" is a reserved pooling sentinel in the daemon;
// registering it as a real project would pool unrelated memories. This mirrors
// memSaveHandler's `project != "default"` guard exactly (spec: Never Register
// "default", mem_save Escape Behavior Unchanged). The refusal surfaces as
// project_unknown and nothing is written.
func TestMemSessionSummary_DirectoryBasenameDefault_Refused(t *testing.T) {
	t.Parallel()

	// A non-git directory whose basename is "default": Derive falls through to
	// basename("default") with no error.
	dir := filepath.Join(t.TempDir(), "default")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var saveCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "some-project"}}, nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			saveCalled = true
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":   "## Goal\ndefault basename",
		"project":   "",
		"directory": dir,
	})

	if !res.IsError {
		t.Fatal("expected IsError=true: derived name 'default' must never register")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
	}
	if saveCalled {
		t.Fatal("SaveMemory must not be called when derived name is 'default'")
	}
}

// ─── 2.5: mem_save Escape Behavior Unchanged (parity guard) ─────────────────

// TestMemSave_ProvenanceEscape_Parity_Unchanged is a PR2 guard asserting the
// mem_save provenance-gated escape (derived && project != "default") is
// unaffected by the session-summary self-heal work: a real derived name still
// bypasses project_unknown and writes, while the "default" sentinel from an
// underivable directory still fails project_unknown without writing.
func TestMemSave_ProvenanceEscape_Parity_Unchanged(t *testing.T) {
	t.Parallel()

	t.Run("real derived name still bypasses and writes", func(t *testing.T) {
		t.Parallel()
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git not available")
		}
		dir := t.TempDir()
		initGitRepoMCP(t, dir, "https://github.com/org/parity-repo.git")

		var savedProject string
		store := &mockStore{
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
			"title":     "parity",
			"content":   "parity content",
			"type":      "architecture",
			"project":   "",
			"directory": dir,
		})
		if res.IsError {
			t.Fatalf("mem_save escape should still succeed, got error: %s", textContent(t, res))
		}
		if savedProject != "parity-repo" {
			t.Errorf("saved project = %q, want %q", savedProject, "parity-repo")
		}
	})

	t.Run("default sentinel still rejected", func(t *testing.T) {
		t.Parallel()
		var saveCalled bool
		store := &mockStore{
			knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
				return []project.KnownProject{{Name: "some-project"}}, nil
			},
			saveMemoryFn: func(*models.Memory) (int64, error) {
				saveCalled = true
				return 1, nil
			},
		}
		session := connectTestServer(t, store)

		res := callTool(t, session, "mem_save", map[string]any{
			"title":     "parity default",
			"content":   "should not persist",
			"type":      "architecture",
			"project":   "",
			"directory": "/totally/nonexistent/path/parity/abc123",
		})
		if !res.IsError {
			t.Fatal("expected IsError=true: 'default' sentinel must not bypass project_unknown")
		}
		body := decodeJSONResponse(t, res)
		if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
			t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
		}
		if saveCalled {
			t.Fatal("SaveMemory must not be called for the 'default' sentinel")
		}
	})
}
