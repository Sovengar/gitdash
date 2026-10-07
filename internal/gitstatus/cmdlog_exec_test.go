package gitstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/testutil"
)

// The recorder is global because gitdash's git execs are in this package and not in the TUI: threading it through Collect/StreamPool/Fetch/Run would add nothing.
func installRecorder(t *testing.T) *cmdlog.Recorder {
	t.Helper()
	rec := cmdlog.New(cmdlog.DefaultCap)
	cmdlog.SetRecorder(rec)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	return rec
}

func lastEntry(t *testing.T, rec *cmdlog.Recorder) cmdlog.Entry {
	t.Helper()
	entries := rec.Entries()
	if len(entries) == 0 {
		t.Fatal("the command log is empty: the exec was not recorded")
	}
	return entries[len(entries)-1]
}

func TestRunRecordsArgvAndResult(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)

	if _, err := Run(context.Background(), dir, "pull", "--ff-only"); err != nil {
		t.Fatalf("pull: %v", err)
	}

	e := lastEntry(t, rec)
	if got, want := e.Command(), "git pull --ff-only"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if e.Repo != filepath.Base(dir) {
		t.Errorf("Repo = %q, want %q (the dir's basename)", e.Repo, filepath.Base(dir))
	}
	if e.Dir != dir {
		t.Errorf("Dir = %q, want %q", e.Dir, dir)
	}
	if e.Action != "pull" {
		t.Errorf("Action = %q, want %q", e.Action, "pull")
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Outcome != "fast-forward" {
		t.Errorf("Outcome = %q, want %q", e.Outcome, "fast-forward")
	}
	if e.Dur <= 0 {
		t.Error("Dur left unmeasured: the duration is part of the record")
	}
	if e.Intent {
		t.Error("Intent = true in an execution")
	}
}

func TestRunRecordsFailure(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.CommitFiles(t, dir, map[string]string{"local.txt": "local"}, "local")
	testutil.FetchLocal(t, dir)

	if _, err := Run(context.Background(), dir, "pull", "--ff-only"); err == nil {
		t.Fatal("pull --ff-only over a diverged repo should fail")
	}
	e := lastEntry(t, rec)
	if e.Exit == 0 {
		t.Error("Exit = 0 in a failed pull")
	}
	if e.Outcome != "diverged" {
		t.Errorf("Outcome = %q, want %q", e.Outcome, "diverged")
	}
}

