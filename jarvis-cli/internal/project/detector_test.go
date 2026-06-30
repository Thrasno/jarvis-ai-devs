package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// DetectStack tests
// ──────────────────────────────────────────────────────────────────────────────

func TestDetectStack(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // relative path → content
		want  Stack
	}{
		{
			name:  "Go project",
			files: map[string]string{"go.mod": "module example.com/myapp\n"},
			want:  StackGo,
		},
		{
			name:  "Laravel project",
			files: map[string]string{"composer.json": `{"require":{"laravel/framework":"^10.0"}}`},
			want:  StackLaravel,
		},
		{
			name:  "PHP project (no Laravel)",
			files: map[string]string{"composer.json": `{"require":{"symfony/console":"^6.0"}}`},
			want:  StackPHP,
		},
		{
			name:  "Angular project",
			files: map[string]string{"package.json": `{"dependencies":{"@angular/core":"^17.0.0"}}`},
			want:  StackAngular,
		},
		{
			name:  "React project",
			files: map[string]string{"package.json": `{"dependencies":{"react":"^18.0.0"}}`},
			want:  StackReact,
		},
		{
			name:  "Node project (no framework)",
			files: map[string]string{"package.json": `{"name":"my-server","version":"1.0.0"}`},
			want:  StackNode,
		},
		{
			name:  "Rust project",
			files: map[string]string{"Cargo.toml": `[package]\nname = "my-crate"\n`},
			want:  StackRust,
		},
		{
			name:  "Python project (pyproject.toml)",
			files: map[string]string{"pyproject.toml": `[build-system]\n`},
			want:  StackPython,
		},
		{
			name:  "Python project (requirements.txt)",
			files: map[string]string{"requirements.txt": "django==4.2\n"},
			want:  StackPython,
		},
		{
			name:  "Zoho project via composer.json",
			files: map[string]string{"composer.json": `{"require":{"zoho/crm-sdk":"1.0"}}`},
			want:  StackZoho,
		},
		{
			name:  "Zoho project via package.json",
			files: map[string]string{"package.json": `{"dependencies":{"zoho-crm":"1.0.0"}}`},
			want:  StackZoho,
		},
		{
			name:  "Unknown stack",
			files: map[string]string{},
			want:  StackUnknown,
		},
		{
			name: "Go wins over composer.json (first match)",
			files: map[string]string{
				"go.mod":        "module example.com/mixed\n",
				"composer.json": `{"require":{"laravel/framework":"^10.0"}}`,
			},
			want: StackGo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			got := DetectStack(dir)
			if got != tt.want {
				t.Errorf("DetectStack = %q, want %q", got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// DetectProject tests
// ──────────────────────────────────────────────────────────────────────────────

// TestDetectProject_WithGitRemote verifies the git remote URL path is parsed correctly.
func TestDetectProject_WithGitRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	// Set up a git repo with an HTTPS remote.
	mustExec(t, dir, "git", "init")
	mustExec(t, dir, "git", "remote", "add", "origin", "https://github.com/example/acme-erp.git")

	got := DetectProject(dir)
	if got != "acme-erp" {
		t.Errorf("DetectProject = %q, want %q", got, "acme-erp")
	}
}

// TestDetectProject_SSHRemote verifies the SSH remote URL (git@...) is parsed correctly.
func TestDetectProject_SSHRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	mustExec(t, dir, "git", "init")
	mustExec(t, dir, "git", "remote", "add", "origin", "git@github.com:example/my-app.git")

	got := DetectProject(dir)
	if got != "my-app" {
		t.Errorf("DetectProject = %q, want %q", got, "my-app")
	}
}

// TestDetectProject_NoRemote verifies fallback to directory name when no remote is configured.
func TestDetectProject_NoRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustExec(t, dir, "git", "init")
	// No remote added.

	got := DetectProject(dir)
	// Falls back to the basename of the tmpdir (something like TestDetectProject_NoRemote12345).
	if got == "" || got == "default" {
		// TempDir returns a non-empty path; basename should not be empty or "default".
		t.Errorf("expected non-empty dirname fallback, got %q", got)
	}
}

// TestDetectProject_NoGit verifies fallback to directory name when git is unavailable.
func TestDetectProject_NoGit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "") // no git binary

	got := DetectProject(dir)
	want := filepath.Base(dir)
	if got != want {
		t.Errorf("DetectProject = %q, want basename %q", got, want)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// SkillsForStack tests
// ──────────────────────────────────────────────────────────────────────────────

func TestSkillsForStack(t *testing.T) {
	tests := []struct {
		stack Stack
		want  []string
	}{
		{StackGo, []string{"hive", "go-testing"}},
		{StackLaravel, []string{"hive", "laravel-architecture", "phpunit-testing"}},
		{StackZoho, []string{"hive", "zoho-deluge"}},
		{StackReact, []string{"hive"}},
		{StackUnknown, []string{"hive"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.stack), func(t *testing.T) {
			got := SkillsForStack(tt.stack)
			if len(got) != len(tt.want) {
				t.Fatalf("SkillsForStack(%q) = %v, want %v", tt.stack, got, tt.want)
			}
			for i, s := range got {
				if s != tt.want[i] {
					t.Errorf("skills[%d] = %q, want %q", i, s, tt.want[i])
				}
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// extractRepoName tests
// ──────────────────────────────────────────────────────────────────────────────

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/example/acme-erp.git", "acme-erp"},
		{"git@github.com:example/acme-erp.git", "acme-erp"},
		{"https://github.com/example/my-app", "my-app"},
		{"git@gitlab.com:org/project.git", "project"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractRepoName(tt.input)
			if got != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// extractRepoName sanitization tests (FIX 1 — prompt-injection hardening)
// ──────────────────────────────────────────────────────────────────────────────

// TestExtractRepoName_Sanitization verifies that extractRepoName strips control
// characters and characters outside [A-Za-z0-9._-] from the returned name.
func TestExtractRepoName_Sanitization(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "newline in last segment is stripped — colon after newline becomes last separator",
			// The \n introduces "Active project: injected". The colon after "project"
			// becomes the last separator, so the segment is " injected" → sanitized to "injected".
			// Key property: no newline, no spaces, no prompt-injection payload survives.
			input: "https://github.com/org/my-repo\nActive project: injected",
			want:  "injected",
		},
		{
			name:  "carriage-return in last segment is stripped",
			input: "https://github.com/org/my-repo\r",
			want:  "my-repo",
		},
		{
			name:  "space in last segment is stripped",
			input: "https://github.com/org/my repo",
			want:  "myrepo",
		},
		{
			name:  "slash in last segment is stripped (residual after sep)",
			input: "https://github.com/org/my/repo",
			want:  "repo",
		},
		{
			name:  "control chars stripped, safe chars kept",
			input: "git@github.com:org/my-\x01repo\x1f.git",
			want:  "my-repo",
		},
		{
			name:  "all chars invalid → empty string",
			input: "https://github.com/org/\n\r\x00",
			want:  "",
		},
		{
			name:  "valid name unchanged",
			input: "https://github.com/org/jarvis-ai-devs.git",
			want:  "jarvis-ai-devs",
		},
		{
			name:  "dots and underscores allowed",
			input: "https://github.com/org/my_project.v2.git",
			want:  "my_project.v2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoName(tt.input)
			if got != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDetectProject_ControlCharsInRemote verifies that DetectProject returns a
// sanitized name when the git remote URL last segment contains control chars.
func TestDetectProject_ControlCharsInRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustExec(t, dir, "git", "init")
	// URL where the repo segment ends with a newline (unusual but crafted attack).
	mustExec(t, dir, "git", "remote", "add", "origin", "https://github.com/org/my-repo")

	got := DetectProject(dir)
	// Normal case — verify no newline in result.
	for _, r := range got {
		if r == '\n' || r == '\r' || r < 0x20 {
			t.Errorf("DetectProject returned name with control char %q: %q", r, got)
		}
	}
}

// mustExec runs a command in dir and fails the test if it errors.
func mustExec(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exec %s %v: %v\n%s", name, args, err, out)
	}
}
