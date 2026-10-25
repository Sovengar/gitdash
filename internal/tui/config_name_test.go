package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func TestConfigNameOpensTheRenameOverlay(t *testing.T) {
	m := cursorOn(t, newPullModel(t), "/tmp/old-clean")
	m, _ = press(m, "c")
	m, _ = press(m, "n")
	if !m.nameOpen {
		t.Fatal("c n did not open the rename overlay")
	}
	if m.namePath != "/tmp/old-clean" {
		t.Errorf("namePath = %q, want the cursor's repo", m.namePath)
	}
}

func TestConfigNameWritesTheMarkerAndRenames(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	marker := "# keep me\nprimary_group = \"backend\"\n\n[ai.pull]\nprompt = \"rebase {branch}\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: filepath.Base(dir), HasRepo: true}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m, _ = press(m, "c")
	m, _ = press(m, "n")
	m = typeText(m, "renamed")
	m, cmd := press(m, "enter")

	if m.nameOpen {
		t.Error("the overlay stayed open after saving")
	}
	if cmd == nil {
		t.Fatal("the rename should toast")
	}
	if nm, ok := cmd().(notifyMsg); !ok || nm.level != toastSuccess || !strings.Contains(nm.text, "renamed") {
		t.Errorf("notification = %v, want a success naming the new name", cmd())
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# keep me\nprimary_group = \"backend\"\nname = \"renamed\"\n\n[ai.pull]\nprompt = \"rebase {branch}\"\n"
	if string(raw) != want {
		t.Errorf("marker =\n%q\nwant\n%q", string(raw), want)
	}
	if m.projects[0].Name != "renamed" {
		t.Errorf("the in-memory project kept name %q", m.projects[0].Name)
	}
}

func TestConfigNameEmptyRestoresTheFolderName(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte("name = \"demo\"\nsync_branch = \"main\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true, SyncBranch: "main"}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m, _ = press(m, "c")
	m, _ = press(m, "n")
	if got := m.nameInput.Value(); got != "demo" {
		t.Errorf("prefill = %q, want the current override", got)
	}
	m.nameInput.SetValue("")
	m, _ = press(m, "enter")

	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "sync_branch = \"main\"\n"; string(raw) != want {
		t.Errorf("marker = %q, want %q", string(raw), want)
	}
	if want := filepath.Base(dir); m.projects[0].Name != want {
		t.Errorf("in-memory name = %q, want the folder name %q", m.projects[0].Name, want)
	}
}

func TestConfigNameEscCancelsWithoutWriting(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	original := "name = \"demo\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t,
		[]discovery.Project{{Path: dir, Name: "demo", HasRepo: true}},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m, _ = press(m, "c")
	m, _ = press(m, "n")
	m = typeText(m, "nope")
	m, _ = press(m, "esc")

	if m.nameOpen {
		t.Error("esc did not close the overlay")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".gitdash.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Errorf("marker = %q, want it untouched", string(raw))
	}
	if m.projects[0].Name != "demo" {
		t.Errorf("in-memory name = %q, want demo", m.projects[0].Name)
	}
}

func TestConfigPromptNamesTheNameVariant(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "c")
	if got := m.promptLine(); !strings.Contains(got, "n name") {
		t.Errorf("prompt = %q, want the n name variant", got)
	}
}