func TestFetchRecordsTheClassItWasGiven(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")

	if err := Fetch(context.Background(), dir, cmdlog.ClassAuto, "fetch", "--prune"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	e := lastEntry(t, rec)
	if e.Class != cmdlog.ClassAuto {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassAuto)
	}
	if e.Action != "fetch" {
		t.Errorf("Action = %q, want %q", e.Action, "fetch")
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
}

// They are recorded (so it can be audited why a behind took long) but as ClassRead, which the panel hides by default: with 60 repos that would be 240 lines per scan.
func TestCollectRecordsTheReadsAsRead(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	Collect(context.Background(), dir, "main", false)

	entries := rec.Entries()
	if len(entries) < 3 {
		t.Fatalf("entries = %d, want >= 3 (status, log, worktree list…)", len(entries))
	}
	for _, e := range entries {
		if e.Class != cmdlog.ClassRead {
			t.Errorf("Class = %v, want %v for %q", e.Class, cmdlog.ClassRead, e.Command())
		}
	}
	for _, e := range entries {
		if e.Outcome != "" {
			t.Errorf("Outcome = %q in read %q, want \"\"", e.Outcome, e.Command())
		}
	}
	var sawStatus bool
	for _, e := range entries {
		if strings.HasPrefix(e.Command(), "git status") {
			sawStatus = true
		}
	}
	if !sawStatus {
		t.Errorf("the status was not recorded; there was: %v", entries)
	}
}

func TestRemoteURLExitsForRunGitAndStaysInTheLog(t *testing.T) {
	rec := installRecorder(t)
	dir, origin := testutil.NewRepo(t, true)

	got, err := RemoteURL(context.Background(), dir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if got != origin {
		t.Errorf("RemoteURL = %q, want %q (the repo's origin)", got, origin)
	}
	e := lastEntry(t, rec)
	if want := "git remote get-url origin"; e.Command() != want {
		t.Errorf("Command() = %q, want %q", e.Command(), want)
	}
	if e.Class != cmdlog.ClassRead {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassRead)
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
}

// The URL comes trimmed: git's remote output carries the newline, and without TrimSpace the host looked up in the forge map would match nothing.
func TestRemoteURLClipsTheOutput(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)

	got, err := RemoteURL(context.Background(), dir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if got != strings.TrimSpace(got) || strings.ContainsAny(got, "\n\r") {
		t.Errorf("RemoteURL = %q, want the URL without line breaks", got)
	}
}

func TestRemoteURLWithoutRemoteFailsWithTheReason(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false) // no upstream and no remote

	if _, err := RemoteURL(context.Background(), dir); err == nil {
		t.Fatal("RemoteURL in a repo with no origin should fail")
	} else if !strings.Contains(err.Error(), "remote") {
		t.Errorf("err = %q, want git's reason about the remote", err)
	}
}

func TestCollectNotReadsTheRemote(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	Collect(context.Background(), dir, "main", false)

	for _, e := range rec.Entries() {
		if strings.Contains(e.Command(), "remote") {
			t.Errorf("Collect issued a remote read: %q", e.Command())
		}
	}
}

func TestRemoveWorktreeArgv(t *testing.T) {
	got := strings.Join(append([]string{"git"}, RemoveWorktreeArgv("/tmp/wt", false)...), " ")
	if want := "git worktree remove /tmp/wt"; got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
	got = strings.Join(append([]string{"git"}, RemoveWorktreeArgv("/tmp/wt", true)...), " ")
	if want := "git worktree remove --force /tmp/wt"; got != want {
		t.Errorf("argv forzado = %q, want %q", got, want)
	}
}

func TestExecWithoutRecorderNotBreaks(t *testing.T) {
	cmdlog.SetRecorder(nil)
	dir, _ := testutil.NewRepo(t, true)
	snap := Collect(context.Background(), dir, "main", false)
	if snap.Err != "" {
		t.Fatalf("Collect with no recorder failed: %s", snap.Err)
	}
	if _, err := Run(context.Background(), dir, "status"); err != nil {
		t.Fatalf("Run with no recorder failed: %v", err)
	}
}

func TestFetchWithoutArgsUsesTheDefault(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	if err := Fetch(context.Background(), dir, cmdlog.ClassAction); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	e := lastEntry(t, rec)
	if got, want := e.Command(), "git fetch --prune"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if e.Action != "fetch" {
		t.Errorf("Action = %q, want fetch", e.Action)
	}
}

// An exec that fails writing nothing to stderr (the real case: the context is cancelled mid-scan) must keep the process error as its reason, otherwise the Snapshot reaches the UI with an empty reason and the user cannot see why the repo disappeared.
func TestExecWithoutStderrKeepsTheReason(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runGit(ctx, dir, cmdlog.ClassRead, "status")
	if err == nil {
		t.Fatal("runGit with a cancelled context should fail")
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("err = %q, want the process reason (context canceled)", err)
	}
	snap := Collect(ctx, dir, "", false)
	if snap.Err == "" {
		t.Fatal("Collect with a cancelled context and no Err")
	}
	if !strings.Contains(snap.Err, context.Canceled.Error()) {
		t.Errorf("snap.Err = %q, want the process reason", snap.Err)
	}
}

// An exec that never started (cancelled context) has no git exit code: it is recorded as -1, meaning "did not run", and not as 1, which the panel would read as a real git failure.
func TestExecWithoutCodeOfOutputIsRecordsAsLessOne(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := runGit(ctx, dir, cmdlog.ClassRead, "status"); err == nil {
		t.Fatal("runGit with a cancelled context should fail")
	}
	if e := lastEntry(t, rec); e.Exit != -1 {
		t.Errorf("Exit = %d, want -1 (it never ran)", e.Exit)
	}
}

func TestFirstLine(t *testing.T) {
	casos := []struct{ in, want string }{
		{"no break", "no break"},
		{"", ""},
		{"one\ntwo", "one"},
		{"\nfirst", ""},
		{"\n", ""},
		{"with\r\n", "with\r"},
		{"three\nlines\nand more", "three"},
	}
	for _, c := range casos {
		if got := firstLine(c.in); got != c.want {
			t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// An empty argv is reachable from user config (`[commands] pull = ""` → CmdArgs → strings.Fields → []) and the TUI ALWAYS has a recorder installed, so without this guard `Action: args[0]` blows up the action goroutine and with it the whole app; it is recorded with an empty verb, not with a panic.
func TestExecWithoutArgsNotPanics(t *testing.T) {
	rec := installRecorder(t)
	dir, _ := testutil.NewRepo(t, true)

	if _, err := Run(context.Background(), dir); err == nil {
		t.Fatal("git with no arguments should fail")
	}
	e := lastEntry(t, rec)
	if e.Action != "" {
		t.Errorf("Action = %q, want empty (no verb to name)", e.Action)
	}
	if len(e.Argv) != 1 || e.Argv[0] != "git" {
		t.Errorf("Argv = %v, want [git]", e.Argv)
	}
}
