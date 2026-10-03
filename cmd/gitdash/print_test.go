// Tests del modo --print: las columnas y el orden de la tabla one-shot.
//
// El modo print es un camino de producción (lo usan scripts y hooks) cuya
// salida se lee con los ojos o se parsea desde fuera: un "0" de relleno o una
// columna que se invierte rompen al consumidor sin que se note en la TUI. Por
// eso las funciones de formateo se prueban una a una y con los bordes, no solo
// la tabla entera.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

// printSync muestra la desviación vs la sync branch con la rama visible:
// `<rama> ↓N`, `<rama>`, `<rama> —` (ref que no existe) o `—` sin rama.
func TestPrintSync(t *testing.T) {
	casos := []struct {
		nombre string
		snap   gitstatus.Snapshot
		want   string
	}{
		{"sin sync branch", gitstatus.Snapshot{}, "—"},
		{"ref que no existe", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: false}, "main —"},
		{"al día: la rama y nada más", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true}, "main"},
		{"con retraso", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true, SyncBehind: 3}, "main ↓3"},
		{"solo syncKnown false con retraso (la unknown manda)", gitstatus.Snapshot{SyncBranch: "main", SyncKnown: false, SyncBehind: 3}, "main —"},
	}
	for _, c := range casos {
		if got := printSync(c.snap); got != c.want {
			t.Errorf("%s: printSync = %q, want %q", c.nombre, got, c.want)
		}
	}
}

// printState es solo el working tree, y la tabla está quieta cuando no hay
// nada: un repo limpio sale con la celda vacía, no con un "0".
func TestPrintState(t *testing.T) {
	casos := []struct {
		nombre          string
		st              gitstatus.State
		tracked, untked int
		want            string
	}{
		{"error", gitstatus.StateError, 5, 5, "error"},
		{"sin repo", gitstatus.StateNoRepo, 0, 0, "no repo"},
		{"limpio", gitstatus.StateClean, 0, 0, ""},
		{"solo tracked", gitstatus.StateDirty, 2, 0, "2"},
		{"solo untracked", gitstatus.StateDirty, 0, 1, "?1"},
		{"ambos", gitstatus.StateDirty, 2, 1, "2 ?1"},
		{"diverged cuenta como sucio", gitstatus.StateDiverged, 1, 0, "1"},
	}
	for _, c := range casos {
		snap := gitstatus.Snapshot{Status: gitstatus.Status{TrackedChanges: c.tracked, Untracked: c.untked}}
		if got := printState(c.st, snap); got != c.want {
			t.Errorf("%s: printState = %q, want %q", c.nombre, got, c.want)
		}
	}
}

// printUpDown: no-up cuando no hay upstream trackeado, y vacío cuando no hay
// nada que decir (en sync, error o sin repo: un 0 de "↑0↓0" no informa).
func TestPrintUpDown(t *testing.T) {
	casos := []struct {
		nombre        string
		st            gitstatus.State
		hasUp         bool
		ahead, behind int
		err           string
		want          string
	}{
		{"sin repo", gitstatus.StateNoRepo, true, 3, 3, "", ""},
		{"con error", gitstatus.StateError, true, 3, 3, "fatal", ""},
		{"sin upstream", gitstatus.StateClean, false, 0, 0, "", "no-up"},
		{"en sync", gitstatus.StateClean, true, 0, 0, "", ""},
		{"solo ahead", gitstatus.StateAhead, true, 2, 0, "", "↑2"},
		{"solo behind", gitstatus.StateBehind, true, 0, 3, "", "↓3"},
		{"divergido", gitstatus.StateDiverged, true, 1, 2, "", "↑1↓2"},
		{"detached sin upstream", gitstatus.StateDetached, false, 0, 0, "", "no-up"},
	}
	for _, c := range casos {
		snap := gitstatus.Snapshot{
			Status: gitstatus.Status{HasUpstream: c.hasUp, Ahead: c.ahead, Behind: c.behind},
			Err:    c.err,
		}
		if got := printUpDown(c.st, snap); got != c.want {
			t.Errorf("%s: printUpDown = %q, want %q", c.nombre, got, c.want)
		}
	}
}

// relativeTimePrint: los mismos buckets que la TUI pero con el "ago", y "-" para
// lo que no es un epoch real (repo sin commits).
func TestRelativeTimePrint(t *testing.T) {
	ahora := time.Now()
	casos := []struct {
		nombre string
		edad   time.Duration
		epoch  int64
		want   string
	}{
		{"sin epoch", 0, 0, "-"},
		{"epoch negativo", 0, -5, "-"},
		{"ahora", 0, ahora.Unix(), "now"},
		{"minutos", 7 * time.Minute, 0, "7m ago"},
		{"horas", 5 * time.Hour, 0, "5h ago"},
		{"días", 3 * 24 * time.Hour, 0, "3d ago"},
	}
	for _, c := range casos {
		epoch := c.epoch
		if c.epoch == 0 && c.want != "-" {
			epoch = ahora.Add(-c.edad).Unix()
		}
		if got := relativeTimePrint(epoch); got != c.want {
			t.Errorf("%s: relativeTimePrint = %q, want %q", c.nombre, got, c.want)
		}
	}
}

