package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

func rowDe(name string, state gitstatus.State, lastCommit int64) row {
	return row{
		project: discovery.Project{Path: "/tmp/" + name, Name: name, HasRepo: true},
		snap:    gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", HasUpstream: true}, LastCommit: lastCommit},
		state:   state,
	}
}

func namesOf(rows []row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.project.Name)
	}
	return out
}

// The comparator has to be a STRICT ORDER: if two rows tied on all three keys and it said "less" in both directions, the insertion sort would swap them and the order would stop being stable across scans.
func TestSortRowsAttentionFirst(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name string
		rows []row
		want []string
	}{
		{
			name: "the score wins over the date",
			rows: []row{
				rowDe("clean-recent", gitstatus.StateClean, now),
				rowDe("dirty-old", gitstatus.StateDirty, now-100000),
			},
			want: []string{"dirty-old", "clean-recent"},
		},
		{
			name: "with the same score, the more recent commit",
			rows: []row{
				rowDe("old", gitstatus.StateClean, now-100000),
				rowDe("new", gitstatus.StateClean, now),
			},
			want: []string{"new", "old"},
		},
		{
			name: "with the same score and date, the name",
			rows: []row{
				rowDe("zeta", gitstatus.StateClean, now),
				rowDe("alpha", gitstatus.StateClean, now),
			},
			want: []string{"alpha", "zeta"},
		},
		{
			name: "the name ignores case",
			rows: []row{
				rowDe("Zeta", gitstatus.StateClean, now),
				rowDe("alpha", gitstatus.StateClean, now),
			},
			want: []string{"alpha", "Zeta"},
		},
		{
			name: "a total tie keeps the input order",
			rows: []row{
				rowDe("same", gitstatus.StateClean, now),
				rowDe("same", gitstatus.StateClean, now),
			},
			want: []string{"same", "same"},
		},
	}
	for _, c := range cases {
		rows := append([]row(nil), c.rows...)
		sortRows(rows)
		if got := namesOf(rows); !equalStrings(got, c.want) {
			t.Errorf("%s: order = %v, want %v", c.name, got, c.want)
		}
	}

	for _, n := range []int{0, 1, 3, 5, 9, 17} {
		t.Run(fmt.Sprintf("%d rows already sorted", n), func(t *testing.T) {
			rows := make([]row, 0, n)
			for i := range n {
				rows = append(rows, rowDe(fmt.Sprintf("r%02d", i), gitstatus.StateDirty, now-int64(i)))
			}
			sortRows(rows)
			want := namesOf(append([]row(nil), rows...))
			for i, got := range namesOf(rows) {
				if got != want[i] {
					t.Fatalf("n=%d: row %d ended up as %q, want %q (the list is not sorted)", n, i, got, want[i])
				}
			}
		})
		t.Run(fmt.Sprintf("%d rows reversed", n), func(t *testing.T) {
			rows := make([]row, 0, n)
			for i := range n {
				rows = append(rows, rowDe(fmt.Sprintf("r%02d", i), gitstatus.StateDirty, now-int64(n-i)))
			}
			sortRows(rows)
			for i, got := range namesOf(rows) {
				if want := fmt.Sprintf("r%02d", n-1-i); got != want {
					t.Fatalf("n=%d: row %d ended up as %q, want %q", n, i, got, want)
				}
			}
		})
	}

	a := rowDe("x", gitstatus.StateClean, now)
	b := rowDe("x", gitstatus.StateClean, now)
	if rowLess(a, b) && rowLess(b, a) {
		t.Error("rowLess says 'less' in both directions with identical rows")
	}
}

// The edge (a column that fills the width exactly) is where a `> ` in the wrong place hides a column that does fit.
func TestFitColumnsFitExact(t *testing.T) {
	total := 0
	for _, c := range tableColumns {
		total += c.width
	}
	if total != colName+colBranch+colWT+colUpDown+colSync {
		t.Fatalf("precondition: the total width is %d", total)
	}

	if got := fitColumns(0); got != 1 {
		t.Errorf("inner=0: %d columns, want 1 (NAME never drops)", got)
	}
	if got := fitColumns(-10); got != 1 {
		t.Errorf("inner=-10: %d columns, want 1", got)
	}
	sum := 0
	for i, c := range tableColumns {
		sum += c.width
		if got := fitColumns(sum); got != i+1 {
			t.Errorf("inner=%d (width of %s exact): %d columns, want %d", sum, c.title, got, i+1)
		}
		if want := max(1, i); fitColumns(sum-1) != want {
			t.Errorf("inner=%d (one cell less than %s): %d columns, want %d", sum-1, c.title, fitColumns(sum-1), want)
		}
	}
	if got := fitColumns(total); got != len(tableColumns) {
		t.Errorf("inner=%d: %d columns, want %d", total, got, len(tableColumns))
	}
	if got := fitColumns(total + 50); got != len(tableColumns) {
		t.Errorf("inner=%d: %d columns, want %d", total+50, got, len(tableColumns))
	}
}

