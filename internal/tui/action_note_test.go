package tui

import (
	"strings"
	"testing"
)

func TestActionNoteOk(t *testing.T) {
	if got := actionNote("pull", "repo-a", "", "", "", false); got != "pull ok repo-a" {
		t.Errorf("actionNote ok = %q", got)
	}
}

func TestActionNoteOkShowsResolvedCommand(t *testing.T) {
	got := actionNote("pull_rebase", "repo-a", "git pull --rebase --autostash", "", "", false)
	if !strings.Contains(got, "git pull --rebase --autostash") {
		t.Errorf("actionNote = %q, want the resolved argv", got)
	}
}

func TestActionNotePullDiverged(t *testing.T) {
	out := "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
		"hint:\n" +
		"fatal: Not possible to fast-forward, aborting.\n"
	got := actionNote("pull_ff", "repo-a", "git pull --ff-only", out,
		"fatal: Not possible to fast-forward, aborting.", false)
	if !strings.Contains(got, "pull_ff failed repo-a") {
		t.Errorf("actionNote = %q, want the failure prefix", got)
	}
	if !strings.Contains(got, "Not possible to fast-forward") {
		t.Errorf("actionNote = %q, want the reason real", got)
	}
	if !strings.Contains(got, "diverged") {
		t.Errorf("actionNote = %q, want the divergence hint", got)
	}
}

func TestActionNotePullNotUpstream(t *testing.T) {
	got := actionNote("pull", "repo-b", "git pull",
		"There is no tracking information for the current branch.\n",
		"There is no tracking information for the current branch.", false)
	if !strings.Contains(got, "no tracking information") {
		t.Errorf("actionNote = %q, want the reason", got)
	}
	if !strings.Contains(got, "P publishes it") {
		t.Errorf("actionNote = %q, want the upstream hint", got)
	}
}

func TestActionNotePushNotDivergedHint(t *testing.T) {
	got := actionNote("push", "repo-c", "git push",
		"fatal: Not possible to fast-forward, aborting.",
		"fatal: Not possible to fast-forward, aborting.", false)
	if strings.Contains(got, "diverged") {
		t.Errorf("actionNote push = %q, it must not carry the divergence hint", got)
	}
}

func TestActionNoteRebaseInProgressWins(t *testing.T) {
	out := "CONFLICT (content): Merge conflict in f.txt\n"
	got := actionNote("pull_rebase", "repo-a", "git pull --rebase --autostash", out,
		"error: could not apply 1234567... local", true)
	if !strings.Contains(got, "mid-rebase") {
		t.Errorf("actionNote = %q, want the mid-rebase warning", got)
	}
	if !strings.Contains(got, "rebase --continue") {
		t.Errorf("actionNote = %q, want how to continue", got)
	}
	if strings.Contains(got, "diverged") || strings.Contains(got, "no upstream") {
		t.Errorf("actionNote = %q, the secondary hints must be left out", got)
	}
}

func TestActionNotePushIgnoresRebaseFlag(t *testing.T) {
	got := actionNote("push", "repo-c", "git push", "fatal: x", "fatal: x", true)
	if strings.Contains(got, "mid-rebase") {
		t.Errorf("actionNote push = %q, it must not mention rebase", got)
	}
}

func TestActionNoteShowsTheArgvOnlyIfExists(t *testing.T) {
	t.Run("with argv on success", func(t *testing.T) {
		got := actionNote("pull", "api", "git pull --rebase", "", "", false)
		if !strings.Contains(got, "git pull --rebase") {
			t.Errorf("note = %q, want the argv", got)
		}
	})
	t.Run("without argv on success", func(t *testing.T) {
		got := actionNote("pull", "api", "", "", "", false)
		if strings.Contains(got, "git ") || strings.Contains(got, "()") {
			t.Errorf("note = %q, want no argv and no empty parentheses", got)
		}
	})
	t.Run("with argv on failure", func(t *testing.T) {
		got := actionNote("pull", "api", "git pull --rebase", "", "could not apply abc", false)
		if !strings.Contains(got, "git pull --rebase") {
			t.Errorf("note = %q, want the argv and the reason", got)
		}
		if !strings.Contains(got, "could not apply") {
			t.Errorf("note = %q, want the reason de git", got)
		}
	})
	t.Run("without argv on failure", func(t *testing.T) {
		got := actionNote("push", "api", "", "", "permission denied", false)
		if strings.Contains(got, "—  ") {
			t.Errorf("note = %q, want no dangling dash", got)
		}
		if !strings.Contains(got, "permission denied") {
			t.Errorf("note = %q, want the reason", got)
		}
	})
}
