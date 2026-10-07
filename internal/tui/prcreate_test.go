package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/forge"
	"gitdash/internal/forge/tool"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func prRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Init(t, dir)
	if remote != "" {
		gitOutT(t, dir, "remote", "add", "origin", remote)
	}
	return dir
}

func prModel(t *testing.T, dir string) (Model, *cmdlog.Recorder) {
	t.Helper()
	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapDirty(1, 0)})
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })
	return cursorOn(t, m, dir), cmdlog.Active()
}

// A real forge CLI cannot be exercised without network or token: what is tested is what gitdash does around it (the argv it builds, the one it runs and what it records), and the stub closes that circle without going out to the network.
func forgeStub(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argvFile + "\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile
}

func sinBinario(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func prConfig(t *testing.T, toml string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := config.LoadFrom(path)
	if warn != "" {
		t.Fatalf("config: %s", warn)
	}
	return cfg
}

func prSubmitsPR(t *testing.T, m Model, title string) (Model, tea.Cmd) {
	t.Helper()
	m = openPROverlay(t, m)
	m = typeText(m, title)
	m, cmd := press(m, prSubmitKey)
	if cmd == nil {
		t.Fatal("the submit returned no cmd")
	}
	msg := cmd()
	if _, ok := msg.(prStartMsg); !ok {
		t.Fatalf("submit = %T, want prStartMsg", msg)
	}
	out, start := m.Update(msg)
	return out.(Model), start
}

// Without waiting for it, that Collect is still alive when the test ends and can log its reads into the next test's recorder.
func awaitPR(t *testing.T, m *Model) prResultMsg {
	t.Helper()
	var res prResultMsg
	waitEvent(t, m, func(ev event) bool {
		r, ok := ev.(prResultMsg)
		if ok {
			res = r
		}
		return ok
	})
	if res.reject == "" {
		waitEvent(t, m, func(ev event) bool {
			_, ok := ev.(statusMsg)
			return ok
		})
	}
	return res
}

func prExec(rec *cmdlog.Recorder) *cmdlog.Entry {
	var got *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "pr" {
			continue
		}
		cp := e
		got = &cp
	}
	return got
}

func lastToast(m Model) string {
	ts := m.toasts.toasts
	if len(ts) == 0 {
		return ""
	}
	return ts[len(ts)-1].text
}

func toastBlock(m Model) string {
	return strings.Join(m.toasts.lines(), "\n")
}

func TestPRCreationCompletesRecordsTheArgvResolved(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	argvFile := forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/42")
	m, rec := prModel(t, dir)

	m, _ = prSubmitsPR(t, m, "Add the sync branch base")
	if m.running[dir] != "pr" {
		t.Fatalf("running = %q, want pr (the creation blocks the second press)", m.running[dir])
	}

	res := awaitPR(t, &m)
	if res.reject != "" {
		t.Fatalf("the creation was rejected: %s", res.reject)
	}
	if res.err != nil {
		t.Fatalf("gh failed: %v", res.err)
	}

	e := prExec(rec)
	if e == nil {
		t.Fatal("the creation's exec was not recorded")
	}
	want := []string{
		"gh", "pr", "create",
		"-t", "Add the sync branch base",
		"-b", "",
		"-B", "main",
		"-H", "main",
		"-R", "acme/widget",
	}
	if !reflect.DeepEqual(e.Argv, want) {
		t.Errorf("argv = %#v\nwant %#v", e.Argv, want)
	}
	if e.Class != cmdlog.ClassAction {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassAction)
	}
	if e.Exit != 0 {
		t.Errorf("Exit = %d, want 0", e.Exit)
	}
	if e.Dur <= 0 {
		t.Error("Dur left unmeasured: the creation is not a terminal handoff")
	}
	if e.Repo != filepath.Base(dir) || e.Dir != dir {
		t.Errorf("Repo/Dir = %q/%q, want %q/%q", e.Repo, e.Dir, filepath.Base(dir), dir)
	}
	ran, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("the stub left no argv behind: %v", err)
	}
	if got := strings.Split(strings.TrimRight(string(ran), "\n"), "\n"); got[0] != "pr" || got[1] != "create" {
		t.Errorf("executed argv = %v, want gh pr create's", got)
	}
	if m.running[dir] != "" {
		t.Errorf("the repo is still busy: %q", m.running[dir])
	}
	if got := lastToast(m); !strings.Contains(got, "https://github.com/acme/widget/pull/42") {
		t.Errorf("toast = %q, want the PR's URL", got)
	}
}

