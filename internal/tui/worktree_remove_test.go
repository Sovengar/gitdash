package tui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func gitOutT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func removeWtModel(t *testing.T, parent string, wts ...gitstatus.Worktree) (Model, discovery.Project) {
	t.Helper()
	p, st := repoWithWorktrees(filepath.Base(parent), parent, wts...)
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	if e, ok := m.selectedEntry(); !ok || e.kind != kindWorktree {
		t.Fatalf("the cursor did not land on a subrow: %+v", e)
	}
	return m, p
}

func armedOver(t *testing.T, m Model, e tableEntry, force bool) Model {
	t.Helper()
	m.armed = &armedRemoval{
		wtPath: e.wt.Path, parent: e.parent,
		name: filepath.Base(e.wt.Path), force: force,
	}
	return m
}

func waitEvent(t *testing.T, m *Model, match func(event) bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			updated, _ := m.Update(ev)
			*m = updated.(Model)
			if match(ev) {
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for the expected event")
		}
	}
}

func applyNotify(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestRemoveWorktreeArmOnFirstPress(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	before := m.cursor

	m, cmd := press(m, "D")

	if m.armed == nil {
		t.Fatal("D did not arm the confirmation")
	}
	if m.armed.wtPath != "/tmp/wt-a" || m.armed.name != "wt-a" {
		t.Errorf("armed = %+v, want wt-a", m.armed)
	}
	if m.armed.force {
		t.Error("the first arming must not force")
	}
	if m.armed.parent != p.Path {
		t.Errorf("parent = %q, want %q", m.armed.parent, p.Path)
	}
	if len(m.running) != 0 {
		t.Errorf("there must be no running actions: %v", m.running)
	}
	if cmd != nil {
		t.Error("arming must not launch any Cmd")
	}
	if m.cursor != before {
		t.Errorf("the cursor changed: %d, want %d", m.cursor, before)
	}
}

func TestRemoveWorktreeSecondPressExecutes(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))

	m, _ = press(m, "D")
	m, _ = press(m, "D")

	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Fatalf("running[parent] = %q, want worktree_remove", got)
	}
}

func TestRemoveWorktreeEscCancels(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D")

	m, _ = press(m, "esc")

	if m.armed != nil {
		t.Error("esc did not disarm the confirmation")
	}
	if m.running["/tmp/parent-repo"] != "" {
		t.Error("esc must not launch any action")
	}
}

func TestRemoveWorktreeOnRepoRowInfo(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)

	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)

	if m.armed != nil {
		t.Error("D on a repo row must not arm")
	}
	if len(m.toasts.toasts) == 0 {
		t.Fatal("no toast")
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastInfo || last.text != "select a worktree" {
		t.Errorf("toast = %+v, want info 'select a worktree'", last)
	}
}

func TestRemoveWorktreeOnHeaderOrEmptyInfo(t *testing.T) {
	grouped := discovery.Project{Path: "/tmp/g1", Name: "g1", PrimaryGroup: "g", HasRepo: true}
	m := newTestModel(t, []discovery.Project{grouped},
		map[string]gitstatus.Snapshot{"/tmp/g1": snapClean()})
	if e, _ := m.selectedEntry(); e.kind != kindPrimary {
		t.Fatalf("precondition: entry = %+v, want a header", e)
	}
	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)
	if m.armed != nil {
		t.Error("D on a header must not arm")
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast header = %+v", last)
	}

	empty := newTestModel(t, nil, nil)
	empty, cmd = press(empty, "D")
	empty = applyNotify(empty, cmd)
	if empty.armed != nil {
		t.Error("D on an empty table must not arm")
	}
	if last := empty.toasts.toasts[len(empty.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("empty toast = %+v", last)
	}
}

func TestRemoveWorktreeCursorMoveDisarms(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m, _ = press(m, "D")
	before := m.cursor

	m, _ = press(m, "down")
	if m.armed != nil {
		t.Error("down did not disarm")
	}
	if m.cursor != before+1 {
		t.Errorf("down did not navigate: cursor=%d, want %d", m.cursor, before+1)
	}

	m, _ = press(m, "D")
	m, _ = press(m, "up")
	if m.armed != nil {
		t.Error("up did not disarm")
	}
	if m.cursor != before {
		t.Errorf("up did not navigate: cursor=%d, want %d", m.cursor, before)
	}
}

