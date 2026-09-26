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
	"os"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/config"
	"gitdash/internal/tui"
)

func main() {
	printMode := flag.Bool("print", false, "print the status table and exit")
	flag.Parse()

	cfg, warn := config.Load()
	if warn != "" {
		fmt.Fprintln(os.Stderr, "gitdash:", warn) // notificar sin abortar
	}

	if *printMode {
		runPrint(cfg)
		return
	}

	model := tui.New(cfg)
	// El aviso va también a la TUI: stderr se queda detrás del alt screen.
	model.NotifyConfig(warn)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gitdash:", err)
		os.Exit(1)
	}
}
