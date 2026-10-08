package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cache"
	"gitdash/internal/cmdlog"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// An overlay that no longer fits is closed with a warning: staying with the keyboard captured and no visible form means writing blind.
		if m.pr != nil && !m.prFits() {
			m.closePR()
			m.toasts.showWarning("terminal too small — closed the PR form")
		}
		m.prFit()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		m.toasts.update()
		return m, tickCmd()

	case scanProjectsMsg:
		m.projects = msg.projects
		m.clampCursor()
		if msg.note != "" {
			m.toasts.showError("roots: " + msg.note)
		}
		return m.withPump(nil)

	case statusMsg:
		m.states[msg.path] = msg.snap
		delete(m.running, msg.path)
		m.clampCursor()
		return m.withPump(nil)

	case collectDoneMsg:
		m.scanning = false
		if path, err := cache.Path(); err == nil {
			projects := m.projects
			go func() { _ = cache.Save(path, projects) }()
		}
		if m.cfg.FetchAuto {
			m.fetchBatchCmd(m.fetchTargets(), cmdlog.ClassAuto)
		}
		return m.withPump(nil)

	case fetchStateMsg:
		m.fetchStates[msg.path] = msg.state
		if msg.state == "failed" {
			m.toasts.showError(fmt.Sprintf("fetch failed %s: %s", m.nameOf(msg.path), msg.err))
		}
		return m.withPump(nil)

	case fetchDoneMsg:
		if msg.failed > 0 {
			m.toasts.showWarning(fmt.Sprintf("fetch: %d ok, %d failed", msg.ok, msg.failed))
		} else if msg.ok == 1 {
			m.toasts.showSuccess("fetch ok")
		} else {
			m.toasts.showSuccess(fmt.Sprintf("fetch ok (%d repos)", msg.ok))
		}
		return m.withPump(nil)

	case actionMsg:
		delete(m.running, msg.path)
		// sync's git output (rebase/applied/dropping lines) is transient and the argv is already audited in the `l` panel, so it reports through its toast only and drops any action block the card was holding for this repo.
		if toastOnlyActions[msg.kind] {
			delete(m.lastAction, msg.path)
		} else {
			m.lastAction[msg.path] = actionResult{kind: msg.kind, cmd: msg.cmd, output: msg.output, err: msg.err}
		}
		note := actionNote(msg.kind, m.nameOf(msg.path), msg.cmd, msg.output, msg.err, msg.rebaseInProgress)
		// The toast-only actions drop the argv, so on success the classified verdict is what tells the user what the sync actually did.
		if msg.err == "" && toastOnlyActions[msg.kind] && msg.outcome != "" {
			note += " — " + msg.outcome
		}
		if msg.err != "" {
			m.toasts.showError(note)
		} else {
			m.toasts.showSuccess(note)
		}
		return m.withPump(nil)

	case worktreeRemovedMsg:
		tok, inflight := m.removeTokens[msg.parent]
		switch {
		case inflight && msg.gen == tok:
			delete(m.removeTokens, msg.parent)
			if m.running[msg.parent] == "worktree_remove" {
				delete(m.running, msg.parent)
			}
		case !inflight:
			// No registered attempt (cancelled with esc): the residual running is released and the result dropped without touching banner or toast.
			if m.running[msg.parent] == "worktree_remove" {
				delete(m.running, msg.parent)
			}
			return m.withPump(nil)
		default:
			// Different token: the attempt was superseded (a background statusMsg released running[parent] while a removal was in flight and the user relaunched, overwriting removeTokens[parent]), so running[parent] now belongs to the new attempt and must be neither released nor banner-touched.
			return m.withPump(nil)
		}
		m.lastAction[msg.parent] = actionResult{kind: "worktree_remove", cmd: msg.cmd, output: msg.output, err: msg.err}
		// The armed state is only mutated if it still points at the same worktree (or is nil), so a later arming on another worktree is not overwritten by a late result.
		targetsArmed := m.armed == nil || m.armed.matches(msg.parent, msg.wtPath)
		if msg.err == "" {
			if targetsArmed {
				m.armed = nil
			}
			m.toasts.showSuccess("worktree removed " + msg.name)
			return m.withPump(nil)
		}
		m.toasts.showError(fmt.Sprintf("worktree remove failed %s: %s", msg.name, msg.err))
		if !targetsArmed {
			return m.withPump(nil)
		}
		if msg.force {
			m.armed = nil
		} else {
			m.armed = &armedRemoval{
				wtPath: msg.wtPath, parent: msg.parent,
				name: msg.name, force: true,
			}
		}
		return m.withPump(nil)

	case execDoneMsg:
		delete(m.running, msg.path)
		// The handoff lends the terminal to the child, so there is no output to record: the log keeps the argv and how it ended, with Dur = 0 (measuring it would mean storing the start instant in the model).
		cmdlog.RecordExec(cmdlog.Entry{
			Repo:   m.nameOf(msg.path),
			Dir:    msg.path,
			Class:  cmdlog.ClassAction,
			Action: msg.action,
			Argv:   msg.argv,
			Exit:   execExit(msg.err),
		})
		cmd := m.recollectCmd(msg.path)
		if msg.err != nil {
			m.toasts.showError(fmt.Sprintf("command: %v", msg.err))
		}
		return m, cmd

	// The render answers through the events channel, so the pump is rearmed; it measures its own duration (a capture can be measured, a handoff cannot) and needs no recollect: git-sim draws, it does not mutate the repo. The overlay clears only for the repo that answered: another render can still be in flight.
	case visualDoneMsg:
		delete(m.running, msg.path)
		if m.visualBusy != nil && m.visualBusy.path == msg.path {
			m.visualBusy = nil
		}
		cmdlog.RecordExec(cmdlog.Entry{
			Repo:   m.nameOf(msg.path),
			Dir:    msg.path,
			Class:  cmdlog.ClassAction,
			Action: "visual",
			Argv:   msg.argv,
			Exit:   execExit(msg.err),
			Dur:    msg.dur,
		})
		switch {
		case msg.err != nil:
			m.toasts.showError(fmt.Sprintf("visual %s: %v", msg.sub, msg.err))
		case msg.openErr != nil:
			m.toasts.showWarning(fmt.Sprintf("visual %s: image kept at %s (%v)", msg.sub, msg.image, msg.openErr))
		default:
			m.toasts.showSuccess(fmt.Sprintf("visual %s — opened %s", msg.sub, filepath.Base(msg.image)))
		}
		return m.withPump(nil)

	case cmdResultMsg:
		delete(m.running, msg.path)
		m.lastCmd[msg.path] = cmdResult{command: msg.command, output: msg.output, exit: msg.exit}
		if msg.exit != "0" {
			m.toasts.showError(fmt.Sprintf("! %s — exit %s", msg.command, msg.exit))
		} else {
			m.toasts.showInfo(fmt.Sprintf("! %s — ok", msg.command))
		}
		return m.withPump(nil)

	case notifyMsg:
		m.toasts.show(msg.text, msg.level)
		return m, nil

		// Its own message (and not an effect of the submit key) so accepting and executing stay two steps: the submission is observable in m.prPending with no process having left.
	case prStartMsg:
		return m, m.prCreateCmd()

		// reject != "" means nothing ran, so there is no exec to record and no state to recollect: only the warning, which says what is missing.
	case prResultMsg:
		delete(m.running, msg.path)
		if msg.reject != "" {
			m.toasts.showWarning(msg.reject)
			return m.withPump(nil)
		}
		level, note := prNote(m.nameOf(msg.path), msg)
		if level == toastSuccess {
			m.toasts.showSuccess(note)
		} else {
			m.toasts.showError(note)
		}
		// gh pushes the head branch when it has no upstream, so the snapshot's ahead/behind may have changed: recollect regardless, like after a handoff.
		return m.withPump(m.recollectCmd(msg.path))

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// Rearms the event pump after consuming a channel event (each Cmd reads ONE event): without it no state ever arrives.
func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	return m, tea.Batch(cmd, waitForEvent(m.events))
}

