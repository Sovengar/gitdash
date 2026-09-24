// Tests de worktrees: parseo canned y recolección real.
package gitstatus

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/testutil"
)

func TestParseWorktrees(t *testing.T) {
	out := "worktree /repos/main\nHEAD abc1234def\nbranch refs/heads/main\n\n" +
		"worktree /repos/wt1\nHEAD 1234567890abc\nbranch refs/heads/wt-1\n\n" +
		"worktree /repos/wt2\nHEAD def5678lorem\ndetached\n\n"
	wts := ParseWorktrees(out, "/repos/main")
	if len(wts) != 2 {
		t.Fatalf("wts = %d, want 2 (main excluido)", len(wts))
	}
	if wts[0].Path != "/repos/wt1" || wts[0].Branch != "wt-1" || wts[0].Head != "1234567" {
		t.Errorf("wts[0] = %+v", wts[0])
	}
	if wts[1].Path != "/repos/wt2" || wts[1].Branch != "" {
		t.Errorf("wts[1] = %+v (detached: branch vacía)", wts[1])
	}
}

// El conteo incluye worktrees aunque no tengan marcador.
func TestCollectWorktrees(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtA := t.TempDir()
	testutil.MakeWorktree(t, dir, wtA, "wt-a")
	wtB := t.TempDir()
	testutil.MakeWorktree(t, dir, wtB, "wt-b")

	snap := Collect(t.Context(), dir, "")
	if len(snap.Worktrees) != 2 {
		t.Fatalf("wts = %d, want 2 (marcador o no)", len(snap.Worktrees))
	}
	if snap.Worktrees[0].Branch != "wt-a" {
		t.Errorf("branch = %q, want wt-a", snap.Worktrees[0].Branch)
	}
}

// gitOut ejecuta git en dir y devuelve stdout (helper local de los tests).
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// RemoveWorktree borra el worktree (carpeta + registro) y NUNCA la rama.
func TestRemoveWorktreeRemovesAndKeepsBranch(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-a")
	testutil.MakeWorktree(t, dir, wtDir, "wt-a")

	if _, err := RemoveWorktree(t.Context(), dir, wtDir, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("la carpeta del worktree sigue existiendo: %v", err)
	}
	// El registro desaparece: git ya no lo lista.
	if snap := Collect(t.Context(), dir, ""); len(snap.Worktrees) != 0 {
		t.Errorf("worktrees tras borrar = %d, want 0", len(snap.Worktrees))
	}
	// La rama sobrevive.
	if branches := gitOut(t, dir, "branch", "--list", "wt-a"); !strings.Contains(branches, "wt-a") {
		t.Errorf("la rama wt-a desapareció: %q", branches)
	}
}

// Un worktree sucio falla sin force y se borra con force.
func TestRemoveWorktreeDirtyNeedsForce(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-dirty")
	testutil.MakeWorktree(t, dir, wtDir, "wt-dirty")
	testutil.WriteUntracked(t, wtDir, map[string]string{"pendiente.txt": "sin commitear"})

	out, err := RemoveWorktree(t.Context(), dir, wtDir, false)
	if err == nil {
		t.Fatalf("se esperaba error sin force; out=%q", out)
	}
	if reason := FailureReason(out, err); reason == "" {
		t.Error("FailureReason vacío para el fallo sin force")
	}
	if _, statErr := os.Stat(wtDir); statErr != nil {
		t.Errorf("el worktree sucio se borró pese al fallo: %v", statErr)
	}

	if _, err := RemoveWorktree(t.Context(), dir, wtDir, true); err != nil {
		t.Fatalf("RemoveWorktree force: %v", err)
	}
	if _, statErr := os.Stat(wtDir); !os.IsNotExist(statErr) {
		t.Errorf("el worktree sigue existiendo tras force: %v", statErr)
	}
}

// Un worktree en detached se borra igual (no depende de tener rama).
func TestRemoveWorktreeDetached(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-det")
	testutil.MakeWorktree(t, dir, wtDir, "wt-det")
	testutil.Detach(t, wtDir)

	if _, err := RemoveWorktree(t.Context(), dir, wtDir, false); err != nil {
		t.Fatalf("RemoveWorktree detached: %v", err)
	}
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("la carpeta detached sigue existiendo: %v", err)
	}
}

// Un worktree huérfano (carpeta borrada a mano) no rompe: git lo auto-purga o
// devuelve un error real, nunca un pánico. Un path que no es worktree sí
// devuelve error.
func TestRemoveWorktreePrunableFailsGracefully(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	wtDir := filepath.Join(t.TempDir(), "wt-prune")
	testutil.MakeWorktree(t, dir, wtDir, "wt-prune")
	if err := os.RemoveAll(wtDir); err != nil {
		t.Fatal(err)
	}

	// Carpeta ya ausente: el registro huérfano se limpia sin pánico.
	out, err := RemoveWorktree(t.Context(), dir, wtDir, false)
	if err != nil && FailureReason(out, err) == "" {
		t.Errorf("error sin motivo resumible: err=%v out=%q", err, out)
	}
	if snap := Collect(t.Context(), dir, ""); len(snap.Worktrees) != 0 {
		t.Errorf("el registro huérfano sigue listado: %+v", snap.Worktrees)
	}
	// El force sobre un path ya inexistente tampoco rompe.
	_, _ = RemoveWorktree(t.Context(), dir, wtDir, true)

	// Un path que no es worktree devuelve error real (sin pánico).
	notWT := filepath.Join(t.TempDir(), "no-worktree")
	if err := os.MkdirAll(notWT, 0o755); err != nil {
		t.Fatal(err)
	}
	out2, err2 := RemoveWorktree(t.Context(), dir, notWT, false)
	if err2 == nil {
		t.Fatalf("se esperaba error para un path no-worktree; out=%q", out2)
	}
	if FailureReason(out2, err2) == "" {
		t.Error("FailureReason vacío para un path no-worktree")
	}
}
