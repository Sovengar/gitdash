package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// The binary is stubbed in PATH, never skipped: a skip removes the whole path from the profile.
func wtStub(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "wt")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))
}

func TestOpenWorktreeCmdFallsBackToTheRepoPath(t *testing.T) {
	wtStub(t)
	m, spy := handoffRecorder(t)

	if cmd := m.openWorktreeCmd("/tmp/repo", "", "feature", false); cmd == nil {
		t.Fatal("openWorktreeCmd returned no command")
	}
	if got := m.running["/tmp/repo"]; got != "worktree_switch" {
		t.Errorf("running[/tmp/repo] = %q, want worktree_switch", got)
	}
	if spy.dir != "/tmp/repo" {
		t.Errorf("Dir = %q, want the fallback path", spy.dir)
	}
	want := []string{"wt", "-C", "/tmp/repo", "switch", "feature", "--no-cd"}
	if !slices.Equal(spy.argv, want) {
		t.Errorf("argv = %q, want %q", spy.argv, want)
	}
}

func TestOpenWorktreeCmdWarnsWhenTheRefreshPathIsBusy(t *testing.T) {
	wtStub(t)
	m, _ := handoffRecorder(t)
	m.running = map[string]string{"/tmp/repo": "pull"}

	cmd := m.openWorktreeCmd("/tmp/repo", "/tmp/repo", "feature", false)
	if cmd == nil {
		t.Fatal("a busy repo returned no warning")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || msg.level != toastWarning || !strings.Contains(msg.text, "already running in") || !strings.Contains(msg.text, "pull") {
		t.Errorf("notice = %+v, want a busy warning naming pull", msg)
	}
}

func TestOpenWorktreeCmdCreatesTheBranch(t *testing.T) {
	wtStub(t)
	m, spy := handoffRecorder(t)

	if cmd := m.openWorktreeCmd("/tmp/repo", "/tmp/repo", "feature", true); cmd == nil {
		t.Fatal("openWorktreeCmd returned no command")
	}
	want := []string{"wt", "-C", "/tmp/repo", "switch", "--create", "--base=@", "feature", "--no-cd"}
	if !slices.Equal(spy.argv, want) {
		t.Errorf("argv = %q, want %q", spy.argv, want)
	}
}

func TestRenameProjectSkipsOtherPaths(t *testing.T) {
	m := newTestModel(t, []discovery.Project{
		proj("first", "/tmp/first", true),
		proj("second", "/tmp/second", true),
	}, nil)

	m.renameProject("/tmp/second", "renamed")

	if m.projects[0].Name != "first" {
		t.Errorf("the other project was renamed to %q", m.projects[0].Name)
	}
	if m.projects[1].Name != "renamed" {
		t.Errorf("the matching project kept name %q", m.projects[1].Name)
	}
}

func TestPickerEmptyListIsANoOp(t *testing.T) {
	for _, tc := range []struct {
		name, mode, filter string
	}{
		{"empty filter", pickerModeCurrent, ""},
		{"sync mode with a typed name", pickerModeSync, "feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newPullModel(t)
			m.picker = pickerOn("/tmp/old-clean", tc.mode, "c", nil, "main")
			m.picker.filter.SetValue(tc.filter)

			m, cmd := press(m, "enter")

			if m.picker == nil {
				t.Error("the picker closed on a no-op selection")
			}
			if cmd != nil || len(m.running) != 0 {
				t.Errorf("the no-op launched something: cmd=%v running=%v", cmd, m.running)
			}
		})
	}
}

func TestPickerWorktreeCreatesTheTypedName(t *testing.T) {
	wtStub(t)
	m, spy := handoffRecorder(t)
	m.projects = []discovery.Project{proj("demo", "/tmp/repo", true)}
	m.states = map[string]gitstatus.Snapshot{"/tmp/repo": snapOnBranch("main")}
	m.picker = pickerOn("/tmp/repo", pickerModeWorktree, "c", nil, "main")
	m.picker.refreshPath = "/tmp/repo"
	m.picker.filter.SetValue("feature")

	out, _ := press(*m, "enter")
	m = &out

	if m.picker != nil {
		t.Error("the picker stayed open after creating the branch")
	}
	if got := m.running["/tmp/repo"]; got != "worktree_switch" {
		t.Errorf("running = %q, want worktree_switch", got)
	}
	want := []string{"wt", "-C", "/tmp/repo", "switch", "--create", "--base=@", "feature", "--no-cd"}
	if !slices.Equal(spy.argv, want) {
		t.Errorf("argv = %q, want %q", spy.argv, want)
	}
}

