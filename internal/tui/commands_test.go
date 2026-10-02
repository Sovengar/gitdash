// Tests de las acciones con handoff de terminal: tecla g (lazygit) y
// modo comando `!` (input al final de la ficha del panel) via worktrunk.
package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
	"gitdash/internal/testutil"
)

// El plazo de un comando `!` son 5 MINUTOS, escritos como producto
// (`5 * time.Minute`). El mutante de ARITHMETIC_BASE lo convierte en
// `5 / time.Minute`, o sea 8.3ms: cualquier comando real —un build, un test, un
// `git fetch`— se cortaría antes de imprimir nada, y el usuario vería un fallo
// seco sin salida.
//
// Se comprueba con un `sleep` de medio segundo, que pasa con 5m y no con 8ms. No
// se baja la constante ni se espera a que expire de verdad: un test de 5 minutos
// para tapar un mutante no es un test, es un castigo. La aserción mira el código
// de salida, que es donde se ve el corte por contexto.
func TestComandoNoSeCortaEnMedioSegundo(t *testing.T) {
	out, code := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "sleep 0.5; echo tardado")
	if code != 0 {
		t.Fatalf("un comando de medio segundo debe salir con 0, dio %d (salida %q): "+
			"el plazo de %v lo cortó antes de terminar", code, out, commandTimeout())
	}
	if !strings.Contains(out, "tardado") {
		t.Errorf("el comando no llegó a imprimir su salida: %q", out)
	}
}

func TestRunShellCmdCapturaSalidaYExit(t *testing.T) {
	out, code := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "echo hola && exit 0")
	if code != 0 {
		t.Fatalf("code = %d, quiero 0", code)
	}
	if !strings.Contains(out, "hola") {
		t.Fatalf("output = %q, quiero contener \"hola\"", out)
	}

	out2, code2 := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "stderr-va-junto 1>&2; exit 3")
	if code2 != 3 {
		t.Fatalf("code = %d, quiero 3", code2)
	}
	if !strings.Contains(out2, "stderr-va-junto") {
		t.Fatalf("stderr no fue capturado: %q", out2)
	}
}

// flujo completo del modo comando: `!` abre el input al final de la ficha del
// panel, teclear, enter lanza el comando en el repo y marca running "cmd".
func TestBangModoComandoFlujo(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "!") // abre el input
	if !m.cmdOpen {
		t.Fatal("! no abrió el modo comando")
	}

	m, _ = press(m, "g")
	m, _ = press(m, "i")
	m, _ = press(m, "t")
	if got := m.cmdInput.Value(); got != "git" {
		t.Fatalf("input = %q, quiero \"git\"", got)
	}

	m, _ = press(m, "enter")
	if m.cmdOpen {
		t.Fatal("enter no cerró el modo comando")
	}
	if m.running[p.Path] != "cmd" {
		t.Fatalf("running = %q, quiero \"cmd\"", m.running[p.Path])
	}
}

// esc cancela el input sin lanzar nada; enter con input vacío tampoco.
func TestBangModoComandoCancelar(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "!")
	m, _ = press(m, "esc")
	if m.cmdOpen {
		t.Fatal("esc no cerró el modo comando")
	}
	if _, busy := m.running[p.Path]; busy {
		t.Fatal("esc lanzó un comando")
	}

	m, _ = press(m, "!")
	m, cmd3 := press(m, "enter")
	if m.cmdOpen {
		t.Fatal("enter con input vacío no cerró el modo comando")
	}
	// enter vacío abre shell interactiva ($SHELL), si existe
	if _, err := shellPath(); err == nil && m.running[p.Path] != "shell" {
		t.Fatalf("enter vacío debería abrir shell interactiva, running = %q", m.running[p.Path])
	} else if err != nil && cmd3 == nil {
		t.Fatal("sin $SHELL debería notificar")
	}
}

// shellPath replica el fallback de openShellCmd.
func shellPath() (string, error) {
	s := os.Getenv("SHELL")
	if s == "" {
		s = "/bin/sh"
	}
	return exec.LookPath(s)
}

// ! en un repo sin git notifica en vez de abrir el input.
func TestBangSinRepo(t *testing.T) {
	p := proj("bare", "/tmp/gitdash-test/bare", false)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, cmd := press(m, "!")
	if m.cmdOpen {
		t.Fatal("! abrió el input en un repo sin git")
	}
	if cmd == nil {
		t.Fatal("sin repo debería notificar")
	}
}

// tecla g: marca lazygit como running en el repo; sin repo solo notifica.
func TestGLazygit(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	if !hasLazygit() {
		t.Skip("lazygit no instalado")
	}
	m, cmd := press(m, "g")
	if m.running[p.Path] != "lazygit" {
		t.Fatalf("running = %q, quiero \"lazygit\"", m.running[p.Path])
	}
	if cmd == nil {
		t.Fatal("g no devolvió tea.Cmd")
	}

	sin := proj("bare", "/tmp/gitdash-test/bare", false)
	m2 := newTestModel(t, []discovery.Project{sin},
		map[string]gitstatus.Snapshot{sin.Path: snapClean()})
	m2, cmd2 := press(m2, "g")
	if _, busy := m2.running[sin.Path]; busy {
		t.Fatal("g lanzó lazygit en un repo sin git")
	}
	if cmd2 == nil {
		t.Fatal("sin repo debería notificar")
	}
}

