package tui

import (
	"strings"
	"testing"
)

func newPullModel(t *testing.T) Model {
	t.Helper()
	projects, states := fixtureProjects()
	return newTestModel(t, projects, states)
}

// Rows are sorted attention-first, so the cursor position is NOT the fixture's: without this the tests would point at the wrong repo without erroring.
func cursorOn(t *testing.T, m Model, path string) Model {
	t.Helper()
	for i, e := range m.entries() {
		// Worktree sub-rows are located by the worktree path and not by their parent, because it is the sub-row that says which repo the action applies to.
		if e.kind == kindRepo && e.r.project.Path == path {
			m.cursor = i
			return m
		}
		if e.kind == kindWorktree && e.wt.Path == path {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("the fixture has no row %s", path)
	return m
}

func TestPullArmsTheSelectorWithoutRun(t *testing.T) {
	m := newPullModel(t)
	before := len(m.running)
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("p did not arm the selector")
	}
	if len(m.running) != before {
		t.Errorf("p launched an action: running = %v, want no changes", m.running)
	}
}

func TestSelectorDispatchesEachVariant(t *testing.T) {
	for key, kind := range map[string]string{
		"p": "pull",
		"r": "pull_rebase",
		"f": "pull_ff",
		"m": "pull_merge",
	} {
		m := newPullModel(t)
		m = cursorOn(t, m, "/tmp/old-clean")
		m, _ = press(m, "p")
		m, _ = press(m, key)
		if m.pullArmed != nil {
			t.Errorf("p%s left the selector armed", key)
		}
		if m.running["/tmp/old-clean"] != kind {
			t.Errorf("p%s → running = %q, want %q", key, m.running["/tmp/old-clean"], kind)
		}
	}
}

// A key that is not a variant cancels the selector and continues on its normal course: if it were consumed, the app would get stuck waiting for a second keystroke that never comes.
func TestSelectorKeyNotVariantCancelsAndFollows(t *testing.T) {
	m := newPullModel(t)
	start := m.cursor
	m, _ = press(m, "p")
	if m.pullArmed == nil {
		t.Fatal("precondition: the selector was not armed")
	}
	m, _ = press(m, "j") // not a variant: it cancels and moves the cursor
	if m.pullArmed != nil {
		t.Error("the selector stays armed after a non-variant key")
	}
	if m.cursor == start {
		t.Error("the non-variant key did not run its own action (cursor still)")
	}
	if len(m.running) != 0 {
		t.Errorf("the cancel launched an action: %v", m.running)
	}
}

// With the selector armed, `f` is fetch and `r` is rescan, which is why the armed state has to consume the key before the normal routing.
func TestSelectorNotFiresActionsOfTheTable(t *testing.T) {
	for _, key := range []string{"f", "r"} {
		m := newPullModel(t)
		m, _ = press(m, "p")
		m, _ = press(m, key)
		if m.fetchStates["/tmp/old-clean"] == "fetching" {
			t.Errorf("p%s fired a fetch: the selector did not consume the key", key)
		}
		if m.scanning {
			t.Errorf("p%s fired a rescan: the selector did not consume the key", key)
		}
	}
}

func TestSelectorNotArmsWithoutRepo(t *testing.T) {
	m := newPullModel(t)
	idx := -1
	for i, e := range m.entries() {
		if e.kind == kindRepo && !e.r.project.HasRepo {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("the fixture has no row without a repo")
	}
	m.cursor = idx
	m, _ = press(m, "p")
	if m.pullArmed != nil {
		t.Errorf("the selector armed on a row without a repo: %+v", m.pullArmed)
	}
	if len(m.running) != 0 {
		t.Errorf("it launched an action on a row without a repo: %v", m.running)
	}
}

func TestSelectorPromptAnnouncesTheFourVariants(t *testing.T) {
	m := newPullModel(t)
	if got := m.pullPrompt(); got != "" {
		t.Errorf("pullPrompt with no selector = %q, want empty", got)
	}
	m = cursorOn(t, m, "/tmp/old-clean")
	m, _ = press(m, "p")
	got := m.pullPrompt()
	for _, want := range []string{"p default", "r rebase", "f ff-only", "m merge", "esc cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not mention %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "old-clean") {
		t.Errorf("the prompt does not name the target repo:\n%s", got)
	}
}

// The prompt is painted in the keybinds section and not in a toast: a toast expires after 3 s and the selector lives until the next keystroke.
func TestSelectorPromptVisibleInKeybinds(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "p")
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "ff-only") {
		t.Errorf("the armed selector is not announced in the dashboard:\n%s", out)
	}
	if banner := sectionContent(t, out, "gitdash"); strings.Contains(banner, "ff-only") {
		t.Errorf("the prompt leaked into the stats section:\n%s", banner)
	}
	if kb := sectionContent(t, out, "keybinds"); !strings.Contains(kb, "ff-only") {
		t.Errorf("the prompt is not in the keybinds section:\n%s", kb)
	}
}

func TestSelectorEscCancels(t *testing.T) {
	m := newPullModel(t)
	m, _ = press(m, "p")
	m, _ = press(m, "esc")
	if m.pullArmed != nil {
		t.Error("esc did not cancel the selector")
	}
	if len(m.running) != 0 {
		t.Errorf("esc launched an action: %v", m.running)
	}
}

func TestDetailShowsTheCommandResolved(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	updated, _ := m.Update(actionMsg{
		path: "/tmp/old-clean", kind: "pull_rebase",
		cmd:    "git pull --rebase --autostash",
		output: "Rebase aplicado",
	})
	m = updated.(Model)
	out := stripANSI(m.View().Content)
	if !strings.Contains(out, "git pull --rebase --autostash") {
		t.Errorf("the detail does not show the executed command:\n%s", out)
	}
	if !strings.Contains(out, "last rebase") {
		t.Errorf("the detail does not name the variant:\n%s", out)
	}
}

func TestDetailPushShowsCommand(t *testing.T) {
	m := newPullModel(t)
	m = cursorOn(t, m, "/tmp/old-clean")
	updated, _ := m.Update(actionMsg{
		path: "/tmp/old-clean", kind: "push", cmd: "git push", output: "",
	})
	m = updated.(Model)
	if out := stripANSI(m.View().Content); !strings.Contains(out, "git push") {
		t.Errorf("the detail does not show the executed push:\n%s", out)
	}
}
