// A VIEW MODE, not a prefix-key armed state: a form lives N keystrokes, so the key that opens cannot be the one that submits, and the overlay takes the whole keyboard the way the command log panel does.
package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/forge"
)

// Not `enter`: in the body enter is a newline, and in single-line fields one would expect it to close them, so with typing as the main activity only a modified key can mean "submit".
const prSubmitKey = "ctrl+s"

const (
	// The height belongs to the layout, not to the text, so the textarea is sized with what is left over: a field that changes height while typing makes everything below it jump.
	prFixedLines = 6
	prLabelWidth = 12
	// Discounted from the value width because the input always draws its cursor at the end, inside its View but outside the width it was given: without this margin the line overflows the box and bordered cuts the last character.
	prValueSlack = 1
	// Below this the label and the value stop being distinguishable at a glance, and it is the label column width, the one sitting right next to it.
	prMinValueWidth = 12
)

// Below this the form is not drawn at all (half a box is worse than none) nor even opened; a function and not a const, because Go does not instrument constant expressions and the mutant would stay NOT COVERED forever.
func prMinBodyLines() int { return prFixedLines + 2 }

// Derived from the parts rather than a round number, so what it guarantees is that the input width is a width and not a clamp to 1 of a negative; a function for the same reason as prMinBodyLines.
func prMinWidth() int { return 2 + prLabelWidth + prValueSlack + prMinValueWidth }

// The head is not a field: it was read from the snapshot when arming and is shown dimmed, because editing it by hand would contradict the repo the row claims to come from.
type prField int

const (
	prFieldTitle prField = iota
	prFieldBase
	prFieldDraft
	prFieldBody
)

// The tab order and its wrap-around come from here, not from a list repeated in two places.
const prFieldCount = 4

func (f prField) next(delta int) prField {
	return prField((int(f) + delta + prFieldCount) % prFieldCount)
}

// A struct and not a bool because it carries the whole form: a separate flag could stay true with nothing behind it; path, name and head are captured WHEN ARMING, since the cursor may move before the submit key.
type prDraft struct {
	path  string
	name  string
	head  string
	draft bool

	title  textinput.Model
	body   textarea.Model
	baseIn textinput.Model
	focus  prField
	// It lives in the form and not in a toast on purpose: a toast expires after 3 s, which is exactly while the user is looking at the offending field.
	err string
}

// It lives in the model so execution is a separate step, testable without launching processes: the UI collects, forge/tool runs.
type prSubmission struct {
	path   string
	params forge.Params
}

func newPROverlay(r row, head, base string) *prDraft {
	title := textinput.New()
	// No prompt: the one the input brings ("> ") would duplicate the label on the left, and two labels for the same field read worse than one.
	title.Prompt = ""
	title.Placeholder = "what changes and why…"

	body := textarea.New()
	body.ShowLineNumbers = false
	body.Placeholder = "detail, context, checklist…"

	baseIn := textinput.New()
	baseIn.Prompt = ""
	baseIn.SetValue(base)
	baseIn.CursorEnd()

	return &prDraft{
		path:   r.project.Path,
		name:   r.project.Name,
		head:   head,
		title:  title,
		body:   body,
		baseIn: baseIn,
		focus:  prFieldTitle,
	}
}

// The input is the only source of truth: a parallel prefilled string could drift from the value the user sees, and what matters is the one they see.
func (d *prDraft) baseValue() string {
	return strings.TrimSpace(d.baseIn.Value())
}

