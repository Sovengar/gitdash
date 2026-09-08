// Parsing puro de la salida de git — sin I/O, testeable con salidas canned
// (spec 0001 R5).
package gitstatus

import (
	"fmt"
	"strconv"
	"strings"
)

// Status es el estado git derivado de `git status --porcelain=v2 --branch`.
type Status struct {
	Branch         string // nombre de rama o sha corto si detached
	Detached       bool   // HEAD detached (S5.5)
	Upstream       string // p. ej. "origin/main"
	HasUpstream    bool   // hay upstream trackeado (S5.4)
	OID            string // sha completo de HEAD (branch.oid)
	Ahead          int    // commits locales sin subir (↑)
	Behind         int    // commits del upstream sin bajar (↓)
	TrackedChanges int    // ficheros trackeados modificados (M/A/D/R/u)
	Untracked      int    // ficheros sin trackear (?)
}

// Dirty es el total de cambios pendientes en el working tree.
func (s Status) Dirty() int { return s.TrackedChanges + s.Untracked }

// HasPending reporta si hay cambios que subir o bajar.
func (s Status) HasPending() bool { return s.Ahead > 0 || s.Behind > 0 }

// FileEntry es un fichero cambiado con su código porcelain (S10.1).
type FileEntry struct {
	Code string // "M ", "A ", "D ", "R ", "MM", "??"-estilo: dos chars
	Path string
}

// Commit es un commit reciente para el detalle (S10.1).
type Commit struct {
	Sha     string
	When    int64 // epoch
	Subject string
}

// State es el estado derivado del repo para UI/orden/filtros (S5).
type State int

const (
	StateClean State = iota
	StateNoUpstream
	StateDetached
	StateBehind
	StateAhead
	StateDirty
	StateDiverged
	StateNoRepo
	StateError
)

// String devuelve la etiqueta del estado para UI/modo print.
func (st State) String() string {
	switch st {
	case StateClean:
		return "clean"
	case StateNoUpstream:
		return "no upstream"
	case StateDetached:
		return "detached"
	case StateBehind:
		return "behind"
	case StateAhead:
		return "ahead"
	case StateDirty:
		return "dirty"
	case StateDiverged:
		return "diverged"
	case StateNoRepo:
		return "no repo"
	default:
		return "error"
	}
}

// Derive deduce el estado del repo según precedencia: diverged > dirty >
// ahead > behind > detached > no-upstream > clean. Dirty es independiente
// del upstream: un repo dirty sin upstream se reporta dirty.
func (s Status) Derive() State {
	switch {
	case s.Ahead > 0 && s.Behind > 0:
		return StateDiverged // S5.2
	case s.Dirty() > 0:
		return StateDirty // S5.3
	case s.Ahead > 0:
		return StateAhead // S5.2
	case s.Behind > 0:
		return StateBehind // S5.2
	case s.Detached:
		return StateDetached // S5.5
	case !s.HasUpstream:
		return StateNoUpstream // S5.4
	default:
		return StateClean // S5.1
	}
}

// Score es la prioridad de atención para el orden del panel y el modo
// print (R6): error > diverged > dirty > ahead/behind > info > clean.
func (st State) Score() int {
	switch st {
	case StateError:
		return 6
	case StateDiverged:
		return 5
	case StateDirty:
		return 4
	case StateAhead, StateBehind:
		return 3
	case StateNoUpstream, StateDetached, StateNoRepo:
		return 2
	default:
		return 0
	}
}

