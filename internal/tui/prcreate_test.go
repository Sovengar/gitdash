// Tests de la creación de PR/MR: la ejecución de lo que el overlay recogió.
// Estilo del repo: modelo directo (New + Update + inspección), sin teatest.
package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/forge"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// prRepo crea un repo git real con origin apuntando a la URL dada ("" = sin
// remoto) y devuelve su path. Tiene que existir de verdad: la acción lee
// `git remote get-url origin` en ese directorio, así que un path simulado
// fallaría antes de llegar al forge.
func prRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Init(t, dir)
	if remote != "" {
		gitOutT(t, dir, "remote", "add", "origin", remote)
	}
	return dir
}

// prModel monta el modelo sobre ese repo con el cursor en su fila, y devuelve
// el recorder del command log (que New instala).
func prModel(t *testing.T, dir string) (Model, *cmdlog.Recorder) {
	t.Helper()
	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapDirty(1, 0)})
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	return cursorOn(t, m, dir), cmdlog.Active()
}

// forgeStub escribe una CLI de forge falsa en un directorio propio, lo pone
// primero en el PATH y devuelve el fichero donde el stub vuelca su argv. La CLI
// real no se puede ejercitar sin red ni token: lo que se prueba es lo que
// gitdash hace alrededor —el argv que compone, el que ejecuta y lo que
// registra—, y el stub es lo que cierra ese círculo sin salir a la red.
func forgeStub(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argvFile + "\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile
}

// sinBinario deja el PATH con lo justo para leer el remoto y nada más: git por
// symlink (la lectura del remote lo necesita) y ninguna CLI de forge, que es lo
// que hace alcanzable el camino de LookPath.
func sinBinario(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git no disponible")
	}
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// prConfig carga un config.toml de verdad y devuelve su sección [forge.*]: el
// camino completo (LoadFrom → mapa de hosts y prefijos → argv) es lo que hay
// que probar, no el mapa montado a mano.
func prConfig(t *testing.T, toml string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := config.LoadFrom(path)
	if warn != "" {
		t.Fatalf("config: %s", warn)
	}
	return cfg
}

// prEnvíaPR lleva el modelo desde el dashboard hasta la creación lanzada: abre
// el overlay, escribe el título y envía, incluido el salto a prStartMsg. El Cmd
// que devuelve es el que llega al runtime después, que en el caso de un repo
// ocupado es el aviso de "ya hay una acción en curso".
func prEnvíaPR(t *testing.T, m Model, title string) (Model, tea.Cmd) {
	t.Helper()
	m = openPROverlay(t, m)
	m = typeText(m, title)
	m, cmd := press(m, prSubmitKey)
	if cmd == nil {
		t.Fatal("el submit no devolvió cmd")
	}
	msg := cmd()
	if _, ok := msg.(prStartMsg); !ok {
		t.Fatalf("submit = %T, want prStartMsg", msg)
	}
	out, start := m.Update(msg)
	return out.(Model), start
}

// awaitPR espera el desenlace y, si hubo ejecución, también el statusMsg del
// recollect que la acompaña: sin esperarlo, ese Collect sigue vivo cuando el
// test acaba y puede registrar sus lecturas en el recorder del siguiente.
func awaitPR(t *testing.T, m *Model) prResultMsg {
	t.Helper()
	var res prResultMsg
	waitEvent(t, m, func(ev event) bool {
		r, ok := ev.(prResultMsg)
		if ok {
			res = r
		}
		return ok
	})
	if res.reject == "" {
		waitEvent(t, m, func(ev event) bool {
			_, ok := ev.(statusMsg)
			return ok
		})
	}
	return res
}

// prExec devuelve la entrada de ejecución de la acción pr, o nil. Se busca por
// acción y no por posición: el recollect deja lecturas detrás.
func prExec(rec *cmdlog.Recorder) *cmdlog.Entry {
	var got *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "pr" {
			continue
		}
		cp := e
		got = &cp
	}
	return got
}

// lastToast devuelve el texto del último aviso vivo, o "" si no hay ninguno.
func lastToast(m Model) string {
	ts := m.toasts.toasts
	if len(ts) == 0 {
		return ""
	}
	return ts[len(ts)-1].text
}

