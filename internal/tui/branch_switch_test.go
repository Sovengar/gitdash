package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func pickerOn(path, mode, variant string, branches []string, current string) *branchPicker {
	return &branchPicker{
		path: path, mode: mode, variant: variant,
		branches: branches, current: current,
	}
}

func branchIntent(action string) *cmdlog.Entry {
	var found *cmdlog.Entry
	for _, e := range cmdlog.Entries() {
		if e.Intent && e.Action == action {
			cp := e
			found = &cp
		}
	}
	return found
}

func TestBranchArmsWithoutRun(t *testing.T) {
	m := newPullModel(t)
	before := len(m.running)
	m, _ = press(m, "b")
	if m.branchArmed == nil {
		t.Fatal("b did not arm the selector")
	}
	if len(m.running) != before {
		t.Errorf("b launched an action: running = %v", m.running)
	}
}

func TestBranchSelectorPromptVisibleInKeybinds(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "b")
	out := stripANSI(m.View().Content)
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "c checkout") || !strings.Contains(kb, "s sync ref") {
		t.Errorf("the armed branch prompt is not in the keybinds section:\n%s", kb)
	}
}

func TestBranchSelectorKeyNotVariantCancelsAndFollows(t *testing.T) {
	m := newPullModel(t)
	start := m.cursor
	m, _ = press(m, "b")
	m, _ = press(m, "j")
	if m.branchArmed != nil {
		t.Error("the selector stays armed after a non-variant key")
	}
	if m.cursor == start {
		t.Error("the non-variant key did not run its own action (cursor still)")
	}
}

func TestBranchVariantOpensPicker(t *testing.T) {
	for key, mode := range map[string]string{"c": pickerModeCurrent, "s": pickerModeSync} {
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/old-clean")
		m, _ = press(m, "b")
		m, _ = press(m, key)
		if m.branchArmed != nil {
			t.Errorf("b%s left the selector armed", key)
		}
		if m.picker == nil {
			t.Fatalf("b%s did not open the picker", key)
		}
		if m.picker.mode != mode {
			t.Errorf("b%s mode = %q, want %q", key, m.picker.mode, mode)
		}
		if m.picker.variant != key {
			t.Errorf("b%s variant = %q, want %q", key, m.picker.variant, key)
		}
	}
}

