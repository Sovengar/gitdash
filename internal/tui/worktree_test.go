// Tests de la TUI para worktrees plegados e indicador.
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
	// convergencia del indicador en NAME
	m2 := newTestModel(t, []discovery.Project{main}, states)
	r := m2.rows()[0]
	text, _ := m2.nameCell(r)
	if !strings.Contains(text, "(2 wt)") {
		t.Errorf("indicador = %q, want '(2 wt)'", text)
	}
}

// El detalle lista los worktrees con rama y path.
func TestDetailWorktrees(t *testing.T) {
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
		t.Errorf("detalle sin worktrees:\n%s", out)
	}
}

// Celda SYNC con la rama visible — behind, en sync (tabla
// quieta), ref missing, sin rama resuelta y override del marcador.
func TestSyncCell(t *testing.T) {
	base := proj("api", "/tmp/api", true)
	behind := snapClean()
	behind.SyncBranch, behind.SyncBehind, behind.SyncKnown = "main", 3, true
	inSync := snapClean()
	inSync.SyncBranch, inSync.SyncKnown = "main", true
	missingRef := snapClean()
	missingRef.SyncBranch = "main" // SyncKnown=false: comparación fallida
	noBranch := snapClean()

	cases := []struct {
		name string
		snap gitstatus.Snapshot
		want string
	}{
		{"behind", behind, "main ↓3"},
		{"en sync", inSync, "main"},
		{"ref missing", missingRef, "main —"},
		{"sin rama", noBranch, "—"},
	}
	for _, tc := range cases {
		m := newTestModel(t, []discovery.Project{base},
			map[string]gitstatus.Snapshot{"/tmp/api": tc.snap})
		if text, _ := m.syncCell(m.rows()[0]); text != tc.want {
			t.Errorf("%s: sync = %q, want %q", tc.name, text, tc.want)
		}
	}

	// La rama del override aparece por fila
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

// ---- expansión y operaciones de worktrees ----

// wt construye un worktree del snapshot para los tests.
func wt(path, branch string) gitstatus.Worktree {
	return gitstatus.Worktree{Path: path, Branch: branch, Head: "abc1234"}
}

// repoWithWorktrees devuelve el proyecto y su snapshot con worktrees.
func repoWithWorktrees(name, path string, wts ...gitstatus.Worktree) (discovery.Project, map[string]gitstatus.Snapshot) {
	p := proj(name, path, true)
	s := snapClean()
	s.Worktrees = wts
	return p, map[string]gitstatus.Snapshot{path: s}
}

// worktreeNames lista los basenames de las sub-filas kindWorktree.
func worktreeNames(entries []tableEntry) []string {
	var out []string
	for _, e := range entries {
		if e.kind == kindWorktree {
			out = append(out, filepath.Base(e.wt.Path))
		}
	}
	return out
}

// Repo con worktrees, plegado por defecto (▸ + contador, sin sub-filas).
func TestWorktreeCollapsedByDefault(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)

	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Fatalf("sub-filas sin expandir: %v", got)
	}
	text, _ := m.nameCell(m.rows()[0])
	if !strings.Contains(text, "▸") || !strings.Contains(text, "(2 wt)") {
		t.Errorf("NAME = %q, want ▸ y (2 wt)", text)
	}
	if strings.Contains(stripANSI(m.View().Content), "↳") {
		t.Error("no debe haber sub-filas en la vista")
	}
}

// `space` alterna expandido/plegado.
func TestWorktreeToggleS30_2_3(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)

	m, _ = press(m, " ")
	if !m.expanded[p.Path] {
		t.Fatal("space no expandió")
	}
	if got := worktreeNames(m.entries()); len(got) != 2 {
		t.Fatalf("sub-filas = %v, want 2", got)
	}
	if text, _ := m.nameCell(m.rows()[0]); !strings.Contains(text, "▾") {
		t.Errorf("glyph = %q, want ▾", text)
	}

	m, _ = press(m, " ")
	if m.expanded[p.Path] {
		t.Error("space no volvió a plegar")
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("sub-filas tras plegar = %v", got)
	}
	if text, _ := m.nameCell(m.rows()[0]); !strings.Contains(text, "▸") {
		t.Errorf("glyph = %q, want ▸", text)
	}
}

