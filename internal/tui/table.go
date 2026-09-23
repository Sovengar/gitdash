// Filas de la tabla: filtrado, orden, agrupación y render de celdas
// (spec 0001 R6-R7; 0002 R15-R16).
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/group"
)

// row agrupa un proyecto con su snapshot vivo para render.
type row struct {
	project discovery.Project
	snap    gitstatus.Snapshot
	state   gitstatus.State
}

// entryKind distingue fila de grupo (header plegable) y fila de repo.
type entryKind int

const (
	kindRepo entryKind = iota
	kindPrimary
	kindSecondary
	kindWorktree // 0006 R30: sub-fila de worktree bajo su repo principal
)

// tableEntry es una fila navegable de la tabla: header primario, header
// secundario, repo o sub-fila de worktree (patrón buildTree de vroom R24,
// extendido 0003 R20 y 0006 R30). group lleva la clave de plegado: nombre
// del primario o `primario/secundario` para los headers de nivel 2.
type tableEntry struct {
	kind  entryKind
	group string // válido en kindPrimary/kindSecondary
	r     row    // válido en kindRepo

	// 0006 R31/R33: datos de la sub-fila de worktree (path/rama/head) y el
	// path del repo principal para el detalle. No reutiliza `r` porque un
	// worktree no tiene Snapshot/State propios.
	wt     gitstatus.Worktree
	parent string
}

// groupKey compone la clave de plegado de un secundario (S20.5: evita
// colisión de nombres entre primarios distintos).
func groupKey(primary, secondary string) string {
	return primary + "/" + secondary
}

// groupLabel compone el texto compuesto `primary/secondary` de un proyecto
// (R21): solo primario si no hay secundario, "" si ninguno.
func groupLabel(p discovery.Project) string {
	switch {
	case p.PrimaryGroup != "" && p.SecondaryGroup != "":
		return groupKey(p.PrimaryGroup, p.SecondaryGroup)
	case p.PrimaryGroup != "":
		return p.PrimaryGroup
	default:
		return ""
	}
}

// pendingStates son los estados que el filtro `n` considera no-limpios:
// cambios pendientes de subir/bajar o en el working tree. Los errores y los
// proyectos sin repo se excluyen (R7, interpretación documentada en spec).
func pendingStates(st gitstatus.State) bool {
	switch st {
	case gitstatus.StateDirty, gitstatus.StateAhead, gitstatus.StateBehind, gitstatus.StateDiverged:
		return true
	}
	return false
}

// rows construye la lista de filas visibles: filtra (n, /) y ordena
// atención-primero, actividad, nombre (R6, R7).
func (m *Model) rows() []row {
	out := make([]row, 0, len(m.projects))
	for _, p := range m.projects {
		snap := m.states[p.Path]
		st := snap.State(p.HasRepo)
		if m.onlyDirty && !pendingStates(st) {
			continue
		}
		if m.search != "" && !matchRepo(p, snap, m.search) {
			continue
		}
		if m.worktreeHidden(p) { // 0002 R15: wt plegados bajo su repo principal
			continue
		}
		out = append(out, row{project: p, snap: snap, state: st})
	}

	sortRows(out)
	return out
}

// entries compone las filas navegables de la tabla (0002 R16, 0003 R19/R20):
// las filas base (filtro+sort) se agrupan con group.Arrange y los bloques
// colapsados omiten sus miembros: plegar un primario oculta también sus
// headers secundarios; plegar un secundario solo sus repos.
func (m *Model) entries() []tableEntry {
	base := m.rows()
	arranged := group.Arrange(toEntries(base))

	out := make([]tableEntry, 0, len(arranged)+2)
	skipPrim, skipSec := "", "" // bloques colapsados cuyas filas se omiten
	for i, e := range arranged {
		if group.IsPrimaryHeader(arranged, i) {
			out = append(out, tableEntry{kind: kindPrimary, group: e.Primary})
			if m.collapsed[e.Primary] {
				skipPrim, skipSec = e.Primary, ""
				continue
			}
			skipPrim = ""
		}
		if skipPrim == "" && group.IsSecondaryHeader(arranged, i) {
			key := groupKey(e.Primary, e.Secondary)
			out = append(out, tableEntry{kind: kindSecondary, group: key})
			if m.collapsed[key] {
				skipSec = key
				continue
			}
			skipSec = ""
		}
		if skipPrim != "" || skipSec != "" {
			continue
		}
		r := row{project: e.Proj, snap: e.Snap, state: e.State}
		out = append(out, tableEntry{kind: kindRepo, r: r})
		// 0006 R30/R31: sub-filas de worktree justo bajo su padre visible,
		// solo si el repo está expandido (persistido o inducido por R40).
		// Se inyectan DESPUÉS de group.Arrange, así no alteran headers,
		// orden, contadores ni resumen (0003 R19/R20 intactos). El guard de
		// plegado (skipPrim/skipSec) ya garantiza R36.2.
		out = append(out, m.worktreeEntries(r)...)
	}
	return out
}

