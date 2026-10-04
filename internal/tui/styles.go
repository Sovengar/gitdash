package tui

import "charm.land/lipgloss/v2"

type lipglossStyle = lipgloss.Style

// Column widths, all wider than their header so pad() guarantees a separator between columns.
const (
	colName     = 26
	colBranch   = 24
	colWT       = 11
	colUpDown   = 9
	colSync     = 12
	colActivity = 10
	colFetch    = 9
)

var (
	styleCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	styleClean    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleDirty    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleAhead    = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleBehind   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleDiverged = lipgloss.NewStyle().Foreground(lipgloss.Color("201"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	styleFetchRun = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleFetchBad = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	styleBar  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleHint = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleSel  = lipgloss.NewStyle().Bold(true)

	styleGroupHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleSecondaryHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

	styleWorktree = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	styleDetailKey = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))

	// Command log panel: the execution reads normal while the intent (the key) stays faint, because it is context and not result.
	styleLogExec   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleLogIntent = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleLogOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))

	borderColor = lipgloss.Color("238")

	styleToastSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleToastError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleToastInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleToastWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
)