// Space sobre repo sin worktrees es no-op.
func TestWorktreeExpandNoRepoWts(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m, _ = press(m, " ")
	if len(m.expanded) != 0 {
		t.Errorf("expansión cambiada en repo sin worktrees: %v", m.expanded)
	}
}

// Space sobre un header de grupo no cambia el plegado.
func TestWorktreeExpandOnHeader(t *testing.T) {
	p := discovery.Project{Path: "/a", Name: "a", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/a": s})
	if m.entries()[0].kind != kindPrimary {
		t.Fatalf("precondición: cursor no en header: %+v", m.entries()[0])
	}
	m, _ = press(m, " ")
	if len(m.expanded) != 0 || m.collapsed["g"] {
		t.Errorf("header alterado: expanded=%v collapsed=%v", m.expanded, m.collapsed)
	}
}

// Space sobre una sub-fila no cambia la expansión del padre.
func TestWorktreeExpandOnSubrow(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down") // repo → sub-fila
	if e, _ := m.selectedEntry(); e.kind != kindWorktree {
		t.Fatalf("cursor no en sub-fila: %+v", e)
	}
	before := m.expanded[p.Path]
	m, _ = press(m, " ")
	if m.expanded[p.Path] != before {
		t.Errorf("la expansión del padre cambió: %v", m.expanded)
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("sub-fila desapareció: %v", got)
	}
}

// Cobertura total, principal no sub-fila, dedupe.
func TestWorktreeCoverageAndDedupe(t *testing.T) {
	main := proj("multi", "/tmp/multi", true)
	// Worktree descubierto con marcador → dedupe: sub-fila, no top-level.
	disc := discovery.Project{
		Path: "/tmp/wt-marcado", Name: "wt-marcado", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi",
	}
	mainSnap := snapClean()
	mainSnap.Worktrees = []gitstatus.Worktree{
		wt("/tmp/wt-marcado", "feat/marcado"), // con marcador
		wt("/tmp/wt-root", "feat/root"),       // sin marcador, dentro de root
		wt("/outside/wt-fuera", "feat/fuera"), // sin marcador, fuera de roots
	}
	m := newTestModel(t, []discovery.Project{main, disc},
		map[string]gitstatus.Snapshot{"/tmp/multi": mainSnap, "/tmp/wt-marcado": snapClean()})

	m, _ = press(m, " ")
	got := worktreeNames(m.entries())
	if len(got) != 3 {
		t.Fatalf("sub-filas = %v, want 3", got)
	}
	for _, e := range m.entries() {
		if e.kind == kindWorktree && filepath.Clean(e.wt.Path) == "/tmp/multi" {
			t.Error("el principal aparece como sub-fila")
		}
	}
	// El worktree descubierto no aparece como fila top-level
	for _, e := range m.entries() {
		if e.kind == kindRepo && e.r.project.Path == "/tmp/wt-marcado" {
			t.Error("worktree descubierto aparece como fila de repo")
		}
	}
	n := 0
	for _, name := range got {
		if name == "wt-marcado" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("worktree descubierto aparece %d veces como sub-fila, want 1", n)
	}
}

// MEDIUM-2: el dedupe normaliza paths (symlink/barra final/`.`): un worktree
// descubierto con MainRepo no idéntico al principal sigue oculto y su detalle
// reutiliza el snapshot vivo.
func TestWorktreeDedupeNormalizedPathsMEDIUM_2(t *testing.T) {
	main := proj("multi", "/tmp/multi", true)
	// Descubierto con marcador; path con barra final y MainRepo con `/./`.
	disc := discovery.Project{
		Path: "/tmp/multi/wt-feat/", Name: "wt-feat", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi/.",
	}
	mainSnap := snapClean()
	// El snapshot del principal reporta el path sin barra final.
	mainSnap.Worktrees = []gitstatus.Worktree{wt("/tmp/multi/wt-feat", "feat")}
	discSnap := snapDirty(3, 0)
	m := newTestModel(t, []discovery.Project{main, disc},
		map[string]gitstatus.Snapshot{"/tmp/multi": mainSnap, "/tmp/multi/wt-feat/": discSnap})

	// El descubierto queda oculto pese a los paths no idénticos.
	for _, r := range m.rows() {
		if filepath.Clean(r.project.Path) == "/tmp/multi/wt-feat" {
			t.Errorf("MEDIUM-2: worktree descubierto visible como top-level: %q", r.project.Path)
		}
	}
	m, _ = press(m, " ")
	got := worktreeNames(m.entries())
	if len(got) != 1 || got[0] != "wt-feat" {
		t.Fatalf("MEDIUM-2: sub-filas = %v, want [wt-feat]", got)
	}
	// El dedupe materializa el snapshot vivo del descubierto (paths limpiados).
	var wtEntry tableEntry
	for _, e := range m.entries() {
		if e.kind == kindWorktree {
			wtEntry = e
		}
	}
	r := m.worktreeRow(wtEntry.wt)
	if r.project.Name != "wt-feat" || r.snap.Status.TrackedChanges != 3 {
		t.Errorf("MEDIUM-2: el detalle no reutiliza el snapshot del descubierto: name=%q tracked=%d",
			r.project.Name, r.snap.Status.TrackedChanges)
	}
}

// Huérfano sigue visible con [wt] y no es expandible. Aunque su
// snapshot tenga worktrees (caso realista: `git worktree list` desde el
// propio worktree incluye el principal), NO debe mostrar glyph ni contador
// (dead affordance, HIGH-1).
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
		t.Errorf("huérfano sin tag [wt]: %q", text)
	}
	if strings.Contains(text, "wt)") || strings.Contains(text, "▸") || strings.Contains(text, "▾") {
		t.Errorf("HIGH-1: huérfano con glyph/contador (dead affordance): %q", text)
	}
	m, _ = press(m, " ")
	if len(m.expanded) != 0 {
		t.Errorf("huérfano expandible: %v", m.expanded)
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("sub-filas en huérfano: %v", got)
	}
}

