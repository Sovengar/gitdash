// Tests del modelo TUI con datos sintéticos (patrón de vroom): sin teatest,
// se construye el Model, se le envían mensajes y se inspecciona el estado.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
	"gitdash/internal/testutil"
)

// newTestModel construye un modelo con proyectos y snapshots dados.
func newTestModel(t *testing.T, projects []discovery.Project, states map[string]gitstatus.Snapshot) Model {
	t.Helper()
	m := New(config.Defaults())
	m.width, m.height = 120, 30
	m.projects = projects
	m.states = states
	m.scanning = false
	// Usar directorio temporal para tests (aislar del estado real)
	m.store = state.NewStoreAt(t.TempDir())
	m.collapsed = map[string]bool{}
	m.expanded = map[string]bool{}
	// Cancelar el contexto al terminar el test mata el trabajo en vuelo (fetch
	// por repo, acciones, handoffs) ANTES de que el siguiente test instale su
	// recorder: si no, una goroutine suelta registra sus lecturas de git en el
	// log del test siguiente y lo hace fallar por lo que hizo otro.
	t.Cleanup(m.cancel)
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

// sectionContent devuelve el interior de la sección bordada con ese título. Los
// avisos armados se pintan en una sección concreta (keybinds): para comprobar
// dónde acaba cada cosa hay que mirar dentro de la caja, no en el texto plano.
func sectionContent(t *testing.T, view, title string) string {
	t.Helper()
	lines := strings.Split(view, "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, "╭ "+title+" ") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("no está la sección %q en la vista:\n%s", title, view)
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "╰") {
			return strings.Join(lines[start+1:i], "\n")
		}
	}
	t.Fatalf("la sección %q no se cierra:\n%s", title, view)
	return ""
}

// namedKeys son las teclas sintéticas que no son un único rune. Los
// modificadores no se deducen del texto (gotcha 6: el input necesita Code, y
// el Mod aparte), así que una "ctrl+s" sin Mod sería una "s" y "shift+tab" un
// "tab".
var namedKeys = map[string]tea.KeyPressMsg{
	"enter":     {Code: tea.KeyEnter},
	"esc":       {Code: tea.KeyEsc},
	"up":        {Code: tea.KeyUp},
	"down":      {Code: tea.KeyDown},
	"home":      {Code: tea.KeyHome},
	"end":       {Code: tea.KeyEnd},
	"backspace": {Code: tea.KeyBackspace},
	"tab":       {Code: tea.KeyTab},
	"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
	"ctrl+s":    {Code: 's', Mod: tea.ModCtrl},
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
}

