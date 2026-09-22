package terminalui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// KeyHint is a key and description pair for the help bar.
type KeyHint struct {
	Key  string
	Desc string
}

// AvailableContentHeight returns the height left after reserving fixed header
// and footer lines. A zero height means terminal size is not known yet; callers
// use that sentinel to preserve their unbounded pre-resize behavior.
func AvailableContentHeight(terminalHeight, headerLines, footerLines int) int {
	if terminalHeight <= 0 {
		return 0
	}
	if headerLines < 0 {
		headerLines = 0
	}
	if footerLines < 0 {
		footerLines = 0
	}
	fixedLines := headerLines + footerLines
	if fixedLines >= terminalHeight {
		return 0
	}
	return terminalHeight - fixedLines
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

// ContentWidth returns the panel content width for a known terminal width.
// It retains the pre-resize 80-column fallback only when width is unknown.
func ContentWidth(termWidth int) int {
	w := termWidth
	if w <= 0 {
		w = 80
	}
	return max(1, w-4)
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
// same physical line. Long breadcrumbs are truncated to the terminal width.
func HeaderRow(breadcrumb string, badge string, termWidth int) string {
	w := termWidth
	if w < 14 {
		w = 80
	}
	badge = truncateDisplay(badge, w)
	breadcrumb = truncateDisplay(breadcrumb, max(0, w-lipgloss.Width(badge)))
	remaining := w - lipgloss.Width(breadcrumb)
	return breadcrumb + lipgloss.PlaceHorizontal(remaining, lipgloss.Right, badge)
}

// HelpBar renders the footer help bar with key hints left-aligned and the mode
// badge right-aligned on one display-width-safe physical line.
func HelpBar(hints []KeyHint, mode string, termWidth int) string {
	w := termWidth
	if w <= 0 {
		w = 80
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, HelpKeyStyle.Render(h.Key)+" "+HelpDescStyle.Render(h.Desc))
	}
	badge := ModeBadge(mode)
	hintStr := strings.Join(parts, HelpDescStyle.Render("  ·  "))
	badge = truncateDisplay(badge, w)
	available := max(0, w-lipgloss.Width(badge))
	if lipgloss.Width(hintStr) > available {
		hintStr = strings.Join(parts, HelpDescStyle.Render(" · "))
	}
	if lipgloss.Width(hintStr) > available {
		hintStr = strings.Join(parts, HelpDescStyle.Render("·"))
	}
	hintStr = truncateDisplay(hintStr, available)
	remaining := w - lipgloss.Width(hintStr)
	return hintStr + lipgloss.PlaceHorizontal(remaining, lipgloss.Right, badge)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func truncateDisplay(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	value = ansiEscape.ReplaceAllString(value, "")
	var result strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := lipgloss.Width(string(r))
		if used+runeWidth > width {
			break
		}
		result.WriteRune(r)
		used += runeWidth
	}
	return result.String()
}

// PhysicalLines wraps text by display-cell width and returns the rows a
// terminal will occupy. It is ANSI-aware through Lip Gloss width handling.
func PhysicalLines(content string, width int) []string {
	if width < 1 {
		width = 1
	}
	if content == "" {
		return []string{""}
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(content), "\n")
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
