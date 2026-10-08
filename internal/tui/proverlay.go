// A VIEW MODE, not a prefix-key armed state: a form lives N keystrokes, so the key that opens cannot be the one that submits, and the overlay takes the whole keyboard the way the command log panel does.
package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gitdash/internal/forge"
	"gitdash/internal/gitstatus"
)

// Not `enter`: in the body enter is a newline, and in single-line fields one would expect it to close them, so with typing as the main activity only a modified key can mean "submit".
const prSubmitKey = "ctrl+s"

// The template key is NOT rebindable: it is a character inserted into the body, not an action of the dashboard, and the config's keymap does not reach inside the modal.
const prTemplateKey = "ctrl+t"

const (
	prLabelWidth = 12
	// Discounted from the value width because the input always draws its cursor at the end, inside its View but outside the width it was given: without this margin the line overflows the box and bordered cuts the last character.
	prValueSlack = 1
	// Below this the label and the value stop being distinguishable at a glance, and it is the label column width, the one sitting right next to it.
	prMinValueWidth = 12
	// The margin the modal leaves on each side of the terminal.
	prModalMargin = 4
	// The body's skeleton: a non-destructive insert at the cursor, not a template that replaces what is written.
	prBodySkeleton = "## Summary\n\n## Test plan\n\n"
)

// The modal's minimums are derived functions from their parts (Go does not instrument constant expressions, so a `const` would leave its arithmetic mutant NOT COVERED): fixed content lines, the always-reserved picker pane and the body floor.
func prFixedLines() int   { return 6 } // title, base, head, draft, notice, body label
func prPaneLines() int    { return 5 } // the filter line plus the picker window, reserved so tabbing never resizes the modal
func prBodyMinLines() int { return 2 }

func prPickerWindow() int { return prPaneLines() - 1 }
func prMinInterior() int  { return prFixedLines() + prPaneLines() + prBodyMinLines() }
func prMinModal() int     { return prMinInterior() + 2 }

// The body is comfortable in a content-sized modal: the preferred height is what makes the textarea
// taller than a couple of rows, and the terminal only gates the minimum (it does not stretch the box).
func prBodyPreferredLines() int { return 6 }
func prInteriorPreferred() int  { return prFixedLines() + prPaneLines() + prBodyPreferredLines() }

// Derived from the parts rather than a round number, so what it guarantees is that the input width is a width and not a clamp to 1 of a negative.
func prMinWidth() int { return 2 + prLabelWidth + prValueSlack + prMinValueWidth }

type prField int

const (
	prFieldTitle prField = iota
	prFieldBase
	prFieldHead
	prFieldDraft
	prFieldBody
)

// The tab order and its wrap-around come from here, not from a list repeated in two places.
const prFieldCount = 5

func (f prField) next(delta int) prField {
	return prField((int(f) + delta + prFieldCount) % prFieldCount)
}

// The committed value (what prParams reads and the field line paints) and, beside it, the transient
// filter and highlight that only live while the field is focused: the field is never edited
// character by character, the filter is.
type prPicker struct {
	value  string
	filter textinput.Model
	cursor int
	scroll int
}

// A struct and not a bool because it carries the whole form: a separate flag could stay true with nothing behind it; path and name are captured WHEN ARMING, since the cursor may move before the submit key.
type prDraft struct {
	path  string
	name  string
	draft bool

	title textinput.Model
	body  textarea.Model
	base  prPicker
	head  prPicker
	focus prField
	// It lives in the form and not in a toast on purpose: a toast expires after 3 s, which is exactly while the user is looking at the offending field.
	err string

	// One read per form open serves both pickers: refsLoading is the state before it returns, refsErr the failure, refsReady the success.
	refs      []gitstatus.Ref
	refsReady bool
	refsErr   string
}

// It lives in the model so execution is a separate step, testable without launching processes: the UI collects, forge/tool runs.
type prSubmission struct {
	path   string
	params forge.Params
}

// The result of the bounded on-demand read: it carries the path so a late answer is attributed (and dropped) when the modal has closed or moved on.
type prRefsMsg struct {
	path string
	refs []gitstatus.Ref
	err  string
}

func newPicker(value string) prPicker {
	in := textinput.New()
	in.Prompt = ""
	return prPicker{value: value, filter: in}
}

func newPROverlay(r row, head, base string) *prDraft {
	title := textinput.New()
	// No prompt: the one the input brings ("> ") would duplicate the label on the left, and two labels for the same field read worse than one.
	title.Prompt = ""
	title.Placeholder = "what changes and why…"

	body := textarea.New()
	body.ShowLineNumbers = false
	body.Placeholder = "detail, context, checklist…"

	return &prDraft{
		path:  r.project.Path,
		name:  r.project.Name,
		title: title,
		body:  body,
		base:  newPicker(base),
		head:  newPicker(head),
		focus: prFieldTitle,
	}
}

