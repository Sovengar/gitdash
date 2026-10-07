package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

type row struct {
	project discovery.Project
	snap    gitstatus.Snapshot
	state   gitstatus.State
}

type entryKind int

const (
	kindRepo entryKind = iota
	kindPrimary
	kindSecondary
	kindWorktree
)

// A navigable row: primary header, secondary header, repo or worktree sub-row (vroom's buildTree, extended); group carries the fold key, the primary name or `primary/secondary` for level-2 headers.
type tableEntry struct {
	kind  entryKind
	group string
	r     row

	// It does not reuse `r` because a worktree has no Snapshot/State of its own.
	wt     gitstatus.Worktree
	parent string
}

func groupKey(primary, secondary string) string {
	return primary + "/" + secondary
}

func groupLabel(p discovery.Project) string {
	// if and not switch: the coverage instrument only gives a block to the CONDITION of an if, while in a switch it gives the block to the case body and leaves the condition out, so no mutation of these lines could even run against the tests.
	if p.PrimaryGroup != "" && p.SecondaryGroup != "" {
		return groupKey(p.PrimaryGroup, p.SecondaryGroup)
	}
	if p.PrimaryGroup != "" {
		return p.PrimaryGroup
	}
	return ""
}

func pendingStates(st gitstatus.State) bool {
	switch st {
	case gitstatus.StateDirty, gitstatus.StateAhead, gitstatus.StateBehind, gitstatus.StateDiverged:
		return true
	}
	return false
}

func (m *Model) rows() []row {
	out := make([]row, 0, len(m.projects))
	for _, p := range m.projects {
		snap := m.states[p.Path]
		st := snap.State(p.HasRepo)
		if m.onlyDirty && !pendingStates(st) {
			continue
		}
		if m.search != "" && !matchRepo(p, snap, m.search) {
			continue
		}
		if m.worktreeHidden(p) {
			continue
		}
		out = append(out, row{project: p, snap: snap, state: st})
	}

	sortRows(out)
	return out
}

func (m *Model) entries() []tableEntry {
	base := m.rows()
	arranged := group.Arrange(toEntries(base))

	out := make([]tableEntry, 0, len(arranged)+2)
	skipPrim, skipSec := "", ""
	for i, e := range arranged {
		if group.IsPrimaryHeader(arranged, i) {
			out = append(out, tableEntry{kind: kindPrimary, group: e.Primary})
			if m.collapsed[e.Primary] {
				skipPrim, skipSec = e.Primary, ""
				continue
			}
			// skipSec is reset too: a collapsed secondary of the previous primary must not filter the next block (specially when this primary has none).
			skipPrim, skipSec = "", ""
		}
		if skipPrim == "" && group.IsSecondaryHeader(arranged, i) {
			key := groupKey(e.Primary, e.Secondary)
			out = append(out, tableEntry{kind: kindSecondary, group: key})
			if m.collapsed[key] {
				skipSec = key
				continue
			}
			skipSec = ""
		}
		if skipPrim != "" || skipSec != "" {
			continue
		}
		r := row{project: e.Proj, snap: e.Snap, state: e.State}
		out = append(out, tableEntry{kind: kindRepo, r: r})
		// Injected AFTER group.Arrange so they do not alter headers, order, counters or summary, which stay intact; the skipPrim/skipSec guards already guarantee the isolation.
		out = append(out, m.worktreeEntries(r)...)
	}
	return out
}

// A search hit on a worktree induces the expansion transiently, without touching m.expanded.
func (m *Model) worktreeEntries(r row) []tableEntry {
	if !m.expandable(r) || !m.repoExpanded(r) {
		return nil
	}
	var out []tableEntry
	for _, wt := range r.snap.Worktrees {
		if m.search != "" && !worktreeMatches(wt, m.search) {
			continue
		}
		out = append(out, tableEntry{kind: kindWorktree, wt: wt, parent: r.project.Path})
	}
	return out
}

func (m *Model) expandable(r row) bool {
	return !r.project.IsWorktree && len(r.snap.Worktrees) > 0
}

func (m *Model) repoExpanded(r row) bool {
	if m.expanded[r.project.Path] {
		return true
	}
	if m.search == "" {
		return false
	}
	for _, wt := range r.snap.Worktrees {
		if worktreeMatches(wt, m.search) {
			return true
		}
	}
	return false
}

func toEntries(rs []row) []group.Entry {
	out := make([]group.Entry, 0, len(rs))
	for _, r := range rs {
		out = append(out, group.Entry{
			Primary:   r.project.PrimaryGroup,
			Secondary: r.project.SecondaryGroup,
			Proj:      r.project,
			Snap:      r.snap,
			State:     r.state,
		})
	}
	return out
}

