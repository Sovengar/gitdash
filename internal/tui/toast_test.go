package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestToastShowAndExpires(t *testing.T) {
	var tm toastManager
	tm.showSuccess("pull ok")
	if len(tm.toasts) != 1 {
		t.Fatalf("toasts = %d, want 1", len(tm.toasts))
	}
	tm.toasts[0].created = time.Now().Add(-toastDuration() - time.Second)
	tm.update()
	if len(tm.toasts) != 0 {
		t.Errorf("the toast did not expire: %d alive", len(tm.toasts))
	}
}

func TestToastStack(t *testing.T) {
	var tm toastManager
	tm.showInfo("a")
	tm.showWarning("b")
	tm.showError("c")
	if got := len(tm.blocks()); got != 3 {
		t.Fatalf("blocks = %d, want 3 stacked", got)
	}
	if got := len(tm.lines()); got != 3 {
		t.Fatalf("lines = %d, want 3 (one per short toast)", got)
	}
}

func TestToastWrapShowsHintComplete(t *testing.T) {
	msg := "pull failed diverged-node: fatal: Not possible to fast-forward — diverged? pull --rebase manual"
	var tm toastManager
	tm.showError(msg)

	block := tm.blocks()[0]
	if len(block) < 2 {
		t.Fatalf("expected word-wrap across several lines, got %d", len(block))
	}
	flat := collapse(strings.Join(block, "\n"))
	if !strings.Contains(flat, "pull --rebase manual") {
		t.Errorf("the actionable hint is not visible:\n%s", flat)
	}
	if !strings.Contains(flat, "Not possible to fast-forward") {
		t.Errorf("the reason was lost:\n%s", flat)
	}
	for i, l := range block {
		if w := ansi.StringWidth(l); w > toastMaxWidth {
			t.Errorf("line %d width = %d, want <= %d", i, w, toastMaxWidth)
		}
	}
}

func TestToastWordLongNotOverflows(t *testing.T) {
	var tm toastManager
	tm.showError(strings.Repeat("x", 500))
	for i, l := range tm.lines() {
		if w := ansi.StringWidth(l); w > toastMaxWidth {
			t.Errorf("line %d width = %d, want <= %d", i, w, toastMaxWidth)
		}
	}
	if got := ansi.Strip(strings.Join(tm.lines(), "")); strings.Contains(got, "…") {
		t.Errorf("the wrap must not truncate with ellipsis: %q", got)
	}
}

func TestToastWidthClamped(t *testing.T) {
	var tm toastManager
	tm.showError(strings.Repeat("x", 500))
	line := tm.lines()[0]
	if w := ansi.StringWidth(line); w > toastMaxWidth {
		t.Errorf("width = %d, want <= %d", w, toastMaxWidth)
	}
	var tm2 toastManager
	tm2.showInfo("ok")
	if w := ansi.StringWidth(tm2.lines()[0]); w < toastMinWidth {
		t.Errorf("width = %d, want >= %d", w, toastMinWidth)
	}
}

func TestToastIcons(t *testing.T) {
	cases := map[toastLevel]string{
		toastSuccess: "✓", toastError: "✗", toastInfo: "ℹ", toastWarning: "⚠",
	}
	for level, icon := range cases {
		if got := toastIcon(level); got != icon {
			t.Errorf("icon(%d) = %q, want %q", level, got, icon)
		}
	}
}

func TestOverlayANSISafe(t *testing.T) {
	base := strings.Join([]string{
		pad("line0", 40),
		pad("line1", 40),
		"\x1b[31m" + pad("line2", 38) + "ZZ" + "\x1b[0m",
		pad("line3", 40),
	}, "\n")
	out := overlayToasts(base, [][]string{{"\x1b[32mOK\x1b[0m"}}, 40, 4, 1)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 40 {
			t.Errorf("line %d width = %d, want 40", i, w)
		}
	}
	spliced := lines[2]
	flat := ansi.Strip(spliced)
	if !strings.Contains(flat, "OK") {
		t.Errorf("toast missing at the bottom right corner: %q", flat)
	}
	if !strings.Contains(flat, "line2") {
		t.Errorf("the left text was lost: %q", flat)
	}
	if !strings.HasSuffix(flat, "Z") {
		t.Errorf("the text right of the toast was not preserved: %q", flat)
	}
	if !strings.Contains(spliced, "\x1b[31m") {
		t.Errorf("the colour of the line below was lost: %q", spliced)
	}
	if !strings.Contains(spliced, "\x1b[32m") {
		t.Errorf("the toast's colour was lost: %q", spliced)
	}
}

