// Vista de detalle de un repo (spec 0001 R10).
package tui

import (
	"fmt"
	"strings"
)

// renderDetail compone el panel de detalle del repo seleccionado con datos
// vivos del snapshot (S10.1, S10.2).
func (m *Model) renderDetail(r row) string {
	var b strings.Builder

	p := r.project
	title := p.Name
	if p.Group != "" {
		title += "  ·  " + p.Group
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
	stateText, stateStyle := m.stateCell(r)
	upDownText, upDownStyle := m.upDownCell(r)
	b.WriteString(key("branch  ") + branch + "\n")
	b.WriteString(key("upstream") + " " + upstream + "\n")
	dirtyLine := stateStyle.Render(stateText)
	if upDownText != "" {
		dirtyLine += " " + upDownStyle.Render(upDownText)
	}
	b.WriteString(key("state   ") + dirtyLine + "\n")

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

	b.WriteString("\n" + styleHint.Render("esc back"))
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
