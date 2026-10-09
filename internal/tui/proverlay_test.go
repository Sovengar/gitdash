package tui

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitdash/internal/cmdlog"
	"gitdash/internal/discovery"
	"gitdash/internal/gitstatus"
	"gitdash/internal/testutil"
)

func renameBranchInitial(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "branch", "-m", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch -m %s in %s: %v\n%s", branch, dir, err, out)
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

func modalContent(t *testing.T, m Model) string {
	t.Helper()
	if m.pr == nil {
		t.Fatalf("the overlay is not open")
	}
	return stripANSI(m.prModal())
}

func loadRefs(m Model, refs ...gitstatus.Ref) Model {
	m.pr.refs = refs
	m.pr.refsReady = true
	m.pr.refsErr = ""
	return m
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
	if got := m.pr.headValue(); got != "main" {
		t.Errorf("head = %q, want main (the snapshot's branch)", got)
	}
	if got := m.prParams().Base; got != "main" {
		t.Errorf("base = %q, want main (sync branch or config default)", got)
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
		t.Errorf("notice = %v, want notifyMsg", cmd())
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
	if !strings.Contains(modalContent(t, m), m.pr.err) {
		t.Errorf("the notice is not in the panel:\n%s", stripANSI(m.View().Content))
	}
	if len(m.toasts.blocksFor(m.width)) != 0 {
		t.Error("the validation error was emitted as a toast")
	}
}

func TestPROverlayBaseEmptyNotCloses(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "a title")
	m = loadRefs(m, gitstatus.Ref{Name: "main"})
	m.pr.base.value = "" // a picker normally carries a value; the submit still validates

	m, _ = press(m, prSubmitKey)

	if m.pr == nil {
		t.Fatal("the overlay closed with no base")
	}
	if m.prPending != nil {
		t.Error("a submit with no base was published")
	}
	if !strings.Contains(m.pr.err, "base") {
		t.Errorf("notice = %q, want mentions the base", m.pr.err)
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

// The box is located by its text without ANSI and the RAW lines are read, because that is where the style lives: the focused label and the unfocused one only differ in color. It reads the modal directly (not the spliced view), since the dashboard behind shares its border glyphs.
func formLines(t *testing.T, m Model) []string {
	t.Helper()
	if m.pr == nil {
		t.Fatalf("the form's box is not open")
	}
	return strings.Split(m.prModal(), "\n")
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

// Walking the five fields with tab fixes that the focused label is its own: an inverted `focus == prFieldX` would mark the wrong one and leave the right one unmarked.
func TestPROverlayHighlightsTheLabelOfTheFieldWithTheFocus(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	for _, want := range []prField{prFieldTitle, prFieldBase, prFieldHead, prFieldDraft, prFieldBody} {
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
	case prFieldHead:
		return "head"
	case prFieldDraft:
		return "draft"
	case prFieldBody:
		return "body"
	}
	return ""
}

func TestPROverlayTabWalksTheFields(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	want := []prField{prFieldTitle, prFieldBase, prFieldHead, prFieldDraft, prFieldBody}

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

// Leaving a branch field clears its transient filter: re-entering starts from the committed value and the full list.
func TestPROverlayLeavingABranchFieldClearsTheFilter(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m, gitstatus.Ref{Name: "main"}, gitstatus.Ref{Name: "dev"})
	m = focusField(t, m, prFieldBase)
	m = typeText(m, "dev")
	if m.pr.base.filter.Value() != "dev" {
		t.Fatalf("precondition: filter = %q", m.pr.base.filter.Value())
	}

	m, _ = press(m, "tab") // base -> head

	if m.pr.base.filter.Value() != "" {
		t.Errorf("the base filter survived the tab: %q", m.pr.base.filter.Value())
	}
}

func TestPROverlayMarksTheFieldWithTheFocus(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldDraft)

	content := modalContent(t, m)
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

	m = typeText(m, "first")
	m, _ = press(m, "enter")
	m = typeText(m, "second")

	if m.pr == nil {
		t.Fatal("enter in the body submitted the form")
	}
	if got := m.prParams().Body; got != "first\nsecond" {
		t.Errorf("Body = %q, want two lines", got)
	}
}

// The template key inserts a non-destructive skeleton at the cursor: what is already written stays.
func TestPROverlayBodyTemplateInsertsAtTheCursor(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldBody)
	m = typeText(m, "before")

	m, _ = press(m, "ctrl+t")

	body := m.prParams().Body
	if !strings.HasPrefix(body, "before") {
		t.Errorf("the skeleton overwrote the text: %q", body)
	}
	for _, want := range []string{"## Summary", "## Test plan"} {
		if !strings.Contains(body, want) {
			t.Errorf("the skeleton misses %q:\n%s", want, body)
		}
	}
}

// The template key only acts in the body; in another field it falls through to the widget without inserting anything.
func TestPROverlayBodyTemplateOnlyInTheBody(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "title text")

	m, _ = press(m, "ctrl+t")

	if m.prParams().Body != "" {
		t.Errorf("the template fired outside the body: %q", m.prParams().Body)
	}
	if got := m.prParams().Title; got != "title text" {
		t.Errorf("the title changed: %q", got)
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

	m, _ = press(m, prSubmitKey)

	p := m.prPending.params
	if p.Title != "a title" {
		t.Errorf("Title = %q, want clipped", p.Title)
	}
	if p.Base != "main" {
		t.Errorf("Base = %q, want clipped", p.Base)
	}
}

func TestPROverlayEscClosesWithoutEffects(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = typeText(m, "cleared")
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
	m = typeText(m, "cleared")
	m = focusField(t, m, prFieldDraft)
	m, _ = press(m, " ")
	m = focusField(t, m, prFieldBase)
	m.pr.base.value = ""
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
		t.Errorf("selectors stayed armed: %v %v", m.pullArmed, m.visualArmed)
	}
}

func TestPROverlayCloseDropsTheFocusOfTheFields(t *testing.T) {
	for _, c := range []struct {
		name  string
		focus prField
	}{
		{"the title", prFieldTitle},
		{"the base", prFieldBase},
		{"the head", prFieldHead},
		{"the body", prFieldBody},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := focusField(t, openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api")), c.focus)
			// A pointer to the widget and not its value: m.pr becomes nil on close and what matters is the widget Blur was called on; the close reads the widget THROUGH the pointer instead of through a method value, since `w.Focused` would bind to a copy and always answer the same.
			var conFoco func() bool
			switch c.focus {
			case prFieldTitle:
				w := &m.pr.title
				conFoco = func() bool { return w.Focused() }
			case prFieldBase:
				w := &m.pr.base.filter
				conFoco = func() bool { return w.Focused() }
			case prFieldHead:
				w := &m.pr.head.filter
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

// The overlay floats over the dashboard and the whole still measures EXACTLY what the terminal does: a height that does not add up means something is off-screen.
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
	if !formPainted(t, m) {
		t.Fatalf("the open overlay is not painted:\n%s", stripANSI(out))
	}
	// The dashboard stays painted behind the modal: the repos box's title is above the modal's top edge.
	if !strings.Contains(stripANSI(out), "╭ repos ") {
		t.Errorf("the table is not painted behind the modal:\n%s", stripANSI(out))
	}
}

// The modal floats: the dashboard is rendered normally and keeps its panels (the form does not replace the body any more).
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

	// Below the minimum the resize closes the form and the hints come back (the warning no longer belongs on screen).
	m = resize(m, m.width, 12)
	kb = sectionContent(t, stripANSI(m.View().Content), "keybinds")
	if strings.Contains(kb, "ctrl+s") {
		t.Errorf("the PR legend survived the close:\n%s", kb)
	}
}

func TestPROverlayNotOpensInTerminalSmall(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.height = prMinModal() - 1

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

// A NARROW but tall terminal is the case the height alone does not see: without the minimum width the label column subtraction drives the input width negative.
func TestPROverlayNotOpensInTerminalNarrow(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.width, m.height = prMinWidth()-1, 40

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

// The height is an edge and not an approximation: the first height that opens is the modal's exact minimum, and one line less does not open.
func TestPROverlayOpensFairInTheHeightMinimum(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")

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
	if opens != prMinModal() {
		t.Errorf("the first height that opens = %d, want the exact minimum %d", opens, prMinModal())
	}

	atEdge := m
	atEdge.height = opens
	atEdge, _ = press(atEdge, "O")
	if atEdge.pr == nil {
		t.Fatalf("it did not open at %d lines", opens)
	}
	if got := atEdge.prInterior(); got != prMinInterior() {
		t.Errorf("it opens at %d lines with an interior of %d, want the exact minimum %d", opens, got, prMinInterior())
	}

	smaller := m
	smaller.height = opens - 1
	smaller, _ = press(smaller, "O")
	if smaller.pr != nil {
		t.Errorf("it opened at %d lines, one below the minimum (%d)", opens-1, prMinModal())
	}
}

// The modal has its own minimum height, derived from its parts; at it the box measures exactly prMinModal and one line below the form does not paint.
func TestPROverlayModalOnlyFromTheHeightMinimum(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m.height = prMinModal()
	if !m.prFits() {
		t.Fatalf("at %d lines the modal does not fit its own minimum %d", m.height, prMinModal())
	}
	if got := len(strings.Split(m.prModal(), "\n")); got != prMinModal() {
		t.Errorf("box height = %d, want %d", got, prMinModal())
	}
	m.height = prMinModal() - 1
	if m.prFits() {
		t.Errorf("at %d lines (one below %d) the modal still fits", m.height, prMinModal())
	}
}

// prFit's split is a view contract: the inputs take the label column and the margin off the modal's width, the body takes the modal's interior.
func TestPROverlayTheWidgetsIsSizeOnTheGap(t *testing.T) {
	const width, height = 100, 40
	const prBodyPrompt = 2

	m := openPROverlay(t, resize(newPROverlayModel(t, "/tmp/dirty-api"), width, height))
	inner := m.prModalWidth() - 2

	if got := m.pr.body.Width(); got != inner-prBodyPrompt {
		t.Errorf("body width = %d, want %d (the modal's interior minus the prompt)",
			got, inner-prBodyPrompt)
	}
	if got, want := m.pr.title.Width(), m.prValueWidth(); got != want {
		t.Errorf("title width = %d, want %d", got, want)
	}
	if got, want := m.pr.base.filter.Width(), max(1, inner-4); got != want {
		t.Errorf("base filter width = %d, want %d", got, want)
	}
	if got, want := m.pr.head.filter.Width(), max(1, inner-4); got != want {
		t.Errorf("head filter width = %d, want %d", got, want)
	}
	if got, want := m.pr.body.Height(), m.prBodyHeight(); got != want {
		t.Errorf("body height = %d, want %d (the interior the fixed lines and the pane leave)", got, want)
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

// The real branch is in the parent repo's inventory; without that fallback the overlay would show an empty head and the PR would go out with -H omitted.
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
	if got := m.pr.headValue(); got != "feat/wt" {
		t.Errorf("head = %q, want %q (it comes from the inventory, not the empty snapshot)", got, "feat/wt")
	}
	if got := m.prParams().Head; got != "feat/wt" {
		t.Errorf("Params.Head = %q, want %q", got, "feat/wt")
	}
}

// Both axes have a minimum and the test holds them AT THE EDGE: one line/column less does not open and the exact minimum does.
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
		m := newPROverlayModel(t, "/tmp/dirty-api")
		m.width = 100

		m.height = prMinModal() - 1
		if open, _ := press(m, "O"); open.pr != nil {
			t.Errorf("height %d: the overlay opened and below that it does not fit", m.height)
		}
		m.height = prMinModal()
		open, _ := press(m, "O")
		if open.pr == nil {
			t.Fatalf("height %d: the overlay did not open", m.height)
		}
		if got := open.prInterior(); got != prMinInterior() {
			t.Errorf("height %d: interior = %d, want the exact minimum %d",
				m.height, got, prMinInterior())
		}
	})
}

func TestPROverlayTheMinimumOfHeightNotIsAFloorOfParty(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	m.width = 100
	m.height = prMinModal() - 2
	if open, _ := press(m, "O"); open.pr != nil {
		t.Errorf("height %d (two below the threshold %d): it opened", m.height, prMinModal())
	}
}

// The minimums are tested against the pieces they are made of, because that is the contract the comments write: a test comparing against the constant itself does not pin it (if the composition changed the test would move with it and the mutant would survive).
func TestPROverlayTheBudgetIsTheOneTheCommentStates(t *testing.T) {
	const fixed = 6 // four single-line fields + notice + body label
	if prFixedLines() != fixed {
		t.Errorf("prFixedLines() = %d, want %d", prFixedLines(), fixed)
	}
	if want := prFixedLines() + prPaneLines() + prBodyMinLines(); prMinInterior() != want {
		t.Errorf("prMinInterior() = %d, want %d (fixed + pane + body floor)", prMinInterior(), want)
	}
	if want := prMinInterior() + 2; prMinModal() != want {
		t.Errorf("prMinModal() = %d, want %d (interior + borders)", prMinModal(), want)
	}
	if want := prFixedLines() + prPaneLines() + prBodyPreferredLines(); prInteriorPreferred() != want {
		t.Errorf("prInteriorPreferred() = %d, want %d", prInteriorPreferred(), want)
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

// The modal width is pinned by LITERAL numbers: a test recomputing it from prModalWidth() would move with a margin mutant and leave it alive.
func TestPROverlayModalWidthLiteralMargin(t *testing.T) {
	m := newPROverlayModel(t, "/tmp/dirty-api")
	cases := []struct{ width, want int }{
		{100, 96}, // 100 - prModalMargin
		{200, 196},
		{30, 27}, // clamped up to prMinWidth
		{27, 27},
	}
	for _, c := range cases {
		m.width = c.width
		if got := m.prModalWidth(); got != c.want {
			t.Errorf("width %d: prModalWidth() = %d, want %d", c.width, got, c.want)
		}
	}
}

// The body height is pinned by LITERAL numbers: body.Height() == prBodyHeight() holds under a mutant of prBodyHeight's arithmetic and proves nothing.
func TestPROverlayBodyHeightLiteral(t *testing.T) {
	m := openPROverlay(t, resize(newPROverlayModel(t, "/tmp/dirty-api"), 100, 40))
	if got := m.prBodyHeight(); got != 6 {
		t.Errorf("prBodyHeight() at a tall terminal = %d, want 6 (the preferred body)", got)
	}
	m.height = prMinModal()
	if got := m.prBodyHeight(); got != 2 {
		t.Errorf("prBodyHeight() at the minimum = %d, want 2 (the body floor)", got)
	}
}

// A committed ref longer than the value column is cut in the field line (marker included), so the modal keeps its exact width.
func TestPROverlayFieldLineCutsAnOverlongCommittedRef(t *testing.T) {
	m := openPROverlay(t, resize(newPROverlayModel(t, "/tmp/dirty-api"), 100, 40))
	m.pr.head.value = strings.Repeat("x", 200)

	line := m.prFieldDisplay(prFieldHead)
	if w := ansi.StringWidth(line); w > m.prValueWidth() {
		t.Errorf("field value width = %d, want <= %d", w, m.prValueWidth())
	}
	if !strings.HasSuffix(line, "…") {
		t.Errorf("the overlong ref is not marked as cut: %q", line)
	}
	for i, l := range strings.Split(stripANSI(m.View().Content), "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d", i, w, m.width)
		}
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

// ---------- branch pickers ----------

func TestPROverlayBasePickerListsLocalAndRemoteAndHighlightsTheValue(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m,
		gitstatus.Ref{Name: "main"},
		gitstatus.Ref{Name: "dev"},
		gitstatus.Ref{Name: "origin/main", Remote: true},
	)
	m = focusField(t, m, prFieldBase)

	content := modalContent(t, m)
	for _, want := range []string{"main", "dev", "origin/main"} {
		if !strings.Contains(content, want) {
			t.Errorf("the pane does not list %q:\n%s", want, content)
		}
	}
	p := m.pr.picker(prFieldBase)
	refs := m.pr.filteredRefs(prFieldBase)
	if refs[p.cursor].Name != "main" {
		t.Errorf("the highlight is on %q, want the current base value main", refs[p.cursor].Name)
	}
}

func TestPROverlayPickerFiltersAsYouType(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m,
		gitstatus.Ref{Name: "main"},
		gitstatus.Ref{Name: "feature/login"},
		gitstatus.Ref{Name: "dev"},
		gitstatus.Ref{Name: "origin/dev", Remote: true},
	)
	m = focusField(t, m, prFieldBase)

	m = typeText(m, "dev")

	got := names(m.pr.filteredRefs(prFieldBase))
	want := []string{"dev", "origin/dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the filter kept %v, want %v", got, want)
	}
	if m.pr.picker(prFieldBase).cursor != 0 {
		t.Errorf("the highlight moved to %d, want the first match", m.pr.picker(prFieldBase).cursor)
	}
	if content := modalContent(t, m); strings.Contains(content, "feature/login") {
		t.Errorf("a non-matching branch is still listed:\n%s", content)
	}
	if m.pr.base.value != "main" {
		t.Errorf("typing changed the committed value: %q", m.pr.base.value)
	}
}

func TestPROverlayPickerArrowsAndEnterCommit(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m,
		gitstatus.Ref{Name: "main"},
		gitstatus.Ref{Name: "dev"},
		gitstatus.Ref{Name: "origin/dev", Remote: true},
	)
	m = focusField(t, m, prFieldBase)
	m = typeText(m, "dev") // [dev, origin/dev]

	m, _ = press(m, "down")
	m, _ = press(m, "enter")

	if got := m.pr.base.value; got != "origin/dev" {
		t.Errorf("the committed value = %q, want the highlighted origin/dev", got)
	}
	if got := m.pr.base.filter.Value(); got != "" {
		t.Errorf("the filter was not cleared: %q", got)
	}
	if got := names(m.pr.filteredRefs(prFieldBase)); len(got) != 3 {
		t.Errorf("the pane did not return to the full list: %v", got)
	}
	if m.pr.focus != prFieldHead {
		t.Errorf("enter did not advance to the next field: focus = %d, want %d", m.pr.focus, prFieldHead)
	}
	// And from head the next stop is the draft toggle, the field after it in the tab order.
	m, _ = press(m, "enter")
	if m.pr.focus != prFieldDraft {
		t.Errorf("enter from head did not reach the draft: focus = %d, want %d", m.pr.focus, prFieldDraft)
	}
}

func TestPROverlayPickerHighlightClamped(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m, gitstatus.Ref{Name: "main"}, gitstatus.Ref{Name: "dev"})
	m = focusField(t, m, prFieldBase)

	for range 5 {
		m, _ = press(m, "down")
	}
	if got := m.pr.picker(prFieldBase).cursor; got != 1 {
		t.Errorf("down past the end left the highlight at %d, want 1", got)
	}
	for range 5 {
		m, _ = press(m, "up")
	}
	if got := m.pr.picker(prFieldBase).cursor; got != 0 {
		t.Errorf("up past the start left the highlight at %d, want 0", got)
	}
}

