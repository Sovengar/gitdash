// Package tool runs forge CLIs as subprocesses with a homogeneous environment (English locale, no interaction, per-call deadline, error keeping the exit code), living outside forge because forge is pure by contract.
package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// A function, not a constant: Go does not instrument constant expressions, so a package const would leave the ARITHMETIC_BASE mutant of `30 * time.Second` permanently NOT COVERED.
func DefaultTimeout() time.Duration { return 30 * time.Second }

type Runner struct {
	Bin     string
	Timeout time.Duration
	// Extra goes last so it overrides whatever the user environment brings.
	Extra []string
}

func New(bin string, extra ...string) *Runner {
	return &Runner{Bin: bin, Timeout: DefaultTimeout(), Extra: extra}
}

type Error struct {
	Bin      string
	Args     []string
	ExitCode int
	Msg      string
	Err      error
}

func (e *Error) Error() string {
	base := fmt.Sprintf("%s %s: %s", e.Bin, strings.Join(e.Args, " "), e.Msg)
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

// stdout is returned even on failure: a CLI can exit non-zero and carry the useful reason there.
func (r *Runner) Run(ctx context.Context, args ...string) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, r.Bin, args...)
	cmd.Env = Env(r.Extra...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := FirstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		terr := &Error{Bin: r.Bin, Args: args, Msg: msg, Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			terr.ExitCode = exit.ExitCode()
		}
		return out.String(), terr
	}
	return out.String(), nil
}

// Only the locale is dropped (English messages) plus the non-interactive flags: the rest of the user environment stays, because without it the CLIs lose exactly what authenticates them.
func Env(extra ...string) []string {
	env := os.Environ()
	// No capacity reserved by hand: env is the ceiling anyway, and a literal number here would be a place to mutate with no observable effect.
	out := make([]string, 0, len(env))
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="):
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1")
	return append(out, extra...)
}

func ExitCode(err error) int {
	var terr *Error
	if errors.As(err, &terr) {
		return terr.ExitCode
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 0
}

// A multi-line reason would push the panel height, and only the first line goes into the toast.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
