package hiveui

import "github.com/charmbracelet/lipgloss"

// Style tokens for the Hive TUI. Palette mirrors tui/steps.go for shared
// semantics; badge colors (208, 196, 39, 244) extend where steps.go has no
// equivalent token.
var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	columnHeaderStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	cursorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	selectedRowStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	dimTextStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	badgeHealthy      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	badgeDegraded     = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	badgeOffline      = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	badgeDeleted      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	badgeWarning      = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	badgeCritical     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	timelineSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	readOnlyBanner    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Italic(true)
	guardPending      = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
)
