// Tests del panel del command log: qué se ve, dónde se ve y que no rompa el
// layout. Siguen el patrón del repo (modelo directo, sin teatest).
package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// logModel construye un modelo sobre el fixture estándar y devuelve el recorder
// que el propio New instaló. New() crea uno limpio en cada llamada, así que
// este es el log de este test y no el de otro.
func logModel(t *testing.T) (Model, *cmdlog.Recorder) {
	t.Helper()
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	return m, cmdlog.Active()
}

// sembrar mete entradas controladas en el log del modelo.
func sembrar(rec *cmdlog.Recorder, entries ...cmdlog.Entry) {
	for _, e := range entries {
		cmdlog.RecordExec(e)
	}
}

// execEntry describe una ejecución de ejemplo para sembrar el log.
func execEntry(repo, action, cmdLine, outcome string, exit int) cmdlog.Entry {
	return cmdlog.Entry{
		Repo: repo, Action: action, Class: cmdlog.ClassAction,
		Argv: strings.Fields(cmdLine), Outcome: outcome, Exit: exit,
		Dur: 146 * time.Millisecond,
	}
}

// lastEntry devuelve la última entrada del log.
func lastEntry(t *testing.T, rec *cmdlog.Recorder) cmdlog.Entry {
	t.Helper()
	entries := rec.Entries()
	if len(entries) == 0 {
		t.Fatal("el command log está vacío: el exec no se registró")
	}
	return entries[len(entries)-1]
}

// El caso que motivó la feature: `pp` sobre un repo con pull.rebase=true en el
// gitconfig. El argv es `git pull` a secas (los flags los pondría gitdash y
// pisarían la política del usuario) y el resultado real —rebase con autostash—
// solo aparece en la salida de git. El panel tiene que enseñar las dos cosas.
func TestPanelEnsenaElComandoRealYSuResultado(t *testing.T) {
	m, rec := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")

	// p arma el selector; la segunda p elige la variante default → git pull.
	m, _ = press(m, "p")
	m, _ = press(m, "p")

	sembrar(rec,
		cmdlog.Entry{Repo: "dirty-api", Key: "p", Action: "pull", Class: cmdlog.ClassAction},
		execEntry("dirty-api", "pull", "git pull", "rebase+autostash", 0),
	)

	m, _ = press(m, "l")
	if !m.logOpen {
		t.Fatal("l no abrió el panel")
	}
	plano := stripANSI(m.View().Content)
	log := sectionContent(t, plano, "log")
	if !strings.Contains(log, "git pull") {
		t.Errorf("el panel no enseña el argv:\n%s", log)
	}
	if !strings.Contains(log, "rebase+autostash") {
		t.Errorf("el panel no enseña el resultado real del pull:\n%s", log)
	}
	if !strings.Contains(log, "key p") {
		t.Errorf("el panel no enseña la tecla que lo disparó:\n%s", log)
	}
}

// El selector de pull deja dos intenciones: la que arma y la que elige variante.
// La segunda es la que explica el argv, porque `git pull` y
// `git pull --rebase` comparten la política que decide el gitconfig.
func TestPanelDistingueLaVarianteDePull(t *testing.T) {
	m, rec := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")
	m, _ = press(m, "p")
	m, _ = press(m, "r") // variante rebase explícita

	sembrar(rec,
		cmdlog.Entry{Repo: "dirty-api", Key: "r", Action: "pull_rebase", Class: cmdlog.ClassAction},
		execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase+autostash", 0),
	)
	m, _ = press(m, "l")

	log := sectionContent(t, stripANSI(m.View().Content), "log")
	if !strings.Contains(log, "pull_rebase") {
		t.Errorf("no se ve la variante elegida:\n%s", log)
	}
	if !strings.Contains(log, "--rebase") {
		t.Errorf("no se ve el argv con los flags:\n%s", log)
	}
}