// On failure it adds git's real reason and an actionable hint, and rebaseInProgress wins over the others: a clashing pull --rebase left half-rewritten history, and saying only "failed" invites a retry that is worse than nothing.
func actionNote(kind, name, cmd, output, errStr string, rebaseInProgress bool) string {
	// The toast-only actions keep their verdict short: the argv stays in the `l` panel, so repeating it in the toast only adds noise.
	if toastOnlyActions[kind] {
		cmd = ""
	}
	if errStr == "" {
		if cmd == "" {
			return fmt.Sprintf("%s ok %s", kind, name)
		}
		return fmt.Sprintf("%s ok %s (%s)", kind, name, cmd)
	}
	note := fmt.Sprintf("%s failed %s: %s", kind, name, errStr)
	if cmd != "" {
		note += " — " + cmd
	}
	switch {
	case rebaseInProgress && isRebaseKind(kind):
		note += " — mid-rebase: resolve the conflicts and `git rebase --continue` (or `--abort`)"
	case IsPullKind(kind):
		switch {
		case strings.Contains(output, "Not possible to fast-forward"),
			strings.Contains(output, "divergent"):
			note += " — diverged: try the selector's rebase (p then r)"
		case strings.Contains(errStr, "no tracking information"),
			strings.Contains(errStr, "no upstream"):
			note += " — no upstream: P publishes it and sets up tracking"
		}
	}
	return note
}

