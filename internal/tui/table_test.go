// Tests de la tabla: orden de filas, reparto de columnas por ancho y las
// celdas. Son funciones puras o casi puras, así que los bordes se pueden fijar
// sin pelearse con el render completo.
package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

// rowDe construye una fila con el estado que sea, para no atar el test al
// score concrete de un estado (que es cosa de gitstatus, no de la tabla).
func rowDe(name string, state gitstatus.State, lastCommit int64) row {
	return row{
		project: discovery.Project{Path: "/tmp/" + name, Name: name, HasRepo: true},
		snap:    gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", HasUpstream: true}, LastCommit: lastCommit},
		state:   state,
	}
}

func nombresDe(rows []row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.project.Name)
	}
	return out
}

// El orden de la tabla es la promesa que hace que los repos con cambios salgan
// arriba: score primero, luego el más reciente, luego el nombre. Y el
// comparador tiene que ser una ORDEN ESTRICTA: si dos filas empataran en las tres
// claves y el comparador dijera "menor" en los dos sentidos, el insertion sort
// las intercambiaría y el orden dejaría de ser estable entre escaneos.
func TestSortRowsAtencionPrimero(t *testing.T) {
	ahora := time.Now().Unix()
	casos := []struct {
		nombre string
		rows   []row
		want   []string
	}{
		{
			nombre: "el score manda sobre la fecha",
			rows: []row{
				rowDe("clean-reciente", gitstatus.StateClean, ahora),
				rowDe("dirty-viejo", gitstatus.StateDirty, ahora-100000),
			},
			want: []string{"dirty-viejo", "clean-reciente"},
		},
		{
			nombre: "a igual score, el commit más reciente",
			rows: []row{
				rowDe("viejo", gitstatus.StateClean, ahora-100000),
				rowDe("nuevo", gitstatus.StateClean, ahora),
			},
			want: []string{"nuevo", "viejo"},
		},
		{
			nombre: "a igual score y fecha, el nombre",
			rows: []row{
				rowDe("zeta", gitstatus.StateClean, ahora),
				rowDe("alfa", gitstatus.StateClean, ahora),
			},
			want: []string{"alfa", "zeta"},
		},
		{
			nombre: "el nombre ignora mayúsculas",
			rows: []row{
				rowDe("Zeta", gitstatus.StateClean, ahora),
				rowDe("alfa", gitstatus.StateClean, ahora),
			},
			want: []string{"alfa", "Zeta"},
		},
		{
			nombre: "empate total conserva el orden de entrada",
			rows: []row{
				rowDe("mismo", gitstatus.StateClean, ahora),
				rowDe("mismo", gitstatus.StateClean, ahora),
			},
			want: []string{"mismo", "mismo"},
		},
	}
	for _, c := range casos {
		rows := append([]row(nil), c.rows...)
		sortRows(rows)
		if got := nombresDe(rows); !equalStrings(got, c.want) {
			t.Errorf("%s: orden = %v, want %v", c.nombre, got, c.want)
		}
	}

	// Los casos de arriba son todos de dos filas, que es el mínimo que hace
	// trabajo el segundo bucle. Con más filas el insertion sort tiene que seguir
	// metiendo elementos EN SU SITIO, no soloparable el primero con el segundo: un
	// recorrido que solo compara vecinos deja la lista a medio ordenar y dos
	// elementos ya ordenados.
	for _, n := range []int{0, 1, 3, 5, 9, 17} {
		t.Run(fmt.Sprintf("%d filas ya ordenadas", n), func(t *testing.T) {
			rows := make([]row, 0, n)
			for i := range n {
				// score decreciente: el orden correcto es el de entrada.
				rows = append(rows, rowDe(fmt.Sprintf("r%02d", i), gitstatus.StateDirty, ahora-int64(i)))
			}
			sortRows(rows)
			want := nombresDe(append([]row(nil), rows...))
			for i, got := range nombresDe(rows) {
				if got != want[i] {
					t.Fatalf("n=%d: la fila %d quedó como %q, want %q (la lista no está ordenada)", n, i, got, want[i])
				}
			}
		})
		// Al revés: el commit CRECE con el índice, así que el orden correcto es
		// r(n-1) … r00. Todos con el mismo score, para que solo mande la fecha.
		t.Run(fmt.Sprintf("%d filas al revés", n), func(t *testing.T) {
			rows := make([]row, 0, n)
			for i := range n {
				rows = append(rows, rowDe(fmt.Sprintf("r%02d", i), gitstatus.StateDirty, ahora-int64(n-i)))
			}
			sortRows(rows)
			for i, got := range nombresDe(rows) {
				if want := fmt.Sprintf("r%02d", n-1-i); got != want {
					t.Fatalf("n=%d: la fila %d quedó como %q, want %q", n, i, got, want)
				}
			}
		})
	}

	// rowLess no puede decir "menor" en los dos sentidos: es lo que rompe el
	// insertion sort cuando dos filas empatan.
	a := rowDe("x", gitstatus.StateClean, ahora)
	b := rowDe("x", gitstatus.StateClean, ahora)
	if rowLess(a, b) && rowLess(b, a) {
		t.Error("rowLess dice 'menor' en las dos direcciones con filas idénticas")
	}
}

