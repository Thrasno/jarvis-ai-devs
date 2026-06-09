package hiveui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// panelWidth returns a safe panel width based on terminal width, floored at 80.
// It subtracts a 2-char margin on each side.
func panelWidth(termWidth int) int {
	w := termWidth
	if w < 80 {
		w = 80
	}
	return w - 4
}

// sectionHeader renders "▸ LABEL ──────" filling to the given display width.
func sectionHeader(label string, width int) string {
	prefix := sectionLabelStyle.Render("▸ " + label)
	// "▸ " = 2 chars + label + " " trailing space before dashes
	visibleLen := 2 + lipgloss.Width(label) + 1
	dashes := width - visibleLen
	if dashes < 0 {
		dashes = 0
	}
	line := separatorStyle.Render(strings.Repeat("─", dashes))
	return prefix + " " + line + "\n"
}

// borderedPanel wraps content in a rounded Lipgloss border at the given width.
// Width is floored at 10 to prevent degenerate layouts.
func borderedPanel(content string, width int) string {
	if width < 10 {
		width = 10
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Width(width).
		Padding(0, 1).
		Render(content)
}

// typeBadge returns a background-colored badge for a memory observation type.
// Input is normalized to lowercase; unknown types fall back to dim text style.
func typeBadge(category string) string {
	key := strings.ToLower(strings.TrimSpace(category))
	if key == "" || key == "-" {
		return dimTextStyle.Render("-")
	}
	if style, ok := typeBadgeMap[key]; ok {
		return style.Render(key)
	}
	return dimTextStyle.Render(key)
}

// modeBadge returns the appropriate styled badge for a named mode or state.
func modeBadge(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "normal", "":
		return badgeNormal.Render("NORMAL")
	case "auth failed", "auth_failed":
		return badgeAuthFailed.Render("auth failed")
	case "destructive":
		return badgeDestructive.Render("destructive")
	case "local-only", "local_only":
		return badgeLocalOnly.Render("LOCAL-ONLY")
	case "offline":
		return badgeOfflineMode.Render("OFFLINE")
	case "secrets":
		return badgeSecrets.Render("secrets")
	case "decision":
		return badgeDecision.Render("decision")
	default:
		return badgeNormal.Render(strings.ToUpper(mode))
	}
}

// statusDot returns a colored dot indicator for a service or health state.
func statusDot(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "healthy", "running", "ok", "yes":
		return dotHealthy.Render("●")
	case "degraded", "warning":
		return dotDegraded.Render("●")
	case "failed", "error", "no", "offline":
		return dotFailed.Render("●")
	default:
		return dotOffline.Render("○")
	}
}

// headerRow renders a breadcrumb on the left and a badge right-aligned on the
// same line, padded to termWidth. Width is floored at 80.
func headerRow(breadcrumb string, badge string, termWidth int) string {
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

// KeyHint is a key and description pair for the help bar.
type KeyHint struct {
	Key  string
	Desc string
}

// helpBar renders the footer help bar with key hints left-aligned and the mode
// badge right-aligned, all within termWidth. Width is floored at 80.
func helpBar(hints []KeyHint, mode string, termWidth int) string {
	w := termWidth
	if w < 80 {
		w = 80
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, helpKeyStyle.Render(h.Key)+" "+helpDescStyle.Render(h.Desc))
	}
	hintStr := strings.Join(parts, helpDescStyle.Render("  ·  "))
	badge := modeBadge(mode)
	hintWidth := lipgloss.Width(hintStr)
	remaining := w - hintWidth
	if remaining < lipgloss.Width(badge) {
		remaining = lipgloss.Width(badge)
	}
	return hintStr + lipgloss.PlaceHorizontal(remaining, lipgloss.Right, badge)
}

// selectedRow renders content with the selection highlight (mauve bg, base fg),
// padded to the given display width.
func selectedRow(content string, width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().
		Background(colorMauve).
		Foreground(colorBase).
		Bold(true).
		Width(width).
		Render(content)
}
