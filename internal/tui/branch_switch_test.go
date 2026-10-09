package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func pickerBranches(names ...string) []gitstatus.Branch {
	out := make([]gitstatus.Branch, 0, len(names))
	for _, n := range names {
		out = append(out, gitstatus.Branch{Name: n, HasUpstream: true})
	}
	return out
}

func pickerOn(path, mode, variant string, branches []gitstatus.Branch, current string) *branchPicker {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "filter…"
	in.SetWidth(pickerInputWidth(120))
	in.Focus()
	return &branchPicker{
		path: path, mode: mode, variant: variant,
		branches: branches, current: current, filter: in,
	}
}

func localBranch(name string) gitstatus.Branch {
	return gitstatus.Branch{Name: name, HasUpstream: true}
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

func TestPickerArrowAndJKNavigation(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

	m, _ = press(m, "down")
	if m.picker.cursor != 1 {
		t.Errorf("down moved the cursor to %d, want 1", m.picker.cursor)
	}
	m, _ = press(m, "down")
	if m.picker.cursor != 1 {
		t.Errorf("down ran past the last branch: cursor = %d", m.picker.cursor)
	}
	m, _ = press(m, "up")
	if m.picker.cursor != 0 {
		t.Errorf("up moved the cursor to %d, want 0", m.picker.cursor)
	}
	// j/k keep navigating while the filter is empty.
	m, _ = press(m, "j")
	if m.picker.cursor != 1 {
		t.Errorf("j with an empty filter moved the cursor to %d, want 1", m.picker.cursor)
	}
	m, _ = press(m, "k")
	if m.picker.cursor != 0 {
		t.Errorf("k with an empty filter moved the cursor to %d, want 0", m.picker.cursor)
	}
	m, _ = press(m, "esc")
	if m.picker != nil {
		t.Error("esc did not close the picker")
	}
}

func TestPickerFiltersAsYouType(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("alpha", "beta", "gamma"), "alpha")

	m, _ = press(m, "b")
	m, _ = press(m, "e")
	m, _ = press(m, "t")
	if got := m.picker.filter.Value(); got != "bet" {
		t.Fatalf("filter = %q, want bet", got)
	}
	list := m.picker.filtered()
	if len(list) != 1 || list[0].Name != "beta" {
		t.Fatalf("filtered = %v, want only beta", list)
	}
	// Backspace reopens the list (three times: "bet" → "be" still matches beta).
	for range 3 {
		m, _ = press(m, "backspace")
	}
	if len(m.picker.filtered()) != len(m.picker.branches) {
		t.Errorf("backspace did not reopen the list: %v", m.picker.filtered())
	}
}

func TestPickerEnterUsesTheFilteredSelection(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feature")
	testutil.Checkout(t, dir, "main")
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m.picker = pickerOn(dir, pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

	m, _ = press(m, "f")
	m, _ = press(m, "e")
	m, _ = press(m, "enter")
	if m.picker != nil {
		t.Error("the picker stayed open after selecting")
	}
	if m.running[dir] != "checkout" {
		t.Fatalf("enter did not launch the checkout: running = %v", m.running)
	}
	acc, _ := collectAction(t, m)
	if acc.err != "" {
		t.Fatalf("checkout failed: %q", acc.err)
	}
	if acc.cmd != "git checkout feature" {
		t.Errorf("resolved argv = %q, want git checkout feature", acc.cmd)
	}
}

func TestPickerRemoteBranchChecksOutTheLocalName(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "other")
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("other")})
	m.picker = pickerOn(dir, pickerModeCurrent, "c", []gitstatus.Branch{
		localBranch("other"),
		{Name: "origin/main", Remote: true, HasUpstream: true},
	}, "other")

	m, _ = press(m, "down")
	m, _ = press(m, "enter")
	if m.running[dir] != "checkout" {
		t.Fatalf("enter did not launch the checkout: running = %v", m.running)
	}
	acc, _ := collectAction(t, m)
	if acc.cmd != "git checkout main" {
		t.Errorf("resolved argv = %q, want the local name (git checkout main)", acc.cmd)
	}
}