func TestPRLeavesIntentInTheLog(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m, _ = prSubmitsPR(t, m, "a title")
	awaitPR(t, &m)

	var intent *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent && e.Action == "pr" {
			cp := e
			intent = &cp
		}
	}
	if intent == nil {
		t.Fatal("the pr action left no intent in the log")
	}
	if intent.Key != "O" {
		t.Errorf("Key = %q, want O (the action's key)", intent.Key)
	}
	if intent.Dir != dir {
		t.Errorf("Dir = %q, want the row's repo", intent.Dir)
	}
}

func TestPRNotOpensWithTheLogOpen(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	m, _ = press(m, "l") // opens the log panel
	before := len(rec.Entries())

	m, cmd := press(m, "O")

	if m.pr != nil {
		t.Error("the overlay opened with the log panel open")
	}
	if cmd != nil {
		t.Errorf("the key launched something: %v", cmd())
	}
	if len(rec.Entries()) != before {
		t.Error("the key left a log entry for something that did not happen")
	}
}

func TestPRTheHeadExitsOfTheSnapshot(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	snap := m.states[dir]
	snap.Status.Branch = "feat/pr"
	m.states[dir] = snap

	m, _ = prSubmitsPR(t, m, "a title")
	awaitPR(t, &m)

	e := prExec(rec)
	if e == nil {
		t.Fatal("the exec was not recorded")
	}
	if !containsPair(e.Argv, "-H", "feat/pr") {
		t.Errorf("argv = %v, want -H feat/pr", e.Argv)
	}
}

func TestPRTheArgvOfThePanelGoesSanitized(t *testing.T) {
	// Two different paths to the same limit. A paste with an OSC (which hijacks the terminal title) goes through no filter at all: the bubbles widget strips control characters but not sequences, and the panel cannot rely on every path filtering. Format characters (Cf: bidi, zero-width) do arrive through the form's real path, because they are not control characters.
	cases := []struct {
		name     string
		title    string
		visible  string   // the part of the title that must still be visible
		pasted   bool     // the title arrives via the submission, not via the input
		injected []string // what must NOT appear in the view
	}{
		{"osc", "\x1b]0;hijacked\x07red final", "red final", true, []string{"\x1b]0;"}},
		{"bidi", "title\u202Emid\u202C", "title", false, []string{"\u202e", "\u202c"}},
		{"zero-width", "before\u200Bafter", "before", false, []string{"\u200b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := prRepo(t, "git@github.com:acme/widget.git")
			forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/7")
			m, rec := prModel(t, dir)

			if c.pasted {
				m.prPending = &prSubmission{path: dir, params: forge.Params{
					Title: c.title, Base: "main", Head: "main",
				}}
				out, _ := m.Update(prStartMsg{})
				m = out.(Model)
			} else {
				m = openPROverlay(t, m)
				m.pr.title.SetValue(c.title)
				var cmd tea.Cmd
				m, cmd = press(m, prSubmitKey)
				if cmd == nil {
					t.Fatal("the submit returned no cmd")
				}
				out, _ := m.Update(cmd())
				m = out.(Model)
			}
			awaitPR(t, &m)

			e := prExec(rec)
			if e == nil {
				t.Fatal("the exec was not recorded")
			}
			if !containsPair(e.Argv, "-t", c.title) {
				t.Fatalf("argv = %v, want the whole title as one element", e.Argv)
			}

			m, _ = press(m, "l") // opens the log panel
			painted := m.View().Content
			for _, s := range c.injected {
				if strings.Contains(painted, s) {
					t.Errorf("the untrusted text %q reached the view", s)
				}
			}
			visible := stripANSI(painted)
			if !strings.Contains(visible, "pr create") {
				t.Errorf("the panel did not paint the argv:\n%s", visible)
			}
			if !strings.Contains(visible, c.visible) {
				t.Errorf("the sanitising took the whole argv away:\n%s", visible)
			}
		})
	}
}

func TestPRWithoutRemoteNotRunsNothing(t *testing.T) {
	dir := prRepo(t, "") // repo without origin
	argvFile := forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m, _ = prSubmitsPR(t, m, "a title")
	res := awaitPR(t, &m)

	if res.reject == "" {
		t.Fatal("without a remote it should be rejected")
	}
	if !strings.Contains(res.reject, "no origin remote") {
		t.Errorf("notice = %q, want it to mention the missing remote", res.reject)
	}
	if prExec(rec) != nil {
		t.Error("an exec was recorded with no remote")
	}
	if _, err := os.Stat(argvFile); err == nil {
		t.Error("the CLI ran with no remote")
	}
	if m.running[dir] != "" {
		t.Errorf("the repo was left busy: %q", m.running[dir])
	}
	if got := lastToast(m); !strings.Contains(got, "no origin remote") {
		t.Errorf("toast = %q, want the remote warning", got)
	}
}

