package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gitdash/internal/tui/bordered"
)

func (m Model) section(title, content string) string {
	if title != "" {
		title = " " + title + " "
	}
	return bordered.RenderWithTitle(bordered.Rounded(), borderColor, title, content, m.width)
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
	return m.section("gitdash", strings.Join(parts, "  "))
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

// Entries arrive already computed: the preview panel needs the same ones and recomposing them here would duplicate the Arrange on every render.
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
	// rellenaHasta instead of a `for len(rows) < bodyLines`: the box measures what the layout says, and comparing inside the loop turns the mutant into a hang.
	rows = rellenaHasta(rows, bodyLines)

	header := "  " + headerColumns(m.width)
	return m.section("repos", styleHint.Render(header)+"\n"+strings.Join(rows, "\n"))
}

// It paints the same card clipped to its height and padded with empty lines, since the layout owns the height: without padding the box would shrink moving from a clean repo to one with 30 files.
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
	default:
		title, content = e.group, m.renderGroupSummary(e, rows)
	}
	return m.section(title, fitLines(content, rows))
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
	return m.section("keybinds", strings.Join(rendered, "\n"))
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
	return m.cardTail(b.String(), rows)
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
