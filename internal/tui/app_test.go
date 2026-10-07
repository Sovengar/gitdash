package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
	"gitdash/internal/testutil"
)

func newTestModel(t *testing.T, projects []discovery.Project, states map[string]gitstatus.Snapshot) Model {
	t.Helper()
	m := New(config.Defaults())
	m.width, m.height = 120, 30
	m.projects = projects
	m.states = states
	m.scanning = false
	m.store = state.NewStoreAt(t.TempDir())
	m.collapsed = map[string]bool{}
	m.expanded = map[string]bool{}
	// Cancelling the context at test end kills the work in flight (per-repo fetch, actions, handoffs) BEFORE the next test installs its recorder: otherwise a stray goroutine logs its git reads into the next test's log and fails it for what another test did.
	t.Cleanup(m.cancel)
	return m
}

func proj(name, path string, hasRepo bool) discovery.Project {
	return discovery.Project{Path: path, Name: name, HasRepo: hasRepo}
}

func snapClean() gitstatus.Snapshot {
	return gitstatus.Snapshot{
		Status:     gitstatus.Status{Branch: "main", HasUpstream: true, Upstream: "origin/main"},
		LastCommit: time.Now().Add(-2 * time.Hour).Unix(),
	}
}

func snapDirty(tracked, untracked int) gitstatus.Snapshot {
	s := snapClean()
	s.Status.TrackedChanges = tracked
	s.Status.Untracked = untracked
	return s
}

func snapAhead(n int) gitstatus.Snapshot {
	s := snapClean()
	s.Status.Ahead = n
	return s
}

func snapBehind(n int) gitstatus.Snapshot {
	s := snapClean()
	s.Status.Behind = n
	return s
}

func snapDiverged(a, b int) gitstatus.Snapshot {
	s := snapClean()
	s.Status.Ahead = a
	s.Status.Behind = b
	return s
}

func snapNoUpstream() gitstatus.Snapshot {
	s := snapClean()
	s.Status.HasUpstream = false
	s.Status.Upstream = ""
	return s
}

// Armed warnings are painted inside a specific section (keybinds), so checking where something landed means looking inside the box and not in the plain text.
func sectionContent(t *testing.T, view, title string) string {
	t.Helper()
	lines := strings.Split(view, "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, "╭ "+title+" ") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("the section %q is not in the view:\n%s", title, view)
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "╰") {
			return strings.Join(lines[start+1:i], "\n")
		}
	}
	t.Fatalf("the section %q does not close:\n%s", title, view)
	return ""
}

// Modifiers are not deduced from the text (the input needs Code and the Mod separately), so a "ctrl+s" without Mod would be an "s" and "shift+tab" a "tab".
var namedKeys = map[string]tea.KeyPressMsg{
	"enter":     {Code: tea.KeyEnter},
	"esc":       {Code: tea.KeyEsc},
	"up":        {Code: tea.KeyUp},
	"down":      {Code: tea.KeyDown},
	"home":      {Code: tea.KeyHome},
	"end":       {Code: tea.KeyEnd},
	"backspace": {Code: tea.KeyBackspace},
	"tab":       {Code: tea.KeyTab},
	"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
	"ctrl+s":    {Code: 's', Mod: tea.ModCtrl},
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
}

func press(m Model, key string) (Model, tea.Cmd) {
	km, named := namedKeys[key]
	if !named {
		km = tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	}
	out, cmd := m.Update(km)
	return out.(Model), cmd
}

func fixtureProjects() ([]discovery.Project, map[string]gitstatus.Snapshot) {
	projects := []discovery.Project{
		proj("old-clean", "/tmp/old-clean", true),
		proj("dirty-api", "/tmp/dirty-api", true),
		proj("ahead-lib", "/tmp/ahead-lib", true),
		proj("behind-web", "/tmp/behind-web", true),
		proj("no-up-cli", "/tmp/no-up-cli", true),
		proj("no-repo-docs", "/tmp/no-repo-docs", false),
	}
	states := map[string]gitstatus.Snapshot{
		"/tmp/old-clean":    snapClean(),
		"/tmp/dirty-api":    snapDirty(1, 2),
		"/tmp/ahead-lib":    snapAhead(2),
		"/tmp/behind-web":   snapBehind(3),
		"/tmp/no-up-cli":    snapNoUpstream(),
		"/tmp/no-repo-docs": {},
	}
	return projects, states
}

