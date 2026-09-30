// Tests de la ficha del repo (detail.go): el presupuesto de las listas, los
// recortes por ancho y los avisos de truncado.
package tui

import (
	"fmt"
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// listBudget reparte las líneas de una lista. Lo que se mira aquí son los bordes
// Justos, porque un "> " mal puesto cambia la respuesta justo cuando la lista
// cabe exacta (y ahí no debe quedar un aviso de "N más" con N=0).
func TestListBudget(t *testing.T) {
	casos := []struct {
		nombre       string
		avail, items int
		wantShown    int
		wantRest     bool
	}{
		{"sin hueco para la lista", minListBlockLines - 1, 5, 0, false},
		{"sin hueco y sin elementos", 0, 0, 0, false},
		{"hueco mínimo, sin elementos", minListBlockLines, 0, 0, false},
		{"hueco mínimo, un elemento (cabe el hueco)", minListBlockLines, 1, 0, false},
		{"hueco mínimo, tres elementos", minListBlockLines, 3, 0, false},
		{"una línea de elemento, cabe exacta", minListBlockLines + 1, 1, 1, false},
		{"una línea de elemento, no cabe", minListBlockLines + 1, 2, 0, true},
		{"cabe exacta al límite", minListBlockLines + 3, 3, 3, false},
		{"uno de más", minListBlockLines + 3, 4, 2, true},
		{"holgada", 20, 2, 2, false},
		{"muy larga", 10, 100, 7, true},
	}
	for _, c := range casos {
		shown, rest := listBudget(c.avail, c.items)
		if shown != c.wantShown || rest != c.wantRest {
			t.Errorf("%s: listBudget(%d, %d) = (%d, %v), want (%d, %v)",
				c.nombre, c.avail, c.items, shown, rest, c.wantShown, c.wantRest)
		}
	}
}

// El aviso de "N más" cuenta lo que de verdad queda fuera, no lo que se pintó.
func TestListBudgetElAvisoCuentaLoQueFalta(t *testing.T) {
	for _, items := range []int{4, 10, 37} {
		avail := minListBlockLines + 3
		shown, rest := listBudget(avail, items)
		if !rest {
			t.Fatalf("items=%d: se esperaba aviso con avail=%d", items, avail)
		}
		if faltan := items - shown; shown+faltan != items || faltan <= 0 {
			t.Errorf("items=%d: shown=%d → faltan %d, want %d (> 0 y shown+rest=items)",
				items, shown, items-shown, items-shown)
		}
	}
}

// actionTail se queda con las ÚLTIMAS n líneas: la cola de un comando es donde
// está el resultado, y los newlines finales no son una línea más.
func TestActionTail(t *testing.T) {
	casos := []struct {
		nombre, out string
		n           int
		want        string
	}{
		{"vacío", "", 3, ""},
		{"solo newlines", "\n\n", 3, ""},
		{"una línea", "hola", 3, "hola"},
		{"cabe entero", "a\nb", 3, "a\nb"},
		{"cabe exacto", "a\nb\nc", 3, "a\nb\nc"},
		{"recorta por detrás", "a\nb\nc\nd", 2, "c\nd"},
		{"más líneas de las que caben", "a\nb\nc", 1, "c"},
		{"n=0", "a\nb", 0, ""},
		{"n negativo", "a\nb", -1, ""},
		{"ignora el newline final", "a\nb\n", 5, "a\nb"},
	}
	for _, c := range casos {
		if got := actionTail(c.out, c.n); got != c.want {
			t.Errorf("%s: actionTail(%q, %d) = %q, want %q", c.nombre, c.out, c.n, got, c.want)
		}
	}
}

func TestAsOrDash(t *testing.T) {
	if got := asOrDash(""); got != "-" {
		t.Errorf("asOrDash(\"\") = %q, want -", got)
	}
	if got := asOrDash("main"); got != "main" {
		t.Errorf("asOrDash(main) = %q, want main", got)
	}
}

// fitLines/clipTo son el contrato de alto de la caja: fitLines RELLENA (la caja
// mide lo que dice el layout) y clipTo NO (lo que viene detrás tiene que verse
// siempre). Los dos bordes (n == líneas y n == 0) son donde un recorte mal
// puesto se nota.
func TestFitLinesRellenaYClipToNo(t *testing.T) {
	casos := []struct {
		nombre, content string
		n               int
		wantFit         []string
		wantClip        []string
	}{
		{"cabe exacto", "a\nb", 2, []string{"a", "b"}, []string{"a", "b"}},
		{"sobra", "a\nb\nc", 2, []string{"a", "b"}, []string{"a", "b"}},
		{"falta", "a", 3, []string{"a", "", ""}, []string{"a"}},
		{"vacío", "", 2, []string{"", ""}, []string{""}},
		{"n=0 con contenido", "a\nb", 0, []string{""}, []string{""}},
		{"n=0 vacío", "", 0, []string{""}, []string{""}},
		{"n negativo", "a", -1, []string{""}, []string{""}},
	}
	for _, c := range casos {
		if got := strings.Split(fitLines(c.content, c.n), "\n"); !equalStrings(got, c.wantFit) {
			t.Errorf("%s: fitLines(%q, %d) = %q, want %q", c.nombre, c.content, c.n, got, c.wantFit)
		}
		if got := strings.Split(clipTo(c.content, c.n), "\n"); !equalStrings(got, c.wantClip) {
			t.Errorf("%s: clipTo(%q, %d) = %q, want %q", c.nombre, c.content, c.n, got, c.wantClip)
		}
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

// detailRowWith monta un repo con todo lo que la ficha sabe pintar, para poder
// mirarla campo a campo sin pelearse con el box del panel.
func detailRowWith(t *testing.T, path string, snap gitstatus.Snapshot) (Model, row) {
	t.Helper()
	p := discovery.Project{Path: path, Name: "api", PrimaryGroup: "vsocial", HasRepo: true}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{path: snap})
	return m, row{project: p, snap: snap, state: snap.State(true)}
}

// Cada campo truncado usa el ancho que le deja su prefijo, no el ancho entero:
// si el recorte se calculara al revés, la línea se saldría de la caja y el
// usuario vería un path cortado donde cabía entero (o al revés).
func TestFichaRecortaCadaCampoASuAncho(t *testing.T) {
	const width = 60
	largo := strings.Repeat("largo", 30) // 150 chars, de sobra para cortar

	// El path del repo, con el resto de campos en corto: es el que lleva el
	// prefijo más ancho ("path    ").
	pathLargo := "/tmp/api/" + largo
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: largo + ".go"}}
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: largo + " commit"}}
	m, r := detailRowWith(t, pathLargo, snap)
	m.width = width
	m.lastAction[pathLargo] = actionResult{kind: "pull_rebase", cmd: "git pull --rebase " + largo, output: ""}

	out := stripANSI(m.renderDetail(r, 40))
	for _, c := range []struct {
		que  string
		want string
	}{
		{"path del repo", truncate(pathLargo, max(20, width-13))},
		{"fichero", truncate(largo+".go", max(20, width-8))},
		{"commit", truncate(largo+" commit", max(20, width-24))},
		{"argv", truncate("git pull --rebase "+largo, max(20, width-30))},
	} {
		if !strings.Contains(out, c.want) {
			t.Errorf("el %s no está recortado a su ancho (want %d chars: %q…):\n%s", c.que, len(c.want), c.want[:20], out)
		}
	}

	// La ruta del worktree es relativa al repo y va a su propio ancho: la
	// sangría y las dos columnas fijas se llevan 38 de las 60 líneas.
	m2, r2 := detailRowWith(t, "/tmp/api", func() gitstatus.Snapshot {
		s := snapClean()
		s.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt/" + largo, Branch: "feat", Head: "abc1234"}}
		return s
	}())
	m2.width = width
	outWT := stripANSI(m2.renderDetail(r2, 40))
	if want := truncate("wt/"+largo, max(20, width-16)); !strings.Contains(outWT, want) {
		t.Errorf("la ruta del worktree no está recortada a su ancho (want %d chars: %q…):\n%s", len(want), want[:20], outWT)
	}

	// El suelo de 20 caracteres también es un contrato: con una terminal
	// estrecha un path se recorta a 20 como mínimo, en vez de desaparecer o de
	// comerse la caja.
	m.width = 30
	if want := truncate(pathLargo, 20); !strings.Contains(stripANSI(m.renderDetail(r, 40)), want) {
		t.Errorf("con width=30 el path debería recortarse al suelo de 20 chars (%q…)", want[:20])
	}
}

