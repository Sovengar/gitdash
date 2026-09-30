// Overlay de creación de PR/MR: el formulario que arma el argv que ejecutará
// gh o glab (internal/forge). Esta TUI solo RECOGE los parámetros —la
// ejecución es otro paso—, así que nada aquí lanza un proceso.
//
// Es un VIEW MODE, no un estado armado de prefix-key como pullArmed o
// visualArmed. Esos viven de una pulsación: la primera arma y la segunda
// elige variante, y una tecla que no sea variante cancela. Un formulario vive
// N pulsaciones (se escribe un título, varias líneas de cuerpo, se cambia la
// base), así que la tecla que abre no puede ser la que envía, y la que envía
// no puede cancelar todo lo escrito. Por eso el overlay captura el teclado
// entero mientras está abierto, con el mismo molde que el panel del command
// log (logOpen / handleLogKey): se consulta antes del enrutado normal y
// suelta su estado al cerrarse.
package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/forge"
)

// prSubmitKey envía el formulario. No es `enter`: en el cuerpo `enter` es un
// salto de línea, y en los campos de una línea se esperaría que cerraran. Con
// texto siendo la actividad principal del overlay, la única tecla que puede
// significar "enviar" y no "escribir" es una con modificador.
//
// NO es una acción de la config: no se rebindea, y por eso es la única tecla
// del overlay que el aviso escribe literal.
const prSubmitKey = "ctrl+s"

const (
	// prFixedLines son las líneas que el formulario gasta FIJAS: los cuatro
	// campos (title, base, head, draft), la línea de aviso y el rótulo del
	// cuerpo. El alto lo decide el layout, no lo que ocupe el texto, así que el
	// textarea se dimensiona con lo que sobra en vez de crecer: un campo que
	// cambia de alto al escribirse hace saltar todo lo que tiene debajo.
	prFixedLines = 6
	// prMinBodyLines es el alto mínimo del overlay: los rótulos y al menos
	// dos líneas de cuerpo. Por debajo el formulario no se dibuja (media caja
	// es peor que no abrirlo) y ni siquiera se abre.
	prMinBodyLines = prFixedLines + 2
	// prLabelWidth es la columna de los rótulos: dos de sangría, el marcador
	// de foco, la etiqueta rellenada a 7 y el espacio que la separa del valor.
	prLabelWidth = 12
	// prValueSlack es lo que se le descuenta al ancho del valor: la celda que
	// el input dibuja SIEMPRE al final, su cursor, que va dentro del View pero
	// no dentro del ancho que se le pasó. Sin esta celda de margen la línea
	// se sale de la caja y bordered recorta el último carácter.
	prValueSlack = 1
	// prMinValueWidth es la columna de valor más estrecha que el formulario
	// admite: lo mínimo para leer lo que se está escribiendo sin scrollear. Es
	// el ancho de la columna de rótulos, que es lo que hay justo al lado: por
	// debajo el rótulo y el valor ya no se distinguen de un vistazo.
	prMinValueWidth = 12
	// prMinWidth es el ancho EXTERIOR mínimo del overlay: los dos bordes de la
	// caja, la columna de rótulos, la celda de margen del input y la columna de
	// valor mínima. Es el otro eje del mismo principio que prMinBodyLines —por
	// debajo el formulario no se abre, porque no cabe entero— y se deriva de
	// las partes en vez de salirse de un número redondo: lo que garantiza es
	// que el ancho del input sea un ancho, y no un clamp a 1 de un negativo.
	prMinWidth = 2 + prLabelWidth + prValueSlack + prMinValueWidth
)

// prField es el campo con el foco del formulario. El head NO es un campo: se
// leyó del snapshot al armar y se enseña atenuado, porque editarlo a mano
// contradiría el repo del que la fila afirma venir.
type prField int

const (
	prFieldTitle prField = iota
	prFieldBase
	prFieldDraft
	prFieldBody
)

// prFieldCount es el número de campos: el orden de tabulación y su vuelta
// completa salen de aquí, no de una lista repetida en dos sitios.
const prFieldCount = 4

// next avanza el foco dando la vuelta: el cuerpo, que ocupa el resto del
// panel, es el último porque es el único que se recorre escribiendo.
func (f prField) next(delta int) prField {
	return prField((int(f) + delta + prFieldCount) % prFieldCount)
}

