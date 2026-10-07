package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// snapOnBranch keeps the snapshot's branch in step with the checked-out one: the same-branch guard reads it and a stale "main" would refuse a sync the repo can do.
func snapOnBranch(branch string) gitstatus.Snapshot {
	s := snapClean()
	s.Status.Branch = branch
	return s
}

// collectActionMsg waits for the action result alone; a failing sync never sends a status, so waiting for one would hang.
// The deadline is short on purpose: a mutant that suppresses the action must fail fast and be killed, not cut as a per-mutant timeout.
func collectActionMsg(t *testing.T, m Model) *actionMsg {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			if e, ok := ev.(actionMsg); ok {
				cp := e
				return &cp
			}
		case <-deadline:
			t.Fatal("the action did not publish its result")
		}
	}
}

// collectAction drains events until the action result and the recollected status have both arrived, mirroring how a real keypress closes the loop.
func collectAction(t *testing.T, m Model) (*actionMsg, bool) {
	t.Helper()
	acc := collectActionMsg(t, m)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			if _, ok := ev.(statusMsg); ok {
				return acc, true
			}
		case <-deadline:
			t.Fatal("after a successful action the repo state was not re-collected")
		}
	}
}

// execArgvs returns the argv of every action-class exec recorded (not the intents, not the scan/bookkeeping reads), in order.
func execArgvs() [][]string {
	var out [][]string
	for _, e := range cmdlog.Entries() {
		if !e.Intent && e.Class == cmdlog.ClassAction {
			out = append(out, e.Argv)
		}
	}
	return out
}

// syncIntent returns the last "sync" intent the command log recorded, or nil: the intent is left by the generic routing even when a guard refuses, so the log explains a keypress with no exec after it.
func syncIntent() *cmdlog.Entry {
	var found *cmdlog.Entry
	for _, e := range cmdlog.Entries() {
		if e.Intent && e.Action == "sync" {
			cp := e
			found = &cp
		}
	}
	return found
}

func TestSyncRunsFetchThenPullOnTheResolvedBranch(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	acc, sawStatus := collectAction(t, m)

	if acc.err != "" {
		t.Fatalf("sync failed: %q (output %q)", acc.err, acc.output)
	}
	if acc.kind != "sync" {
		t.Errorf("kind = %q, want sync", acc.kind)
	}
	if acc.outcome == "" {
		t.Errorf("sync did not classify its outcome: %+v", acc)
	}
	if want := "git fetch origin && git pull --rebase --autostash origin main"; acc.cmd != want {
		t.Errorf("resolved argv = %q, want %q", acc.cmd, want)
	}
	if !sawStatus {
		t.Error("after a successful sync the repo state was not re-collected")
	}

	argvs := execArgvs()
	if len(argvs) != 2 {
		t.Fatalf("recorded %d execs, want 2 (fetch + pull): %v", len(argvs), argvs)
	}
	if got := strings.Join(argvs[0], " "); got != "git fetch origin" {
		t.Errorf("first exec = %q, want the explicit fetch first", got)
	}
	if got := strings.Join(argvs[1], " "); got != "git pull --rebase --autostash origin main" {
		t.Errorf("second exec = %q, want the pull", got)
	}
}

// The marker's sync_branch wins over the global config, and the argv is where that resolution becomes visible.
func TestSyncUsesTheMarkerBranchOverTheGlobal(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "main"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m.cfg.SyncBranch = "release" // must lose against the declared one
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	acc, _ := collectAction(t, m)

	if want := "git fetch origin && git pull --rebase --autostash origin main"; acc.cmd != want {
		t.Errorf("resolved argv = %q, want the marker's branch (%q)", acc.cmd, want)
	}
}

// The base argv comes from `commands.sync`; only the tail (origin + ref) is the repo's.
func TestSyncHonoursTheConfiguredBase(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m.cfg.Commands["sync"] = "pull --rebase" // no autostash, the user's choice
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	acc, _ := collectAction(t, m)

	if want := "git fetch origin && git pull --rebase origin main"; acc.cmd != want {
		t.Errorf("resolved argv = %q, want the configured base (%q)", acc.cmd, want)
	}
}

func TestSyncWithoutRepoWarns(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("no-repo", "/tmp/sync-no-repo", false)},
		map[string]gitstatus.Snapshot{"/tmp/sync-no-repo": {}})
	m = cursorOn(t, m, "/tmp/sync-no-repo")

	_, cmd := press(m, "s")
	if len(m.running) != 0 {
		t.Errorf("sync launched on a row without a repo: %v", m.running)
	}
	if cmd == nil {
		t.Fatal("with no repo it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("notification = %v", cmd())
	}
	if syncIntent() == nil {
		t.Error("the no-repo refusal left no intent, so the log cannot explain the keypress")
	}
}

