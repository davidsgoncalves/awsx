package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleFaint = lipgloss.NewStyle().Faint(true)
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)