func TestPROverlayPickerTypedRefUsedVerbatimOnNoMatch(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m, gitstatus.Ref{Name: "main"})
	m = focusField(t, m, prFieldBase)
	m = typeText(m, "deadbeef")

	m, _ = press(m, "enter")

	if got := m.pr.base.value; got != "deadbeef" {
		t.Errorf("value = %q, want the typed text used verbatim", got)
	}
	if m.pr.focus != prFieldHead {
		t.Errorf("the verbatim commit did not advance: focus = %d, want %d", m.pr.focus, prFieldHead)
	}
}

// Nothing to commit (the list is still loading and nothing is typed) leaves enter a no-op: the focus does not move on a commit that never happened.
func TestPROverlayPickerEnterWithoutACommitStays(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = focusField(t, m, prFieldBase)
	before := m.pr.base.value

	m, _ = press(m, "enter")

	if m.pr.focus != prFieldBase {
		t.Errorf("enter without a commit moved the focus: %d, want %d", m.pr.focus, prFieldBase)
	}
	if got := m.pr.base.value; got != before {
		t.Errorf("the value changed without a commit: %q, want %q", got, before)
	}
}

func TestPROverlayHeadPickerOffersLocalsOnly(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m,
		gitstatus.Ref{Name: "main"},
		gitstatus.Ref{Name: "dev"},
		gitstatus.Ref{Name: "origin/main", Remote: true},
	)
	m = focusField(t, m, prFieldHead)

	if got := names(m.pr.filteredRefs(prFieldHead)); !reflect.DeepEqual(got, []string{"main", "dev"}) {
		t.Errorf("head pane = %v, want locals only", got)
	}
	if content := modalContent(t, m); strings.Contains(content, "origin/") {
		t.Errorf("a remote name reaches the head pane:\n%s", content)
	}
}

