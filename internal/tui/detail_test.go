package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// They are the only two renderDetail lines whose text comes from the error instead of a repo datum, so a content test does not cover them by accident: without this `MarkerErr` or `snap.Err` could vanish from the card and no test would notice (they would be NOT COVERED, not survivors).
func TestCardPaintsTheErrorsOfTheRepo(t *testing.T) {
	cases := []struct {
		name      string
		markerErr string
		gitErr    string
		want      []string
		absent    []string
	}{
		{
			name:      "broken marker",
			markerErr: "line 3: invalid value",
			want:      []string{"marker:", "line 3: invalid value"},
			absent:    []string{"git:"},
		},
		{
			name:   "repo without git",
			gitErr: "fatal: not a git repository",
			want:   []string{"git:", "fatal: not a git repository"},
			absent: []string{"marker:"},
		},
		{
			name:      "both at once",
			markerErr: "bad",
			gitErr:    "broken",
			want:      []string{"marker:", "bad", "git:", "broken"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := "/tmp/api"
			snap := snapClean()
			snap.Err = c.gitErr
			p := discovery.Project{
				Path: path, Name: "api", PrimaryGroup: "vsocial",
				HasRepo: true, MarkerErr: c.markerErr,
			}
			m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{path: snap})
			r := row{project: p, snap: snap, state: snap.State(true)}

			out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("the card does not say %q:\n%s", w, out)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(out, a) {
					t.Errorf("the card says %q and should not (there is no such error):\n%s", a, out)
				}
			}
		})
	}
}

// The EXACT fits are what is looked at here, because a `> ` in the wrong place changes the answer precisely when the list fits exactly (and there no "N more" warning with N=0 should be left).
func TestListBudget(t *testing.T) {
	cases := []struct {
		name         string
		avail, items int
		wantShown    int
		wantRest     bool
	}{
		{"no room for the list", minListBlockLines - 1, 5, 0, false},
		{"no room and no items", 0, 0, 0, false},
		{"minimum room, no items", minListBlockLines, 0, 0, false},
		{"minimum room, one item (the gap fits)", minListBlockLines, 1, 0, false},
		{"minimum room, three items", minListBlockLines, 3, 0, false},
		{"one item line, fits exactly", minListBlockLines + 1, 1, 1, false},
		{"one item line, does not fit", minListBlockLines + 1, 2, 0, true},
		{"fits exactly at the limit", minListBlockLines + 3, 3, 3, false},
		{"one too many", minListBlockLines + 3, 4, 2, true},
		{"roomy", 20, 2, 2, false},
		{"very long", 10, 100, 7, true},
	}
	for _, c := range cases {
		shown, rest := listBudget(c.avail, c.items)
		if shown != c.wantShown || rest != c.wantRest {
			t.Errorf("%s: listBudget(%d, %d) = (%d, %v), want (%d, %v)",
				c.name, c.avail, c.items, shown, rest, c.wantShown, c.wantRest)
		}
	}
}

func TestListBudgetTheWarningCountsWhatIsMissing(t *testing.T) {
	for _, items := range []int{4, 10, 37} {
		avail := minListBlockLines + 3
		shown, rest := listBudget(avail, items)
		if !rest {
			t.Fatalf("items=%d: expected a warning with avail=%d", items, avail)
		}
		if missing := items - shown; shown+missing != items || missing <= 0 {
			t.Errorf("items=%d: shown=%d → missing %d, want %d (> 0 and shown+rest=items)",
				items, shown, items-shown, items-shown)
		}
	}
}

func TestActionTail(t *testing.T) {
	cases := []struct {
		name, out string
		n         int
		want      string
	}{
		{"empty", "", 3, ""},
		{"only newlines", "\n\n", 3, ""},
		{"one line", "text", 3, "text"},
		{"fits whole", "a\nb", 3, "a\nb"},
		{"fits exactly", "a\nb\nc", 3, "a\nb\nc"},
		{"clips from behind", "a\nb\nc\nd", 2, "c\nd"},
		{"more lines than fit", "a\nb\nc", 1, "c"},
		{"n=0", "a\nb", 0, ""},
		{"negative n", "a\nb", -1, ""},
		{"ignores the trailing newline", "a\nb\n", 5, "a\nb"},
	}
	for _, c := range cases {
		if got := actionTail(c.out, c.n); got != c.want {
			t.Errorf("%s: actionTail(%q, %d) = %q, want %q", c.name, c.out, c.n, got, c.want)
		}
	}
}

