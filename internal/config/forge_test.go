// Tests de la configuración de forges: qué hosts son de cada proveedor y qué
// prefijo de subcarpeta lleva cada instancia. Es la capa que hace que un
// GitLab self-managed (que vive en /git/, no en la raíz del host) resuelva en
// vez de quedarse sin forge.
package config

import (
	"strings"
	"testing"
)

// Los hosts públicos se resuelven sin declarar nada: sin ellos, hasta un remote
// de github.com devolvería "forge desconocido" y la acción de PR fallaría en
// silencio.
func TestForgeDefaults(t *testing.T) {
	cfg := Defaults()
	hosts := cfg.ForgeHosts()
	for host, want := range map[string]string{
		"github.com": "github",
		"gitlab.com": "gitlab",
	} {
		if got := hosts[host]; got != want {
			t.Errorf("ForgeHosts()[%q] = %q, want %q", host, got, want)
		}
	}
	// gitlab.com vive en la raíz de su host: un prefijo inventado mandaría
	// todas sus URLs web a /git/grupo/proy, que no existe.
	if got := cfg.ForgePrefixes()["gitlab.com"]; got != "" {
		t.Errorf("prefijo de gitlab.com = %q, want \"\" (la raíz del host)", got)
	}
}

// La forma que necesita un GitLab self-managed en subcarpeta: el api_base
// absoluto nombra el host al que aplica y de su path sale el prefijo del clone
// (/git/api/v4/ → "git"). Y el host público sigue con su default, porque son dos
// instancias distintas declaradas en la misma sección.
func TestForgeSelfManagedEnSubcarpeta(t *testing.T) {
	path := write(t, `
[forge.gitlab]
api_base = "https://git.example.com/git/api/v4/"
hosts = ["git.example.com"]
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := cfg.ForgeHosts()["git.example.com"]; got != "gitlab" {
		t.Errorf("ForgeHosts()[git.example.com] = %q, want gitlab", got)
	}
	if got := cfg.ForgePrefixes()["git.example.com"]; got != "git" {
		t.Errorf("prefijo = %q, want \"git\" (derivado de /git/api/v4/)", got)
	}
	if got := cfg.ForgePrefixes()["gitlab.com"]; got != "" {
		t.Errorf("prefijo de gitlab.com = %q, want \"\": el api_base absoluto nombra", got)
	}
}

// Un api_base relativo no nombra ningún host, así que aplica a todos los del
// proveedor: es la forma de declarar una instancia entera en subcarpeta.
func TestForgeAPIBaseRelativoAplicaATodos(t *testing.T) {
	path := write(t, `
[forge.gitlab]
api_base = "/api/v4/"
`)
	cfg, _ := LoadFrom(path)
	if got := cfg.ForgePrefixes()["gitlab.com"]; got != "" {
		t.Errorf("prefijo = %q, want \"\" (raíz del host)", got)
	}
}

// El prefijo se deriva del path, no del api_base entero: un host que se declara
// sin api_base (el caso de una instancia en la raíz) se queda sin prefijo en
// vez de heredar el de otra.
func TestForgeHostSinAPIBaseNoHeredaElPrefijo(t *testing.T) {
	path := write(t, `
[forge.gitlab]
api_base = "https://git.example.com/git/api/v4/"
hosts = ["otro.example.com"]
`)
	cfg, _ := LoadFrom(path)
	prefixes := cfg.ForgePrefixes()
	if got := prefixes["otro.example.com"]; got != "" {
		t.Errorf("prefijo de otro.example.com = %q, want \"\" (no lo nombra el api_base)", got)
	}
	if got := prefixes["git.example.com"]; got != "" {
		t.Errorf("git.example.com no se declaró, pero aparece con prefijo %q", got)
	}
}

// Los hosts se AÑADEN a los defaults, no los sustituyen: declarar el GitLab de
// la casa no puede dejar fuera a gitlab.com. Y declarar dos veces el mismo host
// no lo duplica.
func TestForgeHostsSeAcumulan(t *testing.T) {
	path := write(t, `
[forge.gitlab]
api_base = "https://a.example.com/git/api/v4/"
hosts = ["a.example.com", "a.example.com", "b.example.com"]
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if got := len(cfg.Forges["gitlab"].Hosts); got != 3 {
		t.Errorf("hosts = %v, want gitlab.com + a + b", cfg.Forges["gitlab"].Hosts)
	}
	if cfg.ForgeHosts()["gitlab.com"] != "gitlab" {
		t.Error("declarar un host self-managed borró el público")
	}
}

// El nombre del proveedor se normaliza: viene de una clave escrita a mano, y un
// "GitLab" que no casara con la constante dejaría la acción sin puerta.
func TestForgeNombreNormalizado(t *testing.T) {
	path := write(t, `
[forge.GitLab]
hosts = ["a.example.com"]
`)
	cfg, warn := LoadFrom(path)
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
	path := write(t, `
[forge.bitbucket]
hosts = ["bitbucket.org"]
`)
	cfg, warn := LoadFrom(path)
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
	path := write(t, `
[forge.gitlab]
api_base = ""
`)
	cfg, _ := LoadFrom(path)
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
	path := write(t, `
[keybindings]
pr = "W"
`)
	cfg, warn := LoadFrom(path)
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