// fitColumns es el contrato de ancho de la tabla: una columna entra si cabe
// ENTELA, y siempre entra al menos NAME. El borde (columna que llena el ancho
// justo) es donde un "> " mal puesto esconde una columna que sí cabe.
func TestFitColumnsEncajeExacto(t *testing.T) {
	total := 0
	for _, c := range tableColumns {
		total += c.width
	}
	if total != colName+colBranch+colWT+colUpDown+colSync+colActivity+colFetch {
		t.Fatalf("precondición: el ancho total son %d", total)
	}

	// Un ancho por debajo de todo: solo NAME, nunca cero columnas.
	if got := fitColumns(0); got != 1 {
		t.Errorf("inner=0: %d columnas, want 1 (NAME nunca cae)", got)
	}
	if got := fitColumns(-10); got != 1 {
		t.Errorf("inner=-10: %d columnas, want 1", got)
	}
	// Cada columna entra justo cuando su ancho acumulado se alcanza EXACTO.
	acum := 0
	for i, c := range tableColumns {
		acum += c.width
		if got := fitColumns(acum); got != i+1 {
			t.Errorf("inner=%d (ancho de %s exacto): %d columnas, want %d", acum, c.title, got, i+1)
		}
		// Una celda menos: la columna ya no entra. El suelo es 1 (NAME nunca
		// cae), así que para la primera columna el resultado sigue siendo 1.
		if want := max(1, i); fitColumns(acum-1) != want {
			t.Errorf("inner=%d (una celda menos de %s): %d columnas, want %d", acum-1, c.title, fitColumns(acum-1), want)
		}
	}
	// Y con espacio de sobra, todas.
	if got := fitColumns(total); got != len(tableColumns) {
		t.Errorf("inner=%d: %d columnas, want %d", total, got, len(tableColumns))
	}
	if got := fitColumns(total + 50); got != len(tableColumns) {
		t.Errorf("inner=%d: %d columnas, want %d", total+50, got, len(tableColumns))
	}
}

// El ancho que se descuenta (los bordes y la sangría de "  ") es 4: la
// cabecera se compone sobre `width-4`. Si se sumara en vez de restarse, un
// terminal estrecho enseñaría columnas de más que se saldrían del borde. La
// cabecera es exactamente la lista de columnas que caben, con su ancho.
func TestCabeceraUsaElAnchoInterior(t *testing.T) {
	for _, width := range []int{24, 30, 40, 55, 60, 80, 120, 200} {
		want := fitColumns(width - 4)
		var esperado strings.Builder
		for _, c := range tableColumns[:want] {
			esperado.WriteString(pad(c.title, c.width))
		}
		if got := headerColumns(width); got != esperado.String() {
			t.Errorf("width=%d: cabecera = %q, want %q", width, got, esperado.String())
		}
	}
}

// dirtyTail es "N ?M" y solo pone las partes que HAY: un repo con 3 untracked y
// ningún tracked es "?3", no "0 ?3". Un "0" a la izquierda del separador
// parece un dato y no lo es.
func TestDirtyTailSoloPoneLoQueHay(t *testing.T) {
	casos := []struct {
		nombre          string
		tracked, untked int
		want            string
	}{
		{"limpio", 0, 0, ""},
		{"solo tracked", 2, 0, "2"},
		{"solo untracked", 0, 3, "?3"},
		{"ambos", 2, 3, "2 ?3"},
	}
	for _, c := range casos {
		s := gitstatus.Status{TrackedChanges: c.tracked, Untracked: c.untked}
		if got := dirtyTail(row{snap: gitstatus.Snapshot{Status: s}}); got != c.want {
			t.Errorf("%s: dirtyTail = %q, want %q", c.nombre, got, c.want)
		}
	}
}

// Una celda vacía no debe llevar el color del estado: la columna Work Tree de
// un repo que solo va ahead se pinta con el estilo de "clean" (nada), no con
// el de "ahead". Es la diferencia entre una tabla quieta y una que insinúa.
func TestWtCellVaciaNoHeredaElColorDelEstado(t *testing.T) {
	m := newTestModel(t, nil, nil)
	casos := []struct {
		nombre string
		state  gitstatus.State
		want   string
	}{
		{"ahead sin cambios", gitstatus.StateAhead, ""},
		{"behind sin cambios", gitstatus.StateBehind, ""},
		{"clean", gitstatus.StateClean, ""},
		{"no-upstream sin cambios", gitstatus.StateNoUpstream, ""},
	}
	for _, c := range casos {
		s := gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", Ahead: 2}}
		if c.state == gitstatus.StateBehind {
			s.Status = gitstatus.Status{Branch: "main", Behind: 2}
		}
		if c.state == gitstatus.StateNoUpstream {
			s.Status = gitstatus.Status{Branch: "main"}
		}
		got, style := m.wtCell(row{snap: s, state: c.state})
		if got != c.want {
			t.Errorf("%s: wtCell texto = %q, want %q", c.nombre, got, c.want)
		}
		if want := styleClean.Render(c.want); style.Render(got) != want {
			t.Errorf("%s: wtCell estilo = %q, want el de clean %q", c.nombre, style.Render(got), want)
		}
	}
	// Y un repo sucio sí lleva su color de estado.
	s := gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", TrackedChanges: 2}}
	if got, style := m.wtCell(row{snap: s, state: gitstatus.StateDirty}); got != "2" || style.Render(got) != styleDirty.Render(got) {
		t.Errorf("dirty: wtCell = %q con estilo %q, want \"2\" con el de dirty", got, style.Render(got))
	}
}