func TestSortAttentionFirst(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	rows := m.rows()
	if len(rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(rows))
	}
	if rows[0].project.Name != "dirty-api" {
		t.Errorf("rows[0] = %s, want dirty-api", rows[0].project.Name)
	}
	if rows[len(rows)-1].project.Name != "old-clean" {
		t.Errorf("last = %s, want old-clean", rows[len(rows)-1].project.Name)
	}
}

func TestSortActivityTie(t *testing.T) {
	recent := gitstatus.Snapshot{Status: snapClean().Status, LastCommit: time.Now().Unix()}
	old := snapClean()
	projects := []discovery.Project{proj("aaa", "/a", true), proj("bbb", "/b", true)}
	states := map[string]gitstatus.Snapshot{"/a": old, "/b": recent}
	m := newTestModel(t, projects, states)
	rows := m.rows()
	if rows[0].project.Name != "bbb" {
		t.Errorf("rows[0] = %s, want bbb (more recent)", rows[0].project.Name)
	}
}

func TestFilterOnlyDirty(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if len(m.rows()) != 6 {
		t.Fatalf("with no filter rows = %d", len(m.rows()))
	}

	m, _ = press(m, "d")
	rows := m.rows()
	if len(rows) != 3 { // dirty-api, ahead-lib, behind-web
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for _, r := range rows {
		if !pendingStates(r.state) {
			t.Errorf("row %s is not pending", r.project.Name)
		}
	}

	m, _ = press(m, "d")
	if len(m.rows()) != 6 {
		t.Errorf("toggle off: rows = %d, want 6", len(m.rows()))
	}
}

func TestSearch(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "/")
	if !m.searchActive {
		t.Fatal("'/' did not open the input")
	}
	for _, c := range "api" {
		m, _ = press(m, string(c))
	}
	rows := m.rows()
	if len(rows) != 1 || rows[0].project.Name != "dirty-api" {
		t.Errorf("live: rows = %v", rowNames(rows))
	}

	m, _ = press(m, "enter")
	if m.searchActive || m.search != "api" {
		t.Errorf("confirm: active=%v search=%q", m.searchActive, m.search)
	}

	m, _ = press(m, "/")
	for range 3 {
		m, _ = press(m, "backspace")
	}
	m, _ = press(m, "esc")
	if m.search != "" {
		t.Errorf("esc did not clear the filter (search=%q)", m.search)
	}
}

func TestSearchImmediateFeedback(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "/")
	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "[/") {
		t.Errorf("[/…] missing when entering filter mode:\n%s", out)
	}
	if !strings.Contains(out, "name/group") {
		t.Errorf("placeholder not visible on open:\n%s", out)
	}

	m, _ = press(m, "a")
	m, _ = press(m, "enter")
	out = stripANSI(m.renderDashboard())
	if !strings.Contains(out, "[/a]") {
		t.Errorf("after confirming, [/a] is missing:\n%s", out)
	}
}

func TestSearchMatchesGroup(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/x", Name: "api", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/y", Name: "cli", PrimaryGroup: "other", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/x": snapClean(), "/y": snapClean()}
	m := newTestModel(t, projects, states)
	m.search = "vsocial"
	rows := m.rows()
	if len(rows) != 1 || rows[0].project.Name != "api" {
		t.Errorf("search by primary failed: %v", rowNames(rows))
	}
	m2 := newTestModel(t, projects, states)
	m2.search = "backend"
	rows = m2.rows()
	if len(rows) != 1 || rows[0].project.Name != "api" {
		t.Errorf("search by secondary failed: %v", rowNames(rows))
	}
}

func TestNavigationBounds(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	for range 10 {
		m, _ = press(m, "j")
	}
	if m.cursor != len(m.rows())-1 {
		t.Errorf("cursor = %d, want %d (lower bound)", m.cursor, len(m.rows())-1)
	}
	for range 10 {
		m, _ = press(m, "k")
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (upper bound)", m.cursor)
	}
}

func TestPanelShowsTheFiles(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	s.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	states["/tmp/dirty-api"] = s
	m := newTestModel(t, projects, states)

	out := stripANSI(m.renderDashboard())
	for _, want := range []string{"main.go", "/tmp/dirty-api", "╭ dirty-api "} {
		if !strings.Contains(out, want) {
			t.Errorf("the panel does not say %q:\n%s", want, out)
		}
	}
	before := stripANSI(m.renderDashboard())
	m, _ = press(m, "enter")
	if !strings.HasPrefix(stripANSI(m.renderDashboard()), before[:40]) {
		t.Errorf("enter on a repo with no worktrees changed the view:\n%s", stripANSI(m.renderDashboard()))
	}
	if len(m.expanded) != 0 || len(m.collapsed) != 0 {
		t.Errorf("enter on a repo with no worktrees folded something: expanded=%v collapsed=%v", m.expanded, m.collapsed)
	}
}