func TestPROverlayPaneLoadingEmptyAndErrorStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		m := focusField(t, openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api")), prFieldBase)
		if !strings.Contains(modalContent(t, m), "loading branches") {
			t.Errorf("the loading state is not painted:\n%s", modalContent(t, m))
		}
	})
	t.Run("error keeps the value and the escape hatch", func(t *testing.T) {
		m := focusField(t, openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api")), prFieldBase)
		m.pr.refsReady = true
		m.pr.refsErr = "boom"
		if !strings.Contains(modalContent(t, m), "could not list branches") {
			t.Errorf("the error state is not painted:\n%s", modalContent(t, m))
		}
		if m.pr.base.value != "main" {
			t.Errorf("the read error dropped the field value: %q", m.pr.base.value)
		}
		m = typeText(m, "typed/ref")
		m, _ = press(m, "enter")
		if m.pr.base.value != "typed/ref" {
			t.Errorf("typing by hand after an error did not work: %q", m.pr.base.value)
		}
	})
	t.Run("no match", func(t *testing.T) {
		m := loadRefs(focusField(t, openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api")), prFieldBase),
			gitstatus.Ref{Name: "main"})
		m = typeText(m, "zzz")
		if !strings.Contains(modalContent(t, m), "no matches") {
			t.Errorf("the no-match state is not painted:\n%s", modalContent(t, m))
		}
	})
}

// The bounded read is one exec per form open, only for that repo, and a fresh one starts on reopen.
func TestPROverlayRefReadRunsPerOpenAndIsAuditable(t *testing.T) {
	dir, origin := testutil.NewRepo(t, true)
	testutil.PushUpstreamCommits(t, origin, 1, "up")
	testutil.FetchLocal(t, dir)
	p := proj(filepath.Base(dir), dir, true)
	m := newTestModel(t, []discovery.Project{p}, map[string]gitstatus.Snapshot{dir: snapClean()})
	m = cursorOn(t, m, dir)
	// The model installs its own recorder in New; the test's is installed after so it captures the read.
	rec := cmdlog.New(cmdlog.DefaultCap)
	cmdlog.SetRecorder(rec)
	t.Cleanup(func() { cmdlog.SetRecorder(nil) })

	m = openPROverlay(t, m)
	waitEvent(t, &m, func(ev event) bool { _, ok := ev.(prRefsMsg); return ok })
	if !m.pr.refsReady {
		t.Fatal("the ref read never landed in the form")
	}
	if len(m.pr.refs) == 0 {
		t.Fatal("the ref read returned no branches")
	}
	if got := countRefReads(rec); got != 1 {
		t.Fatalf("for-each-ref executions = %d, want 1", got)
	}
	if e := lastRefRead(rec); e.Class != cmdlog.ClassRead {
		t.Errorf("Class = %v, want %v", e.Class, cmdlog.ClassRead)
	}

	m, _ = press(m, "esc")
	m = openPROverlay(t, m)
	waitEvent(t, &m, func(ev event) bool { _, ok := ev.(prRefsMsg); return ok })

	if got := countRefReads(rec); got != 2 {
		t.Errorf("for-each-ref executions = %d, want 2 (a fresh read per open)", got)
	}
}

// A late answer is attributed to the repo it was read for: with no modal or with the form moved on, it is dropped.
func TestPROverlayRefsMsgAttributedToTheOpenForm(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	refs := []gitstatus.Ref{{Name: "dev"}}

	out, _ := m.Update(prRefsMsg{path: "/tmp/other", refs: refs})
	m = out.(Model)
	if m.pr.refsReady {
		t.Error("a result for another repo was accepted")
	}

	out, _ = m.Update(prRefsMsg{path: "/tmp/dirty-api", refs: refs})
	m = out.(Model)
	if !m.pr.refsReady || len(m.pr.refs) != 1 {
		t.Errorf("the result for the open repo was dropped: ready=%v refs=%v", m.pr.refsReady, m.pr.refs)
	}

	closed := newPROverlayModel(t, "/tmp/dirty-api")
	out, _ = closed.Update(prRefsMsg{path: "/tmp/dirty-api", refs: refs})
	if out.(Model).pr != nil {
		t.Error("a result arrived with no modal open")
	}
}

// Non-picker fields have no list and no value: the head/base helpers fall back cleanly (a `—` placeholder and an empty slice), never a nil dereference.
func TestPROverlayNonPickerFieldsHaveNoRefs(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))

	if got := m.pr.filteredRefs(prFieldTitle); got != nil {
		t.Errorf("filteredRefs(title) = %v, want nil", got)
	}
	if got := m.pr.fieldValue(prFieldTitle); got != "" {
		t.Errorf("fieldValue(title) = %q, want empty", got)
	}
	if got := m.prFieldDisplay(prFieldTitle); got != styleDim.Render("—") {
		t.Errorf("fieldDisplay(title) = %q, want the dim placeholder", got)
	}
	m.pr.base.value = ""
	if !strings.Contains(modalContent(t, m), "—") {
		t.Errorf("an empty committed value has no placeholder:\n%s", modalContent(t, m))
	}
}

