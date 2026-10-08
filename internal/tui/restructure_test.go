package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// committedSnap builds a clean snapshot whose commits are listed newest first, as gitstatus collects them.
func committedSnap(subjects ...string) gitstatus.Snapshot {
	s := snapClean()
	for _, sub := range subjects {
		s.Commits = append(s.Commits, gitstatus.Commit{Sha: "0000000", When: time.Now().Add(-time.Hour).Unix(), Subject: sub})
	}
	return s
}

// ── Table columns ────────────────────────────────────────────────────────────

func TestRowsPaintNoLastCommitAge(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	row := stripANSI(m.renderRow(m.rows()[0], false, m.width))
	if !strings.Contains(row, "api") {
		t.Fatalf("the fixture row does not paint its name: %q", row)
	}
	if strings.Contains(row, "2h") {
		t.Errorf("the row paints a last-commit age cell:\n%s", row)
	}
}

func TestHeaderAndRowsShareTheFirstColumnOffset(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 80

	header := stripANSI(headerSlot + headerColumns(m.width))
	if got := strings.Index(header, "NAME"); got != rowPrefixWidth {
		t.Errorf("the header's first column starts at %d, want %d: %q", got, rowPrefixWidth, header)
	}
	e := m.entries()[0]
	if e.kind != kindRepo {
		t.Fatalf("the fixture entry is not a repo: %+v", e)
	}
	row := stripANSI(m.renderRow(e.r, false, m.width))
	if got := strings.Index(row, e.r.project.Name); got != rowPrefixWidth {
		t.Errorf("the row's name starts at %d, want %d: %q", got, rowPrefixWidth, row)
	}
}

// ── Fetch glyph attached left of NAME ────────────────────────────────────────

func TestFetchSlotGlyphAndNameDoesNotShift(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})

	for _, c := range []struct {
		state, slot string
	}{
		{"", "    "},
		{"fetching", "  ⟳ "},
		{"failed", "  ✗ "},
	} {
		m.fetchStates = map[string]string{"/tmp/api": c.state}
		row := stripANSI(m.renderRow(m.rows()[0], false, m.width))
		if !strings.HasPrefix(row, c.slot) {
			t.Errorf("state %q: the row starts %q, want %q", c.state, row[:4], c.slot)
		}
		if got := utf8.RuneCountInString(row[:strings.Index(row, "api")]); got != rowPrefixWidth {
			t.Errorf("state %q: the NAME starts at %d, want %d (it must not shift)", c.state, got, rowPrefixWidth)
		}
	}
}

func TestFetchSlotFollowsTheRepoNotTheCursor(t *testing.T) {
	projects := []discovery.Project{proj("a-repo", "/tmp/a", true), proj("b-repo", "/tmp/b", true)}
	m := newTestModel(t, projects, map[string]gitstatus.Snapshot{"/tmp/a": snapClean(), "/tmp/b": snapClean()})
	m.fetchStates = map[string]string{"/tmp/a": "fetching"}

	m.cursor = 0
	beforeA := stripANSI(m.renderRow(m.rows()[0], false, m.width))
	m.cursor = 1
	rowA := stripANSI(m.renderRow(m.rows()[0], false, m.width))
	rowB := stripANSI(m.renderRow(m.rows()[1], false, m.width))
	if !strings.HasPrefix(rowA, "  ⟳ ") {
		t.Errorf("repo A lost its spinner when the cursor moved: %q", rowA)
	}
	if strings.HasPrefix(rowB, "  ⟳ ") {
		t.Errorf("repo B painted the spinner of A: %q", rowB)
	}
	if rowA != beforeA {
		t.Errorf("repo A's row changed with the cursor:\n before=%q\n after =%q", beforeA, rowA)
	}
}

