// Package tui implements the gitdash dashboard on Bubbletea v2.
package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
	"gitdash/internal/cmdlog"
	"gitdash/internal/config"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/state"
)

type event interface{}

type scanProjectsMsg struct {
	projects []discovery.Project
	note     string
}

type statusMsg struct {
	path string
	snap gitstatus.Snapshot
}

type collectDoneMsg struct{}

type fetchStateMsg struct {
	path  string
	state string
	err   string
}

type fetchDoneMsg struct{ ok, failed int }

// cmd is the resolved argv (the pull policy can come from the gitconfig, so the UI cannot assume flags) and rebaseInProgress means the pull --rebase left the repo mid-rebase instead of failing clean.
type actionMsg struct {
	path, kind, cmd  string
	output           string
	err              string
	rebaseInProgress bool
}

// gen is the attempt token: a result whose token is no longer current is discarded (cancelled with esc or superseded).
type worktreeRemovedMsg struct {
	parent, wtPath, name string
	output, err          string
	force                bool
	gen                  int
	cmd                  string
}

// action and argv travel so the command log can record what was launched: a handoff captures no output, so the argv is all that is left.
type execDoneMsg struct {
	path, action string
	argv         []string
	err          error
}

type cmdResultMsg struct {
	path, command, output, exit string
}

type notifyMsg struct {
	text  string
	level toastLevel
}

type tickMsg struct{}

type actionResult struct {
	kind, cmd, output, err string
}

type pullOption struct {
	key   string
	kind  string
	label string
	git   bool
}

// Single source of the variants (a hardcoded list elsewhere silently missed a new one), and `pull_ai` is absent from PullKinds because it is not a pull of git but its own handoff.
var pullOptions = []pullOption{
	{"p", "pull", "default", true},
	{"r", "pull_rebase", "rebase", true},
	{"f", "pull_ff", "ff-only", true},
	{"m", "pull_merge", "merge", true},
	{"a", "pull_ai", "AI", false},
}

// Derived from pullOptions so the two cannot desync, and each variant exists so the gitconfig policy can be overridden without editing gitdash's config.
var PullKinds = func() map[string]string {
	kinds := make(map[string]string, len(pullOptions))
	for _, o := range pullOptions {
		if o.git {
			kinds[o.key] = o.kind
		}
	}
	return kinds
}()

func IsPullKind(kind string) bool {
	for _, k := range PullKinds {
		if k == kind {
			return true
		}
	}
	return false
}

type visualOption struct {
	key           string
	sub           string
	label         string
	needsUpstream bool
}

// Single source of the variants, so a new one reaches the prompt and the labels; it is absent from PullKinds because git-sim is not git.
var visualOptions = []visualOption{
	{"p", "pull", "pull", false},
	{"m", "merge", "merge", true},
	{"r", "rebase", "rebase", true},
}

func visualOptionForKey(key string) (visualOption, bool) {
	for _, o := range visualOptions {
		if o.key == key {
			return o, true
		}
	}
	return visualOption{}, false
}

func visualOptionForSub(sub string) (visualOption, bool) {
	for _, o := range visualOptions {
		if o.sub == sub {
			return o, true
		}
	}
	return visualOption{}, false
}

// Captures the path when armed: the second key resolves on that row, not on whatever sits under the cursor by then.
type armedPull struct {
	path string
}

// Captures path, upstream and behind when armed so the variant argv and the no-op guard are decided by the chosen row, not by the cursor later.
type armedVisual struct {
	path     string
	upstream string
	behind   int // commits the upstream is missing from HEAD, as the last scan saw them
}

type armedRemoval struct {
	wtPath string
	parent string
	name   string
	force  bool
}

func (a armedRemoval) matches(parent, wtPath string) bool {
	return filepath.Clean(a.parent) == filepath.Clean(parent) &&
		filepath.Clean(a.wtPath) == filepath.Clean(wtPath)
}

