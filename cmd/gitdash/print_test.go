// Tests del modo --print (0004 R29; 0006 R38): no incorpora sub-filas de
// worktree ni glyphs de expansión.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/config"
	"gitdash/internal/testutil"
)

// S38.1: `gitdash --print` no incluye sub-filas ni glyphs de expansión.
func TestPrintNoWorktreeSubrowsS38_1(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "repo")
	testutil.Init(t, dir)
	testutil.Marker(t, dir, "repo-principal", "", "", false)
	testutil.CommitFiles(t, dir, map[string]string{"base.txt": "base"}, "base")
	wtDir := filepath.Join(root, "wt-feat")
	testutil.MakeWorktree(t, dir, wtDir, "feat/x")

	cfg := config.Defaults()
	cfg.Roots = []string{root}
	cfg.Marker = ".gitdash.toml"
	cfg.SyncBranch = "main"

	out := captureStdout(t, func() { runPrint(cfg) })
	if !strings.Contains(out, "repo-principal") {
		t.Fatalf("S38.1: el repo principal no aparece:\n%s", out)
	}
	for _, bad := range []string{"↳", "▸", "▾"} {
		if strings.Contains(out, bad) {
			t.Errorf("S38.1: --print incluye el glyph %q:\n%s", bad, out)
		}
	}
	if strings.Contains(out, "wt-feat") {
		t.Errorf("S38.1: --print lista el worktree como fila:\n%s", out)
	}
}

// captureStdout ejecuta fn capturando todo lo escrito en os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}