func TestSubrowsAndHeadersKeepTheFetchSlot(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "feat/x"))
	p.PrimaryGroup = "backend"
	m := newTestModel(t, []discovery.Project{p}, st)
	m = cursorOn(t, m, "/tmp/multi")
	m, _ = press(m, "enter") // expands the worktrees

	var header, repo, sub string
	for _, e := range m.entries() {
		switch e.kind {
		case kindPrimary:
			header = stripANSI(m.renderEntry(e, false, m.width))
		case kindRepo:
			repo = stripANSI(m.renderEntry(e, false, m.width))
		case kindWorktree:
			sub = stripANSI(m.renderEntry(e, false, m.width))
		}
	}
	if header == "" || repo == "" || sub == "" {
		t.Fatalf("the fixture did not produce header/repo/subrow:\n%q\n%q\n%q", header, repo, sub)
	}
	for name, line := range map[string]string{"header": header, "repo": repo, "subrow": sub} {
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("the %s does not reserve the 4-cell prefix: %q", name, line)
		}
	}
	// The BRANCH column starts after the prefix plus NAME, so the subrow's branch aligns with the repo's.
	if a, b := strings.Index(repo, "main"), strings.Index(sub, "feat/x"); a != b {
		t.Errorf("the subrow's BRANCH starts at %d and the repo's at %d: %q / %q", b, a, sub, repo)
	}
}

// ── COMMITS panel ────────────────────────────────────────────────────────────

func TestCommitsPanelWidthGate(t *testing.T) {
	projects, states := fixtureProjects()
	for _, c := range []struct {
		width, panel, table int
		show                bool
	}{
		{117, 0, 117, false},
		{118, 0, 118, false},
		{119, 30, 88, true},
		{120, 31, 88, true}, // the cap: the table sits at its minimum
	} {
		m := newTestModel(t, projects, states)
		m.width = c.width
		lay := m.layout()
		if lay.showPanel != c.show {
			t.Errorf("width=%d: showPanel = %v, want %v", c.width, lay.showPanel, c.show)
		}
		flat := stripANSI(m.renderDashboard())
		if got := strings.Contains(flat, "╭ commits · "); got != c.show {
			t.Errorf("width=%d: commits panel drawn = %v, want %v:\n%s", c.width, got, c.show, flat)
		}
		if c.show {
			if lay.panelWidth != c.panel || lay.tableWidth != c.table {
				t.Errorf("width=%d: panel/table = %d/%d, want %d/%d", c.width, lay.panelWidth, lay.tableWidth, c.panel, c.table)
			}
			if got := fitColumns(lay.tableWidth - rowPrefixWidth - 2); got != len(tableColumns) {
				t.Errorf("width=%d: the split table shows %d columns, want the full %d", c.width, got, len(tableColumns))
			}
		} else {
			if lay.tableWidth != c.width {
				t.Errorf("width=%d: tableWidth = %d, want the full width", c.width, lay.tableWidth)
			}
			if strings.Contains(flat, "commits") {
				t.Errorf("width=%d: the commits of the cursor's repo are shown with no panel:\n%s", c.width, flat)
			}
		}
	}
}

// Past its floor the panel takes a third of the terminal, capped so the table keeps its columns.
func TestCommitsPanelGrowsWithTheWidth(t *testing.T) {
	projects, states := fixtureProjects()
	for _, c := range []struct {
		width, panel, table int
	}{
		{119, 30, 88},  // the floor: the old fixed width
		{121, 32, 88},  // the cap: the table at its minimum
		{168, 56, 111}, // the share: 168/3
		{400, 133, 266},
	} {
		m := newTestModel(t, projects, states)
		m.width = c.width
		lay := m.layout()
		if !lay.showPanel {
			t.Fatalf("width=%d: no panel: %+v", c.width, lay)
		}
		if lay.panelWidth != c.panel || lay.tableWidth != c.table {
			t.Errorf("width=%d: panel/table = %d/%d, want %d/%d", c.width, lay.panelWidth, lay.tableWidth, c.panel, c.table)
		}
	}
}

// The wider body is spent on the subject: at 213 the inner 69 leaves 51 runes before the ellipsis.
func TestCommitsPanelSubjectUsesTheWiderBody(t *testing.T) {
	long := strings.Repeat("subject", 10)
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": committedSnap(long)})
	m.width = 213
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "commits · api"))
	var line string
	for _, l := range panel {
		if strings.Contains(l, "subj") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("the commit subject is not painted:\n%v", panel)
	}
	if want := truncate(long, 51); !strings.Contains(line, want) {
		t.Errorf("the subject is not clipped at the wider body's 51 (want %q): %q", want, line)
	}
}