func TestRemoveWorktreeOtherKeysDisarm(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D")

	m2, _ := press(m, "d")
	if m2.armed != nil {
		t.Error("d did not disarm")
	}
	if !m2.onlyDirty {
		t.Error("d did not apply the dirty filter")
	}

	m3, _ := press(m, "/")
	if m3.armed != nil {
		t.Error("/ did not disarm")
	}
	if !m3.searchActive {
		t.Error("/ did not open the search")
	}

	m4, _ := press(m, "enter")
	if m4.armed != nil {
		t.Error("tab did not disarm")
	}

	m5, _ := press(m, "enter")
	if m5.armed != nil {
		t.Error("space did not disarm")
	}
}

func TestRemoveWorktreeFailureArmsForce(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.armed = nil
	m.removeTokens[p.Path] = 7
	m.running[p.Path] = "worktree_remove"

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		output: "fatal: contains files modified", err: "fatal: contains files modified",
		gen: 7,
	})
	m = updated.(Model)

	if m.armed == nil || !m.armed.force {
		t.Fatalf("the force was not armed: %+v", m.armed)
	}
	if m.armed.wtPath != "/tmp/wt-a" {
		t.Errorf("forced over %q, want wt-a", m.armed.wtPath)
	}
	if m.running[p.Path] != "" {
		t.Error("the failure did not release the parent's running")
	}
	if _, ok := m.removeTokens[p.Path]; ok {
		t.Errorf("the token must be consumed: %v", m.removeTokens)
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "files modified") {
		t.Errorf("toast = %+v, want an error with the real reason", last)
	}
	if act := m.lastAction[p.Path]; act.kind != "worktree_remove" || act.err == "" {
		t.Errorf("lastAction = %+v, want kind worktree_remove with an error", act)
	}
}

func TestRemoveWorktreeForceExecutes(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	e, _ := m.selectedEntry()
	m = armedOver(t, m, e, true)

	m, _ = press(m, "D")

	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Fatalf("running[parent] = %q", got)
	}
	select {
	case ev := <-m.events:
		msg, ok := ev.(worktreeRemovedMsg)
		if !ok {
			t.Fatalf("first event = %T, want worktreeRemovedMsg", ev)
		}
		if !msg.force {
			t.Error("the run did not use force")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worktreeRemovedMsg never arrived")
	}
}

func TestRemoveWorktreeForceFailureDisarms(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.armed = nil
	m.removeTokens[p.Path] = 9
	m.running[p.Path] = "worktree_remove"

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: cannot do it", force: true, gen: 9,
	})
	m = updated.(Model)

	if m.armed != nil {
		t.Errorf("the forced failure must not re-arm: %+v", m.armed)
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "cannot do it") {
		t.Errorf("toast = %+v, want error", last)
	}
}

func TestRemoveWorktreeSuccessDisarmsAndRecollects(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-real")
	testutil.MakeWorktree(t, dir, wtDir, "wt-real")
	snap := gitstatus.Collect(t.Context(), dir, "main", false)
	if len(snap.Worktrees) != 1 {
		t.Fatalf("fixture: worktrees = %d, want 1", len(snap.Worktrees))
	}

	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	m, _ = press(m, "D")
	m, _ = press(m, "D")

	waitEvent(t, &m, func(ev event) bool {
		sm, ok := ev.(statusMsg)
		return ok && sm.path == dir && len(sm.snap.Worktrees) == 0
	})

	if m.armed != nil {
		t.Errorf("the success must disarm: %+v", m.armed)
	}
	if m.running[dir] != "" {
		t.Errorf("the parent is still busy: %v", m.running[dir])
	}
	if len(m.states[dir].Worktrees) != 0 {
		t.Errorf("the parent's snapshot still holds the worktree: %+v", m.states[dir].Worktrees)
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("the subrow is still visible: %v", got)
	}
	found := false
	for _, to := range m.toasts.toasts {
		if to.level == toastSuccess && strings.Contains(to.text, "worktree removed") {
			found = true
		}
	}
	if !found {
		t.Errorf("no success toast: %+v", m.toasts.toasts)
	}
}