// worktreeEntries compone las sub-filas de worktree de un repo (0006 R30).
// Vacío si el repo no es expandible o no está expandido. Con búsqueda activa
// (R40) solo se muestran los worktrees que matchean; la coincidencia por
// worktree induce la expansión de forma transitoria (S40.5), sin tocar
// m.expanded.
func (m *Model) worktreeEntries(r row) []tableEntry {
	if !m.expandable(r) || !m.repoExpanded(r) {
		return nil
	}
	var out []tableEntry
	for _, wt := range r.snap.Worktrees {
		if m.search != "" && !worktreeMatches(wt, m.search) {
			continue
		}
		out = append(out, tableEntry{kind: kindWorktree, wt: wt, parent: r.project.Path})
	}
	return out
}

// expandable reporta si un repo puede expandir worktrees (0006 R30/R31):
// solo repos no-worktree con worktrees inventariados. El principal ya viene
// excluido del snapshot (S31.2) y un worktree huérfano no expande (S31.4/R39).
func (m *Model) expandable(r row) bool {
	return !r.project.IsWorktree && len(r.snap.Worktrees) > 0
}

// repoExpanded reporta la expansión efectiva (0006 R40): la persistida, o
// la inducida transitoriamente por una búsqueda que matchea algún worktree.
func (m *Model) repoExpanded(r row) bool {
	if m.expanded[r.project.Path] {
		return true
	}
	if m.search == "" {
		return false
	}
	for _, wt := range r.snap.Worktrees {
		if worktreeMatches(wt, m.search) {
			return true
		}
	}
	return false
}

// groupOfEntry devuelve la clave de plegado de la entrada: el contenedor más
// interno para repos (secundario si tiene, si no primario: S20.3) o la clave
// del propio header.
func groupOfEntry(e tableEntry) string {
	switch e.kind {
	case kindPrimary, kindSecondary:
		return e.group
	default:
		p := e.r.project
		if p.PrimaryGroup == "" {
			return group.Ungrouped // sección final plegable
		}
		if p.SecondaryGroup != "" {
			return groupKey(p.PrimaryGroup, p.SecondaryGroup)
		}
		return p.PrimaryGroup
	}
}

// toEntries adapta las filas base a las Entry del package group.
func toEntries(rs []row) []group.Entry {
	out := make([]group.Entry, 0, len(rs))
	for _, r := range rs {
		out = append(out, group.Entry{
			Primary:   r.project.PrimaryGroup,
			Secondary: r.project.SecondaryGroup,
			Proj:      r.project,
			Snap:      r.snap,
			State:     r.state,
		})
	}
	return out
}

// worktreeHidden reporta si un worktree descubierto debe plegarse bajo su
// repo principal (0002 R15/S15.1). El fallback de huérfanos (S15.3):
// si el repo principal no está entre los descubiertos, sigue visible.
// Los paths se normalizan (0006 S31.3): symlink o barra final no duplican.
func (m *Model) worktreeHidden(p discovery.Project) bool {
	if !p.IsWorktree || p.MainRepo == "" {
		return false
	}
	main := filepath.Clean(p.MainRepo)
	for _, q := range m.projects {
		if filepath.Clean(q.Path) == main {
			return true
		}
	}
	return false
}

// matchSearch compara nombre, primary y secondary, case-insensitive (S7.2,
// 0003 S18.5).
func matchSearch(p discovery.Project, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.PrimaryGroup), q) ||
		strings.Contains(strings.ToLower(p.SecondaryGroup), q)
}

// matchRepo amplía matchSearch con los worktrees del repo (0006 R40): un
// repo es visible si él mismo matchea o si matchea algún worktree suyo
// (rama o basename del directorio), aunque el repo no matchee por sí mismo.
func matchRepo(p discovery.Project, snap gitstatus.Snapshot, q string) bool {
	if matchSearch(p, q) {
		return true
	}
	for _, wt := range snap.Worktrees {
		if worktreeMatches(wt, q) {
			return true
		}
	}
	return false
}