// --- el ciclo completo ---

// El camino entero: overlay → submit → gh ejecutado → exec registrada con el
// argv resuelto. Es el test que ata las tres piezas (config, forge, tool) al
// dashboard; sin él cada una pasaría por su lado y no se crearía nada.
func TestPRCreacionCompletaRegistraElArgvResuelto(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	argvFile := forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/42")
	m, rec := prModel(t, dir)

	m, _ = prEnvíaPR(t, m, "Add the sync branch base")
	if m.running[dir] != "pr" {
		t.Fatalf("running = %q, want pr (la creación bloquea la segunda pulsación)", m.running[dir])
	}

	res := awaitPR(t, &m)
	if res.reject != "" {
		t.Fatalf("la creación se rechazó: %s", res.reject)
	}
	if res.err != nil {
		t.Fatalf("gh falló: %v", res.err)
	}

	// El argv que registra el log es el que se ejecutó, y sale del REMOTE del
	// repo: sin -R, gh deduciría el destino y el PR podría acabar creado en
	// otro sitio sin que nada fallara.
	e := prExec(rec)
	if e == nil {
		t.Fatal("no se registró el exec de la creación")
	}
	want := []string{
		"gh", "pr", "create",
		"-t", "Add the sync branch base",
		"-b", "",
		"-B", "main",
		"-H", "main",
		"-R", "acme/widget",
	}
	if !reflect.DeepEqual(e.Argv, want) {
		t.Errorf("argv = %#v\nwant %#v", e.Argv, want)
	}
	if e.Class != cmdlog.ClassAction {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassAction)
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	// A diferencia de los handoffs, aquí SÍ se mide: el proceso corre detrás con
	// la salida capturada, así que su duración existe.
	if e.Dur <= 0 {
		t.Error("Dur sin medir: la creación no es un handoff de terminal")
	}
	if e.Repo != filepath.Base(dir) || e.Dir != dir {
		t.Errorf("Repo/Dir = %q/%q, want %q/%q", e.Repo, e.Dir, filepath.Base(dir), dir)
	}
	// Y el proceso llegó a correr de verdad, con esos mismos argumentos.
	ran, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("el stub no dejó su argv: %v", err)
	}
	if got := strings.Split(strings.TrimRight(string(ran), "\n"), "\n"); got[0] != "pr" || got[1] != "create" {
		t.Errorf("argv ejecutado = %v, want el de gh pr create", got)
	}
	// Al volver, el repo queda libre y el aviso lleva la URL: es lo que el
	// usuario quiere del toast.
	if m.running[dir] != "" {
		t.Errorf("el repo sigue ocupado: %q", m.running[dir])
	}
	if got := lastToast(m); !strings.Contains(got, "https://github.com/acme/widget/pull/42") {
		t.Errorf("toast = %q, want la URL del PR", got)
	}
}

// La pulsación deja intención en el log. Sin esa línea, el panel solo diría que
// corrió un gh y no de dónde salió la orden: con la política de pull delegada en
// el gitconfig, el log existe justo para eso.
func TestPRDejaIntencionEnElLog(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m, _ = prEnvíaPR(t, m, "un título")
	awaitPR(t, &m)

	var intent *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent && e.Action == "pr" {
			cp := e
			intent = &cp
		}
	}
	if intent == nil {
		t.Fatal("la acción pr no dejó intención en el log")
	}
	if intent.Key != "O" {
		t.Errorf("Key = %q, want O (la tecla de la acción)", intent.Key)
	}
	if intent.Dir != dir {
		t.Errorf("Dir = %q, want el repo de la fila", intent.Dir)
	}
}

// Con el log abierto la tecla de PR no hace nada: el formulario ocuparía el
// cuerpo y el panel ya lo ocupa. Y no deja ni una intención fantasma de algo que
// no ocurrió.
func TestPRNoAbreConElLogAbierto(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	m, _ = press(m, "l") // abre el panel del log
	before := len(rec.Entries())

	m, cmd := press(m, "O")

	if m.pr != nil {
		t.Error("se abrió el overlay con el panel del log abierto")
	}
	if cmd != nil {
		t.Errorf("la tecla launchó algo: %v", cmd())
	}
	if len(rec.Entries()) != before {
		t.Error("la tecla dejó una entrada en el log por algo que no ocurrió")
	}
}

