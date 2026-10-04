package gitstatus

import (
	"errors"
	"strings"
	"testing"

	"gitdash/internal/testutil"
)

func TestFailureReason(t *testing.T) {
	out := "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
		"hint:\n" +
		"fatal: Not possible to fast-forward, aborting.\n"
	if got := FailureReason(out, errors.New("exit status 128")); got != "fatal: Not possible to fast-forward, aborting." {
		t.Errorf("FailureReason = %q, want the fatal line", got)
	}
	if got := FailureReason("", errors.New("exit status 1")); got != "exit status 1" {
		t.Errorf("FailureReason with no output = %q, want the fallback", got)
	}
}

func TestPullDivergedReportsReason(t *testing.T) {
	diverged, origin := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, diverged, map[string]string{"l.txt": "l"}, "local")
	testutil.PushUpstreamCommits(t, origin, 2, "div-")
	testutil.FetchLocal(t, diverged)

	// The flag is explicit: the message being checked is the ff-only one, and leaving it to the gitconfig would make the test depend on the machine.
	out, err := Run(t.Context(), diverged, "pull", "--ff-only")
	if err == nil {
		t.Fatalf("the diverged pull did not fail:\n%s", out)
	}
	reason := FailureReason(out, err)
	if !strings.Contains(reason, "Not possible to fast-forward") {
		t.Errorf("reason = %q, want 'Not possible to fast-forward'", reason)
	}
	if reason == err.Error() {
		t.Errorf("reason = %q: it is reporting the exit status, not the reason", reason)
	}
}

func TestPullReasonIgnoresLocale(t *testing.T) {
	t.Setenv("LC_ALL", "es_ES.UTF-8")
	t.Setenv("LANG", "es_ES.UTF-8")

	dir, _ := testutil.NewRepo(t, false) // no upstream
	out, err := Run(t.Context(), dir, "pull")
	if err == nil {
		t.Fatalf("the pull with no upstream did not fail:\n%s", out)
	}
	reason := FailureReason(out, err)
	if !strings.Contains(reason, "no tracking information") {
		t.Errorf("reason = %q, want an English message despite the locale", reason)
	}
}

// FailureReason has three outcomes and only the first was being tested: empty output with an error falls back to the process error (what a network failure or a downed remote looks like, where git prints nothing), and empty output WITHOUT an error means there is nothing to say, so the UI must be able to paint an empty string instead of an invented "error" (a failure with no text looks like a failure with no explanation).
func TestFailureReasonWithoutOutput(t *testing.T) {
	boom := errors.New("exit status 128")
	if got := FailureReason("", boom); got != "exit status 128" {
		t.Errorf("FailureReason empty with an error = %q, want the error text", got)
	}
	out := "\n\nhint: if this were a rebase...\n   \nhint: another hint\n"
	if got := FailureReason(out, boom); got != "exit status 128" {
		t.Errorf("FailureReason hints-only = %q, want the fallback to the error", got)
	}
	if got := FailureReason("", nil); got != "" {
		t.Errorf("FailureReason with nothing = %q, want empty", got)
	}
	if got := FailureReason("fatal: could not read Username\nhint: x\n", boom); got != "fatal: could not read Username" {
		t.Errorf("FailureReason = %q, want the first output line", got)
	}
}