// Sin worktrees o snapshot sin recolectar → sin glyph ni no-op.
func TestWorktreeNoGlyphS31_5_6(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	text, _ := m.nameCell(m.rows()[0])
	if strings.Contains(text, "wt)") || strings.Contains(text, "▸") || strings.Contains(text, "▾") {
		t.Errorf("NAME con glyph/contador sin worktrees: %q", text)
	}
	m, _ = press(m, " ")
	if len(m.expanded) != 0 {
		t.Errorf("space expandió un snapshot sin worktrees: %v", m.expanded)
	}
}

// El cursor alcanza la sub-fila y las acciones la resuelven.
func TestWorktreeCursor(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree || filepath.Base(e.wt.Path) != "wt-a" {
		t.Fatalf("cursor = %+v", e)
	}
	r, ok := m.selected()
	if !ok || r.project.Path != "/tmp/wt-a" {
		t.Errorf("selected() = %+v, want path del worktree", r.project)
	}
}

// El scroll sigue a la sub-fila fuera de la ventana visible.
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
	m.height = 8 // ventana pequeña
	m.cursor = len(m.entries()) - 1
	m, _ = press(m, " ") // expandir el último repo
	m, _ = press(m, "down")

	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "wt-a") {
		t.Errorf("la sub-fila no está visible tras el scroll:\n%s", out)
	}
}