// Por defecto solo se ven las acciones: las lecturas del scan son ~4 por repo
// (con 60 repos, 240 líneas) y no son lo que se viene a mirar.
func TestPanelOcultaLasLecturasPorDefecto(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec,
		cmdlog.Entry{Repo: "dirty-api", Action: "status", Class: cmdlog.ClassRead,
			Argv: []string{"git", "status", "--porcelain=v2", "--branch"}, Exit: 0},
		execEntry("dirty-api", "pull", "git pull", "up-to-date", 0),
	)
	m, _ = press(m, "l")

	log := sectionContent(t, stripANSI(m.View().Content), "log")
	if strings.Contains(log, "porcelain") {
		t.Errorf("la lectura del scan se ve sin pedirlo:\n%s", log)
	}
	if !strings.Contains(log, "git pull") {
		t.Errorf("la acción no se ve:\n%s", log)
	}
}

// `a` amplía el filtro a lecturas y fetch automático, y lo dice en el título.
func TestPanelAlternaElFiltroConA(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec,
		cmdlog.Entry{Repo: "dirty-api", Action: "status", Class: cmdlog.ClassRead,
			Argv: []string{"git", "status", "--porcelain=v2", "--branch"}, Exit: 0},
		cmdlog.Entry{Repo: "dirty-api", Action: "fetch", Class: cmdlog.ClassAuto,
			Argv: []string{"git", "fetch", "--prune"}, Exit: 0},
	)
	m, _ = press(m, "l")
	if m.logShowAll {
		t.Fatal("logShowAll = true de salida")
	}
	m, _ = press(m, "a")
	if !m.logShowAll {
		t.Fatal("a no activó logShowAll")
	}
	plano := stripANSI(m.View().Content)
	if !strings.Contains(plano, "log · all") {
		t.Errorf("el título no refleja el filtro:\n%s", plano)
	}
	log := sectionContent(t, plano, "log")
	if !strings.Contains(log, "porcelain") || !strings.Contains(log, "fetch --prune") {
		t.Errorf("con el filtro amplio deben verse lecturas y fetch automático:\n%s", log)
	}
	m, _ = press(m, "a")
	if strings.Contains(stripANSI(m.View().Content), "porcelain") {
		t.Error("al volver a filtrar por acciones, la lectura sigue ahí")
	}
}

// j/k desplazan el panel en vez de mover el cursor de la tabla: si no, el panel
// se movería mientras el usuario cree que navega por repos.
func TestPanelScrollNoMueveElCursor(t *testing.T) {
	m, rec := logModel(t)
	// Más entradas que líneas visibles: si no, no habría nada que desplazar.
	for range 40 {
		sembrar(rec, execEntry("dirty-api", "pull", "git pull", "up-to-date", 0))
	}
	m, _ = press(m, "l")
	if m.logOffset != 0 {
		t.Fatalf("logOffset = %d al abrir, want 0 (anclado en la cola)", m.logOffset)
	}
	cursor := m.cursor
	m, _ = press(m, "k")
	if m.cursor != cursor {
		t.Errorf("k movió el cursor de la tabla: %d → %d", cursor, m.cursor)
	}
	if m.logOffset == 0 {
		t.Error("k no desplazó el panel")
	}
	m, _ = press(m, "j")
	if m.logOffset != 0 {
		t.Errorf("logOffset = %d tras volver abajo, want 0", m.logOffset)
	}
}

// El offset nunca deja líneas en blanco al final: se recorta contra las líneas
// que caben de verdad. Con 40 entradas y 21 visibles, el tope son 19 líneas de
// scroll; pulsar k 50 veces no puede dejar el panel medio vacío.
func TestPanelOffsetSeRecorta(t *testing.T) {
	m, rec := logModel(t)
	for range 40 {
		sembrar(rec, execEntry("dirty-api", "pull", "git pull", "up-to-date", 0))
	}
	m, _ = press(m, "l")
	visible := max(1, m.layout().bodyLines-1)
	want := max(0, len(rec.Entries())-visible)
	for range 50 {
		m, _ = press(m, "k")
	}
	if m.logOffset != want {
		t.Errorf("logOffset = %d tras 50 scrolls, want %d (entradas %d - visibles %d)",
			m.logOffset, want, len(rec.Entries()), visible)
	}
	// Con el tope alcanzado, la primera línea pintada es la más antigua viva.
	if first := m.logSection(visible + 1); !strings.Contains(first, "git pull") {
		t.Errorf("con el tope del scroll no se ve ninguna entrada:\n%s", first)
	}
}