// The deducted width (borders and the 4-cell row prefix) is 6: adding it instead of subtracting would teach a narrow terminal columns that overflow the border. The column counts are literal and not recomputed with fitColumns(width-6).
func TestHeaderUsesTheWidthInterior(t *testing.T) {
	for _, c := range []struct {
		width int
		cols  int
	}{
		{24, 1},
		{30, 1},
		{40, 1},
		{55, 1},
		{60, 2},
		{80, 4},
		{86, 4},
		{120, 5},
		{200, 5},
	} {
		var expected strings.Builder
		for _, tc := range tableColumns[:c.cols] {
			expected.WriteString(pad(tc.title, tc.width))
		}
		if got := headerColumns(c.width); got != expected.String() {
			t.Errorf("width=%d: header = %q, want %q (%d columns)", c.width, got, expected.String(), c.cols)
		}
	}
}

// A "0" on the left of the separator looks like data and is not.
func TestDirtyTailOnlySetsWhatIsThere(t *testing.T) {
	cases := []struct {
		name            string
		tracked, untked int
		want            string
	}{
		{"clean", 0, 0, ""},
		{"only tracked", 2, 0, "2"},
		{"only untracked", 0, 3, "?3"},
		{"both", 2, 3, "2 ?3"},
	}
	for _, c := range cases {
		s := gitstatus.Status{TrackedChanges: c.tracked, Untracked: c.untked}
		if got := dirtyTail(row{snap: gitstatus.Snapshot{Status: s}}); got != c.want {
			t.Errorf("%s: dirtyTail = %q, want %q", c.name, got, c.want)
		}
	}
}

// An empty cell must not wear the state colour: that is the difference between a quiet table and one hinting at something.
func TestWtCellEmptyNotInheritsTheColorOfTheState(t *testing.T) {
	m := newTestModel(t, nil, nil)
	cases := []struct {
		name  string
		state gitstatus.State
		want  string
	}{
		{"ahead unchanged", gitstatus.StateAhead, ""},
		{"behind unchanged", gitstatus.StateBehind, ""},
		{"clean", gitstatus.StateClean, ""},
		{"no-upstream unchanged", gitstatus.StateNoUpstream, ""},
	}
	for _, c := range cases {
		s := gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", Ahead: 2}}
		if c.state == gitstatus.StateBehind {
			s.Status = gitstatus.Status{Branch: "main", Behind: 2}
		}
		if c.state == gitstatus.StateNoUpstream {
			s.Status = gitstatus.Status{Branch: "main"}
		}
		got, style := m.wtCell(row{snap: s, state: c.state})
		if got != c.want {
			t.Errorf("%s: wtCell text = %q, want %q", c.name, got, c.want)
		}
		if want := styleClean.Render(c.want); style.Render(got) != want {
			t.Errorf("%s: wtCell style = %q, want the clean one %q", c.name, style.Render(got), want)
		}
	}
	s := gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", TrackedChanges: 2}}
	if got, style := m.wtCell(row{snap: s, state: gitstatus.StateDirty}); got != "2" || style.Render(got) != styleDirty.Render(got) {
		t.Errorf("dirty: wtCell = %q with style %q, want \"2\" with the dirty one", got, style.Render(got))
	}
}