// Un marcador malformado tiene que verse en el nombre (con el estilo de
// aviso), no pasar por un nombre normal: es la única señal de que el
// .gitdash.toml de ese repo está roto.
func TestNameCellAvisaDelMarcadorRoto(t *testing.T) {
	m := newTestModel(t, nil, nil)
	bueno := discovery.Project{Path: "/tmp/ok", Name: "ok", HasRepo: true}
	roto := discovery.Project{Path: "/tmp/roto", Name: "roto", HasRepo: true, MarkerErr: "línea 3: valor inválido"}

	if _, style := m.nameCell(row{project: bueno, state: gitstatus.StateClean}); style.Render("x") != styleSel.Render("x") {
		t.Error("un marcador sano no debe llevar el estilo de aviso")
	}
	nombre, style := m.nameCell(row{project: roto, state: gitstatus.StateClean})
	if style.Render(nombre) != styleWarn.Render(nombre) {
		t.Errorf("marcador roto: estilo = %q, want el de aviso", style.Render(nombre))
	}
	if !strings.Contains(nombre, "roto") {
		t.Errorf("marcador roto: nombre = %q", nombre)
	}
}

// El worktree de un repo cuenta en el agregado del grupo. Con la guarda mal
// puesta (aceptando un cero) el "wt" desaparece de todos los resúmenes.
func TestGroupStatsCuentaLosWorktrees(t *testing.T) {
	conWt := row{project: discovery.Project{Path: "/a", PrimaryGroup: "g", HasRepo: true},
		snap: gitstatus.Snapshot{
			Status:    gitstatus.Status{Branch: "main"},
			Worktrees: []gitstatus.Worktree{{Path: "/a/w1"}, {Path: "/a/w2"}},
		},
		state: gitstatus.StateClean}
	sinWt := row{project: discovery.Project{Path: "/b", PrimaryGroup: "g", HasRepo: true},
		snap:  gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main"}},
		state: gitstatus.StateClean}

	m := newTestModel(t, []discovery.Project{conWt.project, sinWt.project},
		map[string]gitstatus.Snapshot{"/a": conWt.snap, "/b": sinWt.snap})
	m.collapsed = map[string]bool{"g": true} // plegado: el agregado NO cambia

	st := m.groupStats("g")
	if st.repos != 2 {
		t.Errorf("repos = %d, want 2", st.repos)
	}
	if st.worktrees != 2 {
		t.Errorf("worktrees = %d, want 2 (los del repo con wt)", st.worktrees)
	}
	// Y un grupo sin worktrees no inventa ninguno.
	if st := m.groupStats(group.Ungrouped); st.worktrees != 0 {
		t.Errorf("grupo sin worktrees = %d, want 0", st.worktrees)
	}
}

// relativeTime es la columna ACTIVITY: los buckets son los que usa el ojo
// ("hace 2d"), así que el borde entre ellos tiene que caer donde dice.
func TestRelativeTimeBuckets(t *testing.T) {
	ahora := time.Now()
	casos := []struct {
		nombre string
		edad   time.Duration
		want   string
	}{
		{"futuro o epoch 0", -time.Hour, "-"},
		{"ahora", 0, "now"},
		{"minutos", 5 * time.Minute, "5m"},
		{"horas", 3 * time.Hour, "3h"},
		{"días", 3 * 24 * time.Hour, "3d"},
		{"semanas", 3 * 7 * 24 * time.Hour, "3w"},
		{"meses", 3 * 30 * 24 * time.Hour, "3mo"},
		{"años", 400 * 24 * time.Hour, "13mo"},

		// Los bordes. Los casos de arriba están en el CENTRO de cada bucket y
		// ningún mutante de la guarda (`<` por `>`) los alcanza: con el signo
		// cambiado, 3h cae en el bucket de días y da "0d", no "3h"... salvo que
		// el centro de un bucket ancho dé el mismo número tras el salto, que es
		// justo lo que pasa con "3mo" (3 meses = 90 días, y 90 días cae igual en
		// el bucket de meses con el signo cambiado).
		//
		// Lo que sí distingue son los bordes exactos, porque ahí el signo decide
		// si el valor cae en este bucket o en el siguiente: 59m es "59m" con `<`
		// y "1h" con `>`.
		{"justo antes de la hora", 59*time.Minute + 59*time.Second, "59m"},
		{"justo en la hora", 60 * time.Minute, "1h"},
		{"justo antes del día", 23*time.Hour + 59*time.Minute, "23h"},
		{"justo en el día", 24 * time.Hour, "1d"},
		{"justo antes de la semana", 7*24*time.Hour - time.Hour, "6d"},
		{"justo en la semana", 7 * 24 * time.Hour, "1w"},
		{"justo antes del mes", 30*24*time.Hour - time.Hour, "4w"},
		{"justo en el mes", 30 * 24 * time.Hour, "1mo"},
	}
	for _, c := range casos {
		epoch := ahora.Add(-c.edad).Unix()
		if c.want == "-" {
			epoch = 0
		}
		if got := relativeTime(epoch); got != c.want {
			t.Errorf("%s: relativeTime(-%s) = %q, want %q", c.nombre, c.edad, got, c.want)
		}
	}
}

