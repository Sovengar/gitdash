// Tests del layout por secciones bordadas, el presupuesto de alto y la
// migración del feedback de acciones a toasts.
package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

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
	// El panel más pequeño que entra es el de la cabecera de la ficha, y solo
	// si a la tabla le quedan minBodyLines filas. La cabecera son detailHeadLines
	// líneas, así que el alto mínimo se deriva de ella en vez de ir a fuego.
	first := minPanelHeight(t)
	if l := computeLayout(first, false, defaultHintLines, false); l.previewLines != detailHeadLines {
		t.Errorf("h=%d: previewLines = %d, want %d", first, l.previewLines, detailHeadLines)
	}
	// Por debajo, nada de panel y el reparto de siempre: hints y stats intactos y
	// la tabla con lo que sobra.
	for h := 14; h < first; h++ {
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
	for h := first; h <= 80; h++ {
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

// minPanelHeight es el alto de terminal más bajo en el que el panel de preview
// entra. Se busca en vez de estar a fuego porque depende de detailHeadLines: si
// la cabecera de la ficha crece o encoge, el suelo se mueve solo.
func minPanelHeight(t *testing.T) int {
	t.Helper()
	for h := 0; h <= 120; h++ {
		if computeLayout(h, false, defaultHintLines, false).previewLines > 0 {
			return h
		}
	}
	t.Fatal("el panel no entra a ninguna altura")
	return 0
}

// altoTotal suma todo lo que el layout reserva (cajas y bordes incluidos) más el
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

// El presupuesto en los altos degenerados es una cuenta justa: cada sección se
// queda mientras quepa dejando al menos una línea de cuerpo. Los casos que
// importan son los de encaje EXACTO, donde la sección cabe justo, porque ahí es
// donde un "> " mal puesto esconde una sección que sí cabe.
func TestLayoutEnElEncajeExacto(t *testing.T) {
	// h=7: chrome de la tabla (3) + stats (3) = 6, y sobra exactamente una
	// línea de cuerpo. Stats se quedan (caben justas); una línea menos ya no.
	if l := computeLayout(7, false, defaultHintLines, false); !l.showStats {
		t.Errorf("h=7: stats ocultas aunque caben justas: %+v", l)
	}
	if l := computeLayout(6, false, defaultHintLines, false); l.showStats {
		t.Errorf("h=6: stats visibles sin sitio: %+v", l)
	}

	// El share del panel es 2/5 del alto LIBRE (el que no ocupan el chrome y las
	// secciones), con minPreviewLines por abajo. A h=41 le quedan 28 líneas
	// libres, así que el panel se lleva 28*2/5 = 11 y a la tabla le sobran 17.
	// El 2/5 es el contrato: no un "casi la mitad" que qualquer cálculo dé.
	const h = 41
	free := h - (tableChrome + statsSectionLines + keybindsChrome + defaultHintLines + previewChrome)
	if want := free * previewShare / 5; want <= minPreviewLines {
		t.Fatalf("h=%d: free=%d da un share de %d, que no ejercita el 2/5 (caería al mínimo)", h, free, want)
	}
	l := computeLayout(h, false, defaultHintLines, false)
	if l.previewLines != free*previewShare/5 {
		t.Errorf("h=%d: previewLines = %d, want %d (2/5 de %d libres)", h, l.previewLines, free*previewShare/5, free)
	}
	if want := h - (tableChrome + statsSectionLines + keybindsChrome + defaultHintLines + previewChrome + l.previewLines); l.bodyLines != want {
		t.Errorf("h=%d: bodyLines = %d, want %d", h, l.bodyLines, want)
	}

	// El panel se queda con SU SHARE como tope (es aditivo: no se come el
	// dashboard entero) y solo baja de ahí cuando el resto no lo permite. En
	// este tramo es la tabla la que marca el techo: el panel crece hasta
	// dejarle EXACTAMENTE minBodyLines filas, ni una menos. Una tabla más
	// grande o más pequeña seguiría "cayendo bien" en cualquier test que solo
	// compruebe que el panel cabe, así que el borde se mira de cerca.
	for h := 14; h <= 40; h++ {
		got := computeLayout(h, false, defaultHintLines, false)
		if got.previewLines == 0 {
			continue
		}
		top := panelHeight(h, tableChrome, 0, defaultHintLines)
		if got.previewLines > top {
			t.Errorf("h=%d: previewLines = %d, want <= %d (el share manda)", h, got.previewLines, top)
		}
		// Con el panel en su tope no había nada más grande que buscar; si está
		// por debajo, una línea más tiene que ser la que no cupiera.
		if mas := fitLayout(h, tableChrome, 0, got.previewLines+1, defaultHintLines, false); got.previewLines < top &&
			mas.bodyLines >= minBodyLines && mas.mismaChromeQue(got) {
			t.Errorf("h=%d: el panel se quedó en %d pudiendo llegar a %d (bodyLines=%d, top=%d)",
				h, got.previewLines, got.previewLines+1, mas.bodyLines, top)
		}
		if got.previewLines < top && got.bodyLines != minBodyLines {
			t.Errorf("h=%d: panel en %d por debajo de su share %d con bodyLines=%d, want exactamente %d",
				h, got.previewLines, top, got.bodyLines, minBodyLines)
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

// --- indicador de actividad, resumen de grupo y presupuestos de sección ---

// El indicador de actividad cuenta las acciones EN CURSO: una sola no lleva
// sufijo (nada de "+0"), N acciones llevan "+(N-1)", y solo cuentan las que
// lanzan git. El orden es por path para que el render sea determinista.
func TestIndicadorActividadCuentaLasAcciones(t *testing.T) {
	projects, states := fixtureProjects()

	t.Run("ninguna", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		if got := m.activityIndicator(); got != "" {
			t.Errorf("indicador = %q, want vacío sin acciones", got)
		}
	})

	t.Run("una sola sin sufijo", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.running = map[string]string{"/tmp/dirty-api": "pull"}
		got := m.activityIndicator()
		if !strings.Contains(got, "pull dirty-api…") {
			t.Errorf("indicador = %q, want la acción en curso", got)
		}
		if strings.Contains(got, "+") {
			t.Errorf("indicador = %q, want sin sufijo con una sola acción", got)
		}
	})

	t.Run("tres con +2", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.running = map[string]string{
			"/tmp/dirty-api":     "pull",
			"/tmp/old-clean":     "push",
			"/tmp/worktree-host": "worktree_remove",
		}
		got := m.activityIndicator()
		if !strings.Contains(got, "+2") {
			t.Errorf("indicador = %q, want +2 con tres acciones", got)
		}
	})

	t.Run("cada kind cuenta", func(t *testing.T) {
		// push y worktree_remove no son pull, pero también son acciones que
		// lanzan git: si se colaran fuera del indicador, el usuario vería la
		// fila quieta mientras el push está en marcha.
		for _, kind := range []string{"pull", "pull_rebase", "pull_ff", "pull_merge", "push", "worktree_remove"} {
			m := newTestModel(t, projects, states)
			m.running = map[string]string{"/tmp/dirty-api": kind}
			if got := m.runningActions(); len(got) != 1 {
				t.Errorf("kind %q: runningActions = %v, want 1", kind, got)
			}
		}
	})

	t.Run("lo que no lanza git no se cuenta", func(t *testing.T) {
		// fetch y el handoff de pull_ai ceden la terminal o son lecturas: no
		// son una acción sobre un repo que el usuario pueda esperar.
		for _, kind := range []string{"fetch", "fetch_all", "pull_ai", "scan", "rescan"} {
			m := newTestModel(t, projects, states)
			m.running = map[string]string{"/tmp/dirty-api": kind}
			if got := m.runningActions(); len(got) != 0 {
				t.Errorf("kind %q: runningActions = %v, want vacío", kind, got)
			}
			if got := m.activityIndicator(); got != "" {
				t.Errorf("kind %q: indicador = %q, want vacío", kind, got)
			}
		}
	})

	t.Run("orden por path", func(t *testing.T) {
		m := newTestModel(t, projects, states)
		m.running = map[string]string{
			"/tmp/worktree-host": "push",
			"/tmp/dirty-api":     "pull",
			"/tmp/old-clean":     "push",
		}
		got := m.runningActions()
		want := []string{"pull dirty-api…", "push old-clean…", "push worktree-host…"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("runningActions = %v, want %v (orden por path)", got, want)
		}
	})
}

// El resumen de un grupo solo pinta los estados que HAY: un grupo limpio son
// cuatro líneas a cero que no le dicen nada al usuario que está decidiendo si
// abrirlo. Y con estados, cada uno aparece exactamente una vez.
func TestResumenDeGrupoSoloPintaLoQueHay(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
		{Path: "/b", Name: "b", PrimaryGroup: "backend", HasRepo: true},
	}
	clean := map[string]gitstatus.Snapshot{"/a": snapClean(), "/b": snapClean()}

	t.Run("limpio solo repos", func(t *testing.T) {
		m := newTestModel(t, projects, clean)
		out := stripANSI(m.renderGroupSummary(groupHeader("backend"), 20))
		if !strings.Contains(out, "repos    2") {
			t.Errorf("falta el total de repos:\n%s", out)
		}
		for _, cero := range []string{"errors", "dirty", "ahead", "behind", "wt "} {
			if strings.Contains(out, cero) {
				t.Errorf("pinta %q a cero:\n%s", cero, out)
			}
		}
	})

	t.Run("con cada estado", func(t *testing.T) {
		states := map[string]gitstatus.Snapshot{
			"/a": func() gitstatus.Snapshot {
				s := snapDirty(1, 0)
				s.Status.Ahead = 2
				return s
			}(),
			"/b": func() gitstatus.Snapshot {
				s := snapClean()
				s.Status.Behind = 3
				return s
			}(),
		}
		m := newTestModel(t, projects, states)
		out := stripANSI(m.renderGroupSummary(groupHeader("backend"), 20))
		for _, quiere := range []string{"dirty    1", "ahead    1", "behind   1"} {
			if !strings.Contains(out, quiere) {
				t.Errorf("falta %q en el resumen:\n%s", quiere, out)
			}
		}
		if strings.Contains(out, "errors") {
			t.Errorf("errors a cero en un grupo sin errores:\n%s", out)
		}
	})
}

// groupHeader construye la entrada de un header primario, que es lo que hay
// bajo el cursor cuando se navega por la vista agrupada.
func groupHeader(key string) tableEntry {
	return tableEntry{kind: kindPrimary, group: key}
}

// Con el panel sin alto (terminal baja) no se dibuja ninguna caja: una ficha de
// 0 líneas con el título vacío es un borde colgado en medio del dashboard.
func TestPreviewSinAltoNoDibujaCaja(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.height = 14 // por debajo del alto mínimo del panel
	lay := computeLayout(m.height, false, defaultHintLines, false)
	if lay.previewLines != 0 {
		t.Fatalf("h=%d: el layout dio panel de %d; el test necesita un alto sin panel", m.height, lay.previewLines)
	}
	if got := m.previewSection(lay, m.entries()); got != "" {
		t.Errorf("previewSection con 0 líneas = %q, want vacío", got)
	}
	plano := stripANSI(m.renderDashboard())
	if strings.Contains(plano, "╭ ") && strings.Count(plano, "╭ ") != 3 {
		t.Errorf("se dibujó una sección de más sin panel:\n%s", plano)
	}
}

// El presupuesto de hints manda sobre cuántas hints hay: si el layout deja
// una línea, la caja tiene una línea, aunque haya tres hints que enseñar.
func TestKeybindsRespetaElPresupuestoDeHints(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if len(m.cfg.HintBarLines()) < 2 {
		t.Fatalf("el test necesita varias hints: %v", m.cfg.HintBarLines())
	}
	for _, n := range []int{0, 1, 2} {
		out := stripANSI(m.keybindsSection(n))
		got := len(nonEmptyLines(strings.Split(sectionContent(t, out, "keybinds"), "\n")))
		if got != n {
			t.Errorf("n=%d: %d líneas de hints, want %d:\n%s", n, got, n, out)
		}
	}
}

// Solo la fila bajo el cursor lleva la marca del cursor: si el marcado se
// invirtiese, el usuario leería como seleccionada una fila en la que no está.
func TestCursorMarcaUnaSolaFila(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	entries := m.entries()
	if len(entries) < 3 {
		t.Fatalf("el test necesita 3 entradas, hay %d", len(entries))
	}
	m.cursor = 1
	out := stripANSI(m.tableSection(len(entries), entries))
	lineas := strings.Split(out, "\n")
	var marcadas []string
	for i, l := range lineas {
		if strings.Contains(l, "▸") {
			marcadas = append(marcadas, strings.TrimSpace(l))
		}
		_ = i
	}
	if len(marcadas) != 1 {
		t.Fatalf("filas marcadas = %d, want 1:\n%s", len(marcadas), out)
	}
	if want := stripANSI(m.renderEntry(entries[1], true)); !strings.Contains(marcadas[0], strings.TrimSpace(want)) {
		t.Errorf("la fila marcada = %q, want la del cursor %q", marcadas[0], strings.TrimSpace(want))
	}
}

// nonEmptyLines cuenta las líneas con contenido real de una caja: los bordes
// verticales y el relleno no cuentan como línea pintada.
func nonEmptyLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.Trim(strings.TrimSpace(l), "│| ") != "" {
			out = append(out, l)
		}
	}
	return out
}

