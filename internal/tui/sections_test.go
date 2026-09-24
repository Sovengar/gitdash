// Tests del layout por secciones bordadas, el presupuesto de alto y la
// migración del feedback de acciones a toasts.
package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
) // Cada área funcional se dibuja en su propia sección bordeada, todas con el
// ancho exterior de la terminal.
func TestDashboardSeccionesBordeadas(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	out := m.renderDashboard()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Errorf("líneas = %d, want %d (alto exacto de la terminal)", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
	plano := stripANSI(out)
	for _, title := range []string{"╭ gitdash ", "╭ repos ", "╭ keybinds "} {
		if !strings.Contains(plano, title) {
			t.Errorf("falta la sección %q:\n%s", title, plano)
		}
	}
	for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
		if !strings.Contains(plano, glyph) {
			t.Errorf("falta el glifo redondeado %q", glyph)
		}
	}
}

// La tabla scrollea dentro de su sección: la fila bajo el cursor permanece
// visible y las demás secciones siguen presentes.
func TestTablaScrolleaEnSuSeccion(t *testing.T) {
	var projects []discovery.Project
	states := map[string]gitstatus.Snapshot{}
	for i := 0; i < 20; i++ {
		p := proj(fmt.Sprintf("repo%02d", i), fmt.Sprintf("/tmp/repo%02d", i), true)
		projects = append(projects, p)
		states[p.Path] = snapClean()
	}
	m := newTestModel(t, projects, states)
	m.height = 20
	m.cursor = len(m.entries()) - 1

	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "repo19") {
		t.Errorf("la fila bajo el cursor no está visible:\n%s", out)
	}
	if !strings.Contains(out, "╭ gitdash ") || !strings.Contains(out, "╭ keybinds ") {
		t.Errorf("las demás secciones no siguen visibles:\n%s", out)
	}
}

// Terminal baja degrada secciones en orden antes de romper el layout.
func TestLayoutDegradaEnTerminalBaja(t *testing.T) {
	lay := computeLayout(40, false, false)
	if !lay.showStats || !lay.showKeybinds || lay.hintLines != 3 {
		t.Errorf("altura amplia: %+v, want todo visible", lay)
	}
	if lay.bodyLines != 40-(statsSectionLines+tableChrome+keybindsChrome+3) {
		t.Errorf("bodyLines = %d, want %d", lay.bodyLines, 40-(statsSectionLines+tableChrome+keybindsChrome+3))
	}

	// altura intermedia: se recortan las hints antes de ocultar secciones
	mid := computeLayout(10, false, false)
	if mid.hintLines != 1 || !mid.showKeybinds {
		t.Errorf("h=10: %+v, want keybinds con 1 hint", mid)
	}

	// más baja: keybinds fuera, stats aún visible
	baja := computeLayout(9, false, false)
	if baja.showKeybinds || !baja.showStats {
		t.Errorf("h=9: %+v, want keybinds oculto y stats visible", baja)
	}

	// muy baja: stats fuera; la tabla conserva al menos una fila
	for h := 0; h <= 8; h++ {
		l := computeLayout(h, false, false)
		if l.bodyLines < 1 {
			t.Errorf("h=%d: bodyLines = %d, want >= 1", h, l.bodyLines)
		}
		if h <= 6 && l.showStats {
			t.Errorf("h=%d: stats visible en terminal demasiado baja: %+v", h, l)
		}
	}
}