func TestPanelAbsentAt80x24(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width, m.height = 80, 24
	lay := m.layout()
	if lay.showPanel {
		t.Fatalf("the panel is drawn at 80x24: %+v", lay)
	}
	out := stripANSI(m.renderDashboard())
	for _, col := range []string{"NAME", "BRANCH", "Work Tree", "↑↓up"} {
		if !strings.Contains(out, col) {
			t.Errorf("the 4-column table misses %q:\n%s", col, out)
		}
	}
	if strings.Contains(out, "SYNC") {
		t.Errorf("SYNC fits at 80 and should not:\n%s", out)
	}
	if strings.Contains(out, "commits") {
		t.Errorf("a commits panel is drawn at 80x24:\n%s", out)
	}
}

func TestTopBandIsRectangular(t *testing.T) {
	projects, states := fixtureProjects()
	for h := 1; h <= 40; h++ {
		m := newTestModel(t, projects, states)
		m.width, m.height = 119, h
		out := m.renderDashboard()
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != 119 {
				t.Errorf("h=%d line %d width = %d, want 119: %q", h, i, w, ansi.Strip(l))
			}
		}
		lay := m.layout()
		if !lay.showPanel {
			t.Fatalf("h=%d: the panel should be drawn at width 119", h)
		}
		entries := m.entries()
		want := lay.bodyLines + tableChrome
		if got := len(strings.Split(m.tableSection(lay.bodyLines, entries, lay.tableWidth), "\n")); got != want {
			t.Errorf("h=%d: the table box measures %d lines, want %d", h, got, want)
		}
		if got := len(strings.Split(m.panelSection(lay, entries), "\n")); got != want {
			t.Errorf("h=%d: the panel box measures %d lines, want the table's %d", h, got, want)
		}
	}
}

func TestPanelListsTheCommits(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": committedSnap("newest", "older", "oldest")})
	m.width = 119
	lay := m.layout()
	if !lay.showPanel {
		t.Fatalf("no panel at width 119: %+v", lay)
	}
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "commits · api"))
	if len(panel) != lay.bodyLines+1 {
		t.Errorf("the panel paints %d lines, want the table's body rows + 1 = %d", len(panel), lay.bodyLines+1)
	}
	if !strings.Contains(panel[0], "main (current)") {
		t.Errorf("the panel does not label the current branch group:\n%v", panel)
	}
	if !strings.Contains(panel[1], "newest") || !strings.Contains(panel[2], "older") || !strings.Contains(panel[3], "oldest") {
		t.Errorf("the commits are not newest first under their group:\n%v", panel)
	}
}

// The sync branch opens a second labelled group, below the current one.
func TestPanelGroupsCurrentAndSync(t *testing.T) {
	snap := committedSnap("c1", "c2")
	snap.SyncBranch = "master"
	snap.SyncCommits = []gitstatus.Commit{{Sha: "1234567", When: time.Now().Add(-time.Hour).Unix(), Subject: "s1"}}
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snap})
	m.width = 119
	box := sectionContent(t, stripANSI(m.View().Content), "commits · api")
	for _, want := range []string{"main (current)", "c1", "master (sync)", "s1"} {
		if !strings.Contains(box, want) {
			t.Errorf("the panel does not paint %q:\n%s", want, box)
		}
	}
	if strings.Index(box, "main (current)") > strings.Index(box, "master (sync)") {
		t.Errorf("the sync group is not below the current one:\n%s", box)
	}
}

// The group is only born with the repo declaring a DIFFERENT sync ref and having its commits: on
// the sync branch itself the two lists would duplicate, and without commits the label lies.
func TestPanelSyncGroupOnlyWhenItHasCommits(t *testing.T) {
	commits := []gitstatus.Commit{{Sha: "1234567", When: time.Now().Add(-time.Hour).Unix(), Subject: "s1"}}
	cases := []struct {
		name              string
		syncBranch        string
		injectSyncCommits bool
	}{
		{"on the sync branch itself", "main", true},
		{"the ref has no commits", "master", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snap := committedSnap("c1")
			snap.SyncBranch = c.syncBranch
			if c.injectSyncCommits {
				snap.SyncCommits = commits
			}
			m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
				map[string]gitstatus.Snapshot{"/tmp/api": snap})
			m.width = 119
			box := sectionContent(t, stripANSI(m.View().Content), "commits · api")
			if strings.Contains(box, "(sync)") {
				t.Errorf("a sync group was painted:\n%s", box)
			}
			if !strings.Contains(box, "main (current)") {
				t.Errorf("the current group lost its label:\n%s", box)
			}
		})
	}
}

