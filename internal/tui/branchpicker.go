package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
)

// Variant modes of the `b` selector: `c` checks the branch out, `s` writes it as the repo's sync ref.
const (
	pickerModeCurrent = "current"
	pickerModeSync    = "sync"
)

// The overlay box: width clamps, and the chrome is borders + filter line + hint line.
const (
	pickerBoxMaxWidth = 64
	pickerBoxMinWidth = 24
	pickerBoxChrome   = 4
	pickerMaxRows     = 12
)

// A modal OVERLAY, not a view mode: the table and stats stay visible behind it and the box floats
// centered. The variant key opens it and `enter`/`esc` close it.
type branchPicker struct {
	path     string
	mode     string
	variant  string
	branches []gitstatus.Branch
	current  string
	loading  bool
	err      string
	cursor   int
	// filter narrows the list as you type; j/k only stay navigation while it is empty, since once
	// there is text every letter belongs to the input.
	filter textinput.Model
}

// Kept for the compact arm line the `x` selector paints in keybinds.
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

// The modal is not a terminal handoff: keys are read here, the text goes to the filter, and only
// the global abort falls through so the terminal is never trapped.
func (m Model) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, nil, false
	}
	list := m.picker.filtered()
	switch key {
	case "esc":
		m.picker = nil
		return m, nil, true
	case "up":
		if m.picker.cursor > 0 {
			m.picker.cursor--
		}
		return m, nil, true
	case "down":
		if m.picker.cursor < len(list)-1 {
			m.picker.cursor++
		}
		return m, nil, true
	case "enter":
		out, cmd := m.selectPickerBranch()
		return out, cmd, true
	}
	if (key == "j" || key == "k") && m.picker.filter.Value() == "" {
		if key == "j" && m.picker.cursor < len(list)-1 {
			m.picker.cursor++
		}
		if key == "k" && m.picker.cursor > 0 {
			m.picker.cursor--
		}
		return m, nil, true
	}
	in, cmd := m.picker.filter.Update(msg)
	m.picker.filter = in
	m.picker.cursor = 0
	return m, cmd, true
}

func (m Model) selectPickerBranch() (tea.Model, tea.Cmd) {
	p := m.picker
	if p == nil || p.loading || p.err != "" {
		return m, nil
	}
	list := p.filtered()
	if len(list) == 0 {
		return m, nil
	}
	b := list[p.cursor]
	target := checkoutTarget(b)

	if p.mode == pickerModeCurrent {
		// The intent is the confirming key, not the one that armed the selector (same rule as the worktree removal).
		m.recordPickerIntent(p, "checkout")
		if target == p.current {
			m.picker = nil
			return m, m.toastCmd(toastInfo, fmt.Sprintf("already on %s", target))
		}
		m.picker = nil
		return m, m.startActionArgs(p.path, "checkout", [][]string{{"checkout", target}})
	}

	m.recordPickerIntent(p, "branch_sync")
	err := discovery.SetMarkerSyncBranch(p.path, m.cfg.Marker, target)
	m.picker = nil
	if err != nil {
		return m, m.toastCmd(toastError, fmt.Sprintf("marker write failed: %v", err))
	}
	m.setSyncBranch(p.path, target)
	m.recollectCmd(p.path)
	return m, m.toastCmd(toastSuccess, fmt.Sprintf("sync branch %s → %s", m.nameOf(p.path), target))
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

// A remote ref is checked out through its local name: `git checkout origin/foo` would land detached,
// while the bare name lets git create or use the tracking local branch (DWIM).
func checkoutTarget(b gitstatus.Branch) string {
	if b.Remote {
		return strings.TrimPrefix(b.Name, "origin/")
	}
	return b.Name
}

func (p *branchPicker) filtered() []gitstatus.Branch {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	if q == "" {
		return p.branches
	}
	out := make([]gitstatus.Branch, 0, len(p.branches))
	for _, b := range p.branches {
		if strings.Contains(strings.ToLower(b.Name), q) {
			out = append(out, b)
		}
	}
	return out
}

func pickerBoxWidth(width int) int {
	return min(pickerBoxMaxWidth, max(pickerBoxMinWidth, width-6))
}

func pickerInputWidth(width int) int {
	return max(8, pickerBoxWidth(width)-4)
}

// The busy/error/empty box is one line; the list box is the filter line, the rows and the hint line.
func (m Model) pickerOverlay(width, height int) []string {
	p := m.picker
	if p == nil {
		return nil
	}
	boxWidth := pickerBoxWidth(width)
	title := "branches · " + m.nameOf(p.path)
	rowsAvail := min(pickerMaxRows, max(1, height-pickerBoxChrome))

	var content []string
	switch {
	case p.err != "":
		content = []string{styleError.Render("  branch list failed: " + p.err)}
	case p.loading:
		content = []string{styleHint.Render("  loading branches…")}
	default:
		list := p.filtered()
		content = append(content, "  "+p.filter.View())
		if len(list) == 0 {
			content = append(content, styleHint.Render("  no matching branches"))
		} else {
			content = append(content, m.pickerRows(list, rowsAvail)...)
		}
	}
	content = append(content, styleHint.Render("  ↑/↓ move · type filter · enter select · esc cancel"))

	box := m.section(title, strings.Join(content, "\n"), boxWidth)
	return strings.Split(box, "\n")
}

func (m Model) pickerRows(list []gitstatus.Branch, rows int) []string {
	width := max(1, pickerBoxWidth(m.width)-6)
	offset := pickerWindow(len(list), m.picker.cursor, rows)
	out := make([]string, 0, rows)
	for i, b := range list[offset:min(offset+rows, len(list))] {
		mark := " "
		if offset+i == m.picker.cursor {
			mark = styleCursor.Render("▸")
		}
		// The style goes on AFTER truncating: ANSI makes the width computation wrong.
		text := truncate(branchLabel(b, m.picker.current), width)
		if b.Name == m.picker.current {
			text = styleDim.Render(text)
		}
		out = append(out, "  "+mark+" "+text)
	}
	return rellenaHasta(out, rows)
}

func branchLabel(b gitstatus.Branch, current string) string {
	switch {
	case b.Name == current:
		return b.Name + " (current)"
	case b.Remote:
		return b.Name + " (remote)"
	case !b.HasUpstream:
		return b.Name + " (no upstream)"
	default:
		return b.Name
	}
}

// Keeps the cursor in the window without storing an offset: the window only has to follow navigation.
func pickerWindow(n, cursor, visible int) int {
	if n <= visible {
		return 0
	}
	return min(max(0, cursor-visible+1), n-visible)
}

// ANSI-safe centered splice (same technique as overlayToasts): the block is drawn centered over the
// base, which stays visible around it.
func overlayCentered(base string, block []string, width, height int) string {
	if len(block) == 0 || width <= 0 {
		return base
	}
	lines := strings.Split(base, "\n")
	if height <= 0 || height > len(lines) {
		height = len(lines)
	}
	block = clampBlock(block, width)
	bw := blockWidth(block)
	x := max(0, (width-bw)/2)
	y := max(0, (height-len(block))/2)
	for j, b := range block[:min(len(block), max(0, len(lines)-y))] {
		row := y + j
		lines[row] = ansi.Truncate(lines[row], x, "") + b + ansi.TruncateLeft(lines[row], x+bw, "")
	}
	return strings.Join(lines, "\n")
}
