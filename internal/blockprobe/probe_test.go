package blockprobe

import "testing"

// TestIntentionalFailure exists only to prove that CI blocks a red PR.
// It is deleted together with this probe branch.
func TestIntentionalFailure(t *testing.T) {
	t.Fatal("intentional failure for the CI blocking probe")
}
