// Pure parsing of git output: no I/O, so it is testable with canned output.
package gitstatus

import (
	"fmt"
	"strconv"
	"strings"
)

type Status struct {
	Branch         string
	Detached       bool
	Upstream       string
	HasUpstream    bool
	OID            string
	Ahead          int
	Behind         int
	TrackedChanges int
	Untracked      int
}

func (s Status) Dirty() int { return s.TrackedChanges + s.Untracked }

type FileEntry struct {
	Code string
	Path string
}

type Commit struct {
	Sha     string
	When    int64
	Subject string
}

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

func (s Status) Derive() State {
	// if-chain instead of switch: precedence stays explicit and every condition is measurable for coverage.
	if s.Ahead > 0 && s.Behind > 0 {
		return StateDiverged
	}
	if s.Dirty() > 0 {
		return StateDirty
	}
	if s.Ahead > 0 {
		return StateAhead
	}
	if s.Behind > 0 {
		return StateBehind
	}
	if s.Detached {
		return StateDetached
	}
	if !s.HasUpstream {
		return StateNoUpstream
	}
	return StateClean
}

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

// Unknown lines are ignored on purpose so a future git version cannot break parsing.
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
				st.Detached = true
			case strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")"):
				// Unborn branch: "# branch.head (main)" in a repo with no commits.
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
			st.TrackedChanges++
			if f, ok := parseEntry(line[2:], 7); ok {
				files = appendFile(files, f)
			}
		case strings.HasPrefix(line, "2 "):
			st.TrackedChanges++
			if f, ok := parseEntry(line[2:], 8); ok {
				f.Path, _, _ = strings.Cut(f.Path, "\t")
				files = appendFile(files, f)
			}
		case strings.HasPrefix(line, "u "):
			st.TrackedChanges++
			if f, ok := parseEntry(line[2:], 9); ok {
				files = appendFile(files, f)
			}
		case strings.HasPrefix(line, "? "):
			st.Untracked++
			files = appendFile(files, FileEntry{Code: "??", Path: line[2:]})
		}
	}
	return st, files
}

func appendFile(files []FileEntry, f FileEntry) []FileEntry {
	if len(files) >= maxFiles {
		return files
	}
	return append(files, f)
}

const maxFiles = 100

// The path is everything after the fixed fields ("1 "=7, "2 "=8, "u "=9), spaces included.
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

func parseAB(s string) (ahead, behind int) {
	var sign byte
	_, err := fmt.Sscanf(s, "%c%d %c%d", &sign, &ahead, &sign, &behind)
	if err != nil {
		return 0, 0
	}
	return ahead, behind
}

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
			flush() // blocks may not be separated by a blank line
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			sha := strings.TrimPrefix(line, "HEAD ")
			// Clamp instead of a length guard: truncating to 7 is a no-op when the sha is already 7 or shorter.
			cur.Head = sha[:min(len(sha), 7)]
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	flush()
	return out2
}