func hasLazygit() bool {
	_, err := exec.LookPath("lazygit")
	return err == nil
}

// El cursor del input ! cae sobre el primer rune del placeholder (bubbles
// v2 placeholderView): debe ser un espacio, no una letra que parezca
// tecleada (bug del "! c" fantasma).
func TestBangPlaceholderCursorLimpio(t *testing.T) {
	m := New(config.Defaults())
	m.cmdOpen = true
	v := stripANSI(m.cmdInput.View())
	if !strings.HasPrefix(v, "!  ") {
		t.Fatalf("view = %q, quizo empezar por \"!  \" (prompt + espacio del placeholder)", v)
	}
}

// El input de `!` se pinta al final de la ficha del panel y SIEMPRE visible:
// aunque la ficha haya llenado la caja, lo que se recorta es la ficha, no el
// prompt (escribir un comando sin verlo es escribir a ciegas).
func TestBangInputVisibleEnElPanel(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	// Ficha larga: ficheros y commits de sobra para llenar el panel.
	for i := 0; i < 20; i++ {
		s.Files = append(s.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/f%02d.go", i)})
	}
	for i := 0; i < 10; i++ {
		s.Commits = append(s.Commits, gitstatus.Commit{
			Sha: fmt.Sprintf("%07d", i), Subject: fmt.Sprintf("commit %d", i), When: time.Now().Unix(),
		})
	}
	states["/tmp/dirty-api"] = s
	m := cursorOn(t, newTestModel(t, projects, states), "/tmp/dirty-api")
	lay := m.layout()
	if lay.previewLines < detailHeadLines+cmdInputLines {
		t.Fatalf("precondición: el panel es demasiado pequeño (%d)", lay.previewLines)
	}

	m, _ = press(m, "!")
	if !m.cmdOpen {
		t.Fatal("! no abrió el input")
	}
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "dirty-api"))
	if len(panel) != lay.previewLines {
		t.Errorf("el panel mide %d líneas, want %d (el input no puede desbordarlo)",
			len(panel), lay.previewLines)
	}
	if last := lastNonEmpty(panel); !strings.HasPrefix(last, "! ") {
		t.Errorf("la última línea del panel es %q, want el prompt del input\n%v", last, panel)
	}
	// Lo que se recorta es la ficha por arriba, no la cabecera: el repo y su
	// estado siguen estando.
	if !strings.HasPrefix(panel[0], "path") || !strings.Contains(panel[0], "/tmp/dirty-api") {
		t.Errorf("la cabecera de la ficha no está: %q", panel[0])
	}
	// Con el input abierto la ficha no revive los keybinds del pie.
	if strings.Contains(strings.Join(panel, "\n"), "g lazygit") {
		t.Errorf("con el input abierto vuelven los keybinds del pie:\n%v", panel)
	}
}

// Sin input abierto, la ficha no lleva pie: las teclas de la fila viven en la
// sección de keybinds, y aquí solo se pintan datos del repo.
func TestFichaNoRepiteLosKeybinds(t *testing.T) {
	projects, states := fixtureProjects()
	m := cursorOn(t, newTestModel(t, projects, states), "/tmp/old-clean")
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "old-clean"))
	for _, dup := range []string{"g lazygit", "! cmd", "lazygit"} {
		if last := lastNonEmpty(panel); strings.Contains(last, dup) {
			t.Errorf("la ficha termina en %q, quiere un dato del repo", last)
		}
		if strings.Contains(strings.Join(panel, "\n"), dup) {
			t.Errorf("la ficha repite %q, que ya está en keybinds:\n%v", dup, panel)
		}
	}
}

