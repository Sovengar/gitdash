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
	casos := []struct {
		name      string
		markerErr string
		gitErr    string
		want      []string
		ausente   []string
	}{
		{
			name:      "marcador roto",
			markerErr: "line 3: invalid value",
			want:      []string{"marker:", "line 3: invalid value"},
			ausente:   []string{"git:"},
		},
		{
			name:    "repo without git",
			gitErr:  "fatal: not a git repository",
			want:    []string{"git:", "fatal: not a git repository"},
			ausente: []string{"marker:"},
		},
		{
			name:      "both at once",
			markerErr: "bad",
			gitErr:    "roto",
			want:      []string{"marker:", "bad", "git:", "roto"},
		},
	}
	for _, c := range casos {
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

			out := stripANSI(m.renderDetail(r, 40))
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("the card does not say %q:\n%s", w, out)
				}
			}
			for _, a := range c.ausente {
				if strings.Contains(out, a) {
					t.Errorf("the card says %q and should not (there is no such error):\n%s", a, out)
				}
			}
		})
	}
}

// The EXACT fits are what is looked at here, because a `> ` in the wrong place changes the answer precisely when the list fits exactly (and there no "N more" warning with N=0 should be left).
func TestListBudget(t *testing.T) {
	casos := []struct {
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
		{"holgada", 20, 2, 2, false},
		{"muy larga", 10, 100, 7, true},
	}
	for _, c := range casos {
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
	casos := []struct {
		name, out string
		n         int
		want      string
	}{
		{"empty", "", 3, ""},
		{"solo newlines", "\n\n", 3, ""},
		{"one line", "hola", 3, "hola"},
		{"fits whole", "a\nb", 3, "a\nb"},
		{"fits exactly", "a\nb\nc", 3, "a\nb\nc"},
		{"clips from behind", "a\nb\nc\nd", 2, "c\nd"},
		{"more lines than fit", "a\nb\nc", 1, "c"},
		{"n=0", "a\nb", 0, ""},
		{"n negativo", "a\nb", -1, ""},
		{"ignores the trailing newline", "a\nb\n", 5, "a\nb"},
	}
	for _, c := range casos {
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
	casos := []struct {
		name, content string
		n             int
		wantFit       []string
		wantClip      []string
	}{
		{"fits exactly", "a\nb", 2, []string{"a", "b"}, []string{"a", "b"}},
		{"sobra", "a\nb\nc", 2, []string{"a", "b"}, []string{"a", "b"}},
		{"missing", "a", 3, []string{"a", "", ""}, []string{"a"}},
		{"empty", "", 2, []string{"", ""}, []string{""}},
		{"n=0 with content", "a\nb", 0, []string{""}, []string{""}},
		{"n=0 empty", "", 0, []string{""}, []string{""}},
		{"n negativo", "a", -1, []string{""}, []string{""}},
	}
	for _, c := range casos {
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
	const width = 60
	long := strings.Repeat("long", 30) // 150 chars, plenty to clip

	pathLargo := "/tmp/api/" + long
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: long + ".go"}}
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: long + " commit"}}
	m, r := detailRowWith(t, pathLargo, snap)
	m.width = width
	m.lastAction[pathLargo] = actionResult{kind: "pull_rebase", cmd: "git pull --rebase " + long, output: ""}

	out := stripANSI(m.renderDetail(r, 40))
	for _, c := range []struct {
		that string
		want string
	}{
		{"repo path", truncate(pathLargo, max(20, width-13))},
		{"file", truncate(long+".go", max(20, width-8))},
		{"commit", truncate(long+" commit", max(20, width-24))},
		{"argv", truncate("git pull --rebase "+long, max(20, width-30))},
	} {
		if !strings.Contains(out, c.want) {
			t.Errorf("the %s is not clipped to its width (want %d chars: %q…):\n%s", c.that, len(c.want), c.want[:20], out)
		}
	}

	m2, r2 := detailRowWith(t, "/tmp/api", func() gitstatus.Snapshot {
		s := snapClean()
		s.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt/" + long, Branch: "feat", Head: "abc1234"}}
		return s
	}())
	m2.width = width
	outWT := stripANSI(m2.renderDetail(r2, 40))
	if want := truncate("wt/"+long, max(20, width-16)); !strings.Contains(outWT, want) {
		t.Errorf("the worktree path is not clipped to its width (want %d chars: %q…):\n%s", len(want), want[:20], outWT)
	}

	// The 20-character floor is a contract too: with a narrow terminal a path is clipped to 20 at minimum instead of disappearing or eating the box.
	m.width = 30
	if want := truncate(pathLargo, 20); !strings.Contains(stripANSI(m.renderDetail(r, 40)), want) {
		t.Errorf("with width=30 the path should clip to the 20-char floor (%q…)", want[:20])
	}
}