func TestSyncWithoutSyncBranchWarns(t *testing.T) {
	p := proj("demo", "/tmp/sync-no-branch", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})
	m.cfg.SyncBranch = "" // no marker override either
	m = cursorOn(t, m, p.Path)

	_, cmd := press(m, "s")
	if len(m.running) != 0 {
		t.Errorf("sync launched with no sync branch: %v", m.running)
	}
	if cmd == nil {
		t.Fatal("with no sync branch it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no sync branch") {
		t.Errorf("notification = %v", cmd())
	}
}

// On the sync branch there is no other branch to update, so `s` must refuse before any git process leaves.
func TestSyncRefusesOnTheSyncBranch(t *testing.T) {
	p := proj("demo", "/tmp/sync-on-branch", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapOnBranch("main")})
	m = cursorOn(t, m, p.Path)

	_, cmd := press(m, "s")
	if len(m.running) != 0 {
		t.Errorf("sync ran on the sync branch: %v", m.running)
	}
	if cmd == nil {
		t.Fatal("on the sync branch it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "sync branch") {
		t.Errorf("notification = %v", cmd())
	}
	// Like the other pre-exec guards, the refused key still leaves its intent: the log explains why no exec follows.
	if len(execArgvs()) != 0 {
		t.Errorf("a refused sync executed something: %v", execArgvs())
	}
	if syncIntent() == nil {
		t.Error("the refusal left no intent, so the log cannot explain the keypress")
	}
}