func press(m Model, key string) (Model, tea.Cmd) {
	km, named := namedKeys[key]
	if !named {
		km = tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
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

func TestSortAttentionFirst(t *testing.T) {
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

func TestSortActivityTie(t *testing.T) {
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

func TestFilterOnlyDirty(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if len(m.rows()) != 6 {
		t.Fatalf("sin filtro rows = %d", len(m.rows()))
	}

	m, _ = press(m, "d")
	rows := m.rows()
	if len(rows) != 3 { // dirty-api, ahead-lib, behind-web
		t.Fatalf("rows = %d, want 3", len(rows))
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

func TestSearch(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "/")
	if !m.searchActive {
		t.Fatal("'/' no abrió el input")
	}
	for _, c := range "api" {
		m, _ = press(m, string(c))
	}
	rows := m.rows()
	if len(rows) != 1 || rows[0].project.Name != "dirty-api" {
		t.Errorf("en vivo: rows = %v", rowNames(rows))
	}

	m, _ = press(m, "enter")
	if m.searchActive || m.search != "api" {
		t.Errorf("confirmar: active=%v search=%q", m.searchActive, m.search)
	}

	// reabrir con el filtro activo: limpiar el input y esc limpia el filtro
	m, _ = press(m, "/")
	for range 3 {
		m, _ = press(m, "backspace")
	}
	m, _ = press(m, "esc")
	if m.search != "" {
		t.Errorf("esc no limpió el filtro (search=%q)", m.search)
	}
}

// Feedback visual inmediato — al pulsar / la sección de filtro pinta [/|] con
// el cursor del input (o su placeholder) ANTES de teclear nada.
func TestSearchImmediateFeedback(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	m, _ = press(m, "/")
	out := stripANSI(m.renderDashboard())
	if !strings.Contains(out, "[/") {
		t.Errorf("falta [/…] al entrar en filter mode:\n%s", out)
	}
	// placeholder visible con input vacío (nombre/grupo…)
	if !strings.Contains(out, "name/group") {
		t.Errorf("placeholder no visible al abrir:\n%s", out)
	}

	// al confirmar el flag persiste con el texto confirmado
	m, _ = press(m, "a")
	m, _ = press(m, "enter")
	out = stripANSI(m.renderDashboard())
	if !strings.Contains(out, "[/a]") {
		t.Errorf("tras confirmar falta [/a]:\n%s", out)
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
		t.Errorf("búsqueda por primario falló: %v", rowNames(rows))
	}
	m2 := newTestModel(t, projects, states)
	m2.search = "backend"
	rows = m2.rows()
	if len(rows) != 1 || rows[0].project.Name != "api" {
		t.Errorf("búsqueda por secundario falló: %v", rowNames(rows))
	}
}

func TestNavigationBounds(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	for range 10 {
		m, _ = press(m, "j")
	}
	if m.cursor != len(m.rows())-1 {
		t.Errorf("cursor = %d, want %d (límite inferior)", m.cursor, len(m.rows())-1)
	}
	for range 10 {
		m, _ = press(m, "k")
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (límite superior)", m.cursor)
	}
}

// La ficha del repo bajo el cursor se ve sin abrir nada: `enter` ya no abre un
// detalle (plega worktrees y grupos), así que los ficheros cambiados tienen que
// estar en la sección del panel.
func TestPanelMuestraLosFicheros(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	s.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	states["/tmp/dirty-api"] = s
	// dirty-api es la primera fila (score 4): cursor en 0
	m := newTestModel(t, projects, states)

	out := stripANSI(m.renderDashboard())
	for _, want := range []string{"main.go", "/tmp/dirty-api", "╭ dirty-api "} {
		if !strings.Contains(out, want) {
			t.Errorf("el panel no dice %q:\n%s", want, out)
		}
	}
	// Y `enter` sobre un repo sin worktrees no pliega nada ni altera la vista.
	before := stripANSI(m.renderDashboard())
	m, _ = press(m, "enter")
	if !strings.HasPrefix(stripANSI(m.renderDashboard()), before[:40]) {
		t.Errorf("enter en un repo sin worktrees cambió la vista:\n%s", stripANSI(m.renderDashboard()))
	}
	if len(m.expanded) != 0 || len(m.collapsed) != 0 {
		t.Errorf("enter en un repo sin worktrees plegó algo: expanded=%v collapsed=%v", m.expanded, m.collapsed)
	}
}

func TestDetailShowsLastAction(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.lastAction["/tmp/old-clean"] = actionResult{kind: "pull", output: "error: pull diverged\n", err: "exit 1"}
	m.search = "old-clean"
	r, _ := m.selected()
	out := m.renderDetail(r, m.height)
	if !strings.Contains(out, "pull") || !strings.Contains(out, "failed") || !strings.Contains(out, "diverged") {
		t.Errorf("detalle sin última acción:\n%s", out)
	}
}

func TestGuardNoRepo(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.search = "no-repo" // cursor sobre el proyecto sin repo
	m.cursor = 0

	for _, key := range []string{"p", "P", "f"} {
		_, cmd := press(m, key)
		if cmd == nil {
			t.Errorf("tecla %s sin guard sobre no-repo", key)
			continue
		}
		msg := cmd()
		if nm, ok := msg.(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
			if key != "f" { // f sobre no-repo: fetchTargets lo excluye, cmd nil es válido
				t.Errorf("tecla %s notificó %v", key, msg)
			}
		}
	}
}

func TestBlockRunningAction(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m.search = "old-clean"
	m.running["/tmp/old-clean"] = "pull"

	_, cmd := press(m, "P")
	if cmd == nil {
		t.Fatal("push sin cmd de bloqueo")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notificación = %v", cmd())
	}
}

func TestSummary(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	total, dirty, ahead, behind := m.summary()
	if total != 6 || dirty != 1 || ahead != 1 || behind != 1 {
		t.Errorf("summary = (%d, %d, %d, %d), want (6, 1, 1, 1)", total, dirty, ahead, behind)
	}
}

func TestViewContainsTable(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	out := m.View().Content
	for _, want := range []string{"gitdash", "dirty-api", "↑2", "↓3", "6 repos"} {
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("la vista no contiene %q", want)
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

// --- el scan con una raíz que no se puede leer ---

// Una raíz ilegible no puede tirar el escaneo entero: `discovery.Scan`
// acumula el error por raíz y sigue con las demás. El pipeline de la TUI tiene
// que llevar ese aviso en la nota (de ahí sale el toast) SIN perder los repos
// que sí se leyeron. Si la nota se pierde, un `root` mal escrito en la config
// se manifiesta como "no encuentro repos" en vez de "esta raíz no existe", que
// es justo la confusión que el aviso evita.
func TestElScanReportaLaRaizIlegibleYConservaLosReposQueSi(t *testing.T) {
	bueno, _ := testutil.NewRepo(t, true)
	testutil.Marker(t, bueno, "ok", "g", "s", false)

	// Una raíz que es un FICHERO, no un directorio: el caso que Scan reporta.
	malo := filepath.Join(t.TempDir(), "esto-no-es-un-directorio")
	if err := os.WriteFile(malo, []byte("x"), 0o644); err != nil {
		t.Fatalf("preparando la raíz mala: %v", err)
	}

	cfg := config.Defaults()
	cfg.Roots = []string{bueno, malo}
	m := New(cfg)
	t.Cleanup(m.cancel)

	m.startScanCmd()
	sp := firstScanMsg(t, m)

	if sp.note == "" {
		t.Fatal("nota vacía: la raíz ilegible no llega al usuario, y un root mal escrito parece que no hay repos")
	}
	if !strings.Contains(sp.note, "root ilegible") {
		t.Errorf("la nota no nombra el problema: %q", sp.note)
	}
	var paths []string
	for _, p := range sp.projects {
		paths = append(paths, p.Path)
	}
	if len(paths) != 1 || paths[0] != bueno {
		t.Errorf("el escaneo tiró los repos que sí se leían: %v", paths)
	}
}

// firstScanMsg consume el primer evento del pump con plazo: si el pipeline no
// emitiera nada, el test se quedaría colgado hasta el timeout global de go test
// en lugar de fallar con un mensaje que diga qué pasó.
func firstScanMsg(t *testing.T, m Model) scanProjectsMsg {
	t.Helper()
	type res struct {
		msg tea.Msg
	}
	ch := make(chan res, 1)
	go func() { ch <- res{waitForEvent(m.events)()} }()
	select {
	case r := <-ch:
		sp, ok := r.msg.(scanProjectsMsg)
		if !ok {
			t.Fatalf("primer evento = %T, want scanProjectsMsg", r.msg)
		}
		return sp
	case <-time.After(10 * time.Second):
		t.Fatal("el scan no emitió ningún evento en 10s")
		return scanProjectsMsg{}
	}
}

// --- el fin del scan: cache y fetch automático ---

// `collectDoneMsg` no lo manejaba ningún test, así que el cierre del scan no
// estaba sujeto: ni la cache que hace que la app pinte al instante al arrancar,
// ni el fetch automático. Invertir la guarda de `cache.Path()` deja la app
// aparente y sana mientras no cachea nunca, y por eso esto mira el fichero de
// verdad y no un retorno.
func TestElFinDelScanGuardaLaCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	m := newTestModel(t,
		[]discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m.cfg.FetchAuto = false // este test es el de la cache, no el del fetch

	m.Update(collectDoneMsg{})

	ruta := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "gitdash", "repos.json")
	esperaFichero(t, ruta)

	var c cache.File
	if err := json.Unmarshal(readFile(t, ruta), &c); err != nil {
		t.Fatalf("cache ilegible: %v", err)
	}
	if len(c.Repos) != 1 || c.Repos[0].Path != "/tmp/api" {
		t.Errorf("la cache no guardó lo escaneado: %+v", c.Repos)
	}
}

// El otro mitad del cierre: con fetch automático y un repo con upstream, el
// fin del scan Lanza el fetch. Se mira el evento, no el retorno, porque
// `fetchBatchCmd` publica por el canal y devuelve siempre nil.
func TestElFinDelScanLanzaElFetchAutomatico(t *testing.T) {
	casos := []struct {
		nombre    string
		fetchAuto bool
		snap      gitstatus.Snapshot
		quiere    bool
	}{
		{"con upstream", true, snapClean(), true},
		{"sin fetch automatico", false, snapClean(), false},
		{"repo sin upstream", true, gitstatus.Snapshot{}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			m := newTestModel(t,
				[]discovery.Project{proj("api", "/tmp/api", true)},
				map[string]gitstatus.Snapshot{"/tmp/api": c.snap})
			m.cfg.FetchAuto = c.fetchAuto

			m.Update(collectDoneMsg{})

			// /tmp/api no es un repo, así que el fetch falla; lo que importa es
			// que se lanzara. El evento "fetching" sale antes de tocar git.
			// El plazo solo es largo para el caso que debe LANZAR: en los
			// negativos el evento "fetching" (que sale antes de tocar git) no
			// puede tardar, así que esperar más solo alarga la suite.
			plazo := 200 * time.Millisecond
			if c.quiere {
				plazo = 5 * time.Second
			}
			visto := esperaEvento(t, m, plazo, func(ev event) bool {
				fs, ok := ev.(fetchStateMsg)
				return ok && fs.path == "/tmp/api"
			})
			if c.quiere && !visto {
				t.Error("no se lanzó el fetch automático al terminar el scan")
			}
			if !c.quiere && visto {
				t.Error("se lanzó un fetch que no tocaba: sin fetch automático o sin upstream")
			}
		})
	}
}

// esperaFichero espera a que la cache aparezca: el guardado sale en una
// goroutine (no bloquea el render), así que no basta con mirar una vez.
func esperaFichero(t *testing.T, ruta string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ruta); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("la cache nunca apareció en %s", ruta)
}

func readFile(t *testing.T, ruta string) []byte {
	t.Helper()
	raw, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leyendo %s: %v", ruta, err)
	}
	return raw
}

// esperaEvento consume eventos del pump hasta que uno cumpla pred o se agote el
// plazo. Sin plazo, un fetch que no se lanza deja el test colgado hasta el
// timeout global en vez de fallar diciendo qué faltaba.
func esperaEvento(t *testing.T, m Model, plazo time.Duration, pred func(event) bool) bool {
	t.Helper()
	ch := make(chan bool, 1)
	go func() {
		for {
			ev := waitForEvent(m.events)()
			if ev == nil {
				return
			}
			if pred(ev) {
				ch <- true
				return
			}
		}
	}()
	select {
	case ok := <-ch:
		return ok
	case <-time.After(plazo):
		return false
	}
}

// --- New sin sitio donde guardar el plegado ---

// Las dos guardas de nil de New. El store lo crea `state.NewStore()` y New
// descarta su error, así que con el estado sin resolver (ni XDG_STATE_HOME ni
// HOME) el store es nil DE VERDAD: son las guardas las que evitan que
// `LoadCollapsed` se ejecute sobre un puntero nulo. Sin ellas, New revienta
// con un nil pointer dereference al arrancar, y un usuario sin HOME (contenedor,
// systemd tmpfiles, un home que no monta) no podría ni abrir la app.
//
// El segundo caso es el control: si la guarda se invirtiera, el plegado
// persistido dejaría de cargarse aunque el sitio exista, que es el otro modo de
// romper lo mismo.
func TestNewAguantaQueNoHayDondeGuardarElPlegado(t *testing.T) {
	t.Run("sin directorio de estado", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", "")

		m := New(config.Defaults()) // no debe reventar
		t.Cleanup(m.cancel)

		if len(m.collapsed) != 0 || len(m.expanded) != 0 {
			t.Errorf("sin store no hay plegado que cargar, pero %v / %v",
				m.collapsed, m.expanded)
		}
	})

	t.Run("con plegado persistido", func(t *testing.T) {
		base := t.TempDir()
		store := state.NewStoreAt(filepath.Join(base, "gitdash"))
		if err := os.MkdirAll(store.Base(), 0o755); err != nil {
			t.Fatal(err)
		}
		raw := fmt.Sprintf(`{"g/s":true,%q:true}`, state.WorktreePrefix+"/tmp/api")
		if err := os.WriteFile(store.CollapsedFile(), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XDG_STATE_HOME", base)

		m := New(config.Defaults())
		t.Cleanup(m.cancel)

		if !m.collapsed["g/s"] {
			t.Error("el grupo colapsado persistido no se cargó")
		}
		if !m.expanded["/tmp/api"] {
			t.Errorf("el worktree expandido persistido no se cargó: %v", m.expanded)
		}
	})
}

// --- avisos y resultados obsoletos ---

// El `fetch ok` a secas solo es para UN repo: con varios se cuenta, porque
// "fetch ok" a secas no dice si se sincronizaron 3 repos o 1. Y con alguno
// fallido el aviso es otro entero: el número de fallos es lo que el usuario
// necesita ver primero. Los tres arms se prueban porque el del medio (ok == 1)
// era el único sin sujetar.
func TestElFinDelFetchDistingueCuantosReposSincroniza(t *testing.T) {
	casos := []struct {
		nombre       string
		ok, failed   int
		wantContiene string
	}{
		{"uno solo", 1, 0, "fetch ok"},
		{"varios", 3, 0, "fetch ok (3 repos)"},
		{"ninguno y todos fallan", 0, 2, "2 failed"},
		{"mezcla", 2, 1, "2 ok, 1 failed"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)}, nil)

			// Update devuelve el modelo nuevo: hay que leer ESE, no el de antes.
			// Los mapas se comparten entre copias, así que un test que se queda
			// con el viejo pasa en verde sin haber comprobado nada.
			mm, _ := m.Update(fetchDoneMsg{ok: c.ok, failed: c.failed})
			m = mm.(Model)

			got := lastToast(m)
			if !strings.Contains(got, c.wantContiene) {
				t.Errorf("fetch %d ok / %d failed: el aviso dice %q, want contiene %q",
					c.ok, c.failed, got, c.wantContiene)
			}
		})
	}
}

