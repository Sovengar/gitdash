package gitstatus

import (
	"strconv"
	"strings"
	"sync"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/testutil"
)

// porcelainClean es la salida típica de un repo limpio sincronizado.
const porcelainClean = `# branch.oid 3f4e0c0f6a4e3c5a5d5b5e5a5d5b5e5a5d5b5e5a
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
`

func TestParseClean(t *testing.T) {
	st, files := ParsePorcelain(porcelainClean)
	if st.Branch != "main" || st.Upstream != "origin/main" || !st.HasUpstream {
		t.Errorf("st = %+v", st)
	}
	if st.Ahead != 0 || st.Behind != 0 || st.Dirty() != 0 || len(files) != 0 {
		t.Errorf("falla: %+v files=%v", st, files)
	}
	if got := st.Derive(); got != StateClean {
		t.Errorf("derive = %v, want clean", got)
	}
}

func TestParseAheadBehindDiverged(t *testing.T) {
	ahead := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +2 -0", 1)
	st, _ := ParsePorcelain(ahead)
	if st.Ahead != 2 || st.Behind != 0 {
		t.Errorf("ahead: %+v", st)
	}
	if st.Derive() != StateAhead {
		t.Errorf("ahead derive = %v", st.Derive())
	}

	behind := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +0 -3", 1)
	st, _ = ParsePorcelain(behind)
	if st.Ahead != 0 || st.Behind != 3 {
		t.Errorf("behind: %+v", st)
	}
	if st.Derive() != StateBehind {
		t.Errorf("behind derive = %v", st.Derive())
	}

	diverged := strings.Replace(porcelainClean, "# branch.ab +0 -0", "# branch.ab +2 -3", 1)
	st, _ = ParsePorcelain(diverged)
	if st.Derive() != StateDiverged {
		t.Errorf("diverged derive = %v", st.Derive())
	}
}

func TestParseDirty(t *testing.T) {
	out := porcelainClean + `1 .M NRM 100644 100644 100644 abc def src/main.go
? README.md
? docs/
2 R. N... 100644 100644 100644 abc def R100 new.txt	old.txt
`
	st, files := ParsePorcelain(out)
	if st.TrackedChanges != 2 || st.Untracked != 2 {
		t.Errorf("tracked=%d untracked=%d", st.TrackedChanges, st.Untracked)
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty", st.Derive())
	}
	if len(files) != 4 {
		t.Fatalf("files = %d, want 4", len(files))
	}
	if files[0].Path != "src/main.go" || files[0].Code != ".M" {
		t.Errorf("files[0] = %+v", files[0])
	}
	if files[1].Code != "??" {
		t.Errorf("files[1] = %+v", files[1])
	}
	if files[3].Path != "new.txt" {
		t.Errorf("rename path = %q, want new.txt", files[3].Path)
	}
}

func TestParseNoUpstream(t *testing.T) {
	st, _ := ParsePorcelain("# branch.oid abc\n# branch.head main\n")
	if st.HasUpstream {
		t.Error("upstream detectado sin línea")
	}
	if st.Derive() != StateNoUpstream {
		t.Errorf("derive = %v, want no-upstream", st.Derive())
	}
}

func TestParseDetached(t *testing.T) {
	out := strings.Replace(porcelainClean, "# branch.head main", "# branch.head (detached)", 1)
	st, _ := ParsePorcelain(out)
	if !st.Detached || st.Branch != "" {
		t.Errorf("detached: %+v", st)
	}
}

func TestParseUnbornBranch(t *testing.T) {
	st, _ := ParsePorcelain("# branch.oid (initial)\n# branch.head (main)\n")
	if st.Branch != "main" || st.Detached {
		t.Errorf("unborn: %+v", st)
	}
}

func TestParseLog(t *testing.T) {
	out := "abc1234\x001700000000\x00feat: uno\ndef5678\x001699000000\x00fix: dos\n"
	commits := ParseLog(out)
	if len(commits) != 2 {
		t.Fatalf("commits = %d", len(commits))
	}
	if commits[0].Sha != "abc1234" || commits[0].Subject != "feat: uno" || commits[0].When != 1700000000 {
		t.Errorf("commits[0] = %+v", commits[0])
	}
}

