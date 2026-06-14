package terminalui_test

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// TestPanelWidth_Floor verifies that a terminal width below 80 is floored to 80
// and the margin subtraction is applied correctly.
func TestPanelWidth_Floor(t *testing.T) {
	// PanelWidth with termWidth 40: floor to 80, then 80 - 4 = 76.
	got := terminalui.PanelWidth(40)
	if got != 76 {
		t.Errorf("PanelWidth(40) = %d; want 76", got)
	}
}

// TestPanelWidth_Wide verifies that a terminal width above 80 passes through
// after subtracting the 4-char margin.
func TestPanelWidth_Wide(t *testing.T) {
	got := terminalui.PanelWidth(120)
	if got != 116 {
		t.Errorf("PanelWidth(120) = %d; want 116", got)
	}
}

// TestBorderedPanel_ContainsContent verifies that BorderedPanel includes the
// provided content string in its output.
func TestBorderedPanel_ContainsContent(t *testing.T) {
	content := "hello terminalui"
	result := terminalui.BorderedPanel(content, 80)
	if len(result) == 0 {
		t.Fatal("BorderedPanel returned empty string")
	}
	if !containsStr(result, content) {
		t.Errorf("BorderedPanel output does not contain %q\ngot: %q", content, result)
	}
}

// TestHelpBar_ContainsAllHints verifies that HelpBar renders all supplied
// key/description pairs in the output string.
func TestHelpBar_ContainsAllHints(t *testing.T) {
	hints := []terminalui.KeyHint{
		{Key: "q", Desc: "quit"},
		{Key: "↵", Desc: "select"},
	}
	result := terminalui.HelpBar(hints, "normal", 100)
	for _, h := range hints {
		if !containsStr(result, h.Key) {
			t.Errorf("HelpBar output missing key %q", h.Key)
		}
		if !containsStr(result, h.Desc) {
			t.Errorf("HelpBar output missing desc %q", h.Desc)
		}
	}
}

// TestStatusDot_DistinctPerState verifies that StatusDot returns a non-empty
// string for each of the named states and that all three are mutually distinct.
func TestStatusDot_DistinctPerState(t *testing.T) {
	active := terminalui.StatusDot("active")
	inactive := terminalui.StatusDot("inactive")
	errState := terminalui.StatusDot("error")

	for _, s := range []string{active, inactive, errState} {
		if s == "" {
			t.Error("StatusDot returned empty string for a named state")
		}
	}
	if active == inactive {
		t.Errorf("StatusDot(active) == StatusDot(inactive): %q", active)
	}
	if active == errState {
		t.Errorf("StatusDot(active) == StatusDot(error): %q", active)
	}
	if inactive == errState {
		t.Errorf("StatusDot(inactive) == StatusDot(error): %q", inactive)
	}
}

// containsStr reports whether s contains substr.
func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
