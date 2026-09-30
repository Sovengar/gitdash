package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stub escribe un script ejecutable en t.TempDir() y devuelve su path. El binario
// de un forge no se puede probar de verdad sin red ni token, así que lo que se
// prueba es lo que el Runner hace alrededor: argv, entorno, plazo y captura.
func stub(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// El entorno del subproceso es homogéneo: sin el locale del usuario (los
// mensajes de git y de las CLIs se leen en inglés) y sin nada que pueda abrir
// una pregunta. Un LC_ALL del usuario filtrado pondría los mensajes en el
// idioma que sea y el aviso al usuario dejaría de entenderse.
func TestEnvEsHomogeneo(t *testing.T) {
	t.Setenv("LC_ALL", "es_ES.UTF-8")
	t.Setenv("LANG", "es_ES.UTF-8")
	t.Setenv("LANGUAGE", "es")
	t.Setenv("LC_MESSAGES", "es_ES.UTF-8")

	env := Env()
	for _, banned := range []string{"LANG=", "LANGUAGE=", "LC_MESSAGES="} {
		for _, kv := range env {
			if strings.HasPrefix(kv, banned) {
				t.Errorf("Env conserva %q: %q", banned, kv)
			}
		}
	}
	n := 0
	for _, kv := range env {
		if kv == "LC_ALL=C" {
			n++
		}
		if strings.HasPrefix(kv, "LC_ALL=") && kv != "LC_ALL=C" {
			t.Errorf("LC_ALL no forzado a C: %q", kv)
		}
	}
	if n != 1 {
		t.Errorf("LC_ALL=C aparece %d veces, quiero 1 (el del usuario no debe survive)", n)
	}
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "NO_COLOR=1"} {
		if !slicesHas(env, want) {
			t.Errorf("Env no contiene %q", want)
		}
	}
}

// Lo que no es locale se conserva: sin PATH el subproceso ni arranca, y sin el
// entorno del usuario las CLIs pierden la config que las autentica.
func TestEnvConservaElResto(t *testing.T) {
	t.Setenv("GITDASH_TEST_MARKER", "sigue-vivo")
	env := Env()
	if !slicesHas(env, "GITDASH_TEST_MARKER=sigue-vivo") {
		t.Error("Env tiró una variable que no es locale")
	}
	if len(env) == 0 {
		t.Error("Env está vacío")
	}
}

func TestEnvAgregaLasVariablesDelForge(t *testing.T) {
	env := Env("GH_PROMPT_DISABLED=1")
	if !slicesHas(env, "GH_PROMPT_DISABLED=1") {
		t.Error("Env no agrego la variable extra")
	}
}

// El motivo del fallo sale de stderr, y el código de salida se preserva: son
// las dos cosas que necesita el toast para decir qué pasó. stderr vacío cae al
// error del propio proceso, porque un Msg vacío se clasificaría como fallo de
// red sin motivo y dejaría al usuario sin nada que hacer.
func TestRunCapturaStderrYCodigoDeSalida(t *testing.T) {
	bin := stub(t, "#!/bin/sh\necho 'el motivo real' >&2\nexit 3\n")
	_, err := (&Runner{Bin: bin}).Run(context.Background(), "-t", "x")
	var cerr *Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, quiero *tool.Error", err)
	}
	if cerr.Msg != "el motivo real" {
		t.Errorf("Msg = %q, quiero el stderr", cerr.Msg)
	}
	if cerr.ExitCode != 3 {
		t.Errorf("ExitCode = %d, quiero 3", cerr.ExitCode)
	}
	if cerr.Bin != bin || !slicesHas(cerr.Args, "-t") {
		t.Errorf("Error no recuerda el argv: %+v", cerr)
	}
	if !strings.Contains(cerr.Error(), "exit 3") {
		t.Errorf("el mensaje no menciona el código: %q", cerr)
	}

	// stderr vacío: el motivo sale del error del proceso y no queda vacío.
	_, err = (&Runner{Bin: stub(t, "#!/bin/sh\nexit 7\n")}).Run(context.Background())
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, quiero *tool.Error", err)
	}
	if cerr.Msg == "" {
		t.Error("sin stderr el motivo no puede quedar vacío")
	}
	if cerr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, quiero 7", cerr.ExitCode)
	}
	if cerr.Unwrap() == nil {
		t.Error("Unwrap devuelve nil: se pierde la causa de exec")
	}
}

// stdout vuelve incluso en error. Una CLI puede salir con código distinto de
// cero y traer el dato útil en stdout; tirarlo deja al usuario sin el mensaje
// que sí llegó.
func TestRunDevuelveStdoutAunqueFalle(t *testing.T) {
	bin := stub(t, "#!/bin/sh\necho 'salida válida'\nexit 1\n")
	out, err := (&Runner{Bin: bin}).Run(context.Background())
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if strings.TrimSpace(out) != "salida válida" {
		t.Errorf("stdout = %q, quiero la salida válida aunque el comando falle", out)
	}
}

