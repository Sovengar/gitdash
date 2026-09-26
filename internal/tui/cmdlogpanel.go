// Vista del command log: qué se ha ejecutado de verdad, con qué resultado y
// por qué. Es un view mode más (como detailOpen), no un overlay: necesita
// scroll y muchas líneas, y tapar la tabla obligaría a reservar su alto igual
// que hacen los toasts.
package tui

import (
	"fmt"
	"strings"
	"time"

	"gitdash/internal/cmdlog"
)

// Anchos de columna del panel. El argv se lleva lo que sobra: en un log el
// comando es lo que hay que leer entero.
const (
	logColTime    = 12 // "15:04:05.000"
	logColKind    = 7  // "key", "pull", "fetch", "worktree" (truncado)
	logColRepo    = 14
	logColOutcome = 16
	logColVerdict = 8 // "412ms", "exit 1"
	logColSep     = 2
	// logMinArgv es el mínimo que se le concede al argv antes de empezar a
	// quitarle columnas a las demás. 20 cabe "git pull --ff-only": por debajo
	// de eso el comando se lee a medias, que es justo lo que el panel existe
	// para evitar. La variante más larga (`--rebase --autostash`) se recorta,
	// pero la línea de intención de arriba ya nombra la variante.
	logMinArgv = 20
)

// logColumns reparte el ancho interior entre las columnas. Degrada en orden de
// menor valor: primero el veredicto, luego el resultado y el repo, y solo
// entonces el argv. bordered ya recorta cada línea al ancho interior, así que
// esto es para no desperdiciar terminal, no para evitar desbordes.
type logColumns struct {
	kind, repo, outcome, verdict, argv int
}

func computeLogColumns(inner int) logColumns {
	c := logColumns{kind: logColKind, repo: logColRepo, outcome: logColOutcome, verdict: logColVerdict}
	fixed := func() int {
		return logColTime + logColSep + c.kind + logColSep + c.repo + logColSep + c.outcome + logColSep + c.verdict
	}
	for fixed()+logMinArgv > inner {
		switch {
		case c.verdict > 0:
			c.verdict = 0
		case c.outcome > 0:
			c.outcome = 0
		case c.repo > 0:
			c.repo = 0
		case c.kind > 4:
			c.kind = 4
		default:
			// Sin sitio para más: el argv se queda con lo que hay.
			c.argv = max(0, inner-logColTime-logColSep)
			return c
		}
	}
	c.argv = max(logMinArgv, inner-fixed())
	return c
}

// logEntries devuelve las entradas visibles con el filtro aplicado, y refresca
// la copia cacheada solo cuando el ring ha cambiado. Sin esto, cada frame
// copiaría 500 entradas para pintar las ~30 últimas.
func (m *Model) logEntries() []cmdlog.Entry {
	if seq := cmdlog.LastSeq(); seq != m.logCacheSeq {
		m.logCache = cmdlog.Entries()
		m.logCacheSeq = seq
	}
	if m.logShowAll {
		return m.logCache
	}
	// Por defecto solo las acciones del usuario: las lecturas del scan son
	// ~4 por repo y repetirían la misma pregunta.
	out := make([]cmdlog.Entry, 0, len(m.logCache))
	for _, e := range m.logCache {
		if e.Class == cmdlog.ClassAction {
			out = append(out, e)
		}
	}
	return out
}

// logSection pinta el panel con cabecera, líneas visibles y pie con el
// desplazamiento. El offset cuenta desde la cola: 0 es "lo más reciente al
// final", que es donde se mira un log; las entradas nuevas no te sacan de
// sitio si estabas scrolleado arriba.
func (m *Model) logSection(bodyLines int) string {
	entries := m.logEntries()
	cols := computeLogColumns(max(0, m.width-2))

	// El scroll parte de las últimas N líneas que caben (una para la
	// cabecera), y se recorta para que el desplazamiento nunca deje huecos
	// al final.
	visible := bodyLines - 1
	offset := min(m.logOffset, max(0, len(entries)-visible))
	if visible < 1 {
		visible = 1
	}
	start := max(0, len(entries)-visible-offset)

	rows := make([]string, 0, visible)
	for _, e := range entries[start : start+min(visible, len(entries)-start)] {
		rows = append(rows, m.logLine(e, cols))
	}
	// Rellenar hasta el presupuesto: la caja no debe encogerse porque el log
	// tenga pocas entradas.
	for len(rows) < visible {
		rows = append(rows, "")
	}

	title := "log · actions"
	if m.logShowAll {
		title = "log · all"
	}
	header := "  " + m.logHeader(cols)
	body := styleHint.Render(header) + "\n" + strings.Join(rows, "\n")
	return m.section(title, body)
}

// logHeader rotula las columnas con los mismos anchos que las líneas.
func (m Model) logHeader(c logColumns) string {
	var b strings.Builder
	b.WriteString(pad("TIME", logColTime))
	if c.kind > 0 {
		b.WriteString(pad("KIND", c.kind) + strings.Repeat(" ", logColSep))
	}
	if c.repo > 0 {
		b.WriteString(pad("REPO", c.repo) + strings.Repeat(" ", logColSep))
	}
	if c.argv > 0 {
		b.WriteString(pad("COMMAND", c.argv) + strings.Repeat(" ", logColSep))
	}
	if c.outcome > 0 {
		b.WriteString(pad("RESULT", c.outcome) + strings.Repeat(" ", logColSep))
	}
	if c.verdict > 0 {
		b.WriteString("VERDICT")
	}
	return b.String()
}