// pad rellena a un ancho EXACTO y nunca trunca: es lo que permite que una
// celda ocupe su columna aunque el texto sea más corto.
func TestPadRellenaAlAncho(t *testing.T) {
	for _, c := range []struct {
		nombre, in string
		w          int
		want       string
	}{
		{"rellena", "ab", 5, "ab   "},
		{"exacto", "abcde", 5, "abcde"},
		{"ancho 0", "abc", 0, "abc"},
		{"ancho negativo", "abc", -2, "abc"},
		{"vacío", "", 3, "   "},
	} {
		if got := pad(c.in, c.w); got != c.want {
			t.Errorf("%s: pad(%q, %d) = %q, want %q", c.nombre, c.in, c.w, got, c.want)
		}
	}
}

// syncOf y nameOf resuelven por path: si el bucle se parara en el primer
// proyecto en vez de en el que COINCIDE, el segundo repo (y los siguientes)
// heredarían el sync branch y el nombre del primero.
func TestSyncOfYNombreResuelvenElPathQueCorresponde(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/a", Name: "alpha", SyncBranch: "main", HasRepo: true},
		{Path: "/b", Name: "beta", SyncBranch: "develop", HasRepo: true},
		{Path: "/c", Name: "gamma", HasRepo: true}, // sin override: global
		{Path: "/d", Name: "delta", IsWorktree: true, HasRepo: true},
	}
	m := newTestModel(t, projects, map[string]gitstatus.Snapshot{})
	m.cfg.SyncBranch = "global"

	for _, c := range []struct{ path, sync, name string }{
		{"/a", "main", "alpha"},
		{"/b", "develop", "beta"},
		{"/c", "global", "gamma"},
		{"/d", "global", "d"}, // un worktree se nombra por su directorio
		{"/no-descubierto", "global", "no-descubierto"},
		{"", "global", ""},
	} {
		if got, _ := m.syncOf(c.path); got != c.sync {
			t.Errorf("syncOf(%q) = %q, want %q", c.path, got, c.sync)
		}
		if got := m.nameOf(c.path); got != c.name {
			t.Errorf("nameOf(%q) = %q, want %q", c.path, got, c.name)
		}
	}
}

// El segundo valor de syncOf decide si la referencia puede caer a master, y es
// la precedencia completa de la feature: solo el default (nadie la declaró)
// la admite. Un override del marcador o una config global explícita se
// respetan tal cual, aunque la rama no exista: cambiarles la referencia en
// silencio taparía un error de configuración detrás de un número plausible.
func TestSyncOfFallbackSoloEnElDefault(t *testing.T) {
	casos := []struct {
		nombre   string
		project  discovery.Project
		global   string
		explicit bool
		want     string
		wantFB   bool
	}{
		{"default sin declarar", discovery.Project{Path: "/a"}, "main", false, "main", true},
		{"marcador declarado", discovery.Project{Path: "/a", SyncBranch: "develop"}, "main", false, "develop", false},
		{"config global explícita", discovery.Project{Path: "/a"}, "release", true, "release", false},
		{"marcador sobre global explícita", discovery.Project{Path: "/a", SyncBranch: "develop"}, "release", true, "develop", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m := newTestModel(t, []discovery.Project{c.project}, map[string]gitstatus.Snapshot{})
			m.cfg.SyncBranch = c.global
			m.cfg.SyncBranchExplicit = c.explicit
			got, gotFB := m.syncOf("/a")
			if got != c.want || gotFB != c.wantFB {
				t.Errorf("syncOf = (%q, %v), want (%q, %v)", got, gotFB, c.want, c.wantFB)
			}
		})
	}
}

// worktreeIndent es la sangría que las sub-filas suman a la de las filas de
// repo: el glyph "  ↳ " va dentro de la celda NAME, y la caja se indenta una
// celda más para que el worktree se lea como hijo de su repo.
const worktreeIndent = 2

