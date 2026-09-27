package tmpgate

import "testing"

// TestClamp solo cubre el caso dentro de rango: los mutantes de frontera
// (boundary) sobreviven a proposito, para comprobar que el gate bloquea.
func TestClamp(t *testing.T) {
	if got := Clamp(5, 0, 10); got != 5 {
		t.Fatalf("Clamp(5,0,10) = %d, want 5", got)
	}
}
