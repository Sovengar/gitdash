// Package gitstatus reads git state through subprocesses: no git library, the git binary is the only dependency.
package gitstatus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
)

// syncFallbackBranch is probed when the configured sync ref is missing: many repos still live on master.
const syncFallbackBranch = "master"

// commitsLogDepth caps the commits panel's two logs: the layout paints only what fits, so this is a fetch ceiling, not what the panel always shows.
const commitsLogDepth = 15

type Snapshot struct {
	Status      Status
	Files       []FileEntry
	Commits     []Commit
	SyncCommits []Commit
	LastCommit  int64
	Worktrees   []Worktree
	SyncBranch  string
	SyncBehind  int
	SyncKnown   bool
	Err         string
}

type Worktree struct {
	Path   string
	Branch string
	Head   string
}

func (s Snapshot) State(hasRepo bool) State {
	if s.Err != "" {
		return StateError
	}
	if !hasRepo {
		return StateNoRepo
	}
	return s.Status.Derive()
}

// Collect never fails hard: the error travels inside the Snapshot so the UI can still render the repo.
func Collect(ctx context.Context, dir, syncBranch string, allowFallback bool) Snapshot {
	var snap Snapshot

	out, err := runGit(ctx, dir, cmdlog.ClassRead, "status", "--porcelain=v2", "--branch")
	if err != nil {
		snap.Err = firstLine(err.Error())
		return snap
	}
	st, files := ParsePorcelain(string(out))
	st.Branch = normalizeBranch(st)
	snap.Status, snap.Files = st, files

	if syncBranch != "" {
		// Filled even when the comparison fails: the UI still shows the branch name.
		snap.SyncBranch = syncBranch
		n, ok := syncBehind(ctx, dir, syncBranch)
		if !ok && allowFallback && syncBranch != syncFallbackBranch {
			// The fallback probe lives here and not earlier: main resolves in every modern repo, so an extra rev-list per repo per cycle would only be paid where it is needed.
			n, ok = syncBehind(ctx, dir, syncFallbackBranch)
			if ok {
				snap.SyncBranch = syncFallbackBranch
			}
		}
		if ok {
			snap.SyncBehind = n
			snap.SyncKnown = true
		}
		// The panel's second group: the same ref the divergence counts, so what is compared and
		// what is shown cannot disagree; skipped on the sync branch itself (the lists would duplicate).
		if snap.SyncBranch != snap.Status.Branch {
			if syncOut, err := runGit(ctx, dir, cmdlog.ClassRead, "log", "-"+strconv.Itoa(commitsLogDepth), "--format=%h%x00%ct%x00%s", snap.SyncBranch); err == nil {
				snap.SyncCommits = ParseLog(string(syncOut))
				markOneSided(snap.SyncCommits, oneSidedShas(ctx, dir, "HEAD.."+snap.SyncBranch))
			}
		}
	}

	logOut, err := runGit(ctx, dir, cmdlog.ClassRead, "log", "-"+strconv.Itoa(commitsLogDepth), "--format=%h%x00%ct%x00%s")
	if err == nil {
		snap.Commits = ParseLog(string(logOut))
		snap.LastCommit = lastCommitWhen(snap.Commits)
		if snap.SyncKnown && snap.SyncBranch != snap.Status.Branch {
			markOneSided(snap.Commits, oneSidedShas(ctx, dir, snap.SyncBranch+"..HEAD"))
		}
	}
	// A repo with no commits is legitimate, so the log error is ignored.

	if wtOut, err := runGit(ctx, dir, cmdlog.ClassRead, "worktree", "list", "--porcelain"); err == nil {
		snap.Worktrees = ParseWorktrees(string(wtOut), dir)
	}
	return snap
}

// Split out because the empty-log branch is only reachable with a git that exits 0 and prints nothing, which no fixture produces.
func lastCommitWhen(commits []Commit) int64 {
	if len(commits) == 0 {
		return 0
	}
	return commits[0].When
}

func syncBehind(ctx context.Context, dir, sync string) (int, bool) {
	out, err := runGit(ctx, dir, cmdlog.ClassRead, "rev-list", "--count", "HEAD.."+sync)
	if err != nil {
		return 0, false
	}
	// known comes from the conversion, so an error check here is unreachable; anything unexpected still degrades to known=false.
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	return n, err == nil
}

// oneSidedShas returns the full shas of the (at most commitsLogDepth) commits the range reserves for one side; a sha and never a position is what the panel marks, because `git log` is date-ordered and a merge can put a shared commit above a one-sided one.
func oneSidedShas(ctx context.Context, dir, rng string) map[string]bool {
	out, err := runGit(ctx, dir, cmdlog.ClassRead, "rev-list", "--max-count="+strconv.Itoa(commitsLogDepth), rng)
	if err != nil {
		return nil
	}
	shas := map[string]bool{}
	for _, sha := range strings.Fields(string(out)) {
		shas[sha] = true
	}
	return shas
}

// markOneSided matches by prefix: the log writes abbreviated shas, rev-list prints full ones. An empty parsed sha would prefix-match everything, so it is skipped.
func markOneSided(commits []Commit, shas map[string]bool) {
	for i := range commits {
		if commits[i].Sha == "" {
			continue
		}
		for sha := range shas {
			if strings.HasPrefix(sha, commits[i].Sha) {
				commits[i].OneSided = true
			}
		}
	}
}

func normalizeBranch(st Status) string {
	if st.Detached && st.Branch == "" && len(st.OID) >= 7 {
		return st.OID[:7]
	}
	return st.Branch
}