type Model struct {
	cfg      config.Config
	projects []discovery.Project
	states   map[string]gitstatus.Snapshot

	store     *state.Store
	cursor    int
	offset    int
	onlyDirty bool
	search    string

	// A field and not a direct call because the handoff is the only thing a test cannot run (it suspends the program, and nobody resumes a suspended program); injecting it lets the test return the exit message without yielding.
	handoff handoffFunc

	searchActive bool
	searchInput  textinput.Model
	collapsed    map[string]bool

	expanded map[string]bool

	scanning bool

	events  chan event
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model

	fetchStates map[string]string
	running     map[string]string
	lastAction  map[string]actionResult

	toasts toastManager

	armed       *armedRemoval
	pullArmed   *armedPull
	visualArmed *armedVisual
	// removeTokens maps parent repo path → current attempt token (the in-flight guard is per parent, so the token is too); a result whose token is no longer current is discarded.
	removeGen    int
	removeTokens map[string]int

	width, height int

	// logCache avoids re-copying the ring on every frame: it refreshes only when the last sequence changes.
	logOpen     bool
	logShowAll  bool
	logOffset   int
	logCache    []cmdlog.Entry
	logCacheSeq int

	// pr == nil is closed; it is a view mode like the log panel and not a prefix-key armed state, because a form lives for N keystrokes.
	pr        *prDraft
	prPending *prSubmission

	cmdOpen  bool
	cmdInput textinput.Model

	lastCmd map[string]cmdResult
}

type cmdResult struct {
	command, output, exit string
}

const searchPlaceholder = "name/group…"

func New(cfg config.Config) Model {
	// The command log only exists in the TUI, which is where there are keys to audit; --print does not install it and tests substitute their own.
	cmdlog.SetRecorder(cmdlog.New(cmdlog.DefaultCap))
	ctx, cancel := context.WithCancel(context.Background())
	store, _ := state.NewStore()
	m := Model{
		cfg:         cfg,
		store:       store,
		states:      map[string]gitstatus.Snapshot{},
		events:      make(chan event, 256),
		ctx:         ctx,
		cancel:      cancel,
		fetchStates: map[string]string{},
		running:     map[string]string{},
		lastAction:  map[string]actionResult{},
		lastCmd:     map[string]cmdResult{},
		collapsed:   map[string]bool{},
		expanded:    map[string]bool{},

		removeTokens: map[string]int{},
		handoff:      tea.ExecProcess,
	}
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	in := textinput.New()
	in.Placeholder = searchPlaceholder
	in.Prompt = "/" // the prompt renders as [/here][cursor], not "filter: "

	ci := textinput.New()
	// The first placeholder rune sits under the cursor (bubbles v2 placeholderView), hence the leading space so the cursor does not cover a letter.
	ci.Placeholder = " npm test · git status… (empty enter = interactive shell)"
	ci.Prompt = "! "
	m.cmdInput = ci
	m.searchInput = in
	m.scanning = true

	if path, err := cache.Path(); err == nil {
		m.projects = cache.Load(path, cfg.Marker)
	}
	// The load splits both spaces by prefix: WorktreePrefix keys go to `expanded` (true = expanded), the rest to `collapsed` (true = collapsed).
	if store != nil {
		if persisted := store.LoadCollapsed(); persisted != nil {
			m.loadPersisted(persisted)
		}
	}
	return m
}

func (m *Model) loadPersisted(persisted map[string]bool) {
	for k, v := range persisted {
		if path, ok := strings.CutPrefix(k, state.WorktreePrefix); ok {
			m.expanded[path] = v
			continue
		}
		m.collapsed[k] = v
	}
}

// Without this a config problem would only show on stderr (invisible behind the alt screen) and the affected key would look dead.
func (m *Model) NotifyConfig(warn string) {
	m.toasts.showWarning(warn)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startScanCmd(),
		waitForEvent(m.events),
		m.spinner.Tick,
		tickCmd(),
	)
}

