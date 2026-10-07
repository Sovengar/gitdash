package tui

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func renameBranchInitial(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "branch", "-m", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch -m %s en %s: %v\n%s", branch, dir, err, out)
	}
}

func newPROverlayModel(t *testing.T, path string) Model {
	t.Helper()
	projects, states := fixtureProjects()
	m := newTestModel(t, projects, states)
	if path == "" {
		return m
	}
	return cursorOn(t, m, path)
}

func openPROverlay(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(m, "O")
	if m.pr == nil {
		t.Fatal("the PR key did not open the overlay")
	}
	return m
}

// Typing rune by rune is what a keyboard does and the only way the bubbles widgets get both Code AND Text.
func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, string(r))
	}
	return m
}

func resize(m Model, width, height int) Model {
	out, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return out.(Model)
}

// The box is looked up by its title with its border and not by the bare word: the overlay's keybinds prompt also says "new PR" when open, so the loose search would give a false positive with the overlay closed and would leave unchecked exactly what these tests want to check.
func formPainted(t *testing.T, m Model) bool {
	t.Helper()
	for _, l := range strings.Split(stripANSI(m.View().Content), "\n") {
		if strings.Contains(l, "╭ new PR · ") {
			return true
		}
	}
	return false
}

func focusField(t *testing.T, m Model, want prField) Model {
	t.Helper()
	for range prFieldCount {
		if m.pr.focus == want {
			return m
		}
		m, _ = press(m, "tab")
	}
	t.Fatalf("the focus did not reach %d (it stayed at %d)", want, m.pr.focus)
	return m
}

func TestPROverlayOpensTheForm(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "new PR · dirty-api") {
		t.Errorf("the form does not say which repo it opens for:\n%s", out)
	}
}

func TestPROverlayNotRunsNothing(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	before := len(m.running)

	m, _ = press(m, "O")
	m = typeText(m, "a title")
	m, _ = press(m, prSubmitKey)

	if got := len(m.running); got != before {
		t.Errorf("the overlay launched %d actions: the execution is T4's", got-before)
	}
	if m.prPending == nil {
		t.Fatal("the submit never reached prPending: T4 has nothing to run")
	}
}

func TestPROverlayCapturesTheRowOnTheArm(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	if m.pr.path != "/tmp/dirty-api" {
		t.Errorf("path = %q", m.pr.path)
	}
	if m.pr.name != "dirty-api" {
		t.Errorf("name = %q", m.pr.name)
	}
	if m.pr.head != "main" {
		t.Errorf("head = %q, want main (the snapshot's branch)", m.pr.head)
	}
	if got := m.prParams().Base; got != "main" {
		t.Errorf("base = %q, want main (sync branch o default de config)", got)
	}
}

func TestPROverlayPrefetchesTheSyncOfTheRepo(t *testing.T) {
	projects, states := fixtureProjects()
	s := states["/tmp/dirty-api"]
	s.SyncBranch = "develop"
	states["/tmp/dirty-api"] = s
	m := newTestModel(t, projects, states)
	m.cfg.SyncBranch = "main" // the global default, which must NOT win
	m = openPROverlay(t, cursorOn(t, m, "/tmp/dirty-api"))

	if got := m.prParams().Base; got != "develop" {
		t.Errorf("base = %q, want develop (the repo's sync branch)", got)
	}
	if out := stripANSI(m.View().Content); !strings.Contains(out, "develop") {
		t.Errorf("the prefilled base is not visible in the panel:\n%s", out)
	}
}

func TestPROverlayPrefetchesMasterInRepoWithoutMain(t *testing.T) {
	dir, _ := testutil.NewRepo(t, false)
	renameBranchInitial(t, dir, "master") // the repo no longer has main

	snap := gitstatus.Collect(t.Context(), dir, "main", true)
	if snap.SyncBranch != "master" {
		t.Fatalf("fixture: SyncBranch = %q, want master", snap.SyncBranch)
	}
	p := proj(filepath.Base(dir), dir, true)
	m := openPROverlay(t, newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snap}))

	if got := m.prParams().Base; got != "master" {
		t.Errorf("base = %q, want master (the repo's resolved ref)", got)
	}
	if out := stripANSI(m.View().Content); !strings.Contains(out, "master") {
		t.Errorf("the prefilled base is not visible in the panel:\n%s", out)
	}
}

