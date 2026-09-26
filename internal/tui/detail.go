// Vista de detalle de un repo.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gitdash/internal/gitstatus"
)

// detailHeadLines son las líneas fijas de la ficha antes de las listas: path,
// hueco, branch, upstream, state y sync. Se reservan siempre, así que las
// listas (worktrees, ficheros, commits) se reparten el resto del alto.
const detailHeadLines = 6

// minListBlockLines es lo que consume una lista antes de enseñar un solo
// elemento: el hueco que la separa de la ficha y su cabecera. Por debajo no se
// pinta la lista (ni la cabecera, que sin elementos no dice nada).
const minListBlockLines = 2

// listBudget reparte las líneas disponibles (avail) entre una lista de n
// elementos, contando el hueco y la cabecera. Devuelve cuántos se pintan y si
// hay que avisar de los que quedan fuera.
//
// El aviso se reserva una línea cuando la lista no cabe entera: sin él, un
// corte al final de la caja parecería que la lista se acababa ahí. Cuando solo
// hay sitio para un elemento, el elemento gana y el aviso se cae — la cabecera
// de la lista ya dice cuántos hay.
func listBudget(avail, n int) (shown int, rest bool) {
	if avail < minListBlockLines {
		return 0, false
	}
	room := avail - minListBlockLines // hueco + cabecera
	if n <= room {
		return n, false
	}
	if room < 1 {
		return 0, false
	}
	return room - 1, true
}

// asOrDash devuelva el texto o "-" si vacío.
func asOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// detailView es el contexto de pintado de una ficha: cuántas líneas hay y si es
// la vista completa (enter) o el panel de preview. El presupuesto interno
// (cuántos worktrees/ficheros/tails se listan) sale de rows y no de la altura
// de la terminal: en un panel de 8 líneas, un tope calculado con el alto
// completo pintaría una lista entera que luego se recorta sin decir cuántas
// filas faltaron.
type detailView struct {
	rows int
	full bool
}

// detailFull es la ficha a pantalla completa: su presupuesto son las líneas de
// contenido que le da el layout, no la terminal entera (si no, las listas se
// cuentan como si cupieran cuando luego las recorta la caja).
func detailFull(rows int) detailView { return detailView{rows: rows, full: true} }

// detailPreview es la ficha del panel de preview, que vive en rows líneas.
func detailPreview(rows int) detailView { return detailView{rows: rows} }

// detailFooter es la ayuda al pie de la ficha. Cambia con el modo porque lo que
// hay que leer es distinto: en la vista completa `esc` es la salida; en el
// panel, `enter` es lo que amplía la ficha. Con el input de `!` abierto lo
// relevante es qué hace enter, y eso vale en los dos modos.
func (v detailView) detailFooter(cmdOpen bool) string {
	if cmdOpen {
		return "enter run ($SHELL -c en el repo) · enter vacío = shell interactiva · esc cancel"
	}
	if v.full {
		return "esc back · g lazygit · ! cmd"
	}
	return "enter detail · g lazygit · ! cmd"
}

