package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
	"gitdash/internal/testutil"
)

// The `!` deadline is 5 MINUTES written as a product, so the ARITHMETIC_BASE mutant makes it 8.3ms and every real command dies with no output; checked with a half-second sleep and the exit code, never by waiting it out.
func TestCommandNotIsCutsInMiddleSecond(t *testing.T) {
	out, code := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "sleep 0.5; echo delayed")
	if code != 0 {
		t.Fatalf("a half-second command must exit 0, got %d (output %q): "+
			"the %v deadline cut it short before it finished", code, out, commandTimeout())
	}
	if !strings.Contains(out, "delayed") {
		t.Errorf("the command never printed its output: %q", out)
	}
}

func TestRunShellCmdCapturesOutputAndExit(t *testing.T) {
	out, code := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "echo hello && exit 0")
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("output = %q, want to contain \"hello\"", out)
	}

	out2, code2 := runShellCmd(t.Context(), t.TempDir(), "/bin/sh", "stderr-goes-together 1>&2; exit 3")
	if code2 != 3 {
		t.Fatalf("code = %d, want 3", code2)
	}
	if !strings.Contains(out2, "stderr-goes-together") {
		t.Fatalf("stderr was not captured: %q", out2)
	}
}

func TestBangModeCommandFlow(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "!") // opens the input
	if !m.cmdOpen {
		t.Fatal("! did not open the command mode")
	}

	m, _ = press(m, "g")
	m, _ = press(m, "i")
	m, _ = press(m, "t")
	if got := m.cmdInput.Value(); got != "git" {
		t.Fatalf("input = %q, want \"git\"", got)
	}

	m, _ = press(m, "enter")
	if m.cmdOpen {
		t.Fatal("enter did not close the command mode")
	}
	if m.running[p.Path] != "cmd" {
		t.Fatalf("running = %q, want \"cmd\"", m.running[p.Path])
	}
}

func TestBangModeCommandCancel(t *testing.T) {
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, _ = press(m, "!")
	m, _ = press(m, "esc")
	if m.cmdOpen {
		t.Fatal("esc did not close the command mode")
	}
	if _, busy := m.running[p.Path]; busy {
		t.Fatal("esc launched a command")
	}

	m, _ = press(m, "!")
	m, cmd3 := press(m, "enter")
	if m.cmdOpen {
		t.Fatal("enter with an empty input did not close the command mode")
	}
	if _, err := shellPath(); err == nil && m.running[p.Path] != "shell" {
		t.Fatalf("an empty enter should open an interactive shell, running = %q", m.running[p.Path])
	} else if err != nil && cmd3 == nil {
		t.Fatal("without $SHELL it should notify")
	}
}

func shellPath() (string, error) {
	s := os.Getenv("SHELL")
	if s == "" {
		s = "/bin/sh"
	}
	return exec.LookPath(s)
}

func TestBangWithoutRepo(t *testing.T) {
	p := proj("bare", "/tmp/gitdash-test/bare", false)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, cmd := press(m, "!")
	if m.cmdOpen {
		t.Fatal("! opened the input in a repo without git")
	}
	if cmd == nil {
		t.Fatal("without a repo it should notify")
	}
}

// A fake lazygit FIRST in the PATH, the same trick the forge CLIs use: with the old `t.Skip("lazygit not installed")` the whole path was skipped on CI and coverage depended on the machine.
func withFakeLazygit(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "lazygit")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGLazygit(t *testing.T) {
	withFakeLazygit(t) // LookPath has to pass on any machine
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m := newTestModel(t, []discovery.Project{p},
		map[string]gitstatus.Snapshot{p.Path: snapClean()})

	m, cmd := press(m, "g")
	if m.running[p.Path] != "lazygit" {
		t.Fatalf("running = %q, want \"lazygit\"", m.running[p.Path])
	}
	if cmd == nil {
		t.Fatal("g returned no tea.Cmd")
	}

	without := proj("bare", "/tmp/gitdash-test/bare", false)
	m2 := newTestModel(t, []discovery.Project{without},
		map[string]gitstatus.Snapshot{without.Path: snapClean()})
	m2, cmd2 := press(m2, "g")
	if _, busy := m2.running[without.Path]; busy {
		t.Fatal("g launched lazygit in a repo without git")
	}
	if cmd2 == nil {
		t.Fatal("without a repo it should notify")
	}
}