func TestPROverlayWithoutRowNotOpens(t *testing.T) {
	projects := []discovery.Project{
		{Path: "/x", Name: "api", PrimaryGroup: "vsocial", HasRepo: true},
		{Path: "/y", Name: "cli", PrimaryGroup: "vsocial", HasRepo: true},
	}
	states := map[string]gitstatus.Snapshot{"/x": snapClean(), "/y": snapClean()}
	m := newTestModel(t, projects, states)
	header := -1
	for i, e := range m.entries() {
		if e.kind != kindRepo {
			header = i
			break
		}
	}
	if header < 0 {
		t.Fatal("the fixture has no group headers")
	}
	m.cursor = header

	_, cmd := press(m, "O")

	if cmd == nil {
		t.Fatal("with no row there was no notice")
	}
	if _, ok := cmd().(notifyMsg); !ok {
		t.Errorf("aviso = %v, want notifyMsg", cmd())
	}
	if m.pr != nil {
		t.Error("with no row the overlay opened")
	}
}

func TestPROverlayWithoutRepoNotOpens(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/no-repo-docs")

	_, cmd := press(m, "O")

	if cmd == nil {
		t.Fatal("with no repo there was no notice")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "no git repo") {
		t.Errorf("notice = %v, want no git repo", cmd())
	}
	if m.pr != nil {
		t.Error("with no repo the overlay opened")
	}
}

func TestPROverlayNotOpensAboutThePanelOfTheLog(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m, _ = press(m, "l") // opens the log panel

	_, cmd := press(m, "O")

	if m.pr != nil {
		t.Error("the overlay opened with the log panel open")
	}
	if cmd != nil {
		t.Errorf("the key launched something: %v", cmd())
	}
	if !strings.Contains(stripANSI(m.View().Content), "log") {
		t.Error("the log panel stopped being painted")
	}
}

func TestPROverlayCapturesTheKeyboardInteger(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	m = typeText(m, "pfl")

	if m.pullArmed != nil {
		t.Error("the p of the title armed the pull selector")
	}
	if m.logOpen {
		t.Error("the l of the title opened the log panel")
	}
	if len(m.running) != 0 {
		t.Errorf("the f of the title launched a fetch: %v", m.running)
	}
	if got := m.prParams().Title; got != "pfl" {
		t.Errorf("title = %q, want \"pfl\"", got)
	}
}

func TestPROverlayLeavesExitWithCtrlC(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	_, cmd := press(m, "ctrl+c")

	if cmd == nil {
		t.Fatal("ctrl+c produced no cmd inside the overlay")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c = %T, want tea.QuitMsg", cmd())
	}
}

func TestPROverlayTitleEmptyNotCloses(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	m, cmd := press(m, prSubmitKey)

	if m.pr == nil {
		t.Fatal("the overlay closed with no title")
	}
	if m.prPending != nil {
		t.Error("a submit with no title was published")
	}
	if cmd != nil {
		t.Errorf("the validation launched something: %v", cmd())
	}
	if m.pr.err == "" {
		t.Error("there is no validation notice")
	}
	if !strings.Contains(sectionContent(t, stripANSI(m.View().Content), "new PR · dirty-api"), m.pr.err) {
		t.Errorf("the notice is not in the panel:\n%s", stripANSI(m.View().Content))
	}
	if len(m.toasts.blocksFor(m.width)) != 0 {
		t.Error("the validation error was emitted as a toast")
	}
}

func TestPROverlayBaseEmptyNotCloses(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "a title")
	m = focusField(t, m, prFieldBase)
	for range 10 { // the prefilled base is cleared entirely
		m, _ = press(m, "backspace")
	}

	m, _ = press(m, prSubmitKey)

	if m.pr == nil {
		t.Fatal("the overlay closed with no base")
	}
	if m.prPending != nil {
		t.Error("a submit with no base was published")
	}
	if !strings.Contains(m.pr.err, "base") {
		t.Errorf("aviso = %q, want menciona la base", m.pr.err)
	}
}

func TestPROverlayTheWarningDisappearsOnTheWrite(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m, _ = press(m, prSubmitKey)
	if m.pr.err == "" {
		t.Fatal("with no title there is no notice")
	}

	m = typeText(m, "a")

	if m.pr.err != "" {
		t.Errorf("the notice stays after typing: %q", m.pr.err)
	}
}

