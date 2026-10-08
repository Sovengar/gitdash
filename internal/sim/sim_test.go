package sim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The stub comes first in the PATH: coverage cannot depend on an installed git-sim, and the original PATH is kept because the fixtures need the real `git`.
func writeStub(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRenderAnswersWithThePrintedPath(t *testing.T) {
	writeStub(t, "git-sim", "#!/bin/sh\necho \"$GS_IMAGE\"\n")
	image := filepath.Join(t.TempDir(), "sim.jpg")
	t.Setenv("GS_IMAGE", image)

	got, err := New().Render(context.Background(), t.TempDir(), []string{"git-sim", "--output-only-path", "merge", "feat"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != image {
		t.Errorf("image = %q, want the printed path %q", got, image)
	}
}

func TestRenderFailureKeepsMessageAndExit(t *testing.T) {
	writeStub(t, "git-sim", "#!/bin/sh\necho \"git-sim error: Branch 'main' is already included\" >&2\nexit 1\n")

	_, err := New().Render(context.Background(), "/tmp", []string{"git-sim", "merge", "main"})
	if err == nil {
		t.Fatal("a failing git-sim gave nil")
	}
	var simErr *Error
	if !errors.As(err, &simErr) {
		t.Fatalf("error = %T, want *sim.Error", err)
	}
	if !strings.Contains(err.Error(), "already included") {
		t.Errorf("error = %q, want git-sim's own message", err)
	}
	if simErr.ExitCode != 1 || !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("error = %q, want the exit code kept", err)
	}
	if !strings.Contains(err.Error(), "-C /tmp") {
		t.Errorf("error = %q, want the working dir (without it the message cannot be located)", err)
	}
}

func TestRenderWithoutOutputSaysNoImage(t *testing.T) {
	writeStub(t, "git-sim", "#!/bin/sh\nexit 0\n")

	_, err := New().Render(context.Background(), "/tmp", []string{"git-sim", "merge", "main"})
	if err == nil || !strings.Contains(err.Error(), "produced no image") {
		t.Fatalf("error = %v, want the produced-no-image verdict", err)
	}
	if strings.Contains(err.Error(), "exit") {
		t.Errorf("error = %q, want no exit code: it did NOT fail, the image is what is missing", err)
	}
}

func TestRenderWithoutCommandRefuses(t *testing.T) {
	if _, err := New().Render(context.Background(), "/tmp", nil); err == nil {
		t.Error("an empty argv gave nil: exec would panic on argv[0]")
	}
}

func TestRenderWithoutConfiguredTimeoutUsesTheDefault(t *testing.T) {
	writeStub(t, "git-sim", "#!/bin/sh\necho /tmp/x.jpg\n")

	if _, err := (&Runner{}).Render(context.Background(), "/tmp", []string{"git-sim"}); err != nil {
		t.Errorf("Render with no configured timeout: %v", err)
	}
}

func TestRenderTimeoutKillsTheHang(t *testing.T) {
	writeStub(t, "git-sim", "#!/bin/sh\nsleep 5\n")

	_, err := (&Runner{Timeout: 50 * time.Millisecond}).Render(context.Background(), "/tmp", []string{"git-sim", "merge", "main"})
	if err == nil {
		t.Fatal("a hung git-sim was not killed by the timeout")
	}
}

// The deadline is 60 SECONDS written as a product, so the ARITHMETIC_BASE mutant makes it 60ms and every real render dies; the assertion is in units, because comparing against DefaultTimeout itself cannot fail (same error on both sides).
func TestDefaultTimeoutIsSixtySeconds(t *testing.T) {
	if DefaultTimeout() != 60*time.Second {
		t.Errorf("DefaultTimeout() = %v, want 60s (a %v kills any real git-sim render)",
			DefaultTimeout(), DefaultTimeout())
	}
	if New().Timeout != DefaultTimeout() {
		t.Errorf("New().Timeout = %v, want the default %v", New().Timeout, DefaultTimeout())
	}
}

// Same reasoning as TestDefaultTimeoutIsSixtySeconds: the grace is a product too, and its mutant (a grace of ~0 makes Render kill the wait of a well-behaved child, a huge one delays every hung render) is only observable as its own value.
func TestPipeCloseGraceIs250Millis(t *testing.T) {
	if pipeCloseGrace() != 250*time.Millisecond {
		t.Errorf("pipeCloseGrace() = %v, want 250ms", pipeCloseGrace())
	}
}

func TestAvailableFollowsThePATH(t *testing.T) {
	if !Available("git") {
		t.Error("git is not Available: the fixtures need it, so the PATH is broken")
	}
	writeStub(t, "git-sim", "#!/bin/sh\nexit 0\n")
	if !Available("git-sim") {
		t.Error("the stub is not Available")
	}
	if Available("git-sim-that-does-not-exist") {
		t.Error("a missing binary is Available")
	}
}

// Without a display git-sim's desktop-viewer call never returns, and a user's git_sim_* is the first one git-sim reads: keeping it would void the forcing.
func TestEnvForcesNoAutoOpenAndDropsYours(t *testing.T) {
	t.Setenv("git_sim_auto_open", "true")
	t.Setenv("git_sim_img_format", "png")

	var seen int
	for _, kv := range env() {
		switch kv {
		case "git_sim_auto_open=false":
			seen++
		case "git_sim_img_format=png":
			t.Error("env keeps a user git_sim_*: the first one wins and voids the forcing")
		case "PATH=" + os.Getenv("PATH"):
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("forcing and environment appear %d times, want 2 (the forcing is not duplicated)", seen)
	}
}

func TestMessageFirstNonEmptyLine(t *testing.T) {
	if got := message("\n\n  first\nsecond\n", errors.New("boom")); got != "first" {
		t.Errorf("message = %q, want the first non-empty line (git-sim prefixes banner lines)", got)
	}
	if got := message("", errors.New("boom")); got != "boom" {
		t.Errorf("message = %q, want the wrapped error (nothing in stderr)", got)
	}
}

func TestImagePathLastNonEmptyLine(t *testing.T) {
	if got := imagePath("\n/tmp/a.jpg\n\n"); got != "/tmp/a.jpg" {
		t.Errorf("imagePath = %q, want the last non-empty line", got)
	}
	if got := imagePath("  \n"); got != "" {
		t.Errorf("imagePath = %q, want empty", got)
	}
}

func TestErrorStringShape(t *testing.T) {
	if got := (&Error{Args: []string{"merge", "x"}, Msg: "nope"}).Error(); got != "git-sim merge x: nope" {
		t.Errorf("Error = %q", got)
	}
	if got := (&Error{Args: []string{"merge"}, Msg: "nope", ExitCode: 2}).Error(); got != "git-sim merge: nope (exit 2)" {
		t.Errorf("Error = %q", got)
	}
}

func TestErrorUnwraps(t *testing.T) {
	boom := errors.New("boom")
	if got := (&Error{Err: boom}).Unwrap(); !errors.Is(got, boom) {
		t.Errorf("Unwrap = %v, want the wrapped error (execExit reads the exit code through it)", got)
	}
}