func TestOrDashPrint(t *testing.T) {
	if got := orDashPrint(""); got != "-" {
		t.Errorf("orDashPrint(\"\") = %q, want -", got)
	}
	if got := orDashPrint("backend"); got != "backend" {
		t.Errorf("orDashPrint = %q, want el valor", got)
	}
}

// groupLabelPrint: `primary/secondary`, solo el primario, o vacío (que la
// columna GROUP convierte en "-").
func TestGroupLabelPrint(t *testing.T) {
	casos := []struct {
		nombre string
		p      discovery.Project
		want   string
	}{
		{"los dos niveles", discovery.Project{PrimaryGroup: "vsocial", SecondaryGroup: "backend"}, "vsocial/backend"},
		{"solo primario", discovery.Project{PrimaryGroup: "vsocial"}, "vsocial"},
		{"ninguno", discovery.Project{}, ""},
		{"solo secundario (no se muestra solo)", discovery.Project{SecondaryGroup: "backend"}, ""},
	}
	for _, c := range casos {
		if got := groupLabelPrint(c.p); got != c.want {
			t.Errorf("%s: groupLabelPrint = %q, want %q", c.nombre, got, c.want)
		}
	}
}

// hasProject decide si un worktree se omite de la tabla (porque su repo
// principal ya sale como fila).
func TestHasProject(t *testing.T) {
	projects := []discovery.Project{{Path: "/a"}, {Path: "/b"}}
	if !hasProject(projects, "/b") {
		t.Error("hasProject no encontró /b")
	}
	if hasProject(projects, "/c") {
		t.Error("hasProject encontró /c, que no está")
	}
	if hasProject(nil, "/a") {
		t.Error("hasProject encontró algo en una lista vacía")
	}
}