// Terminal estrecha: el contenido se recorta al interior, sin wrap ni cajas
// más altas que el presupuesto.
func TestTerminalEstrechaNoRompe(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 24

	out := m.renderDashboard()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Errorf("líneas = %d, want %d (sin wrap)", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
}

// La sección de filtro aparece solo con filtro activo o confirmado.
func TestSeccionFiltroCondicional(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	if strings.Contains(stripANSI(m.renderDashboard()), "╭ filter ") {
		t.Error("la sección de filtro se ve sin filtro activo")
	}

	m, _ = press(m, "/")
	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "╭ filter ") {
		t.Errorf("no apareció la sección de filtro:\n%s", out)
	}
	if !strings.Contains(out, "[/") || !strings.Contains(out, "name/group") {
		t.Errorf("el input en vivo no se muestra:\n%s", out)
	}

	m, _ = press(m, "a")
	m, _ = press(m, "enter")
	if out := stripANSI(m.renderDashboard()); !strings.Contains(out, "[/a]") {
		t.Errorf("el filtro confirmado no se muestra:\n%s", out)
	}

	// limpiar el filtro oculta la sección y devuelve el alto a la tabla
	m, _ = press(m, "/")
	m, _ = press(m, "backspace")
	m, _ = press(m, "esc")
	if strings.Contains(stripANSI(m.renderDashboard()), "╭ filter ") {
		t.Error("la sección de filtro no desapareció al limpiar")
	}
}

// El detalle se envuelve en una sección bordeada con título del repo; la
// navegación y el cierre (esc) siguen funcionando.
func TestDetalleSeccionBordada(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "enter") // dirty-api es la primera fila
	if !m.detailOpen {
		t.Fatal("enter no abrió el detalle")
	}
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "╭ dirty-api ") {
		t.Errorf("el título del borde no identifica el repo:\n%s", out)
	}
	if !strings.Contains(out, "╭ gitdash ") || !strings.Contains(out, "╭ keybinds ") {
		t.Errorf("stats/keybinds no acompañan al detalle:\n%s", out)
	}

	m, _ = press(m, "esc")
	if m.detailOpen {
		t.Error("esc no cerró el detalle")
	}
}

// El título de la sección de detalle identifica el repo con su grupo y, en
// una sub-fila de worktree, con la marca [worktree].
func TestDetalleTituloIdentificaRepo(t *testing.T) {
	grouped := discovery.Project{Path: "/x", Name: "api", PrimaryGroup: "backend", HasRepo: true}
	m := newTestModel(t, []discovery.Project{grouped},
		map[string]gitstatus.Snapshot{"/x": snapClean()})
	m, _ = press(m, "down") // saltar el header de grupo
	m, _ = press(m, "enter")
	if out := stripANSI(m.View().Content); !strings.Contains(out, "╭ api · backend ") {
		t.Errorf("el título no incluye el grupo:\n%s", out)
	}

	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m = newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, " ")
	m, _ = press(m, "down")
	m, _ = press(m, "enter")
	if out := stripANSI(m.View().Content); !strings.Contains(out, "╭ wt-a [worktree] ") {
		t.Errorf("el título no marca el worktree:\n%s", out)
	}
}

// El indicador de actividad sobrevive a anchos estrechos (va primero en stats).
func TestIndicadorActividadAnchoEstrecho(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 40
	m.running["/tmp/old-clean"] = "pull"

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "working") {
		t.Errorf("el indicador de acción en curso no sobrevive a width=40:\n%s", out)
	}
	for i, l := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d", i, w, m.width)
		}
	}
}

// El fallo de una acción conserva el hint accionable en el toast renderizado,
// incluso cuando el mensaje excede el ancho máximo del toast.
func TestToastDeFalloConHintVisible(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	updated, _ := m.Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull",
		output: "fatal: Not possible to fast-forward, aborting.",
		err:    "exit 1",
	})
	m = updated.(Model)

	rendered := collapse(strings.Join(m.toasts.lines(), "\n"))
	if !strings.Contains(rendered, "pull --rebase manual") {
		t.Errorf("el hint no queda visible en el toast renderizado:\n%s", rendered)
	}
}

// La línea de notificación permanente ya no existe.
func TestSinLineaPermanenteDeNotificacion(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := stripANSI(m.renderDashboard())
	if strings.Contains(out, strings.Repeat("─", m.width)) {
		t.Errorf("se sigue dibujando el separador de la barra inferior:\n%s", out)
	}

	// una notificación no deja línea permanente: expira y desaparece
	updated, _ := m.Update(notifyMsg{text: "boom", level: toastError})
	m = updated.(Model)
	if len(m.toasts.toasts) != 1 {
		t.Fatalf("la notificación no se convirtió en toast")
	}
	m.toasts.toasts[0].created = time.Now().Add(-toastDuration - time.Second)
	m.toasts.update()
	if strings.Contains(stripANSI(m.View().Content), "boom") {
		t.Error("la notificación quedó en pantalla de forma permanente")
	}
}

