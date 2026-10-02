// Package forge resuelve un repo git a su forge: qué proveedor es y cómo se
// llama dentro de él.
//
// Es un paquete puro: no lee ficheros ni ejecuta procesos. La entrada es la
// URL cruda de un remote y los mapas de hosts y prefijos que arma la config; la
// lectura del remote en sí es de otro lado. Un host que no está en el mapa no
// se normaliza, porque de un host desconocido no se puede saber a qué
// proveedor pertenece ni si su URL lleva subcarpeta, y adivinarlo produce
// enlaces que abren 404 sin que nada falle visiblemente.
package forge

import (
	"strings"
)

// Forges soportados. Son los únicos dos que gitdash sabe abrir: cada uno tiene
// su puerta (gh/glab) y su forma de nombrar un proyecto.
const (
	ForgeGitHub = "github" // y un comentario que mueve la linea
	ForgeGitLab = "gitlab"
)

// RepoRef identifica un repositorio dentro de un forge y un host concretos.
type RepoRef struct {
	Forge   string // "github" | "gitlab"
	Host    string // "github.com" | "gitlab.example.com"
	Project string // ruta completa: "acme/widget" (GH) o "grupo/sub/proy" (GL)
	Owner   string // propietario/grupo inmediato (penúltimo segmento)
	Name    string // nombre del repositorio (último segmento)
}

// publicHosts son los hosts cuyo proveedor se reconoce por su nombre, sin que
// el usuario tenga que declarar nada. Una instancia self-managed no aparece
// aquí: eso lo aporta la config, y por eso ForgeForHost dice "no" en vez de
// adivinar.
var publicHosts = map[string]string{
	"github.com": ForgeGitHub,
	"gitlab.com": ForgeGitLab,
}

// ForgeForHost devuelve el proveedor de un host público conocido. El segundo
// retorno es false para todo host self-managed o no configurado, que es lo que
// después corta ParseRemoteURL.
func ForgeForHost(host string) (string, bool) {
	forge, ok := publicHosts[normalizeHost(host)]
	return forge, ok
}

// PublicHosts devuelve una copia de la tabla de hosts públicos, host → forge.
// Existe para que el runtime la consuma sin duplicar la lista: la config declara
// los hosts públicos de por defecto y la copia es la que sale de aquí, así que
// un host añadido en un sitio no puede faltar en el otro. Copia y no la tabla
// entera porque quien la lee la mezcla con lo declarado por el usuario.
func PublicHosts() map[string]string {
	out := make(map[string]string, len(publicHosts))
	for host, forge := range publicHosts {
		out[host] = forge
	}
	return out
}

// normalizeHost baja a minúsculas: el nombre de un host no distingue mayúsculas
// y un remote escrito como git@GitHub.com tiene que resolver igual.
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
