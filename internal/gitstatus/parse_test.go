package gitstatus

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gitdash/internal/discovery"
	"gitdash/internal/testutil"
)

const porcelainClean = `# branch.oid 3f4e0c0f6a4e3c5a5d5b5e5a5d5b5e5a5d5b5e5a
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
`

func TestParseClean(t *testing.T) {
	st, files := ParsePorcelain(porcelainClean)
	if st.Branch != "main" || st.Upstream != "origin/main" || !st.HasUpstream {
		t.Errorf("st = %+v", st)
	}
	if st.Ahead != 0 || st.Behind != 0 || st.Dirty() != 0 || len(files) != 0 {
		t.Errorf("fails: %+v files=%v", st, files)
	}
	if got := st.Derive(); got != StateClean {
		t.Errorf("derive = %v, want clean", got)
	}
}

func TestParseAheadBehindDiverged(t *testing.T) {
	ahead := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +2 -0", 1)
	st, _ := ParsePorcelain(ahead)
	if st.Ahead != 2 || st.Behind != 0 {
		t.Errorf("ahead: %+v", st)
	}
	if st.Derive() != StateAhead {
		t.Errorf("ahead derive = %v", st.Derive())
	}

	behind := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +0 -3", 1)
	st, _ = ParsePorcelain(behind)
	if st.Ahead != 0 || st.Behind != 3 {
		t.Errorf("behind: %+v", st)
	}
	if st.Derive() != StateBehind {
		t.Errorf("behind derive = %v", st.Derive())
	}

	diverged := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +2 -3", 1)
	st, _ = ParsePorcelain(diverged)
	if st.Derive() != StateDiverged {
		t.Errorf("diverged derive = %v", st.Derive())
	}
}

func TestParseDirty(t *testing.T) {
	out := porcelainClean + `1 .M NRM 100644 100644 100644 abc def src/main.go
? README.md
? docs/
2 R. N... 100644 100644 100644 abc def R100 new.txt	old.txt
`
	st, files := ParsePorcelain(out)
	if st.TrackedChanges != 2 || st.Untracked != 2 {
		t.Errorf("tracked=%d untracked=%d", st.TrackedChanges, st.Untracked)
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty", st.Derive())
	}
	if len(files) != 4 {
		t.Fatalf("files = %d, want 4", len(files))
	}
	if files[0].Path != "src/main.go" || files[0].Code != ".M" {
		t.Errorf("files[0] = %+v", files[0])
	}
	if files[1].Code != "??" {
		t.Errorf("files[1] = %+v", files[1])
	}
	if files[3].Path != "new.txt" {
		t.Errorf("rename path = %q, want new.txt", files[3].Path)
	}
}

func TestParseNotUpstream(t *testing.T) {
	st, _ := ParsePorcelain("# branch.oid abc\n# branch.head main\n")
	if st.HasUpstream {
		t.Error("upstream detected with no line")
	}
	if st.Derive() != StateNoUpstream {
		t.Errorf("derive = %v, want no-upstream", st.Derive())
	}
}

func TestParseDetached(t *testing.T) {
	out := strings.Replace(porcelainClean, "# branch.head main", "# branch.head (detached)", 1)
	st, _ := ParsePorcelain(out)
	if !st.Detached || st.Branch != "" {
		t.Errorf("detached: %+v", st)
	}
}

func TestParseUnbornBranch(t *testing.T) {
	st, _ := ParsePorcelain("# branch.oid (initial)\n# branch.head (main)\n")
	if st.Branch != "main" || st.Detached {
		t.Errorf("unborn: %+v", st)
	}
}

func TestParseLog(t *testing.T) {
	out := "abc1234\x001700000000\x00feat: one\ndef5678\x001699000000\x00fix: two\n"
	commits := ParseLog(out)
	if len(commits) != 2 {
		t.Fatalf("commits = %d", len(commits))
	}
	if commits[0].Sha != "abc1234" || commits[0].Subject != "feat: one" || commits[0].When != 1700000000 {
		t.Errorf("commits[0] = %+v", commits[0])
	}
}

func TestCollectClean(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	st := Collect(t.Context(), dir, "", false)
	if st.Err != "" {
		t.Fatalf("err = %q", st.Err)
	}
	if st.Status.Derive() != StateClean || st.LastCommit == 0 {
		t.Errorf("snap = %+v", snapSummary(st))
	}
}

