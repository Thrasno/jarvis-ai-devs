// Package tui tests run under forced Ascii color profile (via TestMain) to ensure
// deterministic output regardless of terminal environment.
package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}