// logLine compone una entrada. La intención se pinta tenue y sin veredicto:
// es contexto de "qué pediste", no un resultado. El código de salida y la
// duración van juntos porque sin uno de los dos la línea no dice nada.
func (m Model) logLine(e cmdlog.Entry, c logColumns) string {
	style := styleLogExec
	if e.Intent {
		style = styleLogIntent
	}
	var b strings.Builder
	b.WriteString(style.Render(pad(e.At.Format("15:04:05.000"), logColTime)))
	if c.kind > 0 {
		// La intención se rotula con su tecla ("key p"): es lo que explica
		// el argv de la línea siguiente.
		kind := "key " + e.Key
		if !e.Intent {
			kind = "exec"
		}
		b.WriteString("  " + style.Render(pad(truncate(kind, c.kind), c.kind)))
	}
	if c.repo > 0 {
		b.WriteString("  " + style.Render(pad(truncate(e.Repo, c.repo), c.repo)))
	}
	if c.argv > 0 {
		cmdline := e.Action
		if !e.Intent {
			cmdline = e.Command()
		}
		if cmdline == "" {
			cmdline = "-"
		}
		b.WriteString("  " + style.Render(pad(truncate(cmdline, c.argv), c.argv)))
	}
	if c.outcome > 0 {
		outcome := e.Outcome
		if outcome == "" && !e.Intent {
			outcome = "-"
		}
		b.WriteString("  " + m.logOutcomeStyle(e).Render(pad(truncate(outcome, c.outcome), c.outcome)))
	}
	if c.verdict > 0 {
		b.WriteString("  " + m.logVerdictStyle(e).Render(pad(truncate(logVerdict(e), c.verdict), c.verdict)))
	}
	return b.String()
}

// logOutcomeStyle colorea el resultado: lo que integró commits con éxito es
// verde, y un conflicto o un rechazo en rojo. Un resultado sin color (up-to-date
// gris) es un acierto también: no había nada que hacer.
func (m Model) logOutcomeStyle(e cmdlog.Entry) lipglossStyle {
	if e.Outcome == "" {
		return styleLogIntent
	}
	switch e.Outcome {
	case "rebase", "rebase+autostash", "merge", "fast-forward", "pushed":
		return styleLogOK
	case "up-to-date":
		return styleHint
	default:
		return styleError
	}
}

// logVerdictStyle verde si el proceso salió con 0.
func (m Model) logVerdictStyle(e cmdlog.Entry) lipglossStyle {
	if e.Exit == 0 {
		return styleHint
	}
	return styleError
}

// logVerdict compone la columna final: duración si la hubo, y el código de
// salida solo cuando no fue 0 o no se pudo obtener. Una intención no tiene
// veredicto: no ha corrido nada todavía.
func logVerdict(e cmdlog.Entry) string {
	if e.Intent {
		return ""
	}
	switch {
	case e.Exit > 0:
		return fmt.Sprintf("exit %d", e.Exit)
	case e.Exit < 0 && e.Dur == 0:
		return "no exit"
	case e.Dur >= time.Second:
		return fmt.Sprintf("%.1fs", e.Dur.Seconds())
	case e.Dur == 0:
		return ""
	default:
		return fmt.Sprintf("%dms", e.Dur.Milliseconds())
	}
}

// logLegend es el aviso persistente del panel: qué teclas hacen qué, igual que
// los avisos armados. Comparte la sección de keybinds porque comparte función
// con las hints ("qué hago ahora"), y se fuerza su visibilidad: sin ella el
// panel sería un texto plano sin explicación de cómo se navega.
func (m Model) logLegend() string {
	toggle := "show all (reads + auto fetch)"
	if m.logShowAll {
		toggle = "show actions only"
	}
	keys := m.cfg.KeyFor("log")
	return fmt.Sprintf("command log: j/k scroll · a %s · %s or esc back", toggle, keys)
}

// logScroll mueve el panel n líneas hacia atrás (n > 0) o hacia la cola
// (n < 0). El offset se recorta contra el número de líneas visibles, que
// depende del alto real de la sección: un offset mayor solo dejaría líneas en
// blanco abajo.
func (m *Model) logScroll(n, visible int) {
	maxOffset := max(0, len(m.logEntries())-visible)
	m.logOffset = min(max(0, m.logOffset+n), maxOffset)
}

// handleLogKey enruta las teclas propias del panel. Se consulta ANTES del
// enrutado normal de la tabla (igual que los estados armados): j/k/up/down
// desplazan el panel en vez de mover el cursor, y el resto de teclas sigue su
// curso normal para que la app nunca quede encerrada aquí dentro.
func (m *Model) handleLogKey(key string, bodyLines int) bool {
	visible := max(1, bodyLines-1)
	switch key {
	case "j", "down":
		m.logScroll(-1, visible)
		return true
	case "k", "up":
		m.logScroll(1, visible)
		return true
	case "a":
		m.logShowAll = !m.logShowAll
		m.logOffset = 0 // el filtro cambia cuántas hay: el offset se recalcula
		return true
	case "esc":
		m.logOpen = false
		return true
	default:
		action := m.actionForKey(key)
		if action == "log" {
			m.logOpen = false
			return true
		}
	}
	return false
}

// toggleLog abre o cierra el panel. Al abrir se ancla en la cola (lo más
// reciente visible) y al cerrar se sueltan los estados armados: navegar fuera
// deja el aviso sin sentido, y dejarlo armado obligaría a acertar la tecla
// siguiente desde un panel que ya no está.
func (m *Model) toggleLog() {
	m.logOpen = !m.logOpen
	if m.logOpen {
		m.logOffset = 0
		return
	}
	m.armed = nil
	m.pullArmed = nil
}