// El head del PR es la branch del repo: sin -H, gh deduciría la rama y el PR
// podría salir contra otra.
func TestPRElHeadSaleDelSnapshot(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	snap := m.states[dir]
	snap.Status.Branch = "feat/pr"
	m.states[dir] = snap

	m, _ = prEnvíaPR(t, m, "un título")
	awaitPR(t, &m)

	e := prExec(rec)
	if e == nil {
		t.Fatal("no se registró el exec")
	}
	if !containsPair(e.Argv, "-H", "feat/pr") {
		t.Errorf("argv = %v, want -H feat/pr", e.Argv)
	}
}

// --- el argv no confiable ---

// El argv del panel lleva el título y el cuerpo, que escribió una persona: si
// una secuencia de escape llegara a pintarse, el panel inyectaría en la
// terminal. El registro guarda el argv CRUDO (es lo que se ejecutó) y lo sanea
// quien PINTA, así que la comprobación es sobre la vista.
func TestPRElArgvDelPanelVaSaneado(t *testing.T) {
	// Dos caminos distintos hacia el mismo límite. El pegado con un OSC (que
	// secuestra el título de la terminal) no pasa por ningún filtro: el widget
	// de bubbles quita los caracteres de control pero no las secuencias, y el
	// panel no puede depender de que todos los caminos filtren. Los caracteres
	// de formato (Cf: bidi, zero-width) sí llegan por el camino real del
	// formulario, porque no son de control.
	casos := []struct {
		nombre   string
		title    string
		visible  string   // la parte del título que tiene que verse igual
		pegado   bool     // el título entra por el envío, no por el input
		injected []string // lo que NO puede aparecer en la vista
	}{
		{"osc", "\x1b]0;secuestrado\x07rojo final", "rojo final", true, []string{"\x1b]0;"}},
		{"bidi", "titulo\u202Emid\u202C", "titulo", false, []string{"\u202e", "\u202c"}},
		{"zero-width", "antes\u200Bdespues", "antes", false, []string{"\u200b"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dir := prRepo(t, "git@github.com:acme/widget.git")
			forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/7")
			m, rec := prModel(t, dir)

			if c.pegado {
				// El envío entra ya publicado: es el camino de un pegado que
				// ningún widget filtró.
				m.prPending = &prSubmission{path: dir, params: forge.Params{
					Title: c.title, Base: "main", Head: "main",
				}}
				out, _ := m.Update(prStartMsg{})
				m = out.(Model)
			} else {
				m = openPROverlay(t, m)
				m.pr.title.SetValue(c.title)
				var cmd tea.Cmd
				m, cmd = press(m, prSubmitKey)
				if cmd == nil {
					t.Fatal("el submit no devolvió cmd")
				}
				out, _ := m.Update(cmd())
				m = out.(Model)
			}
			awaitPR(t, &m)

			// El argv se ejecutó tal cual: el registro no miente sobre lo que
			// corrió (y el texto llegó al argv, que es lo que hace meaningful
			// la comprobación de la vista).
			e := prExec(rec)
			if e == nil {
				t.Fatal("no se registró el exec")
			}
			if !containsPair(e.Argv, "-t", c.title) {
				t.Fatalf("argv = %v, want el título íntegro como un elemento", e.Argv)
			}

			m, _ = press(m, "l") // abre el panel del log
			painted := m.View().Content
			for _, s := range c.injected {
				if strings.Contains(painted, s) {
					t.Errorf("el texto no confiable %q llegó a la vista", s)
				}
			}
			// Y el argv se sigue viendo: el saneo quita lo peligroso, no la
			// línea entera. La parte visible del título (la que queda antes de
			// la secuencia) tiene que estar, o la comprobación anterior sería
			// vacuía por un recorte de columna.
			visible := stripANSI(painted)
			if !strings.Contains(visible, "pr create") {
				t.Errorf("el panel no pintó el argv:\n%s", visible)
			}
			if !strings.Contains(visible, c.visible) {
				t.Errorf("el saneo se llevó el argv entero:\n%s", visible)
			}
		})
	}
}

// --- cuando no se puede crear ---