func TestCollectDirty(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	testutil.WriteUncommitted(t, dir, map[string]string{"base.txt": "changed"})
	testutil.WriteUntracked(t, dir, map[string]string{"un1.txt": "x", "un2.txt": "y"})

	st := Collect(t.Context(), dir, "", false)
	if st.Status.TrackedChanges != 1 || st.Status.Untracked != 2 {
		t.Errorf("tracked=%d untracked=%d", st.Status.TrackedChanges, st.Status.Untracked)
	}
	if st.Status.Derive() != StateDirty {
		t.Errorf("derive = %v", st.Status.Derive())
	}
}

func TestCollectAheadBehindDiverged(t *testing.T) {
	ahead, _ := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, ahead, map[string]string{"extra.txt": "x"}, "local 1")
	testutil.CommitFiles(t, ahead, map[string]string{"extra2.txt": "x"}, "local 2")

	st := Collect(t.Context(), ahead, "", false)
	if st.Status.Ahead != 2 || st.Status.Derive() != StateAhead {
		t.Errorf("ahead: %+v", snapSummary(st))
	}

	behind, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 3, "behind-")
	testutil.FetchLocal(t, behind) // without a fetch the repo does not see the behind
	st = Collect(t.Context(), behind, "", false)
	if st.Status.Behind != 3 || st.Status.Derive() != StateBehind {
		t.Errorf("behind: %+v", snapSummary(st))
	}

	diverged, origin2 := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, diverged, map[string]string{"l.txt": "l"}, "local")
	testutil.PushUpstreamCommits(t, origin2, 2, "div-")
	testutil.FetchLocal(t, diverged)
	st = Collect(t.Context(), diverged, "", false)
	if st.Status.Ahead != 1 || st.Status.Behind != 2 || st.Status.Derive() != StateDiverged {
		t.Errorf("diverged: %+v", snapSummary(st))
	}
}

func TestCollectNotUpstream(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	st := Collect(t.Context(), dir, "", false)
	if st.Status.HasUpstream || st.Status.Derive() != StateNoUpstream {
		t.Errorf("no-upstream: %+v", snapSummary(st))
	}
}

func TestCollectDetached(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	testutil.Detach(t, dir)
	st := Collect(t.Context(), dir, "", false)
	if !st.Status.Detached {
		t.Fatalf("detached: %+v", st.Status)
	}
	if len(st.Status.Branch) != 7 {
		t.Errorf("branch detached = %q, want the short sha", st.Status.Branch)
	}
}

func TestCollectCorrupt(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.BreakGit(t, dir)
	st := Collect(t.Context(), dir, "", false)
	if st.Err == "" {
		t.Fatal("corrupt with no error")
	}
	if st.State(true) != StateError {
		t.Errorf("state = %v, want error", st.State(true))
	}
}

// The cap is checked at the exact value (100) and at the one past it (101), which is where a `len >= maxFiles` guard can get it wrong.
func TestParseListFilesClamped(t *testing.T) {
	for _, tc := range []struct{ lines, want int }{
		{maxFiles - 1, maxFiles - 1},
		{maxFiles, maxFiles},
		{maxFiles + 1, maxFiles},
		{maxFiles + 25, maxFiles},
	} {
		var b strings.Builder
		b.WriteString(porcelainClean)
		for i := 0; i < tc.lines; i++ {
			b.WriteString("? file" + strconv.Itoa(i) + ".txt\n")
		}
		_, files := ParsePorcelain(b.String())
		if len(files) != tc.want {
			t.Errorf("with %d changes files = %d, want %d (cap %d)", tc.lines, len(files), tc.want, maxFiles)
		}
	}
}

// A truncated entry line (fewer fields than required) is dropped, parsing what it can without indexing past the slice; a "1 " line with 7 fields instead of 8 is exactly that guard's edge.
func TestParseLineTruncatedIsDiscards(t *testing.T) {
	cases := []struct {
		name, body string
		beforePath int
	}{
		{"1 with 7 fields (the path is missing)", "M. NRM 100644 100644 100644 abc def", 7},
		{"1 with 6 fields", "M. NRM 100644 100644 abc def", 7},
		{"1 empty", "", 7},
		{"2 with 7 fields (the path is missing)", "R. N... 100644 100644 abc", 8},
		{"u with 8 fields (the path is missing)", "UU N... 100644 100644 100644 abc def", 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := parseEntry(c.body, c.beforePath); ok {
				t.Errorf("parseEntry(%q, %d) = true, want false (incomplete line)", c.body, c.beforePath)
			}
		})
	}
	st, files := ParsePorcelain(porcelainClean + "1 .M NRM 100644 100644 100644 abc def\n")
	if len(files) != 0 {
		t.Errorf("files = %d, want 0 (the truncated line is dropped)", len(files))
	}
	if st.TrackedChanges != 1 {
		t.Errorf("tracked = %d, want 1 (the change counts even when it cannot be detailed)", st.TrackedChanges)
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty (a truncated change still counts)", st.Derive())
	}
}

