// Package tmpgate existe solo para verificar el gate de mutation testing.
package tmpgate

// Clamp limita v al rango [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
