// Composición de las secciones bordadas del dashboard (stats, filtro, tabla o
// detalle, keybinds). Cada sección usa el ancho exterior de la terminal.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gitdash/internal/tui/bordered"
)

// section envuelve el contenido en una caja bordada con título embebido.
func (m Model) section(title, content string) string {
	if title != "" {
		title = " " + title + " "
	}
	return bordered.RenderWithTitle(bordered.Rounded(), borderColor, title, content, m.width)
}

// layout calcula el reparto de alto para el estado actual del modelo.
func (m Model) layout() layout {
	return computeLayout(m.height, m.searchActive || m.search != "", m.detailOpen)
}

// compose apila las secciones visibles: stats, filtro, la sección central
// (tabla o detalle) y keybinds, sin líneas en blanco entre ellas.
func (m Model) compose(lay layout, middle string) string {
	sections := make([]string, 0, 4)
	if lay.showStats {
		sections = append(sections, m.statsSection())
	}
	if lay.showFilter {
		sections = append(sections, m.filterSection())
	}
	sections = append(sections, middle)
	if lay.showKeybinds {
		sections = append(sections, m.keybindsSection(lay.hintLines))
	}
	return strings.Join(sections, "\n")
}

// statsSection resume el estado global. El indicador compacto de actividad va
// primero para que sobreviva al recorte en anchos estrechos; después el
// resumen y, por último, los nombres de las acciones en curso.
func (m Model) statsSection() string {
	parts := make([]string, 0, 4)
	if activity := m.activityIndicator(); activity != "" {
		parts = append(parts, activity)
	}
	total, dirty, ahead, behind := m.summary()
	summary := fmt.Sprintf("%d repos · %d dirty · %d ahead · %d behind", total, dirty, ahead, behind)
	if m.onlyDirty {
		summary += " [dirty]"
	}
	parts = append(parts, styleBar.Render(summary))
	for path, kind := range m.running {
		if kind == "pull" || kind == "push" || kind == "sync" {
			parts = append(parts, styleFetchRun.Render(fmt.Sprintf("%s %s…", kind, m.nameOf(path))))
		}
	}
	return m.section("gitdash", strings.Join(parts, "  "))
}

// activityIndicator es el indicador compacto de "algo en curso": scan, fetch o
// una acción pull/push/sync.
func (m Model) activityIndicator() string {
	switch {
	case m.scanning:
		return m.spinner.View() + " scanning"
	case m.fetchingAll():
		return m.spinner.View() + " fetching"
	case m.runningAction():
		return m.spinner.View() + " working"
	default:
		return ""
	}
}

// runningAction reporta si hay algún pull/push/sync en curso.
func (m Model) runningAction() bool {
	for _, kind := range m.running {
		if kind == "pull" || kind == "push" || kind == "sync" {
			return true
		}
	}
	return false
}

// filterSection muestra el input de búsqueda en vivo o el filtro confirmado.
func (m Model) filterSection() string {
	var content string
	if m.searchActive {
		in := m.searchInput
		in.Placeholder = ""
		content = styleWarn.Render("[") + in.View()
		if in.Value() == "" {
			content += styleDim.Render(searchPlaceholder)
		}
		content += styleWarn.Render("]")
	} else {
		content = styleWarn.Render("[/" + m.search + "]")
	}
	return m.section("filter", content)
}

// tableSection dibuja la cabecera y las filas visibles con scroll, rellenando
// hasta el alto del presupuesto para mantener estable el alto de la caja.
func (m Model) tableSection(bodyLines int) string {
	entries := m.entries()
	m.syncOffset(len(entries), bodyLines)

	var rows []string
	if len(entries) == 0 && !m.scanning {
		hint := "no repositories — create a .gitdash.toml in your projects"
		if m.search != "" || m.onlyDirty {
			hint = "no repositories match the current filter"
		}
		rows = append(rows, "  "+hint)
	} else {
		for i := m.offset; i < min(len(entries), m.offset+bodyLines); i++ {
			rows = append(rows, m.renderEntry(entries[i], i == m.cursor))
		}
	}
	for len(rows) < bodyLines {
		rows = append(rows, "")
	}

	header := "  " + pad("NAME", colName) + pad("BRANCH", colBranch) +
		pad("Work Tree", colWT) + pad("↑↓up", colUpDown) + pad("SYNC", colSync) +
		pad("ACTIVITY", colActivity) + pad("FETCH", colFetch)
	return m.section("repos", styleHint.Render(header)+"\n"+strings.Join(rows, "\n"))
}

// keybindsSection muestra hasta hintLines líneas de hints.
func (m Model) keybindsSection(hintLines int) string {
	lines := m.cfg.HintBarLines()
	if hintLines < len(lines) {
		lines = lines[:max(0, hintLines)]
	}
	rendered := make([]string, 0, len(lines))
	for _, l := range lines {
		rendered = append(rendered, styleHint.Render(l))
	}
	return m.section("keybinds", strings.Join(rendered, "\n"))
}

// detailSection envuelve el contenido del detalle, recortado a bodyLines.
func (m Model) detailSection(title, content string, bodyLines int) string {
	return m.section(title, clipLines(content, bodyLines))
}

// detailTitle compone el título del borde para un repo (nombre + grupo y
// marca de worktree).
func detailTitle(r row) string {
	title := r.project.Name
	if g := groupLabel(r.project); g != "" {
		title += " · " + g
	}
	if r.project.IsWorktree {
		title += " [worktree]"
	}
	return title
}

// worktreeTitle compone el título del borde para una sub-fila de worktree.
func worktreeTitle(e tableEntry) string {
	return filepath.Base(e.wt.Path) + " [worktree]"
}

// clipLines limita el contenido a n líneas (el detalle puede exceder el alto).
func clipLines(content string, n int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