func TestCollectWithoutCommits(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	st := Collect(t.Context(), dir, "", false)
	if st.Err != "" {
		t.Fatalf("err = %q", st.Err)
	}
	if st.LastCommit != 0 {
		t.Errorf("LastCommit = %d, want 0 (no commits)", st.LastCommit)
	}
	if len(st.Commits) != 0 {
		t.Errorf("commits = %d, want 0", len(st.Commits))
	}
}

// An empty log with exit 0 is not what a repo with no commits does (it fails), but a wrapped git can do it: the last commit date is 0, not an out-of-range index.
func TestLastCommitWhen(t *testing.T) {
	cases := []struct {
		name    string
		commits []Commit
		want    int64
	}{
		{"nil", nil, 0},
		{"empty", []Commit{}, 0},
		{"one", []Commit{{Sha: "a", When: 1700000000}}, 1700000000},
		{"several, it uses the first", []Commit{{When: 99}, {When: 1}}, 99},
	}
	for _, c := range cases {
		if got := lastCommitWhen(c.commits); got != c.want {
			t.Errorf("%s: lastCommitWhen = %d, want %d", c.name, got, c.want)
		}
	}
}

// The edge of that truncation is exactly 7 characters: with fewer there is nothing to show and with more it is cut.
func TestNormalizeBranchDetachedShaShort(t *testing.T) {
	cases := []struct {
		name string
		st   Status
		want string
	}{
		{"7-char sha", Status{Detached: true, OID: "abc1234"}, "abc1234"},
		{"long sha", Status{Detached: true, OID: "abc1234567890def"}, "abc1234"},
		{"6-char sha (below the minimum)", Status{Detached: true, OID: "abc123"}, ""},
		{"no oid", Status{Detached: true}, ""},
		{"attached with a branch", Status{Branch: "main", OID: "abc1234"}, "main"},
		{"attached with a branch and no detached marker", Status{Branch: "main", OID: "abc1234", Detached: false}, "main"},
		{"detached but with a branch", Status{Detached: true, Branch: "main"}, "main"},
	}
	for _, c := range cases {
		if got := normalizeBranch(c.st); got != c.want {
			t.Errorf("%s: normalizeBranch(%+v) = %q, want %q", c.name, c.st, got, c.want)
		}
	}
}

func TestStreamPoolConcurrencyInvalidates(t *testing.T) {
	dirs := make([]string, 3)
	projects := make([]discovery.Project, 3)
	for i := range dirs {
		d, _ := testutil.NewRepo(t, false)
		dirs[i] = d
		projects[i] = discovery.Project{Path: d, HasRepo: true}
	}
	for _, c := range []int{0, -1, 1, 2} {
		var mu sync.Mutex
		got := map[string]bool{}
		StreamPool(t.Context(), projects, "", false, c, func(path string, _ Snapshot) {
			mu.Lock()
			got[path] = true
			mu.Unlock()
		})
		if len(got) != len(projects) {
			t.Errorf("concurrency %d: emit = %d, want %d", c, len(got), len(projects))
		}
	}
}

// The pool's ceiling is a MULTIPLICATION per CPU, so the ARITHMETIC_BASE mutant (NumCPU()-4) stops bounding the peak, which is why the test watches the PEAK of emit and asserts >= cpus instead of == n (the scheduler need not overlap all N emits).
func TestStreamPoolCeilingIsMultiplication(t *testing.T) {
	cpus := runtime.NumCPU()
	n := cpus * 2
	projects := make([]discovery.Project, n)
	for i := range projects {
		projects[i] = discovery.Project{Path: fmt.Sprintf("/no/such/path/%d", i)}
	}

	var mu sync.Mutex
	inFlight, peak := 0, 0
	StreamPool(t.Context(), projects, "", false, n, func(_ string, _ Snapshot) {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()
		// Without this pause the emits resolve before the next goroutine reaches the semaphore and the peak would always measure 1.
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
	})

	if peak < cpus {
		t.Errorf("peak of simultaneous emits = %d, want >= %d (the NumCPU()*4 cap not applied; "+
			"with NumCPU()-4 the peak stays at %d or less)", peak, cpus, cpus-4)
	}
}

