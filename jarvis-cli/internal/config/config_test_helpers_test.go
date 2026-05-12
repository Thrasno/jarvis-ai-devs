package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testModuleRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve jarvis-cli module root: runtime.Caller(0) failed")
	}

	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if hasConfigTestLayout(dir) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("resolve jarvis-cli module root from %q: expected go.mod, embed/orchestrator/sdd-orchestrator.md, and internal/config/layer1.md in an ancestor", file)
		}
	}
}

func hasConfigTestLayout(dir string) bool {
	for _, rel := range []string{
		"go.mod",
		filepath.Join("embed", "orchestrator", "sdd-orchestrator.md"),
		filepath.Join("internal", "config", "layer1.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			return false
		}
	}
	return true
}

func readConfigTestFile(t *testing.T, rel string) string {
	t.Helper()

	path := filepath.Join(testModuleRoot(t), rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config contract fixture %q: %v", path, err)
	}
	return string(b)
}