// Orphan fallback: if the main repo is not among the discovered ones it stays visible, and paths are normalized so a symlink or trailing slash does not duplicate it.
func (m *Model) worktreeHidden(p discovery.Project) bool {
	if !p.IsWorktree || p.MainRepo == "" {
		return false
	}
	main := filepath.Clean(p.MainRepo)
	for _, q := range m.projects {
		if filepath.Clean(q.Path) == main {
			return true
		}
	}
	return false
}

func matchSearch(p discovery.Project, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.PrimaryGroup), q) ||
		strings.Contains(strings.ToLower(p.SecondaryGroup), q)
}

func matchRepo(p discovery.Project, snap gitstatus.Snapshot, q string) bool {
	if matchSearch(p, q) {
		return true
	}
	for _, wt := range snap.Worktrees {
		if worktreeMatches(wt, q) {
			return true
		}
	}
	return false
}

func worktreeMatches(wt gitstatus.Worktree, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(wt.Branch), q) ||
		strings.Contains(strings.ToLower(filepath.Base(wt.Path)), q)
}

func sortRows(rows []row) {
	// Two `range` loops instead of `j--`: an inverted `j++` leaves `j > 0` permanently true and the walk never ends, a hang no test can kill (TIMED_OUT does not even reach mutants_total), so the mutation would escape the gate unnoticed.
	for i := range len(rows) - 1 {
		for k := range i + 1 {
			j := i + 1 - k
			if j > 0 && rowLess(rows[j], rows[j-1]) {
				rows[j], rows[j-1] = rows[j-1], rows[j]
			}
		}
	}
}

func rowLess(a, b row) bool {
	sa, sb := a.state.Score(), b.state.Score()
	if sa != sb {
		return sa > sb
	}
	if a.snap.LastCommit != b.snap.LastCommit {
		return a.snap.LastCommit > b.snap.LastCommit
	}
	return strings.ToLower(a.project.Name) < strings.ToLower(b.project.Name)
}

func (m *Model) summary() (total, dirty, ahead, behind int) {
	for _, p := range m.projects {
		if m.worktreeHidden(p) {
			continue
		}
		snap := m.states[p.Path]
		total++
		switch snap.State(p.HasRepo) {
		case gitstatus.StateDirty, gitstatus.StateDiverged:
			dirty++
		}
		if snap.Status.Ahead > 0 {
			ahead++
		}
		if snap.Status.Behind > 0 {
			behind++
		}
	}
	return total, dirty, ahead, behind
}

// Cells return plain text plus a style and the render pads BEFORE styling, so the alignment cannot be broken by ANSI codes.

func (m *Model) renderEntry(e tableEntry, selected bool, width int) string {
	switch e.kind {
	case kindPrimary:
		line := m.primaryHeaderLine(e.group)
		return m.rowCursor(selected) + "  " + line
	case kindSecondary:
		line := m.secondaryHeaderLine(e.group)
		return m.rowCursor(selected) + "  " + indentHeader + line
	case kindWorktree:
		// Dedicated render, NOT renderRow: an empty Snapshot would read as no-up/clean, that is, as false state information.
		return m.renderWorktreeRow(e.wt, selected, width)
	default:
		return m.renderRow(e.r, selected, width)
	}
}

const indentHeader = "  "

type tableColumn struct {
	title string
	width int
}

var tableColumns = []tableColumn{
	{"NAME", colName},
	{"BRANCH", colBranch},
	{"Work Tree", colWT},
	{"↑↓up", colUpDown},
	{"SYNC", colSync},
}

func fitColumns(innerWidth int) int {
	used := 0
	for i, c := range tableColumns {
		if used+c.width > innerWidth {
			return max(1, i)
		}
		used += c.width
	}
	return len(tableColumns)
}

// width includes the two borders and the 4-cell row prefix (cursor + fetch slot): -6 is what keeps
// the header on the exact same columns as the rows, with no overflow past the right border.
func headerColumns(width int) string {
	var b strings.Builder
	for _, c := range tableColumns[:fitColumns(width-rowPrefixWidth-2)] {
		b.WriteString(pad(c.title, c.width))
	}
	return b.String()
}

// The prefix is cursor (2) + fetch slot (2) on every row kind, so names never shift while a fetch
// runs and the worktree/header left edge matches the repo rows.
const headerSlot = "    "

func (m *Model) rowCursor(selected bool) string {
	if selected {
		return styleCursor.Render("▸ ")
	}
	return "  "
}

func (m *Model) fetchSlot(path string) string {
	text, style := m.fetchCell(path)
	return style.Render(pad(truncate(text, fetchSlotWidth), fetchSlotWidth))
}