// Rearms the channel read after every consumed event (one event per tea.Cmd), otherwise only the first message ever arrives.
func waitForEvent(ch <-chan event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

func sendEvent(ctx context.Context, ch chan<- event, ev event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// The one-scan-at-a-time guard lives in the `r` key handler, since New already sets scanning=true.
func (m *Model) startScanCmd() tea.Cmd {
	m.scanning = true

	ctx, cancel := context.WithCancel(m.ctx)
	events := m.events
	cfg := m.cfg
	go func() {
		defer cancel()
		projects, err := discovery.Scan(cfg)
		note := ""
		if err != nil {
			note = err.Error()
		}
		sendEvent(ctx, events, scanProjectsMsg{projects: projects, note: note})
		if ctx.Err() != nil {
			return
		}
		gitstatus.StreamPool(ctx, projects, cfg.SyncBranch, cfg.SyncBranchExplicit, 8, func(path string, snap gitstatus.Snapshot) {
			sendEvent(ctx, events, statusMsg{path: path, snap: snap})
		})
		if ctx.Err() != nil {
			return
		}
		sendEvent(ctx, events, collectDoneMsg{})
	}()
	return nil
}

func (m *Model) fetchTargets() []string {
	var paths []string
	for _, p := range m.projects {
		if !p.HasRepo {
			continue
		}
		snap := m.states[p.Path]
		if !snap.Status.HasUpstream || snap.Err != "" {
			continue
		}
		if m.fetchStates[p.Path] == "fetching" {
			continue
		}
		paths = append(paths, p.Path)
	}
	return paths
}

// class separates the keypress fetch from the automatic one (same argv, different origin) and the Cmd always returns nil, so callers must not accumulate the result: an `if c != nil` on it was dead code.
func (m *Model) fetchBatchCmd(paths []string, class cmdlog.Class) tea.Cmd {
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	events := m.events
	timeout := m.cfg.FetchTimeout
	concurrency := m.cfg.FetchConcurrency
	args := m.cfg.CmdArgs("fetch")

	go func() {
		defer cancel()
		var wg sync.WaitGroup
		sem := make(chan struct{}, max(1, concurrency))
		var mu sync.Mutex
		ok, failed := 0, 0

		for _, path := range paths {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				sendEvent(ctx, events, fetchStateMsg{path: p, state: "fetching"})
				if !acquireSlot(ctx, sem) {
					return
				}
				defer func() { <-sem }()
				fctx, fcancel := context.WithTimeout(ctx, timeout)
				defer fcancel()
				if err := gitstatus.Fetch(fctx, p, class, args...); err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					sendEvent(ctx, events, fetchStateMsg{path: p, state: "failed", err: err.Error()})
					return
				}
				mu.Lock()
				ok++
				mu.Unlock()
				sendEvent(ctx, events, fetchStateMsg{path: p, state: "ok"})
				sync, syncFallback := m.syncOf(p)
				sendEvent(ctx, events, statusMsg{path: p, snap: gitstatus.Collect(ctx, p, sync, syncFallback)})
			}(path)
		}
		wg.Wait()
		sendEvent(ctx, events, fetchDoneMsg{ok: ok, failed: failed})
	}()
	return nil
}

// Split out from the inline select because the "cancelled while waiting for a slot" branch is only reachable with more repos than slots (provoking it needed a Peterson race), and with a free function the test fills the semaphore and cancels deterministically.
func acquireSlot(ctx context.Context, sem chan struct{}) bool {
	select {
	case sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *Model) startActionCmd(path, kind string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = kind
	appCtx := m.ctx
	events := m.events
	args := m.cfg.CmdArgs(kind)
	// The resolved argv is composed on the main goroutine (reading cfg) so the message is deterministic with respect to the key that triggered it.
	resolved := "git " + strings.Join(args, " ")
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, actionTimeout())
		defer cancel()
		out, err := gitstatus.Run(ctx, path, args...)
		errStr := ""
		if err != nil {
			// The process error is always "exit status 1"; the real reason is in git's combined output.
			errStr = gitstatus.FailureReason(out, err)
		}
		msg := actionMsg{path: path, kind: kind, cmd: resolved, output: out, err: errStr}
		if errStr != "" && IsPullKind(kind) {
			msg.rebaseInProgress = gitstatus.RebaseInProgress(ctx, path)
		}
		sendEvent(appCtx, events, msg)
		if err == nil {
			sync, syncFallback := m.syncOf(path)
			sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path, sync, syncFallback)})
		}
	}()
	return nil
}

func (m *Model) busyActionCmd(path string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	return nil
}