func TestPROverlaySendValidClosesAndPublishes(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "Add the sync branch base")

	m, _ = press(m, prSubmitKey)

	if m.pr != nil {
		t.Error("the overlay stayed open after a valid submit")
	}
	if m.prPending == nil {
		t.Fatal("the submit was not published")
	}
	if m.prPending.path != "/tmp/dirty-api" {
		t.Errorf("path = %q", m.prPending.path)
	}
	if got := m.prPending.params.Title; got != "Add the sync branch base" {
		t.Errorf("Title = %q", got)
	}
	if formPainted(t, m) {
		t.Errorf("the form's panel is still painted:\n%s", stripANSI(m.View().Content))
	}
}

func TestPROverlayDraftToggle(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	if m.prParams().Draft {
		t.Error("draft starts enabled")
	}
	m = focusField(t, m, prFieldDraft)

	m, _ = press(m, " ")
	if !m.prParams().Draft {
		t.Error("space did not enable draft")
	}
	m, _ = press(m, "enter")
	if m.prParams().Draft {
		t.Error("enter did not disable draft")
	}
	m = focusField(t, m, prFieldTitle)
	m = typeText(m, " ")
	if m.pr.draft {
		t.Error("a space in the title enabled draft")
	}
	if got := m.pr.title.Value(); got != " " {
		t.Errorf("the title's input = %q, want a space", got)
	}
}

// The box is located by its text without ANSI and the RAW lines are read, because that is where the style lives: the focused label and the unfocused one only differ in color.
func formLines(t *testing.T, m Model) []string {
	t.Helper()
	all := strings.Split(m.View().Content, "\n")
	start := -1
	for i, l := range all {
		if strings.Contains(stripANSI(l), "╭ new PR · "+m.pr.name+" ") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("the form's box is not in the view:\n%s", stripANSI(m.View().Content))
	}
	for i := start; i < len(all); i++ {
		if strings.Contains(stripANSI(all[i]), "╯") {
			return all[start : i+1]
		}
	}
	t.Fatalf("the form's box does not close in the view:\n%s", stripANSI(m.View().Content))
	return nil
}

// The ANSI of the view is looked at and not the text, because labels ALWAYS carry the label text, so without the style this check would distinguish nothing.
func highlightedLabels(t *testing.T, m Model) []string {
	t.Helper()
	raw := formLines(t, m)
	var out []string
	for _, label := range []string{"title", "base", "head", "draft", "body"} {
		for _, line := range raw {
			if !strings.Contains(stripANSI(line), label) {
				continue
			}
			if strings.Contains(line, styleWarn.Bold(true).Render(pad(label, 7))) {
				out = append(out, label)
			}
			break
		}
	}
	return out
}

// Walking the four fields with tab fixes that the focused label is its own: an inverted `focus == prFieldX` would mark the wrong one and leave the right one unmarked.
func TestPROverlayHighlightsTheLabelOfTheFieldWithTheFocus(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	for _, want := range []prField{prFieldTitle, prFieldBase, prFieldDraft, prFieldBody} {
		m = focusField(t, m, want)
		if got := highlightedLabels(t, m); !reflect.DeepEqual(got, []string{prLabelFor(want)}) {
			t.Errorf("with the focus on %q it highlights %v, want only [%q]", prLabelFor(want), got, prLabelFor(want))
		}
	}
}

func prLabelFor(f prField) string {
	switch f {
	case prFieldTitle:
		return "title"
	case prFieldBase:
		return "base"
	case prFieldDraft:
		return "draft"
	case prFieldBody:
		return "body"
	}
	return "head"
}

func TestPROverlayTabWalksTheFields(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	want := []prField{prFieldTitle, prFieldBase, prFieldDraft, prFieldBody}

	for _, f := range want {
		if m.pr.focus != f {
			t.Fatalf("focus at %d after opening, want %d", m.pr.focus, f)
		}
		m, _ = press(m, "tab")
	}
	if m.pr.focus != prFieldTitle {
		t.Errorf("the focus did not go back to the start: %d", m.pr.focus)
	}
	m, _ = press(m, "shift+tab")
	if m.pr.focus != prFieldBody {
		t.Errorf("shift+tab did not reach the last: %d", m.pr.focus)
	}
}

