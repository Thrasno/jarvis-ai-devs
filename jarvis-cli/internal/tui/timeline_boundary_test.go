package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalTUIDoesNotImportHiveUI(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob Go files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected internal/tui Go files to scan")
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(content), "jarvis-cli/internal/hiveui") {
			t.Fatalf("%s imports internal/hiveui; timeline launch ownership belongs in cmd/jarvis", file)
		}
	}
}