// Un `worktreeRemovedMsg` cuyo token no es el vigente es de un intento que se
// sustituyó (el usuario relanzó el borrado, o un scan liberó el running por
// su cuenta). Si se aceptara, borraría el token del intento vigente y le
// enseñaría al usuario un éxito por un worktree que quizá sigue ahí. La rama
// existe justo para esto y no estaba probada.
func TestElBorradoDeWorktreeObsoletoNoTocaElIntentoVigente(t *testing.T) {
	const parent = "/tmp/api"
	m := newTestModel(t, []discovery.Project{proj("api", parent, true)}, nil)
	m.removeTokens = map[string]int{parent: 7}
	m.running[parent] = "worktree_remove"

	mm, _ := m.Update(worktreeRemovedMsg{
		parent: parent, wtPath: "/tmp/wt-a", name: "wt-a",
		gen: 3, // intento antiguo: el vigente es el 7
	})
	m = mm.(Model)

	if got := m.removeTokens[parent]; got != 7 {
		t.Errorf("el resultado obsoleto consumió el token del intento vigente: %d, want 7", got)
	}
	if _, sigue := m.removeTokens[parent]; !sigue {
		t.Error("el token vigente desapareció: el relanzamiento se queda sin poder comprobar su resultado")
	}
	if m.running[parent] != "worktree_remove" {
		t.Errorf("un resultado obsoleto liberó el running de otro intento: %q", m.running[parent])
	}
	if txt := lastToast(m); strings.Contains(txt, "wt-a") {
		t.Errorf("un resultado obsoleto pintó un aviso: %q", txt)
	}
}