func TestPickerWorktreeWarnsWhenTheBranchAlreadyHasAWorktree(t *testing.T) {
	parent := "/tmp/repo"
	snap := snapOnBranch("main")
	snap.Worktrees = []gitstatus.Worktree{{Path: "/tmp/repo-wt", Branch: "feature", Head: "abc1234"}}
	m := newTestModel(t, []discovery.Project{proj("demo", parent, true)},
		map[string]gitstatus.Snapshot{parent: snap})
	m.picker = pickerOn(parent, pickerModeWorktree, "c", []gitstatus.Branch{localBranch("feature")}, "main")
	m.picker.refreshPath = parent

	m, cmd := press(m, "enter")

	if m.picker != nil {
		t.Error("the picker stayed open")
	}
	if len(m.running) != 0 {
		t.Errorf("the existing worktree launched a process: %v", m.running)
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || msg.level != toastInfo || !strings.Contains(msg.text, "worktree already exists at /tmp/repo-wt") {
		t.Errorf("notice = %+v, want the existing-worktree hint", msg)
	}
}

func TestCommandOverlayShowsTheNameModal(t *testing.T) {
	m := newPullModel(t)
	m.nameOpen = true
	m.namePath = "/tmp/old-clean"

	got := strings.Join(m.commandOverlay(), "\n")
	if !strings.Contains(got, "name · old-clean") || !strings.Contains(got, "enter save · esc cancel") {
		t.Errorf("overlay = %q, want the name modal", got)
	}
}

func TestCmdResultOverlayClosesWithEnterAndEsc(t *testing.T) {
	for _, key := range []string{"enter", "esc"} {
		m := newPullModel(t)
		m.cmdResultOpen = &cmdResult{command: "ls", output: "ok", exit: "0"}

		m, _ = press(m, key)

		if m.cmdResultOpen != nil {
			t.Errorf("%s did not close the command-result overlay", key)
		}
	}
}

func TestConfigNameMarkerWriteFailIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapOnBranch("main")})
	m, _ = press(m, "c")
	m, _ = press(m, "n")
	m = typeText(m, "renamed")
	m, cmd := press(m, "enter")

	if m.nameOpen {
		t.Error("the overlay stayed open after a failed write")
	}
	if cmd == nil {
		t.Fatal("a failed write should warn")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || msg.level != toastError || !strings.Contains(msg.text, "marker write failed") {
		t.Errorf("notice = %+v, want the marker-write error", msg)
	}
	if m.projects[0].Name != "demo" {
		t.Errorf("a failed write renamed the project to %q", m.projects[0].Name)
	}
}

func TestSwitchAndConfigWarnWithoutARepo(t *testing.T) {
	for _, tc := range []struct {
		key  string
		kept func(Model) bool
	}{
		{"s", func(m Model) bool { return m.picker == nil }},
		{"c", func(m Model) bool { return m.configArmed == nil }},
	} {
		t.Run(tc.key, func(t *testing.T) {
			m := cursorOn(t, newPullModel(t), "/tmp/no-repo-docs")
			m, cmd := press(m, tc.key)
			m = applyNotify(m, cmd)

			if !tc.kept(m) {
				t.Errorf("%s opened its selector on a row without a repo", tc.key)
			}
			if len(m.running) != 0 {
				t.Errorf("%s launched an action on a row without a repo: %v", tc.key, m.running)
			}
			if got := lastToast(m); !strings.Contains(got, "no git repo") {
				t.Errorf("notice = %q, want the no-repo hint", got)
			}
		})
	}
}

func TestSwitchOnAWorktreeRefreshesTheParent(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/multi-wt", "feature"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	if e, ok := m.selectedEntry(); !ok || e.kind != kindWorktree {
		t.Fatalf("the cursor did not land on the subrow: %+v", e)
	}

	m, _ = press(m, "s")

	if m.picker == nil {
		t.Fatal("s did not open the switch picker")
	}
	if m.picker.refreshPath != "/tmp/multi" {
		t.Errorf("refreshPath = %q, want the parent", m.picker.refreshPath)
	}
	if m.picker.path != "/tmp/multi-wt" {
		t.Errorf("picker path = %q, want the worktree", m.picker.path)
	}
}

func TestPullVariantLabelFallsBackToTheKind(t *testing.T) {
	if got := pullVariantLabel("invented"); got != "invented" {
		t.Errorf("pullVariantLabel(invented) = %q, want the kind", got)
	}
}
