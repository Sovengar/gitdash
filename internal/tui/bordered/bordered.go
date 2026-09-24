// Package bordered dibuja cajas con borde redondeado y título embebido en la
// línea superior. El contenido se recorta (ANSI-aware) al ancho interior en
// vez de re-envolverse: el wrap rompería el alto calculado por el layout.
package bordered

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Alineación del título dentro de la línea de borde.
const (
	AlignLeft = iota
	AlignCenter
	AlignRight
)

// Rounded es el borde redondeado (╭ ╮ ╰ ╯) por defecto.
func Rounded() lipgloss.Border { return lipgloss.RoundedBorder() }

// RenderWithTitle dibuja un borde con título arriba alineado a la izquierda y
// sin leyenda inferior.
func RenderWithTitle(border lipgloss.Border, borderFg color.Color, title, content string, width int) string {
	return RenderWithTitles(border, borderFg, title, AlignLeft, "", AlignLeft, content, width)
}

// RenderWithTitles dibuja un borde con título en la línea superior y una
// leyenda opcional en la inferior, cada uno con su alineación. Un título vacío
// deja la línea completa de relleno. El ancho exterior es width (mínimo 2) y el
// interior width-2; cada línea de contenido se recorta al interior.
func RenderWithTitles(border lipgloss.Border, borderFg color.Color, topTitle string, topAlign int, bottomTitle string, bottomAlign int, content string, width int) string {
	if width < 2 {
		width = 2
	}

	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}

	var style *ansi.Style
	if borderFg != nil {
		s := ansi.NewStyle().ForegroundColor(borderFg)
		style = &s
	}

	var b strings.Builder
	b.WriteString(borderLine(style, border.TopLeft, border.Top, border.TopRight, innerWidth, topAlign, topTitle))
	b.WriteString("\n")
	for _, line := range contentLines(style, border.Left, border.Right, content, innerWidth) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(borderLine(style, border.BottomLeft, border.Bottom, border.BottomRight, innerWidth, bottomAlign, bottomTitle))
	return b.String()
}

// borderLine compone una línea de borde con el título embebido y alineado.
func borderLine(style *ansi.Style, left, fill, right string, innerWidth, align int, title string) string {
	if fill == "" {
		fill = " "
	}
	titleWidth := ansi.StringWidth(title)
	if titleWidth > innerWidth {
		title = ansi.Truncate(title, innerWidth, "")
		titleWidth = innerWidth
	}

	remaining := innerWidth - titleWidth
	var leftPad, rightPad int
	switch align {
	case AlignRight:
		leftPad = remaining
	case AlignCenter:
		leftPad = remaining / 2
		rightPad = remaining - leftPad
	default:
		rightPad = remaining
	}

	return styled(style, left) +
		styled(style, strings.Repeat(fill, leftPad)) +
		styled(style, title) +
		styled(style, strings.Repeat(fill, rightPad)) +
		styled(style, right)
}

// contentLines recorta cada línea de contenido al ancho interior y le añade
// los bordes laterales. Contenido vacío produce una línea en blanco interior.
func contentLines(style *ansi.Style, leftChar, rightChar, content string, innerWidth int) []string {
	if leftChar == "" {
		leftChar = " "
	}
	if rightChar == "" {
		rightChar = " "
	}

	raw := strings.Split(content, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if w := ansi.StringWidth(line); w > innerWidth {
			line = ansi.Truncate(line, innerWidth, "")
		}
		if pad := innerWidth - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		lines = append(lines, styled(style, leftChar)+line+styled(style, rightChar))
	}
	if len(lines) == 0 {
		lines = append(lines, styled(style, leftChar)+strings.Repeat(" ", innerWidth)+styled(style, rightChar))
	}
	return lines
}

// styled envuelve el texto en el estilo ANSI del borde (si lo hay).
func styled(style *ansi.Style, s string) string {
	if style == nil {
		return s
	}
	return style.Styled(s)
}
