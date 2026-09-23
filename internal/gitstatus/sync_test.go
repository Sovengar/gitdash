// Tests de la desviación vs sync branch (spec 0002 R14).
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

// S14.1+S14.5: behind = commits de sync (m1) ausentes en la rama actual,
// NO los propios de feat (merge-base, no diff de tips).
func TestSyncBehindS14_1(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "main")
	if snap.Err != "" {
		t.Fatalf("S5.6: err = %q", snap.Err)
	}
	if !snap.SyncKnown || snap.SyncBranch != "main" {
		t.Fatalf("sync no conocida: %+v", snap)
	}
	if snap.SyncBehind != 1 {
		t.Errorf("S14.5: behind = %d, want 1 (solo m1 ausente)", snap.SyncBehind)
	}
}

// S14.3: HEAD en la propia sync branch → ✓ (behind 0, known).
func TestSyncOnBranchS14_3(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "feat")
	if !snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("S14.3: known=%v behind=%d", snap.SyncKnown, snap.SyncBehind)
	}
}

// S14.4 + 0004 R23: sync branch inexistente → comparación desconocida,
// pero la rama resuelta queda rellena para verse como "<rama> —" en la UI.
func TestSyncMissingS14_4(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "no-existe")
	if snap.SyncKnown {
		t.Errorf("S14.4: known=true con ref inexistente")
	}
	if snap.SyncBranch != "no-existe" {
		t.Errorf("0004 R23: SyncBranch = %q, want no-existe (siempre rellena)", snap.SyncBranch)
	}
	if snap.Err != "" {
		t.Errorf("S14.4: el error de sync no debe ensuciar Err: %q", snap.Err)
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

// StreamPool propaga el override del marcador (R14: override > global).
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