func TestAsOrDash(t *testing.T) {
	if got := asOrDash(""); got != "-" {
		t.Errorf("asOrDash(\"\") = %q, want -", got)
	}
	if got := asOrDash("main"); got != "main" {
		t.Errorf("asOrDash(main) = %q, want main", got)
	}
}

func TestFitLinesFillsAndClipToNot(t *testing.T) {
	cases := []struct {
		name, content string
		n             int
		wantFit       []string
		wantClip      []string
	}{
		{"fits exactly", "a\nb", 2, []string{"a", "b"}, []string{"a", "b"}},
		{"extra", "a\nb\nc", 2, []string{"a", "b"}, []string{"a", "b"}},
		{"missing", "a", 3, []string{"a", "", ""}, []string{"a"}},
		{"empty", "", 2, []string{"", ""}, []string{""}},
		{"n=0 with content", "a\nb", 0, []string{""}, []string{""}},
		{"n=0 empty", "", 0, []string{""}, []string{""}},
		{"negative n", "a", -1, []string{""}, []string{""}},
	}
	for _, c := range cases {
		if got := strings.Split(fitLines(c.content, c.n), "\n"); !equalStrings(got, c.wantFit) {
			t.Errorf("%s: fitLines(%q, %d) = %q, want %q", c.name, c.content, c.n, got, c.wantFit)
		}
		if got := strings.Split(clipTo(c.content, c.n), "\n"); !equalStrings(got, c.wantClip) {
			t.Errorf("%s: clipTo(%q, %d) = %q, want %q", c.name, c.content, c.n, got, c.wantClip)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func detailRowWith(t *testing.T, path string, snap gitstatus.Snapshot) (Model, row) {
	t.Helper()
	p := discovery.Project{Path: path, Name: "api", PrimaryGroup: "vsocial", HasRepo: true}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{path: snap})
	return m, row{project: p, snap: snap, state: snap.State(true)}
}

func TestCardClipsEachFieldAItsWidth(t *testing.T) {
	long := strings.Repeat("long", 30) // 120 chars, plenty to clip

	pathLargo := "/tmp/api/" + long
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: long + ".go"}}
	m, r := detailRowWith(t, pathLargo, snap)
	m.width = 120
	m.lastAction[pathLargo] = actionResult{kind: "pull_rebase", cmd: "git pull --rebase " + long, output: ""}

	// Literal widths, not the same max(...) the card computes: a derived expectation moves with the mutant.
	// At width 120 the detail box is 88 wide (inner 86), so the path clips at 78 and the argv at 88-30 = 58.
	out := stripANSI(m.renderDetail(r, 40, m.layout().cardWidth, m.layout().cardSplit))
	for _, c := range []struct {
		that string
		want string
	}{
		{"repo path", truncate(pathLargo, 78)},            // inner - 8
		{"argv", truncate("git pull --rebase "+long, 58)}, // box - 30
	} {
		if !strings.Contains(out, c.want) {
			t.Errorf("the %s is not clipped to its width (%q…):\n%s", c.that, c.want[:20], out)
		}
	}

	// The files live in their own box: 31 wide at 120 (inner 29), so the path clips at 24.
	files := stripANSI(m.filesList(r, 40, m.layout().filesWidth-2))
	if want := truncate(long+".go", 24); !strings.Contains(files, want) {
		t.Errorf("the file is not clipped to its box (%q…):\n%s", want[:20], files)
	}

	m2, r2 := detailRowWith(t, "/tmp/api", func() gitstatus.Snapshot {
		s := snapClean()
		s.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt/" + long, Branch: "feat", Head: "abc1234"}}
		return s
	}())
	m2.width = 120
	outWT := stripANSI(m2.renderDetail(r2, 40, m2.layout().cardWidth, m2.layout().cardSplit))
	if want := truncate("wt/"+long, 74); !strings.Contains(outWT, want) { // inner - 2 - wtBranchWidth
		t.Errorf("the worktree path is not clipped to its width (%q…):\n%s", want[:20], outWT)
	}
}