// The input cursor lands on the first rune of the placeholder (bubbles v2 placeholderView), which must be a space and not a letter that looks typed (the phantom "! c" bug).
func TestBangPlaceholderCursorClean(t *testing.T) {
	m := New(config.Defaults())
	m.cmdOpen = true
	v := stripANSI(m.cmdInput.View())
	if !strings.HasPrefix(v, "!  ") {
		t.Fatalf("view = %q, it wanted to start with \"!  \" (prompt + placeholder space)", v)
	}
}

func TestBangInputVisibleInThePanel(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	for i := 0; i < 20; i++ {
		s.Files = append(s.Files, gitstatus.FileEntry{Code: ".M", Path: fmt.Sprintf("pkg/f%02d.go", i)})
	}
	for i := 0; i < 10; i++ {
		s.Commits = append(s.Commits, gitstatus.Commit{
			Sha: fmt.Sprintf("%07d", i), Subject: fmt.Sprintf("commit %d", i), When: time.Now().Unix(),
		})
	}
	states["/tmp/dirty-api"] = s
	m := cursorOn(t, newTestModel(t, projects, states), "/tmp/dirty-api")
	lay := m.layout()
	if lay.previewLines < detailHeadLines+cmdInputLines {
		t.Fatalf("precondition: the panel is too small (%d)", lay.previewLines)
	}

	m, _ = press(m, "!")
	if !m.cmdOpen {
		t.Fatal("! did not open the input")
	}
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "dirty-api"))
	if len(panel) != lay.previewLines {
		t.Errorf("the panel measures %d lines, want %d (the input must not overflow it)",
			len(panel), lay.previewLines)
	}
	if last := lastNonEmpty(panel); !strings.HasPrefix(last, "! ") {
		t.Errorf("the panel's last line is %q, want the input's prompt\n%v", last, panel)
	}
	if !strings.HasPrefix(panel[0], "path") || !strings.Contains(panel[0], "/tmp/dirty-api") {
		t.Errorf("the card's header is missing: %q", panel[0])
	}
	if strings.Contains(strings.Join(panel, "\n"), "g lazygit") {
		t.Errorf("with the input open the footer keybinds come back:\n%v", panel)
	}
}

func TestCardNotRepeatsTheKeybinds(t *testing.T) {
	projects, states := fixtureProjects()
	m := cursorOn(t, newTestModel(t, projects, states), "/tmp/old-clean")
	panel := panelLines(t, sectionContent(t, stripANSI(m.View().Content), "old-clean"))
	for _, dup := range []string{"g lazygit", "! cmd", "lazygit"} {
		if last := lastNonEmpty(panel); strings.Contains(last, dup) {
			t.Errorf("the card ends at %q, it wants some repo data", last)
		}
		if strings.Contains(strings.Join(panel, "\n"), dup) {
			t.Errorf("the card repeats %q, which is already in keybinds:\n%v", dup, panel)
		}
	}
}

