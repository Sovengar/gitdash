package forge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitdash/internal/forge/tool"
)

// echoArgs imprime argc y después cada argumento, separados por NUL. El NUL es
// el separador porque es el único byte que exec no deja pasar dentro de un
// argumento: es lo que permite afirmar que un cuerpo con saltos de línea llega
// entero, cosa que un separador de línea no podría demostrar.
const echoArgs = `#!/bin/sh
printf 'argc=%s\n' "$#"
printf '%s\0' "$@"
`

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// diffArgv compara elemento por elemento y señala el primero que difiere. Un
// argv que "casi" cuadra es el que rompe en silencio: una -b donde iba una -B
// crea el PR contra otra rama y nada falla visiblemente.
func diffArgv(got, want []string) string {
	for i := 0; i < len(got) || i < len(want); i++ {
		switch {
		case i >= len(got):
			return fmt.Sprintf("argv[%d] falta: quiero %q (len %d < %d)", i, want[i], len(got), len(want))
		case i >= len(want):
			return fmt.Sprintf("argv[%d] = %q sobra (len %d > %d)", i, got[i], len(got), len(want))
		case got[i] != want[i]:
			return fmt.Sprintf("argv[%d] = %q, quiero %q", i, got[i], want[i])
		}
	}
	return ""
}

// La forma exacta del argv de cada CLI, elemento por elemento. Las dos
// referencias de los tests viven en parse_test.go.
func TestBuildCreateArgv(t *testing.T) {
	full := Params{
		Title:  "Corrige el pull del overlay",
		Body:   "Cuerpo del PR",
		Base:   "main",
		Head:   "feat/pr-opening",
		Draft:  true,
		Labels: []string{"bug", "tui"},
	}
	cases := []struct {
		name string
		ref  RepoRef
		p    Params
		want []string
	}{
		{
			name: "github mínimo: título, cuerpo y base",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "acme/widget"},
		},
		{
			name: "github completo: draft y labels repetibles",
			ref:  refGH("acme/widget"),
			p:    full,
			want: []string{
				"gh", "pr", "create",
				"-t", "Corrige el pull del overlay",
				"-b", "Cuerpo del PR",
				"-B", "main",
				"-H", "feat/pr-opening",
				"-d",
				"-l", "bug", "-l", "tui",
				"-R", "acme/widget",
			},
		},
		{
			name: "github sin base ni head: no aparecen",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-R", "acme/widget"},
		},
		{
			name: "github sin host: -R es owner/repo pelado",
			ref:  RepoRef{Forge: ForgeGitHub, Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "acme/widget"},
		},
		{
			name: "github enterprise: -R lleva el host delante",
			ref:  RepoRef{Forge: ForgeGitHub, Host: "github.example.com", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "github.example.com/acme/widget"},
		},
		{
			name: "gitlab mínimo: -d es el cuerpo y -b la base",
			ref:  refGL("grupo/sub/proy"),
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"glab", "mr", "create", "-t", "T", "-d", "C", "-b", "main", "-y", "-R", "grupo/sub/proy"},
		},
		{
			name: "gitlab completo: draft largo y labels repetibles",
			ref:  refGL("grupo/sub/proy"),
			p:    full,
			want: []string{
				"glab", "mr", "create",
				"-t", "Corrige el pull del overlay",
				"-d", "Cuerpo del PR",
				"-b", "main",
				"-s", "feat/pr-opening",
				"--draft",
				"-l", "bug", "-l", "tui",
				"-y",
				"-R", "grupo/sub/proy",
			},
		},
		{
			name: "gitlab sin base ni head: no aparecen",
			ref:  refGL("grupo/sub/proy"),
			p:    Params{Title: "T", Body: "C"},
			want: []string{"glab", "mr", "create", "-t", "T", "-d", "C", "-y", "-R", "grupo/sub/proy"},
		},
		{
			// El prefijo de subcarpeta es del host, no del proyecto: -R recibe la
			// ruta dentro de la instancia, que ParseRemoteURL ya dejó sin prefijo.
			name: "gitlab en subcarpeta: -R no lleva el prefijo",
			ref:  RepoRef{Forge: ForgeGitLab, Host: "gitlab.example.com", Project: "grupo/proy", ClonePrefix: "git"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"glab", "mr", "create", "-t", "T", "-d", "C", "-b", "main", "-y", "-R", "grupo/proy"},
		},
		{
			name: "forge desconocido",
			ref:  RepoRef{Forge: "bitbucket", Host: "bitbucket.org", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: nil,
		},
		{
			name: "forge vacío",
			ref:  RepoRef{Host: "github.com", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: nil,
		},
		{
			name: "ref cero",
			ref:  RepoRef{},
			p:    Params{Title: "T", Body: "C"},
			want: nil,
		},
		{
			name: "forge con mayúsculas y espacios",
			ref:  RepoRef{Forge: " GitHub ", Host: "github.com", Project: "acme/widget"},
			p:    Params{Title: "T", Body: "C", Base: "main"},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-B", "main", "-R", "acme/widget"},
		},
		{
			name: "sin labels: ningún -l",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Labels: []string{}},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-R", "acme/widget"},
		},
		{
			name: "labels sucios: se recortan y los vacíos se descartan",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Labels: []string{" bug ", "", "   ", "tui"}},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-l", "bug", "-l", "tui", "-R", "acme/widget"},
		},
		{
			name: "labels repetidos se repiten, no se fusionan",
			ref:  refGH("acme/widget"),
			p:    Params{Title: "T", Body: "C", Labels: []string{"a", "a"}},
			want: []string{"gh", "pr", "create", "-t", "T", "-b", "C", "-l", "a", "-l", "a", "-R", "acme/widget"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if d := diffArgv(BuildCreateArgv(tc.ref, tc.p), tc.want); d != "" {
				t.Fatalf("%s", d)
			}
		})
	}
}