// Esc es la vía de salida de la confirmación de borrado, y su valor está en
// que SE CONSUME: la confirmación tiene prioridad sobre el resto del teclado
// (también sobre el esc que cierra la búsqueda). Si esc no se consumiera,
// desarmaría el borrado y además cerraría la búsqueda: dos efectos de una
// tecla, que es justo lo que el bloque commentado prohíbe. Por eso el testigo
// es la búsqueda, no el propio desarme (que ocurre igual en los dos caminos).
func TestEscDesarmaElBorradoYNoLlegaAlResto(t *testing.T) {
	const parent = "/tmp/api"
	esc := tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"}

	t.Run("con confirmación armada", func(t *testing.T) {
		m := newTestModel(t, []discovery.Project{proj("api", parent, true)}, nil)
		m.armed = &armedRemoval{wtPath: "/tmp/wt-a", parent: parent, name: "wt-a"}
		m.searchActive = true
		m.search = "api"

		mm, _ := m.Update(esc)
		m = mm.(Model)

		if m.armed != nil {
			t.Error("esc no desarmó la confirmación: la app queda esperando otra tecla")
		}
		if len(m.removeTokens) != 0 {
			t.Errorf("esc registró un intento de borrado: %v", m.removeTokens)
		}
		if m.running[parent] != "" {
			t.Errorf("esc dejó el repo en running: %q", m.running[parent])
		}
		// La parte que distingue este camino del default: la tecla se paró aquí.
		if !m.searchActive || m.search != "api" {
			t.Errorf("esc siguió su curso y tocó la búsqueda: searchActive=%v search=%q; "+
				"la confirmación tiene que consumirla", m.searchActive, m.search)
		}
	})

	// El control: sin nada armado, esc sí cierra la búsqueda. Si este caso
	// pasara también con la confirmación, el anterior no distinguiría nada.
	t.Run("sin nada armado", func(t *testing.T) {
		m := newTestModel(t, []discovery.Project{proj("api", parent, true)}, nil)
		m.searchActive = true
		m.search = "api"

		mm, _ := m.Update(esc)
		m = mm.(Model)

		if m.searchActive {
			t.Error("sin nada armado, esc tiene que cerrar la búsqueda")
		}
		if m.search != "" {
			t.Errorf("esc en la búsqueda no limpió el filtro: %q", m.search)
		}
	})
}

