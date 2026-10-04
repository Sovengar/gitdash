// Package cmdlog audits what really ran: with the pull policy in the gitconfig the argv is not enough (a bare git pull may merge, rebase or rebase with autostash), and the recorder is a global no-op because the exec points live in two packages.
package cmdlog

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultCap = 500

// Class tags an entry by noise: the scan issues about four reads per repo (status, log, worktree list, rev-list), so they are hidden by default.
type Class int

const (
	ClassRead Class = iota
	ClassAction
	ClassAuto
)

func (c Class) String() string {
	switch c {
	case ClassAction:
		return "action"
	case ClassAuto:
		return "auto"
	default:
		return "read"
	}
}

// Intents and executions are both stored because the intent is the only thing telling "I pressed p and picked rebase" from "the gitconfig decided".
type Entry struct {
	Seq int
	At  time.Time

	Intent bool

	Class Class
	// Repo is the directory basename, not the path: the panel is read next to the table, which already shows names.
	Repo   string
	Dir    string
	Key    string
	Action string
	Argv   []string
	Exit   int
	// Dur is 0 for intents and terminal handoffs: bubbletea lends the terminal to the child, and measuring it would mean storing the start instant in the model.
	Dur     time.Duration
	Outcome string
}

func (e Entry) Command() string { return strings.Join(e.Argv, " ") }

// Safe for concurrent use: the scan runs git in a pool and the ring is written from all of those goroutines.
type Recorder struct {
	mu    sync.Mutex
	buf   []Entry
	next  int
	count int
	seq   int
}

func New(cap int) *Recorder {
	if cap <= 0 {
		cap = DefaultCap
	}
	return &Recorder{buf: make([]Entry, cap)}
}

func (r *Recorder) add(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq = r.seq
	r.buf[r.next] = e
	r.next = (r.next + 1) % len(r.buf)
	if r.count < len(r.buf) {
		r.count++
	}
}

func (r *Recorder) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, r.count)
	start := (r.next - r.count + len(r.buf)) % len(r.buf)
	for i := range r.count {
		out = append(out, r.buf[(start+i)%len(r.buf)])
	}
	return out
}

// The panel uses this to know its cached copy is stale without re-copying the ring on every frame.
func (r *Recorder) LastSeq() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq
}

// Atomic pointer instead of a mutex: the write sits on every exec's hot path, and nil means logging off (the default outside the TUI).
var current atomic.Pointer[Recorder]

func SetRecorder(r *Recorder) { current.Store(r) }

func Active() *Recorder { return current.Load() }

func (r *Recorder) addExec(e Entry) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	r.add(e)
}

func (r *Recorder) addIntent(e Entry) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	e.Intent = true
	e.Exit = -1
	r.add(e)
}

func RecordExec(e Entry) {
	if r := current.Load(); r != nil {
		r.addExec(e)
	}
}

func RecordIntent(e Entry) {
	if r := current.Load(); r != nil {
		r.addIntent(e)
	}
}

func Entries() []Entry {
	if r := current.Load(); r != nil {
		return r.Entries()
	}
	return nil
}

func LastSeq() int {
	if r := current.Load(); r != nil {
		return r.LastSeq()
	}
	return 0
}
