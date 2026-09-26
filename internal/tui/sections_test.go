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

// Terminal baja degrada secciones en orden antes de romper el layout. El panel
// de preview es lo primero que cede (se encoge hasta su share y después se va
// entero), y mientras está la tabla no baja de minBodyLines.
func TestLayoutDegradaEnTerminalBaja(t *testing.T) {
	// Altura amplia: todo visible y el panel con su share del alto libre.
	wide := computeLayout(40, false, defaultHintLines, false)
	if !wide.showStats || !wide.showKeybinds || wide.hintLines != defaultHintLines {
		t.Errorf("altura amplia: %+v, want todo visible", wide)
	}
	if wide.previewLines < minPreviewLines {
		t.Errorf("previewLines = %d, want >= %d", wide.previewLines, minPreviewLines)
	}
	if want := 40 - (statsSectionLines + tableChrome + keybindsChrome +
		defaultHintLines + previewChrome + wide.previewLines); wide.bodyLines != want {
		t.Errorf("bodyLines = %d, want %d", wide.bodyLines, want)
	}

	// con menos hints configuradas, la reserva no sobra alto (LOW: reserva vs
	// HintBarLines): el panel crece con el hueco que dejan.
	pocas := computeLayout(40, false, 1, false)
	if pocas.hintLines != 1 {
		t.Errorf("keybindsLines=1: hintLines = %d, want 1", pocas.hintLines)
	}
	if pocas.previewLines <= wide.previewLines {
		t.Errorf("con 1 hint el panel = %d, want > %d (se lleva el hueco libre)",
			pocas.previewLines, wide.previewLines)
	}

	// altura intermedia: se recortan las hints antes de ocultar secciones
	mid := computeLayout(10, false, defaultHintLines, false)
	if mid.hintLines != 1 || !mid.showKeybinds {
		t.Errorf("h=10: %+v, want keybinds con 1 hint", mid)
	}

	// más baja: keybinds fuera, stats aún visible
	baja := computeLayout(9, false, defaultHintLines, false)
	if baja.showKeybinds || !baja.showStats {
		t.Errorf("h=9: %+v, want keybinds oculto y stats visible", baja)
	}

	// muy baja: stats fuera; la tabla conserva al menos una fila
	for h := 0; h <= 8; h++ {
		l := computeLayout(h, false, defaultHintLines, false)
		if l.bodyLines < 1 {
			t.Errorf("h=%d: bodyLines = %d, want >= 1", h, l.bodyLines)
		}
		if h <= 6 && l.showStats {
			t.Errorf("h=%d: stats visible en terminal demasiado baja: %+v", h, l)
		}
	}

	// con keepKeybinds (aviso armado), keybinds nunca se degrada: es la única
	// fuente de las teclas que espera la app. Se recortan stats y panel antes.
	for h := 0; h <= 8; h++ {
		l := computeLayout(h, false, 1, true)
		if !l.showKeybinds || l.hintLines != 1 {
			t.Errorf("h=%d con keepKeybinds: keybinds degradada: %+v", h, l)
		}
		if l.bodyLines < 1 {
			t.Errorf("h=%d con keepKeybinds: bodyLines = %d, want >= 1", h, l.bodyLines)
		}
	}
	// Y sin keepKeybinds se comporta como antes: degrada antes de crunchar.
	for h := 0; h <= 8; h++ {
		if l := computeLayout(h, false, 1, false); l.hintLines != 0 || l.showKeybinds {
			t.Errorf("h=%d sin keepKeybinds: keybinds debería caerse: %+v", h, l)
		}
	}
}