// A detached HEAD without a sha has no branch name to paint: the group falls back to its tag only.
func TestPanelUnnamedBranchGroup(t *testing.T) {
	snap := committedSnap("c1")
	snap.Status.Branch = ""
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snap})
	m.width = 119
	box := sectionContent(t, stripANSI(m.View().Content), "commits · api")
	if !strings.Contains(box, "current") {
		t.Errorf("the group lost its label:\n%s", box)
	}
	if strings.Contains(box, "(current)") {
		t.Errorf("a group without a branch name was labelled as if it had one:\n%s", box)
	}
}

// The height is shared between the groups, and each one always keeps room for one commit: the
// last group cannot be clipped away by the first.
func TestPanelSharesTheHeightBetweenGroups(t *testing.T) {
	snap := committedSnap("c1", "c2", "c3")
	snap.SyncBranch = "master"
	snap.SyncCommits = []gitstatus.Commit{
		{Sha: "aaaaaaa", When: time.Now().Add(-time.Hour).Unix(), Subject: "s1"},
		{Sha: "bbbbbbb", When: time.Now().Add(-time.Hour).Unix(), Subject: "s2"},
	}
	m := newTestModel(t, nil, nil)
	for _, c := range []struct {
		rows           int
		wantC2, wantS2 bool
	}{
		{4, false, false}, // (4-2)/2 = 1 per group
		{6, true, true},   // (6-2)/2 = 2 per group
		{2, false, false}, // the floor of one per group, with no room to spare
	} {
		out := stripANSI(m.commitsPanel(snap, c.rows, 28))
		if !strings.Contains(out, "c1") || !strings.Contains(out, "s1") {
			t.Errorf("rows=%d: a group lost its first commit:\n%s", c.rows, out)
		}
		if got := strings.Contains(out, "c2"); got != c.wantC2 {
			t.Errorf("rows=%d: c2 painted = %v, want %v:\n%s", c.rows, got, c.wantC2, out)
		}
		if got := strings.Contains(out, "s2"); got != c.wantS2 {
			t.Errorf("rows=%d: s2 painted = %v, want %v:\n%s", c.rows, got, c.wantS2, out)
		}
	}
}

func TestPanelTruncatesTheSubjectNeverWraps(t *testing.T) {
	long := strings.Repeat("subject", 20)
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": committedSnap(long)})
	m.width = 119
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "commits · api"))
	line := ""
	for _, l := range panel {
		if strings.Contains(l, "subj") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("the commit subject is not painted:\n%v", panel)
	}
	if !strings.HasSuffix(strings.TrimRight(line, " "), "…") {
		t.Errorf("the long subject is not truncated: %q", line)
	}
}

func TestPanelEmptyCommitsPlaceholder(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m.width = 119
	box := sectionContent(t, stripANSI(m.View().Content), "commits · api")
	if !strings.Contains(box, "no commits") {
		t.Errorf("the empty panel does not say so:\n%s", box)
	}
}

func TestPanelGroupHeaderShowsTheAggregate(t *testing.T) {
	m := previewModel(t)
	m.width = 119
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindPrimary {
		t.Fatalf("the cursor is not on a primary header: %+v", e)
	}
	box := sectionContent(t, stripANSI(m.View().Content), "commits · vsocial")
	if !strings.Contains(box, "repos    3") {
		t.Errorf("the panel does not aggregate the group:\n%s", box)
	}
	if !strings.Contains(box, "dirty    1") || !strings.Contains(box, "behind   1") {
		t.Errorf("the panel does not paint the group state:\n%s", box)
	}
	if strings.Contains(box, "ahead") {
		t.Errorf("the panel paints a state that is not there:\n%s", box)
	}
}

