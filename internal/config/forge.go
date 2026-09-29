// Configuración de forges: qué hosts son de GitHub y cuáles de GitLab, y en qué
// subcarpeta vive cada instancia.
//
// Hace falta porque forge.ParseRemoteURL NO adivina: un host que no está
// declarado devuelve false a propósito, porque de un host desconocido no se
// puede saber a qué proveedor pertenece ni si su URL web lleva prefijo, y
// adivinarlo produce enlaces que abren 404 sin que nada falle visiblemente. Sin
// esto, abrir un PR sobre el GitLab self-managed de la casa (que vive en
// /git/, no en la raíz del host) no podría ni resolverse ni ejecutarse.
//
// La forma es [forge.<proveedor>] con dos claves, y el ejemplo que lo explica:
//
//	[forge.gitlab]
//	# api_base de la instancia que añades. Al ser ABSOLUTO nombra el host al
//	# que aplica: de ahí sale también la subcarpeta del clone (/git/api/v4/ →
//	# "git"). Un api_base relativo ("/api/v4/") aplica a todos los hosts del
//	# proveedor.
//	api_base = "https://git.example.com/git/api/v4/"
//	# Hosts donde vive este proveedor. Se AÑADEN a los de por defecto, así que
//	# aquí solo va lo que no es github.com/gitlab.com.
//	hosts = ["git.example.com"]
//
// Los hosts públicos (github.com, gitlab.com) vienen de DefaultForges, derivados
// de la misma tabla que usa forge: una sola lista, para que el mapa de runtime y
// el parser no puedan discrepar.
package config

import (
	"net/url"
	"strings"

	"gitdash/internal/forge"
)

// DefaultGitLabAPIBase es el api_base REST de una instancia de GitLab en la raíz
// de su host ("https://gitlab.com" + "/api/v4/"). Es el único default que
// aporta algo: de él sale el prefijo de subcarpeta, y vacío significa "la
// instancia vive en la raíz". GitHub no lleva ninguno porque su api_base
// (/api/v3/) nunca lleva prefijo de clone, y declararlo sería un default que no
// dice nada.
const DefaultGitLabAPIBase = "/api/v4/"

// ForgeConfig declara dónde vive un proveedor: los hosts donde vive y el
// api_base de la instancia. La lista de hosts y el api_base son el mismo hecho
// dicho de dos formas (el host se deduce del api_base absoluto), y se guardan
// separados porque un host puede declararse sin api_base: el caso normal de
// "esta instancia está en la raíz del host".
type ForgeConfig struct {
	Hosts   []string
	APIBase string
}

// DefaultForges son los proveedores que gitdash conoce sin que el usuario
// declare nada: los dos públicos. Los hosts salen de forge (su tabla), así que
// añadir ahí un host nuevo lo hace aparecer aquí sin tocar este paquete.
func DefaultForges() map[string]ForgeConfig {
	out := make(map[string]ForgeConfig, 2)
	for host, name := range forge.PublicHosts() {
		f := out[name]
		f.Hosts = append(f.Hosts, host)
		out[name] = f
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
// defaults, o lo declarado antes). Los hosts se AÑADEN —gitlab.com sigue
// funcionando mientras se declara el self-managed— y el api_base sustituye al
// anterior, porque es el de la instancia que se acaba de declarar.
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
	for _, h := range f.Hosts {
		h = normalizeHost(h)
		if h == "" || containsHost(cur.Hosts, h) {
			continue
		}
		cur.Hosts = append(cur.Hosts, h)
	}
	if f.APIBase != nil && strings.TrimSpace(*f.APIBase) != "" {
		cur.APIBase = strings.TrimSpace(*f.APIBase)
	}
	c.Forges[name] = cur
}

// ForgeHosts resuelve el mapa host → proveedor que consume
// forge.ParseRemoteURL. Los hosts públicos y los declarados se mezclan en una
// sola pasada, así que el runtime y la config no pueden discrepar.
func (c Config) ForgeHosts() map[string]string {
	out := make(map[string]string, len(c.Forges)+2)
	for name, f := range c.Forges {
		for _, h := range f.Hosts {
			if h = normalizeHost(h); h != "" {
				out[h] = name
			}
		}
	}
	return out
}

// ForgePrefixes resuelve host → prefijo de subcarpeta del clone, derivado del
// api_base con forge.PrefixFromAPIBase. Un prefijo equivocado no rompe la
// creación (el argv no lo lleva: el proyecto ya viene sin él) pero rompe la URL
// web, así que sale del MISMO dato que el host en vez de declararse aparte.
func (c Config) ForgePrefixes() map[string]string {
	def := DefaultForges()
	out := make(map[string]string, len(c.Forges)+2)
	for name, f := range c.Forges {
		// Un api_base absoluto nombra la instancia a la que aplica, y por tanto
		// a UN host. Los demás hosts del proveedor se quedan con el api_base de
		// por defecto: es la diferencia entre "esta instancia vive en /git/" y
		// "gitlab.com vive en la raíz", que declaradas en la misma sección.
		named := apiBaseHost(f.APIBase)
		for _, h := range f.Hosts {
			h = normalizeHost(h)
			if h == "" {
				continue
			}
			api := f.APIBase
			if named != "" && !strings.EqualFold(named, h) {
				api = def[name].APIBase
			}
			out[h] = forge.PrefixFromAPIBase(apiBasePath(api))
		}
	}
	return out
}

// apiBaseHost devuelve el host que nombra un api_base absoluto, o "" si el
// api_base es relativo (o está vacío): un api_base relativo no puede atribuirse
// a ningún host en concreto.
func apiBaseHost(apiBase string) string {
	u, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

// api_base de la subcarpeta se deriva de ahí, nunca del esquema ni del host (el
// prefijo vive en el path, y forge.PrefixFromAPIBase lo espera así).
func apiBasePath(apiBase string) string {
	apiBase = strings.TrimSpace(apiBase)
	u, err := url.Parse(apiBase)
	if err != nil {
		return apiBase
	}
	return u.Path
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

// containsHost evita hosts repetidos al fusionar la config con los defaults.
func containsHost(hosts []string, host string) bool {
	for _, h := range hosts {
		if normalizeHost(h) == host {
			return true
		}
	}
	return false
}
