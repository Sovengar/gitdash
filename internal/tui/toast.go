package tui

import (
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

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

// A function and not a package const for a measured reason: Go does not instrument constant expressions, so a package-level `3 * time.Second` generates no block and its ARITHMETIC_BASE mutant stays NOT COVERED forever.
func toastDuration() time.Duration { return 3 * time.Second }

type toast struct {
	text     string
	level    toastLevel
	created  time.Time
	duration time.Duration
}

type toastManager struct {
	toasts []toast
}

// Carriage returns are dropped so the text never breaks the render with overlay lines (the `\n` are kept: they make a multi-line toast).
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

func (t *toastManager) update() { t.updateAt(time.Now()) }

// Same cut as gitstatus/tui/table.go:relativeAge and for the same reason: the guard is a `<` on the duration, so the only thing separating `< duration` from `< duration - 1` is an age exactly equal to the duration, which cannot be built with time.Now() inside.
func (t *toastManager) updateAt(now time.Time) {
	active := t.toasts[:0]
	for _, to := range t.toasts {
		if now.Sub(to.created) < to.duration {
			active = append(active, to)
		}
	}
	t.toasts = active
}

func (t *toastManager) blocks() [][]string { return t.blocksFor(0) }

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

func (t *toastManager) lines() []string {
	var out []string
	for _, block := range t.blocks() {
		out = append(out, block...)
	}
	return out
}

func renderToast(to toast, maxWidth int) []string {
	width := toastWidth(to.text, maxWidth)
	wrapped := wrapText(to.text, width-4)
	out := make([]string, 0, len(wrapped))
	for i, seg := range wrapped {
		prefix := "  "
		if i == 0 {
			prefix = toastIcon(to.level) + " "
		}
		line := toastStyle(to.level).Render(prefix + seg)
		line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
		out = append(out, line)
	}
	return out
}

// The two chained clamps are the same as two guards, and the upper one wins if the terminal is narrower than the minimum.
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
			// The inner loop YIELDS PROGRESS by construction and not by a safeguard: splitWidth always returns a non-empty head strictly shorter than w when w does not fit, so `w = tail` cannot stay the same, and that is why no `if head == "" { break }` is needed here.
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
	// Never empty: strings.Split gives at least one paragraph and each paragraph leaves at least one line (the empty ones, explicitly); callers depend on it (renderToast sizes with len(wrapped) and blockWidth with the first line), and the guard that defended it was unreachable for this very reason.
	return lines
}

// CONTRACT: with `maxWidth >= 1` and non-empty s it returns a non-empty head strictly shorter than s, which is what lets wrapText cut without a progress guard; a rune wider than maxWidth is cut anyway to guarantee progress.
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

// The splice is ANSI-safe: it is clipped by cells with ansi.Truncate/TruncateLeft and each block is bounded to the terminal width so nothing overflows; when they do not fit, the most recent toast wins.
func overlayToasts(base string, blocks [][]string, width, height, reserved int) string {
	if len(blocks) == 0 || width <= 0 {
		return base
	}
	lines := strings.Split(base, "\n")
	if height <= 0 || height > len(lines) {
		height = len(lines)
	}
	bottom := height - reserved - 1
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
			// A block that does not fit whole is drawn by its last rows (the end of the message carries the actionable detail); with keep <= 0 there is no free row and the most recent win, which the 0 floor covers without another guard.
			block = block[min(bh, max(0, bh-(bottom+1))):]
			if len(block) == 0 {
				break
			}
			top = 0
		}
		for j, b := range block[:min(len(block), max(0, len(lines)-top))] {
			y := top + j
			lines[y] = ansi.Truncate(lines[y], x, "") + b + ansi.TruncateLeft(lines[y], x+bw, "")
		}
		bottom = top - 2
	}
	return strings.Join(lines, "\n")
}

// Clipping to the line's own width is identity, so the clamp replaces the guard.
func clampBlock(block []string, width int) []string {
	out := make([]string, 0, len(block))
	for _, line := range block {
		out = append(out, ansi.Truncate(line, min(ansi.StringWidth(line), width), ""))
	}
	return out
}

func blockWidth(block []string) int {
	w := 0
	for _, line := range block {
		w = max(w, ansi.StringWidth(line))
	}
	return w
}