// El cuerpo vacío se emite IGUAL: es el flag que quita la pregunta. Si se
// omitiera, gh y glab abrirían editor o prompt, y gitdash no tiene TTY que
// ceder. La base y el head sí se omiten cuando vienen vacíos porque sus CLIs
// tienen un default sano y un valor vacío ahí es un error, no una omisión.
func TestBuildCreateArgvEmiteElCuerpoVacio(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  RepoRef
		want []string
	}{
		{"github", refGH("acme/widget"), []string{"gh", "pr", "create", "-t", "", "-b", "", "-R", "acme/widget"}},
		{"gitlab", refGL("grupo/proy"), []string{"glab", "mr", "create", "-t", "", "-d", "", "-y", "-R", "grupo/proy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if d := diffArgv(BuildCreateArgv(tc.ref, Params{}), tc.want); d != "" {
				t.Fatalf("%s", d)
			}
		})
	}
}

// LA TRAMPA CENTRAL: en gh -b es el cuerpo y -B la base; en glab -b es la base
// y -d la descripción. Cruzarlas no da error, crea el PR contra otra rama o
// manda el texto equivocado al forge. Este test mira qué flag precede
// realmente a cada valor, no la forma del argv entero.
func TestBuildCreateArgvNoCruzaCuerpoYBase(t *testing.T) {
	p := Params{Title: "TITULO", Body: "CUERPO", Base: "BASE", Head: "HEAD"}
	cases := []struct {
		name     string
		ref      RepoRef
		wantBody string
		wantBase string
		wantHead string
	}{
		{"gh", refGH("acme/widget"), "-b", "-B", "-H"},
		{"glab", refGL("grupo/proy"), "-d", "-b", "-s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv := BuildCreateArgv(tc.ref, p)
			for _, c := range []struct{ value, want string }{
				{"CUERPO", tc.wantBody},
				{"BASE", tc.wantBase},
				{"HEAD", tc.wantHead},
				{"TITULO", "-t"},
			} {
				if got := flagBefore(argv, c.value); got != c.want {
					t.Errorf("el flag delante de %q es %q, quiero %q (argv: %q)", c.value, got, c.want, argv)
				}
			}
		})
	}
}

// flagBefore devuelve el flag que precede a value, o "" si no lo precede
// ninguno. Cada valor tiene que aparecer una vez como valor de flag y solo
// como valor de flag, así que el primer flag que lo precede es el bueno.
func flagBefore(argv []string, value string) string {
	for i := 1; i < len(argv); i++ {
		if argv[i] == value && strings.HasPrefix(argv[i-1], "-") {
			return argv[i-1]
		}
	}
	return ""
}

// glab NO tiene --hostname en `mr create` (sí lo tiene en `auth login`):
// verificado contra glab 1.119.0, que responde "Unknown flag: --hostname" y
// muere antes de hacer nada. La instancia se elige con GITLAB_HOST en el
// entorno (PromptEnv). Si este argv metiera el flag, gitdash no podría crear
// ni un MR en la máquina del usuario.
func TestBuildCreateArgvNoPasaHostnameALaCli(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  RepoRef
	}{
		{"github", refGH("acme/widget")},
		{"gitlab", RepoRef{Forge: ForgeGitLab, Host: "umane.emeal.nttdata.com", Project: "grupo/proy", ClonePrefix: "git"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv := BuildCreateArgv(tc.ref, Params{Title: "T", Body: "C", Base: "main", Head: "h"})
			for _, bad := range []string{"--hostname", "--api-host", "--yes-and-no"} {
				if slices.Contains(argv, bad) {
					t.Errorf("argv %q contiene %q", argv, bad)
				}
			}
		})
	}
}

// glab necesita -y porque el flag es el que salta la confirmación de envío:
// sin él, aunque título y descripción vengan explícitos, glab pregunta. Es el
// equivalente funcional del "todo explícito" de gh, que no lo necesita.
func TestBuildCreateArgvGlabSaltaLaConfirmacion(t *testing.T) {
	argv := BuildCreateArgv(refGL("grupo/proy"), Params{Title: "T", Body: "C", Base: "main"})
	if !slices.Contains(argv, "-y") {
		t.Errorf("argv de glab %q no lleva -y: glab pediría confirmación de envío", argv)
	}
}