// panelLines devuelve el interior de una caja sin sus bordes laterales, para
// poder mirar líneas concretas del panel.
func panelLines(t *testing.T, box string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(box, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimSuffix(strings.TrimPrefix(l, "│"), "│")
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

// lastNonEmpty devuelve la última línea con contenido.
func lastNonEmpty(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// Un handoff (lazygit, editor, shell) que falla tiene que ENSEÑAR el motivo: el
// toast es lo único que ve el usuario, porque la terminal ya se ha cerrado. Con
// la guarda invertida, un handoff correcto avisaría de un error que no hubo, y
// uno fallido no diría nada.
func TestHandoffConErrorAvisa(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	for _, c := range []struct {
		nombre   string
		err      error
		quiere   string
		quiereNo string
	}{
		{"fallido avisa el motivo", errHandoff("no such file or directory"), "command", ""},
		{"correcto no avisa nada", nil, "", "command:"},
	} {
		out, _ := m.Update(execDoneMsg{path: "/tmp/old-clean", action: "lazygit", err: c.err})
		plano := stripANSI(out.(Model).View().Content)
		if c.quiere != "" && !strings.Contains(plano, c.quiere) {
			t.Errorf("%s: no se ve %q:\n%s", c.nombre, c.quiere, plano)
		}
		if c.quiereNo != "" && strings.Contains(plano, c.quiereNo) {
			t.Errorf("%s: avisó %q sin motivo:\n%s", c.nombre, c.quiereNo, plano)
		}
	}
}

type errHandoff string

func (e errHandoff) Error() string { return string(e) }

// El recuento del fetch distingue lo que fue de lo que falló: un fetch que
// falla en un repo no puede terminar diciendo "todo ok". Los contadores se
// publican en fetchDoneMsg, que es lo que la barra y el toast resumen.
func TestFetchDoneReportaOkYFailed(t *testing.T) {
	out, _ := newTestModel(t, nil, nil).Update(fetchDoneMsg{ok: 2, failed: 1})
	m := out.(Model)
	plano := stripANSI(m.View().Content)
	if !strings.Contains(plano, "2 ok") && !strings.Contains(plano, "ok 2") {
		t.Errorf("el resumen no dice cuántos fueron bien:\n%s", plano)
	}
	if !strings.Contains(plano, "1 failed") && !strings.Contains(plano, "failed 1") {
		t.Errorf("el resumen no dice cuántos fallaron:\n%s", plano)
	}
	// Sin fallos no hay línea de fallo: la tabla está quieta.
	out, _ = newTestModel(t, nil, nil).Update(fetchDoneMsg{ok: 3})
	plano = stripANSI(out.(Model).View().Content)
	if strings.Contains(plano, "failed") {
		t.Errorf("sin fallos se pintó la línea de fallo:\n%s", plano)
	}
}

// El recuento del fetch distingue lo que fue de lo que falló: con un repo roto
// entre los que se barren, el resumen tiene que decir "1 failed", no "todo ok".
// Los contadores se publican en fetchDoneMsg, que es lo que la barra resume.
func TestFetchAllCuentaLosQueFallan(t *testing.T) {
	bueno, _ := testutil.NewRepo(t, false)
	roto, _ := testutil.NewRepo(t, false)
	testutil.BreakGit(t, roto) // el fetch de este repo falla
	projects := []discovery.Project{
		proj("bueno", bueno, true),
		proj("roto", roto, true),
	}
	states := map[string]gitstatus.Snapshot{bueno: snapClean(), roto: snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "F") // fetch_all
	deadline := time.After(30 * time.Second)
	var done fetchDoneMsg
	for done.ok == 0 && done.failed == 0 {
		select {
		case ev := <-m.events:
			if fd, ok := ev.(fetchDoneMsg); ok {
				done = fd
			}
		case <-deadline:
			t.Fatal("no se observó fetchDoneMsg")
		}
	}
	if done.ok != 1 || done.failed != 1 {
		t.Errorf("fetchDoneMsg = %+v, want ok=1 failed=1", done)
	}
}

// El plegado persistido se guarda como un mapa único: las claves de grupo tal
// cual y los worktrees expandidos bajo su prefijo. Un repo expandido sin ningún
// grupo plegado es el caso donde una pista de capacidad mal calculada se
// convertiría en un tamaño negativo.
func TestSaveCollapsedConMasExpandidosQuePlegados(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.expanded = map[string]bool{"/tmp/multi": true, "/tmp/otro": true, "/tmp/tercero": true}
	m.collapsed = map[string]bool{} // ningún grupo plegado: hint negativo si se resta

	m.saveCollapsed()

	got := m.store.LoadCollapsed()
	if len(got) != 3 {
		t.Fatalf("persistidos = %v, want 3 claves", got)
	}
	for _, path := range []string{"/tmp/multi", "/tmp/otro", "/tmp/tercero"} {
		key := state.WorktreePrefix + path
		if !got[key] {
			t.Errorf("falta la clave del worktree expandido %q: %v", key, got)
		}
	}
}

// Una acción de git CORRIENDO de verdad: el ciclo completo tiene que cerrar —
// el comando ejecuta con un timeout que no lo mata de entrada, publica su
// resultado con el motivo real de git y, si fue bien, re-colecciona el estado
// del repo. Un timeout de 0 (o un "solo re-colecta si falló") se ven aquí.
func TestAccionDeGitCorreYReColecciona(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)         // con upstream
	testutil.PushUpstreamCommits(t, origin, 1, "up") // el repo local está detrás
	testutil.FetchLocal(t, dir)                      // con eso, ff limpio
	states := map[string]gitstatus.Snapshot{dir: snapClean()}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)}, states)
	m = cursorOn(t, m, dir)

	m, _ = press(m, "p") // arma el selector
	m, _ = press(m, "p") // variante default → git pull

	// El re-collect va DESPUÉS del resultado, en la misma goroutine: hay que
	// seguir leyendo después de ver la acción.
	deadline := time.After(60 * time.Second)
	var acc *actionMsg
	var huboStatus bool
	for acc == nil || !huboStatus {
		select {
		case ev := <-m.events:
			switch e := ev.(type) {
			case actionMsg:
				cp := e
				acc = &cp
			case statusMsg:
				huboStatus = true
			}
		case <-deadline:
			t.Fatalf("la acción no cerró el ciclo (acc=%v status=%v)", acc != nil, huboStatus)
		}
	}
	if acc.err != "" {
		t.Fatalf("un pull que debía integrar falló: %q (output %q)", acc.err, acc.output)
	}
	if acc.kind != "pull" {
		t.Errorf("kind = %q, want pull", acc.kind)
	}
	if !huboStatus {
		t.Error("tras una acción correcta no se re-colectó el estado del repo")
	}
}