func TestPickerEnterOnCurrentBranchIsNoOp(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

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
	m.picker = pickerOn(dir, pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

	m, _ = press(m, "down")
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
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", loading: true, filter: textinput.New()}
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
	m.picker = pickerOn(dir, pickerModeSync, "s", pickerBranches("develop", "main"), "feature")

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

func TestBranchSyncWritesTheRemoteRefAsPicked(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.PushUpstreamCommits(t, origin, 2, "up")
	testutil.FetchLocal(t, dir)
	marker := "# keep\nname = \"demo\"\nsync_branch = \"main\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "main"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("feature")})
	m.picker = pickerOn(dir, pickerModeSync, "s",
		[]gitstatus.Branch{{Name: "origin/main", Remote: true, HasUpstream: true}}, "feature")

	m, cmd := press(m, "enter")
	if m.picker != nil {
		t.Fatal("the picker stayed open after selecting")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "origin/main") {
		t.Errorf("notification = %v, want the full remote ref", cmd())
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# keep\nname = \"demo\"\nsync_branch = \"origin/main\"\n"
	if string(raw) != want {
		t.Errorf("marker =\n%q\nwant the picked ref\n%q", string(raw), want)
	}
	if m.projects[0].SyncBranch != "origin/main" {
		t.Errorf("in-memory sync branch = %q, want origin/main", m.projects[0].SyncBranch)
	}

	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			sm, ok := ev.(statusMsg)
			if !ok || sm.path != dir {
				continue
			}
			if sm.snap.SyncBranch != "origin/main" {
				t.Errorf("snapshot sync branch = %q, want origin/main", sm.snap.SyncBranch)
			}
			if !sm.snap.SyncKnown {
				t.Errorf("snapshot did not resolve the remote sync ref: %+v", sm.snap)
			}
			if len(sm.snap.SyncCommits) == 0 {
				t.Errorf("snapshot has no sync commits for the remote ref")
			}
			return
		case <-deadline:
			t.Fatal("the sync-branch write did not refresh the snapshot")
		}
	}
}

func TestOpenBranchPickerFetchesTheList(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.NewBranch(t, dir, "feature")
	testutil.Checkout(t, dir, "main")
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
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
			names := make([]string, 0, len(bm.branches))
			for _, b := range bm.branches {
				names = append(names, b.Name)
			}
			got := strings.Join(names, ",")
			for _, want := range []string{"main", "feature", "origin/main"} {
				if !strings.Contains(got, want) {
					t.Errorf("branches = %v, want %q", names, want)
				}
			}
			return
		case <-deadline:
			t.Fatal("the picker never published its list")
		}
	}
}

func TestBranchesMsgDroppedForClosedPicker(t *testing.T) {
	m := newPullModel(t)
	updated, _ := m.Update(branchesMsg{path: "/tmp/old-clean", mode: pickerModeCurrent, branches: pickerBranches("main")})
	m = updated.(Model)
	if m.picker != nil {
		t.Error("a branches message created a picker")
	}
}

func TestBranchesMsgAcceptedForOpenPicker(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", nil, "main")
	m.picker.loading = true
	updated, _ := m.Update(branchesMsg{path: "/tmp/old-clean", mode: pickerModeCurrent, branches: pickerBranches("main", "feature")})
	m = updated.(Model)
	if m.picker == nil {
		t.Fatal("the matching answer closed the picker")
	}
	if m.picker.loading {
		t.Error("the picker still shows loading after the list arrived")
	}
	if len(m.picker.branches) != 2 {
		t.Errorf("branches = %d, want 2", len(m.picker.branches))
	}
	if m.picker.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.picker.cursor)
	}
}

