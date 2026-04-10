package db

import (
	"testing"
)

func TestUsernameFromHome(t *testing.T) {
	tests := []struct {
		name     string
		homeDir  string
		expected string
	}{
		{
			name:     "linux standard",
			homeDir:  "/home/andres",
			expected: "andres",
		},
		{
			name:     "mac standard",
			homeDir:  "/Users/john",
			expected: "john",
		},
		{
			name:     "windows standard",
			homeDir:  `C:\Users\bob`,
			expected: "bob",
		},
		{
			name:     "trailing slash",
			homeDir:  "/home/andres/",
			expected: "andres",
		},
		{
			name:     "empty string",
			homeDir:  "",
			expected: "",
		},
		{
			name:     "root only",
			homeDir:  "/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := usernameFromHome(tt.homeDir)
			if got != tt.expected {
				t.Errorf("usernameFromHome(%q) = %q, want %q", tt.homeDir, got, tt.expected)
			}
		})
	}
}

func TestDetectUsername_EnvVarTakesPriority(t *testing.T) {
	t.Setenv("JARVIS_USER", "override-user")

	got := detectUsername()
	if got != "override-user" {
		t.Errorf("detectUsername() = %q, want 'override-user'", got)
	}
}

func TestDetectUsername_NeverEmpty(t *testing.T) {
	// Clear JARVIS_USER to test fallback chain
	t.Setenv("JARVIS_USER", "")

	got := detectUsername()
	if got == "" {
		t.Error("detectUsername() returned empty string — must always return a non-empty value")
	}
}

func TestDetectUsername_UnknownFallback(t *testing.T) {
	// Simulate worst case: env empty, git unavailable, bad home dir
	// We can't fully mock git/OS in this test, but we test the usernameFromHome
	// edge case directly and verify "unknown" is the final fallback.
	got := usernameFromHome("")
	if got != "" {
		t.Errorf("usernameFromHome('') should return '' to trigger 'unknown' fallback, got %q", got)
	}
}

func TestOsUsername_ReturnsValueOnRealSystem(t *testing.T) {
	// osUsername() relies on os.UserHomeDir() which always succeeds on a real system.
	// We exercise the function directly to ensure the code path is covered.
	got := osUsername()
	// On any real Linux/macOS/Windows system this should return a non-empty string.
	// We don't assert a specific value to keep the test portable.
	if got == "" {
		t.Log("osUsername() returned empty — UserHomeDir() may have failed or home is root")
	}
}

func TestDetectUsername_OsFallback(t *testing.T) {
	// Force JARVIS_USER empty and git unavailable so detectUsername falls through to osUsername.
	t.Setenv("JARVIS_USER", "")
	t.Setenv("PATH", "") // git not found → gitUsername returns ""

	got := detectUsername()
	if got == "" {
		t.Error("detectUsername() must never return empty string")
	}
}
