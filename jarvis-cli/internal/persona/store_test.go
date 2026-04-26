package persona

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubTempFile struct {
	path     string
	writeErr error
	syncErr  error
	closeErr error
}

func (f *stubTempFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *stubTempFile) Sync() error {
	return f.syncErr
}

func (f *stubTempFile) Close() error {
	return f.closeErr
}

func (f *stubTempFile) Name() string {
	return f.path
}

func TestUserPresetStore_SaveAndLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const slug = "Custom Mentor"
	const content = "name: custom-mentor\nnotes: |\n  ## Core Principle\n"

	savedPath, err := SaveUserPresetFile(slug, []byte(content))
	if err != nil {
		t.Fatalf("SaveUserPresetFile(%q): %v", slug, err)
	}

	wantPath := filepath.Join(home, ".jarvis", "personas", "custom-mentor.yaml")
	if savedPath != wantPath {
		t.Fatalf("saved path = %q, want %q", savedPath, wantPath)
	}

	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("stat saved file %q: %v", savedPath, err)
	}

	loadedPath, loadedContent, err := LoadUserPresetFile("custom mentor")
	if err != nil {
		t.Fatalf("LoadUserPresetFile(%q): %v", slug, err)
	}

	if loadedPath != wantPath {
		t.Fatalf("loaded path = %q, want %q", loadedPath, wantPath)
	}
	if string(loadedContent) != content {
		t.Fatalf("loaded content = %q, want %q", string(loadedContent), content)
	}
}

func TestUserPresetDir_UsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := UserPresetDir()
	if err != nil {
		t.Fatalf("UserPresetDir(): %v", err)
	}

	want := filepath.Join(home, ".jarvis", "personas")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestUserPresetStore_RejectsPathTraversal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		slug string
	}{
		{name: "parent traversal", slug: "../escape"},
		{name: "absolute path", slug: "/tmp/hack"},
		{name: "nested path", slug: "team/preset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := SaveUserPresetFile(tt.slug, []byte("name: x\n")); err == nil {
				t.Fatalf("SaveUserPresetFile(%q) expected error, got nil", tt.slug)
			}

			if _, _, err := LoadUserPresetFile(tt.slug); err == nil {
				t.Fatalf("LoadUserPresetFile(%q) expected error, got nil", tt.slug)
			}
		})
	}
}

func TestValidatePresetSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr string
	}{
		{name: "empty", slug: "", wantErr: "cannot be empty"},
		{name: "contains slash", slug: "team/preset", wantErr: "path separators"},
		{name: "contains backslash", slug: `team\preset`, wantErr: "path separators"},
		{name: "contains traversal", slug: "..", wantErr: "path traversal"},
		{name: "uppercase is rejected", slug: "Tony", wantErr: "only lowercase"},
		{name: "underscore is rejected", slug: "tony_stark", wantErr: "only lowercase"},
		{name: "valid slug", slug: "tony-stark"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePresetSlug(tt.slug)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validatePresetSlug(%q) unexpected error: %v", tt.slug, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validatePresetSlug(%q) expected error containing %q", tt.slug, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validatePresetSlug(%q) error = %q, want contains %q", tt.slug, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadUserPresetFile_MissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := LoadUserPresetFile("missing")
	if err == nil {
		t.Fatal("expected error for missing user preset file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestUserPresetPath_NormalizesSlugAndKeepsCanonicalDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := userPresetPath(" Custom Mentor ")
	if err != nil {
		t.Fatalf("userPresetPath: %v", err)
	}

	want := filepath.Join(home, ".jarvis", "personas", "custom-mentor.yaml")
	if path != want {
		t.Fatalf("userPresetPath = %q, want %q", path, want)
	}
}

func TestSaveUserPresetFile_FailsWhenJarvisPathIsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	jarvisPath := filepath.Join(home, ".jarvis")
	if err := os.WriteFile(jarvisPath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("seed .jarvis as file: %v", err)
	}

	_, err := SaveUserPresetFile("mentor", []byte("name: mentor\n"))
	if err == nil {
		t.Fatal("expected mkdir failure when ~/.jarvis is a file")
	}
	if !strings.Contains(err.Error(), "create user preset dir") {
		t.Fatalf("error = %q, want create user preset dir context", err.Error())
	}
}

func TestSaveUserPresetFile_FailsWhenCreateTempCannotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".jarvis", "personas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir personas dir: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod personas read-only: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	_, err := SaveUserPresetFile("mentor", []byte("name: mentor\n"))
	if err == nil {
		t.Fatal("expected create temp file failure")
	}
	if !strings.Contains(err.Error(), "create temp preset file") {
		t.Fatalf("error = %q, want create temp preset file context", err.Error())
	}
}

func TestSaveUserPresetFile_FailsWhenDestinationIsDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	targetPath := filepath.Join(home, ".jarvis", "personas", "mentor.yaml")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("mkdir destination directory: %v", err)
	}

	_, err := SaveUserPresetFile("mentor", []byte("name: mentor\n"))
	if err == nil {
		t.Fatal("expected rename failure when destination is a directory")
	}
	if !strings.Contains(err.Error(), "rename temp preset file") {
		t.Fatalf("error = %q, want rename temp preset file context", err.Error())
	}
}

func TestSaveUserPresetFile_FailsWhenTempWriteFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := newTempPresetFile
	t.Cleanup(func() { newTempPresetFile = original })
	newTempPresetFile = func(_, _ string) (tempPresetFile, error) {
		return &stubTempFile{path: filepath.Join(home, ".jarvis", "personas", "preset-write.tmp"), writeErr: errors.New("boom write")}, nil
	}

	_, err := SaveUserPresetFile("mentor", []byte("name: mentor\n"))
	if err == nil {
		t.Fatal("expected write temp preset file error")
	}
	if !strings.Contains(err.Error(), "write temp preset file") {
		t.Fatalf("error = %q, want write temp preset file context", err.Error())
	}
}

func TestSaveUserPresetFile_FailsWhenTempSyncFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := newTempPresetFile
	t.Cleanup(func() { newTempPresetFile = original })
	newTempPresetFile = func(_, _ string) (tempPresetFile, error) {
		return &stubTempFile{path: filepath.Join(home, ".jarvis", "personas", "preset-sync.tmp"), syncErr: errors.New("boom sync")}, nil
	}

	_, err := SaveUserPresetFile("mentor", []byte("name: mentor\n"))
	if err == nil {
		t.Fatal("expected sync temp preset file error")
	}
	if !strings.Contains(err.Error(), "sync temp preset file") {
		t.Fatalf("error = %q, want sync temp preset file context", err.Error())
	}
}

func TestSaveUserPresetFile_FailsWhenTempCloseFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := newTempPresetFile
	t.Cleanup(func() { newTempPresetFile = original })
	newTempPresetFile = func(_, _ string) (tempPresetFile, error) {
		return &stubTempFile{path: filepath.Join(home, ".jarvis", "personas", "preset-close.tmp"), closeErr: errors.New("boom close")}, nil
	}

	_, err := SaveUserPresetFile("mentor", []byte("name: mentor\n"))
	if err == nil {
		t.Fatal("expected close temp preset file error")
	}
	if !strings.Contains(err.Error(), "close temp preset file") {
		t.Fatalf("error = %q, want close temp preset file context", err.Error())
	}
}