// The expectations are literal run lengths and NOT the same `max(20, width-N)` the card computes: a test that derives its expectation from the expression under test moves with the mutant and kills nothing.
func TestCardClipsTheListsToLiteralWidths(t *testing.T) {
	// ONE repeated rune, not a repeated word: with "longlong…" a suffix test only matches at multiples of the pattern's period, so a run one word longer would slip through.
	run := strings.Repeat("x", 150)
	m, r := detailRowWith(t, "/tmp/api", func() gitstatus.Snapshot {
		s := snapClean()
		s.Files = []gitstatus.FileEntry{{Code: ".M", Path: run + ".go"}}
		s.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: run + " commit"}}
		return s
	}())
	m.width = 60
	out := stripANSI(m.renderDetail(r, 40))
	// At width 60 the file row gets 60-8 = 52 columns and the commit row 60-24 = 36, so the clip leaves 51 and 35 runes before the ellipsis.
	for _, c := range []struct {
		that, marker string
		shown        int
	}{
		{"file", ".M", 51},
		{"commit", "abc1234", 35},
	} {
		var line string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, c.marker) && strings.Contains(l, "x") {
				line = strings.TrimSuffix(l, "…")
				break
			}
		}
		// The EXACT length is what kills the mutant: a wider clip still contains the shorter expectation, so any "is the expected prefix present" check lets it through.
		if got := trailingRun(line, 'x'); got != c.shown {
			t.Errorf("the %s paints a run of %d, want its literal %d (%d columns):\n%s", c.that, got, c.shown, c.shown+1, out)
		}
	}
}

func trailingRun(s string, r rune) int {
	return len(s) - strings.LastIndexFunc(s, func(c rune) bool { return c != r }) - 1
}

func TestCardShowsTheArgvOfTheLastAction(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	m, r := detailRowWith(t, path, snap)

	m.lastAction[path] = actionResult{kind: "pull_ai"}
	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "last pull (AI)") && !strings.Contains(out, "AI") {
		t.Errorf("the no-argv variant of the action is missing:\n%s", out)
	}
	if strings.Contains(out, "git pull") {
		t.Errorf("it invented an argv where there is none:\n%s", out)
	}

	m.lastAction[path] = actionResult{kind: "pull_rebase", cmd: "git pull --rebase --autostash", output: ""}
	out = stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "git pull --rebase --autostash") {
		t.Errorf("the resolved argv does not appear in the card:\n%s", out)
	}
}

// The number in the warning is the difference between "you are missing 3" and a count that does not add up with the header.
func TestCardTheWarningCountsTheMissingFiles(t *testing.T) {
	path := "/tmp/api"
	snap := snapDirty(0, 0)
	const total = 20
	for i := 0; i < total; i++ {
		snap.Files = append(snap.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/file%02d.go", i)})
	}
	m, r := detailRowWith(t, path, snap)

	rows := detailHeadLines + minListBlockLines + 5 // gap + header + 4
	out := stripANSI(m.renderDetail(r, rows))
	if !strings.Contains(out, fmt.Sprintf("files (%d)", total)) {
		t.Errorf("the header does not count the %d files:\n%s", total, out)
	}
	wantAviso := fmt.Sprintf("… %d more", total-4)
	if !strings.Contains(out, wantAviso) {
		t.Errorf("the warning does not say %q:\n%s", wantAviso, out)
	}
	if !strings.Contains(out, "file00.go") {
		t.Errorf("the first of the list was not painted:\n%s", out)
	}
	if strings.Contains(out, "file04.go") {
		t.Errorf("a file that did not fit was painted:\n%s", out)
	}
}

func TestCardWithTheGapMinimumOfTheList(t *testing.T) {
	path := "/tmp/api"
	snap := snapDirty(1, 1)
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: "fix"}}
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
	m, r := detailRowWith(t, path, snap)

	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines))
	if !strings.Contains(out, "worktrees (1)") {
		t.Errorf("with avail=%d the first list must paint its header:\n%s", minListBlockLines, out)
	}
	for _, sinHueco := range []string{"files (", "commits"} {
		if strings.Contains(out, sinHueco) {
			t.Errorf("with avail=%d the list %q had no room and was painted anyway:\n%s", minListBlockLines, sinHueco, out)
		}
	}

	out = stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines-1))
	for _, header := range []string{"worktrees (", "files (", "commits"} {
		if strings.Contains(out, header) {
			t.Errorf("with avail=%d the list %q was painted with no room:\n%s", minListBlockLines-1, header, out)
		}
	}
}