// With the read done and no refs at all the pane says so instead of staying blank.
func TestPROverlayPaneSaysNoBranches(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m) // ready, no refs
	m = focusField(t, m, prFieldBase)

	if !strings.Contains(modalContent(t, m), "no branches") {
		t.Errorf("the empty list is not told:\n%s", modalContent(t, m))
	}
}

// More refs than the window show the "… N more" tail, so the pane height stays fixed. With exactly one
// more than the window the tail must appear: that pins prPickerWindow to prPaneLines()-1.
func TestPROverlayPickerRowsShowMore(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	var refs []gitstatus.Ref
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		refs = append(refs, gitstatus.Ref{Name: n})
	}
	m = loadRefs(m, refs...)
	m = focusField(t, m, prFieldBase)

	if !strings.Contains(modalContent(t, m), "more") {
		t.Errorf("the window does not warn about the dropped tail:\n%s", modalContent(t, m))
	}
}

// The tail carries the EXACT number dropped (len(refs)-end), so a mutant of that arithmetic changes the text.
func TestPROverlayPickerRowsExactMoreCount(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	var refs []gitstatus.Ref
	for _, n := range []string{"a", "b", "c", "d", "e"} { // 5 refs, window 4 -> 3 shown, 2 dropped
		refs = append(refs, gitstatus.Ref{Name: n})
	}
	m = loadRefs(m, refs...)
	m = focusField(t, m, prFieldBase)

	if got := modalContent(t, m); !strings.Contains(got, "… 2 more") {
		t.Errorf("the tail does not carry the exact dropped count:\n%s", got)
	}
}