// worktreeMatches compara la rama y el basename del directorio del worktree
// contra la búsqueda (0006 R40.1/R40.2), case-insensitive.
func worktreeMatches(wt gitstatus.Worktree, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(wt.Branch), q) ||
		strings.Contains(strings.ToLower(filepath.Base(wt.Path)), q)
}

// sortRows ordena in-place: score desc, último commit desc, nombre asc (R6).
func sortRows(rows []row) {
	// insertion sort simple: los repos son cientos, no miles.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rowLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func rowLess(a, b row) bool {
	sa, sb := a.state.Score(), b.state.Score() // R6: atención-primero
	if sa != sb {
		return sa > sb
	}
	if a.snap.LastCommit != b.snap.LastCommit {
		return a.snap.LastCommit > b.snap.LastCommit
	}
	return strings.ToLower(a.project.Name) < strings.ToLower(b.project.Name)
}

// summary cuenta los estados de todos los proyectos para la barra (R6).
// Los worktree plegados no cuentan como repos (0002 R15).
func (m *Model) summary() (total, dirty, ahead, behind int) {
	for _, p := range m.projects {
		if m.worktreeHidden(p) {
			continue
		}
		snap := m.states[p.Path]
		total++
		switch snap.State(p.HasRepo) {
		case gitstatus.StateDirty, gitstatus.StateDiverged:
			dirty++
		}
		if snap.Status.Ahead > 0 {
			ahead++
		}
		if snap.Status.Behind > 0 {
			behind++
		}
	}
	return total, dirty, ahead, behind
}

// ---- render de celdas: cada celda devuelve texto plano + estilo;
// el render final hace pad(texto) y luego aplica el estilo, así la
// alineación nunca se rompe por códigos ANSI.

// renderEntry compone una línea navegable: header primario, header
// secundario (indentado) o fila de repo (0003 R20). El cursor sigue la
// misma convención que las filas de repo.
func (m *Model) renderEntry(e tableEntry, selected bool) string {
	switch e.kind {
	case kindPrimary:
		line := m.primaryHeaderLine(e.group)
		if selected {
			return styleCursor.Render("▸ ") + line
		}
		return "  " + line
	case kindSecondary:
		line := m.secondaryHeaderLine(e.group)
		if selected {
			return styleCursor.Render("▸ ") + indentHeader + line
		}
		return "  " + indentHeader + line
	case kindWorktree:
		// 0006 R34: render DEDICADO. NO usar renderRow: un Snapshot vacío se
		// leería como no-up/clean (falsa información de estado).
		return m.renderWorktreeRow(e.wt, selected)
	default:
		return m.renderRow(e.r, selected)
	}
}

// indentHeader es el sangrado de los headers secundarios dentro de su
// primario (0003 R20).
const indentHeader = "  "

// renderWorktreeRow compone una sub-fila de worktree (0006 R34): indentada
// con glyph propio `↳`, distinta de una fila de repo y de un header de grupo.
// Muestra el basename del worktree y su rama (o `(detached)` con el head
// corto). Las celdas que dependen de estado git por-worktree (Work Tree,
// ↑↓up, SYNC, ACTIVITY, FETCH) quedan vacías/dim: la UI NO promete
// dirty/ahead/behind/sync por worktree (R34.4). Respeta el contrato de celda
// `(texto, estilo)` con pad() ANTES del estilo.
func (m *Model) renderWorktreeRow(wt gitstatus.Worktree, selected bool) string {
	// R34.3: en detached no hay rama; el head corto es el dato disponible.
	branch := wt.Branch
	if branch == "" {
		branch = "(detached)"
		if wt.Head != "" {
			branch += " " + wt.Head
		}
	}
	cells := []struct {
		text  string
		style lipglossStyle
		width int
	}{
		{"  ↳ " + filepath.Base(wt.Path), styleWorktree, colName},
		{branch, styleWorktree, colBranch},
		{"", styleDim, colWT},
		{"", styleDim, colUpDown},
		{"—", styleDim, colSync},
		{"", styleDim, colActivity},
		{"", styleDim, colFetch},
	}
	line := ""
	for _, c := range cells {
		line += c.style.Render(pad(truncate(c.text, c.width), c.width))
	}
	if selected {
		return styleCursor.Render("▸ ") + line
	}
	return "  " + line
}

// primaryHeaderLine dibuja `▾/▸ primario (n)`: expandido/colapsado con el
// número de repos del primario incluidos sus secundarios (S20.4).
func (m *Model) primaryHeaderLine(g string) string {
	glyph := "▾"
	if m.collapsed[g] {
		glyph = "▸"
	}
	return styleGroupHeader.Render(glyph + " " + fmt.Sprintf("%s (%d)", g, m.countPrimary(g)))
}