// The expectations are literal run lengths and NOT the same expression the card computes: a test that derives its expectation from the expression under test moves with the mutant and kills nothing.
func TestCardClipsTheListsToLiteralWidths(t *testing.T) {
	// ONE repeated rune, not a repeated word: with "longlong…" a suffix test only matches at multiples of the pattern's period, so a run one word longer would slip through.
	run := strings.Repeat("x", 150)
	m, r := detailRowWith(t, "/tmp/api", func() gitstatus.Snapshot {
		s := snapClean()
		s.Files = []gitstatus.FileEntry{{Code: ".M", Path: run + ".go"}}
		return s
	}())
	m.width = 120
	// The files box is 31 wide at width 120 (inner 29): the path gets inner-5 = 24 columns, so the clip leaves 23 x's before the ellipsis.
	out := stripANSI(m.filesList(r, 40, 29))
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, ".M") && strings.Contains(l, "x") {
			line = strings.TrimSuffix(l, "…")
			break
		}
	}
	if got := trailingRun(line, 'x'); got != 23 {
		t.Errorf("the file paints a run of %d, want its literal 23 (24 columns):\n%s", got, out)
	}
}

func trailingRun(s string, r rune) int {
	return len(s) - strings.LastIndexFunc(s, func(c rune) bool { return c != r }) - 1
}

// Literal widths: the lists column grows to 2/5 of the inner width above cardRightWidth, and the
// fields keep the rest above cardLeftWidth.
func TestCardColumns(t *testing.T) {
	for _, c := range []struct {
		inner, left, right int
	}{
		{61, 36, 24},   // the split's floor: a 63-cell terminal
		{118, 71, 46},  // a 120-cell terminal
		{246, 147, 98}, // an ultrawide one
	} {
		left, right := cardColumns(c.inner)
		if left != c.left || right != c.right {
			t.Errorf("cardColumns(%d) = (%d, %d), want (%d, %d)", c.inner, left, right, c.left, c.right)
		}
	}
}

// The split moves with the terminal: the detail box is what is left after the files box and the gap.
func TestCardSplitDividerFollowsTheWidth(t *testing.T) {
	for _, c := range []struct {
		width, detail, files int
	}{
		{63, 36, 26},   // the split's floor: the files box at its minimum
		{64, 37, 26},   // below the panel floor the share holds
		{120, 88, 31},  // the panel on: both boxes mirror the top band
		{246, 146, 99}, // ultrawide: the share grows
	} {
		snap := snapClean()
		snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
		m, _ := detailRowWith(t, "/tmp/api", snap)
		m.width = c.width
		lay := m.layout()
		if !lay.cardSplit {
			t.Fatalf("width=%d: the band did not split", c.width)
		}
		if lay.cardWidth != c.detail || lay.filesWidth != c.files {
			t.Errorf("width=%d: detail/files = %d/%d, want %d/%d", c.width, lay.cardWidth, lay.filesWidth, c.detail, c.files)
		}
	}
}

// Both boxes share the band's height: the files box is padded to the detail box's rows.
func TestCardTwoBoxesShareTheHeight(t *testing.T) {
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	m, _ := detailRowWith(t, "/tmp/api", snap)
	m.width, m.height = 120, 30
	m = cursorOn(t, m, "/tmp/api")
	lay := m.layout()
	out := stripANSI(m.View().Content)
	detail := panelLines(t, sectionContent(t, out, "api"))
	files := panelLines(t, sectionContent(t, out, "files (1)"))
	if len(detail) != lay.previewLines || len(files) != lay.previewLines {
		t.Errorf("detail/files boxes = %d/%d lines, want both %d", len(detail), len(files), lay.previewLines)
	}
	if lines := strings.Split(out, "\n"); len(lines) != m.height {
		t.Errorf("the view paints %d lines, want %d", len(lines), m.height)
	}
}
func TestCardCleanRepoPaintsNoSeparator(t *testing.T) {
	long := "/" + strings.Repeat("dir/", 25) + "repo" // 105 chars: whole only because the fields own the card
	m, r := detailRowWith(t, long, snapClean())
	m.width = 120
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if strings.Contains(out, "│") {
		t.Errorf("the clean card paints the separator:\n%s", out)
	}
	if !strings.Contains(out, long) {
		t.Errorf("the path is not painted whole with the right column empty:\n%s", out)
	}

	// The fields clip at the full inner width, not at the split's left column: 120-2-8 = 110.
	over := "/" + strings.Repeat("x", 150)
	m2, r2 := detailRowWith(t, over, snapClean())
	m2.width = 120
	out2 := stripANSI(m2.renderDetail(r2, 40, m2.width, m2.layout().cardSplit))
	if want := truncate(over, 110); !strings.Contains(out2, want) {
		t.Errorf("with no lists the path is not clipped at the full 110 cells (want %q):\n%s", truncate(want, 20), out2)
	}
}

