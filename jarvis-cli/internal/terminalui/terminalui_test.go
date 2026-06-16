package terminalui_test

import (
	"os"
	"strings"
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
	if !strings.Contains(result, content) {
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
		if !strings.Contains(result, h.Key) {
			t.Errorf("HelpBar output missing key %q", h.Key)
		}
		if !strings.Contains(result, h.Desc) {
			t.Errorf("HelpBar output missing desc %q", h.Desc)
		}
	}
}

func TestSectionHeader_RendersSemanticContent(t *testing.T) {
	for _, tt := range []struct {
		name  string
		label string
		width int
		wants []string
	}{
		{name: "label with separator", label: "Memory", width: 32, wants: []string{"▸ Memory", "─"}},
		{name: "empty label remains deterministic", label: "", width: 4, wants: []string{"▸ ", "─"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := terminalui.SectionHeader(tt.label, tt.width)
			if result == "" {
				t.Fatal("SectionHeader returned empty string")
			}
			if !strings.HasSuffix(result, "\n") {
				t.Fatalf("SectionHeader should end with newline, got %q", result)
			}
			assertContainsAll(t, result, tt.wants...)
		})
	}
}

func TestTypeBadge_RendersNormalizedVisibleText(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "known type", input: "  BUGFIX  ", want: "bugfix"},
		{name: "empty type uses dash", input: "", want: "-"},
		{name: "unknown type is normalized", input: "Custom Type", want: "custom type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := terminalui.TypeBadge(tt.input)
			if result == "" {
				t.Fatal("TypeBadge returned empty string")
			}
			if !strings.Contains(result, tt.want) {
				t.Fatalf("TypeBadge(%q) missing %q in %q", tt.input, tt.want, result)
			}
		})
	}
}

func TestModeBadge_RendersSemanticModeText(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty mode defaults to normal", input: "", want: "NORMAL"},
		{name: "offline mode", input: "offline", want: "OFFLINE"},
		{name: "unknown mode uppercases input", input: "review", want: "REVIEW"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := terminalui.ModeBadge(tt.input)
			if result == "" {
				t.Fatal("ModeBadge returned empty string")
			}
			if !strings.Contains(result, tt.want) {
				t.Fatalf("ModeBadge(%q) missing %q in %q", tt.input, tt.want, result)
			}
		})
	}
}

func TestHeaderRow_RendersBreadcrumbAndBadge(t *testing.T) {
	for _, tt := range []struct {
		name       string
		breadcrumb string
		badge      string
		width      int
		wants      []string
	}{
		{name: "normal row", breadcrumb: "Jarvis / Hive", badge: terminalui.ModeBadge("offline"), width: 40, wants: []string{"Jarvis / Hive", "OFFLINE"}},
		{name: "minimal row", breadcrumb: "", badge: terminalui.ModeBadge(""), width: 1, wants: []string{"NORMAL"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := terminalui.HeaderRow(tt.breadcrumb, tt.badge, tt.width)
			if result == "" {
				t.Fatal("HeaderRow returned empty string")
			}
			assertContainsAll(t, result, tt.wants...)
		})
	}
}

func TestSelectedRow_RendersContentAndMinimalWidth(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		width   int
		want    string
	}{
		{name: "selected content", content: "Sync memory", width: 24, want: "Sync memory"},
		{name: "empty content still renders padded row", content: "", width: 0, want: " "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := terminalui.SelectedRow(tt.content, tt.width)
			if result == "" {
				t.Fatal("SelectedRow returned empty string")
			}
			if !strings.Contains(result, tt.want) {
				t.Fatalf("SelectedRow(%q, %d) missing %q in %q", tt.content, tt.width, tt.want, result)
			}
		})
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

func assertContainsAll(t *testing.T, result string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(result, want) {
			t.Fatalf("output missing %q in %q", want, result)
		}
	}
}