// The read is launched when the form opens, not folded into the scan: one `for-each-ref` per form open, only for that repo, and a fresh one on every open. Like recollectCmd it starts the goroutine as a side effect (the result arrives as an event), so no Cmd has to be executed for the read to run.
func (m *Model) startPRRefs(path string) {
	appCtx := m.ctx
	events := m.events
	go func() {
		refs, err := gitstatus.BranchRefs(appCtx, path)
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		sendEvent(appCtx, events, prRefsMsg{path: path, refs: refs, err: errStr})
	}()
}

func (d *prDraft) picker(f prField) *prPicker {
	switch f {
	case prFieldBase:
		return &d.base
	case prFieldHead:
		return &d.head
	default:
		return nil
	}
}

// The list the picker offers for a field: base sees locals and remotes, head locals only.
func (d *prDraft) filteredRefs(f prField) []gitstatus.Ref {
	p := d.picker(f)
	if p == nil {
		return nil
	}
	return filterRefs(d.refs, p.filter.Value(), f == prFieldHead)
}

func (d *prDraft) fieldValue(f prField) string {
	if p := d.picker(f); p != nil {
		return p.value
	}
	return ""
}

// The input is the only source of truth: a parallel prefilled string could drift from the value the user sees, and what matters is the one they see.
func (d *prDraft) baseValue() string { return strings.TrimSpace(d.base.value) }
func (d *prDraft) headValue() string { return strings.TrimSpace(d.head.value) }

func (d *prDraft) draftLabel() string {
	if d.draft {
		return "on"
	}
	return "off"
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
	// The modal floats over the dashboard and owns its own minimum; if the terminal cannot hold it, it does not open, because entering an overlay that is not drawn means writing blind.
	if !m.prFits() {
		m.pr = nil
		return m, m.toastCmd(toastInfo, "terminal too small for the PR form")
	}
	// Opening the overlay drops the armed selectors: their warning lives in the keybinds section, which now paints the form's legend, and the second key they wait for is never coming.
	m.armed = nil
	m.pullArmed = nil
	m.visualArmed = nil
	m.prFit()
	m.startPRRefs(m.pr.path)
	return m, m.prFocus(prFieldTitle)
}

// A narrow or short terminal is what the modal's derived minima gate: below them the box cannot paint its content and opening would mean writing blind.
func (m Model) prFits() bool {
	return m.pr != nil && m.height >= prMinModal() && m.width >= prMinWidth()
}

// The modal is clamped to the terminal: it takes the width minus its margin, never narrower than the minimum the fields need, never wider than the terminal.
func (m Model) prModalWidth() int {
	return min(max(m.width-prModalMargin, prMinWidth()), m.width)
}

// With prMinWidth as floor (what gates opening) it never goes negative, so max(1, …) is only a net in case width or height change by another path.
func (m Model) prValueWidth() int {
	return max(1, m.prModalWidth()-2-prLabelWidth-prValueSlack)
}

// The interior is content-sized (preferred) and only shrinks when the terminal cannot hold it, never below its derived minimum; the body takes what the fixed lines and the pane leave.
func (m Model) prInterior() int {
	return min(prInteriorPreferred(), max(prMinInterior(), m.height-2))
}

func (m Model) prBodyHeight() int {
	return max(1, m.prInterior()-prFixedLines()-prPaneLines())
}

// Called on open and on every resize, never in the render: the width and the pane height come from the modal, not from what the text measures today.
func (m *Model) prFit() {
	if m.pr == nil {
		return
	}
	value := m.prValueWidth()
	paneWidth := max(1, m.prModalWidth()-2-4)
	m.pr.title.SetWidth(value)
	m.pr.base.filter.SetWidth(paneWidth)
	m.pr.head.filter.SetWidth(paneWidth)
	m.pr.body.SetWidth(max(1, m.prModalWidth()-2))
	m.pr.body.SetHeight(m.prBodyHeight())
}

// It releases the widgets' focus (so no cursor blinks in a field nobody writes any more) and the whole struct, so reopening inherits neither the text nor the draft of the previous attempt.
func (m *Model) closePR() {
	if m.pr != nil {
		m.pr.title.Blur()
		m.pr.base.filter.Blur()
		m.pr.head.filter.Blur()
		m.pr.body.Blur()
	}
	m.pr = nil
}