// Sin remote no hay destino: toast que dice qué configurar, y NADA ejecutado
// (ni proceso, ni entrada en el log, ni recollect).
func TestPRSinRemoteNoEjecutaNada(t *testing.T) {
	dir := prRepo(t, "") // repo sin origin
	argvFile := forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m, _ = prEnvíaPR(t, m, "un título")
	res := awaitPR(t, &m)

	if res.reject == "" {
		t.Fatal("sin remote debería rechazarse")
	}
	if !strings.Contains(res.reject, "no origin remote") {
		t.Errorf("aviso = %q, want menciona el remote que falta", res.reject)
	}
	if prExec(rec) != nil {
		t.Error("se registró un exec sin remote")
	}
	if _, err := os.Stat(argvFile); err == nil {
		t.Error("la CLI se ejecutó sin remote")
	}
	if m.running[dir] != "" {
		t.Errorf("el repo quedó ocupado: %q", m.running[dir])
	}
	if got := lastToast(m); !strings.Contains(got, "no origin remote") {
		t.Errorf("toast = %q, want el aviso de remote", got)
	}
}

// Un host que no está declarado en [forge.*] no es un forge: se dice QUÉ
// hacer, porque si no la acción falla en silencio y parece un bug.
func TestPRForgeDesconocidoNoEjecutaNada(t *testing.T) {
	dir := prRepo(t, "git@git.example.com:acme/widget.git")
	argvFile := forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m, _ = prEnvíaPR(t, m, "un título")
	res := awaitPR(t, &m)

	if res.reject == "" {
		t.Fatal("un host sin declarar debería rechazarse")
	}
	for _, want := range []string{"no forge", "[forge.github]", "[forge.gitlab]"} {
		if !strings.Contains(res.reject, want) {
			t.Errorf("aviso = %q, want menciona %q", res.reject, want)
		}
	}
	if prExec(rec) != nil {
		t.Error("se registró un exec con un forge desconocido")
	}
	if _, err := os.Stat(argvFile); err == nil {
		t.Error("la CLI se ejecutó sin forge conocido")
	}
}

// Sin gh (ni glab) instalado se avisa, como con lazygit, y no se inventa un
// ejecutable.
func TestPRSinBinarioNoEjecutaNada(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	sinBinario(t)
	m, rec := prModel(t, dir)

	m, _ = prEnvíaPR(t, m, "un título")
	res := awaitPR(t, &m)

	if !strings.Contains(res.reject, "gh not installed") {
		t.Errorf("aviso = %q, want \"gh not installed\"", res.reject)
	}
	if prExec(rec) != nil {
		t.Error("se registró un exec sin binario")
	}
	if m.running[dir] != "" {
		t.Errorf("el repo quedó ocupado: %q", m.running[dir])
	}
}

// Con una acción ya en curso, el segundo envío no relanza: el aviso lo dice y
// el envío se consume (no se queda en cola para ejecutarse más tarde).
func TestPRNoRelanzaConElRepoOcupado(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	m.running[dir] = "lazygit"

	m, cmd := prEnvíaPR(t, m, "un título")
	m = applyNotify(m, cmd)

	if m.running[dir] != "lazygit" {
		t.Errorf("running = %q, want la acción original intacta", m.running[dir])
	}
	if m.prPending != nil {
		t.Error("el envío quedó en cola: se ejecutaría más tarde sin pedirlo")
	}
	if prExec(rec) != nil {
		t.Error("se registró un exec con el repo ocupado")
	}
	if got := lastToast(m); !strings.Contains(got, "already running") {
		t.Errorf("toast = %q, want el aviso de acción en curso", got)
	}
}

// --- la instancia self-managed ---

