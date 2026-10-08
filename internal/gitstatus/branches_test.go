package gitstatus

import (
	"strings"
	"testing"

	"gitdash/internal/testutil"
)

func TestLocalBranchesListsEveryLocalBranch(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feature")
	testutil.Checkout(t, dir, "main")

	branches, err := LocalBranches(t.Context(), dir)
	if err != nil {
		t.Fatalf("LocalBranches failed: %v", err)
	}
	got := strings.Join(branches, ",")
	for _, want := range []string{"main", "feature"} {
		if !strings.Contains(got, want) {
			t.Errorf("branches = %v, want %q", branches, want)
		}
	}
}

func TestLocalBranchesFailsOutsideARepo(t *testing.T) {
	if _, err := LocalBranches(t.Context(), t.TempDir()); err == nil {
		t.Error("LocalBranches outside a repo = nil error, want a failure")
	}
}