func TestNameCellWarnsOfTheMarkerBroken(t *testing.T) {
	m := newTestModel(t, nil, nil)
	good := discovery.Project{Path: "/tmp/ok", Name: "ok", HasRepo: true}
	broken := discovery.Project{Path: "/tmp/broken", Name: "broken", HasRepo: true, MarkerErr: "line 3: invalid value"}

	if _, style := m.nameCell(row{project: good, state: gitstatus.StateClean}); style.Render("x") != styleSel.Render("x") {
		t.Error("a healthy marker must not carry the warning style")
	}
	name, style := m.nameCell(row{project: broken, state: gitstatus.StateClean})
	if style.Render(name) != styleWarn.Render(name) {
		t.Errorf("broken marker: style = %q, want the warning one", style.Render(name))
	}
	if !strings.Contains(name, "broken") {
		t.Errorf("broken marker: name = %q", name)
	}
}

// With the guard misplaced (accepting a zero) the "wt" would vanish from every summary.
func TestGroupStatsCountsTheWorktrees(t *testing.T) {
	conWt := row{project: discovery.Project{Path: "/a", PrimaryGroup: "g", HasRepo: true},
		snap: gitstatus.Snapshot{
			Status:    gitstatus.Status{Branch: "main"},
			Worktrees: []gitstatus.Worktree{{Path: "/a/w1"}, {Path: "/a/w2"}},
		},
		state: gitstatus.StateClean}
	sinWt := row{project: discovery.Project{Path: "/b", PrimaryGroup: "g", HasRepo: true},
		snap:  gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main"}},
		state: gitstatus.StateClean}

	m := newTestModel(t, []discovery.Project{conWt.project, sinWt.project},
		map[string]gitstatus.Snapshot{"/a": conWt.snap, "/b": sinWt.snap})
	m.collapsed = map[string]bool{"g": true} // collapsed: the aggregate does NOT change

	st := m.groupStats("g")
	if st.repos != 2 {
		t.Errorf("repos = %d, want 2", st.repos)
	}
	if st.worktrees != 2 {
		t.Errorf("worktrees = %d, want 2 (those of the repo with wt)", st.worktrees)
	}
	if st := m.groupStats(group.Ungrouped); st.worktrees != 0 {
		t.Errorf("group without worktrees = %d, want 0", st.worktrees)
	}
}

func TestRelativeTimeBuckets(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"future or epoch 0", -time.Hour, "-"},
		{"now", 0, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 3 * 24 * time.Hour, "3d"},
		{"weeks", 3 * 7 * 24 * time.Hour, "3w"},
		{"months", 3 * 30 * 24 * time.Hour, "3mo"},
		{"years", 400 * 24 * time.Hour, "13mo"},

		// The cases above sit at the CENTRE of each bucket, where no guard mutant reaches them (with the sign flipped 3h gives "0d", except "3mo", whose 90 days land in the months bucket either way); only the exact edges tell the sign apart.
		{"just before the hour", 59*time.Minute + 59*time.Second, "59m"},
		{"right at the hour", 60 * time.Minute, "1h"},
		{"just before the day", 23*time.Hour + 59*time.Minute, "23h"},
		{"right at the day", 24 * time.Hour, "1d"},
		{"just before the week", 7*24*time.Hour - time.Hour, "6d"},
		{"right at the week", 7 * 24 * time.Hour, "1w"},
		{"just before the month", 30*24*time.Hour - time.Hour, "4w"},
		{"right at the month", 30 * 24 * time.Hour, "1mo"},
	}
	for _, c := range cases {
		epoch := now.Add(-c.age).Unix()
		if c.want == "-" {
			epoch = 0
		}
		if got := relativeTime(epoch); got != c.want {
			t.Errorf("%s: relativeTime(-%s) = %q, want %q", c.name, c.age, got, c.want)
		}
	}
}

func TestPadFillsOnTheWidth(t *testing.T) {
	for _, c := range []struct {
		name, in string
		w        int
		want     string
	}{
		{"fills", "ab", 5, "ab   "},
		{"exact", "abcde", 5, "abcde"},
		{"width 0", "abc", 0, "abc"},
		{"negative width", "abc", -2, "abc"},
		{"empty", "", 3, "   "},
	} {
		if got := pad(c.in, c.w); got != c.want {
			t.Errorf("%s: pad(%q, %d) = %q, want %q", c.name, c.in, c.w, got, c.want)
		}
	}
}