// Update devuelve `m, nil` para un mensaje que no sabe tratar, y eso no es un
// error: es lo que evita que un msg desconocido (el de otro modulo, o uno futuro)
// pare la TUI. El mensaje tiene que pasar por el switch entero y salir por el
// return de abajo.
func TestUpdateIgnoraMensajesDesconocidos(t *testing.T) {
	m := newTestModel(t, nil, nil)
	out, cmd := m.Update(struct{ tea.Msg }{})
	if out == nil {
		t.Fatal("Update devolvio nil en vez del modelo")
	}
	if cmd != nil {
		t.Error("Update devolvio un comando para un mensaje desconocido, want nil")
	}
}

// El TickMsg del spinner llega solo (lo emite el propio spinner mientras corre),
// asi que ningun test lo produce: hay que mandarlo a mano. Lo que se comprueba es
// que el spinner AVANZA y que su Update devuelve el siguiente tick, que es lo
// que lo mantiene girando; si el case se perdiera, el spinner se congelaria en
// el primer frame y nadie se enteraria salvo mirando.
func TestSpinnerAvanzaConSuTick(t *testing.T) {
	m := newTestModel(t, nil, nil)
	antes := m.spinner.View()
	out, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Error("el TickMsg del spinner no devolvio comando, want el siguiente tick")
	}
	despues := out.(Model).spinner.View()
	if despues == antes && len(antes) > 0 {
		t.Errorf("el spinner no se movio: %q -> %q", antes, despues)
	}
}

// `enter` sobre el input del comando `!` con el cursor en una fila SIN repo no
// puede lanzar nada, y lo dice. El caso importa porque el camino de abajo abre una
// shell: sin el aviso, `!` + enter sobre una carpeta sin repo abriria una terminal
// en un sitio donde no hay nada que hacer.
func TestComandoSinRepoAvisa(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	m = cursorOn(t, m, "/tmp/no-repo-docs")
	m.cmdOpen = true
	m.cmdInput.SetValue("ls")

	out, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter sin repo no devolvio comando, want un aviso")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("el comando devolvió %T, want un aviso", cmd())
	}
	if !strings.Contains(msg.text, "no git repo") {
		t.Errorf("aviso = %q, want que diga que no hay repo", msg.text)
	}
	// Y el input se cierra igualmente: la accion termino.
	if out.cmdOpen {
		t.Error("el input del comando sigue abierto tras el aviso")
	}
}

// `enter` sin fila bajo el cursor: no hay repo, asi que tampoco hay aviso que
// dar. Devuelve nil y cierra el input, que es lo unico que se puede hacer.
func TestComandoSinFilaNoAvisa(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.cmdOpen = true
	m.cmdInput.SetValue("ls")
	out, cmd := press(m, "enter")
	if cmd != nil {
		t.Errorf("enter sin fila devolvió %#v, want nil", cmd())
	}
	if out.cmdOpen {
		t.Error("el input sigue abierto sin fila bajo el cursor")
	}
}

// `fetch_all` sin ningun repo con upstream tiene que decirlo, no lanzar un batch
// vacio: un fetch de nada es un comando que no falla y no hace nada, y el usuario
// no ve por que no paso nada.
func TestFetchAllSinUpstreamAvisa(t *testing.T) {
	m := newTestModel(t, nil, nil)
	_, cmd := press(m, "F")
	if cmd == nil {
		t.Fatal("fetch_all sin repos devolvio nil, want un aviso")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("devolvió %T, want un aviso", cmd())
	}
	if !strings.Contains(msg.text, "no repositories") {
		t.Errorf("aviso = %q, want que diga que no hay nada que traer", msg.text)
	}
}

// `r` (rescan) con un scan ya en marcha no arranca un segundo: el aviso es
// "scan already running". El caso es el que evita el doble workerPool, que
// fightaria por el canal de eventos.
func TestRescanConScanEnMarchaAvisa(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.scanning = true
	_, cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("rescan con scan en marcha devolvio nil, want un aviso")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("devolvió %T, want un aviso", cmd())
	}
	if !strings.Contains(msg.text, "already running") {
		t.Errorf("aviso = %q, want 'scan already running'", msg.text)
	}
}

// busyActionCmd es la guarda que comparten las acciones que lanzan un comando en
// un repo: si ya hay algo en marcha en ESE repo, avisa en vez de lanzar. Sus tres
// salidas son distintas y las tres se usan: el aviso, el nil (nada en marcha, la
// accion puede seguir), y... solo dos, en realidad. El test las fija las dos y
// comprueba que el aviso nombra la accion que esta corriendo.
func TestBusyActionCmd(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.running = map[string]string{"/tmp/api": "pull"}

	if cmd := m.busyActionCmd("/tmp/api"); cmd == nil {
		t.Fatal("busyActionCmd sobre un repo ocupado devolvio nil, want un aviso")
	} else if msg, ok := cmd().(notifyMsg); !ok {
		t.Errorf("devolvió %T, want un aviso", cmd())
	} else if !strings.Contains(msg.text, "pull") {
		t.Errorf("aviso = %q, want que nombre la accion en marcha", msg.text)
	}

	// Otro repo no se ve afectado: el bloqueo es por path, no global.
	if cmd := m.busyActionCmd("/tmp/otro"); cmd != nil {
		t.Errorf("busyActionCmd sobre un repo libre devolvió %#v, want nil", cmd())
	}
}

