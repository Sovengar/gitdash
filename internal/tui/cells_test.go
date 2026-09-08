// Tests de celdas del rediseño 0004: Work Tree solo working tree (R26),
// ↑↓up explícito (R27), tabla quieta (R28) y FETCH transitorio (R24).
package tui

import (
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// S26.1/S26.2/S28: counts sin bola; limpio en silencio.
func TestWtCellQuietR26(t *testing.T) {
	base := proj("api", "/tmp/api", true)

	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	if text, _ := m.wtCell(m.rows()[0]); text != "" {
		t.Errorf("S26.2: wt limpio = %q, want vacío (tabla quieta)", text)
	}

	dirty := snapDirty(2, 1)
	m = newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": dirty})
	if text, _ := m.wtCell(m.rows()[0]); text != "2 ?1" {
		t.Errorf("S26.1: wt = %q, want '2 ?1' (sin ●)", text)
	}

	errSnap := snapClean()
	errSnap.Err = "git: fatal"
	m = newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": errSnap})
	if text, _ := m.wtCell(m.rows()[0]); text != "⚠" {
		t.Errorf("S26.4: wt error = %q, want ⚠", text)
	}

	noRepo := proj("dir", "/tmp/dir", false)
	m = newTestModel(t, []discovery.Project{noRepo},
		map[string]gitstatus.Snapshot{"/tmp/dir": gitstatus.Snapshot{}})
	if text, _ := m.wtCell(m.rows()[0]); text != "∅" {
		t.Errorf("S26.4: wt no-repo = %q, want ∅", text)
	}
}

// S26.3: detached solo en BRANCH, y el dirty NO se suprime.
func TestWtDetachedKeepsDirtyS26_3(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	s := snapClean()
	s.Status.Detached = true
	s.Status.Branch = "abc1234"
	s.Status.TrackedChanges = 3

	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": s})
	if text, _ := m.wtCell(m.rows()[0]); text != "3" {
		t.Errorf("S26.3: wt detached+dirty = %q, want '3' (dirty no suprimido)", text)
	}
	branch, _ := m.branchCell(m.rows()[0])
	if !strings.Contains(branch, "abc1234 (detached)") {
		t.Errorf("S26.3: branch = %q, want 'abc1234 (detached)'", branch)
	}
	// único sitio: la fila completa menciona detached una sola vez
	if n := strings.Count(stripANSI(m.renderRow(m.rows()[0], false)), "detached"); n != 1 {
		t.Errorf("S26.3: detached aparece %d veces en la fila, want 1", n)
	}
}

// S27: ↑↓up — diverged aquí, no-up explícito, vacío en sync.
func TestUpDownR27(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	cases := []struct {
		name string
		snap gitstatus.Snapshot
		want string
	}{
		{"S27.1 diverged", snapDiverged(2, 3), "↑2↓3"},
		{"S27.2 no-up", snapNoUpstream(), "no-up"},
		{"S27.3 en sync", snapClean(), ""},
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

// S24: FETCH transitorio — éxito vacío, ⟳ corriendo, ✗ persistente.
func TestFetchCellTransientR24(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	m := newTestModel(t, []discovery.Project{base},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})

	check := func(state, want string) {
		t.Helper()
		m.fetchStates = map[string]string{"/tmp/api": state}
		if text, _ := m.fetchCell("/tmp/api"); text != want {
			t.Errorf("S24: fetch %q = %q, want %q", state, text, want)
		}
	}
	check("ok", "") // S24.1: sin ✓ permanente
	check("fetching", "⟳ fetch")
	check("failed", "✗ fetch")
	check("", "")
}

// S25.3: headers separados — ningún título pegado al siguiente.
func TestHeaderSpacingR25(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := stripANSI(m.View().Content)

	for _, bad := range []string{"ACTIVITYFETCH", "Tree↑", "upSYNC", "NAMEBRANCH"} {
		if strings.Contains(out, bad) {
			t.Errorf("S25.3: header pegado: %q", bad)
		}
	}
	for _, want := range []string{"Work Tree", "↑↓up", "SYNC", "ACTIVITY", "FETCH"} {
		if !strings.Contains(out, want) {
			t.Errorf("S25.1: falta el header %q", want)
		}
	}
	if strings.Contains(out, "GROUP") {
		t.Errorf("S25.1: la columna GROUP no debe estar en la TUI")
	}
}
