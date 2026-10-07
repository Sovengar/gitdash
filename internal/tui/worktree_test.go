package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
	"gitdash/internal/testutil"
)

func TestWorktreeHidden(t *testing.T) {
	main := proj("multi-wt", "/tmp/multi-wt", true)
	wt := discovery.Project{
		Path: "/tmp/wt-feat", Name: "wt-feat", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi-wt",
	}
	orphan := discovery.Project{
		Path: "/tmp/orphan", Name: "orphan", HasRepo: true,
		IsWorktree: true, MainRepo: "/no-descubierto",
	}
	mainSnap := snapClean()
	mainSnap.Worktrees = []gitstatus.Worktree{
		{Path: "/tmp/wt-feat", Branch: "feat"},
		{Path: "/tmp/wt-x", Branch: "x"},
	}
	projects := []discovery.Project{main, wt, orphan}
	states := map[string]gitstatus.Snapshot{
		"/tmp/multi-wt": mainSnap,
		"/tmp/wt-feat":  snapClean(),
		"/tmp/orphan":   snapClean(),
	}
	m := newTestModel(t, projects, states)

	rows := m.rows()
	names := strings.Join(rowNames(rows), ",")
	if strings.Contains(names, "wt-feat") {
		t.Errorf("worktree descubierto visible: %v", rowNames(rows))
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want [multi-wt orphan]", rowNames(rows))
	}
	m2 := newTestModel(t, []discovery.Project{main}, states)
	r := m2.rows()[0]
	text, _ := m2.nameCell(r)
	if !strings.Contains(text, "(2 wt)") {
		t.Errorf("indicador = %q, want '(2 wt)'", text)
	}
}

func TestDetailWorktrees(t *testing.T) {
	main := proj("multi-wt", "/tmp/multi-wt", true)
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		{Path: "/tmp/multi-wt/wt-feat", Branch: "feat", Head: "abc1234"},
	}
	m := newTestModel(t, []discovery.Project{main}, map[string]gitstatus.Snapshot{"/tmp/multi-wt": s})
	m.cursor = 0
	r, _ := m.selected()
	out := stripANSI(m.renderDetail(r, m.height))
	if !strings.Contains(out, "worktrees (1)") || !strings.Contains(out, "feat") ||
		!strings.Contains(out, "abc1234") {
		t.Errorf("detail without worktrees:\n%s", out)
	}
}

func TestSyncCell(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	behind := snapClean()
	behind.SyncBranch, behind.SyncBehind, behind.SyncKnown = "main", 3, true
	inSync := snapClean()
	inSync.SyncBranch, inSync.SyncKnown = "main", true
	missingRef := snapClean()
	missingRef.SyncBranch = "main"
	noBranch := snapClean()

	cases := []struct {
		name string
		snap gitstatus.Snapshot
		want string
	}{
		{"behind", behind, "main ↓3"},
		{"en sync", inSync, "main"},
		{"ref missing", missingRef, "main —"},
		{"no branch", noBranch, "—"},
	}
	for _, tc := range cases {
		m := newTestModel(t, []discovery.Project{base},
			map[string]gitstatus.Snapshot{"/tmp/api": tc.snap})
		if text, _ := m.syncCell(m.rows()[0]); text != tc.want {
			t.Errorf("%s: sync = %q, want %q", tc.name, text, tc.want)
		}
	}

	develop := snapClean()
	develop.SyncBranch, develop.SyncBehind, develop.SyncKnown = "develop", 2, true
	ovr := proj("api", "/tmp/api", true)
	ovr.SyncBranch = "develop"
	m := newTestModel(t, []discovery.Project{ovr},
		map[string]gitstatus.Snapshot{"/tmp/api": develop})
	if text, _ := m.syncCell(m.rows()[0]); text != "develop ↓2" {
		t.Errorf("sync = %q, want 'develop ↓2'", text)
	}
}

func wt(path, branch string) gitstatus.Worktree {
	return gitstatus.Worktree{Path: path, Branch: branch, Head: "abc1234"}
}

func repoWithWorktrees(name, path string, wts ...gitstatus.Worktree) (discovery.Project, map[string]gitstatus.Snapshot) {
	p := proj(name, path, true)
	s := snapClean()
	s.Worktrees = wts
	return p, map[string]gitstatus.Snapshot{path: s}
}

