package forge

import (
	"net/url"
	"strings"
)

// ParseRemoteURL normaliza la URL cruda de un remote a un RepoRef. Reconoce la
// forma SCP (git@host:owner/repo.git) y las URL con esquema, y descarta el
// sufijo .git, la barra final y el puerto.
//
// hosts mapea host → forge: un host que no está ahí devuelve false. Es
// deliberado, porque "no lo conozco" y "es una ruta local" tienen que
// distinguirse: la primera es un repo sin forge (todavía no configurado) y la
// segunda no es un remote y ni siquiera tiene host.
//
// prefixes mapea host → relative URL root de la instancia. El prefijo se quita
// del path para que un remoto en subcarpeta (https://host/git/grupo/proy.git)
// normalice al mismo Project que uno en la raíz: si no, la misma instancia
// daría dos proyectos distintos según cómo se clonara. El prefijo configurado
// se guarda en la ref aunque el path no lo traiga, porque es el de la URL web y
// no el del remoto.
func ParseRemoteURL(raw string, hosts, prefixes map[string]string) (RepoRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RepoRef{}, false
	}

	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		// Host está vacío en file:///… y en otros esquemas sin servidor: no hay
		// forge al que atribuir el repo.
		if err != nil || u.Host == "" {
			return RepoRef{}, false
		}
		// Hostname() y no Host: el puerto (ssh://git@host:2222/o/r) no es parte
		// de la identidad, y dejarlo rompería tanto el lookup de hosts como la
		// URL web.
		host = u.Hostname()
		path = u.Path
	} else if at := strings.Index(raw, "@"); at >= 0 {
		rest := raw[at+1:]
		colon := strings.Index(rest, ":")
		// `colon < 1`, no `colon < 0`: un `:` en la posición 0 deja el HOST
		// vacío, y un remote sin host no identifica nada. Aceptarlo produce un
		// RepoRef con Host "" que resuelve por el mapa y sale como `-R owner/repo`
		// en el argv de `gh pr create`: un PR contra un repo que no sabe quién
		// es. El caso es raro pero el daño es un PR en el sitio equivocado.
		if colon < 1 {
			return RepoRef{}, false
		}
		host, path = rest[:colon], rest[colon+1:]
	} else {
		// Sin esquema ni usuario@host: no es un remote (una ruta local, o nada).
		return RepoRef{}, false
	}
	host = normalizeHost(host)

	path = strings.Trim(strings.TrimSuffix(strings.TrimSpace(path), ".git"), "/")
	prefix := strings.Trim(prefixes[host], "/")
	if prefix != "" {
		path = strings.TrimPrefix(path, prefix+"/")
	}
	// Menos de dos segmentos no describe un proyecto: un owner sin repo, o el
	// prefijo solo.
	parts := strings.Split(path, "/")
	forge, ok := hosts[host]
	if !ok || forge == "" || len(parts) < 2 {
		return RepoRef{}, false
	}
	// Un segmento vacío (doble barra) haría que Owner y Name no significen lo
	// que dicen: mejor no normalizar que normalizar a medias.
	for _, p := range parts {
		if p == "" {
			return RepoRef{}, false
		}
	}
	return RepoRef{
		Forge:   forge,
		Host:    host,
		Project: strings.Join(parts, "/"),
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}, true
}

// PrefixFromAPIBase deriva el relative URL root de la instancia a partir del
// api_base REST de GitLab, quitando el sufijo "api/v4": "/git/api/v4/" →
// "git", "/api/v4/" → "" (la instancia vive en la raíz del host).
//
// Devuelve "" cuando el api_base está vacío o no tiene esa forma, en vez de
// adivinar. Un prefijo inventado manda todas las URLs web a una ruta que no
// existe, mientras que la raíz solo cuesta el caso raro de un self-managed en
// subcarpeta, y ese está en la config, no en la inferencia.
func PrefixFromAPIBase(apiBase string) string {
	p := strings.Trim(strings.TrimSpace(apiBase), "/")
	const rest = "api/v4"
	// Un api_base absoluto no es un relative URL root: usarlo tal cual
	// produciría una URL con el esquema duplicado.
	if p == "" || strings.Contains(p, "://") {
		return ""
	}
	if p == rest {
		return ""
	}
	if strings.HasSuffix(p, "/"+rest) {
		// El Trim inicial ya quitó las barras de los extremos, así que el
		// prefijo sale limpio.
		return strings.TrimSuffix(p, "/"+rest)
	}
	return ""
}