func TestPRForgeUnknownNotRunsNothing(t *testing.T) {
	dir := prRepo(t, "git@git.example.com:acme/widget.git")
	argvFile := forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m, _ = prSubmitsPR(t, m, "a title")
	res := awaitPR(t, &m)

	if res.reject == "" {
		t.Fatal("an undeclared host should be rejected")
	}
	for _, want := range []string{"no forge", "[forge.github]", "[forge.gitlab]"} {
		if !strings.Contains(res.reject, want) {
			t.Errorf("notice = %q, want mentions %q", res.reject, want)
		}
	}
	if prExec(rec) != nil {
		t.Error("an exec was recorded with an unknown forge")
	}
	if _, err := os.Stat(argvFile); err == nil {
		t.Error("the CLI ran with no known forge")
	}
}

func TestPRWithoutBinaryNotRunsNothing(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	sinBinario(t)
	m, rec := prModel(t, dir)

	m, _ = prSubmitsPR(t, m, "a title")
	res := awaitPR(t, &m)

	if !strings.Contains(res.reject, "gh not installed") {
		t.Errorf("notice = %q, want \"gh not installed\"", res.reject)
	}
	if prExec(rec) != nil {
		t.Error("an exec was recorded with no binary")
	}
	if m.running[dir] != "" {
		t.Errorf("the repo was left busy: %q", m.running[dir])
	}
}

func TestPRNotRelaunchWithTheRepoBusy(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	m.running[dir] = "lazygit"

	m, cmd := prSubmitsPR(t, m, "a title")
	m = applyNotify(m, cmd)

	if m.running[dir] != "lazygit" {
		t.Errorf("running = %q, want the original action untouched", m.running[dir])
	}
	if m.prPending != nil {
		t.Error("the submit was queued: it would run later without being asked")
	}
	if prExec(rec) != nil {
		t.Error("an exec was recorded with the repo busy")
	}
	if got := lastToast(m); !strings.Contains(got, "already running") {
		t.Errorf("toast = %q, want the action-in-progress warning", got)
	}
}

func TestPRGitLabSelfManagedRemovesThePrefixOfSubfolder(t *testing.T) {
	dir := prRepo(t, "git@git.example.com:group/sub/widget.git")
	argvFile := forgeStub(t, "glab", "echo https://git.example.com/git/group/sub/widget/-/merge_requests/3")
	m, rec := prModel(t, dir)
	m.cfg.Forges = prConfig(t, `
[forge.gitlab]
enabled = true
host = "git.example.com"
api_base = "/git/api/v4/"
`).Forges

	m, _ = prSubmitsPR(t, m, "Fix the subfolder")

	res := awaitPR(t, &m)
	if res.reject != "" {
		t.Fatalf("the creation was rejected: %s", res.reject)
	}
	if res.err != nil {
		t.Fatalf("glab failed: %v", res.err)
	}

	e := prExec(rec)
	if e == nil {
		t.Fatal("the exec was not recorded")
	}
	want := []string{
		"glab", "mr", "create",
		"-t", "Fix the subfolder",
		"-d", "",
		"-b", "main",
		"-s", "main",
		"-y", "-R", "group/sub/widget",
	}
	if !reflect.DeepEqual(e.Argv, want) {
		t.Errorf("argv = %#v\nwant %#v", e.Argv, want)
	}
	ran, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("the stub left no argv behind: %v", err)
	}
	if got := strings.Split(strings.TrimRight(string(ran), "\n"), "\n"); got[0] != "mr" {
		t.Errorf("executed argv = %v, want glab mr create's", got)
	}
}

func TestPRFailureOfTheCLIToastWithTheReason(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo 'could not create PR: head branch already exists' >&2\nexit 1")
	m, rec := prModel(t, dir)

	m, _ = prSubmitsPR(t, m, "a title")
	res := awaitPR(t, &m)

	if res.err == nil {
		t.Fatal("the creation should have failed")
	}
	if got := lastToast(m); !strings.Contains(got, "head branch already exists") {
		t.Errorf("toast = %q, want the CLI's reason", got)
	}
	e := prExec(rec)
	if e == nil {
		t.Fatal("the failure was not recorded in the log")
	}
	if e.Exit != 1 {
		t.Errorf("Exit = %d, want 1", e.Exit)
	}
	if m.running[dir] != "" {
		t.Errorf("the repo was left busy: %q", m.running[dir])
	}
}