func TestBranchNotArmsWithoutRepo(t *testing.T) {
	m := newPullModel(t)
	idx := -1
	for i, e := range m.entries() {
		if e.kind == kindRepo && !e.r.project.HasRepo {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("the fixture has no row without a repo")
	}
	m.cursor = idx
	m, _ = press(m, "b")
	if m.branchArmed != nil {
		t.Errorf("the selector armed on a row without a repo: %+v", m.branchArmed)
	}
}

func TestBranchKeyIsRebindable(t *testing.T) {
	m := newPullModel(t)
	m.cfg.Keybindings["branch"] = "w"
	if got := m.cfg.KeyFor("branch"); got != "w" {
		t.Fatalf("KeyFor(branch) = %q, want the rebound key", got)
	}
	if hints := strings.Join(m.cfg.HintBarLines(), "\n"); !strings.Contains(hints, "w branch ▸") {
		t.Errorf("the hint bar did not follow the rebind: %v", m.cfg.HintBarLines())
	}
	m, _ = press(m, "w")
	if m.branchArmed == nil {
		t.Fatal("the rebound key did not arm the selector")
	}
}

func TestPickerNavigationAndEsc(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", []string{"main", "feature"}, "main")

	m, _ = press(m, "j")
	if m.picker.cursor != 1 {
		t.Errorf("j moved the cursor to %d, want 1", m.picker.cursor)
	}
	m, _ = press(m, "down")
	if m.picker.cursor != 1 {
		t.Errorf("down ran past the last branch: cursor = %d", m.picker.cursor)
	}
	m, _ = press(m, "esc")
	if m.picker != nil {
		t.Error("esc did not close the picker")
	}
}

func TestPickerEnterOnCurrentBranchIsNoOp(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", []string{"main", "feature"}, "main")

	m, cmd := press(m, "enter")
	if m.picker != nil {
		t.Error("the picker stayed open after selecting")
	}
	if len(m.running) != 0 {
		t.Errorf("selecting the current branch launched a process: %v", m.running)
	}
	if cmd == nil {
		t.Fatal("the no-op should toast")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already on main") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestPickerEnterCheckoutRuns(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feature")
	testutil.Checkout(t, dir, "main")
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m.picker = pickerOn(dir, pickerModeCurrent, "c", []string{"feature", "main"}, "main")

	m, _ = press(m, "enter")
	if m.picker != nil {
		t.Error("the picker stayed open after selecting")
	}
	if m.running[dir] != "checkout" {
		t.Fatalf("enter did not launch the checkout: running = %v", m.running)
	}
	intent := branchIntent("checkout")
	if intent == nil || intent.Key != "bc" || intent.Dir != dir {
		t.Errorf("intent = %+v, want key bc on the repo", intent)
	}
	acc, _ := collectAction(t, m)
	if acc.err != "" {
		t.Fatalf("checkout failed: %q", acc.err)
	}
	if acc.cmd != "git checkout feature" {
		t.Errorf("resolved argv = %q, want git checkout feature", acc.cmd)
	}
	argvs := execArgvs()
	if len(argvs) != 1 || strings.Join(argvs[0], " ") != "git checkout feature" {
		t.Errorf("execs = %v, want only git checkout feature", argvs)
	}
}

func TestPickerEnterWhileLoadingIsNoOp(t *testing.T) {
	m := newPullModel(t)
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", loading: true}
	m, _ = press(m, "enter")
	if m.picker == nil {
		t.Error("loading enter closed the picker, want a no-op")
	}
	if len(m.running) != 0 {
		t.Errorf("loading enter launched a process: %v", m.running)
	}
}

func TestBranchSyncWritesMarkerAndRefreshes(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	marker := "# keep me\nname = \"demo\"\nsync_branch = \"main\"\n\n[ai.pull]\nprompt = \"rebase {branch}\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "main"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m.picker = pickerOn(dir, pickerModeSync, "s", []string{"develop", "main"}, "feature")

	m, cmd := press(m, "enter")
	if m.picker != nil {
		t.Error("the picker stayed open after selecting")
	}
	if cmd == nil {
		t.Fatal("the sync-branch write should toast")
	}
	if nm, ok := cmd().(notifyMsg); !ok || nm.level != toastSuccess || !strings.Contains(nm.text, "develop") {
		t.Errorf("notification = %v, want a success naming the branch", cmd())
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# keep me\nname = \"demo\"\nsync_branch = \"develop\"\n\n[ai.pull]\nprompt = \"rebase {branch}\"\n"
	if string(raw) != want {
		t.Errorf("marker =\n%q\nwant\n%q", string(raw), want)
	}
	if m.projects[0].SyncBranch != "develop" {
		t.Errorf("the in-memory project kept sync branch %q", m.projects[0].SyncBranch)
	}
	intent := branchIntent("branch_sync")
	if intent == nil || intent.Key != "bs" {
		t.Errorf("intent = %+v, want key bs", intent)
	}

	// The recollect must land so the UI shows the new comparison ref without a restart.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			sm, ok := ev.(statusMsg)
			if !ok {
				continue
			}
			if sm.path != dir {
				t.Fatalf("status for %q, want %q", sm.path, dir)
			}
			if sm.snap.SyncBranch != "develop" {
				t.Errorf("recollected sync branch = %q, want develop", sm.snap.SyncBranch)
			}
			return
		case <-deadline:
			t.Fatal("the sync-branch write did not refresh the snapshot")
		}
	}
}

func TestOpenBranchPickerFetchesTheList(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feature")
	testutil.Checkout(t, dir, "main")
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})

	m.openBranchPicker(dir, pickerModeCurrent, "c")
	if m.picker == nil || !m.picker.loading {
		t.Fatal("the picker did not open in a loading state")
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			bm, ok := ev.(branchesMsg)
			if !ok {
				continue
			}
			if bm.err != "" {
				t.Fatalf("branch fetch failed: %s", bm.err)
			}
			got := strings.Join(bm.branches, ",")
			if !strings.Contains(got, "main") || !strings.Contains(got, "feature") {
				t.Errorf("branches = %v, want main and feature", bm.branches)
			}
			return
		case <-deadline:
			t.Fatal("the picker never published its list")
		}
	}
}

func TestBranchesMsgDroppedForClosedPicker(t *testing.T) {
	m := newPullModel(t)
	updated, _ := m.Update(branchesMsg{path: "/tmp/old-clean", mode: pickerModeCurrent, branches: []string{"main"}})
	m = updated.(Model)
	if m.picker != nil {
		t.Error("a branches message created a picker")
	}
}

