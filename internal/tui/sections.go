package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gitdash/internal/gitstatus"
	"gitdash/internal/tui/bordered"
)

func (m Model) section(title, content string, width int) string {
	if title != "" {
		title = " " + title + " "
	}
	return bordered.RenderWithTitle(bordered.Rounded(), borderColor, title, content, width)
}

// The overlay's width fits its longest line ("this takes a couple of seconds · esc close") plus the box's borders.
const visualOverlayWidth = 44

// The render overlay is a centered box over the dashboard (prdash style): a toast would expire after 3s while a bigger history can render for longer, and a view mode would hide the table the render belongs to. Empty when nothing renders, which is what makes the splice a no-op.
func (m Model) visualOverlayLines() []string {
	if m.visualBusy == nil {
		return nil
	}
	content := m.spinner.View() + styleFetchRun.Render(" rendering "+m.visualBusy.sub+"…") + "\n" +
		styleDim.Render("this takes a couple of seconds · esc close")
	return strings.Split(m.section("simulate: "+m.visualBusy.sub, content, visualOverlayWidth), "\n")
}

// With a warning in keybinds (pull selector, removal confirmation, log or PR legend) what is forced is that section's visibility: degrading it would leave the app waiting for a key without saying which.
func (m Model) layout() layout {
	// The PR overlay lives IN the body (it is the dashboard that gets replaced), so its minimum is passed through: the layout must not steal height from a preview panel that will not be drawn below.
	formMin := 0
	if m.pr != nil {
		formMin = prMinBodyLines()
	}
	lay := computeLayout(m.height, m.searchActive || m.search != "", m.keybindsLines(), m.promptLine() != "", formMin)
	if m.logOpen || m.pr != nil {
		// The log and the form REPLACE the table and the card disappears, so the total number of terminal lines is what has to be conserved: the column header and the whole card go back to the body's height, otherwise the panel measures 3 lines short and the keybinds move up.
		freed := 1
		if lay.previewLines > 0 {
			freed += previewChrome + lay.previewLines
		}
		lay.bodyLines += freed
		lay.previewLines = 0
	}
	// The width split lives here too: the height search stays height-only and every pane reads its
	// width from this single value, so a degradation decision cannot disagree with what is painted.
	lay.detailWidth = m.width
	lay.cardSplit = m.width-2 >= cardLeftWidth+cardSepWidth+cardRightWidth
	// The commits box and the files box share one width: the lists column's share plus the files
	// box's own borders, capped so the table keeps all its columns. Below the floor the panel drops.
	_, share := cardColumns(m.width - 2)
	right := min(share+2, m.width-1-minTableWidth())
	if right >= commitsPanelWidth {
		lay.showPanel = true
		lay.panelWidth = right
		lay.tableWidth = m.width - 1 - right
	} else {
		lay.tableWidth = m.width
	}
	// The bottom band mirrors the top one: left box == tableWidth and right box == panelWidth when
	// the panel is on; with the panel off the files box keeps the card's share of the width.
	if lay.cardSplit {
		if lay.showPanel {
			lay.detailWidth = lay.tableWidth
			lay.filesWidth = lay.panelWidth
		} else {
			lay.filesWidth = share + 2
			lay.detailWidth = m.width - 1 - lay.filesWidth
		}
	}
	return lay
}

// The height budget derives from here so the box never measures more than what it holds.
func (m Model) keybindsLines() int {
	if m.promptLine() != "" {
		return 1
	}
	return min(defaultHintLines, max(0, len(m.cfg.HintBarLines())))
}

func (m Model) promptLine() string {
	// The armed states' precedence is the requirement (the most specific wins), and that is why it is an if instead of the case order.
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

// Armed warnings do NOT go here: they live in keybinds, where the user already looks for the keys.
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
	return m.section("gitdash", strings.Join(parts, "  "), m.width)
}

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