// Una acción de vista real que hace
// desaparecer las sub-filas bajo el cursor reclampa el cursor. Se usa el
// filtro `d` (el repo es limpio, así deja de pasar el filtro) porque
// hace `space` no-op sobre una sub-fila, así que no hay un camino de usuario
// que pliegue el padre desde ella.
func TestWorktreeFoldKeepsCursor(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	m, _ = press(m, "down") // cursor en la última sub-fila
	if m.cursor != 2 {
		t.Fatalf("precondición: cursor = %d, want 2", m.cursor)
	}

	m, _ = press(m, "d") // filtro dirty: el repo limpio desaparece con sus sub-filas
	entries := m.entries()
	if len(entries) == 0 {
		if m.cursor != 0 {
			t.Errorf("cursor = %d con tabla vacía, want 0", m.cursor)
		}
	} else if m.cursor < 0 || m.cursor >= len(entries) {
		t.Errorf("cursor fuera de rango: %d (entries=%d)", m.cursor, len(entries))
	}

	m, _ = press(m, "d") // revertir el filtro
	if got := worktreeNames(m.entries()); len(got) != 2 {
		t.Errorf("el repo no siguió expandido al revertir: %v", got)
	}
}

// Pull/sync/push/recollect operan sobre el path del worktree.
func TestWorktreeActionsPathS33_2_4(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	for _, tc := range []struct{ key, kind string }{
		{"p", "pull"}, {"s", "sync"}, {"P", "push"}, {"R", "collect"},
	} {
		m := newTestModel(t, []discovery.Project{p}, st)
		m, _ = press(m, " ")
		m, _ = press(m, "down")
		m, _ = press(m, tc.key)
		if m.running["/tmp/wt-a"] != tc.kind {
			t.Errorf("%s en running = %q, want %q", tc.key, m.running["/tmp/wt-a"], tc.kind)
		}
	}
}

// Fetch corre sobre el path del worktree bajo el cursor. Se observa
// el canal de eventos: el batch publica fetchStateMsg con el path objetivo.
func TestWorktreeFetchPath(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	r, ok := m.selected()
	if !ok || r.project.Path != "/tmp/wt-a" || !r.project.HasRepo {
		t.Fatalf("fetch operaría sobre %+v, want worktree", r.project)
	}

	m, _ = press(m, "f")
	// El batch corre en goroutine: esperamos el evento con el path del worktree.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-m.events:
			if fs, ok := ev.(fetchStateMsg); ok && filepath.Clean(fs.path) == "/tmp/wt-a" {
				if fs.state != "fetching" {
					t.Errorf("primer estado = %q, want fetching", fs.state)
				}
				return
			}
		case <-deadline:
			t.Fatal("no se observó fetchStateMsg para el path del worktree")
		}
	}
}

// Lazygit/update/editor resuelven el path del worktree.
func TestWorktreeHandoffPath(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")

	// editor: el Cmd de handoff se construye (dir = path del worktree).
	me, cmd := press(m, "e")
	if cmd == nil {
		t.Error("editor sin tea.Cmd")
	}
	if r, _ := me.selected(); r.project.Path != "/tmp/wt-a" {
		t.Errorf("editor path = %q", r.project.Path)
	}

	// update: comando configurado → running sobre el path del worktree.
	mu := newTestModel(t, []discovery.Project{p}, st)
	mu, _ = press(mu, " ")
	mu, _ = press(mu, "down")
	mu.cfg.Commands["update"] = "echo"
	mu, _ = press(mu, "u")
	if mu.running["/tmp/wt-a"] != "update" {
		t.Errorf("update running = %q", mu.running["/tmp/wt-a"])
	}

	// lazygit: solo si está instalado (mismo skip que el resto de tests).
	if !hasLazygit() {
		return
	}
	mg := newTestModel(t, []discovery.Project{p}, st)
	mg, _ = press(mg, " ")
	mg, _ = press(mg, "down")
	mg, _ = press(mg, "g")
	if mg.running["/tmp/wt-a"] != "lazygit" {
		t.Errorf("lazygit running = %q", mg.running["/tmp/wt-a"])
	}
}