// The tail (diagnostics, last action, `$ cmd`) keeps its last line at the bottom of the detail box.
func TestCardTailStaysAtTheBottom(t *testing.T) {
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	m, r := detailRowWith(t, "/tmp/api", snap)
	m.width = 120
	const rows = 12
	m.lastCmd["/tmp/api"] = cmdResult{command: "ls", output: "one\ntwo", exit: "0"}
	out := stripANSI(m.renderDetail(r, rows, m.width, m.layout().cardSplit))
	lines := strings.Split(out, "\n")
	if len(lines) > rows {
		t.Fatalf("the detail box paints %d lines, want at most %d:\n%s", len(lines), rows, out)
	}
	if last := lines[len(lines)-1]; last != "  two" {
		t.Errorf("the tail lost its last line at the bottom: %q\n%s", last, out)
	}
}

// The complaint that started this: at 120 cells the whole path and upstream fit, so cutting them
// there while the lists column has room left is wrong.
func TestCardWidePathAndUpstreamAreVisible(t *testing.T) {
	path := "/home/dev/work/visualco/assignator-api-vsocial"
	snap := snapClean()
	snap.Status.Upstream = "origin/deploy/SEP26Assignee"
	m, r := detailRowWith(t, path, snap)
	m.width = 120
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, path) {
		t.Errorf("the path is not painted whole at width 120:\n%s", out)
	}
	if !strings.Contains(out, "origin/deploy/SEP26Assignee") {
		t.Errorf("the upstream is not painted whole at width 120:\n%s", out)
	}
}

func TestCardShowsTheArgvOfTheLastAction(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	m, r := detailRowWith(t, path, snap)

	m.lastAction[path] = actionResult{kind: "pull_ai"}
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "last pull (AI)") && !strings.Contains(out, "AI") {
		t.Errorf("the no-argv variant of the action is missing:\n%s", out)
	}
	if strings.Contains(out, "git pull") {
		t.Errorf("it invented an argv where there is none:\n%s", out)
	}

	m.lastAction[path] = actionResult{kind: "pull_rebase", cmd: "git pull --rebase --autostash", output: ""}
	out = stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "git pull --rebase --autostash") {
		t.Errorf("the resolved argv does not appear in the card:\n%s", out)
	}
}

// The number in the warning is the difference between "you are missing 3" and a count that does not add up with the box title.
func TestCardTheWarningCountsTheMissingFiles(t *testing.T) {
	path := "/tmp/api"
	snap := snapDirty(0, 0)
	const total = 20
	for i := 0; i < total; i++ {
		snap.Files = append(snap.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/file%02d.go", i)})
	}
	m, r := detailRowWith(t, path, snap)

	rows := detailHeadLines + minListBlockLines + 5 // the files box's own budget
	out := stripANSI(m.filesSection(r, rows, 60))
	if !strings.Contains(out, fmt.Sprintf("files (%d)", total)) {
		t.Errorf("the title does not count the %d files:\n%s", total, out)
	}
	wantWarning := "… 10 more"
	if !strings.Contains(out, wantWarning) {
		t.Errorf("the warning does not say %q:\n%s", wantWarning, out)
	}
	if !strings.Contains(out, "file00.go") {
		t.Errorf("the first of the list was not painted:\n%s", out)
	}
	if strings.Contains(out, "file10.go") {
		t.Errorf("a file that did not fit was painted:\n%s", out)
	}
}

func TestCardListsPaintUnderTheSplit(t *testing.T) {
	path := "/tmp/api"
	snap := snapDirty(1, 1)
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: "fix"}}
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
	m, r := detailRowWith(t, path, snap)

	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "worktrees (1)") {
		t.Errorf("the detail box does not paint the worktrees list:\n%s", out)
	}
	if strings.Contains(out, "files (1)") {
		t.Errorf("the files list still lives in the detail box:\n%s", out)
	}
	if strings.Contains(out, "commits") {
		t.Errorf("the card paints the commits block, which left for the panel:\n%s", out)
	}
	if files := stripANSI(m.filesSection(r, detailHeadLines+minListBlockLines, m.width)); !strings.Contains(files, "files (1)") || !strings.Contains(files, "main.go") {
		t.Errorf("the files box does not paint the files list:\n%s", files)
	}
}