// ParsePorcelain interpreta la salida completa de
// `git status --porcelain=v2 --branch` y devuelve el status más los
// ficheros cambiados (hasta maxFiles). Ignora silenciosamente líneas
// desconocidas para tolerar versiones futuras de git.
func ParsePorcelain(out string) (Status, []FileEntry) {
	var st Status
	var files []FileEntry

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			val := strings.TrimPrefix(line, "# branch.head ")
			switch {
			case val == "(detached)":
				st.Detached = true // S5.5
			case strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")"):
				// rama no nacida: "# branch.head (main)" en repos sin commits
				st.Branch = strings.Trim(val, "()")
			default:
				st.Branch = val
			}
		case strings.HasPrefix(line, "# branch.upstream "):
			st.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
			st.HasUpstream = true
		case strings.HasPrefix(line, "# branch.oid "):
			st.OID = strings.TrimPrefix(line, "# branch.oid ")
		case strings.HasPrefix(line, "# branch.ab "):
			st.Ahead, st.Behind = parseAB(strings.TrimPrefix(line, "# branch.ab "))
		case strings.HasPrefix(line, "1 "):
			st.TrackedChanges++ // S5.3
			if f, ok := parseEntry(line[2:], 7); ok {
				files = appendFile(files, f)
			}
		case strings.HasPrefix(line, "2 "):
			st.TrackedChanges++ // renames también cuentan
			if f, ok := parseEntry(line[2:], 8); ok {
				f.Path, _, _ = strings.Cut(f.Path, "\t")
				files = appendFile(files, f)
			}
		case strings.HasPrefix(line, "u "):
			st.TrackedChanges++ // unmerged
			if f, ok := parseEntry(line[2:], 9); ok {
				files = appendFile(files, f)
			}
		case strings.HasPrefix(line, "? "):
			st.Untracked++ // S5.3
			files = appendFile(files, FileEntry{Code: "??", Path: line[2:]})
		}
		// "#" restantes (branch.oid), "!" (ignored) y "": ignorados
	}
	return st, files
}

// appendFile añade una entrada respetando el tope maxFiles.
func appendFile(files []FileEntry, f FileEntry) []FileEntry {
	if len(files) >= maxFiles {
		return files
	}
	return append(files, f)
}

// maxFiles es el tope de ficheros mostrados en el detalle.
const maxFiles = 100

// parseEntry extrae código (XY) y ruta de una línea de entrada v2.
// fieldsBeforePath es el número de campos fijos entre el código y la ruta
// ("1 ": 7 — sub mH mI mW hH hI; "2 ": 8 — añade Xscore; "u ": 9).
// La ruta es todo lo restante, espacios incluidos.
func parseEntry(body string, fieldsBeforePath int) (FileEntry, bool) {
	fields := strings.SplitN(body, " ", fieldsBeforePath+1)
	if len(fields) < fieldsBeforePath+1 {
		return FileEntry{}, false
	}
	return FileEntry{
		Code: fields[0],
		Path: fields[fieldsBeforePath],
	}, true
}

// parseAB interpreta "+2 -3" de la línea branch.ab (S5.2).
func parseAB(s string) (ahead, behind int) {
	var sign byte
	_, err := fmt.Sscanf(s, "%c%d %c%d", &sign, &ahead, &sign, &behind)
	if err != nil {
		return 0, 0
	}
	return ahead, behind
}

// ParseLog interpreta la salida de git log con formato %h<NUL>%ct<NUL>%s.
func ParseLog(out string) []Commit {
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		when, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		commits = append(commits, Commit{
			Sha:     parts[0],
			When:    when,
			Subject: parts[2],
		})
	}
	return commits
}

// ParseWorktrees interpreta `git worktree list --porcelain` y devuelve los
// worktrees aparte del repo principal (mainPath) (0002 R15). Bloques
// separados por línea en blanco; claves: worktree, HEAD, branch/bare/
// detached. Líneas desconocidas/prunable se ignoran (tolerancia git futuro).
func ParseWorktrees(out, mainPath string) []Worktree {
	var out2 []Worktree
	var cur Worktree
	flush := func() {
		if cur.Path != "" && strings.TrimSpace(cur.Path) != strings.TrimSpace(mainPath) {
			out2 = append(out2, cur)
		}
		cur = Worktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush() // los bloques pueden no venir separados por línea vacía
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			sha := strings.TrimPrefix(line, "HEAD ")
			if len(sha) >= 7 {
				sha = sha[:7] // sha corto para UI compacta
			}
			cur.Head = sha
		case strings.HasPrefix(line, "branch "):
			// refs/heads/feat → feat
			ref := strings.TrimPrefix(line, "branch ")
			cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	flush()
	return out2
}