func TestSyncOfAndNameResolveTheMatchingPath(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "alpha", SyncBranch: "main", HasRepo: true},
		{Path: "/b", Name: "beta", SyncBranch: "develop", HasRepo: true},
		{Path: "/c", Name: "gamma", HasRepo: true}, // no override: global
		{Path: "/d", Name: "delta", IsWorktree: true, HasRepo: true},
	}
	m := newTestModel(t, projects, map[string]gitstatus.Snapshot{})
	m.cfg.SyncBranch = "global"

	for _, c := range []struct{ path, sync, name string }{
		{"/a", "main", "alpha"},
		{"/b", "develop", "beta"},
		{"/c", "global", "gamma"},
		{"/d", "global", "d"}, // a worktree is named by its directory
		{"/undiscovered", "global", "undiscovered"},
		{"", "global", ""},
	} {
		if got, _ := m.syncOf(c.path); got != c.sync {
			t.Errorf("syncOf(%q) = %q, want %q", c.path, got, c.sync)
		}
		if got := m.nameOf(c.path); got != c.name {
			t.Errorf("nameOf(%q) = %q, want %q", c.path, got, c.name)
		}
	}
}

func TestSyncOfFallbackOnlyInTheDefault(t *testing.T) {
	cases := []struct {
		name     string
		project  discovery.Project
		global   string
		explicit bool
		want     string
		wantFB   bool
	}{
		{"undeclared default", discovery.Project{Path: "/a"}, "main", false, "main", true},
		{"declared marker", discovery.Project{Path: "/a", SyncBranch: "develop"}, "main", false, "develop", false},
		{"explicit global config", discovery.Project{Path: "/a"}, "release", true, "release", false},
		{"marker over the explicit global", discovery.Project{Path: "/a", SyncBranch: "develop"}, "release", true, "develop", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(t, []discovery.Project{c.project}, map[string]gitstatus.Snapshot{})
			m.cfg.SyncBranch = c.global
			m.cfg.SyncBranchExplicit = c.explicit
			got, gotFB := m.syncOf("/a")
			if got != c.want || gotFB != c.wantFB {
				t.Errorf("syncOf = (%q, %v), want (%q, %v)", got, gotFB, c.want, c.wantFB)
			}
		})
	}
}

// The header and the rows must PAINT THE SAME COLUMNS: if the row computed its width with a different criterion than the header, every datum would land under a column that does not exist (or the row would overflow the border, which is what the box hides). The header widths and the worktree row lengths are literal, not recomputed with fitColumns.
func TestHeaderAndRowsFitTheSameColumns(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	entries := m.entries()

	headerWidths := map[int]int{24: 26, 30: 26, 40: 26, 55: 26, 60: 50, 80: 70, 86: 70, 120: 82, 200: 82}
	for _, width := range []int{24, 30, 40, 55, 60, 80, 86, 120, 200} {
		m.width = width
		headerWidth := len([]rune(headerColumns(width)))
		if headerWidth != headerWidths[width] {
			t.Errorf("width=%d: the header composes %d cells, want the literal %d", width, headerWidth, headerWidths[width])
		}
		for _, e := range entries {
			if e.kind != kindRepo {
				continue
			}
			row := stripANSI(m.renderRow(e.r, false, width))
			if got := len([]rune(row)) - rowPrefixWidth; got != headerWidth {
				t.Errorf("width=%d: the row of %s composes %d cells and the header %d",
					width, e.r.project.Name, got, headerWidth)
			}
		}
	}

	// A worktree sub-row composes its own columns (with its indent and glyph), so it needs its own fixture: a model with no worktrees has no entries of that type and the check would look at nothing.
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	mw := newTestModel(t, []discovery.Project{p}, st)
	mw, _ = press(mw, "enter") // expands the worktrees
	var visible int
	for _, e := range mw.entries() {
		if e.kind != kindWorktree {
			continue
		}
		visible++
		// Literal total lengths (4-cell prefix + the columns that fit); a derived expectation would move with fitColumns(width-6). Width 86 pins the 82-cell boundary between the 4- and 5-column splits.
		wantLen := map[int]int{24: 30, 40: 30, 60: 54, 80: 74, 86: 74, 120: 86}
		for _, width := range []int{24, 40, 60, 80, 86, 120} {
			mw.width = width
			row := stripANSI(mw.renderWorktreeRow(e.wt, false, width))
			if got := len([]rune(row)); got != wantLen[width] {
				t.Errorf("width=%d: the worktree row measures %d, want the literal %d (4-cell prefix + columns): %q",
					width, got, wantLen[width], row)
			}
		}
	}
	if visible == 0 {
		t.Fatal("the fixture produced no worktree subrows: the check would be vacuous")
	}
}

