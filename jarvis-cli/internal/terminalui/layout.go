package terminalui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// KeyHint is a key and description pair for the help bar.
type KeyHint struct {
	Key  string
	Desc string
}

// PanelWidth returns a safe panel width based on terminal width, floored at 80.
// It subtracts a 2-char margin on each side.
func PanelWidth(termWidth int) int {
	w := termWidth
	if w < 80 {
		w = 80
	}
	return w - 4
}

// SectionHeader renders "▸ LABEL ──────" filling to the given display width.
func SectionHeader(label string, width int) string {
	prefix := SectionLabelStyle.Render("▸ " + label)
	// "▸ " = 2 chars + label + " " trailing space before dashes
	visibleLen := 2 + lipgloss.Width(label) + 1
	dashes := width - visibleLen
	if dashes < 0 {
		dashes = 0
	}
	line := SeparatorStyle.Render(strings.Repeat("─", dashes))
	return prefix + " " + line + "\n"
}

// BorderedPanel wraps content in a rounded Lipgloss border at the given width.
// Width is floored at 10 to prevent degenerate layouts.
func BorderedPanel(content string, width int) string {
	if width < 10 {
		width = 10
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSurface1).
		Width(width).
		Padding(0, 1).
		Render(content)
}

// TypeBadge returns a background-colored badge for a memory observation type.
// Input is normalized to lowercase; unknown types fall back to dim text style.
func TypeBadge(category string) string {
	key := strings.ToLower(strings.TrimSpace(category))
	if key == "" || key == "-" {
		return DimTextStyle.Render("-")
	}
	if style, ok := typeBadgeMap[key]; ok {
		return style.Render(key)
	}
	return DimTextStyle.Render(key)
}

// ModeBadge returns the appropriate styled badge for a named mode or state.
func ModeBadge(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "normal", "":
		return BadgeNormal.Render("NORMAL")
	case "auth failed", "auth_failed":
		return BadgeAuthFailed.Render("auth failed")
	case "destructive":
		return BadgeDestructive.Render("destructive")
	case "local-only", "local_only":
		return BadgeLocalOnly.Render("LOCAL-ONLY")
	case "offline":
		return BadgeOfflineMode.Render("OFFLINE")
	case "secrets":
		return BadgeSecrets.Render("secrets")
	case "decision":
		return BadgeDecision.Render("decision")
	default:
		return BadgeNormal.Render(strings.ToUpper(mode))
	}
}

// StatusDot returns a colored dot indicator for a service or health state.
// The filled dot (●) indicates active/healthy; the half dot (◉) indicates
// degraded/warning; the empty dot (○) indicates offline/inactive; the cross
// dot (⊗) indicates failed/error. Distinct glyphs ensure tests are stable
// under the Ascii color profile where ANSI codes are suppressed.
func StatusDot(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "healthy", "running", "ok", "yes", "active":
		return DotHealthy.Render("●")
	case "degraded", "warning":
		return DotDegraded.Render("◉")
	case "failed", "error", "no":
		return DotFailed.Render("⊗")
	case "offline", "inactive":
		return DotOffline.Render("○")
	default:
		return DotOffline.Render("○")
	}
}

// HeaderRow renders a breadcrumb on the left and a badge right-aligned on the
// same line, padded to termWidth. Width is floored at 80.
func HeaderRow(breadcrumb string, badge string, termWidth int) string {
	w := termWidth
	if w < 80 {
		w = 80
	}
	// Compute remaining space after the breadcrumb for right-aligning the badge.
	leftWidth := lipgloss.Width(breadcrumb)
	remaining := w - leftWidth
	if remaining < lipgloss.Width(badge) {
		remaining = lipgloss.Width(badge)
	}
	rightPart := lipgloss.PlaceHorizontal(remaining, lipgloss.Right, badge)
	return breadcrumb + rightPart
}

// HelpBar renders the footer help bar with key hints left-aligned and the mode
// badge right-aligned, all within termWidth. Width is floored at 80.
func HelpBar(hints []KeyHint, mode string, termWidth int) string {
	w := termWidth
	if w < 80 {
		w = 80
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, HelpKeyStyle.Render(h.Key)+" "+HelpDescStyle.Render(h.Desc))
	}
	hintStr := strings.Join(parts, HelpDescStyle.Render("  ·  "))
	badge := ModeBadge(mode)
	hintWidth := lipgloss.Width(hintStr)
	remaining := w - hintWidth
	if remaining < lipgloss.Width(badge) {
		remaining = lipgloss.Width(badge)
	}
	return hintStr + lipgloss.PlaceHorizontal(remaining, lipgloss.Right, badge)
}

// SelectedRow renders content with the selection highlight (mauve bg, base fg),
// padded to the given display width.
func SelectedRow(content string, width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().
		Background(ColorMauve).
		Foreground(ColorBase).
		Bold(true).
		Width(width).
		Render(content)
}
