// Composición de las secciones bordadas del dashboard (stats, filtro, tabla o
// detalle, keybinds). Cada sección usa el ancho exterior de la terminal.
package tui

import (
	"fmt"
	"path/filepath"
	"sort"
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

// layout calcula el reparto de alto para el estado actual del modelo. Con una
// confirmación de borrado o un selector de pull armados se fuerza la
// visibilidad de stats para que el prompt no desaparezca en terminales bajas.
func (m Model) layout() layout {
	armed := m.armed != nil || m.pullArmed != nil
	return computeLayout(m.height, m.searchActive || m.search != "", m.detailOpen, len(m.cfg.HintBarLines()), armed)
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
// resumen. El propio indicador ya nombra las acciones en curso, así que no se
// duplican aparte.
func (m Model) statsSection() string {
	parts := make([]string, 0, 3)
	if activity := m.activityIndicator(); activity != "" {
		parts = append(parts, activity)
	}
	if m.armed != nil {
		// Aviso persistente de confirmación: sobrevive hasta la segunda
		// pulsación o la cancelación (los toasts expiran a los 3 s).
		parts = append(parts, styleWarn.Render(m.removePrompt()))
	}
	if m.pullArmed != nil {
		// El selector de variante se anuncia en el mismo sitio y con el mismo
		// carácter: los dos son estados de "una tecla más". La barra se apila
		// sobre el resumen para que el prompt sea la primera línea, no la segunda.
		parts = append([]string{styleWarn.Render(m.pullPrompt())}, parts...)
	}
	total, dirty, ahead, behind := m.summary()
	summary := fmt.Sprintf("%d repos · %d dirty · %d ahead · %d behind", total, dirty, ahead, behind)
	if m.onlyDirty {
		summary += " [dirty]"
	}
	parts = append(parts, styleBar.Render(summary))
	return m.section("gitdash", strings.Join(parts, "  "))
}

// activityIndicator es el indicador compacto de "algo en curso": scan, fetch o
// una acción pull/push (a la que añade el repo cuando hay sitio).
func (m Model) activityIndicator() string {
	switch {
	case m.scanning:
		return m.spinner.View() + " scanning"
	case m.fetchingAll():
		return m.spinner.View() + " fetching"
	}
	running := m.runningActions()
	if len(running) == 0 {
		return ""
	}
	label := running[0]
	if extra := len(running) - 1; extra > 0 {
		label = fmt.Sprintf("%s +%d", label, extra)
	}
	return m.spinner.View() + " " + label
}

// runningActions lista las acciones en curso como "kind nombre…", ordenadas por
// path para que el render sea determinista (m.running es un mapa).
func (m Model) runningActions() []string {
	paths := make([]string, 0, len(m.running))
	for path, kind := range m.running {
		if IsPullKind(kind) || kind == "push" || kind == "worktree_remove" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, fmt.Sprintf("%s %s…", m.running[path], m.nameOf(path)))
	}
	return out
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

	header := "  " + headerColumns(m.width)
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
