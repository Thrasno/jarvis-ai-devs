package terminalui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

func TestHelpBarFitsDisplayWidthWithoutWrapping(t *testing.T) {
	hints := []terminalui.KeyHint{
		{Key: "j/k", Desc: "scroll"},
		{Key: "pgup/dn", Desc: "page"},
		{Key: "home/end", Desc: "bounds"},
		{Key: "w", Desc: "warnings"},
		{Key: "c", Desc: "config"},
		{Key: "esc", Desc: "back"},
		{Key: "q", Desc: "quit"},
	}
	for _, width := range []int{20, 80, 100} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			footer := terminalui.HelpBar(hints, "normal", width)
			if strings.Contains(footer, "\n") || lipgloss.Width(footer) > width {
				t.Fatalf("HelpBar width = %d with %q, want one display line at most %d cells", lipgloss.Width(footer), footer, width)
			}
		})
	}
}

func TestHeaderRowFitsKnownNarrowWidth(t *testing.T) {
	header := terminalui.HeaderRow("a breadcrumb that is much longer than a narrow terminal", terminalui.ModeBadge("normal"), 20)
	if strings.Contains(header, "\n") || lipgloss.Width(header) > 20 {
		t.Fatalf("HeaderRow width = %d with %q, want one display line at most 20 cells", lipgloss.Width(header), header)
	}
}

func TestPhysicalLinesWrapByDisplayWidth(t *testing.T) {
	lines := terminalui.PhysicalLines("界界界", 4)
	if len(lines) != 2 || lipgloss.Width(lines[0]) > 4 || lipgloss.Width(lines[1]) > 4 {
		t.Fatalf("PhysicalLines = %#v, want two lines no wider than four cells", lines)
	}
}

func TestAvailableContentHeight(t *testing.T) {
	tests := []struct {
		name         string
		terminal     int
		header       int
		footer       int
		wantContents int
	}{
		{name: "normal terminal reserves header and footer", terminal: 24, header: 1, footer: 1, wantContents: 22},
		{name: "tiny terminal does not produce negative content", terminal: 1, header: 1, footer: 1, wantContents: 0},
		{name: "zero terminal height is unbounded sentinel", terminal: 0, header: 1, footer: 1, wantContents: 0},
		{name: "negative dimensions are safe", terminal: -3, header: -1, footer: 1, wantContents: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := terminalui.AvailableContentHeight(tt.terminal, tt.header, tt.footer); got != tt.wantContents {
				t.Fatalf("AvailableContentHeight(%d, %d, %d) = %d, want %d", tt.terminal, tt.header, tt.footer, got, tt.wantContents)
			}
		})
	}
}