func TestCardBranchEmptyIsShowsAsDash(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Status.Branch = ""
	m, r := detailRowWith(t, path, snap)
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "branch  -") {
		t.Errorf("with no branch the card does not show the dash:\n%s", out)
	}
}

func TestCardNotPaintsListsEmpty(t *testing.T) {
	path := "/tmp/api"
	m, r := detailRowWith(t, path, snapClean())
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	for _, header := range []string{"files (", "worktrees (", "commits"} {
		if strings.Contains(out, header) {
			t.Errorf("a repo with no data painted the %q list:\n%s", header, out)
		}
	}
	if strings.Contains(out, "│") {
		t.Errorf("with no lists the card paints the separator:\n%s", out)
	}
	for _, field := range []string{"path", "branch", "upstream", "state", "sync", "activity"} {
		if !strings.Contains(out, field) {
			t.Errorf("the header field %q is missing:\n%s", field, out)
		}
	}
}

// The 3-line floor is a contract too: with the minimal card 3 lines are still visible, not 0.
func TestCardClipsTheQueueOfTheLastAction(t *testing.T) {
	path := "/tmp/api"
	prologue := strings.Repeat("line\n", 30)
	tailLines := "result1\nresult2\nresult3\nresult4\nresult5"

	t.Run("with room", func(t *testing.T) {
		m, r := detailRowWith(t, path, snapClean())
		m.lastAction[path] = actionResult{kind: "pull_rebase", output: prologue + tailLines + "\n"}
		out := stripANSI(m.renderDetail(r, detailHeadLines+10, m.width, m.layout().cardSplit))
		if !strings.Contains(out, "result5") || !strings.Contains(out, "result3") {
			t.Errorf("the end of the output is not visible:\n%s", out)
		}
		if strings.Contains(out, "result1") {
			t.Errorf("the tail swallowed the beginning of the output:\n%s", out)
		}
	})

	t.Run("with wide room", func(t *testing.T) {
		m, r := detailRowWith(t, path, snapClean())
		m.lastAction[path] = actionResult{kind: "pull_rebase", output: prologue + tailLines + "\n"}
		out := stripANSI(m.renderDetail(r, 44, m.width, m.layout().cardSplit))
		if !strings.Contains(out, "line") || !strings.Contains(out, "result5") {
			t.Errorf("with plenty of room the full output is not visible:\n%s", out)
		}
	})
}

func TestCardMinimumWorktreeClipsThePathAndRespectsTheBranch(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/multi/wt/"+strings.Repeat("long", 30), "feat/x"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.width = 60
	e := tableEntry{kind: kindWorktree, wt: gitstatus.Worktree{
		Path:   "/tmp/multi/wt/" + strings.Repeat("long", 30),
		Branch: "feat/x",
		Head:   "abc1234",
	}, parent: "/tmp/multi"}

	out := stripANSI(m.renderWorktreeDetail(e, 20, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "feat/x") {
		t.Errorf("the worktree branch is not visible:\n%s", out)
	}
	if strings.Contains(out, "(detached)") {
		t.Errorf("a worktree with a branch was painted as detached:\n%s", out)
	}
	want := truncate("/tmp/multi/wt/"+strings.Repeat("long", 30), max(20, 60-13))
	if !strings.Contains(out, want) {
		t.Errorf("the worktree path is not clipped to its width (want %d chars):\n%s", len(want), out)
	}
	for _, fake := range []string{"no-up", "clean", "state"} {
		if strings.Contains(out, fake) {
			t.Errorf("the minimal card invented %q:\n%s", fake, out)
		}
	}
}

func TestCardWorktreeWithPathRelative(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt-feat", Branch: "feat", Head: "abc1234"}}
	m, r := detailRowWith(t, path, snap)
	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "wt-feat") {
		t.Errorf("the worktree path is not visible:\n%s", out)
	}
	if strings.Contains(out, "/tmp/api/wt-feat") {
		t.Errorf("the worktree path was not made relative:\n%s", out)
	}
}