func TestDetailShowsLastAction(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.lastAction["/tmp/old-clean"] = actionResult{kind: "pull", output: "error: pull diverged\n", err: "exit 1"}
	m.search = "old-clean"
	r, _ := m.selected()
	out := m.renderDetail(r, m.height)
	if !strings.Contains(out, "pull") || !strings.Contains(out, "failed") || !strings.Contains(out, "diverged") {
		t.Errorf("detail without the last action:\n%s", out)
	}
}

func TestGuardNotRepo(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.search = "no-repo" // cursor on the project with no repo
	m.cursor = 0

	for _, key := range []string{"p", "P", "f"} {
		_, cmd := press(m, key)
		if cmd == nil {
			t.Errorf("key %s without a guard over no-repo", key)
			continue
		}
		msg := cmd()
		if nm, ok := msg.(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
			if key != "f" { // f on a no-repo project: fetchTargets excludes it, a nil cmd is valid
				t.Errorf("key %s notified %v", key, msg)
			}
		}
	}
}

func TestBlockRunningAction(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.search = "old-clean"
	m.running["/tmp/old-clean"] = "pull"

	_, cmd := press(m, "P")
	if cmd == nil {
		t.Fatal("push without a blocking cmd")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestSummary(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	total, dirty, ahead, behind := m.summary()
	if total != 6 || dirty != 1 || ahead != 1 || behind != 1 {
		t.Errorf("summary = (%d, %d, %d, %d), want (6, 1, 1, 1)", total, dirty, ahead, behind)
	}
}

func TestViewContainsTable(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := m.View().Content
	for _, want := range []string{"gitdash", "dirty-api", "↑2", "↓3", "6 repos"} {
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("the view does not contain %q", want)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		epoch int64
		want  string
	}{
		{0, "-"},
		{now.Add(-30 * time.Second).Unix(), "now"},
		{now.Add(-5 * time.Minute).Unix(), "5m"},
		{now.Add(-3 * time.Hour).Unix(), "3h"},
		{now.Add(-2 * 24 * time.Hour).Unix(), "2d"},
	}
	for _, c := range cases {
		if got := relativeTime(c.epoch); got != c.want {
			t.Errorf("relativeTime(%d) = %q, want %q", c.epoch, got, c.want)
		}
	}
}

func TestTruncatePad(t *testing.T) {
	if got := truncate("abcdefghijkl", 8); got != "abcdefg…" {
		t.Errorf("truncate = %q", got)
	}
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
}

func rowNames(rows []row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.project.Name)
	}
	return out
}

// If the note were lost, a mistyped `root` in the config would show up as "I find no repos" instead of "this root does not exist", which is exactly the confusion the warning avoids.
func TestTheScanReportsTheUnreadableRootAndKeepsTheReadableRepos(t *testing.T) {
	good, _ := testutil.NewRepo(t, true)
	testutil.Marker(t, good, "ok", "g", "s", false)

	bad := filepath.Join(t.TempDir(), "this-is-not-a-directory")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatalf("preparing the bad root: %v", err)
	}

	cfg := config.Defaults()
	cfg.Roots = []string{good, bad}
	m := New(cfg)
	t.Cleanup(m.cancel)

	m.startScanCmd()
	sp := firstScanMsg(t, m)

	if sp.note == "" {
		t.Fatal("empty note: the unreadable root never reaches the user, and a mistyped root looks like there are no repos")
	}
	if !strings.Contains(sp.note, "unreadable root") {
		t.Errorf("the note does not name the problem: %q", sp.note)
	}
	var paths []string
	for _, p := range sp.projects {
		paths = append(paths, p.Path)
	}
	if len(paths) != 1 || paths[0] != good {
		t.Errorf("the scan threw away the repos that could be read: %v", paths)
	}
}

// With a deadline, a pipeline that emits nothing fails with a message saying what happened instead of hanging until the global go test timeout.
func firstScanMsg(t *testing.T, m Model) scanProjectsMsg {
	t.Helper()
	type res struct {
		msg tea.Msg
	}
	ch := make(chan res, 1)
	go func() { ch <- res{waitForEvent(m.events)()} }()
	select {
	case r := <-ch:
		sp, ok := r.msg.(scanProjectsMsg)
		if !ok {
			t.Fatalf("first event = %T, want scanProjectsMsg", r.msg)
		}
		return sp
	case <-time.After(10 * time.Second):
		t.Fatal("the scan emitted no event in 10s")
		return scanProjectsMsg{}
	}
}

