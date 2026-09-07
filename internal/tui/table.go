// Filas de la tabla: filtrado, orden y render de celdas (spec 0001 R6-R7).
package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// row agrupa un proyecto con su snapshot vivo para render.
type row struct {
	project discovery.Project
	snap    gitstatus.Snapshot
	state   gitstatus.State
}

// pendingStates son los estados que el filtro `n` considera no-limpios:
// cambios pendientes de subir/bajar o en el working tree. Los errores y los
// proyectos sin repo se excluyen (R7, interpretación documentada en spec).
func pendingStates(st gitstatus.State) bool {
	switch st {
	case gitstatus.StateDirty, gitstatus.StateAhead, gitstatus.StateBehind, gitstatus.StateDiverged:
		return true
	}
	return false
}

// rows construye la lista de filas visibles: filtra (n, /) y ordena
// atención-primero, actividad, nombre (R6, R7).
func (m *Model) rows() []row {
	out := make([]row, 0, len(m.projects))
	for _, p := range m.projects {
		snap := m.states[p.Path]
		st := snap.State(p.HasRepo)
		if m.onlyDirty && !pendingStates(st) {
			continue
		}
		if m.search != "" && !matchSearch(p, m.search) {
			continue
		}
		out = append(out, row{project: p, snap: snap, state: st})
	}

	sortRows(out)
	return out
}

// matchSearch compara nombre y grupo, case-insensitive (S7.2).
func matchSearch(p discovery.Project, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Group), q)
}

// sortRows ordena in-place: score desc, último commit desc, nombre asc (R6).
func sortRows(rows []row) {
	// insertion sort simple: los repos son cientos, no miles.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rowLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func rowLess(a, b row) bool {
	sa, sb := a.state.Score(), b.state.Score() // R6: atención-primero
	if sa != sb {
		return sa > sb
	}
	if a.snap.LastCommit != b.snap.LastCommit {
		return a.snap.LastCommit > b.snap.LastCommit
	}
	return strings.ToLower(a.project.Name) < strings.ToLower(b.project.Name)
}

// summary cuenta los estados de todos los proyectos para la barra (R6).
func (m *Model) summary() (total, dirty, ahead, behind int) {
	for _, p := range m.projects {
		snap := m.states[p.Path]
		total++
		switch snap.State(p.HasRepo) {
		case gitstatus.StateDirty, gitstatus.StateDiverged:
			dirty++
		}
		if snap.Status.Ahead > 0 {
			ahead++
		}
		if snap.Status.Behind > 0 {
			behind++
		}
	}
	return total, dirty, ahead, behind
}

// ---- render de celdas: cada celda devuelve texto plano + estilo;
// el render final hace pad(texto) y luego aplica el estilo, así la
// alineación nunca se rompe por códigos ANSI.

// renderRow compone una línea de la tabla con cursor opcional (R6).
func (m *Model) renderRow(r row, selected bool) string {
	name, nameStyle := m.nameCell(r)
	group, groupStyle := m.groupCell(r)
	branch, branchStyle := m.branchCell(r)
	state, stateStyle := m.stateCell(r)
	upDown, upDownStyle := m.upDownCell(r)
	activity, activityStyle := activityCell(r)
	fetch, fetchStyle := m.fetchCell(r.project.Path)

	cells := []struct {
		text  string
		style lipglossStyle
		width int
	}{
		{name, nameStyle, colName},
		{group, groupStyle, colGroup},
		{branch, branchStyle, colBranch},
		{state, stateStyle, colState},
		{upDown, upDownStyle, colUpDown},
		{activity, activityStyle, colActivity},
		{fetch, fetchStyle, colFetch},
	}

	line := ""
	for _, c := range cells {
		line += c.style.Render(pad(truncate(c.text, c.width), c.width))
	}
	if selected {
		return styleCursor.Render("▸ ") + line
	}
	return "  " + line
}

func (m *Model) nameCell(r row) (string, lipglossStyle) {
	name := r.project.Name
	if r.project.IsWorktree {
		name += " [wt]" // S3.2: tag de worktree
	}
	if r.project.MarkerErr != "" {
		return name, styleWarn // S4.3: marcador malformado visible
	}
	return name, styleSel
}

func (m *Model) groupCell(r row) (string, lipglossStyle) {
	return orDash(r.project.Group), styleDim
}

