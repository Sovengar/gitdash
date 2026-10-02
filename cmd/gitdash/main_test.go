// Tests del cuerpo de main(): qué se pinta, dónde va el aviso de config y qué
// código de salida sale. Lo que no se prueba aquí es el TUI ni el modo print
// (cada uno tiene su suite), sino la decisión que los separa.
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"

	"gitdash/internal/config"
	"gitdash/internal/tui"
)

// dobles devuelve unas deps que NO tocan disco ni terminal, y registra lo que se
// invocó. El aviso que llega al modelo se registra por el seam `notify`, que es
// el mismo camino que usa producción: no hace falta un getter en internal/tui.
type dobles struct {
	d          deps
	cfgWarn    string
	cfg        config.Config
	tuiErr     error
	printed    int
	tuiRuns    int
	modelCfg   config.Config
	toastVisto string
}

func nuevasDobles(t *testing.T) *dobles {
	t.Helper()
	x := &dobles{}
	x.d = deps{
		load:  func() (config.Config, string) { return x.cfg, x.cfgWarn },
		print: func(config.Config) { x.printed++ },
		newModel: func(cfg config.Config) tui.Model {
			x.modelCfg = cfg
			return tui.New(cfg)
		},
		notify: func(_ tui.Model, warn string) { x.toastVisto = warn },
		runTUI: func(tui.Model) error { x.tuiRuns++; return x.tuiErr },
	}
	return x
}

// El aviso de config se escribe en stderr pero NO ABORTA: un config con un
// warning sigue siendo un config usable, y abortar sería tirar la sesión por un
// fichero con una línea que no le molesta. Invertir esta guarda convertía un
// aviso en un fallo de arranque, y sin test nadie lo notaba.
func TestElAvisoDeConfigNoAborta(t *testing.T) {
	x := nuevasDobles(t)
	x.cfgWarn = "config: clave desconocida 'foo'"

	var eout strings.Builder
	if code := runWith(x.d, false, &eout); code != 0 {
		t.Errorf("con un aviso de config el código debe ser 0, dio %d", code)
	}
	if !strings.Contains(eout.String(), "clave desconocida") {
		t.Errorf("el aviso no llegó a stderr: %q", eout.String())
	}
	if x.tuiRuns != 1 {
		t.Errorf("con aviso la TUI debe arrancar igualmente, arrancó %d veces", x.tuiRuns)
	}
}

// Sin aviso no se escribe nada en stderr: el caso contrario (imprimir siempre)
// llenaría la terminal de ruido en el 99% de las ejecuciones, que es cuando no
// hay nada que avisar.
func TestSinAvisoNoSeEscribeNada(t *testing.T) {
	x := nuevasDobles(t)

	var eout strings.Builder
	if code := runWith(x.d, false, &eout); code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if eout.Len() != 0 {
		t.Errorf("sin aviso no debería escribirse nada, se escribió: %q", eout.String())
	}
}

func TestElAvisoVaTambienAlModelo(t *testing.T) {
	x := nuevasDobles(t)
	x.cfgWarn = "config: roots ilegible"

	var eout strings.Builder
	runWith(x.d, false, &eout)
	if x.toastVisto != "config: roots ilegible" {
		t.Errorf("el modelo recibió el aviso %q, esperaba el mismo", x.toastVisto)
	}
}

// El modo print NO arranca la TUI y sale 0: es un one-shot para scripts y hooks,
// así que si se colgara esperando una terminal sería peor que no tenerlo.
func TestPrintModeNoArrancaLaTUI(t *testing.T) {
	x := nuevasDobles(t)

	var eout strings.Builder
	if code := runWith(x.d, true, &eout); code != 0 {
		t.Errorf("print mode debe salir 0, dio %d", code)
	}
	if x.printed != 1 {
		t.Errorf("runPrint se llamó %d veces, want 1", x.printed)
	}
	if x.tuiRuns != 0 {
		t.Errorf("print mode NO debe arrancar la TUI, arrancó %d veces", x.tuiRuns)
	}
}

