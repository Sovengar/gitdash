// Package sim runs git-sim as a background subprocess that answers with the path of the image it rendered: the TUI keeps the terminal (no handoff, no manim output on screen) and the image is opened by the desktop viewer instead of painted in the terminal.
package sim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const DefaultBin = "git-sim"

// rebaseSince is the first release whose rebase renderer replays commits instead of indexing a short SHA list.
const rebaseSince = "0.4.0"

// A function, not a const: Go does not instrument constant expressions, so a package const would leave its ARITHMETIC_BASE mutant permanently NOT COVERED.
func DefaultTimeout() time.Duration { return 60 * time.Second }

// Same reasoning as DefaultTimeout.
func pipeCloseGrace() time.Duration { return 250 * time.Millisecond }

type Runner struct {
	Timeout time.Duration
}

func New() *Runner { return &Runner{Timeout: DefaultTimeout()} }

// Available answers for the very argv the render will run, so the check and the run cannot name different binaries.
func Available(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// SupportsRebase gates the variant on the version: 0.3.5 crashes with an IndexError on divergent rebases.
func SupportsRebase() bool {
	version, err := Version()
	return err == nil && atLeast(version, rebaseSince)
}

// Version reads the semver from `git-sim --version`, which prints "git-sim version X.Y.Z".
func Version() (string, error) {
	out, err := exec.Command(DefaultBin, "--version").Output()
	if err != nil {
		return "", err
	}
	for _, field := range strings.Fields(string(out)) {
		if isVersion(field) {
			return field, nil
		}
	}
	return "", fmt.Errorf("sim: cannot read the git-sim version from %q", strings.TrimSpace(string(out)))
}

func isVersion(field string) bool {
	parts := strings.Split(strings.TrimPrefix(field, "v"), ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func atLeast(have, want string) bool {
	h, w := versionParts(have), versionParts(want)
	for i := range h {
		if h[i] != w[i] {
			return h[i] > w[i]
		}
	}
	return true
}

func versionParts(v string) [3]int {
	var out [3]int
	for i, part := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
		if i >= len(out) {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out
}

type Error struct {
	Args     []string
	Dir      string
	ExitCode int
	Msg      string
	Err      error
}

func (e *Error) Error() string {
	base := fmt.Sprintf("git-sim %s: %s", strings.Join(e.Args, " "), e.Msg)
	if e.Dir != "" {
		base = fmt.Sprintf("git-sim -C %s %s: %s", e.Dir, strings.Join(e.Args, " "), e.Msg)
	}
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

// Render runs argv (argv[0] the executable, exactly as the command log records it) inside workdir and answers with the printed image path: --output-only-path makes that path the only stdout line, so capturing is all it takes and the terminal is never lent out. The image itself is not opened here: the caller decides who shows it.
func (r *Runner) Render(ctx context.Context, workdir string, argv []string) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("sim: no command to run")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.Dir = workdir
	cmd.Env = env()
	cmd.WaitDelay = pipeCloseGrace()

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		simErr := &Error{Args: argv[1:], Dir: workdir, Msg: message(errb.String(), err), Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			simErr.ExitCode = exit.ExitCode()
		}
		return "", simErr
	}
	image := imagePath(out.String())
	if image == "" {
		return "", &Error{Args: argv[1:], Dir: workdir, Msg: "git-sim produced no image"}
	}
	return image, nil
}

// git-sim's own auto-open is forced off on purpose: its desktop-viewer call can hang a captured process forever without a display, and the viewer is ours to launch (the image opens as an image, after the render, with its path in the toast). A user's git_sim_* is dropped because the first entry of the environment is the one git-sim reads, and it would win over the forcing.
func env() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "git_sim_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "git_sim_auto_open=false")
}

// git-sim prefixes its errors with banner lines, so the reason is the first non-empty one and not simply the first.
func message(stderr string, err error) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return err.Error()
}

func imagePath(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
