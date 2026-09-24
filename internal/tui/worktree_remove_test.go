// Tests del borrado de worktree desde la TUI (modelo directo, sin teatest):
// armado en dos pulsaciones, forzado de segundo nivel, cancelación y limpieza.
package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// removeWtModel construye un modelo con un repo y sus worktrees, lo expande y
// deja el cursor sobre la primera sub-fila de worktree.
func removeWtModel(t *testing.T, parent string, wts ...gitstatus.Worktree) (Model, discovery.Project) {
	t.Helper()
	p, st := repoWithWorktrees(filepath.Base(parent), parent, wts...)
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ") // expandir worktrees
	m, _ = press(m, "down")
	if e, ok := m.selectedEntry(); !ok || e.kind != kindWorktree {
		t.Fatalf("el cursor no quedó sobre una sub-fila: %+v", e)
	}
	return m, p
}

// armedOver arma el estado de confirmación manualmente (para saltarse la
// primera pulsación en los tests del segundo nivel).
func armedOver(t *testing.T, m Model, e tableEntry, force bool) Model {
	t.Helper()
	m.armed = &armedRemoval{
		wtPath: e.wt.Path, parent: e.parent,
		name: filepath.Base(e.wt.Path), force: force,
	}
	return m
}

// waitEvent consume eventos del canal aplicándolos al modelo hasta que uno
// cumpla match (o expire el plazo).
func waitEvent(t *testing.T, m *Model, match func(event) bool) {	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-m.events:
			updated, _ := m.Update(ev)
			*m = updated.(Model)
			if match(ev) {
				return
			}
		case <-deadline:
			t.Fatal("timeout esperando el evento esperado")
		}
	}
}

// applyNotify ejecuta el Cmd de un toast y entrega el mensaje al modelo (los
// toasts no llegan al estado hasta que el runtime ejecuta el Cmd).
func applyNotify(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// La primera D sobre una sub-fila arma la confirmación sin ejecutar nada.
func TestRemoveWorktreeArmOnFirstPress(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	before := m.cursor

	m, cmd := press(m, "D")

	if m.armed == nil {
		t.Fatal("D no armó la confirmación")
	}
	if m.armed.wtPath != "/tmp/wt-a" || m.armed.name != "wt-a" {
		t.Errorf("armed = %+v, want wt-a", m.armed)
	}
	if m.armed.force {
		t.Error("el primer armado no debe forzar")
	}
	if m.armed.parent != p.Path {
		t.Errorf("parent = %q, want %q", m.armed.parent, p.Path)
	}
	if len(m.running) != 0 {
		t.Errorf("no debe haber acciones en curso: %v", m.running)
	}
	if cmd != nil {
		t.Error("armar no debe lanzar ningún Cmd")
	}
	if m.cursor != before {
		t.Errorf("el cursor cambió: %d, want %d", m.cursor, before)
	}
}

// La segunda D lanza el borrado con force=false.
func TestRemoveWorktreeSecondPressExecutes(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))

	m, _ = press(m, "D") // armar
	m, _ = press(m, "D") // ejecutar

	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Fatalf("running[parent] = %q, want worktree_remove", got)
	}
}

// esc cancela la confirmación y NO cierra el detalle.
func TestRemoveWorktreeEscCancels(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D")
	m.detailOpen = true

	m, _ = press(m, "esc")

	if m.armed != nil {
		t.Error("esc no desarmó la confirmación")
	}
	if !m.detailOpen {
		t.Error("esc cerró el detalle pese a estar armado")
	}
	if m.running["/tmp/parent-repo"] != "" {
		t.Error("esc no debe lanzar ninguna acción")
	}
}

// D sobre una fila de repo es no-op con toast info.
func TestRemoveWorktreeOnRepoRowInfo(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st) // sin expandir: cursor en el repo

	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)

	if m.armed != nil {
		t.Error("D sobre una fila de repo no debe armar")
	}
	if len(m.toasts.toasts) == 0 {
		t.Fatal("sin toast")
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastInfo || last.text != "select a worktree" {
		t.Errorf("toast = %+v, want info 'select a worktree'", last)
	}
}

// D sobre un header de grupo o una tabla vacía: mismo toast, sin armado.
func TestRemoveWorktreeOnHeaderOrEmptyInfo(t *testing.T) {
	grouped := discovery.Project{Path: "/tmp/g1", Name: "g1", PrimaryGroup: "g", HasRepo: true}
	m := newTestModel(t, []discovery.Project{grouped},
		map[string]gitstatus.Snapshot{"/tmp/g1": snapClean()})
	if e, _ := m.selectedEntry(); e.kind != kindPrimary {
		t.Fatalf("precondición: entrada = %+v, want header", e)
	}
	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)
	if m.armed != nil {
		t.Error("D sobre un header no debe armar")
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast header = %+v", last)
	}

	// Tabla vacía.
	empty := newTestModel(t, nil, nil)
	empty, cmd = press(empty, "D")
	empty = applyNotify(empty, cmd)
	if empty.armed != nil {
		t.Error("D sobre tabla vacía no debe armar")
	}
	if last := empty.toasts.toasts[len(empty.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast vacío = %+v", last)
	}
}