func TestPROverlayMarksTheFieldWithTheFocus(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldDraft)

	content := sectionContent(t, stripANSI(m.View().Content), "new PR · dirty-api")
	if !strings.Contains(content, "▸ draft") {
		t.Errorf("the focused field is not marked:\n%s", content)
	}
	if strings.Contains(content, "▸ title") {
		t.Errorf("the focus appears in two fields:\n%s", content)
	}
}

func TestPROverlayBodyIsMultiline(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldBody)

	m = typeText(m, "primera")
	m, _ = press(m, "enter")
	m = typeText(m, "segunda")

	if m.pr == nil {
		t.Fatal("enter in the body submitted the form")
	}
	if got := m.prParams().Body; got != "primera\nsegunda" {
		t.Errorf("Body = %q, want two lines", got)
	}
}

// What is typed arrives WHOLE in the parameters (spaces, quotes, newlines, shell signs): a text escaped or normalized here would be a PR with a changed body and nothing failing.
func TestPROverlayTheParamsArriveWhole(t *testing.T) {
	title := `Add "sync branch" base $HOME & 'quotes'`
	// Not tab nor any other control character: the bubbles widgets expand them on insert (SetValue sanitizes, and the textarea cannot paint a tab without breaking its own column count); everything else has to arrive verbatim, because the body is what gh/glab will show in the PR.
	body := "## What changes\n\n- `git remote` with quotes: \"x\" and $VAR\n- $(whoami) and `id`\n- \"on the side\" and other things\n"
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, title)
	m = focusField(t, m, prFieldBody)
	m.pr.body.SetValue(body)
	m = focusField(t, m, prFieldDraft)
	m, _ = press(m, " ")
	m, _ = press(m, prSubmitKey)

	if m.prPending == nil {
		t.Fatal("the submit was not published")
	}
	p := m.prPending.params
	if p.Title != title {
		t.Errorf("Title = %q, want %q", p.Title, title)
	}
	if p.Body != body {
		t.Errorf("Body = %q, want %q", p.Body, body)
	}
	if p.Base != "main" {
		t.Errorf("Base = %q", p.Base)
	}
	if p.Head != "main" {
		t.Errorf("Head = %q, want the repo's branch", p.Head)
	}
	if !p.Draft {
		t.Error("Draft = false, want the toggle enabled")
	}
}

func TestPROverlayClipsTheTextOfALine(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "  a title  ")
	m = focusField(t, m, prFieldBase)
	m = typeText(m, "  ")

	m, _ = press(m, prSubmitKey)

	p := m.prPending.params
	if p.Title != "a title" {
		t.Errorf("Title = %q, want recortado", p.Title)
	}
	if p.Base != "main" {
		t.Errorf("Base = %q, want recortado", p.Base)
	}
}

func TestPROverlayEscClosesWithoutEffects(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "borrado")
	m, _ = press(m, "esc")

	if m.pr != nil {
		t.Error("esc did not close the overlay")
	}
	if m.prPending != nil {
		t.Error("esc published a submit")
	}
	if len(m.running) != 0 {
		t.Errorf("esc launched something: %v", m.running)
	}
	if m.promptLine() != "" {
		t.Errorf("esc left the keybinds notice: %q", m.promptLine())
	}
}

func TestPROverlayReopenStartsClean(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "borrado")
	m = focusField(t, m, prFieldDraft)
	m, _ = press(m, " ")
	m = focusField(t, m, prFieldBase)
	for range 10 { // emptying the base makes the submission FAIL
		m, _ = press(m, "backspace")
	}
	m, _ = press(m, prSubmitKey)
	if m.pr.err == "" {
		t.Fatal("with no base there is no notice")
	}
	if m.prPending != nil {
		t.Fatal("the invalid submit was published")
	}
	m, _ = press(m, "esc")

	m = openPROverlay(t, cursorOn(t, m, "/tmp/old-clean"))

	if got := m.prParams(); got.Title != "" || got.Body != "" || got.Draft {
		t.Errorf("the new form inherited state: %+v", got)
	}
	if m.pr.err != "" {
		t.Errorf("the new form inherited the notice: %q", m.pr.err)
	}
	if m.pr.focus != prFieldTitle {
		t.Errorf("the focus ended at %d", m.pr.focus)
	}
	if m.pr.path != "/tmp/old-clean" {
		t.Errorf("path = %q, want the new repo", m.pr.path)
	}
}