func (m *Model) fetchingAll() bool {
	for _, st := range m.fetchStates {
		if st == "fetching" {
			return true
		}
	}
	return false
}

func (m *Model) clampCursor() {
	n := len(m.entries())
	if m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// The PR overlay takes the whole keyboard and is consulted FIRST, before armed selectors, inputs and the table, because the user is typing and "p" or "f" are letters, not actions; the only exception is ctrl+c, which keeps its normal course and closes the app, since swallowing the terminal abort would leave the user without an exit.
	if m.pr != nil {
		if out, cmd, handled := m.handlePRKey(msg); handled {
			return out, cmd
		}
	}

	// esc cancels ALL in-flight removals, not just one: it clears the whole token map so any late result is discarded without re-arming the forced level or touching the banner.
	if key == "esc" && len(m.removeTokens) > 0 {
		clear(m.removeTokens)
	}

	// An armed removal has priority over everything (including the esc that closes the detail and the search/command inputs); any other key disarms and continues on its normal course.
	if m.armed != nil {
		switch {
		case m.actionForKey(key) == "worktree_remove":
			return m.handleWorktreeRemove()
		case key == "esc":
			m.armed = nil
			return m, nil
		default:
			m.armed = nil
		}
	}

	// The pull key only arms and the second key chooses, because the git variants (p/r/f/m) collide with real table actions; any other key cancels and continues normally, which is what keeps the app from waiting forever for a second keystroke.
	if m.pullArmed != nil {
		armed := *m.pullArmed
		m.pullArmed = nil
		if kind, ok := PullKinds[key]; ok {
			// The intent carries the chosen variant and not a bare "pull": that is what explains the argv shown a line below in the log.
			cmdlog.RecordIntent(cmdlog.Entry{
				Class:  cmdlog.ClassAction,
				Repo:   m.nameOf(armed.path),
				Dir:    armed.path,
				Key:    key,
				Action: kind,
			})
			return m, m.startActionCmd(armed.path, kind)
		}
		if key == "a" {
			cmdlog.RecordIntent(cmdlog.Entry{
				Class:  cmdlog.ClassAction,
				Repo:   m.nameOf(armed.path),
				Dir:    armed.path,
				Key:    key,
				Action: "pull_ai",
			})
			return m, m.startPullAICmd(armed.path)
		}
	}

	if m.visualArmed != nil {
		armed := *m.visualArmed
		m.visualArmed = nil
		if o, ok := visualOptionForKey(key); ok {
			if o.needsUpstream && armed.upstream == "" {
				return m, m.toastCmd(toastWarning, "no upstream")
			}
			// git-sim aborts when the ref is already in HEAD, which is exactly what behind == 0 says, so the render is refused instead of burning a couple of seconds on a simulation git-sim will reject; the value comes from the last fetch, hence naming the fetch key.
			if o.needsUpstream && armed.behind == 0 {
				return m, m.toastCmd(toastWarning, fmt.Sprintf(
					"%s already in HEAD — nothing to simulate (%s to fetch)",
					armed.upstream, m.cfg.KeyFor("fetch")))
			}
			cmdlog.RecordIntent(cmdlog.Entry{
				Class:  cmdlog.ClassAction,
				Repo:   m.nameOf(armed.path),
				Dir:    armed.path,
				Key:    key,
				Action: "visual",
			})
			return m, m.startVisualCmd(armed.path, armed.upstream, o.sub)
		}
	}

	// The panel's keys are consulted before the normal routing (like armed states) because j/k collide with table navigation; every other key continues normally, since the panel is a view mode and not a modal.
	if m.logOpen {
		if m.handleLogKey(key, m.layout().bodyLines) {
			return m, nil
		}
	}

	if m.cmdOpen {
		switch key {
		case "enter":
			cmdStr := strings.TrimSpace(m.cmdInput.Value())
			m.cmdOpen = false
			m.cmdInput.Blur()
			m.cmdInput.SetValue("")
			if r, ok := m.selected(); ok {
				if !r.project.HasRepo {
					return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
				}
				m.logIntent(key, "cmd")
				if cmdStr == "" {
					return m, m.openShellCmd(r.project.Path)
				}
				return m, m.openCmdCmd(r.project.Path, cmdStr)
			}
			return m, nil
		case "esc":
			m.cmdOpen = false
			m.cmdInput.Blur()
			return m, nil
		default:
			in, cmd := m.cmdInput.Update(msg)
			m.cmdInput = in
			return m, cmd
		}
	}

	if m.searchActive {
		switch key {
		case "enter":
			m.searchActive = false
			m.searchInput.Blur()
			m.search = strings.TrimSpace(m.searchInput.Value())
			m.clampCursor()
			return m, nil
		case "esc":
			m.searchActive = false
			m.searchInput.Blur()
			if strings.TrimSpace(m.searchInput.Value()) == "" {
				m.search = ""
			}
			m.clampCursor()
			return m, nil
		default:
			in, cmd := m.searchInput.Update(msg)
			m.searchInput = in
			m.search = strings.TrimSpace(m.searchInput.Value())
			m.clampCursor()
			return m, cmd
		}
	}

	// `a` is the panel's own filter here and no selector is armed (its warning would not be painted and the key would be shadowed), and `pr` is guarded too because opening the form behind the panel would not draw it.
	if m.logOpen {
		switch m.actionForKey(key) {
		case "pull", "visual", "pr":
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "esc":
		// Dropping the overlay does NOT cancel the render: it is view state (the truth of "in flight" is m.running), and the completion must still record in the log and toast.
		m.visualBusy = nil
		return m, nil
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
		return m, nil
	case "down", "j":
		m.cursor = min(m.cursor+1, max(0, len(m.entries())-1))
		return m, nil
	case "home":
		m.cursor = 0
		return m, nil
	case "end":
		m.cursor = max(0, len(m.entries())-1)
		return m, nil
	}

	action := m.actionForKey(key)

	// Single point of intent for the actions that launch something, while pure navigation (filter, fold, detail, the log panel itself) is not registered: this is a log of commands, not of keys.
	if launchesCommand(action) {
		m.logIntent(key, action)
	}

	switch action {
	case "dirty":
		m.onlyDirty = !m.onlyDirty
		m.clampCursor()
	case "search":
		m.searchActive = true
		m.searchInput.SetValue(m.search)
		return m, m.searchInput.Focus()
	case "fetch":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
			}
			return m, m.fetchBatchCmd([]string{r.project.Path}, cmdlog.ClassAction)
		}
	case "fetch_all":
		paths := m.fetchTargets()
		if len(paths) == 0 {
			return m, m.toastCmd(toastInfo, "no repositories with upstream to fetch")
		}
		return m, m.fetchBatchCmd(paths, cmdlog.ClassAction)
	case "pull":
		// The pull key does not execute, it arms: the "no repo" guard is resolved when arming, so no selector is left alive on a row with nothing to do.
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			m.pullArmed = &armedPull{path: r.project.Path}
			return m, nil
		}
	case "visual":
		// The visual key only arms too, capturing path and upstream from the chosen row; with no row or no repo there is nothing to preview, so it warns and does not arm.
		r, ok := m.selected()
		if !ok || !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		}
		m.visualArmed = &armedVisual{
			path:     r.project.Path,
			upstream: r.snap.Status.Upstream,
			behind:   r.snap.Status.Behind,
		}
		return m, nil
	case "pr":
		// Like `pull` and `visual` it does not execute: the key only collects the parameters and the form's submit launches gh/glab.
		return m.openPR()
	case "sync":
		// Sync executes directly (no selector): it always reconciles with the repo's sync branch, unlike `pull`, whose variant is a choice.
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			return m, m.startSyncCmd(r.project.Path)
		}
	case "push":
		if r, ok := m.selected(); ok && !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		} else if ok {
			return m, m.startActionCmd(r.project.Path, "push")
		}
	case "editor":
		if r, ok := m.selected(); ok {
			if r.project.MarkerErr != "" {
				return m, m.toastCmd(toastWarning, "marker error — fix .gitdash.toml first")
			}
			return m, m.openEditorCmd(r.project.Path)
		}
	case "lazygit":
		if r, ok := m.selected(); ok {
			if !r.project.HasRepo {
				return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
			}
			return m, m.openLazygitCmd(r.project.Path)
		}
	case "rescan":
		if m.scanning {
			return m, m.toastCmd(toastInfo, "scan already running")
		}
		return m, m.startScanCmd()
	case "recollect":
		if r, ok := m.selected(); ok {
			return m, m.recollectCmd(r.project.Path)
		}
	case "fold":
		return m.toggleFold()
	case "worktree_remove":
		return m.handleWorktreeRemove()
	case "command":
		if r, ok := m.selected(); !ok || !r.project.HasRepo {
			return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
		}
		m.cmdOpen = true
		return m, m.cmdInput.Focus()
	case "log":
		// The normal routing also closes the panel (the key belongs to the log section), which is why the panel is checked before reaching here.
		m.toggleLog()
	}
	return m, nil
}

