// Tests del modelo TUI con datos sintéticos (patrón de vroom): sin teatest,
// se construye el Model, se le envían mensajes y se inspecciona el estado.
package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// newTestModel construye un modelo con proyectos y snapshots dados.
func newTestModel(t *testing.T, projects []discovery.Project, states map[string]gitstatus.Snapshot) Model {
	t.Helper()
	m := New(config.Defaults())
	m.width, m.height = 120, 30
	m.projects = projects
	m.states = states
	m.scanning = false
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

func press(m Model, key string) (Model, tea.Cmd) {
	km := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	if len(key) > 1 {
		codes := map[string]rune{
			"enter": tea.KeyEnter, "esc": tea.KeyEsc,
			"up": tea.KeyUp, "down": tea.KeyDown,
			"home": tea.KeyHome, "end": tea.KeyEnd,
			"backspace": tea.KeyBackspace, "tab": tea.KeyTab,
		}
		if c, ok := codes[key]; ok {
			km = tea.KeyPressMsg{Code: c}
		}
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

func TestSortAttentionFirstR6(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	rows := m.rows()
	if len(rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(rows))
	}
	// score 4 (dirty) > 3 (ahead/behind) > 2 (no-up/no-repo) > 0 (clean)
	if rows[0].project.Name != "dirty-api" {
		t.Errorf("rows[0] = %s, want dirty-api", rows[0].project.Name)
	}
	if rows[len(rows)-1].project.Name != "old-clean" {
		t.Errorf("último = %s, want old-clean", rows[len(rows)-1].project.Name)
	}
}

func TestSortActivityTieR6(t *testing.T) {
	// mismo score (clean): gana el más reciente
	recent := gitstatus.Snapshot{Status: snapClean().Status, LastCommit: time.Now().Unix()}
	old := snapClean()
	projects := []discovery.Project{proj("aaa", "/a", true), proj("bbb", "/b", true)}
	states := map[string]gitstatus.Snapshot{"/a": old, "/b": recent}
	m := newTestModel(t, projects, states)
	rows := m.rows()
	if rows[0].project.Name != "bbb" {
		t.Errorf("rows[0] = %s, want bbb (más reciente)", rows[0].project.Name)
	}
}

func TestFilterOnlyDirtyS7_1(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if len(m.rows()) != 6 {
		t.Fatalf("sin filtro rows = %d", len(m.rows()))
	}

	m, _ = press(m, "d")
	rows := m.rows()
	if len(rows) != 3 { // dirty-api, ahead-lib, behind-web
		t.Fatalf("S7.1: rows = %d, want 3", len(rows))
	}
	for _, r := range rows {
		if !pendingStates(r.state) {
			t.Errorf("fila %s no es pending", r.project.Name)
		}
	}

	m, _ = press(m, "d")
	if len(m.rows()) != 6 {
		t.Errorf("toggle off: rows = %d, want 6", len(m.rows()))
	}
}

func TestSearchS7_2(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "/")
	if !m.searchActive {
		t.Fatal("S7.2: '/' no abrió el input")
	}
	for _, c := range "api" {
		m, _ = press(m, string(c))
	}
	rows := m.rows()
	if len(rows) != 1 || rows[0].project.Name != "dirty-api" {
		t.Errorf("S7.2 en vivo: rows = %v", rowNames(rows))
	}

	m, _ = press(m, "enter")
	if m.searchActive || m.search != "api" {
		t.Errorf("S7.2 confirmar: active=%v search=%q", m.searchActive, m.search)
	}

	// reabrir con el filtro activo: limpiar el input y esc limpia el filtro
	m, _ = press(m, "/")
	for range 3 {
		m, _ = press(m, "backspace")
	}
	m, _ = press(m, "esc")
	if m.search != "" {
		t.Errorf("S7.2: esc no limpió el filtro (search=%q)", m.search)
	}
}

// S7.2: feedback visual inmediato — al pulsar / el título pinta [/|] con el
// el cursor del input (o su placeholder) ANTES de teclear nada.
func TestSearchImmediateFeedback(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "/")
	top := stripANSI(strings.SplitN(m.renderDashboard(), "\n", 2)[0])
	if !strings.Contains(top, "[/") {
		t.Errorf("S7.2: falta [/…] al entrar en filter mode: %q", top)
	}
	// placeholder visible con input vacío (nombre/grupo…)
	if !strings.Contains(top, "name/group") {
		t.Errorf("S7.2: placeholder no visible al abrir: %q", top)
	}

	// al confirmar el flag persiste con el texto confirmado
	m, _ = press(m, "a")
	m, _ = press(m, "enter")
	top = stripANSI(strings.SplitN(m.renderDashboard(), "\n", 2)[0])
	if !strings.Contains(top, "[/a]") {
		t.Errorf("S7.2: tras confirmar falta [/a]: %q", top)
	}
}