func TestPickerPromptAndListInTheDashboard(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", []string{"main", "feature"}, "main")

	out := stripANSI(m.View().Content)
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "enter checkout") {
		t.Errorf("the picker prompt is not in the keybinds section:\n%s", kb)
	}
	box := sectionContent(t, out, "branches · old-clean")
	if !strings.Contains(box, "feature") {
		t.Errorf("the branch list is not painted:\n%s", box)
	}
	if !strings.Contains(box, "main (current)") {
		t.Errorf("the current branch is not marked:\n%s", box)
	}
}

func TestPickerPromptSyncMode(t *testing.T) {
	m := newPullModel(t)
	if got := m.branchArmedPrompt(); got != "" {
		t.Errorf("branchArmedPrompt without a selector = %q, want empty", got)
	}
	if got := m.branchPrompt(); got != "" {
		t.Errorf("branchPrompt without a picker = %q, want empty", got)
	}
	m.picker = pickerOn("/tmp/old-clean", pickerModeSync, "s", []string{"main"}, "main")
	if got := m.branchPrompt(); !strings.Contains(got, "set sync ref") {
		t.Errorf("sync-mode prompt = %q, want the sync wording", got)
	}
	out := stripANSI(m.View().Content)
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "enter set sync ref") {
		t.Errorf("the sync-mode prompt is not in the keybinds section:\n%s", kb)
	}
}

func TestPickerUpAndUnhandledKeys(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", []string{"main", "feature"}, "main")
	m.picker.cursor = 1
	m, _ = press(m, "k")
	if m.picker.cursor != 0 {
		t.Errorf("k did not move the cursor up: %d", m.picker.cursor)
	}
	// An unhandled key is swallowed, not forwarded to the table.
	if out, _, handled := m.handlePickerKey("x"); !handled || out.(Model).picker == nil {
		t.Error("an unhandled key closed or leaked past the picker")
	}
	// The quit keys keep their course so the terminal is never trapped.
	if _, _, handled := m.handlePickerKey("q"); handled {
		t.Error("q was consumed by the picker, want it to reach the quit routing")
	}
	if _, _, handled := m.handlePickerKey("ctrl+c"); handled {
		t.Error("ctrl+c was consumed by the picker, want it to reach the quit routing")
	}
}

func TestPickerWindowFollowsTheCursor(t *testing.T) {
	if got := pickerWindow(3, 1, 5); got != 0 {
		t.Errorf("a list that fits = offset %d, want 0", got)
	}
	if got := pickerWindow(10, 9, 5); got != 5 {
		t.Errorf("cursor at the tail = offset %d, want 5", got)
	}
	if got := pickerWindow(10, 0, 5); got != 0 {
		t.Errorf("cursor at the head = offset %d, want 0", got)
	}
}

func TestBranchSectionEmptyAndError(t *testing.T) {
	m := newPullModel(t)
	if got := m.branchSection(6); got != "" {
		t.Errorf("branchSection without a picker = %q, want empty", got)
	}
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", err: "boom"}
	if box := stripANSI(m.branchSection(6)); !strings.Contains(box, "branch list failed") {
		t.Errorf("error section = %q, want the failure wording", box)
	}
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", branches: nil}
	if box := stripANSI(m.branchSection(6)); !strings.Contains(box, "no local branches") {
		t.Errorf("empty section = %q, want the empty wording", box)
	}
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", loading: true}
	if box := stripANSI(m.branchSection(6)); !strings.Contains(box, "loading branches") {
		t.Errorf("loading section = %q, want the loading wording", box)
	}
}

func TestPickerEnterOnFailedListIsNoOp(t *testing.T) {
	m := newPullModel(t)
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", err: "boom", branches: []string{"main"}}
	m, _ = press(m, "enter")
	if m.picker == nil {
		t.Error("an enter on a failed list closed the picker, want a no-op")
	}
	if len(m.running) != 0 {
		t.Errorf("an enter on a failed list launched a process: %v", m.running)
	}
}

func TestBranchSyncMarkerWriteFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m.picker = pickerOn(dir, pickerModeSync, "s", []string{"develop"}, "main")

	m, cmd := press(m, "enter")
	if m.picker != nil {
		t.Error("the picker stayed open after a failed write")
	}
	if cmd == nil {
		t.Fatal("a failed marker write should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || nm.level != toastError || !strings.Contains(nm.text, "marker write failed") {
		t.Errorf("notification = %v, want the marker-write error", cmd())
	}
	if len(m.running) != 0 {
		t.Errorf("a failed write recollected: %v", m.running)
	}
}