// La leyenda del panel vive en keybinds (el mismo mecanismo que los avisos
// armados): es lo único que dice qué teclas hacen qué, así que no puede
// degradarse mientras el panel esté abierto.
func TestPanelLeyendaEnKeybinds(t *testing.T) {
	m, _ := logModel(t)
	m, _ = press(m, "l")

	if m.keybindsLines() != 1 {
		t.Errorf("keybindsLines = %d con el panel abierto, want 1", m.keybindsLines())
	}
	kb := sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if !strings.Contains(kb, "command log") || !strings.Contains(kb, "scroll") {
		t.Errorf("la leyenda del panel no está en keybinds:\n%s", kb)
	}
	// Y el alto total sigue siendo el de la terminal: la leyenda sustituye a
	// las hints, no se apila con ellas.
	if lines := strings.Split(stripANSI(m.View().Content), "\n"); len(lines) != m.height {
		t.Errorf("líneas = %d, want %d", len(lines), m.height)
	}
	for _, hint := range []string{"j/k move", "f fetch"} {
		if strings.Contains(kb, hint) {
			t.Errorf("la hint %q sigue junto a la leyenda:\n%s", hint, kb)
		}
	}
}

// El panel es un view mode, no una modal: esc y su propia tecla lo cierran.
func TestPanelSeCierra(t *testing.T) {
	m, _ := logModel(t)
	m, _ = press(m, "l")
	m, _ = press(m, "esc")
	if m.logOpen {
		t.Error("esc no cerró el panel")
	}
	m, _ = press(m, "l")
	m, _ = press(m, "l")
	if m.logOpen {
		t.Error("la segunda l no cerró el panel")
	}
}

// Cerrar el panel suelta los estados armados: el aviso queda sin sentido fuera
// de su vista, y dejarlo armado obligaría a acertar la tecla siguiente desde un
// panel que ya no está.
func TestCerrarElPanelSueltaElSelectorArmado(t *testing.T) {
	m, _ := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p no armó el selector")
	}
	m, _ = press(m, "l")
	m, _ = press(m, "esc")
	if m.pullArmed != nil {
		t.Error("cerrar el panel dejó el selector de pull armado")
	}
}

// Abrir el panel es una acción de vista: no debe dejar intención en el log (el
// log es de comandos, no de teclas).
func TestAbrirElPanelNoRegistraIntencion(t *testing.T) {
	m, rec := logModel(t)
	before := len(rec.Entries())
	_, _ = press(m, "l")
	if got := len(rec.Entries()); got != before {
		t.Errorf("abrir el panel registró %d entradas nuevas, want 0", got-before)
	}
}

// Las teclas de navegación pura tampoco se registran: `d` cambia un filtro y no
// lanza ningún proceso.
func TestNavegacionNoLlegaAlLog(t *testing.T) {
	m, rec := logModel(t)
	before := len(rec.Entries())
	for _, k := range []string{"d", "/", "tab", "space", "enter"} {
		_, _ = press(m, k)
	}
	if got := len(rec.Entries()); got != before {
		t.Errorf("las teclas de vista registraron %d entradas, want 0", got-before)
	}
}

// Sin fila bajo el cursor (un header de grupo) la acción no se despacha, así que
// tampoco debe dejar intención: una línea "key p" sin repo ni exec confunde.
func TestIntencionSinFilaNoSeRegistra(t *testing.T) {
	m, rec := logModel(t)
	// El cursor se pone sobre un header de grupo a propósito: selected() es
	// false ahí (los headers no son repos).
	header := -1
	for i, e := range m.entries() {
		if e.kind != kindRepo {
			header = i
			break
		}
	}
	if header < 0 {
		t.Skip("el fixture no tiene headers de grupo")
	}
	m.cursor = header
	if _, ok := m.selected(); ok {
		t.Fatalf("la fila %d debería ser un header, no un repo", header)
	}
	before := len(rec.Entries())
	m, _ = press(m, "p")
	if got := len(rec.Entries()); got != before {
		t.Errorf("una acción sin fila registró %d entradas, want 0", got-before)
	}
}