// El argv de la última acción es lo único que dice qué política aplicó el
// gitconfig: si no se pinta, el detalle miente sobre lo que se ejecutó.
func TestFichaMuestraElArgvDeLaUltimaAccion(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	m, r := detailRowWith(t, path, snap)

	// Sin argv (handoff) no hay línea extra, pero sí la variante y el veredicto.
	m.lastAction[path] = actionResult{kind: "pull_ai"}
	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "last pull (AI)") && !strings.Contains(out, "AI") {
		t.Errorf("falta la variante de la acción sin argv:\n%s", out)
	}
	if strings.Contains(out, "git pull") {
		t.Errorf("se inventó un argv donde no lo hay:\n%s", out)
	}

	// Con argv, la línea aparece con el comando resuelto tal cual.
	m.lastAction[path] = actionResult{kind: "pull_rebase", cmd: "git pull --rebase --autostash", output: ""}
	out = stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "git pull --rebase --autostash") {
		t.Errorf("el argv resuelto no aparece en la ficha:\n%s", out)
	}
}

// Una lista que no cabe se anuncia con lo que REALMENTE falta. El número del
// aviso es la diferencia entre "te faltan 3" y una cuenta que no cuadra con la
// cabecera.
func TestFichaElAvisoCuentaLosFicherosQueFaltan(t *testing.T) {
	path := "/tmp/api"
	snap := snapDirty(0, 0)
	const total = 20
	for i := 0; i < total; i++ {
		snap.Files = append(snap.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/file%02d.go", i)})
	}
	m, r := detailRowWith(t, path, snap)

	// rows justo para la cabecera + cabecera de lista + 4 elementos: entran 4 y
	// el aviso tiene que decir los 16 que quedan.
	rows := detailHeadLines + minListBlockLines + 5 // hueco + cabecera + 4
	out := stripANSI(m.renderDetail(r, rows))
	if !strings.Contains(out, fmt.Sprintf("files (%d)", total)) {
		t.Errorf("la cabecera no cuenta los %d ficheros:\n%s", total, out)
	}
	wantAviso := fmt.Sprintf("… %d más", total-4)
	if !strings.Contains(out, wantAviso) {
		t.Errorf("el aviso no dice %q:\n%s", wantAviso, out)
	}
	// Los 4 pintados son los primeros, no unos cualesquiera.
	if !strings.Contains(out, "file00.go") {
		t.Errorf("no se pintó el primero de la lista:\n%s", out)
	}
	if strings.Contains(out, "file04.go") {
		t.Errorf("se pintó un fichero que no cabía:\n%s", out)
	}
}

// El presupuesto de las listas es COMPARTIDO, así que el hueco mínimo solo
// financia la primera lista que aparece (la de worktrees): con hueco para una,
// las siguientes no tienen ni su separación. Y con una línea menos que el
// mínimo, no cabe ninguna: sin separación ni cabecera.
func TestFichaConElHuecoMinimoDeLaLista(t *testing.T) {
	path := "/tmp/api"
	snap := snapDirty(1, 1)
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: "fix"}}
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt", Branch: "feat", Head: "abc1234"}}
	m, r := detailRowWith(t, path, snap)

	// avail == el mínimo exacto: solo worktrees se lleva el hueco.
	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines))
	if !strings.Contains(out, "worktrees (1)") {
		t.Errorf("con avail=%d la primera lista debe pintar su cabecera:\n%s", minListBlockLines, out)
	}
	for _, sinHueco := range []string{"files (", "commits"} {
		if strings.Contains(out, sinHueco) {
			t.Errorf("con avail=%d la lista %q no tenía hueco y aun así se pintó:\n%s", minListBlockLines, sinHueco, out)
		}
	}

	// avail == el mínimo menos uno: no cabe ninguna lista, ni cabecera.
	out = stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines-1))
	for _, cabecera := range []string{"worktrees (", "files (", "commits"} {
		if strings.Contains(out, cabecera) {
			t.Errorf("con avail=%d se pintó la lista %q sin sitio:\n%s", minListBlockLines-1, cabecera, out)
		}
	}
}

