package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitdash/internal/discovery"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	projects := []discovery.Project{
		{Path: t.TempDir(), Name: "api", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: t.TempDir(), Name: "wt", IsWorktree: true, HasRepo: true},
	}
	for _, p := range projects {
		if err := os.WriteFile(filepath.Join(p.Path, ".gitdash.toml"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(path, projects); err != nil {
		t.Fatal(err)
	}

	got := Load(path, ".gitdash.toml")
	if len(got) != 2 {
		t.Fatalf("load = %d, want 2", len(got))
	}
	if got[0].Name != "api" || got[0].PrimaryGroup != "vsocial" ||
		got[0].SecondaryGroup != "backend" || !got[0].HasRepo {
		t.Errorf("got[0] = %+v", got[0])
	}
	if !got[1].IsWorktree {
		t.Errorf("worktree perdido: %+v", got[1])
	}
}

func TestLoadDiscardsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	alive := t.TempDir()
	if err := os.WriteFile(filepath.Join(alive, ".gitdash.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []discovery.Project{
		{Path: alive, Name: "vivo"},
		{Path: filepath.Join(t.TempDir(), "borrado"), Name: "muerto"},
	}); err != nil {
		t.Fatal(err)
	}

	got := Load(path, ".gitdash.toml")
	if len(got) != 1 || got[0].Name != "vivo" {
		t.Errorf("got = %+v", got)
	}
}

func TestLoadCorruptSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("got = %+v, want nil sin crash", got)
	}
}

func TestLoadWrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"repos":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("versión futura aceptada: %+v", got)
	}
}

// Cache v2 (con clave group) se ignora silenciosamente.
func TestLoadV2Ignored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	raw := `{"version":2,"repos":[{"path":"/x","name":"api","group":"vsocial","has_repo":true}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path, ".gitdash.toml"); got != nil {
		t.Errorf("cache v2 aceptado: %+v", got)
	}
}

func TestSaveValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "repos.json")
	if err := Save(path, nil); err != nil {
		t.Fatal(err)
	}
	var f File
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != version {
		t.Errorf("json inválido: %v", err)
	}
}

// --- Path: la puerta que decide dónde vive la cache ---

// Todos los tests del paquete pasan un path a mano, así que la función que
// resuelve $XDG_CACHE_HOME no se ejecutaba nunca. Es la que decide dónde está
// repos.json, el fichero que hace que la app pinte al instante en vez de
// quedarse vacía mientras escanea.
func TestPathRespetaXDGCacheHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := filepath.Join(dir, DirName, FileName); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// Sin XDG_CACHE_HOME ni HOME no hay directorio de usuario, y Path tiene que
// decirlo. Devolver una ruta inventada haría que Save escribiera en un sitio
// que nadie lee, en silencio.
func TestPathSinDirectorioDeUsuarioDaError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if got, err := Path(); err == nil {
		t.Errorf("Path sin HOME ni XDG = %q, want error", got)
	}
}

// Una entrada sin Path no se puede validar contra el marcador (.Stat sobre ""
// es el directorio de trabajo del proceso, no un repo) y ademas no sirve para
// navegar: se descarta ANTES de mirar el marcador, no despues. Si el filtro se
// moviera detras, una entrada corrupta colada en el repos.json haria que el
// dashboard arrancara con una fila que no existe en disco.
func TestLoadDescartaEntradaSinPath(t *testing.T) {
	dir := t.TempDir()
	vivo := t.TempDir()
	// Load solo acepta un repo cuyo marcador exista en disco: sin el, la
	// entrada se descarta por el filtro de estaleness, no por el de Path.
	if err := os.WriteFile(filepath.Join(vivo, ".gitdash.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "repos.json")
	raw := fmt.Sprintf(`{"version":%d,"repos":[
		{"path":%q,"name":"vivo"},
		{"path":"","name":"sin-path"},
		{"name":"ni-campo"}
	]}`, version, vivo)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path, ".gitdash.toml")
	if len(got) != 1 {
		t.Fatalf("Load devolvio %d repos, want 1: %+v", len(got), got)
	}
	if got[0].Name != "vivo" {
		t.Errorf("se quedo %q, want el que tiene marcador", got[0].Name)
	}
}

// Save es best-effort pero no silencioso: si no puede crear el directorio ni
// escribir, devuelve el error para que la UI lo diga. El caso que se comprueba es
// el de un path cuyo directorio padre es un FICHERO, que hace fallar MkdirAll
// sin permisos ni root.
func TestSavePropagaErrorDeDirectorio(t *testing.T) {
	bloque := filepath.Join(t.TempDir(), "soy-un-fichero")
	if err := os.WriteFile(bloque, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// El directorio padre de `repos.json` es un fichero normal.
	err := Save(filepath.Join(bloque, "repos.json"), nil)
	if err == nil {
		t.Fatal("Save = nil, want error: no puede crear un directorio debajo de un fichero")
	}
}

// Y el camino feliz de crear un arbol de directorios que no existe todavia: es lo
// que pasa en el primer arranque, cuando ~/.config/gitdash no existe.
func TestSaveCreaLosDirectoriosQueFaltan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "repos.json")
	if err := Save(path, []discovery.Project{
		{Path: "/x", Name: "x", HasRepo: true},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load(path, ".gitdash.toml")
	// El marcador no existe todavia, asi que Load lo descarta: lo que importa
	// aqui es que el fichero existe y es JSON valido con la version buena.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("el fichero no se escribio: %v", err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("el fichero no es JSON valido: %v", err)
	}
	if f.Version != version {
		t.Errorf("version = %d, want %d", f.Version, version)
	}
	if len(f.Repos) != 1 || f.Repos[0].Name != "x" {
		t.Errorf("repos = %+v, want el unico proyecto guardado", f.Repos)
	}
	_ = got
}
