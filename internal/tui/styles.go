// Estilos lipgloss del dashboard.
package tui

import "charm.land/lipgloss/v2"

// lipglossStyle alias corto para las firmas de las celdas.
type lipglossStyle = lipgloss.Style

// anchos de columna de la tabla (todo ancho > longitud de
// su header para que pad() garantice separador entre columnas).
const (
	colName     = 26
	colBranch   = 24
	colWT       = 11 // "Work Tree" (9) + separador
	colUpDown   = 9  // "↑↓up"
	colSync     = 12 // "SYNC" + "<rama> ↓NN"
	colActivity = 10 // "ACTIVITY" (8) + separador
	colFetch    = 9  // "FETCH"
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
	styleFetchBad = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	styleBar       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleHint      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	styleSel       = lipgloss.NewStyle().Bold(true)

	// Header de grupo (vroom)
	styleGroupHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	// Header secundario (nivel 2, menos protagonista)
	styleSecondaryHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

	// Sub-fila de worktree (indentada, distinta de repo/header)
	styleWorktree = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	styleDetailKey   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleDetailTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
)