// Cada elemento del argv llega como elemento. Es lo que hace seguro pasar un
// título escrito por la persona: sin comillas de por medio, un ";" o un
// "$(...)" no son sintaxis, son texto.
func TestRunPasaElArgvElementoPorElemento(t *testing.T) {
	nasty := `doble "comilla" & $(id) ; echo inyectado | cat`
	bin := stub(t, "#!/bin/sh\nprintf 'argc=%s\\n' \"$#\"\nprintf '%s\\0' \"$@\"\n")
	out, err := (&Runner{Bin: bin}).Run(context.Background(),
		"pr", "create", "-t", nasty, "-b", "cuerpo con\ttab y\nnueva línea", "-B", "main")
	if err != nil {
		t.Fatalf("Run falló: %v", err)
	}
	if !strings.HasPrefix(out, "argc=8\n") {
		t.Errorf("argc = %q, quiero 8 elementos", strings.SplitN(out, "\n", 2)[0])
	}
	_, payload, _ := strings.Cut(out, "\n")
	if got := strings.Split(strings.TrimSuffix(payload, "\x00"), "\x00"); len(got) != 8 {
		t.Fatalf("llegaron %d elementos: %q", len(got), got)
	} else if got[3] != nasty {
		t.Errorf("el valor no llegó entero: %q", got[3])
	} else if got[5] != "cuerpo con\ttab y\nnueva línea" {
		t.Errorf("el cuerpo con saltos de línea no llegó entero: %q", got[5])
	}
}

// Sin timeout, una CLI colgada deja la TUI esperando para siempre. Timeout=0
// (cero explícito, el valor de un Runner construido a mano) y negativo tienen
// que caer al default igual: con el plazo en cero el proceso muere al instante
// aunque sea trivial.
func TestRunAplicaElTimeoutPorDefecto(t *testing.T) {
	bin := stub(t, "#!/bin/sh\necho ok\n")
	if _, err := (&Runner{Bin: bin}).Run(context.Background()); err != nil {
		t.Errorf("timeout=0 debería usar el default, pero falló: %v", err)
	}
	if _, err := (&Runner{Bin: bin, Timeout: -time.Second}).Run(context.Background()); err != nil {
		t.Errorf("un timeout negativo debería caer al default, pero falló: %v", err)
	}
	r := &Runner{Bin: bin}
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.Timeout != 0 {
		t.Errorf("Run mutó el Timeout del Runner: %v", r.Timeout)
	}
	// Y un plazo positivo y corto se respeta: el comando duerme más de lo que
	// se le da y muere por plazo.
	slow := stub(t, "#!/bin/sh\nsleep 5\n")
	if _, err := (&Runner{Bin: slow, Timeout: 50 * time.Millisecond}).Run(context.Background()); err == nil {
		t.Error("un comando que excede el timeout tiene que fallar")
	}
}

// El binario inexistente es un error de entorno (gh no instalado), no un
// timeout ni un fallo de la CLI: tiene que llegar como el error que es, con un
// motivo que el toast pueda enseñar en vez de un "not found" sin contexto.
func TestRunConBinarioInexistente(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-existe")
	_, err := (&Runner{Bin: missing}).Run(context.Background())
	if err == nil {
		t.Fatal("se esperaba error")
	}
	var cerr *Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, quiero *tool.Error", err)
	}
	if cerr.Err == nil {
		t.Error("la causa de exec no se preservó")
	}
	if !strings.Contains(cerr.Msg, "no-existe") {
		t.Errorf("Msg = %q, quiero que nombre el binario que falta", cerr.Msg)
	}
	if cerr.ExitCode != 0 {
		t.Errorf("ExitCode = %d, quiero 0: no llegó a ejecutarse", cerr.ExitCode)
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, quiero 0", got)
	}
	if got := ExitCode(errors.New("otro")); got != 0 {
		t.Errorf("ExitCode(otro) = %d, quiero 0", got)
	}
	if got := ExitCode(&Error{ExitCode: 42}); got != 42 {
		t.Errorf("ExitCode(*Error) = %d, quiero 42", got)
	}
	if got := ExitCode(errWrap{&Error{ExitCode: 9}}); got != 9 {
		t.Errorf("ExitCode(envuelto) = %d, quiero 9", got)
	}
	// Y el *exec.ExitError crudo, que es lo que devuelve exec sin envolver.
	_, err := (&Runner{Bin: stub(t, "#!/bin/sh\nexit 6\n")}).Run(context.Background())
	var cerr *Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, quiero *tool.Error", err)
	}
	if got := ExitCode(cerr.Err); got != 6 {
		t.Errorf("ExitCode(exec.ExitError) = %d, quiero 6", got)
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"uno":          "uno",
		"uno\ndos":     "uno",
		"uno\n\ndos":   "uno",
		"uno\n":        "uno",
		"":             "",
		"\n":           "",
		"con 3 lineas": "con 3 lineas",
		"motivo\nargv": "motivo",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, quiero %q", in, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	r := New("gh", "GH_PROMPT_DISABLED=1")
	if r.Bin != "gh" {
		t.Errorf("Bin = %q", r.Bin)
	}
	if r.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, quiero %v", r.Timeout, DefaultTimeout)
	}
	if !slicesHas(r.Extra, "GH_PROMPT_DISABLED=1") {
		t.Errorf("Extra = %q", r.Extra)
	}
}

type errWrap struct{ err error }

func (e errWrap) Error() string { return "envuelto: " + e.err.Error() }
func (e errWrap) Unwrap() error { return e.err }

func slicesHas(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
