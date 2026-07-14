package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Style definitions for the TUI.
var (
	// Colors
	ColorGreen  = lipgloss.Color("#00ff00")
	ColorRed    = lipgloss.Color("#ff0000")
	ColorYellow = lipgloss.Color("#ffff00")
	ColorCyan   = lipgloss.Color("#00ffff")
	ColorBlue   = lipgloss.Color("#0088ff")
	ColorOrange = lipgloss.Color("#ff8800")
	ColorGray   = lipgloss.Color("#888888")
	ColorWhite  = lipgloss.Color("#ffffff")

	// App styles
	AppStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			BorderForeground(ColorCyan)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			MarginBottom(1)

	// Tab styles
	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorBlue).
			Padding(0, 2).
			MarginRight(1)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				Padding(0, 2).
				MarginRight(1)

	// Category header
	CatLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorYellow).
			MarginTop(1).
			MarginBottom(0)

	CatSummaryStyle = lipgloss.NewStyle().
			Foreground(ColorGray).
			Italic(true).
			MarginLeft(2)

	// Item states
	ItemOKStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	ItemOutdatedStyle = lipgloss.NewStyle().
				Foreground(ColorYellow)

	ItemErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	ItemSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true)

	ItemCursorStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Background(lipgloss.Color("#333333"))

	// Version formatting
	VerCurrentStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	VerArrowStyle = lipgloss.NewStyle().
			Foreground(ColorOrange)

	VerNewStyle = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Bold(true)

	// Progress bars
	BarStyle = lipgloss.NewStyle().
			Height(1)

	BarFilled = lipgloss.NewStyle().
			Background(ColorGreen)

	BarEmpty = lipgloss.NewStyle().
			Background(ColorGray)

	// Footer / keybindings
	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorGray).
			Italic(true).
			MarginTop(1)

	// Spinner
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(ColorCyan)

	// Log styles
	LogSuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	LogErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	// Reclaimable amount style
	ReclaimStyle = lipgloss.NewStyle().
			Foreground(ColorOrange).
			Bold(true)

	// Selection indicator
	CheckboxStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	// Button style
	ButtonStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorBlue).
			Padding(0, 1).
			Bold(true)
)
