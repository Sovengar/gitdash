package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A forge's real binary cannot be tested without network or token, so what is tested is what the Runner does around it: argv, environment, deadline and capture.
func stub(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnvIsHomogeneous(t *testing.T) {
	t.Setenv("LC_ALL", "es_ES.UTF-8")
	t.Setenv("LANG", "es_ES.UTF-8")
	t.Setenv("LANGUAGE", "es")
	t.Setenv("LC_MESSAGES", "es_ES.UTF-8")

	env := Env()
	for _, banned := range []string{"LANG=", "LANGUAGE=", "LC_MESSAGES="} {
		for _, kv := range env {
			if strings.HasPrefix(kv, banned) {
				t.Errorf("Env keeps %q: %q", banned, kv)
			}
		}
	}
	n := 0
	for _, kv := range env {
		if kv == "LC_ALL=C" {
			n++
		}
		if strings.HasPrefix(kv, "LC_ALL=") && kv != "LC_ALL=C" {
			t.Errorf("LC_ALL not forced to C: %q", kv)
		}
	}
	if n != 1 {
		t.Errorf("LC_ALL=C appears %d times, want 1 (the user's must not survive)", n)
	}
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "NO_COLOR=1"} {
		if !slicesHas(env, want) {
			t.Errorf("Env does not contain %q", want)
		}
	}
}

func TestEnvKeepsTheRest(t *testing.T) {
	t.Setenv("GITDASH_TEST_MARKER", "still-alive")
	env := Env()
	if !slicesHas(env, "GITDASH_TEST_MARKER=still-alive") {
		t.Error("Env dropped a variable that is not locale")
	}
	if len(env) == 0 {
		t.Error("Env is empty")
	}
}

func TestEnvAddsTheVariablesOfTheForge(t *testing.T) {
	env := Env("GH_PROMPT_DISABLED=1")
	if !slicesHas(env, "GH_PROMPT_DISABLED=1") {
		t.Error("Env did not add the extra variable")
	}
}

// An empty stderr falls back to the process error because an empty Msg would classify as a network failure with no reason, leaving the user with nothing to do.
func TestRunCapturesStderrAndCodeOfOutput(t *testing.T) {
	bin := stub(t, "#!/bin/sh\necho 'the real reason' >&2\nexit 3\n")
	_, err := (&Runner{Bin: bin}).Run(context.Background(), "-t", "x")
	var terr *Error
	if !errors.As(err, &terr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if terr.Msg != "the real reason" {
		t.Errorf("Msg = %q, want the stderr", terr.Msg)
	}
	if terr.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", terr.ExitCode)
	}
	if terr.Bin != bin || !slicesHas(terr.Args, "-t") {
		t.Errorf("Error does not remember the argv: %+v", terr)
	}
	if !strings.Contains(terr.Error(), "exit 3") {
		t.Errorf("the message does not mention the code: %q", terr)
	}

	_, err = (&Runner{Bin: stub(t, "#!/bin/sh\nexit 7\n")}).Run(context.Background())
	if !errors.As(err, &terr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if terr.Msg == "" {
		t.Error("without stderr the reason cannot be empty")
	}
	if terr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", terr.ExitCode)
	}
	if terr.Unwrap() == nil {
		t.Error("Unwrap returns nil: exec's cause is lost")
	}
}

