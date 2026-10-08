package gitstatus

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/testutil"
)

func TestParseRefsClassifiesAndDropsHead(t *testing.T) {
	out := "refs/heads/main\nrefs/heads/dev\nrefs/remotes/origin/HEAD\nrefs/remotes/origin/main\n"
	got := ParseRefs(out)
	want := []Ref{
		{Name: "main"},
		{Name: "dev"},
		{Name: "origin/main", Remote: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRefs = %+v\nwant %+v", got, want)
	}
}

// Unknown or unrelated lines (notes, tags, the object format header) are ignored instead of becoming branches.
func TestParseRefsIgnoresWhatIsNotABranch(t *testing.T) {
	out := "refs/heads/main\nrefs/tags/v1\nrefs/notes/commits\n\n"
	if got := ParseRefs(out); !reflect.DeepEqual(got, []Ref{{Name: "main"}}) {
		t.Errorf("ParseRefs = %+v, want only main", got)
	}
	if got := ParseRefs(""); got != nil {
		t.Errorf("ParseRefs(\"\") = %+v, want nil", got)
	}
}

// One bounded read through the shared runner, read class, both heads and remotes, and no origin/HEAD.
func TestBranchRefsReadsThroughRunGitAsRead(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	testutil.NewBranch(t, dir, "feature/x")

	refs, err := BranchRefs(context.Background(), dir)
	if err != nil {
		t.Fatalf("BranchRefs: %v", err)
	}
	var names []string
	for _, r := range refs {
		names = append(names, r.Name)
		if r.Name == "origin/HEAD" {
			t.Errorf("the symbolic remote HEAD was offered: %v", names)
		}
	}
	for _, want := range []string{"main", "feature/x", "origin/main"} {
		if !containsRef(refs, want) {
			t.Errorf("BranchRefs = %v, want %q", names, want)
		}
	}

	e := lastEntry(t, rec)
	if want := "git for-each-ref --format=%(refname) refs/heads refs/remotes"; e.Command() != want {
		t.Errorf("Command() = %q, want %q", e.Command(), want)
	}
	if e.Class != cmdlog.ClassRead {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassRead)
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
}

func TestBranchRefsFailsWithTheReason(t *testing.T) {
	dir := t.TempDir() // not a git repository
	if _, err := BranchRefs(context.Background(), dir); err == nil {
		t.Fatal("BranchRefs outside a repo should fail")
	} else if !strings.Contains(err.Error(), "git") {
		t.Errorf("err = %q, want git's reason", err)
	}
}

func containsRef(refs []Ref, name string) bool {
	for _, r := range refs {
		if r.Name == name {
			return true
		}
	}
	return false
}
