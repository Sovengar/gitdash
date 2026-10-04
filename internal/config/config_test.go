package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Marker != ".gitdash.toml" {
		t.Errorf("marker = %q", cfg.Marker)
	}
	if len(cfg.Roots) != 1 || filepath.Base(cfg.Roots[0]) != "dev" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if !cfg.FetchAuto || cfg.FetchConcurrency != 4 || cfg.FetchTimeout != 30*time.Second {
		t.Errorf("fetch defaults = %+v", cfg)
	}
	if cfg.Editor == "" {
		t.Error("editor default empty")
	}
	found := false
	for _, ex := range cfg.Exclude {
		if ex == "testdata" {
			found = true
		}
	}
	if !found {
		t.Errorf("exclude default without testdata: %v", cfg.Exclude)
	}
}

func TestPartialOverride(t *testing.T) {
	path := write(t, `roots = ["~/code", "~/work"]`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if len(cfg.Roots) != 2 || filepath.Base(cfg.Roots[0]) != "code" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto || cfg.FetchConcurrency != 4 {
		t.Errorf("defaults not kept: %+v", cfg)
	}
}

func TestMalformed(t *testing.T) {
	path := write(t, `roots = [`)
	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("expected a parse warning")
	}
	if cfg.Marker != ".gitdash.toml" || !cfg.FetchAuto {
		t.Errorf("defaults were not used: %+v", cfg)
	}
}

func TestMissingFile(t *testing.T) {
	cfg, warn := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.Marker != ".gitdash.toml" || len(cfg.Roots) != 1 {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := write(t, "roots = [\"~/dev\"]\n")
	cfg, _ := LoadFrom(path)
	if cfg.Roots[0] != filepath.Join(home, "dev") {
		t.Errorf("root = %q, want %q", cfg.Roots[0], filepath.Join(home, "dev"))
	}
}

func TestFetchOverride(t *testing.T) {
	path := write(t, `
[fetch]
auto = false
concurrency = 8
timeout = "10s"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchAuto {
		t.Error("auto should be false")
	}
	if cfg.FetchConcurrency != 8 || cfg.FetchTimeout != 10*time.Second {
		t.Errorf("fetch = %+v", cfg)
	}
}

func TestInvalidValuesIgnored(t *testing.T) {
	path := write(t, `
[fetch]
concurrency = 0
timeout = "nope"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchConcurrency != 4 || cfg.FetchTimeout != 30*time.Second {
		t.Errorf("invalid values not ignored: %+v", cfg)
	}
}

func TestFetchConcurrencyMinimumAccepted(t *testing.T) {
	path := write(t, "[fetch]\nconcurrency = 1\n")
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.FetchConcurrency != 1 {
		t.Errorf("concurrency = %d, want 1 (minimum valid accepted)", cfg.FetchConcurrency)
	}
}

func TestFetchTimeoutNotPositiveIgnored(t *testing.T) {
	for _, timeout := range []string{"0s", "0", "-1s", "-30m"} {
		t.Run(timeout, func(t *testing.T) {
			path := write(t, "[fetch]\ntimeout = "+quote(timeout)+"\n")
			cfg, _ := LoadFrom(path)
			if cfg.FetchTimeout != 30*time.Second {
				t.Errorf("timeout %q → %s, want the 30s default", timeout, cfg.FetchTimeout)
			}
		})
	}
}

func quote(s string) string { return `"` + s + `"` }

func TestKeysMissingKeepDefaults(t *testing.T) {
	path := write(t, `roots = ["/tmp"]`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.Marker != DefaultMarker {
		t.Errorf("marker = %q, want %q", cfg.Marker, DefaultMarker)
	}
	if !reflect.DeepEqual(cfg.Exclude, DefaultExclude) {
		t.Errorf("exclude = %v, want the defaults %v", cfg.Exclude, DefaultExclude)
	}
	if cfg.Editor != Defaults().Editor {
		t.Errorf("editor = %q, want %q", cfg.Editor, Defaults().Editor)
	}
	if cfg.SyncBranch != "main" {
		t.Errorf("sync_branch = %q, want main", cfg.SyncBranch)
	}
}

func TestValuesEmptyKeepDefaults(t *testing.T) {
	path := write(t, `
marker = ""
editor = ""
sync_branch = ""
`)
	cfg, _ := LoadFrom(path)
	if cfg.Marker != DefaultMarker {
		t.Errorf("marker = %q, want %q", cfg.Marker, DefaultMarker)
	}
	if cfg.Editor != Defaults().Editor {
		t.Errorf("editor = %q, want %q", cfg.Editor, Defaults().Editor)
	}
	if cfg.SyncBranch != "main" {
		t.Errorf("sync_branch = %q, want main", cfg.SyncBranch)
	}
}

// `exclude = []` is an explicit intent (prune nothing) and replaces the defaults like any other list: that is what tells "key absent" from "empty list" apart.
func TestExcludeEmptyDeactivatesPrunes(t *testing.T) {
	path := write(t, "exclude = []\n")
	cfg, _ := LoadFrom(path)
	if len(cfg.Exclude) != 0 {
		t.Errorf("exclude = %v, want empty (pruning off)", cfg.Exclude)
	}
}

func TestEditorDefaultFromEnvironment(t *testing.T) {
	t.Run("with EDITOR", func(t *testing.T) {
		t.Setenv("EDITOR", "nano -w")
		if got := Defaults().Editor; got != "nano -w" {
			t.Errorf("editor = %q, want nano -w", got)
		}
	})
	t.Run("without EDITOR", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		if got := Defaults().Editor; got != "vi" {
			t.Errorf("editor = %q, want vi", got)
		}
	})
}

func TestExpandAll(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("without home: %v", err)
	}
	cases := []struct {
		in   string
		want string
	}{
		{"~/dev", filepath.Join(home, "dev")},
		{"~/", home},
		{"~", "~"},
		{"~user/dev", "~user/dev"},
		{"/opt/dev", "/opt/dev"},
		{"dev", "dev"},
		{"", ""},
	}
	for _, c := range cases {
		got := expandAll([]string{c.in})
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("expandAll(%q) = %v, want [%q]", c.in, got, c.want)
		}
	}
}