func TestOverlayClampsWidthTerminal(t *testing.T) {
	base := strings.Join(sliceOf(pad("content", 20), 20), "\n")
	var tm toastManager
	tm.showError(strings.Repeat("word ", 30)) // long message -> 60-cell toast
	blocks := tm.blocks()
	if blockWidth(blocks[0]) <= 20 {
		t.Fatalf("precondition: the toast should exceed the terminal width")
	}
	out := overlayToasts(base, blocks, 20, 20, 0)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 20 {
			t.Errorf("line %d width = %d > 20: %q", i, w, ansi.Strip(l))
		}
	}
	if !strings.Contains(ansi.Strip(out), "word") {
		t.Errorf("the toast was not drawn:\n%s", ansi.Strip(out))
	}
}

func TestOverlayReservesKeybinds(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2", "k0", "k1"}, "\n")
	out := overlayToasts(base, [][]string{{"TOAST"}}, 20, 5, 2)
	lines := strings.Split(out, "\n")
	if lines[3] != "k0" || lines[4] != "k1" {
		t.Errorf("the keybinds section was covered: %q", lines[3:])
	}
	if !strings.Contains(ansi.Strip(lines[2]), "TOAST") {
		t.Errorf("the toast did not stack on the reserved rows: %q", ansi.Strip(lines[2]))
	}
}

func TestOverlayWithoutSpacePrioritisesRecent(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2"}, "\n")
	out := overlayToasts(base, [][]string{{"old"}, {"new"}}, 20, 3, 2)
	lines := strings.Split(out, "\n")
	flat := ansi.Strip(lines[0])
	if !strings.Contains(flat, "new") {
		t.Errorf("the more recent toast is not shown: %q", flat)
	}
	if strings.Contains(stripANSI(out), "old") {
		t.Errorf("the old toast should not fit:\n%s", out)
	}
}

func TestOverlayToastMultiline(t *testing.T) {
	var tm toastManager
	tm.showError("pull failed node: fatal: Not possible to fast-forward — diverged? pull --rebase manual")
	out := overlayToasts(strings.Join(sliceOf(pad("x", 60), 8), "\n"), tm.blocks(), 60, 8, 5)
	lines := strings.Split(out, "\n")
	if len(lines) != 8 {
		t.Fatalf("lines = %d, want 8", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 60 {
			t.Errorf("line %d width = %d > 60", i, w)
		}
	}
	if !strings.Contains(collapse(strings.Join(lines, "\n")), "pull --rebase manual") {
		t.Errorf("the multiline toast was not drawn whole:\n%s", strings.Join(lines, "\n"))
	}
}

func TestToastReWrapsInWidthNarrow(t *testing.T) {
	var tm toastManager
	tm.showError("pull failed diverged-node: fatal: Not possible to fast-forward — diverged? pull --rebase manual")
	blocks := tm.blocksFor(40)
	for i, l := range blocks[0] {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line %d width = %d > 40", i, w)
		}
	}
	flat := collapse(strings.Join(blocks[0], "\n"))
	if !strings.Contains(flat, "pull --rebase manual") {
		t.Errorf("the actionable hint was lost when re-wrapping:\n%s", flat)
	}
}

func TestOverlayBlockTallerThanTheGap(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2"}, "\n")
	block := []string{"t0", "t1", "t2", "t3", "t4"}
	out := overlayToasts(base, [][]string{block}, 20, 3, 0)
	lines := strings.Split(out, "\n")
	flat := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(flat, "t4") {
		t.Errorf("the toast's closing line was not drawn:\n%s", flat)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 20 {
			t.Errorf("line %d width = %d > 20", i, w)
		}
	}
}

func TestWrapTextWidthLessThanTheRune(t *testing.T) {
	lines := wrapText("日本語", 1)
	if got := collapse(strings.Join(lines, "")); got != "日本語" {
		t.Errorf("text was lost: %q", got)
	}
}

func TestToastWithBreaksOfLineNotBreaksSplice(t *testing.T) {
	var tm toastManager
	tm.showError("fatal: first\r\nsecond line")
	blocks := tm.blocksFor(60)
	if len(blocks[0]) != 2 {
		t.Fatalf("lines = %d, want 2 (one per break)", len(blocks[0]))
	}
	out := overlayToasts(strings.Join(sliceOf(pad("x", 60), 6), "\n"), blocks, 60, 6, 2)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("line %d width = %d, want 60", i, w)
		}
	}
	flat := collapse(out)
	if !strings.Contains(flat, "first") || !strings.Contains(flat, "second") {
		t.Errorf("the multiline message text was lost:\n%s", flat)
	}
}

