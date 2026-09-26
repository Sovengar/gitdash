// Tests del borrado de worktree desde la TUI (modelo directo, sin teatest):
// armado en dos pulsaciones, forzado de segundo nivel, cancelación y limpieza.
package tui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// gitOutT ejecuta git en dir y devuelve stdout (para comprobar la rama).
func gitOutT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

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
func waitEvent(t *testing.T, m *Model, match func(event) bool) {
	t.Helper()
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
	// Estado tras la 2ª D: banner limpio y token del intento en vuelo.
	m.armed = nil
	m.removeTokens[p.Path] = 7
	m.running[p.Path] = "worktree_remove"

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		output: "fatal: contiene archivos modificados", err: "fatal: contiene archivos modificados",
		gen: 7,
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
	if _, ok := m.removeTokens[p.Path]; ok {
		t.Errorf("el token debe consumirse: %v", m.removeTokens)
	}
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "archivos modificados") {
		t.Errorf("toast = %+v, want error con el motivo real", last)
	}
	// El detalle conserva la salida/motivo de git.
	if act := m.lastAction[p.Path]; act.kind != "worktree_remove" || act.err == "" {
		t.Errorf("lastAction = %+v, want kind worktree_remove con error", act)
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
	// Estado tras lanzar el forzado: banner limpio y token en vuelo.
	m.armed = nil
	m.removeTokens[p.Path] = 9
	m.running[p.Path] = "worktree_remove"

	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: no se puede", force: true, gen: 9,
	})
	m = updated.(Model)

	if m.armed != nil {
		t.Errorf("el fallo forzado no debe re-armar: %+v", m.armed)
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

// El armado valida la selección al confirmar: si la sub-fila armada ya no es
// la actual, se re-arma sobre la actual en vez de borrar otro worktree.
func TestRemoveWorktreeArmedRevalidatesSelection(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m, _ = press(m, "D") // armado sobre wt-a

	// La selección cambia sin pasar por el desarmado de teclas (p. ej. la
	// sub-fila armada desaparece y el cursor cae en otro worktree).
	m.cursor = 2

	m, _ = press(m, "D")
	if m.running[p.Path] != "" {
		t.Errorf("borró el worktree armado pese a cambiar la selección: %v", m.running)
	}
	if m.armed == nil || m.armed.wtPath != "/tmp/wt-b" {
		t.Errorf("no se re-armó sobre la sub-fila actual: %+v", m.armed)
	}
}

// El aviso persistente se pinta en la sección de keybinds, cambia en el forzado
// y desaparece al cancelar.
func TestRemoveWorktreePromptRender(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))

	m, _ = press(m, "D")
	out := stripANSI(m.View().Content)
	if !strings.Contains(sectionContent(t, out, "keybinds"), "remove worktree wt-a? D to confirm, esc to cancel") {
		t.Errorf("aviso de confirmación ausente de keybinds:\n%s", out)
	}

	// Variante forzada.
	e, _ := m.selectedEntry()
	m = armedOver(t, m, e, true)
	out = stripANSI(m.View().Content)
	if !strings.Contains(sectionContent(t, out, "keybinds"), "remove worktree wt-a? has changes — D to force, esc to cancel") {
		t.Errorf("aviso forzado ausente de keybinds:\n%s", out)
	}

	// Cancelar lo retira y devuelve las hints.
	m, _ = press(m, "esc")
	out = stripANSI(m.View().Content)
	if strings.Contains(out, "remove worktree") {
		t.Error("el aviso sigue visible tras cancelar")
	}
	if !strings.Contains(sectionContent(t, out, "keybinds"), "j/k move") {
		t.Errorf("las hints no volvieron tras cancelar:\n%s", out)
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

// esc durante un borrado en vuelo lo cancela de forma definitiva: el resultado
// tardío de un fallo sucio no re-arma el forzado.
func TestRemoveWorktreeEscDuringFlightIgnoresLateFailure(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m, _ = press(m, "D") // armar
	m, _ = press(m, "D") // lanzar

	if m.armed != nil {
		t.Fatal("el banner debe limpiarse al lanzar el borrado")
	}
	token, inflight := m.removeTokens[p.Path]
	if !inflight || token == 0 {
		t.Fatalf("sin token de intento en vuelo: %v", m.removeTokens)
	}

	m, _ = press(m, "esc") // cancelación definitiva
	if len(m.removeTokens) != 0 {
		t.Fatalf("esc no invalidó el intento en vuelo: %v", m.removeTokens)
	}

	// Llega tarde el fallo sucio del intento cancelado.
	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: sucio", gen: token,
	})
	m = updated.(Model)

	if m.armed != nil {
		t.Errorf("un fallo tardío tras esc no debe re-armar: %+v", m.armed)
	}
	if m.running[p.Path] != "" {
		t.Error("el running del padre debe liberarse al llegar el resultado")
	}
}