// emit runs in up to `concurrency` goroutines, so it MUST be safe for concurrent use.
func StreamPool(ctx context.Context, projects []discovery.Project, defaultSync string, defaultExplicit bool, concurrency int, emit func(path string, snap Snapshot)) {
	// Clamped to 1..4 workers per CPU: a bogus 0 or 99999 in the config must not wedge or flood the machine with git processes.
	concurrency = min(max(concurrency, 1), runtime.NumCPU()*4)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, p := range projects {
		wg.Add(1)
		go func(p discovery.Project) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if !p.HasRepo {
				emit(p.Path, Snapshot{})
				return
			}
			emit(p.Path, Collect(ctx, p.Path, SyncFor(p, defaultSync), SyncForAllowsFallback(p, defaultExplicit)))
		}(p)
	}
	wg.Wait()
}

func SyncFor(p discovery.Project, defaultSync string) string {
	if p.SyncBranch != "" {
		return p.SyncBranch
	}
	return defaultSync
}

// Only when nobody declared the ref: an inherited default is a guess the repo can disprove, a written branch is an intent.
func SyncForAllowsFallback(p discovery.Project, defaultExplicit bool) bool {
	return p.SyncBranch == "" && !defaultExplicit
}

// class is the only thing separating the keypress fetch from the automatic scan, so the caller has to supply it.
func Fetch(ctx context.Context, dir string, class cmdlog.Class, args ...string) error {
	if len(args) == 0 {
		args = []string{"fetch", "--prune"}
	}
	_, err := runGit(ctx, dir, class, args...)
	return err
}

// The only action executor: argv arrives already resolved from config, so there are no per-action wrappers and no hidden flag defaults (pull policy stays in the user's gitconfig).
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	return runGitCombined(ctx, dir, cmdlog.ClassAction, args...)
}

// On demand only, since it is read when opening a PR and never during the scan; it goes through runGit, so the command log records it as a read.
func RemoteURL(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, cmdlog.ClassRead, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Uses --git-path because in a worktree .git is a file and the rebase state lives under .git/worktrees/<name>/.
func RebaseInProgress(ctx context.Context, dir string) bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		out, err := runGit(ctx, dir, cmdlog.ClassRead, "rev-parse", "--git-path", name)
		if err != nil {
			continue
		}
		// Positive condition on purpose: an empty p makes filepath.Join(dir, "") the repo itself, so Stat would see a directory and report a rebase for every repo.
		if p := strings.TrimSpace(string(out)); p != "" {
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return true
			}
		}
	}
	return false
}

func RemoveWorktree(ctx context.Context, repoDir, wtPath string, withForce bool) (string, error) {
	return runGitCombined(ctx, repoDir, cmdlog.ClassAction, RemoveWorktreeArgv(wtPath, withForce)...)
}

// SyncArgv is the single source of the sync argv: the base comes from `commands.sync`
// and the ref from the repo's resolved sync branch.
// The base is copied so a shared default is never mutated, and the remote is explicit
// because `git pull <branch>` without one treats the branch as a remote.
func SyncArgv(base []string, remote, sync string) []string {
	args := append(make([]string, 0, len(base)), base...)
	return append(args, remote, sync)
}

// Split out so the detail and the command log show the same argv without duplicating the build here and drifting from what ran.
func RemoveWorktreeArgv(wtPath string, withForce bool) []string {
	args := []string{"worktree", "remove"}
	if withForce {
		args = append(args, "--force")
	}
	return append(args, wtPath)
}

// LC_ALL=C so git error messages stay recognizable (diverged, no upstream) regardless of the user's locale.
func gitEnv() []string {
	env := os.Environ()
	out := env[:0]
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
	return append(out, "LC_ALL=C")
}

// First non-`hint:` line, because hints are verbose and already shown in full in the detail panel.
func FailureReason(out string, err error) string {
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "hint:") {
			continue
		}
		return l
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func runGit(ctx context.Context, dir string, class cmdlog.Class, args ...string) ([]byte, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	recordExec(dir, class, args, out, err, time.Since(start))
	if err != nil {
		if msg := stderr.String(); msg != "" {
			return nil, fmt.Errorf("git %v: %s", args, firstLine(msg))
		}
		return nil, fmt.Errorf("git %v: %w", args, err)
	}
	return out, nil
}

// The only exec path holding the full output git printed, hence the only one that can classify the outcome.
func runGitCombined(ctx context.Context, dir string, class cmdlog.Class, args ...string) (string, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	recordExec(dir, class, args, []byte(out), err, time.Since(start))
	return out, err
}

// The logged repo name is the directory basename: the exec layer knows neither the project list nor a marker-overridden name.
func recordExec(dir string, class cmdlog.Class, args []string, out []byte, err error, dur time.Duration) {
	if cmdlog.Active() == nil {
		return
	}
	code := 0
	if err != nil {
		code = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
	}
	argv := append([]string{"git"}, args...)
	// An empty argv is reachable from user config (pull = ""); without this guard args[0] panics and takes the action goroutine, and the TUI, down.
	action := ""
	if len(args) > 0 {
		action = args[0]
	}
	cmdlog.RecordExec(cmdlog.Entry{
		Dir:     dir,
		Repo:    filepath.Base(dir),
		Class:   class,
		Action:  action,
		Argv:    argv,
		Exit:    code,
		Dur:     dur,
		Outcome: Classify(args, string(out), code),
	})
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
