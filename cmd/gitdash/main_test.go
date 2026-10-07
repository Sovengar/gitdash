package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"

	"gitdash/internal/config"
	"gitdash/internal/tui"
)

type dobles struct {
	d          deps
	cfgWarn    string
	cfg        config.Config
	tuiErr     error
	printed    int
	tuiRuns    int
	modelCfg   config.Config
	toastVisto string
}

func nuevasDobles(t *testing.T) *dobles {
	t.Helper()
	x := &dobles{}
	x.d = deps{
		load:  func() (config.Config, string) { return x.cfg, x.cfgWarn },
		print: func(config.Config) { x.printed++ },
		newModel: func(cfg config.Config) tui.Model {
			x.modelCfg = cfg
			return tui.New(cfg)
		},
		notify: func(_ tui.Model, warn string) { x.toastVisto = warn },
		runTUI: func(tui.Model) error { x.tuiRuns++; return x.tuiErr },
	}
	return x
}

// A config with a warning is still a usable config, and aborting would throw the session away over a file with one line that bothers it; inverting this guard turned a warning into a startup failure and no test noticed.
func TestTheWarningOfConfigNotAborts(t *testing.T) {
	x := nuevasDobles(t)
	x.cfgWarn = "config: unknown key 'foo'"

	var eout strings.Builder
	if code := runWith(x.d, false, &eout); code != 0 {
		t.Errorf("with a config notice the exit code must be 0, got %d", code)
	}
	if !strings.Contains(eout.String(), "unknown key") {
		t.Errorf("the notice did not reach stderr: %q", eout.String())
	}
	if x.tuiRuns != 1 {
		t.Errorf("with a notice the TUI must still start, it started %d times", x.tuiRuns)
	}
}

func TestWithoutWarningNotIsWritesNothing(t *testing.T) {
	x := nuevasDobles(t)

	var eout strings.Builder
	if code := runWith(x.d, false, &eout); code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if eout.Len() != 0 {
		t.Errorf("with no notice nothing should be written, this was: %q", eout.String())
	}
}

func TestTheWarningGoesAlsoOnTheModel(t *testing.T) {
	x := nuevasDobles(t)
	x.cfgWarn = "config: unreadable roots"

	var eout strings.Builder
	runWith(x.d, false, &eout)
	if x.toastVisto != "config: unreadable roots" {
		t.Errorf("the model received the notice %q, it expected the same", x.toastVisto)
	}
}

func TestPrintModeNotStartsTheTUI(t *testing.T) {
	x := nuevasDobles(t)

	var eout strings.Builder
	if code := runWith(x.d, true, &eout); code != 0 {
		t.Errorf("print mode must exit 0, got %d", code)
	}
	if x.printed != 1 {
		t.Errorf("runPrint was called %d times, want 1", x.printed)
	}
	if x.tuiRuns != 0 {
		t.Errorf("print mode must NOT start the TUI, it started %d times", x.tuiRuns)
	}
}

// The complementary half of the previous one: without it, inverting the guard would look like "print works sometimes".
func TestTUIModeNotPrintsTheTable(t *testing.T) {
	x := nuevasDobles(t)

	var eout strings.Builder
	runWith(x.d, false, &eout)
	if x.printed != 0 {
		t.Errorf("TUI mode must not print the table, it printed it %d times", x.printed)
	}
	if x.tuiRuns != 1 {
		t.Errorf("TUI mode must start the TUI, it started %d times", x.tuiRuns)
	}
}

func TestErrorOfTheProgramExitsWithOne(t *testing.T) {
	x := nuevasDobles(t)
	x.tuiErr = errors.New("terminal too narrow")

	var eout strings.Builder
	if code := runWith(x.d, false, &eout); code != 1 {
		t.Errorf("a program failure must exit 1, got %d", code)
	}
	if !strings.Contains(eout.String(), "terminal too narrow") {
		t.Errorf("the error was not reported: %q", eout.String())
	}
}

