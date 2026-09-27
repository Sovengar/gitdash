package tmpgate

import "testing"

// TestClamp cubre los limites tambien, no solo el caso dentro de rango.
func TestClamp(t *testing.T) {
	casos := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},   // dentro de rango
		{-1, 0, 10, 0},  // por debajo
		{11, 0, 10, 10}, // por encima
		{0, 0, 10, 0},   // justo en el limite inferior
		{10, 0, 10, 10}, // justo en el limite superior
	}
	for _, c := range casos {
		if got := Clamp(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("Clamp(%d,%d,%d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
}