func TestPickerFilterResizesWithTerminal(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main"), "main")
	m = resize(m, 40, 20)
	if m.picker == nil {
		t.Fatal("the resize closed the picker")
	}
	if m.width != 40 {
		t.Errorf("width = %d, want 40", m.width)
	}
}

func TestPickerOverlayKeepsTheDashboardBehind(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

	out := stripANSI(m.View().Content)
	// The table stays visible around the modal, and the modal floats as its own box.
	if !strings.Contains(out, "repos") || !strings.Contains(out, "old-clean") {
		t.Errorf("the dashboard is not behind the overlay:\n%s", out)
	}
	if !strings.Contains(out, "╭ branches · old-clean") {
		t.Errorf("the modal box is not painted:\n%s", out)
	}
	if !strings.Contains(out, "feature") || !strings.Contains(out, "main (current)") {
		t.Errorf("the branch list is not painted:\n%s", out)
	}
	if !strings.Contains(out, "enter select") || !strings.Contains(out, "filter") {
		t.Errorf("the modal does not show its legend:\n%s", out)
	}
	// The hint bar is not replaced by the modal.
	if kb := sectionContent(t, out, "keybinds"); strings.Contains(kb, "enter select") {
		t.Errorf("the modal legend leaked into the keybinds section:\n%s", kb)
	}
}

func TestPickerOverlayLabelsAndEmptyStates(t *testing.T) {
	m := newPullModel(t)
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", err: "boom", filter: textinput.New()}
	if box := stripANSI(strings.Join(m.pickerOverlay(120, 30), "\n")); !strings.Contains(box, "branch list failed") {
		t.Errorf("error overlay = %q, want the failure wording", box)
	}
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", loading: true, filter: textinput.New()}
	if box := stripANSI(strings.Join(m.pickerOverlay(120, 30), "\n")); !strings.Contains(box, "loading branches") {
		t.Errorf("loading overlay = %q, want the loading wording", box)
	}
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c",
		branches: []gitstatus.Branch{{Name: "main", HasUpstream: true}, {Name: "feature"}}, current: "main", filter: textinput.New()}
	box := stripANSI(strings.Join(m.pickerOverlay(120, 30), "\n"))
	if !strings.Contains(box, "feature (no upstream)") {
		t.Errorf("no-upstream branch is not labelled:\n%s", box)
	}
}

func TestPickerWindowFollowsTheCursor(t *testing.T) {
	if got := branchWindow(3, 1, 5); got != 0 {
		t.Errorf("a list that fits = offset %d, want 0", got)
	}
	if got := branchWindow(10, 9, 5); got != 5 {
		t.Errorf("cursor at the tail = offset %d, want 5", got)
	}
	if got := branchWindow(10, 0, 5); got != 0 {
		t.Errorf("cursor at the head = offset %d, want 0", got)
	}
}

// The existing navigation test never presses the keys AT the edges, which is exactly where the
// clamps decide: up/k at the first row must not go negative, j at the last must not run past it.
func TestPickerNavigationClampsAtBothEdges(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

	m, _ = press(m, "up")
	if m.picker.cursor != 0 {
		t.Errorf("up at the first row = %d, want 0", m.picker.cursor)
	}
	m, _ = press(m, "k")
	if m.picker.cursor != 0 {
		t.Errorf("k at the first row = %d, want 0", m.picker.cursor)
	}
	m, _ = press(m, "j")
	if m.picker.cursor != 1 {
		t.Fatalf("j from the first row = %d, want 1", m.picker.cursor)
	}
	m, _ = press(m, "j")
	if m.picker.cursor != 1 {
		t.Errorf("j at the last row = %d, want 1", m.picker.cursor)
	}
	m, _ = press(m, "down")
	if m.picker.cursor != 1 {
		t.Errorf("down at the last row = %d, want 1", m.picker.cursor)
	}
}