// Comando `!` corre con el directorio del worktree.
func TestWorktreeBang(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/gitdash-test/wt", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	m, _ = press(m, "enter") // abre el detalle de la sub-fila
	if !m.detailOpen {
		t.Fatal("enter no abrió el detalle del worktree")
	}
	m, _ = press(m, "!")
	if !m.cmdOpen {
		t.Fatal("! no abrió el input")
	}
	m, _ = press(m, "g")
	m, _ = press(m, "enter")
	if m.running["/tmp/gitdash-test/wt"] != "cmd" {
		t.Errorf("running = %q, want cmd sobre el worktree", m.running["/tmp/gitdash-test/wt"])
	}
}

// Detalle mínimo del worktree (path, rama, head) sin estado inventado.
func TestWorktreeDetail(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi",
		gitstatus.Worktree{Path: "/tmp/wt-detached", Branch: "", Head: "abc1234"})
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	m, _ = press(m, "enter")

	out := stripANSI(m.View().Content)
	for _, want := range []string{"/tmp/wt-detached", "(detached)", "abc1234"} {
		if !strings.Contains(out, want) {
			t.Errorf("detalle sin %q:\n%s", want, out)
		}
	}
	// No se inventa estado git derivado para el worktree: el panel mínimo (no
	// la sección de stats, que sí menciona ahead/behind globales) no los tiene.
	e, ok := m.selectedEntry()
	if !ok {
		t.Fatal("sin entrada seleccionada")
	}
	panel := stripANSI(m.renderWorktreeDetail(e))
	for _, bad := range []string{"no-up", "clean", "ahead", "behind"} {
		if strings.Contains(panel, bad) {
			t.Errorf("detalle inventa estado %q:\n%s", bad, panel)
		}
	}
}

