package state

import (
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

// --- dónde vive el estado ---

// DefaultBaseDir decide el directorio del estado, y TODOS los tests de este
// fichero usan NewStoreAt(t.TempDir()), así que la función no estaba probada en
// absoluto. Es la que decide dónde acaba collapsed.json: si el brazo equivocado
// gana, el estado se escribe donde la app no lo va a leer y el usuario pierde sus
// grupos plegados sin ver ningún error.
//
// La rama de XDG_STATE_HOME manda sobre la de $HOME, y sin ninguna de las dos no
// hay directorio: os.UserHomeDir falla con $HOME vacío en Linux, así que el
// error también es alcanzable.
func TestDefaultBaseDirResuelveDondeViveElEstado(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()

	t.Run("XDG manda sobre HOME", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", xdg)
		t.Setenv("HOME", home)
		got, err := DefaultBaseDir()
		if err != nil {
			t.Fatalf("DefaultBaseDir: %v", err)
		}
		want := filepath.Join(xdg, DirName)
		if got != want {
			t.Errorf("base = %q, want %q (XDG manda)", got, want)
		}
	})

	t.Run("sin XDG se usa el estado bajo el home", func(t *testing.T) {
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

	t.Run("XDG vacío cuenta como no puesto", func(t *testing.T) {
		// XDG_STATE_HOME="" NO es un directorio: la spec lo trata como no
		// definido. Si se aceptara, el estado acabaría en /gitdash, en la raíz.
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", home)
		got, err := DefaultBaseDir()
		if err != nil {
			t.Fatalf("DefaultBaseDir: %v", err)
		}
		if !strings.HasPrefix(got, home) {
			t.Errorf("base = %q, want algo bajo %q: un XDG vacío no es un directorio", got, home)
		}
	})

	t.Run("sin home no hay base y se dice", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		t.Setenv("HOME", "")
		if _, err := DefaultBaseDir(); err == nil {
			t.Error("DefaultBaseDir sin HOME ni XDG devolvió un directorio: el estado se escribiría en un sitio imprevisible")
		}
	})
}

// NewStore propaga el error de DefaultBaseDir en vez de devolver un store que
// escribe en un directorio que no existe: un store silenciosamente inservible
// es peor que un fallo al arrancar, porque el usuario no ve ningún error y
// además pierde lo que tenía plegado.
func TestNewStorePropagaElErrorDeBase(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	store, err := NewStore()
	if err == nil {
		t.Fatalf("NewStore sin home devolvió un store en %q", store.Base())
	}
	if store != nil {
		t.Errorf("con error, NewStore devolvió store = %v, want nil", store)
	}
}

// Y con directorio, NewStore y su base coinciden: es el mismo camino que
// DefaultBaseDir, no otro.
func TestNewStoreUsaLaMismaBase(t *testing.T) {
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