// `collectDoneMsg` was handled by no test, so the end of the scan was unpinned: neither the cache that makes the app paint instantly nor the automatic fetch. Inverting the `cache.Path()` guard leaves the app looking healthy while never caching, which is why this looks at the real file and not at a return value.
func TestTheEndOfTheScanGuardTheCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	m := newTestModel(t,
		[]discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m.cfg.FetchAuto = false // this test is about the cache, not about fetch

	m.Update(collectDoneMsg{})

	path := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "gitdash", "repos.json")
	waitForFile(t, path)

	var c cache.File
	if err := json.Unmarshal(readFile(t, path), &c); err != nil {
		t.Fatalf("unreadable cache: %v", err)
	}
	if len(c.Repos) != 1 || c.Repos[0].Path != "/tmp/api" {
		t.Errorf("the cache did not store what was scanned: %+v", c.Repos)
	}
}

// The event is looked at, not the return value, because `fetchBatchCmd` publishes through the channel and always returns nil.
func TestTheEndOfTheScanLaunchesTheFetchAutomatic(t *testing.T) {
	cases := []struct {
		name      string
		fetchAuto bool
		snap      gitstatus.Snapshot
		want      bool
	}{
		{"with upstream", true, snapClean(), true},
		{"no automatic fetch", false, snapClean(), false},
		{"repo no upstream", true, gitstatus.Snapshot{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			m := newTestModel(t,
				[]discovery.Project{proj("api", "/tmp/api", true)},
				map[string]gitstatus.Snapshot{"/tmp/api": c.snap})
			m.cfg.FetchAuto = c.fetchAuto

			m.Update(collectDoneMsg{})

			// The deadline is only long for the case that must LAUNCH: in the negative ones the "fetching" event (emitted before touching git) cannot be slow, so waiting longer only lengthens the suite.
			timeout := 200 * time.Millisecond
			if c.want {
				timeout = 5 * time.Second
			}
			seen := awaitEvent(t, m, timeout, func(ev event) bool {
				fs, ok := ev.(fetchStateMsg)
				return ok && fs.path == "/tmp/api"
			})
			if c.want && !seen {
				t.Error("the automatic fetch did not launch when the scan ended")
			}
			if !c.want && seen {
				t.Error("a fetch that should not fire ran: no automatic fetch or no upstream")
			}
		})
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the cache never showed up in %s", path)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return raw
}

func awaitEvent(t *testing.T, m Model, timeout time.Duration, pred func(event) bool) bool {
	t.Helper()
	ch := make(chan bool, 1)
	go func() {
		for {
			ev := waitForEvent(m.events)()
			if ev == nil {
				return
			}
			if pred(ev) {
				ch <- true
				return
			}
		}
	}()
	select {
	case ok := <-ch:
		return ok
	case <-time.After(timeout):
		return false
	}
}

// New discards the error from state.NewStore(), so with an unresolvable state the store is nil and these guards keep LoadCollapsed off a null pointer; a user with no HOME could not even open the app.
func TestNewSurvivesWithNoStateDirToSaveTheFolded(t *testing.T) {
	t.Run("no state directory", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", "")

		m := New(config.Defaults()) // it must not blow up
		t.Cleanup(m.cancel)

		if len(m.collapsed) != 0 || len(m.expanded) != 0 {
			t.Errorf("with no store there is no folding to load, but %v / %v",
				m.collapsed, m.expanded)
		}
	})

	t.Run("with persisted folding", func(t *testing.T) {
		base := t.TempDir()
		store := state.NewStoreAt(filepath.Join(base, "gitdash"))
		if err := os.MkdirAll(store.Base(), 0o755); err != nil {
			t.Fatal(err)
		}
		raw := fmt.Sprintf(`{"g/s":true,%q:true}`, state.WorktreePrefix+"/tmp/api")
		if err := os.WriteFile(store.CollapsedFile(), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XDG_STATE_HOME", base)

		m := New(config.Defaults())
		t.Cleanup(m.cancel)

		if !m.collapsed["g/s"] {
			t.Error("the persisted collapsed group was not loaded")
		}
		if !m.expanded["/tmp/api"] {
			t.Errorf("the persisted expanded worktree was not loaded: %v", m.expanded)
		}
	})
}

func TestTheEndOfTheFetchDistinguishesHowManyReposSyncs(t *testing.T) {
	cases := []struct {
		name         string
		ok, failed   int
		wantContains string
	}{
		{"only one", 1, 0, "fetch ok"},
		{"several", 3, 0, "fetch ok (3 repos)"},
		{"none and all fail", 0, 2, "2 failed"},
		{"mixed", 2, 1, "2 ok, 1 failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)}, nil)

			// Update returns the new model and THAT is what has to be read: the maps are shared between copies, so a test keeping the old one passes green without having checked anything.
			mm, _ := m.Update(fetchDoneMsg{ok: c.ok, failed: c.failed})
			m = mm.(Model)

			got := lastToast(m)
			if !strings.Contains(got, c.wantContains) {
				t.Errorf("fetch %d ok / %d failed: the notice says %q, want it to contain %q",
					c.ok, c.failed, got, c.wantContains)
			}
		})
	}
}

