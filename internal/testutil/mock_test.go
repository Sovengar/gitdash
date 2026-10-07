// These helpers' error branches are unreachable against a real *testing.T (t.Fatal kills the covering test), which is why they take a TB; the failure is provoked for real with a FILE where a directory should be (ENOTDIR).
package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ficheroComoPadre(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "i-am-a-file")
	if err := os.WriteFile(f, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestInitFailsWithDirectoryUnreadable(t *testing.T) {
	tb := &MockTB{Temp: tptr(t)}
	Init(tb, filepath.Join(ficheroComoPadre(t), "sub"))

	if !tb.Failed() {
		t.Fatal("Init without failure, want the MkdirAll error (the parent is a file)")
	}
	if !strings.Contains(tb.Failures[0], "not a directory") {
		t.Errorf("failure = %q, want the ENOTDIR one", tb.Failures[0])
	}
	if entries, err := os.ReadDir(filepath.Dir(tb.Failures[0])); err == nil && len(entries) == 0 {
		t.Error("Init created the directory even though mkdir failed")
	}
}

func TestCommitFilesFailsWithPathImpossible(t *testing.T) {
	dir := t.TempDir()

	// A nested file whose parent directory is a FILE: without this block MkdirAll would create it and the helper would pass the mkdir to fail much later, in the `git commit` of a directory that is not a repo. The failure would still be "a failure", just not this line's, and the test would accept a helper that did not even check the mkdir.
	block := filepath.Join(dir, "block")
	if err := os.WriteFile(block, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tb2 := &MockTB{Temp: tptr(t)}
	CommitFiles(tb2, dir, map[string]string{
		filepath.Join("block", "child.txt"): "x",
	}, "should never get here")
	if !tb2.Failed() {
		t.Error("CommitFiles without failure, want the parent's MkdirAll error")
	} else if !strings.Contains(tb2.Failures[0], "mkdir ") || !strings.Contains(tb2.Failures[0], "not a directory") {
		// The FIRST failure has to be the mkdir's ENOTDIR: MockTB does not stop the helper (that is what it is for, recording instead of killing), so the WriteFile and git fail afterwards too; what is checked is that the mkdir really failed and not that it was the first one recorded.
		t.Errorf("first failure = %q, want MkdirAll's ENOTDIR", tb2.Failures[0])
	}

	tb3 := &MockTB{Temp: tptr(t)}
	Init(tb3, dir)
	if tb3.Failed() {
		t.Fatalf("the base repo could not be created: %v", tb3.Failures)
	}
	destination := filepath.Join(dir, "sub", "file.txt")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	tb4 := &MockTB{Temp: tptr(t)}
	CommitFiles(tb4, dir, map[string]string{"sub/file.txt": "content"}, "should never get here")
	if !tb4.Failed() {
		t.Error("CommitFiles over a directory without failure, want the WriteFile error")
	}
}

func TestWriteUncommittedAndUntrackedFail(t *testing.T) {
	t.Run("uncommitted", func(t *testing.T) {
		tb := &MockTB{Temp: tptr(t)}
		WriteUncommitted(tb, ficheroComoPadre(t), map[string]string{"a.txt": "x"})
		if !tb.Failed() {
			t.Error("WriteUncommitted without failure, want the write error")
		}
	})
	t.Run("untracked", func(t *testing.T) {
		tb := &MockTB{Temp: tptr(t)}
		WriteUntracked(tb, ficheroComoPadre(t), map[string]string{"a.txt": "x"})
		if !tb.Failed() {
			t.Error("WriteUntracked without failure, want the write error")
		}
	})
}

func TestMarkerFailsWithPathUnreadable(t *testing.T) {
	tb := &MockTB{Temp: tptr(t)}
	Marker(tb, ficheroComoPadre(t), "n", "g", "s", false)
	if !tb.Failed() {
		t.Error("Marker without failure, want the marker's write error")
	}
}

// git() fails when the command does not exist or returns an error, provoked with an argument git does not understand: this is not a system mock, it is git saying no.
func TestGitFailsWithCommandInvalid(t *testing.T) {
	tb := &MockTB{Temp: tptr(t)}
	git(tb, t.TempDir(), "this-subcommand-does-not-exist")
	if !tb.Failed() {
		t.Fatal("git with an invalid subcommand without failure, want the process error")
	}
	if !strings.Contains(tb.Failures[0], "this-subcommand-does-not-exist") {
		t.Errorf("failure = %q, want it to name the command that ran", tb.Failures[0])
	}
}

func tptr(t *testing.T) testing.TB { return t }

// The double has to answer the three things TB requires, and two of them are not exercised when using it to cover a failure: TempDir without Temp (the guard) and Failed() on a clean double (which must say no).
func TestMockTBRespectsTheContract(t *testing.T) {
	empty := &MockTB{}
	if got := empty.TempDir(); got != "" {
		t.Errorf("TempDir without Temp = %q, want empty", got)
	}
	if empty.Failed() {
		t.Error("a freshly created double has failures, want none")
	}

	empty.Fatalf("failure %d of %d", 3, 7)
	if len(empty.Failures) != 1 {
		t.Fatalf("fallos = %d, want 1", len(empty.Failures))
	}
	if !strings.Contains(empty.Failures[0], "failure 3 de 7") {
		t.Errorf("failure = %q, want the formatted message", empty.Failures[0])
	}
	if !empty.Failed() {
		t.Error("Failed() = false after a Fatalf, want true")
	}

	empty.Fatal("another failure")
	if len(empty.Failures) != 2 {
		t.Errorf("failures = %d, want 2 (Fatal does not overwrite the previous one)", len(empty.Failures))
	}
}

func TestHelpersThatWriteFail(t *testing.T) {
	t.Run("InitBare", func(t *testing.T) {
		tb := &MockTB{Temp: tptr(t)}
		InitBare(tb, filepath.Join(ficheroComoPadre(t), "origin.git"))
		if !tb.Failed() {
			t.Error("InitBare without failure, want the MkdirAll error")
		}
	})

	t.Run("BreakGit", func(t *testing.T) {
		dir := t.TempDir()
		tb := &MockTB{Temp: tptr(t)}
		Init(tb, dir)
		if tb.Failed() {
			t.Fatalf("the base repo could not be created: %v", tb.Failures)
		}
		head := filepath.Join(dir, ".git", "HEAD")
		if err := os.Remove(head); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(head, 0o755); err != nil {
			t.Fatal(err)
		}
		tb2 := &MockTB{Temp: tptr(t)}
		BreakGit(tb2, dir)
		if !tb2.Failed() {
			t.Error("BreakGit over a HEAD that is a directory without failure, want the error")
		}
	})
}

// TempDir without Temp is the only MockTB path the helpers of this package do not walk (all of them call TempDir on a double with Temp set); it is tested apart because it is the double's contract and not the helper's: a MockTB built by hand (with no test to delegate to) must return "" instead of panicking on a nil dereference.
func TestMockTBTempDirWithoutTest(t *testing.T) {
	empty := &MockTB{}
	if got := empty.TempDir(); got != "" {
		t.Errorf("TempDir without Temp = %q, want the empty string", got)
	}
	conTest := &MockTB{Temp: tptr(t)}
	got := conTest.TempDir()
	if got == "" {
		t.Error("TempDir with Temp empty, want a real temp directory")
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("the delegated directory does not exist: %v", err)
	}
}
