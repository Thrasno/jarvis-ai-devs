package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// TestInstallAgentsFromFS_WritesAllFiles verifies that installAgentsFromFS
// writes every file from the provided FS into the destination directory.
func TestInstallAgentsFromFS_WritesAllFiles(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"review-risk.md":        {Data: []byte("# Review Risk")},
		"review-readability.md": {Data: []byte("# Review Readability")},
		"jd-judge-a.md":         {Data: []byte("# JD Judge A")},
	}

	if err := installAgentsFromFS(dest, testFS); err != nil {
		t.Fatalf("installAgentsFromFS: %v", err)
	}

	wantFiles := map[string]string{
		"review-risk.md":        "# Review Risk",
		"review-readability.md": "# Review Readability",
		"jd-judge-a.md":         "# JD Judge A",
	}

	for relPath, wantContent := range wantFiles {
		got, err := os.ReadFile(filepath.Join(dest, relPath))
		if err != nil {
			t.Errorf("file %s not written: %v", relPath, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("file %s content = %q, want %q", relPath, got, wantContent)
		}
	}
}

// TestInstallAgentsFromFS_NilFS verifies that installAgentsFromFS returns an
// error (not a panic) when agentsFS is nil.
func TestInstallAgentsFromFS_NilFS(t *testing.T) {
	dest := t.TempDir()

	err := installAgentsFromFS(dest, nil)
	if err == nil {
		t.Fatal("expected error when agentsFS is nil, got nil")
	}
	if !strings.Contains(err.Error(), "agentsFS is nil") {
		t.Fatalf("expected error to mention agentsFS is nil, got: %v", err)
	}
}

// TestInstallAgentsFromFS_SkipsSubdirectories verifies that installAgentsFromFS
// does not write files inside subdirectories to destDir (flat walker contract).
func TestInstallAgentsFromFS_SkipsSubdirectories(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"top-level.md":        {Data: []byte("# Top Level")},
		"subdir/nested.md":    {Data: []byte("# Nested")},
		"subdir/deep/more.md": {Data: []byte("# Deep")},
	}

	if err := installAgentsFromFS(dest, testFS); err != nil {
		t.Fatalf("installAgentsFromFS: %v", err)
	}

	// Top-level file must be written.
	got, err := os.ReadFile(filepath.Join(dest, "top-level.md"))
	if err != nil {
		t.Fatalf("top-level.md not written: %v", err)
	}
	if string(got) != "# Top Level" {
		t.Errorf("top-level.md content = %q, want %q", got, "# Top Level")
	}

	// Files inside subdirectories must not be written.
	if _, err := os.Stat(filepath.Join(dest, "subdir", "nested.md")); !os.IsNotExist(err) {
		t.Errorf("expected subdir/nested.md to be absent, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "subdir", "deep", "more.md")); !os.IsNotExist(err) {
		t.Errorf("expected subdir/deep/more.md to be absent, got err=%v", err)
	}
}

// TestInstallAgentsFromFS_Idempotent verifies that running installAgentsFromFS
// twice produces no error and the same file content (no duplication or append).
func TestInstallAgentsFromFS_Idempotent(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"review-risk.md": {Data: []byte("# Review Risk")},
		"jd-judge-a.md":  {Data: []byte("# JD Judge A")},
	}

	// First call.
	if err := installAgentsFromFS(dest, testFS); err != nil {
		t.Fatalf("first installAgentsFromFS: %v", err)
	}

	// Second call (idempotency check).
	if err := installAgentsFromFS(dest, testFS); err != nil {
		t.Fatalf("second installAgentsFromFS: %v", err)
	}

	// Content must be exactly what was written, not appended.
	got, err := os.ReadFile(filepath.Join(dest, "review-risk.md"))
	if err != nil {
		t.Fatalf("read file after second call: %v", err)
	}
	if string(got) != "# Review Risk" {
		t.Errorf("content after second call = %q, want %q", got, "# Review Risk")
	}
}
