package project

import "testing"

func TestBlockedProjectUnsafeRootWarningWarnsWithoutRejecting(t *testing.T) {
	for _, root := range []string{"/tmp", "/home/test", "/home/test/Documents", "/home/test/Downloads", "/home/test/Desktop"} {
		t.Run(root, func(t *testing.T) {
			warning, ok := BlockedProjectUnsafeRootWarning(root, "/home/test")
			if !ok {
				t.Fatalf("expected warning for %s", root)
			}
			if warning == "" {
				t.Fatal("expected non-empty warning")
			}
		})
	}
}

func TestBlockedProjectUnsafeRootWarningAllowsNormalProjectRoot(t *testing.T) {
	warning, ok := BlockedProjectUnsafeRootWarning("/home/test/work/jarvis-dev", "/home/test")
	if ok {
		t.Fatalf("expected no warning, got %q", warning)
	}
}