// Un pull --rebase que choca no es un fallo limpio: deja el rebase a medias. El
// mensaje tiene que llevar esa marca (rebaseInProgress), porque es lo que
// cambia el consejo de "reintenta" por "resuelve el rebase". El flag se calcula
// SOLO si la acción falló y era un pull: con la guarda al revés se consultaría
// en los pulls correctos (donde nunca hay rebase) y nunca en los que chocan.
func TestPullQueChocaMarcaElRebaseAMedias(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	// El remoto cambia un fichero y el local el mismo con otro contenido: el
	// pull --rebase no puede reconciliarlo.
	testutil.PushUpstreamFile(t, origin, "conflicto.txt", "remoto", "remote")
	testutil.CommitFiles(t, dir, map[string]string{"conflicto.txt": "local"}, "local")
	testutil.FetchLocal(t, dir)
	// Y el rebase a medias ya presente (lo que deja el intento anterior).
	if err := os.MkdirAll(filepath.Join(dir, ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	states := map[string]gitstatus.Snapshot{dir: snapClean()}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)}, states)
	m = cursorOn(t, m, dir)

	m, _ = press(m, "p")
	m, _ = press(m, "r") // variante rebase explícita

	deadline := time.After(60 * time.Second)
	var acc *actionMsg
	for acc == nil {
		select {
		case ev := <-m.events:
			if e, ok := ev.(actionMsg); ok {
				cp := e
				acc = &cp
			}
		case <-deadline:
			t.Fatal("el pull no publicó su resultado")
		}
	}
	if acc.err == "" {
		t.Fatal("un pull en conflicto no falló (el fixture no choca)")
	}
	if !acc.rebaseInProgress {
		t.Errorf("rebaseInProgress = false con rebase a medias: %+v", acc)
	}
}

