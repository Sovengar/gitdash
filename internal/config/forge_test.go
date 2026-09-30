// Tests de la configuración de forges: qué proveedor vive en qué host y qué
// prefijo de subcarpeta lleva cada instancia. Es la capa que hace que un
// GitLab self-managed (que vive en /git/, no en la raíz del host) resuelva en
// vez de quedarse sin forge.
package config

import (
	"strings"
	"testing"
)

// La forma de la sección [forge.*] que se usa: un host por proveedor, el
// api_base RELATIVO de la instancia y enabled explícito. El prefijo de
// subcarpeta sale de ahí, no de un segundo dato.
const forgeSelfManaged = `
[forge.github]
enabled = true
host = "github.com"

[forge.gitlab]
enabled = true
host = "umane.emeal.nttdata.com"
api_base = "/git/api/v4/"
`

// Los hosts públicos se resuelven sin declarar nada: sin ellos, hasta un remote
// de github.com devolvería "forge desconocido" y la acción de PR fallaría en
// silencio.
func TestForgeDefaults(t *testing.T) {
	cfg := Defaults()
	hosts := cfg.ForgeHosts()
	if len(hosts) != 2 {
		t.Errorf("ForgeHosts() = %v, want solo los dos públicos", hosts)
	}
	for host, want := range map[string]string{
		"github.com": "github",
		"gitlab.com": "gitlab",
	} {
		if got := hosts[host]; got != want {
			t.Errorf("ForgeHosts()[%q] = %q, want %q", host, got, want)
		}
	}
	for name, want := range map[string]ForgeConfig{
		"github": {Enabled: true, Host: "github.com"},
		"gitlab": {Enabled: true, Host: "gitlab.com", APIBase: DefaultGitLabAPIBase},
	} {
		if got := cfg.Forges[name]; got != want {
			t.Errorf("default %q = %+v, want %+v", name, got, want)
		}
	}
	// gitlab.com vive en la raíz de su host: un prefijo inventado mandaría
	// todos sus proyectos a /git/grupo/proy, que no existe. Y al estar vacío,
	// el prefijo ni siquiera entra en el mapa.
	if _, ok := cfg.ForgePrefixes()["gitlab.com"]; ok {
		t.Error("gitlab.com entró en ForgePrefixes con la raíz del host")
	}
	if got := cfg.Forges["gitlab"].ClonePrefix(); got != "" {
		t.Errorf("ClonePrefix() de gitlab.com = %q, want \"\"", got)
	}
}

// La forma que necesita un GitLab self-managed en subcarpeta: un host, y un
// api_base RELATIVO del que sale el prefijo (/git/api/v4/ → "git"). Declarar el
// host sustituye al del default del proveedor, porque son una sola instancia
// por proveedor: no es una lista a la que añadir el self-managed.
func TestForgeSelfManagedEnSubcarpeta(t *testing.T) {
	cfg, warn := LoadFrom(write(t, forgeSelfManaged))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	hosts := cfg.ForgeHosts()
	if len(hosts) != 2 {
		t.Errorf("ForgeHosts() = %v, want el host declarado y el de github", hosts)
	}
	if got := hosts["umane.emeal.nttdata.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[umane.emeal.nttdata.com] = %q, want gitlab", got)
	}
	if _, ok := hosts["gitlab.com"]; ok {
		t.Error("el host declarado no sustituyó al del default del proveedor")
	}
	prefixes := cfg.ForgePrefixes()
	if len(prefixes) != 1 {
		t.Errorf("ForgePrefixes() = %v, want solo el host con prefijo", prefixes)
	}
	if got := prefixes["umane.emeal.nttdata.com"]; got != "git" {
		t.Errorf("prefijo = %q, want \"git\" (derivado de /git/api/v4/)", got)
	}
}

