// Tests del overlay de creación de PR/MR. Patrón del repo: modelo directo
// (New + Update + inspección del estado), sin teatest.
package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// newPROverlayModel construye un modelo sobre el fixture estándar con el
// cursor en la fila indicada. Sin path deja el cursor donde esté (para probar
// el caso "no hay fila").
func newPROverlayModel(t *testing.T, path string) Model {
	t.Helper()
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if path == "" {
		return m
	}
	return cursorOn(t, m, path)
}

// openPROverlay abre el overlay sobre una fila y falla el test si no abre.
func openPROverlay(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(m, "O")
	if m.pr == nil {
		t.Fatal("la tecla del PR no abrió el overlay")
	}
	return m
}

// typeText teclea una cadena rune a rune: es lo que hace un teclado y es la
// única forma de que los widgets de bubbles reciban Code Y Text (gotcha 6).
func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, string(r))
	}
	return m
}

// resize simula un cambio de tamaño de la terminal.
func resize(m Model, width, height int) Model {
	out, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return out.(Model)
}

// formPainted dice si la CAJA del formulario está en la vista. Se busca el
// título con su borde y no la palabra "new PR" a secas: desde que `pr` es una
// acción, los hints de keybinds traen "O new PR" siempre, así que la búsqueda
// laxa daría un falso positivo con el overlay cerrado y dejaría sin comprobar
// justo lo que estos tests quieren comprobar.
func formPainted(t *testing.T, m Model) bool {
	t.Helper()
	for _, l := range strings.Split(stripANSI(m.View().Content), "\n") {
		if strings.Contains(l, "╭ new PR · ") {
			return true
		}
	}
	return false
}

// focusField mueve el foco con tab hasta el campo pedido.
func focusField(t *testing.T, m Model, want prField) Model {
	t.Helper()
	for range prFieldCount {
		if m.pr.focus == want {
			return m
		}
		m, _ = press(m, "tab")
	}
	t.Fatalf("el foco no llegó a %d (se quedó en %d)", want, m.pr.focus)
	return m
}

// --- qué se abre y sobre qué fila ---

// La tecla del overlay abre el panel del formulario sobre el dashboard, y lo
// nombra: sin el nombre del repo no se sabe a qué se va a abrir el PR.
func TestPROverlayAbreElFormulario(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "new PR · dirty-api") {
		t.Errorf("el formulario no dice sobre qué repo se abre:\n%s", out)
	}
}

// Abrirlo no ejecuta nada: la ejecución es T4, aquí solo se recogen parámetros.
// Si apareciera un running o una exec, alguien habría llamado al Runner
// demasiado pronto.
func TestPROverlayNoEjecutaNada(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	before := len(m.running)

	m, _ = press(m, "O")
	m = typeText(m, "un título")
	m, _ = press(m, prSubmitKey)

	if got := len(m.running); got != before {
		t.Errorf("el overlay lanzó %d acciones: la ejecución es de T4", got-before)
	}
	if m.prPending == nil {
		t.Fatal("el envío no llegó a prPending: T4 no tiene nada que ejecutar")
	}
}

// Al armar se capturan path, nombre, branch y sync branch. Es lo que permite
// que el overlay sobreviva a que el cursor se mueva: el envío tiene que salir
// del repo que la fila señalaba al abrir, no del que esté bajo el cursor
// después.
func TestPROverlayCapturaLaFilaAlArmar(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	if m.pr.path != "/tmp/dirty-api" {
		t.Errorf("path = %q", m.pr.path)
	}
	if m.pr.name != "dirty-api" {
		t.Errorf("name = %q", m.pr.name)
	}
	if m.pr.head != "main" {
		t.Errorf("head = %q, want main (la branch del snapshot)", m.pr.head)
	}
	// El fixture no trae SyncBranch en el snapshot: la base cae al default
	// global de la config, que es el otro origen de la resolución.
	if got := m.prParams().Base; got != "main" {
		t.Errorf("base = %q, want main (sync branch o default de config)", got)
	}
}

