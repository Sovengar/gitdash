package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func markerPath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(dir, ".gitdash.toml")
}

func writeRawMarker(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(markerPath(t, dir), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMarker(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(markerPath(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The whole point of the targeted edit: comments and the [ai] prompt must survive byte for byte.
func TestSetMarkerSyncBranchReplacesOneLineAndKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	original := "# repo marker\nname = \"demo\"\nsync_branch = \"main\"\nprimary_group = \"backend\"\n\n# keep me\n[ai.pull]\nprompt = \"rebase the {branch}\"\n"
	writeRawMarker(t, dir, original)

	if err := SetMarkerSyncBranch(dir, ".gitdash.toml", "develop"); err != nil {
		t.Fatal(err)
	}

	want := "# repo marker\nname = \"demo\"\nsync_branch = \"develop\"\nprimary_group = \"backend\"\n\n# keep me\n[ai.pull]\nprompt = \"rebase the {branch}\"\n"
	if got := readMarker(t, dir); got != want {
		t.Errorf("marker =\n%q\nwant\n%q", got, want)
	}
}

func TestSetMarkerSyncBranchInsertsBeforeTheFirstTable(t *testing.T) {
	dir := t.TempDir()
	writeRawMarker(t, dir, "name = \"demo\"\n\n[ai.pull]\nprompt = \"x\"\n")

	if err := SetMarkerSyncBranch(dir, ".gitdash.toml", "develop"); err != nil {
		t.Fatal(err)
	}

	want := "name = \"demo\"\nsync_branch = \"develop\"\n\n[ai.pull]\nprompt = \"x\"\n"
	if got := readMarker(t, dir); got != want {
		t.Errorf("marker =\n%q\nwant\n%q", got, want)
	}
}

func TestSetMarkerSyncBranchAppendsWithNoTable(t *testing.T) {
	dir := t.TempDir()
	writeRawMarker(t, dir, "name = \"demo\"\n")

	if err := SetMarkerSyncBranch(dir, ".gitdash.toml", "develop"); err != nil {
		t.Fatal(err)
	}

	want := "name = \"demo\"\nsync_branch = \"develop\"\n"
	if got := readMarker(t, dir); got != want {
		t.Errorf("marker =\n%q\nwant\n%q", got, want)
	}
}

// A sync_branch inside a table is not the top-level key the parser reads, so it must not be the one rewritten.
func TestSetMarkerSyncBranchIgnoresTableKeys(t *testing.T) {
	dir := t.TempDir()
	writeRawMarker(t, dir, "[ai.pull]\nsync_branch = \"inner\"\n")

	if err := SetMarkerSyncBranch(dir, ".gitdash.toml", "develop"); err != nil {
		t.Fatal(err)
	}

	want := "sync_branch = \"develop\"\n[ai.pull]\nsync_branch = \"inner\"\n"
	if got := readMarker(t, dir); got != want {
		t.Errorf("marker =\n%q\nwant\n%q", got, want)
	}
}

func TestSetMarkerSyncBranchCreatesTheFile(t *testing.T) {
	dir := t.TempDir()

	if err := SetMarkerSyncBranch(dir, ".gitdash.toml", "develop"); err != nil {
		t.Fatal(err)
	}
	if got, want := readMarker(t, dir), "sync_branch = \"develop\"\n"; got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
}

// A directory where the marker should be: EISDIR fails the same everywhere, unlike a 0o000 file (root reads it).
func TestSetMarkerSyncBranchUnreadableWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(markerPath(t, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SetMarkerSyncBranch(dir, ".gitdash.toml", "develop"); err == nil {
		t.Error("a marker that is a directory = nil error, want a failure")
	}
}
