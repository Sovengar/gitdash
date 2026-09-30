// Tests de la desviación vs sync branch.
package gitstatus

import (
	"os/exec"
	"strings"
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

// Behind = commits de sync (m1) ausentes en la rama actual,
// NO los propios de feat (merge-base, no diff de tips).
func TestSyncBehind(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "main", false)
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
	snap := Collect(t.Context(), dir, "feat", false)
	if !snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("known=%v behind=%d", snap.SyncKnown, snap.SyncBehind)
	}
}

// Sync branch inexistente → comparación desconocida,
// pero la rama resuelta queda rellena para verse como "<rama> —" en la UI.
func TestSyncMissing(t *testing.T) {
	dir := setupDivergedFromSync(t)
	snap := Collect(t.Context(), dir, "no-existe", false)
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
	snap := Collect(t.Context(), dir, "", false)
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
	StreamPool(t.Context(), projects, "global-branch", false, 2, func(path string, snap Snapshot) {
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

// gitLocal ejecuta un comando git en dir. Los fixtures de testutil siempre
// nacen en main (Init hace `git init -b main`) y el caso que dispara el
// fallback necesita justo lo contrario, un repo SIN main: renombrar la rama
// inicial no tiene helper.
func gitLocal(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
}

// repoSinMain crea un repo cuya rama principal NO se llama main: media vida de
// repos se quedó en master, y es el caso que hoy pintaba "main —".
func repoSinMain(t *testing.T) string {
	t.Helper()
	dir, _ := testutil.NewRepo(t, false)
	gitLocal(t, dir, "branch", "-m", "master")
	return dir
}

// El default global no es una declaración del usuario: si el repo no tiene la
// rama, la referencia cae a master. Lo que resuelve es lo que se COMPARA y lo
// que se MUESTRA — si la columna se quedara con "main", seguiría enseñando
// "main —" y el `glab mr create -b main` seguiría fallando.
func TestSyncFallbackAMaster(t *testing.T) {
	dir := repoSinMain(t)
	// feat sale de base y master avanza: m1 ausente en feat = 1 detrás, el
	// conteo por merge-base que ya usa el resto de la suite.
	testutil.NewBranch(t, dir, "feat")
	testutil.Checkout(t, dir, "master")
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "master 1")
	testutil.Checkout(t, dir, "feat")
	testutil.CommitFiles(t, dir, map[string]string{"f.txt": "f"}, "feat 1")

	snap := Collect(t.Context(), dir, "main", true)
	if snap.Err != "" {
		t.Fatalf("err = %q", snap.Err)
	}
	if snap.SyncBranch != "master" {
		t.Errorf("SyncBranch = %q, want master (la que resuelve es la que se muestra)", snap.SyncBranch)
	}
	if !snap.SyncKnown {
		t.Fatalf("comparación contra master desconocida: %+v", snap)
	}
	if snap.SyncBehind != 1 {
		t.Errorf("behind = %d, want 1 (m1 ausente en feat)", snap.SyncBehind)
	}
}

// Ninguna de las dos existe: no hay referencia que comparar y la que se
// muestra es la que se pidió, no un nombre inventado.
func TestSyncFallbackSinNingunaDeLasDos(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	gitLocal(t, dir, "branch", "-m", "trunk")

	snap := Collect(t.Context(), dir, "main", true)
	if snap.SyncBranch != "main" {
		t.Errorf("SyncBranch = %q, want main (la original)", snap.SyncBranch)
	}
	if snap.SyncKnown || snap.SyncBehind != 0 {
		t.Errorf("sync = %v/%d, want desconocida sin detrás", snap.SyncKnown, snap.SyncBehind)
	}
}

// El fallback se prueba DENTRO del fallo de la referencia global, así que un
// repo que sí tiene main no gasta un `rev-list` extra contra master. Cuesta
// un git por repo y por ciclo: el caso normal tiene que salir gratis.
func TestSyncFallbackNoLanzaGitSiLaGlobalResuelve(t *testing.T) {
	rec := installRecorder(t)
	dir := setupDivergedFromSync(t) // main = base+m1, feat = base+f1

	snap := Collect(t.Context(), dir, "main", true)
	if snap.SyncBranch != "main" || !snap.SyncKnown || snap.SyncBehind != 1 {
		t.Fatalf("con main presente nada cambia: %+v", snap)
	}
	for _, e := range rec.Entries() {
		if strings.Contains(e.Command(), "master") {
			t.Errorf("git extra contra master con la global resuelta: %q", e.Command())
		}
	}
}

// Sin fallback permitido, un repo en master se queda como estaba: "main —" en
// la columna. Es lo que ven los overrides declarados y la config explícita.
func TestSyncSinFallbackPermitidoNoCambia(t *testing.T) {
	dir := repoSinMain(t)
	testutil.CommitFiles(t, dir, map[string]string{"m.txt": "m"}, "master 1")

	snap := Collect(t.Context(), dir, "main", false)
	if snap.SyncBranch != "main" {
		t.Errorf("SyncBranch = %q, want main (sin fallback no se toca lo pedido)", snap.SyncBranch)
	}
	if snap.SyncKnown {
		t.Error("SyncKnown sin la referencia que se pidió")
	}
}

// La precedencia que decide si hay fallback: SOLO el default (nadie declaró la
// rama) lo admite. Marcador y config global son intenciones del usuario, y
// referenciarles otra rama en silencio sería mentir sobre lo que se comparó.
func TestSyncForFallbackSoloEnElDefault(t *testing.T) {
	casos := []struct {
		nombre       string
		p            discovery.Project
		global       string
		explicit     bool
		want         string
		wantFallback bool
	}{
		{"default sin declarar", discovery.Project{}, "main", false, "main", true},
		{"marcador declarado", discovery.Project{SyncBranch: "develop"}, "main", false, "develop", false},
		{"config global explícita", discovery.Project{}, "release", true, "release", false},
		{"marcador gana sobre global explícita", discovery.Project{SyncBranch: "develop"}, "release", true, "develop", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SyncFor(c.p, c.global); got != c.want {
				t.Errorf("SyncFor = %q, want %q", got, c.want)
			}
			if got := SyncForAllowsFallback(c.p, c.explicit); got != c.wantFallback {
				t.Errorf("SyncForAllowsFallback = %v, want %v", got, c.wantFallback)
			}
		})
	}
}

// El contrato visto desde la recolección, no desde la decisión: una referencia
// que el usuario declaró y no existe se queda como desconocida aunque el repo
// tenga master. Así la columna puede mostrar el fallo en vez de taparlo.
func TestSyncReferenciaDeclaradaNoHaceFallback(t *testing.T) {
	dir := repoSinMain(t)
	snap := Collect(t.Context(), dir, "no-existe", false)
	if snap.SyncKnown {
		t.Error("known con una referencia declarada que no existe")
	}
	if snap.SyncBranch != "no-existe" {
		t.Errorf("SyncBranch = %q, want no-existe (lo declarado manda)", snap.SyncBranch)
	}
}

// El pool propaga la decisión del mismo modo que la recolección directa: sin
// config global explícita el repo en master resuelve a master, y con ella se
// queda como le dijeron. Es la diferencia entre pasar el flag y pasarlo
// siempre en false, que compila igual y no dice lo mismo.
func TestStreamPoolPropagaElFallback(t *testing.T) {
	dir := repoSinMain(t)
	projects := []discovery.Project{{Path: dir, HasRepo: true}}
	for _, c := range []struct {
		nombre   string
		explicit bool
		want     string
		known    bool
	}{
		{"default", false, "master", true},
		{"config explícita", true, "main", false},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			got := map[string]Snapshot{}
			var mu sync.Mutex
			StreamPool(t.Context(), projects, "main", c.explicit, 2, func(path string, snap Snapshot) {
				mu.Lock()
				got[path] = snap
				mu.Unlock()
			})
			snap := got[dir]
			if snap.SyncBranch != c.want || snap.SyncKnown != c.known {
				t.Errorf("SyncBranch/known = %q/%v, want %q/%v", snap.SyncBranch, snap.SyncKnown, c.want, c.known)
			}
		})
	}
}

// El coste del fallback es exactamente un git, y solo en los repos que lo
// necesitan: la referencia global se intenta primero, falla, y entonces se
// prueba master. El log es donde se ve ese intento.
func TestSyncFallbackLanzaUnGitContraMaster(t *testing.T) {
	rec := installRecorder(t)
	dir := repoSinMain(t)
	Collect(t.Context(), dir, "main", true)

	var probadaMain, probadaMaster bool
	for _, e := range rec.Entries() {
		switch e.Command() {
		case "git rev-list --count HEAD..main":
			probadaMain = true
			if e.Exit == 0 {
				t.Error("main resolvió en un repo que no la tiene")
			}
		case "git rev-list --count HEAD..master":
			probadaMaster = true
			if e.Exit != 0 {
				t.Errorf("Exit = %d, want 0 (master sí existe)", e.Exit)
			}
		}
	}
	if !probadaMain {
		t.Error("no se intentó la referencia global antes de caer a master")
	}
	if !probadaMaster {
		t.Error("el fallback no llegó a probar master")
	}
}