// Mover el cursor desarma y la navegación se aplica.
func TestRemoveWorktreeCursorMoveDisarms(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m, _ = press(m, "D")
	before := m.cursor

	m, _ = press(m, "down")
	if m.armed != nil {
		t.Error("down no desarmó")
	}
	if m.cursor != before+1 {
		t.Errorf("down no navegó: cursor=%d, want %d", m.cursor, before+1)
	}

	m, _ = press(m, "D")
	m, _ = press(m, "up")
	if m.armed != nil {
		t.Error("up no desarmó")
	}
	if m.cursor != before {
		t.Errorf("up no navegó: cursor=%d, want %d", m.cursor, before)
	}
}

// Cualquier otra tecla desarma (filtro, búsqueda, plegado, expansión).
func TestRemoveWorktreeOtherKeysDisarm(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D")

	// d = filtro dirty
	m2, _ := press(m, "d")
	if m2.armed != nil {
		t.Error("d no desarmó")
	}
	if !m2.onlyDirty {
		t.Error("d no aplicó el filtro dirty")
	}

	// / = búsqueda
	m3, _ := press(m, "/")
	if m3.armed != nil {
		t.Error("/ no desarmó")
	}
	if !m3.searchActive {
		t.Error("/ no abrió la búsqueda")
	}

	// tab sobre la sub-fila: no-op de plegado pero desarma
	m4, _ := press(m, "tab")
	if m4.armed != nil {
		t.Error("tab no desarmó")
	}

	// space sobre la sub-fila: no-op de expansión pero desarma
	m5, _ := press(m, " ")
	if m5.armed != nil {
		t.Error("space no desarmó")
	}
}

// Un fallo del primer intento muestra el motivo de git y arma el forzado.
func TestRemoveWorktreeFailureArmsForce(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.armed = &armedRemoval{wtPath: "/tmp/wt-a", parent: p.Path, name: "wt-a"}
	m.running[p.Path] = "worktree_remove"

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		output: "fatal: contiene archivos modificados", err: "fatal: contiene archivos modificados",
	})
	m = updated.(Model)

	if m.armed == nil || !m.armed.force {
		t.Fatalf("no se armó el forzado: %+v", m.armed)
	}
	if m.armed.wtPath != "/tmp/wt-a" {
		t.Errorf("forzado sobre %q, want wt-a", m.armed.wtPath)
	}
	if m.running[p.Path] != "" {
		t.Error("el fallo no liberó el running del padre")
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "archivos modificados") {
		t.Errorf("toast = %+v, want error con el motivo real", last)
	}
}

// El forzado armado ejecuta con force=true.
func TestRemoveWorktreeForceExecutes(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	e, _ := m.selectedEntry()
	m = armedOver(t, m, e, true)

	m, _ = press(m, "D")

	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Fatalf("running[parent] = %q", got)
	}
	select {
	case ev := <-m.events:
		msg, ok := ev.(worktreeRemovedMsg)
		if !ok {
			t.Fatalf("primer evento = %T, want worktreeRemovedMsg", ev)
		}
		if !msg.force {
			t.Error("la ejecución no fue con force")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no llegó worktreeRemovedMsg")
	}
}

// Un fallo forzado desarma y muestra el error (sin re-armar, sin bucle).
func TestRemoveWorktreeForceFailureDisarms(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.armed = &armedRemoval{wtPath: "/tmp/wt-a", parent: p.Path, name: "wt-a", force: true}
	m.running[p.Path] = "worktree_remove"

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: no se puede", force: true,
	})
	m = updated.(Model)

	if m.armed != nil {
		t.Errorf("el fallo forzado debe desarmar: %+v", m.armed)
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "no se puede") {
		t.Errorf("toast = %+v, want error", last)
	}
}

// El éxito desarma, libera el padre y re-colecciona (la sub-fila desaparece).
// Integración con un repo git real.
func TestRemoveWorktreeSuccessDisarmsAndRecollects(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-real")
	testutil.MakeWorktree(t, dir, wtDir, "wt-real")
	snap := gitstatus.Collect(t.Context(), dir, "main")
	if len(snap.Worktrees) != 1 {
		t.Fatalf("fixture: worktrees = %d, want 1", len(snap.Worktrees))
	}

	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})
	m, _ = press(m, " ")    // expandir
	m, _ = press(m, "down") // sub-fila
	m, _ = press(m, "D")    // armar
	m, _ = press(m, "D")    // ejecutar

	waitEvent(t, &m, func(ev event) bool {
		sm, ok := ev.(statusMsg)
		return ok && sm.path == dir && len(sm.snap.Worktrees) == 0
	})

	if m.armed != nil {
		t.Errorf("el éxito debe desarmar: %+v", m.armed)
	}
	if m.running[dir] != "" {
		t.Errorf("el padre sigue ocupado: %v", m.running[dir])
	}
	if len(m.states[dir].Worktrees) != 0 {
		t.Errorf("el snapshot del padre conserva el worktree: %+v", m.states[dir].Worktrees)
	}
	if got := worktreeNames(m.entries()); len(got) != 0 {
		t.Errorf("la sub-fila sigue visible: %v", got)
	}
	found := false
	for _, to := range m.toasts.toasts {
		if to.level == toastSuccess && strings.Contains(to.text, "worktree removed") {
			found = true
		}
	}
	if !found {
		t.Errorf("sin toast de éxito: %+v", m.toasts.toasts)
	}
}

