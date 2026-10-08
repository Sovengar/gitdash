package gitstatus

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/testutil"
)

func setupDivergedFromSync(t *testing.T) (dir string) {
	t.Helper()
	dir, _ = testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feat")
	testutil.CommitFiles(t, dir, map[string]string{"f.txt": "f"}, "feat 1")
	testutil.Checkout(t, dir, "main")
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "main 1")
	testutil.Checkout(t, dir, "feat")
	return dir
}

// Behind counts the sync commits (m1) missing from the current branch, NOT feat's own ones (merge-base, not a diff of tips).
func TestSyncBehind(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "main", false)
	if snap.Err != "" {
		t.Fatalf("err = %q", snap.Err)
	}
	if !snap.SyncKnown || snap.SyncBranch != "main" {
		t.Fatalf("sync not known: %+v", snap)
	}
	if snap.SyncBehind != 1 {
		t.Errorf("behind = %d, want 1 (only m1 missing)", snap.SyncBehind)
	}
	// The panel's second group comes from the same ref the divergence counts, newest first.
	if len(snap.SyncCommits) != 2 || snap.SyncCommits[0].Subject != "main 1" {
		t.Errorf("SyncCommits = %+v, want the sync branch's log newest first", snap.SyncCommits)
	}
	if snap.Commits[0].Subject != "feat 1" {
		t.Errorf("the current branch's commits = %+v, want feat's own", snap.Commits)
	}
	// The mark follows the sha, so the same commits the count claims are the ones flagged.
	if !snap.Commits[0].OneSided || snap.Commits[1].OneSided {
		t.Errorf("current flags = %v/%v, want the feat-only marked and the shared not", snap.Commits[0].OneSided, snap.Commits[1].OneSided)
	}
	if !snap.SyncCommits[0].OneSided || snap.SyncCommits[1].OneSided {
		t.Errorf("sync flags = %v/%v, want the sync-only marked and the shared not", snap.SyncCommits[0].OneSided, snap.SyncCommits[1].OneSided)
	}
}

func TestSyncOnBranch(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "feat", false)
	if !snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("known=%v behind=%d", snap.SyncKnown, snap.SyncBehind)
	}
	// On the sync branch itself the two lists would duplicate: no second log, no second group.
	if len(snap.SyncCommits) != 0 {
		t.Errorf("SyncCommits = %+v, want none on the sync branch", snap.SyncCommits)
	}
}

func TestSyncMissing(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "nonexistent", false)
	if snap.SyncKnown {
		t.Errorf("known=true with a nonexistent ref")
	}
	if snap.SyncBranch != "nonexistent" {
		t.Errorf("SyncBranch = %q, want nonexistent (always filled)", snap.SyncBranch)
	}
	if len(snap.SyncCommits) != 0 {
		t.Errorf("SyncCommits = %+v, want none without a ref to log", snap.SyncCommits)
	}
	if snap.Err != "" {
		t.Errorf("the sync error must not pollute Err: %q", snap.Err)
	}
}

func TestSyncDisabled(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "", false)
	if snap.SyncKnown || snap.SyncBranch != "" {
		t.Errorf("sync = %v/%d/%q, want unknown", snap.SyncKnown, snap.SyncBehind, snap.SyncBranch)
	}
}

func TestStreamPoolSyncOverride(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "main 1")
	testutil.NewBranch(t, dir, "feat")
	projects := []discovery.Project{
		{Path: dir, HasRepo: true, SyncBranch: "main"},
	}

	got := map[string]Snapshot{}
	var mu sync.Mutex
	StreamPool(t.Context(), projects, "global-branch", false, 2, func(path string, snap Snapshot) {
		mu.Lock()
		got[path] = snap
		mu.Unlock()
	})
	snap := got[dir]
	if !snap.SyncKnown || snap.SyncBranch != "main" {
		t.Errorf("override not applied: %+v", snap)
	}
}

