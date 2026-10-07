package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"gitdash/internal/testutil"
)

func writeMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMarkerPromptReadsAIPull(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	writeMarker(t, dir, "[ai.pull]\nprompt = \"fix the rebase\"\n")

	got, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "fix the rebase" {
		t.Errorf("prompt = %q", got)
	}
}

func TestMarkerPromptMissing(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	testutil.Marker(t, dir, "api", "", "", false)

	got, err := MarkerPrompt(dir, ".gitdash.toml", "pull")
	if err != nil || got != "" {
		t.Errorf("prompt/err = %q/%v, want empty/nil", got, err)
	}
}

func TestMarkerPromptWithoutFile(t *testing.T) {
	got, err := MarkerPrompt(t.TempDir(), ".gitdash.toml", "pull")
	if err != nil || got != "" {
		t.Errorf("prompt/err = %q/%v, want empty/nil", got, err)
	}
}

func TestMarkerPromptMalformed(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	writeMarker(t, dir, "name = [broken\n")

	if _, err := MarkerPrompt(dir, ".gitdash.toml", "pull"); err == nil {
		t.Error("expected an error with a malformed marker")
	}
}

func TestScanToleratesTableAI(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "api")
	testutil.Init(t, proj)
	writeMarker(t, proj, "[ai.pull]\nprompt = \"fix\"\n")

	projects, err := Scan(cfgRoots(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	if projects[0].MarkerErr != "" {
		t.Errorf("MarkerErr = %q, want empty", projects[0].MarkerErr)
	}
}
