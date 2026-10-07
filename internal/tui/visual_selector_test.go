package tui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func withVisualEnv(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	exe := filepath.Join(bin, "git-sim")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The fake comes first so it beats the machine's real git-sim, and the original PATH is kept because the fixtures need `git`.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func TestVisualArmsWithoutRun(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	before := len(m.running)

	m, _ = press(m, "v")

	if m.visualArmed == nil {
		t.Fatal("v did not arm the visual selector")
	}
	if m.visualArmed.path != "/tmp/old-clean" {
		t.Errorf("armed path = %q, want /tmp/old-clean", m.visualArmed.path)
	}
	if m.visualArmed.upstream != "origin/main" {
		t.Errorf("armed upstream = %q, want origin/main", m.visualArmed.upstream)
	}
	if len(m.running) != before {
		t.Errorf("v launched something: running = %v, want no changes", m.running)
	}
}

func TestVisualNotArmsWithoutRow(t *testing.T) {
	m := newTestModel(t, nil, map[string]gitstatus.Snapshot{})
	_, cmd := press(m, "v")
	if m.visualArmed != nil {
		t.Errorf("the selector armed with no row: %+v", m.visualArmed)
	}
	if cmd == nil {
		t.Fatal("with no row it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestVisualNotArmsWithoutRepo(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/no-repo-docs")
	_, cmd := press(m, "v")
	if m.visualArmed != nil {
		t.Errorf("the selector armed on a row without a repo: %+v", m.visualArmed)
	}
	if cmd == nil {
		t.Fatal("with no repo it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestVisualKeyNotVariantCancelsAndFollows(t *testing.T) {
	m := newPullModel(t)
	start := m.cursor
	m, _ = press(m, "v")
	if m.visualArmed == nil {
		t.Fatal("precondition: it was not armed")
	}
	m, _ = press(m, "j")
	if m.visualArmed != nil {
		t.Error("the selector stays armed after a non-variant key")
	}
	if m.cursor == start {
		t.Error("the non-variant key did not run its action (cursor still)")
	}
	if len(m.running) != 0 {
		t.Errorf("the cancel launched something: %v", m.running)
	}
}

func TestVisualEscCancels(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "v")
	m, _ = press(m, "esc")
	if m.visualArmed != nil {
		t.Error("esc did not cancel the selector")
	}
	if len(m.running) != 0 {
		t.Errorf("esc launched something: %v", m.running)
	}
}

func TestVisualResolvesAboutTheArmed(t *testing.T) {
	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")
	m = cursorOn(t, m, "/tmp/dirty-api")

	m, cmd := press(m, "p")
	if cmd == nil {
		t.Fatal("the variant launched nothing")
	}
	if m.running["/tmp/old-clean"] != "visual" {
		t.Errorf("running = %q, want visual in /tmp/old-clean", m.running["/tmp/old-clean"])
	}
	if _, ok := m.running["/tmp/dirty-api"]; ok {
		t.Error("the variant resolved on the row under the cursor, not on the armed one")
	}
}

func TestVisualDispatchesEachVariant(t *testing.T) {
	for _, key := range []string{"p", "m", "r"} {
		withVisualEnv(t)
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/behind-web")
		m, _ = press(m, "v")

		m, cmd := press(m, key)
		if m.visualArmed != nil {
			t.Errorf("v%s left the selector armed", key)
		}
		if m.running["/tmp/behind-web"] != "visual" {
			t.Errorf("v%s → running = %q, want visual", key, m.running["/tmp/behind-web"])
		}
		if cmd == nil {
			t.Errorf("v%s returned no tea.Cmd", key)
		}
	}
}

func TestVisualArgvVariants(t *testing.T) {
	dir := filepath.Join("/cache", "gitdash", "git-sim")
	cases := []struct {
		sub      string
		upstream string
		want     []string
	}{
		{"pull", "origin/main", []string{"git-sim", "--media-dir", dir, "pull"}},
		{"merge", "origin/main", []string{"git-sim", "--media-dir", dir, "merge", "origin/main"}},
		{"rebase", "origin/main", []string{"git-sim", "--media-dir", dir, "rebase", "origin/main"}},
	}
	for _, c := range cases {
		got := visualArgv(c.sub, c.upstream, dir)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("visualArgv(%q) = %#v, want %#v", c.sub, got, c.want)
		}
	}
}

func TestVisualArgvWithoutFlagsOfPreview(r *testing.T) {
	dir := "/cache/gitdash/git-sim"
	for _, o := range visualOptions {
		for _, arg := range visualArgv(o.sub, "origin/main", dir) {
			if arg == "-d" || arg == "--animate" || arg == "--output-only-path" {
				r.Errorf("the %q variant includes %q in the argv", o.sub, arg)
			}
		}
	}
}

func TestVisualHalfDirUnderCacheAndIsCreates(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	dir, err := visualMediaDir()
	if err != nil {
		t.Fatalf("visualMediaDir: %v", err)
	}
	want := filepath.Join(cacheRoot, "gitdash", "git-sim")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("the media-dir was not created: err=%v", err)
	}
}

