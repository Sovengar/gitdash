// Tests del selector de variante de pull: la tecla `p` arma, la segunda tecla
// elige, y cualquier otra cancela.
package tui

import (
	"strings"
	"testing"
)

// newPullModel construye un modelo sobre el fixture estándar (atajo para no
// repetir el desempaquetado de fixtureProjects en cada test).
func newPullModel(t *testing.T) Model {
	t.Helper()
	projects, states := fixtureProjects()
	return newTestModel(t, projects, states)
}

// cursorOn mueve el cursor a la fila del repo indicado. Las filas se ordenan
// attention-first, así que la posición del cursor NO es la del fixture: sin
// esto los tests apuntarían al repo equivocado sin dar error.
func cursorOn(t *testing.T, m Model, path string) Model {
	t.Helper()
	for i, e := range m.entries() {
		if e.kind == kindRepo && e.r.project.Path == path {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("el fixture no tiene la fila %s", path)
	return m
}

// `p` no ejecuta: arma el selector. Si ejecutara, la política vendría
// hardcodeada y las variantes no existirían.
func TestPullArmaElSelectorSinEjecutar(t *testing.T) {
	m := newPullModel(t)
	before := len(m.running)
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p no armó el selector")
	}
	if len(m.running) != before {
		t.Errorf("p lanzó una acción: running = %v, want sin cambios", m.running)
	}
}

// Las cuatro opciones despachan su kind sobre el repo bajo el cursor.
func TestSelectorDespachaCadaVariante(t *testing.T) {
	for key, kind := range map[string]string{
		"p": "pull",
		"r": "pull_rebase",
		"f": "pull_ff",
		"m": "pull_merge",
	} {
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/old-clean")
		m, _ = press(m, "p")
		m, _ = press(m, key)
		if m.pullArmed != nil {
			t.Errorf("p%s dejó el selector armado", key)
		}
		if m.running["/tmp/old-clean"] != kind {
			t.Errorf("p%s → running = %q, want %q", key, m.running["/tmp/old-clean"], kind)
		}
	}
}

// Una tecla que no es variante cancela el selector y sigue su curso normal: si
// se comiera, la app quedaría pegada esperando una segunda pulsación que nunca
// llega.
func TestSelectorTeclaNoVarianteCancelaYSigue(t *testing.T) {
	m := newPullModel(t)
	start := m.cursor
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("precondición: el selector no se armó")
	}
	m, _ = press(m, "j") // no es una variante: cancela y mueve el cursor
	if m.pullArmed != nil {
		t.Error("el selector sigue armado tras una tecla no-variante")
	}
	if m.cursor == start {
		t.Error("la tecla no-variante no ejecutó su propia acción (cursor quieto)")
	}
	if len(m.running) != 0 {
		t.Errorf("la cancelación lanzó una acción: %v", m.running)
	}
}

// Con el selector armado, `f` es fetch y `r` es rescan: por eso el estado
// armado tiene que consumir la tecla antes del enrutado normal.
func TestSelectorNoDisparaAccionesDeLaTabla(t *testing.T) {
	for _, key := range []string{"f", "r"} {
		m := newPullModel(t)
		m, _ = press(m, "p")
		m, _ = press(m, key)
		if m.fetchStates["/tmp/old-clean"] == "fetching" {
			t.Errorf("p%s disparó un fetch: el selector no consumió la tecla", key)
		}
		if m.scanning {
			t.Errorf("p%s disparó un rescan: el selector no consumió la tecla", key)
		}
	}
}

// Sin repo no hay nada que pulls: no se arma un selector que no puede resolver.
func TestSelectorNoArmaSinRepo(t *testing.T) {
	m := newPullModel(t)
	idx := -1
	for i, e := range m.entries() {
		if e.kind == kindRepo && !e.r.project.HasRepo {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("el fixture no tiene una fila sin repo")
	}
	m.cursor = idx
	m, _ = press(m, "p")
	if m.pullArmed != nil {
		t.Errorf("el selector se armó sobre una fila sin repo: %+v", m.pullArmed)
	}
	if len(m.running) != 0 {
		t.Errorf("lanzó una acción sobre una fila sin repo: %v", m.running)
	}
}

// El prompt tiene que anunciar las cuatro variantes: es el único sitio donde se
// explica qué hace la segunda tecla.
func TestSelectorPromptAnunciaLasCuatroVariantes(t *testing.T) {
	m := newPullModel(t)
	if got := m.pullPrompt(); got != "" {
		t.Errorf("pullPrompt sin selector = %q, want vacío", got)
	}
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "p")
	got := m.pullPrompt()
	for _, want := range []string{"p default", "r rebase", "f ff-only", "m merge", "esc cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("el prompt no menciona %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "old-clean") {
		t.Errorf("el prompt no nombra el repo objetivo:\n%s", got)
	}
}

// El prompt se pinta en la sección de keybinds (no en un toast): un toast expira
// a los 3 s y el selector vive hasta la siguiente tecla.
func TestSelectorPromptVisibleEnKeybinds(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "p")
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "ff-only") {
		t.Errorf("el selector armado no se anuncia en el dashboard:\n%s", out)
	}
	// Y no en el banner de stats: ahí solo van el resumen y la actividad.
	if banner := sectionContent(t, out, "gitdash"); strings.Contains(banner, "ff-only") {
		t.Errorf("el prompt se coló en la sección de stats:\n%s", banner)
	}
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "ff-only") {
		t.Errorf("el prompt no está en la sección de keybinds:\n%s", kb)
	}
}

// El selector se cancela con esc sin ejecutar nada.
func TestSelectorEscCancela(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "p")
	m, _ = press(m, "esc")
	if m.pullArmed != nil {
		t.Error("esc no canceló el selector")
	}
	if len(m.running) != 0 {
		t.Errorf("esc lanzó una acción: %v", m.running)
	}
}

// El detalle guarda el argv resuelto: con `p` sin flags, lo que reconcilió fue
// el gitconfig del usuario y no hay otra forma de saberlo.
func TestDetalleMuestraElComandoResuelto(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	updated, _ := m.Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull_rebase",
		cmd:    "git pull --rebase --autostash",
		output: "Rebase aplicado",
	})
	m = updated.(Model)
	m.detailOpen = true
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "git pull --rebase --autostash") {
		t.Errorf("el detalle no muestra el comando ejecutado:\n%s", out)
	}
	// Y lo nombra con la variante, no con el kind interno.
	if !strings.Contains(out, "last rebase") {
		t.Errorf("el detalle no nombra la variante:\n%s", out)
	}
}

// Un push también deja su argv en el detalle.
func TestDetallePushMuestraComando(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	updated, _ := m.Update(actionMsg{
		path: "/tmp/old-clean", kind: "push", cmd: "git push", output: "",
	})
	m = updated.(Model)
	m.detailOpen = true
	if out := stripANSI(m.View().Content); !strings.Contains(out, "git push") {
		t.Errorf("el detalle no muestra el push ejecutado:\n%s", out)
	}
}