func TestPROverlayOpenDropsTheSelectorsArmed(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p did not arm the selector")
	}
	m, _ = press(m, "v")
	if m.visualArmed == nil {
		t.Fatal("v did not arm the visual selector")
	}

	m, _ = press(m, "O")

	if m.pullArmed != nil || m.visualArmed != nil {
		t.Errorf("quedaron selectores armados: %v %v", m.pullArmed, m.visualArmed)
	}
}

func TestPROverlayCloseDropsTheFocusOfTheFields(t *testing.T) {
	for _, c := range []struct {
		name string
		foco prField
	}{
		{"the title", prFieldTitle},
		{"la base", prFieldBase},
		{"the body", prFieldBody},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := focusField(t, openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api")), c.foco)
			// A pointer to the widget and not its value: m.pr becomes nil on close and what matters is the widget Blur was called on; the close reads the widget THROUGH the pointer instead of through a method value, since `w.Focused` would bind to a copy and always answer the same.
			var conFoco func() bool
			switch c.foco {
			case prFieldTitle:
				w := &m.pr.title
				conFoco = func() bool { return w.Focused() }
			case prFieldBase:
				w := &m.pr.baseIn
				conFoco = func() bool { return w.Focused() }
			default:
				w := &m.pr.body
				conFoco = func() bool { return w.Focused() }
			}
			if !conFoco() {
				t.Fatalf("precondition: %s did not have the focus", c.name)
			}

			m, _ = press(m, "esc")

			if m.pr != nil {
				t.Fatal("esc did not close the overlay")
			}
			if conFoco() {
				t.Errorf("%s still has the focus after closing", c.name)
			}
		})
	}
}

// The overlay is one more box of the dashboard and the whole still measures EXACTLY what the terminal does: a height that does not add up means something is off-screen.
func TestPROverlayIsFitsInTheTerminal(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	out := m.View().Content
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) != m.height {
		t.Fatalf("lines = %d, want %d", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d: %q", i, w, m.width, ansi.Strip(l))
		}
	}
	lay := m.layout()
	content := sectionContent(t, stripANSI(out), "new PR · dirty-api")
	if got := len(strings.Split(content, "\n")); got != lay.bodyLines {
		t.Errorf("box height = %d, want the budget %d", got, lay.bodyLines)
	}
	if strings.Contains(stripANSI(out), "NAME") {
		t.Errorf("the table is still painted with the overlay open:\n%s", stripANSI(out))
	}
	if strings.Contains(stripANSI(out), "upstream") {
		t.Errorf("the repo's card is still painted with the overlay open:\n%s", stripANSI(out))
	}
}

func TestPROverlayPromptInKeybinds(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	if m.promptLine() != "" {
		t.Fatalf("with no overlay there is a prompt: %q", m.promptLine())
	}
	m, _ = press(m, "O")

	if m.keybindsLines() != 1 {
		t.Errorf("keybindsLines = %d, want 1 (only the notice)", m.keybindsLines())
	}
	kb := sectionContent(t, stripANSI(m.View().Content), "keybinds")
	for _, want := range []string{"new PR", "dirty-api", "ctrl+s", "esc"} {
		if !strings.Contains(kb, want) {
			t.Errorf("the warning does not mention %q:\n%s", want, kb)
		}
	}
	if strings.Contains(kb, "j/k move") {
		t.Errorf("the hints are still there with the notice in place:\n%s", kb)
	}

	m.height = 6
	kb = sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if !strings.Contains(kb, "ctrl+s") {
		t.Errorf("the notice disappears on a short terminal:\n%s", kb)
	}
}

func TestPROverlayNotOpensInTerminalSmall(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.height = 12 // below prMinBodyLines() the body does not reach
	if m.layout().bodyLines >= prMinBodyLines() {
		t.Skipf("precondition: at %d lines the body does fit (%d)", m.height, m.layout().bodyLines)
	}

	_, cmd := press(m, "O")

	if m.pr != nil {
		t.Error("an overlay that does not fit opened")
	}
	if cmd == nil {
		t.Fatal("there was no notice that it does not fit")
	}
	if formPainted(t, m) {
		t.Errorf("an overlay that does not fit was painted:\n%s", stripANSI(m.View().Content))
	}
}

