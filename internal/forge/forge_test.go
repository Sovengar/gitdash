// Tests del reconhecimento de un host por su forge (ForgeForHost). La identidad
// completa del repo (Forge, Host, Project, Owner, Name) la prueba parse_test.go.
package forge

import "testing"

// refGH construye la referencia de un proyecto de GitHub. La usan también los
// tests del argv de creación.
func refGH(project string) RepoRef {
	return RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: project, Owner: "acme", Name: "widget"}
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

// PublicHosts devuelve una COPIA: quien la mezcla con lo declarado por el usuario
// no puede cambiar la tabla de reconocimiento de forge por el camino.
func TestPublicHostsEsCopia(t *testing.T) {
	hosts := PublicHosts()
	if len(hosts) != 2 {
		t.Fatalf("PublicHosts() = %v, want los dos públicos", hosts)
	}
	delete(hosts, "github.com")
	if _, ok := PublicHosts()["github.com"]; !ok {
		t.Error("borrar de la copia eliminó el host de la tabla")
	}
	if _, ok := ForgeForHost("github.com"); !ok {
		t.Error("ForgeForHost dejó de reconocer github.com")
	}
}