// secondaryHeaderLine dibuja `▾/▸ secundario (n)` (sin el primario: ya está
// en su header padre).
func (m *Model) secondaryHeaderLine(key string) string {
	glyph := "▾"
	if m.collapsed[key] {
		glyph = "▸"
	}
	_, sec, _ := strings.Cut(key, "/")
	return styleSecondaryHeader.Render(glyph + " " + fmt.Sprintf("%s (%d)", sec, m.countSecondary(key)))
}

// countPrimary cuenta los repos visibles del primario (después de filtros,
// antes de plegado), incluidos los de sus secundarios.
func (m *Model) countPrimary(g string) int {
	n := 0
	for _, e := range group.Arrange(toEntries(m.rows())) {
		if e.Primary == g {
			n++
		}
	}
	return n
}

// countSecondary cuenta los repos visibles del secundario `prim/sec`.
func (m *Model) countSecondary(key string) int {
	n := 0
	for _, e := range group.Arrange(toEntries(m.rows())) {
		if e.Primary != "" && groupKey(e.Primary, e.Secondary) == key {
			n++
		}
	}
	return n
}

// renderRow compone una línea de la tabla con cursor opcional (R6).
// 0004 R25: sin columna GROUP (los headers plegables ya identifican el
// grupo); Work Tree sustituye a STATE (R26).
func (m *Model) renderRow(r row, selected bool) string {
	name, nameStyle := m.nameCell(r)
	branch, branchStyle := m.branchCell(r)
	wt, wtStyle := m.wtCell(r)
	upDown, upDownStyle := m.upDownCell(r)
	sync, syncStyle := m.syncCell(r)
	activity, activityStyle := activityCell(r)
	fetch, fetchStyle := m.fetchCell(r.project.Path)

	cells := []struct {
		text  string
		style lipglossStyle
		width int
	}{
		{name, nameStyle, colName},
		{branch, branchStyle, colBranch},
		{wt, wtStyle, colWT},
		{upDown, upDownStyle, colUpDown},
		{sync, syncStyle, colSync},
		{activity, activityStyle, colActivity},
		{fetch, fetchStyle, colFetch},
	}

	line := ""
	for _, c := range cells {
		line += c.style.Render(pad(truncate(c.text, c.width), c.width))
	}
	if selected {
		return styleCursor.Render("▸ ") + line
	}
	return "  " + line
}

func (m *Model) nameCell(r row) (string, lipglossStyle) {
	name := r.project.Name
	if r.project.IsWorktree {
		name += " [wt]" // S3.2: tag de worktree (huérfanos y modo print)
	}
	// 0002 R15/S15.4: indicador de worktrees del repo principal; 0006 R34.5:
	// glyph de expansión `▸/▾` junto al contador. Misma guarda que
	// `expandable` (0006 HIGH-1): un worktree huérfano (kindRepo +
	// IsWorktree) no es expandible, así que no debe mostrar glyph ni
	// contador (dead affordance).
	if m.expandable(r) {
		glyph := "▸"
		if m.repoExpanded(r) {
			glyph = "▾"
		}
		name += fmt.Sprintf(" %s (%d wt)", glyph, len(r.snap.Worktrees))
	}
	if r.project.MarkerErr != "" {
		return name, styleWarn // S4.3: marcador malformado visible
	}
	return name, styleSel
}

// wtCell compone la columna Work Tree: SOLO working tree (0004 R26).
// `N ?M` (tracked/untracked), vacío si limpio (tabla quieta, R28), `∅`
// no-repo y `⚠` error. Sin `●`, `⇅`, `detached` ni `no-upstream`: detached
// vive solo en BRANCH y no-up solo en ↑↓up.
func (m *Model) wtCell(r row) (string, lipglossStyle) {
	switch r.state {
	case gitstatus.StateError:
		return "⚠", styleError // S4.3: el detalle muestra el error completo
	case gitstatus.StateNoRepo:
		return "∅", styleDim // S3.3
	}
	if r.snap.Status.Dirty() > 0 {
		return dirtyTail(r), m.styleFor(r)
	}
	return "", styleClean
}

func (m *Model) branchCell(r row) (string, lipglossStyle) {
	switch {
	case r.state == gitstatus.StateNoRepo || r.snap.Status.Branch == "":
		return "-", styleDim
	case r.snap.Status.Detached:
		return r.snap.Status.Branch + " (detached)", m.styleFor(r) // S5.5
	default:
		return r.snap.Status.Branch, m.styleFor(r)
	}
}