// A failure painted with the ✓ of success is worse than not warning, because the user does not look again; both branches are checked, which is what ties them to the result, since with only one an inverted `== toastSuccess` would keep painting half the cases right.
func TestPRTheWarningOfTheOutcomeCarriesItsLevel(t *testing.T) {
	cases := []struct {
		name string
		stub string
		want toastLevel
	}{
		{"success", "echo https://github.com/acme/widget/pull/42", toastSuccess},
		{"failure", "echo 'could not create PR: head branch already exists' >&2\nexit 1", toastError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := prRepo(t, "git@github.com:acme/widget.git")
			forgeStub(t, "gh", c.stub)
			m, _ := prModel(t, dir)

			m, _ = prSubmitsPR(t, m, "a title")
			res := awaitPR(t, &m)
			if res.reject != "" {
				t.Fatalf("the creation was rejected: %s", res.reject)
			}
			if c.want == toastError && res.err == nil {
				t.Fatal("the creation should have failed")
			}
			if c.want == toastSuccess && res.err != nil {
				t.Fatalf("the creation should not have failed: %v", res.err)
			}

			ts := m.toasts.toasts
			if len(ts) == 0 {
				t.Fatal("no notice was left")
			}
			last := ts[len(ts)-1]
			if last.level != c.want {
				t.Errorf("notice level = %v (%s), want %v", last.level, last.text, c.want)
			}
			painted := stripANSI(toastBlock(m))
			if !strings.Contains(painted, toastIcon(c.want)) {
				t.Errorf("the notice does not carry the %v icon:\n%s", c.want, painted)
			}
			other := toastSuccess
			if c.want == toastSuccess {
				other = toastError
			}
			if strings.Contains(painted, toastIcon(other)) {
				t.Errorf("the painted notice carries the opposite level's icon:\n%s", painted)
			}
		})
	}
}

func TestPRPromptNamesTheKeyOfTheConfig(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m.cfg.Keybindings["pr"] = "W"

	got := m.prPrompt()

	if !strings.Contains(got, "W opens") {
		t.Errorf("the notice does not name the configured key:\n%s", got)
	}
	if strings.Contains(got, "O opens") {
		t.Errorf("the notice names the default key:\n%s", got)
	}
	kb := sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if !strings.Contains(kb, "W opens") {
		t.Errorf("keybinds without the configured key:\n%s", kb)
	}
}

func TestPRSendInvalidNotLaunches(t *testing.T) {
	dir := prRepo(t, "git@github.com:acme/widget.git")
	forgeStub(t, "gh", "echo https://github.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)

	m = openPROverlay(t, m)
	m, cmd := press(m, prSubmitKey)

	if cmd != nil {
		t.Errorf("an invalid submit launched something: %v", cmd())
	}
	if m.prPending != nil {
		t.Error("a submit with no title was published")
	}
	if prExec(rec) != nil {
		t.Error("an exec was recorded for an invalid submit")
	}
}

func containsPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func TestPRGitReasonGetsTheReasonOfTheArgv(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want string
	}{
		{
			"with no remote there is no reason: the notice already says it",
			"git [remote get-url origin]: No such remote 'origin'",
			"",
		},
		{
			"a broken repo shows its reason, without the argv",
			"git [remote get-url origin]: fatal: not a git repository",
			"fatal: not a git repository",
		},
		{
			"git absent too (the reason comes from exec, without the prefix)",
			"exec: \"git\": executable file not found in $PATH",
			"exec: \"git\": executable file not found in $PATH",
		},
		{
			"a reason with git in front but no brace is left whole",
			"git something rare",
			"git something rare",
		},
		{
			"a brace in the middle does not trim",
			"wrapped: git [x]: detail",
			"wrapped: git [x]: detail",
		},
	}
	for _, c := range cases {
		if got := prGitReason(errors.New(c.err)); got != c.want {
			t.Errorf("%s: prGitReason = %q, want %q", c.name, got, c.want)
		}
	}
}