// El resultado tardío de un borrado sobre A no pisa el armado sobre B.
func TestRemoveWorktreeLateResultDoesNotClobberOtherArmed(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m, _ = press(m, "D") // armar A
	m, _ = press(m, "D") // lanzar A
	token := m.removeTokens[p.Path]

	// El usuario se mueve a B y lo arma mientras A está en vuelo.
	m, _ = press(m, "down")
	m, _ = press(m, "D")
	if m.armed == nil || m.armed.name != "wt-b" {
		t.Fatalf("no se armó B: %+v", m.armed)
	}

	// Llega tarde el fallo sucio de A.
	updated, _ := m.Update(worktreeRemovedMsg{
		parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
		err: "fatal: sucio A", gen: token,
	})
	m = updated.(Model)

	if m.armed == nil || m.armed.name != "wt-b" || m.armed.force {
		t.Errorf("el resultado tardío de A pisó el armado de B: %+v", m.armed)
	}
}

// D sobre un header secundario es no-op con toast info.
func TestRemoveWorktreeOnSecondaryHeaderInfo(t *testing.T) {
	p := discovery.Project{
		Path: "/s", Name: "s", HasRepo: true,
		PrimaryGroup: "g", SecondaryGroup: "sub",
	}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{"/s": snapClean()})
	m.cursor = 1
	if e, _ := m.selectedEntry(); e.kind != kindSecondary {
		t.Fatalf("precondición: entrada = %+v, want header secundario", e)
	}

	m, cmd := press(m, "D")
	m = applyNotify(m, cmd)

	if m.armed != nil {
		t.Error("D sobre un header secundario no debe armar")
	}
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastInfo ||
		last.text != "select a worktree" {
		t.Errorf("toast = %+v, want info 'select a worktree'", last)
	}
}

// `!` (modo comando) desarma la confirmación y abre el input.
func TestRemoveWorktreeCommandKeyDisarms(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.detailOpen = true
	m, _ = press(m, "D") // armar

	m, _ = press(m, "!")

	if m.armed != nil {
		t.Error("! no desarmó la confirmación")
	}
	if !m.cmdOpen {
		t.Error("! no abrió el modo comando")
	}
}

// End-to-end con un repo real: worktree sucio → fallo sin force → forzado →
// éxito, con la rama intacta.
func TestRemoveWorktreeDirtyForceSuccessEndToEnd(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-dirty")
	testutil.MakeWorktree(t, dir, wtDir, "wt-dirty")
	testutil.WriteUntracked(t, wtDir, map[string]string{"pendiente.txt": "x"})
	snap := gitstatus.Collect(t.Context(), dir, "main")
	if len(snap.Worktrees) != 1 {
		t.Fatalf("fixture: worktrees = %d, want 1", len(snap.Worktrees))
	}

	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	m, _ = press(m, "D") // armar
	m, _ = press(m, "D") // intento sin force → falla

	waitEvent(t, &m, func(ev event) bool {
		mm, ok := ev.(worktreeRemovedMsg)
		return ok && mm.err != "" && !mm.force
	})
	if m.armed == nil || !m.armed.force {
		t.Fatalf("no se armó el forzado tras el fallo sucio: %+v", m.armed)
	}

	m, _ = press(m, "D") // forzar → éxito
	waitEvent(t, &m, func(ev event) bool {
		sm, ok := ev.(statusMsg)
		return ok && sm.path == dir && len(sm.snap.Worktrees) == 0
	})

	if m.armed != nil {
		t.Errorf("el éxito no desarmó: %+v", m.armed)
	}
	if m.running[dir] != "" {
		t.Errorf("el padre sigue ocupado: %v", m.running[dir])
	}
	if len(m.states[dir].Worktrees) != 0 {
		t.Errorf("el snapshot conserva el worktree: %+v", m.states[dir].Worktrees)
	}
	if branches := gitOutT(t, dir, "branch", "--list", "wt-dirty"); !strings.Contains(branches, "wt-dirty") {
		t.Errorf("la rama wt-dirty desapareció: %q", branches)
	}
}