func TestToastDurationZeroExpiresAlready(t *testing.T) {
	var tm toastManager
	tm.showInfo("live")
	tm.toasts = append(tm.toasts, toast{text: "instant", level: toastInfo, created: time.Now(), duration: 0})
	tm.toasts = append(tm.toasts, toast{text: "expired", level: toastInfo, created: time.Now().Add(-time.Minute), duration: -time.Second})

	tm.update()
	for _, to := range tm.toasts {
		if to.duration <= 0 {
			t.Errorf("a toast with duration %v survived the update: %q", to.duration, to.text)
		}
	}
	if len(tm.toasts) != 1 {
		t.Fatalf("alive = %d, want 1 (only the one with a normal duration)", len(tm.toasts))
	}
}

// The +4 is the contract: computed the other way round, a 30-cell message would give a 26-cell toast and the text would overflow the box.
func TestToastWidthIsTextMoreChrome(t *testing.T) {
	for _, c := range []struct {
		name     string
		text     string
		maxWidth int
		want     int
	}{
		{"short text, at the floor", "ok", toastMaxWidth, toastMinWidth},
		{"half text, chrome included", strings.Repeat("x", 30), toastMaxWidth, 34},
		{"long text, at the ceiling", strings.Repeat("x", 200), toastMaxWidth, toastMaxWidth},
		{"narrow terminal", strings.Repeat("x", 200), 20, 20},
		{"terminal narrower than the floor", "ok", 10, 10},
		{"no width", "ok", 0, 0},
	} {
		if got := toastWidth(c.text, c.maxWidth); got != c.want {
			t.Errorf("%s: toastWidth(%d chars, max %d) = %d, want %d",
				c.name, ansi.StringWidth(c.text), c.maxWidth, got, c.want)
		}
	}
}

func TestWrapTextWithoutWidthReturnsTheTextInteger(t *testing.T) {
	for _, w := range []int{0, -1, -100} {
		got := wrapText("text world whole", w)
		if len(got) != 1 || got[0] != "text world whole" {
			t.Errorf("wrapText(_, %d) = %q, want the whole text on one line", w, got)
		}
	}
}

func TestWrapTextSplitsOnlyTheWordThatDoesNotFitFairly(t *testing.T) {
	for _, c := range []struct {
		name, text string
		w          int
		want       []string
	}{
		{"one word of exactly the width", "text", 4, []string{"text"}},
		{"two words that just fit", "a b", 3, []string{"a b"}},
		{"the two words fill the exact width", "text world", 10, []string{"text world"}},
		{"one more of those that fit", "a b", 2, []string{"a", "b"}},
		{"the second does not fit even split into words", "text world", 4, []string{"text", "worl", "d"}},
	} {
		got := wrapText(c.text, c.w)
		if !equalStrings(got, c.want) {
			t.Errorf("%s: wrapText(%q, %d) = %q, want %q", c.name, c.text, c.w, got, c.want)
		}
	}
}

func TestWrapTextClosesTheLineBeforeOfSplitTheLong(t *testing.T) {
	longStr := strings.Repeat("z", 25)
	got := wrapText("clips "+longStr, 10)
	flat := collapse(strings.Join(got, " "))
	if !strings.Contains(flat, "clips") {
		t.Errorf("the short word was lost while splitting the long one: %q", got)
	}
	if !strings.Contains(flat, "zzz") {
		t.Errorf("the long word was not split: %q", got)
	}
	for i, l := range got {
		if w := ansi.StringWidth(l); w > 10 {
			t.Errorf("line %d width = %d > 10: %q", i, w, l)
		}
	}
	if !equalStrings(got[:1], []string{"clips"}) {
		t.Errorf("first line = %q, want only the short word", got[0])
	}
}

func TestSplitWidth(t *testing.T) {
	for _, c := range []struct {
		name, in string
		w        int
		wantHead string
		wantTail string
	}{
		{"fits whole", "abc", 3, "abc", ""},
		{"it fits with room to spare", "abc", 5, "abc", ""},
		{"it clips at the end", "abcd", 2, "ab", "cd"},
		{"wide rune with room 1", "日本", 3, "日", "本"},
		{"a rune wider than the width", "日", 1, "日", ""},
		{"width 0, one rune", "ab", 0, "a", "b"},
		{"empty", "", 3, "", ""},
	} {
		head, tail := splitWidth(c.in, c.w)
		if head != c.wantHead || tail != c.wantTail {
			t.Errorf("%s: splitWidth(%q, %d) = (%q, %q), want (%q, %q)", c.name, c.in, c.w, head, tail, c.wantHead, c.wantTail)
		}
	}
	for _, s := range []string{"日本語", "abcdef", "a b c", "x"} {
		for w := 0; w <= 6; w++ {
			head, tail := splitWidth(s, w)
			if head+tail != s {
				t.Errorf("splitWidth(%q, %d) does not rebuild the original: %q + %q", s, w, head, tail)
			}
		}
	}
}

