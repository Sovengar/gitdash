package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
)

// Variant modes of the `b` selector: `c` checks the branch out, `s` writes it as the repo's sync ref.
const (
	pickerModeCurrent = "current"
	pickerModeSync    = "sync"
)

// A view mode, not an armed state: the list lives N keystrokes and has to be navigated, so the
// variant key opens it and `enter`/`esc` close it.
type branchPicker struct {
	path     string
	mode     string
	variant  string // the key that chose the mode (c/s), kept for the intent
	branches []string
	current  string
	loading  bool
	err      string
	cursor   int
}

// Painted by promptLine in the keybinds section while `b` waits for its second key.
func (m Model) branchArmedPrompt() string {
	if m.branchArmed == nil {
		return ""
	}
	variants := make([]string, 0, len(branchOptions))
	for _, o := range branchOptions {
		variants = append(variants, fmt.Sprintf("%s %s", o.key, o.label))
	}
	return fmt.Sprintf("branch %s: %s · esc cancel", m.nameOf(m.branchArmed.path), strings.Join(variants, " · "))
}

// Painted by promptLine in the keybinds section, like the armed states: it says what the keys do while the picker lives.
func (m Model) branchPrompt() string {
	if m.picker == nil {
		return ""
	}
	target := "checkout"
	if m.picker.mode == pickerModeSync {
		target = "set sync ref"
	}
	return fmt.Sprintf("branch %s: j/k move · enter %s · esc cancel", m.nameOf(m.picker.path), target)
}

// Returns handled=false only for the global quit keys: any other unbound key is swallowed so a
// keystroke cannot fire an underlying table action behind the picker.
func (m Model) handlePickerKey(key string) (tea.Model, tea.Cmd, bool) {
	if key == "ctrl+c" || key == "q" {
		return m, nil, false
	}
	switch key {
	case "esc":
		m.picker = nil
		return m, nil, true
	case "up", "k":
		if m.picker.cursor > 0 {
			m.picker.cursor--
		}
		return m, nil, true
	case "down", "j":
		if m.picker.cursor < len(m.picker.branches)-1 {
			m.picker.cursor++
		}
		return m, nil, true
	case "enter":
		out, cmd := m.selectPickerBranch()
		return out, cmd, true
	}
	return m, nil, true
}

func (m Model) selectPickerBranch() (tea.Model, tea.Cmd) {
	p := m.picker
	if p == nil || p.loading || p.err != "" || len(p.branches) == 0 {
		return m, nil
	}
	branch := p.branches[p.cursor]

	if p.mode == pickerModeCurrent {
		// The intent is the confirming key, not the one that armed the selector (same rule as the worktree removal).
		m.recordPickerIntent(p, "checkout")
		if branch == p.current {
			m.picker = nil
			return m, m.toastCmd(toastInfo, fmt.Sprintf("already on %s", branch))
		}
		m.picker = nil
		return m, m.startActionArgs(p.path, "checkout", [][]string{{"checkout", branch}})
	}

	m.recordPickerIntent(p, "branch_sync")
	err := discovery.SetMarkerSyncBranch(p.path, m.cfg.Marker, branch)
	m.picker = nil
	if err != nil {
		return m, m.toastCmd(toastError, fmt.Sprintf("marker write failed: %v", err))
	}
	m.setSyncBranch(p.path, branch)
	m.recollectCmd(p.path)
	return m, m.toastCmd(toastSuccess, fmt.Sprintf("sync branch %s → %s", m.nameOf(p.path), branch))
}

func (m *Model) recordPickerIntent(p *branchPicker, action string) {
	cmdlog.RecordIntent(cmdlog.Entry{
		Class:  cmdlog.ClassAction,
		Repo:   m.nameOf(p.path),
		Dir:    p.path,
		Key:    m.cfg.KeyFor("branch") + p.variant,
		Action: action,
	})
}

// The recollect resolves the ref from the project list, so the in-memory entry has to carry the just-written branch or the UI would keep comparing against the old one.
func (m *Model) setSyncBranch(path, branch string) {
	for i := range m.projects {
		if m.projects[i].Path == path {
			m.projects[i].SyncBranch = branch
		}
	}
}

func (m Model) branchSection(bodyLines int) string {
	p := m.picker
	if p == nil {
		return ""
	}
	visible := max(1, bodyLines-1)
	title := "branches · " + m.nameOf(p.path)

	if p.err != "" {
		return m.section(title, fitLines("  "+styleError.Render("branch list failed: "+p.err), visible), m.width)
	}
	if p.loading {
		return m.section(title, fitLines("  "+styleHint.Render("loading branches…"), visible), m.width)
	}
	if len(p.branches) == 0 {
		return m.section(title, fitLines("  "+styleHint.Render("no local branches"), visible), m.width)
	}

	inner := max(1, m.width-2)
	labelWidth := max(1, inner-6)
	offset := pickerWindow(len(p.branches), p.cursor, visible)
	rows := make([]string, 0, visible)
	for i, b := range p.branches[offset:min(offset+visible, len(p.branches))] {
		mark := " "
		if offset+i == p.cursor {
			mark = styleCursor.Render("▸")
		}
		// The style goes on AFTER truncating: ANSI makes the width computation wrong.
		text := b
		if b == p.current {
			text = b + " (current)"
		}
		text = truncate(text, labelWidth)
		if b == p.current {
			text = styleDim.Render(text)
		}
		rows = append(rows, "  "+mark+" "+text)
	}
	rows = rellenaHasta(rows, visible)
	return m.section(title, styleHint.Render("  # branch")+"\n"+strings.Join(rows, "\n"), m.width)
}

// Keeps the cursor in the window without storing an offset: the window only has to follow navigation.
func pickerWindow(n, cursor, visible int) int {
	if n <= visible {
		return 0
	}
	return min(max(0, cursor-visible+1), n-visible)
}
