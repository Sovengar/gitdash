package tui

import (
	"reflect"
	"testing"

	"gitdash/internal/gitstatus"
)

func refs() []gitstatus.Ref {
	return []gitstatus.Ref{
		{Name: "main"},
		{Name: "feature/login"},
		{Name: "dev"},
		{Name: "origin/main", Remote: true},
		{Name: "origin/dev", Remote: true},
	}
}

func TestFilterRefsIsCaseInsensitiveContains(t *testing.T) {
	got := names(filterRefs(refs(), "DEV", false))
	want := []string{"dev", "origin/dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterRefs(DEV) = %v, want %v", got, want)
	}
	if n := len(filterRefs(refs(), "", false)); n != 5 {
		t.Errorf("empty filter kept %d refs, want all 5", n)
	}
	if n := len(filterRefs(refs(), "zzz", false)); n != 0 {
		t.Errorf("a filter with no match kept %d refs, want 0", n)
	}
}

// The head picker offers locals only: origin/feat is not a valid PR head for gh/glab.
func TestFilterRefsLocalOnlyDropsRemotes(t *testing.T) {
	got := names(filterRefs(refs(), "", true))
	want := []string{"main", "feature/login", "dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("local-only = %v, want %v", got, want)
	}
	got = names(filterRefs(refs(), "main", true))
	if !reflect.DeepEqual(got, []string{"main"}) {
		t.Errorf("local-only filter main = %v, want [main]", got)
	}
}

func TestIndexOfRef(t *testing.T) {
	if got := indexOfRef(refs(), "dev"); got != 2 {
		t.Errorf("indexOfRef(dev) = %d, want 2", got)
	}
	if got := indexOfRef(refs(), "typed-by-hand"); got != 0 {
		t.Errorf("indexOfRef(absent) = %d, want 0", got)
	}
}

func TestClampHighlight(t *testing.T) {
	cases := []struct{ n, cursor, want int }{
		{0, 3, 0}, {3, 0, 0}, {3, 3, 2}, {3, -1, 0}, {3, 1, 1},
	}
	for _, c := range cases {
		if got := clampHighlight(c.n, c.cursor); got != c.want {
			t.Errorf("clampHighlight(%d, %d) = %d, want %d", c.n, c.cursor, got, c.want)
		}
	}
}

// The window keeps the cursor visible, drops the tail off the end and reports the "… N more" flag
// only when there really are refs past the window.
func TestPickerWindow(t *testing.T) {
	cases := []struct {
		name               string
		n, cursor, rows    int
		wantStart, wantEnd int
		wantMore           bool
	}{
		{"fits whole", 3, 1, 4, 0, 3, false},
		{"exactly rows", 4, 1, 4, 0, 4, false},
		{"cursor within the window", 6, 1, 4, 0, 3, true},
		{"cursor at the window edge scrolls", 6, 3, 4, 1, 4, true},
		{"cursor scrolls the window", 6, 4, 4, 2, 5, true},
		{"last ref", 6, 5, 4, 3, 6, false},
		{"empty", 0, 0, 4, 0, 0, false},
		{"zero rows", 4, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		start, end, more := pickerWindow(c.n, c.cursor, c.rows)
		if start != c.wantStart || end != c.wantEnd || more != c.wantMore {
			t.Errorf("%s: pickerWindow(%d,%d,%d) = (%d,%d,%v), want (%d,%d,%v)",
				c.name, c.n, c.cursor, c.rows, start, end, more, c.wantStart, c.wantEnd, c.wantMore)
		}
	}
}

func names(rs []gitstatus.Ref) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}
