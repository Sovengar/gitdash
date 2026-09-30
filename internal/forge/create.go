package forge

import "strings"

// Params son los datos con los que se crea un PR/MR. Todo lo que la CLI
// aceptaría preguntar viaja aquí explícito: esa es la condición para que la
// creación no necesite TTY y se pueda lanzar como un exec capturado, sin
// handoff de terminal como el de `pull_ai` o `lazygit`.
type Params struct {
	Title  string
	Body   string
	Base   string
	Head   string
	Draft  bool
	Labels []string
}

// CreateBin devuelve el binario que crea el PR/MR del ref, o "" para un forge
// que no soportamos. El "" no es un caso raro: es lo que corta antes de
// intentar ejecutar nada.
func CreateBin(ref RepoRef) string {
	switch normalizeForge(ref.Forge) {
	case ForgeGitHub:
		return "gh"
	case ForgeGitLab:
		return "glab"
	default:
		return ""
	}
}

// PromptEnv devuelve las variables que quitan cualquier pregunta de la CLI del
// ref, y el host contra el que tiene que trabajar.
//
// El host va en el entorno y no en el argv a propósito: `glab mr create` no
// tiene flag `--hostname` (sí lo tiene `auth login`, verificado contra glab
// 1.119.0, que responde "Unknown flag: --hostname" y muere antes de hacer nada)
// y su forma documentada de elegir instancia es GITLAB_HOST, que resuelve
// contra qué api_base y qué token se habla. Para una instancia self-managed en
// subcarpeta como la del usuario (host/git/) es lo que evita que glab hable con
// gitlab.com.
func PromptEnv(ref RepoRef) []string {
	switch normalizeForge(ref.Forge) {
	case ForgeGitHub:
		return []string{"GH_PROMPT_DISABLED=1"}
	case ForgeGitLab:
		if h := normalizeHost(ref.Host); h != "" {
			return []string{"GITLAB_HOST=" + h}
		}
		return nil
	default:
		return nil
	}
}

// BuildCreateArgv arma el argv completo —argv[0] incluido— para crear el PR/MR
// del ref, o nil si el forge no está soportado. Un argv "best effort" para un
// forge desconocido sería peor que ninguno: gh y glab no comparten ni un flag
// con sentido, así que lo que saldría es la puerta de otro.
//
// -R va siempre, en las dos CLIs, para que la ref resuelta sea la única fuente
// de verdad: si se deja que la CLI lo deduzca del remote, un PR puede acabar
// creado contra otro repositorio del que la fila de la tabla afirma, sin que
// nada falle visiblemente. gh lo formatea [HOST/]OWNER/REPO, así que en un host
// distinto de github.com el host viaja delante; glab toma solo la ruta dentro de
// la instancia, que ya viene sin el prefijo de subcarpeta porque lo quita
// ParseRemoteURL.
func BuildCreateArgv(ref RepoRef, p Params) []string {
	switch normalizeForge(ref.Forge) {
	case ForgeGitHub:
		return ghCreateArgv(ref, p)
	case ForgeGitLab:
		return glabCreateArgv(ref, p)
	default:
		return nil
	}
}

// ghCreateArgv arma `gh pr create`.
//
// LA TRAMPA: en gh -b es el CUERPO y -B es la BASE, y se distinguen solo por el
// tamaño. Cruzarlas no da error: manda el texto como base y el PR sale contra
// una rama que no existe (o contra la que fuera por defecto). Los tests lo
// comprueban sobre qué flag precede a cada valor, no sobre la forma del argv
// entero.
func ghCreateArgv(ref RepoRef, p Params) []string {
	argv := []string{"gh", "pr", "create", "-t", p.Title}
	// El cuerpo se emite siempre, incluso vacío: es el flag que quita la
	// pregunta. Omitirlo dejaría a gh abrir editor o prompt, y gitdash no tiene
	// TTY que ceder.
	argv = append(argv, "-b", p.Body)
	// Base y head, en cambio, solo si vienen: sus CLIs tienen un default sano y
	// un valor vacío ahí es un error, no una omisión.
	if p.Base != "" {
		argv = append(argv, "-B", p.Base)
	}
	if p.Head != "" {
		argv = append(argv, "-H", p.Head)
	}
	if p.Draft {
		argv = append(argv, "-d")
	}
	// -l se repite en vez de listar con comas: un label con coma es legal en
	// ambos forges, y la lista con comas partiría ese label en dos.
	argv = append(argv, labelArgs(p.Labels)...)
	return append(argv, "-R", ghRepoArg(ref))
}

// glabCreateArgv arma `glab mr create`.
//
// LA MISMA TRAMPA, AL REVÉS: en glab -b es la BASE (--target-branch) y -d es
// la DESCRIPCIÓN. El nombre del flag es el que dice lo contrario en cada CLI,
// así que el mapa está uno al lado del otro a propósito: leerlos por analogía
// con gh es justo el error que hace este comentario.
func glabCreateArgv(ref RepoRef, p Params) []string {
	argv := []string{"glab", "mr", "create", "-t", p.Title}
	argv = append(argv, "-d", p.Body)
	if p.Base != "" {
		argv = append(argv, "-b", p.Base)
	}
	if p.Head != "" {
		argv = append(argv, "-s", p.Head)
	}
	if p.Draft {
		// Largo a propósito: glab no tiene corto para draft (--wip sería otro
		// flag con otro nombre para lo mismo).
		argv = append(argv, "--draft")
	}
	argv = append(argv, labelArgs(p.Labels)...)
	// -y es el equivalente funcional del "todo explícito" de gh: salta la
	// confirmación de envío, que glab pediría aunque título, descripción y base
	// vengan dados. El repo vacío sale igual: `-R ""` no significaría nada, y un
	// "-y" suelto es una invocación que ya no se puede atribuir a nada.
	if ref.Project != "" {
		argv = append(argv, "-y", "-R", ref.Project)
	} else {
		argv = append(argv, "-y")
	}
	return argv
}

// labelArgs repite `-l <label>` por cada label, en el orden en que vienen.
// Los vacíos se descartan: un `-l ""` lo acepta la CLI y lo rechaza la API, que
// es un fallo que se manifiesta mucho después y sin decir qué label era.
func labelArgs(labels []string) []string {
	var out []string
	for _, l := range labels {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, "-l", l)
		}
	}
	return out
}

// ghRepoArg arma el valor de -R para gh: [HOST/]OWNER/REPO. El host solo hace
// falta cuando no es github.com, porque ahí es lo que le dice a gh que no
// interprete la ruta como del sitio público.
func ghRepoArg(ref RepoRef) string {
	host := normalizeHost(ref.Host)
	if host != "" && host != "github.com" {
		return host + "/" + strings.Trim(ref.Project, "/")
	}
	return strings.Trim(ref.Project, "/")
}

// normalizeForge baja a minúsculas y recorta, igual que normalizeHost lo hace
// con el host: el forge viene de un mapa de config que escribió una persona, y
// un "GitHub" que no casara con la constante dejaría la acción sin puerta y sin
// decir por qué.
func normalizeForge(forge string) string {
	return strings.ToLower(strings.TrimSpace(forge))
}
