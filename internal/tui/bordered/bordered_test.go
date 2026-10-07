package bordered

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderWithTitleWidthExactAndGlyphs(t *testing.T) {
	out := RenderWithTitle(Rounded(), lipgloss.Color("238"), " gitdash ", "text", 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (border + content + border)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d width = %d, want 20: %q", i, w, l)
		}
	}
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("wrong top corners: %q", top)
	}
	if !strings.Contains(top, " gitdash ") {
		t.Errorf("title not embedded in the top line: %q", top)
	}
	bot := ansi.Strip(lines[2])
	if !strings.HasPrefix(bot, "╰") || !strings.HasSuffix(bot, "╯") {
		t.Errorf("wrong bottom corners: %q", bot)
	}
}

// Without a border color NO escape is emitted at all: the border style only exists if there is a color, and `ansi.Style` with a nil color is not "no style" but a `\x1b[39m` (default fg) around each chunk, so this case tells an unpainted border from a white one.
func TestRenderWithoutColorNotEmitsANSI(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, " title ", "content", 24)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("a colourless border emitted ANSI: %q", out)
	}
	if !strings.Contains(ansi.Strip(out), " title ") {
		t.Errorf("the title was lost by not painting: %q", out)
	}
}

func TestRenderUsesTheFillOfTheBorder(t *testing.T) {
	t.Run("own fill", func(t *testing.T) {
		out := RenderWithTitle(Rounded(), nil, " t ", "c", 12)
		top := ansi.Strip(strings.Split(out, "\n")[0])
		if !strings.Contains(top, "─") {
			t.Errorf("border fill = %q, want the Border's ─ fill", top)
		}
	})
	t.Run("empty fill", func(t *testing.T) {
		b := lipgloss.Border{TopLeft: "|", Top: "", TopRight: "|", BottomLeft: "|", Bottom: "", BottomRight: "|", Left: "!", Right: "!"}
		out := RenderWithTitle(b, nil, "", "c", 10)
		lines := strings.Split(out, "\n")
		if got := ansi.Strip(lines[0]); got != "|        |" {
			t.Errorf("top line = %q, want | + spaces + |", got)
		}
		if got := ansi.Strip(lines[1]); got != "!c       !" {
			t.Errorf("content line = %q, want the Border's side borders", got)
		}
	})
}

func TestRenderWithTitleClipsWithoutWrap(t *testing.T) {
	long := strings.Repeat("x", 100)
	out := RenderWithTitle(Rounded(), nil, "", long, 12)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (clipping adds no lines)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 12 {
			t.Errorf("line %d width = %d, want 12", i, w)
		}
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != 12 {
		t.Errorf("clipped content width = %d, want 12", got)
	}
}

func TestRenderWithTitleClippingANSI(t *testing.T) {
	content := "\x1b[31m" + strings.Repeat("ab", 40) + "\x1b[0m"
	out := RenderWithTitle(Rounded(), nil, "", content, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if w := ansi.StringWidth(lines[1]); w != 10 {
		t.Errorf("ANSI width = %d, want 10: %q", w, lines[1])
	}
	if !strings.Contains(lines[1], "\x1b[") {
		t.Errorf("the content's ANSI was lost: %q", lines[1])
	}
}

func TestRenderWithTitleWidthMinimum(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "long title", "x", 1)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 2 {
			t.Errorf("line %d width = %d, want 2 (clamp)", i, w)
		}
	}
}

func TestRenderWithTitleContentEmpty(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", "", 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[1]); got != "│      │" { // inner width = 8-2
		t.Errorf("empty inner line = %q, want border + inner fill", got)
	}
}

func TestRenderWithTitlesLegendLower(t *testing.T) {
	out := RenderWithTitles(Rounded(), nil, " top ", AlignLeft, " bottom ", AlignRight, "c", 24)
	lines := strings.Split(out, "\n")
	bot := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(bot, " bottom ") {
		t.Errorf("bottom legend missing: %q", bot)
	}
	if !strings.HasSuffix(bot, "╯") {
		t.Errorf("bottom right corner missing: %q", bot)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 24 {
			t.Errorf("line %d width = %d, want 24", i, w)
		}
	}
}

