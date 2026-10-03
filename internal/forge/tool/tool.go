// Package tool ejecuta las CLIs de forge por subproceso con un entorno
// homogéneo: locale inglés, sin interacción, plazo por invocación y error que
// conserva el código de salida y el stderr.
//
// Vive en su propio paquete y no en forge porque forge es puro por contrato
// (ver el doc de ese paquete): aquí sí se lanzan procesos. El runner no sabe
// nada de GitHub ni de GitLab — recibe el binario y las variables extra — así
// que el conocimiento de cada CLI se queda del lado de forge, que es el que
// sabe qué host y qué flags son los de cada una.
package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout es el plazo por invocación de una CLI de forge. Sin él, un
// `gh` colgado deja la TUI esperando para siempre.
//
// Es una FUNCIÓN y no una const por lo mismo que los plazos del TUI: Go no
// instrumenta las expresiones de constante, así que una const de paquete no
// genera bloque de cobertura y el mutante de ARITHMETIC_BASE de `30 *
// time.Second` sale NOT COVERED para siempre. Dentro de una función sí se
// instrumenta, y el mutante pasa a ejecutarse. Con `30 / time.Second` el plazo
// sería 0: un timeout inmediato en cada invocación, que es un fallo que se
// parece sospechosamente a "gh está roto".
func DefaultTimeout() time.Duration { return 30 * time.Second }

// Runner ejecuta un binario con plazo y entorno no interactivo.
type Runner struct {
	Bin     string
	Timeout time.Duration
	// Extra son las variables que el forge necesita (GITLAB_HOST,
	// GH_PROMPT_DISABLED). Van al final, así que pisan lo que venga del entorno
	// del usuario.
	Extra []string
}

// New construye un Runner con el plazo por defecto y las variables extra dadas.
func New(bin string, extra ...string) *Runner {
	return &Runner{Bin: bin, Timeout: DefaultTimeout(), Extra: extra}
}

// Error es el fallo de una CLI, con el argv y el código de salida preservados y
// la causa (Unwrap) expuesta para poder clasificarlo sin depender del texto.
type Error struct {
	Bin      string
	Args     []string
	ExitCode int
	Msg      string
	Err      error
}

// Error compone el mensaje del fallo incluyendo el código de salida.
func (e *Error) Error() string {
	base := fmt.Sprintf("%s %s: %s", e.Bin, strings.Join(e.Args, " "), e.Msg)
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

// Unwrap expone la causa subyacente (p. ej. *exec.ExitError).
func (e *Error) Unwrap() error { return e.Err }

// Run ejecuta el binario con los args dados y devuelve stdout. Ante un fallo
// devuelve stdout igualmente, porque una CLI puede salir con código distinto de
// cero y traer el motivo útil en stdout, más un *Error con el código y la
// primera línea de stderr.
func (r *Runner) Run(ctx context.Context, args ...string) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, r.Bin, args...)
	cmd.Env = Env(r.Extra...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := FirstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		cerr := &Error{Bin: r.Bin, Args: args, Msg: msg, Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			cerr.ExitCode = exit.ExitCode()
		}
		return out.String(), cerr
	}
	return out.String(), nil
}

// Env compone el entorno del subproceso: descarta el locale del usuario para
// forzar mensajes en inglés y añade el modo no interactivo. Lo que no es locale
// se conserva entero, porque sin el entorno del usuario las CLIs pierden justo
// lo que las autentica.
func Env(extra ...string) []string {
	env := os.Environ()
	// Sin capacidad reservada a mano: `env` ya es el techo (se filtran entradas
	// y se añaden cuatro), y un número escrito aquí es un sitio que mutar sin
	// efecto observable. La slice crece con append, que es lo que harla de
	// todas formas.
	out := make([]string, 0, len(env))
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="):
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1")
	return append(out, extra...)
}

// ExitCode devuelve el código de salida de un error de CLI, o 0.
func ExitCode(err error) int {
	var cerr *Error
	if errors.As(err, &cerr) {
		return cerr.ExitCode
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 0
}

// FirstLine recorta un mensaje a su primera línea: es lo que entra en el toast,
// y un motivo de varias líneas empujaría el alto del panel.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
