// Tests de worktrees: parseo canned y recolección real.
package gitstatus

import (
	"testing"

	"gitdash/internal/testutil"
)

func TestParseWorktrees(t *testing.T) {
	out := "worktree /repos/main\nHEAD abc1234def\nbranch refs/heads/main\n\n" +
		"worktree /repos/wt1\nHEAD 1234567890abc\nbranch refs/heads/wt-1\n\n" +
		"worktree /repos/wt2\nHEAD def5678lorem\ndetached\n\n"
	wts := ParseWorktrees(out, "/repos/main")
	if len(wts) != 2 {
		t.Fatalf("wts = %d, want 2 (main excluido)", len(wts))
	}
	if wts[0].Path != "/repos/wt1" || wts[0].Branch != "wt-1" || wts[0].Head != "1234567" {
		t.Errorf("wts[0] = %+v", wts[0])
	}
	if wts[1].Path != "/repos/wt2" || wts[1].Branch != "" {
		t.Errorf("wts[1] = %+v (detached: branch vacía)", wts[1])
	}
}

// el conteo incluye worktrees aunque no tengan marcador.
func TestCollectWorktrees(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtA := t.TempDir()
	testutil.MakeWorktree(t, dir, wtA, "wt-a")
	wtB := t.TempDir()
	testutil.MakeWorktree(t, dir, wtB, "wt-b")

	snap := Collect(t.Context(), dir, "")
	if len(snap.Worktrees) != 2 {
		t.Fatalf("wts = %d, want 2 (marcador o no)", len(snap.Worktrees))
	}
	if snap.Worktrees[0].Branch != "wt-a" {
		t.Errorf("branch = %q, want wt-a", snap.Worktrees[0].Branch)
	}
}