// It does not use recollectCmd because that guard would collide with the running flag of this very action; on success it also publishes the parent's snapshot so the sub-row disappears.
func (m *Model) removeWorktreeCmd(parent, wtPath, name string, withForce bool, token int) tea.Cmd {
	if cmd := m.busyActionCmd(parent); cmd != nil {
		return cmd
	}
	m.running[parent] = "worktree_remove"
	appCtx := m.ctx
	events := m.events
	syncBranch, syncFallback := m.syncOf(parent)
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, actionTimeout())
		defer cancel()
		out, err := gitstatus.RemoveWorktree(ctx, parent, wtPath, withForce)
		errStr := ""
		if err != nil {
			errStr = gitstatus.FailureReason(out, err)
		}
		sendEvent(appCtx, events, worktreeRemovedMsg{
			parent: parent, wtPath: wtPath, name: name,
			output: out, err: errStr, force: withForce, gen: token,
			cmd: "git " + strings.Join(gitstatus.RemoveWorktreeArgv(wtPath, withForce), " "),
		})
		if err == nil {
			sendEvent(appCtx, events, statusMsg{path: parent, snap: gitstatus.Collect(appCtx, parent, syncBranch, syncFallback)})
		}
	}()
	return nil
}

func (m *Model) recollectCmd(path string) tea.Cmd {
	if _, busy := m.running[path]; busy {
		return nil
	}
	m.running[path] = "collect"
	appCtx := m.ctx
	events := m.events
	go func() {
		sync, syncFallback := m.syncOf(path)
		sendEvent(appCtx, events, statusMsg{path: path, snap: gitstatus.Collect(appCtx, path, sync, syncFallback)})
	}()
	return nil
}

func (m *Model) toastCmd(level toastLevel, text string) tea.Cmd {
	return func() tea.Msg { return notifyMsg{text: text, level: level} }
}

type handoffFunc func(*exec.Cmd, tea.ExecCallback) tea.Cmd

// A method and not a closure inside tea.ExecProcess for two reasons: that closure only runs when the process ends and no test can lend the terminal to observe it (the program is suspended meanwhile), and the command log depends on the action and argv being right, since a "editor" where "lazygit" was meant is invisible in the code.
func (m *Model) handoffDone(action, path string, argv []string, err error) tea.Msg {
	return execDoneMsg{path: path, action: action, argv: argv, err: err}
}

func (m *Model) openEditorCmd(path string) tea.Cmd {
	argv := []string{m.cfg.Editor}
	cmd := exec.Command(m.cfg.Editor)
	cmd.Dir = path
	return m.handoff(cmd, func(err error) tea.Msg {
		return m.handoffDone("editor", path, argv, err)
	})
}

func (m *Model) openLazygitCmd(path string) tea.Cmd {
	if _, err := exec.LookPath("lazygit"); err != nil {
		return m.toastCmd(toastWarning, "lazygit not installed")
	}
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "lazygit"
	argv := []string{"lazygit"}
	cmd := exec.Command("lazygit")
	cmd.Dir = path
	return m.handoff(cmd, func(err error) tea.Msg {
		return m.handoffDone("lazygit", path, argv, err)
	})
}

// The TUI resolves the vars (config cannot import gitstatus), and a worktree with no marker has no snapshot: its branch comes from the parent's worktree list and the rest is left literal instead of invented.
func (m *Model) aiVars(path string) map[string]string {
	snap, ok := m.effectiveSnapshot(path)
	if !ok {
		vars := map[string]string{}
		if wt, ok := m.worktreeFor(path); ok {
			vars["branch"] = worktreeBranchLabel(wt)
		}
		return vars
	}
	return map[string]string{
		"branch":   snap.Status.Branch,
		"upstream": snap.Status.Upstream,
		"state":    snap.State(true).String(),
		"ahead":    strconv.Itoa(snap.Status.Ahead),
		"behind":   strconv.Itoa(snap.Status.Behind),
		"sync":     snap.SyncBranch,
	}
}

// A discovered project is indexed by the project path and not the row's, because the two can differ (symlinks, trailing slashes).
func (m *Model) effectiveSnapshot(path string) (gitstatus.Snapshot, bool) {
	if p, ok := m.discoveredByPath(path); ok {
		snap, ok := m.states[p.Path]
		return snap, ok
	}
	snap, ok := m.states[path]
	return snap, ok
}