func fixtureWithGroupsAndErrors() ([]discovery.Project, map[string]gitstatus.Snapshot) {
	projects := []discovery.Project{
		{Path: "/tmp/g-double", Name: "double", HasRepo: true, PrimaryGroup: "vsocial", SecondaryGroup: "backend"},
		{Path: "/tmp/g-only", Name: "only", HasRepo: true, PrimaryGroup: "vsocial"},
		{Path: "/tmp/g-without", Name: "without", HasRepo: true},
		{Path: "/tmp/g-error", Name: "broken", HasRepo: true, PrimaryGroup: "vsocial"},
		{Path: "/tmp/g-norepo", Name: "norepo", HasRepo: false, PrimaryGroup: "vsocial"},
		{Path: "/tmp/g-without-branch", Name: "nobranch", HasRepo: true, PrimaryGroup: "vsocial"},
	}
	states := map[string]gitstatus.Snapshot{
		"/tmp/g-double":         snapClean(),
		"/tmp/g-only":           snapClean(),
		"/tmp/g-without":        snapClean(),
		"/tmp/g-error":          {Status: gitstatus.Status{Branch: "main"}, Err: "fatal: I do not write"},
		"/tmp/g-norepo":         {},
		"/tmp/g-without-branch": {Status: gitstatus.Status{}},
	}
	return projects, states
}

func TestGroupLabelOfTwoLevels(t *testing.T) {
	cases := []struct {
		name string
		p    discovery.Project
		want string
	}{
		{"both levels", discovery.Project{PrimaryGroup: "vsocial", SecondaryGroup: "backend"}, "vsocial/backend"},
		{"primary only", discovery.Project{PrimaryGroup: "vsocial"}, "vsocial"},
		{"no group", discovery.Project{}, ""},
		{"secondary only is not shown (one level does not nest)", discovery.Project{SecondaryGroup: "backend"}, ""},
	}
	for _, c := range cases {
		if got := groupLabel(c.p); got != c.want {
			t.Errorf("%s: groupLabel = %q, want %q", c.name, got, c.want)
		}
	}
	if got := groupKey("vsocial", "backend"); got != "vsocial/backend" {
		t.Errorf("groupKey = %q, want vsocial/backend", got)
	}
}