// Sin rama (un snapshot que no trae branch.head) la ficha pone "-": es un dato
// ausente, no una cadena vacía pegada al borde.
func TestFichaRamaVaciaSeMuestraComoGuion(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Status.Branch = ""
	m, r := detailRowWith(t, path, snap)
	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "branch  -") {
		t.Errorf("sin rama, la ficha no muestra el guion:\n%s", out)
	}
}

// Un repo limpio no lista bloques vacíos: ni "files (0)", ni "worktrees (0)",
// ni una cabecera "commits" sin commits detrás. Las listas salen solo si hay algo
// que enseñar, que es justo lo que distingue una ficha de un formulario.
func TestFichaNoPintaListasVacias(t *testing.T) {
	path := "/tmp/api"
	m, r := detailRowWith(t, path, snapClean())
	out := stripANSI(m.renderDetail(r, 40))
	for _, cabecera := range []string{"files (", "worktrees (", "commits"} {
		if strings.Contains(out, cabecera) {
			t.Errorf("un repo sin datos pintó la lista %q:\n%s", cabecera, out)
		}
	}
	// La cabecera de estado sigue entera: los 5 campos fijos no dependen de las
	// listas.
	for _, campo := range []string{"path", "branch", "upstream", "state", "sync"} {
		if !strings.Contains(out, campo) {
			t.Errorf("falta el campo %q de la cabecera:\n%s", campo, out)
		}
	}
}