func (m *Model) branchCell(r row) (string, lipglossStyle) {
	switch {
	case r.state == gitstatus.StateNoRepo || r.snap.Status.Branch == "":
		return "-", styleDim
	case r.snap.Status.Detached:
		return r.snap.Status.Branch + " (detached)", m.styleFor(r) // S5.5
	default:
		return r.snap.Status.Branch, m.styleFor(r)
	}
}

// stateCell compone la columna de estado (dirty/untracked/errores).
func (m *Model) stateCell(r row) (string, lipglossStyle) {
	switch r.state {
	case gitstatus.StateError:
		return "⚠ error", styleError
	case gitstatus.StateNoRepo:
		return "∅ no repo", styleDim
	case gitstatus.StateNoUpstream:
		return "no-upstream", styleWarn
	case gitstatus.StateDetached:
		return "detached", styleWarn
	case gitstatus.StateClean:
		return "✓", styleClean
	case gitstatus.StateDiverged:
		return "⇅ " + dirtyTail(r), styleDiverged
	case gitstatus.StateDirty:
		return "● " + dirtyTail(r), styleDirty
	default: // ahead/behind
		return "·", styleClean
	}
}

// dirtyTail compone "N ?M" para tracked/untracked (S5.3).
func dirtyTail(r row) string {
	s := r.snap.Status
	out := ""
	if s.TrackedChanges > 0 {
		out += fmt.Sprintf("%d", s.TrackedChanges)
	}
	if s.Untracked > 0 {
		out += fmt.Sprintf(" ?%d", s.Untracked)
	}
	return strings.TrimSpace(out)
}

// upDownCell compone ↑ahead/↓behind (S5.2) o — sin upstream.
func (m *Model) upDownCell(r row) (string, lipglossStyle) {
	s := r.snap.Status
	if r.state == gitstatus.StateNoRepo {
		return "", styleClean
	}
	if !s.HasUpstream {
		return "—", styleDim
	}
	switch {
	case s.Ahead > 0 && s.Behind > 0:
		return fmt.Sprintf("↑%d↓%d", s.Ahead, s.Behind), styleDiverged
	case s.Ahead > 0:
		return fmt.Sprintf("↑%d", s.Ahead), styleAhead
	case s.Behind > 0:
		return fmt.Sprintf("↓%d", s.Behind), styleBehind
	default:
		return "", styleClean
	}
}

// activityCell devuelve la fecha relativa del último commit (R6).
func activityCell(r row) (string, lipglossStyle) {
	return relativeTime(r.snap.LastCommit), styleDim
}

// fetchCell compone el estado del fetch de la fila (S6.3).
func (m *Model) fetchCell(path string) (string, lipglossStyle) {
	switch m.fetchStates[path] {
	case "fetching":
		return "⟳ fetch", styleFetchRun
	case "ok":
		return "✓", styleFetchOk
	case "failed":
		return "✗ fetch", styleFetchBad
	default:
		return "", styleClean
	}
}

// styleFor elige el estilo según el estado derivado.
func (m *Model) styleFor(r row) lipglossStyle {
	switch r.state {
	case gitstatus.StateError:
		return styleError
	case gitstatus.StateNoRepo:
		return styleDim
	case gitstatus.StateNoUpstream:
		return styleWarn
	case gitstatus.StateDetached:
		return styleWarn
	case gitstatus.StateDiverged:
		return styleDiverged
	case gitstatus.StateDirty:
		return styleDirty
	case gitstatus.StateAhead:
		return styleAhead
	case gitstatus.StateBehind:
		return styleBehind
	default:
		return styleClean
	}
}

// ---- helpers de texto ----

func truncate(s string, w int) string {
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	runes := []rune(s)
	return string(runes[:w-1]) + "…"
}

// pad rellena a la derecha midiendo runes (solo texto plano, sin ANSI).
func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// stripANSI quita secuencias ANSI para comparar contenido en tests.
func stripANSI(s string) string {
	var out strings.Builder
	inSeq := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inSeq = true
		case inSeq && (r == 'm' || r == 'K'):
			inSeq = false
		case !inSeq:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// relativeTime formatea un epoch como "3h", "2d" (R6).
func relativeTime(epoch int64) string {
	if epoch <= 0 {
		return "-"
	}
	d := time.Since(time.Unix(epoch, 0))
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}