func TestCardBranchEmptyIsShowsAsDash(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Status.Branch = ""
	m, r := detailRowWith(t, path, snap)
	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "branch  -") {
		t.Errorf("with no branch the card does not show the dash:\n%s", out)
	}
}

func TestCardNotPaintsListsEmpty(t *testing.T) {
	path := "/tmp/api"
	m, r := detailRowWith(t, path, snapClean())
	out := stripANSI(m.renderDetail(r, 40))
	for _, header := range []string{"files (", "worktrees (", "commits"} {
		if strings.Contains(out, header) {
			t.Errorf("a repo with no data painted the %q list:\n%s", header, out)
		}
	}
	for _, field := range []string{"path", "branch", "upstream", "state", "sync"} {
		if !strings.Contains(out, field) {
			t.Errorf("the header field %q is missing:\n%s", field, out)
		}
	}
}

// The 3-line floor is a contract too: with the minimal card 3 lines are still visible, not 0.
func TestCardClipsTheQueueOfTheLastAction(t *testing.T) {
	path := "/tmp/api"
	prologo := strings.Repeat("line\n", 30)
	finales := "resultado1\nresultado2\nresultado3\nresultado4\nresultado5"

	t.Run("with room", func(t *testing.T) {
		m, r := detailRowWith(t, path, snapClean())
		m.lastAction[path] = actionResult{kind: "pull_rebase", output: prologo + finales + "\n"}
		out := stripANSI(m.renderDetail(r, detailHeadLines+10))
		if !strings.Contains(out, "resultado5") || !strings.Contains(out, "resultado3") {
			t.Errorf("the end of the output is not visible:\n%s", out)
		}
		if strings.Contains(out, "resultado1") {
			t.Errorf("the tail swallowed the beginning of the output:\n%s", out)
		}
	})

	t.Run("with room amplio", func(t *testing.T) {
		m, r := detailRowWith(t, path, snapClean())
		m.lastAction[path] = actionResult{kind: "pull_rebase", output: prologo + finales + "\n"}
		out := stripANSI(m.renderDetail(r, 44))
		if !strings.Contains(out, "line") || !strings.Contains(out, "resultado5") {
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

	out := stripANSI(m.renderWorktreeDetail(e, 20))
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
	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "wt-feat") {
		t.Errorf("the worktree path is not visible:\n%s", out)
	}
	if strings.Contains(out, "/tmp/api/wt-feat") {
		t.Errorf("the worktree path was not made relative:\n%s", out)
	}
}

func TestCardTheListsShareTheBudget(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	for i := 0; i < 3; i++ {
		snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
			Path: path + "/wt", Branch: "feat", Head: "abc1234",
		})
	}
	for i := 0; i < 6; i++ {
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

	for rows := detailHeadLines + 2; rows <= 40; rows++ {
		out := stripANSI(m.renderDetail(r, rows))
		if got := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); got > rows {
			t.Errorf("rows=%d: the card paints %d lines, it overflows the box", rows, got)
		}
	}

	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines+3))
	if !strings.Contains(out, "worktrees (3)") {
		t.Errorf("with minimum room no item list should be visible:\n%s", out)
	}
	if strings.Contains(out, "files (") || strings.Contains(out, "commits") {
		t.Errorf("the following lists painted a header with no room:\n%s", out)
	}
}

func TestCardTheWarningCountsTheMissingWorktrees(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	const total = 8
	for i := 0; i < total; i++ {
		snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
			Path:   path + "/wt",
			Branch: fmt.Sprintf("feat-%d", i),
			Head:   "abc1234",
		})
	}
	m, r := detailRowWith(t, path, snap)

	rows := detailHeadLines + minListBlockLines + 4
	out := stripANSI(m.renderDetail(r, rows))
	if !strings.Contains(out, fmt.Sprintf("worktrees (%d)", total)) {
		t.Errorf("the header does not count the worktrees:\n%s", out)
	}
	if want := fmt.Sprintf("… %d more", total-3); !strings.Contains(out, want) {
		t.Errorf("the worktrees warning does not say %q:\n%s", want, out)
	}
}

