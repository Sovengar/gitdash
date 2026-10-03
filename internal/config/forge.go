// Configuración de forges: qué proveedor vive en qué host y en qué subcarpeta.
//
// Hace falta porque forge.ParseRemoteURL NO adivina: un host que no está
// declarado devuelve false a propósito, porque de un host desconocido no se
// puede saber a qué proveedor pertenece ni si su URL lleva prefijo, y adivinarlo
// produce enlaces que abren 404 sin que nada falle visiblemente. Sin esto,
// abrir un MR sobre el GitLab self-managed de la casa (que vive en /git/, no en
// la raíz del host) no podría ni resolverse ni ejecutarse.
//
// La forma es [forge.<proveedor>], y el ejemplo que lo explica:
//
//	[forge.github]
//	enabled = true
//	host = "github.com"
//
//	[forge.gitlab]
//	enabled = true
//	host = "umane.emeal.nttdata.com"
//	# api_base RELATIVO de la instancia ("/api/v4/", "/git/api/v4/"): de su
//	# path sale el prefijo de la subcarpeta del clone ("git"). Vacío = raíz.
//	api_base = "/git/api/v4/"
//	# clone_base es el override explícito de ese prefijo, para cuando la
//	# instancia no deduce su subcarpeta de la ruta del API. Vacío = derivado.
//	clone_base = "git"
//
// Un host POR proveedor, no una lista: es una instancia por puerta (gh/glab), y
// el prefijo sale del mismo dato que el host. enabled = false apaga el
// proveedor entero (su host deja de resolver a forge), que es como se deja una
// puerta instalada sin Instances declaradas.
//
// Los hosts públicos (github.com, gitlab.com) vienen de DefaultForges, derivados
// de la misma tabla que usa forge: una sola lista, para que el mapa de runtime y
// el parser no puedan discrepar.
package config

import (
	"strings"

	"gitdash/internal/forge"
)

// DefaultGitLabAPIBase es el api_base REST de una instancia de GitLab en la raíz
// de su host ("/api/v4/", relativo como los que declara el usuario). Es el
// único default que aporta algo: de él sale el prefijo de subcarpeta, y vacío
// significa "la instancia vive en la raíz". GitHub no lleva ninguno porque su
// api_base (/api/v3/) nunca lleva prefijo de clone, y declararlo sería un
// default que no dice nada.
const DefaultGitLabAPIBase = "/api/v4/"

// ForgeConfig declara dónde vive un proveedor: un host, si está activo, el
// api_base de esa instancia y el prefijo de subcarpeta (derivado, o forzado
// con CloneBase).
type ForgeConfig struct {
	// Enabled es la puerta: apagado, su host no resuelve a ningún forge. El
	// default es true, así que un proveedor declarado sin la clave sigue
	// funcionando.
	Enabled bool
	// Host es la instancia donde vive el proveedor. Singular a propósito: la
	// puerta (gh/glab) es la misma para todas las instancias, y el prefijo de
	// subcarpeta sale de su api_base, así que una lista de hosts necesitaría un
	// segundo dato que nadie tiene (a cuál de ellos aplica cada api_base).
	Host string
	// APIBase es el api_base REST RELATIVO de la instancia: "/api/v4/" o
	// "/git/api/v4/". Vacío = la instancia vive en la raíz del host.
	APIBase string
	// CloneBase sobrescribe el prefijo de subcarpeta cuando el api_base no lo
	// dice (una instancia montada fuera de la ruta estándar). Vacío = se deriva
	// de APIBase.
	CloneBase string
}

// ClonePrefix devuelve el relative URL root de la instancia: la subcarpeta en la
// que vive el forge ("git" para un GitLab self-managed bajo /git/), vacía si
// está en la raíz del host.
//
// CloneBase manda cuando viene, porque es el override explícito; si no, se
// deriva del api_base con forge.PrefixFromAPIBase, que devuelve vacío ante una
// forma inesperada en vez de adivinar. Nunca devuelve barras: el prefijo sale
// limpio porque es el que se compara contra el path del remoto.
func (f ForgeConfig) ClonePrefix() string {
	if base := strings.Trim(strings.TrimSpace(f.CloneBase), "/"); base != "" {
		return base
	}
	return forge.PrefixFromAPIBase(f.APIBase)
}