// --- tests de recolección real con repos fixture ---

func TestCollectClean(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	st := Collect(t.Context(), dir, "")
	if st.Err != "" {
		t.Fatalf("err = %q", st.Err)
	}
	if st.Status.Derive() != StateClean || st.LastCommit == 0 {
		t.Errorf("snap = %+v", snapSummary(st))
	}
}

func TestCollectDirty(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	// base.txt está trackeado: modificarlo cuenta como cambio tracked.
	testutil.WriteUncommitted(t, dir, map[string]string{"base.txt": "changed"})
	testutil.WriteUntracked(t, dir, map[string]string{"un1.txt": "x", "un2.txt": "y"})

	st := Collect(t.Context(), dir, "")
	if st.Status.TrackedChanges != 1 || st.Status.Untracked != 2 {
		t.Errorf("tracked=%d untracked=%d", st.Status.TrackedChanges, st.Status.Untracked)
	}
	if st.Status.Derive() != StateDirty {
		t.Errorf("derive = %v", st.Status.Derive())
	}
}

func TestCollectAheadBehindDiverged(t *testing.T) {
	ahead, _ := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, ahead, map[string]string{"extra.txt": "x"}, "local 1")
	testutil.CommitFiles(t, ahead, map[string]string{"extra2.txt": "x"}, "local 2")

	st := Collect(t.Context(), ahead, "")
	if st.Status.Ahead != 2 || st.Status.Derive() != StateAhead {
		t.Errorf("ahead: %+v", snapSummary(st))
	}

	behind, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 3, "behind-")
	testutil.FetchLocal(t, behind) // sin fetch el repo no ve el behind
	st = Collect(t.Context(), behind, "")
	if st.Status.Behind != 3 || st.Status.Derive() != StateBehind {
		t.Errorf("behind: %+v", snapSummary(st))
	}

	diverged, origin2 := testutil.NewRepo(t, true)
	testutil.CommitFiles(t, diverged, map[string]string{"l.txt": "l"}, "local")
	testutil.PushUpstreamCommits(t, origin2, 2, "div-")
	testutil.FetchLocal(t, diverged)
	st = Collect(t.Context(), diverged, "")
	if st.Status.Ahead != 1 || st.Status.Behind != 2 || st.Status.Derive() != StateDiverged {
		t.Errorf("diverged: %+v", snapSummary(st))
	}
}

func TestCollectNoUpstream(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	st := Collect(t.Context(), dir, "")
	if st.Status.HasUpstream || st.Status.Derive() != StateNoUpstream {
		t.Errorf("no-upstream: %+v", snapSummary(st))
	}
}

func TestCollectDetached(t *testing.T) {
	dir, _ := testutil.NewRepo(t, true)
	testutil.Detach(t, dir)
	st := Collect(t.Context(), dir, "")
	if !st.Status.Detached {
		t.Fatalf("detached: %+v", st.Status)
	}
	if len(st.Status.Branch) != 7 {
		t.Errorf("branch detached = %q, want sha corto", st.Status.Branch)
	}
}

func TestCollectCorrupt(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	// .git corrupto: el binario git falla y el error viaja en el Snapshot.
	testutil.BreakGit(t, dir)
	st := Collect(t.Context(), dir, "")
	if st.Err == "" {
		t.Fatal("corrupto sin error")
	}
	if st.State(true) != StateError {
		t.Errorf("state = %v, want error", st.State(true))
	}
}

// El detalle está acotado: la lista de ficheros se corta en maxFiles aunque el
// repo tenga más. El tope se comprueba en el exacto (100) y en el que lo pasa
// (101), que es donde la guarda `len >= maxFiles` se puede equivocar.
func TestParseListaFicherosAcotada(t *testing.T) {
	for _, tc := range []struct{ lineas, want int }{
		{maxFiles - 1, maxFiles - 1},
		{maxFiles, maxFiles},
		{maxFiles + 1, maxFiles},
		{maxFiles + 25, maxFiles},
	} {
		var b strings.Builder
		b.WriteString(porcelainClean)
		for i := 0; i < tc.lineas; i++ {
			b.WriteString("? file" + strconv.Itoa(i) + ".txt\n")
		}
		_, files := ParsePorcelain(b.String())
		if len(files) != tc.want {
			t.Errorf("con %d cambios files = %d, want %d (tope %d)", tc.lineas, len(files), tc.want, maxFiles)
		}
	}
}