// prDraft es el estado del overlay. Es una struct y no un bool porque lleva
// el formulario entero (widgets incluidos): un flag aparte podría quedar a
// true sin contenido detrás, que es el estado imposible de depurar.
//
// path, name y head se capturan AL ARMAR y no se vuelven a leer del cursor:
// el overlay vive N pulsaciones y para entonces el cursor puede estar en otro
// repo. Es el mismo motivo por el que armedVisual captura behind. La base
// viaja prellenada en su input, que es de donde se lee.
type prDraft struct {
	path  string // repo del PR: de dónde sale el argv y a quién se recollecta
	name  string // nombre visible, para el título del panel
	head  string // branch actual del repo: el head del PR (solo lectura)
	draft bool   // abrir el PR en borrador

	title  textinput.Model
	body   textarea.Model
	baseIn textinput.Model // la sync branch capturada al armar, ya cambiable
	focus  prField
	// err es el último error de validación. Vive en el formulario y no en
	// los toasts a propósito: un toast expira a los 3 s y caduca justo
	// mientras el usuario está mirando el campo culpable. Un error de
	// validación es estado del formulario, no un evento.
	err string
}

// prSubmission es un envío que el overlay aceptó y que todavía nadie ha
// ejecutado. Vive en el modelo para que la ejecución sea un paso aparte,
// testeable sin lanzar procesos: la UI recoge, forge/tool corre.
//
// Lo consume prCreateCmd, que llega con un mensaje propio (prStartMsg): aceptar
// el formulario y ejecutarlo son dos pasos, y entre ellos el envío se puede
// inspeccionar sin que ningún proceso haya salido.
type prSubmission struct {
	path   string
	params forge.Params
}

// newPROverlay arma el formulario para la fila r, con la base prellenada y el
// head ya resuelto por quien llama (un worktree sin marcador no tiene snapshot
// propio y su rama sale del inventario del padre). El foco arranca en el
// título: es lo primero que se escribe y lo único que no tiene un valor por
// defecto útil.
//
// La guarda del panel del log no va aquí: este constructor no sabe si el
// formulario se va a poder dibujar.
func newPROverlay(r row, head, base string) *prDraft {
	title := textinput.New()
	// Sin prompt: el que trae el input ("> ") duplicaría el rótulo de la
	// izquierda, y dos rótulos del mismo campo se leen peor que uno.
	title.Prompt = ""
	title.Placeholder = "qué cambia y por qué…"

	body := textarea.New()
	body.ShowLineNumbers = false
	body.Placeholder = "detalle, contexto, checklist…"

	baseIn := textinput.New()
	baseIn.Prompt = ""
	baseIn.SetValue(base)
	// El cursor al final: la base viene prellenada y se edita replacing, no
	// desde el principio.
	baseIn.CursorEnd()

	return &prDraft{
		path:   r.project.Path,
		name:   r.project.Name,
		head:   head,
		title:  title,
		body:   body,
		baseIn: baseIn,
		focus:  prFieldTitle,
	}
}

// baseValue es la base escrita en el campo. El input es la única fuente de la
// verdad: un string paralelo prellenado junto al input podría divergir del
// valor que el usuario ve, y lo que importa es el que ve.
func (d *prDraft) baseValue() string {
	return strings.TrimSpace(d.baseIn.Value())
}