// La cabecera y las filas deben PINTAR LAS MISMAS COLUMNAS. Es el invariante
// que sostiene la legibilidad de la tabla: si la fila calculara su ancho con
// otro criterio que la cabecera, cada dato caería debajo de una columna que no
// existe (o la fila se saldría del borde, que es lo que tapa el box).
func TestCabeceraYFilasCabenLasMismasColumnas(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	entries := m.entries()

	for _, width := range []int{24, 30, 40, 55, 60, 80, 120, 200} {
		m.width = width
		// La fila se compone con 2 de sangría + el ancho de sus columnas; la
		// cabecera, solo con el de las suyas. Si eligen distinto número de
		// columnas, los anchos no cuadran.
		anchoCabecera := len([]rune(headerColumns(width)))
		for _, e := range entries {
			if e.kind != kindRepo {
				continue
			}
			fila := stripANSI(m.renderRow(e.r, false))
			if got := len([]rune(fila)) - 2; got != anchoCabecera {
				t.Errorf("width=%d: la fila de %s compone %d celdas y la cabecera %d",
					width, e.r.project.Name, got, anchoCabecera)
			}
		}
	}

	// La sub-fila de worktree compone las suyas por su cuenta (con su sangría
	// y su glyph), así que necesita su propio fixture: un modelo sin worktrees
	// no tiene entradas de ese tipo y la comprobación no miraría nada.
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"), wt("/tmp/wt-b", "b"))
	mw := newTestModel(t, []discovery.Project{p}, st)
	mw, _ = press(mw, "enter") // despliega los worktrees
	var vistas int
	for _, e := range mw.entries() {
		if e.kind != kindWorktree {
			continue
		}
		vistas++
		for _, width := range []int{24, 40, 60, 80, 120} {
			mw.width = width
			fila := stripANSI(mw.renderWorktreeRow(e.wt, false))
			// La sub-fila es la fila de repo con la misma sangría ("  ") más el
			// glyph ↳ ya dentro de NAME, así que mide lo mismo que la suma de las
			// columnas + 2. Comparar contra la suma (y no contra la longitud de
			// otra fila) es lo que ata este ancho al reparto.
			esperado := worktreeIndent
			for _, c := range tableColumns[:fitColumns(width-4)] {
				esperado += c.width
			}
			if len([]rune(fila)) != esperado {
				t.Errorf("width=%d: la fila de worktree mide %d, want %d (columnas + sangría): %q",
					width, len([]rune(fila)), esperado, fila)
			}
		}
	}
	if vistas == 0 {
		t.Fatal("el fixture no produjo sub-filas de worktree: la comprobación sería vacía")
	}
}

// --- ramas que la fixture no tenía ---

// fixtureProjects no declara grupos ni incluye un repo con error, así que tres
// ramas de las celdas nunca se pintaron: el grupo compuesto, la celda de rama
// de un repo que no es repo, y el contador de errores del resumen de grupo. Las
// tres son cosas que el usuario ve en pantalla, y sin un test el mutante que
// las rompe pasa.
func fixtureConGruposYErrores() ([]discovery.Project, map[string]gitstatus.Snapshot) {
	projects := []discovery.Project{
		{Path: "/tmp/g-doble", Name: "doble", HasRepo: true, PrimaryGroup: "vsocial", SecondaryGroup: "backend"},
		{Path: "/tmp/g-solo", Name: "solo", HasRepo: true, PrimaryGroup: "vsocial"},
		{Path: "/tmp/g-sin", Name: "sin", HasRepo: true},
		{Path: "/tmp/g-error", Name: "roto", HasRepo: true, PrimaryGroup: "vsocial"},
		{Path: "/tmp/g-norepo", Name: "norepo", HasRepo: false, PrimaryGroup: "vsocial"},
		{Path: "/tmp/g-sinrama", Name: "sinrama", HasRepo: true, PrimaryGroup: "vsocial"},
	}
	states := map[string]gitstatus.Snapshot{
		"/tmp/g-doble": snapClean(),
		"/tmp/g-solo":  snapClean(),
		"/tmp/g-sin":   snapClean(),
		"/tmp/g-error": {Status: gitstatus.Status{Branch: "main"}, Err: "fatal: no escribo"},
		// Un repo con repo pero sin rama: un unborn HEAD. La celda tiene que
		// distinguirlo del "no repo": no es lo mismo no tener repositorio que
		// tenerlo y no tener rama donde mirar.
		"/tmp/g-norepo":  {},
		"/tmp/g-sinrama": {Status: gitstatus.Status{}},
	}
	return projects, states
}

// El rótulo del grupo son dos niveles: "primario/secundario" cuando hay los dos,
// solo el primario cuando no, y vacío cuando no hay grupo (la columna lo
// convierte en "-").
func TestGroupLabelDeDosNiveles(t *testing.T) {
	casos := []struct {
		nombre string
		p      discovery.Project
		want   string
	}{
		{"los dos niveles", discovery.Project{PrimaryGroup: "vsocial", SecondaryGroup: "backend"}, "vsocial/backend"},
		{"solo primario", discovery.Project{PrimaryGroup: "vsocial"}, "vsocial"},
		{"sin grupo", discovery.Project{}, ""},
		{"solo secundario no se muestra (un nivel no anida)", discovery.Project{SecondaryGroup: "backend"}, ""},
	}
	for _, c := range casos {
		if got := groupLabel(c.p); got != c.want {
			t.Errorf("%s: groupLabel = %q, want %q", c.nombre, got, c.want)
		}
	}
	if got := groupKey("vsocial", "backend"); got != "vsocial/backend" {
		t.Errorf("groupKey = %q, want vsocial/backend", got)
	}
}