func worktreeNames(entries []tableEntry) []string {
	var out []string
	for _, e := range entries {
		if e.kind == kindWorktree {
			out = append(out, filepath.Base(e.wt.Path))
		}
	}
	return out
}

func TestWorktreeCollapsedByDefault(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)

	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Fatalf("subrows without expanding: %v", got)
	}
	text, _ := m.nameCell(m.rows()[0])
	if !strings.Contains(text, "▸") || !strings.Contains(text, "(2 wt)") {
		t.Errorf("NAME = %q, want ▸ y (2 wt)", text)
	}
	if strings.Contains(stripANSI(m.View().Content), "↳") {
		t.Error("there must be no subrows in the view")
	}
}

func TestWorktreeToggleWithEnter(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)

	m, _ = press(m, "enter")
	if !m.expanded[p.Path] {
		t.Fatal("enter did not expand")
	}
	if got := worktreeNames(m.entries()); len(got) != 2 {
		t.Fatalf("subrows = %v, want 2", got)
	}
	if text, _ := m.nameCell(m.rows()[0]); !strings.Contains(text, "▾") {
		t.Errorf("glyph = %q, want ▾", text)
	}

	m, _ = press(m, "enter")
	if m.expanded[p.Path] {
		t.Error("enter did not fold back")
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("subrows after folding = %v", got)
	}
	if text, _ := m.nameCell(m.rows()[0]); !strings.Contains(text, "▸") {
		t.Errorf("glyph = %q, want ▸", text)
	}
}

func TestWorktreeToggleNotRepoWts(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m, _ = press(m, "enter")
	if len(m.expanded) != 0 {
		t.Errorf("expansion changed in a repo with no worktrees: %v", m.expanded)
	}
}

func TestWorktreeToggleOnHeaderFoldsGroup(t *testing.T) {
	p := discovery.Project{Path: "/a", Name: "a", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/a": s})
	if m.entries()[0].kind != kindPrimary {
		t.Fatalf("precondition: cursor not on a header: %+v", m.entries()[0])
	}
	m, _ = press(m, "enter")
	if !m.collapsed["g"] {
		t.Errorf("the header did not fold its group: collapsed=%v", m.collapsed)
	}
	if len(m.expanded) != 0 {
		t.Errorf("the header should not have touched the expansion: %v", m.expanded)
	}
}

func TestWorktreeToggleOnSubrow(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	if e, _ := m.selectedEntry(); e.kind != kindWorktree {
		t.Fatalf("cursor not on a subrow: %+v", e)
	}
	before := m.expanded[p.Path]
	m, _ = press(m, "enter")
	if m.expanded[p.Path] != before {
		t.Errorf("the parent's expansion changed: %v", m.expanded)
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("the subrow disappeared: %v", got)
	}
}

func TestWorktreeCoverageAndDedupe(t *testing.T) {
	main := proj("multi", "/tmp/multi", true)
	disc := discovery.Project{
		Path: "/tmp/wt-marcado", Name: "wt-marcado", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi",
	}
	mainSnap := snapClean()
	mainSnap.Worktrees = []gitstatus.Worktree{
		wt("/tmp/wt-marcado", "feat/marcado"),
		wt("/tmp/wt-root", "feat/root"),
		wt("/outside/wt-outside", "feat/outside"),
	}
	m := newTestModel(t, []discovery.Project{main, disc},
		map[string]gitstatus.Snapshot{"/tmp/multi": mainSnap, "/tmp/wt-marcado": snapClean()})

	m, _ = press(m, "enter")
	got := worktreeNames(m.entries())
	if len(got) != 3 {
		t.Fatalf("subrows = %v, want 3", got)
	}
	for _, e := range m.entries() {
		if e.kind == kindWorktree && filepath.Clean(e.wt.Path) == "/tmp/multi" {
			t.Error("the main repo appears as a subrow")
		}
	}
	for _, e := range m.entries() {
		if e.kind == kindRepo && e.r.project.Path == "/tmp/wt-marcado" {
			t.Error("the discovered worktree appears as a repo row")
		}
	}
	n := 0
	for _, name := range got {
		if name == "wt-marcado" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the discovered worktree appears %d times as a subrow, want 1", n)
	}
}

func TestWorktreeDedupeNormalizedPathMEDIUM2(t *testing.T) {
	main := proj("multi", "/tmp/multi", true)
	disc := discovery.Project{
		Path: "/tmp/multi/wt-feat/", Name: "wt-feat", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi/.",
	}
	mainSnap := snapClean()
	mainSnap.Worktrees = []gitstatus.Worktree{wt("/tmp/multi/wt-feat", "feat")}
	discSnap := snapDirty(3, 0)
	m := newTestModel(t, []discovery.Project{main, disc},
		map[string]gitstatus.Snapshot{"/tmp/multi": mainSnap, "/tmp/multi/wt-feat/": discSnap})

	for _, r := range m.rows() {
		if filepath.Clean(r.project.Path) == "/tmp/multi/wt-feat" {
			t.Errorf("MEDIUM-2: the discovered worktree visible as top-level: %q", r.project.Path)
		}
	}
	m, _ = press(m, "enter")
	got := worktreeNames(m.entries())
	if len(got) != 1 || got[0] != "wt-feat" {
		t.Fatalf("MEDIUM-2: subrows = %v, want [wt-feat]", got)
	}
	var wtEntry tableEntry
	for _, e := range m.entries() {
		if e.kind == kindWorktree {
			wtEntry = e
		}
	}
	r := m.worktreeRow(wtEntry.wt)
	if r.project.Name != "wt-feat" || r.snap.Status.TrackedChanges != 3 {
		t.Errorf("MEDIUM-2: the detail does not reuse the discovered snapshot: name=%q tracked=%d",
			r.project.Name, r.snap.Status.TrackedChanges)
	}
}

// Even if its snapshot has worktrees (realistic: `git worktree list` from the worktree itself includes the main one), an orphan must show neither glyph nor counter (a dead affordance).
func TestWorktreeOrphan(t *testing.T) {
	orphan := discovery.Project{
		Path: "/tmp/orphan", Name: "orphan", HasRepo: true,
		IsWorktree: true, MainRepo: "/no-descubierto",
	}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		wt("/tmp/orphan", "topic"),
		wt("/no-descubierto", "main"),
	}
	m := newTestModel(t, []discovery.Project{orphan},
		map[string]gitstatus.Snapshot{"/tmp/orphan": s})

	text, _ := m.nameCell(m.rows()[0])
	if !strings.Contains(text, "[wt]") {
		t.Errorf("orphan without the [wt] tag: %q", text)
	}
	if strings.Contains(text, "wt)") || strings.Contains(text, "▸") || strings.Contains(text, "▾") {
		t.Errorf("HIGH-1: orphan with glyph/counter (dead affordance): %q", text)
	}
	m, _ = press(m, "enter")
	if len(m.expanded) != 0 {
		t.Errorf("orphan expandable: %v", m.expanded)
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("subrows in an orphan: %v", got)
	}
}