// Los dos avisos armados se callan cuando no hay nada armado. No es un detalle:
// promptLine() es el UNICO punto por el que keybinds pinta un aviso, y sin este
// caso un prompt de un selector que ya se canceló se quedaría pintado encima de
// las hints, ocupando la línea que las hints necesitan.
func TestPromptsArmadosSeCallanSinArmar(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if got := m.visualPrompt(); got != "" {
		t.Errorf("visualPrompt sin armar = %q, want vacio", got)
	}
	if got := m.removePrompt(); got != "" {
		t.Errorf("removePrompt sin armar = %q, want vacio", got)
	}
	// Y promptLine, que es lo UNICO que keybinds consulta para pintar un aviso:
	// sin ningun estado armado devuelve la cadena vacia, no un aviso residual de
	// un selector que ya se cancelo.
	if got := m.promptLine(); got != "" {
		t.Errorf("promptLine sin armar = %q, want vacio", got)
	}
}

// toggleFold sin nada bajo el cursor es un no-op: no hay header que plegar ni
// worktree que ocultar, y sin guarda el cursor se moveria a un indice que no
// existe.
func TestToggleFoldSinFilaNoSeMueve(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if len(m.entries()) != 0 {
		t.Fatalf("el modelo sin proyectos tiene %d entradas", len(m.entries()))
	}
	out, cmd := m.toggleFold()
	if cmd != nil {
		t.Errorf("toggleFold sin fila devolvió %#v, want nil", cmd())
	}
	if got := out.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d tras plegar sin fila, want 0", got)
	}
}

// NotifyConfig es lo que main llama con el aviso de config, y lo que hace que el
// usuario lo vea: stderr se queda detrás del alt screen, así que si el aviso no
// llegara al modelo se perdería sin más. Se comprueba que encola un toast con el
// texto, no que "no reviente".
func TestNotifyConfigEncolaElAviso(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.NotifyConfig("config: no se pudo leer el fichero")
	if len(m.toasts.toasts) != 1 {
		t.Fatalf("toasts = %d, want 1 (el aviso tiene que verse, no ir a stderr)", len(m.toasts.toasts))
	}
	got := m.toasts.toasts[0]
	if got.level != toastWarning {
		t.Errorf("nivel = %v, want warning (un aviso de config no es un exito)", got.level)
	}
	if !strings.Contains(got.text, "no se pudo leer") {
		t.Errorf("texto = %q, want el aviso entero", got.text)
	}
	// Y sale en lo que se pinta de verdad, que es donde el usuario lo lee: los
	// bloques del overlay. Con un bloque vacio el aviso no existiria en pantalla.
	bloques := m.toasts.blocks()
	if len(bloques) != 1 {
		t.Fatalf("bloques = %d, want 1", len(bloques))
	}
	pintado := stripANSI(strings.Join(bloques[0], " "))
	if !strings.Contains(pintado, "no se pudo leer") {
		t.Errorf("el bloque pintado = %q, want el texto del aviso", pintado)
	}
}

// saveCollapsed con store nil no hace nada. Es el caso de un modelo construido
// sin store (por ejemplo un test, o el modo --print si compartiera modelo), y la
// guarda evita un nil-pointer en CADA tecla de plegado.
func TestSaveCollapsedSinStoreNoRevienta(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.store = nil
	m.collapsed = map[string]bool{"backend": true}
	m.saveCollapsed() // no debe hacer nada, y sobre todo no reventar
	if !m.collapsed["backend"] {
		t.Error("el estado en memoria cambio: saveCollapsed no debe tocarlo")
	}
}

// visualOptionForKey con una tecla que no es ninguna variante devuelve false, y
// el selector visual usa ese false para no hacer nada. Sin ese caso, una tecla
// desconocida se traduciria a una variante de git-sim inventada.
func TestVisualOptionParaTeclaDesconocida(t *testing.T) {
	for _, tecla := range []string{"", "x", "enter", "ESC", "mm"} {
		if o, ok := visualOptionForKey(tecla); ok {
			t.Errorf("visualOptionForKey(%q) = %+v, want no (no es ninguna variante)", tecla, o)
		}
	}
	// Y las que sí son, con su subcomando, para que el fallback no se confunda
	// con un caso vacío.
	for _, o := range visualOptions {
		got, ok := visualOptionForKey(o.key)
		if !ok {
			t.Errorf("visualOptionForKey(%q) = no, want %+v", o.key, o)
		}
		if got.sub != o.sub {
			t.Errorf("visualOptionForKey(%q).sub = %q, want %q", o.key, got.sub, o.sub)
		}
	}
}

// tickCmd devuelve un tea.Tick de un segundo, y el closure que emite tickMsg solo
// se ejecuta cuando esa tea.Cmd se invoca. El caso importa porque tickMsg es lo
// que expira los toasts: si el tick dejara de emitirse, un aviso se quedaría
// pintado para siempre y la tabla parecería congelada.
//
// Cuesta un segundo porque el timer es real: no hay reloj inyectable en
// bubbletea, y falsearlo exigiria el seam entero de tea.Tick por un segundo de
// suite. Un segundo en la suite entera es un precio aceptable.
func TestTickCmdEmiteElTick(t *testing.T) {
	start := time.Now()
	msg := tickCmd()()
	if _, ok := msg.(tickMsg); !ok {
		t.Fatalf("tickCmd()() = %#v, want un tickMsg", msg)
	}
	if d := time.Since(start); d < 900*time.Millisecond {
		t.Errorf("el tick volvio en %v, want ~1s (un tick que no espera no expira nada)", d)
	}
}

// --- pump de eventos ---

// Init tiene que arrancar las tres cosas de las que depende el arranque: el
// scan, la bomba de eventos y el tick. No se comprueba que hagan su trabajo (eso
// lo cubren los tests de pipeline), sino que el Cmd existe: un `Init` que
// devolviera nil arrancaría una app sana que nunca vuelve a pintar.
func TestInitArrancaElPipeline(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	if cmd := m.Init(); cmd == nil {
		t.Error("Init = nil, want un Cmd: sin scan, sin pump y sin tick la app no arranca")
	}
}