func TestCardTheListsOwnTheirBudgets(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	for i := 0; i < 40; i++ {
		snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
			Path: path + "/wt", Branch: "feat", Head: "abc1234",
		})
	}
	for i := 0; i < 40; i++ {
		snap.Files = append(snap.Files, gitstatus.FileEntry{
			Code: ".M", Path: fmt.Sprintf("pkg/f%d.go", i),
		})
	}
	for i := 0; i < 5; i++ {
		snap.Commits = append(snap.Commits, gitstatus.Commit{
			Sha: "abc1234", When: 1700000000, Subject: fmt.Sprintf("c%d", i),
		})
	}
	m, r := detailRowWith(t, path, snap)

	for rows := detailHeadLines; rows <= 40; rows++ {
		out := stripANSI(m.renderDetail(r, rows, m.width, m.layout().cardSplit))
		if got := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); got > rows {
			t.Errorf("rows=%d: the card paints %d lines, it overflows the box", rows, got)
		}
	}

	out := stripANSI(m.renderDetail(r, detailHeadLines+6, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "worktrees (40)") {
		t.Errorf("the detail box does not paint the worktrees list:\n%s", out)
	}
	// Independent budgets: an oversized worktrees list does not starve the files box of its own warning.
	if got := strings.Count(out, "… "); got != 1 {
		t.Errorf("the detail box paints %d truncation warnings, want 1:\n%s", got, out)
	}
	files := stripANSI(m.filesSection(r, detailHeadLines+6, m.width))
	if !strings.Contains(files, "files (40)") {
		t.Errorf("the files box does not paint the files list:\n%s", files)
	}
	if got := strings.Count(files, "… "); got != 1 {
		t.Errorf("the files box paints %d truncation warnings, want 1:\n%s", got, files)
	}
	if strings.Contains(out, "commits") || strings.Contains(files, "commits") {
		t.Errorf("the card paints the commits block:\n%s", out)
	}
}

func TestCardTheWarningCountsTheMissingWorktrees(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	const total = 20
	for i := 0; i < total; i++ {
		snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
			Path:   path + "/wt",
			Branch: fmt.Sprintf("feat-%d", i),
			Head:   "abc1234",
		})
	}
	m, r := detailRowWith(t, path, snap)

	rows := detailHeadLines + 6
	out := stripANSI(m.renderDetail(r, rows, m.width, m.layout().cardSplit))
	if !strings.Contains(out, fmt.Sprintf("worktrees (%d)", total)) {
		t.Errorf("the header does not count the worktrees:\n%s", out)
	}
	if want := "… 17 more"; !strings.Contains(out, want) {
		t.Errorf("the worktrees warning does not say %q:\n%s", want, out)
	}
}

func TestCardTheGapMinimumItSpendsTheFirstList(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	m, r := detailRowWith(t, path, snap) // no worktrees nor commits

	out := stripANSI(m.filesSection(r, minListBlockLines+1, 60))
	if !strings.Contains(out, "files (1)") || !strings.Contains(out, "main.go") {
		t.Errorf("with minimum room the files list is missing:\n%s", out)
	}
}

func TestCardMinimumWorktreeNamesItsRepo(t *testing.T) {
	wt := gitstatus.Worktree{Path: "/tmp/multi/wt-feat", Branch: "feat", Head: "abc1234"}
	p := discovery.Project{Path: "/tmp/multi", Name: "multi", HasRepo: true}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	e := tableEntry{kind: kindWorktree, wt: wt, parent: "/tmp/multi"}

	if out := stripANSI(m.renderWorktreeDetail(e, 20, m.width, m.layout().cardSplit)); !strings.Contains(out, "multi") {
		t.Errorf("the card does not name the parent repo:\n%s", out)
	}
	e.parent = ""
	if out := stripANSI(m.renderWorktreeDetail(e, 20, m.width, m.layout().cardSplit)); strings.Contains(out, "repo ") {
		t.Errorf("with no parent repo the repo line was painted:\n%s", out)
	}
}

func TestOrDash(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want -", got)
	}
	if got := orDash("abc1234"); got != "abc1234" {
		t.Errorf("orDash = %q, want the value", got)
	}
}

// The commits block left the card for the top-right panel: at no width does the card paint it.
func TestCardNeverPaintsCommits(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	for i := 0; i < 5; i++ {
		snap.Commits = append(snap.Commits, gitstatus.Commit{Sha: "abc1234", When: 1700000000, Subject: fmt.Sprintf("c%d", i)})
	}
	m, r := detailRowWith(t, path, snap)

	for _, width := range []int{30, 62, 80, 120, 200} {
		m.width = width
		out := stripANSI(m.renderDetail(r, 40, width, m.layout().cardSplit))
		if strings.Contains(out, "commits") {
			t.Errorf("width=%d: the card paints the commits block:\n%s", width, out)
		}
	}
}