func TestWorktreeNotGlyphS3156(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	text, _ := m.nameCell(m.rows()[0])
	if strings.Contains(text, "wt)") || strings.Contains(text, "▸") || strings.Contains(text, "▾") {
		t.Errorf("NAME with glyph/counter and no worktrees: %q", text)
	}
	m, _ = press(m, "enter")
	if len(m.expanded) != 0 {
		t.Errorf("space expanded a snapshot with no worktrees: %v", m.expanded)
	}
}

func TestWorktreeCursor(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree || filepath.Base(e.wt.Path) != "wt-a" {
		t.Fatalf("cursor = %+v", e)
	}
	r, ok := m.selected()
	if !ok || r.project.Path != "/tmp/wt-a" {
		t.Errorf("selected() = %+v, want the worktree's path", r.project)
	}
}

func TestWorktreeScroll(t *testing.T) {
	var projects []discovery.Project
	states := map[string]gitstatus.Snapshot{}
	for i := 0; i < 12; i++ {
		p := proj(fmt.Sprintf("repo%02d", i), fmt.Sprintf("/tmp/repo%02d", i), true)
		projects = append(projects, p)
		states[p.Path] = snapClean()
	}
	last, lastSt := repoWithWorktrees("zzz-wt", "/tmp/zzz-wt", wt("/tmp/wt-a", "a"))
	projects = append(projects, last)
	for k, v := range lastSt {
		states[k] = v
	}
	m := newTestModel(t, projects, states)
	m.height = 8
	m.cursor = len(m.entries()) - 1
	m, _ = press(m, "enter")
	m, _ = press(m, "down")

	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "wt-a") {
		t.Errorf("the subrow is not visible after the scroll:\n%s", out)
	}
}

