// Tests del panel del command log: qué se ve, dónde se ve y que no rompa el
// layout. Siguen el patrón del repo (modelo directo, sin teatest).
package tui

import (
	"fmt"
	osexec "os/exec"
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

// sembrar mete ejecuciones controladas en el log del modelo.
func sembrar(rec *cmdlog.Recorder, entries ...cmdlog.Entry) {
	for _, e := range entries {
		cmdlog.RecordExec(e)
	}
}

// sembrarIntent mete una INTENCIÓN por su canal (RecordIntent), no como
// ejecución: sembrarla con RecordExec deja Intent=false y la línea se rotula
// "exec" con la Key dentro del argv, así que cualquier aserción sobre la tecla
// la satisfaría la entrada equivocada.
func sembrarIntent(e cmdlog.Entry) {
	e.Intent = true
	e.Exit = 0
	e.Dur = 0
	e.Argv = nil
	e.Outcome = ""
	cmdlog.RecordIntent(e)
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
// entryFor busca por acción en vez de por posición. El modelo que devuelve
// logModel tiene el scan de fondo corriendo, así que una entrada de `git status`
// puede anexarse DESPUÉS de la que el test acaba de synchronousar: "la última"
// depende de la carrera de goroutines y el test falla una de cada quince. La
// acción identifica la entrada sin depender de quién escribió después.
func entryFor(t *testing.T, rec *cmdlog.Recorder, action string) cmdlog.Entry {
	t.Helper()
	entries := rec.Entries()
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Action == action {
			return entries[i]
		}
	}
	t.Fatalf("el command log no tiene ninguna entrada con acción %q", action)
	return cmdlog.Entry{}
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

	sembrarIntent(cmdlog.Entry{Repo: "dirty-api", Key: "p", Action: "pull", Class: cmdlog.ClassAction})
	sembrar(rec, execEntry("dirty-api", "pull", "git pull", "rebase+autostash", 0))

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

	sembrarIntent(cmdlog.Entry{Repo: "dirty-api", Key: "r", Action: "pull_rebase", Class: cmdlog.ClassAction})
	sembrar(rec, execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase+autostash", 0))
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

// El mínimo de UNA línea visible es lo que hace que el panel siga mostrando algo
// con una terminal de 1 o 2 líneas, donde bodyLines-1 da 0 o negativo. Con el
// mínimo a 0 la caja se pintaría VACÍA (después de recortar, el bucle de entradas
// no it'd nothing que pintar) y el usuario se quedaría sin log en un panel
// estrecho, sin ningún error: por eso esta aserción mira el contenido y no solo
// el alto.
func TestPanelMuestraAlMenosUnaLinea(t *testing.T) {
	for _, alto := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("terminal de %d lineas", alto), func(t *testing.T) {
			m, rec := logModel(t)
			for i := range 5 {
				sembrar(rec, execEntry(fmt.Sprintf("repo-%d", i), "pull", "git pull", "up-to-date", 0))
			}
			m, _ = press(m, "l")
			m.height = alto
			m.width = 60

			// logSection gasta una línea en la cabecera, así que se le pasa el
			// presupuesto entero del cuerpo y el propio `visible := max(1, n-1)`
			// hace la resta: lo que se quiere decidir es que de 1 o 2 líneas
			// visibles quede alguna entrada pintada.
			out := stripANSI(m.logSection(alto))
			if !strings.Contains(out, "git pull") {
				t.Errorf("con %d lineas de cuerpo el panel no pintó ninguna entrada:\n%q", alto, out)
			}
		})
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

// Abrir el panel también suelta el selector: dentro de él `a` es "show all", no
// la variante AI, así que un selector armado secuestraría la tecla.
func TestAbrirElPanelSueltaElSelectorArmado(t *testing.T) {
	m, _ := logModel(t)
	m = cursorOn(t, m, "/tmp/dirty-api")
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p no armó el selector")
	}
	m, _ = press(m, "l") // abre el panel
	if m.pullArmed != nil {
		t.Error("abrir el panel dejó el selector de pull armado")
	}
	m, _ = press(m, "a")
	if !m.logShowAll {
		t.Error("a no alternó el filtro del panel")
	}
	if len(m.running) != 0 {
		t.Errorf("a lanzó una acción: %v", m.running)
	}
}

// Estando en el panel, `p` no arma el selector: el panel es un view mode y sus
// teclas mandan, así que `a` nunca queda shadowed.
func TestArmarPullDentroDelPanelNoArma(t *testing.T) {
	m, _ := logModel(t)
	m, _ = press(m, "l")
	m, _ = press(m, "p")
	if m.pullArmed != nil {
		t.Error("p armó el selector dentro del panel del log")
	}
	m, _ = press(m, "a")
	if !m.logShowAll {
		t.Error("a no alternó el filtro: el selector shadoweó la tecla")
	}
	if len(m.running) != 0 {
		t.Errorf("a lanzó una acción: %v", m.running)
	}
}

// Con un input activo la guarda del panel no puede correr: la `p` es del filtro
// y del input de `!`, no la variante del selector. Sin esto se la comía.
func TestPanelNoSeComeLaPDeLosInputs(t *testing.T) {
	t.Run("filtro", func(t *testing.T) {
		m, _ := logModel(t)
		m, _ = press(m, "l") // abre el panel
		m, _ = press(m, "/") // activa el filtro
		m, _ = press(m, "x")
		m, _ = press(m, "p")
		if got := m.searchInput.Value(); got != "xp" {
			t.Errorf("filtro = %q, want \"xp\" (la p se perdió)", got)
		}
	})
	t.Run("comando", func(t *testing.T) {
		m, _ := logModel(t)
		m, _ = press(m, "l")
		m, _ = press(m, "!") // abre el input de comando
		m, _ = press(m, "p")
		if got := m.cmdInput.Value(); got != "p" {
			t.Errorf("cmdInput = %q, want \"p\" (la p se perdió)", got)
		}
	})
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
		sembrar(rec, execEntry("dirty-api", "pull", "git pull --rebase --autostash", "rebase+autostash", 0))
		sembrarIntent(cmdlog.Entry{Repo: "dirty-api", Key: "r", Action: "pull_rebase", Class: cmdlog.ClassAction})
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

// computeLogColumns degrada por columnas y en un ORDEN fijo (veredicto →
// resultado → repo → kind). Los bordes importan: en el ancho donde las columnas
// fijas + el argv mínimo caben EXACTAMENTE, no se degrada nada, y un "> " mal
// puesto empezaría a tirar columnas sin necesidad.
func TestLogColumnsEnElEncajeExacto(t *testing.T) {
	// El ancho de holgura con las cinco columnas: 12+2+7+2+14+2+16+2+8 = 65.
	holgura := logColTime + 4*logColSep + logColKind + logColRepo + logColOutcome + logColVerdict
	if holgura != 65 {
		t.Fatalf("precondición: el ancho de holgura son %d, el test mide 65", holgura)
	}

	// Encaje justo: cabe el argv mínimo y no se toca ninguna columna.
	justo := holgura + logMinArgv
	t.Run("encaje exacto, nada se degrada", func(t *testing.T) {
		c := computeLogColumns(justo)
		if c.kind != logColKind || c.repo != logColRepo || c.outcome != logColOutcome || c.verdict != logColVerdict {
			t.Errorf("inner=%d: se degradó una columna que cabía: %+v", justo, c)
		}
		if c.argv != logMinArgv {
			t.Errorf("inner=%d: argv = %d, want %d", justo, c.argv, logMinArgv)
		}
	})

	// Una celda menos: la primera que cae es la de menos valor (el veredicto).
	t.Run("una celda menos, cae el veredicto", func(t *testing.T) {
		c := computeLogColumns(justo - 1)
		if c.verdict != 0 {
			t.Errorf("inner=%d: veredicto = %d, want 0 (es lo que menos vale)", justo-1, c.verdict)
		}
		for _, c2 := range []struct {
			nombre string
			got    int
			want   int
		}{{"kind", c.kind, logColKind}, {"repo", c.repo, logColRepo}, {"outcome", c.outcome, logColOutcome}} {
			if c2.got != c2.want {
				t.Errorf("inner=%d: %s = %d, want %d (no debía caer aún)", justo-1, c2.nombre, c2.got, c2.want)
			}
		}
	})

	// El argv nunca baja de logMinArgv mientras quepa el resto, y solo se queda
	// sin argv cuando ya no queda ni la hora.
	t.Run("argv al suelo, nunca menos", func(t *testing.T) {
		for inner := justo - 1; inner >= logColTime+logColSep+logMinArgv; inner-- {
			c := computeLogColumns(inner)
			if c.argv < logMinArgv {
				t.Errorf("inner=%d: argv = %d, want >= %d", inner, c.argv, logMinArgv)
			}
		}
		// Ya sin sitio para el argv mínimo, se queda con lo que hay: nunca
		// negativo y nunca inventando ancho (restando, no sumando).
		for _, inner := range []int{logColTime + logColSep + logMinArgv - 1, 10, 5, 1, 0, -5} {
			c := computeLogColumns(inner)
			if c.argv < 0 {
				t.Errorf("inner=%d: argv = %d, want >= 0", inner, c.argv)
			}
			if c.argv > max(0, inner-logColTime-logColSep) {
				t.Errorf("inner=%d: argv = %d quiere más ancho del que hay (%d)",
					inner, c.argv, max(0, inner-logColTime-logColSep))
			}
		}
	})
}

// La cabecera y las líneas respetan las columnas que el reparto ha dejado: una
// columna con ancho 0 no se rotula ni se pinta. Si la guarda fuera ">=" en vez
// de ">", un terminal estrecho enseñaría cabeceras de columnas que ya no
// existen (y el reparto se vería roto en la propia cabecera).
func TestPanelLasColumnasDegradadasNoSePintan(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec, execEntry("api", "pull", "git pull", "fast-forward", 0))
	m.logOpen = true

	t.Run("ancho amplio, todas las columnas", func(t *testing.T) {
		m.width = 200
		header := m.logHeader(computeLogColumns(max(0, m.width-2)))
		for _, quiere := range []string{"TIME", "KIND", "REPO", "COMMAND", "RESULT", "VERDICT"} {
			if !strings.Contains(header, quiere) {
				t.Errorf("con ancho amplio falta la columna %q: %q", quiere, header)
			}
		}
	})

	t.Run("ancho estrecho, solo las que quedan", func(t *testing.T) {
		// A 40 de ancho solo caben hora, kind (4) y el argv.
		m.width = 40
		c := computeLogColumns(max(0, m.width-2))
		if c.verdict != 0 || c.outcome != 0 || c.repo != 0 {
			t.Fatalf("precondición: a width=40 deberían caer repo/resultado/veredicto: %+v", c)
		}
		header := m.logHeader(c)
		for _, noDebe := range []string{"REPO", "RESULT", "VERDICT"} {
			if strings.Contains(header, noDebe) {
				t.Errorf("columna degradada %q todavía en la cabecera: %q", noDebe, header)
			}
		}
		for _, quiere := range []string{"TIME", "KIND", "COMMAND"} {
			if !strings.Contains(header, quiere) {
				t.Errorf("con ancho estrecho falta %q: %q", quiere, header)
			}
		}
		// La cabecera son exactamente las columnas que quedan, sin separadores
		// fantasma: una columna de ancho 0 no deja ni su rótulo ni su hueco.
		want := pad("TIME", logColTime) + pad("KIND", c.kind) + strings.Repeat(" ", logColSep) +
			pad("COMMAND", c.argv) + strings.Repeat(" ", logColSep)
		if header != want {
			t.Errorf("cabecera = %q, want %q", header, want)
		}
		// Y el cuerpo de la línea: el comando se ve, y el repo (que ya no tiene
		// columna) tampoco aparece.
		lay := m.layout()
		out := stripANSI(m.logSection(max(3, lay.bodyLines)))
		if !strings.Contains(out, "git pull") {
			t.Errorf("el comando no se ve con ancho estrecho:\n%s", out)
		}
		if strings.Contains(out, "api") {
			t.Errorf("un repo sin columna se pintó igualmente:\n%s", out)
		}
	})

	t.Run("sin sitio para el argv, no hay columna de comando", func(t *testing.T) {
		// Solo cabe la hora: el argv se queda a 0 y no se rotula.
		m.width = logColTime + logColSep // 14
		c := computeLogColumns(max(0, m.width-2))
		if c.argv != 0 {
			t.Fatalf("precondición: a width=%d el argv debería quedar a 0: %+v", m.width, c)
		}
		header := m.logHeader(c)
		if strings.Contains(header, "COMMAND") {
			t.Errorf("columna de comando sin ancho en la cabecera: %q", header)
		}
		if !strings.Contains(header, "TIME") {
			t.Errorf("la hora siempre está: %q", header)
		}
		lay := m.layout()
		out := stripANSI(m.logSection(max(3, lay.bodyLines)))
		if strings.Contains(out, "git pull") {
			t.Errorf("el comando se pintó sin columna:\n%s", out)
		}
	})
}

// El offset del panel se recorta contra las líneas visibles: ni se sale del
// rango (líneas en blanco al final) ni deja huecos. Los dos extremos son los
// bordes de esa aritmética.
func TestPanelOffsetEnLosExtremos(t *testing.T) {
	m, rec := logModel(t)
	for i := 0; i < 6; i++ {
		sembrar(rec, execEntry("repo"+string(rune('a'+i)), "pull", "git pull", "fast-forward", 0))
	}
	m.logOpen = true

	t.Run("offset 0 ve la cola", func(t *testing.T) {
		m.logOffset = 0
		out := stripANSI(m.logSection(4))
		if !strings.Contains(out, "repoe") {
			t.Errorf("con offset 0 no se ve la entrada más reciente:\n%s", out)
		}
	})
	t.Run("scrollear arriba y volver abajo se recorta", func(t *testing.T) {
		m.logOffset = 0
		visible := 3
		for range 20 {
			m.logScroll(1, visible) // k: hacia atrás, hacia lo más antiguo
		}
		if m.logOffset == 0 {
			t.Error("20 scrolls arriba no movieron el offset")
		}
		maxOffset := max(0, len(m.logEntries())-visible)
		if m.logOffset > maxOffset {
			t.Errorf("offset = %d, want <= %d (dejaría líneas en blanco)", m.logOffset, maxOffset)
		}
		if m.logOffset != maxOffset {
			t.Errorf("offset = %d, want %d (el tope con 6 entradas y 3 visibles)", m.logOffset, maxOffset)
		}
		for range 40 {
			m.logScroll(-1, visible) // j: hacia la cola
		}
		if m.logOffset != 0 {
			t.Errorf("offset = %d al llegar abajo, want 0 (la cola siempre visible)", m.logOffset)
		}
		// Con el log entero en pantalla el offset es 0 aunque se inserte más.
		m.logScroll(1, 100)
		if m.logOffset != 0 {
			t.Errorf("offset = %d con todas las entradas visibles, want 0", m.logOffset)
		}
	})
	t.Run("la sección se rellena sin huecos ni panic con cualquier alto", func(t *testing.T) {
		// Con 1 línea de cuerpo la sección se queda en 1 fila visible (es el
		// suelo del log, para que la cabecera no se quede sola); el resto de
		// alturas se rellenan exacto.
		if got := strings.Count(stripANSI(m.logSection(1)), "\n") + 1; got < 1 {
			t.Errorf("bodyLines=1: caja de %d líneas", got)
		}
		for bodyLines := 2; bodyLines <= 12; bodyLines++ {
			for off := 0; off <= 8; off++ {
				m.logOffset = off
				out := m.logSection(bodyLines)
				plano := stripANSI(out)
				// Una caja de alto fijo: la cabecera de columnas + las filas
				// visibles + los dos bordes, sin huecos al final.
				want := bodyLines + 2 // 1 cabecera + (bodyLines-1) filas + 2 bordes
				if got := strings.Count(plano, "\n") + 1; got != want {
					t.Fatalf("bodyLines=%d offset=%d: caja de %d líneas, want %d", bodyLines, off, got, want)
				}
				for i, l := range strings.Split(plano, "\n") {
					if w := ansi.StringWidth(l); w > m.width {
						t.Errorf("bodyLines=%d offset=%d línea %d ancho = %d > %d", bodyLines, off, i, w, m.width)
					}
				}
			}
		}
	})
}

// El saneo también tiene que dejar pasar el texto UTF-8 válido, no solo atacar
// lo que parece una secuencia. Los tests anteriores usaban payload de un byte
// (RuneError), que es el caso fácil; un emoji o un acento fuera de BMP no puede
// perderse, porque el argv del prompt del marcador es texto que el usuario
// escribió y perderlo sería un bug de verdad.
//
// El `\u2028` de "linea\u2028separador" ya lo cubre otra suite; aquí lo que se
// ata es que un rune de 4 bytes sobrevive entero y que un RuneError DE TAMAÑO
// MAYOR que 1 (byte suelto dentro de una secuencia multibyte) no se descarta por
// error: la guarda es `size <= 1`, no "es RuneError".
func TestSanitizeLogTextConservaUTF8Valido(t *testing.T) {
	for _, tc := range []struct {
		nombre, in, want string
	}{
		{"emoji de 4 bytes", "a👍b", "a👍b"},
		{"acentos", "acción ñandú", "acción ñandú"},
		{"emoji al inicio", "🚀 go", "🚀 go"},
		{"emoji al final", "go 🚀", "go 🚀"},
		{"dos emojis", "🚀🎯 fin", "🚀🎯 fin"},
		{"emoji con control alrededor", "a\x1b[31m👍\x1b[0mb", "a👍b"},
		{"multibyte y RuneError juntos", "👍\xffn", "👍n"},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			if got := sanitizeLogText(tc.in); got != tc.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Una secuencia OSC sin cerrar que acaba en ESC no puede reventar: el salto de
// índices al mirar el terminador ST tiene que comprobar el límite antes de
// indexar, no después.
func TestSanitizeLogTextNoReventaConSecuenciasSinCerrar(t *testing.T) {
	casos := []struct {
		nombre, in string
		want       string
	}{
		{"OSC sin cerrar", "\x1b]0;title", ""},
		{"OSC que acaba en ESC", "\x1b]0;abc\x1b", ""},
		{"OSC con BEL", "\x1b]0;abc\x07cola", "cola"},
		// Payload de UN byte: el escaneo tiene que arrancar en él, no después.
		// Si se salta de más, "cola" se pierde y el argv del prompt del
		// marcador se traga texto legítimo.
		{"OSC de un byte con BEL", "x\x1b]\x07cola", "xcola"},
		{"OSC de un byte con ST", "x\x1b]\x1b\\cola", "xcola"},
		{"OSC de dos bytes", "x\x1b]a\x07cola", "xcola"},
		{"OSC con ST completo", "\x1b]0;abc\x1b\\cola", "cola"},
		{"CSI sin cerrar", "\x1b[38;5", ""},
		{"CSI cerrado en @", "\x1b[@cola", "cola"},
		{"CSI cerrado en ~", "\x1b[1~cola", "cola"},
		// Un ESC que no abre CSI ni OSC se come a sí mismo y al carácter
		// siguiente (un "ESC c" suelto no puede quedarse pegado al texto).
		{"ESC suelto", "\x1bcola", "ola"},
		{"ESC al final", "cola\x1b", "cola"},
		{"ESC vacío", "\x1b", ""},
		{"varios seguidos", "\x1b[\x1b]\x1b\\cola", "cola"},
	}
	for _, c := range casos {
		if got := sanitizeLogText(c.in); got != c.want {
			t.Errorf("%s: sanitizeLogText(%q) = %q, want %q", c.nombre, c.in, got, c.want)
		}
	}
}

// El veredicto se colorea por el resultado real del proceso: verde si salió 0,
// rojo si no. Un exec exitoso pintado de rojo (o al revés) haría dudar de un
// pull que sí funcionó.
func TestPanelVerdictColoreadoPorElExit(t *testing.T) {
	m, rec := logModel(t)
	sembrar(rec,
		execEntry("ok", "pull", "git pull", "fast-forward", 0),
		execEntry("ko", "pull", "git pull", "diverged", 1),
	)
	for _, c := range []struct {
		nombre string
		e      cmdlog.Entry
		want   lipglossStyle
	}{
		{"exit 0 es el estilo tranquilo", cmdlog.Entry{Exit: 0}, styleHint},
		{"exit distinto de 0 es error", cmdlog.Entry{Exit: 1}, styleError},
		{"exit 128 también", cmdlog.Entry{Exit: 128}, styleError},
		{"sin exit (-1) es error", cmdlog.Entry{Exit: -1}, styleError},
	} {
		if got, want := m.logVerdictStyle(c.e).Render("x"), c.want.Render("x"); got != want {
			t.Errorf("%s: estilo = %q, want %q", c.nombre, got, want)
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
		// El segundo exacto: a partir de un segundo se pintan segundos, y no
		// milisegundos. Con un ">" en vez de un ">=", un comando de 1.000 s
		// caería en el final y saldría "1000ms" al lado de un "2.5s" de al lado.
		{"un segundo justo", cmdlog.Entry{Exit: 0, Dur: time.Second}, "1.0s"},
		{"milésimas por debajo del segundo, en ms", cmdlog.Entry{Exit: 0, Dur: 999 * time.Millisecond}, "999ms"},
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

	e := entryFor(t, rec, "lazygit")
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
	snap := gitstatus.Collect(t.Context(), dir, "main", false)

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

// El argv puede llevar texto no confiable (el prompt del marcador). El panel
// sanea antes de pintar: fuera caracteres de control y secuencias de escape,
// saltos de línea colapsados a un espacio.
func TestSanitizeLogTextEliminaControlYColapsaLineas(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"osc52", "x\x1b]52;c;cGF3bmVk\x07y", "xy"},
		{"csi", "a\x1b[31mrojo\x1b[0mb", "arojob"},
		{"multilinea", "linea1\nlinea2\r\nlinea3", "linea1 linea2 linea3"},
		{"tabulador", "a\tb", "a b"},
		{"c0", "a\x01b\x7fc", "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogText(tc.in); got != tc.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// El formato Unicode (bidi, zero-width) y los separadores de línea no pueden
// sobrevivir: reordenarían o partirían visualmente una línea del log.
func TestSanitizeLogTextQuitaFormatoYZeroWidth(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"rtl-override", "a\u202eb", "ab"},
		{"zwsp", "a\u200bb", "ab"},
		{"line-separator", "a\u2028b", "ab"},
		{"para-separator", "a\u2029b", "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogText(tc.in); got != tc.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// skipEscape devuelve el índice TRAS la secuencia de escape. Esa palabra es el
// contrato entero de la función, y cada +1/+2 de sus returns es un mutante de
// ARITHMETIC_BASE que se lleva un byte del texto legítimo.
//
// Los casos eligen entradas donde el byte de más o de menos NO es un control que
// el saneo iba a descartar igual, porque si lo fuera el mutante sería equivalente
// y ningún test podría distinguirlo. Con un carácter IMPRIMIBLE justo detrás del
// terminador, saltarse un byte de más deja texto de más y saltarse uno de menos
// deja el terminador pintado.
func TestSkipEscapeConsumeLaSecuenciaYNadaMas(t *testing.T) {
	casos := []struct {
		nombre  string
		in      string
		want    string
		wantIdx int
	}{
		// wantIdx es el índice del PRIMER carácter que ya no es de la secuencia,
		// con BEL en el byte 5 (ESC=0, ]=1, 0=2, ;=3, t=4): "cola" empieza en el
		// 6. Con `return j-1` en vez de `j+1` el índice sería el 5 —el BEL— y el
		// texto saldría cortado. Los casos que ya existían usaban un BEL seguido
		// de texto donde el byte de más se perdía igual, así que no lo veían.
		{"BEL seguido de letra", "\x1b]0;t\x07cola", "cola", 6},
		{"ST seguido de letra", "\x1b]0;t\x1b\\cola", "cola", 7},
		{"CSI cerrado en letra", "\x1b[31mX", "X", 5},
		// El ESC suelto se come a sí mismo y al SIGUIENTE ("ESC c" deja "ola",
		// ver TestSanitizeLogTextNoReventaConSecuenciasSinCerrar), así que aquí el
		// índice es lo único que se afirma: el texto de este caso no es el
		// contrato de skipEscape.
		{"ESC suelto", "\x1bcola", "ola", 2},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			// El índice devuelto es lo que se compara: si se salta un byte de más,
			// el texto siguiente sale truncado; si se salta uno de menos, el
			// terminador se cuela en la salida.
			if got := skipEscape(c.in, 0); got != c.wantIdx {
				t.Errorf("skipEscape(%q, 0) = %d, want %d", c.in, got, c.wantIdx)
			}
			if got := sanitizeLogText(c.in); got != c.want {
				t.Errorf("sanitizeLogText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Una entrada siempre ocupa UNA línea y no deja pasar el payload inyectado: el
// prompt no confiable no puede añadir líneas al panel ni emitir escapes.
func TestPanelSaneaElArgvDeUnaEntrada(t *testing.T) {
	for _, prompt := range []string{
		"x\x1b]52;c;cGF3bmVk\x07y",
		"linea1\nlinea2\nlinea3",
		"x\u202ey\u200bz",
	} {
		m, rec := logModel(t)
		sembrar(rec, cmdlog.Entry{
			Repo: "dirty-api", Class: cmdlog.ClassAction, Action: "pull_ai",
			Argv: []string{"jcode", "-run", prompt}, Exit: 0,
		})
		m.width, m.height = 200, 30
		m, _ = press(m, "l")

		raw := m.View().Content
		if strings.Contains(raw, "]52;c;") || strings.Contains(raw, "cGF3bmVk") {
			t.Errorf("prompt %q: el payload de escape llegó al panel", prompt)
		}
		if strings.ContainsRune(raw, '\u202e') || strings.ContainsRune(raw, '\u200b') {
			t.Errorf("prompt %q: formato/zero-width llegó al panel", prompt)
		}
		plano := stripANSI(raw)
		if lines := strings.Split(plano, "\n"); len(lines) != 30 {
			t.Errorf("prompt %q: líneas = %d, want 30 (un prompt multilínea no puede romper el alto)", prompt, len(lines))
		}
		if log := sectionContent(t, plano, "log"); !strings.Contains(log, "jcode") {
			t.Errorf("prompt %q: el argv saneado no se pintó:\n%s", prompt, log)
		}
	}
}

// El argv del panel es texto no confiable (lleva el prompt del marcador), así
// que puede traer bytes que no son UTF-8 válido: al pintarlos, el terminal
// decide qué hacer con ellos. El saneador los descarta, y esa rama no estaba
// ejercitada por ningún test: `size <= 1` es exactamente la condición que
// distingue un rune mal decodificado de un RuneError legítimo (un U+FFFD
// escrito de verdad viene con size == 3 y TIENE que sobrevivir).
func TestSanitizeLogTextDescartaBytesQueNoSonUTF8(t *testing.T) {
	casos := []struct {
		nombre, in, want string
	}{
		{"byte suelto", "go\xfftest", "gotest"},
		{"byte suelto entre letras", "a\xffb", "ab"},
		{"secuencia truncada", "\xe4\xb8", ""},
		{"byte valido conako", "ca\xfe", "ca"},
		// U+FFFD de verdad (3 bytes) no es un error de decodificacion: es
		// texto, y elTerminal lo pinta. Descartarlo seria perder informacion.
		{"U+FFFD legitimo", "a\uFFFDb", "a\uFFFDb"},
		// El invalido se va y el U+FFFD de verdad se queda: son cosas distintas
		// aunque los dos se pinten como el mismo signo.
		{"mixto", "x\xffy\uFFFDz", "xy\uFFFDz"},
	}
	for _, c := range casos {
		if got := sanitizeLogText(c.in); got != c.want {
			t.Errorf("%s: sanitizeLogText(%q) = %q, want %q", c.nombre, c.in, got, c.want)
		}
	}
}

// execExit con un error REAL de proceso es el caso que se ve en el panel: un
// git que sale con codigo 1 tiene que aparecer como "exit 1", no como el -1 que
// significa "no se ni siemple arranco". La diferencia es la que permite
// distinguir un conflicto de un repo de unarotta.
//
// El error sale de ejecutar un comando de verdad, porque un *exec.ExitError no
// se puede fabricar a mano (su ExitCode lee del proceso).
func TestExecExitConErrorReal(t *testing.T) {
	cmd := osexec.Command("sh", "-c", "exit 3")
	err := cmd.Run()
	if err == nil {
		t.Fatal("el comando de prueba no fallo")
	}
	if got := execExit(err); got != 3 {
		t.Errorf("execExit(exit 3) = %d, want 3", got)
	}
}

// logIntent sin recorder global no hace nada, y es lo correcto: el command log
// es opt-in (--print no lo instala, y los tests pueden no hacerlo). Sin ese
// return, una TUI sin log reventaria al pulsar cualquier tecla, que es
// exactamente lo que pasaria en --print si compartiera modelo.
func TestLogIntentSinRecorderNoRevienta(t *testing.T) {
	// El recorder se desactiva DESPUES de construir el modelo: New lo instala
	// siempre, y lo que se prueba es que a partir de ahi no registrar nada no
	// revienta. Si se apagara antes, newTestModel volvería a instalarlo.
	m := newTestModel(t, nil, nil)
	prev := cmdlog.Active()
	cmdlog.SetRecorder(nil)
	t.Cleanup(func() { cmdlog.SetRecorder(prev) })

	if cmdlog.Active() != nil {
		t.Fatal("no se pudo desactivar el recorder")
	}
	// Con el recorder apagado, ni una intencion ni un exec deben romper nada.
	m.logIntent("p", "pull")
	// Y la via global de exec, que es la que usan los handoffs y el `!`.
	cmdlog.RecordExec(cmdlog.Entry{Class: cmdlog.ClassAction, Key: "p", Action: "pull"})
	// Con el log apagado no hay ni una entrada que consultar.
	if got := cmdlog.Entries(); len(got) != 0 {
		t.Errorf("cmdlog.Entries() = %d con el log apagado, want 0", len(got))
	}
}

// Una ejecución sin argv (y sin acción) se pinta como un guion, no como una
// columna en blanco: una columna vacía se lee como "el comando se perdió" y el
// guion como "no hay comando que enseñar". Las intenciones nunca pasan por
// aquí (tienen siempre Key y Action), así que el caso es del exec.
func TestLogLineSinArgvPoneGuion(t *testing.T) {
	m, _ := logModel(t)
	cols := logColumns{kind: logColKind, repo: logColRepo, outcome: logColOutcome, verdict: logColVerdict, argv: 24}
	line := stripANSI(m.logLine(cmdlog.Entry{Class: cmdlog.ClassAction, Repo: "api"}, cols))
	if !strings.Contains(line, "-") {
		t.Errorf("linea sin argv = %q, want el guion en la columna del comando", line)
	}
}