func TestCardTheLineOfSyncHasFourShapes(t *testing.T) {
	for _, c := range []struct {
		name   string
		snap   gitstatus.Snapshot
		want   string
		noWant string
	}{
		{
			"no sync branch: there is nothing to compare against",
			gitstatus.Snapshot{SyncBranch: "", SyncKnown: false},
			"— (no sync branch)", "ref missing",
		},
		{
			"the branch exists but the ref does not: the config is broken",
			gitstatus.Snapshot{SyncBranch: "main", SyncKnown: false},
			"main (ref missing)", "(ok)",
		},
		{
			"behind: there are N commits to pull",
			gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true, SyncBehind: 3},
			"main (↓3)", "(ok)",
		},
		{
			"up to date",
			gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true, SyncBehind: 0},
			"main (ok)", "↓0",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := "/tmp/api"
			snap := snapClean()
			snap.SyncBranch = c.snap.SyncBranch
			snap.SyncKnown = c.snap.SyncKnown
			snap.SyncBehind = c.snap.SyncBehind
			m, r := detailRowWith(t, path, snap)

			out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
			if !strings.Contains(out, c.want) {
				t.Errorf("the sync line does not say %q:\n%s", c.want, out)
			}
			if c.noWant != "" && strings.Contains(out, c.noWant) {
				t.Errorf("the sync line says %q, which is a different shape:\n%s", c.noWant, out)
			}
		})
	}
}

func TestCardSyncBehindZeroNotIsPaintsAsDelay(t *testing.T) {
	snap := snapClean()
	snap.SyncBranch = "main"
	snap.SyncKnown = true
	snap.SyncBehind = 0
	m, r := detailRowWith(t, "/tmp/api", snap)

	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if strings.Contains(out, "↓0") {
		t.Errorf("a behind of 0 was painted as behind:\n%s", out)
	}
	if !strings.Contains(out, "main (ok)") {
		t.Errorf("with no behind the sync line does not say (ok):\n%s", out)
	}
}

func TestCardTheVerdictOfTheLastCommand(t *testing.T) {
	const path = "/tmp/api"

	for _, c := range []struct {
		name string
		exit string
		want string
	}{
		{"exit 0 is the clean case", "0", "exit 0"},
		{"a non-zero exit code is a failure and it is named", "3", "exit 3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, r := detailRowWith(t, path, snapClean())
			m.lastCmd[path] = cmdResult{command: "go test ./...", output: "ok\n", exit: c.exit}

			out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
			if !strings.Contains(out, "go test ./...") {
				t.Errorf("the command does not appear in the card:\n%s", out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("the verdict does not say %q:\n%s", c.want, out)
			}
			other := "exit 0"
			if c.want == "exit 0" {
				other = "exit 3"
			}
			if strings.Contains(out, other) {
				t.Errorf("%q appeared, the verdict of the other case:\n%s", other, out)
			}
		})
	}
}

func TestCardTheOutputOfTheCommandRespectsTheBudget(t *testing.T) {
	const path = "/tmp/api"
	m, r := detailRowWith(t, path, snapClean())
	m.lastCmd[path] = cmdResult{
		command: "ls",
		output:  "one\ntwo\nthree\nfour\nfive\n",
		exit:    "0",
	}

	withHeight := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(withHeight, "five") {
		t.Errorf("with room the command output does not appear:\n%s", withHeight)
	}

	m.width = 60
	short := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	for _, line := range strings.Split(short, "\n") {
		if w := len([]rune(line)); w > m.width {
			t.Errorf("the card measures %d with a %d terminal: %q", w, m.width, line)
		}
	}
}

func TestCardTheQueueOfTheCommandRespectsItsBudget(t *testing.T) {
	const path = "/tmp/api"

	linesVisibles := func(rows, lines int) int {
		m, r := detailRowWith(t, path, snapClean())
		m.lastCmd[path] = cmdResult{
			command: "ls",
			output:  strings.TrimSuffix(strings.Repeat("output\n", lines), "\n"),
			exit:    "0",
		}
		out := stripANSI(m.renderDetail(r, rows, m.width, m.layout().cardSplit))
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "output") {
				n++
			}
		}
		return n
	}

	for _, rows := range []int{20, 30, 40, 60} {
		prev := -1
		for _, lines := range []int{1, 3, 8, 20, 40, 80} {
			n := linesVisibles(rows, lines)
			if n < prev {
				t.Errorf("rows=%d: with %d lines %d are visible and with fewer %d were: "+
					"more output cannot hide more output", rows, lines, n, prev)
			}
			prev = n
			if budget := max(3, rows-12); n > budget {
				t.Errorf("rows=%d: the tail shows %d lines and its budget is %d",
					rows, n, budget)
			}
			if budget := max(3, rows-12); lines > budget && n >= lines {
				t.Errorf("rows=%d: the tail showed all %d lines, "+
					"when its budget is %d", rows, lines, budget)
			}
		}
	}
}