// A block that fits exactly from row 0 is drawn whole: if the top clipping were measured with `> ` instead of `< `, a toast filling the space would be left with only its last line.
func TestOverlayBlockThatFitsIsClippedFromAbove(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2", "l3"}, "\n")
	block := []string{"t0", "t1", "t2"} // 3 block rows and 3 free ones: top == 0 exactly
	flat := stripANSI(overlayToasts(base, [][]string{block}, 20, 4, 1))
	for _, want := range []string{"t0", "t1", "t2"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the block that just fit was not painted whole, %q is missing:\n%s", want, flat)
		}
	}
	outPoco := stripANSI(overlayToasts(base, [][]string{block}, 20, 4, 2))
	if !strings.Contains(outPoco, "t2") {
		t.Errorf("with a gap of 2 rows the block's closing line should have been left:\n%s", outPoco)
	}
	if strings.Contains(outPoco, "t0") {
		t.Errorf("with a gap of 2 rows the block's beginning did not fit:\n%s", outPoco)
	}
}

func TestOverlayHeightZeroUsesTheViewWhole(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2"}, "\n")
	for _, h := range []int{0, -1} {
		out := stripANSI(overlayToasts(base, [][]string{{"TOAST"}}, 20, h, 0))
		if !strings.Contains(out, "TOAST") {
			t.Errorf("height=%d: the toast was not drawn on the last row:\n%s", h, out)
		}
	}
}

// A negative reserved (the layout never gives one, but the function does not restrict it) has to clip the block instead of writing below the end of the view.
func TestOverlayWithReservedNegativeNotIsExitsOfTheView(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2"}, "\n")
	for _, reserved := range []int{-1, -5} {
		out := overlayToasts(base, [][]string{{"t0", "t1"}}, 20, 3, reserved)
		if lines := strings.Split(out, "\n"); len(lines) != 3 {
			t.Errorf("reserved=%d: the view changed height: %d lines", reserved, len(lines))
		}
	}
}

