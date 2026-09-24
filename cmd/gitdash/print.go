// Modo --print: tabla one-shot sin UI, homenaje a git-statuses.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// printRow es la fila de la tabla de salida.
type printRow struct {
	name, group, branch, state, upDown, sync, wt, activity, path string
	score, lastCommit                                            int
}

// runPrint ejecuta discovery + recolección (sin fetch) e imprime la tabla.
// Sin repos imprime un mensaje y sale 0.
func runPrint(cfg config.Config) {
	projects, err := discovery.Scan(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gitdash:", err)
	}
	if len(projects) == 0 {
		fmt.Println("no repositories found")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var mu sync.Mutex
	states := map[string]gitstatus.Snapshot{}
	gitstatus.StreamPool(ctx, projects, cfg.SyncBranch, 8, func(path string, snap gitstatus.Snapshot) {
		mu.Lock()
		states[path] = snap
		mu.Unlock()
	})

	rows := make([]printRow, 0, len(projects))
	for _, p := range projects {
		// Worktree plegado bajo su repo principal descubierto
		if p.IsWorktree && p.MainRepo != "" && hasProject(projects, p.MainRepo) {
			continue
		}
		snap := states[p.Path]
		st := snap.State(p.HasRepo)
		name := p.Name
		if p.IsWorktree {
			// restos visibles solo si su repo principal no está descubierto
			name += " [wt]"
		}
		branch := snap.Status.Branch
		if snap.Status.Detached {
			branch += " (detached)"
		}
		if branch == "" {
			branch = "-"
		}
		wt := ""
		if n := len(snap.Worktrees); n > 0 {
			wt = fmt.Sprintf("%d", n)
		}
		rows = append(rows, printRow{
			name:       name,
			group:      orDashPrint(groupLabelPrint(p)),
			branch:     branch,
			state:      printState(st, snap),
			upDown:     printUpDown(st, snap),
			sync:       printSync(snap),
			activity:   relativeTimePrint(snap.LastCommit),
			path:       p.Path,
			wt:         wt,
			score:      st.Score(),
			lastCommit: int(snap.LastCommit),
		})
	}

	// mismo orden que la TUI: atención-primero, actividad, nombre
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		if rows[i].lastCommit != rows[j].lastCommit {
			return rows[i].lastCommit > rows[j].lastCommit
		}
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})

	// GROUP se conserva en print (tabla plana, sin headers de
	// grupo); WT = working tree; WTS = contador de worktrees (renombrado
	// desde WT para liberar la sigla); ↑↓up explícito.
	// La tabla va a stdout: un fallo de escritura no es accionable aquí.
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tGROUP\tBRANCH\tWT\t↑↓up\tSYNC\tWTS\tACTIVITY\tPATH")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.name, r.group, r.branch, r.state, r.upDown, r.sync, r.wt, r.activity, r.path)
	}
	_ = w.Flush()
}

// hasProject reporta si algún proyecto descubierto tiene esa ruta.
func hasProject(projects []discovery.Project, path string) bool {
	for _, p := range projects {
		if p.Path == path {
			return true
		}
	}
	return false
}

// printSync formatea la desviación vs sync branch con la rama visible:
// `<rama> ↓N`, `<rama>`, `<rama> —`, `—`.
func printSync(snap gitstatus.Snapshot) string {
	if snap.SyncBranch == "" {
		return "—"
	}
	if !snap.SyncKnown {
		return snap.SyncBranch + " —" // ref inexistente
	}
	if snap.SyncBehind == 0 {
		return snap.SyncBranch // rama visible sin tick
	}
	return fmt.Sprintf("%s ↓%d", snap.SyncBranch, snap.SyncBehind)
}

// printState compone la columna WT del modo print: solo
// working tree — counts o vacío (tabla quieta). Los estados de ciclo
// de vida viven en otras columnas: detached en BRANCH, no-up en ↑↓up.
func printState(st gitstatus.State, snap gitstatus.Snapshot) string {
	switch st {
	case gitstatus.StateError:
		return "error"
	case gitstatus.StateNoRepo:
		return "no repo"
	}
	s := snap.Status
	if s.Dirty() == 0 {
		return ""
	}
	// mismo formato que dirtyTail de la TUI: el 0 de tracked se
	// omite → solo untracked es "?1", no "0 ?1".
	var b strings.Builder
	if s.TrackedChanges > 0 {
		fmt.Fprintf(&b, "%d", s.TrackedChanges)
	}
	if s.Untracked > 0 {
		fmt.Fprintf(&b, " ?%d", s.Untracked)
	}
	return b.String()
}

// printUpDown compone ↑↓up para print: `no-up` sin upstream
// trackeado, vacío en sync; errores y no-repo vacíos.
func printUpDown(st gitstatus.State, snap gitstatus.Snapshot) string {
	if st == gitstatus.StateNoRepo || snap.Err != "" {
		return ""
	}
	s := snap.Status
	if !s.HasUpstream {
		return "no-up"
	}
	switch {
	case s.Ahead > 0 && s.Behind > 0:
		return fmt.Sprintf("↑%d↓%d", s.Ahead, s.Behind)
	case s.Ahead > 0:
		return fmt.Sprintf("↑%d", s.Ahead)
	case s.Behind > 0:
		return fmt.Sprintf("↓%d", s.Behind)
	default:
		return ""
	}
}

func relativeTimePrint(epoch int64) string {
	if epoch <= 0 {
		return "-"
	}
	d := time.Since(time.Unix(epoch, 0))
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func orDashPrint(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// groupLabelPrint compone `primary/secondary` para la columna GROUP
// solo primario si no hay secundario, "-" si ninguno.
func groupLabelPrint(p discovery.Project) string {
	switch {
	case p.PrimaryGroup != "" && p.SecondaryGroup != "":
		return p.PrimaryGroup + "/" + p.SecondaryGroup
	case p.PrimaryGroup != "":
		return p.PrimaryGroup
	default:
		return ""
	}
}
