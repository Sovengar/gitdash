package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadCollapsed(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	groups := map[string]bool{
		"backend":          true,
		"vsocial/backend":  false,
		"vsocial/frontend": true,
	}

	if err := store.SaveCollapsed(groups); err != nil {
		t.Fatalf("SaveCollapsed: %v", err)
	}

	loaded := store.LoadCollapsed()
	if loaded == nil {
		t.Fatal("LoadCollapsed returned nil")
	}
	if len(loaded) != 3 {
		t.Errorf("len(loaded) = %d, want 3", len(loaded))
	}
	for k, v := range groups {
		if loaded[k] != v {
			t.Errorf("loaded[%q] = %v, want %v", k, loaded[k], v)
		}
	}
}

func TestLoadCollapsedMissingFile(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	loaded := store.LoadCollapsed()
	if loaded != nil {
		t.Errorf("LoadCollapsed on missing file = %v, want nil", loaded)
	}
}

func TestLoadCollapsedCorruptFile(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreAt(dir)

	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{corrupt json"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := store.LoadCollapsed()
	if loaded != nil {
		t.Errorf("LoadCollapsed on corrupt file = %v, want nil", loaded)
	}
}

func TestSaveCollapsedAtomic(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	groups := map[string]bool{"backend": true}
	if err := store.SaveCollapsed(groups); err != nil {
		t.Fatal(err)
	}
	// Verify no .tmp file left after successful save
	if _, err := os.Stat(store.CollapsedFile() + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file left after SaveCollapsed")
	}
	// Overwrite with new state
	groups["frontend"] = false
	if err := store.SaveCollapsed(groups); err != nil {
		t.Fatal(err)
	}

	loaded := store.LoadCollapsed()
	if loaded["frontend"] != false {
		t.Errorf("loaded[frontend] = %v, want false", loaded["frontend"])
	}
}

func TestLoadCollapsedEmptyMap(t *testing.T) {
	store := NewStoreAt(t.TempDir())

	path := store.CollapsedFile()
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := store.LoadCollapsed()
	if loaded == nil {
		t.Fatal("LoadCollapsed on empty JSON returned nil")
	}
	if len(loaded) != 0 {
		t.Errorf("len(loaded) = %d, want 0", len(loaded))
	}
}

func TestCompositeKeysNotCollision(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	// Two secondary groups with same name "backend" under different primaries
	groups := map[string]bool{
		"alfa/backend": true,
		"beta/backend": false,
	}

	if err := store.SaveCollapsed(groups); err != nil {
		t.Fatal(err)
	}

	loaded := store.LoadCollapsed()
	if loaded["alfa/backend"] != true {
		t.Errorf("loaded[alfa/backend] = %v, want true", loaded["alfa/backend"])
	}
	if loaded["beta/backend"] != false {
		t.Errorf("loaded[beta/backend] = %v, want false", loaded["beta/backend"])
	}
}

func TestWorktreeNamespaceCoexists(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	combined := map[string]bool{
		"backend":                     true,  // collapsed group
		WorktreePrefix + "/tmp/multi": true,  // expanded worktree
		WorktreePrefix + "/tmp/other": false, // expanded and then folded
	}
	if err := store.SaveCollapsed(combined); err != nil {
		t.Fatal(err)
	}
	loaded := store.LoadCollapsed()
	if !loaded["backend"] {
		t.Error("group key lost")
	}
	if !loaded[WorktreePrefix+"/tmp/multi"] {
		t.Error("expansion key lost")
	}
	if loaded[WorktreePrefix+"/tmp/other"] {
		t.Error("inverted polarity persisted wrong")
	}
	if _, ok := loaded["/tmp/multi"]; ok {
		t.Error("the key was stored without namespace")
	}
}

// Every test here uses NewStoreAt(t.TempDir()), so DefaultBaseDir was untested: it decides where collapsed.json lands, and the wrong branch writes the state where the app will not read it, with no error for the user.
func TestDefaultBaseDirResolvesWhereLivesTheState(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()

	t.Run("XDG wins over HOME", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", xdg)
		t.Setenv("HOME", home)
		got, err := DefaultBaseDir()
		if err != nil {
			t.Fatalf("DefaultBaseDir: %v", err)
		}
		want := filepath.Join(xdg, DirName)
		if got != want {
			t.Errorf("base = %q, want %q (XDG rules)", got, want)
		}
	})

	t.Run("without XDG the state lives under home", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)
		got, err := DefaultBaseDir()
		if err != nil {
			t.Fatalf("DefaultBaseDir: %v", err)
		}
		want := filepath.Join(home, ".local", "state", DirName)
		if got != want {
			t.Errorf("base = %q, want %q", got, want)
		}
	})

	t.Run("an empty XDG counts as unset", func(t *testing.T) {
		// XDG_STATE_HOME="" is NOT a directory: the spec treats it as undefined, and accepting it would put the state in /gitdash, at the root.
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)
		got, err := DefaultBaseDir()
		if err != nil {
			t.Fatalf("DefaultBaseDir: %v", err)
		}
		if !strings.HasPrefix(got, home) {
			t.Errorf("base = %q, want something under %q: an empty XDG is not a directory", got, home)
		}
	})

	t.Run("without home there is no base and it says so", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", "")
		if _, err := DefaultBaseDir(); err == nil {
			t.Error("DefaultBaseDir without HOME nor XDG returned a directory: the state would be written somewhere unpredictable")
		}
	})
}