// syncOffset es pura y su contrato tiene tres bordes: la ventana entera cabe
// (offset a 0), el cursor justo en la última fila visible (NO debe scrollear:
// si no, la fila saltaría al moving) y el cursor fuera de la ventana.
func TestSyncOffsetBordes(t *testing.T) {
	casos := []struct {
		nombre             string
		total, window      int
		cursor, offsetPrev int
		want               int
	}{
		{"todo cabe", 3, 5, 2, 4, 0},
		{"todo cabe justo", 5, 5, 4, 3, 0},
		{"cursor en la primera fila visible", 10, 5, 3, 3, 3},
		{"cursor en la última fila visible", 10, 5, 7, 3, 3},
		{"una por debajo de la última", 10, 5, 8, 3, 4},
		{"cursor por encima de la ventana", 10, 5, 1, 4, 1},
		{"scrolleado al fondo, cursor al inicio", 10, 5, 0, 5, 0},
		{"ventana de 1", 10, 1, 4, 3, 4},
		{"sin filas", 0, 5, 0, 2, 0},
	}
	for _, c := range casos {
		m := newTestModel(t, nil, nil)
		m.cursor, m.offset = c.cursor, c.offsetPrev
		m.syncOffset(c.total, c.window)
		if m.offset != c.want {
			t.Errorf("%s: offset = %d, want %d (cursor %d, total %d, window %d)",
				c.nombre, m.offset, c.want, c.cursor, c.total, c.window)
		}
	}
}

