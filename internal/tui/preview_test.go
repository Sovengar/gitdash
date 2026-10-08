package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

func previewModel(t *testing.T) Model {
	t.Helper()
	projects := []discovery.Project{
		{Path: "/tmp/api", Name: "api", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/tmp/web", Name: "web", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/tmp/cli", Name: "cli", PrimaryGroup: "vsocial", HasRepo: true},
		proj("loose", "/tmp/loose", true),
	}
	api := snapDirty(2, 1)
	api.Files = []gitstatus.FileEntry{
		{Code: ".M", Path: "main.go"},
		{Code: "??", Path: "notes.md"},
	}
	api.SyncBranch, api.SyncKnown = "main", true
	states := map[string]gitstatus.Snapshot{
		"/tmp/api":   api,
		"/tmp/web":   snapBehind(3),
		"/tmp/cli":   snapClean(),
		"/tmp/loose": snapClean(),
	}
	return newTestModel(t, projects, states)
}

func TestPreviewShowsTheRepoOfTheCursor(t *testing.T) {
	m := cursorOn(t, previewModel(t), "/tmp/api")
	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "api · vsocial/backend")
	for _, want := range []string{"/tmp/api", "branch", "state", "sync"} {
		if !strings.Contains(panel, want) {
			t.Errorf("the panel does not say %q:\n%s", want, panel)
		}
	}
	if files := sectionContent(t, out, "files (2)"); !strings.Contains(files, "main.go") {
		t.Errorf("the files box does not list the repo's files:\n%s", files)
	}

	m, _ = press(m, "down")
	moved := stripANSI(m.View().Content)
	if got := sectionContent(t, moved, "web · vsocial/backend"); !strings.Contains(got, "↓3") {
		t.Errorf("the panel did not follow the cursor:\n%s", got)
	}
}

func TestPreviewTitleOnlyInTheBorder(t *testing.T) {
	m := cursorOn(t, previewModel(t), "/tmp/api")
	out := stripANSI(m.View().Content)

	if n := strings.Count(out, "api · vsocial/backend"); n != 1 {
		t.Errorf("the title appears %d times, want 1 (only on the border):\n%s", n, out)
	}
	if panel := sectionContent(t, out, "api · vsocial/backend"); strings.Contains(panel, "api · vsocial/backend") {
		t.Errorf("the title repeats inside the box:\n%s", panel)
	}
}

func TestPreviewInSubRowOfWorktree(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-detached", ""))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter") // expands the worktrees
	m, _ = press(m, "down")  // cursor on the sub-row
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree {
		t.Fatalf("the cursor is not on the subrow: %+v", e)
	}

	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "wt-detached [worktree]")
	if !strings.Contains(panel, "(detached)") {
		t.Errorf("the panel does not show the worktree branch:\n%s", panel)
	}
	for _, dup := range []string{"g lazygit", "! cmd"} {
		if strings.Contains(panel, dup) {
			t.Errorf("the panel repeats %q, which is already in keybinds:\n%s", dup, panel)
		}
	}
	for _, bad := range []string{"no-up", "clean", "ahead", "behind"} {
		if strings.Contains(panel, bad) {
			t.Errorf("the panel invents the state %q:\n%s", bad, panel)
		}
	}
}

func TestPreviewSummaryOfGroup(t *testing.T) {
	m := previewModel(t)
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindPrimary {
		t.Fatalf("the cursor is not on a primary header: %+v", e)
	}

	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "vsocial")
	if !strings.Contains(panel, "repos    3") {
		t.Errorf("the panel does not count the group's repos:\n%s", panel)
	}
	if !strings.Contains(panel, "dirty    1") || !strings.Contains(panel, "behind   1") {
		t.Errorf("the panel does not aggregate the group state:\n%s", panel)
	}
	if strings.Contains(panel, "ahead") {
		t.Errorf("the panel paints a state that is not there:\n%s", panel)
	}

	m, _ = press(m, "down")
	e, ok = m.selectedEntry()
	if !ok || e.kind != kindSecondary {
		t.Fatalf("the cursor is not on a secondary header: %+v", e)
	}
	panel = sectionContent(t, stripANSI(m.View().Content), "vsocial/backend")
	if !strings.Contains(panel, "repos    2") {
		t.Errorf("the secondary does not count only its own repos:\n%s", panel)
	}
	if strings.Contains(panel, "ahead") {
		t.Errorf("the secondary painted a state it does not have:\n%s", panel)
	}
}

