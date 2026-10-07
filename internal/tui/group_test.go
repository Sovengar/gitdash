package tui

import (
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

func TestGroupedView(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)

	entries := m.entries()
	if len(entries) != 5 {
		t.Fatalf("entries = %d, want 5", len(entries))
	}
	if entries[0].kind != kindPrimary || entries[0].group != "backend" {
		t.Errorf("entry 0 = %+v, want header backend", entries[0])
	}
	if entries[3].kind != kindPrimary || entries[3].group != group.Ungrouped {
		t.Errorf("entry 3 = %+v, want header (ungrouped)", entries[3])
	}

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▾ backend (2)") || !strings.Contains(out, "▾ (ungrouped) (1)") {
		t.Errorf("view without headers:\n%s", out)
	}
}

func TestFoldToggle(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)
	if len(m.entries()) != 3 {
		t.Fatalf("60: entries = %d, want header+2", len(m.entries()))
	}

	m, _ = press(m, "enter") // cursor on the backend header
	if len(m.entries()) != 1 {
		t.Errorf("folded entries = %d, want 1 (only header)", len(m.entries()))
	}

	m, _ = press(m, "enter")
	if len(m.entries()) != 3 {
		t.Errorf("expanded entries = %d, want 3", len(m.entries()))
	}

	m, _ = press(m, "enter")
	if len(m.entries()) != 1 || !m.collapsed["backend"] {
		t.Errorf("enter did not fold the header (entries=%d)", len(m.entries()))
	}
}

func TestGroupsWithFilter(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "api", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "web", PrimaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "cli", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)
	m.search = "api"

	entries := m.entries()
	groups := map[string]int{}
	for _, e := range entries {
		if e.kind == kindPrimary {
			groups[e.group]++
		}
	}
	if len(entries) != 2 || len(groups) != 1 || groups["backend"] != 1 {
		t.Errorf("entries = %v, want only backend", rowsOf(entries))
	}
}

func TestNestedView(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/d", Name: "d", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{
		"/a": snapClean(), "/b": snapClean(), "/c": snapClean(), "/d": snapClean(),
	}
	m := newTestModel(t, projects, states)

	entries := m.entries()
	want := rowsOf(entries)
	if len(entries) != 8 {
		t.Fatalf("entries = %d\n%s", len(entries), want)
	}
	if entries[0].kind != kindPrimary || entries[0].group != "vsocial" {
		t.Errorf("entry 0 = %+v, want header primary vsocial", entries[0])
	}
	if entries[1].kind != kindSecondary || entries[1].group != "vsocial/backend" {
		t.Errorf("entry 1 = %+v, want secondary header vsocial/backend", entries[1])
	}
	if entries[4].kind != kindSecondary || entries[4].group != "vsocial/frontend" {
		t.Errorf("entry 4 = %+v, want secondary header vsocial/frontend", entries[4])
	}
	if entries[5].r.project.Name != "b" {
		t.Errorf("entry 5 = %+v, want repo b", entries[5])
	}

	out := stripANSI(m.View().Content)
	for _, want := range []string{"▾ vsocial (3)", "▾ backend (2)", "▾ frontend (1)", "▾ (ungrouped) (1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("view without %q:\n%s", want, out)
		}
	}
}

func TestFoldSecondary(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down") // cursor on the backend secondary header
	if m.entries()[m.cursor].kind != kindSecondary {
		t.Fatalf("cursor at %+v, want a secondary header", m.entries()[m.cursor])
	}
	m, _ = press(m, "enter")
	entries := m.entries()
	if len(entries) != 4 {
		t.Errorf("entries = %s, want without the backend repos", rowsOf(entries))
	}
	if !m.collapsed["vsocial/backend"] {
		t.Errorf("key vsocial/backend not folded")
	}
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▸ backend (1)") || !strings.Contains(out, "▾ frontend (1)") {
		t.Errorf("glyphs incorrectos:\n%s", out)
	}
}

func TestFoldPrimaryHidesSecondaryHeaders(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "enter") // cursor on the vsocial primary header: folds it
	if n := len(m.entries()); n != 1 {
		t.Errorf("entries = %s, want only header primary", rowsOf(m.entries()))
	}
}

func TestEnterInRepoNotFoldsTheGroup(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", HasRepo: true}, // no secondary
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down")
	m, _ = press(m, "down")
	m, _ = press(m, "enter")
	if len(m.collapsed) != 0 {
		t.Errorf("enter on a repo with no worktrees folded the group: %v", m.collapsed)
	}

	m2 := newTestModel(t, projects, states)
	m2, _ = press(m2, "down")
	m2, _ = press(m2, "enter")
	if !m2.collapsed["vsocial/backend"] || m2.collapsed["vsocial"] {
		t.Errorf("the secondary did not fold its block: %v", m2.collapsed)
	}

	m3 := newTestModel(t, projects, states)
	m3, _ = press(m3, "enter")
	if !m3.collapsed["vsocial"] {
		t.Errorf("the primary did not fold its block: %v", m3.collapsed)
	}
}

func TestFoldKeysNotCollision(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "alpha", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "beta", SecondaryGroup: "backend", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "down") // alpha/backend secondary header
	m, _ = press(m, "enter")
	if !m.collapsed["alpha/backend"] {
		t.Fatalf("alpha/backend not folded: %v", m.collapsed)
	}
	entries := m.entries()
	for _, e := range entries {
		if e.kind == kindRepo && e.r.project.Name == "b" {
			return // repo b stays visible: beta/backend is intact
		}
	}
	t.Errorf("repo b disappeared when folding alpha/backend: %s", rowsOf(entries))
}

func TestPrimaryCountIncludesSecondary(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "vsocial", SecondaryGroup: "frontend", HasRepo: true},
		{Path: "/c", Name: "c", PrimaryGroup: "vsocial", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean(), "/c": snapClean()}
	m := newTestModel(t, projects, states)

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "▾ vsocial (3)") {
		t.Errorf("count primary incorrect:\n%s", out)
	}
}

// Regression: skipSec used to filter across primaries and hid projects under an expanded header.
func TestFoldSecondaryDoesNotLeakToNextPrimary(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "projects/mine", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}
	m := newTestModel(t, projects, states)

	m.collapsed["vsocial/backend"] = true

	entries := m.entries()
	if len(entries) != 4 {
		t.Fatalf("entries = %s, want the projects/mine repo visible", rowsOf(entries))
	}
	if entries[2].kind != kindPrimary || entries[2].group != "projects/mine" {
		t.Errorf("entry 2 = %+v, want header projects/mine", entries[2])
	}
	if entries[3].kind != kindRepo || entries[3].r.project.Name != "b" {
		t.Errorf("entry 3 = %+v, want repo b visible", entries[3])
	}
}

func rowsOf(entries []tableEntry) string {
	out := ""
	for _, e := range entries {
		switch e.kind {
		case kindPrimary:
			out += "|P:" + e.group
		case kindSecondary:
			out += "|S:" + e.group
		default:
			out += "|repo:" + e.r.project.Name
		}
	}
	return out
}
