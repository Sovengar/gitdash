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

type printRow struct {
	name, group, branch, state, upDown, sync, wt, activity, path string
	score, lastCommit                                            int
}

// Split out because this mapping holds the decisions the table shows ([wt] suffix, "(detached)", the "-" for an empty branch, the worktree count) and each one can be checked on fabricated rows instead of on real repos in detached HEAD, which cannot be provoked at will.
func printRowOf(p discovery.Project, snap gitstatus.Snapshot) printRow {
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
	wt := ""
	if n := len(snap.Worktrees); n > 0 {
		wt = fmt.Sprintf("%d", n)
	}
	return printRow{
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
	}
}

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
	gitstatus.StreamPool(ctx, projects, cfg.SyncBranch, cfg.SyncBranchExplicit, 8, func(path string, snap gitstatus.Snapshot) {
		mu.Lock()
		states[path] = snap
		mu.Unlock()
	})

	rows := make([]printRow, 0, len(projects))
	for _, p := range projects {
		if p.IsWorktree && p.MainRepo != "" && hasProject(projects, p.MainRepo) {
			continue
		}
		rows = append(rows, printRowOf(p, states[p.Path]))
	}

	// Same order as the TUI: attention first, then activity, then name.
	sortPrintRows(rows)

	// Column order mirrors the TUI, so both read the same.
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tGROUP\tBRANCH\tWT\t↑↓up\tSYNC\tWTS\tACTIVITY\tPATH")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.name, r.group, r.branch, r.state, r.upDown, r.sync, r.wt, r.activity, r.path)
	}
	_ = w.Flush()
}

// Split out because it is the only way to test the order with fabricated rows: real repos land their commits in the same second, so the date tiebreak cannot be fixed.
func sortPrintRows(rows []printRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		if rows[i].lastCommit != rows[j].lastCommit {
			return rows[i].lastCommit > rows[j].lastCommit
		}
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})
}

func hasProject(projects []discovery.Project, path string) bool {
	for _, p := range projects {
		if p.Path == path {
			return true
		}
	}
	return false
}

func printSync(snap gitstatus.Snapshot) string {
	if snap.SyncBranch == "" {
		return "—"
	}
	if !snap.SyncKnown {
		return snap.SyncBranch + " —"
	}
	if snap.SyncBehind == 0 {
		return snap.SyncBranch
	}
	return fmt.Sprintf("%s ↓%d", snap.SyncBranch, snap.SyncBehind)
}

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
	// Same format as the TUI's dirtyTail: the 0 of tracked is omitted and the separator only appears when something precedes it, so untracked-only is "?1" and not "0 ?1" (the leading space misaligns the cell and looks like a phantom number).
	var b strings.Builder
	if s.TrackedChanges > 0 {
		fmt.Fprintf(&b, "%d", s.TrackedChanges)
	}
	if s.Untracked > 0 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "?%d", s.Untracked)
	}
	return b.String()
}

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