// A worktree sub-row is a deliberate no-op (folding the parent's group from there would be a surprise), and both states persist in the same collapsed.json (worktrees under their own prefix).
func (m Model) toggleFold() (tea.Model, tea.Cmd) {
	e, ok := entryAt(m.entries(), m.cursor)
	if !ok {
		return m, nil
	}
	switch e.kind {
	case kindPrimary, kindSecondary:
		m.collapsed[e.group] = !m.collapsed[e.group]
	case kindRepo:
		if !m.expandable(e.r) {
			return m, nil
		}
		m.expanded[e.r.project.Path] = !m.expanded[e.r.project.Path]
	default:
		return m, nil
	}
	m.clampCursor()
	m.saveCollapsed()
	return m, nil
}

// Recording "I pressed enter to fold" says nothing about which commands ran, so pure view actions (filter, search, fold, the log panel, quit) leave no entry.
var commandActions = map[string]bool{
	"fetch": true, "fetch_all": true, "pull": true, "push": true,
	"sync":    true,
	"lazygit": true, "editor": true, "rescan": true, "recollect": true,
	"command": true, "worktree_remove": true, "visual": true, "pr": true,
}

func launchesCommand(action string) bool { return commandActions[action] }

// Pressing `p` on a group header is not a command anybody would want audited, so actions needing a row leave no intent without one.
var rowActions = map[string]bool{
	"fetch": true, "pull": true, "push": true, "lazygit": true,
	"sync":   true,
	"editor": true, "recollect": true, "command": true, "worktree_remove": true,
	"visual": true, "pr": true,
}