// El orden de la tabla es la misma promesa que en la TUI: atención-primero,
// luego el commit más reciente, luego el nombre. Se prueba con filas
// fabricadas porque sobre repos reales los commits caen en el mismo segundo y
// el desempate por fecha no se puede fijar.
func TestSortPrintRows(t *testing.T) {
	fila := func(name string, score, last int) printRow {
		return printRow{name: name, score: score, lastCommit: last}
	}
	for _, c := range []struct {
		nombre string
		rows   []printRow
		want   []string
	}{
		{
			"el score manda sobre la fecha",
			[]printRow{fila("clean-reciente", 1, 200), fila("dirty-viejo", 5, 100)},
			[]string{"dirty-viejo", "clean-reciente"},
		},
		{
			"a igual score, el commit más reciente",
			[]printRow{fila("viejo", 5, 100), fila("nuevo", 5, 200)},
			[]string{"nuevo", "viejo"},
		},
		{
			"a igual score y fecha, el nombre",
			[]printRow{fila("zeta", 5, 200), fila("alfa", 5, 200)},
			[]string{"alfa", "zeta"},
		},
		{
			"el nombre ignora mayúsculas",
			[]printRow{fila("Zeta", 5, 200), fila("alfa", 5, 200)},
			[]string{"alfa", "Zeta"},
		},
	} {
		rows := append([]printRow(nil), c.rows...)
		sortPrintRows(rows)
		var got []string
		for _, r := range rows {
			got = append(got, r.name)
		}
		if !equalStrings(got, c.want) {
			t.Errorf("%s: orden = %v, want %v", c.nombre, got, c.want)
		}
		// OJO: el orden de dos filas que empatan en las TRES claves no es un
		// contrato — sort.Slice no es estable, así que ni el código original ni
		// una mutación del comparador pueden prometer nada ahí.
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// La tabla lleva la cabecera con los nombres de columna y las celdas con el
// dato, sin inventar ceros de relleno.
func TestPrintTablaCabeceraYCeldas(t *testing.T) {
	root := t.TempDir()
	var sucio string
	for _, nombre := range []string{"sucio", "limpio"} {
		dir := filepath.Join(root, nombre)
		testutil.Init(t, dir)
		testutil.Marker(t, dir, nombre, "", "", false)
		testutil.CommitFiles(t, dir, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")
		if nombre == "sucio" {
			sucio = dir
		}
	}
	// Solo el sucio tiene cambios: el limpio se queda con la celda WT vacía.
	testutil.WriteUncommitted(t, sucio, map[string]string{"nuevo.txt": "x"})

	out := captureStdout(t, func() { runPrint(printConfig(root)) })
	plano := strings.Join(strings.Fields(out), " ")
	for _, cabecera := range []string{"NAME", "GROUP", "BRANCH", "WT", "↑↓up", "SYNC", "WTS", "ACTIVITY", "PATH"} {
		if !strings.Contains(plano, cabecera) {
			t.Errorf("falta la columna %q en la cabecera:\n%s", cabecera, plano)
		}
	}
	// El untracked sale como "?1" y no como "0 ?1" ni " ?1": con un marcador de
	// relleno delante la columna se desalinea y el consumidor ve un número que
	// no existe.
	if !strings.Contains(plano, "?1") {
		t.Errorf("el untracked no aparece:\n%s", plano)
	}
	if strings.Contains(plano, "0 ?1") {
		t.Errorf("la celda WT imprimió un 0 de tracked que no existe:\n%s", plano)
	}
	for _, nombre := range []string{"sucio", "limpio"} {
		if !strings.Contains(plano, nombre) {
			t.Errorf("falta el repo %q en la salida:\n%s", nombre, plano)
		}
	}
}

// printConfig es la config mínima que hace que print descubra el fixture.
func printConfig(root string) config.Config {
	cfg := config.Defaults()
	cfg.Roots = []string{root}
	cfg.Marker = ".gitdash.toml"
	cfg.SyncBranch = "main"
	return cfg
}

// Sin repos discovered, print lo dice y sale 0 (no una tabla vacía sin
// explicación: un script que parsea esto necesita distinguir "no hay" de "todo
// limpio").
func TestPrintSinRepos(t *testing.T) {
	vacio := t.TempDir()
	out := captureStdout(t, func() { runPrint(printConfig(vacio)) })
	if !strings.Contains(out, "no repositories found") {
		t.Errorf("sin repos, print = %q, want el aviso", out)
	}
}

// captureStdout ejecuta fn capturando todo lo escrito en os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// Un worktree cuyo repo principal esta DESCUBIERTO en la misma corrida no es una
// fila: sale plegado debajo de su repo, y en print (tabla plana, sin cabeceras
// de grupo) eso significa no imprimirla. Sin esto el worktree aparece como un
// repo mas y el recuento de repos de la tabla no cuadra con el de la TUI.
//
// El worktree lleva marcador propio a proposito (es lo que hace que lo
// descubra el walk) y sin `.git` no seria worktree: lo que activa el plegado es
// que `.git` sea un FICHERO `gitdir:`.
func TestPrintPlegaElWorktreeBajoSuRepoPrincipal(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "principal")
	testutil.Init(t, principal)
	testutil.Marker(t, principal, "principal", "", "", false)
	testutil.CommitFiles(t, principal, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")

	wt := filepath.Join(root, "feature")
	testutil.MakeWorktree(t, principal, wt, "feature")
	// El marcador del worktree se commitea CON su contenido: el worktree nace de
	// HEAD, que ya trae el `.gitdash.toml` del principal, y reescribirlo sin
	// commitear lo dejaria modificado para siempre.
	testutil.CommitFiles(t, wt, map[string]string{".gitdash.toml": "name = \"feature\"\n"}, "marcador del worktree")

	out := captureStdout(t, func() { runPrint(printConfig(root)) })
	plano := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(plano, "principal") {
		t.Fatalf("el repo principal no sale:\n%s", out)
	}
	if strings.Contains(plano, "feature") {
		t.Errorf("el worktree se imprimio como si fuera un repo mas:\n%s", out)
	}
}

// El caso limite del plegado: un worktree cuyo repo principal NO esta
// descubierto (esta fuera de los roots) SI se imprime. Si no, un worktree de un
// repo que no se escanea desapareceria de la tabla sin dejar rastro, que es peor
// que mostrarlo de mas.
func TestPrintNoPlegaElWorktreeSinPrincipalDescubierto(t *testing.T) {
	root := t.TempDir()
	// El repo principal vive FUERA del root que se escanea.
	fuera := t.TempDir()
	principal := filepath.Join(fuera, "oculto")
	testutil.Init(t, principal)
	testutil.Marker(t, principal, "oculto", "", "", false)
	testutil.CommitFiles(t, principal, map[string]string{"base.txt": "base", ".gitdash.toml": ""}, "base")

	wt := filepath.Join(root, "suelto")
	testutil.MakeWorktree(t, principal, wt, "suelto")
	testutil.CommitFiles(t, wt, map[string]string{".gitdash.toml": "name = \"suelto\"\n"}, "marcador del worktree")

	out := captureStdout(t, func() { runPrint(printConfig(root)) })
	plano := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(plano, "suelto") {
		t.Errorf("el worktree sin principal descubierto no sale:\n%s", out)
	}
}
