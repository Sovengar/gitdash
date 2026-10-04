// gitdash — TUI git status dashboard for the repos discovered by marker file (.gitdash.toml), inspired by bircni/git-statuses.
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

// The painted model is a seam, not the real TUI: what is testable is the decision (stderr warning, print vs TUI, exit code), and notify stays a separate seam so main can verify the warning without tui exporting a getter.
type deps struct {
	load     func() (config.Config, string)
	print    func(config.Config)
	newModel func(config.Config) tui.Model
	notify   func(tui.Model, string)
	runTUI   func(tui.Model) error
}

func depsProd() deps {
	return deps{
		load:     config.Load,
		print:    runPrint,
		newModel: tui.New,
		notify:   func(m tui.Model, warn string) { m.NotifyConfig(warn) },
		runTUI:   func(m tui.Model) error { _, err := tea.NewProgram(m).Run(); return err },
	}
}

// The body of main: it decides what to paint and returns the exit code without leaving the process, which is what makes both guards checkable with doubles.
func run(printMode bool, eout io.Writer) int {
	return runWith(depsProd(), printMode, eout)
}

func runWith(d deps, printMode bool, eout io.Writer) int {
	cfg, warn := d.load()
	if warn != "" {
		// Write error dropped on purpose: eout is stderr and a problem is already being reported; if stderr fails there is nobody left to tell, and aborting would leave the user with no dashboard.
		_, _ = fmt.Fprintln(eout, "gitdash:", warn)
	}

	if printMode {
		d.print(cfg)
		return 0
	}

	model := d.newModel(cfg)
	// The warning also goes to the TUI, because stderr stays behind the alt screen.
	d.notify(model, warn)
	if err := d.runTUI(model); err != nil {
		_, _ = fmt.Fprintln(eout, "gitdash:", err)
		return 1
	}
	return 0
}