// DefaultForges son los proveedores que gitdash conoce sin que el usuario
// declare nada: los dos públicos. Los hosts salen de forge (su tabla), así que
// añadir ahí un host nuevo lo hace aparecer aquí sin tocar este paquete.
func DefaultForges() map[string]ForgeConfig {
	out := make(map[string]ForgeConfig, 2)
	for host, name := range forge.PublicHosts() {
		out[name] = ForgeConfig{Enabled: true, Host: host}
	}
	gl := out[forge.ForgeGitLab]
	gl.APIBase = DefaultGitLabAPIBase
	out[forge.ForgeGitLab] = gl
	return out
}

// supportedForge reporta si gitdash sabe abrir PRs en ese proveedor. La lista
// son las dos constantes de forge, no una copia: una tercera puerta (glab) sin
// su argv aquí sería una acción que falla sin explicación.
func supportedForge(name string) bool {
	switch normalizeForgeName(name) {
	case forge.ForgeGitHub, forge.ForgeGitLab:
		return true
	}
	return false
}

// addForge mezcla lo declarado por el usuario sobre lo que ya hay (los
// defaults, o lo declarado antes). Cada clave sustituye a la anterior cuando
// viene: con un host y un api_base por proveedor, "declarar" es exactamente
// "sustituir la instancia". Lo ausente conserva lo anterior, y un valor vacío
// se trata como ausente (mismo criterio que el resto del loader).
//
// El nombre del proveedor se normaliza a minúsculas: viene de una clave de
// config escrita a mano, y un "GitLab" que no casara con la constante dejaría
// la acción sin puerta.
func (c *Config) addForge(rawName string, f forgeConfig) {
	name := normalizeForgeName(rawName)
	if name == "" {
		return
	}
	cur := c.Forges[name]
	if f.Enabled != nil {
		cur.Enabled = *f.Enabled
	}
	if f.Host != nil {
		if h := normalizeHost(*f.Host); h != "" {
			cur.Host = h
		}
	}
	if f.APIBase != nil && strings.TrimSpace(*f.APIBase) != "" {
		cur.APIBase = strings.TrimSpace(*f.APIBase)
	}
	if f.CloneBase != nil && strings.TrimSpace(*f.CloneBase) != "" {
		cur.CloneBase = strings.TrimSpace(*f.CloneBase)
	}
	c.Forges[name] = cur
}

// ForgeHosts resuelve el mapa host → proveedor que consume
// forge.ParseRemoteURL. Un proveedor deshabilitado no aporta su host, así que
// sus remotos vuelven a ser "forge desconocido" en vez de esperar una puerta
// apagada.
func (c Config) ForgeHosts() map[string]string {
	out := make(map[string]string, len(c.Forges))
	for name, f := range c.Forges {
		if !f.Enabled {
			continue
		}
		if h := normalizeHost(f.Host); h != "" {
			out[h] = name
		}
	}
	return out
}

// ForgePrefixes resuelve host → prefijo de subcarpeta del clone, derivado con
// ForgeConfig.ClonePrefix. Un prefijo equivocado no rompe la creación (el argv
// no lo lleva: el proyecto ya viene sin él) pero rompe la URL web, así que sale
// del MISMO dato que el host en vez de declararse aparte.
//
// Solo entran los hosts con prefijo no vacío: la raíz del host es la ausencia de
// prefijo, y una entrada "" solo añadiría ruido al mapa que lee el parser.
func (c Config) ForgePrefixes() map[string]string {
	out := make(map[string]string, len(c.Forges))
	for _, f := range c.Forges {
		if !f.Enabled {
			continue
		}
		h := normalizeHost(f.Host)
		if h == "" {
			continue
		}
		if p := f.ClonePrefix(); p != "" {
			out[h] = p
		}
	}
	return out
}

// normalizeForgeName baja a minúsculas y recorta el nombre del proveedor, como
// el parser lo hace con el suyo: los dos vienen de la misma config y tienen que
// casar con las mismas constantes.
func normalizeForgeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// normalizeHost baja a minúsculas el host: el nombre de un host no distingue
// mayúsculas y el mapa de runtime se busca por él.
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