func TestStreamPool(t *testing.T) {
	a, _ := testutil.NewRepo(t, true)
	b, _ := testutil.NewRepo(t, false)
	testutil.WriteUncommitted(t, b, map[string]string{"m.txt": "m"})

	projects := []discovery.Project{
		{Path: a, HasRepo: true},
		{Path: b, HasRepo: true},
	}
	got := map[string]State{}
	var mu sync.Mutex
	StreamPool(t.Context(), projects, "", false, 2, func(path string, st Snapshot) {
		mu.Lock()
		got[path] = st.State(true)
		mu.Unlock()
	})
	if len(got) != 2 {
		t.Fatalf("emit = %d paths, want 2", len(got))
	}
	if got[a] != StateClean || got[b] != StateDirty {
		t.Errorf("states = %v", got)
	}
}

func snapSummary(s Snapshot) string {
	return s.Err + "|" + s.Status.Derive().String()
}

// A conflict counts as a tracked change, and it is the only case where the state column's count says what has to be done: a repo with a conflict and nothing else must show as dirty, not clean.
func TestParseConflictsCountAsDirty(t *testing.T) {
	out := porcelainClean + `u UU N... 100644 100644 100644 100644 abc def ghi conflicted.go
u AA N... 100644 100644 100644 100644 abc def ghi both-new.go
`
	st, files := ParsePorcelain(out)
	if st.TrackedChanges != 2 {
		t.Errorf("tracked = %d, want 2 (both conflicts)", st.TrackedChanges)
	}
	if st.Untracked != 0 {
		t.Errorf("untracked = %d, want 0", st.Untracked)
	}
	if st.Dirty() != 2 {
		t.Errorf("Dirty = %d, want 2", st.Dirty())
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty: a repo with a conflict is not clean", st.Derive())
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0].Code != "UU" || files[0].Path != "conflicted.go" {
		t.Errorf("files[0] = %+v", files[0])
	}
	// The `u ` line's code is the XY pair of both sides verbatim: "AA" is a file added by both sides and must neither be reduced to the first letter nor lose it.
	if files[1].Code != "AA" {
		t.Errorf("files[1].Code = %q, want AA (the full XY pair)", files[1].Code)
	}
}

// Each case is checked against the literal it must return: a String() returning "detached" for StateDirty would pass any test only looking at non-emptiness, and the default must say "error" because the text goes straight into the table.
func TestStateStringAndHasScore(t *testing.T) {
	for _, c := range []struct {
		st    State
		name  string
		score int
	}{
		{StateClean, "clean", 0},
		{StateNoUpstream, "no upstream", 2},
		{StateDetached, "detached", 2},
		{StateBehind, "behind", 3},
		{StateAhead, "ahead", 3},
		{StateDirty, "dirty", 4},
		{StateDiverged, "diverged", 5},
		{StateNoRepo, "no repo", 2},
		{StateError, "error", 6},
		{State(99), "error", 0},
	} {
		if got := c.st.String(); got != c.name {
			t.Errorf("State(%d).String() = %q, want %q", int(c.st), got, c.name)
		}
		if got := c.st.Score(); got != c.score {
			t.Errorf("State(%d).Score() = %d, want %d", int(c.st), got, c.score)
		}
	}
}

// The assertion states the ORDER between groups instead of each loose number: the three informational states share a score on purpose (none needs immediate attention, so ties are correct and the path breaks them) and what must not happen is one of them sneaking in front of dirty.
func TestScoreSortsForAttention(t *testing.T) {
	groups := []struct {
		name   string
		states []State
	}{
		{"error", []State{StateError}},
		{"diverged", []State{StateDiverged}},
		{"dirty", []State{StateDirty}},
		{"ahead/behind", []State{StateAhead, StateBehind}},
		{"informational", []State{StateDetached, StateNoUpstream, StateNoRepo}},
		{"clean", []State{StateClean}},
	}
	for i := 1; i < len(groups); i++ {
		prev, cur := groups[i-1], groups[i]
		for _, a := range prev.states {
			for _, b := range cur.states {
				if State(b).Score() >= State(a).Score() {
					t.Errorf("%s (%v, %d) does not come BEFORE %s (%v, %d)",
						a, prev.name, State(a).Score(),
						b, cur.name, State(b).Score())
				}
			}
		}
	}
}