// twoParentModel construye un modelo con dos repos (a, b) y un worktree cada
// uno, ambos expandidos. Las entradas navegables son [a, a-wt, b, b-wt].
func twoParentModel(t *testing.T) Model {
	t.Helper()
	base := time.Now().Add(-2 * time.Hour).Unix()
	snapA := snapClean()
	snapA.LastCommit = base
	snapA.Worktrees = []gitstatus.Worktree{wt("/tmp/pa-wt", "a")}
	snapB := snapClean()
	snapB.LastCommit = base
	snapB.Worktrees = []gitstatus.Worktree{wt("/tmp/pb-wt", "b")}
	pA := proj("a", "/tmp/pa", true)
	pB := proj("b", "/tmp/pb", true)
	m := newTestModel(t, []discovery.Project{pA, pB},
		map[string]gitstatus.Snapshot{"/tmp/pa": snapA, "/tmp/pb": snapB})
	m.expanded["/tmp/pa"] = true
	m.expanded["/tmp/pb"] = true
	return m
}

// Con dos borrados solapados en padres distintos, el resultado tardío del
// primero libera su running (sin fuga) y se resuelve, sin tocar el intento del
// segundo. El token es por padre, no global.
func TestRemoveWorktreeConcurrentParentsNoLeak(t *testing.T) {
	m := twoParentModel(t)

	// Entradas: [a, a-wt, b, b-wt]; se lanza el borrado de A.
	m.cursor = 1
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokA := m.removeTokens["/tmp/pa"]
	if tokA == 0 {
		t.Fatalf("A no quedó en vuelo: %v", m.removeTokens)
	}

	// Se lanza el borrado de B (padre distinto, en paralelo).
	m.cursor = 3
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokB := m.removeTokens["/tmp/pb"]
	if tokB == 0 || tokB == tokA {
		t.Fatalf("tokens no independientes: A=%d B=%d", tokA, tokB)
	}

	// Llega tarde el fallo sucio de A.
	updated, _ := m.Update(worktreeRemovedMsg{
		parent: "/tmp/pa", wtPath: "/tmp/pa-wt", name: "pa-wt",
		err: "fatal: sucio A", gen: tokA,
	})
	m = updated.(Model)

	if m.running["/tmp/pa"] != "" {
		t.Errorf("fuga de running en A: %q", m.running["/tmp/pa"])
	}
	if m.running["/tmp/pb"] != "worktree_remove" {
		t.Errorf("el intento de B se alteró: %q", m.running["/tmp/pb"])
	}
	if _, ok := m.removeTokens["/tmp/pa"]; ok {
		t.Errorf("token de A no consumido: %v", m.removeTokens)
	}
	if m.removeTokens["/tmp/pb"] != tokB {
		t.Errorf("token de B alterado: %v", m.removeTokens)
	}
	// El desenlace de A se maneja: fallo sucio → armado de forzado sobre A.
	if m.armed == nil || !m.armed.force || m.armed.name != "pa-wt" {
		t.Errorf("desenlace de A no aplicado: %+v", m.armed)
	}

	// El resultado de B sigue resolviéndose con normalidad.
	updated, _ = m.Update(worktreeRemovedMsg{
		parent: "/tmp/pb", wtPath: "/tmp/pb-wt", name: "pb-wt", gen: tokB,
	})
	m = updated.(Model)
	if m.running["/tmp/pb"] != "" {
		t.Errorf("fuga de running en B tras el éxito: %q", m.running["/tmp/pb"])
	}
	if _, ok := m.removeTokens["/tmp/pb"]; ok {
		t.Errorf("token de B no consumido: %v", m.removeTokens)
	}
}