// A NARROW but tall terminal is the case the height alone does not see: the body height is plenty while the label column subtraction drives the input width negative, so without the minimum width the only thing between the user and an unusable box was prFit's max(1, …).
func TestPROverlayNotOpensInTerminalNarrow(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.width, m.height = prMinWidth()-1, 40
	if m.layout().bodyLines < prMinBodyLines() {
		t.Skipf("precondition: at %d lines the height does not reach (%d)", m.height, m.layout().bodyLines)
	}

	_, cmd := press(m, "O")

	if m.pr != nil {
		t.Errorf("an overlay of width %d opened with a minimum of %d", m.width, prMinWidth())
	}
	if cmd == nil {
		t.Fatal("there was no notice that it does not fit")
	}
	if nm, ok := cmd().(notifyMsg); !ok || !strings.Contains(nm.text, "too small") {
		t.Errorf("notice = %v, want the does-not-fit one", cmd())
	}
	if formPainted(t, m) {
		t.Errorf("an overlay that does not fit was painted:\n%s", stripANSI(m.View().Content))
	}
}

func TestPROverlayOpensInTheWidthMinimum(t *testing.T) {
	m := resize(newPROverlayModel(t, "/tmp/dirty-api"), prMinWidth(), 40)

	m, _ = press(m, "O")

	if m.pr == nil {
		t.Fatalf("it did not open at the minimum width (%d)", prMinWidth())
	}
	if got := m.pr.title.Width(); got != prMinValueWidth {
		t.Errorf("input width = %d, want %d (the minimum value column)", got, prMinValueWidth)
	}
	if got := m.prValueWidth(); got != prMinValueWidth {
		t.Errorf("prValueWidth = %d, want %d", got, prMinValueWidth)
	}
	if !formPainted(t, m) {
		t.Errorf("the open overlay is not painted:\n%s", stripANSI(m.View().Content))
	}
}

// The height is an edge and not an approximation: the first height that opens is the one leaving the body EXACTLY at the form's minimum, and one line less does not open, so a `>=` there would accept a line too few.
func TestPROverlayOpensFairInTheHeightMinimum(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")

	// The first height that opens is searched and not hardcoded, so a change in the layout's split shows up here instead of freezing a number that no longer means anything.
	var opens int
	for h := 8; h <= 60; h++ {
		probe := m
		probe.height = h
		probe, _ = press(probe, "O")
		if probe.pr != nil {
			opens = h
			break
		}
	}
	if opens == 0 {
		t.Fatal("the form opens at no height")
	}

	enBorde := m
	enBorde.height = opens
	enBorde, _ = press(enBorde, "O")
	if enBorde.pr == nil {
		t.Fatalf("it did not open at %d lines", opens)
	}
	if got := enBorde.layout().bodyLines; got != prMinBodyLines() {
		t.Errorf("it opens at %d lines with a body of %d, want the exact minimum %d", opens, got, prMinBodyLines())
	}

	menor := m
	menor.height = opens - 1
	menor, _ = press(menor, "O")
	if menor.pr != nil {
		t.Errorf("it opened at %d lines, one below the minimum (%d)", opens-1, prMinBodyLines())
	}
}

// prSection is the other side of the same edge: with the EXACT gap it paints and with one less it returns "" so the caller does not leave half a box; if layout and prSection disagreed on the minimum, the overlay would open with its box not drawn.
func TestPROverlayPrSectionOnlyFromTheHeightMinimum(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	if got := m.prSection(prMinBodyLines() - 1); got != "" {
		t.Errorf("with one line less it painted %d lines, want the empty section:\n%s",
			len(strings.Split(got, "\n")), got)
	}
	got := m.prSection(prMinBodyLines())
	if got == "" {
		t.Fatalf("with the minimum height it painted nothing:\n%s", stripANSI(m.View().Content))
	}
	if n := len(strings.Split(got, "\n")); n != prMinBodyLines()+2 {
		t.Errorf("box height = %d, want %d (2 borders + %d)", n, prMinBodyLines()+2, prMinBodyLines())
	}
	if !strings.Contains(stripANSI(got), "dirty-api") {
		t.Errorf("the minimum-height section does not say which repo it is for:\n%s", stripANSI(got))
	}
}

