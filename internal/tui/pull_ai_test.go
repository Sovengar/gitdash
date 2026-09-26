// Tests de "pull con IA": la variante `a` del selector lanza directamente un
// handoff al comando AI configurado globalmente, con el prompt del marcador
// como un único elemento de argv. Estilo del repo: Model directo + Update.
package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// newAIModel construye un modelo con un repo real en dir (el marcador tiene que
// existir en disco porque `a` lo relee), el prompt dado en su marcador y el
// comando AI en la config global. Devuelve el modelo con el cursor ya en el repo.
func newAIModel(t *testing.T, dir, prompt, command string) Model {
	t.Helper()
	content := ""
	if prompt != "" {
		content = "[ai.pull]\nprompt = \"" + prompt + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p := proj("demo", dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapClean()})
	if command != "" {
		m.cfg.AICommands = map[string]string{"pull": command}
	}
	return cursorOn(t, m, dir)
}

// Sin prompt en el marcador no se lanza nada: no hay nada que pedirle a la IA.
func TestPullAINoLanzaSinPrompt(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "", "/bin/echo {prompt}")

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a lanzó el handoff sin prompt: running = %q", m.running[dir])
	}
	if m.pullArmed != nil {
		t.Error("el selector quedó armado tras elegir la variante AI")
	}
	if cmd == nil {
		t.Fatal("sin prompt debería notificar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no AI pull prompt") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Con prompt pero sin comando AI configurado tampoco hay handoff.
func TestPullAINoLanzaSinCommand(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla el rebase", "")

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a lanzó el handoff sin comando configurado: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("sin comando debería notificar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "command not configured") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Un binario inexistente se detecta con LookPath: toast, sin handoff.
func TestPullAINoLanzaSinBinario(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla el rebase", "definitely-not-a-bin-xyz {prompt}")

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a lanzó el handoff con un binario inexistente: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("sin binario debería notificar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "not installed") {
		t.Errorf("notificación = %v", cmd())
	}
}

// `p a` lanza el handoff en el mismo acto, sin estado armado intermedio.
func TestPullAILanzaElHandoffDirecto(t *testing.T) {
	if _, err := exec.LookPath("/bin/echo"); err != nil {
		t.Skip("/bin/echo no disponible")
	}
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla el rebase", "/bin/echo {prompt}")

	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("precondición: p no armó el selector")
	}
	m, cmd := press(m, "a")

	if m.pullArmed != nil {
		t.Error("el selector sigue armado: a no lanzó directo")
	}
	if m.running[dir] != "pull_ai" {
		t.Errorf("running = %q, want pull_ai", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("el handoff no devolvió tea.Cmd")
	}
}

// Con una acción ya en curso en el repo, `a` no relanza.
func TestPullAIBlockeadoSiYaCorre(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla el rebase", "/bin/echo {prompt}")
	m.running[dir] = "lazygit"

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if m.running[dir] != "lazygit" {
		t.Errorf("running = %q, want el original intacto", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("con la acción en curso debería notificar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notificación = %v", cmd())
	}
}

// Regresión del selector: esc y una tecla no-variante no lanzan nada.
func TestSelectorEscYTeclaNoVariante(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "p")
	m, _ = press(m, "esc")
	if m.pullArmed != nil || len(m.running) != 0 {
		t.Errorf("esc dejó estado: armado=%v running=%v", m.pullArmed, m.running)
	}

	m2 := newPullModel(t)
	start := m2.cursor
	m2, _ = press(m2, "p")
	m2, _ = press(m2, "j")
	if m2.pullArmed != nil {
		t.Error("la tecla no-variante no canceló el selector")
	}
	if m2.cursor == start {
		t.Error("la tecla no-variante no siguió su curso normal (cursor quieto)")
	}
	if len(m2.running) != 0 {
		t.Errorf("la cancelación lanzó una acción: %v", m2.running)
	}
}

// pull_ai no es una variante de pull de git: no entra en PullKinds ni pasa por
// el camino de startActionCmd (running es pull_ai, no un verbo git).
func TestPullAINoEsPullKind(t *testing.T) {
	if IsPullKind("pull_ai") {
		t.Error("pull_ai no debería ser un PullKind")
	}
	if len(PullKinds) != 4 {
		t.Errorf("PullKinds = %d, want 4 (p/r/f/m)", len(PullKinds))
	}
	if _, err := exec.LookPath("/bin/echo"); err != nil {
		t.Skip("/bin/echo no disponible")
	}
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla", "/bin/echo {prompt}")
	m, _ = press(m, "p")
	m, _ = press(m, "a")
	if got := m.running[dir]; got != "pull_ai" {
		t.Errorf("running = %q, want pull_ai", got)
	}
}

// Una plantilla cuyo primer campo queda vacío ({branch} sin rama) no puede
// producir el toast engañoso "<vacío> not installed": se avisa del ejecutable
// vacío y no hay handoff.
func TestPullAINoLanzaConEjecutableVacio(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla el rebase", "{branch} {prompt}")
	m.states[dir] = gitstatus.Snapshot{} // sin rama: {branch} → ""

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a lanzó el handoff con ejecutable vacío: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("debería notificar")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "empty executable") {
		t.Errorf("notificación = %v, want aviso de ejecutable vacío", cmd())
	}
}

// Un worktree sin snapshot propio no reporta estado falso: la rama sale del
// inventario del padre (la misma fuente que la sub-fila) y el resto no se
// inventa. Antes daba branch vacío y un state "no upstream" inexistente.
func TestPullAIAiVarsWorktreeSinSnapshot(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi",
		wt("/tmp/wt-a", "feat/x"), wt("/tmp/wt-d", ""))
	m := newTestModel(t, []discovery.Project{p}, st)

	vars := m.aiVars("/tmp/wt-a")
	if vars["branch"] != "feat/x" {
		t.Errorf("branch = %q, want feat/x (el de `git worktree list`)", vars["branch"])
	}
	if vars["state"] != "" {
		t.Errorf("state = %q, want vacío (no se inventa estado)", vars["state"])
	}
	if vars["upstream"] != "" || vars["ahead"] != "" || vars["behind"] != "" {
		t.Errorf("placeholders inventados: %+v", vars)
	}
	// En detached el head corto es el único dato, igual que en la sub-fila.
	if got := m.aiVars("/tmp/wt-d")["branch"]; got != "(detached) abc1234" {
		t.Errorf("branch detached = %q, want \"(detached) abc1234\"", got)
	}
}

// El camino real marcador → argv: un prompt adversarial escrito en un
// `.gitdash.toml` de verdad entra como UN solo elemento de argv, sin que el TOML
// ni el shell lo partan.
func TestPullAIArgvDesdeElMarcadorReal(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	// En TOML básico \n es un salto real y \" una comilla: el prompt lleva de
	// todo (salto, comillas, `$`, `;`).
	content := "[ai.pull]\nprompt = \"linea1\\nlinea2 \\\"con comillas\\\" y $VAR; rm -rf /\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p := proj("demo", dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapClean()})
	m.cfg.AICommands = map[string]string{"pull": "jcode -run {prompt}"}

	prompt, err := discovery.MarkerPrompt(dir, m.cfg.Marker, "pull")
	if err != nil {
		t.Fatalf("MarkerPrompt: %v", err)
	}
	want := "linea1\nlinea2 \"con comillas\" y $VAR; rm -rf /"
	if prompt != want {
		t.Fatalf("prompt del marcador = %q, want %q", prompt, want)
	}

	argv := m.pullAIArgv(dir, prompt)
	if len(argv) != 3 || argv[2] != want {
		t.Errorf("argv = %#v, want el prompt íntegro como último elemento", argv)
	}
}

// El prompt del selector anuncia también la variante AI.
func TestPullPromptIncluyeAI(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "p")
	got := m.pullPrompt()
	for _, want := range []string{"p default", "r rebase", "f ff-only", "m merge", "a AI", "esc cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("el prompt no menciona %q:\n%s", want, got)
		}
	}
}

