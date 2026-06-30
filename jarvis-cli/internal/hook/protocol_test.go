package hook

import (
	"strings"
	"testing"
)

// TestBuildHiveProtocolText_EmptyCanonical_ReturnsBaseline verifies that
// BuildHiveProtocolText("") returns the standard HiveProtocolText unchanged
// (back-compat: no canonical line injected) (T-12).
func TestBuildHiveProtocolText_EmptyCanonical_ReturnsBaseline(t *testing.T) {
	t.Parallel()

	got := BuildHiveProtocolText("")
	if got != HiveProtocolText {
		t.Errorf("BuildHiveProtocolText(\"\") should return HiveProtocolText unchanged\ngot:  %q\nwant: %q", got, HiveProtocolText)
	}
}

// TestBuildHiveProtocolText_WithCanonical_ContainsProtocolAndPinLine verifies
// that BuildHiveProtocolText("jarvis-ai-devs") returns content that includes
// both the base HiveProtocolText and the canonical pin line (T-12).
func TestBuildHiveProtocolText_WithCanonical_ContainsProtocolAndPinLine(t *testing.T) {
	t.Parallel()

	got := BuildHiveProtocolText("jarvis-ai-devs")

	if !strings.Contains(got, "Hive Memory Protocol") {
		t.Errorf("result should contain base protocol text; got: %q", got)
	}
	wantLine := "Active project: jarvis-ai-devs — use this exact name as the project argument in all mem_* calls."
	if !strings.Contains(got, wantLine) {
		t.Errorf("result should contain canonical pin line %q; got: %q", wantLine, got)
	}
}

// TestBuildHiveProtocolText_CanonicalIsExact verifies that the canonical name
// in the output is the EXACT string passed in (no normalization, no lowercasing)
// (T-12).
func TestBuildHiveProtocolText_CanonicalIsExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		canonical string
	}{
		{"lowercase with hyphens", "jarvis-ai-devs"},
		{"mixed case", "My-Project"},
		{"uppercase", "MYPROJECT"},
		{"with dots", "my.project.v2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := BuildHiveProtocolText(tt.canonical)
			if !strings.Contains(got, tt.canonical) {
				t.Errorf("canonical name %q not found verbatim in output: %q", tt.canonical, got)
			}
		})
	}
}