// testutil fixtures are always born on main (Init does `git init -b main`) and the fallback case needs the opposite, a repo WITHOUT main: renaming the initial branch has no helper.
func gitLocal(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func repoWithoutMain(t *testing.T) string {
	t.Helper()
	dir, _ := testutil.NewRepo(t, false)
	gitLocal(t, dir, "branch", "-m", "master")
	return dir
}

// What resolves is what gets COMPARED and what is SHOWN: leaving "main" in the column would keep showing "main —" and `glab mr create -b main` would keep failing.
func TestSyncFallbackAMaster(t *testing.T) {
	dir := repoWithoutMain(t)
	testutil.NewBranch(t, dir, "feat")
	testutil.Checkout(t, dir, "master")
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "master 1")
	testutil.Checkout(t, dir, "feat")
	testutil.CommitFiles(t, dir, map[string]string{"f.txt": "f"}, "feat 1")

	snap := Collect(t.Context(), dir, "main", true)
	if snap.Err != "" {
		t.Fatalf("err = %q", snap.Err)
	}
	if snap.SyncBranch != "master" {
		t.Errorf("SyncBranch = %q, want master (what resolves is what is shown)", snap.SyncBranch)
	}
	if !snap.SyncKnown {
		t.Fatalf("comparison against master unknown: %+v", snap)
	}
	if snap.SyncBehind != 1 {
		t.Errorf("behind = %d, want 1 (m1 missing on feat)", snap.SyncBehind)
	}
	// What resolves is also what gets logged: the fallback's commits feed the second group.
	if len(snap.SyncCommits) == 0 || snap.SyncCommits[0].Subject != "master 1" {
		t.Errorf("SyncCommits = %+v, want master's log", snap.SyncCommits)
	}
}

func TestSyncFallbackWithoutNoneOfTheTwo(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	gitLocal(t, dir, "branch", "-m", "trunk")

	snap := Collect(t.Context(), dir, "main", true)
	if snap.SyncBranch != "main" {
		t.Errorf("SyncBranch = %q, want main (the original)", snap.SyncBranch)
	}
	if snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("sync = %v/%d, want unknown with nothing behind it", snap.SyncKnown, snap.SyncBehind)
	}
}

func TestSyncFallbackNotLaunchesGitIfTheGlobalResolves(t *testing.T) {
	rec := installRecorder(t)
	dir := setupDivergedFromSync(t)

	snap := Collect(t.Context(), dir, "main", true)
	if snap.SyncBranch != "main" || !snap.SyncKnown || snap.SyncBehind != 1 {
		t.Fatalf("with main present nothing changes: %+v", snap)
	}
	for _, e := range rec.Entries() {
		if strings.Contains(e.Command(), "master") {
			t.Errorf("extra git against master with the global resolved: %q", e.Command())
		}
	}
}

func TestSyncWithoutFallbackAllowedNotChanges(t *testing.T) {
	dir := repoWithoutMain(t)
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "master 1")

	snap := Collect(t.Context(), dir, "main", false)
	if snap.SyncBranch != "main" {
		t.Errorf("SyncBranch = %q, want main (without fallback what was asked for is not touched)", snap.SyncBranch)
	}
	if snap.SyncKnown {
		t.Error("SyncKnown without the requested ref")
	}
}

func TestSyncForFallbackOnlyInTheDefault(t *testing.T) {
	cases := []struct {
		name         string
		p            discovery.Project
		global       string
		explicit     bool
		want         string
		wantFallback bool
	}{
		{"undeclared default", discovery.Project{}, "main", false, "main", true},
		{"declared marker", discovery.Project{SyncBranch: "develop"}, "main", false, "develop", false},
		{"explicit global config", discovery.Project{}, "release", true, "release", false},
		{"marker wins over the explicit global", discovery.Project{SyncBranch: "develop"}, "release", true, "develop", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SyncFor(c.p, c.global); got != c.want {
				t.Errorf("SyncFor = %q, want %q", got, c.want)
			}
			if got := SyncForAllowsFallback(c.p, c.explicit); got != c.wantFallback {
				t.Errorf("SyncForAllowsFallback = %v, want %v", got, c.wantFallback)
			}
		})
	}
}