func TestRemoveWorktreeSubrowDisappearsAfterSuccess(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m.cursor = 2

	sinWt := snapClean()
	sinWt.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	updated, _ := m.Update(statusMsg{path: p.Path, snap: sinWt})
	m = updated.(Model)

	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("subrows = %v, want [wt-a]", got)
	}
	if n := len(m.entries()); m.cursor >= n {
		t.Errorf("cursor out of range: %d (entries=%d)", m.cursor, n)
	}
}

func TestRemoveWorktreeDetachedRow(t *testing.T) {
	det := gitstatus.Worktree{Path: "/tmp/wt-det", Head: "abc1234"}
	m, p := removeWtModel(t, "/tmp/parent-repo", det)

	m, _ = press(m, "D")
	if m.armed == nil || m.armed.name != "wt-det" {
		t.Fatalf("it did not arm over the detached: %+v", m.armed)
	}
	m, _ = press(m, "D")
	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Errorf("running = %q, want worktree_remove", got)
	}
}

func TestRemoveWorktreeNotArmedOnMissingParent(t *testing.T) {
	p := discovery.Project{Path: "", Name: "no-path", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"": s})
	m.expanded[""] = true
	m, _ = press(m, "down")
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree {
		t.Fatalf("precondition: entry = %+v", e)
	}
	if e.parent != "" {
		t.Fatalf("precondition: parent = %q, want empty", e.parent)
	}

	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)

	if m.armed != nil {
		t.Error("it must not arm with an empty parent")
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast = %+v", last)
	}
}

func TestRemoveWorktreeArmedRevalidatesSelection(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m, _ = press(m, "D")

	m.cursor = 2

	m, _ = press(m, "D")
	if m.running[p.Path] != "" {
		t.Errorf("it deleted the armed worktree despite the selection changing: %v", m.running)
	}
	if m.armed == nil || m.armed.wtPath != "/tmp/wt-b" {
		t.Errorf("it did not re-arm on the current subrow: %+v", m.armed)
	}
}

func TestRemoveWorktreePromptRender(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))

	m, _ = press(m, "D")
	out := stripANSI(m.View().Content)
	if !strings.Contains(sectionContent(t, out, "keybinds"), "remove worktree wt-a? D to confirm, esc to cancel") {
		t.Errorf("the confirmation warning is missing from keybinds:\n%s", out)
	}

	e, _ := m.selectedEntry()
	m = armedOver(t, m, e, true)
	out = stripANSI(m.View().Content)
	if !strings.Contains(sectionContent(t, out, "keybinds"), "remove worktree wt-a? has changes — D to force, esc to cancel") {
		t.Errorf("forced warning missing from keybinds:\n%s", out)
	}

	m, _ = press(m, "esc")
	out = stripANSI(m.View().Content)
	if strings.Contains(out, "remove worktree") {
		t.Error("the warning is still visible after cancelling")
	}
	if !strings.Contains(sectionContent(t, out, "keybinds"), "j/k move") {
		t.Errorf("the hints did not come back after cancelling:\n%s", out)
	}
}

func TestRemoveWorktreeBindingOverride(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.cfg.Keybindings["worktree_remove"] = "W"

	m, _ = press(m, "D")
	if m.armed != nil {
		t.Fatal("D still arms after the rebind")
	}
	m, _ = press(m, "W")
	if m.armed == nil {
		t.Fatal("W did not arm after the rebind")
	}
	m, _ = press(m, "W")
	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Errorf("W did not run: running = %q", got)
	}
}