func TestBranchCellWithoutBranchAndDetached(t *testing.T) {
	m := newTestModel(t, []discovery.Project{}, map[string]gitstatus.Snapshot{})
	for _, c := range []struct {
		name string
		r    row
		want string
	}{
		{"no repo", row{state: gitstatus.StateNoRepo}, "-"},
		{"repo with no branch (unborn HEAD)", row{state: gitstatus.StateClean}, "-"},
		{"normal branch", row{state: gitstatus.StateClean, snap: gitstatus.Snapshot{Status: gitstatus.Status{Branch: "feat/x"}}}, "feat/x"},
		{"detached", row{state: gitstatus.StateDetached, snap: gitstatus.Snapshot{Status: gitstatus.Status{Branch: "abc123", Detached: true}}}, "abc123 (detached)"},
	} {
		got, _ := m.branchCell(c.r)
		if got != c.want {
			t.Errorf("%s: branchCell = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGroupStatsCountsTheErrors(t *testing.T) {
	projects, states := fixtureWithGroupsAndErrors()
	m := newTestModel(t, projects, states)

	st := m.groupStats(groupKey("vsocial", ""))
	if st.repos != 4 {
		t.Errorf("repos = %d, want 4", st.repos)
	}
	if st.errors != 1 {
		t.Errorf("errors = %d, want 1 (the repo with gitstatus.Err does not count as clean)", st.errors)
	}
	if st := m.groupStats("nothing/here"); st.repos != 0 || st.errors != 0 {
		t.Errorf("nonexistent group = %+v, want everything at zero", st)
	}
}

// With the last band uncovered, an "8mo" could become "2400d" with nothing complaining.
func TestRelativeTimeWalksItsBands(t *testing.T) {
	now := time.Now().Unix()
	for _, c := range []struct {
		name string
		age  time.Duration
		want string
	}{
		{"just now", 0, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 2 * 24 * time.Hour, "2d"},
		{"weeks", 10 * 24 * time.Hour, "1w"},
		{"months", 60 * 24 * time.Hour, "2mo"},
		// A "3h" case does not tell a 24h threshold from a 23h one: it takes an age falling BETWEEN the two, which is exactly what a boundary mutant moves (the real age is the requested one plus the microseconds the test takes, so the cases stay below the threshold, never right on it).
		{"59s is still now", 59 * time.Second, "now"},
		{"30m is already minutes", 30 * time.Minute, "30m"},
		{"1h1m is still hours", time.Hour + time.Minute, "1h"},
		{"23h59m is still hours", 23*time.Hour + 59*time.Minute, "23h"},
		{"6d23h is still days", 6*24*time.Hour + 23*time.Hour, "6d"},
		{"7d1h is already weeks", 7*24*time.Hour + time.Hour, "1w"},
		{"29d23h is still weeks", 29*24*time.Hour + 23*time.Hour, "4w"},
		{"30d1h is already months", 30*24*time.Hour + time.Hour, "1mo"},
	} {
		if got := relativeTime(now - int64(c.age.Seconds())); got != c.want {
			t.Errorf("%s: relativeTime = %q, want %q", c.name, got, c.want)
		}
	}
	if got := relativeTime(0); got != "-" {
		t.Errorf("no epoch: relativeTime = %q, want -", got)
	}
	if got := relativeTime(-5); got != "-" {
		t.Errorf("negative epoch: relativeTime = %q, want -", got)
	}
}

func TestStripANSIOnlyRemovesSequences(t *testing.T) {
	if got := stripANSI("\x1b[31mred\x1b[0m"); got != "red" {
		t.Errorf("SGR: %q, want red", got)
	}
	if got := stripANSI("before\x1b[2Kafter"); got != "beforeafter" {
		t.Errorf("erase: %q, want beforeafter", got)
	}
	if got := stripANSI("no escapes"); got != "no escapes" {
		t.Errorf("no escapes: %q", got)
	}
	if got := stripANSI("x\x1b[1m"); got != "x" {
		t.Errorf("unterminated sequence: %q, want x", got)
	}
	if got := stripANSI("↑2 ↓0 ?q"); got != "↑2 ↓0 ?q" {
		t.Errorf("text with symbols: %q", got)
	}
}

// The whole map is checked against the style's NAME and not against another state: what matters is that a diverged repo is not painted as a dirty one, and comparing styles with each other would not say that (styleError and styleDirty would be "different" anyway).
func TestStyleForState(t *testing.T) {
	m := newTestModel(t, nil, nil)
	for _, c := range []struct {
		state gitstatus.State
		want  string
	}{
		{gitstatus.StateError, styleError.Render("x")},
		{gitstatus.StateNoRepo, styleDim.Render("x")},
		{gitstatus.StateNoUpstream, styleWarn.Render("x")},
		{gitstatus.StateDetached, styleWarn.Render("x")},
		{gitstatus.StateDiverged, styleDiverged.Render("x")},
		{gitstatus.StateDirty, styleDirty.Render("x")},
		{gitstatus.StateAhead, styleAhead.Render("x")},
		{gitstatus.StateBehind, styleBehind.Render("x")},
	} {
		got := m.styleFor(row{state: c.state})
		if got.Render("x") != c.want {
			t.Errorf("styleFor(%v) = %q, want %q", c.state, got.Render("x"), c.want)
		}
	}
}

func TestHeaderPrimaryChangesGlyphOnTheFold(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.projects = []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
	}
	if got := m.primaryHeaderLine("backend"); !strings.Contains(got, "▾") {
		t.Errorf("expanded header = %q, want the ▾ glyph", got)
	}
	m.collapsed = map[string]bool{"backend": true}
	got := m.primaryHeaderLine("backend")
	if !strings.Contains(got, "▸") || strings.Contains(got, "▾") {
		t.Errorf("collapsed header = %q, want the ▸ glyph", got)
	}
}

// Without worktreeHidden a synthetic worktree would make the status bar's count grow with every worktree.
func TestSummaryNotCountsWorktreesFolded(t *testing.T) {
	main := discovery.Project{Path: "/repos/api", Name: "api", PrimaryGroup: "backend", HasRepo: true}
	wt := discovery.Project{
		Path: "/repos/api-wt", Name: "api-wt",
		PrimaryGroup: "backend", HasRepo: true,
		IsWorktree: true, MainRepo: main.Path,
	}
	only := newTestModel(t, []discovery.Project{main}, map[string]gitstatus.Snapshot{
		main.Path: snapClean(),
	})
	conWt := newTestModel(t, []discovery.Project{main, wt}, map[string]gitstatus.Snapshot{
		main.Path: snapClean(),
		wt.Path:   snapClean(),
	})
	a, _, _, _ := only.summary()
	b, _, _, _ := conWt.summary()
	if a != b {
		t.Errorf("summary with a folded worktree = %d repos, want %d (it does not count)", b, a)
	}

	orphan := newTestModel(t, []discovery.Project{wt}, map[string]gitstatus.Snapshot{
		wt.Path: snapClean(),
	})
	c, _, _, _ := orphan.summary()
	if c != 1 {
		t.Errorf("summary with an orphan worktree = %d repos, want 1 (there is no main repo to hide it)", c)
	}
}

func TestRepoExpandedInducedForSearch(t *testing.T) {
	proj := discovery.Project{Path: "/api", Name: "api", HasRepo: true}
	snap := snapClean()
	snap.Worktrees = []gitstatus.Worktree{{Path: "/api-wt", Branch: "feature/x"}}
	m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: snap})
	r := row{project: proj, snap: snap}

	if m.repoExpanded(r) {
		t.Error("repoExpanded = true with no search nor expansion (want collapsed)")
	}
	m.expanded = map[string]bool{"/api": true}
	if !m.repoExpanded(r) {
		t.Error("repoExpanded = false with the repo marked expanded")
	}
	m.expanded = map[string]bool{}
	m.search = "feature"
	if !m.repoExpanded(r) {
		t.Error("repoExpanded = false with a search matching a worktree (it has to induce it)")
	}
	m.search = "this-worktree-does-not-exist"
	if m.repoExpanded(r) {
		t.Error("repoExpanded = true with a search matching no worktree")
	}
}

