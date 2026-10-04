package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func TestPrintSync(t *testing.T) {
	casos := []struct {
		name string
		snap gitstatus.Snapshot
		want string
	}{
		{"no sync branch", gitstatus.Snapshot{}, "—"},
		{"a ref that does not exist", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: false}, "main —"},
		{"up to date: la rama y nada more", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true}, "main"},
		{"behind", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true, SyncBehind: 3}, "main ↓3"},
		{"only syncKnown false behind (unknown rules)", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: false, SyncBehind: 3}, "main —"},
	}
	for _, c := range casos {
		if got := printSync(c.snap); got != c.want {
			t.Errorf("%s: printSync = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPrintState(t *testing.T) {
	casos := []struct {
		name            string
		st              gitstatus.State
		tracked, untked int
		want            string
	}{
		{"error", gitstatus.StateError, 5, 5, "error"},
		{"no repo", gitstatus.StateNoRepo, 0, 0, "no repo"},
		{"clean", gitstatus.StateClean, 0, 0, ""},
		{"solo tracked", gitstatus.StateDirty, 2, 0, "2"},
		{"solo untracked", gitstatus.StateDirty, 0, 1, "?1"},
		{"ambos", gitstatus.StateDirty, 2, 1, "2 ?1"},
		{"diverged counts as dirty", gitstatus.StateDiverged, 1, 0, "1"},
	}
	for _, c := range casos {
		snap := gitstatus.Snapshot{Status: gitstatus.Status{TrackedChanges: c.tracked, Untracked: c.untked}}
		if got := printState(c.st, snap); got != c.want {
			t.Errorf("%s: printState = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPrintUpDown(t *testing.T) {
	casos := []struct {
		name          string
		st            gitstatus.State
		hasUp         bool
		ahead, behind int
		err           string
		want          string
	}{
		{"no repo", gitstatus.StateNoRepo, true, 3, 3, "", ""},
		{"with an error", gitstatus.StateError, true, 3, 3, "fatal", ""},
		{"no upstream", gitstatus.StateClean, false, 0, 0, "", "no-up"},
		{"en sync", gitstatus.StateClean, true, 0, 0, "", ""},
		{"solo ahead", gitstatus.StateAhead, true, 2, 0, "", "↑2"},
		{"solo behind", gitstatus.StateBehind, true, 0, 3, "", "↓3"},
		{"divergido", gitstatus.StateDiverged, true, 1, 2, "", "↑1↓2"},
		{"detached with no upstream", gitstatus.StateDetached, false, 0, 0, "", "no-up"},
	}
	for _, c := range casos {
		snap := gitstatus.Snapshot{
			Status: gitstatus.Status{HasUpstream: c.hasUp, Ahead: c.ahead, Behind: c.behind},
			Err:    c.err,
		}
		if got := printUpDown(c.st, snap); got != c.want {
			t.Errorf("%s: printUpDown = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRelativeTimePrint(t *testing.T) {
	now := time.Now()
	casos := []struct {
		name  string
		edad  time.Duration
		epoch int64
		want  string
	}{
		{"no epoch", 0, 0, "-"},
		{"epoch negativo", 0, -5, "-"},
		{"now", 0, now.Unix(), "now"},
		{"minutos", 7 * time.Minute, 0, "7m ago"},
		{"horas", 5 * time.Hour, 0, "5h ago"},
		{"days", 3 * 24 * time.Hour, 0, "3d ago"},
	}
	for _, c := range casos {
		epoch := c.epoch
		if c.epoch == 0 && c.want != "-" {
			epoch = now.Add(-c.edad).Unix()
		}
		if got := relativeTimePrint(epoch); got != c.want {
			t.Errorf("%s: relativeTimePrint = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOrDashPrint(t *testing.T) {
	if got := orDashPrint(""); got != "-" {
		t.Errorf("orDashPrint(\"\") = %q, want -", got)
	}
	if got := orDashPrint("backend"); got != "backend" {
		t.Errorf("orDashPrint = %q, want the value", got)
	}
}

func TestGroupLabelPrint(t *testing.T) {
	casos := []struct {
		name string
		p    discovery.Project
		want string
	}{
		{"both levels", discovery.Project{PrimaryGroup: "vsocial", SecondaryGroup: "backend"}, "vsocial/backend"},
		{"solo primario", discovery.Project{PrimaryGroup: "vsocial"}, "vsocial"},
		{"ninguno", discovery.Project{}, ""},
		{"secondary only (it is not shown alone)", discovery.Project{SecondaryGroup: "backend"}, ""},
	}
	for _, c := range casos {
		if got := groupLabelPrint(c.p); got != c.want {
			t.Errorf("%s: groupLabelPrint = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestHasProject(t *testing.T) {
	projects := []discovery.Project{{Path: "/a"}, {Path: "/b"}}
	if !hasProject(projects, "/b") {
		t.Error("hasProject did not find /b")
	}
	if hasProject(projects, "/c") {
		t.Error("hasProject found /c, which is not there")
	}
	if hasProject(nil, "/a") {
		t.Error("hasProject found something in an empty list")
	}
}

func TestSortPrintRows(t *testing.T) {
	row := func(name string, score, last int) printRow {
		return printRow{name: name, score: score, lastCommit: last}
	}
	for _, c := range []struct {
		name string
		rows []printRow
		want []string
	}{
		{
			"the score wins over the date",
			[]printRow{row("clean-reciente", 1, 200), row("dirty-old", 5, 100)},
			[]string{"dirty-old", "clean-reciente"},
		},
		{
			"with the same score, the more recent commit",
			[]printRow{row("old", 5, 100), row("new", 5, 200)},
			[]string{"new", "old"},
		},
		{
			"with the same score and date, the name",
			[]printRow{row("zeta", 5, 200), row("alfa", 5, 200)},
			[]string{"alfa", "zeta"},
		},
		{
			"the name ignores case",
			[]printRow{row("Zeta", 5, 200), row("alfa", 5, 200)},
			[]string{"alfa", "Zeta"},
		},
	} {
		rows := append([]printRow(nil), c.rows...)
		sortPrintRows(rows)
		var got []string
		for _, r := range rows {
			got = append(got, r.name)
		}
		if !equalStrings(got, c.want) {
			t.Errorf("%s: orden = %v, want %v", c.name, got, c.want)
		}
		// Careful: the order of two rows tying on all THREE keys is not a contract (sort.Slice is not stable), so neither the original code nor a mutation of the comparator can promise anything there.
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

func TestPrintTableHeaderAndCells(t *testing.T) {
	root := t.TempDir()
	var dirty string
	for _, name := range []string{"dirty", "clean"} {
		dir := filepath.Join(root, name)
		testutil.Init(t, dir)
		testutil.Marker(t, dir, name, "", "", false)
		testutil.CommitFiles(t, dir, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")
		if name == "dirty" {
			dirty = dir
		}
	}
	testutil.WriteUncommitted(t, dirty, map[string]string{"new.txt": "x"})

	out := captureStdout(t, func() { runPrint(printConfig(root)) })
	plano := strings.Join(strings.Fields(out), " ")
	for _, header := range []string{"NAME", "GROUP", "BRANCH", "WT", "↑↓up", "SYNC", "WTS", "ACTIVITY", "PATH"} {
		if !strings.Contains(plano, header) {
			t.Errorf("the column %q is missing from the header:\n%s", header, plano)
		}
	}
	if !strings.Contains(plano, "?1") {
		t.Errorf("the untracked does not appear:\n%s", plano)
	}
	if strings.Contains(plano, "0 ?1") {
		t.Errorf("the WT cell printed a tracked 0 that does not exist:\n%s", plano)
	}
	for _, name := range []string{"dirty", "clean"} {
		if !strings.Contains(plano, name) {
			t.Errorf("the repo %q is missing from the output:\n%s", name, plano)
		}
	}
}

func printConfig(root string) config.Config {
	cfg := config.Defaults()
	cfg.Roots = []string{root}
	cfg.Marker = ".gitdash.toml"
	cfg.SyncBranch = "main"
	return cfg
}

func TestPrintWithoutRepos(t *testing.T) {
	empty := t.TempDir()
	out := captureStdout(t, func() { runPrint(printConfig(empty)) })
	if !strings.Contains(out, "no repositories found") {
		t.Errorf("with no repos, print = %q, want the notice", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// The worktree carries its own marker on purpose (that is what makes the walk discover it) and without `.git` it would not be a worktree: what triggers the folding is `.git` being a `gitdir:` FILE.
func TestPrintFoldsTheWorktreeUnderItsRepoMain(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "principal")
	testutil.Init(t, principal)
	testutil.Marker(t, principal, "principal", "", "", false)
	testutil.CommitFiles(t, principal, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")

	wt := filepath.Join(root, "feature")
	testutil.MakeWorktree(t, principal, wt, "feature")
	// The worktree's marker is committed WITH its content: the worktree is born from HEAD, which already carries the main repo's `.gitdash.toml`, and rewriting it without committing would leave it modified forever.
	testutil.CommitFiles(t, wt, map[string]string{".gitdash.toml": "name = \"feature\"\n"}, "worktree marker")

	out := captureStdout(t, func() { runPrint(printConfig(root)) })
	plano := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(plano, "principal") {
		t.Fatalf("the main repo does not appear:\n%s", out)
	}
	if strings.Contains(plano, "feature") {
		t.Errorf("the worktree was printed as if it were one more repo:\n%s", out)
	}
}

// Otherwise a worktree of a repo that is not scanned would vanish from the table without a trace, which is worse than showing one too many.
func TestPrintNotFoldsTheWorktreeWithoutMainDiscovered(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	principal := filepath.Join(outside, "hidden")
	testutil.Init(t, principal)
	testutil.Marker(t, principal, "hidden", "", "", false)
	testutil.CommitFiles(t, principal, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")

	wt := filepath.Join(root, "suelto")
	testutil.MakeWorktree(t, principal, wt, "suelto")
	testutil.CommitFiles(t, wt, map[string]string{".gitdash.toml": "name = \"suelto\"\n"}, "worktree marker")

	out := captureStdout(t, func() { runPrint(printConfig(root)) })
	plano := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(plano, "suelto") {
		t.Errorf("the worktree with no discovered main repo does not appear:\n%s", out)
	}
}