// El panel se va antes que las secciones que ya existían: por debajo del alto en
// el que cabe sin costarle nada, el dashboard es exactamente el que había antes
// de la feature.
func TestPreviewPanelEsLoUltimoEnCaerse(t *testing.T) {
	// Con room: panel + todo lo demás.
	if l := computeLayout(30, false, defaultHintLines, false); l.previewLines == 0 {
		t.Errorf("h=30: sin panel: %+v", l)
	}
	// El panel más pequeño que entra es el de la cabecera de la ficha (6), y solo
	// si a la tabla le quedan minBodyLines filas.
	if l := computeLayout(22, false, defaultHintLines, false); l.previewLines != detailHeadLines {
		t.Errorf("h=22: previewLines = %d, want %d", l.previewLines, detailHeadLines)
	}
	// Por debajo, nada de panel y el reparto de siempre: hints y stats intactos y
	// la tabla con lo que sobra.
	for h := 14; h <= 21; h++ {
		l := computeLayout(h, false, defaultHintLines, false)
		if l.previewLines != 0 {
			t.Errorf("h=%d: previewLines = %d, want 0 (el panel no puede pedir más)", h, l.previewLines)
		}
		if !l.showStats || !l.showKeybinds || l.hintLines != defaultHintLines {
			t.Errorf("h=%d: el panel se llevó algo que ya existía: %+v", h, l)
		}
		want := h - (statsSectionLines + tableChrome + keybindsChrome + defaultHintLines)
		if l.bodyLines != want {
			t.Errorf("h=%d: bodyLines = %d, want %d", h, l.bodyLines, want)
		}
	}
	// Y con el panel presente nunca se recorta una hint ni se oculta una sección.
	for h := 22; h <= 80; h++ {
		l := computeLayout(h, false, defaultHintLines, false)
		if l.previewLines == 0 {
			continue
		}
		if !l.showStats || !l.showKeybinds || l.hintLines != defaultHintLines {
			t.Errorf("h=%d: panel con previewLines=%d pero el resto degradado: %+v", h, l.previewLines, l)
		}
		if l.bodyLines < minBodyLines {
			t.Errorf("h=%d: panel con previewLines=%d y bodyLines=%d: %+v", h, l.previewLines, l.bodyLines, l)
		}
	}
}

// La suma de las secciones tiene que dar la altura de la terminal en cualquier
// alto y combinación de flags: si no, alguna caja se sale de la pantalla (o
// empuja los keybinds fuera). La única excepción es el suelo del cuerpo central
// (una fila aunque no quede nada más), y por eso la exactitud se exige solo
// cuando el cuerpo central no está en ese suelo.
func TestLayoutAltoExactoEnTodasLasAlturas(t *testing.T) {
	for h := 0; h <= 60; h++ {
		for _, tc := range []struct {
			name      string
			hasFilter bool
			keybinds  int
			keep      bool
		}{
			{"dashboard", false, defaultHintLines, false},
			{"filtro", true, defaultHintLines, false},
			{"armado", false, 1, true},
			{"armado+filtro", true, 1, true},
			{"sin hints", false, 0, false},
		} {
			l := computeLayout(h, tc.hasFilter, tc.keybinds, tc.keep)
			total := l.altoTotal(tc.hasFilter)
			if l.bodyLines < 1 {
				t.Errorf("%s h=%d: bodyLines = %d, want >= 1", tc.name, h, l.bodyLines)
			}
			if total < h {
				t.Errorf("%s h=%d: alto total = %d < %d (hueco en la vista): %+v",
					tc.name, h, total, h, l)
			}
			if l.bodyLines > 1 && total != h {
				t.Errorf("%s h=%d: alto total = %d, want %d: %+v", tc.name, h, total, h, l)
			}
		}
	}
}

// altoTotal suma todo lo que el layout reserva (cajas y Borders incluidos) más el
// cuerpo central. Es la altura que la vista tiene que medir.
func (l layout) altoTotal(hasFilter bool) int {
	n := tableChrome
	if hasFilter {
		n += filterSectionLines
	}
	if l.showStats {
		n += statsSectionLines
	}
	if l.showKeybinds {
		n += keybindsChrome + l.hintLines
	}
	if l.previewLines > 0 {
		n += previewChrome + l.previewLines
	}
	return n + l.bodyLines
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

// El indicador de actividad sobrevive a anchos estrechos (va primero en stats)
// y nombra la acción en curso sin duplicarla.
func TestIndicadorActividadAnchoEstrecho(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 40
	m.running["/tmp/old-clean"] = "pull"

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "pull old-clean") {
		t.Errorf("el indicador de acción en curso no sobrevive a width=40:\n%s", out)
	}
	// "sin duplicar" se refiere al indicador: el hint bar menciona la tecla
	// pull legítimamente, así que se cuenta la etiqueta del indicador, no la
	// palabra suelta.
	if n := strings.Count(out, "pull old-clean"); n != 1 {
		t.Errorf("el indicador aparece %d veces, want 1 (sin duplicar):\n%s", n, out)
	}
	for i, l := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d", i, w, m.width)
		}
	}
}