// Accepting it would drop the current attempt's token and show the user a success for a worktree that may still be there.
func TestTheDeletedOfWorktreeObsoleteNotTouchesTheAttemptCurrent(t *testing.T) {
	const parent = "/tmp/api"
	m := newTestModel(t, []discovery.Project{proj("api", parent, true)}, nil)
	m.removeTokens = map[string]int{parent: 7}
	m.running[parent] = "worktree_remove"

	mm, _ := m.Update(worktreeRemovedMsg{
		parent: parent, wtPath: "/tmp/wt-a", name: "wt-a",
		gen: 3, // old attempt: the current one is the 7
	})
	m = mm.(Model)

	if got := m.removeTokens[parent]; got != 7 {
		t.Errorf("the stale result consumed the live attempt's token: %d, want 7", got)
	}
	if _, still := m.removeTokens[parent]; !still {
		t.Error("the live token vanished: the relaunch can no longer check its result")
	}
	if m.running[parent] != "worktree_remove" {
		t.Errorf("a stale result released another attempt's running: %q", m.running[parent])
	}
	if txt := lastToast(m); strings.Contains(txt, "wt-a") {
		t.Errorf("a stale result painted a notice: %q", txt)
	}
}

// The witness is the search and not the disarm itself (which happens either way), because what distinguishes the two paths is that esc is consumed: two effects from one keystroke is exactly what the armed block forbids.
func TestEscDisarmsTheDeletedAndNotArrivesOnTheRest(t *testing.T) {
	const parent = "/tmp/api"
	esc := tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"}

	t.Run("with a confirmation armed", func(t *testing.T) {
		m := newTestModel(t, []discovery.Project{proj("api", parent, true)}, nil)
		m.armed = &armedRemoval{wtPath: "/tmp/wt-a", parent: parent, name: "wt-a"}
		m.searchActive = true
		m.search = "api"

		mm, _ := m.Update(esc)
		m = mm.(Model)

		if m.armed != nil {
			t.Error("esc did not disarm the confirmation: the app stays waiting for another key")
		}
		if len(m.removeTokens) != 0 {
			t.Errorf("esc recorded a deletion attempt: %v", m.removeTokens)
		}
		if m.running[parent] != "" {
			t.Errorf("esc left the repo running: %q", m.running[parent])
		}
		if !m.searchActive || m.search != "api" {
			t.Errorf("esc followed its course and touched the search: searchActive=%v search=%q; "+
				"the confirmation must consume it", m.searchActive, m.search)
		}
	})

	// The control: with nothing armed, esc does close the search; if this case also passed with the confirmation armed, the previous one would distinguish nothing.
	t.Run("with nothing armed", func(t *testing.T) {
		m := newTestModel(t, []discovery.Project{proj("api", parent, true)}, nil)
		m.searchActive = true
		m.search = "api"

		mm, _ := m.Update(esc)
		m = mm.(Model)

		if m.searchActive {
			t.Error("with nothing armed, esc has to close the search")
		}
		if m.search != "" {
			t.Errorf("esc in the search did not clear the filter: %q", m.search)
		}
	})
}

// An unknown message must not stop the TUI, which is what returning `m, nil` for it achieves; it has to travel the whole switch and leave through the return below.
func TestUpdateIgnoresMessagesUnknown(t *testing.T) {
	m := newTestModel(t, nil, nil)
	out, cmd := m.Update(struct{ tea.Msg }{})
	if out == nil {
		t.Fatal("Update returned nil instead of the model")
	}
	if cmd != nil {
		t.Error("Update returned a command for an unknown message, want nil")
	}
}

// tickMsg is what expires toasts, so if it stopped being emitted a warning would stay painted forever and the table would look frozen.
func TestSpinnerAdvancesWithItsTick(t *testing.T) {
	m := newTestModel(t, nil, nil)
	before := m.spinner.View()
	out, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Error("the spinner's TickMsg returned no command, want the next tick")
	}
	after := out.(Model).spinner.View()
	if after == before && len(before) > 0 {
		t.Errorf("the spinner did not move: %q -> %q", before, after)
	}
}

