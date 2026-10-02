// Tests de los caminos de error de los helpers.
//
// Los helpers de este paquete son lo que la suite entera usa para montar repos
// git de verdad, y casi todas sus ramas son del mismo tipo: la llamada al
// sistema falla. Contra un *testing.T de verdad no hay forma de llegar a ellas,
// porque t.Fatal mata el test que las fuera a cubrir. Por eso los helpers toman
// un TB y aqui se usa MockTB, que registra el fallo en vez de abortar.
//
// El fallo se provoca de verdad, no se falsea: se apunta a un path cuyo padre es
// un FICHERO, y os.MkdirAll devuelve ENOTDIR. Si un dia dejara de fallar, estos
// tests fallarian, que es justo lo que un test tiene que hacer.
package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ficheroComoPadre crea un path cuyo padre inmediato es un fichero normal. Todo
// lo que intente crear un directorio debajo falla con ENOTDIR.
func ficheroComoPadre(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "soy-un-fichero")
	if err := os.WriteFile(f, []byte("contenido"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestInitFallaConDirectorioIlegible(t *testing.T) {
	tb := &MockTB{Temp: tptr(t)}
	Init(tb, filepath.Join(ficheroComoPadre(t), "sub"))

	if !tb.Failed() {
		t.Fatal("Init = sin fallo, want el error de MkdirAll (el padre es un fichero)")
	}
	if !strings.Contains(tb.Failures[0], "not a directory") {
		t.Errorf("fallo = %q, want el de ENOTDIR", tb.Failures[0])
	}
	// Y no se ha creado nada: el helper se detuvo en el mkdir, sin llegar a
	// git init.
	if entries, err := os.ReadDir(filepath.Dir(tb.Failures[0])); err == nil && len(entries) == 0 {
		t.Error("Init creo el directorio pese a fallar el mkdir")
	}
}

func TestCommitFilesFallaConRutaImposible(t *testing.T) {
	dir := t.TempDir()

	// Un fichero anidado cuyo directorio padre es un fichero.
	tb2 := &MockTB{Temp: tptr(t)}
	CommitFiles(tb2, dir, map[string]string{
		filepath.Join("bloque", "hijo.txt"): "x",
	}, "no deberia llegar aqui")
	if !tb2.Failed() {
		t.Error("CommitFiles = sin fallo, want el error de MkdirAll del padre")
	}

	// Y con un directorio ya existente pero sin permiso de escritura en el
	// destino final: el fichero existe como DIRECTORIO, y WriteFile sobre un
	// directorio falla con EISDIR.
	tb3 := &MockTB{Temp: tptr(t)}
	Init(tb3, dir)
	if tb3.Failed() {
		t.Fatalf("el repo base no se pudo crear: %v", tb3.Failures)
	}
	destino := filepath.Join(dir, "sub", "archivo.txt")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	tb4 := &MockTB{Temp: tptr(t)}
	CommitFiles(tb4, dir, map[string]string{"sub/archivo.txt": "contenido"}, "no deberia llegar")
	if !tb4.Failed() {
		t.Error("CommitFiles sobre un directorio = sin fallo, want el error de WriteFile")
	}
}

func TestWriteUncommittedYUntrackedFallan(t *testing.T) {
	t.Run("uncommitted", func(t *testing.T) {
		tb := &MockTB{Temp: tptr(t)}
		WriteUncommitted(tb, ficheroComoPadre(t), map[string]string{"a.txt": "x"})
		if !tb.Failed() {
			t.Error("WriteUncommitted = sin fallo, want el error de escritura")
		}
	})
	t.Run("untracked", func(t *testing.T) {
		tb := &MockTB{Temp: tptr(t)}
		WriteUntracked(tb, ficheroComoPadre(t), map[string]string{"a.txt": "x"})
		if !tb.Failed() {
			t.Error("WriteUntracked = sin fallo, want el error de escritura")
		}
	})
}

func TestMarkerFallaConPathIlegible(t *testing.T) {
	tb := &MockTB{Temp: tptr(t)}
	Marker(tb, ficheroComoPadre(t), "n", "g", "s", false)
	if !tb.Failed() {
		t.Error("Marker = sin fallo, want el error de escritura del marcador")
	}
}

// git() falla cuando el comando no existe o devuelve error. Se provoca con un
// argumento que git no entiende: no es un mock del sistema, es git diciendo que
// no.
func TestGitFallaConComandoInvalido(t *testing.T) {
	tb := &MockTB{Temp: tptr(t)}
	// `--no-existe-esta-opcion` hace que git salga con codigo 129.
	git(tb, t.TempDir(), "no-existe-este-subcomando")
	if !tb.Failed() {
		t.Fatal("git con un subcomando invalido = sin fallo, want el error del proceso")
	}
	if !strings.Contains(tb.Failures[0], "no-existe-este-subcomando") {
		t.Errorf("fallo = %q, want que nombre el comando que se ejecuto", tb.Failures[0])
	}
}

// tptr: los dobles necesitan un testing.T real para TempDir, asi que se les pasa
// el del test que los usa. *testing.T satisface el interface.
func tptr(t *testing.T) testing.TB { return t }

// El doble tiene que saber responder a las tres cosas que TB exige, y dos de
// ellas no se ejercitan al usarlo para cubrir un fallo: TempDir sin Temp (el
// guard), y Failed() de un doble limpio (que debe decir que no).
func TestMockTBRespetaElContrato(t *testing.T) {
	// Sin Temp: TempDir devuelve "" y no revienta. Sin esto, un doble
	// construido a pelo (sin test que delegar) reventaria en NewRepo.
	vacio := &MockTB{}
	if got := vacio.TempDir(); got != "" {
		t.Errorf("TempDir sin Temp = %q, want vacio", got)
	}
	if vacio.Failed() {
		t.Error("un doble recien creado = con fallos, want ninguno")
	}

	// Fatalf con formato: el mensaje sale formateado, con los args puestos.
	vacio.Fatalf("fallo %d de %d", 3, 7)
	if len(vacio.Failures) != 1 {
		t.Fatalf("fallos = %d, want 1", len(vacio.Failures))
	}
	if !strings.Contains(vacio.Failures[0], "fallo 3 de 7") {
		t.Errorf("fallo = %q, want el mensaje formateado", vacio.Failures[0])
	}
	if !vacio.Failed() {
		t.Error("Failed() = false tras un Fatalf, want true")
	}

	// Y Fatal a secas se acumula en vez de sobreescribir: dos fallos son dos
	// fallos, y un helper que falla dos veces debe decir las dos.
	vacio.Fatal("otro fallo")
	if len(vacio.Failures) != 2 {
		t.Errorf("fallos = %d, want 2 (Fatal no pisa el anterior)", len(vacio.Failures))
	}
}

// Los tres helpers que escriben en un sitio que no puede existir. Todos usan el
// mismo patron que ya se prueba arriba (un fichero donde deberia ir el
// directorio), pero cada uno con su llamada: MkdirAll, WriteFile y
// WriteFile sobre un HEAD dentro de un .git que no existe.
func TestHelpersQueEscribenFallan(t *testing.T) {
	t.Run("InitBare", func(t *testing.T) {
		tb := &MockTB{Temp: tptr(t)}
		InitBare(tb, filepath.Join(ficheroComoPadre(t), "origin.git"))
		if !tb.Failed() {
			t.Error("InitBare = sin fallo, want el error de MkdirAll")
		}
	})

	t.Run("BreakGit", func(t *testing.T) {
		// Un repo de verdad, y luego .git/HEAD es un DIRECTORIO: escribir
		// encima falla con EISDIR. Sin tocar el HEAD real (que dejaria el repo
		// inutilizable para el resto del test).
		dir := t.TempDir()
		tb := &MockTB{Temp: tptr(t)}
		Init(tb, dir)
		if tb.Failed() {
			t.Fatalf("no se pudo crear el repo base: %v", tb.Failures)
		}
		head := filepath.Join(dir, ".git", "HEAD")
		if err := os.Remove(head); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(head, 0o755); err != nil {
			t.Fatal(err)
		}
		tb2 := &MockTB{Temp: tptr(t)}
		BreakGit(tb2, dir)
		if !tb2.Failed() {
			t.Error("BreakGit sobre un HEAD que es directorio = sin fallo, want el error")
		}
	})
}
