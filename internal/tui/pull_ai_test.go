package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func newAIModel(t *testing.T, dir, prompt, command string) Model {
	t.Helper()
	content := ""
	if prompt != "" {
		content = "[ai.pull]\nprompt = \"" + prompt + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p := proj("demo", dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapClean()})
	if command != "" {
		m.cfg.AICommands = map[string]string{"pull": command}
	}
	return cursorOn(t, m, dir)
}

func TestPullAINotLaunchesWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "", "/bin/echo {prompt}")

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a launched the handoff without a prompt: running = %q", m.running[dir])
	}
	if m.pullArmed != nil {
		t.Error("the selector stayed armed after picking the AI variant")
	}
	if cmd == nil {
		t.Fatal("without a prompt it should notify")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no AI pull prompt") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestPullAINotLaunchesWithoutCommand(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix the rebase", "")

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a launched the handoff without a configured command: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("without a command it should notify")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "command not configured") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestPullAINotLaunchesWithoutBinary(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix the rebase", "definitely-not-a-bin-xyz {prompt}")

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a launched the handoff with a nonexistent binary: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("without a binary it should notify")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "not installed") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestPullAILaunchesTheHandoffDirect(t *testing.T) {
	if _, err := exec.LookPath("/bin/echo"); err != nil {
		t.Skip("/bin/echo not available")
	}
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix the rebase", "/bin/echo {prompt}")

	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("precondition: p did not arm the selector")
	}
	m, cmd := press(m, "a")

	if m.pullArmed != nil {
		t.Error("the selector stays armed: a did not launch directly")
	}
	if m.running[dir] != "pull_ai" {
		t.Errorf("running = %q, want pull_ai", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("the handoff returned no tea.Cmd")
	}
}

func TestPullAIBlockedIfAlreadyRuns(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix the rebase", "/bin/echo {prompt}")
	m.running[dir] = "lazygit"

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if m.running[dir] != "lazygit" {
		t.Errorf("running = %q, want the original untouched", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("with the action already running it should notify")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestSelectorEscAndKeyNotVariant(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "p")
	m, _ = press(m, "esc")
	if m.pullArmed != nil || len(m.running) != 0 {
		t.Errorf("esc left state: armed=%v running=%v", m.pullArmed, m.running)
	}

	m2 := newPullModel(t)
	start := m2.cursor
	m2, _ = press(m2, "p")
	m2, _ = press(m2, "j")
	if m2.pullArmed != nil {
		t.Error("the non-variant key did not cancel the selector")
	}
	if m2.cursor == start {
		t.Error("the non-variant key did not follow its normal course (cursor still)")
	}
	if len(m2.running) != 0 {
		t.Errorf("the cancel launched an action: %v", m2.running)
	}
}

func TestPullAINotIsPullKind(t *testing.T) {
	if IsPullKind("pull_ai") {
		t.Error("pull_ai should not be a PullKind")
	}
	if len(PullKinds) != 4 {
		t.Errorf("PullKinds = %d, want 4 (p/r/f/m)", len(PullKinds))
	}
	if _, err := exec.LookPath("/bin/echo"); err != nil {
		t.Skip("/bin/echo not available")
	}
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix", "/bin/echo {prompt}")
	m, _ = press(m, "p")
	m, _ = press(m, "a")
	if got := m.running[dir]; got != "pull_ai" {
		t.Errorf("running = %q, want pull_ai", got)
	}
}

func TestPullAINotLaunchesWithExecutableEmpty(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix the rebase", "{branch} {prompt}")
	m.states[dir] = gitstatus.Snapshot{} // no branch: {branch} -> ""

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a launched the handoff with an empty executable: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("it should notify")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "empty executable") {
		t.Errorf("notification = %v, want the empty-executable warning", cmd())
	}
}

func TestPullAIAiVarsWorktreeWithoutSnapshot(t *testing.T) {
	p, st := repoWithWorktrees("multi", "/tmp/multi",
		wt("/tmp/wt-a", "feat/x"), wt("/tmp/wt-d", ""))
	m := newTestModel(t, []discovery.Project{p}, st)

	vars := m.aiVars("/tmp/wt-a")
	if vars["branch"] != "feat/x" {
		t.Errorf("branch = %q, want feat/x (the one from `git worktree list`)", vars["branch"])
	}
	if vars["state"] != "" {
		t.Errorf("state = %q, want empty (no state is invented)", vars["state"])
	}
	if vars["upstream"] != "" || vars["ahead"] != "" || vars["behind"] != "" {
		t.Errorf("invented placeholders: %+v", vars)
	}
	if got := m.aiVars("/tmp/wt-d")["branch"]; got != "(detached) abc1234" {
		t.Errorf("branch detached = %q, want \"(detached) abc1234\"", got)
	}
}

func TestPullAIArgvFromTheMarkerReal(t *testing.T) {
	dir := t.TempDir()
	testutil.Init(t, dir)
	content := "[ai.pull]\nprompt = \"line1\\nline2 \\\"with quotes\\\" and $VAR; rm -rf /\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	p := proj("demo", dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapClean()})
	m.cfg.AICommands = map[string]string{"pull": "jcode -run {prompt}"}

	prompt, err := discovery.MarkerPrompt(dir, m.cfg.Marker, "pull")
	if err != nil {
		t.Fatalf("MarkerPrompt: %v", err)
	}
	want := "line1\nline2 \"with quotes\" and $VAR; rm -rf /"
	if prompt != want {
		t.Fatalf("marker prompt = %q, want %q", prompt, want)
	}

	argv := m.pullAIArgv(dir, prompt)
	if len(argv) != 3 || argv[2] != want {
		t.Errorf("argv = %#v, want the whole prompt as the last element", argv)
	}
}

func TestPullPromptIncludesAI(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "p")
	got := m.pullPrompt()
	for _, want := range []string{"p default", "r rebase", "f ff-only", "m merge", "a AI", "esc cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not mention %q:\n%s", want, got)
		}
	}
}

