// gitdash — panel de estados git en TUI (inspirado en bircni/git-statuses).
//
// Descubre proyectos por fichero marcador (.gitdash.toml) en los roots
// configurados, recolecta el estado git de cada uno (branch, dirty,
// ahead/behind) vía subprocess git, y lo muestra en un dashboard Bubbletea
// con fetch automático en batches y acciones pull/push/editor.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/config"
	"gitdash/internal/tui"
)

func main() {
	printMode := flag.Bool("print", false, "print the status table and exit")
	flag.Parse()
	os.Exit(run(*printMode, os.Stderr))
}

// El modelo que se pinta es un seam, no el TUI real: la TUI necesita una
// terminal y aquí lo que hay que probar es la DECISIÓN (aviso a stderr, elegir
// print o TUI, código de salida), no el alt screen. Además main() no es
// testeable por sí solo —llama a os.Exit— y sus dos guards (el aviso de config y
// el error del programa) se quedaban sin cubrir sin importarlo.
//
// notify va aparte de newModel a propósito: el aviso que se encola al modelo es
// una decisión de main (si se perdiera, el usuario no lo vería nunca, porque
// stderr se queda detrás del alt screen), y para observarlo desde package main
// haría falta un getter en tui. Con el aviso en el seam, main lo verifica sin
// que tui exporte nada; qué hace el toast con el texto lo prueba tui.
type deps struct {
	// load devuelve la config y su aviso. Es config.Load en producción.
	load func() (config.Config, string)
	// print es runPrint, que escribe en os.Stdout a propósito (ver print.go).
	print func(config.Config)
	// newModel construye el modelo a partir de la config.
	newModel func(config.Config) tui.Model
	// notify entrega el aviso de config al modelo. Es Model.NotifyConfig.
	notify func(tui.Model, string)
	// runTUI arranca el programa y devuelve su error.
	runTUI func(tui.Model) error
}

// depsProd son las dependencias reales.
func depsProd() deps {
	return deps{
		load:     config.Load,
		print:    runPrint,
		newModel: tui.New,
		notify:   func(m tui.Model, warn string) { m.NotifyConfig(warn) },
		runTUI:   func(m tui.Model) error { _, err := tea.NewProgram(m).Run(); return err },
	}
}

// run es el cuerpo de main: decide qué pintar y devuelve el código de salida, sin
// salir del proceso. Todo lo que hace falta para tapar los dos guards vive aquí
// y es comprobable con dobles.
func run(printMode bool, eout io.Writer) int {
	return runWith(depsProd(), printMode, eout)
}

func runWith(d deps, printMode bool, eout io.Writer) int {
	cfg, warn := d.load()
	if warn != "" {
		fmt.Fprintln(eout, "gitdash:", warn) // notificar sin abortar
	}

	if printMode {
		d.print(cfg)
		return 0
	}

	model := d.newModel(cfg)
	// El aviso va también a la TUI: stderr se queda detrás del alt screen.
	d.notify(model, warn)
	if err := d.runTUI(model); err != nil {
		fmt.Fprintln(eout, "gitdash:", err)
		return 1
	}
	return 0
}