func TestRemoveWorktreeBusyParentWarns(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.running[p.Path] = "pull"

	m, _ = press(m, "D")
	if m.armed == nil {
		t.Fatal("the arming must not be blocked by the busy parent")
	}

	m, cmd := press(m, "D")
	if got := m.running[p.Path]; got != "pull" {
		t.Errorf("running = %q, want pull (without launching)", got)
	}
	if m.armed == nil {
		t.Error("the arming must not break silently")
	}
	if cmd == nil {
		t.Fatal("the warning toast was expected")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || msg.level != toastWarning {
		t.Errorf("warning = %+v, want warning", msg)
	}
}

func TestRemoveWorktreeEscDuringFlightIgnoresLateFailure(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D")
	m, _ = press(m, "D")

	if m.armed != nil {
		t.Fatal("the banner must be cleared when the deletion is launched")
	}
	token, inflight := m.removeTokens[p.Path]
	if !inflight || token == 0 {
		t.Fatalf("no in-flight attempt token: %v", m.removeTokens)
	}

	m, _ = press(m, "esc")
	if len(m.removeTokens) != 0 {
		t.Fatalf("esc did not invalidate the in-flight attempt: %v", m.removeTokens)
	}

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: dirty", gen: token,
	})
	m = updated.(Model)

	if m.armed != nil {
		t.Errorf("a late failure after esc must not re-arm: %+v", m.armed)
	}
	if m.running[p.Path] != "" {
		t.Error("the parent's running must be released when the result arrives")
	}
}

func TestRemoveWorktreeLateResultDoesNotClobberOtherArmed(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	token := m.removeTokens[p.Path]

	m, _ = press(m, "down")
	m, _ = press(m, "D")
	if m.armed == nil || m.armed.name != "wt-b" {
		t.Fatalf("B did not arm: %+v", m.armed)
	}

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: dirty A", gen: token,
	})
	m = updated.(Model)

	if m.armed == nil || m.armed.name != "wt-b" || m.armed.force {
		t.Errorf("A's late result overwrote B's arming: %+v", m.armed)
	}
}

func TestRemoveWorktreeOnSecondaryHeaderInfo(t *testing.T) {
	p := discovery.Project{
		Path: "/s", Name: "s", HasRepo: true,
		PrimaryGroup: "g", SecondaryGroup: "sub",
	}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/s": snapClean()})
	m.cursor = 1
	if e, _ := m.selectedEntry(); e.kind != kindSecondary {
		t.Fatalf("precondition: entry = %+v, want a secondary header", e)
	}

	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)

	if m.armed != nil {
		t.Error("D on a secondary header must not arm")
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast = %+v, want info 'select a worktree'", last)
	}
}

func TestRemoveWorktreeCommandKeyDisarms(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D")

	m, _ = press(m, "!")

	if m.armed != nil {
		t.Error("! did not disarm the confirmation")
	}
	if !m.cmdOpen {
		t.Error("! did not open the command mode")
	}
}

func TestRemoveWorktreeDirtyForceSuccessEndToEnd(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-dirty")
	testutil.MakeWorktree(t, dir, wtDir, "wt-dirty")
	testutil.WriteUntracked(t, wtDir, map[string]string{"untracked.txt": "x"})
	snap := gitstatus.Collect(t.Context(), dir, "main", false)
	if len(snap.Worktrees) != 1 {
		t.Fatalf("fixture: worktrees = %d, want 1", len(snap.Worktrees))
	}

	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	m, _ = press(m, "D")
	m, _ = press(m, "D")

	waitEvent(t, &m, func(ev event) bool {
		mm, ok := ev.(worktreeRemovedMsg)
		return ok && mm.err != "" && !mm.force
	})
	if m.armed == nil || !m.armed.force {
		t.Fatalf("the force was not armed after the dirty failure: %+v", m.armed)
	}

	m, _ = press(m, "D")
	waitEvent(t, &m, func(ev event) bool {
		sm, ok := ev.(statusMsg)
		return ok && sm.path == dir && len(sm.snap.Worktrees) == 0
	})

	if m.armed != nil {
		t.Errorf("the success did not disarm: %+v", m.armed)
	}
	if m.running[dir] != "" {
		t.Errorf("the parent is still busy: %v", m.running[dir])
	}
	if len(m.states[dir].Worktrees) != 0 {
		t.Errorf("the snapshot still holds the worktree: %+v", m.states[dir].Worktrees)
	}
	if branches := gitOutT(t, dir, "branch", "--list", "wt-dirty"); !strings.Contains(branches, "wt-dirty") {
		t.Errorf("the wt-dirty branch disappeared: %q", branches)
	}
}