// prFit's split is a view contract: the inputs take what is left after the label column and the margin cell, the body takes ALL the interior (its ┃ prompt marks the left edge) plus the height the fixed lines leave.
func TestPROverlayTheWidgetsIsSizeOnTheGap(t *testing.T) {
	const width, height = 100, 40
	const prBodyPrompt = 2

	m := openPROverlay(t, resize(newPROverlayModel(t, "/tmp/dirty-api"), width, height))

	if got := m.pr.body.Width(); got != width-2-prBodyPrompt {
		t.Errorf("body width = %d, want %d (the box's whole interior, prompt included)",
			got, width-2-prBodyPrompt)
	}
	wantValor := width - 2 - prLabelWidth - prValueSlack
	for _, in := range []struct {
		name string
		got  int
	}{{"title", m.pr.title.Width()}, {"base", m.pr.baseIn.Width()}} {
		if in.got != wantValor {
			t.Errorf("width of the %s input = %d, want %d (the interior minus labels and margin)",
				in.name, in.got, wantValor)
		}
	}
	if got, want := m.pr.body.Height(), m.layout().bodyLines-prFixedLines; got != want {
		t.Errorf("body height = %d, want %d (what the %d fixed lines leave)", got, want, prFixedLines)
	}
}

func TestPROverlayResizeInsufficientCloses(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	m = resize(m, m.width, 12)

	if m.pr != nil {
		t.Error("the overlay stayed open with no room to paint")
	}
	if len(m.toasts.blocksFor(m.width)) == 0 {
		t.Error("there was no notice that it closed for lack of height")
	}
}

func TestPROverlayResizeEnoughNotCloses(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "still here")

	m = resize(m, 100, 40)

	if m.pr == nil {
		t.Fatal("the overlay closed on a resize that does admit it")
	}
	if got := m.prParams().Title; got != "still here" {
		t.Errorf("the resize lost the text: %q", got)
	}
	if !formPainted(t, m) {
		t.Errorf("the overlay is not painted after the resize:\n%s", stripANSI(m.View().Content))
	}
}

// The real branch is in the parent repo's inventory; without that fallback the overlay would show an empty head and the PR would go out with -H omitted, i.e. against whatever the CLI guesses the current branch is, and the field the user reads would not be the one being sent.
func TestPRHeadOfWorktreeWithoutMarkerComesOfTheInventory(t *testing.T) {
	projects, states := fixtureProjects()
	parent := projects[0].Path
	wtPath := "/tmp/gitdash-pr-wt"
	snap := states[parent]
	snap.Worktrees = append(snap.Worktrees, gitstatus.Worktree{
		Path: wtPath, Branch: "feat/wt", Head: "abc1234",
	})
	states[parent] = snap

	m := newTestModel(t, projects, states)
	m.expanded[parent] = true
	m = cursorOn(t, m, wtPath)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'O', Text: "O"})
	m = out.(Model)
	if m.pr == nil {
		t.Fatal("the overlay did not open over the worktree subrow")
	}
	if got := m.pr.head; got != "feat/wt" {
		t.Errorf("head = %q, want %q (it comes from the inventory, not the empty snapshot)", got, "feat/wt")
	}
	if got := m.prParams().Head; got != "feat/wt" {
		t.Errorf("Params.Head = %q, quiero %q", got, "feat/wt")
	}
}

// Both axes have a minimum and the test holds them AT THE EDGE: one line less does not open and one more does, so a mutant changing the 2 or the 12 shows up here and a `>=` in the place of a `>` does not.
func TestPROverlayOnlyOpensInTheBorderExact(t *testing.T) {
	t.Run("width", func(t *testing.T) {
		m := newPROverlayModel(t, "/tmp/dirty-api")
		m.height = 60

		m.width = prMinWidth() - 1
		m2, cmd := press(m, "O")
		if m2.pr != nil {
			t.Errorf("width %d (< minimum %d): the overlay opened", prMinWidth()-1, prMinWidth())
		}
		if cmd == nil {
			t.Error("an overlay that does not fit does not say why: the user presses and nothing happens")
		}
		if m2.prPending != nil {
			t.Error("an overlay that does not fit published a submit")
		}

		m.width = prMinWidth()
		m3, _ := press(m, "O")
		if m3.pr == nil {
			t.Errorf("width %d (the exact minimum): the overlay did not open", prMinWidth())
		}
	})

	t.Run("height", func(t *testing.T) {
		// The height threshold is not begged for: the first height at which the form fits is searched and the check confirms that just above it the body comes up short, so the test pins the minimum without writing a magic number.
		m := newPROverlayModel(t, "/tmp/dirty-api")
		m.width = 100

		var first int
		for h := 8; h < 40; h++ {
			m.height = h
			if open, _ := press(m, "O"); open.pr != nil {
				first = h
				break
			}
		}
		if first == 0 {
			t.Fatalf("the overlay opened at no height between 8 and 39")
		}

		m.height = first - 1
		if open, _ := press(m, "O"); open.pr != nil {
			t.Errorf("height %d: the overlay opened and below that it does not fit", first-1)
		}
		m.height = first
		open, _ := press(m, "O")
		if open.pr == nil {
			t.Fatalf("height %d: the overlay did not open", first)
		}
		if got := open.layout().bodyLines; got != prMinBodyLines() {
			t.Errorf("height %d: bodyLines = %d, want the exact minimum %d",
				first, got, prMinBodyLines())
		}
	})
}