// The pane marker sits ONLY on the highlighted row: the others are blank. An inverted index would move it.
func TestPROverlayPaneMarkerOnlyOnTheHighlightedRow(t *testing.T) {
	m := openPROverlay(t, newPROverlayModel(t, "/tmp/dirty-api"))
	m = loadRefs(m,
		gitstatus.Ref{Name: "main"},
		gitstatus.Ref{Name: "dev"},
		gitstatus.Ref{Name: "origin/main", Remote: true},
	)
	m = focusField(t, m, prFieldBase)

	rows := m.pickerRows(m.pr.picker(prFieldBase))
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	// base.value is "main", so the highlight is index 0.
	if !strings.Contains(rows[0], "▸") {
		t.Errorf("the highlighted row carries no marker: %q", stripANSI(rows[0]))
	}
	for _, i := range []int{1, 2} {
		if strings.Contains(rows[i], "▸") {
			t.Errorf("row %d wears the marker without the highlight: %q", i, stripANSI(rows[i]))
		}
	}
}

// A ref longer than the value column is cut in the pane with the marker, and the modal clips it to the box: a larger `inner` would let the row run past the border and the clip would eat the cut marker.
func TestPROverlayPaneCutsALongRefAndKeepsTheEllipsis(t *testing.T) {
	m := openPROverlay(t, resize(newPROverlayModel(t, "/tmp/dirty-api"), 100, 40))
	m = loadRefs(m, gitstatus.Ref{Name: strings.Repeat("x", 200)})
	m = focusField(t, m, prFieldBase)

	// The long ref is the only row carrying x's, so its painted line is found unambiguously (the body placeholder also has "…", and the field lines carry "▸").
	var paneLine string
	for _, l := range strings.Split(modalContent(t, m), "\n") {
		if strings.Contains(l, "xxx") {
			paneLine = l
			break
		}
	}
	if paneLine == "" {
		t.Fatalf("the pane row is not painted:\n%s", modalContent(t, m))
	}
	// The row must fill the box exactly: the cut marker flush against the right border. A larger inner lets the row overflow (the clip eats "…") and a smaller one leaves padding after it, so both are visible.
	if !strings.HasSuffix(paneLine, "…│") {
		t.Errorf("the pane row does not end cut-flush against the border (inner wrong): %q", paneLine)
	}
	for i, l := range strings.Split(stripANSI(m.View().Content), "\n") {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d width = %d, want %d", i, w, m.width)
		}
	}
}

func countRefReads(rec *cmdlog.Recorder) int {
	n := 0
	for _, e := range rec.Entries() {
		if strings.HasPrefix(e.Command(), "git for-each-ref") {
			n++
		}
	}
	return n
}

func lastRefRead(rec *cmdlog.Recorder) cmdlog.Entry {
	var got cmdlog.Entry
	for _, e := range rec.Entries() {
		if strings.HasPrefix(e.Command(), "git for-each-ref") {
			got = e
		}
	}
	return got
}
