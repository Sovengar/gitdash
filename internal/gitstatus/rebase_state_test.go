package gitstatus

import (
	"os"
	"path/filepath"
	"testing"

	"gitdash/internal/testutil"
)

func TestRebaseInProgressFalse(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, dir, map[string]string{"l.txt": "local\n"}, "local")
	testutil.PushUpstreamFile(t, origin, "u.txt", "remote\n", "remote")
	testutil.FetchLocal(t, dir)

	if out, err := Run(t.Context(), dir, "pull", "--rebase"); err != nil {
		t.Fatalf("the pull of a non-diverged repo failed:\n%s", out)
	}
	if RebaseInProgress(t.Context(), dir) {
		t.Error("RebaseInProgress = true in a repo with no rebase")
	}
}

func TestRebaseInProgressTrue(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, dir, map[string]string{"c.txt": "local\n"}, "local")
	testutil.PushUpstreamFile(t, origin, "c.txt", "remote\n", "remote")
	testutil.FetchLocal(t, dir)

	out, err := Run(t.Context(), dir, "pull", "--rebase")
	if err == nil {
		t.Skipf("the pull did not clash (fixture has no conflict):\n%s", out)
	}
	if !RebaseInProgress(t.Context(), dir) {
		t.Errorf("RebaseInProgress = false after a clashing rebase:\n%s", out)
	}
}

func TestRebaseInProgressNotRepo(t *testing.T) {
	if RebaseInProgress(t.Context(), t.TempDir()) {
		t.Error("RebaseInProgress = true outside a repo")
	}
}

func TestRebaseInProgressInWorktree(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	wt := t.TempDir()
	testutil.MakeWorktree(t, dir, wt, "feat")

	testutil.CommitFiles(t, wt, map[string]string{"c.txt": "local\n"}, "local")
	testutil.PushUpstreamFile(t, origin, "c.txt", "remote\n", "remote")
	testutil.FetchLocal(t, wt)
	if out, err := Run(t.Context(), wt, "pull", "--rebase", "origin", "main"); err == nil {
		t.Skipf("the pull did not clash:\n%s", out)
	}

	if !RebaseInProgress(t.Context(), wt) {
		t.Error("RebaseInProgress = false in a worktree with a rebase in progress")
	}
	if RebaseInProgress(t.Context(), dir) {
		t.Error("the worktree's rebase was attributed to the main repo")
	}
}

// The worktree test above mounts the real case (a pull --rebase that clashes); this one mounts the early case, the one you get when the conflict happens in the commit being applied: git creates the state directory before attempting the pick, so the warning must also appear with no rewritten history.
func TestRebaseInProgressWithoutPicking(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)

	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		t.Run(name, func(t *testing.T) {
			state := filepath.Join(dir, ".git", name)
			if err := os.MkdirAll(state, 0o755); err != nil {
				t.Fatal(err)
			}
			if !RebaseInProgress(t.Context(), dir) {
				t.Errorf("RebaseInProgress = false with %s on disk, want true", name)
			}
			if err := os.RemoveAll(state); err != nil {
				t.Fatal(err)
			}
			if RebaseInProgress(t.Context(), dir) {
				t.Errorf("RebaseInProgress = true after removing %s", name)
			}
		})
	}
}
