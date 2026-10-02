// Tests de la detección de rebase a medias. Un pull --rebase que choca no es un
// fallo limpio: deja la historia reescrita a medias y el índice en conflicto,
// y la UI tiene que distinguirlo de "falló y no pasó nada".
package gitstatus

import (
	"os"
	"path/filepath"
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

// Un rebase a medias del que no se llego a pickear nada deja el directorio de
// estado (rebase-merge o rebase-apply) en el worktree del repo principal, no en
// un worktree real. Los dos nombres cuentan, y quitarlos devuelve a false.
//
// El test de worktree de arriba monta el caso real (un pull --rebase que
// choca); este monta el caso temprano, que es el que se da cuando el conflicto
// se produce en el commit que se esta aplicando: git crea el directorio de
// estado antes de intentar el pick, asi que el aviso tiene que aparecer tambien
// sin historial reescrito.
func TestRebaseInProgressSinPickear(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)

	for _, nombre := range []string{"rebase-merge", "rebase-apply"} {
		t.Run(nombre, func(t *testing.T) {
			estado := filepath.Join(dir, ".git", nombre)
			if err := os.MkdirAll(estado, 0o755); err != nil {
				t.Fatal(err)
			}
			if !RebaseInProgress(t.Context(), dir) {
				t.Errorf("RebaseInProgress = false con %s en disco, want true", nombre)
			}
			if err := os.RemoveAll(estado); err != nil {
				t.Fatal(err)
			}
			if RebaseInProgress(t.Context(), dir) {
				t.Errorf("RebaseInProgress = true tras quitar %s", nombre)
			}
		})
	}
}