// La sync branch del snapshot gana sobre el default global cuando la hay: es
// el override del marcador, y la base prellenada es la del repo, no la de la
// config a pelo.
func TestPROverlayPrefechaLaSyncDelRepo(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	s.SyncBranch = "develop"
	states["/tmp/dirty-api"] = s
	m := newTestModel(t, projects, states)
	m.cfg.SyncBranch = "main" // el default global, que NO debe ganar
	m = openPROverlay(t, cursorOn(t, m, "/tmp/dirty-api"))

	if got := m.prParams().Base; got != "develop" {
		t.Errorf("base = %q, want develop (la sync branch del repo)", got)
	}
	if out := stripANSI(m.View().Content); !strings.Contains(out, "develop") {
		t.Errorf("la base prellenada no se ve en el panel:\n%s", out)
	}
}

// Sin fila (el cursor está en un header de grupo) avisa y no abre: un header no
// es un repo, así que no hay nada a lo que abrirle un PR.
func TestPROverlaySinFilaNoAbre(t *testing.T) {
	// Con grupo primario aparecen headers, que no son repos: selected()
	// devuelve false ahí, que es el caso "no hay fila" de verdad.
	projects := []discovery.Project{
		{Path: "/x", Name: "api", PrimaryGroup: "vsocial", HasRepo: true},
		{Path: "/y", Name: "cli", PrimaryGroup: "vsocial", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/x": snapClean(), "/y": snapClean()}
	m := newTestModel(t, projects, states)
	header := -1
	for i, e := range m.entries() {
		if e.kind != kindRepo {
			header = i
			break
		}
	}
	if header < 0 {
		t.Fatal("el fixture no tiene headers de grupo")
	}
	m.cursor = header

	_, cmd := press(m, "O")

	if cmd == nil {
		t.Fatal("sin fila no hubo aviso")
	}
	if _, ok := cmd().(notifyMsg); !ok {
		t.Errorf("aviso = %v, want notifyMsg", cmd())
	}
	if m.pr != nil {
		t.Error("sin fila se abrió el overlay")
	}
}

// Sobre una fila sin repo git avisa y no abre: no hay remote del que sacar el
// forge, así que abrir el formulario sería prometer algo que no se puede hacer.
func TestPROverlaySinRepoNoAbre(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/no-repo-docs")

	_, cmd := press(m, "O")

	if cmd == nil {
		t.Fatal("sin repo no hubo aviso")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("aviso = %v, want no git repo", cmd())
	}
	if m.pr != nil {
		t.Error("sin repo se abrió el overlay")
	}
}

// El log ya es el cuerpo del dashboard: dos overlays a la vez no tienen sitio.
// Con el panel abierto la tecla no hace nada, en vez de abrir un formulario
// detrás de un panel que lo tapa entero.
func TestPROverlayNoAbreSobreElPanelDelLog(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m, _ = press(m, "l") // abre el panel del log

	_, cmd := press(m, "O")

	if m.pr != nil {
		t.Error("se abrió el overlay con el panel del log abierto")
	}
	if cmd != nil {
		t.Errorf("la tecla launchó algo: %v", cmd())
	}
	if !strings.Contains(stripANSI(m.View().Content), "log") {
		t.Error("el panel del log dejó de pintarse")
	}
}

// --- teclado ---

// El overlay se lleva el teclado entero: escribir "p" en el título no puede
// armar el selector de pull, ni "f" un fetch, ni "l" abrir el log. Es LA
// diferencia con un prefix-key, que justamente existe para_NO_ hacer esto.
func TestPROverlayCapturaElTecladoEntero(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	m = typeText(m, "pfl")

	if m.pullArmed != nil {
		t.Error("la p del título armó el selector de pull")
	}
	if m.logOpen {
		t.Error("la l del título abrió el panel del log")
	}
	if len(m.running) != 0 {
		t.Errorf("la f del título lanzó un fetch: %v", m.running)
	}
	if got := m.prParams().Title; got != "pfl" {
		t.Errorf("title = %q, want \"pfl\"", got)
	}
}

// ctrl+c sigue cerrando la app: un view mode que se traga el abort del
// terminal deja al usuario sin salida.
func TestPROverlayDejaSalirConCtrlC(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	_, cmd := press(m, "ctrl+c")

	if cmd == nil {
		t.Fatal("ctrl+c no produjo cmd dentro del overlay")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c = %T, want tea.QuitMsg", cmd())
	}
}

// --- validación ---

// Enviar sin título no cierra y avisa EN EL PANEL, no en un toast: un toast
// expira a los 3 s y se va justo cuando el usuario está mirando el campo
// culpable.
func TestPROverlayTituloVacioNoCierra(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	m, cmd := press(m, prSubmitKey)

	if m.pr == nil {
		t.Fatal("el overlay se cerró sin título")
	}
	if m.prPending != nil {
		t.Error("se publicó un envío sin título")
	}
	if cmd != nil {
		t.Errorf("la validación launchó algo: %v", cmd())
	}
	if m.pr.err == "" {
		t.Error("no hay aviso de validación")
	}
	if !strings.Contains(sectionContent(t, stripANSI(m.View().Content), "new PR · dirty-api"), m.pr.err) {
		t.Errorf("el aviso no está en el panel:\n%s", stripANSI(m.View().Content))
	}
	// Ni toast ni nada que se pueda perder: el aviso es estado del formulario.
	if len(m.toasts.blocksFor(m.width)) != 0 {
		t.Error("el error de validación se emitió como toast")
	}
}

// Enviar sin base tampoco: es el otro flag obligatorio.
func TestPROverlayBaseVaciaNoCierra(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "un título")
	m = focusField(t, m, prFieldBase)
	for range 10 { // la base prellenada se borra entera
		m, _ = press(m, "backspace")
	}

	m, _ = press(m, prSubmitKey)

	if m.pr == nil {
		t.Fatal("el overlay se cerró sin base")
	}
	if m.prPending != nil {
		t.Error("se publicó un envío sin base")
	}
	if !strings.Contains(m.pr.err, "base") {
		t.Errorf("aviso = %q, want menciona la base", m.pr.err)
	}
}

// El aviso se va al escribir: corrige lo que ha corregido y nada más.
func TestPROverlayElAvisoDesapareceAlEscribir(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m, _ = press(m, prSubmitKey)
	if m.pr.err == "" {
		t.Fatal("sin título no hay aviso")
	}

	m = typeText(m, "a")

	if m.pr.err != "" {
		t.Errorf("el aviso sigue tras escribir: %q", m.pr.err)
	}
}

// Enviar con los dos campos válidos cierra, publica los parámetros y deja el
// overlay limpio.
func TestPROverlayEnvioValidoCierraYPublica(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "Add the sync branch base")

	m, _ = press(m, prSubmitKey)

	if m.pr != nil {
		t.Error("el overlay quedó abierto tras un envío válido")
	}
	if m.prPending == nil {
		t.Fatal("no se publicó el envío")
	}
	if m.prPending.path != "/tmp/dirty-api" {
		t.Errorf("path = %q", m.prPending.path)
	}
	if got := m.prPending.params.Title; got != "Add the sync branch base" {
		t.Errorf("Title = %q", got)
	}
	if formPainted(t, m) {
		t.Errorf("el panel del formulario sigue pintado:\n%s", stripANSI(m.View().Content))
	}
}

