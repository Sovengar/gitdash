package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gitdash/internal/gitstatus"
)

const detailHeadLines = 6

// The worktrees list lives in the card's right column: the branch takes a fixed slice and the
// relative path gets whatever is left.
const wtBranchWidth = 10

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

// The `!` input is ALWAYS painted on the box's last line, even if the card filled the box (writing blind is worse than not seeing the rest), so what overflows is cut from the top and a short body is padded; without input there is no footer, the keys are already in keybinds.
func (m *Model) cardTail(body string, rows int) string {
	if !m.cmdOpen {
		return body
	}
	bodyRows := max(1, rows-cmdInputLines)
	lines := rellenaHasta(strings.Split(clipTo(body, bodyRows), "\n"), bodyRows)
	return strings.Join(lines, "\n") + "\n\n" + m.cmdInput.View()
}

// The card's shape comes from the layout: `split` is lay.cardSplit. Split, the fields and the
// worktrees stack in the detail box (files get their own peer box); collapsed, the single box keeps
// the stacked lists for terminals below the split floor.
func (m *Model) renderDetail(r row, rows, width int, split bool) string {
	rows = max(1, rows)
	tail := m.detailTail(r, rows, width)
	var body string
	if split {
		inner := max(1, width-2)
		// The tail keeps its lines at the bottom: the worktrees only get what is left after the head and the tail.
		avail := rows - detailHeadLines - strings.Count(tail, "\n")
		body = strings.TrimRight(m.detailFields(r, inner), "\n") +
			m.detailWorktrees(r, avail, inner)
	} else {
		body = m.detailCollapsed(r, m.detailFields(r, cardLeftWidth), rows)
	}
	if tail != "" {
		body += "\n" + tail
	}
	// Drop the builders' trailing newline so the box measures exactly the layout's height.
	return m.cardTail(strings.TrimRight(body, "\n"), rows)
}

// Fields first (the 6-line head). `left` never drops below cardLeftWidth, so the clip needs no floor.
func (m *Model) detailFields(r row, left int) string {
	var b strings.Builder
	p := r.project
	key := styleDetailKey.Render
	value := left - 8

	// The path is one more header field and not a loose line: on its own line (with the gap that separated it) it took a height the lists need, and its value is dimmed because it is context, not state.
	b.WriteString(key("path    ") + styleHint.Render(truncate(p.Path, value)) + "\n")

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
	b.WriteString(key("activity") + " " + styleDim.Render(relativeTime(r.snap.LastCommit)) + "\n")
	return b.String()
}

// The worktrees list moved out of the old right column into the detail box: the branch takes a
// fixed slice and the relative path gets whatever is left.
func (m *Model) detailWorktrees(r row, avail, inner int) string {
	n := len(r.snap.Worktrees)
	if n == 0 || avail < minListBlockLines {
		return ""
	}
	shown, rest := listBudget(avail, n)
	var b strings.Builder
	b.WriteString("\n" + styleDetailKey.Render(fmt.Sprintf("worktrees (%d)", n)) + "\n")
	for _, wt := range r.snap.Worktrees[:shown] {
		rel, err := filepath.Rel(r.project.Path, wt.Path)
		if err != nil {
			rel = wt.Path
		}
		b.WriteString("  " + styleDim.Render(pad(truncate(wt.Branch, wtBranchWidth), wtBranchWidth)) +
			truncate(rel, max(1, inner-2-wtBranchWidth)) + "\n")
	}
	if rest {
		b.WriteString(styleHint.Render(fmt.Sprintf("  … %d more", n-shown)) + "\n")
	}
	return b.String()
}