// El panel no puede desbordar la terminal en ningún ancho.
func TestPanelRespetaElAncho(t *testing.T) {
	for _, width := range []int{200, 120, 80, 60, 40} {
		m, rec := logModel(t)
		sembrar(rec,
			execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase+autostash", 0),
			cmdlog.Entry{Repo: "dirty-api", Key: "r", Action: "pull_rebase", Class: cmdlog.ClassAction},
		)
		m.width, m.height = width, 24
		m, _ = press(m, "l")
		for i, l := range strings.Split(stripANSI(m.View().Content), "\n") {
			if got := ansi.StringWidth(l); got != width {
				t.Errorf("width=%d línea %d: ancho = %d, want %d: %q", width, i, got, width, ansi.Strip(l))
			}
		}
	}
}

// Con el terminal estrecho el comando se conserva y se caen las columnas menos
// útiles: a 60 el argv entero sigue leyéndose.
func TestPanelPriorizaElComandoEnAnchoEstrecho(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec, execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase", 0))
	m.width, m.height = 60, 24
	m, _ = press(m, "l")

	log := sectionContent(t, stripANSI(m.View().Content), "log")
	if !strings.Contains(log, "git pull --rebase --autostash") {
		t.Errorf("a 60 columnas el comando completo debería caber:\n%s", log)
	}
}

// El panel sustituye a la tabla y a la ficha, así que el alto total tiene que
// seguir cuadrando con el de la terminal en cualquier alto: es el invariante que
// más fácil se rompe al cambiar el reparto entre secciones.
func TestLogOcupaLaTerminalEnTodosLosAltos(t *testing.T) {
	for _, height := range []int{12, 16, 20, 24, 30, 45, 60} {
		for _, withEntries := range []bool{false, true} {
			m, rec := logModel(t)
			if withEntries {
				for range 50 {
					sembrar(rec, execEntry("dirty-api", "pull", "git pull", "rebase", 0))
				}
			}
			m.width, m.height = 100, height
			m, _ = press(m, "l")
			lines := strings.Split(stripANSI(m.View().Content), "\n")
			if len(lines) != height {
				t.Errorf("height=%d entradas=%v: líneas = %d, want %d", height, withEntries, len(lines), height)
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != 100 {
					t.Errorf("height=%d línea %d: ancho = %d, want 100: %q", height, i, got, ansi.Strip(l))
				}
			}
		}
	}
}

// computeLogColumns nunca reparte un ancho negativo ni deja el argv sin sitio.
func TestLogColumnsNoRompenConAnchosImposibles(t *testing.T) {
	for _, inner := range []int{-5, 0, 1, 10, 40, 78, 200} {
		c := computeLogColumns(inner)
		if c.argv < 0 || c.kind < 0 || c.repo < 0 || c.outcome < 0 || c.verdict < 0 {
			t.Errorf("inner=%d: columna negativa: %+v", inner, c)
		}
		if c.argv == 0 && inner > logColTime {
			t.Errorf("inner=%d: sin sitio para el argv: %+v", inner, c)
		}
	}
}

