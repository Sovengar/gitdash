// Package tmpgate is a temporary fixture whose only purpose is to leave
// surviving mutants behind, so the Mutation (diff) required check can be
// exercised against a real run instead of a hope. It is committed and pushed on
// purpose, and it is deleted again before the branch is merged: main must not
// inherit dead code.
//
// Clamp is the shape that works: its two guards each produce a
// CONDITIONALS_NEGATION mutant that the test kills and a
// CONDITIONALS_BOUNDARY mutant that the test cannot, so the run is guaranteed to
// end red with exactly two survivors that are not in .mutation-allowlist.
package tmpgate

// Clamp returns v confined to [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