func TestCardClipsTheCommandToTheWidthThatIsLeft(t *testing.T) {
	const path = "/tmp/api"
	long := strings.Repeat("command", 30) // 210 chars, plenty to clip

	for _, width := range []int{40, 60, 90, 200, 260} {
		m, r := detailRowWith(t, path, snapClean())
		m.width = width
		m.lastCmd[path] = cmdResult{command: long, output: "", exit: "0"}

		out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
		var line string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "command") {
				line = l
				break
			}
		}
		if line == "" {
			t.Fatalf("width=%d: the command does not appear in the card:\n%s", width, out)
		}
		if w := len([]rune(line)); w > width {
			t.Errorf("width=%d: the command line measures %d and overflows the box: %q",
				width, w, line)
		}
		// In a wide enough terminal the whole command fits: clipping it always would hide information that can be read.
		if width >= 240 {
			if !strings.Contains(line, long) {
				t.Errorf("width=%d: the command was clipped needlessly: %q", width, line)
			}
		}
	}
}

func TestHeaderDistinguishesDetachedAndWithoutUpstream(t *testing.T) {
	proj := discovery.Project{Path: "/api", Name: "api", HasRepo: true}

	t.Run("detached keeps the branch", func(t *testing.T) {
		s := snapClean()
		s.Status.Branch = "feat/x"
		s.Status.Detached = true
		m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
		out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24, m.width, m.layout().cardSplit))
		if !strings.Contains(out, "feat/x (detached)") {
			t.Errorf("card = %q, want the branch with the (detached) suffix", out)
		}
	})

	t.Run("no branch", func(t *testing.T) {
		s := snapClean()
		s.Status.Branch = ""
		m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
		out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24, m.width, m.layout().cardSplit))
		if !strings.Contains(out, "branch  -") {
			t.Errorf("the card = %q, want '-' for the branch", out)
		}
	})

	t.Run("no upstream", func(t *testing.T) {
		s := snapClean()
		s.Status.HasUpstream = false
		s.Status.Upstream = ""
		m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
		out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24, m.width, m.layout().cardSplit))
		if !strings.Contains(out, "no upstream") {
			t.Errorf("card = %q, want '— (no upstream)'", out)
		}
	})
}

func TestWorktreeOfAnotherRootStillAppears(t *testing.T) {
	proj := discovery.Project{Path: "/home/api", Name: "api", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		{Path: "/mnt/wt-other", Branch: "feat/x"},
	}
	m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
	out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "feat/x") {
		t.Errorf("card = %q, want the worktree of another root to appear", out)
	}
}

// filepath.Rel only fails when it cannot make the two paths comparable, and since the repo path and the worktree path come from different sources that is reachable (a relative project path, left by the old cache or a moved repository); without the `rel = wt.Path` fallback a Rel failure would leave the line empty and the worktree would disappear from the card.
func TestCardWorktreeWithPathNotComparableFallATheAbsolute(t *testing.T) {
	rel := "api"
	abs := filepath.Join(t.TempDir(), "feature")
	m, r := detailRowWith(t, rel, func() gitstatus.Snapshot {
		s := snapClean()
		s.Worktrees = []gitstatus.Worktree{{Path: abs, Branch: "feature", Head: "abc1234"}}
		return s
	}())
	m.width = 120

	out := stripANSI(m.renderDetail(r, 40, m.width, m.layout().cardSplit))
	if !strings.Contains(out, "feature") {
		t.Fatalf("the worktree does not appear in the card:\n%s", out)
	}
	// With an impossible Rel the path falls back to the absolute and the right column shows its prefix.
	if want := string([]rune(abs)[:11]); !strings.Contains(out, want) {
		t.Errorf("with an impossible Rel the path should fall back to the absolute %q:\n%s", abs, out)
	}
}