func sliceOf(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func collapse(s string) string {
	return strings.Join(strings.Fields(ansi.Strip(s)), " ")
}

func TestOverlayNoop(t *testing.T) {
	if got := overlayToasts("x", nil, 10, 5, 0); got != "x" {
		t.Errorf("no toasts = %q, want x", got)
	}
	if got := overlayToasts("x", [][]string{{"t"}}, 0, 5, 0); got != "x" {
		t.Errorf("no width = %q, want x", got)
	}
}

// A warning with no text paints a box with a lone icon and nothing inside, which is worse than not warning; the `\r` are dropped and the `\n` are not, because a multi-line toast is legitimate.
func TestToastEmptyNotIsQueues(t *testing.T) {
	var tm toastManager
	tm.show("", toastInfo)
	if len(tm.toasts) != 0 {
		t.Fatalf("toasts = %d, want 0 (a notice with no text is not painted)", len(tm.toasts))
		// A text carrying only \r IS queued and comes out empty on purpose: the guard looks at the text BEFORE stripping the \r, which is what keeps show cheap, and changing that order would make show clean it twice.
		tm.show("\r", toastInfo)
		if len(tm.toasts) != 1 {
			t.Errorf("toasts = %d after a text of only \r, want 1 (the guard looks before cleaning)", len(tm.toasts))
		}
		tm.toasts = nil
	}
	tm.show("hello", toastInfo)
	if len(tm.toasts) != 1 {
		t.Errorf("toasts = %d, want 1", len(tm.toasts))
	}
}

// An EMPTY paragraph is an empty line in the result and not a gap: a three-line warning with one blank must take three, because if it were dropped the box would measure one line less.
func TestWrapTextKeepsParagraphsEmpty(t *testing.T) {
	got := wrapText("one\n\nthree", 40)
	want := []string{"one", "", "three"}
	if len(got) != len(want) {
		t.Fatalf("wrapText = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// What gets painted needs at least one row, and a block of height 0 is skipped by the stacking.
func TestWrapTextEmptyReturnsALine(t *testing.T) {
	if got := wrapText("   ", 40); len(got) != 1 {
		t.Errorf("wrapText(%q) = %q, want one line", "   ", got)
	}
}

// A block clipped to zero lines is not drawn but must not abort the stacking: without the `continue` an empty block (a message left with no width in a 1-cell terminal) would eat the last toast's row and the message would be lost.
func TestOverlayBlockEmptyNotIsEatsTheRow(t *testing.T) {
	base := strings.Join([]string{"one", "two", "three"}, "\n")
	got := overlayToasts(base, [][]string{{}, {"more"}, {}}, 40, 3, 0)
	if !strings.Contains(got, "more") {
		t.Errorf("the empty block ate the row of the one that did fit:\n%s", got)
	}
	if strings.Count(got, "\n")+1 != 3 {
		t.Errorf("the base changed height:\n%q", got)
	}
}

// The guard is `< to.duration`, so a toast whose age equals its duration is already expired and one 1ns short of it is still alive; no other pair of values tells the two guards apart, and with the clock inside update that age could not be built at all.
func TestToastExpiresInTheInstantExact(t *testing.T) {
	const d = 3 * time.Second
	now := time.Now()
	for _, c := range []struct {
		name  string
		age   time.Duration
		alive bool
	}{
		{"1ns before expiring", d - time.Nanosecond, true},
		{"right as it expired", d, false},
		{"after expiring", d + time.Nanosecond, false},
	} {
		var tm toastManager
		tm.show("hello", toastInfo)
		tm.toasts[0].duration = d
		tm.toasts[0].created = now.Add(-c.age)

		tm.updateAt(now)

		if got := len(tm.toasts) == 1; got != c.alive {
			t.Errorf("%s: updateAt leaves %d toasts, want alive=%v", c.name, len(tm.toasts), c.alive)
		}
	}
}

func TestToastUpdateUsesTheClockReal(t *testing.T) {
	var tm toastManager
	tm.show("hello", toastInfo)
	tm.toasts[0].duration = time.Millisecond
	tm.toasts[0].created = time.Now().Add(-time.Hour) // long expired

	tm.update()

	if len(tm.toasts) != 0 {
		t.Errorf("update left %d toasts, want 0 (the expired one has to go)", len(tm.toasts))
	}
}

// The row pin is the point: with the block of 2 on a canvas of 8 it lands on rows 3-4 (top = (8-2)/2), and a top computed with the sign flipped would paint it two rows lower.
func TestOverlayCenteredSplicesAtTheMiddleRows(t *testing.T) {
	base := strings.Join([]string{"l0", "l1", "l2", "l3", "l4", "l5", "l6", "l7"}, "\n")

	got := strings.Split(overlayCentered(base, []string{"BOX", "two"}, 20, 8), "\n")

	if len(got) != 8 {
		t.Fatalf("the canvas changed height: %d rows", len(got))
	}
	if got[3] != "l3BOX" || got[4] != "l4two" {
		t.Errorf("rows 3-4 = %q / %q, want the block spliced there", got[3], got[4])
	}
	if got[0] != "l0" || got[7] != "l7" {
		t.Errorf("the rows outside the block changed: %q / %q", got[0], got[7])
	}
}

// The column pin is the point: with a 20-wide canvas and a 3-wide block the box starts at cell 8 (x = (20-3)/2) and the tail keeps the base's cells 12-20, so a centering with the sign flipped, the division moved or a tail one cell off cannot pass.
func TestOverlayCenteredPlacesTheBlockAtTheMiddleColumns(t *testing.T) {
	base := strings.Join([]string{
		strings.Repeat(".", 20),
		strings.Repeat(".", 20),
		strings.Repeat(".", 20),
		strings.Repeat(".", 20),
		strings.Repeat(".", 20),
	}, "\n")

	got := strings.Split(overlayCentered(base, []string{"BOX"}, 20, 5), "\n")

	want := strings.Repeat(".", 8) + "BOX" + strings.Repeat(".", 9)
	if got[2] != want {
		t.Errorf("row 2 = %q, want the block at the middle columns %q", got[2], want)
	}
}

// Every degenerate input is identity: an empty block iterates zero rows and a canvas of zero clips the count to zero, so nothing can be painted where there is no room.
func TestOverlayCenteredDegenerateIsIdentity(t *testing.T) {
	base := "l0\nl1\nl2"
	block := []string{"BOX"}
	for _, c := range []struct {
		name string
		w, h int
		blk  []string
	}{
		{"no block", 40, 3, nil},
		{"no width", 0, 3, block},
		{"no height", 40, 0, block},
		{"negative height", 40, -1, block},
	} {
		if got := overlayCentered(base, c.blk, c.w, c.h); got != base {
			t.Errorf("%s changed the canvas: %q", c.name, got)
		}
	}
}