// renderDetail compone el panel de detalle del repo seleccionado con datos
// vivos del snapshot. El título (nombre, grupo, marca de worktree) NO se pinta
// aquí: lo lleva el borde de la sección, que es donde está en las dos vistas.
func (m *Model) renderDetail(r row, v detailView) string {
	var b strings.Builder

	p := r.project
	rows := max(1, v.rows)
	b.WriteString(styleHint.Render(truncate(p.Path, max(20, m.width-4))) + "\n\n")

	key := styleDetailKey.Render

	branch := r.snap.Status.Branch
	if r.snap.Status.Detached {
		branch += " (detached)"
	}
	if branch == "" {
		branch = "-"
	}
	upstream := orDash(r.snap.Status.Upstream)
	if !r.snap.Status.HasUpstream {
		upstream = "— (no upstream)"
	}
	b.WriteString(key("branch  ") + branch + "\n")
	b.WriteString(key("upstream") + " " + upstream + "\n")
	// Working tree + deriva vs upstream. El detalle SÍ es
	// verboso: "clean" explícito en vez de celda vacía.
	wtText, wtStyle := m.wtCell(r)
	if wtText == "" {
		wtText, wtStyle = "clean", styleClean
	}
	stateLine := wtStyle.Render(wtText)
	if upDownText, upDownStyle := m.upDownCell(r); upDownText != "" {
		stateLine += " " + upDownStyle.Render(upDownText)
	}
	b.WriteString(key("state   ") + stateLine + "\n")

	// sync branch vs HEAD: la rama resuelta siempre
	// visible, con su desviación o el motivo de la falta.
	syncLine := "— (sin sync branch)"
	switch {
	case r.snap.SyncBranch == "":
	case !r.snap.SyncKnown:
		syncLine = r.snap.SyncBranch + " (ref missing)"
	case r.snap.SyncBehind > 0:
		syncLine = r.snap.SyncBranch + fmt.Sprintf(" (↓%d)", r.snap.SyncBehind)
	default:
		syncLine = r.snap.SyncBranch + " (ok)"
	}
	b.WriteString(key("sync    ") + syncLine + "\n")

	// Cuántas filas de listas caben en lo que queda tras la cabecera de estado.
	// Es lo que permite que el panel enseñe "… N más" en vez de cortar la lista
	// a media: el presupuesto se reparte entre las listas que sí quepan.
	avail := max(0, rows-detailHeadLines)

	// worktrees del repo: rama, sha y ruta (relativa al
	// repo cuando sea posible, absoluta en caso contrario).
	if n := len(r.snap.Worktrees); n > 0 && avail >= minListBlockLines {
		shown, rest := listBudget(avail, n)
		b.WriteString("\n" + key(fmt.Sprintf("worktrees (%d)", n)) + "\n")
		for _, wt := range r.snap.Worktrees[:shown] {
			rel, err := filepath.Rel(r.project.Path, wt.Path)
			if err != nil {
				rel = wt.Path
			}
			b.WriteString("  " + styleDim.Render(pad(wt.Branch, 24)) +
				styleWarn.Render(pad(asOrDash(wt.Head), 12)) +
				truncate(rel, max(20, m.width-16)) + "\n")
		}
		if rest {
			b.WriteString(styleHint.Render(fmt.Sprintf("  … %d más", n-shown)) + "\n")
		}
	}

	if p.MarkerErr != "" {
		b.WriteString("\n" + styleError.Render("marker: "+p.MarkerErr) + "\n")
	}
	if r.snap.Err != "" {
		b.WriteString("\n" + styleError.Render("git: "+r.snap.Err) + "\n")
	}

	if n := len(r.snap.Files); n > 0 && avail >= minListBlockLines {
		shown, rest := listBudget(avail, n)
		b.WriteString("\n" + key(fmt.Sprintf("files (%d)", n)) + "\n")
		for _, f := range r.snap.Files[:shown] {
			b.WriteString("  " + styleWarn.Render(pad(f.Code, 3)) + truncate(f.Path, max(20, m.width-8)) + "\n")
		}
		if rest {
			b.WriteString(styleHint.Render(fmt.Sprintf("  … %d más", n-shown)) + "\n")
		}
	}

	if n := len(r.snap.Commits); n > 0 && avail >= minListBlockLines {
		shown, _ := listBudget(avail, n)
		b.WriteString("\n" + key("commits") + "\n")
		for _, c := range r.snap.Commits[:shown] {
			fmt.Fprintf(&b, "  %s %s %s\n",
				styleDim.Render(pad(c.Sha, 8)),
				pad(relativeTime(c.When), 6),
				truncate(c.Subject, max(20, m.width-24)),
			)
		}
	}

	if act, ok := m.lastAction[p.Path]; ok {
		verdict := styleClean.Render("ok")
		if act.err != "" {
			verdict = styleError.Render("failed")
		}
		b.WriteString("\n" + key("last "+pullVariantLabel(act.kind)) + " " + verdict + "\n")
		// El argv resuelto es lo único que revela la política real: con
		// `p` sin flags, lo que reconcilió fue el gitconfig, no gitdash.
		if act.cmd != "" {
			b.WriteString(styleHint.Render(indent(truncate(act.cmd, max(20, m.width-30)), "  ")) + "\n")
		}
		if tail := actionTail(act.output, max(3, rows-20)); tail != "" {
			b.WriteString(styleHint.Render(indent(tail, "  ")) + "\n")
		}
	}

	if cr, ok := m.lastCmd[p.Path]; ok {
		verdict := styleClean.Render("exit 0")
		if cr.exit != "0" {
			verdict = styleError.Render("exit " + cr.exit)
		}
		b.WriteString("\n" + key("$ "+truncate(cr.command, max(20, m.width-30))) + " " + verdict + "\n")
		if tail := actionTail(cr.output, max(3, rows-12)); tail != "" {
			b.WriteString(styleHint.Render(indent(tail, "  ")) + "\n")
		}
	}

	if m.cmdOpen {
		b.WriteString("\n" + styleDetailKey.Render(m.cmdInput.Prompt) +
			m.cmdInput.View() + "\n")
	}
	b.WriteString("\n" + styleHint.Render(v.detailFooter(m.cmdOpen)))
	return b.String()
}

// renderWorktreeDetail compone el detalle de una sub-fila de worktree.
// Si el worktree fue descubierto con marcador y tiene snapshot
// vivo, se delega al detalle completo; si no, panel mínimo con los datos que
// trae `worktree list` (path/rama/head) SIN inventar estado git derivado.
func (m *Model) renderWorktreeDetail(e tableEntry, v detailView) string {
	if p, ok := m.discoveredByPath(e.wt.Path); ok {
		if snap, ok := m.states[p.Path]; ok {
			return m.renderDetail(row{project: p, snap: snap, state: snap.State(p.HasRepo)}, v)
		}
	}
	return m.renderWorktreeMinimal(e.wt, e.parent, v)
}

// renderWorktreeMinimal es el panel de detalle mínimo de un worktree sin
// snapshot propio: path, rama (o `(detached)`), head. No muestra
// dirty/ahead/behind/sync. El título lo lleva el borde de la sección, igual que
// en la ficha completa.
func (m *Model) renderWorktreeMinimal(wt gitstatus.Worktree, parent string, v detailView) string {
	var b strings.Builder

	b.WriteString(styleHint.Render(truncate(wt.Path, max(20, m.width-4))) + "\n\n")

	key := styleDetailKey.Render
	branch := wt.Branch
	if branch == "" {
		branch = "(detached)"
	}
	b.WriteString(key("branch  ") + branch + "\n")
	b.WriteString(key("head    ") + asOrDash(wt.Head) + "\n")
	if parent != "" {
		b.WriteString(key("repo    ") + filepath.Base(parent) + "\n")
	}

	b.WriteString("\n" + styleHint.Render(v.detailFooter(false)))
	return b.String()
}

// actionTail recorta la salida de una acción a sus últimas n líneas.
func actionTail(out string, n int) string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// indent antepone prefix a cada línea.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
