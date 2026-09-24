// Tests del texto de notificación de acciones terminadas (motivo real del
// fallo + hint accionable).
package tui

import (
	"strings"
	"testing"
)

func TestActionNoteOk(t *testing.T) {
	if got := actionNote("pull", "repo-a", "", ""); got != "pull ok repo-a" {
		t.Errorf("actionNote ok = %q", got)
	}
}

// Un pull divergido: el motivo real de git va en la notificación y se añade
// el hint de rebase.
func TestActionNotePullDiverged(t *testing.T) {
	out := "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
		"hint:\n" +
		"fatal: Not possible to fast-forward, aborting.\n"
	got := actionNote("pull", "repo-a", out, "fatal: Not possible to fast-forward, aborting.")
	if !strings.Contains(got, "pull failed repo-a") {
		t.Errorf("actionNote = %q, want prefijo de fallo", got)
	}
	if !strings.Contains(got, "Not possible to fast-forward") {
		t.Errorf("actionNote = %q, want el motivo real", got)
	}
	if !strings.Contains(got, "diverged?") {
		t.Errorf("actionNote = %q, want el hint de diverged", got)
	}
}

// Sin upstream: el motivo se muestra y el hint apunta a configurar el tracking.
func TestActionNotePullNoUpstream(t *testing.T) {
	got := actionNote("pull", "repo-b",
		"There is no tracking information for the current branch.\n",
		"There is no tracking information for the current branch.")
	if !strings.Contains(got, "no tracking information") {
		t.Errorf("actionNote = %q, want el motivo", got)
	}
	if !strings.Contains(got, "--set-upstream-to") {
		t.Errorf("actionNote = %q, want el hint de upstream", got)
	}
}

// El hint de diverged solo aplica a pull/sync, no a push.
func TestActionNotePushNoDivergedHint(t *testing.T) {
	got := actionNote("push", "repo-c", "fatal: Not possible to fast-forward, aborting.",
		"fatal: Not possible to fast-forward, aborting.")
	if strings.Contains(got, "diverged?") {
		t.Errorf("actionNote push = %q, no debe llevar hint de diverged", got)
	}
}