// Un título con espacios, comillas y metacaracteres de shell tiene que llegar
// como UN elemento de argv. El test es end-to-end a propósito: pasa el argv
// por un proceso real y lee lo que el kernel le entregó, en vez de comprobar
// que un string se ve bien en un diff. Un título con comillas o $(...) que se
// partiera sería una inyección en la CLI.
func TestBuildCreateArgvUnValorEsUnSoloElemento(t *testing.T) {
	nastyTitle := "arregla el \"pull\" & $(whoami) `id` ; rm -rf / | tee $(pwd) && echo \"fin\""
	nastyBody := "línea 1\nlínea 2\tcon tabulador, \"comillas\" y $HOME"

	for _, tc := range []struct {
		name string
		ref  RepoRef
		base string
		head string
	}{
		{"gh", refGH("acme/widget"), "-B", "-H"},
		{"glab", refGL("grupo/proy"), "-b", "-s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv := BuildCreateArgv(tc.ref, Params{
				Title:  nastyTitle,
				Body:   nastyBody,
				Base:   "main",
				Head:   "feat/x",
				Labels: []string{"con espacio y \"comilla\""},
			})
			r := &tool.Runner{Bin: writeScript(t, echoArgs)}
			out, err := r.Run(context.Background(), argv...)
			if err != nil {
				t.Fatalf("el stub falló: %v", err)
			}
			got := parseEchoedArgs(t, out)
			// argc del proceso = len(argv). Si un valor se hubiera partido o
			// pegado, el conteo no cuadra y el elemento con el título no
			// aparece entero.
			if len(got) != len(argv) {
				t.Fatalf("el proceso recibió %d elementos, el argv tiene %d:\n%s", len(got), len(argv), out)
			}
			for i := range argv {
				if got[i] != argv[i] {
					t.Fatalf("argv[%d]: el proceso recibió %q, se pasó %q", i, got[i], argv[i])
				}
			}
			// Y el título entero aparece una sola vez, como elemento propio.
			if n := countEqual(got, nastyTitle); n != 1 {
				t.Errorf("el título llegó %d veces como elemento entero, quiero 1", n)
			}
			if flagBefore(argv, nastyTitle) != "-t" {
				t.Errorf("el título no llega detrás de -t: %q", argv)
			}
		})
	}
}

func parseEchoedArgs(t *testing.T, out string) []string {
	t.Helper()
	_, payload, ok := strings.Cut(out, "\n")
	if !ok {
		t.Fatalf("el stub no imprimió argc: %q", out)
	}
	// El NUL final es el que deja el printf del stub; se quita para no
	// inventar un argumento vacío.
	payload = strings.TrimSuffix(payload, "\x00")
	return strings.Split(payload, "\x00")
}

func countEqual(got []string, want string) int {
	n := 0
	for _, g := range got {
		if g == want {
			n++
		}
	}
	return n
}

// El binario que crea el PR sale del forge, y no hay binario para uno que no
// soportamos: el "" es lo que corta antes de intentar ejecutar nada.
func TestCreateBin(t *testing.T) {
	cases := []struct {
		ref  RepoRef
		want string
	}{
		{refGH("acme/widget"), "gh"},
		{refGL("grupo/proy"), "glab"},
		{RepoRef{Forge: " GitLab "}, "glab"},
		{RepoRef{Forge: "bitbucket"}, ""},
		{RepoRef{}, ""},
	}
	for _, tc := range cases {
		if got := CreateBin(tc.ref); got != tc.want {
			t.Errorf("CreateBin(%q) = %q, quiero %q", tc.ref.Forge, got, tc.want)
		}
	}
}

// El host viaja en el entorno, no en el argv: es lo único que le dice a cada
// CLI contra qué instancia y con qué token trabaja.
func TestPromptEnv(t *testing.T) {
	cases := []struct {
		name string
		ref  RepoRef
		want []string
	}{
		{"github", refGH("acme/widget"), []string{"GH_PROMPT_DISABLED=1"}},
		{
			name: "gitlab self-managed",
			ref:  RepoRef{Forge: ForgeGitLab, Host: "umane.emeal.nttdata.com", Project: "grupo/proy"},
			want: []string{"GITLAB_HOST=umane.emeal.nttdata.com"},
		},
		{"gitlab sin host: sin host no hay a qué apuntar", RepoRef{Forge: ForgeGitLab, Project: "grupo/proy"}, nil},
		{"forge no soportado", RepoRef{Forge: "bitbucket"}, nil},
		{"ref vacía", RepoRef{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PromptEnv(tc.ref); !slices.Equal(got, tc.want) {
				t.Errorf("PromptEnv = %q, quiero %q", got, tc.want)
			}
		})
	}
}