// La cola de la última acción se recorta al hueco que queda en la ficha: con
// una ficha corta se ven las últimas líneas (donde está el resultado), no el
// principio. El suelo de 3 líneas también es contrato: con la ficha mínima
// siguen viéndose 3, no 0.
func TestFichaRecortaLaColaDeLaUltimaAccion(t *testing.T) {
	path := "/tmp/api"
	prologo := strings.Repeat("linea\n", 30)
	finales := "resultado1\nresultado2\nresultado3\nresultado4\nresultado5"

	t.Run("con hueco", func(t *testing.T) {
		m, r := detailRowWith(t, path, snapClean())
		m.lastAction[path] = actionResult{kind: "pull_rebase", output: prologo + finales + "\n"}
		out := stripANSI(m.renderDetail(r, detailHeadLines+10))
		// avail para la cola = max(3, rows-20) = 3: con una ficha de 15 líneas el
		// hueco es pequeño y solo caben 3.
		if !strings.Contains(out, "resultado5") || !strings.Contains(out, "resultado3") {
			t.Errorf("el cierre de la salida no se ve:\n%s", out)
		}
		if strings.Contains(out, "resultado1") {
			t.Errorf("la cola se metió el principio de la salida:\n%s", out)
		}
	})

	t.Run("con hueco amplio", func(t *testing.T) {
		m, r := detailRowWith(t, path, snapClean())
		m.lastAction[path] = actionResult{kind: "pull_rebase", output: prologo + finales + "\n"}
		out := stripANSI(m.renderDetail(r, 44))
		// rows-20 = 24 líneas de cola: caben las 30 de la salida, así que se ve
		// el principio y el cierre.
		if !strings.Contains(out, "linea") || !strings.Contains(out, "resultado5") {
			t.Errorf("con hueco amplio no se ve la salida completa:\n%s", out)
		}
	})
}

