// Toasts efímeros: feedback de acciones y errores en overlay (esquina inferior
// derecha). Sustituyen la línea de notificación permanente del dashboard.
package tui

import (
	"strings"
	"time"

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
	toastDuration = 3 * time.Second
	toastMinWidth = 20
	toastMaxWidth = 60
)

// toast es un aviso efímero de una línea con su instante de creación.
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

// show encola un toast; texto vacío se ignora.
func (t *toastManager) show(text string, level toastLevel) {
	if text == "" {
		return
	}
	t.toasts = append(t.toasts, toast{
		text:     text,
		level:    level,
		created:  time.Now(),
		duration: toastDuration,
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

// lines devuelve los toasts renderizados (una línea cada uno), de más antiguo a
// más reciente (el más reciente se apila abajo).
func (t *toastManager) lines() []string {
	out := make([]string, 0, len(t.toasts))
	for _, to := range t.toasts {
		out = append(out, renderToast(to))
	}
	return out
}

// renderToast compone icono + texto con el color del nivel, recortado a su
// ancho adaptativo (clamp) midiendo sin ANSI.
func renderToast(to toast) string {
	line := toastStyle(to.level).Render(toastIcon(to.level) + " " + to.text)
	width := toastWidth(to.text)
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

// toastWidth es el ancho objetivo del toast, clampado entre mínimo y máximo.
func toastWidth(text string) int {
	w := ansi.StringWidth(text) + 4
	if w < toastMinWidth {
		w = toastMinWidth
	}
	if w > toastMaxWidth {
		w = toastMaxWidth
	}
	return w
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

// overlayToasts dibuja las toasts en la esquina inferior derecha de base,
// apilando hacia arriba y reservando las reserved filas inferiores (la sección
// de keybinds) para no taparlas. El splice es ANSI-safe: se recorta por celdas
// con ansi.Truncate/TruncateLeft. Si no caben, prioriza el toast más reciente.
func overlayToasts(base string, toasts []string, width, height, reserved int) string {
	if len(toasts) == 0 || width <= 0 {
		return base
	}
	lines := strings.Split(base, "\n")
	if height <= 0 || height > len(lines) {
		height = len(lines)
	}
	for i := 0; i < len(toasts); i++ {
		block := toasts[len(toasts)-1-i] // el más reciente abajo
		bw := ansi.StringWidth(block)
		x := width - bw - 1
		if x < 0 {
			x = 0
		}
		y := height - reserved - 1 - i
		if y < 0 {
			break // no cabe: se priorizan los toasts más recientes
		}
		if y >= len(lines) {
			continue
		}
		line := lines[y]
		lines[y] = ansi.Truncate(line, x, "") + block + ansi.TruncateLeft(line, x+bw, "")
	}
	return strings.Join(lines, "\n")
}
