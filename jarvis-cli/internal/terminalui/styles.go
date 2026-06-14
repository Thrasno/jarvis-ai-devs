package terminalui

import "github.com/charmbracelet/lipgloss"

var (
	// Mode/status badges (with background)
	BadgeNormal      = lipgloss.NewStyle().Background(ColorSurface1).Foreground(ColorText).Padding(0, 1)
	BadgeAuthFailed  = lipgloss.NewStyle().Background(ColorRed).Foreground(ColorBase).Bold(true).Padding(0, 1)
	BadgeDestructive = lipgloss.NewStyle().Background(ColorRed).Foreground(ColorBase).Bold(true).Padding(0, 1)
	BadgeLocalOnly   = lipgloss.NewStyle().Background(ColorSurface1).Foreground(ColorOverlay2).Padding(0, 1)
	BadgeOfflineMode = lipgloss.NewStyle().Background(ColorRed).Foreground(ColorBase).Bold(true).Padding(0, 1)
	BadgeSecrets     = lipgloss.NewStyle().Background(ColorPeach).Foreground(ColorBase).Padding(0, 1)
	BadgeDecision    = lipgloss.NewStyle().Background(ColorMauve).Foreground(ColorBase).Padding(0, 1)

	// Health badge aliases (severity/state helper functions)
	BadgeHealthy  = lipgloss.NewStyle().Background(ColorGreen).Foreground(ColorBase).Bold(true).Padding(0, 1)
	BadgeDegraded = lipgloss.NewStyle().Background(ColorYellow).Foreground(ColorBase).Bold(true).Padding(0, 1)
	BadgeOffline  = lipgloss.NewStyle().Background(ColorSurface1).Foreground(ColorOverlay2).Padding(0, 1)
	BadgeCritical = lipgloss.NewStyle().Background(ColorRed).Foreground(ColorBase).Bold(true).Padding(0, 1)
	BadgeWarning  = lipgloss.NewStyle().Background(ColorYellow).Foreground(ColorBase).Bold(true).Padding(0, 1)

	// Status dots
	DotHealthy  = lipgloss.NewStyle().Foreground(ColorGreen)
	DotDegraded = lipgloss.NewStyle().Foreground(ColorYellow)
	DotOffline  = lipgloss.NewStyle().Foreground(ColorOverlay2)
	DotFailed   = lipgloss.NewStyle().Foreground(ColorRed)

	// typeBadgeMap maps observation type names to their badge styles.
	typeBadgeMap = map[string]lipgloss.Style{
		"decision":     lipgloss.NewStyle().Background(ColorMauve).Foreground(ColorBase).Padding(0, 1),
		"bugfix":       lipgloss.NewStyle().Background(ColorRed).Foreground(ColorBase).Padding(0, 1),
		"pattern":      lipgloss.NewStyle().Background(ColorTeal).Foreground(ColorBase).Padding(0, 1),
		"architecture": lipgloss.NewStyle().Background(ColorBlue).Foreground(ColorBase).Padding(0, 1),
		"config":       lipgloss.NewStyle().Background(ColorPeach).Foreground(ColorBase).Padding(0, 1),
		"preference":   lipgloss.NewStyle().Background(ColorFlamingo).Foreground(ColorBase).Padding(0, 1),
		"discovery":    lipgloss.NewStyle().Background(ColorGreen).Foreground(ColorBase).Padding(0, 1),
	}
)
