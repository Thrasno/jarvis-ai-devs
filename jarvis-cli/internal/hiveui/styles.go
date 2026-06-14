package hiveui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

// Palette and design-system styles are owned by internal/terminalui.
// Package-level aliases allow existing hiveui-internal callers to compile
// without touching every reference site in model.go.
var (
	colorBase     = terminalui.ColorBase
	colorSurface0 = terminalui.ColorSurface0
	colorSurface1 = terminalui.ColorSurface1
	colorOverlay2 = terminalui.ColorOverlay2
	colorText     = terminalui.ColorText
	colorMauve    = terminalui.ColorMauve
	colorRed      = terminalui.ColorRed
	colorGreen    = terminalui.ColorGreen
	colorYellow   = terminalui.ColorYellow
	colorPeach    = terminalui.ColorPeach
	colorTeal     = terminalui.ColorTeal
	colorBlue     = terminalui.ColorBlue
	colorFlamingo = terminalui.ColorFlamingo

	// Semantic text styles — aliases to terminalui equivalents
	titleStyle        = terminalui.TitleStyle
	breadcrumbStyle   = terminalui.BreadcrumbStyle
	breadcrumbCurrent = terminalui.BreadcrumbCurrentStyle
	columnHeaderStyle = terminalui.ColumnHeaderStyle
	cursorStyle       = terminalui.CursorStyle
	dimTextStyle      = terminalui.DimTextStyle
	helpKeyStyle      = terminalui.HelpKeyStyle
	helpDescStyle     = terminalui.HelpDescStyle
	sectionLabelStyle = terminalui.SectionLabelStyle
	separatorStyle    = terminalui.SeparatorStyle

	// hiveui-only render vars: used exclusively within this package
	readOnlyBanner = lipgloss.NewStyle().Foreground(terminalui.ColorTeal)
	guardPending   = lipgloss.NewStyle().Foreground(terminalui.ColorYellow).Bold(true)

	// Mode/status badges — aliases to terminalui
	badgeNormal      = terminalui.BadgeNormal
	badgeAuthFailed  = terminalui.BadgeAuthFailed
	badgeDestructive = terminalui.BadgeDestructive
	badgeLocalOnly   = terminalui.BadgeLocalOnly
	badgeOfflineMode = terminalui.BadgeOfflineMode
	badgeSecrets     = terminalui.BadgeSecrets
	badgeDecision    = terminalui.BadgeDecision

	// Status dots — aliases to terminalui
	dotHealthy  = terminalui.DotHealthy
	dotDegraded = terminalui.DotDegraded
	dotOffline  = terminalui.DotOffline
	dotFailed   = terminalui.DotFailed

	// Health badge aliases — aliases to terminalui
	badgeHealthy  = terminalui.BadgeHealthy
	badgeDegraded = terminalui.BadgeDegraded
	badgeOffline  = terminalui.BadgeOffline
	badgeCritical = terminalui.BadgeCritical
	badgeWarning  = terminalui.BadgeWarning
)
