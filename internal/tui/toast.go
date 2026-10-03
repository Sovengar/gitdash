// Toasts efímeros: feedback de acciones y errores en overlay (esquina inferior
// derecha). Sustituyen la línea de notificación permanente del dashboard.
package tui

import (
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// toastLevel clasifica el toast: color e icono.
type toastLevel int

const (
	toastSuccess toastLevel = iota
	toastError
	toastInfo
	toastWarning
)

const (
	toastMinWidth = 20
	toastMaxWidth = 60
)

// toastDuration es el plazo de un aviso. Va en una FUNCION y no en una const de
// paquete por una razon medida: Go no instrumenta las expresiones de constante,
// asi que un `3 * time.Second` a nivel de paquete no genera bloque y su mutante
// de ARITHMETIC_BASE sale NOT COVERED para siempre, con o sin tests. Dentro de
// una funcion si se instrumenta, y el mutante pasa a ser ejecutable.
func toastDuration() time.Duration { return 3 * time.Second }

// toast es un aviso efímero (posiblemente de varias líneas) con su instante de
// creación.
type toast struct {
	text     string
	level    toastLevel
	created  time.Time
	duration time.Duration
}

// toastManager mantiene la cola de toasts vivos y los expira por tiempo.
type toastManager struct {
	toasts []toast
}

// show encola un toast; texto vacío se ignora. Se descartan los retornos de
// carro para que el texto nunca rompa el render por líneas del overlay (los
// saltos `\n` sí se conservan: producen un toast de varias líneas).
func (t *toastManager) show(text string, level toastLevel) {
	if text == "" {
		return
	}
	t.toasts = append(t.toasts, toast{
		text:     strings.ReplaceAll(text, "\r", ""),
		level:    level,
		created:  time.Now(),
		duration: toastDuration(),
	})
}

func (t *toastManager) showSuccess(text string) { t.show(text, toastSuccess) }
func (t *toastManager) showError(text string)   { t.show(text, toastError) }
func (t *toastManager) showInfo(text string)    { t.show(text, toastInfo) }
func (t *toastManager) showWarning(text string) { t.show(text, toastWarning) }

// update descarta los toasts expirados (se llama en cada tick de 1s).
func (t *toastManager) update() {
	now := time.Now()
	active := t.toasts[:0]
	for _, to := range t.toasts {
		if now.Sub(to.created) < to.duration {
			active = append(active, to)
		}
	}
	t.toasts = active
}

// blocks devuelve un bloque de líneas por toast al ancho máximo del toast (no
// conoce la terminal): de más antiguo a más reciente, el más reciente se apila
// abajo.
func (t *toastManager) blocks() [][]string { return t.blocksFor(0) }

// blocksFor envuelve cada toast al ancho disponible de la terminal, de modo
// que en terminales estrechas el mensaje se re-envuelve a más líneas en vez de
// truncarse. termWidth <= 0 usa el ancho máximo del toast.
func (t *toastManager) blocksFor(termWidth int) [][]string {
	maxWidth := toastMaxWidth
	if termWidth > 0 {
		maxWidth = max(1, min(toastMaxWidth, termWidth-1))
	}
	out := make([][]string, 0, len(t.toasts))
	for _, to := range t.toasts {
		out = append(out, renderToast(to, maxWidth))
	}
	return out
}

// lines aplana los bloques en líneas (para tests y medición).
func (t *toastManager) lines() []string {
	var out []string
	for _, block := range t.blocks() {
		out = append(out, block...)
	}
	return out
}

// renderToast compone el toast como caja de ancho adaptativo (clamp) y hace
// word-wrap del mensaje, de modo que el texto completo (p.ej. el hint
// accionable de un fallo) quede visible en varias líneas. maxWidth es el ancho
// máximo disponible (terminal); cada línea se rellena al ancho del toast
// midiendo sin ANSI.
func renderToast(to toast, maxWidth int) []string {
	width := toastWidth(to.text, maxWidth)
	wrapped := wrapText(to.text, width-4)
	out := make([]string, 0, len(wrapped))
	for i, seg := range wrapped {
		prefix := "  " // continuación alineada bajo el icono
		if i == 0 {
			prefix = toastIcon(to.level) + " "
		}
		line := toastStyle(to.level).Render(prefix + seg)
		line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
		out = append(out, line)
	}
	return out
}

// toastWidth es el ancho objetivo del toast: el del texto + 4 (icono, hueco y
// margen), acotado por abajo a toastMinWidth y por arriba a toastMaxWidth y al
// ancho disponible. Los dos clamps encadenados son los mismos que dos guardas,
// y el de arriba gana si el terminal es más estrecho que el mínimo.
func toastWidth(text string, maxWidth int) int {
	return min(max(ansi.StringWidth(text)+4, toastMinWidth), maxWidth)
}

func toastIcon(level toastLevel) string {
	switch level {
	case toastSuccess:
		return "✓"
	case toastError:
		return "✗"
	case toastWarning:
		return "⚠"
	default:
		return "ℹ"
	}
}

func toastStyle(level toastLevel) lipgloss.Style {
	switch level {
	case toastSuccess:
		return styleToastSuccess
	case toastError:
		return styleToastError
	case toastWarning:
		return styleToastWarning
	default:
		return styleToastInfo
	}
}

// wrapText parte el texto por palabras en líneas de a lo sumo maxWidth celdas;
// una palabra más ancha que maxWidth se corta duro para no desbordar.
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := ""
		for _, w := range words {
			// El bucle inner CEDE PROGRESO por construcción, no por una
			// salvaguarda: `splitWidth` devuelve siempre un `head` no vacío y
			// estrictamente más corto que `w` cuando `w` no cabe, así que
			// `w = tail` no puede quedarse igual. Ese es el contrato que
			// garantiza el contrato de splitWidth, y por eso aquí no hace
			// falta un `if head == "" { break }` que ningún test podía matar.
			for ansi.StringWidth(w) > maxWidth {
				head, tail := splitWidth(w, maxWidth)
				if cur != "" {
					lines = append(lines, cur)
					cur = ""
				}
				lines = append(lines, head)
				w = tail
			}
			if cur == "" {
				cur = w
			} else if ansi.StringWidth(cur)+1+ansi.StringWidth(w) <= maxWidth {
				cur += " " + w
			} else {
				lines = append(lines, cur)
				cur = w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	// Nunca se devuelve vacío: `strings.Split` da al menos un párrafo y cada
	// párrafo deja al menos una línea (los vacíos, explícitamente). Los
	// callers dependen de eso —`renderToast` dimensiona con len(wrapped) y
	// `blockWidth` con la primera línea—, y la guarda que lo defendía
	// (`if len(lines) == 0`) era inalcanzable por esta misma razón.
	return lines
}

// splitWidth corta s en el mayor prefijo que cabe en maxWidth celdas. Si la
// primera runa ya es más ancha que maxWidth se corta igualmente (una runa),
// para garantizar progreso y no entrar en bucle.
//
// CONTRATO: si `maxWidth >= 1` y `s` no está vacía, devuelve un `head` NO vacío
// y estrictamente más corto que `s`. Es lo que permite al bucle de wrapText
// cortar sin guarda de progreso, y hay un test que lo reconstruye entero.
func splitWidth(s string, maxWidth int) (string, string) {
	width := 0
	for i, r := range s {
		rw := ansi.StringWidth(string(r))
		if width+rw > maxWidth {
			if i == 0 {
				_, size := utf8.DecodeRuneInString(s)
				return s[:size], s[size:]
			}
			return s[:i], s[i:]
		}
		width += rw
	}
	return s, ""
}

// overlayToasts dibuja los toasts en la esquina inferior derecha de base,
// apilando hacia arriba y reservando las reserved filas inferiores (la sección
// de keybinds) para no taparlas. El splice es ANSI-safe: se recorta por celdas
// con ansi.Truncate/TruncateLeft y cada bloque se acota al ancho de terminal
// para no desbordar. Si no caben, prioriza el toast más reciente.
func overlayToasts(base string, blocks [][]string, width, height, reserved int) string {
	if len(blocks) == 0 || width <= 0 {
		return base
	}
	lines := strings.Split(base, "\n")
	if height <= 0 || height > len(lines) {
		height = len(lines)
	}
	bottom := height - reserved - 1 // última fila disponible para el más nuevo
	for i := 0; i < len(blocks); i++ {
		block := clampBlock(blocks[len(blocks)-1-i], width)
		bh := len(block)
		if bh == 0 {
			continue
		}
		bw := blockWidth(block)
		x := max(0, width-bw-1)
		top := bottom - bh + 1
		if top < 0 {
			// El bloque no cabe entero: se dibujan sus últimas filas (el
			// cierre del mensaje, que suele llevar el detalle accionable) en
			// lugar de descartarlo.
			// Con keep <= 0 no queda ninguna fila libre: se priorizan los más
			// recientes. El recorte con suelo en 0 cubre ese caso (deja el
			// bloque vacío) sin una guarda más.
			block = block[min(bh, max(0, bh-(bottom+1))):]
			if len(block) == 0 {
				break
			}
			top = 0
		}
		// Solo se pintan las filas que caen dentro de la vista: un bloque
		// solapado por abajo se recorta en vez de salirse del splice.
		for j, b := range block[:min(len(block), max(0, len(lines)-top))] {
			y := top + j
			lines[y] = ansi.Truncate(lines[y], x, "") + b + ansi.TruncateLeft(lines[y], x+bw, "")
		}
		bottom = top - 2 // una fila en blanco entre toasts apilados
	}
	return strings.Join(lines, "\n")
}

// clampBlock acota cada línea del bloque al ancho de terminal. Recortar a un
// ancho igual al suyo es identidad, así que el clamp sustituye a la guarda.
func clampBlock(block []string, width int) []string {
	out := make([]string, 0, len(block))
	for _, line := range block {
		out = append(out, ansi.Truncate(line, min(ansi.StringWidth(line), width), ""))
	}
	return out
}

// blockWidth es el ancho visible máximo de un bloque.
func blockWidth(block []string) int {
	w := 0
	for _, line := range block {
		w = max(w, ansi.StringWidth(line))
	}
	return w
}
