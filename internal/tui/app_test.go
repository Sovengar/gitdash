// Tests del modelo TUI con datos sintéticos (patrón de vroom): sin teatest,
// se construye el Model, se le envían mensajes y se inspecciona el estado.
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
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
