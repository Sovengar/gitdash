// Tests de la normalización de un remote a RepoRef.
package forge

import (
	"strings"
	"testing"
)

// hosts de los tests: un GitHub y un GitLab self-managed.
func testHosts() map[string]string {
	return map[string]string{
		"github.com":         ForgeGitHub,
		"gitlab.example.com": ForgeGitLab,
	}
}

// refGL construye la referencia esperada de un proyecto GitLab del host de
// pruebas, con el prefijo de subcarpeta ya aplicado.
func refGL(project string) RepoRef {
	parts := strings.Split(project, "/")
	return RepoRef{
		Forge:   ForgeGitLab,
		Host:    "gitlab.example.com",
		Project: project,
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}
}

// La tabla de formas de remote: scp, esquemas, sin .git, barra final, puerto,
// subgrupos y los tres negativos (host desconocido, ruta local, vacío).
func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		hosts    map[string]string
		prefixes map[string]string
		want     RepoRef
		wantOK   bool
	}{
		{
			name:   "scp",
			raw:    "git@github.com:acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "scp sin .git",
			raw:    "git@github.com:acme/widget",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			// La @ en la posición 0: el usuario vacío es legal en scp (ssh
			// toma el usuario actual) y es justo el borde de `at >= 0`. Con un
			// `at > 0` esta remote cae al caso "sin usuario@host" y se pierde
			// un repo que sí tiene forge.
			name:   "scp con usuario vacío",
			raw:    "@github.com:acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "https",
			raw:    "https://github.com/acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "ssh sin .git",
			raw:    "ssh://git@github.com/acme/widget",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "barra final",
			raw:    "https://github.com/acme/widget/",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "espacios alrededor",
			raw:    "  https://github.com/acme/widget.git\n",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "host en minúsculas",
			raw:    "git@GitHub.com:acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "puerto en la url se descarta",
			raw:    "ssh://git@github.com:2222/acme/widget.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"},
			wantOK: true,
		},
		{
			name:   "owner y name con puntos y guiones",
			raw:    "https://github.com/acme-corp/widget.js.git",
			want:   RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: "acme-corp/widget.js", Owner: "acme-corp", Name: "widget.js"},
			wantOK: true,
		},
		{
			name:   "gitlab con subgrupo",
			raw:    "https://gitlab.example.com/grupo/sub/proy.git",
			want:   RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com", Project: "grupo/sub/proy", Owner: "sub", Name: "proy"},
			wantOK: true,
		},
		{
			name:     "gitlab en subcarpeta de instancia",
			raw:      "https://gitlab.example.com/git/grupo/sub/proy.git",
			prefixes: map[string]string{"gitlab.example.com": "git"},
			want:     refGL("grupo/sub/proy"),
			wantOK:   true,
		},
		{name: "host desconocido", raw: "https://bitbucket.org/acme/widget.git", wantOK: false},
		{name: "ruta local", raw: "/home/u/dev/widget", wantOK: false},
		{name: "ruta local con esquema file", raw: "file:///home/u/dev/widget", wantOK: false},
		{name: "vacío", raw: "", wantOK: false},
		{name: "sin owner ni repo", raw: "https://github.com/acme.git", wantOK: false},
		{name: "solo owner", raw: "https://github.com/", wantOK: false},
		{name: "esquema sin host", raw: "https:///acme/widget.git", wantOK: false},
		{name: "scp sin path", raw: "git@github.com:", wantOK: false},
		{name: "user sin host", raw: "git@", wantOK: false},
		{name: "hosts vacío", raw: "https://github.com/acme/widget.git", hosts: map[string]string{}, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hosts := testHosts()
			if tc.hosts != nil {
				hosts = tc.hosts
			}
			got, ok := ParseRemoteURL(tc.raw, hosts, tc.prefixes)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quiero %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("ref = %+v, quiero %+v", got, tc.want)
			}
		})
	}
}

// El prefijo de subcarpeta se quita del path: un remoto "en subcarpeta"
// normaliza al mismo Project que uno en la raíz. Si no se quitara, la misma
// instancia daría dos proyectos distintos según cómo se clonara.
func TestParseRemoteURLStripsClonePrefix(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": ForgeGitLab}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	want := refGL("grupo/sub/proy")

	cases := []struct {
		name string
		raw  string
	}{
		{"sin prefijo", "https://gitlab.example.com/grupo/sub/proy.git"},
		{"con prefijo", "https://gitlab.example.com/git/grupo/sub/proy.git"},
		{"con prefijo y barra final", "https://gitlab.example.com/git/grupo/sub/proy/"},
		{"scp con prefijo", "git@gitlab.example.com:git/grupo/sub/proy.git"},
		{"scp con prefijo sin .git", "git@gitlab.example.com:git/grupo/sub/proy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(tc.raw, hosts, prefixes)
			if !ok {
				t.Fatalf("no parseó %q", tc.raw)
			}
			if got != want {
				t.Fatalf("ref = %+v, quiero %+v", got, want)
			}
		})
	}
}

// El prefijo configurado se guarda en la ref aunque el remoto no lo traiga: es
// lo que hace falta para reconstruir la URL web de una instancia en
// subcarpeta, y el prefijo de la URL web es el configurado, no el del remoto.
func TestParseRemoteURLKeepsConfiguredPrefix(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": ForgeGitLab}
	prefixes := map[string]string{"gitlab.example.com": "/git/"}

	got, ok := ParseRemoteURL("https://gitlab.example.com/grupo/proy.git", hosts, prefixes)
	if !ok {
		t.Fatal("no parseó")
	}
	if got.Project != "grupo/proy" {
		t.Fatalf("Project = %q, quiero %q (un path sin el prefijo no se toca)", got.Project, "grupo/proy")
	}
	if got.Owner != "grupo" || got.Name != "proy" {
		t.Fatalf("Owner/Name = %q/%q, quiero grupo/proy", got.Owner, got.Name)
	}
}

// Derivación del relative URL root desde el api_base de GitLab. Un api_base con
// forma inesperada devuelve raíz: adivinar el prefijo manda toda URL web a una
// ruta que no existe.
func TestPrefixFromAPIBase(t *testing.T) {
	cases := []struct {
		name    string
		apiBase string
		want    string
	}{
		{"instancia en subcarpeta", "/git/api/v4/", "git"},
		{"instancia en raíz", "/api/v4/", ""},
		{"instancia en subcarpeta sin barras", "git/api/v4", "git"},
		{"subcarpeta con más niveles", "/custom/api/v4/", "custom"},
		{"vacío", "", ""},
		{"api_base inesperado: v3", "/git/api/v3/", ""},
		{"api_base inesperado: subruta", "/api/v4/projects", ""},
		{"api_base inesperado: solo prefijo api", "/git/api/", ""},
		{"api_base absoluto", "https://gitlab.example.com/api/v4/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrefixFromAPIBase(tc.apiBase); got != tc.want {
				t.Fatalf("PrefixFromAPIBase(%q) = %q, quiero %q", tc.apiBase, got, tc.want)
			}
		})
	}
}
