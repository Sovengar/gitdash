// Tests de la TUI para worktrees plegados e indicador (0002 R15).
package tui

import (
	"strings"
	"testing"
	"time"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

func TestWorktreeHiddenS15_1(t *testing.T) {
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
		t.Errorf("S15.1: worktree descubierto visible: %v", rowNames(rows))
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want [multi-wt orphan]", rowNames(rows))
	}
	// convergencia del indicador en NAME
	m2 := newTestModel(t, []discovery.Project{main}, states)
	r := m2.rows()[0]
	text, _ := m2.nameCell(r)
	if !strings.Contains(text, "(2 wt)") {
		t.Errorf("S15.1: indicador = %q, want '(2 wt)'", text)
	}
}

// S15.2: el detalle lista los worktrees con rama y path.
func TestDetailWorktreesS15_2(t *testing.T) {
	main := proj("multi-wt", "/tmp/multi-wt", true)
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		{Path: "/tmp/multi-wt/wt-feat", Branch: "feat", Head: "abc1234"},
	}
	m := newTestModel(t, []discovery.Project{main}, map[string]gitstatus.Snapshot{"/tmp/multi-wt": s})
	m.cursor = 0
	r, _ := m.selected()
	out := stripANSI(m.renderDetail(r))
	if !strings.Contains(out, "worktrees (1)") || !strings.Contains(out, "feat") ||
		!strings.Contains(out, "abc1234") {
		t.Errorf("S15.2: detalle sin worktrees:\n%s", out)
	}
}

// S23 (0004): celda SYNC con la rama visible — behind, en sync (tabla
// quieta), ref missing, sin rama resuelta y override del marcador.
func TestSyncCellS23(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	behind := snapClean()
	behind.SyncBranch, behind.SyncBehind, behind.SyncKnown = "main", 3, true
	inSync := snapClean()
	inSync.SyncBranch, inSync.SyncKnown = "main", true
	missingRef := snapClean()
	missingRef.SyncBranch = "main" // SyncKnown=false: comparación fallida (0004 R23)
	noBranch := snapClean()

	cases := []struct {
		name string
		snap gitstatus.Snapshot
		want string
	}{
		{"S23.1 behind", behind, "main ↓3"},
		{"S23.3 en sync", inSync, "main"},
		{"S23.4 ref missing", missingRef, "main —"},
		{"S23.5 sin rama", noBranch, "—"},
	}
	for _, tc := range cases {
		m := newTestModel(t, []discovery.Project{base},
			map[string]gitstatus.Snapshot{"/tmp/api": tc.snap})
		if text, _ := m.syncCell(m.rows()[0]); text != tc.want {
			t.Errorf("%s: sync = %q, want %q", tc.name, text, tc.want)
		}
	}

	// S23.2: la rama del override aparece por fila
	develop := snapClean()
	develop.SyncBranch, develop.SyncBehind, develop.SyncKnown = "develop", 2, true
	ovr := proj("api", "/tmp/api", true)
	ovr.SyncBranch = "develop"
	m := newTestModel(t, []discovery.Project{ovr},
		map[string]gitstatus.Snapshot{"/tmp/api": develop})
	if text, _ := m.syncCell(m.rows()[0]); text != "develop ↓2" {
		t.Errorf("S23.2: sync = %q, want 'develop ↓2'", text)
	}
}

// S16 (preparación): helper de snapshot con primario no vacío.
func groupedProj(name, path, primary string) discovery.Project {
	return discovery.Project{Path: path, Name: name, PrimaryGroup: primary, HasRepo: true}
}

func snapCleanAt(age time.Duration) gitstatus.Snapshot {
	s := snapClean()
	s.LastCommit = time.Now().Add(-age).Unix()
	return s
}