func TestPanelWorktreeShowsItsOwnCommits(t *testing.T) {
	main := discovery.Project{Path: "/tmp/multi", Name: "multi", HasRepo: true}
	mainSnap := committedSnap("PARENT")
	mainSnap.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-marked", "feat")}
	child := discovery.Project{Path: "/tmp/wt-marked", Name: "wt-marked", HasRepo: true, IsWorktree: true, MainRepo: "/tmp/multi"}
	childSnap := committedSnap("CHILD")
	m := newTestModel(t, []discovery.Project{main, child}, map[string]gitstatus.Snapshot{
		"/tmp/multi":     mainSnap,
		"/tmp/wt-marked": childSnap,
	})
	m.width = 119
	m, _ = press(m, "enter")
	m = cursorOn(t, m, "/tmp/wt-marked")

	flat := stripANSI(m.View().Content)
	if !strings.Contains(flat, "CHILD") {
		t.Errorf("the panel does not paint the worktree's commits:\n%s", flat)
	}
	if strings.Contains(flat, "PARENT") {
		t.Errorf("the panel paints the parent repo's commits:\n%s", flat)
	}

	// No live snapshot for that path: a dim placeholder, never the parent's commits.
	other := tableEntry{kind: kindWorktree, wt: gitstatus.Worktree{Path: "/tmp/undiscovered-wt", Branch: "feat"}}
	if got := stripANSI(m.worktreeCommitsPanel(other, 10, 28)); !strings.Contains(got, "no commits") {
		t.Errorf("without a snapshot the panel = %q, want the placeholder", got)
	}
}

func TestPanelWithoutRowKeepsTheEmptyHint(t *testing.T) {
	m := previewModel(t)
	m.width = 119
	m.search = "zzz-no-match"
	m.clampCursor()
	box := sectionContent(t, stripANSI(m.View().Content), "commits")
	if !strings.Contains(box, "no repositories match") {
		t.Errorf("the panel does not use the shared empty hint:\n%s", box)
	}
}

func TestLogReplacesTheWholeBody(t *testing.T) {
	m := previewModel(t)
	m.width = 119
	m, _ = press(m, "l")
	if !m.logOpen {
		t.Fatal("l did not open the log")
	}
	flat := stripANSI(m.renderDashboard())
	if strings.Contains(flat, "╭ commits · ") {
		t.Errorf("the commits panel is drawn with the log open:\n%s", flat)
	}
	if strings.Contains(flat, "╭ api · ") {
		t.Errorf("the card is drawn with the log open:\n%s", flat)
	}
}

// ── Detail card ──────────────────────────────────────────────────────────────

func cardModel(t *testing.T) (Model, row) {
	t.Helper()
	snap := snapDirty(1, 1)
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
	m, r := detailRowWith(t, "/tmp/api", snap)
	m.width = 80
	return m, r
}

func TestCardIsTwoColumns(t *testing.T) {
	m, _ := cardModel(t)
	m = cursorOn(t, m, "/tmp/api")
	lay := m.layout()
	if !lay.cardSplit {
		t.Fatalf("width 80: the card did not split: %+v", lay)
	}
	box := sectionContent(t, stripANSI(m.View().Content), "api")
	for _, field := range []string{"path", "branch", "upstream", "state", "sync", "activity"} {
		if !strings.Contains(box, field) {
			t.Errorf("the left column misses the field %q:\n%s", field, box)
		}
	}
	if !strings.Contains(box, "files (1)") || !strings.Contains(box, "main.go") {
		t.Errorf("the right column misses the files list:\n%s", box)
	}
	if !strings.Contains(box, "worktrees (1)") {
		t.Errorf("the right column misses the worktrees list:\n%s", box)
	}
}

func TestCardCollapsesWhenNarrow(t *testing.T) {
	m, r := cardModel(t)
	m.width = 63
	if !m.layout().cardSplit {
		t.Error("width 63: the card should split")
	}
	m.width = 62
	if m.layout().cardSplit {
		t.Error("width 62: the card should collapse")
	}

	split := lineWith(m.renderDetail(r, 40, 63, true), "path")
	if !strings.Contains(split, "│") {
		t.Errorf("at width 63 the card is not split (no column separator): %q", split)
	}
	collapsed := lineWith(m.renderDetail(r, 40, 62, false), "path")
	if strings.Contains(collapsed, "│") {
		t.Errorf("at width 62 the card still shows the separator: %q", collapsed)
	}
}