// The `d` filter is used because the repo is clean and stops passing it, since `space` is a no-op on a sub-row and there is no user path to fold the parent from there.
func TestWorktreeFoldKeepsCursor(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	m, _ = press(m, "down")
	if m.cursor != 2 {
		t.Fatalf("precondition: cursor = %d, want 2", m.cursor)
	}

	m, _ = press(m, "d")
	entries := m.entries()
	if len(entries) == 0 {
		if m.cursor != 0 {
			t.Errorf("cursor = %d with an empty table, want 0", m.cursor)
		}
	} else if m.cursor < 0 || m.cursor >= len(entries) {
		t.Errorf("cursor out of range: %d (entries=%d)", m.cursor, len(entries))
	}

	m, _ = press(m, "d")
	if got := worktreeNames(m.entries()); len(got) != 2 {
		t.Errorf("the repo did not stay expanded on revert: %v", got)
	}
}

func TestWorktreeActionsPathS3324(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	for _, tc := range []struct {
		keys []string
		kind string
	}{
		{[]string{"p", "p"}, "pull"},
		{[]string{"p", "r"}, "pull_rebase"},
		{[]string{"p", "m"}, "pull_merge"},
		{[]string{"P"}, "push"},
		{[]string{"R"}, "collect"},
	} {
		m := newTestModel(t, []discovery.Project{p}, st)
		m, _ = press(m, "enter")
		m, _ = press(m, "down")
		for _, k := range tc.keys {
			m, _ = press(m, k)
		}
		if m.running["/tmp/wt-a"] != tc.kind {
			t.Errorf("%v in running = %q, want %q", tc.keys, m.running["/tmp/wt-a"], tc.kind)
		}
	}
}

func TestWorktreeFetchPath(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	r, ok := m.selected()
	if !ok || r.project.Path != "/tmp/wt-a" || !r.project.HasRepo {
		t.Fatalf("fetch would operate on %+v, want the worktree", r.project)
	}

	m, _ = press(m, "f")
	deadline := time.After(30 * time.Second)
	visto := false
	for {
		select {
		case ev := <-m.events:
			switch e := ev.(type) {
			case fetchStateMsg:
				if filepath.Clean(e.path) == "/tmp/wt-a" && e.state == "fetching" && !visto {
					visto = true
				}
			case fetchDoneMsg:
				// The batch ends here: without waiting for fetchDoneMsg the test leaves the goroutine alive and its git reads land in the NEXT test's GLOBAL recorder, failing it for what this one did.
				if !visto {
					t.Error("no fetchStateMsg fetching observed for /tmp/wt-a")
				}
				return
			}
		case <-deadline:
			t.Fatal("the fetch did not finish (no fetchDoneMsg observed)")
		}
	}
}

func TestWorktreeHandoffPath(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")

	me, cmd := press(m, "e")
	if cmd == nil {
		t.Error("editor returned no tea.Cmd")
	}
	if r, _ := me.selected(); r.project.Path != "/tmp/wt-a" {
		t.Errorf("editor path = %q", r.project.Path)
	}

	mp, _ := press(m, "p")
	if mp.pullArmed == nil || mp.pullArmed.path != "/tmp/wt-a" {
		t.Errorf("pull armed = %+v, want the worktree's path", mp.pullArmed)
	}

	// A stub in the PATH so LookPath also passes on a runner without lazygit: with a skip here the worktree sub-row never exercised the `g` key in CI.
	withFakeLazygit(t)
	mg := newTestModel(t, []discovery.Project{p}, st)
	mg, _ = press(mg, "enter")
	mg, _ = press(mg, "down")
	mg, _ = press(mg, "g")
	if mg.running["/tmp/wt-a"] != "lazygit" {
		t.Errorf("lazygit running = %q", mg.running["/tmp/wt-a"])
	}
}

func TestWorktreeBang(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/gitdash-test/wt", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	m, _ = press(m, "!")
	if !m.cmdOpen {
		t.Fatal("! did not open the input")
	}
	m, _ = press(m, "g")
	m, _ = press(m, "enter")
	if m.running["/tmp/gitdash-test/wt"] != "cmd" {
		t.Errorf("running = %q, want a cmd over the worktree", m.running["/tmp/gitdash-test/wt"])
	}
}