// --- el draft toggle ---

// El toggle se gira con space y con enter, y solo con el foco encima: con el
// foco en otro campo, space es un espacio.
func TestPROverlayDraftToggle(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	if m.prParams().Draft {
		t.Error("el draft arranca activado")
	}
	m = focusField(t, m, prFieldDraft)

	m, _ = press(m, " ")
	if !m.prParams().Draft {
		t.Error("space no activó el draft")
	}
	m, _ = press(m, "enter")
	if m.prParams().Draft {
		t.Error("enter no desactivó el draft")
	}
	// Con el foco fuera, space es un espacio: no toca el toggle.
	m = focusField(t, m, prFieldTitle)
	m = typeText(m, " ")
	if m.pr.draft {
		t.Error("un espacio en el título activó el draft")
	}
	// Se mira el valor CRUDO del input, no el de los parámetros: un espacio
	// solo se recorta al publicar, que es otra cosa.
	if got := m.pr.title.Value(); got != " " {
		t.Errorf("el input del título = %q, want un espacio", got)
	}
}

// --- el foco ---

// tab recorre los campos en orden y vuelve al primero; shift+tab va al revés.
// El foco da la vuelta por aritmética, no por una lista: un campo nuevo que no
// se summoneda a la lista sería invisible.
func TestPROverlayTabRecorreLosCampos(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	want := []prField{prFieldTitle, prFieldBase, prFieldDraft, prFieldBody}

	for _, f := range want {
		if m.pr.focus != f {
			t.Fatalf("foco en %d tras abrir, want %d", m.pr.focus, f)
		}
		m, _ = press(m, "tab")
	}
	if m.pr.focus != prFieldTitle {
		t.Errorf("el foco no volvió al principio: %d", m.pr.focus)
	}
	m, _ = press(m, "shift+tab")
	if m.pr.focus != prFieldBody {
		t.Errorf("shift+tab no fue al último: %d", m.pr.focus)
	}
}

