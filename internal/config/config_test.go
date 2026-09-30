package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Marker != ".gitdash.toml" {
		t.Errorf("marker = %q", cfg.Marker)
	}
	if len(cfg.Roots) != 1 || filepath.Base(cfg.Roots[0]) != "dev" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if !cfg.FetchAuto || cfg.FetchConcurrency != 4 || cfg.FetchTimeout != 30*time.Second {
		t.Errorf("fetch defaults = %+v", cfg)
	}
	if cfg.Editor == "" {
		t.Error("editor default vacío")
	}
	found := false
	for _, ex := range cfg.Exclude {
		if ex == "testdata" {
			found = true
		}
	}
	if !found {
		t.Errorf("exclude default sin testdata: %v", cfg.Exclude)
	}
}

func TestPartialOverride(t *testing.T) {
	path := write(t, `roots = ["~/code", "~/work"]`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if len(cfg.Roots) != 2 || filepath.Base(cfg.Roots[0]) != "code" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto || cfg.FetchConcurrency != 4 {
		t.Errorf("defaults no conservados: %+v", cfg)
	}
}

func TestMalformed(t *testing.T) {
	path := write(t, `roots = [`)
	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("se esperaba warning de parseo")
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto {
		t.Errorf("no se usaron defaults: %+v", cfg)
	}
}

func TestMissingFile(t *testing.T) {
	cfg, warn := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.Marker != ".gitdash.toml" || len(cfg.Roots) != 1 {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := write(t, "roots = [\"~/dev\"]\n")
	cfg, _ := LoadFrom(path)
	if cfg.Roots[0] != filepath.Join(home, "dev") {
		t.Errorf("root = %q, want %q", cfg.Roots[0], filepath.Join(home, "dev"))
	}
}

func TestFetchOverride(t *testing.T) {
	path := write(t, `
[fetch]
auto = false
concurrency = 8
timeout = "10s"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchAuto {
		t.Error("auto debería ser false")
	}
	if cfg.FetchConcurrency != 8 || cfg.FetchTimeout != 10*time.Second {
		t.Errorf("fetch = %+v", cfg)
	}
}

func TestInvalidValuesIgnored(t *testing.T) {
	path := write(t, `
[fetch]
concurrency = 0
timeout = "nope"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchConcurrency != 4 || cfg.FetchTimeout != 30*time.Second {
		t.Errorf("valores inválidos no ignorados: %+v", cfg)
	}
}

// El mínimo válido de concurrency es 1 (un único fetch a la vez), no 0 ni 2: el
// borde de la guarda `c >= 1` es exactamente este valor.
func TestFetchConcurrencyMinimoAceptado(t *testing.T) {
	path := write(t, "[fetch]\nconcurrency = 1\n")
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchConcurrency != 1 {
		t.Errorf("concurrency = %d, want 1 (mínimo válido aceptado)", cfg.FetchConcurrency)
	}
}

// Un timeout de 0 (o negativo) es tan inválido como "nope": cae al default. El
// borde de la guarda `d > 0` es el 0, así que es 0 y no "un valor raro".
func TestFetchTimeoutNoPositivoIgnorado(t *testing.T) {
	for _, timeout := range []string{"0s", "0", "-1s", "-30m"} {
		t.Run(timeout, func(t *testing.T) {
			path := write(t, "[fetch]\ntimeout = "+quote(timeout)+"\n")
			cfg, _ := LoadFrom(path)
			if cfg.FetchTimeout != 30*time.Second {
				t.Errorf("timeout %q → %s, want el default 30s", timeout, cfg.FetchTimeout)
			}
		})
	}
}

func quote(s string) string { return `"` + s + `"` }

// Una clave ausente conserva su default: el `nil` del puntero y la ausencia de
// la clave son la misma cosa, y el valor por defecto sobrevive a cualquier
// config parcial.
func TestClavesAusentesConservanDefaults(t *testing.T) {
	path := write(t, `roots = ["/tmp"]`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.Marker != DefaultMarker {
		t.Errorf("marker = %q, want %q", cfg.Marker, DefaultMarker)
	}
	if !reflect.DeepEqual(cfg.Exclude, DefaultExclude) {
		t.Errorf("exclude = %v, want los defaults %v", cfg.Exclude, DefaultExclude)
	}
	if cfg.Editor != Defaults().Editor {
		t.Errorf("editor = %q, want %q", cfg.Editor, Defaults().Editor)
	}
	if cfg.SyncBranch != "main" {
		t.Errorf("sync_branch = %q, want main", cfg.SyncBranch)
	}
}

// Una clave presente pero vacía conserva el default: escribir `marker = ""` no
// es "quitar el marcador", es no decir nada.
func TestValoresVaciosConservanDefaults(t *testing.T) {
	path := write(t, `
marker = ""
editor = ""
sync_branch = ""
`)
	cfg, _ := LoadFrom(path)
	if cfg.Marker != DefaultMarker {
		t.Errorf("marker = %q, want %q", cfg.Marker, DefaultMarker)
	}
	if cfg.Editor != Defaults().Editor {
		t.Errorf("editor = %q, want %q", cfg.Editor, Defaults().Editor)
	}
	if cfg.SyncBranch != "main" {
		t.Errorf("sync_branch = %q, want main", cfg.SyncBranch)
	}
}

// `exclude = []` es una intención explícita (no podar nada) y sustituye a los
// defaults igual que cualquier otra lista: es lo que distingue "clave ausente"
// de "lista vacía".
func TestExcludeVacioDesactivaPodas(t *testing.T) {
	path := write(t, "exclude = []\n")
	cfg, _ := LoadFrom(path)
	if len(cfg.Exclude) != 0 {
		t.Errorf("exclude = %v, want vacío (podas desactivadas)", cfg.Exclude)
	}
}

// El editor por defecto sale de $EDITOR y solo cae a "vi" cuando no hay ninguno.
func TestEditorDefaultDesdeEntorno(t *testing.T) {
	t.Run("con EDITOR", func(t *testing.T) {
		t.Setenv("EDITOR", "nano -w")
		if got := Defaults().Editor; got != "nano -w" {
			t.Errorf("editor = %q, want nano -w", got)
		}
	})
	t.Run("sin EDITOR", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		if got := Defaults().Editor; got != "vi" {
			t.Errorf("editor = %q, want vi", got)
		}
	})
}

// expandAll solo toca un `~` seguido de separador, y solo cuando hay algo detrás
// o nada: `~/x` y `~/` se expanden; `~`, `~user` y las rutas absolutas no.
func TestExpandAll(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("sin home: %v", err)
	}
	cases := []struct {
		in   string
		want string
	}{
		{"~/dev", filepath.Join(home, "dev")},
		{"~/", home},
		{"~", "~"},
		{"~user/dev", "~user/dev"},
		{"/opt/dev", "/opt/dev"},
		{"dev", "dev"},
		{"", ""},
	}
	for _, c := range cases {
		got := expandAll([]string{c.in})
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("expandAll(%q) = %v, want [%q]", c.in, got, c.want)
		}
	}
}

// roots con `~` se expanden al cargarlos; la lista vacía no inventa entradas.
func TestExpandAllListaVacia(t *testing.T) {
	if got := expandAll(nil); len(got) != 0 {
		t.Errorf("expandAll(nil) = %v, want vacío", got)
	}
}

// El plegado tiene default `enter` (cubre worktrees y grupos) y es configurable
// como el resto de keybindings. Su hint lleva la tecla configurada y solo una vez.
func TestFoldKeybinding(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("fold") != "enter" {
		t.Errorf("default fold = %q, want enter", cfg.KeyFor("fold"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "enter fold") {
		t.Errorf("hint de plegado ausente: %v", cfg.HintBarLines())
	}

	// Rebind via config.toml.
	path := write(t, `
[keybindings]
fold = "w"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("fold") != "w" {
		t.Errorf("fold = %q, want w", cfg.KeyFor("fold"))
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "w fold") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
	if strings.Contains(hints, "enter fold") {
		t.Errorf("el hint sigue con la tecla anterior: %v", cfg.HintBarLines())
	}
}

// El borrado de worktree tiene default `D` y es reconfigurable.
func TestDefaultKeybindingsWorktreeRemove(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("worktree_remove") != "D" {
		t.Errorf("default worktree_remove = %q, want D", cfg.KeyFor("worktree_remove"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "D remove wt") {
		t.Errorf("hint de borrado ausente: %v", cfg.HintBarLines())
	}

	// Rebind via config.toml.
	path := write(t, `
[keybindings]
worktree_remove = "W"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("worktree_remove") != "W" {
		t.Errorf("worktree_remove = %q, want W", cfg.KeyFor("worktree_remove"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "W remove wt") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}

// El preview visual tiene default `v` y es reconfigurable; su hint lleva la
// etiqueta sin la tecla.
func TestDefaultKeybindingsVisual(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("visual") != "v" {
		t.Errorf("default visual = %q, want v", cfg.KeyFor("visual"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "v visual") {
		t.Errorf("hint de visual ausente: %v", cfg.HintBarLines())
	}

	path := write(t, `
[keybindings]
visual = "V"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("visual") != "V" {
		t.Errorf("visual = %q, want V", cfg.KeyFor("visual"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "V visual") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}

// Una config vieja con acciones que ya no existen (detail, expand) avisa en vez
// de dejar la tecla muerta: sin aviso, `detail = "enter"` hace que enter no haga
// nada y parece un bug de la TUI.
func TestKeybindingsObsoletosAvisan(t *testing.T) {
	path := write(t, `
[keybindings]
detail = "enter"
expand = "space"
fold   = "w"
`)
	cfg, warn := LoadFrom(path)
	if !strings.Contains(warn, "detail") || !strings.Contains(warn, "expand") {
		t.Errorf("aviso sin las acciones obsoletas: %q", warn)
	}
	if strings.Contains(warn, "fold") {
		t.Errorf("avisa de una acción válida: %q", warn)
	}
	// Las obsoletas no se cuelan en el mapa (su tecla queda libre) y las
	// válidas sí se aplican.
	if _, ok := cfg.Keybindings["detail"]; ok {
		t.Error("la acción obsoleta quedó en el mapa de teclas")
	}
	if cfg.KeyFor("fold") != "w" {
		t.Errorf("fold = %q, want w", cfg.KeyFor("fold"))
	}
}

// --- la puerta de entrada real: Load() y Path() ---

// Path es lo que la app llama al arrancar para saber dónde está su config. Todos
// los tests del paquete pasaban un path a mano con LoadFrom, así que la función
// que resuelve XDG_CONFIG_HOME no tenía ni un test: el mismo brazo que decide
// dónde vive el fichero nunca se ejecutó.
func TestPathRespetaXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := filepath.Join(dir, DirName, FileName); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// Sin XDG_CONFIG_HOME ni HOME no hay directorio de usuario, y Path tiene que
// decirlo en vez de devolver una ruta que no existe (un "/gitdash/config.toml"
// silencioso escribiría donde nadie lee).
func TestPathSinDirectorioDeUsuarioDaError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if got, err := Path(); err == nil {
		t.Errorf("Path sin HOME ni XDG = %q, want error", got)
	}
}

// Load es lo que arranca de verdad: lee Path y delega en LoadFrom. Sin fichero
// de config devuelve los defaults SIN aviso (un usuario que aún no ha configurado
// nada no tiene un error que ver), y con fichero, los valores de ahí.
func TestLoadSinFicheroDaDefaultsSilenciosos(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, warn := Load()
	if warn != "" {
		t.Errorf("sin config, Load avisa %q; un usuario sin configurar no tiene un error que ver", warn)
	}
	if len(cfg.Roots) == 0 {
		t.Error("sin config, Load no devolvió los roots por defecto")
	}
	if cfg.Marker == "" {
		t.Error("sin config, Load no devolvió el marcador por defecto")
	}
}

// Y que Load lee de verdad lo que hay en el path que resuelve: si leyera otro
// fichero, los roots del usuario no se aplicarían y descubriría repos donde no
// hay.
func TestLoadLeeElFicheroDelPathResuelto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	raiz := t.TempDir()
	cuerpo := fmt.Sprintf("roots = [%q]\nsync_branch = \"develop\"\n", raiz)
	if err := os.WriteFile(filepath.Join(dir, DirName, FileName), []byte(cuerpo), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, warn := Load()
	if warn != "" {
		t.Errorf("config válida, Load avisa %q", warn)
	}
	if len(cfg.Roots) != 1 || cfg.Roots[0] != raiz {
		t.Errorf("Roots = %v, want [%q]", cfg.Roots, raiz)
	}
	if cfg.SyncBranch != "develop" {
		t.Errorf("SyncBranch = %q, want develop", cfg.SyncBranch)
	}
}

// SyncBranchExplicit separa "el usuario puso sync_branch" de "es el default":
// el default es una suposición que el repo puede desmentir (y ahí la
// referencia cae a master), una rama escrita es una intención que no se toca.
func TestSyncBranchExplicit(t *testing.T) {
	t.Run("sin declarar", func(t *testing.T) {
		cfg, _ := LoadFrom(write(t, `roots = ["/tmp"]`))
		if cfg.SyncBranchExplicit {
			t.Error("SyncBranchExplicit con la clave ausente: el default no es una declaración")
		}
		if cfg.SyncBranch != "main" {
			t.Errorf("SyncBranch = %q, want main (el default)", cfg.SyncBranch)
		}
	})
	t.Run("declarada", func(t *testing.T) {
		cfg, _ := LoadFrom(write(t, `sync_branch = "develop"`))
		if !cfg.SyncBranchExplicit {
			t.Error("SyncBranchExplicit = false con sync_branch en el TOML")
		}
		if cfg.SyncBranch != "develop" {
			t.Errorf("SyncBranch = %q, want develop", cfg.SyncBranch)
		}
	})
	// `sync_branch = ""` no es una declaración: es una casilla vacía, y por
	// la misma regla que no pisa el default tampoco puede cerrar el fallback.
	t.Run("declarada pero vacía", func(t *testing.T) {
		cfg, _ := LoadFrom(write(t, `sync_branch = ""`))
		if cfg.SyncBranchExplicit {
			t.Error("SyncBranchExplicit = true con sync_branch vacío")
		}
	})
}

// Un comando declarado vacío NO sustituye al default: es una casilla sin
// rellenar, no una orden de "no hacer nada". Aceptarlo dejaba la acción con un
// argv vacío, que es un panic alcanzable desde el panel del log.
func TestComandoVacioNoPisaElDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path,
		[]byte("[commands]\npull = \"\"\npush = \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFrom(path)

	def := DefaultCommands()
	for _, k := range []string{"pull", "push"} {
		if cfg.Commands[k] != def[k] {
			t.Errorf("commands[%q] = %q, want el default %q: un valor vacío es una casilla sin rellenar, no una orden",
				k, cfg.Commands[k], def[k])
		}
	}
}

// Y la otra mitad del guard: un comando VACÍO de una acción que no existe ni
// tiene default no se registra. Si se registrara, el mapa tendría una entrada
// con argv vacío que el log presentaría como si algo se hubiera ejecutado.
func TestComandoVacioDeUnaAccionDesconocidaNoSeRegistra(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path,
		[]byte("[commands]\ninventada = \"\"\nreal = \"log --oneline -5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFrom(path)

	if v, ok := cfg.Commands["inventada"]; ok {
		t.Errorf("commands[\"inventada\"] = %q registrada: un comando vacío no es un comando", v)
	}
	// El namespace es abierto: una acción nueva con valor SÍ se acepta, que es
	// lo que permite el `[ai.pull]` y cualquier comando propio.
	if got := cfg.Commands["real"]; got != "log --oneline -5" {
		t.Errorf("commands[\"real\"] = %q, want el valor declarado", got)
	}
}