// La celda de rama: "-" para un repo sin repositorio y para uno sin rama (un
// unborn HEAD), el nombre con "(detached)" colgado cuando toca, y el nombre a
// secas en el caso normal.
func TestBranchCellSinRamaYDetached(t *testing.T) {
	m := newTestModel(t, []discovery.Project{}, map[string]gitstatus.Snapshot{})
	for _, c := range []struct {
		nombre string
		r      row
		want   string
	}{
		{"sin repo", row{state: gitstatus.StateNoRepo}, "-"},
		{"repo sin rama (unborn HEAD)", row{state: gitstatus.StateClean}, "-"},
		{"rama normal", row{state: gitstatus.StateClean, snap: gitstatus.Snapshot{Status: gitstatus.Status{Branch: "feat/x"}}}, "feat/x"},
		{"detached", row{state: gitstatus.StateDetached, snap: gitstatus.Snapshot{Status: gitstatus.Status{Branch: "abc123", Detached: true}}}, "abc123 (detached)"},
	} {
		got, _ := m.branchCell(c.r)
		if got != c.want {
			t.Errorf("%s: branchCell = %q, want %q", c.nombre, got, c.want)
		}
	}
}

// El resumen de un grupo cuenta los repos CON error, que no es lo mismo que
// no contarlos: es lo que hace que el header de un grupo con un repo roto
// avise en vez de parecer limpio.
func TestGroupStatsCuentaLosErrores(t *testing.T) {
	projects, states := fixtureConGruposYErrores()
	m := newTestModel(t, projects, states)

	// El grupo "vsocial" tiene los cuatro repos con grupo, uno de ellos roto.
	st := m.groupStats(groupKey("vsocial", ""))
	if st.repos != 4 {
		t.Errorf("repos = %d, want 4", st.repos)
	}
	if st.errors != 1 {
		t.Errorf("errors = %d, want 1 (el repo con gitstatus.Err no cuenta como limpio)", st.errors)
	}
	// Y un grupo que no existe no inventa nada.
	if st := m.groupStats("nada/aqui"); st.repos != 0 || st.errors != 0 {
		t.Errorf("grupo inexistente = %+v, want todo a cero", st)
	}
}

// relativeTime recorre sus seis franjas: now / minutos / horas / días / semanas
// / meses. Con la última sin cubrir, un "8mo" podría volverse "2400d" sin que
// nada se quejara.
func TestRelativeTimeRecorreSusFranjas(t *testing.T) {
	ahora := time.Now().Unix()
	for _, c := range []struct {
		nombre string
		edad   time.Duration
		want   string
	}{
		{"recién", 0, "now"},
		{"minutos", 5 * time.Minute, "5m"},
		{"horas", 3 * time.Hour, "3h"},
		{"días", 2 * 24 * time.Hour, "2d"},
		{"semanas", 10 * 24 * time.Hour, "1w"},
		{"meses", 60 * 24 * time.Hour, "2mo"},
		// Y los bordes, con un caso a cada lado de cada umbral. Un caso "3h"
		// no distingue un umbral de 24h de uno de 23h: hace falta una edad que
		// caiga ENTRE los dos, que es justo lo que un mutante de borde mueve.
		// (La edad real es la pedida más los microsegundos que tarda el test,
		// así que los casos van por debajo del umbral, nunca justo encima.)
		{"59s todavía es now", 59 * time.Second, "now"},
		{"30m ya son minutos", 30 * time.Minute, "30m"},
		{"1h1m todavía son horas", time.Hour + time.Minute, "1h"},
		{"23h59m todavía son horas", 23*time.Hour + 59*time.Minute, "23h"},
		{"6d23h todavía son días", 6*24*time.Hour + 23*time.Hour, "6d"},
		{"7d1h ya son semanas", 7*24*time.Hour + time.Hour, "1w"},
		{"29d23h todavía son semanas", 29*24*time.Hour + 23*time.Hour, "4w"},
		{"30d1h ya son meses", 30*24*time.Hour + time.Hour, "1mo"},
	} {
		if got := relativeTime(ahora - int64(c.edad.Seconds())); got != c.want {
			t.Errorf("%s: relativeTime = %q, want %q", c.nombre, got, c.want)
		}
	}
	if got := relativeTime(0); got != "-" {
		t.Errorf("sin epoch: relativeTime = %q, want -", got)
	}
	if got := relativeTime(-5); got != "-" {
		t.Errorf("epoch negativo: relativeTime = %q, want -", got)
	}
}

