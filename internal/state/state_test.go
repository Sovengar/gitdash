package state

import (
	"os"
	"path/filepath"
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

	// Save initial state
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

func TestCompositeKeysNoCollision(t *testing.T) {
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

// Las claves de expansión de worktrees conviven con las de grupo
// bajo el namespace WorktreePrefix sin colisionar (misma forma de store).
func TestWorktreeNamespaceCoexists(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	combined := map[string]bool{
		"backend":                     true,  // grupo plegado
		WorktreePrefix + "/tmp/multi": true,  // worktree expandido
		WorktreePrefix + "/tmp/otro":  false, // expandido y luego plegado
	}
	if err := store.SaveCollapsed(combined); err != nil {
		t.Fatal(err)
	}
	loaded := store.LoadCollapsed()
	if !loaded["backend"] {
		t.Error("clave de grupo perdida")
	}
	if !loaded[WorktreePrefix+"/tmp/multi"] {
		t.Error("clave de expansión perdida")
	}
	if loaded[WorktreePrefix+"/tmp/otro"] {
		t.Error("polaridad invertida mal persistida")
	}
	if _, ok := loaded["/tmp/multi"]; ok {
		t.Error("la clave se guardó sin namespace")
	}
}