func TestNewStorePropagatesTheErrorOfBase(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	store, err := NewStore()
	if err == nil {
		t.Fatalf("NewStore without home returned a store at %q", store.Base())
	}
	if store != nil {
		t.Errorf("with an error, NewStore returned store = %v, want nil", store)
	}
}

func TestNewStoreUsesTheSameBase(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if want := filepath.Join(xdg, DirName); store.Base() != want {
		t.Errorf("Base = %q, want %q", store.Base(), want)
	}
}

// SaveCollapsed is atomic and best-effort, and the checked errors are the ones it can return (base dir is a FILE, destination is a DIRECTORY); both must name the file, since it runs on every fold keystroke.
func TestSaveCollapsedPropagatesErrors(t *testing.T) {
	t.Run("base is a file", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "state")
		if err := os.WriteFile(base, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := NewStoreAt(base).SaveCollapsed(map[string]bool{"a": true})
		if err == nil {
			t.Fatal("SaveCollapsed = nil, want error: the base is a file")
		}
		if !strings.Contains(err.Error(), "collapsed.json") && !strings.Contains(err.Error(), "state directory") {
			t.Errorf("the error does not name the failure: %v", err)
		}
	})

	t.Run("destination is a directory", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, FileName, "block"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := NewStoreAt(base).SaveCollapsed(map[string]bool{"a": true})
		if err == nil {
			t.Fatal("SaveCollapsed = nil, want error: the destination is a directory")
		}
		if !strings.Contains(err.Error(), "collapsed.json") {
			t.Errorf("the error does not name the file: %v", err)
		}
	})
}

func TestSaveCollapsedNotBreaksThePrevious(t *testing.T) {
	base := t.TempDir()
	store := NewStoreAt(base)
	if err := store.SaveCollapsed(map[string]bool{"old": true}); err != nil {
		t.Fatal(err)
	}
	antes, err := os.ReadFile(store.CollapsedFile())
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveCollapsed(map[string]bool{"new": true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.CollapsedFile())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]bool
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("collapsed.json ended up corrupt: %v\n%s", err, raw)
	}
	if !got["new"] || got["old"] {
		t.Errorf("collapsed.json = %v, want only the new state", got)
	}
	if _, err := os.Stat(store.CollapsedFile() + ".tmp"); err == nil {
		t.Error("the .tmp stayed on disk: the Rename did not happen")
	}
	_ = antes
}

// The third atomic-save failure point (the WriteFile of the .tmp, provoked with a DIRECTORY named collapsed.json.tmp) exists because permissions are unusable: root reads a 0o000 and the test would differ locally and in CI.
func TestSaveCollapsedFailsOnTheWriteTheTemporary(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, FileName+".tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := NewStoreAt(base).SaveCollapsed(map[string]bool{"a": true})
	if err == nil {
		t.Fatal("SaveCollapsed = nil, want error: the temp file is a directory")
	}
	if !strings.Contains(err.Error(), "could not write collapsed.json") {
		t.Errorf("error = %q, want it to name the temp-file write failure", err)
	}
}