func panelLines(t *testing.T, box string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(box, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimSuffix(strings.TrimPrefix(l, "│"), "│")
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

func lastNonEmpty(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// The toast is the only thing the user sees because the terminal has already closed, and with the guard inverted a correct handoff would warn about an error that did not happen while a failed one would say nothing.
func TestHandoffWithErrorWarns(t *testing.T) {
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)

	for _, c := range []struct {
		name       string
		err        error
		want       string
		wantAbsent string
	}{
		{"the failed one reports the reason", errHandoff("no such file or directory"), "command", ""},
		{"the successful one reports nothing", nil, "", "command:"},
	} {
		out, _ := m.Update(execDoneMsg{path: "/tmp/old-clean", action: "lazygit", err: c.err})
		flat := stripANSI(out.(Model).View().Content)
		if c.want != "" && !strings.Contains(flat, c.want) {
			t.Errorf("%s: %q is not visible:\n%s", c.name, c.want, flat)
		}
		if c.wantAbsent != "" && strings.Contains(flat, c.wantAbsent) {
			t.Errorf("%s: it reported %q with no reason:\n%s", c.name, c.wantAbsent, flat)
		}
	}
}

type errHandoff string

func (e errHandoff) Error() string { return string(e) }

func TestFetchDoneReportsOkAndFailed(t *testing.T) {
	out, _ := newTestModel(t, nil, nil).Update(fetchDoneMsg{ok: 2, failed: 1})
	m := out.(Model)
	flat := stripANSI(m.View().Content)
	if !strings.Contains(flat, "2 ok") && !strings.Contains(flat, "ok 2") {
		t.Errorf("the summary does not say how many succeeded:\n%s", flat)
	}
	if !strings.Contains(flat, "1 failed") && !strings.Contains(flat, "failed 1") {
		t.Errorf("the summary does not say how many failed:\n%s", flat)
	}
	out, _ = newTestModel(t, nil, nil).Update(fetchDoneMsg{ok: 3})
	flat = stripANSI(out.(Model).View().Content)
	if strings.Contains(flat, "failed") {
		t.Errorf("with no failures the failure line was painted:\n%s", flat)
	}
}

func TestFetchAllCountsTheOnesThatFail(t *testing.T) {
	good, _ := testutil.NewRepo(t, false)
	broken, _ := testutil.NewRepo(t, false)
	testutil.BreakGit(t, broken) // this repo's fetch fails
	projects := []discovery.Project{
		proj("good", good, true),
		proj("broken", broken, true),
	}
	states := map[string]gitstatus.Snapshot{good: snapClean(), broken: snapClean()}
	m := newTestModel(t, projects, states)

	m, _ = press(m, "F") // fetch_all
	deadline := time.After(30 * time.Second)
	var done fetchDoneMsg
	for done.ok == 0 && done.failed == 0 {
		select {
		case ev := <-m.events:
			if fd, ok := ev.(fetchDoneMsg); ok {
				done = fd
			}
		case <-deadline:
			t.Fatal("no fetchDoneMsg observed")
		}
	}
	if done.ok != 1 || done.failed != 1 {
		t.Errorf("fetchDoneMsg = %+v, want ok=1 failed=1", done)
	}
}

func TestSaveCollapsedWithMoreExpandedThanFolded(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi", wt("/tmp/wt-a", "a"))
	m := newTestModel(t, []discovery.Project{p}, st)
	m.expanded = map[string]bool{"/tmp/multi": true, "/tmp/other": true, "/tmp/third": true}
	m.collapsed = map[string]bool{} // no collapsed group: a negative hint if it is subtracted

	m.saveCollapsed()

	got := m.store.LoadCollapsed()
	if len(got) != 3 {
		t.Fatalf("persisted = %v, want 3 keys", got)
	}
	for _, path := range []string{"/tmp/multi", "/tmp/other", "/tmp/third"} {
		key := state.WorktreePrefix + path
		if !got[key] {
			t.Errorf("the expanded worktree key %q is missing: %v", key, got)
		}
	}
}

func TestActionOfGitRunsAndReCollects(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)         // with upstream
	testutil.PushUpstreamCommits(t, origin, 1, "up") // the local repo is behind
	testutil.FetchLocal(t, dir)                      // with that, a clean fast-forward
	states := map[string]gitstatus.Snapshot{dir: snapClean()}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)}, states)
	m = cursorOn(t, m, dir)

	m, _ = press(m, "p") // arms the selector
	m, _ = press(m, "p") // default variant -> git pull

	deadline := time.After(60 * time.Second)
	var acc *actionMsg
	var huboStatus bool
	for acc == nil || !huboStatus {
		select {
		case ev := <-m.events:
			switch e := ev.(type) {
			case actionMsg:
				cp := e
				acc = &cp
			case statusMsg:
				huboStatus = true
			}
		case <-deadline:
			t.Fatalf("the action did not close the loop (acc=%v status=%v)", acc != nil, huboStatus)
		}
	}
	if acc.err != "" {
		t.Fatalf("a pull that should have integrated failed: %q (output %q)", acc.err, acc.output)
	}
	if acc.kind != "pull" {
		t.Errorf("kind = %q, want pull", acc.kind)
	}
	if !huboStatus {
		t.Error("after a successful action the repo state was not re-collected")
	}
}