// The path below opens a shell, so without the warning `!` + enter on a folder with no repo would open a terminal where there is nothing to do.
func TestCommandWithoutRepoWarns(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m = cursorOn(t, m, "/tmp/no-repo-docs")
	m.cmdOpen = true
	m.cmdInput.SetValue("ls")

	out, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter with no repo returned no command, want a notice")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("the command returned %T, want a notice", cmd())
	}
	if !strings.Contains(msg.text, "no git repo") {
		t.Errorf("notice = %q, want it to say there is no repo", msg.text)
	}
	if out.cmdOpen {
		t.Error("the command's input stays open after the notice")
	}
}

func TestCommandWithoutRowNotWarns(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.cmdOpen = true
	m.cmdInput.SetValue("ls")
	out, cmd := press(m, "enter")
	if cmd != nil {
		t.Errorf("enter with no row returned %#v, want nil", cmd())
	}
	if out.cmdOpen {
		t.Error("the input stays open with no row under the cursor")
	}
}

// A fetch of nothing is a command that does not fail and does nothing, and the user would not see why nothing happened.
func TestFetchAllWithoutUpstreamWarns(t *testing.T) {
	m := newTestModel(t, nil, nil)
	_, cmd := press(m, "F")
	if cmd == nil {
		t.Fatal("fetch_all with no repos returned nil, want a notice")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("returned %T, want a notice", cmd())
	}
	if !strings.Contains(msg.text, "no repositories") {
		t.Errorf("notice = %q, want it to say there is nothing to fetch", msg.text)
	}
}

// This is the case that avoids the double workerPool, which would fight over the event channel.
func TestRescanWithScanInRunsWarns(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.scanning = true
	_, cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("rescan with a scan in progress returned nil, want a notice")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("returned %T, want a notice", cmd())
	}
	if !strings.Contains(msg.text, "already running") {
		t.Errorf("warning = %q, want 'scan already running'", msg.text)
	}
}

func TestBusyActionCmd(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.running = map[string]string{"/tmp/api": "pull"}

	if cmd := m.busyActionCmd("/tmp/api"); cmd == nil {
		t.Fatal("busyActionCmd over a busy repo returned nil, want a notice")
	} else if msg, ok := cmd().(notifyMsg); !ok {
		t.Errorf("returned %T, want a notice", cmd())
	} else if !strings.Contains(msg.text, "pull") {
		t.Errorf("notice = %q, want it to name the action in progress", msg.text)
	}

	if cmd := m.busyActionCmd("/tmp/other"); cmd != nil {
		t.Errorf("busyActionCmd over a free repo returned %#v, want nil", cmd())
	}
}

// promptLine() is the ONLY point where keybinds paints a warning, so without this case the prompt of an already cancelled selector would stay painted on top of the hints, taking the line they need.
func TestPromptsArmedIsAreSilentWithoutArm(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if got := m.visualPrompt(); got != "" {
		t.Errorf("visualPrompt with nothing armed = %q, want empty", got)
	}
	if got := m.removePrompt(); got != "" {
		t.Errorf("removePrompt with nothing armed = %q, want empty", got)
	}
	if got := m.promptLine(); got != "" {
		t.Errorf("promptLine with nothing armed = %q, want empty", got)
	}
}

func TestToggleFoldWithoutRowNotIsMoves(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if len(m.entries()) != 0 {
		t.Fatalf("the model with no projects has %d entries", len(m.entries()))
	}
	out, cmd := m.toggleFold()
	if cmd != nil {
		t.Errorf("toggleFold with no row returned %#v, want nil", cmd())
	}
	if got := out.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d after folding with no row, want 0", got)
	}
}

// stderr stays behind the alt screen, so if the warning did not reach the model it would be lost with no trace.
func TestNotifyConfigQueuesTheWarning(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.NotifyConfig("config: the file could not be read")
	if len(m.toasts.toasts) != 1 {
		t.Fatalf("toasts = %d, want 1 (the notice has to be visible, not go to stderr)", len(m.toasts.toasts))
	}
	got := m.toasts.toasts[0]
	if got.level != toastWarning {
		t.Errorf("level = %v, want warning (a config notice is not a success)", got.level)
	}
	if !strings.Contains(got.text, "could not be read") {
		t.Errorf("text = %q, want the whole notice", got.text)
	}
	blocks := m.toasts.blocks()
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	painted := stripANSI(strings.Join(blocks[0], " "))
	if !strings.Contains(painted, "could not be read") {
		t.Errorf("the painted block = %q, want the notice text", painted)
	}
}