func TestVisualWithoutUpstream(t *testing.T) {
	for _, key := range []string{"m", "r"} {
		withVisualEnv(t)
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/no-up-cli")
		m, _ = press(m, "v")
		if m.visualArmed == nil || m.visualArmed.upstream != "" {
			t.Fatalf("precondition: armed upstream = %+v", m.visualArmed)
		}
		m, cmd := press(m, key)
		if m.visualArmed != nil {
			t.Errorf("%s left the selector armed", key)
		}
		if _, busy := m.running["/tmp/no-up-cli"]; busy {
			t.Errorf("%s launched no-upstream: running=%v", key, m.running)
		}
		if cmd == nil {
			t.Fatalf("%s no-upstream should warn", key)
		}
		if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no upstream") {
			t.Errorf("%s notification = %v", key, cmd())
		}
	}

	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/no-up-cli")
	m, _ = press(m, "v")
	m, cmd := press(m, "p")
	if cmd == nil || m.running["/tmp/no-up-cli"] != "visual" {
		t.Errorf("p no-upstream did not launch: running=%v", m.running)
	}
}

func TestVisualBlocksWhatIntegratesNothing(t *testing.T) {
	for _, path := range []string{"/tmp/old-clean", "/tmp/ahead-lib"} {
		for _, key := range []string{"m", "r"} {
			withVisualEnv(t)
			m := newPullModel(t)
			m = cursorOn(t, m, path)
			m, _ = press(m, "v")
			if m.visualArmed == nil || m.visualArmed.behind != 0 {
				t.Fatalf("precondition: armed = %+v", m.visualArmed)
			}

			m, cmd := press(m, key)
			if m.visualArmed != nil {
				t.Errorf("v%s left the selector armed", key)
			}
			if _, busy := m.running[path]; busy {
				t.Errorf("v%s handed over the terminal with nothing to integrate: running=%v", key, m.running)
			}
			if cmd == nil {
				t.Fatalf("v%s with nothing to integrate should warn", key)
			}
			nm, ok := cmd().(notifyMsg)
			if !ok || !strings.Contains(nm.text, "nothing to simulate") {
				t.Errorf("v%s notification = %v", key, cmd())
			}
		}
	}
}

func TestVisualPullNotBlocksWithBehindZero(t *testing.T) {
	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if cmd == nil || m.running["/tmp/old-clean"] != "visual" {
		t.Errorf("p with behind 0 did not launch: running=%v", m.running)
	}
}

func TestVisualWarningNamesTheKeyOfFetch(t *testing.T) {
	withVisualEnv(t)
	m := newPullModel(t)
	m.cfg.Keybindings["fetch"] = "F"
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")
	_, cmd := press(m, "m")

	nm, ok := cmd().(notifyMsg)
	if !ok {
		t.Fatalf("notification = %v", cmd())
	}
	if !strings.Contains(nm.text, "F to fetch") {
		t.Errorf("the warning does not name the configured key: %q", nm.text)
	}
	if strings.Contains(nm.text, "f to fetch") {
		t.Errorf("the warning hardcoded the default key: %q", nm.text)
	}
}

func TestVisualWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no git-sim
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if _, busy := m.running["/tmp/old-clean"]; busy {
		t.Errorf("it launched the handoff without a binary: running=%v", m.running)
	}
	if cmd == nil {
		t.Fatal("without a binary it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "git-sim not installed") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestVisualHalfDirNotCreatable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(blocker, "sub"))

	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if _, busy := m.running["/tmp/old-clean"]; busy {
		t.Errorf("it launched the handoff with an uncreatable media-dir: running=%v", m.running)
	}
	if cmd == nil {
		t.Fatal("an uncreatable media-dir should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "media dir") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestVisualBlockedIfAlreadyRuns(t *testing.T) {
	withVisualEnv(t)
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	m.running["/tmp/old-clean"] = "lazygit"
	m, _ = press(m, "v")

	m, cmd := press(m, "p")
	if m.running["/tmp/old-clean"] != "lazygit" {
		t.Errorf("running = %q, want the original untouched", m.running["/tmp/old-clean"])
	}
	if cmd == nil {
		t.Fatal("with an action already running it should warn")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "already running") {
		t.Errorf("notification = %v", cmd())
	}
}

func TestVisualPromptAndKeybinds(t *testing.T) {
	m := newPullModel(t)
	if got := m.promptLine(); got != "" {
		t.Errorf("promptLine with nothing armed = %q, want empty", got)
	}
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "v")

	got := m.promptLine()
	for _, want := range []string{"p pull", "m merge", "r rebase", "esc cancel", "old-clean"} {
		if !strings.Contains(got, want) {
			t.Errorf("the warning does not mention %q:\n%s", want, got)
		}
	}
	if m.keybindsLines() != 1 {
		t.Errorf("keybindsLines = %d, want 1 (the warning replaces the hints)", m.keybindsLines())
	}
	out := stripANSI(m.View().Content)
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "merge") {
		t.Errorf("the warning is not in keybinds:\n%s", kb)
	}
}

func TestVisualAndPanelOfTheLog(t *testing.T) {
	m := newPullModel(t)
	m.visualArmed = &armedVisual{path: "/tmp/old-clean"}
	m.toggleLog()
	if m.visualArmed != nil {
		t.Error("opening the panel did not drop the visual selector")
	}

	m2 := newPullModel(t)
	m2.logOpen = true
	m2, _ = press(m2, "v")
	if m2.visualArmed != nil {
		t.Error("v armed the selector with the log panel open")
	}
}

func TestVisualExecDoneRecords(t *testing.T) {
	m, rec := logModel(t)
	path := "/tmp/old-clean"
	argv := []string{"git-sim", "--media-dir", "/cache/gitdash/git-sim", "merge", "origin/main"}

	updated, _ := m.Update(execDoneMsg{path: path, action: "visual", argv: argv, err: nil})
	_ = updated.(Model)

	var got *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent || e.Action != "visual" {
			continue
		}
		cp := e
		got = &cp
	}
	if got == nil {
		t.Fatal("the visual exec was not recorded")
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

func TestVisualIntentRecorded(t *testing.T) {
	withVisualEnv(t)
	m, rec := logModel(t)
	m = cursorOn(t, m, "/tmp/behind-web")
	m, _ = press(m, "v")
	_, _ = press(m, "m")

	var variantIntent *cmdlog.Entry
	for _, e := range rec.Entries() {
		if e.Intent && e.Key == "m" {
			cp := e
			variantIntent = &cp
		}
	}
	if variantIntent == nil {
		t.Fatal("the variant's intent was not recorded")
	}
	if variantIntent.Action != "visual" || variantIntent.Repo != "behind-web" {
		t.Errorf("intent = %+v", variantIntent)
	}
}

// The visual selector neither consults nor blocks on a rebase in progress: git-sim does not mutate the real repo, so previewing a half-resolved rebase is useful.
func TestVisualNotBlocksForRebaseInCourse(t *testing.T) {
	withVisualEnv(t)
	dir, _ := testutil.NewRepo(t, true) // with upstream origin/main
	if err := os.MkdirAll(filepath.Join(dir, ".git", "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !gitstatus.RebaseInProgress(context.Background(), dir) {
		t.Fatal("precondition: the repo does not report a rebase in progress")
	}

	p := proj("demo", dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapBehind(1)})
	m = cursorOn(t, m, dir)
	m, _ = press(m, "v")
	m, cmd := press(m, "m")
	if cmd == nil || m.running[dir] != "visual" {
		t.Errorf("the rebase in progress blocked the preview: running=%v cmd=%v", m.running, cmd != nil)
	}
}

// The path is unreachable from the UI (the second key can only be a variant), so it is pinned here for the table and the argv not to desync silently.
func TestVisualArgvSubUnknownNotInventsRef(t *testing.T) {
	dir := "/cache/gitdash/git-sim"
	got := visualArgv("squash", "origin/main", dir)
	want := []string{"git-sim", "--media-dir", dir, "squash"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("visualArgv(%q) = %#v, want %#v", "squash", got, want)
	}
}

// Without a cache directory there is no media-dir, and without media-dir git-sim writes `git-sim_media/` INSIDE the repo (leaving it dirty); the failure propagates so the caller warns instead of launching.
func TestVisualHalfDirWithoutCacheDirGivesError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if dir, err := visualMediaDir(); err == nil {
		t.Errorf("visualMediaDir = %q, want an error with no cache directory", dir)
	}
}