// La ficha mínima de un worktree sin snapshot propio enseña path, rama y head,
// con la ruta recortada al ancho que le deja su prefijo (como cualquier otro
// campo) y sin inventar estado git.
func TestFichaMinimaWorktreeRecortaElPathYRespetaLaRama(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/multi/wt/"+strings.Repeat("largo", 30), "feat/x"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.width = 60
	e := tableEntry{kind: kindWorktree, wt: gitstatus.Worktree{
		Path:   "/tmp/multi/wt/" + strings.Repeat("largo", 30),
		Branch: "feat/x",
		Head:   "abc1234",
	}, parent: "/tmp/multi"}

	out := stripANSI(m.renderWorktreeDetail(e, 20))
	// La rama se respeta: no es un worktree detached.
	if !strings.Contains(out, "feat/x") {
		t.Errorf("la rama del worktree no se ve:\n%s", out)
	}
	if strings.Contains(out, "(detached)") {
		t.Errorf("un worktree con rama se pintó como detached:\n%s", out)
	}
	// El path va recortado a su ancho, no entero (ni con un signo cambiado).
	want := truncate("/tmp/multi/wt/"+strings.Repeat("largo", 30), max(20, 60-13))
	if !strings.Contains(out, want) {
		t.Errorf("el path del worktree no está recortado a su ancho (want %d chars):\n%s", len(want), out)
	}
	// Y sigue sin inventar estado git.
	for _, falso := range []string{"no-up", "clean", "state"} {
		if strings.Contains(out, falso) {
			t.Errorf("la ficha mínima inventó %q:\n%s", falso, out)
		}
	}
}

// La ruta de un worktree se pinta RELATIVA al repo cuando puede: un worktree
// cuelga siempre del repo, y repetir el prefijo entero en cada línea es ruido
// que se come el ancho de la lista.
func TestFichaWorktreeConRutaRelativa(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/api/wt-feat", Branch: "feat", Head: "abc1234"}}
	m, r := detailRowWith(t, path, snap)
	out := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(out, "wt-feat") {
		t.Errorf("la ruta del worktree no se ve:\n%s", out)
	}
	if strings.Contains(out, "/tmp/api/wt-feat") {
		t.Errorf("la ruta del worktree no se relativizó:\n%s", out)
	}
}

// Las tres listas COMPITEN por el mismo hueco de la ficha. Si cada una se creyera
// con el presupuesto entero, la ficha se saldría de la caja y fitLines la
// recortaría por arriba sin avisar: el usuario vería el final de los commits sin
// el aviso de "N más" de los ficheros, que es justo lo que la lista promete.
func TestFichaLasListasCompartenElPresupuesto(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	for i := 0; i < 3; i++ {
		snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
			Path: path + "/wt", Branch: "feat", Head: "abc1234",
		})
	}
	for i := 0; i < 6; i++ {
		snap.Files = append(snap.Files, gitstatus.FileEntry{
			Code: ".M", Path: fmt.Sprintf("pkg/f%d.go", i),
		})
	}
	for i := 0; i < 5; i++ {
		snap.Commits = append(snap.Commits, gitstatus.Commit{
			Sha: "abc1234", When: 1700000000, Subject: fmt.Sprintf("c%d", i),
		})
	}
	m, r := detailRowWith(t, path, snap)

	// La ficha nunca puede pintar más líneas de las que el layout le da.
	for rows := detailHeadLines + 2; rows <= 40; rows++ {
		out := stripANSI(m.renderDetail(r, rows))
		if got := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); got > rows {
			t.Errorf("rows=%d: la ficha pinta %d líneas, se sale de la caja", rows, got)
		}
	}

	// Con hueco para una sola lista, la primera se lleva el presupuesto y las
	// otras se quedan sin él (con su aviso de "N más", que es el que avisa de
	// que ahí no cabía nada más).
	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines+3))
	if !strings.Contains(out, "worktrees (3)") {
		t.Errorf("con hueco mínimo no debería verse ninguna lista de elementos:\n%s", out)
	}
	if strings.Contains(out, "files (") || strings.Contains(out, "commits") {
		t.Errorf("las listas siguientes pintaron cabecera sin hueco:\n%s", out)
	}
}