func TestSearchMatchesGroup(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/x", Name: "api", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/y", Name: "cli", PrimaryGroup: "otros", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/x": snapClean(), "/y": snapClean()}
	m := newTestModel(t, projects, states)
	m.search = "vsocial"
	rows := m.rows()
	if len(rows) != 1 || rows[0].project.Name != "api" {
		t.Errorf("S18.5: búsqueda por primario falló: %v", rowNames(rows))
	}
	m2 := newTestModel(t, projects, states)
	m2.search = "backend"
	rows = m2.rows()
	if len(rows) != 1 || rows[0].project.Name != "api" {
		t.Errorf("S18.5: búsqueda por secundario falló: %v", rowNames(rows))
	}
}

func TestNavigationBoundsS6_2(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	for range 10 {
		m, _ = press(m, "j")
	}
	if m.cursor != len(m.rows())-1 {
		t.Errorf("S6.2: cursor = %d, want %d (límite inferior)", m.cursor, len(m.rows())-1)
	}
	for range 10 {
		m, _ = press(m, "k")
	}
	if m.cursor != 0 {
		t.Errorf("S6.2: cursor = %d, want 0 (límite superior)", m.cursor)
	}
}

func TestDetailOpenS10_1(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	s.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	states["/tmp/dirty-api"] = s
	// dirty-api es la primera fila (score 4): cursor en 0
	m := newTestModel(t, projects, states)
	m, _ = press(m, "enter")
	if !m.detailOpen {
		t.Fatal("S10.1: enter no abrió el detalle")
	}
	r, _ := m.selected()
	out := m.renderDetail(r)
	if !strings.Contains(out, "main.go") || !strings.Contains(out, "dirty-api") {
		t.Errorf("S10.1: detalle sin contenido esperado:\n%s", out)
	}
	m, _ = press(m, "esc")
	if m.detailOpen {
		t.Error("S10.1: esc no cerró el detalle")
	}
}

func TestDetailShowsLastActionS10_2(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.lastAction["/tmp/old-clean"] = actionResult{kind: "pull", output: "error: pull diverged\n", err: "exit 1"}
	m.search = "old-clean"
	r, _ := m.selected()
	out := m.renderDetail(r)
	if !strings.Contains(out, "pull") || !strings.Contains(out, "failed") || !strings.Contains(out, "diverged") {
		t.Errorf("S10.2: detalle sin última acción:\n%s", out)
	}
}

func TestGuardNoRepoS9_6(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.search = "no-repo" // cursor sobre el proyecto sin repo
	m.cursor = 0

	for _, key := range []string{"p", "P", "f"} {
		_, cmd := press(m, key)
		if cmd == nil {
			t.Errorf("S9.6: tecla %s sin guard sobre no-repo", key)
			continue
		}
		msg := cmd()
		if nm, ok := msg.(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
			if key != "f" { // f sobre no-repo: fetchTargets lo excluye, cmd nil es válido
				t.Errorf("S9.6: tecla %s notificó %v", key, msg)
			}
		}
	}
}

func TestBlockRunningActionS9_5(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.search = "old-clean"
	m.running["/tmp/old-clean"] = "pull"

	_, cmd := press(m, "P")
	if cmd == nil {
		t.Fatal("S9.5: push sin cmd de bloqueo")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("S9.5: notificación = %v", cmd())
	}
}

func TestSummaryR6(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	total, dirty, ahead, behind := m.summary()
	if total != 6 || dirty != 1 || ahead != 1 || behind != 1 {
		t.Errorf("summary = (%d, %d, %d, %d), want (6, 1, 1, 1)", total, dirty, ahead, behind)
	}
}

func TestViewContainsTableR6(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := m.View().Content
	for _, want := range []string{"gitdash", "dirty-api", "↑2", "↓3", "6 repos"} {
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("R6: la vista no contiene %q", want)
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
