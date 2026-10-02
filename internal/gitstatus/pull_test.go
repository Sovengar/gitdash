// Tests del reporte de fallos de pull/sync (motivo real de git, no
// "exit status N").
package gitstatus

import (
	"errors"
	"strings"
	"testing"

	"gitdash/internal/testutil"
)

// FailureReason devuelve la primera línea útil de git, saltando los hints
// (verbosos, se ven completos en el detalle) y con fallback al error del
// proceso cuando no hay salida.
func TestFailureReason(t *testing.T) {
	out := "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
		"hint:\n" +
		"fatal: Not possible to fast-forward, aborting.\n"
	if got := FailureReason(out, errors.New("exit status 128")); got != "fatal: Not possible to fast-forward, aborting." {
		t.Errorf("FailureReason = %q, want la línea fatal", got)
	}
	if got := FailureReason("", errors.New("exit status 1")); got != "exit status 1" {
		t.Errorf("FailureReason sin salida = %q, want fallback", got)
	}
}

// Un pull divergido expone el motivo real de git (no "exit status N"), para
// que la UI pueda decidir el hint.
func TestPullDivergedReportsReason(t *testing.T) {
	diverged, origin := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, diverged, map[string]string{"l.txt": "l"}, "local")
	testutil.PushUpstreamCommits(t, origin, 2, "div-")
	testutil.FetchLocal(t, diverged)

	// El flag va explícito: el mensaje que se comprueba es el del ff-only, y
	// dejarlo en manos del gitconfig haría el test dependiente de la máquina.
	out, err := Run(t.Context(), diverged, "pull", "--ff-only")
	if err == nil {
		t.Fatalf("pull divergido no falló:\n%s", out)
	}
	reason := FailureReason(out, err)
	if !strings.Contains(reason, "Not possible to fast-forward") {
		t.Errorf("reason = %q, want 'Not possible to fast-forward'", reason)
	}
	if reason == err.Error() {
		t.Errorf("reason = %q: se está reportando el exit status, no el motivo", reason)
	}
}

// El motivo viaja en inglés aunque el locale del usuario esté en español: sin
// LC_ALL=C la UI no podría reconocer el fallo.
func TestPullReasonIgnoresLocale(t *testing.T) {
	t.Setenv("LC_ALL", "es_ES.UTF-8")
	t.Setenv("LANG", "es_ES.UTF-8")

	dir, _ := testutil.NewRepo(t, false) // sin upstream
	out, err := Run(t.Context(), dir, "pull")
	if err == nil {
		t.Fatalf("pull sin upstream no falló:\n%s", out)
	}
	reason := FailureReason(out, err)
	if !strings.Contains(reason, "no tracking information") {
		t.Errorf("reason = %q, want mensaje en inglés pese al locale", reason)
	}
}

// FailureReason tiene tres salidas y solo se probaba la primera. Las otras dos:
//
//   - salida vacía con error: el mensaje es el del proceso. Es lo que pasa con un
//     fallo de red o un remoto caido, donde git no imprime nada y lo unico que
//     hay que enseñar es el error.
//   - salida vacía SIN error: no hay nada que decir. La UI tiene que poder pintar
//     cadena vacia en vez de un "error" inventado, porque un fallo sin texto se
//     ve como un fallo sin explicacion.
func TestFailureReasonSinSalida(t *testing.T) {
	// Con error del proceso y salida vacía.
	boom := errors.New("exit status 128")
	if got := FailureReason("", boom); got != "exit status 128" {
		t.Errorf("FailureReason vacio con error = %q, want el texto del error", got)
	}
	// Con salida que solo tiene hints y líneas en blanco: todo se salta.
	out := "\n\nhint: si esto fuera un rebase...\n   \nhint: outro hint\n"
	if got := FailureReason(out, boom); got != "exit status 128" {
		t.Errorf("FailureReason solo-hints = %q, want el fallback al error", got)
	}
	// Sin salida y sin error: vacío, no un placeholder.
	if got := FailureReason("", nil); got != "" {
		t.Errorf("FailureReason sin nada = %q, want vacio", got)
	}
	// Y con salida real, gana la salida sobre el error.
	if got := FailureReason("fatal: could not read Username\nhint: x\n", boom); got != "fatal: could not read Username" {
		t.Errorf("FailureReason = %q, want la primera linea de salida", got)
	}
}