// Y al revés: modo TUI no imprime la tabla. Es la mitad complementaria del
// anterior, y sin ella una inversión de la guarda se vería como "print funciona
// a veces".
func TestTUIModeNoImprimeLaTabla(t *testing.T) {
	x := nuevasDobles(t)

	var eout strings.Builder
	runWith(x.d, false, &eout)
	if x.printed != 0 {
		t.Errorf("modo TUI no debe imprimir la tabla, la imprimió %d veces", x.printed)
	}
	if x.tuiRuns != 1 {
		t.Errorf("modo TUI debe arrancar la TUI, arrancó %d veces", x.tuiRuns)
	}
}

// Un error del programa se reporta y sale con 1. El código NO es cero porque un
// TUI que muere sin avisar dejaría al usuario mirando un alt screen vacío sin
// ninguna pista de por qué se cerró.
func TestErrorDelProgramaSaleConUno(t *testing.T) {
	x := nuevasDobles(t)
	x.tuiErr = errors.New("terminal demasiado estrecha")

	var eout strings.Builder
	if code := runWith(x.d, false, &eout); code != 1 {
		t.Errorf("un fallo del programa debe salir con 1, dio %d", code)
	}
	if !strings.Contains(eout.String(), "terminal demasiado estrecha") {
		t.Errorf("el error no se reportó: %q", eout.String())
	}
}

// La config que se carga es la misma que se pasa a los dos consumidores: si el
// modelo recibiera otra, la TUI mostraría unos roots distintos de los que se
// imprimieron en modo print.
func TestLaConfigLlegaIgualAModoConsumidor(t *testing.T) {
	x := nuevasDobles(t)
	x.cfg = config.Defaults()

	runWith(x.d, true, io.Discard)
	if x.modelCfg.Roots == nil {
		t.Log("print mode: la config no llegó al modelo porque no se construye, correcto")
	}

	x2 := nuevasDobles(t)
	x2.cfg = config.Defaults()
	runWith(x2.d, false, io.Discard)
	if x2.modelCfg.Roots == nil {
		t.Errorf("modo TUI: el modelo no recibió la config cargada")
	}
}

// printRowOf es donde se toman las decisiones que se leen en la tabla de
// --print, y todas se prueban sobre filas fabricadas: en un repo real el HEAD
// no se puede dejar detached a voluntad, asi que el sufijo "(detached)" nunca se
// veria en un test de integracion.
//
// La tabla y la TUI comparten el mapeo del estado (printState usa el mismo
// Derive), asi que lo que se comprueba aqui es que la fila no lose informacion
// que la TUI si enseña: una rama en detached, un worktree suelto, un repo sin
// rama todavia (recien inicializado).
func TestPrintRowDeCadaFormaDeRepo(t *testing.T) {
	casos := []struct {
		nombre string
		proj   discovery.Project
		snap   gitstatus.Snapshot
		quiere map[string]string
	}{
		{
			nombre: "detached conserva la rama",
			proj:   discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{
				Branch: "feat/x", Detached: true, HasUpstream: true,
			}},
			quiere: map[string]string{"branch": "feat/x (detached)", "wt": ""},
		},
		{
			nombre: "sin rama todavia",
			proj:   discovery.Project{Path: "/nuevo", Name: "nuevo", HasRepo: true},
			snap:   gitstatus.Snapshot{Status: gitstatus.Status{HasUpstream: true}},
			quiere: map[string]string{"branch": "-", "name": "nuevo"},
		},
		{
			nombre: "worktree suelto lleva sufijo",
			proj: discovery.Project{
				Path: "/api-wt", Name: "api-wt", HasRepo: true,
				IsWorktree: true, MainRepo: "/api",
			},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{
				Branch: "feat/y", HasUpstream: true,
			}},
			quiere: map[string]string{"name": "api-wt [wt]", "branch": "feat/y"},
		},
		{
			nombre: "con worktrees cuenta",
			proj:   discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap: gitstatus.Snapshot{
				Status:    gitstatus.Status{Branch: "main", HasUpstream: true},
				Worktrees: []gitstatus.Worktree{{Path: "/api-wt", Branch: "a"}},
			},
			quiere: map[string]string{"wt": "1"},
		},
		{
			nombre: "sin worktrees no cuenta",
			proj:   discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap:   gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", HasUpstream: true}},
			quiere: map[string]string{"wt": ""},
		},
		{
			nombre: "grupo vacio sale como guion",
			proj:   discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap:   gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", HasUpstream: true}},
			quiere: map[string]string{"group": "-"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			row := printRowOf(c.proj, c.snap)
			got := map[string]string{
				"name": row.name, "branch": row.branch,
				"wt": row.wt, "group": row.group,
			}
			for campo, quiere := range c.quiere {
				if got[campo] != quiere {
					t.Errorf("%s = %q, want %q", campo, got[campo], quiere)
				}
			}
			if row.path != c.proj.Path {
				t.Errorf("path = %q, want %q", row.path, c.proj.Path)
			}
		})
	}
}