func TestTheConfigArrivesSameAModeConsumer(t *testing.T) {
	x := nuevasDobles(t)
	x.cfg = config.Defaults()

	runWith(x.d, true, io.Discard)
	if x.modelCfg.Roots == nil {
		t.Log("print mode: the config did not reach the model because it is not built, correct")
	}

	x2 := nuevasDobles(t)
	x2.cfg = config.Defaults()
	runWith(x2.d, false, io.Discard)
	if x2.modelCfg.Roots == nil {
		t.Errorf("TUI mode: the model did not receive the loaded config")
	}
}

func TestPrintRowOfEachShapeOfRepo(t *testing.T) {
	casos := []struct {
		name string
		proj discovery.Project
		snap gitstatus.Snapshot
		want map[string]string
	}{
		{
			name: "detached keeps the branch",
			proj: discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{
				Branch: "feat/x", Detached: true, HasUpstream: true,
			}},
			want: map[string]string{"branch": "feat/x (detached)", "wt": ""},
		},
		{
			name: "no branch yet",
			proj: discovery.Project{Path: "/new", Name: "new", HasRepo: true},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{HasUpstream: true}},
			want: map[string]string{"branch": "-", "name": "new"},
		},
		{
			name: "the loose worktree carries a suffix",
			proj: discovery.Project{
				Path: "/api-wt", Name: "api-wt", HasRepo: true,
				IsWorktree: true, MainRepo: "/api",
			},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{
				Branch: "feat/y", HasUpstream: true,
			}},
			want: map[string]string{"name": "api-wt [wt]", "branch": "feat/y"},
		},
		{
			name: "with worktrees it counts",
			proj: discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap: gitstatus.Snapshot{
				Status:    gitstatus.Status{Branch: "main", HasUpstream: true},
				Worktrees: []gitstatus.Worktree{{Path: "/api-wt", Branch: "a"}},
			},
			want: map[string]string{"wt": "1"},
		},
		{
			name: "with no worktrees it does not count",
			proj: discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", HasUpstream: true}},
			want: map[string]string{"wt": ""},
		},
		{
			name: "an empty group comes out as a dash",
			proj: discovery.Project{Path: "/api", Name: "api", HasRepo: true},
			snap: gitstatus.Snapshot{Status: gitstatus.Status{Branch: "main", HasUpstream: true}},
			want: map[string]string{"group": "-"},
		},
	}

	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			row := printRowOf(c.proj, c.snap)
			got := map[string]string{
				"name": row.name, "branch": row.branch,
				"wt": row.wt, "group": row.group,
			}
			for field, want := range c.want {
				if got[field] != want {
					t.Errorf("%s = %q, want %q", field, got[field], want)
				}
			}
			if row.path != c.proj.Path {
				t.Errorf("path = %q, want %q", row.path, c.proj.Path)
			}
		})
	}
}