// El aviso "N más" cuenta lo que falta en CADA lista, no solo en la de
// ficheros: los worktrees compiten por el mismo hueco y su cuenta también
// tiene que cuadrar (3 pintados de 8 → "5 más", no "11 más").
func TestFichaElAvisoCuentaLosWorktreesQueFaltan(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	const total = 8
	for i := 0; i < total; i++ {
		snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
			Path:   path + "/wt",
			Branch: fmt.Sprintf("feat-%d", i),
			Head:   "abc1234",
		})
	}
	m, r := detailRowWith(t, path, snap)

	// Con hueco para la cabecera + 3 elementos, se pintan 3 y avisan 5.
	rows := detailHeadLines + minListBlockLines + 4
	out := stripANSI(m.renderDetail(r, rows))
	if !strings.Contains(out, fmt.Sprintf("worktrees (%d)", total)) {
		t.Errorf("la cabecera no cuenta los worktrees:\n%s", out)
	}
	if want := fmt.Sprintf("… %d más", total-3); !strings.Contains(out, want) {
		t.Errorf("el aviso de worktrees no dice %q:\n%s", want, out)
	}
}

// La lista de ficheros entra sola con el hueco mínimo: si no hay worktrees que
// se lo queden antes, son ellos los que la usan.
func TestFichaElHuecoMinimoLoGastaLaPrimeraLista(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Files = []gitstatus.FileEntry{{Code: ".M", Path: "main.go"}}
	m, r := detailRowWith(t, path, snap) // sin worktrees ni commits

	// avail == 2 justo: la cabecera de la lista entra (y no hay dónde pintar el
	// elemento). Con la guarda "> " en vez de ">=" la lista desaparecería entera.
	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines))
	if !strings.Contains(out, "files (1)") {
		t.Errorf("con el hueco mínimo no salió la cabecera de ficheros:\n%s", out)
	}
}

// La ficha mínima de un worktree nombra su repo padre cuando lo tiene: sin esa
// línea, un worktree suelto no dice de qué repo es.
func TestFichaMinimaWorktreeNombraSuRepo(t *testing.T) {
	wt := gitstatus.Worktree{Path: "/tmp/multi/wt-feat", Branch: "feat", Head: "abc1234"}
	p := discovery.Project{Path: "/tmp/multi", Name: "multi", HasRepo: true}
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	e := tableEntry{kind: kindWorktree, wt: wt, parent: "/tmp/multi"}

	if out := stripANSI(m.renderWorktreeDetail(e, 20)); !strings.Contains(out, "multi") {
		t.Errorf("la ficha no nombró el repo padre:\n%s", out)
	}
	// Sin padre conocido, esa línea no se inventa.
	e.parent = ""
	if out := stripANSI(m.renderWorktreeDetail(e, 20)); strings.Contains(out, "repo ") {
		t.Errorf("sin repo padre se pintó la línea repo:\n%s", out)
	}
}

// orDash es el guion de los datos ausentes (head de un worktree, upstream…):
// vacío es "-", cualquier valor es el valor.
func TestOrDash(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want -", got)
	}
	if got := orDash("abc1234"); got != "abc1234" {
		t.Errorf("orDash = %q, want el valor", got)
	}
}

// Igual que el hueco mínimo de ficheros, el de commits: si no hay listas antes
// que se lo queden, son los commits los que pueden usar el hueco mínimo entero.
func TestFichaElHuecoMinimoParaLosCommits(t *testing.T) {
	path := "/tmp/api"
	snap := snapClean()
	snap.Commits = []gitstatus.Commit{{Sha: "abc1234", When: 1700000000, Subject: "fix"}}
	m, r := detailRowWith(t, path, snap) // sin worktrees ni ficheros

	out := stripANSI(m.renderDetail(r, detailHeadLines+minListBlockLines))
	if !strings.Contains(out, "commits") {
		t.Errorf("con el hueco mínimo no salió la cabecera de commits:\n%s", out)
	}
}

// --- la línea de sync: cuatro formas, y solo se pintaba una ---

