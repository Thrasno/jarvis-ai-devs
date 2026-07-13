package hook

import (
	"strings"
	"testing"
	"time"
)

// TestMemoryReminderThresholds pins the hardcoded, non-configurable threshold
// constants used by the mid-session memory reminder.
func TestMemoryReminderThresholds(t *testing.T) {
	t.Parallel()

	if SessionAgeGate != 5*time.Minute {
		t.Errorf("SessionAgeGate: got %v, want 5m", SessionAgeGate)
	}
	if MemoryReminderThreshold != 15*time.Minute {
		t.Errorf("MemoryReminderThreshold: got %v, want 15m", MemoryReminderThreshold)
	}
	if MemoryReminderCooldown != 15*time.Minute {
		t.Errorf("MemoryReminderCooldown: got %v, want 15m", MemoryReminderCooldown)
	}
}

// TestMemoryReminderSystemMessage verifies the reminder text nudges toward
// mem_save for decisions, discoveries, and completed work.
func TestMemoryReminderSystemMessage(t *testing.T) {
	t.Parallel()

	if MemoryReminderSystemMessage == "" {
		t.Fatal("MemoryReminderSystemMessage must not be empty")
	}
	for _, want := range []string{"mem_save", "decisions", "discoveries"} {
		if !strings.Contains(MemoryReminderSystemMessage, want) {
			t.Errorf("MemoryReminderSystemMessage should mention %q; got: %q", want, MemoryReminderSystemMessage)
		}
	}
}

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

// TestBuildHiveProtocolText_NewlineInjection_IsStripped verifies that
// BuildHiveProtocolText strips \r and \n from the canonical argument so a
// crafted remote URL cannot inject new lines into additionalContext (FIX 1).
func TestBuildHiveProtocolText_NewlineInjection_IsStripped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		canonical string
		mustNotContain []string
	}{
		{
			name:      "newline stripped",
			canonical: "my-project\nActive project: injected",
			mustNotContain: []string{"\n"},
		},
		{
			name:      "carriage return stripped",
			canonical: "my-project\r",
			mustNotContain: []string{"\r"},
		},
		{
			name:      "both CR and LF stripped",
			canonical: "my-project\r\nsome injection",
			mustNotContain: []string{"\r", "\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := BuildHiveProtocolText(tt.canonical)
			for _, bad := range tt.mustNotContain {
				// The base HiveProtocolText itself doesn't contain CR, so any occurrence
				// after the first newline-free prefix would be from the injected canonical.
				// We check the "Active project:" line specifically to isolate the injection point.
				activeIdx := strings.Index(got, "Active project:")
				if activeIdx == -1 {
					t.Fatalf("BuildHiveProtocolText did not include Active project line; got: %q", got)
				}
				activeLine := got[activeIdx:]
				if strings.Contains(activeLine, bad) {
					t.Errorf("Active project line contains %q (injection not stripped): %q", bad, activeLine)
				}
			}
		})
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