// `esc` cancela TODOS los borrados en vuelo, y SOLO esc: cualquier otra tecla
// sigue su curso y deja el borrado en marcha. La guarda `len > 0` es solo el
// atajo para no hacer un clear() vacío — como `&&` cortocircuita, no protege
// nada más.
func TestEscCancelaLosBorradosYOtrasTeclasNo(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter") // despliega los worktrees

	// Armar y confirmar un borrado deja un token en vuelo.
	m, _ = press(m, "down") // sub-fila
	m, _ = press(m, "D")
	if m.armed == nil {
		t.Fatal("D no armó el borrado")
	}
	m, _ = press(m, "D")
	if len(m.removeTokens) == 0 {
		t.Fatal("la confirmación no dejó token en vuelo")
	}
	token := m.removeTokens["/tmp/multi"]

	// Una tecla que no sea esc no toca los tokens.
	m, _ = press(m, "j")
	if m.removeTokens["/tmp/multi"] != token {
		t.Errorf("una tecla normal canceló el borrado en vuelo: %v", m.removeTokens)
	}

	// esc sí los limpia todos.
	m, _ = press(m, "esc")
	if len(m.removeTokens) != 0 {
		t.Errorf("esc no canceló los borrados en vuelo: %v", m.removeTokens)
	}
}