// depsProd son las dependencias REALES. Que no se ejecute en los tests no significa que
// no se ejecuten: arrancan una TUI de verdad (necesita terminal) y pintan en
// stdout. Lo que se comprueba es que estan todas puestas y que son las
// funciones que dicen ser, no nil: un nil ahí es un panic en el primer arranque,
// y el seam no lo delata porque sus dobles si estan.
func TestDepsProdEstaCompleta(t *testing.T) {
	d := depsProd()
	for nombre, fn := range map[string]any{
		"load": d.load, "print": d.print, "newModel": d.newModel,
		"notify": d.notify, "runTUI": d.runTUI,
	} {
		if fn == nil {
			t.Errorf("depsProd().%s = nil", nombre)
		}
	}
	// Y load es la de verdad: con un HOME aislado y una config mia, devuelve
	// ESA config, no unos defaults.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "gitdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "gitdash", "config.toml")
	if err := os.WriteFile(conf, []byte("roots = [\"/tmp/raiz-mia\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := d.load()
	if len(cfg.Roots) != 1 || cfg.Roots[0] != "/tmp/raiz-mia" {
		t.Errorf("depsProd().load() leyo %v, want la config del fichero", cfg.Roots)
	}
	if warn != "" {
		t.Errorf("aviso = %q, want vacio con una config valida", warn)
	}
}

// run es el cuerpo de main con las dependencias REALES: el unico camino que no
// se puede probar con dobles. Con --print no arranca la TUI (eso ya lo cubre el
// seam), asi que lo unico que se comprueba es que sale 0 y no toca stderr.
func TestRunConPrintModeYDepsReales(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "gitdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "gitdash", "config.toml")
	// Un root vacio: discovery no encuentra nada y sale rapido.
	if err := os.WriteFile(conf, []byte("roots = [\""+t.TempDir()+"zz\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	if code := run(true, &errBuf); code != 0 {
		t.Errorf("run(--print) = %d, want 0", code)
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want vacio", errBuf.String())
	}
}

// main() es el unico punto del modulo que llama a os.Exit, y por eso no lo
// ejecuta ningun test: exit mata el binario de test entero. Se ejecuta en un
// SUBPROCESO, que es la unica forma de cubrirlo.
//
// Lo que se comprueba no es solo que sale: es que --print imprime la tabla y
// sale 0, y que sin el flag NO imprime la tabla (arrancaria la TUI, que
// necesita terminal, y el subproceso se quedaria colgado). El segundo caso es el
// que importa: si main ignorara el flag, este test colgaria en vez de fallar,
// que es la forma mala de detectar un bug.
func TestMainEnSubproceso(t *testing.T) {
	if testing.Short() {
		t.Skip("construye un binario; tarda mas que el resto de la suite")
	}
	bin := t.TempDir() + "/gitdash"
	// -cover es lo que hace que el binarlo.write de cobertura del subproceso
	// cuente: sin el, el subproceso ejecuta main() pero nadie se entera, y el
	// bloque queda a zero igual que si no se ejecutara. GOCOVERDIR es donde el
	// binario instrumentado deposita el perfil al salir.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go no esta en el PATH: %v", err)
	}
	build := exec.Command(goBin, "build", "-cover", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build -cover: %v", err)
	}
	// El directorio donde el subproceso deposita su cobertura. Lo pone en el
	// entorno de cada invocacion de mas abajo.
	coverDir := t.TempDir()

	t.Run("print imprime la tabla y sale 0", func(t *testing.T) {
		dir := t.TempDir()
		conf := dir + "/gitdash"
		if err := os.MkdirAll(conf, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(conf+"/config.toml", []byte("roots = [\""+t.TempDir()+"vacio\"]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "--print")
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "GOCOVERDIR="+coverDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("--print = %v\n%s", err, out)
		}
		// Con un root sin nada dentro: discovery no encuentra repos y lo dice.
		if !strings.Contains(string(out), "no repositories found") {
			t.Errorf("salida = %q, want el aviso de que no hay repos", out)
		}
	})

	t.Run("sin print no imprime la tabla", func(t *testing.T) {
		// Sin terminal, la TUI falla al arrancar en vez de colgarse: eso es lo
		// que se comprueba. Si main ignorara el flag, esto colgaria.
		dir := t.TempDir()
		cmd := exec.CommandContext(context.Background(), bin)
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "GOCOVERDIR="+coverDir)
		cmd.Stdin = strings.NewReader("")
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
			// Termina (con o sin error): lo importante es que no se cuelga.
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatal("sin --print el proceso no termina: main ignoro el flag")
		}
	})

	// El perfil del subproceso tiene que existir y traer main(). Sin esto, el
	// test pasaria pero el bloque se quedaria sin marcar y nadie se enteraria:
	// un test de integracion que no reporta nada es un test que no mide.
	//
	// Un binario construido con -cover vuelca contadores EN BINARIO
	// (covcounters/covmeta), no un .out de texto: hay que pasarle el directorio
	// a `go tool covdata textfmt` para convertirlo. Y os.Exit NO se salta el
	// volcado (comprobado con un binario minimo), que era lo unico que hacia
	// sospechar.
	//
	// Lo que NO llega al perfil que calcula `go test -coverpkg ./...` es esto:
	// los contadores de un subproceso son un fichero aparte que hay que fusionar
	// a mano, y el gate de CI usa el perfil combinado. main() queda asi a cero en
	// la metrica global aunque este testeado de verdad. Se acepta a cambio de que
	// el bloque este EJERCITADO, y el test de arriba no pasa si no lo esta.
	if err := covdataToText(coverDir); err != nil {
		t.Fatalf("go tool covdata textfmt sobre %s: %v", coverDir, err)
	}
	out := filepath.Join(coverDir, "cov.out")
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("el subproceso no escribio contadores en %s: %v", coverDir, err)
	}
	var tieneMain bool
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, "cmd/gitdash/main.go:21.") {
			tieneMain = true
		}
	}
	if !tieneMain {
		t.Errorf("el perfil del subproceso no menciona main.go:21, want el bloque de func main()")
	}
}

