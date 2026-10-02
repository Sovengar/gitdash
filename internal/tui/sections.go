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

// layout calcula el reparto de alto para el estado actual del modelo. Con un
// aviso en keybinds (selector de pull, confirmación de borrado, leyenda del
// panel del log o del overlay de PR) lo que se fuerza es la visibilidad de esa
// sección: si se degradara, la app quedaría esperando una tecla sin decir
// cuáles.
func (m Model) layout() layout {
	// El overlay de PR vive EN el cuerpo (es el dashboard lo que se sustituye),
	// así que le pasa su mínimo: el layout no debe robarle altura a un panel de
	// preview que no se va a dibujar debajo.
	formMin := 0
	if m.pr != nil {
		formMin = prMinBodyLines()
	}
	lay := computeLayout(m.height, m.searchActive || m.search != "", m.keybindsLines(), m.promptLine() != "", formMin)
	if m.logOpen || m.pr != nil {
		// El log y el formulario SUSTITUYEN a la tabla y la ficha desaparece.
		// Lo que hay que conservar es el total de líneas de la terminal, así
		// que al alto del cuerpo se le devuelve lo que la tabla y la ficha se
		// dejaron:
		//
		//   - la cabecera de columnas (1): la sección del log y la del
		//     formulario son sus 2 bordes + bodyLines, y la de la tabla 2
		//     bordes + cabecera + bodyLines. Los bordes son los mismos, así
		//     que de la tabla solo se libera la línea de cabecera.
		//   - la ficha entera (previewChrome + previewLines), si estaba.
		//
		// Sin esto el panel mide 3 líneas menos que la terminal en cuanto la
		// ficha se dibuja, y los keybinds suben.
		freed := 1
		if lay.previewLines > 0 {
			freed += previewChrome + lay.previewLines
		}
		lay.bodyLines += freed
		lay.previewLines = 0
	}
	return lay
}

// keybindsLines dice cuántas líneas de contenido pintará la sección de
// keybinds: las hints del config (con tope) o, si hay un aviso, solo la del
// prompt. El presupuesto de alto se deriva de aquí para que la caja nunca
// mida más que lo que lleva dentro.
func (m Model) keybindsLines() int {
	if m.promptLine() != "" {
		return 1
	}
	return min(defaultHintLines, max(0, len(m.cfg.HintBarLines())))
}

// promptLine devuelve el aviso persistente de la sección de keybinds: el
// selector de pull, la confirmación de borrado de worktree, la leyenda del panel
// del log o la del overlay de PR. Solo puede haber uno a la vez (la segunda
// pulsación desarma el anterior y abrir un overlay suelta los armados), pero si
// se solaparan mandan los armados y el formulario sobre la leyenda. Vacío si no
// hay nada que anunciar.
func (m Model) promptLine() string {
	// La precedencia de los estados armados es el requisito (gana el mas
	// especifico), y asi queda en if en vez de en el orden de los case.
	switch {
	case m.armed != nil:
		return m.removePrompt()
	case m.pullArmed != nil:
		return m.pullPrompt()
	case m.visualArmed != nil:
		return m.visualPrompt()
	case m.pr != nil:
		return m.prPrompt()
	case m.logOpen:
		return m.logLegend()
	}
	return ""
}

// compose apila las secciones visibles: stats, filtro, la tabla, el panel con
// la ficha del repo bajo el cursor y keybinds, sin líneas en blanco entre ellas.
// preview es "" cuando el panel no tiene alto (terminal baja).
func (m Model) compose(lay layout, middle, preview string) string {
	sections := make([]string, 0, 5)
	if lay.showStats {
		sections = append(sections, m.statsSection())
	}
	if lay.showFilter {
		sections = append(sections, m.filterSection())
	}
	sections = append(sections, middle)
	if preview != "" {
		sections = append(sections, preview)
	}
	if lay.showKeybinds {
		sections = append(sections, m.keybindsSection(lay.hintLines))
	}
	return strings.Join(sections, "\n")
}