// esc cancela A y después se lanza B en otro padre: el resultado tardío de A
// no debe fugar el running de A ni tocar el intento de B.
func TestRemoveWorktreeEscThenOtherParentNoLeak(t *testing.T) {
	m := twoParentModel(t)

	m.cursor = 1
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokA := m.removeTokens["/tmp/pa"]
	if tokA == 0 {
		t.Fatalf("A no quedó en vuelo: %v", m.removeTokens)
	}

	m, _ = press(m, "esc") // cancelación definitiva de A
	if len(m.removeTokens) != 0 {
		t.Fatalf("esc no limpió los tokens: %v", m.removeTokens)
	}

	m.cursor = 3
	m, _ = press(m, "D")
	m, _ = press(m, "D")
	tokB := m.removeTokens["/tmp/pb"]
	if tokB == 0 {
		t.Fatalf("B no quedó en vuelo: %v", m.removeTokens)
	}

	// Llega tarde el resultado de A (cancelado): libera su running y no toca B.
	updated, _ := m.Update(worktreeRemovedMsg{
		parent: "/tmp/pa", wtPath: "/tmp/pa-wt", name: "pa-wt",
		err: "fatal: sucio A", gen: tokA,
	})
	m = updated.(Model)

	if m.running["/tmp/pa"] != "" {
		t.Errorf("fuga de running en A: %q", m.running["/tmp/pa"])
	}
	if m.running["/tmp/pb"] != "worktree_remove" {
		t.Errorf("el intento de B se alteró: %q", m.running["/tmp/pb"])
	}
	if m.removeTokens["/tmp/pb"] != tokB {
		t.Errorf("token de B alterado: %v", m.removeTokens)
	}
	if m.armed != nil {
		t.Errorf("un resultado cancelado no debe armar: %+v", m.armed)
	}
}

// matches normaliza los paths: barras finales, `./` y duplicadas no impiden
// reconocer el mismo worktree.
func TestRemoveWorktreeArmedMatchesNormalization(t *testing.T) {
	a := armedRemoval{parent: "/tmp/p", wtPath: "/tmp/p/wt"}
	cases := []struct {
		parent, wtPath string
		want           bool
	}{
		{"/tmp/p", "/tmp/p/wt", true},
		{"/tmp/p/", "/tmp/p/wt/", true},
		{"/tmp/p/.", "/tmp/p/./wt", true},
		{"/tmp/p", "/tmp/p//wt", true},
		{"/tmp/other", "/tmp/p/wt", false},
		{"/tmp/p", "/tmp/p/otro", false},
	}
	for _, tc := range cases {
		if got := a.matches(tc.parent, tc.wtPath); got != tc.want {
			t.Errorf("matches(%q, %q) = %v, want %v", tc.parent, tc.wtPath, got, tc.want)
		}
	}
	// Un worktree con el mismo basename bajo otro padre no matchea.
	b := armedRemoval{parent: "/tmp/p2", wtPath: "/tmp/p2/wt"}
	if b.matches("/tmp/p", "/tmp/p/wt") {
		t.Error("matches cruzó padres distintos")
	}
}

// Un resultado con token sustituido (t1 obsoleto tras relanzar t2) se ignora
// por completo: no libera el running del intento nuevo, no toca el banner y no
// emite toast. Es la colisión de un statusMsg de fondo que libera
// running[parent] con un borrado en vuelo y permite relanzar.
func TestRemoveWorktreeSubstitutedTokenIgnored(t *testing.T) {
	cases := []struct {
		name string
		err  string
	}{
		{"resultado de fallo obsoleto", "fatal: sucio t1"},
		{"resultado de éxito obsoleto", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
			// Intento nuevo (t2) en vuelo; el banner está armado sobre otro
			// worktree y no debe tocarse.
			m.removeTokens[p.Path] = 2
			m.running[p.Path] = "worktree_remove"
			m.armed = &armedRemoval{wtPath: "/tmp/wt-b", parent: p.Path, name: "wt-b"}

			before := len(m.toasts.toasts)
			updated, _ := m.Update(worktreeRemovedMsg{
				parent: p.Path, wtPath: "/tmp/wt-a", name: "wt-a",
				output: "salida de t1", err: tc.err, gen: 1, // t1 < t2: obsoleto
			})
			m = updated.(Model)

			if m.running[p.Path] != "worktree_remove" {
				t.Errorf("se liberó el running del intento nuevo: %q", m.running[p.Path])
			}
			if m.removeTokens[p.Path] != 2 {
				t.Errorf("se alteró el token vigente: %v", m.removeTokens)
			}
			if m.armed == nil || m.armed.name != "wt-b" {
				t.Errorf("se tocó el banner: %+v", m.armed)
			}
			if len(m.toasts.toasts) != before {
				t.Errorf("se emitió un toast para un resultado obsoleto: %+v", m.toasts.toasts)
			}
			if _, ok := m.lastAction[p.Path]; ok {
				t.Errorf("se guardó lastAction de un resultado obsoleto: %+v", m.lastAction[p.Path])
			}
		})
	}
}
