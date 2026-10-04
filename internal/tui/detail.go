package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gitdash/internal/gitstatus"
)

const detailHeadLines = 5

const minListBlockLines = 2

// The warning line is reserved when the list does not fit whole (a cut at the box edge would look like the list ended there), except when only one element fits, since the header already says how many there are.
func listBudget(avail, n int) (shown int, rest bool) {
	if avail < minListBlockLines {
		return 0, false
	}
	room := avail - minListBlockLines
	if n <= room {
		return n, false
	}
	if room < 1 {
		return 0, false
	}
	return room - 1, true
}

func asOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

const cmdInputLines = 2

// The input is ALWAYS painted at the end, even if the card filled the box (writing blind is worse than not seeing the rest), so what overflows is cut from the top; without input there is no footer, the keys are already in keybinds.
func (m *Model) fichaTail(body string, rows int) string {
	if !m.cmdOpen {
		return body
	}
	return clipTo(body, max(1, rows-cmdInputLines)) +
		"\n\n" + styleDetailKey.Render(m.cmdInput.Prompt) + m.cmdInput.View()
}

// The internal budget derives from rows (the layout's height) and not from the terminal, or a full-height cap would paint a list the box then crops without saying how many rows are missing; the title lives in the border.
func (m *Model) renderDetail(r row, rows int) string {
	var b strings.Builder

	p := r.project
	rows = max(1, rows)

	key := styleDetailKey.Render

	// The path is one more header field and not a loose line: on its own line (with the gap that separated it) it took a height the lists need, and its value is dimmed because it is context, not state.
	b.WriteString(key("path    ") +
		styleHint.Render(truncate(p.Path, max(20, m.width-13))) + "\n")

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
	// The detail IS verbose: an explicit "clean" instead of an empty cell.
	wtText, wtStyle := m.wtCell(r)
	if wtText == "" {
		wtText, wtStyle = "clean", styleClean
	}
	stateLine := wtStyle.Render(wtText)
	if upDownText, upDownStyle := m.upDownCell(r); upDownText != "" {
		stateLine += " " + upDownStyle.Render(upDownText)
	}
	b.WriteString(key("state   ") + stateLine + "\n")

	syncLine := "— (no sync branch)"
	// The first case did nothing (without a sync branch the placeholder stays); nested reads clearer than a switch with an empty arm.
	if r.snap.SyncBranch != "" {
		switch {
		case !r.snap.SyncKnown:
			syncLine = r.snap.SyncBranch + " (ref missing)"
		case r.snap.SyncBehind > 0:
			syncLine = r.snap.SyncBranch + fmt.Sprintf(" (↓%d)", r.snap.SyncBehind)
		default:
			syncLine = r.snap.SyncBranch + " (ok)"
		}
	}
	b.WriteString(key("sync    ") + syncLine + "\n")

	avail := max(0, rows-detailHeadLines)
	// The three lists COMPETE for the same space: without deducting, each would believe it has the whole budget and the card would overflow the box (which fitLines then crops from the top without warning, exactly what the "… N more" line avoids).
	consumido := func(shown int, rest bool) int {
		n := minListBlockLines + shown
		if rest {
			n++
		}
		return n
	}

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
			b.WriteString(styleHint.Render(fmt.Sprintf("  … %d more", n-shown)) + "\n")
		}
		avail -= consumido(shown, rest)
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
			b.WriteString(styleHint.Render(fmt.Sprintf("  … %d more", n-shown)) + "\n")
		}
		avail -= consumido(shown, rest)
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
		// The resolved argv is the only thing that reveals the real policy: with `p` carrying no flags, what reconciled was the gitconfig, not gitdash.
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

	return m.fichaTail(b.String(), rows)
}

// A worktree discovered with a marker and a live snapshot delegates to the full card; without one, a minimal panel with what `worktree list` gives (path/branch/head) and NO invented derived git state.
func (m *Model) renderWorktreeDetail(e tableEntry, rows int) string {
	if p, ok := m.discoveredByPath(e.wt.Path); ok {
		if snap, ok := m.states[p.Path]; ok {
			return m.renderDetail(row{project: p, snap: snap, state: snap.State(p.HasRepo)}, rows)
		}
	}
	return m.renderWorktreeMinimal(e.wt, e.parent, rows)
}

func (m *Model) renderWorktreeMinimal(wt gitstatus.Worktree, parent string, rows int) string {
	var b strings.Builder

	key := styleDetailKey.Render
	b.WriteString(key("path    ") +
		styleHint.Render(truncate(wt.Path, max(20, m.width-13))) + "\n")

	branch := wt.Branch
	if branch == "" {
		branch = "(detached)"
	}
	b.WriteString(key("branch  ") + branch + "\n")
	b.WriteString(key("head    ") + asOrDash(wt.Head) + "\n")
	if parent != "" {
		b.WriteString(key("repo    ") + filepath.Base(parent) + "\n")
	}

	return m.fichaTail(b.String(), rows)
}

func actionTail(out string, n int) string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	// The last n lines with both borders resolved without guards: with n <= 0 nothing is left and with n >= len(lines) everything is; the 0 floor avoids the negative index that WAS a panic (a negative n here used to blow up).
	since := min(len(lines), max(0, len(lines)-n))
	return strings.Join(lines[since:], "\n")
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