func TestPullAIPromptAOnlyArgv(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "x", "jcode -run {prompt}")

	prompt := "first line\nsecond \"with quotes\" and $VAR; rm -rf /"
	argv := m.pullAIArgv(dir, prompt)
	want := []string{"jcode", "-run", prompt}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v, want %#v", argv, want)
	}
}

func TestPullAIAiVarsResolvesTheContext(t *testing.T) {
	dir := t.TempDir()
	m := newAIModel(t, dir, "fix",
		"ai --branch {branch} --upstream {upstream} --state {state} --ahead {ahead} --behind {behind} --sync {sync} {prompt}")
	m.states[dir] = gitstatus.Snapshot{
		Status: gitstatus.Status{
			Branch: "feat/x", Upstream: "origin/feat/x", HasUpstream: true,
			Ahead: 2, Behind: 1,
		},
		SyncBranch: "main",
	}

	argv := m.pullAIArgv(dir, "fix")
	want := []string{
		"ai", "--branch", "feat/x", "--upstream", "origin/feat/x",
		"--state", "diverged", "--ahead", "2", "--behind", "1",
		"--sync", "main", "fix",
	}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %#v\nwant %#v", argv, want)
	}
}

func TestExecDoneRecordsPullAI(t *testing.T) {
	m, rec := logModel(t)
	path := "/tmp/old-clean"
	argv := []string{"jcode", "-run", "fix the rebase"}

	updated, _ := m.Update(execDoneMsg{path: path, action: "pull_ai", argv: argv, err: nil})
	_ = updated.(Model)

	// The background recollect adds reads behind, so the pull_ai entry is not assumed to be the last one: it is looked up by action.
	var got *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "pull_ai" {
			continue
		}
		cp := e
		got = &cp
	}
	if got == nil {
		t.Fatal("the pull_ai exec was not recorded")
	}
	if !reflect.DeepEqual(got.Argv, argv) {
		t.Errorf("argv = %#v, want %#v", got.Argv, argv)
	}
	if got.Dur != 0 {
		t.Errorf("Dur = %v, want 0 (handoff)", got.Dur)
	}
	if got.Exit != 0 {
		t.Errorf("Exit = %d, want 0", got.Exit)
	}
	if got.Class != cmdlog.ClassAction || got.Dir != path {
		t.Errorf("class/dir = %v/%q", got.Class, got.Dir)
	}
}

// An unreadable marker is not "no prompt": it is a broken `.gitdash.toml` the user has to fix, and falling into the empty-prompt warning would make them think writing the prompt is all that is missing.
func TestPullAIWarnsOfMarkerBroken(t *testing.T) {
	dir := t.TempDir()
	// A DIRECTORY named after the marker reads just as badly as a 0o000, without depending on who runs the test (root reads a 0o000).
	if err := os.MkdirAll(filepath.Join(dir, ".gitdash.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, []discovery.Project{proj("demo", dir, true)},
		map[string]gitstatus.Snapshot{dir: snapClean()})
	m.cfg.AICommands = map[string]string{"pull": "/bin/echo {prompt}"}
	m = cursorOn(t, m, dir)

	m, _ = press(m, "p")
	m, cmd := press(m, "a")

	if _, busy := m.running[dir]; busy {
		t.Errorf("a launched the handoff with a broken marker: running = %q", m.running[dir])
	}
	if cmd == nil {
		t.Fatal("a broken marker should notify")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "marker error") {
		t.Errorf("notification = %v, want the broken-marker warning", cmd())
	}
}