func TestCardTheGapMinimumItSpendsTheFirstList(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	m, r := detailRowWith(t, path, snap) // no worktrees nor commits

	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines))
	if !strings.Contains(out, "files (1)") {
		t.Errorf("with minimum room the files header is missing:\n%s", out)
	}
}

func TestCardMinimumWorktreeNamesItsRepo(t *testing.T) {
	wt := gitstatus.Worktree{Path: "/tmp/multi/wt-feat", Branch: "feat", Head: "abc1234"}
	p := discovery.Project{Path: "/tmp/multi", Name: "multi", HasRepo: true}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	e := tableEntry{kind: kindWorktree, wt: wt, parent: "/tmp/multi"}

	if out := stripANSI(m.renderWorktreeDetail(e, 20)); !strings.Contains(out, "multi") {
		t.Errorf("the card does not name the parent repo:\n%s", out)
	}
	e.parent = ""
	if out := stripANSI(m.renderWorktreeDetail(e, 20)); strings.Contains(out, "repo ") {
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

func TestCardTheGapMinimumForTheCommits(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: "fix"}}
	m, r := detailRowWith(t, path, snap) // no worktrees nor files

	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines))
	if !strings.Contains(out, "commits") {
		t.Errorf("with minimum room the commits header is missing:\n%s", out)
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

			out := stripANSI(m.renderDetail(r, 40))
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

	out := stripANSI(m.renderDetail(r, 40))
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

			out := stripANSI(m.renderDetail(r, 40))
			if !strings.Contains(out, "go test ./...") {
				t.Errorf("the command does not appear in the card:\n%s", out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("the verdict does not say %q:\n%s", c.want, out)
			}
			otro := "exit 0"
			if c.want == "exit 0" {
				otro = "exit 3"
			}
			if strings.Contains(out, otro) {
				t.Errorf("%q appeared, the verdict of the other case:\n%s", otro, out)
			}
		})
	}
}

func TestCardTheOutputOfTheCommandRespectsTheBudget(t *testing.T) {
	const path = "/tmp/api"
	m, r := detailRowWith(t, path, snapClean())
	m.lastCmd[path] = cmdResult{
		command: "ls",
		output:  "uno\ndos\ntres\ncuatro\ncinco\n",
		exit:    "0",
	}

	conAlto := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(conAlto, "cinco") {
		t.Errorf("with room the command output does not appear:\n%s", conAlto)
	}

	m.width = 60
	short := stripANSI(m.renderDetail(r, 40))
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
		out := stripANSI(m.renderDetail(r, rows))
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

		out := stripANSI(m.renderDetail(r, 40))
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

	t.Run("detached conserva la rama", func(t *testing.T) {
		s := snapClean()
		s.Status.Branch = "feat/x"
		s.Status.Detached = true
		m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
		out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24))
		if !strings.Contains(out, "feat/x (detached)") {
			t.Errorf("card = %q, want the branch with the (detached) suffix", out)
		}
	})

	t.Run("no branch", func(t *testing.T) {
		s := snapClean()
		s.Status.Branch = ""
		m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
		out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24))
		if !strings.Contains(out, "branch  -") {
			t.Errorf("la ficha = %q, want '-' en la rama", out)
		}
	})

	t.Run("no upstream", func(t *testing.T) {
		s := snapClean()
		s.Status.HasUpstream = false
		s.Status.Upstream = ""
		m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
		out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24))
		if !strings.Contains(out, "no upstream") {
			t.Errorf("card = %q, want '— (no upstream)'", out)
		}
	})
}

func TestWorktreeOfAnotherRootShowsPathAbsolute(t *testing.T) {
	proj := discovery.Project{Path: "/home/api", Name: "api", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		{Path: "/mnt/wt-otro", Branch: "feat/x"},
	}
	m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: s})
	out := stripANSI(m.renderDetail(row{project: proj, snap: s, state: s.State(true)}, 24))
	if !strings.Contains(out, "/mnt/wt-otro") {
		t.Errorf("card = %q, want the absolute path of a worktree in another root", out)
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

	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "feature") {
		t.Fatalf("the worktree does not appear in the card:\n%s", out)
	}
	if !strings.Contains(out, abs) {
		t.Errorf("with an impossible Rel the path should fall back to the absolute %q:\n%s", abs, out)
	}
}
