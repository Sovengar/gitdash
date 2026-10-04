package cmdlog

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// The ring is written from many goroutines at once (git execs run in parallel: pool of 8 in the scan, 4 per fetch batch), so this is the test that guarantees under -race that the ring does not get corrupted.
func TestRecorderConcurrent(t *testing.T) {
	const n = 200
	rec := New(n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec.addExec(Entry{Action: "pull", Argv: []string{"git", "pull"}, Exit: i % 2})
		}()
	}
	wg.Wait()

	entries := rec.Entries()
	if len(entries) != n {
		t.Fatalf("entries = %d, want %d", len(entries), n)
	}
	seen := make(map[int]bool, n)
	for i, e := range entries {
		if e.Seq != i+1 {
			t.Errorf("entries[%d].Seq = %d, want %d", i, e.Seq, i+1)
		}
		if seen[e.Seq] {
			t.Errorf("Seq %d repetida", e.Seq)
		}
		seen[e.Seq] = true
	}
}

func TestRingStepsItOld(t *testing.T) {
	rec := New(3)
	for i := range 5 {
		rec.addExec(Entry{Action: fmt.Sprintf("a%d", i)})
	}
	entries := rec.Entries()
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (the ring's capacity)", len(entries))
	}
	for i, want := range []string{"a2", "a3", "a4"} {
		if entries[i].Action != want {
			t.Errorf("entries[%d] = %q, want %q", i, entries[i].Action, want)
		}
	}
	if entries[0].Seq != 3 {
		t.Errorf("Seq de la primera viva = %d, want 3", entries[0].Seq)
	}
}

func TestNewCapacityForBug(t *testing.T) {
	rec := New(0)
	rec.addExec(Entry{Action: "pull"})
	if got := len(rec.Entries()); got != 1 {
		t.Fatalf("entries = %d, want 1", got)
	}
}

func TestWithoutRecorderGlobalNotBreaks(t *testing.T) {
	SetRecorder(nil)
	if Active() != nil {
		t.Fatal("Active() should be nil after SetRecorder(nil)")
	}
	RecordExec(Entry{Action: "pull", Argv: []string{"git", "pull"}})
	RecordIntent(Entry{Action: "pull", Key: "p"})
	if got := Entries(); got != nil {
		t.Errorf("Entries() = %v, want nil with no recorder", got)
	}
	if got := LastSeq(); got != 0 {
		t.Errorf("LastSeq() = %d, want 0 with no recorder", got)
	}
}

// SetRecorder(nil) is called from t.Cleanup as soon as a test touches the global, so another test of the package running afterwards must not inherit the recorder.
func TestSetRecorderIsRestorable(t *testing.T) {
	rec := New(4)
	SetRecorder(rec)
	t.Cleanup(func() { SetRecorder(nil) })
	RecordExec(Entry{Action: "push", Argv: []string{"git", "push"}})
	if got := LastSeq(); got != 1 {
		t.Fatalf("LastSeq() = %d, want 1", got)
	}
	if Active() != rec {
		t.Error("Active() does not return the installed recorder")
	}
}

func TestIntentNotHasVerdict(t *testing.T) {
	rec := New(4)
	rec.addIntent(Entry{Action: "pull", Key: "p"})
	e := rec.Entries()[0]
	if !e.Intent {
		t.Error("Intent = false, want true")
	}
	if e.Exit != -1 {
		t.Errorf("Exit = %d, want -1 (nada ejecutado)", e.Exit)
	}
	if e.At.IsZero() {
		t.Error("At left empty: the recorder must timestamp the entry")
	}
}

func TestExecKeepsExitZero(t *testing.T) {
	rec := New(4)
	rec.addExec(Entry{Action: "pull", Argv: []string{"git", "pull"}, Exit: 0, Outcome: "rebase"})
	e := rec.Entries()[0]
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Intent {
		t.Error("Intent = true in an execution")
	}
}

func TestCommandRendersArgv(t *testing.T) {
	e := Entry{Argv: []string{"git", "pull", "--rebase", "--autostash"}}
	if got, want := e.Command(), "git pull --rebase --autostash"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
	if got := (Entry{}).Command(); got != "" {
		t.Errorf("Command() of an entry with no argv = %q, want %q", got, "")
	}
}

func TestClassString(t *testing.T) {
	for _, tc := range []struct {
		class Class
		want  string
	}{
		{ClassRead, "read"},
		{ClassAction, "action"},
		{ClassAuto, "auto"},
		{Class(99), "read"}, // unknown value falls to read, not to empty
	} {
		if got := tc.class.String(); got != tc.want {
			t.Errorf("Class(%d).String() = %q, want %q", tc.class, got, tc.want)
		}
	}
}

// The entry's At is set by the recorder and not by the caller: the timestamp must be the moment of recording, not of building the struct.
func TestAddIgnoresAtZero(t *testing.T) {
	rec := New(2)
	antes := time.Now()
	rec.addExec(Entry{Action: "pull"})
	got := rec.Entries()[0].At
	if got.Before(antes) {
		t.Errorf("At = %v, earlier than the moment it was recorded", got)
	}
}