// En anchos estrechos la tabla omite columnas por la derecha en vez de
// truncarlas a medias (la cabecera nunca desborda el borde).
func TestCabeceraOmiteColumnasEstrecho(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.width = 60
	content := m.renderDashboard()
	out := stripANSI(content)
	if strings.Contains(out, "FETCH") {
		t.Errorf("FETCH no debería caber a width=60:\n%s", out)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "BRANCH") {
		t.Errorf("faltan columnas básicas:\n%s", out)
	}
	for i, l := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(l); w > m.width {
			t.Errorf("línea %d ancho = %d > %d", i, w, m.width)
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
	if !strings.Contains(rendered, "divergió") {
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
	if txt := m.toasts.toasts[0].text; !strings.Contains(txt, "failed") || !strings.Contains(txt, "divergió") {
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

// El trabajo en curso de borrado de worktree aparece en el indicador de
// actividad de stats.
func TestRemoveWorktreeRunningVisibleInStats(t *testing.T) {
	m, p := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.running[p.Path] = "worktree_remove"

	if out := stripANSI(m.View().Content); !strings.Contains(out, "worktree_remove") {
		t.Errorf("el borrado en curso no se muestra en stats:\n%s", out)
	}
}

// En una terminal baja el aviso de borrado sigue visible: se degrada stats
// antes que keybinds, porque keybinds es donde se anuncia la tecla.
func TestRemoveWorktreePromptVisibleInShortTerminal(t *testing.T) {
	m, _ := removeWtModel(t, "/tmp/parent-repo", wt("/tmp/wt-a", "a"))
	m.height = 6 // sin el aviso armado, keybinds se oculta a esta altura

	if strings.Contains(stripANSI(m.View().Content), "remove worktree") {
		t.Fatal("precondición: sin armado no debe verse el prompt")
	}

	m, _ = press(m, "D")
	out := stripANSI(m.View().Content)
	if !strings.Contains(sectionContent(t, out, "keybinds"), "remove worktree wt-a? D to confirm") {
		t.Errorf("el prompt no es visible en terminal baja:\n%s", out)
	}
}

// Con el aviso armado, keybinds se queda con una línea de contenido (la del
// prompt) y el alto que sobraba vuelve a la tabla: ni caja inflada ni hints
// compitiendo con el prompt.
func TestPromptArmadoSustituyeLasHints(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	if m.keybindsLines() != defaultHintLines {
		t.Fatalf("sin armado, keybindsLines = %d, want %d", m.keybindsLines(), defaultHintLines)
	}
	if m.promptLine() != "" {
		t.Errorf("sin armado hay prompt: %q", m.promptLine())
	}

	m, _ = press(m, "p")
	if m.keybindsLines() != 1 {
		t.Errorf("armado, keybindsLines = %d, want 1", m.keybindsLines())
	}

	out := m.View().Content
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) != m.height {
		t.Errorf("líneas = %d, want %d (alto exacto de la terminal)", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}

	plano := stripANSI(out)
	kb := sectionContent(t, plano, "keybinds")
	if !strings.Contains(kb, "p default") {
		t.Errorf("el prompt no está en keybinds:\n%s", kb)
	}
	for _, hint := range []string{"j/k move", "f fetch", "q quit"} {
		if strings.Contains(kb, hint) {
			t.Errorf("la hint %q sigue ahí tras armar el prompt:\n%s", hint, kb)
		}
	}

	// Resolver el selector devuelve las hints y con ellas el alto de la caja.
	m, _ = press(m, "esc")
	if m.promptLine() != "" {
		t.Errorf("esc no limpió el prompt: %q", m.promptLine())
	}
	if lines := strings.Split(stripANSI(m.View().Content), "\n"); len(lines) != m.height {
		t.Errorf("tras cancelar, líneas = %d, want %d", len(lines), m.height)
	}
}
