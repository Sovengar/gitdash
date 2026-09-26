package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"gitdash/internal/testutil"
)

// writeMarker escribe un marcador con contenido arbitrario (p. ej. la tabla
// [ai.pull]) en dir.
func writeMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// El prompt se lee del marcador on demand; no se guarda en Project.
func TestMarkerPromptLeeAIPull(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	writeMarker(t, dir, "[ai.pull]\nprompt = \"arreglá el rebase\"\n")

	got, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "arreglá el rebase" {
		t.Errorf("prompt = %q", got)
	}
}

// Sin tabla [ai] el prompt es vacío, no un error.
func TestMarkerPromptAusente(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	testutil.Marker(t, dir, "api", "", "", false)

	got, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err != nil || got != "" {
		t.Errorf("prompt/err = %q/%v, want vacío/nil", got, err)
	}
}

// Un worktree sintético sin marcador no es un error: simplemente no hay prompt.
func TestMarkerPromptSinFichero(t *testing.T) {
	got, err := MarkerPrompt(t.TempDir(), ".gitdash.toml", "pull")
	if err != nil || got != "" {
		t.Errorf("prompt/err = %q/%v, want vacío/nil", got, err)
	}
}

// Un marcador roto degrada a error (la TUI lo convierte en toast).
func TestMarkerPromptMalformado(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	writeMarker(t, dir, "name = [roto\n")

	if _, err := MarkerPrompt(dir, ".gitdash.toml", "pull"); err == nil {
		t.Error("se esperaba error con marcador malformado")
	}
}

// La tabla anidada [ai.pull] parsea limpia: Scan no marca MarkerErr ni la
// guarda en Project.
func TestScanToleraTablaAI(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "api")
	testutil.Init(t, proj)
	writeMarker(t, proj, "[ai.pull]\nprompt = \"arregla\"\n")

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	if projects[0].MarkerErr != "" {
		t.Errorf("MarkerErr = %q, want vacío", projects[0].MarkerErr)
	}
}
