// Tests de la identidad del repo en el forge y de su URL web.
package forge

import "testing"

func refGH(project string) RepoRef {
	return RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: project, Owner: "acme", Name: "widget"}
}

// La URL web es la del repo en el host, con la subcarpeta de la instancia
// aplicada. Es la que se abre en el navegador y la base sobre la que se arma un
// enlace de comparación, así que con la subcarpeta equivocada abre 404 sin que
// nada falle visiblemente.
func TestWebURL(t *testing.T) {
	glRoot := RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com", Project: "grupo/sub/proy", Owner: "sub", Name: "proy"}

	cases := []struct {
		name string
		ref  RepoRef
		want string
	}{
		{"github en raíz", refGH("acme/widget"), "https://github.com/acme/widget"},
		{"gitlab en raíz", glRoot, "https://gitlab.example.com/grupo/sub/proy"},
		{
			name: "gitlab en subcarpeta",
			ref:  RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com", Project: "grupo/sub/proy", Owner: "sub", Name: "proy", ClonePrefix: "git"},
			want: "https://gitlab.example.com/git/grupo/sub/proy",
		},
		{
			name: "prefijo con barras",
			ref:  RepoRef{Host: "github.enterprise.com", Project: "acme/widget", ClonePrefix: "/ent/"},
			want: "https://github.enterprise.com/ent/acme/widget",
		},
		{
			name: "project con .git se recorta",
			ref:  RepoRef{Host: "github.com", Project: "acme/widget.git"},
			want: "https://github.com/acme/widget",
		},
		{"ref vacía", RepoRef{}, ""},
		{"sin project", RepoRef{Host: "github.com", ClonePrefix: "git"}, ""},
		{"sin host", RepoRef{Project: "acme/widget"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WebURL(tc.ref); got != tc.want {
				t.Fatalf("WebURL = %q, quiero %q", got, tc.want)
			}
		})
	}
}

// Round trip: la URL web que arma WebURL vuelve a normalizarse al mismo RepoRef.
// Es lo que hace trustworthy el prefijo de subcarpeta: si la URL web de un repo
// en /git/ no vuelve al mismo proyecto, el prefijo está mal derivado.
func TestWebURLParseRemoteRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		ref      RepoRef
		prefixes map[string]string
	}{
		{
			name: "github en raíz",
			ref:  refGH("acme/widget"),
		},
		{
			name:     "gitlab en subcarpeta",
			ref:      refGL("grupo/sub/proy"),
			prefixes: map[string]string{"gitlab.example.com": "git"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(WebURL(tc.ref), testHosts(), tc.prefixes)
			if !ok || got != tc.ref {
				t.Fatalf("round trip = %+v (%v), quiero %+v", got, ok, tc.ref)
			}
		})
	}
}

// Detección de provider por host: solo los hosts públicos se reconocen sin
// configuración. Una instancia self-managed no está en la lista a propósito
// (eso lo aporta la config) y un host desconocido tiene que ser un no, no un
// github por defecto.
func TestForgeForHost(t *testing.T) {
	cases := []struct {
		host string
		want string
		ok   bool
	}{
		{"github.com", ForgeGitHub, true},
		{"gitlab.com", ForgeGitLab, true},
		{"GitHub.com", ForgeGitHub, true},
		{"  github.com  ", ForgeGitHub, true},
		{"gitlab.example.com", "", false},
		{"bitbucket.org", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got, ok := ForgeForHost(tc.host)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ForgeForHost(%q) = %q, %v; quiero %q, %v", tc.host, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Una ref construida a mano no pasa por el parser, así que el host puede venir
// sin normalizar. La URL web tiene que salir igual.
func TestWebURLNormalizaHostDeUnaRefManual(t *testing.T) {
	ref := RepoRef{Forge: ForgeGitHub, Host: "  GitHub.COM ", Project: "acme/widget"}
	if got := WebURL(ref); got != "https://github.com/acme/widget" {
		t.Errorf("WebURL = %q, quiero %q", got, "https://github.com/acme/widget")
	}
}
