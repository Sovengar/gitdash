// Package bordered draws rounded boxes with a title embedded in the top line. The content is clipped (ANSI-aware) to the inner width instead of being re-wrapped: wrapping would break the height the layout computed.
package bordered

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	AlignLeft = iota
	AlignCenter
	AlignRight
)

func Rounded() lipgloss.Border { return lipgloss.RoundedBorder() }

func RenderWithTitle(border lipgloss.Border, borderFg color.Color, title, content string, width int) string {
	return RenderWithTitles(border, borderFg, title, AlignLeft, "", AlignLeft, content, width)
}

func RenderWithTitles(border lipgloss.Border, borderFg color.Color, topTitle string, topAlign int, bottomTitle string, bottomAlign int, content string, width int) string {
	width = max(width, 2)

	innerWidth := width - 2

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

func borderLine(style *ansi.Style, left, fill, right string, innerWidth, align int, title string) string {
	if fill == "" {
		fill = " "
	}
	titleWidth := ansi.StringWidth(title)
	// Clipping to a width equal to or larger than the current one is identity, so the clamp replaces the guard: a long title is cut, a short one is left alone (and gets no tail appended).
	titleWidth = min(titleWidth, innerWidth)
	title = ansi.Truncate(title, titleWidth, "")

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
		// Clipping to a width equal to or larger than the current one is identity and `cut` never exceeds the line width after clipping, so the padding is always >= 0: one pair of operations instead of two guards differing only on the exact edge.
		cut := min(ansi.StringWidth(line), innerWidth)
		line = ansi.Truncate(line, cut, "") + strings.Repeat(" ", innerWidth-cut)
		lines = append(lines, styled(style, leftChar)+line+styled(style, rightChar))
	}
	return lines
}

func styled(style *ansi.Style, s string) string {
	if style == nil {
		return s
	}
	return style.Styled(s)
}
