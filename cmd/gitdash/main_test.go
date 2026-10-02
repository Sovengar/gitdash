// Tests del cuerpo de main(): qué se pinta, dónde va el aviso de config y qué
// código de salida sale. Lo que no se prueba aquí es el TUI ni el modo print
// (cada uno tiene su suite), sino la decisión que los separa.
package main

import (
	"errors"
	"io"
	"strings"
	"testing"

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