func TestSaveCollapsedWithoutStoreNotPanics(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.store = nil
	m.collapsed = map[string]bool{"backend": true}
	m.saveCollapsed() // it must do nothing, and above all not blow up
	if !m.collapsed["backend"] {
		t.Error("the in-memory state changed: saveCollapsed must not touch it")
	}
}

// Without this case an unknown key would translate into an invented git-sim variant.
func TestVisualOptionForKeyUnknown(t *testing.T) {
	for _, key := range []string{"", "x", "enter", "ESC", "mm"} {
		if o, ok := visualOptionForKey(key); ok {
			t.Errorf("visualOptionForKey(%q) = %+v, want no (it is not a variant)", key, o)
		}
	}
	for _, o := range visualOptions {
		got, ok := visualOptionForKey(o.key)
		if !ok {
			t.Errorf("visualOptionForKey(%q) = no, want %+v", o.key, o)
		}
		if got.sub != o.sub {
			t.Errorf("visualOptionForKey(%q).sub = %q, want %q", o.key, got.sub, o.sub)
		}
	}
}

// It costs a second because the timer is real (bubbletea has no injectable clock, and faking it would need the whole tea.Tick seam for one second of suite).
func TestTickCmdEmitsTheTick(t *testing.T) {
	start := time.Now()
	msg := tickCmd()()
	if _, ok := msg.(tickMsg); !ok {
		t.Fatalf("tickCmd()() = %#v, want a tickMsg", msg)
	}
	if d := time.Since(start); d < 900*time.Millisecond {
		t.Errorf("the tick came back in %v, want ~1s (a tick that does not wait expires nothing)", d)
	}
}

func TestInitStartsThePipeline(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	if cmd := m.Init(); cmd == nil {
		t.Error("Init = nil, want a Cmd: without scan, pump and tick the app does not start")
	}
}

// With the channel closed it must return nil and not an empty event: nil is what the app tells apart from "there is work", and a non-nil value on a closed channel would keep it repainting forever.
func TestWaitForEvent(t *testing.T) {
	t.Run("it delivers the event and rearms", func(t *testing.T) {
		ch := make(chan event, 1)
		ch <- tickMsg{}
		if ev := waitForEvent(ch)(); ev == nil {
			t.Fatal("waitForEvent returned no pending event")
		}
		ch <- tickMsg{}
		if ev := waitForEvent(ch)(); ev == nil {
			t.Error("the pump does not rearm: the second event never arrives")
		}
	})

	t.Run("a closed channel returns nil", func(t *testing.T) {
		ch := make(chan event)
		close(ch)
		if ev := waitForEvent(ch)(); ev != nil {
			t.Errorf("waitForEvent with the channel closed = %#v, want nil", ev)
		}
	})
}

// The other tests inject it by hand, so without this the line that emits it (and with it the real end of the scan) is exercised by nobody: the end would look right in tests and never arrive in the app.
func TestTheScanEmitsCollectDoneOnTheFinish(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "api")
	testutil.Init(t, repo)
	testutil.Marker(t, repo, "api", "", "", false)
	testutil.CommitFiles(t, repo, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")

	cfg := config.Defaults()
	cfg.Roots = []string{root}
	m := New(cfg)
	t.Cleanup(m.cancel)

	m.startScanCmd()
	if !awaitEvent(t, m, 20*time.Second, func(ev event) bool {
		_, ok := ev.(collectDoneMsg)
		return ok
	}) {
		t.Fatal("the scan emitted no collectDoneMsg: the pipeline never closes")
	}
}

// The three filters are the reason the automatic fetch does not hit repos with no upstream, with no repo, or one already in flight: the first two are noise in the git log and the third would mean two `git fetch` in parallel on the same repo.
func TestFetchTargetsOnlyTheReposItShouldFetch(t *testing.T) {
	withRepo := "/tmp/with-repo"
	withoutRepo := "/tmp/no-repo"
	inFlight := "/tmp/in-flight"
	m := newTestModel(t, []discovery.Project{
		proj("with-repo", withRepo, true),
		proj("no-repo", withoutRepo, false),
		proj("in-flight", inFlight, true),
	}, map[string]gitstatus.Snapshot{
		withRepo:    snapClean(),
		withoutRepo: snapClean(),
		inFlight:    snapClean(),
	})
	m.fetchStates[inFlight] = "fetching"

	got := m.fetchTargets()
	if len(got) != 1 || got[0] != withRepo {
		t.Errorf("fetchTargets = %v, want only [%s]", got, withRepo)
	}
}