// State-dependent cells (Work Tree, ↑↓up, SYNC) stay empty/dim because the UI does not promise dirty/ahead/behind/sync per worktree.
func (m *Model) renderWorktreeRow(wt gitstatus.Worktree, selected bool, width int) string {
	branch := worktreeBranchLabel(wt)
	cells := []struct {
		text  string
		style lipglossStyle
		width int
	}{
		{"  ↳ " + filepath.Base(wt.Path), styleWorktree, colName},
		{branch, styleWorktree, colBranch},
		{"", styleDim, colWT},
		{"", styleDim, colUpDown},
		{"—", styleDim, colSync},
	}
	cells = cells[:fitColumns(width-rowPrefixWidth-2)]
	line := ""
	for _, c := range cells {
		line += c.style.Render(pad(truncate(c.text, c.width), c.width))
	}
	return m.rowCursor(selected) + "  " + line
}

func worktreeBranchLabel(wt gitstatus.Worktree) string {
	if wt.Branch != "" {
		return wt.Branch
	}
	if wt.Head != "" {
		return "(detached) " + wt.Head
	}
	return "(detached)"
}

func (m *Model) primaryHeaderLine(g string) string {
	glyph := "▾"
	if m.collapsed[g] {
		glyph = "▸"
	}
	return styleGroupHeader.Render(glyph + " " + fmt.Sprintf("%s (%d)", g, m.countPrimary(g)))
}

func (m *Model) secondaryHeaderLine(key string) string {
	glyph := "▾"
	if m.collapsed[key] {
		glyph = "▸"
	}
	_, sec, _ := strings.Cut(key, "/")
	return styleSecondaryHeader.Render(glyph + " " + fmt.Sprintf("%s (%d)", sec, m.countSecondary(key)))
}

func (m *Model) countPrimary(g string) int {
	n := 0
	for _, e := range group.Arrange(toEntries(m.rows())) {
		if e.Primary == g {
			n++
		}
	}
	return n
}

func (m *Model) countSecondary(key string) int {
	n := 0
	for _, e := range group.Arrange(toEntries(m.rows())) {
		if e.Primary != "" && groupKey(e.Primary, e.Secondary) == key {
			n++
		}
	}
	return n
}

type groupStats struct {
	repos, errors, dirty, ahead, behind int
	worktrees                           int
}

// It uses the same rows as the table (filters applied) but BEFORE folding: a collapsed group still has repos to count, which is exactly what its header shows, so the aggregate must say what the header says.
func (m *Model) groupStats(key string) groupStats {
	var st groupStats
	for _, r := range m.rows() {
		if !inGroup(r.project, key) {
			continue
		}
		st.repos++
		switch r.state {
		case gitstatus.StateError:
			st.errors++
		case gitstatus.StateDirty, gitstatus.StateDiverged:
			st.dirty++
		}
		if r.snap.Status.Ahead > 0 {
			st.ahead++
		}
		if r.snap.Status.Behind > 0 {
			st.behind++
		}
		// No guard: adding 0 changes nothing and `n > 0` was a dead affordance, one more place where a `>=` went unnoticed.
		st.worktrees += len(r.snap.Worktrees)
	}
	return st
}

// The ungrouped section is a literal of the group package and not just another primary, so it is checked apart: without this case a primary named "(ungrouped)" would be counted twice.
func inGroup(p discovery.Project, key string) bool {
	if key == group.Ungrouped {
		return p.PrimaryGroup == ""
	}
	primary, secondary, nested := strings.Cut(key, "/")
	if !nested {
		return p.PrimaryGroup == primary
	}
	return p.PrimaryGroup == primary && p.SecondaryGroup == secondary
}

func entryAt(entries []tableEntry, i int) (tableEntry, bool) {
	if i < 0 || i >= len(entries) {
		return tableEntry{}, false
	}
	return entries[i], true
}

func (m *Model) renderRow(r row, selected bool, width int) string {
	name, nameStyle := m.nameCell(r)
	branch, branchStyle := m.branchCell(r)
	wt, wtStyle := m.wtCell(r)
	upDown, upDownStyle := m.upDownCell(r)
	sync, syncStyle := m.syncCell(r)

	cells := []struct {
		text  string
		style lipglossStyle
		width int
	}{
		{name, nameStyle, colName},
		{branch, branchStyle, colBranch},
		{wt, wtStyle, colWT},
		{upDown, upDownStyle, colUpDown},
		{sync, syncStyle, colSync},
	}
	cells = cells[:fitColumns(width-rowPrefixWidth-2)]

	line := ""
	for _, c := range cells {
		line += c.style.Render(pad(truncate(c.text, c.width), c.width))
	}
	return m.rowCursor(selected) + m.fetchSlot(r.project.Path) + line
}