func TestSyncReferenceDeclaredNotMakesFallback(t *testing.T) {
	dir := repoWithoutMain(t)
	snap := Collect(t.Context(), dir, "nonexistent", false)
	if snap.SyncKnown {
		t.Error("known with a declared ref that does not exist")
	}
	if snap.SyncBranch != "nonexistent" {
		t.Errorf("SyncBranch = %q, want nonexistent (what was declared wins)", snap.SyncBranch)
	}
}

// It is the difference between passing the flag and always passing false, which compiles just as well and does not say the same thing.
func TestStreamPoolPropagatesTheFallback(t *testing.T) {
	dir := repoWithoutMain(t)
	projects := []discovery.Project{{Path: dir, HasRepo: true}}
	for _, c := range []struct {
		name     string
		explicit bool
		want     string
		known    bool
	}{
		{"default", false, "master", true},
		{"explicit config", true, "main", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := map[string]Snapshot{}
			var mu sync.Mutex
			StreamPool(t.Context(), projects, "main", c.explicit, 2, func(path string, snap Snapshot) {
				mu.Lock()
				got[path] = snap
				mu.Unlock()
			})
			snap := got[dir]
			if snap.SyncBranch != c.want || snap.SyncKnown != c.known {
				t.Errorf("SyncBranch/known = %q/%v, want %q/%v", snap.SyncBranch, snap.SyncKnown, c.want, c.known)
			}
		})
	}
}

func TestSyncArgvAppendsTheRemoteAndBranchToTheBase(t *testing.T) {
	base := []string{"pull", "--rebase", "--autostash"}
	got := strings.Join(SyncArgv(base, "origin", "main"), " ")
	if want := "pull --rebase --autostash origin main"; got != want {
		t.Errorf("SyncArgv = %q, want %q", got, want)
	}
	// The base may be a shared default map value: appending to it in place would corrupt every later sync.
	if got := strings.Join(base, " "); got != "pull --rebase --autostash" {
		t.Errorf("the base was mutated: %q", got)
	}
}

func TestSyncFallbackLaunchesAGitAgainstMaster(t *testing.T) {
	rec := installRecorder(t)
	dir := repoWithoutMain(t)
	Collect(t.Context(), dir, "main", true)

	var probadaMain, probadaMaster bool
	for _, e := range rec.Entries() {
		switch e.Command() {
		case "git rev-list --count HEAD..main":
			probadaMain = true
			if e.Exit == 0 {
				t.Error("main resolved in a repo that does not have it")
			}
		case "git rev-list --count HEAD..master":
			probadaMaster = true
			if e.Exit != 0 {
				t.Errorf("Exit = %d, want 0 (master does exist)", e.Exit)
			}
		}
	}
	if !probadaMain {
		t.Error("the global ref was not tried before falling back to master")
	}
	if !probadaMaster {
		t.Error("the fallback never got to try master")
	}
}

// An ahead-only branch marks its own side and nothing on the sync side.
func TestSyncOneSidedAheadOnly(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feat")
	testutil.CommitFiles(t, dir, map[string]string{"f.txt": "f"}, "feat 1")

	snap := Collect(t.Context(), dir, "main", false)
	if !snap.SyncKnown || snap.SyncBehind != 0 {
		t.Fatalf("known/behind = %v/%d, want true/0", snap.SyncKnown, snap.SyncBehind)
	}
	if !snap.Commits[0].OneSided {
		t.Errorf("the feat-only commit is not marked: %+v", snap.Commits)
	}
	for i, c := range snap.SyncCommits {
		if c.OneSided {
			t.Errorf("sync commit %d marked with nothing behind it: %+v", i, c)
		}
	}
}