// La shell de `!` y del handoff es la del usuario ($SHELL), no /bin/sh por
// defecto: es lo que hace que sus aliases y su configuración carguen. El argv
// que queda en el command log es lo que lo demuestra, así que se mira ahí.
func TestShellDelHandoffEsLaDelUsuario(t *testing.T) {
	// La ruta del repo tiene que existir: la acción corre `sh -c` con ese cwd.
	dir := t.TempDir()
	proyectos, states := fixtureProjects()
	states[dir] = snapClean()
	proyectos = append(proyectos, proj("cwd", dir, true))

	escribir := func(m Model, texto string) Model {
		for _, k := range texto {
			m, _ = press(m, string(k))
		}
		return m
	}

	t.Run("con SHELL en el entorno", func(t *testing.T) {
		bash := filepath.Join(t.TempDir(), "mishell")
		if err := os.WriteFile(bash, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SHELL", bash)
		m := newTestModel(t, proyectos, states)
		rec := cmdlog.Active()
		t.Cleanup(func() { cmdlog.SetRecorder(nil) })
		m = cursorOn(t, m, dir)

		m, _ = press(m, m.cfg.KeyFor("command"))
		if !m.cmdOpen {
			t.Fatal("la tecla ! no abrió el input de comando")
		}
		m = escribir(m, "hola")
		m, _ = press(m, "enter") // lanza el comando con la shell del usuario
		assertUltimoArgvShell(t, rec, bash, "cmd")
	})

	t.Run("sin SHELL, cae a /bin/sh", func(t *testing.T) {
		t.Setenv("SHELL", "")
		m := newTestModel(t, proyectos, states)
		rec := cmdlog.Active()
		t.Cleanup(func() { cmdlog.SetRecorder(nil) })
		m = cursorOn(t, m, dir)

		m, _ = press(m, m.cfg.KeyFor("command"))
		m = escribir(m, "hola")
		m, _ = press(m, "enter")
		assertUltimoArgvShell(t, rec, "/bin/sh", "cmd")
	})

}

// userShell es la resolución compartida por los DOS handoffs (`!` con texto y
// la shell interactiva del input vacío). El interactivo no se puede ejercitar
// aquí —tea.ExecProcess no corre sin TTY—, así que la función pura es la que
// ata la garantía para los dos caminos.
func TestUserShell(t *testing.T) {
	t.Run("con SHELL", func(t *testing.T) {
		t.Setenv("SHELL", "/opt/homebrew/bin/fish")
		if got := userShell(); got != "/opt/homebrew/bin/fish" {
			t.Errorf("userShell = %q, want la del usuario", got)
		}
	})
	t.Run("sin SHELL", func(t *testing.T) {
		t.Setenv("SHELL", "")
		if got := userShell(); got != "/bin/sh" {
			t.Errorf("userShell = %q, want /bin/sh", got)
		}
	})
}

// assertUltimoArgvShell comprueba la shell con la que se registró el handoff de
// la acción indicada (`cmd` para `!`, `shell` para la interactiva).
func assertUltimoArgvShell(t *testing.T, rec *cmdlog.Recorder, want, accion string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		for _, e := range rec.Entries() {
			if e.Action != accion || len(e.Argv) == 0 {
				continue
			}
			if e.Argv[0] != want {
				t.Errorf("argv[0] = %q, want la shell %q", e.Argv[0], want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no se registró el handoff %q con la shell %q", accion, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// --- los tres huecos que el run destapó ---

// El fetch de UN solo repo no lleva recuento: "fetch ok" y no "fetch ok (1
// repos)", que además suena mal. El caso vive solo si nadie lo afirma: con la
// condición invertida, un fetch de un repo caería en el mensaje de N.
func TestFetchDeUnSoloRepoNoLlevaRecuento(t *testing.T) {
	out, _ := newTestModel(t, nil, nil).Update(fetchDoneMsg{ok: 1})
	plano := stripANSI(out.(Model).View().Content)
	if !strings.Contains(plano, "fetch ok") {
		t.Errorf("el toast no dice que el fetch fue bien:\n%s", plano)
	}
	if strings.Contains(plano, "(1 repos)") {
		t.Errorf("un fetch de un repo no lleva recuento, y menos con la palabra \"repos\":\n%s", plano)
	}
}

// El comando `!` distinguía el éxito del fallo por su código de salida, y con un
// solo repo el mensaje de un fallo dice exit 0 y el de un éxito lo omite: un
// toast que-announces "falló" con exit 0 hace que el usuario vaya a mirar un
// comando que salió bien.
func TestComandoExitoYSalidaNoSeConfunden(t *testing.T) {
	for _, c := range []struct {
		nombre string
		exit   string
		quiere string
	}{
		{"exit 0 no es un fallo", "0", "ok"},
		{"un código de salida sí lo es", "3", "exit 3"},
	} {
		out, _ := newTestModel(t, nil, nil).Update(cmdResultMsg{
			path: "/tmp/dirty-api", command: "echo hola", exit: c.exit,
		})
		m := out.(Model)
		plano := stripANSI(m.View().Content)
		if !strings.Contains(plano, c.quiere) {
			t.Errorf("%s: el aviso no dice %q:\n%s", c.nombre, c.quiere, plano)
		}
		// El resultado queda guardado para la ficha aunque el comando falle: el
		// código de salida es parte del dato, no solo del aviso.
		if got := m.lastCmd["/tmp/dirty-api"]; got.exit != c.exit {
			t.Errorf("%s: lastCmd.exit = %q, want %q", c.nombre, got.exit, c.exit)
		}
		if got := m.lastCmd["/tmp/dirty-api"]; got.command != "echo hola" {
			t.Errorf("%s: lastCmd.command = %q, want el comando tecleado", c.nombre, got.command)
		}
		// Y el repo queda libre: un `!` que falla no puede dejar la fila
		// bloqueada con "already running" para siempre.
		if _, busy := m.running["/tmp/dirty-api"]; busy {
			t.Errorf("%s: el repo quedó con una acción en curso tras el comando", c.nombre)
		}
	}
}

// `end` sobre una tabla VACÍA tiene que dejar el cursor en 0, no en un índice
// que no existe: el clamp de max(0, len-1) existe justo para ese caso, y sin él
// el cursor se sale de las entradas y el render lee una fila que no hay.
func TestEndSobreTablaVaciaNoSeSaleDelCursor(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if len(m.entries()) != 0 {
		t.Fatalf("el modelo sin proyectos tiene %d entradas, want 0", len(m.entries()))
	}

	m, _ = press(m, "end")
	if got := m.cursor; got != 0 {
		t.Errorf("cursor = %d en una tabla vacía, want 0", got)
	}
	// Y con filas, `end` va a la última de verdad.
	projects, states := fixtureProjects()
	m2 := newTestModel(t, projects, states)
	m2, _ = press(m2, "end")
	if want := len(m2.entries()) - 1; m2.cursor != want {
		t.Errorf("cursor = %d, want %d (la última fila)", m2.cursor, want)
	}
}

// El execDoneMsg de un handoff es lo que el comando de la TUI ve al VOLVER del
// proceso, y de el salen dos cosas: la entrada del log de comandos (con la accion
// y el argv) y el recollect del estado. Por eso la accion tiene que ser la de la
// tecla que se pulso, no la de otra: con "editor" donde deberia decir "lazygit",
// el panel muestra un comando que el usuario nunca ejecuto.
//
// Se prueba el metodo y no el closure de tea.ExecProcess porque el cuerpo de ese
// closure solo corre cuando el proceso termina, con el programa suspendido: no
// hay forma de escribir un test que lo alcance sin ceder la terminal de verdad.
func TestHandoffDoneIdentificaLaAccion(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	for _, accion := range []string{"editor", "lazygit", "pull_ai", "visual", "shell"} {
		msg, ok := m.handoffDone(accion, "/tmp/repo", []string{"argv", "de", accion}, nil).(execDoneMsg)
		if !ok {
			t.Fatalf("handoffDone(%q) no devolvio un execDoneMsg", accion)
		}
		if msg.action != accion {
			t.Errorf("handoffDone(%q).action = %q, want %q (la accion va al log tal cual)",
				accion, msg.action, accion)
		}
		if msg.path != "/tmp/repo" {
			t.Errorf("handoffDone(%q).path = %q, want /tmp/repo", accion, msg.path)
		}
		if len(msg.argv) != 3 || msg.argv[2] != accion {
			t.Errorf("handoffDone(%q).argv = %q, want el argv intacto (es lo que se registra)",
				accion, msg.argv)
		}
		if msg.err != nil {
			t.Errorf("handoffDone(%q).err = %v, want nil", accion, msg.err)
		}
	}

	// Y el error del proceso se propaga tal cual: un handoff que salio con
	// codigo 1 tiene que recollectar igual, no quedarse creyendo que fue limpio.
	boom := errors.New("salida con codigo 1")
	msg, ok := m.handoffDone("editor", "/tmp/repo", []string{"vim"}, boom).(execDoneMsg)
	if !ok {
		t.Fatal("handoffDone no devolvio un execDoneMsg")
	}
	if !errors.Is(msg.err, boom) {
		t.Errorf("err = %v, want el error del proceso propagado", msg.err)
	}
}

// Los dos handoffs con guarda LookPath comprueban el binario ANTES de mirar si
// el repo ya esta ocupado, y en ese orden por un motivo concreto: si la Maquina
// no tiene lazygit, el aviso tiene que ser "no instalado" y no "ya hay un
// lazygit corriendo en este repo", que seria mentira.
//
// Se prueban las dos guardas por separado porque son guards distintas: la
// primera es del entorno, la segunda es del modelo.
func TestHandoffsConGuardaDeLookPath(t *testing.T) {
	t.Run("lazygit no instalado", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // sin lazygit
		mm := newTestModel(t, nil, nil)
		m := &mm
		msg, ok := m.openLazygitCmd("/tmp/repo")().(notifyMsg)
		if !ok {
			t.Fatal("openLazygitCmd sin lazygit no devolvio un notifyMsg")
		}
		if msg.level != toastWarning || !strings.Contains(msg.text, "not installed") {
			t.Errorf("aviso = %+v, want un warning de que no esta instalado", msg)
		}
		// Y no se marco el repo como ocupado: no se lanzo nada.
		if len(m.running) != 0 {
			t.Errorf("running = %v, want vacio (no se lanzo el handoff)", m.running)
		}
	})

	t.Run("shell no encontrada", func(t *testing.T) {
		t.Setenv("SHELL", "una-shell-que-no-existe-98765")
		t.Setenv("PATH", t.TempDir())
		mm := newTestModel(t, nil, nil)
		m := &mm
		msg, ok := m.openShellCmd("/tmp/repo")().(notifyMsg)
		if !ok {
			t.Fatal("openShellCmd sin shell no devolvio un notifyMsg")
		}
		if msg.level != toastWarning || !strings.Contains(msg.text, "shell") {
			t.Errorf("aviso = %+v, want un warning de shell not found", msg)
		}
		if len(m.running) != 0 {
			t.Errorf("running = %v, want vacio", m.running)
		}
	})

	// La guarda de ocupado: con el repo ya en `running`, el segundo handoff se
	// rechaza y lo dice. Aqui el binario SI esta (el shell del sistema), asi que
	// se llega a la segunda guarda, que es la que se quiere probar.
	t.Run("repo ya ocupado", func(t *testing.T) {
		mm := newTestModel(t, nil, nil)
		m := &mm
		m.running = map[string]string{"/tmp/repo": "pull"}
		msg, ok := m.openShellCmd("/tmp/repo")().(notifyMsg)
		if !ok {
			t.Fatal("openShellCmd con el repo ocupado no devolvio un notifyMsg")
		}
		if !strings.Contains(msg.text, "already running") {
			t.Errorf("aviso = %q, want que diga que ya hay algo en marcha", msg.text)
		}
		if !strings.Contains(msg.text, "pull") {
			t.Errorf("aviso = %q, want que nombre la accion que esta corriendo", msg.text)
		}
	})
}

// recorder de handoffs: NO cede la terminal.
//
// tea.ExecProcess suspende el programa entero hasta que el proceso termina, y un
// programa suspendido no lo reanuda nadie. Por eso el `return tea.ExecProcess(...)`
// de cada handoff era un statement que no ejecutaba nadie: era el techo de la
// cobertura de la TUI, no un hueco de tests. Con la inyeccion, el handoff se
// ejecuta entero en el test —guardas, armado del comando y mensaje de salida— y
// lo que se comprueba es lo de verdad: que el comando apunta al repo correcto y
// que el execDoneMsg lleva la accion y el argv que el panel del log va a pintar.
func handoffRecorder(t *testing.T) (*Model, *handoffSpy) {
	t.Helper()
	mm := newTestModel(t, nil, nil)
	m := &mm
	spy := &handoffSpy{t: t, done: func(error) tea.Msg { return nil }}
	m.handoff = spy.exec
	return m, spy
}

type handoffSpy struct {
	t    *testing.T
	done tea.ExecCallback
	argv []string
	dir  string
	vals int
}

func (h *handoffSpy) exec(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	h.argv, h.dir = c.Args, c.Dir
	h.vals++
	// Se guarda el callback sin llamarlo: quien lo llama es el runtime, y el
	// test lo invoca explicitamente para ver el mensaje de salida.
	h.done = fn
	return func() tea.Msg { return nil }
}

// salir entrega el mensaje de salida del proceso, como haria el runtime al
// terminar el handoff.
func (h *handoffSpy) salir(err error) tea.Msg {
	h.t.Helper()
	return h.done(err)
}

func TestHandoffEjecutaElProcesoEnElRepo(t *testing.T) {
	for _, c := range []struct {
		accion string
		abrir  func(m *Model, path string) tea.Cmd
		argv0  string
	}{
		{"editor", (*Model).openEditorCmd, "vi"},
		{"lazygit", (*Model).openLazygitCmd, "lazygit"},
		{"shell", (*Model).openShellCmd, ""},
	} {
		t.Run(c.accion, func(t *testing.T) {
			m, spy := handoffRecorder(t)
			m.cfg.Editor = "vi"
			// El shell del handoff tiene que existir para pasar la guarda.
			if c.accion == "shell" {
				t.Setenv("SHELL", "/bin/sh")
			} else if c.accion == "lazygit" {
				// El LookPath de lazygit mira el PATH de verdad: se le da un
				// ejecutable falso para pasar la guarda sin tener lazygit
				// instalado, y sin lanzar nada (el handoff esta inyectado).
				bin := filepath.Join(t.TempDir(), "lazygit")
				if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))
			}

			if c.abrir(m, "/tmp/repo") == nil {
				t.Fatalf("%s no devolvio comando", c.accion)
			}
			if spy.vals != 1 {
				t.Fatalf("handoff llamado %d veces, want 1", spy.vals)
			}
			if spy.dir != "/tmp/repo" {
				t.Errorf("Dir = %q, want /tmp/repo (el handoff corre DENTRO del repo)", spy.dir)
			}
			if len(spy.argv) == 0 {
				t.Fatalf("argv vacio")
			}
			// Al salir, el mensaje dice que repo y que accion, que es lo que
			// despues aparece en el panel del log.
			msg, ok := spy.salir(nil).(execDoneMsg)
			if !ok {
				t.Fatalf("el handoff de %s no devolvio execDoneMsg al salir", c.accion)
			}
			if msg.path != "/tmp/repo" || msg.action != c.accion {
				t.Errorf("execDoneMsg = %+v, want path=/tmp/repo action=%s", msg, c.accion)
			}
			if len(msg.argv) == 0 || msg.argv[0] != spy.argv[0] {
				t.Errorf("execDoneMsg.argv = %q, want el mismo que se ejecuto (%q)", msg.argv, spy.argv)
			}
		})
	}
}

// Los dos handoffs que tienen guardas de dominio (el comando AI y el preview de
// git-sim) llegan al handoff solo si todo lo de antes esta bien. Los dos casos
// que se comprueban aqui son los que mas easily se rompen sin que se note:
//
//   - el repo ya esta ocupado (running): el segundo handoff se rechaza. Sin este
//     test, dos teclas seguidas podrian ceder la terminal dos veces al mismo repo.
//   - el handoff se arma en el repo, no en el directorio de gitdash.
func TestHandoffsConGuardaDeOcupado(t *testing.T) {
	t.Run("pull_ai con el repo ocupado", func(t *testing.T) {
		mm := newTestModel(t, nil, nil)
		m := &mm
		m.running = map[string]string{"/tmp/repo": "pull"}
		msg, ok := m.openPullAICmd("/tmp/repo", []string{"ai-pull", "--x"})().(notifyMsg)
		if !ok {
			t.Fatal("openPullAICmd con el repo ocupado no devolvio un aviso")
		}
		if !strings.Contains(msg.text, "already running") {
			t.Errorf("aviso = %q, want que diga que ya hay algo en marcha", msg.text)
		}
	})

	t.Run("visual con el repo ocupado", func(t *testing.T) {
		mm := newTestModel(t, nil, nil)
		m := &mm
		m.running = map[string]string{"/tmp/repo": "pull_ai"}
		msg, ok := m.startVisualCmd("/tmp/repo", "origin/main", "merge")().(notifyMsg)
		if !ok {
			t.Fatal("startVisualCmd con el repo ocupado no devolvio un aviso")
		}
		if !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "pull_ai") {
			t.Errorf("aviso = %q, want que nombre la accion que esta corriendo", msg.text)
		}
	})

	// Y el camino de verdad: sin nada ocupado, el handoff se arma con el argv de
	// git-sim y en el repo. El LookPath necesita un git-sim en el PATH.
	t.Run("visual arma el argv y el dir", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "git-sim")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		mm := newTestModel(t, nil, nil)
		m := &mm
		spy := &handoffSpy{t: t, done: func(error) tea.Msg { return nil }}
		m.handoff = spy.exec

		if cmd := m.startVisualCmd("/tmp/repo", "origin/main", "rebase"); cmd == nil {
			t.Fatal("startVisualCmd devolvio nil")
		}
		if spy.dir != "/tmp/repo" {
			t.Errorf("Dir = %q, want /tmp/repo", spy.dir)
		}
		// merge/rebase necesitan el ref del upstream; pull no.
		full := strings.Join(spy.argv, " ")
		if !strings.Contains(full, "rebase") {
			t.Errorf("argv = %q, want el subcomando rebase", full)
		}
		if !strings.Contains(full, "origin/main") {
			t.Errorf("argv = %q, want el ref del upstream (rebase lo exige)", full)
		}
		if !strings.Contains(full, "--media-dir") {
			t.Errorf("argv = %q, want --media-dir SIEMPRE (sin el, git-sim ensucia el repo)", full)
		}
		if m.running["/tmp/repo"] != "visual" {
			t.Errorf("running = %v, want el repo marcado como visual en curso", m.running)
		}

		// El `pull` de git-sim NO lleva ref: no lo exige y pasarselo haria que
		// git-sim comprobara otra cosa.
		spy.vals = 0
		if cmd := m.startVisualCmd("/tmp/repo2", "origin/main", "pull"); cmd == nil {
			t.Fatal("startVisualCmd(pull) devolvio nil")
		}
		if strings.Contains(strings.Join(spy.argv, " "), "origin/main") {
			t.Errorf("argv = %q, want sin ref: el pull de git-sim no lleva argumento", spy.argv)
		}
	})
}