// El token de cada intento de borrado sale de un contador MONOTÓNICO y es el
// propio valor del contador: es lo que descarta el resultado tardío de un
// intento anterior. Si el contador no avanzara (o retrocediera), dos intentos
// podrían compartir token y el segundo aceptaría el resultado del primero.
func TestTokenDeBorradoSaleDeUnContadorMonotonico(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down") // sub-fila del worktree

	antes := m.removeGen
	m, _ = press(m, "D")
	if m.armed == nil {
		t.Fatal("D no armó el borrado")
	}
	m, _ = press(m, "D")
	if m.removeGen <= antes {
		t.Errorf("removeGen = %d, want > %d (contador monotónico)", m.removeGen, antes)
	}
	if tok := m.removeTokens["/tmp/multi"]; tok != m.removeGen {
		t.Errorf("token = %d, want el contador %d", tok, m.removeGen)
	}
}

// El filtro se limpia con esc SOLO si el input está vacío: esc con texto
// escrito es "cancelo la edición, no el filtro". El input de `/` se siembra con
// el filtro actual (para poder editarlo), así que "vacío" hay que provocarlo.
func TestEscLimpiaElFiltroSoloConElInputVacio(t *testing.T) {
	nuevoModelo := func() (Model, discovery.Project, map[string]gitstatus.Snapshot) {
		projects, states := fixtureProjects()
		m := newTestModel(t, projects, states)
		m.search = "viejo"
		return m, projects[0], states
	}

	t.Run("input vacío, esc limpia el filtro", func(t *testing.T) {
		m, _, _ := nuevoModelo()
		m, _ = press(m, "/")
		for range len("viejo") {
			m, _ = press(m, "backspace")
		}
		m, _ = press(m, "esc")
		if m.search != "" {
			t.Errorf("esc con el input vacío dejó el filtro %q", m.search)
		}
	})

	t.Run("input con texto, esc conserva el filtro", func(t *testing.T) {
		m, _, _ := nuevoModelo()
		m, _ = press(m, "/")
		if m.search != "viejo" {
			t.Fatalf("el input no se sembró con el filtro actual: %q", m.search)
		}
		m, _ = press(m, "esc")
		if m.search != "viejo" {
			t.Errorf("esc con texto en el input tiró el filtro: %q", m.search)
		}
		if m.searchActive {
			t.Error("esc con texto en el input dejó la edición activa")
		}
	})
}