// Outcomes that belong in a toast instead of the card's last-action block: sync prints git's rebase/apply/drop lines, which the transient toast summarizes while `l` keeps the whole audit.
var toastOnlyActions = map[string]bool{
	"sync": true,
}

func actionNeedsRow(action string) bool { return rowActions[action] }

func (m Model) actionForKey(key string) string {
	for action, k := range m.cfg.Keybindings {
		if k == key {
			return action
		}
	}
	return ""
}

func (m Model) handleWorktreeRemove() (tea.Model, tea.Cmd) {
	e, ok := m.selectedEntry()
	if !ok || e.kind != kindWorktree || e.parent == "" || e.wt.Path == "" {
		m.armed = nil
		return m, m.toastCmd(toastInfo, "select a worktree")
	}
	if m.armed != nil && m.armed.matches(e.parent, e.wt.Path) {
		if cmd := m.busyActionCmd(m.armed.parent); cmd != nil {
			return m, cmd
		}
		armed := *m.armed
		m.removeGen++
		if m.removeTokens == nil {
			m.removeTokens = map[string]int{}
		}
		m.removeTokens[armed.parent] = m.removeGen
		m.armed = nil
		// The intent is the keystroke that confirms, not the one that armed it (that one was already logged by the action routing).
		cmdlog.RecordIntent(cmdlog.Entry{
			Class:  cmdlog.ClassAction,
			Repo:   filepath.Base(armed.wtPath),
			Dir:    armed.wtPath,
			Key:    m.cfg.KeyFor("worktree_remove"),
			Action: "worktree_remove",
		})
		return m, m.removeWorktreeCmd(armed.parent, armed.wtPath, armed.name, armed.force, m.removeGen)
	}
	// A worktree other than the armed one is never removed, neither on re-arming nor on a first press.
	m.armed = &armedRemoval{
		wtPath: e.wt.Path,
		parent: e.parent,
		name:   filepath.Base(e.wt.Path),
	}
	return m, nil
}