func TestPreviewWithoutRows(t *testing.T) {
	m := previewModel(t)
	m.search = "zzz-no-match"
	m.clampCursor()

	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "repos")
	if !strings.Contains(panel, "no repositories match") {
		t.Errorf("the panel does not explain why it is empty:\n%s", panel)
	}
}

func TestPreviewFillsUntilItsHeight(t *testing.T) {
	m := previewModel(t)
	lay := m.layout()
	if lay.previewLines <= 0 {
		t.Fatalf("no panel at h=%d: %+v", m.height, lay)
	}

	for _, path := range []string{"/tmp/api", "/tmp/loose"} {
		m = cursorOn(t, m, path)
		box := sectionBox(t, m.View().Content, lay)
		if got := strings.Count(box, "\n") + 1; got != lay.previewLines+previewChrome {
			t.Errorf("%s: box = %d lines, want %d (2 borders + %d of content)",
				path, got, lay.previewLines+previewChrome, lay.previewLines)
		}
	}
}

func sectionBox(t *testing.T, view string, lay layout) string {
	t.Helper()
	lines := strings.Split(view, "\n")
	end := len(lines)
	if lay.showKeybinds {
		end -= lay.hintLines + keybindsChrome
	}
	start := end - lay.previewLines - previewChrome
	if start < 0 || start >= len(lines) {
		t.Fatalf("cannot find the panel's box in the view:\n%s", view)
	}
	return strings.Join(lines[start:end], "\n")
}

func TestPreviewAnnouncesTheListsThatDoNotFit(t *testing.T) {
	m := cursorOn(t, previewModel(t), "/tmp/api")
	m.height = 44 // roomy panel: the file list fits whole
	lay := m.layout()
	if lay.previewLines <= detailHeadLines+2 {
		t.Fatalf("precondition: the panel should have room for a list, it is %d", lay.previewLines)
	}

	panel := sectionContent(t, stripANSI(m.View().Content), "files (2)")
	if !strings.Contains(panel, "main.go") {
		t.Errorf("the files list is not visible with room:\n%s", panel)
	}
	if strings.Contains(panel, "more") {
		t.Errorf("a list that fits whole carries no truncation warning:\n%s", panel)
	}

	s := m.states["/tmp/api"]
	for i := 0; i < 20; i++ {
		s.Files = append(s.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/file%02d.go", i)})
	}
	m.states["/tmp/api"] = s
	panel = sectionContent(t, stripANSI(m.View().Content), "files (22)")
	if !strings.Contains(panel, "more") {
		t.Errorf("a truncated list is not announced:\n%s", panel)
	}
}

func TestPreviewNotBreaksTable(t *testing.T) {
	var projects []discovery.Project
	states := map[string]gitstatus.Snapshot{}
	for i := 0; i < 40; i++ {
		p := proj(fmt.Sprintf("repo%02d", i), fmt.Sprintf("/tmp/repo%02d", i), true)
		projects = append(projects, p)
		states[p.Path] = snapClean()
	}
	m := newTestModel(t, projects, states)
	m.cursor = len(m.entries()) - 1

	out := stripANSI(m.renderDashboard())
	if lines := strings.Split(out, "\n"); len(lines) != m.height {
		t.Errorf("lines = %d, want %d", len(lines), m.height)
	}
	for i, l := range strings.Split(m.renderDashboard(), "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d", i, w, m.width)
		}
	}
	if !strings.Contains(out, "repo39") {
		t.Errorf("the cursor's row is not visible:\n%s", out)
	}
	if !strings.Contains(out, "╭ keybinds ") {
		t.Errorf("the keybinds were left out:\n%s", out)
	}
}
