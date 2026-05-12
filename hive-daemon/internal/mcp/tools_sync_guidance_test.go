package mcp

import (
	"strings"
	"testing"
)

func TestSyncNotConfiguredMessageUnixIncludesSecurePermissionGuidance(t *testing.T) {
	t.Parallel()

	message := syncNotConfiguredMessage("linux")

	assertSyncGuidanceFragments(t, message)
	if !strings.Contains(message, "chmod 600") {
		t.Fatalf("Unix-like guidance must include chmod 600 hint; message=%q", message)
	}
}

func TestSyncNotConfiguredMessageWindowsAvoidsUnixOnlyPermissionCommand(t *testing.T) {
	t.Parallel()

	message := syncNotConfiguredMessage("windows")

	assertSyncGuidanceFragments(t, message)
	if strings.Contains(message, "chmod 600") {
		t.Fatalf("Windows guidance must not include chmod 600; message=%q", message)
	}
	if !strings.Contains(message, "Windows") {
		t.Fatalf("Windows guidance should include Windows-specific wording; message=%q", message)
	}
}

func assertSyncGuidanceFragments(t *testing.T, message string) {
	t.Helper()

	wantFragments := []string{
		"sync not configured",
		"HIVE_API_URL",
		"HIVE_API_EMAIL",
		"HIVE_API_PASSWORD",
		"~/.jarvis/sync.json",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(message, fragment) {
			t.Fatalf("message missing %q; message=%q", fragment, message)
		}
	}
}