// Both logs are capped at the panel's depth, pinned as literals so a mutated constant cannot move the assertion with it. The counts are NOT capped: they are the full divergence.
func TestCollectCapsBothLogsAt15(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	for i := 0; i < 17; i++ {
		testutil.CommitFiles(t, dir, map[string]string{"m.txt": fmt.Sprint(i)}, "main "+fmt.Sprint(i))
	}
	testutil.NewBranch(t, dir, "feat")
	for i := 0; i < 17; i++ {
		testutil.CommitFiles(t, dir, map[string]string{"f.txt": fmt.Sprint(i)}, "feat "+fmt.Sprint(i))
	}

	snap := Collect(t.Context(), dir, "main", false)
	if len(snap.Commits) != 15 || len(snap.SyncCommits) != 15 {
		t.Errorf("current/sync commits = %d/%d, want 15/15", len(snap.Commits), len(snap.SyncCommits))
	}
	if snap.SyncBehind != 0 {
		t.Errorf("behind = %d, want 0", snap.SyncBehind)
	}
	// The marks come from the same capped window: the newest and the last painted rows are one-sided, and the sync side has none.
	if !snap.Commits[0].OneSided || !snap.Commits[14].OneSided || snap.SyncCommits[0].OneSided {
		t.Errorf("flags = current[0]=%v current[14]=%v sync[0]=%v, want true/true/false", snap.Commits[0].OneSided, snap.Commits[14].OneSided, snap.SyncCommits[0].OneSided)
	}
}

// commitAt writes files and commits with explicit dates: the merge fixture needs the sync tip strictly
// newer than the branch's own commit, or tied wall-clock seconds would let a positional rule pass it.
func commitAt(t *testing.T, dir, date, msg string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add in %s: %v\n%s", dir, err, out)
	}
	cmd = exec.Command("git", "commit", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q in %s: %v\n%s", msg, dir, err, out)
	}
}

// The merge case that killed the positional rule: the sync tip comes into the current log NEWER than
// the branch's own commit (forced dates, not wall-clock luck), and the marks still follow the sha.
func TestCollectMarksOneSidedCommitsUnderAMerge(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feat")
	commitAt(t, dir, "2026-01-01T00:00:00Z", "c4", map[string]string{"c4.txt": "c4"})
	testutil.Checkout(t, dir, "main")
	commitAt(t, dir, "2026-01-02T00:00:00Z", "S3", map[string]string{"s3.txt": "s3"})
	testutil.Checkout(t, dir, "feat")
	gitLocal(t, dir, "merge", "--no-edit", "-m", "merged", "main")

	snap := Collect(t.Context(), dir, "main", false)
	if snap.SyncBehind != 0 {
		t.Fatalf("behind = %d, want 0 after merging main", snap.SyncBehind)
	}
	flags := map[string]bool{}
	for _, c := range snap.Commits {
		flags[c.Subject] = c.OneSided
	}
	// S3 (shared) sorts ABOVE c4 (one-sided), so a positional rule would mark merged and a shared row and leave c4 neutral: every assertion below fails if the rule comes back.
	if !flags["c4"] || !flags["merged"] {
		t.Errorf("the branch's own commits are not marked: %v", flags)
	}
	if flags["S3"] || flags["base"] {
		t.Errorf("a shared commit is marked as one-sided: %v", flags)
	}
	for _, c := range snap.SyncCommits {
		if c.OneSided {
			t.Errorf("a sync-side commit marked with nothing behind it: %+v", c)
		}
	}
}

// An empty parsed sha would prefix-match every rev-list sha; it is skipped instead of being marked.
func TestMarkOneSidedSkipsAnEmptySha(t *testing.T) {
	commits := []Commit{{Sha: ""}, {Sha: "abc"}}
	markOneSided(commits, map[string]bool{"abc1234def": true})
	if commits[0].OneSided {
		t.Error("an empty sha was marked")
	}
	if !commits[1].OneSided {
		t.Error("the abbreviated sha did not match its full one")
	}
}

func TestOneSidedShasWithAnUnresolvableRange(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	if shas := oneSidedShas(t.Context(), dir, "HEAD..origin/nope"); shas != nil {
		t.Errorf("oneSidedShas = %v, want nil when rev-list fails", shas)
	}
}