// Literal widths: 40-6=34 sits between the 24 floor and the 64 cap, so the subtraction is the only
// term that decides, and 34-4=30 pins the input's own subtraction.
func TestPickerWidthsAtTheClamps(t *testing.T) {
	if got := pickerBoxWidth(40); got != 34 {
		t.Errorf("pickerBoxWidth(40) = %d, want 34", got)
	}
	if got := pickerBoxWidth(20); got != 24 {
		t.Errorf("pickerBoxWidth(20) = %d, want the 24 floor", got)
	}
	if got := pickerBoxWidth(200); got != 64 {
		t.Errorf("pickerBoxWidth(200) = %d, want the 64 cap", got)
	}
	if got := pickerInputWidth(40); got != 30 {
		t.Errorf("pickerInputWidth(40) = %d, want 30", got)
	}
}

// The row budget is height minus the chrome; a small height is the only place where that
// subtraction decides instead of the 12-row cap, so the box's line count pins it.
func TestPickerOverlayRowBudget(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("a", "b", "c", "d", "e"), "a")
	// height 6 - 4 chrome = 2 rows: the filter, 2 branch lines and the hint, plus the two borders.
	if got := len(m.pickerOverlay(40, 6)); got != 6 {
		t.Errorf("overlay at height 6 = %d lines, want 6 (2 rows + chrome)", got)
	}

	many := make([]string, 15)
	for i := range many {
		many[i] = "b" + string(rune('a'+i))
	}
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches(many...), "ba")
	// 15 branches but the 12-row cap binds: 1 filter + 12 rows + 1 hint + 2 borders.
	if got := len(m.pickerOverlay(40, 30)); got != 16 {
		t.Errorf("overlay with 15 branches = %d lines, want 16 (12-row cap)", got)
	}
}

// The inner width is the box width minus the 6 chrome cells: a name longer than that is truncated
// to exactly that many runes, which is what makes the subtraction observable.
func TestPickerRowsTruncateToTheComputedInnerWidth(t *testing.T) {
	m := newPullModel(t)
	m.width = 40 // pickerBoxWidth(40)=34, inner = 34-6 = 28
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches(strings.Repeat("x", 60)), "main")

	rows := m.branchRows(m.picker.filtered(), 1)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := stripANSI(rows[0])
	if n := len([]rune(got)); n != 4+28 {
		t.Errorf("painted row = %d runes, want 32 (28-char truncation + the 4-cell prefix): %q", n, got)
	}
}

// With the window scrolled (offset > 0) the marker must follow the cursor by OFFSET+i; the marker
// and its row are asserted so both the arithmetic and the equality mutation are held down.
func TestPickerRowsMarkOnlyTheCursorRow(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c",
		pickerBranches("b0", "b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9"), "b0")
	m.picker.cursor = 9

	rows := m.branchRows(m.picker.filtered(), 5)
	marks := 0
	for i, r := range rows {
		if strings.Contains(r, "▸") {
			marks++
			if i != 4 {
				t.Errorf("the cursor mark is on row %d, want 4 (window offset 5 + cursor 9)", i)
			}
		}
	}
	if marks != 1 {
		t.Errorf("cursor marks = %d, want exactly 1", marks)
	}
}

// Only the current branch is dimmed, and the ANSI is checked before stripANSI: the style is applied
// to the truncated text, so the exact rendered run is what identifies it.
func TestPickerRowsDimOnlyTheCurrentBranch(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main", "feature"), "main")

	rows := m.branchRows(m.picker.filtered(), 2)
	if !strings.Contains(rows[0], styleDim.Render("main (current)")) {
		t.Errorf("the current branch is not dimmed: %q", rows[0])
	}
	if strings.Contains(rows[1], styleDim.Render("feature")) {
		t.Errorf("a non-current branch is dimmed: %q", rows[1])
	}
}