// La ficha siempre enseña la sync branch resuelta, con su desviación o el motivo
// de la falta. Las cuatro formas son distintas para el usuario: sin sync branch
// no hay contra qué comparar, ref missing es un config roto, ↓N es trabajo
// pendiente y ok es que está al día. Antes solo la cuarta tenía camino.
func TestFichaLaLineaDeSyncTieneCuatroFormas(t *testing.T) {
	for _, c := range []struct {
		nombre string
		snap   gitstatus.Snapshot
		want   string
		noWant string
	}{
		{
			"sin sync branch: no hay contra qué comparar",
			gitstatus.Snapshot{SyncBranch: "", SyncKnown: false},
			"— (sin sync branch)", "ref missing",
		},
		{
			"la rama existe pero el ref no: el config está roto",
			gitstatus.Snapshot{SyncBranch: "main", SyncKnown: false},
			"main (ref missing)", "(ok)",
		},
		{
			"con retraso: hay N commits que bajar",
			gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true, SyncBehind: 3},
			"main (↓3)", "(ok)",
		},
		{
			"al día",
			gitstatus.Snapshot{SyncBranch: "main", SyncKnown: true, SyncBehind: 0},
			"main (ok)", "↓0",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			path := "/tmp/api"
			snap := snapClean()
			snap.SyncBranch = c.snap.SyncBranch
			snap.SyncKnown = c.snap.SyncKnown
			snap.SyncBehind = c.snap.SyncBehind
			m, r := detailRowWith(t, path, snap)

			out := stripANSI(m.renderDetail(r, 40))
			if !strings.Contains(out, c.want) {
				t.Errorf("la línea de sync no dice %q:\n%s", c.want, out)
			}
			if c.noWant != "" && strings.Contains(out, c.noWant) {
				t.Errorf("la línea de sync dice %q, que es de otra forma:\n%s", c.noWant, out)
			}
		})
	}
}

// Y el borde: SyncBehind == 0 con la ref conocida es "ok", no "↓0". Un "↓0" es
// ruido que hace creer que hay algo que bajar.
func TestFichaSyncBehindCeroNoSePintaComoRetraso(t *testing.T) {
	snap := snapClean()
	snap.SyncBranch = "main"
	snap.SyncKnown = true
	snap.SyncBehind = 0
	m, r := detailRowWith(t, "/tmp/api", snap)

	out := stripANSI(m.renderDetail(r, 40))
	if strings.Contains(out, "↓0") {
		t.Errorf("un retraso de 0 se pintó como retraso:\n%s", out)
	}
	if !strings.Contains(out, "main (ok)") {
		t.Errorf("sin retraso, la línea de sync no dice (ok):\n%s", out)
	}
}

// --- el veredicto del último comando `!` ---

// La ficha guarda el último `!` con su código de salida, y ese código es el
// veredicto: sin él, un comando que salió mal se lee como que fue bien. Además el
// comando se recorta a un ancho mínimo (un argv de 20 caracteres mínimo) para que
// no se coma media ficha.
func TestFichaElVeredictoDelUltimoComando(t *testing.T) {
	const path = "/tmp/api"

	for _, c := range []struct {
		nombre string
		exit   string
		want   string
	}{
		{"exit 0 es el caso limpio", "0", "exit 0"},
		{"un código de salida es un fallo, y se nombra", "3", "exit 3"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m, r := detailRowWith(t, path, snapClean())
			m.lastCmd[path] = cmdResult{command: "go test ./...", output: "ok\n", exit: c.exit}

			out := stripANSI(m.renderDetail(r, 40))
			if !strings.Contains(out, "go test ./...") {
				t.Errorf("el comando no aparece en la ficha:\n%s", out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("el veredicto no dice %q:\n%s", c.want, out)
			}
			// Con exit 0 no puede aparecer un código de salida, y con exit 3 no
			// puede aparecer el "exit 0" del caso limpio.
			otro := "exit 0"
			if c.want == "exit 0" {
				otro = "exit 3"
			}
			if strings.Contains(out, otro) {
				t.Errorf("apareció %q, el veredicto del otro caso:\n%s", otro, out)
			}
		})
	}
}

// La salida del comando se enseña al final, y solo si hay alto para ella: con el
// presupuesto justo no cabe y no se pinta una línea a medias.
func TestFichaLaSalidaDelComandoRespetaElPresupuesto(t *testing.T) {
	const path = "/tmp/api"
	m, r := detailRowWith(t, path, snapClean())
	m.lastCmd[path] = cmdResult{
		command: "ls",
		output:  "uno\ndos\ntres\ncuatro\ncinco\n",
		exit:    "0",
	}

	conAlto := stripANSI(m.renderDetail(r, 40))
	if !strings.Contains(conAlto, "cinco") {
		t.Errorf("con presupuesto la salida del comando no aparece:\n%s", conAlto)
	}

	// Un comando largo se recorta, no se sale de la caja.
	m.width = 60
	corto := stripANSI(m.renderDetail(r, 40))
	for _, linea := range strings.Split(corto, "\n") {
		if w := len([]rune(linea)); w > m.width {
			t.Errorf("la ficha mide %d con un terminal de %d: %q", w, m.width, linea)
		}
	}
}