// openPR arma el overlay sobre la fila del cursor. La tecla es la de la acción
// `pr` de la config, así que un rebind la mueve sin tocar este archivo.
//
// La fila se resuelve y se captura AQUÍ, y no al enviar: la tecla que abre y
// la que envía son distintas y entre medias el usuario puede mover el cursor,
// plegar un grupo o abrir el panel del log. Un envío tiene que salir del repo
// que la fila señalaba al abrir.
func (m Model) openPR() (tea.Model, tea.Cmd) {
	// El log ya es el cuerpo del dashboard: dos overlays a la vez no tienen
	// sitio ni sentido. La tecla de `pr` ya está frenada antes, en el enrutado
	// (está en el guard de m.logOpen), así que esta es la segunda red: el
	// switch de acciones que la despacha no sabe qué hay abierto.
	if m.logOpen {
		return m, nil
	}
	r, ok := m.selected()
	if !ok {
		return m, m.toastCmd(toastInfo, "select a repository")
	}
	if !r.project.HasRepo {
		return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
	}

	// La base se prellena con la sync branch que el scan ya resolvió para
	// ese repo (override del marcador > global). El snapshot manda cuando la
	// tiene: es el mismo valor, ya normalizado por la recolección.
	base := r.snap.SyncBranch
	if base == "" {
		// Sin snapshot no hay a quién preguntarle por la referenciaeffective
		// (una sub-fila de worktree no tiene el suyo) ni dónde resolverla: el
		// bool del fallback se queda sin usar a propósito, porque resolverlo
		// aquí sería un `rev-list` en el hilo de la UI.
		base, _ = m.syncOf(r.project.Path)
	}
	// Un worktree sin marcador no tiene snapshot propio, igual que para los
	// placeholders de la IA: la rama sale del inventario del padre, la MISMA
	// fuente que usa la sub-fila, y no se inventa.
	head := r.snap.Status.Branch
	if head == "" {
		if wt, ok := m.worktreeFor(r.project.Path); ok {
			head = worktreeBranchLabel(wt)
		}
	}

	m.pr = newPROverlay(r, head, base)
	// El alto lo decide el layout, así que la pregunta es si el hueco que le
	// toca llega al mínimo del formulario. Si no cabe no se abre: entrar en
	// un overlay que no se dibuja deja al usuario escribiendo a ciegas.
	if !m.prFits() {
		m.pr = nil
		return m, m.toastCmd(toastInfo, "terminal too small for the PR form")
	}
	// Abrir el overlay suelta los selectores armados: su aviso vive en la
	// sección de keybinds, que ahora pinta la leyenda del formulario, y la
	// segunda tecla que esperaban nunca va a llegar.
	m.armed = nil
	m.pullArmed = nil
	m.visualArmed = nil
	m.prFit()
	return m, m.prFocus(prFieldTitle)
}

// prFits dice si el overlay cabe entero en la terminal. El presupuesto de alto
// lo fija el layout (el formulario ES el cuerpo), así que esa pregunta es una:
// ¿el cuerpo que le toca llega al mínimo del formulario? El ancho no lo reparte
// nadie —lo reparte la terminal— pero también tiene su mínimo: es lo que hace
// que el ancho del input sea un ancho y no un clamp a 1 de una resta negativa.
// Un terminal angosto y alto es el caso que el alto solo no ve.
func (m Model) prFits() bool {
	return m.pr != nil && m.layout().bodyLines >= prMinBodyLines && m.width >= prMinWidth
}

// prValueWidth es el ancho de la columna de valor de los inputs: el interior de
// la caja menos la columna de rótulos y la celda de margen del input. Con
// prMinWidth como suelo —que es lo que hace abrir el overlay— nunca sale
// negativo, así que el max(1, …) es solo la red por si el ancho o el alto
// cambiaran por otro camino.
func (m Model) prValueWidth() int {
	return max(1, m.width-2-prLabelWidth-prValueSlack)
}

// prFit dimensiona los widgets al hueco real: el ancho interior de la caja
// menos la columna de rótulos para los inputs, y el alto que sobra tras los
// rótulos y la línea de aviso para el textarea. Se llama al abrir y en cada
// resize, nunca en el render: el alto lo dice el layout, no lo que mida hoy
// el texto (igual que la ficha del repo, que se rellena a su alto en vez de
// encoger la caja).
func (m *Model) prFit() {
	if m.pr == nil {
		return
	}
	rows := m.layout().bodyLines
	inner := max(1, m.width-2)
	value := m.prValueWidth()
	m.pr.title.SetWidth(value)
	m.pr.baseIn.SetWidth(value)
	m.pr.body.SetWidth(inner)
	m.pr.body.SetHeight(max(1, rows-prFixedLines))
}

// closePR cierra el overlay sin efectos. Suelta el foco de los widgets (para
// que no quede un cursor parpadeando en un campo que ya no se escribe) y
// suelta la struct entera, de modo que reabrir no hereda ni el texto ni el
// borrador del intento anterior.
func (m *Model) closePR() {
	if m.pr != nil {
		m.pr.title.Blur()
		m.pr.baseIn.Blur()
		m.pr.body.Blur()
	}
	m.pr = nil
}

// prFocus mueve el foco a un campo: el anterior lo pierde (y con él su cursor
// y su prompt de parpadeo) y el nuevo lo toma. El foco es lo único que
// devuelve una tea.Cmd, así que se propaga.
func (m *Model) prFocus(f prField) tea.Cmd {
	switch m.pr.focus {
	case prFieldTitle:
		m.pr.title.Blur()
	case prFieldBase:
		m.pr.baseIn.Blur()
	case prFieldBody:
		m.pr.body.Blur()
	}
	m.pr.focus = f
	switch f {
	case prFieldTitle:
		return m.pr.title.Focus()
	case prFieldBase:
		return m.pr.baseIn.Focus()
	case prFieldBody:
		return m.pr.body.Focus()
	}
	return nil
}