// Resolved by key (p/r/f/m/a) and not with arrows: the set is short and fixed, and a navigable list would cost two extra keystrokes for the variant used 90% of the time.
func (m Model) pullPrompt() string {
	if m.pullArmed == nil {
		return ""
	}
	variants := make([]string, 0, len(pullOptions))
	for _, o := range pullOptions {
		variants = append(variants, fmt.Sprintf("%s %s", o.key, o.label))
	}
	return fmt.Sprintf("pull %s: %s · esc cancel", m.nameOf(m.pullArmed.path), strings.Join(variants, " · "))
}

func pullVariantLabel(kind string) string {
	for _, o := range pullOptions {
		if o.kind == kind {
			return o.label
		}
	}
	return kind
}

// Variants come from visualOptions, the same source as the labels, so a new one cannot leave the prompt lying.
func (m Model) visualPrompt() string {
	if m.visualArmed == nil {
		return ""
	}
	variants := make([]string, 0, len(visualOptions))
	for _, o := range visualOptions {
		variants = append(variants, fmt.Sprintf("%s %s", o.key, o.label))
	}
	return fmt.Sprintf("visual %s: %s · esc cancel", m.nameOf(m.visualArmed.path), strings.Join(variants, " · "))
}

func (m Model) removePrompt() string {
	if m.armed == nil {
		return ""
	}
	key := m.cfg.KeyFor("worktree_remove")
	if m.armed.force {
		return fmt.Sprintf("remove worktree %s? has changes — %s to force, esc to cancel", m.armed.name, key)
	}
	return fmt.Sprintf("remove worktree %s? %s to confirm, esc to cancel", m.armed.name, key)
}

func (m Model) View() tea.View {
	// No "if there are toasts" guard: overlayToasts is already a no-op on an empty list, and duplicating the check was one more place where a ">=" could hide the difference between painting nothing and painting over the base.
	content := overlayToasts(m.renderDashboard(), m.toasts.blocksFor(m.width), m.width, m.height, m.toastReserve())
	// The render overlay goes LAST, on top of the toasts too: it is what is happening NOW, and a completion toast of another repo must not hide it.
	content = overlayCentered(content, m.visualOverlayLines(), m.width, m.height)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// With the log or the PR overlay open the body is not the table but the log or the form (layout already gave back its height): both are views you go to look at, so they replace the table instead of sharing space with it.
func (m Model) renderDashboard() string {
	lay := m.layout()
	if m.logOpen {
		return m.compose(lay, m.logSection(lay.bodyLines), "")
	}
	if m.pr != nil {
		return m.compose(lay, m.prSection(lay.bodyLines), "")
	}
	entries := m.entries()
	table := m.tableSection(lay.bodyLines, entries, lay.tableWidth)
	middle := table
	if lay.showPanel {
		middle = joinPanes(table, m.panelSection(lay, entries))
	}
	return m.compose(lay, middle, m.previewSection(lay, entries))
}

func (m Model) toastReserve() int {
	lay := m.layout()
	if !lay.showKeybinds {
		return 0
	}
	return keybindsChrome + lay.hintLines
}

func (m *Model) syncOffset(total, window int) {
	if total <= window {
		m.offset = 0
		return
	}
	// Two clamps instead of two guards: the border case "cursor exactly on the first row" would reassign the same value, so the guard is redundant.
	m.offset = min(m.offset, m.cursor)
	m.offset = max(m.offset, m.cursor-window+1)
}