func TestExpandAllListEmpty(t *testing.T) {
	if got := expandAll(nil); len(got) != 0 {
		t.Errorf("expandAll(nil) = %v, want empty", got)
	}
}

func TestFoldKeybinding(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("fold") != "enter" {
		t.Errorf("default fold = %q, want enter", cfg.KeyFor("fold"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "enter fold") {
		t.Errorf("hint de plegado ausente: %v", cfg.HintBarLines())
	}

	path := write(t, `
[keybindings]
fold = "w"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("fold") != "w" {
		t.Errorf("fold = %q, want w", cfg.KeyFor("fold"))
	}
	hints := strings.Join(cfg.HintBarLines(), "\n")
	if !strings.Contains(hints, "w fold") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
	if strings.Contains(hints, "enter fold") {
		t.Errorf("the hint still has the old key: %v", cfg.HintBarLines())
	}
}

func TestDefaultKeybindingsWorktreeRemove(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("worktree_remove") != "D" {
		t.Errorf("default worktree_remove = %q, want D", cfg.KeyFor("worktree_remove"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "D remove wt") {
		t.Errorf("hint de borrado ausente: %v", cfg.HintBarLines())
	}

	path := write(t, `
[keybindings]
worktree_remove = "W"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("worktree_remove") != "W" {
		t.Errorf("worktree_remove = %q, want W", cfg.KeyFor("worktree_remove"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "W remove wt") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}

func TestDefaultKeybindingsVisual(t *testing.T) {
	cfg := Defaults()
	if cfg.KeyFor("visual") != "v" {
		t.Errorf("default visual = %q, want v", cfg.KeyFor("visual"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "v visual") {
		t.Errorf("hint de visual ausente: %v", cfg.HintBarLines())
	}

	path := write(t, `
[keybindings]
visual = "V"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn inesperado: %q", warn)
	}
	if cfg.KeyFor("visual") != "V" {
		t.Errorf("visual = %q, want V", cfg.KeyFor("visual"))
	}
	if !strings.Contains(strings.Join(cfg.HintBarLines(), "\n"), "V visual") {
		t.Errorf("hint rebindeado ausente: %v", cfg.HintBarLines())
	}
}

func TestKeybindingsObsoleteWarn(t *testing.T) {
	path := write(t, `
[keybindings]
detail = "enter"
expand = "space"
fold   = "w"
`)
	cfg, warn := LoadFrom(path)
	if !strings.Contains(warn, "detail") || !strings.Contains(warn, "expand") {
		t.Errorf("warning without the stale actions: %q", warn)
	}
	if strings.Contains(warn, "fold") {
		t.Errorf("it warns about a valid action: %q", warn)
	}
	if _, ok := cfg.Keybindings["detail"]; ok {
		t.Error("the stale action stayed in the key map")
	}
	if cfg.KeyFor("fold") != "w" {
		t.Errorf("fold = %q, want w", cfg.KeyFor("fold"))
	}
}

// Every test of the package passed a path by hand to LoadFrom, so the function resolving XDG_CONFIG_HOME had no test at all: the very branch that decides where the file lives never ran.
func TestPathRespectsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := filepath.Join(dir, DirName, FileName); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// Without XDG_CONFIG_HOME nor HOME there is no user directory, and Path has to say so instead of returning a path that does not exist (a silent "/gitdash/config.toml" would write where nobody reads).
func TestPathWithoutDirectoryOfUserGivesError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if got, err := Path(); err == nil {
		t.Errorf("Path without HOME nor XDG = %q, want error", got)
	}
}

func TestLoadWithoutFileGivesDefaultsSilent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, warn := Load()
	if warn != "" {
		t.Errorf("with no config, Load warns %q; an unconfigured user has no error to see", warn)
	}
	if len(cfg.Roots) == 0 {
		t.Error("with no config, Load did not return the default roots")
	}
	if cfg.Marker == "" {
		t.Error("with no config, Load did not return the default marker")
	}
}

func TestLoadReadsTheFileOfThePathResolved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	raiz := t.TempDir()
	body := fmt.Sprintf("roots = [%q]\nsync_branch = \"develop\"\n", raiz)
	if err := os.WriteFile(filepath.Join(dir, DirName, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, warn := Load()
	if warn != "" {
		t.Errorf("valid config, Load warns %q", warn)
	}
	if len(cfg.Roots) != 1 || cfg.Roots[0] != raiz {
		t.Errorf("Roots = %v, want [%q]", cfg.Roots, raiz)
	}
	if cfg.SyncBranch != "develop" {
		t.Errorf("SyncBranch = %q, want develop", cfg.SyncBranch)
	}
}

func TestSyncBranchExplicit(t *testing.T) {
	t.Run("undeclared", func(t *testing.T) {
		cfg, _ := LoadFrom(write(t, `roots = ["/tmp"]`))
		if cfg.SyncBranchExplicit {
			t.Error("SyncBranchExplicit with the key absent: a default is not a declaration")
		}
		if cfg.SyncBranch != "main" {
			t.Errorf("SyncBranch = %q, want main (the default)", cfg.SyncBranch)
		}
	})
	t.Run("declarada", func(t *testing.T) {
		cfg, _ := LoadFrom(write(t, `sync_branch = "develop"`))
		if !cfg.SyncBranchExplicit {
			t.Error("SyncBranchExplicit = false with sync_branch in the TOML")
		}
		if cfg.SyncBranch != "develop" {
			t.Errorf("SyncBranch = %q, want develop", cfg.SyncBranch)
		}
	})
	t.Run("declared but empty", func(t *testing.T) {
		cfg, _ := LoadFrom(write(t, `sync_branch = ""`))
		if cfg.SyncBranchExplicit {
			t.Error("SyncBranchExplicit = true with an empty sync_branch")
		}
	})
}

// An empty declared command does NOT replace the default: it is an unfilled box and not an order to do nothing, and accepting it left the action with an empty argv, a panic reachable from the log panel.
func TestCommandEmptyNotStepsTheDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path,
		[]byte("[commands]\npull = \"\"\npush = \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFrom(path)

	def := DefaultCommands()
	for _, k := range []string{"pull", "push"} {
		if cfg.Commands[k] != def[k] {
			t.Errorf("commands[%q] = %q, want the default %q: an empty value is a blank box, not an instruction",
				k, cfg.Commands[k], def[k])
		}
	}
}

func TestCommandEmptyOfAActionUnknownNotIsRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path,
		[]byte("[commands]\ninventada = \"\"\nreal = \"log --oneline -5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFrom(path)

	if v, ok := cfg.Commands["inventada"]; ok {
		t.Errorf("commands[\"inventada\"] = %q recorded: an empty command is not a command", v)
	}
	if got := cfg.Commands["real"]; got != "log --oneline -5" {
		t.Errorf("commands[\"real\"] = %q, want the declared value", got)
	}
}

// The skip is checked with a WHOLE hint ("f fetch") and not with the bare word, because "fetch" is a substring of "fetch all" and searching it alone would always be a false positive whenever `fetch_all` were bound.
func TestHintBarSkipsActionsWithoutKey(t *testing.T) {
	cfg := Defaults()
	delete(cfg.Keybindings, "fetch")
	delete(cfg.Keybindings, "pr")
	rows := cfg.HintBarLines()
	if len(rows) != 3 {
		t.Fatalf("HintBarLines returned %d lines, want 3", len(rows))
	}
	antes := Defaults().HintBarLines()
	for i, r := range rows {
		for _, hint := range []string{"f fetch", "O open PR"} {
			if strings.Contains(r, hint) {
				t.Errorf("line %d (%q) advertises %q, which no longer has a key", i, r, hint)
			}
		}
	}
	if len(rows[2]) >= len(antes[2]) {
		t.Errorf("the tools line did not shorten when removing pr: %q", rows[2])
	}
	if !strings.Contains(rows[2], "e edit") {
		t.Errorf("the tools line (%q) lost edit, which is bound", rows[2])
	}
	if !strings.Contains(rows[1], "F fetch all") {
		t.Errorf("the git line (%q) lost fetch all, which was untouched", rows[1])
	}
}

func TestKeyByActionInvertsTheMap(t *testing.T) {
	cfg := Defaults()
	cfg.Keybindings["fold"] = "w"
	inv := cfg.KeyByAction()
	if len(inv) == 0 {
		t.Fatal("KeyByAction = empty")
	}
	if got := inv["w"]; got != "fold" {
		t.Errorf("KeyByAction()[\"w\"] = %q, want fold", got)
	}
	if _, ok := inv["enter"]; ok {
		t.Error("the default fold key (enter) is still in the inverted map after redefining it")
	}
	def := Defaults()
	for action, key := range def.Keybindings {
		if got := def.KeyByAction()[key]; got != action {
			t.Errorf("KeyByAction()[%q] = %q, want %q", key, got, action)
		}
	}
}

func TestKeyForAndCmdArgsFallsOnTheDefault(t *testing.T) {
	cfg := Defaults()
	if got := cfg.KeyFor("invented_action"); got != "" {
		t.Errorf("KeyFor(inventada) = %q, want empty (there is no default for it)", got)
	}
	if got := cfg.CmdArgs("invented_action"); len(got) != 0 {
		t.Errorf("CmdArgs(inventada) = %q, want an empty slice", got)
	}
	delete(cfg.Commands, "pull")
	argv := cfg.CmdArgs("pull")
	if len(argv) == 0 || argv[0] != "pull" {
		t.Errorf("CmdArgs(pull) with no override = %q, want the default starting with pull", argv)
	}
}

// Triggered with a DIRECTORY instead of a file, which makes os.ReadFile fail with EISDIR instead of "does not exist": if that branch returned an empty warning, a config.toml turned into a directory would start the dashboard saying nothing.
func TestLoadFromDirectoryWarns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config.toml")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(dir)
	if warn == "" {
		t.Error("LoadFrom of a directory with no warning, want a notice (an unreadable config is reported)")
	}
	if !strings.HasPrefix(warn, "config: ") {
		t.Errorf("warning = %q, want the 'config: ' prefix", warn)
	}
	if cfg.Marker != Defaults().Marker {
		t.Errorf("Marker = %q, want the default (an unreadable config changes nothing)", cfg.Marker)
	}
}

func TestLoadWithoutUserConfigDirGivesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "nonexistent"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	cfg, warn := Load()
	if warn != "" {
		t.Errorf("Load without HOME = warning %q, want empty (with no path there is nothing to report)", warn)
	}
	if !reflect.DeepEqual(cfg.Roots, Defaults().Roots) {
		t.Errorf("Roots = %v, want the defaults", cfg.Roots)
	}
}

func TestMarkerAndEditorEmptyKeepTheDefault(t *testing.T) {
	path := write(t, `marker = ""
editor = ""
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warning = %q, want empty", warn)
	}
	if cfg.Marker != Defaults().Marker {
		t.Errorf("Marker = %q, want the default %q (an empty string does not clear the marker)",
			cfg.Marker, Defaults().Marker)
	}
	if cfg.Editor != Defaults().Editor {
		t.Errorf("Editor = %q, want the default %q (an empty string does not clear the editor)",
			cfg.Editor, Defaults().Editor)
	}
	path2 := write(t, "marker = \".mi-marcador\"\neditor = \"nano\"\n")
	cfg2, _ := LoadFrom(path2)
	if cfg2.Marker != ".mi-marcador" || cfg2.Editor != "nano" {
		t.Errorf("Marker/Editor = %q/%q, want the file's ones", cfg2.Marker, cfg2.Editor)
	}
}

// The check lives here and not as an `if !ok { label = key }` inside HintBarLines: that fallback was a branch no test could kill (all 17 actions have a label) and in the unlikely case of a missing one it painted a hint with the key and nothing else, which reads as a render bug.
func TestHintActionsAllHaveLabel(t *testing.T) {
	for _, action := range hintActions {
		if _, ok := hintLabels[action]; !ok {
			t.Errorf("the action %q shows in the hint bar but has no label in hintLabels", action)
		}
	}
	for action, label := range hintLabels {
		if label == "" {
			t.Errorf("hintLabels[%q] = %q, want a label", action, label)
		}
	}
}