// handlePRKey enruta las teclas del overlay. Devuelve false para las que NO
// son suyas y tienen que seguir su curso normal, que hoy es una sola: ctrl+c,
// que sigue cerrando la app igual que en el panel del log. Un view mode que
// se traga el abort del terminal deja al usuario sin salida.
//
// El resto NO se reenvía al enrutado normal: se envía al widget del campo con
// el foco. Por eso escribir "p" en el título no arma el selector de pull.
func (m Model) handlePRKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// Keystroke() y no String(): dentro de un formulario la misma letra con y
	// sin modificador tienen que ser dos teclas distintas, y String() devuelve
	// el Text crudo del terminal cuando lo hay, con lo que "ctrl+s" se
	// colapsaría en "s" —la tecla de enviar se volvería una letra del título—.
	// Keystroke() se compone siempre de Mod + Code y no depende de eso.
	key := msg.Keystroke()
	switch key {
	case "ctrl+c":
		return m, nil, false
	case "esc":
		m.closePR()
		return m, nil, true
	case "tab":
		return m, m.prFocus(m.pr.focus.next(1)), true
	case "shift+tab":
		return m, m.prFocus(m.pr.focus.next(-1)), true
	case prSubmitKey:
		return m, m.prSubmit(), true
	}

	switch m.pr.focus {
	case prFieldDraft:
		// El único campo sin widget detrás: space (y enter, que es lo que
		// haría el resto de campos) lo giran. Cualquier otra tecla se
		// ignora en vez de caer en la tabla.
		if key == "space" || key == "enter" {
			m.pr.draft = !m.pr.draft
		}
		return m, nil, true
	case prFieldBody:
		ta, cmd := m.pr.body.Update(msg)
		m.pr.body = ta
		m.pr.err = ""
		return m, cmd, true
	case prFieldBase:
		in, cmd := m.pr.baseIn.Update(msg)
		m.pr.baseIn = in
		m.pr.err = ""
		return m, cmd, true
	default: // prFieldTitle
		in, cmd := m.pr.title.Update(msg)
		m.pr.title = in
		m.pr.err = ""
		return m, cmd, true
	}
}

// prSubmit valida y publica los parámetros. NO ejecuta nada: el argv lo compone
// forge.BuildCreateArgv y lo corre forge/tool, que es otro paso (y otro
// mensaje, prStartMsg).
//
// Publicar el envío en m.prPending es lo que permite que ese paso exista: la
// UI vuelve al dashboard con los parámetros recogidos y es prCreateCmd quien los
// ejecuta y avisa, igual que cualquier otra acción deja su resultado en el log.
// El Cmd que devuelve es el salto al paso siguiente, y nil cuando la validación
// falla (ahí el aviso va en la línea del formulario, no en un toast).
func (m *Model) prSubmit() tea.Cmd {
	p := m.prParams()
	// El aviso va a la línea del formulario, no a un toast: un toast expira a
	// los 3 s y se va justo cuando el usuario está mirando el campo culpable.
	// Aquí además dice cuál de los dos falta, y sigue ahí hasta que se
	// corrige o se cierra.
	switch {
	case p.Title == "":
		m.pr.err = "title required"
		return nil
	case p.Base == "":
		m.pr.err = "base branch required"
		return nil
	}
	m.prPending = &prSubmission{path: m.pr.path, params: p}
	m.closePR()
	return func() tea.Msg { return prStartMsg{} }
}

// prParams recoge los parámetros del overlay abierto, sin validar ni publicar:
// es la lectura, y prSubmit la que decide. Título y base se recortan porque son
// flags de una línea y un espacio al final solo puede ser un descuido; el
// cuerpo NO se toca, porque es texto libre y lo que el usuario escribió (saltos
// de línea, comillas y signos de shell incluidos) tiene que llegar al argv tal
// cual, sin quotes ni escapes inventados por aquí.
func (m Model) prParams() forge.Params {
	if m.pr == nil {
		return forge.Params{}
	}
	return forge.Params{
		Title: strings.TrimSpace(m.pr.title.Value()),
		Body:  m.pr.body.Value(),
		Base:  m.pr.baseValue(),
		Head:  m.pr.head,
		Draft: m.pr.draft,
	}
}

