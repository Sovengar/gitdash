package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSpliceModalCentersAndKeepsTheBaseWidth(t *testing.T) {
	src := []string{
		"AAAAAAAAAAAAAAAAAAAA",
		"BBBBBBBBBBBBBBBBBBBB",
		"CCCCCCCCCCCCCCCCCCCC",
		"DDDDDDDDDDDDDDDDDDDD",
		"EEEEEEEEEEEEEEEEEEEE",
		"FFFFFFFFFFFFFFFFFFFF",
	}
	base := strings.Join(src, "\n")
	modal := strings.Join([]string{pad("MMMMMMMMMM", 10), pad("MMMMMMMMMM", 10), pad("MMMMMMMMMM", 10)}, "\n")

	out := spliceModal(base, modal, 20, 6)
	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("lines = %d, want 6", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d width = %d, want 20", i, w)
		}
	}
	// x = (20-10)/2 = 5, y = (6-3)/2 = 1: the modal replaces columns 5..15 on lines 1..3, sides untouched.
	if lines[0] != src[0] {
		t.Errorf("the line above the modal changed: %q", lines[0])
	}
	for i := 1; i <= 3; i++ {
		want := src[i][:5] + strings.Repeat("M", 10) + src[i][15:]
		if got := ansi.Strip(lines[i]); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
	if lines[4] != src[4] {
		t.Errorf("the line below the modal changed: %q", lines[4])
	}
}

// A modal as wide as the terminal replaces the whole line, which is the exact-width case of the bordered box.
func TestSpliceModalExactWidth(t *testing.T) {
	base := strings.Join([]string{pad("ABCDEFGHIJ", 10), pad("KLMNOPQRST", 10), pad("UVWXYZ0123", 10)}, "\n")
	modal := strings.Join([]string{pad("XXXX", 10), pad("YYYY", 10)}, "\n")

	out := spliceModal(base, modal, 10, 3)
	lines := strings.Split(out, "\n")
	if got := ansi.Strip(lines[0]); got != pad("XXXX", 10) {
		t.Errorf("the first modal line = %q, want it to replace the whole width", got)
	}
	if got := ansi.Strip(lines[1]); got != pad("YYYY", 10) {
		t.Errorf("the second modal line = %q, want it to replace the whole width", got)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("line %d width = %d, want 10", i, w)
		}
	}
}

// ANSI in the base is not broken by the splice: the block replaces cells and the styled tail is rebuilt clipped.
func TestSpliceModalANSISafe(t *testing.T) {
	base := "\x1b[31m" + pad("RED", 7) + "\x1b[0m"
	modal := "\x1b[32m" + pad("OK", 4) + "\x1b[0m"
	out := spliceModal(base, modal, 7, 1)
	if w := ansi.StringWidth(out); w != 7 {
		t.Errorf("width = %d, want 7", w)
	}
	if !strings.Contains(ansi.Strip(out), "OK") {
		t.Errorf("the modal is not painted: %q", ansi.Strip(out))
	}
}

func TestSpliceModalNoModalOrNoWidthIsIdentity(t *testing.T) {
	base := "abc\ndef"
	if got := spliceModal(base, "", 10, 2); got != base {
		t.Errorf("with no modal = %q, want the base", got)
	}
	if got := spliceModal(base, "x", 0, 2); got != base {
		t.Errorf("with no width = %q, want the base", got)
	}
	// A non-positive or out-of-range height falls back to the base's own line count.
	if got := spliceModal(base, "x", 3, 0); got != "axc\ndef" {
		t.Errorf("with height 0 = %q", got)
	}
	if got := spliceModal(base, "x", 3, 99); got != "axc\ndef" {
		t.Errorf("with height beyond the base = %q", got)
	}
}

// Height 0 falls back to the base's line count, so the modal is centred over the base and not stuck at the top: the two branches differ in where y lands.
func TestSpliceModalZeroHeightCentersLikeTheBaseHeight(t *testing.T) {
	base := strings.Join([]string{"aaa", "bbb", "ccc", "ddd", "eee", "fff"}, "\n")

	out := spliceModal(base, "x", 3, 0)

	lines := strings.Split(out, "\n")
	if got := ansi.Strip(lines[0]); got != "aaa" {
		t.Errorf("line 0 = %q, want it untouched (height 0 means the base's 6 lines, not 0)", got)
	}
	if got := ansi.Strip(lines[2]); got != "cxc" {
		t.Errorf("line 2 = %q, want the modal centred (y = (6-1)/2 = 2)", got)
	}
}

// A modal taller than the terminal is clipped to its top rows, never panics.
func TestSpliceModalTallerThanBase(t *testing.T) {
	base := "abc\ndef"
	modal := "1\n2\n3\n4\n5"
	out := spliceModal(base, modal, 3, 2)
	if got := len(strings.Split(out, "\n")); got != 2 {
		t.Errorf("lines = %d, want 2", got)
	}
}
