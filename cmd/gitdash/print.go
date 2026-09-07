// Modo --print: tabla one-shot sin UI (R12), homenaje a git-statuses.
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
	name, group, branch, state, upDown, activity, path string
	score, lastCommit                                  int
}

// runPrint ejecuta discovery + recolección (sin fetch) e imprime la tabla
// (S12.1). Sin repos imprime un mensaje y sale 0 (S12.2).
func runPrint(cfg config.Config) {
	projects, err := discovery.Scan(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gitdash:", err)
	}
	if len(projects) == 0 {
		fmt.Println("no repositories found") // S12.2
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var mu sync.Mutex
	states := map[string]gitstatus.Snapshot{}
	gitstatus.StreamPool(ctx, projects, 8, func(path string, snap gitstatus.Snapshot) {
		mu.Lock()
		states[path] = snap
		mu.Unlock()
	})

	rows := make([]printRow, 0, len(projects))
	for _, p := range projects {
		snap := states[p.Path]
		st := snap.State(p.HasRepo)
		name := p.Name
		if p.IsWorktree {
			name += " [wt]"
		}
		branch := snap.Status.Branch
		if snap.Status.Detached {
			branch += " (detached)"
		}
		if branch == "" {
			branch = "-"
		}
		rows = append(rows, printRow{
			name:       name,
			group:      orDashPrint(p.Group),
			branch:     branch,
			state:      printState(st, snap),
			upDown:     printUpDown(st, snap),
			activity:   relativeTimePrint(snap.LastCommit),
			path:       p.Path,
			score:      st.Score(),
			lastCommit: int(snap.LastCommit),
		})
	}

	// mismo orden que la TUI: atención-primero, actividad, nombre (R6)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		if rows[i].lastCommit != rows[j].lastCommit {
			return rows[i].lastCommit > rows[j].lastCommit
		}
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tGROUP\tBRANCH\tSTATE\t↑↓\tACTIVITY\tPATH")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.name, r.group, r.branch, r.state, r.upDown, r.activity, r.path)
	}
	w.Flush()
}

func printState(st gitstatus.State, snap gitstatus.Snapshot) string {
	switch st {
	case gitstatus.StateDirty:
		s := snap.Status
		out := fmt.Sprintf("dirty (%d", s.TrackedChanges)
		if s.Untracked > 0 {
			out += fmt.Sprintf(" ?%d", s.Untracked)
		}
		return out + ")"
	case gitstatus.StateDiverged:
		return "diverged"
	case gitstatus.StateClean:
		return "clean"
	default:
		return st.String()
	}
}

func printUpDown(st gitstatus.State, snap gitstatus.Snapshot) string {
	s := snap.Status
	if !s.HasUpstream {
		return "—"
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