// El campo con el foco se marca en el panel: sin eso, un formulario de cuatro
// campos es un texto plano y no se sabe dónde se está escribiendo.
func TestPROverlayMarcaElCampoConElFoco(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldDraft)

	content := sectionContent(t, stripANSI(m.View().Content), "new PR · dirty-api")
	if !strings.Contains(content, "▸ draft") {
		t.Errorf("el campo con el foco no está marcado:\n%s", content)
	}
	if strings.Contains(content, "▸ title") {
		t.Errorf("el foco aparece en dos campos:\n%s", content)
	}
}

// --- el cuerpo y los parámetros ---

// El cuerpo es multilínea: enter dentro del cuerpo es un salto de línea, no un
// envío. Por eso el envío tiene su propia tecla con modificador.
func TestPROverlayCuerpoEsMultilinea(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldBody)

	m = typeText(m, "primera")
	m, _ = press(m, "enter")
	m = typeText(m, "segunda")

	if m.pr == nil {
		t.Fatal("enter en el cuerpo envió el formulario")
	}
	if got := m.prParams().Body; got != "primera\nsegunda" {
		t.Errorf("Body = %q, want dos líneas", got)
	}
}

// Lo que se escribe llega ÍNTEGRO a los parámetros: espacios, comillas, saltos
// de línea y signos de shell. Es lo que acaba en el argv, y un texto escapado
// o normalizado aquí sería un PR con el cuerpo cambiado sin que nada fallara.
func TestPROverlayLosParametrosLleganIntegros(t *testing.T) {
	title := `Add "sync branch" base $HOME & 'quotes'`
	// Ni tab ni otro carácter de control: los widgets de bubbles los expanden
	// al insertarlos (SetValue sanea, y el textarea no puede pintar un tab sin
	// romper su propia cuenta de columnas). Todo lo demás tiene que llegar tal
	// cual, porque el cuerpo es lo que gh/glab enseñarán en el PR.
	body := "## Qué cambia\n\n- `git remote` con comillas: \"x\" y $VAR\n- $(whoami) y `id`\n- \"al margen\" y más cosas\n"
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, title)
	m = focusField(t, m, prFieldBody)
	m.pr.body.SetValue(body)
	m = focusField(t, m, prFieldDraft)
	m, _ = press(m, " ")
	m, _ = press(m, prSubmitKey)

	if m.prPending == nil {
		t.Fatal("no se publicó el envío")
	}
	p := m.prPending.params
	if p.Title != title {
		t.Errorf("Title = %q, want %q", p.Title, title)
	}
	if p.Body != body {
		t.Errorf("Body = %q, want %q", p.Body, body)
	}
	if p.Base != "main" {
		t.Errorf("Base = %q", p.Base)
	}
	if p.Head != "main" {
		t.Errorf("Head = %q, want la branch del repo", p.Head)
	}
	if !p.Draft {
		t.Error("Draft = false, want el toggle activado")
	}
}

// Título y base se recortan: son flags de una línea y un espacio al final es un
// descuido, no parte del texto. El cuerpo NO se toca.
func TestPROverlayRecortaElTextoDeUnaLinea(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "  un título  ")
	m = focusField(t, m, prFieldBase)
	m = typeText(m, "  ")

	m, _ = press(m, prSubmitKey)

	p := m.prPending.params
	if p.Title != "un título" {
		t.Errorf("Title = %q, want recortado", p.Title)
	}
	if p.Base != "main" {
		t.Errorf("Base = %q, want recortado", p.Base)
	}
}

// --- cerrar y reabrir ---