func (m *Model) worktreeFor(path string) (gitstatus.Worktree, bool) {
	clean := filepath.Clean(path)
	for _, p := range m.projects {
		for _, wt := range m.states[p.Path].Worktrees {
			if filepath.Clean(wt.Path) == clean {
				return wt, true
			}
		}
	}
	return gitstatus.Worktree{}, false
}

func (m *Model) pullAIArgv(path, prompt string) []string {
	return config.BuildAIArgv(m.cfg.AICommand("pull"), prompt, m.aiVars(path))
}

func (m *Model) startPullAICmd(path string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	prompt, err := discovery.MarkerPrompt(path, m.cfg.Marker, "pull")
	if err != nil {
		return m.toastCmd(toastWarning, "marker error — fix .gitdash.toml first")
	}
	if strings.TrimSpace(prompt) == "" {
		return m.toastCmd(toastInfo, "no AI pull prompt (.gitdash.toml [ai.pull].prompt)")
	}
	if strings.TrimSpace(m.cfg.AICommand("pull")) == "" {
		return m.toastCmd(toastInfo, "ai.pull command not configured (~/.config/gitdash/config.toml)")
	}
	argv := m.pullAIArgv(path, prompt)
	if argv[0] == "" {
		// First field resolved to empty (e.g. "{branch}" with no branch): LookPath would answer " not installed", which explains nothing.
		return m.toastCmd(toastWarning, "ai command: empty executable")
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s not installed", argv[0]))
	}
	return m.openPullAICmd(path, argv)
}

func (m *Model) openPullAICmd(path string, argv []string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "pull_ai"
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = path
	return m.handoff(cmd, func(err error) tea.Msg {
		return m.handoffDone("pull_ai", path, argv, err)
	})
}

// Mandatory, not cosmetic: without `--media-dir` git-sim writes `git-sim_media/` into the repo and gitdash would mark it dirty, so an uncreatable dir must abort with a toast.
func visualMediaDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, cache.DirName, "git-sim")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func visualArgv(sub, upstream, mediaDir string) []string {
	argv := []string{"git-sim", "--media-dir", mediaDir, sub}
	if o, ok := visualOptionForSub(sub); ok && o.needsUpstream && upstream != "" {
		argv = append(argv, upstream)
	}
	return argv
}

func (m *Model) startVisualCmd(path, upstream, sub string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	mediaDir, err := visualMediaDir()
	if err != nil {
		return m.toastCmd(toastError, fmt.Sprintf("git-sim media dir: %v", err))
	}
	if _, err := exec.LookPath("git-sim"); err != nil {
		return m.toastCmd(toastWarning, "git-sim not installed")
	}
	argv := visualArgv(sub, upstream, mediaDir)
	m.running[path] = "visual"
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = path
	return m.handoff(cmd, func(err error) tea.Msg {
		return m.handoffDone("visual", path, argv, err)
	})
}

// A function, not a const: Go does not instrument constant expressions, so a package const would leave its ARITHMETIC_BASE mutant permanently NOT COVERED.
func commandTimeout() time.Duration { return 5 * time.Minute }

// The literal lives in a function on purpose: written inline in WithTimeout, the ARITHMETIC_BASE mutant turns the deadline into `120 / time.Second`, i.e. zero, and then every gitstatus.Run fails instantly in a loop over all repos.
func actionTimeout() time.Duration { return 120 * time.Second }

func runShellCmd(ctx context.Context, dir, shell, command string) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout())
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		code := 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return out.String(), code
	}
	return out.String(), 0
}

// Not a terminal handoff (nvim-style, so the previous output stays visible until the next command) but still through $SHELL -c, which is what loads the user's aliases and rc.
func (m *Model) openCmdCmd(path, command string) tea.Cmd {
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "cmd"
	shell := userShell()
	appCtx := m.ctx
	events := m.events
	go func() {
		start := time.Now()
		out, code := runShellCmd(appCtx, path, shell, command)
		// The `!` command is not classified (its output is arbitrary, not git's): the log keeps the argv and the exit code.
		cmdlog.RecordExec(cmdlog.Entry{
			Repo:   m.nameOf(path),
			Dir:    path,
			Class:  cmdlog.ClassAction,
			Action: "cmd",
			Argv:   append([]string{shell, "-c"}, command),
			Exit:   code,
			Dur:    time.Since(start),
		})
		sendEvent(appCtx, events, cmdResultMsg{
			path: path, command: command, output: out,
			exit: fmt.Sprintf("%d", code),
		})
	}()
	return nil
}