// The flag is computed ONLY when the action failed and was a pull: with the guard reversed it would be consulted on the correct pulls (where there is never a rebase) and never on the clashing ones.
func TestPullThatClashesMarksTheRebaseHalfDone(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamFile(t, origin, "conflict.txt", "remoto", "remote")
	testutil.CommitFiles(t, dir, map[string]string{"conflict.txt": "local"}, "local")
	testutil.FetchLocal(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	states := map[string]gitstatus.Snapshot{dir: snapClean()}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)}, states)
	m = cursorOn(t, m, dir)

	m, _ = press(m, "p")
	m, _ = press(m, "r") // explicit rebase variant

	deadline := time.After(60 * time.Second)
	var acc *actionMsg
	for acc == nil {
		select {
		case ev := <-m.events:
			if e, ok := ev.(actionMsg); ok {
				cp := e
				acc = &cp
			}
		case <-deadline:
			t.Fatal("the pull did not publish its result")
		}
	}
	if acc.err == "" {
		t.Fatal("a clashing pull did not fail (the fixture does not clash)")
	}
	if !acc.rebaseInProgress {
		t.Errorf("rebaseInProgress = false with a mid-rebase: %+v", acc)
	}
}

// $SHELL (the user's) is what makes aliases and config load instead of /bin/sh, and the argv left in the command log is what proves it, so that is what is looked at.
func TestShellOfTheHandoffIsTheOfTheUser(t *testing.T) {
	dir := t.TempDir()
	projects, states := fixtureProjects()
	states[dir] = snapClean()
	projects = append(projects, proj("cwd", dir, true))

	typeIn := func(m Model, text string) Model {
		for _, k := range text {
			m, _ = press(m, string(k))
		}
		return m
	}

	t.Run("with SHELL in the environment", func(t *testing.T) {
		bash := filepath.Join(t.TempDir(), "mishell")
		if err := os.WriteFile(bash, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SHELL", bash)
		m := newTestModel(t, projects, states)
		rec := cmdlog.Active()
		t.Cleanup(func() { cmdlog.SetRecorder(nil) })
		m = cursorOn(t, m, dir)

		m, _ = press(m, m.cfg.KeyFor("command"))
		if !m.cmdOpen {
			t.Fatal("the ! key did not open the command input")
		}
		m = typeIn(m, "hello")
		m, _ = press(m, "enter") // runs the command with the user's shell
		assertLastArgvShell(t, rec, bash, "cmd")
	})

	t.Run("without SHELL it falls back to /bin/sh", func(t *testing.T) {
		t.Setenv("SHELL", "")
		m := newTestModel(t, projects, states)
		rec := cmdlog.Active()
		t.Cleanup(func() { cmdlog.SetRecorder(nil) })
		m = cursorOn(t, m, dir)

		m, _ = press(m, m.cfg.KeyFor("command"))
		m = typeIn(m, "hello")
		m, _ = press(m, "enter")
		assertLastArgvShell(t, rec, "/bin/sh", "cmd")
	})

}

// The interactive one cannot be exercised here (tea.ExecProcess does not run without a TTY), so the pure function is what pins the guarantee for both paths.
func TestUserShell(t *testing.T) {
	t.Run("with SHELL", func(t *testing.T) {
		t.Setenv("SHELL", "/opt/homebrew/bin/fish")
		if got := userShell(); got != "/opt/homebrew/bin/fish" {
			t.Errorf("userShell = %q, want the user's", got)
		}
	})
	t.Run("without SHELL", func(t *testing.T) {
		t.Setenv("SHELL", "")
		if got := userShell(); got != "/bin/sh" {
			t.Errorf("userShell = %q, want /bin/sh", got)
		}
	})
}

func assertLastArgvShell(t *testing.T, rec *cmdlog.Recorder, want, action string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		for _, e := range rec.Entries() {
			if e.Action != action || len(e.Argv) == 0 {
				continue
			}
			if e.Argv[0] != want {
				t.Errorf("argv[0] = %q, want the shell %q", e.Argv[0], want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the handoff %q with the shell %q was not recorded", action, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFetchOfAOnlyRepoNotCarriesCount(t *testing.T) {
	out, _ := newTestModel(t, nil, nil).Update(fetchDoneMsg{ok: 1})
	flat := stripANSI(out.(Model).View().Content)
	if !strings.Contains(flat, "fetch ok") {
		t.Errorf("the toast does not say the fetch went well:\n%s", flat)
	}
	if strings.Contains(flat, "(1 repos)") {
		t.Errorf("a single-repo fetch carries no count, let alone the word \"repos\":\n%s", flat)
	}
}

// A toast announcing "failed" with exit 0 sends the user to inspect a command that succeeded.
func TestCommandSuccessAndOutputNotIsConfuse(t *testing.T) {
	for _, c := range []struct {
		name string
		exit string
		want string
	}{
		{"exit 0 is not a failure", "0", "ok"},
		{"a non-zero exit code is", "3", "exit 3"},
	} {
		out, _ := newTestModel(t, nil, nil).Update(cmdResultMsg{
			path: "/tmp/dirty-api", command: "echo hello", exit: c.exit,
		})
		m := out.(Model)
		flat := stripANSI(m.View().Content)
		if !strings.Contains(flat, c.want) {
			t.Errorf("%s: the warning does not say %q:\n%s", c.name, c.want, flat)
		}
		if got := m.lastCmd["/tmp/dirty-api"]; got.exit != c.exit {
			t.Errorf("%s: lastCmd.exit = %q, want %q", c.name, got.exit, c.exit)
		}
		if got := m.lastCmd["/tmp/dirty-api"]; got.command != "echo hello" {
			t.Errorf("%s: lastCmd.command = %q, want the typed command", c.name, got.command)
		}
		// A failed `!` must not leave the row blocked with "already running" forever.
		if _, busy := m.running["/tmp/dirty-api"]; busy {
			t.Errorf("%s: the repo was left with a running action after the command", c.name)
		}
	}
}

func TestEndAboutTableEmptyNotIsExitsOfTheCursor(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if len(m.entries()) != 0 {
		t.Fatalf("the model with no projects has %d entries, want 0", len(m.entries()))
	}

	m, _ = press(m, "end")
	if got := m.cursor; got != 0 {
		t.Errorf("cursor = %d in an empty table, want 0", got)
	}
	projects, states := fixtureProjects()
	m2 := newTestModel(t, projects, states)
	m2, _ = press(m2, "end")
	if want := len(m2.entries()) - 1; m2.cursor != want {
		t.Errorf("cursor = %d, want %d (the last row)", m2.cursor, want)
	}
}

// execDoneMsg is what the TUI command sees on returning from the handoff, so its action must be the key that was pressed; the closure inside tea.ExecProcess cannot be tested because it only runs with the program suspended.
func TestHandoffDoneIdentifiesTheAction(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	for _, action := range []string{"editor", "lazygit", "pull_ai", "visual", "shell"} {
		msg, ok := m.handoffDone(action, "/tmp/repo", []string{"argv", "de", action}, nil).(execDoneMsg)
		if !ok {
			t.Fatalf("handoffDone(%q) returned no execDoneMsg", action)
		}
		if msg.action != action {
			t.Errorf("handoffDone(%q).action = %q, want %q (the action goes to the log as is)",
				action, msg.action, action)
		}
		if msg.path != "/tmp/repo" {
			t.Errorf("handoffDone(%q).path = %q, want /tmp/repo", action, msg.path)
		}
		if len(msg.argv) != 3 || msg.argv[2] != action {
			t.Errorf("handoffDone(%q).argv = %q, want the argv untouched (that is what gets recorded)",
				action, msg.argv)
		}
		if msg.err != nil {
			t.Errorf("handoffDone(%q).err = %v, want nil", action, msg.err)
		}
	}

	boom := errors.New("output with code 1")
	msg, ok := m.handoffDone("editor", "/tmp/repo", []string{"vim"}, boom).(execDoneMsg)
	if !ok {
		t.Fatal("handoffDone returned no execDoneMsg")
	}
	if !errors.Is(msg.err, boom) {
		t.Errorf("err = %v, want the process error propagated", msg.err)
	}
}

// Both LookPath-guarded handoffs check the binary BEFORE the busy guard: on a machine without lazygit the warning must be "not installed" and not "a lazygit is already running here", which would be a lie.
func TestHandoffsWithGuardOfLookPath(t *testing.T) {
	t.Run("lazygit not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // no lazygit
		mm := newTestModel(t, nil, nil)
		m := &mm
		msg, ok := m.openLazygitCmd("/tmp/repo")().(notifyMsg)
		if !ok {
			t.Fatal("openLazygitCmd without lazygit returned no notifyMsg")
		}
		if msg.level != toastWarning || !strings.Contains(msg.text, "not installed") {
			t.Errorf("notice = %+v, want a warning that it is not installed", msg)
		}
		if len(m.running) != 0 {
			t.Errorf("running = %v, want empty (the handoff was not launched)", m.running)
		}
	})

	t.Run("shell not found", func(t *testing.T) {
		t.Setenv("SHELL", "a-shell-that-does-not-exist-98765")
		t.Setenv("PATH", t.TempDir())
		mm := newTestModel(t, nil, nil)
		m := &mm
		msg, ok := m.openShellCmd("/tmp/repo")().(notifyMsg)
		if !ok {
			t.Fatal("openShellCmd without a shell returned no notifyMsg")
		}
		if msg.level != toastWarning || !strings.Contains(msg.text, "shell") {
			t.Errorf("notice = %+v, want a shell-not-found warning", msg)
		}
		if len(m.running) != 0 {
			t.Errorf("running = %v, want empty", m.running)
		}
	})

	t.Run("repo already busy", func(t *testing.T) {
		mm := newTestModel(t, nil, nil)
		m := &mm
		m.running = map[string]string{"/tmp/repo": "pull"}
		msg, ok := m.openShellCmd("/tmp/repo")().(notifyMsg)
		if !ok {
			t.Fatal("openShellCmd with the repo busy returned no notifyMsg")
		}
		if !strings.Contains(msg.text, "already running") {
			t.Errorf("notice = %q, want it to say something is already running", msg.text)
		}
		if !strings.Contains(msg.text, "pull") {
			t.Errorf("notice = %q, want it to name the action that is running", msg.text)
		}
	})
}

// A handoff recorder does NOT lend the terminal: tea.ExecProcess suspends the whole program and nobody resumes it, which is why every `return tea.ExecProcess(...)` was a statement nobody ran — the TUI's coverage ceiling.
func handoffRecorder(t *testing.T) (*Model, *handoffSpy) {
	t.Helper()
	mm := newTestModel(t, nil, nil)
	m := &mm
	spy := &handoffSpy{t: t, done: func(error) tea.Msg { return nil }}
	m.handoff = spy.exec
	return m, spy
}

type handoffSpy struct {
	t    *testing.T
	done tea.ExecCallback
	argv []string
	dir  string
	vals int
}

func (h *handoffSpy) exec(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	h.argv, h.dir = c.Args, c.Dir
	h.vals++
	h.done = fn
	return func() tea.Msg { return nil }
}

func (h *handoffSpy) finish(err error) tea.Msg {
	h.t.Helper()
	return h.done(err)
}

func TestHandoffRunsTheProcessInTheRepo(t *testing.T) {
	for _, c := range []struct {
		action string
		open   func(m *Model, path string) tea.Cmd
		argv0  string
	}{
		{"editor", (*Model).openEditorCmd, "vi"},
		{"lazygit", (*Model).openLazygitCmd, "lazygit"},
		{"shell", (*Model).openShellCmd, ""},
	} {
		t.Run(c.action, func(t *testing.T) {
			m, spy := handoffRecorder(t)
			m.cfg.Editor = "vi"
			// Only shell and lazygit need the binary on the PATH (the editor does not go through LookPath), and a switch instead of if/else-if because the branches are not interchangeable: lazygit's writes a fake executable to get past LookPath without launching anything (the handoff is injected).
			switch c.action {
			case "shell":
				t.Setenv("SHELL", "/bin/sh")
			case "lazygit":
				bin := filepath.Join(t.TempDir(), "lazygit")
				if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))
			}

			if c.open(m, "/tmp/repo") == nil {
				t.Fatalf("%s returned no command", c.action)
			}
			if spy.vals != 1 {
				t.Fatalf("handoff called %d times, want 1", spy.vals)
			}
			if spy.dir != "/tmp/repo" {
				t.Errorf("Dir = %q, want /tmp/repo (the handoff runs INSIDE the repo)", spy.dir)
			}
			if len(spy.argv) == 0 {
				t.Fatalf("empty argv")
			}
			msg, ok := spy.finish(nil).(execDoneMsg)
			if !ok {
				t.Fatalf("%s's handoff returned no execDoneMsg on exit", c.action)
			}
			if msg.path != "/tmp/repo" || msg.action != c.action {
				t.Errorf("execDoneMsg = %+v, want path=/tmp/repo action=%s", msg, c.action)
			}
			if len(msg.argv) == 0 || msg.argv[0] != spy.argv[0] {
				t.Errorf("execDoneMsg.argv = %q, want the same that ran (%q)", msg.argv, spy.argv)
			}
		})
	}
}

func TestHandoffsWithGuardOfBusy(t *testing.T) {
	t.Run("pull_ai with the repo busy", func(t *testing.T) {
		mm := newTestModel(t, nil, nil)
		m := &mm
		m.running = map[string]string{"/tmp/repo": "pull"}
		msg, ok := m.openPullAICmd("/tmp/repo", []string{"ai-pull", "--x"})().(notifyMsg)
		if !ok {
			t.Fatal("openPullAICmd with the repo busy returned no notice")
		}
		if !strings.Contains(msg.text, "already running") {
			t.Errorf("notice = %q, want it to say something is already running", msg.text)
		}
	})

	t.Run("visual with the repo busy", func(t *testing.T) {
		mm := newTestModel(t, nil, nil)
		m := &mm
		m.running = map[string]string{"/tmp/repo": "pull_ai"}
		msg, ok := m.startVisualCmd("/tmp/repo", "origin/main", "merge")().(notifyMsg)
		if !ok {
			t.Fatal("startVisualCmd with the repo busy returned no notice")
		}
		if !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "pull_ai") {
			t.Errorf("notice = %q, want it to name the action that is running", msg.text)
		}
	})

	t.Run("visual runs git-sim in the background with the argv", func(t *testing.T) {
		openLog := writeVisualStubs(t)
		mm := newTestModel(t, nil, nil)
		m := &mm
		repo := t.TempDir()

		if cmd := m.startVisualCmd(repo, "origin/main", "rebase"); cmd != nil {
			t.Fatal("startVisualCmd returned a command: the render must not suspend the TUI")
		}
		if m.running[repo] != "visual" {
			t.Errorf("running = %v, want the repo marked as visual in progress", m.running)
		}
		dm := waitVisual(t, m.events)
		if dm.err != nil || dm.openErr != nil {
			t.Fatalf("the render failed: err=%v openErr=%v", dm.err, dm.openErr)
		}
		full := strings.Join(dm.argv, " ")
		if !strings.Contains(full, "rebase") {
			t.Errorf("argv = %q, want the rebase subcommand", full)
		}
		if !strings.Contains(full, "origin/main") {
			t.Errorf("argv = %q, want the upstream's ref (rebase requires it)", full)
		}
		if !strings.Contains(full, "--media-dir") {
			t.Errorf("argv = %q, want --media-dir ALWAYS (without it git-sim dirties the repo)", full)
		}
		if !strings.Contains(full, "--output-only-path") {
			t.Errorf("argv = %q, want --output-only-path (the capture's only way back is the printed path)", full)
		}
		if !strings.HasPrefix(dm.image, filepath.Join(os.Getenv("XDG_CACHE_HOME"), "gitdash", "git-sim")) {
			t.Errorf("the image was kept at %q, want the git-sim cache dir", dm.image)
		}
		waitForOpen(t, openLog, dm.image)

		mm2 := newTestModel(t, nil, nil)
		if cmd := mm2.startVisualCmd(repo, "origin/main", "pull"); cmd != nil {
			t.Fatal("startVisualCmd(pull) returned a command")
		}
		dm2 := waitVisual(t, mm2.events)
		if strings.Contains(strings.Join(dm2.argv, " "), "origin/main") {
			t.Errorf("argv = %q, want no ref: git-sim's pull takes no argument", dm2.argv)
		}
	})
}