// Sorted by path so the render is deterministic (m.running is a map).
func (m Model) runningActions() []string {
	paths := make([]string, 0, len(m.running))
	for path, kind := range m.running {
		if isRebaseKind(kind) || kind == "push" || kind == "worktree_remove" {
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
	return m.section("filter", content, m.width)
}

// Entries arrive already computed: the preview panel needs the same ones and recomposing them here would duplicate the Arrange on every render.
func (m Model) tableSection(bodyLines int, entries []tableEntry, width int) string {
	m.syncOffset(len(entries), bodyLines)

	var rows []string
	if len(entries) == 0 && !m.scanning {
		rows = append(rows, "  "+m.emptyTableHint())
	} else {
		for i := m.offset; i < min(len(entries), m.offset+bodyLines); i++ {
			rows = append(rows, m.renderEntry(entries[i], i == m.cursor, width))
		}
	}
	// rellenaHasta instead of a `for len(rows) < bodyLines`: the box measures what the layout says, and comparing inside the loop turns the mutant into a hang.
	rows = rellenaHasta(rows, bodyLines)

	header := headerSlot + headerColumns(width)
	return m.section("repos", styleHint.Render(header)+"\n"+strings.Join(rows, "\n"), width)
}

// The commits panel is the top-right band: same height as the table box, its own title, and a
// fixed 30-cell body. It never falls back into the card, so what is dropped is not shown anywhere.
func (m *Model) panelSection(lay layout, entries []tableEntry) string {
	rows := lay.bodyLines + 1
	inner := max(1, lay.panelWidth-2)
	e, ok := entryAt(entries, m.cursor)
	var title, content string
	switch {
	case !ok:
		title, content = "commits", m.panelEmpty()
	case e.kind == kindRepo:
		title, content = "commits · "+detailTitle(e.r), m.commitsPanel(e.r.snap, rows, inner)
	case e.kind == kindWorktree:
		title, content = "commits · "+worktreeTitle(e), m.worktreeCommitsPanel(e, rows, inner)
	default:
		title, content = "commits · "+e.group, m.groupSummaryText(e)
	}
	return m.section(title, fitLines(content, rows), lay.panelWidth)
}

func (m *Model) panelEmpty() string {
	return styleDim.Render("  " + m.emptyTableHint())
}

type commitGroup struct {
	label   string
	commits []gitstatus.Commit
	accent  lipglossStyle
}

// The sync group only exists with a DIFFERENT ref that has commits of its own: on the sync branch
// itself the two lists would duplicate. Each group carries the colour its one-sided commits get;
// which ones those are is already decided per sha in `Collect`.
func commitGroups(snap gitstatus.Snapshot) []commitGroup {
	label := "current"
	if snap.Status.Branch != "" {
		label = snap.Status.Branch + " (current)"
	}
	groups := []commitGroup{{label: label, commits: snap.Commits, accent: styleAhead}}
	if snap.SyncBranch != "" && snap.SyncBranch != snap.Status.Branch && len(snap.SyncCommits) > 0 {
		groups = append(groups, commitGroup{label: snap.SyncBranch + " (sync)", commits: snap.SyncCommits, accent: styleBehind})
	}
	return groups
}

// Newest first, one line per commit, never wrapped: the subject is cut to whatever the body leaves
// after the sha/age prefix. The height is shared so the first group does not clip the second away,
// and the commits only one branch has take its accent, leaving shared ones neutral.
func (m *Model) commitsPanel(snap gitstatus.Snapshot, rows, inner int) string {
	groups := commitGroups(snap)
	per := max(1, (rows-len(groups))/len(groups))
	// One-sided only reads next to the other list: without the sync group the colours would name a comparison that is not on screen.
	coloured := len(groups) > 1
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(styleDetailKey.Render(truncate(g.label, inner)) + "\n")
		if len(g.commits) == 0 {
			b.WriteString(styleDim.Render("  no commits") + "\n")
			continue
		}
		for _, c := range g.commits[:min(len(g.commits), per)] {
			// The subject is untrusted repo text: it goes through the log panel's sanitiser before painting.
			sha, subject := pad(c.Sha, 8), truncate(sanitizeLogText(c.Subject), max(1, inner-18))
			if coloured && c.OneSided {
				sha, subject = g.accent.Render(sha), g.accent.Render(subject)
			} else {
				sha = styleDim.Render(sha)
			}
			fmt.Fprintf(&b, "%s %s %s\n", sha, pad(relativeTime(c.When), 6), subject)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) worktreeCommitsPanel(e tableEntry, rows, inner int) string {
	if p, ok := m.discoveredByPath(e.wt.Path); ok {
		if snap, ok := m.states[p.Path]; ok {
			return m.commitsPanel(snap, rows, inner)
		}
	}
	// No live snapshot for that path: a dim placeholder, never the parent repo's commits.
	return styleDim.Render("  no commits")
}

// It paints the bottom band for the row under the cursor: the detail box, plus the peer files box
// when the row has files. Both boxes are clipped to their height, since the layout owns it.
func (m *Model) previewSection(lay layout, entries []tableEntry) string {
	if lay.previewLines <= 0 {
		return ""
	}
	rows := lay.previewLines
	e, ok := entryAt(entries, m.cursor)
	switch {
	case !ok:
		return m.singleBox("repos", m.previewEmpty(), rows)
	case e.kind == kindRepo:
		return m.detailBand(detailTitle(e.r), e.r, rows, lay)
	case e.kind == kindWorktree:
		if r, live := m.liveRowForWorktree(e.wt); live {
			return m.detailBand(worktreeTitle(e), r, rows, lay)
		}
		return m.singleBox(worktreeTitle(e), m.renderWorktreeMinimal(e.wt, e.parent, rows, m.width), rows)
	default:
		return m.singleBox(e.group, m.renderGroupSummary(e, rows), rows)
	}
}

// With no files (or below the split floor) the detail box takes the whole band; otherwise the
// detail box and the files box share the width, side by side.
func (m *Model) detailBand(title string, r row, rows int, lay layout) string {
	if !lay.cardSplit || len(r.snap.Files) == 0 {
		m.fitCmdInput(m.width)
		return m.section(title, fitLines(m.renderDetail(r, rows, m.width, lay.cardSplit), rows), m.width)
	}
	m.fitCmdInput(lay.detailWidth)
	detail := m.section(title, fitLines(m.renderDetail(r, rows, lay.detailWidth, true), rows), lay.detailWidth)
	return joinPanes(detail, m.filesSection(r, rows, lay.filesWidth))
}

// The files live in their own bordered box; the count moves from the old in-body heading to the title.
func (m *Model) filesSection(r row, rows, width int) string {
	return m.section(fmt.Sprintf("files (%d)", len(r.snap.Files)),
		fitLines(m.filesList(r, rows, width-2), rows), width)
}

func (m *Model) filesList(r row, avail, inner int) string {
	n := len(r.snap.Files)
	shown, rest := listBudget(avail, n)
	var b strings.Builder
	for _, f := range r.snap.Files[:shown] {
		b.WriteString("  " + styleWarn.Render(pad(f.Code, 3)) +
			truncate(f.Path, max(1, inner-5)) + "\n")
	}
	if rest {
		b.WriteString(styleHint.Render(fmt.Sprintf("  … %d more", n-shown)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// The `!` input is painted inside the detail box, so its width is the box minus the borders and the
// prompt; anything else lets the value run past the border.
func (m *Model) fitCmdInput(boxWidth int) {
	m.cmdInput.SetWidth(max(1, boxWidth-4))
}

func (m *Model) singleBox(title, content string, rows int) string {
	m.fitCmdInput(m.width)
	return m.section(title, fitLines(content, rows), m.width)
}

func (m Model) previewEmpty() string {
	return styleDim.Render("  " + m.emptyTableHint())
}

// Shared by the table and the preview panel, which are read together and so cannot contradict.
func (m Model) emptyTableHint() string {
	if m.search != "" || m.onlyDirty {
		return "no repositories match the current filter"
	}
	return "no repositories — create a .gitdash.toml in your projects"
}

// The prompt replaces the hints instead of stacking on them: both answer "what do I do now", and the hints come back intact once the keystroke is resolved.
func (m Model) keybindsSection(hintLines int) string {
	lines := m.cfg.HintBarLines()
	if prompt := m.promptLine(); prompt != "" {
		lines = []string{styleWarn.Render(prompt)}
	}
	// The budget wins: hints that do not fit are cut (cutting the ones that just fit == doing nothing) and a negative budget leaves the box empty instead of panicking on the slice.
	lines = lines[:max(0, min(hintLines, len(lines)))]
	rendered := make([]string, 0, len(lines))
	for _, l := range lines {
		rendered = append(rendered, styleHint.Render(l))
	}
	return m.section("keybinds", strings.Join(rendered, "\n"), m.width)
}

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

func worktreeTitle(e tableEntry) string {
	return filepath.Base(e.wt.Path) + " [worktree]"
}

// Only the states present are painted: a clean group does not deserve four lines of zeros (the table is quiet for the same reason).
func (m *Model) renderGroupSummary(e tableEntry, rows int) string {
	return m.cardTail(m.groupSummaryText(e), rows)
}

// The pure aggregate text, shared by the card and the commits panel: the panel must not get the
// card's `!` footer, so cardTail stays out of here.
func (m *Model) groupSummaryText(e tableEntry) string {
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
	return b.String()
}

// Horizontal join of two already-rendered panes: the gap is the 1-cell separation and each line
// is padded to its pane width first, so the joined band keeps the terminal's exact width.
func joinPanes(left, right string) string {
	ll := strings.Split(left, "\n")
	rl := strings.Split(right, "\n")
	n := max(len(ll), len(rl))
	out := make([]string, n)
	lpad := rellenaHasta(ll, n)
	rpad := rellenaHasta(rl, n)
	for i := range n {
		out[i] = lpad[i] + strings.Repeat(" ", commitsPanelGap) + rpad[i]
	}
	return strings.Join(out, "\n")
}

func fitLines(content string, n int) string {
	return strings.Join(rellenaHasta(strings.Split(clipTo(content, n), "\n"), n), "\n")
}

func clipTo(content string, n int) string {
	lines := strings.Split(content, "\n")
	// Clipping to the exact number is identity, so min() replaces the guard: here the content can only be cut, never padded.
	return strings.Join(lines[:max(0, min(n, len(lines)))], "\n")
}

// It exists so the three padding loops do not compare `len(...) < n` inside their condition: an inverted `<` turns a fill that should end after ONE turn into an infinite loop, which no test kills and the CI gate never sees (TIMED_OUT).
func rellenaHasta(lines []string, n int) []string {
	for missing := n - len(lines); missing > 0; missing-- {
		lines = append(lines, "")
	}
	return lines
}