// El prompt del marcador entra íntegro como un único elemento de argv: espacios,
// saltos de línea y comillas no lo parten ni lo interpretan.
func TestPullAIPromptUnSoloArgv(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "x", "jcode -run {prompt}")

	prompt := "primera línea\nsegunda \"con comillas\" y $VAR; rm -rf /"
	argv := m.pullAIArgv(dir, prompt)
	want := []string{"jcode", "-run", prompt}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
}

// Los placeholders de contexto salen del snapshot vivo del repo.
func TestPullAIAiVarsResuelveElContexto(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "arregla",
		"ai --branch {branch} --upstream {upstream} --state {state} --ahead {ahead} --behind {behind} --sync {sync} {prompt}")
	m.states[dir] = gitstatus.Snapshot{
		Status: gitstatus.Status{
			Branch: "feat/x", Upstream: "origin/feat/x", HasUpstream: true,
			Ahead: 2, Behind: 1,
		},
		SyncBranch: "main",
	}

	argv := m.pullAIArgv(dir, "arregla")
	want := []string{
		"ai", "--branch", "feat/x", "--upstream", "origin/feat/x",
		"--state", "diverged", "--ahead", "2", "--behind", "1",
		"--sync", "main", "arregla",
	}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v\nwant %#v", argv, want)
	}
}

// La vuelta del handoff registra el exec en el command log con el argv resuelto
// (incluido el prompt) y Dur=0, como los demás handoffs.
func TestExecDoneRegistraPullAI(t *testing.T) {
	m, rec := logModel(t)
	path := "/tmp/old-clean"
	argv := []string{"jcode", "-run", "arregla el rebase"}

	updated, _ := m.Update(execDoneMsg{path: path, action: "pull_ai", argv: argv, err: nil})
	_ = updated.(Model)

	// El recollect de fondo añade lecturas por detrás, así que no se asume que
	// la entrada de pull_ai sea la última: se busca por acción.
	var got *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "pull_ai" {
			continue
		}
		cp := e
		got = &cp
	}
	if got == nil {
		t.Fatal("no se registró el exec de pull_ai")
	}
	if !reflect.DeepEqual(got.Argv, argv) {
		t.Errorf("argv = %#v, want %#v", got.Argv, argv)
	}
	if got.Dur != 0 {
		t.Errorf("Dur = %v, want 0 (handoff)", got.Dur)
	}
	if got.Exit != 0 {
		t.Errorf("Exit = %d, want 0", got.Exit)
	}
	if got.Class != cmdlog.ClassAction || got.Dir != path {
		t.Errorf("clase/dir = %v/%q", got.Class, got.Dir)
	}
}