// El caso que motiva la config de forges: un GitLab en /git/. El argv sale
// contra `glab mr create` y el proyecto es la ruta DENTRO de la instancia, sin
// el prefijo de subcarpeta (que no es parte de la ruta del proyecto). Todo el
// camino, desde el config.toml escrito a mano.
func TestPRGitLabSelfManagedQuitaElPrefijoDeSubcarpeta(t *testing.T) {
	dir := prRepo(t, "git@git.example.com:grupo/sub/widget.git")
	argvFile := forgeStub(t, "glab", "echo https://git.example.com/git/grupo/sub/widget/-/merge_requests/3")
	m, rec := prModel(t, dir)
	m.cfg.Forges = prConfig(t, `
[forge.gitlab]
api_base = "https://git.example.com/git/api/v4/"
hosts = ["git.example.com"]
`).Forges

	m, _ = prEnvíaPR(t, m, "Corregir la subcarpeta")

	res := awaitPR(t, &m)
	if res.reject != "" {
		t.Fatalf("la creación se rechazó: %s", res.reject)
	}
	if res.err != nil {
		t.Fatalf("glab falló: %v", res.err)
	}

	e := prExec(rec)
	if e == nil {
		t.Fatal("no se registró el exec")
	}
	want := []string{
		"glab", "mr", "create",
		"-t", "Corregir la subcarpeta",
		"-d", "",
		"-b", "main",
		"-s", "main",
		"-y", "-R", "grupo/sub/widget",
	}
	if !reflect.DeepEqual(e.Argv, want) {
		t.Errorf("argv = %#v\nwant %#v", e.Argv, want)
	}
	// glab no tiene --hostname en `mr create`: la instancia se elige con
	// GITLAB_HOST, que es lo que evita que hable con gitlab.com. El stub no
	// lo comprueba (eso lo prueba forge/tool), pero el proceso tiene que haber
	// salido con ese argv.
	ran, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("el stub no dejó su argv: %v", err)
	}
	if got := strings.Split(strings.TrimRight(string(ran), "\n"), "\n"); got[0] != "mr" {
		t.Errorf("argv ejecutado = %v, want el de glab mr create", got)
	}
}

// --- el fallo de la CLI ---

// Un fallo de gh sale con su motivo por stderr: el toast lo enseña (el de
// tool.Error, no su Error() completo, que repetiría el argv entero) y el log
// registra el código de salida real.
func TestPRFalloDeLaCLIToastConElMotivo(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo 'could not create PR: head branch already exists' >&2\nexit 1")
	m, rec := prModel(t, dir)

	m, _ = prEnvíaPR(t, m, "un título")
	res := awaitPR(t, &m)

	if res.err == nil {
		t.Fatal("la creación debía fallar")
	}
	if got := lastToast(m); !strings.Contains(got, "head branch already exists") {
		t.Errorf("toast = %q, want el motivo de la CLI", got)
	}
	e := prExec(rec)
	if e == nil {
		t.Fatal("el fallo no se registró en el log")
	}
	if e.Exit != 1 {
		t.Errorf("Exit = %d, want 1", e.Exit)
	}
	if m.running[dir] != "" {
		t.Errorf("el repo quedó ocupado: %q", m.running[dir])
	}
}

// --- la tecla del aviso ---

// El aviso del overlay nombra la tecla por la config: con `pr` rebindeada, un
// aviso escrito a mano dejaría al usuario leyendo una tecla que ya no abre nada.
func TestPRPromptNombraLaTeclaDeLaConfig(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m.cfg.Keybindings["pr"] = "W"

	got := m.prPrompt()

	if !strings.Contains(got, "W opens") {
		t.Errorf("el aviso no nombra la tecla configurada:\n%s", got)
	}
	if strings.Contains(got, "O opens") {
		t.Errorf("el aviso nombra la tecla por defecto:\n%s", got)
	}
	// Y se ve en la sección de keybinds, que es donde se pinta.
	kb := sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if !strings.Contains(kb, "W opens") {
		t.Errorf("keybinds sin la tecla configurada:\n%s", kb)
	}
}

// El envío sin título no lanza nada: el aviso de validación va en el panel y no
// se llega a prCreateCmd.
func TestPREnvioInvalidoNoLanza(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m = openPROverlay(t, m)
	m, cmd := press(m, prSubmitKey)

	if cmd != nil {
		t.Errorf("un envío inválido lanzó algo: %v", cmd())
	}
	if m.prPending != nil {
		t.Error("se publicó un envío sin título")
	}
	if prExec(rec) != nil {
		t.Error("se registró un exec de un envío inválido")
	}
}

// containsPair dice si el argv tiene el flag seguido de su valor (los flags van
// sueltos, como elementos, nunca pegados al valor).
func containsPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}