// Si el worktree fue
// descubierto con marcador y tiene snapshot vivo, el detalle muestra ESE
// snapshot (intención confirmada por el usuario), no un panel mínimo.
func TestWorktreeDetailDiscovered(t *testing.T) {
	main, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-marcado", "feat"))
	disc := discovery.Project{
		Path: "/tmp/wt-marcado", Name: "wt-marcado", HasRepo: true,
		IsWorktree: true, MainRepo: "/tmp/multi",
	}
	st["/tmp/wt-marcado"] = snapDirty(2, 1)
	m := newTestModel(t, []discovery.Project{main, disc}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	m, _ = press(m, "enter")

	e, _ := m.selectedEntry()
	out := stripANSI(m.renderWorktreeDetail(e))
	// El snapshot vivo del worktree descubierto se muestra (estado derivado
	// real: "2 ?1" dirty), no el panel mínimo path/rama/head.
	if !strings.Contains(out, "wt-marcado") || !strings.Contains(out, "state") {
		t.Errorf("detalle de worktree descubierto incompleto:\n%s", out)
	}
	if !strings.Contains(out, "2 ?1") {
		t.Errorf("el detalle no refleja el snapshot vivo:\n%s", out)
	}
	// No es el panel mínimo (que no tiene línea "state").
	if strings.Contains(out, "head    ") {
		t.Errorf("se usó el panel mínimo en vez del snapshot vivo:\n%s", out)
	}
}

// Las notificaciones usan el basename, no la ruta absoluta.
func TestWorktreeNotificationBasename(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if got := m.nameOf("/tmp/projects/wt-feat-x"); got != "wt-feat-x" {
		t.Errorf("nameOf = %q, want basename", got)
	}
}

// La sub-fila se renderiza por el camino dedicado (no
// `renderRow`), es estrictamente distinta de una fila de repo y de un header
// de grupo, muestra la rama y no inventa estado por-worktree (LOW-c/LOW-d).
func TestWorktreeRowRenderS34_1_2_4(t *testing.T) {
	main := discovery.Project{Path: "/tmp/multi", Name: "multi", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-featx", "feat/x")}
	m := newTestModel(t, []discovery.Project{main}, map[string]gitstatus.Snapshot{"/tmp/multi": s})
	m, _ = press(m, "down") // cursor al repo (header primario en 0)
	m, _ = press(m, " ")    // expandir

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
		t.Fatal("no hay sub-fila renderizada")
	}

	// LOW-c: la sub-fila nunca lee un Snapshot vacío como no-up/error/clean.
	for _, bad := range []string{"no-up", "error", "clean", "↑", "↓"} {
		if strings.Contains(wtLine, bad) {
			t.Errorf("la sub-fila inventa estado %q: %q", bad, wtLine)
		}
	}
	if !strings.Contains(wtLine, "↳") || !strings.Contains(wtLine, "wt-featx") {
		t.Errorf("sub-fila no distinguible: %q", wtLine)
	}
	if !strings.Contains(wtLine, "feat/x") {
		t.Errorf("BRANCH sin rama del worktree: %q", wtLine)
	}

	// LOW-d: estrictamente distinta de la fila de repo y del header.
	if wtLine == repoLine || wtLine == headerLine {
		t.Errorf("sub-fila idéntica a repo/header\n repo=%q\n header=%q\n wt=%q", repoLine, headerLine, wtLine)
	}
	if strings.Contains(repoLine, "↳") || strings.Contains(headerLine, "↳") {
		t.Errorf("glyph de sub-fila filtrado a otra fila: repo=%q header=%q", repoLine, headerLine)
	}

	// La sub-fila también se pinta en la vista completa.
	if !strings.Contains(stripANSI(m.View().Content), "↳") {
		t.Error("la sub-fila no aparece en la vista")
	}
}

// Worktree detached muestra (detached) y el head corto.
func TestWorktreeRowDetached(t *testing.T) {
	w := gitstatus.Worktree{Path: "/tmp/wt-det", Branch: "", Head: "abc1234"}
	m := newTestModel(t, nil, nil)
	line := stripANSI(m.renderWorktreeRow(w, false))
	if !strings.Contains(line, "(detached)") || !strings.Contains(line, "abc1234") {
		t.Errorf("sub-fila detached = %q", line)
	}
}

// El NAME del principal muestra ▸/▾ junto a (N wt).
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

// La expansión sobrevive al reinicio; por defecto plegado.
func TestWorktreeExpansionPersistsS35_1_2(t *testing.T) {
	dir := t.TempDir()
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))

	// Sin estado persistido → plegado.
	fresh := newTestModel(t, []discovery.Project{p}, st)
	fresh.loadPersisted(nil)
	if len(worktreeNames(fresh.entries())) != 0 || fresh.expanded[p.Path] {
		t.Error("repo no aparece plegado por defecto")
	}

	m := newTestModel(t, []discovery.Project{p}, st)
	m.store = state.NewStoreAt(dir)
	m, _ = press(m, " ") // expandir y persistir
	if !m.expanded[p.Path] {
		t.Fatal("space no expandió")
	}

	// "reinicio": misma store, modelo nuevo.
	m2 := newTestModel(t, []discovery.Project{p}, st)
	m2.store = state.NewStoreAt(dir)
	m2.loadPersisted(m2.store.LoadCollapsed())
	if !m2.expanded[p.Path] {
		t.Error("la expansión no sobrevivió al reinicio")
	}
	if got := worktreeNames(m2.entries()); len(got) != 2 {
		t.Errorf("sub-filas tras reinicio = %v, want 2", got)
	}
}

