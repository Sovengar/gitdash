package tui

import (
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

func TestWtCellQuiet(t *testing.T) {
	base := proj("api", "/tmp/api", true)

	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	if text, _ := m.wtCell(m.rows()[0]); text != "" {
		t.Errorf("wt clean = %q, want empty (quiet table)", text)
	}

	dirty := snapDirty(2, 1)
	m = newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": dirty})
	if text, _ := m.wtCell(m.rows()[0]); text != "2 ?1" {
		t.Errorf("wt = %q, want '2 ?1' (no dot)", text)
	}

	errSnap := snapClean()
	errSnap.Err = "git: fatal"
	m = newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": errSnap})
	if text, _ := m.wtCell(m.rows()[0]); text != "⚠" {
		t.Errorf("wt error = %q, want ⚠", text)
	}

	noRepo := proj("dir", "/tmp/dir", false)
	m = newTestModel(t, []discovery.Project{noRepo},
		map[string]gitstatus.Snapshot{"/tmp/dir": gitstatus.Snapshot{}})
	if text, _ := m.wtCell(m.rows()[0]); text != "∅" {
		t.Errorf("wt no-repo = %q, want ∅", text)
	}
}

func TestWtDetachedKeepsDirty(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	s := snapClean()
	s.Status.Detached = true
	s.Status.Branch = "abc1234"
	s.Status.TrackedChanges = 3

	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": s})
	if text, _ := m.wtCell(m.rows()[0]); text != "3" {
		t.Errorf("wt detached+dirty = %q, want '3' (dirty not suppressed)", text)
	}
	branch, _ := m.branchCell(m.rows()[0])
	if !strings.Contains(branch, "abc1234 (detached)") {
		t.Errorf("branch = %q, want 'abc1234 (detached)'", branch)
	}
	if n := strings.Count(stripANSI(m.renderRow(m.rows()[0], false)), "detached"); n != 1 {
		t.Errorf("detached appears %d times in the row, want 1", n)
	}
}

func TestUpDown(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	cases := []struct {
		name string
		snap gitstatus.Snapshot
		want string
	}{
		{"diverged", snapDiverged(2, 3), "↑2↓3"},
		{"no-up", snapNoUpstream(), "no-up"},
		{"en sync", snapClean(), ""},
		{"solo ahead", snapAhead(1), "↑1"},
		{"solo behind", snapBehind(4), "↓4"},
	}
	for _, tc := range cases {
		m := newTestModel(t, []discovery.Project{base},
			map[string]gitstatus.Snapshot{"/tmp/api": tc.snap})
		if text, _ := m.upDownCell(m.rows()[0]); text != tc.want {
			t.Errorf("%s: upDown = %q, want %q", tc.name, text, tc.want)
		}
	}
}

func TestFetchCellTransient(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})

	check := func(state, want string) {
		t.Helper()
		m.fetchStates = map[string]string{"/tmp/api": state}
		if text, _ := m.fetchCell("/tmp/api"); text != want {
			t.Errorf("fetch %q = %q, want %q", state, text, want)
		}
	}
	check("ok", "") // no permanent check mark
	check("fetching", "⟳ fetch")
	check("failed", "✗ fetch")
	check("", "")
}

func TestHeaderSpacing(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := stripANSI(m.View().Content)

	for _, bad := range []string{"ACTIVITYFETCH", "Tree↑", "upSYNC", "NAMEBRANCH"} {
		if strings.Contains(out, bad) {
			t.Errorf("header pegado: %q", bad)
		}
	}
	for _, want := range []string{"Work Tree", "↑↓up", "SYNC", "ACTIVITY", "FETCH"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing header %q", want)
		}
	}
	if strings.Contains(out, "GROUP") {
		t.Errorf("the GROUP column must not be in the TUI")
	}
}
