// Package forge resuelve un repo git a su forge: qué proveedor es, cómo se
// llama dentro de él y qué URL web tiene.
//
// Es un paquete puro: no lee ficheros ni ejecuta procesos. La entrada es la
// URL cruda de un remote y los mapas de hosts y prefijos que arma la config; la
// lectura del remote en sí es de otro lado. Un host que no está en el mapa no
// se normaliza, porque de un host desconocido no se puede saber a qué
// proveedor pertenece ni si su URL web lleva subcarpeta, y adivinarlo produce
// enlaces que abren 404 sin que nada falle visiblemente.
package forge

import (
	"fmt"
	"strings"
)

// Forges soportados. Son los únicos dos que gitdash sabe abrir: cada uno tiene
// su puerta (gh/glab) y su forma de nombrar un proyecto.
const (
	ForgeGitHub = "github"
	ForgeGitLab = "gitlab"
)

// RepoRef identifica un repositorio dentro de un forge y un host concretos.
type RepoRef struct {
	Forge   string // "github" | "gitlab"
	Host    string // "github.com" | "gitlab.example.com"
	Project string // ruta completa: "acme/widget" (GH) o "grupo/sub/proy" (GL)
	Owner   string // propietario/grupo inmediato (penúltimo segmento)
	Name    string // nombre del repositorio (último segmento)
	// ClonePrefix es el relative URL root de la instancia: la subcarpeta en la
	// que vive el forge ("git" para un GitLab self-managed bajo /git/), vacía
	// si está en la raíz del host. Viaja en la ref y no como parámetro suelto
	// porque es parte de la identidad de la URL: es lo que distingue dos
	// proyectos con la misma ruta en hosts distintos, y lo que permite
	// reconstruir la URL web sin volver a mirar la config.
	ClonePrefix string
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

// WebURL arma la URL web del repo aplicando el prefijo de subcarpeta de la
// instancia. Es la URL que se abre en el navegador y la base sobre la que se
// arma un enlace de comparación. Una ref sin host o sin project devuelve ""
// (no media URL) en vez de una raíz sin proyecto.
func WebURL(ref RepoRef) string {
	project := strings.TrimSuffix(ref.Project, ".git")
	// normalizeHost también acá, y no solo en el parser: WebURL es pública y
	// una ref construida a mano ("GitHub.com") emitiría una URL con el host en
	// mayúsculas, que el round-trip del parser no detectaría porque nunca ve
	// una ref así.
	host := normalizeHost(ref.Host)
	base := strings.Trim(ref.ClonePrefix, "/")
	if host == "" || project == "" {
		return ""
	}
	if base == "" {
		return fmt.Sprintf("https://%s/%s", host, project)
	}
	return fmt.Sprintf("https://%s/%s/%s", host, base, project)
}

// normalizeHost baja a minúsculas: el nombre de un host no distingue mayúsculas
// y un remote escrito como git@GitHub.com tiene que resolver igual.
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
