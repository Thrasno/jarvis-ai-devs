package terminalui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha palette — https://github.com/catppuccin/catppuccin
var (
	ColorBase     = lipgloss.Color("#1e1e2e")
	ColorSurface0 = lipgloss.Color("#313244")
	ColorSurface1 = lipgloss.Color("#45475a")
	ColorOverlay2 = lipgloss.Color("#9399b2")
	ColorText     = lipgloss.Color("#cdd6f4")
	ColorMauve    = lipgloss.Color("#cba6f7")
	ColorRed      = lipgloss.Color("#f38ba8")
	ColorGreen    = lipgloss.Color("#a6e3a1")
	ColorYellow   = lipgloss.Color("#f9e2af")
	ColorPeach    = lipgloss.Color("#fab387")
	ColorTeal     = lipgloss.Color("#94e2d5")
	ColorBlue     = lipgloss.Color("#89b4fa")
	ColorFlamingo = lipgloss.Color("#f2cdcd")

	// Semantic text styles
	TitleStyle             = lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	BreadcrumbStyle        = lipgloss.NewStyle().Foreground(ColorOverlay2)
	BreadcrumbCurrentStyle = lipgloss.NewStyle().Foreground(ColorMauve).Bold(true)
	ColumnHeaderStyle      = lipgloss.NewStyle().Foreground(ColorOverlay2).Bold(true)
	CursorStyle            = lipgloss.NewStyle().Foreground(ColorMauve).Bold(true)
	DimTextStyle           = lipgloss.NewStyle().Foreground(ColorOverlay2)
	HelpKeyStyle           = lipgloss.NewStyle().Foreground(ColorMauve).Bold(true)
	HelpDescStyle          = lipgloss.NewStyle().Foreground(ColorOverlay2)
	SectionLabelStyle      = lipgloss.NewStyle().Foreground(ColorOverlay2)
	SeparatorStyle         = lipgloss.NewStyle().Foreground(ColorSurface1)
)