// The ODD gap is the rare case, because there the integer division and the split diverge (with a gap of 3, the centre leaves 1 on the left and 2 on the right: the remainder goes right).
func TestRenderWithTitlesAlignsTheTitle(t *testing.T) {
	// The inner width is width-2: with width 9 that is 7 cells and a 4-cell title leaves a gap of 3, odd on purpose because that is the discriminating case.
	const width = 9
	for _, c := range []struct {
		name       string
		align      int
		wantTop    string
		wantBottom string
	}{
		{"left: the whole gap on the right", AlignLeft, "╭text───╮", "╰───────╯"},
		{"centre: 1 on the left and 2 on the right (odd)", AlignCenter, "╭─text──╮", "╰───────╯"},
		{"right: the whole gap on the left", AlignRight, "╭───text╮", "╰───────╯"},
	} {
		out := RenderWithTitles(Rounded(), nil, "text", c.align, "", c.align, "x", width)
		lines := strings.Split(out, "\n")
		if len(lines) != 3 {
			t.Fatalf("%s: %d lines, want 3 (border + content + border):\n%s",
				c.name, len(lines), out)
		}
		if got := lines[0]; got != c.wantTop {
			t.Errorf("%s: top line = %q, want %q", c.name, got, c.wantTop)
		}
		if got := lines[2]; got != c.wantBottom {
			t.Errorf("%s: bottom line = %q, want %q", c.name, got, c.wantBottom)
		}
	}
}

func TestRenderWithTitlesAlignsEachLineForSeforte(t *testing.T) {
	out := RenderWithTitles(Rounded(), nil, "T", AlignLeft, "B", AlignRight, "x", 9)
	lines := strings.Split(out, "\n")
	if got := lines[0]; got != "╭T──────╮" {
		t.Errorf("top line = %q, want ╭T──────╮", got)
	}
	if got := lines[2]; got != "╰──────B╯" {
		t.Errorf("bottom line = %q, want ╰──────B╯", got)
	}
}

func TestRenderWithTitlesWithoutTitleNotLeavesGap(t *testing.T) {
	for _, align := range []int{AlignLeft, AlignCenter, AlignRight, 99} {
		out := RenderWithTitles(Rounded(), nil, "", align, "", align, "x", 9)
		lines := strings.Split(out, "\n")
		if got := lines[0]; got != "╭───────╮" {
			t.Errorf("align=%d without a title: top line = %q, want ╭───────╮", align, got)
		}
		if got := lines[2]; got != "╰───────╯" {
			t.Errorf("align=%d without a title: bottom line = %q, want ╰───────╯", align, got)
		}
	}
}

// The empty border character is replaced by a space, and the reason is width: with the border at "" the box would draw a line one cell short on that side and would look crooked. The case is forced by calling the function with empty borders, which is what Render does when the style does not bring them.
func TestContentLinesReplacesBordersEmpty(t *testing.T) {
	got := contentLines(nil, "", "", "text", 12)
	if len(got) != 1 {
		t.Fatalf("contentLines returned %d lines, want 1", len(got))
	}
	if want := " text         "; got[0] != want {
		t.Errorf("line = %q, want %q (the empty borders are one space each)", got[0], want)
	}
	if w := ansi.StringWidth(got[0]); w != 14 {
		t.Errorf("width = %d, want 14 (12 of interior + 2 borders)", w)
	}
	got = contentLines(nil, "|", "|", "text", 12)
	if got[0] != "|text        |" {
		t.Errorf("line with borders = %q, want the frame borders", got[0])
	}
}

func TestContentLinesClipsWhatDoesNotFit(t *testing.T) {
	got := contentLines(nil, "|", "|", "too long for 6", 6)
	if w := ansi.StringWidth(got[0]); w != 8 {
		t.Errorf("width = %d, want 8 (6 of interior + 2 borders)", w)
	}
	if strings.Contains(got[0], "for") {
		t.Errorf("the line was not clipped: %q", got[0])
	}
}