// A failed `git branch` listing has to reach the picker as an error, not be swallowed by the
// err==nil arm of the goroutine.
func TestOpenBranchPickerSurfacesTheListFailure(t *testing.T) {
	dir := t.TempDir() // not a repo: git branch fails
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})

	m.openBranchPicker(dir, pickerModeCurrent, "c")
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			bm, ok := ev.(branchesMsg)
			if !ok {
				continue
			}
			if bm.err == "" {
				t.Fatal("a failed branch listing did not surface an error to the picker")
			}
			return
		case <-deadline:
			t.Fatal("the picker never answered the failed listing")
		}
	}
}

func TestPickerCTRLCPassesThrough(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("main"), "main")
	msg := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	if _, _, handled := m.handlePickerKey(msg); handled {
		t.Error("ctrl+c was consumed by the picker, want it to reach the quit routing")
	}
}

func TestPickerEnterOnFailedListIsNoOp(t *testing.T) {
	m := newPullModel(t)
	m.picker = &branchPicker{path: "/tmp/old-clean", mode: pickerModeCurrent, variant: "c", err: "boom",
		branches: pickerBranches("main"), filter: textinput.New()}
	m, _ = press(m, "enter")
	if m.picker == nil {
		t.Error("an enter on a failed list closed the picker, want a no-op")
	}
	if len(m.running) != 0 {
		t.Errorf("an enter on a failed list launched a process: %v", m.running)
	}
}

func TestCheckoutTargetStripsTheRemotePrefix(t *testing.T) {
	if got := checkoutTarget(gitstatus.Branch{Name: "origin/feature", Remote: true}); got != "feature" {
		t.Errorf("checkoutTarget(remote) = %q, want feature", got)
	}
	if got := checkoutTarget(gitstatus.Branch{Name: "main"}); got != "main" {
		t.Errorf("checkoutTarget(local) = %q, want main", got)
	}
}

func TestPickerNilGuardsAndLabels(t *testing.T) {
	m := newPullModel(t)
	if got := m.branchArmedPrompt(); got != "" {
		t.Errorf("branchArmedPrompt without a selector = %q, want empty", got)
	}
	if out, cmd := m.selectPickerBranch(); out.(Model).picker != nil || cmd != nil {
		t.Error("selectPickerBranch without a picker did something")
	}
	if got := m.pickerOverlay(120, 30); got != nil {
		t.Errorf("pickerOverlay without a picker = %v, want nil", got)
	}
	if got := branchLabel(gitstatus.Branch{Name: "origin/main", Remote: true}, "local"); got != "origin/main (remote)" {
		t.Errorf("remote label = %q, want the remote wording", got)
	}
}

func TestPickerOverlaySaysWhenNothingMatches(t *testing.T) {
	m := newPullModel(t)
	m.picker = pickerOn("/tmp/old-clean", pickerModeCurrent, "c", pickerBranches("alpha", "beta"), "alpha")
	m, _ = press(m, "z")
	if box := stripANSI(strings.Join(m.pickerOverlay(120, 30), "\n")); !strings.Contains(box, "no matching branches") {
		t.Errorf("empty filter overlay = %q, want the no-match wording", box)
	}
	// enter with nothing matching is a no-op, not a close.
	if out, _ := m.selectPickerBranch(); out.(Model).picker == nil {
		t.Error("enter on an empty list closed the picker")
	}
}

func TestOverlayCenteredGuards(t *testing.T) {
	if got := overlayCentered("base", nil, 10, 3); got != "base" {
		t.Errorf("empty block = %q, want the base", got)
	}
	if got := overlayCentered("base", []string{"x"}, 0, 3); got != "base" {
		t.Errorf("zero width = %q, want the base", got)
	}
	// height beyond the base clamps to the base's own height (the block still lands).
	if got := overlayCentered("a\nb", []string{"x"}, 5, 99); !strings.Contains(got, "x") {
		t.Errorf("height clamp = %q, want the block drawn", got)
	}
}

func TestBranchSyncMarkerWriteFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m.picker = pickerOn(dir, pickerModeSync, "s", pickerBranches("develop"), "main")

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