// A toast mixing both cases would leave the user configuring a remote that does exist.
func TestPRRemoteRejectDistinguishesTheCaseNormal(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want string
	}{
		{
			"no remote: the action, not git's error",
			"git [remote get-url origin]: No such remote 'origin'",
			"no origin remote in widget — configure one first",
		},
		{
			"another reason: the reason, with the read prefix",
			"git [remote get-url origin]: fatal: not a git repository",
			"widget: cannot read origin — fatal: not a git repository",
		},
	}
	for _, c := range cases {
		if got := prRemoteReject("widget", errors.New(c.err)); got != c.want {
			t.Errorf("%s: prRemoteReject = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPRURLYNoteOfTheOutcome(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"the URL is the first line", "https://github.com/a/b/pull/1\n", "https://github.com/a/b/pull/1"},
		{"gh leaves text before", "Creating pull request...\nhttps://github.com/a/b/pull/7\n", "https://github.com/a/b/pull/7"},
		{"http also works", "http://g.c/a/b/-/merge_requests/2", "http://g.c/a/b/-/merge_requests/2"},
		{"with no URL there is nothing to return", "all good\n", ""},
		{"a path that is not a URL is not mistaken for one", "to see it: /a/b/pull/9\n", ""},
		{"http glued to something else does not count", "verhttp://x\n", ""},
		{"https with one slash missing is not a URL", "https:/g.c/a/b\n", ""},
	}
	for _, c := range cases {
		if got := prURL(c.out); got != c.want {
			t.Errorf("%s: prURL = %q, want %q", c.name, got, c.want)
		}
	}

	level, msg := prNote("widget", prResultMsg{out: "https://github.com/a/b/pull/1"})
	if level != toastSuccess || !strings.Contains(msg, "https://github.com/a/b/pull/1") {
		t.Errorf("success with no visible link: (%v, %q)", level, msg)
	}
	if level, msg := prNote("widget", prResultMsg{out: "done"}); level != toastSuccess || strings.Contains(msg, "http") || strings.Contains(msg, "—") {
		t.Errorf("output with no URL: (%v, %q), want a success toast with no link", level, msg)
	}
	cerr := &tool.Error{Bin: "gh", Args: []string{"pr", "create", "-t", "secret title", "-b", "long body"}, ExitCode: 1, Msg: "no commits between main and feat"}
	level, msg = prNote("widget", prResultMsg{err: cerr})
	if level != toastError {
		t.Errorf("failure: level = %v, want error", level)
	}
	if !strings.Contains(msg, "no commits between main and feat") {
		t.Errorf("the failure does not show the reason: %q", msg)
	}
	if strings.Contains(msg, "secret title") || strings.Contains(msg, "long body") {
		t.Errorf("the failure repeats the argv with the body inside: %q", msg)
	}
	if _, msg := prNote("widget", prResultMsg{err: errors.New("context deadline exceeded")}); !strings.Contains(msg, "deadline") {
		t.Errorf("a loose error does not reach the notice: %q", msg)
	}
}

// The case comes from the seam's own shape: the overlay publishes the submission and the Cmd runs one tick later, so a rescan can requeue in between, and without this guard that tick would run the last submission again.
func TestPRCreateCmdWithoutSendNotMakesNothing(t *testing.T) {
	m, _ := prModel(t, prRepo(t, "git@github.com:acme/widget.git"))
	m.prPending = nil
	if cmd := m.prCreateCmd(); cmd != nil {
		t.Errorf("prCreateCmd with no submit = %v, want nil", cmd)
	}
}

// Reachable through the API: `Config.Forges` is an exported field, so a consumer can leave a provider the TOML loader would have discarded (config.LoadFrom warns and ignores it), which is why prCreateCmd's guard is not dead code and why it has a test: without it an empty argv would reach the Runner.
func TestPRForgeInTheMapWithoutDoorNotRunsNothing(t *testing.T) {
	dir := prRepo(t, "git@bit.example.com:acme/widget.git")
	argvFile := forgeStub(t, "gh", "echo https://bit.example.com/acme/widget/pull/1")
	m, rec := prModel(t, dir)
	m.cfg.Forges = map[string]config.ForgeConfig{
		"bitbucket": {Enabled: true, Host: "bit.example.com"},
	}

	m, _ = prSubmitsPR(t, m, "a title")
	res := awaitPR(t, &m)

	if !strings.Contains(res.reject, "has no PR support in gitdash") {
		t.Errorf("notice = %q, want it to name that there is no PR support", res.reject)
	}
	if prExec(rec) != nil {
		t.Error("an exec was recorded for a forge with no door")
	}
	if _, err := os.Stat(argvFile); err == nil {
		t.Error("the CLI ran for a forge with no door")
	}
}