// esc cierra sin publicar nada y sin ejecutar: es "no quiero esto".
func TestPROverlayEscCierraSinEfectos(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "borrado")
	m, _ = press(m, "esc")

	if m.pr != nil {
		t.Error("esc no cerró el overlay")
	}
	if m.prPending != nil {
		t.Error("esc publicó un envío")
	}
	if len(m.running) != 0 {
		t.Errorf("esc lanzó algo: %v", m.running)
	}
	if m.promptLine() != "" {
		t.Errorf("esc dejó el aviso de keybinds: %q", m.promptLine())
	}
}

// Cerrar y reabrir empieza de cero: si quedara algo (el texto del intento
// anterior, el draft, el foco, el error) el segundo formulario escribiría con el
// contenido del primero.
func TestPROverlayReabrirEmpiezaLimpio(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "borrado")
	m = focusField(t, m, prFieldDraft)
	m, _ = press(m, " ")
	m = focusField(t, m, prFieldBase)
	for range 10 { // vaciar la base hace FALLAR el envío
		m, _ = press(m, "backspace")
	}
	m, _ = press(m, prSubmitKey)
	if m.pr.err == "" {
		t.Fatal("sin base no hay aviso")
	}
	if m.prPending != nil {
		t.Fatal("el envío inválido se publicó")
	}
	m, _ = press(m, "esc")

	// Sobre OTRO repo, para que tampoco valga "el estado era del mismo".
	m = openPROverlay(t, cursorOn(t, m, "/tmp/old-clean"))

	if got := m.prParams(); got.Title != "" || got.Body != "" || got.Draft {
		t.Errorf("el formulario nuevo heredó estado: %+v", got)
	}
	if m.pr.err != "" {
		t.Errorf("el formulario nuevo heredó el aviso: %q", m.pr.err)
	}
	if m.pr.focus != prFieldTitle {
		t.Errorf("el foco quedó en %d", m.pr.focus)
	}
	if m.pr.path != "/tmp/old-clean" {
		t.Errorf("path = %q, want el repo nuevo", m.pr.path)
	}
}

// Abrir el overlay suelta los selectores armados: su aviso vive en la sección
// de keybinds, que ahora pinta la leyenda del formulario, y la segunda tecla
// que esperaban nunca va a llegar.
func TestPROverlayAbrirSueltaLosSelectoresArmados(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p no armó el selector")
	}
	m, _ = press(m, "v")
	if m.visualArmed == nil {
		t.Fatal("v no armó el selector visual")
	}

	m, _ = press(m, "O")

	if m.pullArmed != nil || m.visualArmed != nil {
		t.Errorf("quedaron selectores armados: %v %v", m.pullArmed, m.visualArmed)
	}
}

// --- el presupuesto de alto y el render ---