func twoParentModel(t *testing.T) Model {
	t.Helper()
	base := time.Now().Add(-2 * time.Hour).Unix()
	snapA := snapClean()
	snapA.LastCommit = base
	snapA.Worktrees = []gitstatus.Worktree{wt("/tmp/pa-wt", "a")}
	snapB := snapClean()
	snapB.LastCommit = base
	snapB.Worktrees = []gitstatus.Worktree{wt("/tmp/pb-wt", "b")}
	pA := proj("a", "/tmp/pa", true)
	pB := proj("b", "/tmp/pb", true)
	m := newTestModel(t, []discovery.Project{pA, pB},
		map[string]gitstatus.Snapshot{"/tmp/pa": snapA, "/tmp/pb": snapB})
	m.expanded["/tmp/pa"] = true
	m.expanded["/tmp/pb"] = true
	return m
}

// The token is per parent, not global.
func TestRemoveWorktreeConcurrentParentsNotLeak(t *testing.T) {
	m := twoParentModel(t)

	m.cursor = 1
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokA := m.removeTokens["/tmp/pa"]
	if tokA == 0 {
		t.Fatalf("A did not stay in flight: %v", m.removeTokens)
	}

	m.cursor = 3
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokB := m.removeTokens["/tmp/pb"]
	if tokB == 0 || tokB == tokA {
		t.Fatalf("tokens not independent: A=%d B=%d", tokA, tokB)
	}

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: "/tmp/pa", wtPath: "/tmp/pa-wt", name: "pa-wt",
		err: "fatal: dirty A", gen: tokA,
	})
	m = updated.(Model)

	if m.running["/tmp/pa"] != "" {
		t.Errorf("running leak in A: %q", m.running["/tmp/pa"])
	}
	if m.running["/tmp/pb"] != "worktree_remove" {
		t.Errorf("B's attempt was altered: %q", m.running["/tmp/pb"])
	}
	if _, ok := m.removeTokens["/tmp/pa"]; ok {
		t.Errorf("A's token not consumed: %v", m.removeTokens)
	}
	if m.removeTokens["/tmp/pb"] != tokB {
		t.Errorf("altered token of B: %v", m.removeTokens)
	}
	if m.armed == nil || !m.armed.force || m.armed.name != "pa-wt" {
		t.Errorf("A's outcome not applied: %+v", m.armed)
	}

	updated, _ = m.Update(worktreeRemovedMsg{
		parent: "/tmp/pb", wtPath: "/tmp/pb-wt", name: "pb-wt", gen: tokB,
	})
	m = updated.(Model)
	if m.running["/tmp/pb"] != "" {
		t.Errorf("running leak on B after the success: %q", m.running["/tmp/pb"])
	}
	if _, ok := m.removeTokens["/tmp/pb"]; ok {
		t.Errorf("B's token not consumed: %v", m.removeTokens)
	}
}

func TestRemoveWorktreeEscThenOtherParentNotLeak(t *testing.T) {
	m := twoParentModel(t)

	m.cursor = 1
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokA := m.removeTokens["/tmp/pa"]
	if tokA == 0 {
		t.Fatalf("A did not stay in flight: %v", m.removeTokens)
	}

	m, _ = press(m, "esc")
	if len(m.removeTokens) != 0 {
		t.Fatalf("esc did not clear the tokens: %v", m.removeTokens)
	}

	m.cursor = 3
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokB := m.removeTokens["/tmp/pb"]
	if tokB == 0 {
		t.Fatalf("B did not stay in flight: %v", m.removeTokens)
	}

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: "/tmp/pa", wtPath: "/tmp/pa-wt", name: "pa-wt",
		err: "fatal: dirty A", gen: tokA,
	})
	m = updated.(Model)

	if m.running["/tmp/pa"] != "" {
		t.Errorf("running leak in A: %q", m.running["/tmp/pa"])
	}
	if m.running["/tmp/pb"] != "worktree_remove" {
		t.Errorf("B's attempt was altered: %q", m.running["/tmp/pb"])
	}
	if m.removeTokens["/tmp/pb"] != tokB {
		t.Errorf("altered token of B: %v", m.removeTokens)
	}
	if m.armed != nil {
		t.Errorf("a cancelled result must not arm: %+v", m.armed)
	}
}