func lineWith(content, needle string) string {
	for _, l := range strings.Split(content, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

func TestCardCollapsedStacksListsWithTheirWarnings(t *testing.T) {
	// One worktree fits whole and the files list then gets the remainder: the files header fits
	// with no room for an item or a warning.
	t.Run("worktrees whole then files with no room", func(t *testing.T) {
		snap := snapClean()
		snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
		for i := 0; i < 20; i++ {
			snap.Files = append(snap.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("f%d.go", i)})
		}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+5, 62, false))
		if !strings.Contains(out, "worktrees (1)") || !strings.Contains(out, "files (20)") {
			t.Errorf("the collapsed card does not stack both lists:\n%s", out)
		}
		if strings.Contains(out, "… ") {
			t.Errorf("the files list warned with no room for an item:\n%s", out)
		}
	})

	// An oversized worktrees list spends the whole budget, so the files list cannot even paint
	// its header; the exact warning pins the shared budget arithmetic.
	t.Run("oversized worktrees starve the files header", func(t *testing.T) {
		snap := snapClean()
		for i := 0; i < 10; i++ {
			snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{Path: "/tmp/api/wt", Branch: fmt.Sprintf("b%d", i), Head: "abc1234"})
		}
		for i := 0; i < 10; i++ {
			snap.Files = append(snap.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("f%d.go", i)})
		}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+7, 62, false))
		if !strings.Contains(out, "worktrees (10)") || !strings.Contains(out, "… 6 more") {
			t.Errorf("the collapsed card does not announce the omitted worktrees:\n%s", out)
		}
		if strings.Contains(out, "files (10)") {
			t.Errorf("the files list painted after the worktrees spent the budget:\n%s", out)
		}
	})

	t.Run("oversized files warn", func(t *testing.T) {
		snap := snapClean()
		for i := 0; i < 20; i++ {
			snap.Files = append(snap.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("f%d.go", i)})
		}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+4, 62, false))
		if !strings.Contains(out, "files (20)") || !strings.Contains(out, "… 19 more") {
			t.Errorf("the collapsed card does not announce the omitted files:\n%s", out)
		}
	})

	t.Run("empty worktrees paints nothing", func(t *testing.T) {
		snap := snapClean()
		snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+2, 62, false))
		if strings.Contains(out, "worktrees") {
			t.Errorf("an empty worktrees list painted a header:\n%s", out)
		}
	})

	t.Run("worktrees fit at the minimum budget", func(t *testing.T) {
		snap := snapClean()
		snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines, 62, false))
		if !strings.Contains(out, "worktrees (1)") {
			t.Errorf("at the minimum budget the worktrees header is missing:\n%s", out)
		}
	})

	t.Run("files clipped to the left column width", func(t *testing.T) {
		run := strings.Repeat("x", 150)
		snap := snapClean()
		snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: run + ".go"}}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+3, 62, false))
		var line string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, ".M") && strings.Contains(l, "x") {
				line = strings.TrimSuffix(l, "…")
				break
			}
		}
		// cardLeftWidth - 8 = 28 leaves a literal run of 27 before the ellipsis.
		if got := trailingRun(line, 'x'); got != 27 {
			t.Errorf("the collapsed file paints a run of %d, want the literal 27:\n%s", got, out)
		}
	})

	t.Run("empty files paints nothing", func(t *testing.T) {
		m, r := detailRowWith(t, "/tmp/api", snapClean())
		out := stripANSI(m.renderDetail(r, detailHeadLines+2, 62, false))
		if strings.Contains(out, "files") {
			t.Errorf("an empty files list painted a header:\n%s", out)
		}
	})

	t.Run("files fit at the minimum budget", func(t *testing.T) {
		snap := snapClean()
		snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
		m, r := detailRowWith(t, "/tmp/api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines, 62, false))
		if !strings.Contains(out, "files (1)") {
			t.Errorf("at the minimum budget the files header is missing:\n%s", out)
		}
	})

	t.Run("uncomparable path falls back to the absolute", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "feature")
		snap := snapClean()
		snap.Worktrees = []gitstatus.Worktree{{Path: abs, Branch: "feature", Head: "abc1234"}}
		m, r := detailRowWith(t, "api", snap)
		out := stripANSI(m.renderDetail(r, detailHeadLines+4, 62, false))
		if want := string([]rune(abs)[:19]) + "…"; !strings.Contains(out, want) {
			t.Errorf("the collapsed card did not clip the absolute path %q at its floor:\n%s", abs, out)
		}
	})
}

