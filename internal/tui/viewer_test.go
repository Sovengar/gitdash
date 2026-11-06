package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// The desktop viewer is the fallback when terminal-browser is absent: no error
// may leak to the caller for a page that did open.
func TestOpenImageViewerFallsBackToXdgOpen(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin) // no terminal-browser anywhere

	if err := openImageViewer("/tmp/page.html"); err != nil {
		t.Errorf("openImageViewer = %v, want nil", err)
	}
}

// A terminal-browser that cannot execute is not a missing one: the error has to
// reach the caller instead of being swallowed as a successful open.
func TestOpenImageViewerReportsStartError(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "terminal-browser"), []byte("#!/definitely/not/an/interpreter\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	if err := openImageViewer("/tmp/page.html"); err == nil {
		t.Error("openImageViewer = nil with a binary that cannot start")
	}
}
