package editor

import "charm.land/lipgloss/v2"

var (
	colorAccent  = lipgloss.Color("#7D56F4")
	colorError   = lipgloss.Color("#FF5F87")
	colorWarning = lipgloss.Color("#FFAF00")
	colorOK      = lipgloss.Color("#5FD787")
	colorDim     = lipgloss.Color("#8A8A8A")
	colorBorder  = lipgloss.Color("#4E4E4E")

	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorAccent).Padding(0, 1)
	paneStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder)
	focusedPane   = paneStyle.BorderForeground(colorAccent)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#3A3A5A")).Bold(true)
	keyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFAFFF"))
	dimStyle      = lipgloss.NewStyle().Foreground(colorDim)
	errorStyle    = lipgloss.NewStyle().Foreground(colorError)
	warningStyle  = lipgloss.NewStyle().Foreground(colorWarning)
	okStyle       = lipgloss.NewStyle().Foreground(colorOK)
	promptStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorWarning)
)