// Leaving a branch field discards its transient filter, so re-entering starts from the committed value and the full list.
func (m *Model) prFocus(f prField) tea.Cmd {
	m.blurFocused()
	m.pr.focus = f
	switch f {
	case prFieldTitle:
		return m.pr.title.Focus()
	case prFieldBase:
		return m.focusPicker(prFieldBase)
	case prFieldHead:
		return m.focusPicker(prFieldHead)
	case prFieldBody:
		return m.pr.body.Focus()
	}
	return nil
}

func (m *Model) blurFocused() {
	switch m.pr.focus {
	case prFieldTitle:
		m.pr.title.Blur()
	case prFieldBase:
		m.pr.base.filter.Blur()
		m.clearPickerFilter(prFieldBase)
	case prFieldHead:
		m.pr.head.filter.Blur()
		m.clearPickerFilter(prFieldHead)
	case prFieldBody:
		m.pr.body.Blur()
	}
}

// Leaving a branch field discards its transient filter and re-highlights the committed value, so the pane returns to the full list.
func (m *Model) clearPickerFilter(f prField) {
	p := m.pr.picker(f)
	p.filter.SetValue("")
	p.filter.CursorEnd()
	p.cursor = indexOfRef(m.pr.filteredRefs(f), p.value)
	p.scroll = 0
}

// The filter starts empty on focus and the pane opens showing all refs with the committed value highlighted.
func (m *Model) focusPicker(f prField) tea.Cmd {
	p := m.pr.picker(f)
	p.filter.SetValue("")
	p.filter.CursorEnd()
	p.scroll = 0
	p.cursor = indexOfRef(m.pr.filteredRefs(f), p.value)
	return p.filter.Focus()
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
	case prTemplateKey:
		if m.pr.focus == prFieldBody {
			m.pr.body.InsertString(prBodySkeleton)
			m.pr.err = ""
			return m, nil, true
		}
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
		return m, m.prPickerKey(prFieldBase, msg, key), true
	case prFieldHead:
		return m, m.prPickerKey(prFieldHead, msg, key), true
	default:
		in, cmd := m.pr.title.Update(msg)
		m.pr.title = in
		m.pr.err = ""
		return m, cmd, true
	}
}