// prSection pinta el formulario: los rótulos con el valor de cada campo, el
// cuerpo multilínea y la línea de aviso. Devuelve "" si el hueco no llega al
// mínimo, para que quien lo compone no deje una caja a medias.
func (m Model) prSection(rows int) string {
	if m.pr == nil || rows < prMinBodyLines {
		return ""
	}
	// Los campos se pintan con el View() de su widget, no con su Value(): es
	// el View el que trae el cursor, el placeholder y el scroll horizontal
	// cuando el texto no cabe.
	title := m.prFieldLine("title", m.pr.focus == prFieldTitle, m.pr.title.View())
	base := m.prFieldLine("base", m.pr.focus == prFieldBase, m.pr.baseIn.View())
	head := m.prFieldLine("head", false, styleDim.Render(m.pr.head))
	draft := m.prFieldLine("draft", m.pr.focus == prFieldDraft, m.pr.draftLabel())
	// El aviso va ENTRE los campos y el cuerpo, no debajo: a 20 líneas del
	// título, "falta el título" es un mensaje que se busca, no uno que se ve.
	notice := m.prNoticeLine()

	lines := []string{title, base, head, draft, notice, m.prBodyLabel(), m.pr.body.View()}
	return m.section("new PR · "+m.pr.name, fitLines(strings.Join(lines, "\n"), rows))
}

// prNoticeLine es la línea reservada bajo los campos: el error de validación
// cuando hay, vacía cuando no. Se reserva SIEMPRE, incluso sin error, para
// que el textarea no cambie de alto al aparecer un aviso: un campo que salta
// bajo el cursor mientras se escribe es peor que un error que hay que leer dos
// veces.
func (m Model) prNoticeLine() string {
	if m.pr.err == "" {
		return ""
	}
	return "  " + styleError.Render(m.pr.err)
}

// prBodyLabel rotula el cuerpo. Comparte la columna de los rótulos con el
// resto de campos (marcador de foco incluido) pero sin valor: el cuerpo es un
// bloque multilínea y lo que marca dónde empieza es su propio prompt, la regla
// ┃. El TrimRight quita el espacio que deja la columna de valor vacía.
func (m Model) prBodyLabel() string {
	return strings.TrimRight(m.prFieldLine("body", m.pr.focus == prFieldBody, ""), " ")
}

// prLabel compone la sangría con el marcador de foco. El marcador es el mismo
// ▸ que señala la fila del cursor en la tabla: el dashboard ya tiene un
// lenguaje para "esto es lo que estás tocando" y el formulario no necesita
// inventar otro. Va en la columna en vez de rodear el valor porque el input
// rellena su View con espacios hasta el ancho: entre corchetes, el de cierre
// caería contra el borde de la caja.
func (m Model) prLabel(focused bool) string {
	mark := " "
	if focused {
		mark = styleCursor.Render("▸")
	}
	return "  " + mark + " "
}

// prFieldLine compone "  ▸ rótulo   valor". El rótulo se rellena ANTES de
// aplicar estilo, porque el ANSI rompe el cálculo de ancho; y el del campo con
// el foco va en color de aviso, para que el cursor del input y el rótulo
// señalen lo mismo desde dos sitios.
func (m Model) prFieldLine(label string, focused bool, value string) string {
	key := styleDetailKey
	if focused {
		key = styleWarn.Bold(true)
	}
	return m.prLabel(focused) + key.Render(pad(label, 7)) + " " + value
}

// draftLabel nombra el estado del toggle con una palabra, no con un símbolo:
// es la única parte del formulario que no es texto libre y tiene que leerse
// de un vistazo.
func (d *prDraft) draftLabel() string {
	if d.draft {
		return "on"
	}
	return "off"
}

// prPrompt es el aviso persistente del overlay, pintado en la sección de
// keybinds en lugar de las hints. Comparte función con ellas ("qué hago
// ahora") y comparte su vida: existe mientras el overlay está abierto.
//
// La tecla que abre la resuelve la config (KeyFor), no una constante del
// paquete: con un rebind de `pr` un aviso escrito a mano dejaría al usuario
// leyendo una tecla que ya no abre nada. La de enviar NO sale de la config
// porque no es una acción rebindeable (ver prSubmitKey).
func (m Model) prPrompt() string {
	if m.pr == nil {
		return ""
	}
	return fmt.Sprintf("new PR %s: %s → %s · %s opens · tab field · %s create · %s back",
		m.pr.name, m.pr.head, m.pr.baseValue(), m.cfg.KeyFor("pr"), prSubmitKey, "esc")
}
