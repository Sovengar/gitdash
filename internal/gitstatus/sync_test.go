// Tests de la desviación vs sync branch.
package gitstatus

import (
	"sync"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/testutil"
)

// setupDivergedFromSync crea un repo con: main = base+m1, feat = base+f1
// (divergencia real respecto a la sync branch).
func setupDivergedFromSync(t *testing.T) (dir string) {
	t.Helper()
	dir, _ = testutil.NewRepo(t, false)
	testutil.NewBranch(t, dir, "feat")
	testutil.CommitFiles(t, dir, map[string]string{"f.txt": "f"}, "feat 1")
	testutil.Checkout(t, dir, "main")
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "main 1")
	testutil.Checkout(t, dir, "feat")
	return dir
}

// behind = commits de sync (m1) ausentes en la rama actual,
// NO los propios de feat (merge-base, no diff de tips).
func TestSyncBehind(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "main")
	if snap.Err != "" {
		t.Fatalf("err = %q", snap.Err)
	}
	if !snap.SyncKnown || snap.SyncBranch != "main" {
		t.Fatalf("sync no conocida: %+v", snap)
	}
	if snap.SyncBehind != 1 {
		t.Errorf("behind = %d, want 1 (solo m1 ausente)", snap.SyncBehind)
	}
}

// HEAD en la propia sync branch → ✓ (behind 0, known).
func TestSyncOnBranch(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "feat")
	if !snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("known=%v behind=%d", snap.SyncKnown, snap.SyncBehind)
	}
}

// sync branch inexistente → comparación desconocida,
// pero la rama resuelta queda rellena para verse como "<rama> —" en la UI.
func TestSyncMissing(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "no-existe")
	if snap.SyncKnown {
		t.Errorf("known=true con ref inexistente")
	}
	if snap.SyncBranch != "no-existe" {
		t.Errorf("SyncBranch = %q, want no-existe (siempre rellena)", snap.SyncBranch)
	}
	if snap.Err != "" {
		t.Errorf("el error de sync no debe ensuciar Err: %q", snap.Err)
	}
}

// Sin sync branch ("") no se calcula nada.
func TestSyncDisabled(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "")
	if snap.SyncKnown || snap.SyncBranch != "" {
		t.Errorf("sync = %v/%d/%q, want desconocida", snap.SyncKnown, snap.SyncBehind, snap.SyncBranch)
	}
}

// StreamPool propaga el override del marcador (override > global).
func TestStreamPoolSyncOverride(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "main 1")
	testutil.NewBranch(t, dir, "feat")
	projects := []discovery.Project{
		{Path: dir, HasRepo: true, SyncBranch: "main"}, // override
	}

	got := map[string]Snapshot{}
	var mu sync.Mutex
	// emit se invoca concurrentemente (contrato de StreamPool): proteger el mapa.
	StreamPool(t.Context(), projects, "global-branch", 2, func(path string, snap Snapshot) {
		mu.Lock()
		got[path] = snap
		mu.Unlock()
	})
	snap := got[dir]
	// la global "global-branch" no existe: si el override no se respetara,
	// SyncKnown sería false.
	if !snap.SyncKnown || snap.SyncBranch != "main" {
		t.Errorf("override no aplicado: %+v", snap)
	}
}
