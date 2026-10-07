package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
)

// An interface so their error branches stay reachable: os.MkdirAll, os.WriteFile and cmd.CombinedOutput only fail when disk or process fail, and a t.Fatal would kill the very test that covers them, while a double records the failure instead.
type TB interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	TempDir() string
}

func git(t TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func Init(t TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@gitdash.local")
	git(t, dir, "config", "user.name", "gitdash tests")
}

func CommitFiles(t TB, dir string, files map[string]string, msg string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", msg)
}

func Marker(t TB, dir, name, primary, secondary string, malformed bool) {
	t.Helper()
	var content string
	if malformed {
		content = "name = [roto\n"
	} else {
		if name != "" {
			content += "name = \"" + name + "\"\n"
		}
		if primary != "" {
			content += "primary_group = \"" + primary + "\"\n"
		}
		if secondary != "" {
			content += "secondary_group = \"" + secondary + "\"\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitdash.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func InitBare(t TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "--bare", "-b", "main")
}

func AddUpstream(t TB, dir, origin string) {
	t.Helper()
	git(t, dir, "remote", "add", "origin", origin)
	git(t, dir, "push", "-u", "origin", "main")
}

func PushUpstreamCommits(t TB, bare string, n int, prefix string) {
	t.Helper()
	clone := t.TempDir()
	git(t, clone, "clone", "--quiet", bare, ".")
	git(t, clone, "config", "user.email", "test@gitdash.local")
	git(t, clone, "config", "user.name", "upstream")
	for i := 0; i < n; i++ {
		CommitFiles(t, clone, map[string]string{"up_" + prefix + string(rune('a'+i)) + ".txt": "up"}, prefix)
	}
	git(t, clone, "push", "--quiet", "origin", "main")
}

// PushUpstreamCommits only adds new files and cannot produce a conflict, so a conflicting scenario needs both sides to write the same file.
func PushUpstreamFile(t TB, bare, name, content, msg string) {
	t.Helper()
	clone := t.TempDir()
	git(t, clone, "clone", "--quiet", bare, ".")
	git(t, clone, "config", "user.email", "test@gitdash.local")
	git(t, clone, "config", "user.name", "upstream")
	CommitFiles(t, clone, map[string]string{name: content}, msg)
	git(t, clone, "push", "--quiet", "origin", "main")
}

func Detach(t TB, dir string) {
	t.Helper()
	git(t, dir, "checkout", "--detach", "--quiet", "HEAD")
}

func MakeWorktree(t TB, dir, wtDir, branch string) {
	t.Helper()
	git(t, dir, "worktree", "add", "--quiet", wtDir, "-b", branch)
}

func NewRepo(t TB, upstream bool) (dir, origin string) {
	t.Helper()
	dir = t.TempDir()
	Init(t, dir)
	CommitFiles(t, dir, map[string]string{"base.txt": "base"}, "base")
	if upstream {
		origin = filepath.Join(t.TempDir(), "origin.git")
		InitBare(t, origin)
		AddUpstream(t, dir, origin)
	}
	return dir, origin
}

func WriteUncommitted(t TB, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func WriteUntracked(t TB, dir string, files map[string]string) {
	t.Helper()
	WriteUncommitted(t, dir, files)
}

func BreakGit(t TB, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Remote-tracking refs only move on fetch, so tests simulating behind/diverged need this after pushing to origin.
func FetchLocal(t TB, dir string) {
	t.Helper()
	git(t, dir, "fetch", "--quiet", "origin")
}

func Checkout(t TB, dir, branch string) {
	git(t, dir, "checkout", "--quiet", branch)
}

func NewBranch(t TB, dir, branch string) {
	git(t, dir, "checkout", "-qB", branch)
}