// recollectCmd con el repo ocupado devuelve nil en vez de relanzar: el recollect
// es una lectura, y relanzarla sobre un repo que ya tiene una acción de escritura
// en marcha daría un snapshot a medio camino. Nil y no un aviso porque recollect
// no es una tecla que el usuario haya pulsado con una intención (lo dispara el
// propio programa al volver de un handoff).
func TestRecollectConRepoOcupadoNoRelanza(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "pull"}
	antes := len(m.running)

	if cmd := m.recollectCmd("/tmp/api"); cmd != nil {
		t.Errorf("recollectCmd sobre un repo ocupado devolvió %#v, want nil", cmd())
	}
	// Y no lo ha marcado como ocupado otra vez: la guarda tiene que devolver
	// ANTES de escribir en running, no despues.
	if got := m.running["/tmp/api"]; got != "pull" {
		t.Errorf("running = %q, want que siga siendo la accion original", got)
	}
	if len(m.running) != antes {
		t.Errorf("running tiene %d entradas, want %d (la guarda no escribe)", len(m.running), antes)
	}
}

// removeWorktreeCmd con el padre ocupado devuelve el aviso de busyActionCmd, y
// no borra nada. El caso importa por lo que se ve: `d` sobre un worktree cuyo
// repo principal esta haciendo un pull tiene que decir "ya hay un pull en
// marcha", no fallar en silencio ni borrar igual.
func TestRemoveWorktreeConPadreOcupadoAvisa(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "pull"}

	cmd := m.removeWorktreeCmd("/tmp/api", "/tmp/api-wt", "api-wt", false, 1)
	if cmd == nil {
		t.Fatal("removeWorktreeCmd con el padre ocupado devolvio nil, want un aviso")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("devolvió %T, want un aviso", cmd())
	}
	if !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "pull") {
		t.Errorf("aviso = %q, want que nombre la accion en marcha", msg.text)
	}
	// Y no se ha escrito el flag de esta accion: sigue siendo el pull.
	if got := m.running["/tmp/api"]; got != "pull" {
		t.Errorf("running = %q, want pull (no worktree_remove)", got)
	}
}

