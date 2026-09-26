// Tests de la detección de rebase a medias. Un pull --rebase que choca no es un
// fallo limpio: deja la historia reescrita a medias y el índice en conflicto,
// y la UI tiene que distinguirlo de "falló y no pasó nada".
package gitstatus

import (
	"testing"

	"gitdash/internal/testutil"
)

// Repo sano: no hay rebase a medias aunque el pull fuera todo un éxito.
func TestRebaseInProgressFalse(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, dir, map[string]string{"l.txt": "local\n"}, "local")
	testutil.PushUpstreamFile(t, origin, "u.txt", "remote\n", "remote")
	testutil.FetchLocal(t, dir)

	if out, err := Run(t.Context(), dir, "pull", "--rebase"); err != nil {
		t.Fatalf("el pull de un repo no divergente falló:\n%s", out)
	}
	if RebaseInProgress(t.Context(), dir) {
		t.Error("RebaseInProgress = true en un repo sin rebase")
	}
}

// Tras un pull --rebase que choca, el directorio de estado del rebase existe:
// hay que reportarlo como "a medias", no como fallo limpio.
func TestRebaseInProgressTrue(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	// El mismo fichero, distinto contenido en cada lado: conflicto garantizado.
	testutil.CommitFiles(t, dir, map[string]string{"c.txt": "local\n"}, "local")
	testutil.PushUpstreamFile(t, origin, "c.txt", "remote\n", "remote")
	testutil.FetchLocal(t, dir)

	out, err := Run(t.Context(), dir, "pull", "--rebase")
	if err == nil {
		t.Skipf("el pull no chocó (fixture sin conflicto):\n%s", out)
	}
	if !RebaseInProgress(t.Context(), dir) {
		t.Errorf("RebaseInProgress = false tras un rebase en conflicto:\n%s", out)
	}
}

// Un repo que ni siquiera es un repo no puede tener un rebase: la comprobación
// no debe propagar el error de git ni panicar.
func TestRebaseInProgressNoRepo(t *testing.T) {
	if RebaseInProgress(t.Context(), t.TempDir()) {
		t.Error("RebaseInProgress = true fuera de un repo")
	}
}

// El estado del rebase vive en el dir del worktree, no en el del repo
// principal: por eso la comprobación usa `git rev-parse --git-path` en vez de
// mirar `.git/rebase-*` a pelo.
func TestRebaseInProgressEnWorktree(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	wt := t.TempDir()
	testutil.MakeWorktree(t, dir, wt, "feat")

	// Conflicto DENTRO del worktree: principal limpio, worktree a medias. La
	// rama del worktree no trackea nada, así que el rebase se apunta a main
	// explícitamente (es el caso real de "traeme main a mi feature").
	testutil.CommitFiles(t, wt, map[string]string{"c.txt": "local\n"}, "local")
	testutil.PushUpstreamFile(t, origin, "c.txt", "remote\n", "remote")
	testutil.FetchLocal(t, wt)
	if out, err := Run(t.Context(), wt, "pull", "--rebase", "origin", "main"); err == nil {
		t.Skipf("el pull no chocó:\n%s", out)
	}

	if !RebaseInProgress(t.Context(), wt) {
		t.Error("RebaseInProgress = false en un worktree con rebase en curso")
	}
	if RebaseInProgress(t.Context(), dir) {
		t.Error("el rebase del worktree se le atribuyó al repo principal")
	}
}