// Una línea de entrada truncada (menos campos de los que el código exige) se
// descarta: se parsea lo que se pueda sin indexar fuera del slice. Una línea "1 "
// con 7 campos en vez de 8 es exactamente el borde de esa guarda.
func TestParseLineaTruncadaSeDescarta(t *testing.T) {
	casos := []struct {
		nombre, body string
		beforePath   int
	}{
		{"1 con 7 campos (le falta el path)", "M. NRM 100644 100644 100644 abc def", 7},
		{"1 con 6 campos", "M. NRM 100644 100644 abc def", 7},
		{"1 vacía", "", 7},
		{"2 con 7 campos (le falta el path)", "R. N... 100644 100644 abc", 8},
		{"u con 8 campos (le falta el path)", "UU N... 100644 100644 100644 abc def", 9},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, ok := parseEntry(c.body, c.beforePath); ok {
				t.Errorf("parseEntry(%q, %d) = true, want false (línea incompleta)", c.body, c.beforePath)
			}
		})
	}
	// Y el camino real: una línea "1 " truncada dentro del porcelain degrada con
	// elegancia — el cambio cuenta (el repo está sucio de verdad) pero la línea
	// no entra en la lista de ficheros ni revienta el parseo.
	st, files := ParsePorcelain(porcelainClean + "1 .M NRM 100644 100644 100644 abc def\n")
	if len(files) != 0 {
		t.Errorf("files = %d, want 0 (línea truncada descartada)", len(files))
	}
	if st.TrackedChanges != 1 {
		t.Errorf("tracked = %d, want 1 (el cambio cuenta aunque no se pueda detallar)", st.TrackedChanges)
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty (cambio truncado sigue contando)", st.Derive())
	}
}

// Un repo sin ningún commit es un estado legítimo (recién hecho `git init`):
// la recolección no debe explodear al no haber nada que tomar del log.
func TestCollectSinCommits(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir) // sin commit
	st := Collect(t.Context(), dir, "")
	if st.Err != "" {
		t.Fatalf("err = %q", st.Err)
	}
	if st.LastCommit != 0 {
		t.Errorf("LastCommit = %d, want 0 (sin commits)", st.LastCommit)
	}
	if len(st.Commits) != 0 {
		t.Errorf("commits = %d, want 0", len(st.Commits))
	}
}

// El log puede venir vacío con salida 0 (no es lo que hace un repo sin commits,
// que falla, pero un git envuelto puede hacerlo): la fecha del último commit es
// 0, no un índice fuera de rango.
func TestLastCommitWhen(t *testing.T) {
	casos := []struct {
		nombre  string
		commits []Commit
		want    int64
	}{
		{"nil", nil, 0},
		{"vacío", []Commit{}, 0},
		{"uno", []Commit{{Sha: "a", When: 1700000000}}, 1700000000},
		{"varios usa el primero", []Commit{{When: 99}, {When: 1}}, 99},
	}
	for _, c := range casos {
		if got := lastCommitWhen(c.commits); got != c.want {
			t.Errorf("%s: lastCommitWhen = %d, want %d", c.nombre, got, c.want)
		}
	}
}

// En detached sin rama, la columna de rama muestra el sha corto. El borde de
// ese recorte son los 7 caracteres exactos: con menos no hay nada que enseñar y
// con más se corta.
func TestNormalizeBranchDetachedShaCorto(t *testing.T) {
	casos := []struct {
		nombre string
		st     Status
		want   string
	}{
		{"sha de 7", Status{Detached: true, OID: "abc1234"}, "abc1234"},
		{"sha largo", Status{Detached: true, OID: "abc1234567890def"}, "abc1234"},
		{"sha de 6 (no llega al mínimo)", Status{Detached: true, OID: "abc123"}, ""},
		{"sin oid", Status{Detached: true}, ""},
		{"attached con rama", Status{Branch: "main", OID: "abc1234"}, "main"},
		{"attached con rama y sin o detached", Status{Branch: "main", OID: "abc1234", Detached: false}, "main"},
		{"detached pero con rama", Status{Detached: true, Branch: "main"}, "main"},
	}
	for _, c := range casos {
		if got := normalizeBranch(c.st); got != c.want {
			t.Errorf("%s: normalizeBranch(%+v) = %q, want %q", c.nombre, c.st, got, c.want)
		}
	}
}

