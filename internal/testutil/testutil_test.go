package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El marcador es el contrato entre el fixture y el código que se prueba, así que
// el helper tiene que ser fiel: una clave con valor "" se OMITE, no se escribe
// como `name = ""`. Si se escribiera, el discovery leería un proyecto sin nombre
// (o sin grupo) y el test que lo usara estaría probando otra cosa sin que nadie lo
// supiera: el fallo aparecería en el código de producción, no en el fixture.
func TestMarkerOmiteLasClavesVacias(t *testing.T) {
	for _, c := range []struct {
		nombre                   string
		name, primary, secondary string
		quiere                   []string
		noQuiere                 []string
	}{
		{
			nombre: "los tres", name: "api", primary: "vsocial", secondary: "backend",
			quiere:   []string{`name = "api"`, `primary_group = "vsocial"`, `secondary_group = "backend"`},
			noQuiere: nil,
		},
		{
			nombre: "sin nombre", name: "", primary: "vsocial", secondary: "backend",
			quiere:   []string{`primary_group = "vsocial"`, `secondary_group = "backend"`},
			noQuiere: []string{"name"},
		},
		{
			nombre: "sin grupos", name: "api", primary: "", secondary: "",
			quiere:   []string{`name = "api"`},
			noQuiere: []string{"group"},
		},
		{
			nombre: "solo el primario", name: "api", primary: "vsocial", secondary: "",
			quiere:   []string{`name = "api"`, `primary_group = "vsocial"`},
			noQuiere: []string{"secondary_group"},
		},
		{
			// Solo el secundario, sin primario: se escribe igual. El grupo de un
			// nivel no es un error de quien llama, y el fixture no debe
			// inventarse una regla que el producto no tiene.
			nombre: "solo el secundario", name: "", primary: "", secondary: "backend",
			quiere:   []string{`secondary_group = "backend"`},
			noQuiere: []string{"primary_group", "name"},
		},
		{
			nombre: "nada de nada", name: "", primary: "", secondary: "",
			quiere:   nil,
			noQuiere: []string{"name", "group"},
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			dir := t.TempDir()
			Marker(t, dir, c.name, c.primary, c.secondary, false)

			raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
			if err != nil {
				t.Fatalf("el marcador no se escribió: %v", err)
			}
			got := string(raw)
			for _, clave := range c.quiere {
				if !strings.Contains(got, clave) {
					t.Errorf("falta %s en el marcador:\n%s", clave, got)
				}
			}
			for _, clave := range c.noQuiere {
				if strings.Contains(got, clave) {
					t.Errorf("%s aparece en el marcador y debía omitirse:\n%s", clave, got)
				}
			}
		})
	}
}

// Y el marcador malformado tiene que ser TOML inválido de verdad, no un caso
// raro: es lo que hace que la app avise en vez de tragarse el fichero y descubrir
// repos que el usuario no tiene.
func TestMarkerMalformadoEsTOMLInvalido(t *testing.T) {
	dir := t.TempDir()
	Marker(t, dir, "api", "vsocial", "backend", true)
	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatalf("no se escribió el marcador: %v", err)
	}
	// El fixture no declara un parser, así que la prueba es que NO es TOML
	// válido: un array sin cerrar.
	if !strings.Contains(string(raw), "[roto") {
		t.Errorf("el marcador malformado no parece malformado:\n%s", raw)
	}
}