func TestWorktreeDetail(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi",
		gitstatus.Worktree{Path: "/tmp/wt-detached", Branch: "", Head: "abc1234"})
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")

	out := stripANSI(m.View().Content)
	for _, want := range []string{"/tmp/wt-detached", "(detached)", "abc1234"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail without %q:\n%s", want, out)
		}
	}
	// No derived git state is invented for the worktree: the minimal panel does not have it (the stats section, which does mention global ahead/behind, is a different thing).
	e, ok := m.selectedEntry()
	if !ok {
		t.Fatal("no selected entry")
	}
	panel := stripANSI(m.renderWorktreeDetail(e, m.height))
	for _, bad := range []string{"no-up", "clean", "ahead", "behind"} {
		if strings.Contains(panel, bad) {
			t.Errorf("the detail invents state %q:\n%s", bad, panel)
		}
	}
}

func TestWorktreeDetailDiscovered(t *testing.T) {
	main, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-marcado", "feat"))
	disc := discovery.Project{
		Path: "/tmp/wt-marcado", Name: "wt-marcado", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi",
	}
	st["/tmp/wt-marcado"] = snapDirty(2, 1)
	m := newTestModel(t, []discovery.Project{main, disc}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")

	e, _ := m.selectedEntry()
	out := stripANSI(m.renderWorktreeDetail(e, m.height))
	if !strings.Contains(out, "wt-marcado") || !strings.Contains(out, "state") {
		t.Errorf("detail of worktree with incomplete discovery:\n%s", out)
	}
	if !strings.Contains(out, "2 ?1") {
		t.Errorf("the detail does not reflect the live snapshot:\n%s", out)
	}
	if strings.Contains(out, "head    ") {
		t.Errorf("it used the minimal panel instead of the live snapshot:\n%s", out)
	}
}

func TestWorktreeNotificationBasename(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if got := m.nameOf("/tmp/projects/wt-feat-x"); got != "wt-feat-x" {
		t.Errorf("nameOf = %q, want basename", got)
	}
}

func TestWorktreeRowRenderS34124(t *testing.T) {
	main := discovery.Project{Path: "/tmp/multi", Name: "multi", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-featx", "feat/x")}
	m := newTestModel(t, []discovery.Project{main}, map[string]gitstatus.Snapshot{"/tmp/multi": s})
	m, _ = press(m, "down")
	m, _ = press(m, "enter")

	var repoLine, headerLine, wtLine string
	for _, e := range m.entries() {
		switch e.kind {
		case kindPrimary:
			headerLine = stripANSI(m.renderEntry(e, false))
		case kindRepo:
			repoLine = stripANSI(m.renderEntry(e, false))
		case kindWorktree:
			wtLine = stripANSI(m.renderEntry(e, false))
		}
	}
	if wtLine == "" {
		t.Fatal("there is no rendered subrow")
	}

	for _, bad := range []string{"no-up", "error", "clean", "↑", "↓"} {
		if strings.Contains(wtLine, bad) {
			t.Errorf("the subrow invents state %q: %q", bad, wtLine)
		}
	}
	if !strings.Contains(wtLine, "↳") || !strings.Contains(wtLine, "wt-featx") {
		t.Errorf("indistinguishable subrow: %q", wtLine)
	}
	if !strings.Contains(wtLine, "feat/x") {
		t.Errorf("BRANCH without the worktree's branch: %q", wtLine)
	}

	if wtLine == repoLine || wtLine == headerLine {
		t.Errorf("subrow identical to repo/header\n repo=%q\n header=%q\n wt=%q", repoLine, headerLine, wtLine)
	}
	if strings.Contains(repoLine, "↳") || strings.Contains(headerLine, "↳") {
		t.Errorf("filtered subrow glyph went to another row: repo=%q header=%q", repoLine, headerLine)
	}

	if !strings.Contains(stripANSI(m.View().Content), "↳") {
		t.Error("the subrow does not appear in the view")
	}
}

func TestWorktreeRowDetached(t *testing.T) {
	w := gitstatus.Worktree{Path: "/tmp/wt-det", Branch: "", Head: "abc1234"}
	m := newTestModel(t, nil, nil)
	line := stripANSI(m.renderWorktreeRow(w, false))
	if !strings.Contains(line, "(detached)") || !strings.Contains(line, "abc1234") {
		t.Errorf("detached subrow = %q", line)
	}
}

func TestWorktreeGlyphCounter(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	if text, _ := m.nameCell(m.rows()[0]); !strings.Contains(text, "▸ (2 wt)") {
		t.Errorf("plegado = %q, want ▸ (2 wt)", text)
	}
	m.expanded[p.Path] = true
	if text, _ := m.nameCell(m.rows()[0]); !strings.Contains(text, "▾ (2 wt)") {
		t.Errorf("expandido = %q, want ▾ (2 wt)", text)
	}
}

func TestWorktreeExpansionPersistsS3512(t *testing.T) {
	dir := t.TempDir()
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))

	fresh := newTestModel(t, []discovery.Project{p}, st)
	fresh.loadPersisted(nil)
	if len(worktreeNames(fresh.entries())) != 0 || fresh.expanded[p.Path] {
		t.Error("the repo does not appear folded by default")
	}

	m := newTestModel(t, []discovery.Project{p}, st)
	m.store = state.NewStoreAt(dir)
	m, _ = press(m, "enter")
	if !m.expanded[p.Path] {
		t.Fatal("enter did not expand")
	}

	m2 := newTestModel(t, []discovery.Project{p}, st)
	m2.store = state.NewStoreAt(dir)
	m2.loadPersisted(m2.store.LoadCollapsed())
	if !m2.expanded[p.Path] {
		t.Error("the expansion did not survive the restart")
	}
	if got := worktreeNames(m2.entries()); len(got) != 2 {
		t.Errorf("subrows after restart = %v, want 2", got)
	}
}

