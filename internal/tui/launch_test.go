package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// A stub FIRST in the PATH, the same trick as withFakeLazygit: with a `t.Skip` the whole launcher path would be skipped on a runner without the tool and the statements would never reach the coverage profile.
func withFakeTool(t *testing.T, name string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The handoff is injected (a real one suspends the program and nobody resumes it in a test) and the model owns the projects, so the key presses go through the real routing.
func launchModel(t *testing.T, projects []discovery.Project, states map[string]gitstatus.Snapshot) (*Model, *handoffSpy) {
	t.Helper()
	m := newTestModel(t, projects, states)
	spy := &handoffSpy{t: t, done: func(error) tea.Msg { return nil }}
	m.handoff = spy.exec
	return &m, spy
}

func repoModel(t *testing.T) (*Model, *handoffSpy, discovery.Project) {
	t.Helper()
	p := proj("demo", "/tmp/gitdash-test/demo", true)
	m, spy := launchModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	return m, spy, p
}

func jjRepo(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.MkdirAll(filepath.Join(path, ".jj"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func jjModel(t *testing.T, path string) (*Model, *handoffSpy) {
	t.Helper()
	p := proj("demo", path, true)
	m, spy := launchModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{p.Path: snapClean()})
	return m, spy
}

// With nothing armed there is no prompt, or the keybinds line would paint a warning in place of the hints.
func TestLaunchPromptWithNothingArmed(t *testing.T) {
	m := newTestModel(t, nil, nil)
	if got := m.launchPrompt(); got != "" {
		t.Errorf("launchPrompt with nothing armed = %q, want empty", got)
	}
}

// The prefix must not start anything: it only arms and paints the sub-keys, or the first `g` would keep launching lazygit while the prompt says otherwise.
func TestLaunchPrefixArmsAndPrompts(t *testing.T) {
	withFakeLazygit(t)
	m, spy, p := repoModel(t)

	mm, cmd := press(*m, "g")
	if cmd != nil {
		t.Errorf("the prefix returned a command, so something may have started: %v", cmd)
	}
	if spy.vals != 0 {
		t.Errorf("the prefix ran %d handoffs, want 0", spy.vals)
	}
	if len(mm.running) != 0 {
		t.Errorf("running = %v, want empty", mm.running)
	}
	if mm.launchArmed == nil || mm.launchArmed.path != p.Path {
		t.Fatalf("launchArmed = %+v, want the cursor's path", mm.launchArmed)
	}
	want := "g lazygit · j lazyjj · b gitbutler · esc cancel"
	if got := mm.launchPrompt(); !strings.Contains(got, want) {
		t.Errorf("prompt = %q, want it to offer %q", got, want)
	}
	if got := mm.promptLine(); !strings.Contains(got, want) {
		t.Errorf("the keybinds line = %q, want the same offer", got)
	}
	if out := stripANSI(mm.View().Content); !strings.Contains(out, want) {
		t.Errorf("the prompt is not painted:\n%s", out)
	}
}

// The old `g` = lazygit survives behind the prefix with the same argv and execDoneMsg contract.
func TestLaunchLazygitAfterThePrefix(t *testing.T) {
	withFakeLazygit(t)
	m, spy, p := repoModel(t)

	mm, _ := press(*m, "g")
	mm, cmd := press(mm, "g")
	if cmd == nil {
		t.Fatal("the second g returned no tea.Cmd")
	}
	if mm.launchArmed != nil {
		t.Error("the arm survived the dispatch")
	}
	if mm.running[p.Path] != "lazygit" {
		t.Fatalf("running = %q, want lazygit", mm.running[p.Path])
	}
	if spy.vals != 1 || spy.dir != p.Path {
		t.Errorf("handoff = %d runs in %q, want 1 in %q", spy.vals, spy.dir, p.Path)
	}
	if len(spy.argv) != 1 || spy.argv[0] != "lazygit" {
		t.Errorf("argv = %q, want [lazygit]", spy.argv)
	}
	msg, ok := spy.finish(nil).(execDoneMsg)
	if !ok || msg.action != "lazygit" || msg.path != p.Path {
		t.Errorf("execDoneMsg = %+v (%v), want action lazygit on %s", msg, ok, p.Path)
	}
}

// A row with no repo arms nothing at all: the guard sits at arming, so no sub-key can launch in a folder that is not a repo.
func TestLaunchPrefixWithoutRepo(t *testing.T) {
	withFakeLazygit(t)
	m, spy, _ := repoModel(t)
	m.projects = append(m.projects, proj("no-repo-docs", "/tmp/no-repo-docs", false))
	m.states["/tmp/no-repo-docs"] = gitstatus.Snapshot{}
	m.cursor = cursorOn(t, *m, "/tmp/no-repo-docs").cursor

	mm, cmd := press(*m, "g")
	if mm.launchArmed != nil {
		t.Errorf("the prefix armed on a row without a repo: %+v", mm.launchArmed)
	}
	if cmd == nil {
		t.Error("without a repo it should notify")
	}
	if spy.vals != 0 {
		t.Errorf("it ran %d handoffs, want 0", spy.vals)
	}
}

func TestLaunchLazyjj(t *testing.T) {
	path := jjRepo(t)
	withFakeTool(t, "lazyjj")
	m, spy := jjModel(t, path)
	m.cfg = config.Defaults()
	rec := cmdlog.Active()
	if rec == nil {
		t.Fatal("no command log recorder")
	}
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })

	mm, _ := press(*m, "g")
	mm, cmd := press(mm, "j")
	if cmd == nil {
		t.Fatal("g j returned no tea.Cmd")
	}
	if mm.running[path] != "lazyjj" {
		t.Fatalf("running = %q, want lazyjj", mm.running[path])
	}
	if spy.vals != 1 || spy.dir != path {
		t.Errorf("handoff = %d runs in %q, want 1 in %q", spy.vals, spy.dir, path)
	}
	if len(spy.argv) != 1 || spy.argv[0] != "lazyjj" {
		t.Errorf("argv = %q, want [lazyjj]", spy.argv)
	}
	if msg, ok := spy.finish(nil).(execDoneMsg); !ok || msg.action != "lazyjj" {
		t.Errorf("execDoneMsg = %+v (%v), want action lazyjj", msg, ok)
	}
	e := entryFor(t, rec, "lazyjj")
	if !e.Intent || e.Key != "j" || e.Dir != path {
		t.Errorf("command log entry = %+v, want the intent of key j in %s", e, path)
	}
}

// A git repo without .jj is not a jj repo: lazyjj would open on nothing, so the guard warns instead of handing the terminal over.
func TestLaunchLazyjjWithoutJJ(t *testing.T) {
	withFakeTool(t, "lazyjj")
	m, spy, p := repoModel(t)

	mm, _ := press(*m, "g")
	mm, cmd := press(mm, "j")
	if cmd == nil {
		t.Fatal("g j without .jj returned no tea.Cmd")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || !strings.Contains(msg.text, "no jj repo") {
		t.Errorf("notice = %+v (%v), want a warning about no jj repo", msg, ok)
	}
	if _, busy := mm.running[p.Path]; busy {
		t.Errorf("something launched: %v", mm.running)
	}
	if spy.vals != 0 {
		t.Errorf("it ran %d handoffs, want 0", spy.vals)
	}
}

func TestLaunchMissingBinaries(t *testing.T) {
	t.Run("lazyjj", func(t *testing.T) {
		path := jjRepo(t)
		m, spy := jjModel(t, path)
		t.Setenv("PATH", t.TempDir()) // no lazyjj

		mm, _ := press(*m, "g")
		mm, cmd := press(mm, "j")
		msg, ok := cmd().(notifyMsg)
		if !ok || msg.level != toastWarning || !strings.Contains(msg.text, "lazyjj not installed") {
			t.Errorf("notice = %+v (%v), want lazyjj not installed", msg, ok)
		}
		if len(mm.running) != 0 || spy.vals != 0 {
			t.Errorf("running = %v, handoffs = %d, want nothing launched", mm.running, spy.vals)
		}
	})

	t.Run("gitbutler", func(t *testing.T) {
		withFakeLazygit(t)
		m, spy, _ := repoModel(t)
		t.Setenv("PATH", t.TempDir()) // no but

		mm, _ := press(*m, "g")
		mm, cmd := press(mm, "b")
		msg, ok := cmd().(notifyMsg)
		if !ok || msg.level != toastWarning || !strings.Contains(msg.text, "but not installed") {
			t.Errorf("notice = %+v (%v), want but not installed", msg, ok)
		}
		if len(mm.running) != 0 || spy.vals != 0 {
			t.Errorf("running = %v, handoffs = %d, want nothing launched", mm.running, spy.vals)
		}
	})
}

func TestLaunchGitbutler(t *testing.T) {
	withFakeTool(t, "but")
	m, spy, p := repoModel(t)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	rec := cmdlog.Active()
	if rec == nil {
		t.Fatal("no command log recorder")
	}

	mm, _ := press(*m, "g")
	mm, cmd := press(mm, "b")
	if cmd == nil {
		t.Fatal("g b returned no tea.Cmd")
	}
	if mm.running[p.Path] != "gitbutler" {
		t.Fatalf("running = %q, want gitbutler", mm.running[p.Path])
	}
	if spy.vals != 1 || spy.dir != p.Path {
		t.Errorf("handoff = %d runs in %q, want 1 in %q", spy.vals, spy.dir, p.Path)
	}
	if len(spy.argv) != 2 || spy.argv[0] != "but" || spy.argv[1] != "tui" {
		t.Errorf("argv = %q, want [but tui]", spy.argv)
	}
	if msg, ok := spy.finish(nil).(execDoneMsg); !ok || msg.action != "gitbutler" {
		t.Errorf("execDoneMsg = %+v (%v), want action gitbutler", msg, ok)
	}
	e := entryFor(t, rec, "gitbutler")
	if !e.Intent || e.Key != "b" || e.Dir != p.Path {
		t.Errorf("command log entry = %+v, want the intent of key b in %s", e, p.Path)
	}
}

// esc cancels like every armed state, and the prefix can be armed again right after.
func TestLaunchEscCancels(t *testing.T) {
	withFakeLazygit(t)
	m, spy, _ := repoModel(t)

	mm, _ := press(*m, "g")
	mm, _ = press(mm, "esc")
	if mm.launchArmed != nil {
		t.Error("esc did not clear the arm")
	}
	if len(mm.running) != 0 || spy.vals != 0 {
		t.Errorf("esc launched something: %v", mm.running)
	}
	mm, _ = press(mm, "g")
	if mm.launchArmed == nil {
		t.Fatal("the prefix does not arm again after esc")
	}
}

// The sub-keys come from the config, so a rebind reaches the dispatch AND the prompt, and the old key no longer launches (it falls through to its normal course).
func TestLaunchSubkeysAreRebindable(t *testing.T) {
	path := jjRepo(t)
	withFakeTool(t, "lazyjj")
	m, spy := jjModel(t, path)
	m.cfg.Keybindings["lazyjj"] = "x"

	mm, _ := press(*m, "g")
	if got := mm.launchPrompt(); !strings.Contains(got, "x lazyjj") || strings.Contains(got, "j lazyjj") {
		t.Errorf("prompt = %q, want it to advertise the rebound key only", got)
	}
	mm, _ = press(mm, "j") // no longer a sub-key: it cancels and moves the cursor
	if mm.running[path] != "" {
		t.Errorf("the old key still launched lazyjj: %v", mm.running)
	}

	mm, _ = press(*m, "g")
	mm, cmd := press(mm, "x")
	if cmd == nil {
		t.Fatal("the rebound key returned no tea.Cmd")
	}
	if mm.running[path] != "lazyjj" {
		t.Errorf("running = %q, want lazyjj through the rebound key", mm.running[path])
	}
	if spy.vals != 1 {
		t.Errorf("handoff ran %d times, want 1", spy.vals)
	}
}

// The launchers check the binary BEFORE the busy guard, or a machine without the tool would be told something else is already running there.
func TestLaunchBusyGuard(t *testing.T) {
	withFakeTool(t, "but")
	m, spy, p := repoModel(t)
	m.running = map[string]string{p.Path: "pull"}

	cmd := m.openGitbutlerCmd(p.Path)
	msg, ok := cmd().(notifyMsg)
	if !ok || !strings.Contains(msg.text, "already running") || !strings.Contains(msg.text, "pull") {
		t.Errorf("notice = %+v (%v), want a warning naming the running action", msg, ok)
	}
	if spy.vals != 0 {
		t.Errorf("it ran %d handoffs, want 0", spy.vals)
	}
	if m.running[p.Path] != "pull" {
		t.Errorf("running = %q, want the original action untouched", m.running[p.Path])
	}
}

// The panel and the PR overlay drop every armed state, and a launcher arm left alive would wait for a key inside a view that no longer shows the warning.
func TestLaunchArmIsDroppedByTheOtherViews(t *testing.T) {
	withFakeLazygit(t)
	m, _, _ := repoModel(t)

	mm, _ := press(*m, "g")
	mm.toggleLog()
	if mm.launchArmed != nil {
		t.Error("opening the log panel left the launch arm alive")
	}

	mm2, _ := press(*m, "g")
	mm2, _ = press(mm2, "O")
	if mm2.pr == nil {
		t.Fatal("precondition: the PR form did not open")
	}
	if mm2.launchArmed != nil {
		t.Error("opening the PR form left the launch arm alive")
	}
}
