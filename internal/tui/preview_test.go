// Tests del panel de preview: la ficha del repo bajo el cursor, pintada entre
// la tabla y los keybinds sin tener que abrir el detalle.
package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// previewModel monta un dashboard con lo que hay que ver en cada caso del
// panel: un repo dirty con ficheros, un repo con worktrees y un grupo con
// miembros en estados distintos.
func previewModel(t *testing.T) Model {
	t.Helper()
	projects := []discovery.Project{
		{Path: "/tmp/api", Name: "api", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/tmp/web", Name: "web", PrimaryGroup: "vsocial", SecondaryGroup: "backend", HasRepo: true},
		{Path: "/tmp/cli", Name: "cli", PrimaryGroup: "vsocial", HasRepo: true},
		proj("loose", "/tmp/loose", true),
	}
	api := snapDirty(2, 1)
	api.Files = []gitstatus.FileEntry{
		{Code: ".M", Path: "main.go"},
		{Code: "??", Path: "notes.md"},
	}
	api.SyncBranch, api.SyncKnown = "main", true
	states := map[string]gitstatus.Snapshot{
		"/tmp/api":   api,
		"/tmp/web":   snapBehind(3),
		"/tmp/cli":   snapClean(),
		"/tmp/loose": snapClean(),
	}
	return newTestModel(t, projects, states)
}

// El panel muestra la ficha del repo bajo el cursor sin abrir el detalle, y
// sigue al cursor al moverse.
func TestPreviewMuestraElRepoDelCursor(t *testing.T) {
	m := cursorOn(t, previewModel(t), "/tmp/api")
	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "api · vsocial/backend")
	for _, want := range []string{"/tmp/api", "branch", "state", "sync"} {
		if !strings.Contains(panel, want) {
			t.Errorf("el panel no dice %q:\n%s", want, panel)
		}
	}
	if !strings.Contains(panel, "main.go") {
		t.Errorf("el panel no lista los ficheros del repo:\n%s", panel)
	}

	// mover el cursor cambia la ficha
	m, _ = press(m, "down")
	moved := stripANSI(m.View().Content)
	if got := sectionContent(t, moved, "web · vsocial/backend"); !strings.Contains(got, "↓3") {
		t.Errorf("el panel no siguió al cursor:\n%s", got)
	}
}

// El título va solo en el borde de la caja, nunca también dentro: pintado en
// los dos sitios el mismo texto aparecía duplicado bajo el borde.
func TestPreviewTituloSoloEnElBorde(t *testing.T) {
	m := cursorOn(t, previewModel(t), "/tmp/api")
	out := stripANSI(m.View().Content)

	if n := strings.Count(out, "api · vsocial/backend"); n != 1 {
		t.Errorf("el título aparece %d veces, want 1 (solo en el borde):\n%s", n, out)
	}
	if panel := sectionContent(t, out, "api · vsocial/backend"); strings.Contains(panel, "api · vsocial/backend") {
		t.Errorf("el título se repite dentro de la caja:\n%s", panel)
	}
}

// En una sub-fila de worktree el panel muestra la ficha mínima (path, rama,
// head) sin inventar estado git derivado.
func TestPreviewEnSubFilaDeWorktree(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-detached", ""))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter") // despliega los worktrees
	m, _ = press(m, "down")  // cursor en la sub-fila
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree {
		t.Fatalf("el cursor no está en la sub-fila: %+v", e)
	}

	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "wt-detached [worktree]")
	if !strings.Contains(panel, "(detached)") {
		t.Errorf("el panel no muestra la rama del worktree:\n%s", panel)
	}
	// La ayuda al pie son las teclas que operan sobre la fila. Ya no hay `enter
	// detail` que ofrecer: la ficha ES la vista.
	if !strings.Contains(panel, "g lazygit · ! cmd") {
		t.Errorf("el panel no ofrece las teclas de la fila:\n%s", panel)
	}
	for _, bad := range []string{"no-up", "clean", "ahead", "behind"} {
		if strings.Contains(panel, bad) {
			t.Errorf("el panel inventa estado %q:\n%s", bad, panel)
		}
	}
}

// Sobre un header de grupo no hay repo que describir: el panel resume el estado
// agregado de sus miembros, y solo de los que lo necesitan (la tabla es quieta).
func TestPreviewResumenDeGrupo(t *testing.T) {
	m := previewModel(t)
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindPrimary {
		t.Fatalf("el cursor no está en un header primario: %+v", e)
	}

	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "vsocial")
	// 3 repos en vsocial: api dirty, web behind, cli clean.
	if !strings.Contains(panel, "repos    3") {
		t.Errorf("el panel no cuenta los repos del grupo:\n%s", panel)
	}
	if !strings.Contains(panel, "dirty    1") || !strings.Contains(panel, "behind   1") {
		t.Errorf("el panel no agrega el estado del grupo:\n%s", panel)
	}
	// El repos limpio no merece una línea a cero.
	if strings.Contains(panel, "ahead") {
		t.Errorf("el panel pinta un estado que no hay:\n%s", panel)
	}

	// El secundario cuenta solo los suyos (api + web), no el cli del primario.
	m, _ = press(m, "down")
	e, ok = m.selectedEntry()
	if !ok || e.kind != kindSecondary {
		t.Fatalf("el cursor no está en un header secundario: %+v", e)
	}
	panel = sectionContent(t, stripANSI(m.View().Content), "vsocial/backend")
	if !strings.Contains(panel, "repos    2") {
		t.Errorf("el secundario no cuenta solo sus repos:\n%s", panel)
	}
	if strings.Contains(panel, "ahead") {
		t.Errorf("el secundario pintó un estado que no tiene:\n%s", panel)
	}
}