// fetchCell compone el estado TRANSITORIO del fetch (0004 R24): solo ⟳ y ✗.
// En éxito queda vacía (el éxito lo señala la notificación de la barra,
// S8.2); el fallo persiste hasta el próximo fetch de ese repo.
func (m *Model) fetchCell(path string) (string, lipglossStyle) {
	switch m.fetchStates[path] {
	case "fetching":
		return "⟳ fetch", styleFetchRun
	case "failed":
		return "✗ fetch", styleFetchBad
	default:
		return "", styleClean
	}
}

// dirtyTail compone "N ?M" para tracked/untracked (S5.3).
func dirtyTail(r row) string {
	s := r.snap.Status
	out := ""
	if s.TrackedChanges > 0 {
		out += fmt.Sprintf("%d", s.TrackedChanges)
	}
	if s.Untracked > 0 {
		out += fmt.Sprintf(" ?%d", s.Untracked)
	}
	return strings.TrimSpace(out)
}

// upDownCell compone ↑↓ vs upstream (0004 R27): `↑N↓N`/`↑N`/`↓N` — diverged
// vive aquí, sin `⇅` en Work Tree —, `no-up` sin upstream trackeado y vacío
// en sync (tabla quieta, sin `·`). Errores y no-repo quedan vacíos.
func (m *Model) upDownCell(r row) (string, lipglossStyle) {
	s := r.snap.Status
	if r.state == gitstatus.StateNoRepo || r.snap.Err != "" {
		return "", styleClean
	}
	if !s.HasUpstream {
		return "no-up", styleWarn // S5.4: rama sin cuerda al remoto
	}
	switch {
	case s.Ahead > 0 && s.Behind > 0:
		return fmt.Sprintf("↑%d↓%d", s.Ahead, s.Behind), styleDiverged
	case s.Ahead > 0:
		return fmt.Sprintf("↑%d", s.Ahead), styleAhead
	case s.Behind > 0:
		return fmt.Sprintf("↓%d", s.Behind), styleBehind
	default:
		return "", styleClean
	}
}

// syncCell compone la desviación vs sync branch con la rama visible (0004
// R23): `<rama> ↓N` behind (warn), `<rama>` a secas en sync (dim, sin tick),
// `<rama> —` con ref inexistente y `—` sin rama resuelta.
func (m *Model) syncCell(r row) (string, lipglossStyle) {
	if r.state == gitstatus.StateNoRepo || r.snap.SyncBranch == "" {
		return "—", styleDim
	}
	if !r.snap.SyncKnown {
		return r.snap.SyncBranch + " —", styleDim // S23.4: ref inexistente
	}
	if r.snap.SyncBehind == 0 {
		return r.snap.SyncBranch, styleDim // S23.3: tabla quieta
	}
	return fmt.Sprintf("%s ↓%d", r.snap.SyncBranch, r.snap.SyncBehind), styleWarn
}

// activityCell devuelve la fecha relativa del último commit (R6).
func activityCell(r row) (string, lipglossStyle) {
	return relativeTime(r.snap.LastCommit), styleDim
}

// styleFor elige el estilo según el estado derivado.
func (m *Model) styleFor(r row) lipglossStyle {
	switch r.state {
	case gitstatus.StateError:
		return styleError
	case gitstatus.StateNoRepo:
		return styleDim
	case gitstatus.StateNoUpstream:
		return styleWarn
	case gitstatus.StateDetached:
		return styleWarn
	case gitstatus.StateDiverged:
		return styleDiverged
	case gitstatus.StateDirty:
		return styleDirty
	case gitstatus.StateAhead:
		return styleAhead
	case gitstatus.StateBehind:
		return styleBehind
	default:
		return styleClean
	}
}

// ---- helpers de texto ----

func truncate(s string, w int) string {
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	runes := []rune(s)
	return string(runes[:w-1]) + "…"
}

// pad rellena a la derecha midiendo runes (solo texto plano, sin ANSI).
func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// stripANSI quita secuencias ANSI para comparar contenido en tests.
func stripANSI(s string) string {
	var out strings.Builder
	inSeq := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inSeq = true
		case inSeq && (r == 'm' || r == 'K'):
			inSeq = false
		case !inSeq:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// relativeTime formatea un epoch como "3h", "2d" (R6).
func relativeTime(epoch int64) string {
	if epoch <= 0 {
		return "-"
	}
	d := time.Since(time.Unix(epoch, 0))
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}