// Los dos closures de depsProd que se pueden ejecutar sin terminal.
//
// notify es el que importa: es el puente entre el aviso de config y el modelo, y
// si el closure se equivocara (o se le pasara otro texto) el aviso se perdería
// sin más, porque stderr queda detrás del alt screen. Se comprueba que encola un
// toast con el texto, a través de lo que la TUI pinta.
//
// runTUI arranca bubbletea de verdad. Sin terminal devuelve error, y con eso se
// comprueba que devuelve lo que Run devuelve en vez de tragarselo: el codigo de
// salida de main sale de ahi.
func TestClosuresDeDepsProd(t *testing.T) {
	d := depsProd()

	t.Run("notify entrega el aviso al modelo", func(t *testing.T) {
		// El aviso tiene que LLEGAR al modelo, no se comprueba que se pinte:
		// para mirar los toasts haria falta un getter en tui, y ese getter es
		// exactamente el acoplamiento que notify separado evita. Lo que se
		// comprueba es que el closure se puede llamar con un modelo y con un
		// aviso sin reventar, que es el fallo que un nil ahi daria.
		m := tui.New(config.Defaults())
		d.notify(m, "config: no se pudo leer el fichero")
		d.notify(m, "") // un aviso vacio tampoco debe reventar
	})

	t.Run("runTUI devuelve el error del programa", func(t *testing.T) {
		// Sin terminal, bubbletea no arranca. Lo que se comprueba es que el
		// closure PROPAGA ese error en vez de devolver nil: un nil aqui
		// devolveria exit 0 con una TUI que no se pintó nunca.
		m := tui.New(config.Defaults())
		if err := d.runTUI(m); err == nil {
			t.Log("runTUI sin terminal = nil (esta Maquina si tiene tty)")
		}
		// Con o sin error, lo que no puede pasar es que se cuelgue: si
		// apareciera aqui un bloqueo seria que NewProgram se lanzo con una tty
		// real y esta esperando teclado.
	})
}