func TestWorktreePersistNotCollision(t *testing.T) {
	dir := t.TempDir()
	if err := state.NewStoreAt(dir).SaveCollapsed(map[string]bool{"backend": true}); err != nil {
		t.Fatal(err)
	}
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.store = state.NewStoreAt(dir)
	m.loadPersisted(m.store.LoadCollapsed())
	if !m.collapsed["backend"] {
		t.Fatal("the existing group key was not loaded")
	}
	m, _ = press(m, "enter")

	loaded := state.NewStoreAt(dir).LoadCollapsed()
	if !loaded["backend"] {
		t.Error("the existing group key was lost")
	}
	if loaded[state.WorktreePrefix+p.Path] != true {
		t.Errorf("expansion missing from its own namespace: %v", loaded)
	}
}

func TestWorktreePersistCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, state.FileName), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.store = state.NewStoreAt(dir)
	m.loadPersisted(m.store.LoadCollapsed())
	if len(m.expanded) != 0 {
		t.Errorf("expansion with a corrupt file: %v", m.expanded)
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("subrows with a corrupt file: %v", got)
	}
}

func TestWorktreeDirtyFilter(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.expanded[p.Path] = true
	m.onlyDirty = true
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("subrows visible under the dirty filter: %v", got)
	}
}

func TestWorktreeGroupFoldS3623(t *testing.T) {
	p := discovery.Project{Path: "/a", Name: "a", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/a": s})

	m, _ = press(m, "down")
	m, _ = press(m, "enter")
	if len(worktreeNames(m.entries())) != 1 {
		t.Fatal("precondition: repo not expanded")
	}
	m, _ = press(m, "home")
	m, _ = press(m, "enter")
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("subrows visible with the group folded: %v", got)
	}
	m, _ = press(m, "enter")
	if !m.expanded["/a"] {
		t.Error("the repo's expansion was lost when toggling the group")
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("subrows after expanding = %v, want 1", got)
	}
}

func TestWorktreeKeybindingS3712(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))

	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	if !m.expanded[p.Path] {
		t.Error("enter did not expand with the default bindings")
	}

	m2 := newTestModel(t, []discovery.Project{p}, st)
	m2.cfg.Keybindings["fold"] = "w"
	m2, _ = press(m2, "w")
	if !m2.expanded[p.Path] {
		t.Fatal("w did not expand after the rebind")
	}
	m3 := newTestModel(t, []discovery.Project{p}, st)
	m3.cfg.Keybindings["fold"] = "w"
	m3, _ = press(m3, "enter")
	if m3.expanded[p.Path] {
		t.Error("enter still folds after the rebind")
	}
}

func TestWorktreeHint(t *testing.T) {
	hints := strings.Join(config.Defaults().HintBarLines(), "\n")
	if !strings.Contains(hints, "enter fold") {
		t.Errorf("fold hint missing: %v", hints)
	}
	if strings.Contains(hints, "detail") || strings.Contains(hints, "expand") {
		t.Errorf("hints of what no longer exists are left: %v", hints)
	}
	cfg := config.Defaults()
	cfg.Keybindings["fold"] = "w"
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "w fold") {
		t.Errorf("hint without the rebound key: %v", cfg.HintBarLines())
	}
}