// $SHELL (the user's) so the session loads their aliases and rc, /bin/sh only as fallback; split out because two paths use it and the interactive handoff cannot be exercised in a test (tea.ExecProcess needs a TTY).
func userShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func (m *Model) openShellCmd(path string) tea.Cmd {
	shell := userShell()
	if _, err := exec.LookPath(shell); err != nil {
		return m.toastCmd(toastWarning, "shell not found")
	}
	if prev, busy := m.running[path]; busy {
		return m.toastCmd(toastWarning, fmt.Sprintf("%s already running in %s", prev, m.nameOf(path)))
	}
	m.running[path] = "shell"
	cmd := exec.Command(shell)
	cmd.Dir = path
	return m.handoff(cmd, func(err error) tea.Msg {
		return m.handoffDone("shell", path, []string{shell}, err)
	})
}

func execExit(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// Without this line the log would only say "git pull", and "I pressed p and picked rebase" could not be told from "the gitconfig decided for me".
func (m *Model) logIntent(key, action string) {
	if cmdlog.Active() == nil {
		return
	}
	r, ok := m.selected()
	if !ok && actionNeedsRow(action) {
		return
	}
	entry := cmdlog.Entry{Class: cmdlog.ClassAction, Key: key, Action: action}
	if ok {
		entry.Repo = m.nameOf(r.project.Path)
		entry.Dir = r.project.Path
	}
	cmdlog.RecordIntent(entry)
}

func (m *Model) nameOf(path string) string {
	for _, p := range m.projects {
		if p.Path == path {
			if p.IsWorktree {
				return filepath.Base(path)
			}
			return p.Name
		}
	}
	if path == "" {
		return path
	}
	return filepath.Base(path)
}

func (m *Model) syncOf(path string) (string, bool) {
	for _, p := range m.projects {
		if p.Path == path {
			return gitstatus.SyncFor(p, m.cfg.SyncBranch),
				gitstatus.SyncForAllowsFallback(p, m.cfg.SyncBranchExplicit)
		}
	}
	return m.cfg.SyncBranch, !m.cfg.SyncBranchExplicit
}

func (m *Model) saveCollapsed() {
	if m.store == nil {
		return
	}
	// No capacity hint: the map holds a handful of groups, and hinting with collapsed+expanded turned negative (runtime panic) with more expanded worktrees than collapsed groups.
	combined := make(map[string]bool)
	for k, v := range m.collapsed {
		combined[k] = v
	}
	for path, v := range m.expanded {
		combined[state.WorktreePrefix+path] = v
	}
	_ = m.store.SaveCollapsed(combined)
}

func (m *Model) selectedEntry() (tableEntry, bool) {
	return entryAt(m.entries(), m.cursor)
}

// A worktree sub-row resolves to a synthetic row with the worktree path and HasRepo=true, so every operation reading r.project.Path/HasRepo/MarkerErr acts on the worktree.
func (m *Model) selected() (row, bool) {
	e, ok := m.selectedEntry()
	if !ok {
		return row{}, false
	}
	switch e.kind {
	case kindRepo:
		return e.r, true
	case kindWorktree:
		return m.worktreeRow(e.wt), true
	default:
		return row{}, false
	}
}

func (m *Model) worktreeRow(wt gitstatus.Worktree) row {
	if p, ok := m.discoveredByPath(wt.Path); ok {
		snap := m.states[p.Path]
		return row{project: p, snap: snap, state: snap.State(p.HasRepo)}
	}
	return row{project: discovery.Project{
		Path:       wt.Path,
		Name:       filepath.Base(wt.Path),
		HasRepo:    true,
		IsWorktree: true,
	}}
}

func (m *Model) discoveredByPath(path string) (discovery.Project, bool) {
	clean := filepath.Clean(path)
	for _, p := range m.projects {
		if filepath.Clean(p.Path) == clean {
			return p, true
		}
	}
	return discovery.Project{}, false
}
