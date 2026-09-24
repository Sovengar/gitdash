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

	out, err := Pull(t.Context(), diverged)
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
	out, err := Pull(t.Context(), dir)
	if err == nil {
		t.Fatalf("pull sin upstream no falló:\n%s", out)
	}
	reason := FailureReason(out, err)
	if !strings.Contains(reason, "no tracking information") {
		t.Errorf("reason = %q, want mensaje en inglés pese al locale", reason)
	}
}
