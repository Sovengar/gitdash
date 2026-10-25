package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

// An absent marker is the empty content removeTopLevel has to accept: the removal still creates the file.
func TestSetMarkerNameOnAnAbsentMarkerWritesAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := SetMarkerName(dir, ".gitdash.toml", ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 0 {
		t.Errorf("marker = %q, want an empty file", raw)
	}
}