// Diagnostics and the tails of the last action/command; rendered full-width below the split.
func (m *Model) detailTail(r row, rows, width int) string {
	var b strings.Builder
	p := r.project
	key := styleDetailKey.Render

	if p.MarkerErr != "" {
		b.WriteString("\n" + styleError.Render("marker: "+p.MarkerErr) + "\n")
	}
	if r.snap.Err != "" {
		b.WriteString("\n" + styleError.Render("git: "+r.snap.Err) + "\n")
	}

	if act, ok := m.lastAction[p.Path]; ok {
		verdict := styleClean.Render("ok")
		if act.err != "" {
			verdict = styleError.Render("failed")
		}
		b.WriteString("\n" + key("last "+pullVariantLabel(act.kind)) + " " + verdict + "\n")
		// The resolved argv is the only thing that reveals the real policy: with `p` carrying no flags, what reconciled was the gitconfig, not gitdash.
		if act.cmd != "" {
			b.WriteString(styleHint.Render(indent(truncate(act.cmd, max(20, width-30)), "  ")) + "\n")
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
		b.WriteString("\n" + key("$ "+truncate(cr.command, max(20, width-30))) + " " + verdict + "\n")
		if tail := actionTail(cr.output, max(3, rows-12)); tail != "" {
			b.WriteString(styleHint.Render(indent(tail, "  ")) + "\n")
		}
	}
	return b.String()
}

// The collapsed card keeps the pre-restructure stack: fields, worktrees, files, with the lists
// sharing the remaining height as they did before the commits block left.
func (m *Model) detailCollapsed(r row, left string, rows int) string {
	var b strings.Builder
	b.WriteString(left)

	avail := max(0, rows-detailHeadLines)
	used := func(shown int, rest bool) int {
		n := minListBlockLines + shown
		if rest {
			n++
		}
		return n
	}
	key := styleDetailKey.Render
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
				truncate(rel, max(1, cardLeftWidth-16)) + "\n")
		}
		if rest {
			b.WriteString(styleHint.Render(fmt.Sprintf("  … %d more", n-shown)) + "\n")
		}
		avail -= used(shown, rest)
	}

	if n := len(r.snap.Files); n > 0 && avail >= minListBlockLines {
		shown, rest := listBudget(avail, n)
		b.WriteString("\n" + key(fmt.Sprintf("files (%d)", n)) + "\n")
		for _, f := range r.snap.Files[:shown] {
			b.WriteString("  " + styleWarn.Render(pad(f.Code, 3)) +
				truncate(f.Path, max(20, cardLeftWidth-8)) + "\n")
		}
		if rest {
			b.WriteString(styleHint.Render(fmt.Sprintf("  … %d more", n-shown)) + "\n")
		}
	}
	return b.String()
}

// A worktree discovered with a marker and a live snapshot delegates to the full card; without one, a minimal panel with what `worktree list` gives (path/branch/head) and NO invented derived git state.
func (m *Model) renderWorktreeDetail(e tableEntry, rows, width int, split bool) string {
	if r, live := m.liveRowForWorktree(e.wt); live {
		return m.renderDetail(r, rows, width, split)
	}
	return m.renderWorktreeMinimal(e.wt, e.parent, rows, width)
}

// The live row of a worktree path, when it was discovered and has a snapshot: the minimal panel
// would otherwise hide the git state and the marker's fields.
func (m *Model) liveRowForWorktree(wt gitstatus.Worktree) (row, bool) {
	if p, ok := m.discoveredByPath(wt.Path); ok {
		if snap, ok := m.states[p.Path]; ok {
			return row{project: p, snap: snap, state: snap.State(p.HasRepo)}, true
		}
	}
	return row{}, false
}

func (m *Model) renderWorktreeMinimal(wt gitstatus.Worktree, parent string, rows, width int) string {
	var b strings.Builder

	key := styleDetailKey.Render
	b.WriteString(key("path    ") +
		styleHint.Render(truncate(wt.Path, max(20, width-13))) + "\n")

	branch := wt.Branch
	if branch == "" {
		branch = "(detached)"
	}
	b.WriteString(key("branch  ") + branch + "\n")
	b.WriteString(key("head    ") + asOrDash(wt.Head) + "\n")
	if parent != "" {
		b.WriteString(key("repo    ") + filepath.Base(parent) + "\n")
	}

	return m.cardTail(b.String(), rows)
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
