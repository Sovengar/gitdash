package gitstatus

import (
	"os/exec"
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
		t.Errorf("behind = %d, want 1 (solo m1 ausente)", snap.SyncBehind)
	}
}

func TestSyncOnBranch(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "feat", false)
	if !snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("known=%v behind=%d", snap.SyncKnown, snap.SyncBehind)
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
	if snap.Err != "" {
		t.Errorf("the sync error must not pollute Err: %q", snap.Err)
	}
}

func TestSyncDisabled(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "", false)
	if snap.SyncKnown || snap.SyncBranch != "" {
		t.Errorf("sync = %v/%d/%q, want desconocida", snap.SyncKnown, snap.SyncBehind, snap.SyncBranch)
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

func repoSinMain(t *testing.T) string {
	t.Helper()
	dir, _ := testutil.NewRepo(t, false)
	gitLocal(t, dir, "branch", "-m", "master")
	return dir
}

// What resolves is what gets COMPARED and what is SHOWN: leaving "main" in the column would keep showing "main —" and `glab mr create -b main` would keep failing.
func TestSyncFallbackAMaster(t *testing.T) {
	dir := repoSinMain(t)
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
	dir := repoSinMain(t)
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
	casos := []struct {
		name         string
		p            discovery.Project
		global       string
		explicit     bool
		want         string
		wantFallback bool
	}{
		{"undeclared default", discovery.Project{}, "main", false, "main", true},
		{"marcador declarado", discovery.Project{SyncBranch: "develop"}, "main", false, "develop", false},
		{"explicit global config", discovery.Project{}, "release", true, "release", false},
		{"marker wins over the explicit global", discovery.Project{SyncBranch: "develop"}, "release", true, "develop", false},
	}
	for _, c := range casos {
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
	dir := repoSinMain(t)
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
	dir := repoSinMain(t)
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

func TestSyncFallbackLaunchesAGitAgainstMaster(t *testing.T) {
	rec := installRecorder(t)
	dir := repoSinMain(t)
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