// The right column paints a list header exactly when its budget reaches minListBlockLines.
func TestCardSplitListBudgetBoundary(t *testing.T) {
	only := func(mutate func(*gitstatus.Snapshot)) (Model, row) {
		snap := snapClean()
		mutate(&snap)
		return detailRowWith(t, "/tmp/api", snap)
	}
	mw, rw := only(func(s *gitstatus.Snapshot) {
		s.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
	})
	if !strings.Contains(stripANSI(mw.renderDetail(rw, minListBlockLines, 120, true)), "worktrees (1)") {
		t.Error("at rows=minListBlockLines the worktrees header is missing")
	}
	if strings.Contains(stripANSI(mw.renderDetail(rw, minListBlockLines-1, 120, true)), "worktrees (1)") {
		t.Error("below minListBlockLines the worktrees header is painted anyway")
	}
	mf, rf := only(func(s *gitstatus.Snapshot) {
		s.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	})
	if !strings.Contains(stripANSI(mf.renderDetail(rf, minListBlockLines, 120, true)), "files (1)") {
		t.Error("at rows=minListBlockLines the files header is missing")
	}
	if strings.Contains(stripANSI(mf.renderDetail(rf, minListBlockLines-1, 120, true)), "files (1)") {
		t.Error("below minListBlockLines the files header is painted anyway")
	}
}

func TestPROverlayReplacesTheWholeBody(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m.width = 119
	if !formPainted(t, m) {
		t.Fatal("the PR form is not painted")
	}
	flat := stripANSI(m.renderDashboard())
	if strings.Contains(flat, "╭ commits · ") {
		t.Errorf("the commits panel is drawn with the PR overlay open:\n%s", flat)
	}
	if strings.Contains(flat, "╭ dirty-api ") {
		t.Errorf("the card is drawn with the PR overlay open:\n%s", flat)
	}
}

// The collapsed card (width < 63) keeps the ! prompt at its end even when the content overflows.
func TestBangInputVisibleInCollapsedCard(t *testing.T) {
	m := previewModel(t)
	m.width, m.height = 62, 30
	if m.layout().cardSplit {
		t.Fatalf("precondition: width 62 must collapse the card")
	}
	m = cursorOn(t, m, "/tmp/api")
	s := m.states["/tmp/api"]
	for i := 0; i < 20; i++ {
		s.Files = append(s.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/f%02d.go", i)})
	}
	m.states["/tmp/api"] = s

	m, _ = press(m, "!")
	if !m.cmdOpen {
		t.Fatal("! did not open the input")
	}
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "api · vsocial/backend"))
	if last := lastNonEmpty(panel); !strings.HasPrefix(last, "! ") {
		t.Errorf("the collapsed card's last line is %q, want the input's prompt\n%v", last, panel)
	}
}

func TestCardActivityField(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	r, _ := m.selected()
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "activity") || !strings.Contains(out, "2h") {
		t.Errorf("the card does not paint the activity field:\n%s", out)
	}
}

func TestCardFloorMovesWithActivity(t *testing.T) {
	if l := layoutTest(21, false, defaultHintLines, false); l.previewLines != 0 {
		t.Errorf("h=21: previewLines = %d, want 0 (the 6-line head raises the floor)", l.previewLines)
	}
	if l := layoutTest(22, false, defaultHintLines, false); l.previewLines != detailHeadLines {
		t.Errorf("h=22: previewLines = %d, want %d", l.previewLines, detailHeadLines)
	}
}

func TestTinyTerminalDoesNotPanic(t *testing.T) {
	projects, states := fixtureProjects()
	for _, c := range []struct{ w, h int }{{1, 1}, {2, 2}, {10, 3}} {
		m := newTestModel(t, projects, states)
		m.width, m.height = c.w, c.h
		_ = m.renderDashboard()
		lay := m.layout()
		if lay.showPanel {
			t.Errorf("%dx%d: the panel is drawn with no room", c.w, c.h)
		}
		if lay.cardSplit {
			t.Errorf("%dx%d: the card splits with no room", c.w, c.h)
		}
	}
}