// Nil and not a warning, because recollect is not a key the user pressed with an intent (the program itself triggers it on coming back from a handoff).
func TestRecollectWithRepoBusyNotRelaunch(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "pull"}
	before := len(m.running)

	if cmd := m.recollectCmd("/tmp/api"); cmd != nil {
		t.Errorf("recollectCmd over a busy repo returned %#v, want nil", cmd())
	}
	if got := m.running["/tmp/api"]; got != "pull" {
		t.Errorf("running = %q, want it to still be the original action", got)
	}
	if len(m.running) != before {
		t.Errorf("running has %d entries, want %d (the guard does not write)", len(m.running), before)
	}
}

// `d` on a worktree whose main repo is pulling has to say "a pull is already running", and not fail silently nor delete anyway.
func TestRemoveWorktreeWithParentBusyWarns(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "pull"}

	cmd := m.removeWorktreeCmd("/tmp/api", "/tmp/api-wt", "api-wt", false, 1)
	if cmd == nil {
		t.Fatal("removeWorktreeCmd with the parent busy returned nil, want a notice")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("returned %T, want a notice", cmd())
	}
	if !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "pull") {
		t.Errorf("notice = %q, want it to name the action in progress", msg.text)
	}
	if got := m.running["/tmp/api"]; got != "pull" {
		t.Errorf("running = %q, want pull (not worktree_remove)", got)
	}
}

