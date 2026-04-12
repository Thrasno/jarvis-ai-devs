package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteRegistry_FirstRun verifies that WriteRegistry creates .jarvis/skill-registry.md
// with both the Suggested Skills and Custom Skills sections on first run.
func TestWriteRegistry_FirstRun(t *testing.T) {
	dir := t.TempDir()
	skills := []string{"sdd-workflow", "hive", "go-testing"}

	if err := WriteRegistry(dir, "my-project", StackGo, skills); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"my-project",
		"Go",
		"## Suggested Skills",
		"sdd-workflow",
		"hive",
		"go-testing",
		"## Custom Skills",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in registry content:\n%s", want, content)
		}
	}
}

// TestWriteRegistry_Idempotent verifies that re-running WriteRegistry replaces
// the Suggested Skills section but leaves the Custom Skills section intact.
func TestWriteRegistry_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First run: write initial registry.
	if err := WriteRegistry(dir, "my-project", StackGo, []string{"sdd-workflow", "hive", "go-testing"}); err != nil {
		t.Fatalf("first WriteRegistry: %v", err)
	}

	// Manually add a custom skill to simulate user edits.
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	existing, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	withCustom := string(existing) + "- **my-custom-skill**\n"
	if err := os.WriteFile(registryPath, []byte(withCustom), 0644); err != nil {
		t.Fatal(err)
	}

	// Second run: different stack (Laravel), should update Suggested but keep custom.
	if err := WriteRegistry(dir, "my-project", StackLaravel, []string{"sdd-workflow", "hive", "laravel-architecture", "phpunit-testing"}); err != nil {
		t.Fatalf("second WriteRegistry: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	content := string(data)

	// Updated suggested skills present.
	if !strings.Contains(content, "laravel-architecture") {
		t.Error("expected updated 'laravel-architecture' in Suggested Skills")
	}
	// Old stack-specific skill removed from suggestions.
	suggestedIdx := strings.Index(content, "## Suggested Skills")
	customIdx := strings.Index(content, "## Custom Skills")
	if suggestedIdx < 0 || customIdx < 0 {
		t.Fatal("missing expected sections")
	}
	suggestedSection := content[suggestedIdx:customIdx]
	if strings.Contains(suggestedSection, "go-testing") {
		t.Error("go-testing should not appear in Suggested Skills after stack change to Laravel")
	}
	// Custom skill preserved.
	if !strings.Contains(content, "my-custom-skill") {
		t.Error("expected user-added 'my-custom-skill' to be preserved in Custom Skills")
	}
}

// TestWriteRegistry_CustomAbsent verifies that when ## Custom Skills section is absent
// in an existing file, WriteRegistry appends the default empty custom section.
func TestWriteRegistry_CustomAbsent(t *testing.T) {
	dir := t.TempDir()

	// Write a registry without the Custom Skills section.
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		t.Fatal(err)
	}
	noCustom := "# Skill Registry — old-project\n\n## Suggested Skills\n\n- **sdd-workflow**\n"
	if err := os.WriteFile(registryPath, []byte(noCustom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRegistry(dir, "old-project", StackNode, []string{"sdd-workflow", "hive"}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	content := string(data)

	if !strings.Contains(content, "## Custom Skills") {
		t.Error("expected ## Custom Skills section to be appended when absent")
	}
}