// La bomba lee UN evento por Cmd (el patrón de tea) y se rearma tras cada uno.
// Con el canal cerrado tiene que devolver nil, no un evento vacío: un `nil` es
// lo que la app distingue de "hay trabajo", y un valor no-nil con el canal
// cerrado la dejaría repintando para siempre.
func TestWaitForEvent(t *testing.T) {
	t.Run("entrega el evento y se rearma", func(t *testing.T) {
		ch := make(chan event, 1)
		ch <- tickMsg{}
		if ev := waitForEvent(ch)(); ev == nil {
			t.Fatal("waitForEvent no devolvio el evento pendiente")
		}
		ch <- tickMsg{}
		if ev := waitForEvent(ch)(); ev == nil {
			t.Error("la bomba no se rearma: el segundo evento nunca llega")
		}
	})

	t.Run("canal cerrado devuelve nil", func(t *testing.T) {
		ch := make(chan event)
		close(ch)
		if ev := waitForEvent(ch)(); ev != nil {
			t.Errorf("waitForEvent con el canal cerrado = %#v, want nil", ev)
		}
	})
}

// El PRODUCTOR de `collectDoneMsg`: el scan tiene que emitirlo cuando la
// recolección se acaba. Los demás tests lo inyectan a mano, así que sin este la
// línea que lo emite —y con ella el cierre real del scan— no la ejercita
// nadie: el cierre se vería bien en los tests y no llegaría nunca en la app.
func TestElScanEmiteCollectDoneAlTerminar(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "api")
	testutil.Init(t, repo)
	testutil.Marker(t, repo, "api", "", "", false)
	testutil.CommitFiles(t, repo, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")

	cfg := config.Defaults()
	cfg.Roots = []string{root}
	m := New(cfg)
	t.Cleanup(m.cancel)

	m.startScanCmd()
	if !esperaEvento(t, m, 20*time.Second, func(ev event) bool {
		_, ok := ev.(collectDoneMsg)
		return ok
	}) {
		t.Fatal("el scan no emitió collectDoneMsg: el pipeline nunca cierra")
	}
}

// --- a quién se le hace fetch automático ---

// `fetchTargets` decide a qué repos se les hace fetch tras un scan. Los tres
// filtros son la razón de que el fetch automático no golpee repos sin
// upstream, sin repo, ni uno que ya está en curso: los tres son ruido en
// el log de git y, en el caso del que ya está en curso, dos `git fetch` en
// paralelo sobre el mismo repo.
func TestFetchTargetsSoloLosReposQueProcede(t *testing.T) {
	conRepo := "/tmp/con-repo"
	sinRepo := "/tmp/sin-repo"
	enCurso := "/tmp/en-curso"
	m := newTestModel(t, []discovery.Project{
		proj("con-repo", conRepo, true),
		proj("sin-repo", sinRepo, false),
		proj("en-curso", enCurso, true),
	}, map[string]gitstatus.Snapshot{
		conRepo: snapClean(),
		sinRepo: snapClean(),
		enCurso: snapClean(),
	})
	m.fetchStates[enCurso] = "fetching"

	got := m.fetchTargets()
	if len(got) != 1 || got[0] != conRepo {
		t.Errorf("fetchTargets = %v, want solo [%s]", got, conRepo)
	}
}

// Un snapshot con error (por ejemplo el repo roto) tampoco se fetchea: no hay
// nada que traer de un repo que ni siquiera abre.
func TestFetchTargetsSaltaElRepoConError(t *testing.T) {
	roto := "/tmp/roto"
	bueno := "/tmp/bueno"
	m := newTestModel(t, []discovery.Project{
		proj("roto", roto, true), proj("bueno", bueno, true),
	}, map[string]gitstatus.Snapshot{
		roto:  {Err: "no such repository"},
		bueno: snapClean(),
	})
	got := m.fetchTargets()
	if len(got) != 1 || got[0] != bueno {
		t.Errorf("fetchTargets = %v, want solo [%s]", got, bueno)
	}
}

// `rescan` arranca un pipeline nuevo. No se mira el Cmd que devuelve
// (startScanCmd devuelve nil siempre: publica por el canal), sino el estado que
// deja: sin `scanning` la app se creería que no hay nada en curso y el segundo
// rescan se colaría sin avisar.
func TestRescanCuandoNoHayScanEnCurso(t *testing.T) {
	m := newTestModel(t, []discovery.Project{proj("api", "/tmp/api", true)},
		map[string]gitstatus.Snapshot{"/tmp/api": snapClean()})
	m.scanning = false
	m, _ = press(m, "r")
	if !m.scanning {
		t.Error("rescan no marco el scan como en curso")
	}
}