// Una acción terminada genera un toast con el nivel correcto.
func TestAccionGeneraToast(t *testing.T) {
	projects, states := fixtureProjects()

	ok, _ := newTestModel(t, projects, states).Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull", output: "",
	})
	m := ok.(Model)
	if len(m.toasts.toasts) != 1 || m.toasts.toasts[0].level != toastSuccess {
		t.Fatalf("acción ok: %+v, want toast de éxito", m.toasts.toasts)
	}

	ko, _ := newTestModel(t, projects, states).Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull", output: "error: divergent branches", err: "exit 1",
	})
	m = ko.(Model)
	if len(m.toasts.toasts) != 1 || m.toasts.toasts[0].level != toastError {
		t.Fatalf("acción ko: %+v, want toast de error", m.toasts.toasts)
	}
	if txt := m.toasts.toasts[0].text; !strings.Contains(txt, "failed") || !strings.Contains(txt, "diverged") {
		t.Errorf("el toast de error no trae motivo/hint: %q", txt)
	}
}

// Las acciones en curso siguen visibles dentro de la sección de stats.
func TestAccionEnCursoVisibleEnStats(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.running["/tmp/old-clean"] = "pull"

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "pull old-clean") {
		t.Errorf("la acción en curso no se muestra en stats:\n%s", out)
	}
}

// Un fallo de fetch o un error de roots se reportan como toast, sin línea
// permanente en el dashboard.
func TestErroresComoToast(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m2, _ := m.Update(fetchStateMsg{path: "/tmp/old-clean", state: "failed", err: "no route"})
	m = m2.(Model)
	if last := m.toasts.toasts[len(m.toasts.toasts)-1]; last.level != toastError || !strings.Contains(last.text, "no route") {
		t.Errorf("fetch failed no generó toast de error: %+v", last)
	}

	m3, _ := m.Update(scanProjectsMsg{projects: projects, note: "roots ilegibles"})
	m = m3.(Model)
	last := m.toasts.toasts[len(m.toasts.toasts)-1]
	if last.level != toastError || !strings.Contains(last.text, "roots ilegibles") {
		t.Errorf("error de roots no generó toast de error: %+v", last)
	}
	if strings.Contains(stripANSI(m.renderDashboard()), "roots ilegibles") {
		t.Error("el error de roots quedó como línea permanente")
	}
}

// Los toasts se apilan en overlay y no tapan la sección de keybinds.
func TestToastsNoTapanKeybinds(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	for i := 0; i < 3; i++ {
		m.toasts.showInfo(fmt.Sprintf("evento %d", i))
	}

	out := stripANSI(m.View().Content)
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("líneas = %d, want %d", len(lines), m.height)
	}
	if !strings.Contains(out, "╭ keybinds ") || !strings.Contains(out, "╰") {
		t.Errorf("la sección de keybinds se degradó:\n%s", out)
	}
	for _, hint := range []string{"j/k move", "f fetch", "q quit"} {
		if !strings.Contains(out, hint) {
			t.Errorf("hint %q tapada por los toasts:\n%s", hint, out)
		}
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(out, fmt.Sprintf("evento %d", i)) {
			t.Errorf("falta el toast %d apilado:\n%s", i, out)
		}
	}
}

// Los toasts expiran solos en el tick de 1s, sin timers nuevos.
func TestToastsExpiranConElTick(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.toasts.showInfo("hola")

	updated, cmd := m.Update(tickMsg{})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("el tick no se rearmó")
	}
	m.toasts.toasts[0].created = time.Now().Add(-toastDuration - time.Second)
	updated, _ = m.Update(tickMsg{})
	m = updated.(Model)
	if len(m.toasts.toasts) != 0 {
		t.Errorf("el toast no expiró con el tick: %d vivos", len(m.toasts.toasts))
	}
}
