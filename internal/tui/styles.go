package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	helpStyle   = lipgloss.NewStyle().Faint(true)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render
	statusFail  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render
	statusWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render
)