// Al llegar un snapshot del padre ya sin el worktree, la sub-fila desaparece y
// el cursor queda en rango.
func TestRemoveWorktreeSubrowDisappearsAfterSuccess(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m.cursor = 2 // segunda sub-fila

	sinWt := snapClean()
	sinWt.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	updated, _ := m.Update(statusMsg{path: p.Path, snap: sinWt})
	m = updated.(Model)

	if got := worktreeNames(m.entries()); len(got) != 1 {
		t.Errorf("sub-filas = %v, want [wt-a]", got)
	}
	if n := len(m.entries()); m.cursor >= n {
		t.Errorf("cursor fuera de rango: %d (entries=%d)", m.cursor, n)
	}
}

// Armado/borrado sobre una sub-fila detached (sin rama) es equivalente.
func TestRemoveWorktreeDetachedRow(t *testing.T) {
	det := gitstatus.Worktree{Path: "/tmp/wt-det", Head: "abc1234"}
	m, p := removeWtModel(t, "/tmp/parent-repo", det)

	m, _ = press(m, "D")
	if m.armed == nil || m.armed.name != "wt-det" {
		t.Fatalf("no se armó sobre el detached: %+v", m.armed)
	}
	m, _ = press(m, "D")
	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Errorf("running = %q, want worktree_remove", got)
	}
}

// Una sub-fila con parent vacío no arma: toast info.
func TestRemoveWorktreeNoArmedOnMissingParent(t *testing.T) {
	// El parent de una sub-fila lo fija el path del repo padre; para forzar el
	// caso se usa un proyecto con path vacío.
	p := discovery.Project{Path: "", Name: "sin-path", HasRepo: true}
	s := snapClean()
	s.Worktrees = []gitstatus.Worktree{wt("/tmp/wt-a", "a")}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"": s})
	m.expanded[""] = true
	m, _ = press(m, "down")
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree {
		t.Fatalf("precondición: entrada = %+v", e)
	}
	if e.parent != "" {
		t.Fatalf("precondición: parent = %q, want vacío", e.parent)
	}

	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)

	if m.armed != nil {
		t.Error("no debe armar con parent vacío")
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast = %+v", last)
	}
}

// El aviso persistente aparece en la vista, cambia en el forzado y desaparece
// al cancelar.
func TestRemoveWorktreeBannerRender(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))

	m, _ = press(m, "D")
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "remove worktree wt-a? D to confirm, esc to cancel") {
		t.Errorf("aviso de confirmación ausente:\n%s", out)
	}

	// Variante forzada.
	e, _ := m.selectedEntry()
	m = armedOver(t, m, e, true)
	out = stripANSI(m.View().Content)
	if !strings.Contains(out, "remove worktree wt-a? has changes — D to force, esc to cancel") {
		t.Errorf("aviso forzado ausente:\n%s", out)
	}

	// Cancelar lo retira.
	m, _ = press(m, "esc")
	if strings.Contains(stripANSI(m.View().Content), "remove worktree") {
		t.Error("el aviso sigue visible tras cancelar")
	}
}

// El binding es configurable: la nueva tecla arma/ejecuta y D deja de hacerlo.
func TestRemoveWorktreeBindingOverride(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.cfg.Keybindings["worktree_remove"] = "W"

	m, _ = press(m, "D")
	if m.armed != nil {
		t.Fatal("D sigue armando tras el rebind")
	}
	m, _ = press(m, "W")
	if m.armed == nil {
		t.Fatal("W no armó tras el rebind")
	}
	m, _ = press(m, "W")
	if got := m.running[p.Path]; got != "worktree_remove" {
		t.Errorf("W no ejecutó: running = %q", got)
	}
}

// Con el padre ocupado, la ejecución no lanza y avisa (el armado no se pierde).
func TestRemoveWorktreeBusyParentWarns(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.running[p.Path] = "pull"

	m, _ = press(m, "D") // armar permitido aunque el padre esté ocupado
	if m.armed == nil {
		t.Fatal("el armado no debe bloquearse por el padre ocupado")
	}

	m, cmd := press(m, "D") // ejecutar: bloqueado por el guard
	if got := m.running[p.Path]; got != "pull" {
		t.Errorf("running = %q, want pull (sin lanzar)", got)
	}
	if m.armed == nil {
		t.Error("el armado no debe romperse silenciosamente")
	}
	if cmd == nil {
		t.Fatal("se esperaba el toast de aviso")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || msg.level != toastWarning {
		t.Errorf("aviso = %+v, want warning", msg)
	}
}