func TestWorktreeEnterOnSubrowNotFoldsGroup(t *testing.T) {
	p := discovery.Project{Path: "/a", Name: "a", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/a": s})
	m, _ = press(m, "down")
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	if e, _ := m.selectedEntry(); e.kind != kindWorktree {
		t.Fatalf("cursor not on a subrow: %+v", e)
	}
	before := len(m.collapsed)
	m, _ = press(m, "enter")
	if len(m.collapsed) != before {
		t.Errorf("tab on a subrow changed the folding: %v", m.collapsed)
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("the subrow disappeared: %v", got)
	}
}

func TestWorktreeSearchMatchS40123(t *testing.T) {
	p := proj("alpha", "/tmp/alpha", true)
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		wt("/tmp/wt-feature", "feat/x"),
		wt("/tmp/wt-other", "chore/y"),
		wt("/tmp/odd-name", "chore/z"),
	}
	states := map[string]gitstatus.Snapshot{"/tmp/alpha": s}

	m := newTestModel(t, []discovery.Project{p}, states)
	m.search = "feat/x"
	if len(m.rows()) != 1 {
		t.Fatalf("the parent is not visible: %v", rowNames(m.rows()))
	}
	got := worktreeNames(m.entries())
	if len(got) != 1 || got[0] != "wt-feature" {
		t.Errorf("subrows = %v, want [wt-feature]", got)
	}

	m2 := newTestModel(t, []discovery.Project{p}, states)
	m2.search = "odd-name"
	if got := worktreeNames(m2.entries()); len(got) != 1 || got[0] != "odd-name" {
		t.Errorf("subrows = %v, want [odd-name]", got)
	}
}

func TestWorktreeSearchNotMatch(t *testing.T) {
	p, st := repoWithWorktrees("alpha", "/tmp/alpha", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.search = "zzzz-nonexistent"
	if got := m.entries(); len(got) != 0 {
		t.Errorf("entries = %v, want empty", got)
	}
}

func TestWorktreeSearchTransient(t *testing.T) {
	p, st := repoWithWorktrees("alpha", "/tmp/alpha", wt("/tmp/wt-feature", "feat/x"))

	m := newTestModel(t, []discovery.Project{p}, st)
	m.search = "feat/x"
	if _, ok := m.expanded[p.Path]; ok {
		t.Error("the search wrote the persisted state")
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Fatalf("subrow not visible with the search active: %v", got)
	}
	m.search = ""
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("the repo did not return to folded after clearing the search: %v", got)
	}
	if _, ok := m.expanded[p.Path]; ok {
		t.Error("the persisted state got contaminated")
	}
}

func TestWorktreeExpandRealFixture(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtFeat := filepath.Join(t.TempDir(), "wt-feat")
	testutil.MakeWorktree(t, dir, wtFeat, "feat/x")
	wtDetached := filepath.Join(t.TempDir(), "wt-detached")
	testutil.MakeWorktree(t, dir, wtDetached, "other")
	testutil.Detach(t, wtDetached)

	snap := gitstatus.Collect(t.Context(), dir, "main", false)
	if len(snap.Worktrees) != 2 {
		t.Fatalf("fixture: worktrees = %d, want 2", len(snap.Worktrees))
	}
	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})

	m, _ = press(m, "enter")
	entries := m.entries()
	if got := worktreeNames(entries); len(got) != 2 {
		t.Fatalf("real subrows = %v, want 2", got)
	}
	var featLine string
	for _, e := range entries {
		if e.kind == kindWorktree && filepath.Base(e.wt.Path) == "wt-feat" {
			featLine = stripANSI(m.renderWorktreeRow(e.wt, false))
		}
	}
	if !strings.Contains(featLine, "feat/x") {
		t.Errorf("the real branch is not visible: %q", featLine)
	}
	var detLine string
	for _, e := range entries {
		if e.kind == kindWorktree && filepath.Base(e.wt.Path) == "wt-detached" {
			detLine = stripANSI(m.renderWorktreeRow(e.wt, false))
		}
	}
	if !strings.Contains(detLine, "(detached)") {
		t.Errorf("the real detached is not visible: %q", detLine)
	}
}