// The resolved ref becomes the pull's tail argv and the marker can declare it, so a leading dash (a git option) must be refused before any process leaves.
func TestSyncRefusesAnOptionLikeSyncBranch(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "--upload-pack=/bin/echo"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m = cursorOn(t, m, dir)

	_, cmd := press(m, "s")
	if len(m.running) != 0 {
		t.Errorf("sync launched with an option-like ref: %v", m.running)
	}
	if cmd == nil {
		t.Fatal("an option-like ref should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || nm.level != toastWarning {
		t.Errorf("notification = %v, want a warning", cmd())
	}
	if got := execArgvs(); len(got) != 0 {
		t.Errorf("an option-like ref executed something: %v", got)
	}
}

func TestSyncEmptyCommandWarns(t *testing.T) {
	p := proj("demo", "/tmp/sync-empty", true)
	m := newTestModel(t, []discovery.Project{{Path: p.Path, Name: p.Name, HasRepo: true, SyncBranch: "develop"}},
		map[string]gitstatus.Snapshot{p.Path: snapOnBranch("main")})
	m.cfg.Commands["sync"] = "   " // Fields collapses it to no command
	m = cursorOn(t, m, p.Path)

	_, cmd := press(m, "s")
	if len(m.running) != 0 {
		t.Errorf("sync launched with an empty command: %v", m.running)
	}
	if cmd == nil {
		t.Fatal("with an empty command it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "empty sync command") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestSyncHonoursTheRunningLock(t *testing.T) {
	p := proj("demo", "/tmp/sync-locked", true)
	m := newTestModel(t, []discovery.Project{{Path: p.Path, Name: p.Name, HasRepo: true, SyncBranch: "develop"}},
		map[string]gitstatus.Snapshot{p.Path: snapOnBranch("main")})
	m = cursorOn(t, m, p.Path)
	m.running[p.Path] = "fetch"

	_, cmd := press(m, "s")
	if m.running[p.Path] != "fetch" {
		t.Errorf("the lock was overwritten: %q", m.running[p.Path])
	}
	if cmd == nil {
		t.Fatal("a busy repo should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notification = %v", cmd())
	}
}

// A failed fetch must short-circuit: no pull executes after it.
func TestSyncFetchFailureStopsBeforeThePull(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false) // no origin remote
	m := newTestModel(t, []discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "develop"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	acc := collectActionMsg(t, m)
	if acc.err == "" {
		t.Fatal("a fetch with no origin did not fail (the fixture has no remote)")
	}

	argvs := execArgvs()
	if len(argvs) != 1 {
		t.Fatalf("recorded %d execs, want only the failed fetch: %v", len(argvs), argvs)
	}
	if got := strings.Join(argvs[0], " "); got != "git fetch origin" {
		t.Errorf("the exec was %q, want only the fetch", got)
	}
}

func TestSyncLeavesAnIntent(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	intent := syncIntent()
	if intent == nil {
		t.Fatal("s did not leave a sync intent in the command log")
	}
	if intent.Key != "s" {
		t.Errorf("intent key = %q, want s", intent.Key)
	}
	collectAction(t, m) // drain the execs so the goroutine does not outlive the test
}

func TestRunningActionsShowsSync(t *testing.T) {
	p := proj("demo", "/tmp/sync-spinner", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})
	m.running[p.Path] = "sync"

	got := m.runningActions()
	if len(got) != 1 || !strings.Contains(got[0], "sync") {
		t.Errorf("runningActions = %v, want the sync spinner", got)
	}
}

func TestActionNoteSyncMidRebase(t *testing.T) {
	got := actionNote("sync", "repo-a", "git fetch origin && git pull --rebase --autostash origin main",
		"CONFLICT (content): Merge conflict in f.txt\n", "error: could not apply 1234567... local", true)
	if !strings.Contains(got, "mid-rebase") || !strings.Contains(got, "rebase --continue") {
		t.Errorf("actionNote = %q, want the mid-rebase warning", got)
	}
	if strings.Contains(got, "diverged") || strings.Contains(got, "no upstream") {
		t.Errorf("actionNote = %q, the pull-only hints must be left out", got)
	}

	plain := actionNote("sync", "repo-a", "git fetch origin && git pull --rebase --autostash origin main",
		"CONFLICT\n", "error: could not apply 1234567... local", false)
	if strings.Contains(plain, "mid-rebase") {
		t.Errorf("actionNote without the flag = %q, must not mention a rebase", plain)
	}
}

// `s` reports through a short toast and must NOT leave (nor keep) an action block in the card, which is where git's transient rebase/drop lines leaked into the preview.
func TestSyncReportsThroughToastNotTheCard(t *testing.T) {
	p := proj("demo", "/tmp/sync-toast", true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	// A previous action the card was showing must be cleared by a sync, even a successful one.
	m.lastAction[p.Path] = actionResult{kind: "pull", cmd: "git pull", output: "Fast-forward\n"}
	before := len(m.toasts.toasts)

	updated, _ := m.Update(actionMsg{
		path:    p.Path,
		kind:    "sync",
		cmd:     "git fetch origin && git pull --rebase --autostash origin main",
		output:  "dropping abc971f… -- patch contents already upstream\nSuccessfully rebased and updated refs/heads/feature.\n",
		outcome: "rebase",
	})
	m = updated.(Model)

	if act, ok := m.lastAction[p.Path]; ok {
		t.Errorf("sync left an action block in the card: %+v", act)
	}
	if len(m.toasts.toasts) != before+1 {
		t.Fatalf("sync did not toast its outcome: %+v", m.toasts.toasts)
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.text != "sync ok demo — rebase" {
		t.Errorf("toast = %q, want the verdict plus the classified outcome", last.text)
	}
}

// With no classified outcome the verdict must not grow a dangling separator.
func TestSyncToastWithoutOutcomeHasNoSeparator(t *testing.T) {
	p := proj("demo", "/tmp/sync-toast-noout", true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	updated, _ := m.Update(actionMsg{path: p.Path, kind: "sync", cmd: "git pull origin main"})
	m = updated.(Model)
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.text != "sync ok demo" {
		t.Errorf("toast = %q, want the bare verdict with no separator", last.text)
	}
}

// A failed/mid-rebase sync clears the card too, and its toast keeps the reason but still no argv.
func TestSyncFailureClearsTheCardAndKeepsTheToastShort(t *testing.T) {
	p := proj("demo", "/tmp/sync-toast-fail", true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	m.lastAction[p.Path] = actionResult{kind: "pull", cmd: "git pull"}

	updated, _ := m.Update(actionMsg{
		path:             p.Path,
		kind:             "sync",
		cmd:              "git fetch origin && git pull --rebase --autostash origin main",
		err:              "error: could not apply 1234567... local",
		outcome:          "conflict", // must be ignored on failure: the reason already says more
		rebaseInProgress: true,
	})
	m = updated.(Model)

	if act, ok := m.lastAction[p.Path]; ok {
		t.Errorf("a failed sync left an action block in the card: %+v", act)
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if !strings.Contains(last.text, "sync failed demo: error: could not apply") {
		t.Errorf("toast = %q, want the failure reason", last.text)
	}
	if !strings.Contains(last.text, "mid-rebase") {
		t.Errorf("toast = %q, want the mid-rebase warning", last.text)
	}
	if strings.Contains(last.text, "fetch origin") || strings.Contains(last.text, "--autostash") {
		t.Errorf("toast = %q, the argv must stay out of the verdict", last.text)
	}
	if strings.Contains(last.text, " — conflict") {
		t.Errorf("toast = %q, the outcome must not be appended on failure", last.text)
	}
}

// The refusal reads the last snapshot; when it cannot pin the branch down (not collected, stale, detached) git's own result governs instead of refusing a sync the repo can do.
func TestSyncUnknownBranchDoesNotRefuse(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("")})
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	acc, _ := collectAction(t, m)

	if acc.err != "" {
		t.Fatalf("an unknown branch refused the sync: %q", acc.err)
	}
	if acc.outcome == "" {
		t.Errorf("the sync did not classify its outcome: %+v", acc)
	}
	if got := execArgvs(); len(got) != 2 {
		t.Errorf("execs = %v, want fetch + pull", got)
	}
}

// A sync branch missing on origin fails at the pull with git's own reason; the action never invents the legacy fallback ref the SYNC column might be displaying.
func TestSyncBranchMissingOnOriginReportsGitReason(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "release"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m = cursorOn(t, m, dir)

	m, _ = press(m, "s")
	acc := collectActionMsg(t, m)

	if !strings.Contains(acc.err, "couldn't find remote ref") {
		t.Fatalf("failure = %q, want git's missing-remote-ref reason", acc.err)
	}
	argvs := execArgvs()
	if len(argvs) != 2 {
		t.Fatalf("recorded %d execs, want fetch + the failed pull: %v", len(argvs), argvs)
	}
	if got := strings.Join(argvs[1], " "); got != "git pull --rebase --autostash origin release" {
		t.Errorf("pull exec = %q, want the resolved ref and no fallback", got)
	}
}

// `sync` is a configurable action: rebinding the key moves both the routing and the intent it leaves behind.
func TestSyncKeyIsRebindable(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m.cfg.Keybindings["sync"] = "y"
	m = cursorOn(t, m, dir)

	if got := m.cfg.KeyFor("sync"); got != "y" {
		t.Fatalf("KeyFor(sync) = %q, want the rebound key", got)
	}
	if hints := strings.Join(m.cfg.HintBarLines(), "\n"); !strings.Contains(hints, "y sync") {
		t.Errorf("the hint bar did not follow the rebind: %v", m.cfg.HintBarLines())
	}
	m, _ = press(m, "y")
	if m.running[dir] != "sync" {
		t.Fatalf("the rebound key did not launch the sync: running = %v", m.running)
	}
	intent := syncIntent()
	if intent == nil || intent.Key != "y" {
		t.Errorf("intent = %+v, want the rebound key recorded", intent)
	}
	collectAction(t, m)
}

// The command-log panel is a view mode, not a modal: `s` is not among the guarded keys, so it runs and its execs land in the panel.
func TestSyncRunsWithTheLogPanelOpen(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m = cursorOn(t, m, dir)
	m.logOpen = true

	m, _ = press(m, "s")
	acc, _ := collectAction(t, m)

	if acc.err != "" {
		t.Fatalf("sync did not run with the panel open: %q", acc.err)
	}
	if !m.logOpen {
		t.Error("the panel was closed by the sync key")
	}
	if got := execArgvs(); len(got) != 2 {
		t.Errorf("execs = %v, want fetch + pull reaching the panel", got)
	}
}

// A worktree sub-row resolves the ref against the global default (the parent's marker is not consulted) and runs in the worktree directory; with no snapshot of its own the refusal cannot fire.
func TestSyncWorktreeSubrowUsesTheGlobalDefault(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	wtDir := filepath.Join(t.TempDir(), "wt-real")
	testutil.MakeWorktree(t, dir, wtDir, "wt-real")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	snap := gitstatus.Collect(t.Context(), dir, "main", false)
	if len(snap.Worktrees) != 1 {
		t.Fatalf("fixture: worktrees = %d, want 1", len(snap.Worktrees))
	}

	parent := discovery.Project{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "release"}
	m := newTestModel(t, []discovery.Project{parent}, map[string]gitstatus.Snapshot{dir: snap})
	m, _ = press(m, "enter")
	m = cursorOn(t, m, wtDir)

	m, _ = press(m, "s")
	acc, _ := collectAction(t, m)

	if acc.err != "" {
		t.Fatalf("sync in the worktree failed: %q", acc.err)
	}
	want := "git fetch origin && git pull --rebase --autostash origin main"
	if acc.cmd != want {
		t.Errorf("resolved argv = %q, want the global default (%q)", acc.cmd, want)
	}
	for _, e := range cmdlog.Entries() {
		if e.Intent || e.Class != cmdlog.ClassAction {
			continue
		}
		if filepath.Clean(e.Dir) != filepath.Clean(wtDir) {
			t.Errorf("exec ran in %q, want the worktree directory", e.Dir)
		}
	}
}

// Only sync is toast-only; every other action keeps feeding the card and keeps its argv in the toast.
func TestPullStillFillsTheActionCard(t *testing.T) {
	p := proj("demo", "/tmp/pull-card", true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	updated, _ := m.Update(actionMsg{path: p.Path, kind: "pull", cmd: "git pull"})
	m = updated.(Model)
	if act, ok := m.lastAction[p.Path]; !ok || act.kind != "pull" {
		t.Errorf("lastAction = %+v (ok=%v), want the pull kept in the card", act, ok)
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; !strings.Contains(last.text, "git pull") {
		t.Errorf("pull toast = %q, want the argv kept (only sync drops it)", last.text)
	}
}
