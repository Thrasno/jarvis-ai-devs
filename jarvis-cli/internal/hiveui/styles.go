package hiveui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha palette — https://github.com/catppuccin/catppuccin
var (
	colorBase     = lipgloss.Color("#1e1e2e")
	colorSurface0 = lipgloss.Color("#313244")
	colorSurface1 = lipgloss.Color("#45475a")
	colorOverlay2 = lipgloss.Color("#9399b2")
	colorText     = lipgloss.Color("#cdd6f4")
	colorMauve    = lipgloss.Color("#cba6f7")
	colorRed      = lipgloss.Color("#f38ba8")
	colorGreen    = lipgloss.Color("#a6e3a1")
	colorYellow   = lipgloss.Color("#f9e2af")
	colorPeach    = lipgloss.Color("#fab387")
	colorTeal     = lipgloss.Color("#94e2d5")
	colorBlue     = lipgloss.Color("#89b4fa")
	colorFlamingo = lipgloss.Color("#f2cdcd")

	// Semantic text styles
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorMauve)
	breadcrumbStyle   = lipgloss.NewStyle().Foreground(colorOverlay2)
	breadcrumbCurrent = lipgloss.NewStyle().Foreground(colorMauve).Bold(true)
	columnHeaderStyle = lipgloss.NewStyle().Foreground(colorOverlay2).Bold(true)
	cursorStyle       = lipgloss.NewStyle().Foreground(colorMauve).Bold(true)
	dimTextStyle      = lipgloss.NewStyle().Foreground(colorOverlay2)
	helpKeyStyle      = lipgloss.NewStyle().Foreground(colorMauve).Bold(true)
	helpDescStyle     = lipgloss.NewStyle().Foreground(colorOverlay2)
	sectionLabelStyle = lipgloss.NewStyle().Foreground(colorOverlay2)
	separatorStyle    = lipgloss.NewStyle().Foreground(colorSurface1)
	readOnlyBanner    = lipgloss.NewStyle().Foreground(colorTeal)
	guardPending      = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)

	// Mode/status badges (with background)
	badgeNormal      = lipgloss.NewStyle().Background(colorSurface1).Foreground(colorText).Padding(0, 1)
	badgeAuthFailed  = lipgloss.NewStyle().Background(colorRed).Foreground(colorBase).Bold(true).Padding(0, 1)
	badgeDestructive = lipgloss.NewStyle().Background(colorRed).Foreground(colorBase).Bold(true).Padding(0, 1)
	badgeLocalOnly   = lipgloss.NewStyle().Background(colorSurface1).Foreground(colorOverlay2).Padding(0, 1)
	badgeOfflineMode = lipgloss.NewStyle().Background(colorRed).Foreground(colorBase).Bold(true).Padding(0, 1)
	badgeSecrets     = lipgloss.NewStyle().Background(colorPeach).Foreground(colorBase).Padding(0, 1)
	badgeDecision    = lipgloss.NewStyle().Background(colorMauve).Foreground(colorBase).Padding(0, 1)

	// Status dots
	dotHealthy  = lipgloss.NewStyle().Foreground(colorGreen)
	dotDegraded = lipgloss.NewStyle().Foreground(colorYellow)
	dotOffline  = lipgloss.NewStyle().Foreground(colorOverlay2)
	dotFailed   = lipgloss.NewStyle().Foreground(colorRed)

	// Type badge map
	typeBadgeMap = map[string]lipgloss.Style{
		"decision":     lipgloss.NewStyle().Background(colorMauve).Foreground(colorBase).Padding(0, 1),
		"bugfix":       lipgloss.NewStyle().Background(colorRed).Foreground(colorBase).Padding(0, 1),
		"pattern":      lipgloss.NewStyle().Background(colorTeal).Foreground(colorBase).Padding(0, 1),
		"architecture": lipgloss.NewStyle().Background(colorBlue).Foreground(colorBase).Padding(0, 1),
		"config":       lipgloss.NewStyle().Background(colorPeach).Foreground(colorBase).Padding(0, 1),
		"preference":   lipgloss.NewStyle().Background(colorFlamingo).Foreground(colorBase).Padding(0, 1),
		"discovery":    lipgloss.NewStyle().Background(colorGreen).Foreground(colorBase).Padding(0, 1),
	}

	// Badge aliases used by severity/state helper functions
	badgeHealthy  = lipgloss.NewStyle().Background(colorGreen).Foreground(colorBase).Bold(true).Padding(0, 1)
	badgeDegraded = lipgloss.NewStyle().Background(colorYellow).Foreground(colorBase).Bold(true).Padding(0, 1)
	badgeOffline  = lipgloss.NewStyle().Background(colorSurface1).Foreground(colorOverlay2).Padding(0, 1)
	badgeCritical = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	badgeWarning  = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
)
