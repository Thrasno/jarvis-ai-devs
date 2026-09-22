package terminalui_test

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

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
