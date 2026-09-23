// Vista de detalle de un repo (spec 0001 R10; 0002 R14/R15).
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gitdash/internal/gitstatus"
)

// asOrDash devuelva el texto o "-" si vacío.
func asOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// renderDetail compone el panel de detalle del repo seleccionado con datos
// vivos del snapshot (S10.1, S10.2).
func (m *Model) renderDetail(r row) string {
	var b strings.Builder

	p := r.project
	title := p.Name
	if g := groupLabel(p); g != "" {
		title += "  ·  " + g
	}
	if p.IsWorktree {
		title += "  [worktree]"
	}
	b.WriteString(styleDetailTitle.Render(title) + "\n")
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
	// 0004 R26/R27: working tree + deriva vs upstream. El detalle SÍ es
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

	// sync branch vs HEAD (0002 R14; 0004 R23): la rama resuelta siempre
	// visible, con su desviación o el motivo de la falta.
	syncLine := "— (sin sync branch)"
	switch {
	case r.snap.SyncBranch == "":
	case !r.snap.SyncKnown:
		syncLine = r.snap.SyncBranch + " (ref missing)" // S23.4
	case r.snap.SyncBehind > 0:
		syncLine = r.snap.SyncBranch + fmt.Sprintf(" (↓%d)", r.snap.SyncBehind)
	default:
		syncLine = r.snap.SyncBranch + " (ok)"
	}
	b.WriteString(key("sync    ") + syncLine + "\n")

	// worktrees del repo (0002 R15/S15.2): rama, sha y ruta (relativa al
	// repo cuando sea posible, absoluta en caso contrario).
	if n := len(r.snap.Worktrees); n > 0 {
		b.WriteString("\n" + key(fmt.Sprintf("worktrees (%d)", n)) + "\n")
		maxWTs := max(1, m.height-18)
		for i, wt := range r.snap.Worktrees {
			if i >= maxWTs {
				b.WriteString(styleHint.Render(fmt.Sprintf("  … %d más", n-maxWTs)) + "\n")
				break
			}
			rel, err := filepath.Rel(r.project.Path, wt.Path)
			if err != nil {
				rel = wt.Path
			}
			b.WriteString("  " + styleDim.Render(pad(wt.Branch, 24)) +
				styleWarn.Render(pad(asOrDash(wt.Head), 12)) +
				truncate(rel, max(20, m.width-16)) + "\n")
		}
	}

	if p.MarkerErr != "" {
		b.WriteString("\n" + styleError.Render("marker: "+p.MarkerErr) + "\n")
	}
	if r.snap.Err != "" {
		b.WriteString("\n" + styleError.Render("git: "+r.snap.Err) + "\n")
	}

	if len(r.snap.Files) > 0 {
		b.WriteString("\n" + key(fmt.Sprintf("files (%d)", len(r.snap.Files))) + "\n")
		maxFiles := max(3, m.height-16)
		for i, f := range r.snap.Files {
			if i >= maxFiles {
				b.WriteString(styleHint.Render(fmt.Sprintf("  … %d más", len(r.snap.Files)-maxFiles)) + "\n")
				break
			}
			b.WriteString("  " + styleWarn.Render(pad(f.Code, 3)) + truncate(f.Path, max(20, m.width-8)) + "\n")
		}
	}

	if len(r.snap.Commits) > 0 {
		b.WriteString("\n" + key("commits") + "\n")
		for _, c := range r.snap.Commits {
			b.WriteString(fmt.Sprintf("  %s %s %s\n",
				styleDim.Render(pad(c.Sha, 8)),
				pad(relativeTime(c.When), 6),
				truncate(c.Subject, max(20, m.width-24)),
			))
		}
	}

	if act, ok := m.lastAction[p.Path]; ok {
		verdict := styleClean.Render("ok")
		if act.err != "" {
			verdict = styleError.Render("failed")
		}
		b.WriteString("\n" + key("last "+act.kind) + " " + verdict + "\n")
		if tail := actionTail(act.output, max(3, m.height-20)); tail != "" {
			b.WriteString(styleHint.Render(indent(tail, "  ")) + "\n")
		}
	}

	footer := "esc back · g lazygit · ! cmd"
	if cr, ok := m.lastCmd[p.Path]; ok {
		verdict := styleClean.Render("exit 0")
		if cr.exit != "0" {
			verdict = styleError.Render("exit " + cr.exit)
		}
		b.WriteString("\n" + key("$ "+truncate(cr.command, max(20, m.width-30))) + " " + verdict + "\n")
		if tail := actionTail(cr.output, max(3, m.height-12)); tail != "" {
			b.WriteString(styleHint.Render(indent(tail, "  ")) + "\n")
		}
	}

	if m.cmdOpen {
		b.WriteString("\n" + styleDetailKey.Render(m.cmdInput.Prompt) +
			m.cmdInput.View() + "\n")
		footer = "enter run ($SHELL -c en el repo) · enter vacío = shell interactiva · esc cancel"
	}
	b.WriteString("\n" + styleHint.Render(footer))
	return b.String()
}

// renderWorktreeDetail compone el detalle de una sub-fila de worktree
// (0006 R33.6). Si el worktree fue descubierto con marcador y tiene snapshot
// vivo, se delega al detalle completo; si no, panel mínimo con los datos que
// trae `worktree list` (path/rama/head) SIN inventar estado git derivado.
func (m *Model) renderWorktreeDetail(e tableEntry) string {
	if p, ok := m.discoveredByPath(e.wt.Path); ok {
		if snap, ok := m.states[p.Path]; ok {
			return m.renderDetail(row{project: p, snap: snap, state: snap.State(p.HasRepo)})
		}
	}
	return m.renderWorktreeMinimal(e.wt, e.parent)
}

// renderWorktreeMinimal es el panel de detalle mínimo de un worktree sin
// snapshot propio (0006 R33.6): path, rama (o `(detached)`), head. No muestra
// dirty/ahead/behind/sync.
func (m *Model) renderWorktreeMinimal(wt gitstatus.Worktree, parent string) string {
	var b strings.Builder

	title := filepath.Base(wt.Path) + "  [worktree]"
	b.WriteString(styleDetailTitle.Render(title) + "\n")
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

	b.WriteString("\n" + styleHint.Render("esc back · g lazygit · ! cmd"))
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