// Verdict: el código de salida solo cuando no fue 0, la duración cuando la hubo,
// y nada en una intención.
func TestLogVerdict(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    cmdlog.Entry
		want string
	}{
		{"ok con duración", cmdlog.Entry{Exit: 0, Dur: 146 * time.Millisecond}, "146ms"},
		{"fallido", cmdlog.Entry{Exit: 128, Dur: 20 * time.Millisecond}, "exit 128"},
		{"lento", cmdlog.Entry{Exit: 0, Dur: 2500 * time.Millisecond}, "2.5s"},
		{"handoff sin código ni duración", cmdlog.Entry{Exit: -1}, "no exit"},
		{"nada que medir", cmdlog.Entry{Exit: 0}, ""},
		{"intención", cmdlog.Entry{Intent: true, Exit: -1}, ""},
	} {
		if got := logVerdict(tc.e); got != tc.want {
			t.Errorf("%s: logVerdict = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// El color del resultado: lo que integró commits con éxito en verde, un
// conflicto en rojo, y "no había nada que hacer" sin grito de alarma. Los
// estilos de lipgloss no se pueden comparar con == (contienen slices), así que
// se mira el color ANSI que emiten.
func TestLogOutcomeStyle(t *testing.T) {
	m, _ := logModel(t)
	for _, tc := range []struct {
		name    string
		outcome string
		want    string
	}{
		{"rebase con autostash", "rebase+autostash", "38;5;46"}, // verde
		{"conflicto", "conflict", "38;5;196"},                   // rojo
		{"nada que integrar", "up-to-date", "38;5;241"},         // tenue
		{"sin clasificar", "", "38;5;240"},                      // tenue
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := m.logOutcomeStyle(cmdlog.Entry{Outcome: tc.outcome}).Render("X")
			if !strings.Contains(got, tc.want) {
				t.Errorf("outcome %q pintado como %q, want color %s", tc.outcome, got, tc.want)
			}
		})
	}
}

// El log se instala al construir la TUI (es donde hay teclas que auditar).
func TestNewInstalaElRecorder(t *testing.T) {
	cmdlog.SetRecorder(nil)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	_ = New(config.Defaults())
	if cmdlog.Active() == nil {
		t.Error("New no instaló el recorder del command log")
	}
}

// execDoneMsg registra el handoff: sin salida que registrar, pero el argv y cómo
// terminó sí son datos del log.
func TestHandoffRegistraArgvYSalida(t *testing.T) {
	m, rec := logModel(t)

	out, _ := m.Update(execDoneMsg{
		path: "/tmp/dirty-api", action: "lazygit",
		argv: []string{"lazygit"}, err: nil,
	})
	if _, ok := out.(Model); !ok {
		t.Fatal("Update devolvió un modelo de otro tipo")
	}

	e := lastEntry(t, rec)
	if e.Action != "lazygit" {
		t.Errorf("Action = %q, want %q", e.Action, "lazygit")
	}
	if e.Command() != "lazygit" {
		t.Errorf("Command() = %q, want %q", e.Command(), "lazygit")
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Dur != 0 {
		t.Errorf("Dur = %v en un handoff, want 0 (no se mide)", e.Dur)
	}
}

// execExit traduce el error del proceso al código que merece el log.
func TestExecExit(t *testing.T) {
	if got := execExit(nil); got != 0 {
		t.Errorf("execExit(nil) = %d, want 0", got)
	}
	if got := execExit(errNotExec{}); got != -1 {
		t.Errorf("execExit(no-exec-error) = %d, want -1", got)
	}
}

type errNotExec struct{}

func (errNotExec) Error() string { return "no arrancó" }

// El argv del borrado de worktree se perdía: worktreeRemovedMsg no lo llevaba y
// el detail construía actionResult sin cmd, así que la UI no enseñaba nunca qué
// se ejecutó (y el flag --force tampoco). Con la política de pull delegada en
// el gitconfig, un argv invisible es una UI que miente.
func TestBorradoWorktreeRegistraElArgvResuelto(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-real")
	testutil.MakeWorktree(t, dir, wtDir, "wt-real")
	snap := gitstatus.Collect(t.Context(), dir, "main")

	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap})
	rec := cmdlog.Active()
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })

	m, _ = press(m, "enter") // enter es la tecla de plegado/expansión
	m, _ = press(m, "down")
	m, _ = press(m, "D")
	m, _ = press(m, "D")

	want := "git " + strings.Join(gitstatus.RemoveWorktreeArgv(wtDir, false), " ")
	waitEvent(t, &m, func(ev event) bool {
		_, ok := ev.(worktreeRemovedMsg)
		return ok
	})

	act, ok := m.lastAction[dir]
	if !ok {
		t.Fatal("no quedó actionResult del borrado")
	}
	if act.cmd != want {
		t.Errorf("actionResult.cmd = %q, want %q", act.cmd, want)
	}
	// Y en el command log, como acción del usuario.
	var found bool
	for _, e := range rec.Entries() {
		if e.Action == "worktree" && strings.Contains(e.Command(), "worktree remove") {
			found = true
		}
	}
	if !found {
		t.Errorf("el command log no registró el borrado: %v", rec.Entries())
	}
}