// Cuántas líneas de la salida del `!` se ven depende del presupuesto que se le
// da a la cola, así que el barrido afirma las dos cosas que tienen que ser
// ciertas: la cola NUNCA excede su presupuesto, y crece con él. Un presupuesto
// mayor no puede pintar más de lo que la caja da.
func TestFichaLaColaDelComandoRespetaSuPresupuesto(t *testing.T) {
	const path = "/tmp/api"

	lineasVisibles := func(rows, lineas int) int {
		m, r := detailRowWith(t, path, snapClean())
		m.lastCmd[path] = cmdResult{
			command: "ls",
			output:  strings.TrimSuffix(strings.Repeat("salida\n", lineas), "\n"),
			exit:    "0",
		}
		out := stripANSI(m.renderDetail(r, rows))
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "salida") {
				n++
			}
		}
		return n
	}

	for _, rows := range []int{20, 30, 40, 60} {
		prev := -1
		for _, lineas := range []int{1, 3, 8, 20, 40, 80} {
			n := lineasVisibles(rows, lineas)
			if n < prev {
				t.Errorf("rows=%d: con %d líneas se ven %d y con menos se veían %d: "+
					"más salida no puede tapar más salida", rows, lineas, n, prev)
			}
			prev = n
			// El presupuesto de la cola son 12 líneas menos las del resto de la
			// ficha, con un suelo de 3: ni una línea más.
			if presupuesto := max(3, rows-12); n > presupuesto {
				t.Errorf("rows=%d: la cola enseña %d líneas y su presupuesto es %d",
					rows, n, presupuesto)
			}
			// Y si la salida NO cabe en el presupuesto, no se pinta entera: la
			// cola se recorta, que es justo lo que evita que el comando se coma
			// media ficha.
			if presupuesto := max(3, rows-12); lineas > presupuesto && n >= lineas {
				t.Errorf("rows=%d: la cola enseñó las %d líneas enteras, "+
					"cuando su presupuesto es %d", rows, lineas, presupuesto)
			}
		}
	}
}

// El comando del `!` se recorta al ancho que le queda, y ese ancho es el del
// terminal MENOS el de lo que lo acompaña ("$ ", el veredicto y el borde). Con
// un comando largo en una terminal estrecha, la ficha tiene que recortarlo: si no,
// la línea se sale de la caja y el valor se corta contra el borde, que es
// justo lo que el recorte del resto de campos evita.
func TestFichaRecortaElComandoAlAnchoQueLeQueda(t *testing.T) {
	const path = "/tmp/api"
	largo := strings.Repeat("comando", 30) // 210 caracteres, de sobra para cortar

	for _, width := range []int{40, 60, 90, 200, 260} {
		m, r := detailRowWith(t, path, snapClean())
		m.width = width
		m.lastCmd[path] = cmdResult{command: largo, output: "", exit: "0"}

		out := stripANSI(m.renderDetail(r, 40))
		var linea string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "comando") {
				linea = l
				break
			}
		}
		if linea == "" {
			t.Fatalf("width=%d: el comando no aparece en la ficha:\n%s", width, out)
		}
		if w := len([]rune(linea)); w > width {
			t.Errorf("width=%d: la línea del comando mide %d y se sale de la caja: %q",
				width, w, linea)
		}
		// En una terminal lo bastante ancha el comando entero cabe: recortarlo
		// siempre escondería información que sí se puede leer. El presupuesto es
		// el ancho menos 30, así que 210 de comando necesitan 240 de terminal.
		if width >= 240 {
			if !strings.Contains(linea, largo) {
				t.Errorf("width=%d: el comando se recortó sin necesidad: %q", width, linea)
			}
		}
	}
}