// The row is resolved and captured HERE and not at submit time: the opening and the submitting key differ, and in between the user can move the cursor, fold a group or open the log panel, so the submission must come from the repo the row pointed at when it opened.
func (m Model) openPR() (tea.Model, tea.Cmd) {
	// Second net: the `pr` key is already stopped in the routing (the m.logOpen guard), but the action switch that dispatches this does not know what is open.
	if m.logOpen {
		return m, nil
	}
	r, ok := m.selected()
	if !ok {
		return m, m.toastCmd(toastInfo, "select a repository")
	}
	if !r.project.HasRepo {
		return m, m.toastCmd(toastInfo, "no git repo — nothing to do")
	}

	base := r.snap.SyncBranch
	if base == "" {
		// Without a snapshot there is nobody to ask for the effective ref (a worktree sub-row has none) nor where to resolve it, and the fallback bool is left unused on purpose because resolving it here would be a `rev-list` on the UI goroutine.
		base, _ = m.syncOf(r.project.Path)
	}
	head := r.snap.Status.Branch
	if head == "" {
		if wt, ok := m.worktreeFor(r.project.Path); ok {
			head = worktreeBranchLabel(wt)
		}
	}

	m.pr = newPROverlay(r, head, base)
	// The layout owns the height, so the question is whether the slot reaches the form's minimum; if it does not fit it does not open, because entering an overlay that is not drawn means writing blind.
	if !m.prFits() {
		m.pr = nil
		return m, m.toastCmd(toastInfo, "terminal too small for the PR form")
	}
	// Opening the overlay drops the armed selectors: their warning lives in the keybinds section, which now paints the form's legend, and the second key they wait for is never coming.
	m.armed = nil
	m.pullArmed = nil
	m.visualArmed = nil
	m.branchArmed = nil
	m.picker = nil
	m.prFit()
	return m, m.prFocus(prFieldTitle)
}

// A narrow but tall terminal is the case the height alone does not see, and the width minimum is what keeps the input width a width instead of a clamp to 1.
func (m Model) prFits() bool {
	return m.pr != nil && m.layout().bodyLines >= prMinBodyLines() && m.width >= prMinWidth()
}

// With prMinWidth as floor (what gates opening) it never goes negative, so max(1, …) is only a net in case width or height change by another path.
func (m Model) prValueWidth() int {
	return max(1, m.width-2-prLabelWidth-prValueSlack)
}

// Called on open and on every resize, never in the render: the layout owns the height, not what the text measures today (like the repo card, which is padded to its height instead of shrinking the box).
func (m *Model) prFit() {
	if m.pr == nil {
		return
	}
	rows := m.layout().bodyLines
	inner := max(1, m.width-2)
	value := m.prValueWidth()
	m.pr.title.SetWidth(value)
	m.pr.baseIn.SetWidth(value)
	m.pr.body.SetWidth(inner)
	m.pr.body.SetHeight(max(1, rows-prFixedLines))
}

// It releases the widgets' focus (so no cursor blinks in a field nobody writes any more) and the whole struct, so reopening inherits neither the text nor the draft of the previous attempt.
func (m *Model) closePR() {
	if m.pr != nil {
		m.pr.title.Blur()
		m.pr.baseIn.Blur()
		m.pr.body.Blur()
	}
	m.pr = nil
}

func (m *Model) prFocus(f prField) tea.Cmd {
	switch m.pr.focus {
	case prFieldTitle:
		m.pr.title.Blur()
	case prFieldBase:
		m.pr.baseIn.Blur()
	case prFieldBody:
		m.pr.body.Blur()
	}
	m.pr.focus = f
	switch f {
	case prFieldTitle:
		return m.pr.title.Focus()
	case prFieldBase:
		return m.pr.baseIn.Focus()
	case prFieldBody:
		return m.pr.body.Focus()
	}
	return nil
}

// The rest is NOT forwarded to the normal routing but sent to the focused field's widget, which is why typing "p" in the title does not arm the pull selector.
func (m Model) handlePRKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// Keystroke() and not String(): inside a form the same letter with and without modifier must be two different keys, and String() returns the terminal's raw Text when there is one, which would collapse "ctrl+s" into "s".
	key := msg.Keystroke()
	switch key {
	case "ctrl+c":
		return m, nil, false
	case "esc":
		m.closePR()
		return m, nil, true
	case "tab":
		return m, m.prFocus(m.pr.focus.next(1)), true
	case "shift+tab":
		return m, m.prFocus(m.pr.focus.next(-1)), true
	case prSubmitKey:
		return m, m.prSubmit(), true
	}

	switch m.pr.focus {
	case prFieldDraft:
		// The only field with no widget behind it, so space (and enter, which is what the rest would do) toggle it, and any other key is ignored instead of falling through to the table.
		if key == "space" || key == "enter" {
			m.pr.draft = !m.pr.draft
		}
		return m, nil, true
	case prFieldBody:
		ta, cmd := m.pr.body.Update(msg)
		m.pr.body = ta
		m.pr.err = ""
		return m, cmd, true
	case prFieldBase:
		in, cmd := m.pr.baseIn.Update(msg)
		m.pr.baseIn = in
		m.pr.err = ""
		return m, cmd, true
	default:
		in, cmd := m.pr.title.Update(msg)
		m.pr.title = in
		m.pr.err = ""
		return m, cmd, true
	}
}