func TestWorktreeBranchLabel(t *testing.T) {
	for _, c := range []struct {
		wt   gitstatus.Worktree
		want string
	}{
		{gitstatus.Worktree{Branch: "feat/x"}, "feat/x"},
		{gitstatus.Worktree{Head: "abc1234"}, "(detached) abc1234"},
		{gitstatus.Worktree{}, "(detached)"},
	} {
		if got := worktreeBranchLabel(c.wt); got != c.want {
			t.Errorf("worktreeBranchLabel(%+v) = %q, want %q", c.wt, got, c.want)
		}
	}
}

// Only the EXACT thresholds kill the boundary mutants, and that age cannot be built with the clock inside relativeTime, which is why relativeAge takes the age ready-made; the sign matters both ways (at exactly 30 days "4w" and "1mo" are told apart by the number).
func TestRelativeAgeInTheThresholdExact(t *testing.T) {
	for _, c := range []struct {
		name string
		age  time.Duration
		want string
	}{
		{"1ns before the minute", time.Minute - time.Nanosecond, "now"},
		{"right at the minute", time.Minute, "1m"},
		{"1ns before the hour", time.Hour - time.Nanosecond, "59m"},
		{"right at the hour", time.Hour, "1h"},
		{"1ns before the day", 24*time.Hour - time.Nanosecond, "23h"},
		{"right at the day", 24 * time.Hour, "1d"},
		{"1ns before the week", 7*24*time.Hour - time.Nanosecond, "6d"},
		{"right at the week", 7 * 24 * time.Hour, "1w"},
		{"1ns before the month", 30*24*time.Hour - time.Nanosecond, "4w"},
		{"right at the month", 30 * 24 * time.Hour, "1mo"},
	} {
		if got := relativeAge(c.age); got != c.want {
			t.Errorf("%s: relativeAge(%s) = %q, want %q", c.name, c.age, got, c.want)
		}
	}
}

func TestRelativeTimeAnchorFollowsTheEpochCut(t *testing.T) {
	for _, epoch := range []int64{0, -1, -5} {
		if got := relativeTime(epoch); got != "-" {
			t.Errorf("relativeTime(%d) = %q, want -", epoch, got)
		}
	}
	if got := relativeTime(time.Now().Unix()); got == "-" {
		t.Error("relativeTime with a real epoch = \"-\", want the age bucket")
	}
}