// openLazygitCmd con el repo ocupado: el LookPath pasa (hay un lazygit falso en
// el PATH) y se llega a la segunda guarda. Es la unica forma de comprobar que el
// orden de las dos guardas es el correcto: si la de ocupado fuera primero, con
// lazygit ausente diria "ya hay algo en marcha" en una maquina sin lazygit.
func TestLazygitOcupadoDiceLaAccionQueCorre(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "lazygit")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))

	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "push"}

	msg, ok := m.openLazygitCmd("/tmp/api")().(notifyMsg)
	if !ok {
		t.Fatal("openLazygitCmd con el repo ocupado no devolvio un aviso")
	}
	if !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "push") {
		t.Errorf("aviso = %q, want que nombre el push en marcha", msg.text)
	}
}

// `!` con el repo ocupado no abre una shell ni captura nada: avisa. Es el mismo
// guard que los handoffs, aqui para el comando capturado, que ademas es la via
// que mas se usa (no necesita tecla separada).
func TestComandoCapturadoConRepoOcupadoAvisa(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "pull"}

	msg, ok := m.openCmdCmd("/tmp/api", "git status")().(notifyMsg)
	if !ok {
		t.Fatal("openCmdCmd con el repo ocupado no devolvio un aviso")
	}
	if !strings.Contains(msg.text, "already running") {
		t.Errorf("aviso = %q, want que diga que ya hay algo en marcha", msg.text)
	}
	// Y no se ha lanzado nada: el flag sigue siendo el del pull.
	if got := m.running["/tmp/api"]; got != "pull" {
		t.Errorf("running = %q, want pull (el guard devuelve antes de escribir)", got)
	}
}
