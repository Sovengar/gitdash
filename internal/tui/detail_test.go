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
