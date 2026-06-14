package hiveui

import "github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"

// KeyHint is a key and description pair for the help bar.
// Kept as a local type so existing unkeyed struct literals in model.go compile
// without modification. The helpBar wrapper converts to terminalui.KeyHint.
type KeyHint struct {
	Key  string
	Desc string
}

// panelWidth delegates to terminalui.PanelWidth.
func panelWidth(termWidth int) int { return terminalui.PanelWidth(termWidth) }

// sectionHeader delegates to terminalui.SectionHeader.
func sectionHeader(label string, width int) string { return terminalui.SectionHeader(label, width) }

// borderedPanel delegates to terminalui.BorderedPanel.
func borderedPanel(content string, width int) string { return terminalui.BorderedPanel(content, width) }

// typeBadge delegates to terminalui.TypeBadge.
func typeBadge(category string) string { return terminalui.TypeBadge(category) }

// modeBadge delegates to terminalui.ModeBadge.
func modeBadge(mode string) string { return terminalui.ModeBadge(mode) }

// statusDot delegates to terminalui.StatusDot.
func statusDot(state string) string { return terminalui.StatusDot(state) }

// headerRow delegates to terminalui.HeaderRow.
func headerRow(breadcrumb, badge string, termWidth int) string {
	return terminalui.HeaderRow(breadcrumb, badge, termWidth)
}

// helpBar delegates to terminalui.HelpBar, converting the local KeyHint slice
// to terminalui.KeyHint so callers in model.go need no changes.
func helpBar(hints []KeyHint, mode string, termWidth int) string {
	th := make([]terminalui.KeyHint, len(hints))
	for i, h := range hints {
		th[i] = terminalui.KeyHint{Key: h.Key, Desc: h.Desc}
	}
	return terminalui.HelpBar(th, mode, termWidth)
}

// selectedRow delegates to terminalui.SelectedRow.
func selectedRow(content string, width int) string { return terminalui.SelectedRow(content, width) }