// Publishing the submission in m.prPending is what lets that step exist: the UI goes back to the dashboard with the parameters collected, and prCreateCmd runs them and warns.
func (m *Model) prSubmit() tea.Cmd {
	p := m.prParams()
	if p.Title == "" {
		m.pr.err = "title required"
		return nil
	}
	if p.Base == "" {
		m.pr.err = "base branch required"
		return nil
	}
	m.prPending = &prSubmission{path: m.pr.path, params: p}
	m.closePR()
	return func() tea.Msg { return prStartMsg{} }
}

// Title and base are trimmed (single-line flags, a trailing space can only be a slip) but the body is left alone: it is free text and what the user wrote (newlines, quotes, shell signs included) has to reach the argv verbatim, with no quotes or escapes invented here.
func (m Model) prParams() forge.Params {
	if m.pr == nil {
		return forge.Params{}
	}
	return forge.Params{
		Title: strings.TrimSpace(m.pr.title.Value()),
		Body:  m.pr.body.Value(),
		Base:  m.pr.baseValue(),
		Head:  m.pr.head,
		Draft: m.pr.draft,
	}
}

func (m Model) prSection(rows int) string {
	if m.pr == nil || rows < prMinBodyLines() {
		return ""
	}
	// Fields are painted with their widget's View() and not with Value(): View brings the cursor, the placeholder and the horizontal scroll when the text does not fit.
	title := m.prFieldLine("title", m.pr.focus == prFieldTitle, m.pr.title.View())
	base := m.prFieldLine("base", m.pr.focus == prFieldBase, m.pr.baseIn.View())
	head := m.prFieldLine("head", false, styleDim.Render(m.pr.head))
	draft := m.prFieldLine("draft", m.pr.focus == prFieldDraft, m.pr.draftLabel())
	// The warning goes BETWEEN the fields and the body, not below: 20 lines under the title, "the title is missing" is a message you search for instead of one you see.
	notice := m.prNoticeLine()

	lines := []string{title, base, head, draft, notice, m.prBodyLabel(), m.pr.body.View()}
	return m.section("new PR · "+m.pr.name, fitLines(strings.Join(lines, "\n"), rows), m.width)
}

// Reserved ALWAYS, even without an error, so the textarea does not change height when a warning appears: a field jumping under the cursor while typing is worse than an error you have to read twice.
func (m Model) prNoticeLine() string {
	if m.pr.err == "" {
		return ""
	}
	return "  " + styleError.Render(m.pr.err)
}

// The body shares the label column (focus marker included) but has no value: it is a multiline block and what marks where it starts is its own ┃ prompt.
func (m Model) prBodyLabel() string {
	return strings.TrimRight(m.prFieldLine("body", m.pr.focus == prFieldBody, ""), " ")
}

// The marker is the same ▸ that flags the cursor row in the table (the dashboard already has a language for "this is what you are touching"), and it sits in the column because the input fills its View with spaces up to the width, so a closing bracket would hit the box border.
func (m Model) prLabel(focused bool) string {
	mark := " "
	if focused {
		mark = styleCursor.Render("▸")
	}
	return "  " + mark + " "
}

// The label is padded BEFORE styling (ANSI breaks the width computation) and the focused one wears the warning color, so the input cursor and the label say the same thing from two places.
func (m Model) prFieldLine(label string, focused bool, value string) string {
	key := styleDetailKey
	if focused {
		key = styleWarn.Bold(true)
	}
	return m.prLabel(focused) + key.Render(pad(label, 7)) + " " + value
}

func (d *prDraft) draftLabel() string {
	if d.draft {
		return "on"
	}
	return "off"
}

// The opening key comes from the config (KeyFor) and not from a package constant: after a rebind of `pr` a hand-written warning would leave the user reading a key that no longer opens anything.
func (m Model) prPrompt() string {
	if m.pr == nil {
		return ""
	}
	return fmt.Sprintf("new PR %s: %s → %s · %s opens · tab field · %s create · %s back",
		m.pr.name, m.pr.head, m.pr.baseValue(), m.cfg.KeyFor("pr"), prSubmitKey, "esc")
}