// Detached wins over !HasUpstream and this test pins it: reordering the conditions would make a detached HEAD report "no upstream", a different and wrong diagnosis.
func TestDeriveDetachedWinsANotUpstream(t *testing.T) {
	st := Status{Detached: true, HasUpstream: false}
	if got := st.Derive(); got != StateDetached {
		t.Errorf("Derive = %v, want detached (a loose HEAD is not no-upstream)", got)
	}
	st = Status{Detached: true, TrackedChanges: 1, HasUpstream: true}
	if got := st.Derive(); got != StateDirty {
		t.Errorf("Derive = %v, want dirty (uncommitted comes before HEAD)", got)
	}
}

// A line without that shape (a future git, a corrupt line) must return 0/0 instead of propagating an error: ParsePorcelain never fails, it tolerates git versions it does not know.
func TestParseABToleratesLinesNotRecognised(t *testing.T) {
	for _, s := range []string{
		"",
		"abc",
		"+2",
		"+x -3",
	} {
		ahead, behind := parseAB(s)
		if ahead != 0 || behind != 0 {
			t.Errorf("parseAB(%q) = %d/%d, want 0/0 (a line that makes no sense is 0)", s, ahead, behind)
		}
	}
	if a, b := parseAB("+2 -3"); a != 2 || b != 3 {
		t.Errorf("parseAB(\"+2 -3\") = %d/%d, want 2/3", a, b)
	}
	// An extra group is not an error: Sscanf stops at the fourth destination and the rest is ignored, on purpose, because an ahead/behind with a third figure is better ignored than turned into 0/0 (which would say "no divergence" when there is one); here the behaviour is pinned, not judged.
	if a, b := parseAB("+2 -3 -1"); a != 2 || b != 3 {
		t.Errorf("parseAB(\"+2 -3 -1\") = %d/%d, want 2/3 (the leftover group is ignored)", a, b)
	}
}

// A non-numeric timestamp drops the line WHOLE instead of slipping in a Commit with When=0, which would be a commit from 1970.
func TestParseLogDiscardsCommitsMalformed(t *testing.T) {
	out := "abc123\x001700000000\x00primer commit\n" +
		"def456\x00not-a-timestamp\x00second\n" +
		"only-two-fields\n"
	commits := ParseLog(out)
	if len(commits) != 1 {
		t.Fatalf("ParseLog returned %d commits, want 1: %+v", len(commits), commits)
	}
	if commits[0].Sha != "abc123" || commits[0].Subject != "primer commit" {
		t.Errorf("commit = %+v, want the well-formed one", commits[0])
	}
}

// This case is what stops a cancelled scan from launching git subprocesses: without it cancelling would not stop anything until the work in flight finished, so Ctrl-C in the middle of a 50-repo scan would keep it running.
func TestStreamPoolWithContextCancelled(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	projects := []discovery.Project{
		{Path: dir, HasRepo: true},
		{Path: dir + "-nonexistent", HasRepo: true},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var mu sync.Mutex
	emitted := 0
	StreamPool(ctx, projects, "", false, 1, func(string, Snapshot) {
		mu.Lock()
		emitted++
		mu.Unlock()
	})
	if emitted > len(projects) {
		t.Errorf("emits %d events with the context cancelled, want <= %d", emitted, len(projects))
	}
}

// known=false is not the same as a known 0: the first makes the UI say "— (no sync branch)" (data we do not have) while the second paints as "up to date", a false claim about a repo that may have 40 commits.
func TestSyncBehindWithRefNonexistent(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	n, known := syncBehind(t.Context(), dir, "origin/a-branch-that-does-not-exist")
	if known {
		t.Errorf("syncBehind with a nonexistent ref = %d, known=true; want known=false (it is not that it is up to date)", n)
	}
	testutil.CommitFiles(t, dir, map[string]string{"a.txt": "x\n"}, "commit")
	n, known = syncBehind(t.Context(), dir, "main")
	if !known {
		t.Error("syncBehind against HEAD = known=false, want true (0 commits behind is data)")
	}
	if n != 0 {
		t.Errorf("syncBehind against HEAD = %d, want 0", n)
	}
}