func TestFetchTargetsJumpsTheRepoWithError(t *testing.T) {
	broken := "/tmp/broken"
	good := "/tmp/good"
	m := newTestModel(t, []discovery.Project{
		proj("broken", broken, true), proj("good", good, true),
	}, map[string]gitstatus.Snapshot{
		broken: {Err: "no such repository"},
		good:   snapClean(),
	})
	got := m.fetchTargets()
	if len(got) != 1 || got[0] != good {
		t.Errorf("fetchTargets = %v, want only [%s]", got, good)
	}
}

// The state left behind is what is checked, not the returned Cmd (startScanCmd always returns nil and publishes through the channel): without `scanning` the app would believe nothing is in flight and the second rescan would slip in without warning.
func TestRescanWhenNotThereScanInCourse(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m.scanning = false
	m, _ = press(m, "r")
	if !m.scanning {
		t.Error("rescan did not mark the scan as running")
	}
}

// The "cancelled while waiting for a slot" branch is not decorative: a repo that never ran must count neither as ok nor failed, or fetch all over 20 repos closes with "1 ok, 19 failed" after a plain Ctrl-C.
func TestAcquireSlot(t *testing.T) {
	t.Run("with room it takes it", func(t *testing.T) {
		sem := make(chan struct{}, 2)
		if !acquireSlot(context.Background(), sem) {
			t.Error("acquireSlot = false with a free slot, want true")
		}
		if len(sem) != 1 {
			t.Errorf("the slot was not taken: len(sem) = %d, want 1", len(sem))
		}
	})

	t.Run("cancelled with no slot it does not take it", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		sem <- struct{}{} // taken: the next one has to wait
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if acquireSlot(ctx, sem) {
			t.Error("acquireSlot = true with the semaphore full and the context cancelled, want false")
		}
		if len(sem) != 1 {
			t.Errorf("it took a slot that was not its own: len(sem) = %d, want 1", len(sem))
		}
	})

	// A badly written select (prioritizing ctx.Done) would slip through here and return false for no reason.
	t.Run("it waits for a slot to free up", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		sem <- struct{}{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-sem // released the way the previous fetch would
		}()
		if !acquireSlot(ctx, sem) {
			t.Error("acquireSlot = false while waiting for a slot that was freeing, want true")
		}
	})
}

// With concurrency=1 the first fetch holds the only slot for 30s; `exec` and not `sleep & wait` because a grandchild keeps stdout's pipe and Output() never returns (measured), and `shimFlag` is the shim's only signal that the slot is taken since recordExec runs when the process ENDS.
func waitForSlowGit(t *testing.T, shimFlag string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"fetch\" ]; then touch \"" + shimFlag + "\"; exec sleep 30; fi\n" +
		"exec " + real + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func awaitInShim(t *testing.T, shimFlag string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(shimFlag); err == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("no fetch got to start in %s", timeout)
}

const queuedFetches = 12

// Only the command log is watched: with the context already cancelled the shim writes whether or not the guard works, and the fetchState/fetchDone events deliver half the time (981/2000), a 50% false negative worse than no test; rec.Entries() works because recordExec runs from runGit either way.
func TestFetchBatchTheCancelledInWaitsNotRunGit(t *testing.T) {
	shimFlag := filepath.Join(t.TempDir(), "inside")
	waitForSlowGit(t, shimFlag)

	paths := make([]string, queuedFetches)
	projects := make([]discovery.Project, queuedFetches)
	states := make(map[string]gitstatus.Snapshot, queuedFetches)
	for i := range paths {
		// The dirs have to EXIST: with a missing `cmd.Dir` Start fails before running the shim and the test would wait for an entry that never comes.
		paths[i] = t.TempDir()
		projects[i] = proj(fmt.Sprintf("repo-%02d", i), paths[i], true)
		states[paths[i]] = snapClean()
	}
	m := newTestModel(t, projects, states)
	rec := cmdlog.Active()
	t.Cleanup(func() { cmdlog.SetRecorder(nil) }) // the next test's log
	m.cfg.FetchConcurrency = 1                    // a single slot: the rest HAVE to wait

	m.fetchBatchCmd(paths, cmdlog.ClassAction)

	awaitInShim(t, shimFlag, 10*time.Second)

	m.cancel()

	time.Sleep(time.Second)

	var ran []string
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "fetch" {
			continue
		}
		ran = append(ran, e.Dir)
	}
	if len(ran) > 1 {
		t.Errorf("fetches that went out to a subprocess = %v, want only the first: %d of %d queued got to run despite the cancellation",
			ran, len(ran)-1, queuedFetches-1)
	}
}