func TestRemoveWorktreeArmedMatchesNormalization(t *testing.T) {
	a := armedRemoval{parent: "/tmp/p", wtPath: "/tmp/p/wt"}
	cases := []struct {
		parent, wtPath string
		want           bool
	}{
		{"/tmp/p", "/tmp/p/wt", true},
		{"/tmp/p/", "/tmp/p/wt/", true},
		{"/tmp/p/.", "/tmp/p/./wt", true},
		{"/tmp/p", "/tmp/p//wt", true},
		{"/tmp/other", "/tmp/p/wt", false},
		{"/tmp/p", "/tmp/p/other", false},
	}
	for _, tc := range cases {
		if got := a.matches(tc.parent, tc.wtPath); got != tc.want {
			t.Errorf("matches(%q, %q) = %v, want %v", tc.parent, tc.wtPath, got, tc.want)
		}
	}
	b := armedRemoval{parent: "/tmp/p2", wtPath: "/tmp/p2/wt"}
	if b.matches("/tmp/p", "/tmp/p/wt") {
		t.Error("matches crossed different parents")
	}
}

// Ignoring it completely is what avoids the collision of a background statusMsg releasing running[parent] with a removal in flight and letting the user relaunch.
func TestRemoveWorktreeSubstitutedTokenIgnored(t *testing.T) {
	cases := []struct {
		name string
		err  string
	}{
		{"stale failure result", "fatal: dirty t1"},
		{"stale success result", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
			m.removeTokens[p.Path] = 2
			m.running[p.Path] = "worktree_remove"
			m.armed = &armedRemoval{wtPath: "/tmp/wt-b", parent: p.Path, name: "wt-b"}

			before := len(m.toasts.toasts)
			updated, _ := m.Update(worktreeRemovedMsg{
				parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
				output: "t1 output", err: tc.err, gen: 1,
			})
			m = updated.(Model)

			if m.running[p.Path] != "worktree_remove" {
				t.Errorf("the new attempt's running was released: %q", m.running[p.Path])
			}
			if m.removeTokens[p.Path] != 2 {
				t.Errorf("the live token was altered: %v", m.removeTokens)
			}
			if m.armed == nil || m.armed.name != "wt-b" {
				t.Errorf("the banner was touched: %+v", m.armed)
			}
			if len(m.toasts.toasts) != before {
				t.Errorf("a toast was emitted for a stale result: %+v", m.toasts.toasts)
			}
			if _, ok := m.lastAction[p.Path]; ok {
				t.Errorf("lastAction was saved from a stale result: %+v", m.lastAction[p.Path])
			}
		})
	}
}

// The map is created here and not before: indexing a `nil` map would assign to it and blow up, and the path is reached as soon as the app starts (the store may not exist without HOME).
func TestRemoveTokensIsInitializesInTheSecondLevel(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "repo")
	wt := filepath.Join(filepath.Dir(parent), "wt")
	p, st := repoWithWorktrees("repo", parent, gitstatus.Worktree{Path: wt, Branch: "feature"})
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree {
		t.Fatalf("the cursor did not land on a subrow: %+v", e)
	}
	m = armedOver(t, m, e, false)
	m.removeTokens = nil

	m, _ = press(m, "D")
	if m.removeTokens == nil {
		t.Fatal("removeTokens is still nil after confirming the deletion")
	}
	if m.removeTokens[parent] == 0 {
		t.Errorf("the deletion attempt was not recorded: removeTokens = %v", m.removeTokens)
	}
}