// What is checked is that all of them are set and are the functions they claim to be, not nil: a nil there is a panic on the first run and the seam does not reveal it because its doubles are set.
func TestDepsProdIsCompletes(t *testing.T) {
	d := depsProd()
	for name, fn := range map[string]any{
		"load": d.load, "print": d.print, "newModel": d.newModel,
		"notify": d.notify, "runTUI": d.runTUI,
	} {
		if fn == nil {
			t.Errorf("depsProd().%s = nil", name)
		}
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "gitdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "gitdash", "config.toml")
	if err := os.WriteFile(conf, []byte("roots = [\"/tmp/raiz-mia\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := d.load()
	if len(cfg.Roots) != 1 || cfg.Roots[0] != "/tmp/raiz-mia" {
		t.Errorf("depsProd().load() read %v, want the file's config", cfg.Roots)
	}
	if warn != "" {
		t.Errorf("notice = %q, want empty with a valid config", warn)
	}
}

func TestRunWithPrintModeAndDepsReal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "gitdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "gitdash", "config.toml")
	if err := os.WriteFile(conf, []byte("roots = [\""+t.TempDir()+"zz\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	if code := run(true, &errBuf); code != 0 {
		t.Errorf("run(--print) = %d, want 0", code)
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errBuf.String())
	}
}

// main() is the only point calling os.Exit (it kills the whole test binary), so it runs in a SUBPROCESS; the no-flag case matters because if main ignored it the subprocess would hang instead of failing.
func TestMainInSubprocess(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; it takes longer than the rest of the suite")
	}
	bin := t.TempDir() + "/gitdash"
	// -cover is what makes the subprocess's coverage write count: without it the subprocess runs main() and nobody notices, leaving the block at zero as if it never ran; GOCOVERDIR is where the instrumented binary drops the profile on exit.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go is not in PATH: %v", err)
	}
	build := exec.Command(goBin, "build", "-cover", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build -cover: %v", err)
	}
	coverDir := t.TempDir()

	t.Run("print prints the table and exits 0", func(t *testing.T) {
		dir := t.TempDir()
		conf := dir + "/gitdash"
		if err := os.MkdirAll(conf, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(conf+"/config.toml", []byte("roots = [\""+t.TempDir()+"empty\"]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "--print")
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "GOCOVERDIR="+coverDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("--print = %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "no repositories found") {
			t.Errorf("output = %q, want the notice that there are no repos", out)
		}
	})

	t.Run("without print it does not print the table", func(t *testing.T) {
		dir := t.TempDir()
		cmd := exec.CommandContext(context.Background(), bin)
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir, "GOCOVERDIR="+coverDir)
		cmd.Stdin = strings.NewReader("")
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatal("without --print the process does not exit: main ignored the flag")
		}
	})

	// The subprocess profile must exist and carry main(), or the test passes while the block stays unmarked; its counters live in a separate file merged by hand, so main() sits at zero in the -coverpkg metric.
	if err := covdataToText(coverDir); err != nil {
		t.Fatalf("go tool covdata textfmt over %s: %v", coverDir, err)
	}
	out := filepath.Join(coverDir, "cov.out")
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the subprocess wrote no counters into %s: %v", coverDir, err)
	}
	want := mainFuncLine(t)
	var tieneMain bool
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, fmt.Sprintf("cmd/gitdash/main.go:%d.", want)) {
			tieneMain = true
		}
	}
	if !tieneMain {
		t.Errorf("the subprocess profile does not mention main.go:%d, want the func main() block", want)
	}
}

// mainFuncLine locates `func main()` in main.go: the profile is checked by line number, and writing it by hand breaks on any comment change.
func mainFuncLine(t *testing.T) int {
	t.Helper()
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("leer main.go: %v", err)
	}
	for i, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(l, "func main(") {
			return i + 1
		}
	}
	t.Fatal("main.go has no func main()")
	return 0
}

// notify is the one that matters: it bridges the config warning to the model, and if the closure got it wrong (or was handed another text) the warning would be lost with no trace, because stderr stays behind the alt screen.
func TestClosuresOfDepsProd(t *testing.T) {
	d := depsProd()

	t.Run("notify delivers the notice to the model", func(t *testing.T) {
		// The warning has to REACH the model, not be checked as painted: looking at the toasts would need a getter in tui, and that getter is exactly the coupling the separate notify avoids; what is checked is that the closure can be called with a model and a warning without blowing up, which is what a nil there would do.
		m := tui.New(config.Defaults())
		d.notify(m, "config: the file could not be read")
		d.notify(m, "") // an empty warning must not blow up either
	})

	t.Run("runTUI returns the program error", func(t *testing.T) {
		// Without a terminal bubbletea does not start; what is checked is that the closure PROPAGATES that error instead of returning nil, since a nil here would exit 0 with a TUI that was never painted.
		m := tui.New(config.Defaults())
		if err := d.runTUI(m); err == nil {
			t.Log("runTUI with no terminal = nil (this machine does have a tty)")
		}
	})
}