// El pool nunca baja de 1 worker: con concurrency <= 0 (config con un valor
// inválido) tiene que recolectar igualmente, en serie, sin colgarse.
func TestStreamPoolConcurrenciaInvalida(t *testing.T) {
	dirs := make([]string, 3)
	projects := make([]discovery.Project, 3)
	for i := range dirs {
		d, _ := testutil.NewRepo(t, false)
		dirs[i] = d
		projects[i] = discovery.Project{Path: d, HasRepo: true}
	}
	for _, c := range []int{0, -1, 1, 2} {
		var mu sync.Mutex
		got := map[string]bool{}
		StreamPool(t.Context(), projects, "", c, func(path string, _ Snapshot) {
			mu.Lock()
			got[path] = true
			mu.Unlock()
		})
		if len(got) != len(projects) {
			t.Errorf("concurrency %d: emit = %d, want %d", c, len(got), len(projects))
		}
	}
}

func TestStreamPool(t *testing.T) {
	a, _ := testutil.NewRepo(t, true)
	b, _ := testutil.NewRepo(t, false)
	testutil.WriteUncommitted(t, b, map[string]string{"m.txt": "m"})

	projects := []discovery.Project{
		{Path: a, HasRepo: true},
		{Path: b, HasRepo: true},
	}
	got := map[string]State{}
	var mu sync.Mutex
	// emit se invoca concurrentemente (contrato de StreamPool): proteger el mapa.
	StreamPool(t.Context(), projects, "", 2, func(path string, st Snapshot) {
		mu.Lock()
		got[path] = st.State(true)
		mu.Unlock()
	})
	if len(got) != 2 {
		t.Fatalf("emit = %d paths, want 2", len(got))
	}
	if got[a] != StateClean || got[b] != StateDirty {
		t.Errorf("estados = %v", got)
	}
}

func snapSummary(s Snapshot) string {
	return s.Err + "|" + s.Status.Derive().String()
}

// Un conflicto (línea `u ` de porcelain v2) cuenta como cambio trackeado, y es el
// único caso donde el recuento de la columna de estado dice lo que hay que
// hacer: un repo con un conflicto y nada más tiene que salir como sucio, no
// limpio. parseEntry ya sabía leer la línea; lo que no estaba probado es que
// ParsePorcelain la cuente, y sin eso un `--` en el incremento pasaba unnoticed.
func TestParseConflictosCuentanComoSucio(t *testing.T) {
	// `u XY sub m1 m2 m3 mW h1 h2 h3 <path>`: nueve campos antes del path.
	out := porcelainClean + `u UU N... 100644 100644 100644 100644 abc def ghi conflicted.go
u AA N... 100644 100644 100644 100644 abc def ghi ambos-nuevos.go
`
	st, files := ParsePorcelain(out)
	if st.TrackedChanges != 2 {
		t.Errorf("tracked = %d, want 2 (los dos conflictos)", st.TrackedChanges)
	}
	if st.Untracked != 0 {
		t.Errorf("untracked = %d, want 0", st.Untracked)
	}
	if st.Dirty() != 2 {
		t.Errorf("Dirty = %d, want 2", st.Dirty())
	}
	if st.Derive() != StateDirty {
		t.Errorf("derive = %v, want dirty: un repo con un conflicto no está limpio", st.Derive())
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0].Code != "UU" || files[0].Path != "conflicted.go" {
		t.Errorf("files[0] = %+v", files[0])
	}
	// El código de la línea `u ` es el par XY de los dos lados, tal cual: "AA"
	// es un archivo añadido por los dos lados, y no debe reducirse a la primera
	// letra ni perderla.
	if files[1].Code != "AA" {
		t.Errorf("files[1].Code = %q, want AA (el par XY completo)", files[1].Code)
	}
}