// El overlay es una caja más del dashboard: aparece en la sección del
// formulario y el conjunto sigue midiendo EXACTAMENTE lo que la terminal. Un
// alto que no cuadra significa que algo se sale de la pantalla.
func TestPROverlaySeEncuadraEnLaTerminal(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	out := m.View().Content
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) != m.height {
		t.Fatalf("líneas = %d, want %d", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
	// Y la caja del formulario es la del alto que le dio el layout, no la que
	// le daría su contenido.
	lay := m.layout()
	content := sectionContent(t, stripANSI(out), "new PR · dirty-api")
	if got := len(strings.Split(content, "\n")); got != lay.bodyLines {
		t.Errorf("alto de la caja = %d, want el presupuesto %d", got, lay.bodyLines)
	}
	// El cuerpo se lo quedó el formulario: no hay ni tabla ni ficha.
	if strings.Contains(stripANSI(out), "NAME") {
		t.Errorf("la tabla sigue pintándose con el overlay abierto:\n%s", stripANSI(out))
	}
	if strings.Contains(stripANSI(out), "upstream") {
		t.Errorf("la ficha del repo sigue pintándose con el overlay abierto:\n%s", stripANSI(out))
	}
}

// El aviso del overlay sustituye a las hints en keybinds y, como todos los
// avisos, se ve en un terminal donde sin él la sección desaparecería.
func TestPROverlayPromptEnKeybinds(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	if m.promptLine() != "" {
		t.Fatalf("sin overlay hay prompt: %q", m.promptLine())
	}
	m, _ = press(m, "O")

	if m.keybindsLines() != 1 {
		t.Errorf("keybindsLines = %d, want 1 (solo el aviso)", m.keybindsLines())
	}
	kb := sectionContent(t, stripANSI(m.View().Content), "keybinds")
	for _, want := range []string{"new PR", "dirty-api", "ctrl+s", "esc"} {
		if !strings.Contains(kb, want) {
			t.Errorf("el aviso no menciona %q:\n%s", want, kb)
		}
	}
	if strings.Contains(kb, "j/k move") {
		t.Errorf("las hints siguen ahí con el aviso puesto:\n%s", kb)
	}

	// Y en un terminal corto, donde sin aviso keybinds se escondería, sigue
	// estando: es la única fuente de las teclas que el overlay espera.
	m.height = 6
	kb = sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if !strings.Contains(kb, "ctrl+s") {
		t.Errorf("el aviso desaparece en un terminal bajo:\n%s", kb)
	}
}

// Sin alto suficiente el formulario no se dibuja (media caja es peor que
// nada) y ni siquiera se abre: entrar en un overlay invisible sería escribir a
// ciegas.
func TestPROverlayNoAbreEnTerminalPequeña(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.height = 12 // por debajo de prMinBodyLines el cuerpo no llega
	if m.layout().bodyLines >= prMinBodyLines {
		t.Skipf("precondición: a %d líneas el cuerpo sí llega (%d)", m.height, m.layout().bodyLines)
	}

	_, cmd := press(m, "O")

	if m.pr != nil {
		t.Error("se abrió un overlay que no cabe")
	}
	if cmd == nil {
		t.Fatal("no hubo aviso de que no cabe")
	}
	if formPainted(t, m) {
		t.Errorf("se pintó un overlay que no cabe:\n%s", stripANSI(m.View().Content))
	}
}

// Redimensionar a un terminal que ya no lo admite cierra el overlay en vez de
// dejar al usuario escribiendo a un panel que ya no está.
func TestPROverlayResizeInsuficienteCierra(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	m = resize(m, m.width, 12)

	if m.pr != nil {
		t.Error("el overlay siguió abierto sin sitio para pintarse")
	}
	if len(m.toasts.blocksFor(m.width)) == 0 {
		t.Error("no se avisó de que se cerró por falta de alto")
	}
}

// Un resize que sí lo admite lo deja abierto: el formulario sobrevive al
// cambio de tamaño (con su texto), que es lo normal.
func TestPROverlayResizeSuficienteNoCierra(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "sigue aquí")

	m = resize(m, 100, 40)

	if m.pr == nil {
		t.Fatal("el overlay se cerró en un resize que lo admite")
	}
	if got := m.prParams().Title; got != "sigue aquí" {
		t.Errorf("el resize perdió el texto: %q", got)
	}
	if !formPainted(t, m) {
		t.Errorf("el overlay no se pinta tras el resize:\n%s", stripANSI(m.View().Content))
	}
}

// Un worktree sin marcador no tiene snapshot propio: la fila que lo sintetiza
// viene con el Snapshot en cero, así que Status.Branch sale vacío. La rama real
// está en el inventario del repo padre. Sin ese fallback el overlay enseñaría un
// head vacío y el PR saldría con -H omitido, es decir, contra lo que la CLI
// adivine que es la rama actual — y el campo que el usuario lee no sería el que
// se manda.
func TestPRHeadDeWorktreeSinMarcadorVieneDelInventario(t *testing.T) {
	projects, states := fixtureProjects()
	parent := projects[0].Path
	wtPath := "/tmp/gitdash-pr-wt"
	snap := states[parent]
	snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
		Path: wtPath, Branch: "feat/wt", Head: "abc1234",
	})
	states[parent] = snap

	m := newTestModel(t, projects, states)
	// La sub-fila solo existe si el repo está expandido.
	m.expanded[parent] = true
	m = cursorOn(t, m, wtPath)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'O', Text: "O"})
	m = out.(Model)
	if m.pr == nil {
		t.Fatal("el overlay no se abrió sobre la sub-fila de worktree")
	}
	if got := m.pr.head; got != "feat/wt" {
		t.Errorf("head = %q, quiero %q (viene del inventario, no del snapshot vacío)", got, "feat/wt")
	}
	if got := m.prParams().Head; got != "feat/wt" {
		t.Errorf("Params.Head = %q, quiero %q", got, "feat/wt")
	}
}