// Sin colisión con las claves de grupo de collapsed.json.
func TestWorktreePersistNoCollision(t *testing.T) {
	dir := t.TempDir()
	if err := state.NewStoreAt(dir).SaveCollapsed(map[string]bool{"backend": true}); err != nil {
		t.Fatal(err)
	}
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.store = state.NewStoreAt(dir)
	m.loadPersisted(m.store.LoadCollapsed())
	if !m.collapsed["backend"] {
		t.Fatal("la clave de grupo existente no se cargó")
	}
	m, _ = press(m, " ")

	loaded := state.NewStoreAt(dir).LoadCollapsed()
	if !loaded["backend"] {
		t.Error("la clave de grupo existente se perdió")
	}
	if loaded[state.WorktreePrefix+p.Path] != true {
		t.Errorf("expansión ausente del namespace propio: %v", loaded)
	}
}

// Collapsed.json corrupto o ausente degrada a plegado sin fallar.
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
		t.Errorf("expansión con fichero corrupto: %v", m.expanded)
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("sub-filas con fichero corrupto: %v", got)
	}
}

// El filtro dirty oculta al padre y a sus sub-filas.
func TestWorktreeDirtyFilter(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.expanded[p.Path] = true
	m.onlyDirty = true
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("sub-filas visibles bajo filtro dirty: %v", got)
	}
}

// El plegado de grupos manda; independiente de la expansión.
func TestWorktreeGroupFoldS36_2_3(t *testing.T) {
	p := discovery.Project{Path: "/a", Name: "a", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/a": s})

	m, _ = press(m, "down") // cursor al repo
	m, _ = press(m, " ")    // expandir
	if len(worktreeNames(m.entries())) != 1 {
		t.Fatal("precondición: repo no expandido")
	}
	m, _ = press(m, "home") // cursor al header primario
	m, _ = press(m, "tab")  // plegar grupo
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("sub-filas visibles con grupo plegado: %v", got)
	}
	m, _ = press(m, "tab") // desplegar grupo
	if !m.expanded["/a"] {
		t.Error("la expansión del repo se perdió al togglear el grupo")
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("sub-filas tras desplegar = %v, want 1", got)
	}
}

// Default `space` y rebind configurable.
func TestWorktreeKeybindingS37_1_2(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))

	// Default space.
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	if !m.expanded[p.Path] {
		t.Error("space no expandió con los bindings por defecto")
	}

	// Rebind a `w`.
	m2 := newTestModel(t, []discovery.Project{p}, st)
	m2.cfg.Keybindings["expand"] = "w"
	m2, _ = press(m2, "w")
	if !m2.expanded[p.Path] {
		t.Fatal("w no expandió tras el rebind")
	}
	m3 := newTestModel(t, []discovery.Project{p}, st)
	m3.cfg.Keybindings["expand"] = "w"
	m3, _ = press(m3, " ")
	if m3.expanded[p.Path] {
		t.Error("space sigue expandiendo tras el rebind")
	}
}

// El hint de expansión aparece con su tecla configurada.
func TestWorktreeHint(t *testing.T) {
	cfg := config.Defaults()
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "space expand") {
		t.Errorf("hint de expansión ausente: %v", cfg.HintBarLines())
	}
	cfg.Keybindings["expand"] = "w"
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "w expand") {
		t.Errorf("hint sin la tecla rebindeada: %v", cfg.HintBarLines())
	}
}