// Sin filas (filtro que no casa, o solo-dirty sin nada pendiente) el panel lo
// dice en vez de quedarse en blanco.
func TestPreviewSinFilas(t *testing.T) {
	m := previewModel(t)
	m.search = "nada-casa"
	m.clampCursor()

	out := stripANSI(m.View().Content)
	panel := sectionContent(t, out, "repos")
	if !strings.Contains(panel, "no repositories match") {
		t.Errorf("el panel no explica por qué está vacío:\n%s", panel)
	}
}

// La caja del panel mide lo que dice el layout, no lo larga que sea la ficha:
// se rellena con líneas vacías para que la vista siga midiendo la terminal
// exacta al moverse el cursor.
func TestPreviewRellenaHastaSuAlto(t *testing.T) {
	m := previewModel(t)
	lay := m.layout()
	if lay.previewLines <= 0 {
		t.Fatalf("sin panel a h=%d: %+v", m.height, lay)
	}

	for _, path := range []string{"/tmp/api", "/tmp/loose"} {
		m = cursorOn(t, m, path)
		box := sectionBox(t, m.View().Content, lay)
		if got := strings.Count(box, "\n") + 1; got != lay.previewLines+previewChrome {
			t.Errorf("%s: caja = %d líneas, want %d (2 bordes + %d de contenido)",
				path, got, lay.previewLines+previewChrome, lay.previewLines)
		}
	}
}

// sectionBox devuelve la caja del panel completa (bordes incluidos) que empieza
// justo debajo de la tabla, para poder medirla línea a línea.
func sectionBox(t *testing.T, view string, lay layout) string {
	t.Helper()
	lines := strings.Split(view, "\n")
	// la tabla acaba justo antes del panel: previewChrome líneas antes del final
	// del keybinds (o del final de la vista si no hay keybinds).
	end := len(lines)
	if lay.showKeybinds {
		end -= lay.hintLines + keybindsChrome
	}
	start := end - lay.previewLines - previewChrome
	if start < 0 || start >= len(lines) {
		t.Fatalf("no encuentro la caja del panel en la vista:\n%s", view)
	}
	return strings.Join(lines[start:end], "\n")
}

// Una lista que no cabe se anuncia ("… N más") en vez de cortarse a media sin
// decir cuántas filas faltaban: el presupuesto sale del alto del panel, no del
// de la terminal.
func TestPreviewAnunciaLasListasQueNoCaben(t *testing.T) {
	m := cursorOn(t, previewModel(t), "/tmp/api")
	m.height = 44 // panel holgado: la lista de ficheros cabe entera
	lay := m.layout()
	if lay.previewLines <= detailHeadLines+2 {
		t.Fatalf("precondición: el panel debería dar para una lista, es de %d", lay.previewLines)
	}

	panel := sectionContent(t, stripANSI(m.View().Content), "api · vsocial/backend")
	if !strings.Contains(panel, "files (2)") || !strings.Contains(panel, "main.go") {
		t.Errorf("la lista de ficheros no se ve con hueco:\n%s", panel)
	}
	if strings.Contains(panel, "más") {
		t.Errorf("una lista que cabe entera no lleva aviso de truncado:\n%s", panel)
	}

	// Con muchos ficheros, los que no caben se anuncian en vez de desaparecer.
	s := m.states["/tmp/api"]
	for i := 0; i < 20; i++ {
		s.Files = append(s.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/file%02d.go", i)})
	}
	m.states["/tmp/api"] = s
	panel = sectionContent(t, stripANSI(m.View().Content), "api · vsocial/backend")
	if !strings.Contains(panel, "files (22)") {
		t.Errorf("la cabecera no cuenta los ficheros:\n%s", panel)
	}
	if !strings.Contains(panel, "más") {
		t.Errorf("una lista truncada no se anuncia:\n%s", panel)
	}
}

// Con muchos repos, la tabla conserva el scroll y el panel no empuja la fila
// del cursor fuera de la caja.
func TestPreviewNoRompelaTabla(t *testing.T) {
	var projects []discovery.Project
	states := map[string]gitstatus.Snapshot{}
	for i := 0; i < 40; i++ {
		p := proj(fmt.Sprintf("repo%02d", i), fmt.Sprintf("/tmp/repo%02d", i), true)
		projects = append(projects, p)
		states[p.Path] = snapClean()
	}
	m := newTestModel(t, projects, states)
	m.cursor = len(m.entries()) - 1

	out := stripANSI(m.renderDashboard())
	if lines := strings.Split(out, "\n"); len(lines) != m.height {
		t.Errorf("líneas = %d, want %d", len(lines), m.height)
	}
	for i, l := range strings.Split(m.renderDashboard(), "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d ancho = %d, want %d", i, w, m.width)
		}
	}
	if !strings.Contains(out, "repo39") {
		t.Errorf("la fila del cursor no se ve:\n%s", out)
	}
	if !strings.Contains(out, "╭ keybinds ") {
		t.Errorf("los keybinds quedaron fuera:\n%s", out)
	}
}
