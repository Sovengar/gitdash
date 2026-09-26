// Tests del texto de notificación de acciones terminadas (motivo real del
// fallo + hint accionable + argv resuelto).
package tui

import (
	"strings"
	"testing"
)

func TestActionNoteOk(t *testing.T) {
	if got := actionNote("pull", "repo-a", "", "", "", false); got != "pull ok repo-a" {
		t.Errorf("actionNote ok = %q", got)
	}
}

// El argv resuelto viaja en la notificación: con la política de pull en el
// gitconfig, "pull ok" no dice si reconcilió con merge, rebase o ff-only.
func TestActionNoteOkShowsResolvedCommand(t *testing.T) {
	got := actionNote("pull_rebase", "repo-a", "git pull --rebase --autostash", "", "", false)
	if !strings.Contains(got, "git pull --rebase --autostash") {
		t.Errorf("actionNote = %q, want el argv resuelto", got)
	}
}

// Un pull divergido: el motivo real de git va en la notificación y se añade
// el hint de rebase.
func TestActionNotePullDiverged(t *testing.T) {
	out := "hint: Diverging branches can't be fast-forwarded, you need to either:\n" +
		"hint:\n" +
		"fatal: Not possible to fast-forward, aborting.\n"
	got := actionNote("pull_ff", "repo-a", "git pull --ff-only", out,
		"fatal: Not possible to fast-forward, aborting.", false)
	if !strings.Contains(got, "pull_ff failed repo-a") {
		t.Errorf("actionNote = %q, want prefijo de fallo", got)
	}
	if !strings.Contains(got, "Not possible to fast-forward") {
		t.Errorf("actionNote = %q, want el motivo real", got)
	}
	if !strings.Contains(got, "divergió") {
		t.Errorf("actionNote = %q, want el hint de divergencia", got)
	}
}

// Sin upstream: el motivo se muestra y el hint apunta a la tecla que lo
// resuelve (P publica la rama y configura el tracking).
func TestActionNotePullNoUpstream(t *testing.T) {
	got := actionNote("pull", "repo-b", "git pull",
		"There is no tracking information for the current branch.\n",
		"There is no tracking information for the current branch.", false)
	if !strings.Contains(got, "no tracking information") {
		t.Errorf("actionNote = %q, want el motivo", got)
	}
	if !strings.Contains(got, "P la publica") {
		t.Errorf("actionNote = %q, want el hint de upstream", got)
	}
}

// El hint de divergencia solo aplica a las variantes de pull, no a push.
func TestActionNotePushNoDivergedHint(t *testing.T) {
	got := actionNote("push", "repo-c", "git push",
		"fatal: Not possible to fast-forward, aborting.",
		"fatal: Not possible to fast-forward, aborting.", false)
	if strings.Contains(got, "divergió") {
		t.Errorf("actionNote push = %q, no debe llevar hint de divergencia", got)
	}
}

// Un pull --rebase que choca deja el repo a medias. El aviso tiene que decirlo
// explícitamente: "falló" solo invita a reintentar, y reintentar sobre un rebase
// sin resolver es peor que no hacer nada.
func TestActionNoteRebaseInProgressWins(t *testing.T) {
	out := "CONFLICT (content): Merge conflict in f.txt\n"
	got := actionNote("pull_rebase", "repo-a", "git pull --rebase --autostash", out,
		"error: could not apply 1234567... local", true)
	if !strings.Contains(got, "rebase a medias") {
		t.Errorf("actionNote = %q, want el aviso de rebase a medias", got)
	}
	if !strings.Contains(got, "rebase --continue") {
		t.Errorf("actionNote = %q, want cómo continuar", got)
	}
	// El aviso de rebase a medias pisa los hints de divergencia/upstream: son
	// diagnósticos de un estado que ya no aplica.
	if strings.Contains(got, "divergió") || strings.Contains(got, "sin upstream") {
		t.Errorf("actionNote = %q, los hints secundarios deben quedar fuera", got)
	}
}

// El aviso de rebase a medias no aplica a push: un push fallido deja el repo
// como estaba.
func TestActionNotePushIgnoresRebaseFlag(t *testing.T) {
	got := actionNote("push", "repo-c", "git push", "fatal: x", "fatal: x", true)
	if strings.Contains(got, "rebase a medias") {
		t.Errorf("actionNote push = %q, no debe mencionar rebase", got)
	}
}