// enabled = false apaga el proveedor entero: ni su host resuelve a forge ni su
// prefijo entra en el mapa. Es lo que permite dejar una puerta (glab) instalada
// pero sin Instances declaradas.
func TestForgeEnabledFalseDeshabilitaElProveedor(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.gitlab]
enabled = false
host = "umane.emeal.nttdata.com"
api_base = "/git/api/v4/"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if _, ok := cfg.ForgeHosts()["umane.emeal.nttdata.com"]; ok {
		t.Error("el host de un proveedor deshabilitado resolvió a forge")
	}
	if _, ok := cfg.ForgePrefixes()["umane.emeal.nttdata.com"]; ok {
		t.Error("el prefijo de un proveedor deshabilitado entró en el mapa")
	}
	// El flag es POR PROVEEDOR, no por host: gitlab.com tampoco resuelve ya, y
	// github sí (no es un apagón global).
	if _, ok := cfg.ForgeHosts()["gitlab.com"]; ok {
		t.Error("gitlab.com resolvió con su proveedor deshabilitado")
	}
	if got := cfg.ForgeHosts()["github.com"]; got != "github" {
		t.Errorf("ForgeHosts()[github.com] = %q, want github", got)
	}
}

// enabled ausente conserva lo declarado antes: el puntero del TOML crudo
// distingue "no lo he dicho" de "lo he apagado".
func TestForgeEnabledAusenteNoDeshabilita(t *testing.T) {
	cfg, _ := LoadFrom(write(t, `
[forge.gitlab]
host = "git.example.com"
`))
	if got := cfg.ForgeHosts()["git.example.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[git.example.com] = %q, want gitlab", got)
	}
}

// clone_base es el override explícito del prefijo: gana sobre el derivado del
// api_base, y es lo que se declara cuando la instancia NO deduce su subcarpeta
// de la ruta del API.
func TestForgeCloneBaseEsOverrideDelPrefijo(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want string
	}{
		{
			name: "sin clone_base sale del api_base",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/git/api/v4/\"\n",
			want: "git",
		},
		{
			name: "clone_base manda sobre un api_base sin subcarpeta",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/api/v4/\"\nclone_base = \"git\"\n",
			want: "git",
		},
		{
			name: "clone_base manda sobre un api_base con otra subcarpeta",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/git/api/v4/\"\nclone_base = \"otro\"\n",
			want: "otro",
		},
		{
			name: "clone_base con barras se normaliza",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\nclone_base = \"/git/\"\n",
			want: "git",
		},
		{
			name: "api_base inesperado no inventa prefijo",
			toml: "[forge.gitlab]\nhost = \"a.example.com\"\napi_base = \"/git/api/v3/\"\n",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warn := LoadFrom(write(t, tc.toml))
			if warn != "" {
				t.Fatalf("warn inesperado: %q", warn)
			}
			if got := cfg.ForgePrefixes()["a.example.com"]; got != tc.want {
				t.Errorf("prefijo = %q, want %q", got, tc.want)
			}
		})
	}
}

// El prefijo vacío NO va al mapa: con la raíz del host, una entrada "" y su
// ausencia producen el mismo Project, así que meterse solo añade ruido al mapa
// que consume el parser.
func TestForgePrefijoVacioNoEntraEnElMapa(t *testing.T) {
	cfg, _ := LoadFrom(write(t, forgeSelfManaged))
	if _, ok := cfg.ForgePrefixes()["github.com"]; ok {
		t.Error("github (sin api_base) entró en ForgePrefixes")
	}
}

// Un host repetido no aparece dos veces ni pisa al otro proveedor: el mapa es
// host → proveedor, y el host de un default que se vuelve a declarar es el
// mismo host.
func TestForgeHostDuplicadoNoSeRepite(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.github]
host = "github.com"

[forge.gitlab]
host = "gitlab.com"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	hosts := cfg.ForgeHosts()
	if len(hosts) != 2 {
		t.Errorf("ForgeHosts() = %v, want dos hosts, uno por proveedor", hosts)
	}
	for host, want := range map[string]string{
		"github.com": "github",
		"gitlab.com": "gitlab",
	} {
		if got := hosts[host]; got != want {
			t.Errorf("ForgeHosts()[%q] = %q, want %q", host, got, want)
		}
	}
	if got := hosts["github.com"]; got != "github" {
		t.Errorf("el host re-declarado perdió su proveedor: %q", got)
	}
}

