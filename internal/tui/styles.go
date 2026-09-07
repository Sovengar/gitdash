// Estilos lipgloss del dashboard (spec 0001 R6).
package tui

import "charm.land/lipgloss/v2"

// lipglossStyle alias corto para las firmas de las celdas.
type lipglossStyle = lipgloss.Style

// anchos de columna de la tabla (R6).
const (
	colName     = 26
	colGroup    = 10
	colBranch   = 24
	colState    = 12
	colUpDown   = 9
	colActivity = 8
	colFetch    = 9
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	styleCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	styleClean    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleDirty    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // amarillo
	styleAhead    = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))  // verde
	styleBehind   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // cian
	styleDiverged = lipgloss.NewStyle().Foreground(lipgloss.Color("201")) // magenta
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // rojo
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // naranja
	styleFetchRun = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleFetchOk  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleFetchBad = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	styleBar  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleHint = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleSel  = lipgloss.NewStyle().Bold(true)

	styleDetailKey   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleDetailTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
)
