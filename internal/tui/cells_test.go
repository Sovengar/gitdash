// Tests de celdas del rediseño: Work Tree solo working tree,
// ↑↓up explícito, tabla quieta y FETCH transitorio.
package tui

import (
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// Counts sin bola; limpio en silencio.
func TestWtCellQuiet(t *testing.T) {
	base := proj("api", "/tmp/api", true)

	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	if text, _ := m.wtCell(m.rows()[0]); text != "" {
		t.Errorf("wt limpio = %q, want vacío (tabla quieta)", text)
	}

	dirty := snapDirty(2, 1)
	m = newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": dirty})
	if text, _ := m.wtCell(m.rows()[0]); text != "2 ?1" {
		t.Errorf("wt = %q, want '2 ?1' (sin ●)", text)
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

// Detached solo en BRANCH, y el dirty NO se suprime.
func TestWtDetachedKeepsDirty(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	s := snapClean()
	s.Status.Detached = true
	s.Status.Branch = "abc1234"
	s.Status.TrackedChanges = 3

	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": s})
	if text, _ := m.wtCell(m.rows()[0]); text != "3" {
		t.Errorf("wt detached+dirty = %q, want '3' (dirty no suprimido)", text)
	}
	branch, _ := m.branchCell(m.rows()[0])
	if !strings.Contains(branch, "abc1234 (detached)") {
		t.Errorf("branch = %q, want 'abc1234 (detached)'", branch)
	}
	// único sitio: la fila completa menciona detached una sola vez
	if n := strings.Count(stripANSI(m.renderRow(m.rows()[0], false)), "detached"); n != 1 {
		t.Errorf("detached aparece %d veces en la fila, want 1", n)
	}
}

// ↑↓up — diverged aquí, no-up explícito, vacío en sync.
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

// FETCH transitorio — éxito vacío, ⟳ corriendo, ✗ persistente.
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
	check("ok", "") // sin ✓ permanente
	check("fetching", "⟳ fetch")
	check("failed", "✗ fetch")
	check("", "")
}

// Headers separados — ningún título pegado al siguiente.
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
			t.Errorf("falta el header %q", want)
		}
	}
	if strings.Contains(out, "GROUP") {
		t.Errorf("la columna GROUP no debe estar en la TUI")
	}
}
