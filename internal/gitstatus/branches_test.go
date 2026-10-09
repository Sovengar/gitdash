package gitstatus

import (
	"testing"

	"gitdash/internal/testutil"
)

func branchByName(branches []Branch, name string) (Branch, bool) {
	for _, b := range branches {
		if b.Name == name {
			return b, true
		}
	}
	return Branch{}, false
}

func TestBranchesListsLocalRemoteAndUpstreamFlags(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.Checkout(t, dir, "main")
	// A local branch with no upstream: feature was created locally and never pushed.
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)

	branches, err := Branches(t.Context(), dir)
	if err != nil {
		t.Fatalf("Branches failed: %v", err)
	}

	main, ok := branchByName(branches, "main")
	if !ok {
		t.Fatalf("branches = %v, want main", branches)
	}
	if !main.Current || !main.HasUpstream {
		t.Errorf("main = %+v, want current with an upstream", main)
	}

	feature, ok := branchByName(branches, "feature")
	if !ok {
		t.Fatalf("branches = %v, want the local no-upstream branch feature", branches)
	}
	if feature.Current || feature.HasUpstream {
		t.Errorf("feature = %+v, want a non-current branch with no upstream", feature)
	}

	if _, ok := branchByName(branches, "origin/main"); !ok {
		t.Errorf("branches = %v, want the origin remote-tracking ref", branches)
	}
	for _, b := range branches {
		if b.Name == "origin/HEAD" {
			t.Errorf("origin/HEAD is in the list: %v", branches)
		}
	}
}

func TestBranchesFailsOutsideARepo(t *testing.T) {
	if _, err := Branches(t.Context(), t.TempDir()); err == nil {
		t.Error("Branches outside a repo = nil error, want a failure")
	}
}

// The parsers take git's raw output so their defensive guards are reachable without a fixture git cannot produce.
func TestParseLocalBranches(t *testing.T) {
	out := "main\x00origin/main\x00*\nfeature\x00\x00 \n\x00\x00x\n"
	got := parseLocalBranches(out)
	if len(got) != 2 {
		t.Fatalf("parsed %d branches, want 2 (the nameless line is dropped): %+v", len(got), got)
	}
	if got[0].Name != "main" || !got[0].Current || !got[0].HasUpstream {
		t.Errorf("main = %+v, want current with an upstream", got[0])
	}
	if got[1].Name != "feature" || got[1].Current || got[1].HasUpstream {
		t.Errorf("feature = %+v, want plain and no upstream", got[1])
	}
}

func TestParseOriginRemotesSkipsHeadAndOtherRemotes(t *testing.T) {
	got := parseOriginRemotes("origin/main\norigin/HEAD\nupstream/foo\n\n")
	if len(got) != 1 || got[0].Name != "origin/main" {
		t.Errorf("remotes = %+v, want only origin/main", got)
	}
}