// Runes and backspace edit the FILTER only (the committed value is never edited character by character); up/down move the highlight; enter commits the highlighted ref, or the typed text when there is no match. The pane keys are consumed before the widget so arrows never move the form's fields.
func (m *Model) prPickerKey(f prField, msg tea.KeyPressMsg, key string) tea.Cmd {
	p := m.pr.picker(f)
	refs := m.pr.filteredRefs(f)
	switch key {
	case "up":
		p.cursor = max(0, p.cursor-1)
	case "down":
		p.cursor = min(max(0, len(refs)-1), p.cursor+1)
	case "enter":
		switch {
		case len(refs) > 0:
			p.value = refs[clampHighlight(len(refs), p.cursor)].Name
		case strings.TrimSpace(p.filter.Value()) != "":
			// The escape hatch for refs outside the list: a detached sha or a fork's owner:branch.
			p.value = strings.TrimSpace(p.filter.Value())
		}
		p.filter.SetValue("")
		p.filter.CursorEnd()
		p.cursor = indexOfRef(m.pr.filteredRefs(f), p.value)
		p.scroll = 0
	default:
		in, cmd := p.filter.Update(msg)
		p.filter = in
		p.cursor = 0
		p.scroll = 0
		return cmd
	}
	return nil
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

// Title, base and head are trimmed (single-line flags, a trailing space can only be a slip) but the body is left alone: it is free text and what the user wrote (newlines, quotes, shell signs included) has to reach the argv verbatim, with no quotes or escapes invented here.
func (m Model) prParams() forge.Params {
	if m.pr == nil {
		return forge.Params{}
	}
	return forge.Params{
		Title: strings.TrimSpace(m.pr.title.Value()),
		Body:  m.pr.body.Value(),
		Base:  m.pr.baseValue(),
		Head:  m.pr.headValue(),
		Draft: m.pr.draft,
	}
}

// The modal is content-sized and centred by the splice; its lines are exactly prModalWidth wide (bordered pads them), so splicing it preserves the dashboard's width.
func (m Model) prModal() string {
	if m.pr == nil {
		return ""
	}
	title := m.prFieldLine("title", m.pr.focus == prFieldTitle, m.pr.title.View())
	base := m.prFieldLine("base", m.pr.focus == prFieldBase, m.prFieldDisplay(prFieldBase))
	head := m.prFieldLine("head", m.pr.focus == prFieldHead, m.prFieldDisplay(prFieldHead))
	draft := m.prFieldLine("draft", m.pr.focus == prFieldDraft, m.pr.draftLabel())
	// The warning goes BETWEEN the fields and the body, not below: 20 lines under the title, "the title is missing" is a message you search for instead of one you see.
	blocks := []string{title, base, head, draft, m.prPane(), m.prNoticeLine(), m.prBodyLabel()}
	content := strings.Join(blocks, "\n") + "\n" + m.pr.body.View()
	return m.section("new PR · "+m.pr.name, fitLines(content, m.prInterior()), m.prModalWidth())
}

// The committed value is painted in the field line and clipped to the value column: a ref typed by
// hand can be longer than the box, and an untrusted value is sanitised like every other text.
func (m Model) prFieldDisplay(f prField) string {
	v := m.pr.fieldValue(f)
	if v == "" {
		return styleDim.Render("—")
	}
	return truncate(sanitizeLogText(v), m.prValueWidth())
}

// The pane is always the same number of lines (reserved), and only painted with content while a branch field is focused: that is what keeps the modal from resizing per keystroke.
func (m Model) prPane() string {
	p := m.pr.picker(m.pr.focus)
	if p == nil {
		return strings.Join(blankLines(prPaneLines()), "\n")
	}
	lines := []string{m.filterLine(p)}
	switch {
	case !m.pr.refsReady && m.pr.refsErr == "":
		lines = append(lines, styleDim.Render("  loading branches…"))
	case m.pr.refsErr != "":
		lines = append(lines, styleError.Render("  could not list branches"))
	default:
		lines = append(lines, m.pickerRows(p)...)
	}
	return strings.Join(fitList(lines, prPaneLines()), "\n")
}

// The filter is painted from its widget's View, sized so it never runs past the box: the pane's rows are the window of matching refs, with a "… N more" tail when the list does not fit whole.
func (m Model) pickerRows(p *prPicker) []string {
	rows := prPickerWindow()
	refs := m.pr.filteredRefs(m.pr.focus)
	inner := max(1, m.prModalWidth()-2-4)
	if len(refs) == 0 {
		if strings.TrimSpace(p.filter.Value()) != "" {
			return []string{styleDim.Render("  no matches — enter uses the typed ref")}
		}
		return []string{styleDim.Render("  no branches")}
	}
	start, end, more := pickerWindow(len(refs), p.cursor, rows)
	out := make([]string, 0, rows)
	for i := start; i < end; i++ {
		out = append(out, prPickerRow(refs[i].Name, i == p.cursor, inner))
	}
	if more {
		out = append(out, styleHint.Render(fmt.Sprintf("  … %d more", len(refs)-end)))
	}
	return out
}

// The ref name is untrusted text: it is sanitised before painting and truncated to the row's width, never wrapped.
func prPickerRow(name string, selected bool, width int) string {
	mark := "  "
	if selected {
		mark = styleCursor.Render("▸") + " "
	}
	return "  " + mark + truncate(sanitizeLogText(name), max(1, width))
}

func (m Model) filterLine(p *prPicker) string {
	return "  " + p.filter.View()
}

// The body shares the label column (focus marker included) but has no value: it is a multiline block and what marks where it starts is its own ┃ prompt.
func (m Model) prBodyLabel() string {
	return strings.TrimRight(m.prFieldLine("body", m.pr.focus == prFieldBody, ""), " ")
}

// Reserved ALWAYS, even without an error, so the textarea does not change height when a warning appears: a field jumping under the cursor while typing is worse than an error you have to read twice.
func (m Model) prNoticeLine() string {
	if m.pr.err == "" {
		return ""
	}
	return "  " + styleError.Render(m.pr.err)
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

// The opening key comes from the config (KeyFor) and not from a package constant: after a rebind of `pr` a hand-written warning would leave the user reading a key that no longer opens anything.
func (m Model) prPrompt() string {
	if m.pr == nil {
		return ""
	}
	return fmt.Sprintf("new PR %s: %s → %s · %s opens · tab field · %s create · %s back",
		m.pr.name, m.prFieldDisplay(prFieldHead), m.prFieldDisplay(prFieldBase), m.cfg.KeyFor("pr"), prSubmitKey, "esc")
}

// Exactly n lines: the pane reserves its height so the modal never resizes between the fields.
func fitList(lines []string, n int) []string {
	return rellenaHasta(lines[:min(len(lines), n)], n)
}

func blankLines(n int) []string {
	return rellenaHasta(nil, n)
}