// `tab` sobre una sub-fila no pliega grupos.
func TestWorktreeTabOnSubrow(t *testing.T) {
	p := discovery.Project{Path: "/a", Name: "a", PrimaryGroup: "g", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/a": s})
	m, _ = press(m, "down") // repo
	m, _ = press(m, " ")    // expandir
	m, _ = press(m, "down") // sub-fila
	if e, _ := m.selectedEntry(); e.kind != kindWorktree {
		t.Fatalf("cursor no en sub-fila: %+v", e)
	}
	before := len(m.collapsed)
	m, _ = press(m, "tab")
	if len(m.collapsed) != before {
		t.Errorf("tab sobre sub-fila cambió el plegado: %v", m.collapsed)
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("sub-fila desapareció: %v", got)
	}
}

// Match por rama/basename con expansión transitoria.
func TestWorktreeSearchMatchS40_1_2_3(t *testing.T) {
	p := proj("alpha", "/tmp/alpha", true) // no matchea por nombre
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{
		wt("/tmp/wt-feature", "feat/x"),
		wt("/tmp/wt-other", "chore/y"),
		wt("/tmp/nombre-raro", "chore/z"),
	}
	states := map[string]gitstatus.Snapshot{"/tmp/alpha": s}

	// Match por rama.
	m := newTestModel(t, []discovery.Project{p}, states)
	m.search = "feat/x"
	if len(m.rows()) != 1 {
		t.Fatalf("el padre no es visible: %v", rowNames(m.rows()))
	}
	got := worktreeNames(m.entries())
	if len(got) != 1 || got[0] != "wt-feature" {
		t.Errorf("sub-filas = %v, want [wt-feature]", got)
	}

	// Match por basename.
	m2 := newTestModel(t, []discovery.Project{p}, states)
	m2.search = "nombre-raro"
	if got := worktreeNames(m2.entries()); len(got) != 1 || got[0] != "nombre-raro" {
		t.Errorf("sub-filas = %v, want [nombre-raro]", got)
	}
}

// Sin match no se muestra ninguna fila.
func TestWorktreeSearchNoMatch(t *testing.T) {
	p, st := repoWithWorktrees("alpha", "/tmp/alpha", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.search = "zzzz-no-existe"
	if got := m.entries(); len(got) != 0 {
		t.Errorf("entries = %v, want vacío", got)
	}
}

// La expansión inducida por búsqueda es transitoria.
func TestWorktreeSearchTransient(t *testing.T) {
	p, st := repoWithWorktrees("alpha", "/tmp/alpha", wt("/tmp/wt-feature", "feat/x"))

	m := newTestModel(t, []discovery.Project{p}, st)
	m.search = "feat/x"
	if _, ok := m.expanded[p.Path]; ok {
		t.Error("la búsqueda escribió el estado persistido")
	}
	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Fatalf("sub-fila no visible con la búsqueda activa: %v", got)
	}
	m.search = "" // limpiar la búsqueda
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("el repo no volvió a plegado tras limpiar la búsqueda: %v", got)
	}
	if _, ok := m.expanded[p.Path]; ok {
		t.Error("el estado persistido quedó contaminado")
	}
}

// Integración: repo y worktrees git reales (testutil) → sub-filas reales.
func TestWorktreeExpandRealFixture(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtFeat := filepath.Join(t.TempDir(), "wt-feat")
	testutil.MakeWorktree(t, dir, wtFeat, "feat/x")
	wtDetached := filepath.Join(t.TempDir(), "wt-detached")
	testutil.MakeWorktree(t, dir, wtDetached, "otra")
	testutil.Detach(t, wtDetached)

	snap := gitstatus.Collect(t.Context(), dir, "main")
	if len(snap.Worktrees) != 2 {
		t.Fatalf("fixture: worktrees = %d, want 2", len(snap.Worktrees))
	}
	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})

	m, _ = press(m, " ")
	entries := m.entries()
	if got := worktreeNames(entries); len(got) != 2 {
		t.Fatalf("sub-filas reales = %v, want 2", got)
	}
	// La rama real del worktree aparece en la sub-fila.
	var featLine string
	for _, e := range entries {
		if e.kind == kindWorktree && filepath.Base(e.wt.Path) == "wt-feat" {
			featLine = stripANSI(m.renderWorktreeRow(e.wt, false))
		}
	}
	if !strings.Contains(featLine, "feat/x") {
		t.Errorf("rama real no visible: %q", featLine)
	}
	// El worktree detached real muestra (detached).
	var detLine string
	for _, e := range entries {
		if e.kind == kindWorktree && filepath.Base(e.wt.Path) == "wt-detached" {
			detLine = stripANSI(m.renderWorktreeRow(e.wt, false))
		}
	}
	if !strings.Contains(detLine, "(detached)") {
		t.Errorf("detached real no visible: %q", detLine)
	}
}