func TestPROverlayTheMinimumOfHeightNotIsAFloorOfParty(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.width = 100
	var first int
	for h := 8; h < 40; h++ {
		m.height = h
		if open, _ := press(m, "O"); open.pr != nil {
			first = h
			break
		}
	}
	if first == 0 {
		t.Fatal("the overlay opened at no height between 8 and 39")
	}
	m.height = first - 2
	if open, _ := press(m, "O"); open.pr != nil {
		t.Errorf("height %d (two below the threshold %d): it opened",
			first-2, first)
	}
}

// The minimums are tested against the pieces they are made of, because that is the contract the comments write: a test comparing against the constant itself does not pin it (if prMinBodyLines became +3 the test would move with it and the mutant would survive).
func TestPROverlayTheBudgetIsTheOneTheCommentStates(t *testing.T) {
	const fieldsAndLabels = 6
	if prFixedLines != fieldsAndLabels {
		t.Errorf("prFixedLines = %d, want %d (4 fields + notice + body label)",
			prFixedLines, fieldsAndLabels)
	}
	if prMinBodyLines() != prFixedLines+2 {
		t.Errorf("prMinBodyLines() = %d, want %d (the %d fixed + 2 of body)",
			prMinBodyLines(), prFixedLines+2, prFixedLines)
	}
	if prMinValueWidth != prLabelWidth {
		t.Errorf("prMinValueWidth = %d, want %d (the label column's width)",
			prMinValueWidth, prLabelWidth)
	}
	if want := 2 + prLabelWidth + prValueSlack + prMinValueWidth; prMinWidth() != want {
		t.Errorf("prMinWidth() = %d, want %d (2 borders + labels + margin + minimum value)",
			prMinWidth(), want)
	}
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.width = prMinWidth()
	if got, want := m.prValueWidth(), prMinWidth()-2-prLabelWidth-prValueSlack; got != want {
		t.Errorf("prValueWidth at the minimum width = %d, want %d", got, want)
	}
	if got := m.prValueWidth(); got < 1 {
		t.Errorf("prValueWidth = %d: an input of width 0 or less is not an input", got)
	}
}

func TestPROverlayNotOpensWithTheLogOpen(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	if m.pr != nil {
		t.Fatal("the test model already has the overlay open")
	}
	m.logOpen = true
	mm, cmd := m.openPR()
	if mm.(Model).logOpen != true {
		t.Error("openPR with the log open closed the log")
	}
	if mm.(Model).pr != nil {
		t.Error("openPR opened the overlay with the log panel open")
	}
	if cmd != nil {
		t.Errorf("the action returned %#v with the log open, want nil", cmd)
	}
}

func TestDraftLabelDistinguishesOnAndOff(t *testing.T) {
	on := &prDraft{draft: true}
	if got := on.draftLabel(); got != "on" {
		t.Errorf("draftLabel with draft = %q, want on", got)
	}
	off := &prDraft{}
	if got := off.draftLabel(); got != "off" {
		t.Errorf("draftLabel without draft = %q, want off", got)
	}
}

func TestOverlayWithoutOpenReturnsValuesEmpty(t *testing.T) {
	m := newPROverlayModel(t, "")
	if m.pr != nil {
		t.Fatalf("the test model has the overlay open: %+v", m.pr)
	}
	if got := m.prPrompt(); got != "" {
		t.Errorf("prPrompt without overlay = %q, want empty", got)
	}
	params := m.prParams()
	if params.Title != "" || params.Body != "" || params.Base != "" {
		t.Errorf("prParams without overlay = %+v, want forge.Params' zero", params)
	}
}