func TestLazygitBusyNamesTheRunningAction(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "lazygit")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))

	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "push"}

	msg, ok := m.openLazygitCmd("/tmp/api")().(notifyMsg)
	if !ok {
		t.Fatal("openLazygitCmd with the repo busy returned no notice")
	}
	if !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "push") {
		t.Errorf("notice = %q, want it to name the push in progress", msg.text)
	}
}

func TestCommandCapturedWithRepoBusyWarns(t *testing.T) {
	mm := newTestModel(t, nil, nil)
	m := &mm
	m.running = map[string]string{"/tmp/api": "pull"}

	msg, ok := m.openCmdCmd("/tmp/api", "git status")().(notifyMsg)
	if !ok {
		t.Fatal("openCmdCmd with the repo busy returned no notice")
	}
	if !strings.Contains(msg.text, "already running") {
		t.Errorf("notice = %q, want it to say something is already running", msg.text)
	}
	if got := m.running["/tmp/api"]; got != "pull" {
		t.Errorf("running = %q, want pull (the guard returns before writing)", got)
	}
}

// The trust boundary is checked on the way: the prompt comes from the committed marker (untrusted input) and has to travel as ONE argv element, never inside an sh -c.
func TestPullAIHappyPathArmsTheHandoff(t *testing.T) {
	dir := t.TempDir()
	nasty := `fix el rebase; rm -rf / $(whoami) | tee /etc/passwd`
	// Literal TOML with single quotes: a prompt with double quotes would break the TOML, not because of the prompt but because of how it is written in the file; what is tested here is the argv, not the TOML.
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"),
		[]byte("[ai.pull]\nprompt = '"+nasty+"'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "ai-pull")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+":"+os.Getenv("PATH"))

	mm := newTestModel(t, nil, nil)
	m := &mm
	m.cfg.AICommands = map[string]string{"pull": "ai-pull {prompt}"}
	spy := &handoffSpy{t: t, done: func(error) tea.Msg { return nil }}
	m.handoff = spy.exec

	cmd := m.startPullAICmd(dir)
	if cmd == nil {
		t.Fatal("startPullAICmd returned nil, want a handoff")
	}
	cmd()
	if spy.vals != 1 {
		t.Fatalf("the handoff launched %d times, want 1", spy.vals)
	}
	if spy.dir != dir {
		t.Errorf("Dir = %q, want the marker's repo", spy.dir)
	}
	if len(spy.argv) < 2 || spy.argv[0] != "ai-pull" {
		t.Fatalf("argv = %q, want the config's executable and at least one argument", spy.argv)
	}
	var promptArg string
	for _, a := range spy.argv[1:] {
		if a == nasty {
			promptArg = a
		}
	}
	if promptArg != nasty {
		t.Errorf("no argv element is the whole prompt; it got split or interpolated: %q", spy.argv)
	}
	if msg, ok := spy.finish(nil).(execDoneMsg); !ok || msg.action != "pull_ai" {
		t.Errorf("the returned message = %#v, want a pull_ai execDoneMsg", msg)
	}
	if m.running[dir] != "pull_ai" {
		t.Errorf("running = %v, want the repo marked as pull_ai in progress", m.running)
	}
}