func TestRunReturnsStdoutAlthoughFails(t *testing.T) {
	bin := stub(t, "#!/bin/sh\necho 'valid output'\nexit 1\n")
	out, err := (&Runner{Bin: bin}).Run(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.TrimSpace(out) != "valid output" {
		t.Errorf("stdout = %q, want the valid output even though the command failed", out)
	}
}

// Each argv element arrives as one element, which is what makes it safe to pass a title written by a person: with no quotes in between, a ";" or a "$(...)" is text, not syntax.
func TestRunPassesTheArgvElementForElement(t *testing.T) {
	nasty := `double "quote" & $(id) ; echo injected | cat`
	bin := stub(t, "#!/bin/sh\nprintf 'argc=%s\\n' \"$#\"\nprintf '%s\\0' \"$@\"\n")
	out, err := (&Runner{Bin: bin}).Run(context.Background(),
		"pr", "create", "-t", nasty, "-b", "body with\ttab and\nnew line", "-B", "main")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.HasPrefix(out, "argc=8\n") {
		t.Errorf("argc = %q, want 8 elements", strings.SplitN(out, "\n", 2)[0])
	}
	_, payload, _ := strings.Cut(out, "\n")
	if got := strings.Split(strings.TrimSuffix(payload, "\x00"), "\x00"); len(got) != 8 {
		t.Fatalf("got %d elements: %q", len(got), got)
	} else if got[3] != nasty {
		t.Errorf("the value did not arrive whole: %q", got[3])
	} else if got[5] != "body with\ttab and\nnew line" {
		t.Errorf("the body with line breaks did not arrive whole: %q", got[5])
	}
}

func TestRunAppliesTheTimeoutForBug(t *testing.T) {
	bin := stub(t, "#!/bin/sh\necho ok\n")
	if _, err := (&Runner{Bin: bin}).Run(context.Background()); err != nil {
		t.Errorf("timeout=0 should use the default, but failed: %v", err)
	}
	if _, err := (&Runner{Bin: bin, Timeout: -time.Second}).Run(context.Background()); err != nil {
		t.Errorf("a negative timeout should fall back to the default, but failed: %v", err)
	}
	r := &Runner{Bin: bin}
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.Timeout != 0 {
		t.Errorf("Run mutated the Runner's Timeout: %v", r.Timeout)
	}
	slow := stub(t, "#!/bin/sh\nsleep 5\n")
	if _, err := (&Runner{Bin: slow, Timeout: 50 * time.Millisecond}).Run(context.Background()); err == nil {
		t.Error("a command that exceeds the timeout has to fail")
	}
}

func TestRunWithBinaryNonexistent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nonexistent")
	_, err := (&Runner{Bin: missing}).Run(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	var terr *Error
	if !errors.As(err, &terr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if terr.Err == nil {
		t.Error("exec's cause was not preserved")
	}
	if !strings.Contains(terr.Msg, "nonexistent") {
		t.Errorf("Msg = %q, want it to name the missing binary", terr.Msg)
	}
	if terr.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0: it never ran", terr.ExitCode)
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", got)
	}
	if got := ExitCode(errors.New("other")); got != 0 {
		t.Errorf("ExitCode(other) = %d, want 0", got)
	}
	if got := ExitCode(&Error{ExitCode: 42}); got != 42 {
		t.Errorf("ExitCode(*Error) = %d, want 42", got)
	}
	if got := ExitCode(errWrap{&Error{ExitCode: 9}}); got != 9 {
		t.Errorf("ExitCode(wrapped) = %d, want 9", got)
	}
	_, err := (&Runner{Bin: stub(t, "#!/bin/sh\nexit 6\n")}).Run(context.Background())
	var terr *Error
	if !errors.As(err, &terr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if got := ExitCode(terr.Err); got != 6 {
		t.Errorf("ExitCode(exec.ExitError) = %d, want 6", got)
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"one":          "one",
		"one\ntwo":     "one",
		"one\n\ntwo":   "one",
		"one\n":        "one",
		"":             "",
		"\n":           "",
		"with 3 lines": "with 3 lines",
		"reason\nargv": "reason",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	r := New("gh", "GH_PROMPT_DISABLED=1")
	if r.Bin != "gh" {
		t.Errorf("Bin = %q", r.Bin)
	}
	if r.Timeout != DefaultTimeout() {
		t.Errorf("Timeout = %v, want %v", r.Timeout, DefaultTimeout())
	}
	if !slicesHas(r.Extra, "GH_PROMPT_DISABLED=1") {
		t.Errorf("Extra = %q", r.Extra)
	}
}

// The deadline is 30 SECONDS written as a product, so the ARITHMETIC_BASE mutant makes it 30ms and every real `gh pr create` fails; the assertion is in units, because comparing `r.Timeout` with `DefaultTimeout` cannot fail (same error on both sides).
func TestDefaultTimeoutAreThirtySeconds(t *testing.T) {
	if DefaultTimeout() != 30*time.Second {
		t.Errorf("DefaultTimeout() = %v, want 30s (a %v kills any real gh/glab)",
			DefaultTimeout(), DefaultTimeout())
	}
	r := New("gh")
	if r.Timeout != 30*time.Second {
		t.Errorf("Runner.Timeout = %v, want 30s", r.Timeout)
	}
}

type errWrap struct{ err error }

func (e errWrap) Error() string { return "wrapped: " + e.err.Error() }
func (e errWrap) Unwrap() error { return e.err }

func slicesHas(has []string, needle string) bool {
	for _, h := range has {
		if h == needle {
			return true
		}
	}
	return false
}

func TestErrorWithoutCodeNotSaysExit(t *testing.T) {
	e := &Error{Bin: "gh", Args: []string{"pr", "list"}, Msg: "context canceled"}
	got := e.Error()
	want := "gh pr list: context canceled"
	if got != want {
		t.Errorf("Error() = %q, want %q (no exit suffix)", got, want)
	}
	if strings.Contains(got, "exit") {
		t.Errorf("Error() = %q, want without the word exit", got)
	}
	e2 := &Error{Bin: "gh", Args: []string{"pr", "list"}, Msg: "boom", ExitCode: 2}
	if !strings.Contains(e2.Error(), "(exit 2)") {
		t.Errorf("Error() = %q, want the suffix (exit 2)", e2.Error())
	}
}