// Un host vacío o en blanco no inventa una entrada: sin host el proveedor no
// tiene dónde vivir, y un "" en el mapa haría pasar por GitHub a un remoto sin
// host (ForgeForHost normaliza a "" igual).
func TestForgeHostVacioNoEntraEnElMapa(t *testing.T) {
	cfg, _ := LoadFrom(write(t, `
[forge.gitlab]
host = "   "
`))
	for h := range cfg.ForgeHosts() {
		if h == "" {
			t.Error("ForgeHosts() tiene la clave vacía")
		}
	}
	if got := cfg.Forges["gitlab"].Host; got != "gitlab.com" {
		t.Errorf("host = %q, want el del default (el vacío no sustituye)", got)
	}
}

// El nombre del proveedor se normaliza: viene de una clave escrita a mano, y un
// "GitLab" que no casara con la constante dejaría la acción sin puerta.
func TestForgeNombreNormalizado(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.GitLab]
host = "a.example.com"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := cfg.ForgeHosts()["a.example.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[a.example.com] = %q, want gitlab", got)
	}
}

// Un proveedor que no soportamos avisa en vez de aceptarse en silencio: con él,
// cada remote de ese host resolvería a un forge sin puerta y el PR fallaría con
// un motivo que no señala la config.
func TestForgeNoSoportadoAvisa(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[forge.bitbucket]
host = "bitbucket.org"
`))
	if !strings.Contains(warn, "bitbucket") {
		t.Errorf("warn sin el proveedor no soportado: %q", warn)
	}
	if _, ok := cfg.ForgeHosts()["bitbucket.org"]; ok {
		t.Error("el proveedor no soportado quedó en el mapa de hosts")
	}
}

// Un api_base vacío no borra el declarado: la clave ausente y la vacía son lo
// mismo aquí (mismo criterio que el resto de secciones con punteros).
func TestForgeAPIBaseVacioConservaElDefault(t *testing.T) {
	cfg, _ := LoadFrom(write(t, `
[forge.gitlab]
api_base = ""
`))
	if got := cfg.Forges["gitlab"].APIBase; got != DefaultGitLabAPIBase {
		t.Errorf("api_base = %q, want el default %q", got, DefaultGitLabAPIBase)
	}
}

// La acción de PR existe y su tecla no pisa a ninguna otra: dos acciones con la
// misma tecla harían que actionForKey devolviera una u otra al azar (recorre el
// mapa).
func TestDefaultKeybindingsPR(t *testing.T) {
	cfg := Defaults()
	if got := cfg.KeyFor("pr"); got != "O" {
		t.Errorf("default pr = %q, want O", got)
	}
	for action, key := range DefaultKeybindings() {
		if action != "pr" && key == "O" {
			t.Errorf("la acción %q también usa la tecla O", action)
		}
	}
	// La etiqueta va SIN la tecla dentro: la antepone HintBarLines, y si la
	// llevara un rebind produciría hints como "W O new PR".
	if label, ok := hintLabels["pr"]; !ok || label != "new PR" {
		t.Errorf("hintLabels[pr] = %q (%v), want \"new PR\"", label, ok)
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "O new PR") {
		t.Errorf("hint de PR ausente: %v", cfg.HintBarLines())
	}
	// Y tampoco una tecla que el enrutado resuelve por su cuenta.
	for _, fixed := range []string{"q", "k", "j", "up", "down", "home", "end", "enter", "esc", "tab"} {
		if DefaultKeybindings()["pr"] == fixed {
			t.Errorf("pr pisa la tecla fija %q", fixed)
		}
	}
}

// La tecla de PR se rebindea como cualquier otra, y el hint lo sigue.
func TestDefaultKeybindingsPRRebind(t *testing.T) {
	cfg, warn := LoadFrom(write(t, `
[keybindings]
pr = "W"
`))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := cfg.KeyFor("pr"); got != "W" {
		t.Errorf("pr = %q, want W", got)
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "W new PR") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}