// El límite de concurrencia del fetch es un semáforo, y su rama de "cancelado
// esperando hueco" no es decorativa: un repo que no llega a ejecutarse no debe
// contarse ni como ok ni como fallo (si no, `fetch all` sobre 20 repos
// cerraría con "1 ok, 19 failed" tras un simple Ctrl-C).
//
// Inline en el `select` de la goroutine, esta rama no se podía probar sin una
// carrera (cancelar mientras N goroutines esperan turno). Con `adquirirSlot`
// suelta, llenar el semáforo y cancelar es determinista.
func TestAdquirirSlot(t *testing.T) {
	t.Run("con hueco lo toma", func(t *testing.T) {
		sem := make(chan struct{}, 2)
		if !adquirirSlot(context.Background(), sem) {
			t.Error("adquirirSlot = false con un hueco libre, want true")
		}
		if len(sem) != 1 {
			t.Errorf("el hueco no se ocupo: len(sem) = %d, want 1", len(sem))
		}
	})

	t.Run("cancelado sin hueco no lo toma", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		sem <- struct{}{} // ocupado: el siguiente tiene que esperar
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if adquirirSlot(ctx, sem) {
			t.Error("adquirirSlot = true con el semáforo lleno y el contexto cancelado, want false")
		}
		if len(sem) != 1 {
			t.Errorf("ocupó un hueco que no era suyo: len(sem) = %d, want 1", len(sem))
		}
	})

	// Y el caso que de verdad importa en la app: sin cancelar, esperar un hueco
	// que se libera tiene que concederse. Un `select` mal escrito (priorizando
	// ctx.Done) passesía por aquí y devolvería false sin motivo.
	t.Run("espera a que se libere", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		sem <- struct{}{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-sem // libera como lo haría el fetch anterior
		}()
		if !adquirirSlot(ctx, sem) {
			t.Error("adquirirSlot = false esperando un hueco que se liberaba, want true")
		}
	})
}

// gitLentoEspera devuelve un shim de `git` que se cuela en el PATH y se queda
// esperando 30s en cada `fetch`, antes de delegar el resto de verbos en el git de
// verdad. Es lo que hace reproducible la cola del fetch sin red: con concurrency=1
// el primer fetch retiene el único hueco 30s, así que los demás están de verdad
// esperando turno cuando se cancela.
//
// `exec`, no `sleep & wait`: con un nieto, el SIGKILL de CommandContext mata al
// shim pero el nieto se queda con el pipe de stdout y Output() no vuelve nunca
// (medido). Con exec hay un solo proceso y se mata entero.
// `marcar` es el fichero que el shim toca al arrancar un fetch: es la señal de
// "este repo está DENTRO del subprocess ahora mismo". Sin ella no hay forma de
// saber cuándo el hueco quedó ocupado, porque recordExec solo corre cuando el
// subprocess TERMINA (y este se cuelga 30s a propósito).
func gitLentoEspera(t *testing.T, marcar string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git no disponible")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"fetch\" ]; then touch \"" + marcar + "\"; exec sleep 30; fi\n" +
		"exec " + real + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// esperaDentroDelShim espera a que algún fetch haya arrancado de verdad.
func esperaDentroDelShim(t *testing.T, marcar string, plazo time.Duration) {
	t.Helper()
	deadline := time.Now().Add(plazo)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marcar); err == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("ningún fetch llegó a arrancar en %s", plazo)
}

// fetchEnCola es cuántos repos esperan turno en ESTE test: uno tiene el hueco y
// el resto hace cola.
const fetchEnCola = 12

// El guard de `adquirirSlot` DENTRO del batch: un repo que se cancela mientras
// ESPERA TURNO no llega a ejecutar `git fetch`.
//
// El shim retiene el hueco; el command log es lo que se mira. Y hay una razón
// concreta para NO mirar otra cosa, que se tardó tres intentos en averiguar:
//
//   - El shim no sirve: con el contexto ya cancelado, `exec.CommandContext` ni
//     siquiera lanza el binario, así que el shim no escribe tanto si el guard
//     funciona como si no. Medido.
//   - `fetchStateMsg`/`fetchDoneMsg` tampoco: `sendEvent` con el ctx cancelado
//     entrega el mensaje la mitad de las veces (981/2000), así que un test que
//     mira el canal PASA POR ACCIDENTE la mitad de las veces con el bug dentro.
//     Falso negativo del 50%, que es peor que no tener test.
//   - `rec.Entries()` sí: `recordExec` se llama desde `runGit` SIEMPRE, tanto si
//     el subprocess arrancó como si no. Es el único registro del intento, y por
//     eso un fetch que pasó el guard deja entrada y uno que no, no.
func TestFetchBatchLosCanceladosEnEsperaNoEjecutanGit(t *testing.T) {
	marcar := filepath.Join(t.TempDir(), "dentro")
	gitLentoEspera(t, marcar)

	paths := make([]string, fetchEnCola)
	projects := make([]discovery.Project, fetchEnCola)
	states := make(map[string]gitstatus.Snapshot, fetchEnCola)
	for i := range paths {
		// Los dirs tienen que EXISTIR: con `cmd.Dir` inexistente, Start falla
		// antes de ejecutar el shim y el test esperaría una entrada que no llega.
		paths[i] = t.TempDir()
		projects[i] = proj(fmt.Sprintf("repo-%02d", i), paths[i], true)
		states[paths[i]] = snapClean()
	}
	m := newTestModel(t, projects, states)
	rec := cmdlog.Active()
	t.Cleanup(func() { cmdlog.SetRecorder(nil) }) // el log del test siguiente
	m.cfg.FetchConcurrency = 1                    // un solo hueco: el resto TIENE que esperar

	m.fetchBatchCmd(paths, cmdlog.ClassAction)

	// El primero retiene el hueco: su fetch ya está DENTRO del subprocess (el
	// shim lo ha tocado). Los demás ya emitieron su "fetching" y están esperando.
	esperaDentroDelShim(t, marcar, 10*time.Second)

	m.cancel()

	// Margen para que un fetch que hubiera pasado el guard llegue a registrar.
	// Si va a arrancar, arranca enseguida: el shim se cuelga 30s después.
	time.Sleep(time.Second)

	var ejecutados []string
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "fetch" {
			continue
		}
		ejecutados = append(ejecutados, e.Dir)
	}
	if len(ejecutados) > 1 {
		t.Errorf("fetches que salieron a subprocess = %v, want solo el primero: %d de %d en cola llegaron a ejecutar pese a la cancelación",
			ejecutados, len(ejecutados)-1, fetchEnCola-1)
	}
}