// stripANSI quita las secuencias de SGR (termina en 'm') y las de erase (en 'K',
// que es lo que pinta el cursor), y NO se come texto normal que parece
// una secuencia de escape.
func TestStripANSISoloQuitaSecuencias(t *testing.T) {
	if got := stripANSI("\x1b[31mrojo\x1b[0m"); got != "rojo" {
		t.Errorf("SGR: %q, want rojo", got)
	}
	if got := stripANSI("antes\x1b[2Kapués"); got != "antesapués" {
		t.Errorf("erase: %q, want antesapués", got)
	}
	if got := stripANSI("sin escapes"); got != "sin escapes" {
		t.Errorf("sin escapes: %q", got)
	}
	// Una 'm' suelta después de un ESC también cierra la secuencia: el saneo no
	// distingue SGR de erase, cierra con cualquiera de las dos.
	if got := stripANSI("x\x1b[1m"); got != "x" {
		t.Errorf("secuencia sin cerrar: %q, want x", got)
	}
	// El texto normal se conserva entero, incluidos los signos que un consumidor
	// naïvo confundiría.
	if got := stripANSI("↑2 ↓0 ¿q?"); got != "↑2 ↓0 ¿q?" {
		t.Errorf("texto con símbolos: %q", got)
	}
}

// styleFor mapea cada estado derivado a un estilo. El mapa entero se comprueba
// contra el NOMBRE del estilo, no contra otro estado: lo que importa es que un
// repo divergido no se pinte como uno dirty, y comparar estilos entre si no lo
// diria (styleError y styleDirty serian "distintos" igual).
func TestStylePorEstado(t *testing.T) {
	m := newTestModel(t, nil, nil)
	for _, c := range []struct {
		state gitstatus.State
		want  string
	}{
		{gitstatus.StateError, styleError.Render("x")},
		{gitstatus.StateNoRepo, styleDim.Render("x")},
		{gitstatus.StateNoUpstream, styleWarn.Render("x")},
		{gitstatus.StateDetached, styleWarn.Render("x")},
		{gitstatus.StateDiverged, styleDiverged.Render("x")},
		{gitstatus.StateDirty, styleDirty.Render("x")},
		{gitstatus.StateAhead, styleAhead.Render("x")},
		{gitstatus.StateBehind, styleBehind.Render("x")},
	} {
		got := m.styleFor(row{state: c.state})
		if got.Render("x") != c.want {
			t.Errorf("styleFor(%v) = %q, want %q", c.state, got.Render("x"), c.want)
		}
	}
}

// El header de un primario lleva glifo de expandido o de colapsado. El glifo es
// lo UNICO que distingue "pulsar enter despliega" de "pulsar enter pliega", asi
// que se comprueba el glifo y no el texto entero.
func TestHeaderPrimarioCambiaGlifoAlPlegar(t *testing.T) {
	m := newTestModel(t, nil, nil)
	m.projects = []discovery.Project{
		{Path: "/a", Name: "a", PrimaryGroup: "backend", HasRepo: true},
	}
	if got := m.primaryHeaderLine("backend"); !strings.Contains(got, "▾") {
		t.Errorf("header expandido = %q, want el glifo ▾", got)
	}
	m.collapsed = map[string]bool{"backend": true}
	got := m.primaryHeaderLine("backend")
	if !strings.Contains(got, "▸") || strings.Contains(got, "▾") {
		t.Errorf("header colapsado = %q, want el glifo ▸", got)
	}
}

// El worktree sintetico de un repo NO cuenta como repos en la barra, y eso se
// decide con worktreeHidden: si el worktree tiene MainRepo y ese repo esta
// entre los descubiertos, es una fila plegada y no un proyecto. Sin el, la
// cuenta de la barra sube con cada worktree.
func TestSummaryNoCuentaWorktreesPlegados(t *testing.T) {
	main := discovery.Project{Path: "/repos/api", Name: "api", PrimaryGroup: "backend", HasRepo: true}
	wt := discovery.Project{
		Path: "/repos/api-wt", Name: "api-wt",
		PrimaryGroup: "backend", HasRepo: true,
		IsWorktree: true, MainRepo: main.Path,
	}
	solo := newTestModel(t, []discovery.Project{main}, map[string]gitstatus.Snapshot{
		main.Path: snapClean(),
	})
	conWt := newTestModel(t, []discovery.Project{main, wt}, map[string]gitstatus.Snapshot{
		main.Path: snapClean(),
		wt.Path:   snapClean(),
	})
	a, _, _, _ := solo.summary()
	b, _, _, _ := conWt.summary()
	if a != b {
		t.Errorf("summary con worktree plegado = %d repos, want %d (no cuenta)", b, a)
	}

	// Y si el repo principal NO esta entre los descubiertos, el worktree si
	// cuenta: esta solo, es la unica fila de su repo.
	huerfano := newTestModel(t, []discovery.Project{wt}, map[string]gitstatus.Snapshot{
		wt.Path: snapClean(),
	})
	c, _, _, _ := huerfano.summary()
	if c != 1 {
		t.Errorf("summary con worktree huerfano = %d repos, want 1 (no hay principal que lo oculte)", c)
	}
}

