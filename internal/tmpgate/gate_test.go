package tmpgate

import "testing"

// The inputs are deliberately off-boundary: they kill the two negations and
// leave the two boundary mutations equivalent on every reachable input, which
// is what makes this fixture produce survivors instead of a clean run.
func TestClamp(t *testing.T) {
	cases := []struct {
		name      string
		v, lo, hi int
		want      int
	}{
		{"inside", 5, 0, 10, 5},
		{"below", -1, 0, 10, 0},
		{"above", 11, 0, 10, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Clamp(c.v, c.lo, c.hi); got != c.want {
				t.Fatalf("Clamp(%d,%d,%d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
			}
		})
	}
}