func (m *Model) nameCell(r row) (string, lipglossStyle) {
	name := r.project.Name
	if r.project.IsWorktree {
		name += " [wt]"
	}
	// Same guard as `expandable`: an orphan worktree (kindRepo + IsWorktree) is not expandable and must show neither glyph nor counter (a dead affordance).
	if m.expandable(r) {
		glyph := "▸"
		if m.repoExpanded(r) {
			glyph = "▾"
		}
		name += fmt.Sprintf(" %s (%d wt)", glyph, len(r.snap.Worktrees))
	}
	if r.project.MarkerErr != "" {
		return name, styleWarn
	}
	return name, styleSel
}

func (m *Model) wtCell(r row) (string, lipglossStyle) {
	switch r.state {
	case gitstatus.StateError:
		return "⚠", styleError
	case gitstatus.StateNoRepo:
		return "∅", styleDim
	}
	if r.snap.Status.Dirty() > 0 {
		return dirtyTail(r), m.styleFor(r)
	}
	return "", styleClean
}

func (m *Model) branchCell(r row) (string, lipglossStyle) {
	if r.state == gitstatus.StateNoRepo || r.snap.Status.Branch == "" {
		return "-", styleDim
	}
	if r.snap.Status.Detached {
		return r.snap.Status.Branch + " (detached)", m.styleFor(r)
	}
	return r.snap.Status.Branch, m.styleFor(r)
}

func (m *Model) fetchCell(path string) (string, lipglossStyle) {
	switch m.fetchStates[path] {
	case "fetching":
		return "⟳ ", styleFetchRun
	case "failed":
		return "✗ ", styleFetchBad
	default:
		return "", styleClean
	}
}

func dirtyTail(r row) string {
	s := r.snap.Status
	out := ""
	if s.TrackedChanges > 0 {
		out += fmt.Sprintf("%d", s.TrackedChanges)
	}
	if s.Untracked > 0 {
		out += fmt.Sprintf(" ?%d", s.Untracked)
	}
	return strings.TrimSpace(out)
}

func (m *Model) upDownCell(r row) (string, lipglossStyle) {
	s := r.snap.Status
	if r.state == gitstatus.StateNoRepo || r.snap.Err != "" {
		return "", styleClean
	}
	if !s.HasUpstream {
		return "no-up", styleWarn
	}
	if s.Ahead > 0 && s.Behind > 0 {
		return fmt.Sprintf("↑%d↓%d", s.Ahead, s.Behind), styleDiverged
	}
	if s.Ahead > 0 {
		return fmt.Sprintf("↑%d", s.Ahead), styleAhead
	}
	if s.Behind > 0 {
		return fmt.Sprintf("↓%d", s.Behind), styleBehind
	}
	return "", styleClean
}

func (m *Model) syncCell(r row) (string, lipglossStyle) {
	if r.state == gitstatus.StateNoRepo || r.snap.SyncBranch == "" {
		return "—", styleDim
	}
	if !r.snap.SyncKnown {
		return r.snap.SyncBranch + " —", styleDim
	}
	if r.snap.SyncBehind == 0 {
		return r.snap.SyncBranch, styleDim
	}
	return fmt.Sprintf("%s ↓%d", r.snap.SyncBranch, r.snap.SyncBehind), styleWarn
}

func (m *Model) styleFor(r row) lipglossStyle {
	switch r.state {
	case gitstatus.StateError:
		return styleError
	case gitstatus.StateNoRepo:
		return styleDim
	case gitstatus.StateNoUpstream:
		return styleWarn
	case gitstatus.StateDetached:
		return styleWarn
	case gitstatus.StateDiverged:
		return styleDiverged
	case gitstatus.StateDirty:
		return styleDirty
	case gitstatus.StateAhead:
		return styleAhead
	case gitstatus.StateBehind:
		return styleBehind
	default:
		return styleClean
	}
}

func truncate(s string, w int) string {
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	runes := []rune(s)
	return string(runes[:w-1]) + "…"
}

func pad(s string, w int) string {
	// A clamp, not a guard whose edge (`n >= w` vs `n > w`) distinguishes no case: padding too much is identity (Repeat of 0 is "") and pad never truncates, only completes the width.
	return s + strings.Repeat(" ", max(0, w-utf8.RuneCountInString(s)))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func stripANSI(s string) string {
	var out strings.Builder
	inSeq := false
	for _, r := range s {
		if r == '\x1b' {
			inSeq = true
		} else if inSeq && (r == 'm' || r == 'K') {
			inSeq = false
		} else if !inSeq {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func relativeTime(epoch int64) string {
	if epoch <= 0 {
		return "-"
	}
	return relativeAge(time.Since(time.Unix(epoch, 0)))
}

// The cut exists for the edges: the only thing separating `< 1h` from `< 1h+1ns` is an age landing exactly on the threshold, which a test cannot build with the clock inside (time always passes between measuring and reading), so with the age as input the threshold is reachable and the test can name it.
func relativeAge(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d < 7*24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d < 30*24*time.Hour {
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
	return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
}