// repoExpanded tiene tres caminos: expandido por el mapa, inducido por una
// busqueda que matchea un worktree, y colapsado. El segundo es el que hace que
// una busqueda "feature" muestre los worktrees aunque el repo este plegado.
func TestRepoExpandedInducidoPorBusqueda(t *testing.T) {
	proj := discovery.Project{Path: "/api", Name: "api", HasRepo: true}
	snap := snapClean()
	snap.Worktrees = []gitstatus.Worktree{{Path: "/api-wt", Branch: "feature/x"}}
	m := newTestModel(t, []discovery.Project{proj}, map[string]gitstatus.Snapshot{proj.Path: snap})
	r := row{project: proj, snap: snap}

	if m.repoExpanded(r) {
		t.Error("repoExpanded = true sin busqueda ni expansion (want colapsado)")
	}
	m.expanded = map[string]bool{"/api": true}
	if !m.repoExpanded(r) {
		t.Error("repoExpanded = false con el repo marcado expandido")
	}
	m.expanded = map[string]bool{}
	m.search = "feature"
	if !m.repoExpanded(r) {
		t.Error("repoExpanded = false con una busqueda que matchea un worktree (tiene que inducirlo)")
	}
	// Una busqueda que no matchea nada no despliega.
	m.search = "no-existe-este-worktree"
	if m.repoExpanded(r) {
		t.Error("repoExpanded = true con una busqueda que no matchea ningun worktree")
	}
}

// worktreeBranchLabel dice la rama del worktree, o "(detached)" con el sha
// corto si lo tiene, o "(detached)" a secas si no. El caso sin sha es el que
// estaba sin cubrir: es un worktree recién creado sin commit, y sin el sha la
// etiqueta seria un "(detached) " con un espacio colgando.
func TestWorktreeBranchLabel(t *testing.T) {
	for _, c := range []struct {
		wt   gitstatus.Worktree
		want string
	}{
		{gitstatus.Worktree{Branch: "feat/x"}, "feat/x"},
		{gitstatus.Worktree{Head: "abc1234"}, "(detached) abc1234"},
		{gitstatus.Worktree{}, "(detached)"},
	} {
		if got := worktreeBranchLabel(c.wt); got != c.want {
			t.Errorf("worktreeBranchLabel(%+v) = %q, want %q", c.wt, got, c.want)
		}
	}
}

// Los umbrales EXACTOS de cada bucket. Es lo que mata los mutantes de borde
// (`< 1h` por `< 1h - 1ns`): solo una edad que caiga justo en el umbral cambia
// de respuesta, y con el reloj dentro de relativeTime esa edad no se puede
// construir, porque entre calcularla y mirarla siempre pasa tiempo.
//
// Por eso `relativeAge` recibe la edad ya hecha: aquí se la pasan exacta.
//
// El signo importa en las DOS direcciones, y en el último caso las dos dan lo
// mismo a propósito: a los 30 días exactos, `int(d.Hours()/(24*7))` da 4 y
// `int(d.Hours()/(24*30))` da 1, o sea que "4w" y "1mo" son distinguibles por el
// número. Ese caso es el que mata el mutante de 30*24*time.Hour.
func TestRelativeAgeEnElUmbralExacto(t *testing.T) {
	for _, c := range []struct {
		nombre string
		edad   time.Duration
		want   string
	}{
		{"1ns antes del minuto", time.Minute - time.Nanosecond, "now"},
		{"justo el minuto", time.Minute, "1m"},
		{"1ns antes de la hora", time.Hour - time.Nanosecond, "59m"},
		{"justo la hora", time.Hour, "1h"},
		{"1ns antes del día", 24*time.Hour - time.Nanosecond, "23h"},
		{"justo el día", 24 * time.Hour, "1d"},
		{"1ns antes de la semana", 7*24*time.Hour - time.Nanosecond, "6d"},
		{"justo la semana", 7 * 24 * time.Hour, "1w"},
		{"1ns antes del mes", 30*24*time.Hour - time.Nanosecond, "4w"},
		{"justo el mes", 30 * 24 * time.Hour, "1mo"},
	} {
		if got := relativeAge(c.edad); got != c.want {
			t.Errorf("%s: relativeAge(%s) = %q, want %q", c.nombre, c.edad, got, c.want)
		}
	}
}

// Y el ancla de `relativeTime`: con epoch invalido NO se llega a relativeAge, y
// el corte queda en la misma clave que antes de partir la función.
func TestRelativeTimeAnclaSigueCortandoEpochInvalido(t *testing.T) {
	for _, epoch := range []int64{0, -1, -5} {
		if got := relativeTime(epoch); got != "-" {
			t.Errorf("relativeTime(%d) = %q, want -", epoch, got)
		}
	}
	// Y el camino de la derecha sigue Vivo: un epoch real produce la misma
	// cadena que antes (age >= 0 SIEMPRE, así que relativeAge da el bucket).
	if got := relativeTime(time.Now().Unix()); got == "-" {
		t.Error("relativeTime con epoch real = \"-\", want el bucket de la edad")
	}
}