// `e` sobre un repo con el marcador roto avisa en vez de abrir el editor: abrir
// el editor no arregla un TOML inválido y se pierde lo que el usuario iba a
// cambiar.
func TestEditorConMarcadorRotoAvisa(t *testing.T) {
	bueno := proj("ok", "/tmp/ok", true)
	roto := proj("roto", "/tmp/roto", true)
	roto.MarkerErr = "línea 3: valor inválido"
	states := map[string]gitstatus.Snapshot{"/tmp/ok": snapClean(), "/tmp/roto": snapClean()}
	m := newTestModel(t, []discovery.Project{bueno, roto}, states)

	// En el repo sano sí abre el editor (el comando no es nil).
	m = cursorOn(t, m, "/tmp/ok")
	if _, cmd := press(m, "e"); cmd == nil {
		t.Error("e en un repo sano no lanzó el editor")
	}

	// En el roto: toast de aviso, y el launcher del editor no se llama.
	m = cursorOn(t, m, "/tmp/roto")
	_, cmd := press(m, "e")
	if cmd == nil {
		t.Fatal("e con marcador roto no devolvió comando (debería ser el toast)")
	}
	// El aviso viaja como notifyMsg en el comando devuelto (los toasts se
	// pintan en el overlay de View, no en el cuerpo del dashboard).
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "marker error") {
		t.Errorf("e con marcador roto no avisó: %#v", cmd())
	}
}

// El estado del fetch se resuelve por su valor: "fetching" es el único que
// mantiene el spinner, "failed" muestra el fallo y "ok" limpia la columna. Si la
// guarda se invirtiera, el spinner se quedaría pegado con un fetch ya terminado.
func TestFetchStatePorSuValor(t *testing.T) {
	projects, states := fixtureProjects()
	path := "/tmp/old-clean"

	for _, c := range []struct {
		estado       string
		quiereGiro   bool
		quiereFallo  bool
		quiereQuieto bool
	}{
		{"fetching", true, false, false},
		{"failed", false, true, false},
		{"ok", false, false, true},
	} {
		m := newTestModel(t, projects, states)
		out, _ := m.Update(fetchStateMsg{path: path, state: c.estado, err: "sin red"})
		plano := stripANSI(out.(Model).renderDashboard())
		if got := strings.Contains(plano, "fetching"); got != c.quiereGiro {
			t.Errorf("%s: 'fetching' presente = %v, want %v", c.estado, got, c.quiereGiro)
		}
		if got := strings.Contains(plano, "✗ fetch"); got != c.quiereFallo {
			t.Errorf("%s: fallo presente = %v, want %v", c.estado, got, c.quiereFallo)
		}
		if got := strings.Contains(plano, "⟳ fetch"); got != c.quiereGiro {
			t.Errorf("%s: giro presente = %v, want %v", c.estado, got, c.quiereGiro)
		}
		_ = c.quiereQuieto
	}
}