// statsSection resume el estado global. El indicador compacto de actividad va
// primero para que sobreviva al recorte en anchos estrechos; después el
// resumen. El propio indicador ya nombra las acciones en curso, así que no se
// duplican aparte. Los avisos armados NO van aquí: viven en keybinds, que es
// donde el usuario ya busca las teclas.
func (m Model) statsSection() string {
	parts := make([]string, 0, 2)
	if activity := m.activityIndicator(); activity != "" {
		parts = append(parts, activity)
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
// hasta el alto del presupuesto para mantener estable el alto de la caja. Las
// entradas se reciben ya calculadas: el panel de preview necesita las mismas y
// recomponerlas aquí duplicaría el Arrange en cada render.
func (m Model) tableSection(bodyLines int, entries []tableEntry) string {
	m.syncOffset(len(entries), bodyLines)

	var rows []string
	if len(entries) == 0 && !m.scanning {
		rows = append(rows, "  "+m.emptyTableHint())
	} else {
		for i := m.offset; i < min(len(entries), m.offset+bodyLines); i++ {
			rows = append(rows, m.renderEntry(entries[i], i == m.cursor))
		}
	}
	// rellenaHasta en vez de `for len(rows) < bodyLines`: la caja mide lo que
	// dice el layout, y comparar-en-el-bucle hace que el mutante sea un cuelgue.
	rows = rellenaHasta(rows, bodyLines)

	header := "  " + headerColumns(m.width)
	return m.section("repos", styleHint.Render(header)+"\n"+strings.Join(rows, "\n"))
}

// previewSection es la ficha del repo bajo el cursor, sin tener que abrir el
// detalle: va entre la tabla y los keybinds, con el reparto de alto que fija
// computeLayout (prdash hace lo mismo con su panel de ítem).
//
// Pinta exactamente la misma ficha que `enter` —mismo render, mismo título en el
// borde— recortada a su alto y rellenada con líneas vacías. Rellenar importa:
// el alto lo decide el layout, no lo larga que sea la ficha, así que la caja no
// puede encogerse al mover el cursor de un repo limpio a uno con 30 ficheros.
//
// Tres casos, porque el cursor no solo puede estar sobre un repo: un header de
// grupo no tiene ficha (muestra el agregado del grupo) y una tabla vacía no
// tiene nada que enseñar.
func (m *Model) previewSection(lay layout, entries []tableEntry) string {
	if lay.previewLines <= 0 {
		return ""
	}
	rows := lay.previewLines
	e, ok := entryAt(entries, m.cursor)
	var title, content string
	switch {
	case !ok:
		title, content = "repos", m.previewEmpty()
	case e.kind == kindRepo:
		title, content = detailTitle(e.r), m.renderDetail(e.r, rows)
	case e.kind == kindWorktree:
		title, content = worktreeTitle(e), m.renderWorktreeDetail(e, rows)
	default: // header primario o secundario
		title, content = e.group, m.renderGroupSummary(e, rows)
	}
	return m.section(title, fitLines(content, rows))
}

// previewEmpty es lo que dice el panel cuando no hay fila bajo el cursor: la
// tabla vacía por filtro/dirty no es un fallo del panel, y su texto es el mismo
// que ya se ve en la tabla de arriba.
func (m Model) previewEmpty() string {
	return styleDim.Render("  " + m.emptyTableHint())
}

// emptyTableHint explica por qué no hay repos: sin filtro es que no hay
// marcadores, y con filtro es que el filtro no casa con ninguno. Lo comparten la
// tabla y el panel de preview (no pueden contradecirse: se leen a la vez).
func (m Model) emptyTableHint() string {
	if m.search != "" || m.onlyDirty {
		return "no repositories match the current filter"
	}
	return "no repositories — create a .gitdash.toml in your projects"
}

// keybindsSection muestra hasta hintLines líneas de hints o, si hay un aviso
// (un estado armado o el panel del log), el prompt en su lugar. El prompt
// sustituye a las hints (no se apila): compite por el mismo espacio de lectura
// —"qué hago ahora"—, y las hints vuelven intactas en cuanto se resuelve la
// pulsación.
func (m Model) keybindsSection(hintLines int) string {
	lines := m.cfg.HintBarLines()
	if prompt := m.promptLine(); prompt != "" {
		lines = []string{styleWarn.Render(prompt)}
	}
	// El presupuesto manda: si no caben todas las hints, se cortan (recortar las
	// que caben justas == no tocar nada) y un presupuesto negativo deja la caja
	// vacía en vez de un panic por slice.
	lines = lines[:max(0, min(hintLines, len(lines)))]
	rendered := make([]string, 0, len(lines))
	for _, l := range lines {
		rendered = append(rendered, styleHint.Render(l))
	}
	return m.section("keybinds", strings.Join(rendered, "\n"))
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

// renderGroupSummary es la ficha del header de grupo bajo el cursor. El header
// ya dice cuántos repos tiene; lo que no dice es cómo están, y eso es lo que
// hace falta al pasar el cursor por encima para decidir si hay que abrir el
// grupo. Solo se pintan los estados que hay: un grupo limpio no merece cuatro
// líneas a cero (la tabla es quieta por el mismo motivo).
func (m *Model) renderGroupSummary(e tableEntry, rows int) string {
	st := m.groupStats(e.group)
	key := styleDetailKey.Render

	var b strings.Builder
	b.WriteString(key("repos    ") + fmt.Sprint(st.repos) + "\n")
	if st.errors > 0 {
		b.WriteString(key("errors   ") + styleError.Render(fmt.Sprint(st.errors)) + "\n")
	}
	if st.dirty > 0 {
		b.WriteString(key("dirty    ") + styleDirty.Render(fmt.Sprint(st.dirty)) + "\n")
	}
	if st.ahead > 0 {
		b.WriteString(key("ahead    ") + styleAhead.Render(fmt.Sprint(st.ahead)) + "\n")
	}
	if st.behind > 0 {
		b.WriteString(key("behind   ") + styleBehind.Render(fmt.Sprint(st.behind)) + "\n")
	}
	if st.worktrees > 0 {
		b.WriteString(key("wt       ") + fmt.Sprint(st.worktrees) + "\n")
	}
	return m.fichaTail(b.String(), rows)
}

// fitLines ajusta el contenido a exactamente n líneas: recorta por arriba lo que
// sobra y rellena con líneas vacías lo que falta. La caja mide lo que dice el
// layout, no lo que mida la ficha.
func fitLines(content string, n int) string {
	// rellenaHasta en vez de un `for len(lines) < n`: ver el comentario de la
	// función. El efecto es el mismo y el mutante deja de ser un cuelgue.
	return strings.Join(rellenaHasta(strings.Split(clipTo(content, n), "\n"), n), "\n")
}

// clipTo recorta el contenido a n líneas sin rellenar. Es lo que se usa cuando
// lo que viene detrás tiene que estar SIEMPRE visible (el input de `!`).
func clipTo(content string, n int) string {
	lines := strings.Split(content, "\n")
	// Recortar al número exacto es identidad, así que el min() sustituye a la
	// guarda: aquí solo se puede recortar, nunca rellenar.
	return strings.Join(lines[:max(0, min(n, len(lines)))], "\n")
}

// rellenaHasta devuelve las líneas que faltan para llegar a n, sin tocar las que
// ya hay.
//
// Existe para que los tres bucles de relleno del TUI (el alto de la caja, el de
// la sección de repos y el del panel del log) NO comparen `len(...) < n` dentro
// de la condición. Comparar y appendar en el mismo bucle es un patrón del que
// la mutación testing se lleva por delante: `<` invertido en `>=` convierte el
// relleno —que debería terminar tras UNA vuelta— en un bucle infinito, porque la
// condición se cumple antes y después de appendar. El mutante entonces no es un
// bug lento que un test pueda cazar: es un cuelgue, y su único final es el
// timeout por mutante, que lo reporta como TIMED OUT (un estado que ni entra en
// mutants_total ni lo ve el gate de CI).
//
// Separar "cuántas faltan" de "rellenar" hace que la cuenta sea un entero y el
// bucle sea un for normal: `<` invertido deja de ser un cuelgue y pasa a ser el
// bug que es —devolver el número de líneas equivocado—, que sí tiene test.
func rellenaHasta(lines []string, n int) []string {
	for falta := n - len(lines); falta > 0; falta-- {
		lines = append(lines, "")
	}
	return lines
}
