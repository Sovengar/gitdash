package gitstatus

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/testutil"
)

func TestParseWorktrees(t *testing.T) {
	out := "worktree /repos/main\nHEAD abc1234def\nbranch refs/heads/main\n\n" +
		"worktree /repos/wt1\nHEAD 1234567890abc\nbranch refs/heads/wt-1\n\n" +
		"worktree /repos/wt2\nHEAD def5678lorem\ndetached\n\n"
	wts := ParseWorktrees(out, "/repos/main")
	if len(wts) != 2 {
		t.Fatalf("wts = %d, want 2 (main excluded)", len(wts))
	}
	if wts[0].Path != "/repos/wt1" || wts[0].Branch != "wt-1" || wts[0].Head != "1234567" {
		t.Errorf("wts[0] = %+v", wts[0])
	}
	if wts[1].Path != "/repos/wt2" || wts[1].Branch != "" {
		t.Errorf("wts[1] = %+v (detached: empty branch)", wts[1])
	}
}

func TestCollectWorktrees(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtA := t.TempDir()
	testutil.MakeWorktree(t, dir, wtA, "wt-a")
	wtB := t.TempDir()
	testutil.MakeWorktree(t, dir, wtB, "wt-b")

	snap := Collect(t.Context(), dir, "", false)
	if len(snap.Worktrees) != 2 {
		t.Fatalf("wts = %d, want 2 (with marker or not)", len(snap.Worktrees))
	}
	if snap.Worktrees[0].Branch != "wt-a" {
		t.Errorf("branch = %q, want wt-a", snap.Worktrees[0].Branch)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestRemoveWorktreeRemovesAndKeepsBranch(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-a")
	testutil.MakeWorktree(t, dir, wtDir, "wt-a")

	if _, err := RemoveWorktree(t.Context(), dir, wtDir, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("the worktree folder still exists: %v", err)
	}
	if snap := Collect(t.Context(), dir, "", false); len(snap.Worktrees) != 0 {
		t.Errorf("worktrees after deleting = %d, want 0", len(snap.Worktrees))
	}
	if branches := gitOut(t, dir, "branch", "--list", "wt-a"); !strings.Contains(branches, "wt-a") {
		t.Errorf("the wt-a branch disappeared: %q", branches)
	}
}

func TestRemoveWorktreeDirtyNeedsForce(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-dirty")
	testutil.MakeWorktree(t, dir, wtDir, "wt-dirty")
	testutil.WriteUntracked(t, wtDir, map[string]string{"untracked.txt": "uncommitted"})

	out, err := RemoveWorktree(t.Context(), dir, wtDir, false)
	if err == nil {
		t.Fatalf("expected an error without force; out=%q", out)
	}
	if reason := FailureReason(out, err); reason == "" {
		t.Error("FailureReason empty for the failure without force")
	}
	if _, statErr := os.Stat(wtDir); statErr != nil {
		t.Errorf("the dirty worktree was deleted despite the failure: %v", statErr)
	}

	if _, err := RemoveWorktree(t.Context(), dir, wtDir, true); err != nil {
		t.Fatalf("RemoveWorktree force: %v", err)
	}
	if _, statErr := os.Stat(wtDir); !os.IsNotExist(statErr) {
		t.Errorf("the worktree still exists after force: %v", statErr)
	}
}

func TestRemoveWorktreeDetached(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-det")
	testutil.MakeWorktree(t, dir, wtDir, "wt-det")
	testutil.Detach(t, wtDir)

	if _, err := RemoveWorktree(t.Context(), dir, wtDir, false); err != nil {
		t.Fatalf("RemoveWorktree detached: %v", err)
	}
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("the detached folder still exists: %v", err)
	}
}

func TestRemoveWorktreePrunableFailsGracefully(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-prune")
	testutil.MakeWorktree(t, dir, wtDir, "wt-prune")
	if err := os.RemoveAll(wtDir); err != nil {
		t.Fatal(err)
	}

	out, err := RemoveWorktree(t.Context(), dir, wtDir, false)
	if err != nil && FailureReason(out, err) == "" {
		t.Errorf("error with no summarisable reason: err=%v out=%q", err, out)
	}
	if snap := Collect(t.Context(), dir, "", false); len(snap.Worktrees) != 0 {
		t.Errorf("the orphan registration is still listed: %+v", snap.Worktrees)
	}
	_, _ = RemoveWorktree(t.Context(), dir, wtDir, true)

	notWT := filepath.Join(t.TempDir(), "no-worktree")
	if err := os.MkdirAll(notWT, 0o755); err != nil {
		t.Fatal(err)
	}
	out2, err2 := RemoveWorktree(t.Context(), dir, notWT, false)
	if err2 == nil {
		t.Fatalf("expected an error for a non-worktree path; out=%q", out2)
	}
	if FailureReason(out2, err2) == "" {
		t.Error("FailureReason empty for a non-worktree path")
	}
}
